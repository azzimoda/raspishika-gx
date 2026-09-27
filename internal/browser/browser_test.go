package browser

import (
	"context"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/spf13/viper"
)

// The interval used to be read as viper key "browser.restart_interval", which
// AutomaticEnv maps to BROWSER.RESTART_INTERVAL - a variable neither the env file
// nor the config package ever sets. The setting was silently ignored.
func TestRestartIntervalFromConfig(t *testing.T) {
	original := viper.Get(config.KeyBrowserRestartInterval)
	t.Cleanup(func() { viper.Set(config.KeyBrowserRestartInterval, original) })

	viper.Set(config.KeyBrowserRestartInterval, 2*time.Hour)
	if got := restartIntervalFromConfig(); got != 2*time.Hour {
		t.Fatalf("interval = %s, want 2h", got)
	}

	viper.Set(config.KeyBrowserRestartInterval, 0)
	if got := restartIntervalFromConfig(); got != defaultRestartInterval {
		t.Fatalf("unset interval = %s, want %s", got, defaultRestartInterval)
	}

	// Negative disables periodic restarts and must survive the fallback.
	viper.Set(config.KeyBrowserRestartInterval, -1)
	if got := restartIntervalFromConfig(); got != -1 {
		t.Fatalf("negative interval = %s, want -1 (restarter disabled)", got)
	}
}

// Close waits on restartDone, which only the restarter goroutine closes. With
// restarts disabled nothing closed it, so Close blocked forever and the bot could
// never shut down cleanly.
func TestCloseWithoutRestarterReturns(t *testing.T) {
	b := &ChromedpBrowser{
		parentContext: context.Background(),
		stopRestarter: make(chan struct{}),
		restartDone:   make(chan struct{}),
	}
	b.stopRestarterWithoutGoroutine()

	waitForClose(t, b)
}

// The same must hold when the restarter does run, otherwise Close hangs on a
// browser that is healthy.
func TestCloseWithRestarterReturns(t *testing.T) {
	b := &ChromedpBrowser{
		parentContext:   context.Background(),
		restartInterval: time.Hour,
		stopRestarter:   make(chan struct{}),
		restartDone:     make(chan struct{}),
	}
	go b.runRestarter()

	waitForClose(t, b)
}

func waitForClose(t *testing.T, b *ChromedpBrowser) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := b.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close blocked: restartDone is never closed without a running restarter")
	}
}

// Close must stay safe to call more than once.
func TestCloseIsIdempotent(t *testing.T) {
	b := &ChromedpBrowser{
		parentContext: context.Background(),
		stopRestarter: make(chan struct{}),
		restartDone:   make(chan struct{}),
	}
	b.stopRestarterWithoutGoroutine()

	waitForClose(t, b)
	waitForClose(t, b)
}

// A failed allocation must not leave a live context behind. Screenshots only
// rebuild when the context is nil or cancelled, and a context whose allocation
// failed is typically neither, which pinned every later screenshot to a dead
// browser and leaked the process and its temp dir.
func TestDiscardChromedpClearsFailedAllocation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := &ChromedpBrowser{chromedpCtx: ctx, chromedpCancel: cancel}
	if b.needsReinit() {
		t.Fatal("a live context must not need reinit")
	}

	b.discardChromedp()

	if b.chromedpCtx != nil || b.chromedpCancel != nil {
		t.Fatal("failed allocation left the context in place")
	}
	if !b.needsReinit() {
		t.Fatal("a discarded browser must need reinit")
	}
	if ctx.Err() == nil {
		t.Fatal("the underlying context was not cancelled, so the process would leak")
	}
}

// needsReinit must treat a nil context as unusable, not only a cancelled one.
func TestNeedsReinit(t *testing.T) {
	b := &ChromedpBrowser{}

	if !b.needsReinit() {
		t.Fatal("a browser that was never initialized must need reinit")
	}

	ctx, cancel := context.WithCancel(context.Background())
	b.chromedpCtx = ctx
	if b.needsReinit() {
		t.Fatal("a live context must not need reinit")
	}

	cancel()
	if !b.needsReinit() {
		t.Fatal("a cancelled context must need reinit")
	}
}

func TestScale(t *testing.T) {
	tests := []struct {
		width, height int
		scale         float64
		wantW, wantH  int
	}{
		{1920, 1080, 1.0, 1920, 1080},
		{1920, 1080, 0.5, 960, 540},
		{1920, 1080, 0, 0, 0},
		{0, 0, 1.0, 0, 0},
		{1000, 500, 1.5, 1500, 750},
	}
	for _, tt := range tests {
		w, h := scale(tt.width, tt.height, tt.scale)
		if w != tt.wantW || h != tt.wantH {
			t.Errorf("scale(%d, %d, %v) = (%d, %d), want (%d, %d)",
				tt.width, tt.height, tt.scale, w, h, tt.wantW, tt.wantH)
		}
	}
}
