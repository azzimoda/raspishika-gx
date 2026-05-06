package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/playwright-community/playwright-go"
	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/pkg/config"
)

// TODO: Implement regular restart.

func New() (*BrowserService, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("failted to start Playwright instance: %w", err)
	}

	isHeadless := viper.GetBool(config.KeyBrowserHeadless)

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(isHeadless),
		Timeout:  new(float64(viper.GetDuration(config.KeyBrowserTimeout).Milliseconds())),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	width, height := scale(viper.GetInt(config.KeyBrowserWidth), viper.GetInt(config.KeyBrowserHeight), viper.GetFloat64(config.KeyBrowserScale))
	ctx, cancelExecAllocator := chromedp.NewExecAllocator(
		context.Background(),
		chromedp.Flag("headless", isHeadless),
		chromedp.WindowSize(width, height),
	)
	ctx, cancelChromeDP := chromedp.NewContext(ctx, chromedp.WithBrowserOption())

	cancel := func() {
		cancelChromeDP()
		cancelExecAllocator()
	}
	bs := BrowserService{pw: pw, pwBrowser: browser, chromedpCtx: ctx, chromedpCancel: cancel}

	return &bs, nil
}

type BrowserService struct {
	pw             *playwright.Playwright
	pwBrowser      playwright.Browser
	chromedpCtx    context.Context
	chromedpCancel context.CancelFunc
	mu             sync.Mutex
}

func (b *BrowserService) Close() error {
	b.chromedpCancel()
	browserErr := b.pwBrowser.Close()
	pwErr := b.pw.Stop()
	if err := errors.Join(browserErr, pwErr); err != nil {
		return fmt.Errorf("Browser services closed with errors: %w", err)
	}
	return nil
}

func (b *BrowserService) WithContext(f func(playwright.BrowserContext) error) error {
	ctx, err := b.pwBrowser.NewContext()
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
	b.mu.Lock()
	defer b.mu.Unlock()

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
	if err := chromedp.Run(b.chromedpCtx, screenshotElement); err != nil {
		return fmt.Errorf("failed to take screenshot: %w", err)
	}

	// Ensure screenshot directory
	if err := os.MkdirAll(viper.GetString(config.KeyScreenshotDir), 0755); err != nil {
		return fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	if err := os.WriteFile(outputFilename, imageData, 0644); err != nil {
		return fmt.Errorf("failed to write screenshot to file: %w", err)
	}
	return nil
}

func scale(width, height int, scale float64) (int, int) {
	return int(float64(width) * scale), int(float64(height) * scale)
}
