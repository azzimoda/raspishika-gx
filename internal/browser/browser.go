package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/playwright-community/playwright-go"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/pkg/config"
)

func New() (*BrowserService, error) {
	restartInterval := viper.GetDuration("browser.restart_interval")
	if restartInterval == 0 {
		restartInterval = 24 * time.Hour
	}

	bs := &BrowserService{
		restartInterval: restartInterval,
		stopRestarter:   make(chan struct{}),
		restartDone:     make(chan struct{}),
	}

	if err := bs.initializeBrowser(); err != nil {
		return nil, err
	}

	if bs.restartInterval > 0 {
		go bs.runRestarter()
	}

	return bs, nil
}

type BrowserService struct {
	pw             *playwright.Playwright
	pwBrowser      playwright.Browser
	chromedpCtx    context.Context
	chromedpCancel context.CancelFunc

	pwMu        sync.RWMutex
	chromedpMu  sync.RWMutex
	restarterMu sync.Mutex

	restartInterval time.Duration
	stopRestarter   chan struct{}
	restartDone     chan struct{}
	restarting      bool
}

func (b *BrowserService) initializeBrowser() error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("failed to start Playwright instance: %w", err)
	}

	isHeadless := viper.GetBool(config.KeyBrowserHeadless)

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &isHeadless,
		Timeout:  new(float64(viper.GetDuration(config.KeyBrowserTimeout).Milliseconds())),
	})
	if err != nil {
		pw.Stop()
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	width, height := scale(viper.GetInt(config.KeyBrowserWidth), viper.GetInt(config.KeyBrowserHeight), viper.GetFloat64(config.KeyBrowserScale))
	ctx, cancelExecAllocator := chromedp.NewExecAllocator(
		context.Background(),
		chromedp.Flag("headless", isHeadless),
		chromedp.WindowSize(width, height),
	)
	ctx, cancelChromeDP := chromedp.NewContext(ctx, chromedp.WithBrowserOption())

	b.pwMu.Lock()
	b.pw = pw
	b.pwBrowser = browser
	b.pwMu.Unlock()

	b.chromedpMu.Lock()
	b.chromedpCtx = ctx
	b.chromedpCancel = func() {
		cancelChromeDP()
		cancelExecAllocator()
	}
	b.chromedpMu.Unlock()

	b.restarterMu.Lock()
	b.restarting = false
	b.restarterMu.Unlock()

	return nil
}

func (b *BrowserService) Close() error {
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

	// Close Playwright
	b.pwMu.Lock()
	defer b.pwMu.Unlock()

	var errs []error
	if b.pwBrowser != nil {
		if err := b.pwBrowser.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if b.pw != nil {
		if err := b.pw.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("Browser services closed with errors: %v", errors.Join(errs...))
	}
	return nil
}

func (b *BrowserService) runRestarter() {
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

func (b *BrowserService) restart() error {
	// Close old Chromedp
	b.chromedpMu.Lock()
	if b.chromedpCancel != nil {
		b.chromedpCancel()
	}
	b.chromedpMu.Unlock()

	// Close old Playwright
	b.pwMu.Lock()
	if b.pwBrowser != nil {
		if err := b.pwBrowser.Close(); err != nil {
			b.pwMu.Unlock()
			return fmt.Errorf("failed to close old browser: %w", err)
		}
	}
	if b.pw != nil {
		if err := b.pw.Stop(); err != nil {
			b.pwMu.Unlock()
			return fmt.Errorf("failed to stop old playwright: %w", err)
		}
	}
	b.pwMu.Unlock()

	// Initialize new browser
	if err := b.initializeBrowser(); err != nil {
		return fmt.Errorf("failed to initialize new browser: %w", err)
	}

	return nil
}

func (b *BrowserService) WithContext(f func(playwright.BrowserContext) error) error {
	b.pwMu.RLock()
	browser := b.pwBrowser
	b.pwMu.RUnlock()

	if browser == nil {
		return fmt.Errorf("browser not initialized")
	}

	ctx, err := browser.NewContext()
	if err != nil {
		return fmt.Errorf("failed to create browser context: %w", err)
	}
	defer ctx.Close()

	return f(ctx)
}
func (b *BrowserService) WithPage(f func(playwright.Page) error) error {
	return b.WithContext(func(bc playwright.BrowserContext) error {
		page, err := bc.NewPage()
		if err != nil {
			return fmt.Errorf("failed to create page: %w", err)
		}
		defer page.Close()

		return f(page)
	})
}

func (b *BrowserService) TakeScreenshotHTML(html, outputFilename string) error {
	b.chromedpMu.Lock()
	defer b.chromedpMu.Unlock()

	ctx := b.chromedpCtx
	if ctx == nil {
		return fmt.Errorf("browser context not initialized")
	}

	var imageData []byte
	screenshotElement := chromedp.Tasks{
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			frameTree, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			return page.SetDocumentContent(frameTree.Frame.ID, html).Do(ctx)
		}),
		chromedp.FullScreenshot(&imageData, 100),
	}
	if err := chromedp.Run(ctx, screenshotElement); err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	if err := os.MkdirAll(viper.GetString(config.KeyScreenshotDir), 0755); err != nil {
		return fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	if err := os.WriteFile(outputFilename, imageData, 0644); err != nil {
		return fmt.Errorf("failed to write screenshot to file: %w", err)
	}
	return nil
}

func (b *BrowserService) HealthCheck() error {
	b.pwMu.RLock()
	if b.pwBrowser == nil {
		b.pwMu.RUnlock()
		return fmt.Errorf("browser not initialized")
	}
	browser := b.pwBrowser
	b.pwMu.RUnlock()

	page, err := browser.NewPage()
	if err != nil {
		return fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	tempDir := os.TempDir()
	if err := b.TakeScreenshotHTML(`<html><body>test</body></html>`, filepath.Join(tempDir, "screenshot.png")); err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	return nil
}

func scale(width, height int, scale float64) (int, int) {
	return int(float64(width) * scale), int(float64(height) * scale)
}
