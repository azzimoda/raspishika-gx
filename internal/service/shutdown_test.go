package service

import (
	"context"
	"testing"
	"time"
)

func TestBroadcastStopWithoutRun(t *testing.T) {

	b := NewBroadcastService(nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b.Stop(ctx)
	b.Stop(ctx)
}
