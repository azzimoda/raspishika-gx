package justrayrotate

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type fakeRunner struct {
	mu        sync.Mutex
	subs      []Sub
	ups       []string
	downs     int
	upErr     error
	listErr   error
	status    Status
	statusErr error
}

func (f *fakeRunner) List(context.Context) ([]Sub, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subs, f.listErr
}

func (f *fakeRunner) Status(context.Context) (Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, f.statusErr
}

func (f *fakeRunner) Up(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ups = append(f.ups, id)
	return f.upErr
}

func (f *fakeRunner) Down(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.downs++
	return nil
}

func (f *fakeRunner) upCallsLen() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ups)
}

func (f *fakeRunner) upCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ups...)
}

func probeOK(context.Context) error   { return nil }
func probeFail(context.Context) error { return errors.New("down") }

func newTestRotator(t *testing.T, runner *fakeRunner, probe Probe) *Rotator {
	t.Helper()
	cfg := RotatorConfig{
		Probe:            probe,
		CheckInterval:    time.Millisecond,
		FailureThreshold: 3,
		Cooldown:         time.Hour, // avoid extra rotations in fast tests
		MaxRotations:     3,
		LongBackoff:      time.Hour,
	}
	r, err := NewRotator(cfg, runner, NewPicker(ruFlag, "Россия", "mobile operators"))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func subFixture() []Sub {
	subs, _ := ParseSubscriptions([]byte(fixture))
	return subs
}

func runFor(t *testing.T, r *Rotator, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = r.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}

	// Cancel and wait for the loop to exit before returning. A loop left
	// running keeps logging after its test is over, which races any later test
	// that swaps the global logger to capture output.
	cancel()
	<-done
}

func TestRotatorNoRotateWhenProbeOK(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(t, runner, probeOK)

	runFor(t, r, 50*time.Millisecond)

	if calls := runner.upCalls(); len(calls) != 0 {
		t.Fatalf("unexpected rotations: %v", calls)
	}
}

func TestRotatorRotatesAfterThreshold(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()}
	// Cooldown short so the loop can rotate; threshold 1 for a fast test.
	r := newTestRotator(t, runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = time.Millisecond

	runFor(t, r, 100*time.Millisecond)

	calls := runner.upCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one rotation")
	}
	for _, id := range calls {
		if id == "" {
			t.Fatal("rotated to an empty node id")
		}
	}
}

func TestRotatorConsecutiveRotationsCycle(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(t, runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 0 // rotate on every failed probe
	r.cfg.MaxRotations = 100

	runFor(t, r, 80*time.Millisecond)

	calls := runner.upCalls()
	if len(calls) < 4 {
		t.Fatalf("expected a rotation cycle, got %v", calls)
	}
	// Round-robin must alternate across the two eligible nodes (se1, nl1),
	// never picking the RU or mobile-operator ones.
	for _, id := range calls {
		if id != "se1" && id != "nl1" {
			t.Fatalf("rotated to unexpected node %q", id)
		}
	}
	// Se1 and nl1 must appear interleaved, not stick to one node.
	for i := 0; i+1 < len(calls); i++ {
		if calls[i] == calls[i+1] {
			t.Fatalf("consecutive rotation to the same node: %v", calls)
		}
	}
}

func TestRotatorBacksOffAfterMaxRotations(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(t, runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 0
	r.cfg.MaxRotations = 2
	r.cfg.LongBackoff = 100 * time.Millisecond

	runFor(t, r, 60*time.Millisecond)

	if calls := runner.upCalls(); len(calls) > 4 {
		t.Fatalf("expected backoff to pause rotations, got %v", calls)
	}
}

func TestRotatorNoEligibleNode(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(t, runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 0
	r.picker = NewPicker(ruFlag, "Россия", "Sweden", "Netherlands", "mobile operators")

	runFor(t, r, 30*time.Millisecond)

	if calls := runner.upCalls(); len(calls) != 0 {
		t.Fatalf("rotated with no eligible node: %v", calls)
	}
}

// Advancing the pacing clock only after a successful rotation meant a broken
// justray CLI was retried on every single tick, spawning several processes a
// minute and logging an error each time. Failed attempts have to be paced too.
func TestRotatorPacesFailedRotations(t *testing.T) {
	runner := &fakeRunner{subs: subFixture(), upErr: errors.New("justray exploded")}
	r := newTestRotator(t, runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 50 * time.Millisecond
	r.cfg.MaxRotations = 100
	r.cfg.LongBackoff = time.Hour

	// ~200 ticks in 200ms. Without pacing this would attempt ~200 rotations.
	runFor(t, r, 200*time.Millisecond)

	calls := runner.upCalls()
	if len(calls) > 8 {
		t.Fatalf("failed rotations were not paced by the cooldown: %d attempts", len(calls))
	}
	if len(calls) < 2 {
		t.Fatalf("expected the cooldown to still allow retries, got %d attempts", len(calls))
	}
}

// A successful probe has to clear both the failure streak and the rotation
// history, otherwise one earlier incident leaves the daemon refusing to rotate
// for the rest of the long backoff window even after the proxy recovers.
func TestRotatorResetsBackoffAfterRecovery(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()}

	const (
		downBefore = 100 * time.Millisecond
		upFor      = 50 * time.Millisecond
		downAfter  = 100 * time.Millisecond
	)
	// A time-driven probe keeps this to a single Run: Run is not reentrant, and
	// the picker keeps mutable state across rotations.
	start := time.Now()
	var healthyAt atomic.Bool
	var running atomic.Int64
	r := newTestRotator(t, runner, probeFail)
	r.cfg.CheckInterval = 2 * time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = time.Millisecond
	r.cfg.MaxRotations = 2
	r.cfg.LongBackoff = time.Hour // back off hard, so only a reset can unblock us
	r.cfg.Probe = func(context.Context) error {
		elapsed := time.Since(start)
		if elapsed >= downBefore && elapsed < downBefore+upFor {
			if !healthyAt.Swap(true) {
				running.Store(int64(runner.upCallsLen()))
			}
			return nil
		}
		return errors.New("down")
	}

	runFor(t, r, downBefore+upFor+downAfter)

	// MaxRotations attempts, then the long backoff holds until the probe
	// recovers, then the budget is restored and the next outage is handled.
	if got := running.Load(); got != 2 {
		t.Fatalf("rotations before backoff = %d, want 2", got)
	}
	if got := runner.upCallsLen(); got != 4 {
		t.Fatalf("rotations after recovery = %d, want 4 (backoff was not reset)", got)
	}
}

// lockedBuffer collects log output while the rotator goroutine is still
// running, so the race detector stays quiet on the assertion side.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A rotation used to report only that it happened. A node that had gone bad,
// justray being down entirely, and a probe broken by an unresolvable proxy
// address all produced the same line, so the journal could not say why the
// rotator acted - which is the whole reason to keep a rotator running.
func TestRotatorLogsProbeCauseAndDisconnected(t *testing.T) {
	runner := &fakeRunner{subs: subFixture()} // zero Status: justray is down
	cause := errors.New("dial tcp 127.0.0.1:10808: connect: connection refused")
	r := newTestRotator(t, runner, func(context.Context) error { return cause })
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = time.Hour

	var logs lockedBuffer
	prevLogger := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = prevLogger })

	runFor(t, r, 100*time.Millisecond)

	out := logs.String()
	if !strings.Contains(out, "Rotating justray to node") {
		t.Fatalf("no rotation was logged; output: %s", out)
	}
	if !strings.Contains(out, "connection refused") {
		t.Fatalf("rotation line omits the probe cause; output: %s", out)
	}
	if !strings.Contains(out, `"disconnected":true`) {
		t.Fatalf("rotation line omits disconnected=true; output: %s", out)
	}
}

// A connected justray must not be reported as disconnected: that is the case
// where the active node answered but its traffic to Telegram broke, and it is
// the one the node matching exists to get right.
func TestRotatorDoesNotReportDisconnectedWhileConnected(t *testing.T) {
	runner := &fakeRunner{
		subs: subFixture(),
		status: Status{
			Connected: true,
			Node:      "Sweden",
			// Must match a node in the fixture, since the active node is
			// identified by server and port.
			Server: "govpoel.ggisopi.su",
			Port:   443,
		},
	}
	cause := errors.New("probe read: unexpected EOF")
	r := newTestRotator(t, runner, func(context.Context) error { return cause })
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = time.Hour

	var logs lockedBuffer
	prevLogger := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = prevLogger })

	runFor(t, r, 100*time.Millisecond)

	out := logs.String()
	if !strings.Contains(out, "Rotating justray to node") {
		t.Fatalf("no rotation was logged; output: %s", out)
	}
	if !strings.Contains(out, `"disconnected":false`) {
		t.Fatalf("connected justray reported as disconnected; output: %s", out)
	}
	// The active node is reported by ID, so this also proves the server and
	// port in the status were matched to the right node rather than to nothing.
	if !strings.Contains(out, `"previous":"se1"`) {
		t.Fatalf("rotation line omits the active node; output: %s", out)
	}
}
