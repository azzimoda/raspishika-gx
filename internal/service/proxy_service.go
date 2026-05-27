package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/proxyutil"
)

func NewProxyService(repo repository.ProxyRepository) *ProxyService { return &ProxyService{repo: repo} }

type ProxyService struct{ repo repository.ProxyRepository }

var ErrNoAvailableProxy = errors.New("no available proxy")

func (s *ProxyService) FirstAvailable() (string, error) {
	proxies := s.repo.All()
	result, ok := findFirstAsync(proxies, func(p string) bool { return s.Check(p) == nil })
	if ok {
		log.Debug().Str("proxy", result).Msg("Found available proxy")
		return result, nil
	}
	log.Debug().Msg("No available proxy found")
	return "", ErrNoAvailableProxy
}

// NextChecked returns the next proxy from the list that is checked to be available
//
// Returns [ErrNoAvailableProxy] if no proxy is available
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

// Check checks the availability of a proxy and returns an error if it is unavailable
//
// Returns [ErrProxyUnavailable] if the proxy is unavailable, or [ErrEmptyProxy] if the proxy is empty
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
	opts := []bot.Option{bot.WithHTTPClient(proxyFinderTimeout, httpClient),}
	_, err = bot.New("faketoken", opts...)

	ctx, cancel := context.WithTimeout(ctx, proxyFinderTimeout)
	defer cancel()
	if strings.Contains(err.Error(), "not found") {
		// Telegram API is abailable, then the proxy is available
		log.Info().Str("proxy", proxy).Msg("Proxy is available")
		return nil
	} else if !strings.Contains(err.Error(), "context") {
		// Telegram API is unavailable, then the proxy is unavailable
		return ErrProxyUnavailable
	}
	return ErrProxyUnavailable
}

func findFirstAsync[T any](items []T, predicate func(T) bool) (T, bool) {
	resultChan := make(chan T, 1)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		go func(item T) {
			defer wg.Done()
			if predicate(item) {
				select {
				case resultChan <- item:
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
	return result, ok // ok=false means no match found
}
