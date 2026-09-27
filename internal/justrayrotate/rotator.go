package justrayrotate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	DefaultProbeURL         = "https://api.telegram.org/"
)

// Probe checks that the local justray proxy reaches Telegram — the same
// criterion bots use to accept a proxy into their warm pool.
type Probe func(ctx context.Context) error

// NewTelProbe returns a probe that makes an HTTP GET through the given
// SOCKS5 proxy address. Any response (including 4xx) means the proxy reaches
// the target; only transport failures count as "down".
func NewTelProbe(proxyAddr, url string, timeout time.Duration) Probe {
	return func(ctx context.Context) error {
		client, err := proxyutil.NewHTTPProxyClient(proxyAddr)
		if err != nil {
			return err
		}
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
		resp.Body.Close()
		return nil
	}
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

// NewRotator builds a rotator over the given runner and picker.
func NewRotator(cfg RotatorConfig, runner Runner, picker *Picker) *Rotator {
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
	if cfg.Probe == nil {
		cfg.Probe = NewTelProbe("", DefaultProbeURL, DefaultProbeTimeout)
	}
	return &Rotator{cfg: cfg, runner: runner, picker: picker}
}

// Run blocks until ctx is cancelled, probing periodically and rotating the
// justray node on sustained failures.
func (r *Rotator) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.CheckInterval)
	defer ticker.Stop()

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

		if err := r.rotate(ctx); err != nil {
			log.Error().Err(err).Msg("Node rotation failed")
			continue
		}
		lastRotation = time.Now()
		rotations++
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
