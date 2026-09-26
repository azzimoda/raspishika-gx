package vkclient

import (
	"context"
	"errors"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/rs/zerolog/log"
)

// sendRetryAttempts bounds retries of a photo upload+send. Upload failures are
// usually transient VK network/API errors (EOF, flood control), so a short
// bounded retry is enough; permanent errors fail immediately.
const sendRetryAttempts = 3

// retryTransient runs upload until it succeeds, retrying transient errors with
// increasing backoff. Non-transient errors (forbidden, fatal API codes, context
// cancellation) fail immediately. On exhaustion the last error is returned.
func retryTransient(ctx context.Context, upload func() error) error {
	var lastErr error
	for attempt := 0; attempt < sendRetryAttempts; attempt++ {
		if err := upload(); err == nil {
			return nil
		} else {
			lastErr = err
			if !retryableError(ctx, err) {
				return err
			}
		}
		log.Warn().Err(lastErr).Int("attempt", attempt+1).Int("max", sendRetryAttempts).
			Msg("Transient VK error, retrying upload...")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return lastErr
}

// retryableError reports whether a photo upload/send failure is worth retrying.
// Upstream network failures and most VK API codes (flood control, temporary
// limits) are transient; forbidden and fatal-init errors never recover.
func retryableError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var apiErr *api.Error
	if errors.As(err, &apiErr) {
		switch int(apiErr.Code) {
		case 5, 15, 27, 100, 900, 901, 902, 917:
			return false
		}
	}
	return true
}
