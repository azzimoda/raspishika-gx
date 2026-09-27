package service

import (
	"context"

	"github.com/rs/zerolog/log"

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

// Fetch returns justray ahead of the fallback list. A failed fallback fetch is
// downgraded to just justray so the service keeps operating on the local proxy
// instead of dropping it with the free-proxy list.
func (s justrayFirstSource) Fetch(ctx context.Context) ([]string, error) {
	addrs := []string{s.justrayAddr}

	fallback, err := s.fallback.Fetch(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Fallback proxy source failed, keeping justray only")
		return addrs, nil
	}

	for _, a := range fallback {
		if a != s.justrayAddr {
			addrs = append(addrs, a)
		}
	}
	return addrs, nil
}
