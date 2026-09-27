package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azzimoda/go-tg-proxy/proxy"
)

type stubChecker struct {
	latency map[string]time.Duration
	err     map[string]error
	calls   []string
}

func (s *stubChecker) CheckLatency(_ context.Context, addr string) (time.Duration, error) {
	s.calls = append(s.calls, addr)
	if err := s.err[addr]; err != nil {
		return 0, err
	}
	return s.latency[addr], nil
}

// justray is a ~1ms local hop, but TelegramChecker measures the whole round trip
// to Telegram, so a node on a slow upstream is reported as slow. The pool sorts
// ascending by that number, which is why putting justray first in the source list
// was not enough to make it win.
func TestJustrayFirstCheckerRanksJustrayFirst(t *testing.T) {
	inner := &stubChecker{latency: map[string]time.Duration{
		"127.0.0.1:10808": 900 * time.Millisecond, // slow upstream
		"fast:1080":       120 * time.Millisecond,
	}}
	checker := newJustrayFirstChecker("127.0.0.1:10808", inner)

	justray, err := checker.CheckLatency(context.Background(), "127.0.0.1:10808")
	if err != nil {
		t.Fatal(err)
	}
	free, err := checker.CheckLatency(context.Background(), "fast:1080")
	if err != nil {
		t.Fatal(err)
	}
	if justray >= free {
		t.Fatalf("justray latency %s is not below the free proxy's %s, so the pool would pick the free proxy", justray, free)
	}
}

// Flooring the reported latency must not turn a dead justray into a usable one:
// the pool drops anything whose check errors, and that is what hands over to the
// free proxies.
func TestJustrayFirstCheckerPassesErrorsThrough(t *testing.T) {
	inner := &stubChecker{
		latency: map[string]time.Duration{"127.0.0.1:10808": 900 * time.Millisecond},
		err:     map[string]error{"127.0.0.1:10808": proxy.ErrProxyUnavailable},
	}
	checker := newJustrayFirstChecker("127.0.0.1:10808", inner)

	latency, err := checker.CheckLatency(context.Background(), "127.0.0.1:10808")
	if !errors.Is(err, proxy.ErrProxyUnavailable) {
		t.Fatalf("error = %v, want it passed through", err)
	}
	if latency != 0 {
		t.Fatalf("latency = %s, want 0 for a failed check", latency)
	}
}

func TestJustrayFirstCheckerPassesThroughWithoutJustray(t *testing.T) {
	inner := &stubChecker{latency: map[string]time.Duration{"fast:1080": 120 * time.Millisecond}}
	checker := newJustrayFirstChecker("", inner)
	if checker != proxy.Checker(inner) {
		t.Fatal("checker must be the inner one when no justray address is configured")
	}
}

func TestJustrayFirstCheckerStillRunsTheRealCheck(t *testing.T) {
	inner := &stubChecker{latency: map[string]time.Duration{"127.0.0.1:10808": 900 * time.Millisecond}}
	checker := newJustrayFirstChecker("127.0.0.1:10808", inner)

	if _, err := checker.CheckLatency(context.Background(), "127.0.0.1:10808"); err != nil {
		t.Fatal(err)
	}
	if len(inner.calls) != 1 || inner.calls[0] != "127.0.0.1:10808" {
		t.Fatalf("underlying check calls = %v, want justray to actually be checked", inner.calls)
	}
}
