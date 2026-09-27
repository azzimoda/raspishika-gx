package browser

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/sync/singleflight"
)

// defaultRestartInterval is used when BROWSER_RESTART_INTERVAL is unset. Chromium
// is restarted periodically because a long-lived instance slowly leaks and starts
// dropping screenshots.
const defaultRestartInterval = 24 * time.Hour

func New(ctx context.Context) (*ChromedpBrowser, error) {
	b := &ChromedpBrowser{
		parentContext:   ctx,
		restartInterval: restartIntervalFromConfig(),
		stopRestarter:   make(chan struct{}),
		restartDone:     make(chan struct{}),
	}

	if err := b.initializeBrowser(b.parentContext); err != nil {
		return nil, err
	}

	if b.restartInterval > 0 {
		go b.runRestarter()
	} else {
		b.stopRestarterWithoutGoroutine()
	}

	return b, nil
}

// stopRestarterWithoutGoroutine unblocks Close when no restarter was started:
// Close waits on restartDone, which only runRestarter closes.
func (b *ChromedpBrowser) stopRestarterWithoutGoroutine() {
	log.Debug().Msg("Periodic browser restarts are disabled")
	close(b.restartDone)
}

// restartIntervalFromConfig reads BROWSER_RESTART_INTERVAL.
//
// The lookup used the literal "browser.restart_interval", which never matched
// anything: viper.AutomaticEnv resolves that key to the env var
// BROWSER.RESTART_INTERVAL, while both the env file and the config package use
// BROWSER_RESTART_INTERVAL. The setting was therefore ignored and the fallback
// below always won.
//
// A negative value disables periodic restarts; only an unset/zero one falls back
// to the default.
func restartIntervalFromConfig() time.Duration {
	interval := viper.GetDuration(config.KeyBrowserRestartInterval)
	if interval == 0 {
		return defaultRestartInterval
	}
	return interval
}

type ChromedpBrowser struct {
	parentContext  context.Context
	chromedpCtx    context.Context
	chromedpCancel context.CancelFunc

	chromedpMu   sync.RWMutex
	restarterMu  sync.Mutex
	screenshotSF singleflight.Group

	restartInterval time.Duration
	stopRestarter   chan struct{}
	restartDone     chan struct{}
	restarting      bool
}

func (b *ChromedpBrowser) initializeBrowser(ctx context.Context) error {
	b.chromedpMu.Lock()
	defer b.chromedpMu.Unlock()
	return b.reinit(ctx)
}

// reinit (re)builds the chromedp exec allocator and context.
// Callers must hold b.chromedpMu.
func (b *ChromedpBrowser) reinit(ctx context.Context) error {
	// Tear down the previous browser (if any) so its process and temp dir
	// are cleaned up before we start a fresh one.
	if b.chromedpCancel != nil {
		b.chromedpCancel()
	}

	isHeadless := viper.GetBool(config.KeyBrowserHeadless)

	width, height := scale(
		viper.GetInt(config.KeyBrowserWidth),
		viper.GetInt(config.KeyBrowserHeight),
		viper.GetFloat64(config.KeyBrowserScale),
	)

	// NewExecAllocator only applies the options it is given, ignoring
	// DefaultExecAllocatorOptions. Merge the defaults in so container-critical
	// flags (--disable-dev-shm-usage, --disable-gpu, --no-first-run, ...) are
	// present. Otherwise a 64MB /dev/shm in Docker exhausts the shared memory
	// and crashes the browser mid-screenshot ("context canceled").
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", isHeadless),
		chromedp.WindowSize(width, height),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-gpu", true),
		// Chromium 129+ needs this to fall back to software GL when no GPU is
		// present (e.g. in a container), otherwise the GPU process crash-loops.
		chromedp.Flag("enable-unsafe-swiftshader", true),
		// Surface chromium's own stderr (crash reasons, GPU errors) in our logs.
		chromedp.CombinedOutput(chromiumOutputWriter{}),
	)

	ctx, cancelExecAllocator := chromedp.NewExecAllocator(ctx, opts...)
	ctx, cancelChromeDP := chromedp.NewContext(ctx,
		chromedp.WithBrowserOption(chromedp.WithDialTimeout(20*time.Second)),
		chromedp.WithLogf(func(format string, args ...any) {
			log.Trace().Msgf("chromedp: "+format, args...)
		}),
		chromedp.WithErrorf(func(format string, args ...any) {
			log.Error().Msgf("chromedp: "+format, args...)
		}),
	)

	b.chromedpCtx = ctx
	b.chromedpCancel = func() {
		log.Debug().Msg("Cancelling Chromedp context")
		cancelChromeDP()
		cancelExecAllocator()
	}

	// Allocate the browser on the persistent context. If the first Run happened
	// on a per-screenshot timeout context, chromedp would bind the browser
	// process to that context (exec.CommandContext) and kill it once the
	// screenshot finished and its context was cancelled, forcing a re-init on
	// the next call.
	if err := chromedp.Run(ctx); err != nil {
		// Nothing would ever cancel this context afterwards. Screenshots only
		// re-initialize when b.chromedpCtx is nil or already cancelled, and a
		// context whose allocation failed is typically not cancelled, so the
		// dead browser stayed "ready": every later screenshot failed against it
		// and the Chromium process and its temp dir leaked.
		b.discardChromedp()
		return fmt.Errorf("failed to allocate browser: %w", err)
	}

	b.restarterMu.Lock()
	b.restarting = false
	b.restarterMu.Unlock()

	return nil
}

// chromiumOutputWriter forwards chromium's combined stdout/stderr to the log.
type chromiumOutputWriter struct{}

func (chromiumOutputWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		log.Trace().Msg("chromium: " + string(p))
	}
	return len(p), nil
}

// discardChromedp tears down the current browser context and marks it absent, so
// the next screenshot builds a fresh one instead of reusing a dead context.
func (b *ChromedpBrowser) discardChromedp() {
	if b.chromedpCancel != nil {
		b.chromedpCancel()
	}
	b.chromedpCtx = nil
	b.chromedpCancel = nil
}

// needsReinit reports whether the browser has to be rebuilt before the next
// screenshot. A nil context counts as unusable: a failed allocation clears it,
// and reusing that state would mean screenshotting against a dead browser.
func (b *ChromedpBrowser) needsReinit() bool {
	return b.chromedpCtx == nil || b.chromedpCtx.Err() != nil
}

func (b *ChromedpBrowser) Close() error {
	select {
	case <-b.stopRestarter:
	default:
		close(b.stopRestarter)
	}
	<-b.restartDone

	// Close Chromedp
	b.chromedpMu.Lock()
	if b.chromedpCancel != nil {
		b.chromedpCancel()
	}
	b.chromedpMu.Unlock()

	return nil
}

func (b *ChromedpBrowser) runRestarter() {
	defer close(b.restartDone)

	ticker := time.NewTicker(b.restartInterval)
	defer ticker.Stop()

	log.Debug().Msg("Browser restarter is running...")
	for {
		select {
		case <-ticker.C:
			b.restarterMu.Lock()
			if b.restarting {
				b.restarterMu.Unlock()
				continue
			}
			b.restarting = true
			b.restarterMu.Unlock()

			if err := b.restart(); err != nil {
				fmt.Printf("Failed to restart browser: %v\n", err)
			}

			b.restarterMu.Lock()
			b.restarting = false
			b.restarterMu.Unlock()

		case <-b.stopRestarter:
			return
		}
	}
}

func (b *ChromedpBrowser) restart() error {
	log.Info().Msg("Restarting Chromedp...")

	if err := b.initializeBrowser(b.parentContext); err != nil {
		log.Error().Err(err).Msg("Failed to initialize Chromedp")
		return fmt.Errorf("failed to initialize new browser: %w", err)
	}
	log.Info().Msg("Restarted Chromedp successfully")

	return nil
}

func (b *ChromedpBrowser) ScreenshotHTML(html string) ([]byte, error) {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(html)))

	v, err, _ := b.screenshotSF.Do(key, func() (any, error) {
		b.chromedpMu.Lock()
		defer b.chromedpMu.Unlock()

		if b.parentContext.Err() != nil {
			return nil, fmt.Errorf("browser shutting down: %w", b.parentContext.Err())
		}

		// The previous browser may have crashed (e.g. it lost connection), which
		// cancels its context. Re-initialize it so screenshots keep working.
		if b.needsReinit() {
			if err := b.reinit(b.parentContext); err != nil {
				return nil, fmt.Errorf("failed to re-initialize browser: %w", err)
			}
			log.Warn().Msg("Browser context was cancelled; re-initialized Chromedp")
		}

		log.Debug().Msg("Taking screenshot...")

		imageData, err := b.runScreenshot(html)
		if err != nil && errors.Is(err, context.Canceled) && b.chromedpCtx.Err() != nil {
			// The browser died mid-screenshot; re-initialize and retry once.
			log.Warn().Err(err).Msg("Browser died during screenshot, re-initializing and retrying...")
			if rerr := b.reinit(b.parentContext); rerr != nil {
				return nil, fmt.Errorf("failed to re-initialize browser after crash: %w", rerr)
			}
			imageData, err = b.runScreenshot(html)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to take screenshot: %w", err)
		}
		log.Trace().Msg("Taken screenshot")

		return imageData, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

func (b *ChromedpBrowser) runScreenshot(html string) ([]byte, error) {
	var imageData []byte
	timeout := viper.GetDuration(config.KeyBrowserTimeout)
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(b.chromedpCtx, timeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Navigate("about:blank"),
		LogAction("Navigated to about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			frameTree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return fmt.Errorf("failed to get frame tree: %w", err)
			}
			return page.SetDocumentContent(frameTree.Frame.ID, html).Do(ctx)
		}),
		LogAction("Set document content"),
		chromedp.FullScreenshot(&imageData, 100),
		LogAction("Taken full screenshot. Done."),
	}); err != nil {
		return nil, err
	}
	return imageData, nil
}

func (b *ChromedpBrowser) HealthCheck() error {
	if _, err := b.ScreenshotHTML(`<html><body>test</body></html>`); err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	return nil
}

func LogAction(msg string) *chromedpLogAction { return &chromedpLogAction{msg} }

type chromedpLogAction struct{ msg string }

func (a *chromedpLogAction) Do(context.Context) error { log.Trace().Msg(a.msg); return nil }

func scale(width, height int, scale float64) (int, int) {
	return int(float64(width) * scale), int(float64(height) * scale)
}
