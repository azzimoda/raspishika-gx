package service

import (
	"context"
	"time"

	"github.com/azzimoda/go-tg-proxy/proxy"
)

// justrayLatencyFloor is the latency reported for the justray in-bound.
//
// The warm pool ranks strictly by measured latency, so any value below the
// fastest plausible free proxy guarantees the head position. 1ms is a local hop
// in any case, so the number is not meant to be a measurement.
const justrayLatencyFloor = time.Millisecond

// justrayFirstChecker keeps the local justray in-bound at the head of the warm
// pool.
//
// The pool does not consider where a proxy came from, it only sorts by the
// latency the checker reports. TelegramChecker measures the whole end-to-end
// round trip from this process through the proxy to Telegram, not the local hop,
// so a justray node on a slow upstream is reported as slow and loses to a fast
// free proxy. Putting justray first in the source list therefore did not make it
// win: whether it was used came down to how the latency ranking happened to sort
// on that refresh.
//
// The underlying check is still the real one. A justray node that cannot reach
// Telegram returns an error exactly as before, so it is dropped from the pool
// and the free proxies take over; only the reported latency is floored, and only
// for the justray address. A banned justray stays banned, because an error from
// the inner checker is passed through untouched.
type justrayFirstChecker struct {
	inner proxy.Checker
	addr  string
}

func newJustrayFirstChecker(justrayAddr string, inner proxy.Checker) proxy.Checker {
	if justrayAddr == "" {
		return inner
	}
	return justrayFirstChecker{inner: inner, addr: justrayAddr}
}

func (c justrayFirstChecker) CheckLatency(ctx context.Context, proxyAddr string) (time.Duration, error) {
	latency, err := c.inner.CheckLatency(ctx, proxyAddr)
	if err != nil {
		return 0, err
	}
	if proxyAddr == c.addr {
		return justrayLatencyFloor, nil
	}
	return latency, nil
}
