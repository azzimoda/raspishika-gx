package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/repository"
)

const proxyTestData = `[
	{"protocol":"socks5","ip":"1.2.3.4","port":1080,"geolocation":{"country":"US"}},
	{"protocol":"socks5","ip":"5.6.7.8","port":1081,"geolocation":{"country":"DE"}},
	{"protocol":"socks5","ip":"9.9.9.9","port":1082,"geolocation":{"country":"US"}}
]`

type fakeProxyChecker struct {
	mu      sync.Mutex
	latency map[string]time.Duration
	calls   int
}

func (f *fakeProxyChecker) CheckLatency(_ context.Context, proxy string) (time.Duration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	lat, ok := f.latency[proxy]
	if !ok {
		return 0, ErrProxyUnavailable
	}
	return lat, nil
}

func (f *fakeProxyChecker) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestProxyService(t *testing.T, fake *fakeProxyChecker) *ProxyService {
	t.Helper()
	repo := repository.NewProxyRepository()
	if err := repo.UpdateFromJSON([]byte(proxyTestData)); err != nil {
		t.Fatalf("UpdateFromJSON() error: %v", err)
	}
	s := NewProxyService(repo)
	s.checker = fake
	t.Cleanup(s.Stop)
	return s
}

func TestFirstAvailablePicksFastestFromScan(t *testing.T) {
	fake := &fakeProxyChecker{latency: map[string]time.Duration{
		"1.2.3.4:1080": 10 * time.Millisecond,
		"5.6.7.8:1081": 20 * time.Millisecond,
		"9.9.9.9:1082": 30 * time.Millisecond,
	}}
	s := newTestProxyService(t, fake)

	got, err := s.FirstAvailable(context.Background())
	if err != nil {
		t.Fatalf("FirstAvailable() error: %v", err)
	}
	if got != "1.2.3.4:1080" {
		t.Fatalf("FirstAvailable() = %q, want fastest proxy", got)
	}
}

func TestFirstAvailableSkipsDeadPoolMembers(t *testing.T) {
	fake := &fakeProxyChecker{latency: map[string]time.Duration{
		"1.2.3.4:1080": 10 * time.Millisecond,
		"9.9.9.9:1082": 30 * time.Millisecond,
	}}
	s := newTestProxyService(t, fake)

	s.poolMu.Lock()
	s.pool = []pooledProxy{
		{proxy: "1.2.3.4:1080", latency: 10 * time.Millisecond},
		{proxy: "5.6.7.8:1081", latency: 20 * time.Millisecond}, // dead
		{proxy: "9.9.9.9:1082", latency: 30 * time.Millisecond},
	}
	s.poolMu.Unlock()

	got, err := s.FirstAvailable(context.Background())
	if err != nil {
		t.Fatalf("FirstAvailable() error: %v", err)
	}
	if got != "1.2.3.4:1080" {
		t.Fatalf("FirstAvailable() = %q, want fastest live pool proxy", got)
	}
	if calls := fake.callCount(); calls > len(s.pool) {
		t.Fatalf("FirstAvailable() did %d checks, want <= pool size %d", calls, len(s.pool))
	}
}

func TestRevalidatePoolDropsDead(t *testing.T) {
	fake := &fakeProxyChecker{latency: map[string]time.Duration{
		"1.2.3.4:1080": 10 * time.Millisecond,
		"5.6.7.8:1081": 20 * time.Millisecond,
	}}
	s := newTestProxyService(t, fake)

	s.poolMu.Lock()
	s.pool = []pooledProxy{
		{proxy: "1.2.3.4:1080", latency: 10 * time.Millisecond},
		{proxy: "5.6.7.8:1081", latency: 20 * time.Millisecond},
		{proxy: "9.9.9.9:1082", latency: 30 * time.Millisecond}, // dead
	}
	s.poolMu.Unlock()

	s.revalidatePool(context.Background())

	s.poolMu.Lock()
	defer s.poolMu.Unlock()
	for _, pp := range s.pool {
		if pp.proxy == "9.9.9.9:1082" {
			t.Fatal("dead proxy was not dropped from the pool")
		}
	}
}

func TestFirstAvailableNoProxies(t *testing.T) {
	fake := &fakeProxyChecker{latency: map[string]time.Duration{}}
	s := newTestProxyService(t, fake)

	_, err := s.FirstAvailable(context.Background())
	if !errors.Is(err, ErrNoAvailableProxy) {
		t.Fatalf("FirstAvailable() error = %v, want ErrNoAvailableProxy", err)
	}
}
