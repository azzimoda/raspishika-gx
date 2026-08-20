package service

import (
	"context"
	"testing"
	"time"
)

func TestBotServiceStopWithoutStart(t *testing.T) {

	s := NewBotService(nil, nil)
	s.Stop()
	s.Stop()
}

func TestBotServiceRestartAfterStop(t *testing.T) {

	s := NewBotService(nil, nil)
	s.Stop()
	s.Restart()
}

func TestBotServiceStartWithCancelledCtx(t *testing.T) {

	s := NewBotService(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after context was cancelled")
	}
	s.Stop()
}

func TestBroadcastStopWithoutRun(t *testing.T) {

	b := NewBroadcastService(nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b.Stop(ctx)
	b.Stop(ctx)
}
