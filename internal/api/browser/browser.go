// Package browser wraps a Playwright browser instance with health checks
// and periodic restarts.
package browser

import (
	"fmt"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/rs/zerolog/log"
)

type options struct {
	headless        bool
	timeout         time.Duration
	restartInterval time.Duration
}

func defaultOptions() *options {
	return &options{
		headless:        true,
		restartInterval: 0, // Do not restart
		timeout:         30 * time.Second,
	}
}

// Headless sets whether the browser runs without a visible window.
func Headless(value bool) Option { return func(o *options) { o.headless = value } }

// Timeout sets the browser launch timeout.
func Timeout(value time.Duration) Option { return func(o *options) { o.timeout = value } }

// RestartInterval sets how often the browser is restarted.
// A zero value disables restarts.
func RestartInterval(value time.Duration) Option {
	return func(o *options) { o.restartInterval = value }
}

// Option configures a Browser.
type Option func(*options)

// New creates a Browser, launching the Playwright browser.
func New(opts ...Option) (*Browser, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	b := &Browser{
		opts:            options,
		restartInterval: options.restartInterval,
		stopRestarter:   make(chan struct{}),
		restartDone:     make(chan struct{}),
	}

	if err := b.initializeBrowser(); err != nil {
		return nil, err
	}

	if b.restartInterval > 0 {
		go b.runRestarter()
	} else {
		close(b.restartDone)
	}

	return b, nil

}

// Browser is a managed Playwright browser instance.
type Browser struct {
	opts *options

	pw          *playwright.Playwright
	pwBrowser   playwright.Browser
	pwMu        sync.RWMutex
	restarterMu sync.Mutex

	restartInterval time.Duration
	stopRestarter   chan struct{}
	restartDone     chan struct{}
	restarting      bool
}

func (b *Browser) initializeBrowser() error {
	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("failed to start Playwright instance: %w", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: new(b.opts.headless),
		Timeout:  new(float64(b.opts.timeout.Milliseconds())),
	})
	if err != nil {
		pw.Stop()
		return fmt.Errorf("failed to launch browser: %w", err)
	}

	b.pwMu.Lock()
	b.pw = pw
	b.pwBrowser = browser
	b.pwMu.Unlock()

	b.restarterMu.Lock()
	b.restarting = false
	b.restarterMu.Unlock()

	return nil
}

// WithContext runs f inside a fresh browser context and closes it afterwards.
func (b *Browser) WithContext(f func(playwright.BrowserContext) error) error {
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

// WithPage runs f inside a fresh page and closes it afterwards.
func (b *Browser) WithPage(f func(playwright.Page) error) error {
	return b.WithContext(func(bc playwright.BrowserContext) error {
		page, err := bc.NewPage()
		if err != nil {
			return fmt.Errorf("failed to create page: %w", err)
		}
		defer page.Close()

		return f(page)
	})
}

// HealthCheck verifies that the browser can still open a page.
func (b *Browser) HealthCheck() error {
	b.pwMu.RLock()
	if b.pwBrowser == nil {
		b.pwMu.RUnlock()
		return fmt.Errorf("browser not initialized")
	}
	b.pwMu.RUnlock()

	page, err := b.pwBrowser.NewPage()
	if err != nil {
		return fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	return nil
}

// Close stops the restarter, closes the browser and stops Playwright.
func (b *Browser) Close() error {
	select {
	case <-b.stopRestarter:
	default:
		close(b.stopRestarter)
	}
	<-b.restartDone

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
		return fmt.Errorf("browser closed with errors: %w", errs[0])
	}
	return nil
}

func (b *Browser) runRestarter() {
	defer close(b.restartDone)

	ticker := time.NewTicker(b.restartInterval)
	defer ticker.Stop()

	log.Debug().Msg("API browser restarter is running...")
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
				log.Error().Err(err).Msg("Failed to restart API browser")
			}

			b.restarterMu.Lock()
			b.restarting = false
			b.restarterMu.Unlock()

		case <-b.stopRestarter:
			return
		}
	}
}

func (b *Browser) restart() error {
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

	return b.initializeBrowser()
}
