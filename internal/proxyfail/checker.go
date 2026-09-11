package proxyfail

import (
	"context"
	"time"

	"github.com/azzimoda/go-tg-proxy/proxy"
)

// BanChecker wraps a proxy.Checker and treats proxies remembered by the tracker
// as unavailable, so FirstAvailable, pool revalidation and pool top-ups all
// skip them without touching the proxy.Service internals.
//
// TODO: move the ban decision into github.com/azzimoda/go-tg-proxy/proxy.Service.
type BanChecker struct {
	Checker proxy.Checker
	Tracker *FailTracker
}

// CheckLatency reports the proxy as unavailable while it is banned and
// otherwise delegates to the wrapped checker.
func (c BanChecker) CheckLatency(ctx context.Context, proxyAddr string) (time.Duration, error) {
	if c.Tracker != nil && c.Tracker.IsBanned(proxyAddr) {
		return 0, proxy.ErrProxyUnavailable
	}
	return c.Checker.CheckLatency(ctx, proxyAddr)
}
