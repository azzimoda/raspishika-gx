package service

import (
	"context"
	"fmt"

	"github.com/azzimoda/go-tg-proxy/proxy"
	"github.com/rs/zerolog/log"
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
// A failed fallback fetch when justray is configured degrades to a justray-only
// list rather than failing entirely: the checker will validate justray and drop
// it from the pool if it cannot reach Telegram, while the pool revalidate loop
// retries the fallback source on every cycle. Returning an error instead caused
// a cold-start failure whenever the free-proxy source was down from the start
// (e.g. proxifly returning 403), because the repository stayed empty and the
// health check had no proxies at all.
//
// When no justray address is configured, a failed fallback fetch is still
// propagated as an error so a transient upstream failure retries on the next
// call instead of caching an empty list.
func (s justrayFirstSource) Fetch(ctx context.Context) ([]string, error) {
	fallback, err := s.fallback.Fetch(ctx)
	if err != nil {
		if s.justrayAddr != "" {
			log.Warn().Err(err).Str("justray", s.justrayAddr).
				Msg("Fallback proxy source unavailable; using justray only")
			return []string{s.justrayAddr}, nil
		}
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
