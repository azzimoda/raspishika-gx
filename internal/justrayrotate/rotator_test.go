package justrayrotate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu      sync.Mutex
	subs    []Sub
	ups     []string
	downs   int
	upErr   error
	listErr error
}

func (f *fakeRunner) List(context.Context) ([]Sub, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subs, f.listErr
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

func (f *fakeRunner) upCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ups...)
}

func probeOK(context.Context) error   { return nil }
func probeFail(context.Context) error { return errors.New("down") }

func newTestRotator(runner *fakeRunner, probe Probe) *Rotator {
	cfg := RotatorConfig{
		Probe:            probe,
		CheckInterval:    time.Millisecond,
		FailureThreshold: 3,
		Cooldown:         time.Hour, // avoid extra rotations in fast tests
		MaxRotations:     3,
		LongBackoff:      time.Hour,
	}
	return NewRotator(cfg, runner, NewPicker(ruFlag, "Россия", "mobile operators"))
}

func subFixture() []Sub {
	subs, _ := ParseSubscriptions([]byte(fixture))
	return subs
}

func runFor(t *testing.T, ctx context.Context, r *Rotator, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		_ = r.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
	}
}

func TestRotatorNoRotateWhenProbeOK(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(runner, probeOK)

	runFor(t, ctx, r, 50*time.Millisecond)
	cancel()

	if calls := runner.upCalls(); len(calls) != 0 {
		t.Fatalf("unexpected rotations: %v", calls)
	}
}

func TestRotatorRotatesAfterThreshold(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{subs: subFixture()}
	// Cooldown short so the loop can rotate; threshold 1 for a fast test.
	r := newTestRotator(runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = time.Millisecond

	runFor(t, ctx, r, 100*time.Millisecond)
	cancel()

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 0 // rotate on every failed probe
	r.cfg.MaxRotations = 100

	runFor(t, ctx, r, 80*time.Millisecond)
	cancel()

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 0
	r.cfg.MaxRotations = 2
	r.cfg.LongBackoff = 100 * time.Millisecond

	runFor(t, ctx, r, 60*time.Millisecond)
	cancel()

	if calls := runner.upCalls(); len(calls) > 4 {
		t.Fatalf("expected backoff to pause rotations, got %v", calls)
	}
}

func TestRotatorNoEligibleNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := &fakeRunner{subs: subFixture()}
	r := newTestRotator(runner, probeFail)
	r.cfg.CheckInterval = time.Millisecond
	r.cfg.FailureThreshold = 1
	r.cfg.Cooldown = 0
	r.picker = NewPicker(ruFlag, "Россия", "Sweden", "Netherlands", "mobile operators")

	runFor(t, ctx, r, 30*time.Millisecond)
	cancel()

	if calls := runner.upCalls(); len(calls) != 0 {
		t.Fatalf("rotated with no eligible node: %v", calls)
	}
}
