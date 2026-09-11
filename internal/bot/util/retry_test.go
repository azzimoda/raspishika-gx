package botutil

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot"
)

func TestRetryNetworkSucceedsAfterNetworkErrors(t *testing.T) {
	attempts := 0
	var observed []error
	got, err := RetryNetwork(context.Background(), func(e error) { observed = append(observed, e) }, func() (int, error) {
		attempts++
		if attempts < SendRetryAttempts {
			return 0, errors.New("write tcp ... broken pipe")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
	if len(observed) != SendRetryAttempts-1 {
		t.Fatalf("observer must be called for each retried error, got %d", len(observed))
	}
}

func TestRetryNetworkExhaustion(t *testing.T) {
	attempts := 0
	last := errors.New("socks connect ... connection reset by peer")
	var observed []error
	_, err := RetryNetwork(context.Background(), func(e error) { observed = append(observed, e) }, func() (int, error) {
		attempts++
		return 0, last
	})
	if !errors.Is(err, last) {
		t.Fatalf("exhausted retries must return the last error, got %v", err)
	}
	if attempts != SendRetryAttempts {
		t.Fatalf("want %d attempts, got %d", SendRetryAttempts, attempts)
	}
	if len(observed) != SendRetryAttempts {
		t.Fatalf("observer must be called for every network error, got %d", len(observed))
	}
}

func TestRetryNetworkNoRetryOnBusinessError(t *testing.T) {
	attempts := 0
	observed := 0
	_, err := RetryNetwork(context.Background(), func(error) { observed++ }, func() (int, error) {
		attempts++
		return 0, bot.ErrorForbidden
	})
	if !errors.Is(err, bot.ErrorForbidden) {
		t.Fatalf("business error must be returned as-is, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("business error must not be retried, got %d attempts", attempts)
	}
	if observed != 0 {
		t.Fatalf("observer must not be called for business errors, got %d", observed)
	}
}

func TestRetryNetworkCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := RetryNetwork(ctx, nil, func() (int, error) {
		return 0, errors.New("write tcp ... connection timed out")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx must abort the retry, got %v", err)
	}
}

func TestRetryNetworkNilObserver(t *testing.T) {
	if got, err := RetryNetwork(context.Background(), nil, func() (int, error) {
		return 7, nil
	}); err != nil || got != 7 {
		t.Fatalf("nil observer must be a no-op, got %v %v", got, err)
	}
}
