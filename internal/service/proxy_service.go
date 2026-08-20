package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/proxyutil"
)

// ProxyChecker checks a single proxy and reports its round-trip latency.
type ProxyChecker interface {
	CheckLatency(ctx context.Context, proxy string) (time.Duration, error)
}

func NewProxyService(repo repository.ProxyRepository) *ProxyService {
	return &ProxyService{repo: repo, checker: realProxyChecker{}}
}

type ProxyService struct {
	repo    repository.ProxyRepository
	checker ProxyChecker

	poolMu sync.Mutex
	pool   []pooledProxy

	startPoolOnce sync.Once
	stopPool      context.CancelFunc
}

type pooledProxy struct {
	proxy   string
	latency time.Duration
	checked time.Time
}

var ErrNoAvailableProxy = errors.New("no available proxy")

const (
	proxyFinderWorkers = 128
	proxyPoolSize      = 3
	proxyPoolRecheck   = 30 * time.Second
)

// StartPoolLoop starts the background goroutine that keeps the pool of verified
// proxies warm. It is started lazily on the first FirstAvailable call as well.
func (s *ProxyService) StartPoolLoop() {
	s.startPoolOnce.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		s.stopPool = cancel
		go s.poolLoop(ctx)
	})
}

func (s *ProxyService) Stop() {
	if s.stopPool != nil {
		s.stopPool()
	}
}

// FirstAvailable returns the best available proxy from the warm pool.
//
// Falls back to a full scan of the list when the pool is empty.
func (s *ProxyService) FirstAvailable(ctx context.Context) (string, error) {
	s.StartPoolLoop()

	if err := s.UpdateProxies(); err != nil {
		return "", fmt.Errorf("failed to update proxies: %w", err)
	}

	s.poolMu.Lock()
	pool := append([]pooledProxy(nil), s.pool...)
	s.poolMu.Unlock()

	if len(pool) == 0 {
		s.revalidatePool(ctx)
		s.poolMu.Lock()
		pool = append([]pooledProxy(nil), s.pool...)
		s.poolMu.Unlock()
	}

	for _, pp := range pool {
		if lat, err := s.checker.CheckLatency(ctx, pp.proxy); err == nil {
			log.Debug().Str("proxy", pp.proxy).Dur("latency", lat).Msg("Found available proxy")
			return pp.proxy, nil
		}
	}

	// Pool exhausted: refresh it and try once more before giving up.
	s.revalidatePool(ctx)
	s.poolMu.Lock()
	pool = append([]pooledProxy(nil), s.pool...)
	s.poolMu.Unlock()
	for _, pp := range pool {
		if lat, err := s.checker.CheckLatency(ctx, pp.proxy); err == nil {
			log.Debug().Str("proxy", pp.proxy).Dur("latency", lat).Msg("Found available proxy after pool refresh")
			return pp.proxy, nil
		}
	}

	log.Debug().Msg("No available proxy found")
	return "", ErrNoAvailableProxy
}

func (s *ProxyService) poolLoop(ctx context.Context) {
	ticker := time.NewTicker(proxyPoolRecheck)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.revalidatePool(ctx)
		}
	}
}

// revalidatePool re-checks the current pool members, drops dead ones and tops
// the pool back up to proxyPoolSize selecting candidates by lowest latency.
func (s *ProxyService) revalidatePool(ctx context.Context) {
	s.poolMu.Lock()
	current := append([]pooledProxy(nil), s.pool...)
	s.poolMu.Unlock()

	valid := make([]pooledProxy, 0, proxyPoolSize)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, pp := range current {
		wg.Add(1)
		go func(pp pooledProxy) {
			defer wg.Done()
			if lat, err := s.checker.CheckLatency(ctx, pp.proxy); err == nil {
				mu.Lock()
				valid = append(valid, pooledProxy{proxy: pp.proxy, latency: lat, checked: time.Now()})
				mu.Unlock()
			} else {
				log.Warn().Str("proxy", pp.proxy).Err(err).Msg("Pool proxy dropped")
			}
		}(pp)
	}
	wg.Wait()

	// Everything died: refresh the source list before trying to top up.
	if len(valid) == 0 {
		if err := s.UpdateProxies(); err != nil {
			log.Error().Err(err).Msg("Failed to update proxies for pool refresh")
		}
	}

	if need := proxyPoolSize - len(valid); need > 0 {
		proxies := s.repo.All()
		seen := make(map[string]bool, len(valid))
		for _, pp := range valid {
			seen[pp.proxy] = true
		}
		candidates := make([]string, 0, len(proxies))
		for _, p := range proxies {
			if !seen[p] {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) > 0 {
			for _, c := range findBestAsync(candidates, proxyFinderWorkers, need,
				func(ctx context.Context, p string) (time.Duration, bool) {
					lat, err := s.checker.CheckLatency(ctx, p)
					return lat, err == nil
				}) {
				valid = append(valid, pooledProxy{proxy: c.proxy, latency: c.latency, checked: time.Now()})
			}
		}
	}

	sort.Slice(valid, func(i, j int) bool { return valid[i].latency < valid[j].latency })

	s.poolMu.Lock()
	s.pool = valid
	s.poolMu.Unlock()
	log.Debug().Int("poolSize", len(valid)).Msg("Proxy pool revalidated")
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

	if err := s.repo.UpdateFromJSON(body); err != nil {
		return fmt.Errorf("failed to update proxies: %w", err)
	}
	all := s.repo.All()
	log.Debug().Int("count", len(all)).Msg("Proxies updated")

	return nil
}

func getProxies() (*http.Response, error) {
	client := new(http.Client{Timeout: 30 * time.Second})
	var resp *http.Response
	err := retry.New(
		retry.Attempts(5),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.BackOffDelay),
		retry.OnRetry(func(attempt uint, err error) {
			log.Debug().Uint("attempt", attempt).Err(err).Msg("Retrying...")
		}),
	).Do(
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

type realProxyChecker struct{}

// CheckLatency checks the availability of a proxy and returns its round-trip
// latency. The proxy is considered available if a Telegram API call with a fake
// token fails with "not found" — i.e. the proxy reaches Telegram end-to-end.
func (realProxyChecker) CheckLatency(ctx context.Context, proxy string) (time.Duration, error) {
	if proxy == "" {
		return 0, ErrEmptyProxy
	}

	start := time.Now()

	httpClient, err := proxyutil.NewHTTPProxyClient(proxy)
	if err != nil {
		// Proxy is unavailable
		return 0, fmt.Errorf("%w: %w", ErrProxyUnavailable, err)
	}
	opts := []bot.Option{
		bot.WithHTTPClient(proxyFinderTimeout, httpClient),
		bot.WithCheckInitTimeout(proxyFinderTimeout),
	}
	_, err = bot.New("faketoken", opts...)

	ctx, cancel := context.WithTimeout(ctx, proxyFinderTimeout)
	defer cancel()

	if err != nil && errors.Is(err, bot.ErrorNotFound) {
		// Telegram API is available, then the proxy is available
		return time.Since(start), nil
	}
	return 0, ErrProxyUnavailable
}

// Check checks the availability of a proxy and returns an error if it is unavailable.
//
// Returns [ErrProxyUnavailable] if the proxy is unavailable, or [ErrEmptyProxy] if the proxy is empty.
func (s *ProxyService) Check(ctx context.Context, proxy string) error {
	_, err := s.checker.CheckLatency(ctx, proxy)
	return err
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

type proxyCandidate struct {
	proxy   string
	latency time.Duration
}

// findBestAsync concurrently checks candidates and returns up to n best ones
// ordered by latency.
func findBestAsync(items []string, maxConcurrent, n int, check func(context.Context, string) (time.Duration, bool)) []proxyCandidate {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	semaphore := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	results := make([]proxyCandidate, 0)
	var wg sync.WaitGroup

	for _, item := range items {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(item string) {
			defer wg.Done()
			defer func() { <-semaphore }()

			if lat, ok := check(ctx, item); ok {
				mu.Lock()
				results = append(results, proxyCandidate{proxy: item, latency: lat})
				mu.Unlock()
			}
		}(item)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].latency < results[j].latency })
	if n > 0 && len(results) > n {
		results = results[:n]
	}
	return results
}
