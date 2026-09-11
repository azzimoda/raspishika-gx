package messenger

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot"
)

func TestSendWithRetrySucceedsAfterNetworkErrors(t *testing.T) {
	attempts := 0
	got, err := sendWithRetry(context.Background(), nil, nil, func() (int, error) {
		attempts++
		if attempts < 3 {
			return 0, errors.New("write tcp ... broken pipe")
		}
		return 42, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 42 || attempts != 3 {
		t.Fatalf("want 3 attempts and result 42, got attempts=%d result=%d", attempts, got)
	}
}

func TestSendWithRetryNoRetryOnBusinessError(t *testing.T) {
	attempts := 0
	hooked := false
	_, err := sendWithRetry(context.Background(), nil, func(error, string) { hooked = true }, func() (int, error) {
		attempts++
		return 0, bot.ErrorForbidden
	})
	if !errors.Is(err, bot.ErrorForbidden) {
		t.Fatalf("business error must be returned as-is, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("business error must not be retried, got %d attempts", attempts)
	}
	if hooked {
		t.Fatalf("business error must not trigger the failure hook")
	}
}

func TestSendWithRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := sendWithRetry(ctx, nil, nil, func() (int, error) {
		return 0, errors.New("write tcp ... connection timed out")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx must abort the retry, got %v", err)
	}
}

func TestSendWithRetryHookAfterExhaustion(t *testing.T) {
	attempts := 0
	var hooked error
	var hookedProxy string
	getProxy := func() string { return "169.58.97.115:1080" }
	_, err := sendWithRetry(context.Background(), getProxy, func(e error, p string) {
		hooked, hookedProxy = e, p
	}, func() (int, error) {
		attempts++
		return 0, errors.New("socks connect ... connection reset by peer")
	})
	if err == nil {
		t.Fatalf("all attempts must fail")
	}
	if hooked == nil {
		t.Fatalf("failure hook must fire after retries are exhausted")
	}
	if hookedProxy != "169.58.97.115:1080" {
		t.Fatalf("hook must carry the failing proxy, got %q", hookedProxy)
	}
}

func TestSendWithRetryHookFiresAfterNetworkErrorThenReconnect(t *testing.T) {
	attempts := 0
	var hooked error
	var hookedProxy string
	getProxy := func() string { return "169.58.97.115:1080" }
	_, err := sendWithRetry(context.Background(), getProxy, func(e error, p string) {
		hooked, hookedProxy = e, p
	}, func() (int, error) {
		attempts++
		if attempts == 1 {
			return 0, errors.New("write tcp ... connection timed out")
		}
		return 0, errBotNotConnected
	})
	if err == nil {
		t.Fatalf("all attempts must fail")
	}
	if hooked == nil {
		t.Fatalf("a genuine network error seen earlier must fire the hook even if later attempts only reconnect")
	}
	if hookedProxy != "169.58.97.115:1080" {
		t.Fatalf("hook must carry the failing proxy, got %q", hookedProxy)
	}
}

func TestSendWithRetryNoHookOnReconnectingOnly(t *testing.T) {
	attempts := 0
	var hooked error
	_, err := sendWithRetry(context.Background(), nil, func(e error, _ string) { hooked = e }, func() (int, error) {
		attempts++
		return 0, errBotNotConnected
	})
	if err == nil {
		t.Fatalf("all attempts must fail")
	}
	if hooked != nil {
		t.Fatalf("reconnecting bot must not be reported as a proxy failure, got %v", hooked)
	}
}
