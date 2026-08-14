package browser

import (
	"context"
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
	isHeadless := viper.GetBool(config.KeyBrowserHeadless)

	width, height := scale(
		viper.GetInt(config.KeyBrowserWidth),
		viper.GetInt(config.KeyBrowserHeight),
		viper.GetFloat64(config.KeyBrowserScale),
	)
	ctx, cancelExecAllocator := chromedp.NewExecAllocator(ctx,
		chromedp.Flag("headless", isHeadless),
		chromedp.WindowSize(width, height),
	)
	ctx, cancelChromeDP := chromedp.NewContext(ctx,
		chromedp.WithBrowserOption(chromedp.WithDialTimeout(20*time.Second)))

	b.chromedpMu.Lock()
	b.chromedpCtx = ctx
	b.chromedpCancel = func() {
		log.Warn().Msg("BROWSER CONTEXT CANCELLED!!!")
		cancelChromeDP()
		cancelExecAllocator()
	}
	b.chromedpMu.Unlock()

	b.restarterMu.Lock()
	b.restarting = false
	b.restarterMu.Unlock()

	return nil
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

	b.chromedpMu.Lock()
	if b.chromedpCancel != nil {
		b.chromedpCancel()
	}
	b.chromedpMu.Unlock()

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

	log.Debug().Msg("Taking screenshot...")

	ctx := b.chromedpCtx
	if ctx == nil {
		return nil, fmt.Errorf("browser context not initialized")
	}

	var imageData []byte
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
		return nil, fmt.Errorf("failed to take screenshot: %w", err)
	}
	log.Trace().Msg("Taken screenshot")

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
