package service

import (
	"context"
	"fmt"

	"github.com/azzimoda/go-tg-proxy/proxy"
)

// justrayFirstSource prefers a local justray mixed in-bound over a fallback
// source of free proxies. justray (a ~1ms local hop) always wins the latency
// ranking of the warm pool while its upstream is healthy; as soon as it is
// unreachable the checker drops it and the pool tops up with the fallback
// proxies.
type justrayFirstSource struct {
	justrayAddr string
	fallback    proxy.Source
}

// newJustrayFirstSource returns the fallback source as-is when no justray
// address is configured, otherwise a source that always reports justray first.
func newJustrayFirstSource(justrayAddr string, fallback proxy.Source) proxy.Source {
	if justrayAddr == "" {
		return fallback
	}
	return justrayFirstSource{justrayAddr: justrayAddr, fallback: fallback}
}

// Fetch returns justray ahead of the fallback list.
//
// A failed fallback fetch is reported as an error rather than downgraded to a
// justray-only list. proxy.Service caches a successful fetch for an hour, so
// returning a degraded list without an error replaced the whole repository with
// justray and marked it fresh: for the next hour the bot ran with no free-proxy
// fallback even after proxifly had recovered. Propagating the error leaves the
// previous list in place and the next call retries, which is what a transient
// upstream failure needs.
//
// The only cost is a cold start while proxifly is down: the repository is still
// empty, so the bot has no proxy at all until the source answers.
func (s justrayFirstSource) Fetch(ctx context.Context) ([]string, error) {
	fallback, err := s.fallback.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fallback proxy source: %w", err)
	}

	addrs := make([]string, 0, len(fallback)+1)
	addrs = append(addrs, s.justrayAddr)
	for _, a := range fallback {
		if a != s.justrayAddr {
			addrs = append(addrs, a)
		}
	}
	return addrs, nil
}
