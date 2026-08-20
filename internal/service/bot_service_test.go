package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func TestIsTelegramError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		apiCall bool
	}{
		{"telegram not found", bot.ErrorNotFound, true},
		{"telegram conflict", bot.ErrorConflict, true},
		{"telegram forbidden", bot.ErrorForbidden, true},
		{"telegram bad request", bot.ErrorBadRequest, true},
		{"telegram unauthorized", bot.ErrorUnauthorized, true},
		{"telegram too many requests", &bot.TooManyRequestsError{Message: "retry later"}, true},
		{"wrapped telegram error", fmt.Errorf("%w: chat not found", bot.ErrorNotFound), true},
		{"i/o timeout", errors.New("i/o timeout"), false},
		{"connection refused", errors.New("connection refused"), false},
		{"socks connect", errors.New("socks connect tcp 1.2.3.4:1080->api.telegram.org:443: connection refused"), false},
		{"context deadline", fmt.Errorf("error do request for method getUpdates: %w", context.DeadlineExceeded), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTelegramError(tc.err); got != tc.apiCall {
				t.Fatalf("isTelegramError(%v) = %v, want %v", tc.err, got, tc.apiCall)
			}
		})
	}
}

func TestHandleAPIErrorTriggersRestart(t *testing.T) {
	s := NewBotService(nil, nil)
	restarted := make(chan struct{}, 1)
	// Emulate the Start loop that is always blocked receiving from restartChan.
	go func() {
		for range s.restartChan {
			restarted <- struct{}{}
		}
	}()
	time.Sleep(5 * time.Millisecond) // let the drain goroutine park on receive

	s.handleAPIError(errors.New("i/o timeout"))

	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("handleAPIError with a network error should trigger a restart")
	}
}

func TestHandleAPIErrorSkipsRestartForTelegramErrors(t *testing.T) {
	s := NewBotService(nil, nil)
	restarted := make(chan struct{}, 1)
	go func() {
		for range s.restartChan {
			restarted <- struct{}{}
		}
	}()
	time.Sleep(5 * time.Millisecond)

	s.handleAPIError(bot.ErrorNotFound)

	select {
	case <-restarted:
		t.Fatal("handleAPIError with a Telegram API error should not restart")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestBotServiceStalled(t *testing.T) {
	s := NewBotService(nil, nil)
	if !s.stalled(time.Now()) {
		t.Fatal("fresh bot with no activity should be considered stalled")
	}

	s.Touch()
	if s.stalled(time.Now()) {
		t.Fatal("bot with recent activity should not be stalled")
	}

	if !s.stalled(time.Now().Add(proxyStallTimeout + time.Minute)) {
		t.Fatal("bot without activity for longer than proxyStallTimeout should be stalled")
	}
}
