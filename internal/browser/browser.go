package browser

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func New(ctx context.Context) (*ChromedpBrowser, error) {
	restartInterval := viper.GetDuration("browser.restart_interval")
	if restartInterval == 0 {
		restartInterval = 24 * time.Hour
	}

	b := &ChromedpBrowser{
		parentContext:   ctx,
		restartInterval: restartInterval,
		stopRestarter:   make(chan struct{}),
		restartDone:     make(chan struct{}),
	}

	if err := b.initializeBrowser(b.parentContext); err != nil {
		return nil, err
	}

	if b.restartInterval > 0 {
		go b.runRestarter()
	}

	return b, nil
}

type ChromedpBrowser struct {
	parentContext  context.Context
	chromedpCtx    context.Context
	chromedpCancel context.CancelFunc

	chromedpMu  sync.RWMutex
	restarterMu sync.Mutex

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
	b.chromedpMu.Lock()
	defer b.chromedpMu.Unlock()

	if b.parentContext.Err() != nil {
		return nil, fmt.Errorf("browser shutting down: %w", b.parentContext.Err())
	}

	// The previous browser may have crashed (e.g. it lost connection), which
	// cancels its context. Re-initialize it so screenshots keep working.
	if b.chromedpCtx == nil || b.chromedpCtx.Err() != nil {
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
}

func (b *ChromedpBrowser) runScreenshot(html string) ([]byte, error) {
	var imageData []byte
	if err := chromedp.Run(b.chromedpCtx, chromedp.Tasks{
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
