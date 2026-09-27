package justrayrotate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/azzimoda/go-tg-proxy/proxyutil"
)

// Defaults for the rotator loop.
const (
	DefaultCheckInterval    = 20 * time.Second
	DefaultFailureThreshold = 3
	DefaultCooldown         = 60 * time.Second
	DefaultMaxRotations     = 3
	DefaultLongBackoff      = 10 * time.Minute
	DefaultProbeTimeout     = 10 * time.Second
	DefaultProbeURL         = "https://api.telegram.org/bot0/getMe"
)

// Probe checks that the local justray proxy reaches Telegram — the same
// criterion bots use to accept a proxy into their warm pool.
type Probe func(ctx context.Context) error

// NewTelProbe returns a probe that makes an HTTP GET through the given SOCKS5
// proxy address and reports whether the Bot API host answered.
//
// Redirects are not followed on purpose. The Bot API root replies 302 to
// core.telegram.org, so following it ended the probe on a different host: a node
// that could reach core.telegram.org while being blocked at the Bot API looked
// healthy and was never rotated, which is exactly the failure the rotator exists
// to catch. Staying on the configured host also keeps a captive portal or an
// intercepting proxy from passing as Telegram. The default URL is a Bot API
// method for the same reason - it is the endpoint the bot itself depends on.
//
// Any HTTP status counts as reachable, matching how the bot's proxy pool judges a
// proxy: it accepts the "not found" that getMe returns for a fake token. What is
// being tested is that the API host answered through the node, not the status.
//
// The client is built once and reused. Building it per probe left an
// http.Transport with no idle-connection timeout behind on every call, retaining
// a SOCKS5 and a TLS connection plus their goroutines for the process lifetime.
func NewTelProbe(proxyAddr, url string, timeout time.Duration) (Probe, error) {
	// proxy.SOCKS5 happily accepts an empty address and defers the failure to
	// dial time, which would turn a missing proxy setting into a probe that
	// always reports the node as dead.
	if strings.TrimSpace(proxyAddr) == "" {
		return nil, errors.New("probe requires a justray proxy address (JUSTRAY_PROXY_ADDR)")
	}
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("probe requires a target URL (JUSTRAY_PROBE_URL)")
	}
	client, err := proxyutil.NewHTTPProxyClient(proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("build probe client for %q: %w", proxyAddr, err)
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return func(ctx context.Context) error {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}, nil
}

// RotatorConfig configures the rotation loop.
type RotatorConfig struct {
	Probe            Probe
	CheckInterval    time.Duration
	FailureThreshold int
	Cooldown         time.Duration
	MaxRotations     int
	LongBackoff      time.Duration
}

// Rotator watches the local justray proxy and rotates the active node when the
// proxy stops reaching Telegram for a consecutive run of failed probes.
type Rotator struct {
	cfg    RotatorConfig
	runner Runner
	picker *Picker

	failures atomic.Int32
}

// NewRotator builds a rotator over the given runner and picker. A probe is
// mandatory: silently substituting one would have hidden a missing proxy address
// behind an endless stream of "node is down" rotations.
func NewRotator(cfg RotatorConfig, runner Runner, picker *Picker) (*Rotator, error) {
	if cfg.Probe == nil {
		return nil, errors.New("rotator requires a Probe (see NewTelProbe)")
	}
	if cfg.CheckInterval <= 0 {
		cfg.CheckInterval = DefaultCheckInterval
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = DefaultFailureThreshold
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = DefaultCooldown
	}
	if cfg.MaxRotations <= 0 {
		cfg.MaxRotations = DefaultMaxRotations
	}
	if cfg.LongBackoff <= 0 {
		cfg.LongBackoff = DefaultLongBackoff
	}
	return &Rotator{cfg: cfg, runner: runner, picker: picker}, nil
}

// Run blocks until ctx is cancelled, probing periodically and rotating the
// justray node on sustained failures.
func (r *Rotator) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.CheckInterval)
	defer ticker.Stop()

	// lastRotation paces attempts and rotations counts them since the last
	// successful probe. Both are reset on recovery, so a node that comes back -
	// or a probe that succeeds after an unrelated earlier incident - does not
	// leave the rotator stuck in the long backoff for the rest of its life.
	var lastRotation time.Time
	rotations := 0

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}

		if err := r.cfg.Probe(ctx); err == nil {
			r.failures.Store(0)
			lastRotation = time.Time{}
			rotations = 0
			continue
		}
		if r.failures.Add(1) < int32(r.cfg.FailureThreshold) {
			continue
		}

		// Sustained failure: rotate, but pace the attempts and back off
		// entirely after a streak of unusable nodes.
		if time.Since(lastRotation) < r.cfg.Cooldown {
			continue
		}
		if rotations >= r.cfg.MaxRotations && time.Since(lastRotation) < r.cfg.LongBackoff {
			log.Warn().Int("rotations", rotations).Msg("Too many consecutive rotations, backing off")
			continue
		}

		// Record the attempt before rotating, and count it whether or not it
		// worked. Advancing only on success meant a broken justray CLI was
		// retried on every tick forever, spawning several processes a minute
		// and logging an error each time.
		lastRotation = time.Now()
		rotations++
		if err := r.rotate(ctx); err != nil {
			log.Error().Err(err).Msg("Node rotation failed")
			continue
		}
		r.failures.Store(0)
	}
}

func (r *Rotator) rotate(ctx context.Context) error {
	subs, err := r.runner.List(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	nodes := Flatten(subs)

	next, ok := r.picker.Next(nodes, "")
	if !ok {
		return errors.New("no eligible node to rotate to")
	}

	log.Info().Str("node", next.Name).Str("id", next.ID).Msg("Rotating justray to node")

	if err := r.runner.Down(ctx); err != nil {
		log.Debug().Err(err).Msg("justray down (already disconnected?)")
	}
	if err := r.runner.Up(ctx, next.ID); err != nil {
		return fmt.Errorf("justray up %s: %w", next.ID, err)
	}
	return nil
}
