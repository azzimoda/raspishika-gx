package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/proxyutil"
)

func NewProxyService(repo repository.ProxyRepository) *ProxyService { return &ProxyService{repo: repo} }

type ProxyService struct{ repo repository.ProxyRepository }

var ErrNoAvailableProxy = errors.New("no available proxy")

const proxyFinderWorkers = 128

// FirstAvailable returns the first available proxy from the list.
func (s *ProxyService) FirstAvailable() (string, error) {
	if err := s.UpdateProxies(); err != nil {
		return "", fmt.Errorf("failed to update proxies: %w", err)
	}

	proxies := s.repo.All()
	log.Trace().Any("proxies", len(proxies)).Msg("Checking proxies...")

	result, ok := findFirstAsync(proxies, proxyFinderWorkers, func(ctx context.Context, p string) bool { return s.Check(ctx, p) == nil })
	if ok {
		log.Debug().Str("proxy", result).Msg("Found available proxy")
		return result, nil
	}
	log.Debug().Msg("No available proxy found")
	return "", ErrNoAvailableProxy
}

const proxySourceURL = "https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json"
const proxyCacheTTL = 1 * time.Hour

var muUpdate sync.Mutex

// UpdateProxies synchronously updates the proxy list from the source URL.
func (s *ProxyService) UpdateProxies() error {
	muUpdate.Lock()
	defer muUpdate.Unlock()

	if updatedAt := s.repo.UpdatedAt(); time.Since(updatedAt) <= proxyCacheTTL {
		log.Trace().Time("updatedAt", updatedAt).Msg("Proxies are actual")
		return nil
	}

	log.Debug().Msg("Updating proxies...")
	resp, err := getProxies()
	if err != nil {
		return fmt.Errorf("failed to update proxies: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to update proxies: %w", err)
	}

	s.repo.UpdateFromJSON(body)
	all := s.repo.All()
	log.Debug().Int("count", len(all)).Msg("Proxies updated")

	return nil
}

func getProxies() (*http.Response, error) {
	client := new(http.Client{Timeout: 30 * time.Second})
	var resp *http.Response
	err := retry.New(retry.Attempts(5), retry.Delay(100*time.Millisecond), retry.DelayType(retry.BackOffDelay)).Do(
		func() error {
			var err error
			resp, err = client.Get(proxySourceURL)
			return err
		},
	)
	return resp, err
}

// NextChecked returns the next proxy from the list that is checked to be available.
//
// Returns [ErrNoAvailableProxy] if no proxy is available.
func (s *ProxyService) NextChecked(ctx context.Context) (string, error) {
	idxStart, proxy := s.repo.Next()
	idx := idxStart
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		if err := s.Check(ctx, proxy); err == nil {
			return proxy, nil
		}
		idx, proxy = s.repo.Next()
		if idx == idxStart {
			// No more proxies to check, return error
			return "", ErrNoAvailableProxy
		}
	}
}

// Next returns the next proxy from the list
func (s *ProxyService) Next() string {
	_, proxy := s.repo.Next()
	return proxy
}

const proxyFinderTimeout = 5 * time.Second

var ErrEmptyProxy = errors.New("empty proxy")
var ErrProxyUnavailable = errors.New("proxy unavailable")

// Check checks the availability of a proxy and returns an error if it is unavailable.
//
// Returns [ErrProxyUnavailable] if the proxy is unavailable, or [ErrEmptyProxy] if the proxy is empty.
func (s *ProxyService) Check(ctx context.Context, proxy string) error {
	if proxy == "" {
		return ErrEmptyProxy
	}

	// Check the proxy by creating a telegram bot with fake token
	httpClient, err := proxyutil.NewHTTPProxyClient(proxy)
	if err != nil {
		// Proxy is unavailable
		return fmt.Errorf("%w: %w", ErrProxyUnavailable, err)
	}
	opts := []bot.Option{bot.WithHTTPClient(proxyFinderTimeout, httpClient)}
	_, err = bot.New("faketoken", opts...)

	ctx, cancel := context.WithTimeout(ctx, proxyFinderTimeout)
	defer cancel()
	if strings.Contains(err.Error(), "not found") {
		// Telegram API is abailable, then the proxy is available
		return nil
	}
	return ErrProxyUnavailable
}

// HealthCheck performs a health check on the proxy service,
// ensuring the proxy source is available and returns at least one proxy.
func (s *ProxyService) HealthCheck() error {
	if err := s.UpdateProxies(); err != nil {
		return err
	}
	if len(s.repo.All()) == 0 {
		return fmt.Errorf("no proxies available")
	}
	return nil
}

func findFirstAsync[T any](items []T, maxConcurrent int, predicate func(context.Context, T) bool) (T, bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	semaphore := make(chan struct{}, maxConcurrent)
	resultChan := make(chan T, 1)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)

		semaphore <- struct{}{}

		go func(item T) {
			defer wg.Done()
			defer func() { <-semaphore }()

			// Check if context is done before running the predicate
			select {
			case <-ctx.Done():
				return
			default:
			}

			if predicate(ctx, item) {
				select {
				case resultChan <- item:
					cancel()
				default:
				}
			}
		}(item)
	}

	// Wait for all goroutines to finish in a separate goroutine
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	result, ok := <-resultChan
	return result, ok
}
