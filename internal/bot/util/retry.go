package botutil

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// SendRetryAttempts bounds retries of a network-level send. A dead proxy
// usually recovers or the bot is restarted on another proxy in-between
// attempts, so a short bounded retry is enough.
const SendRetryAttempts = 3

// RetryNetwork runs send until it succeeds, retrying network errors with
// increasing backoff. Non-network errors fail immediately; onNetworkError, when
// set, is called for every network error before it is retried (e.g. to observe
// and remember which proxy failed). The retry aborts when ctx is cancelled.
// After the attempts are exhausted the last error is returned.
func RetryNetwork[T any](
	ctx context.Context,
	onNetworkError func(error),
	send func() (T, error),
) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt < SendRetryAttempts; attempt++ {
		v, err := send()
		if err == nil {
			return v, nil
		}
		lastErr = err
		if !IsNetworkError(err) {
			return zero, err
		}
		if onNetworkError != nil {
			onNetworkError(err)
		}
		log.Warn().Err(err).Int("attempt", attempt+1).Int("max", SendRetryAttempts).
			Msg("Network error, retrying send...")
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return zero, lastErr
}
