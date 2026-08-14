package redisdb

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestClient(t *testing.T) (*SmartClient, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client, err := New(&redis.Options{Addr: mr.Addr()})
	if err != nil {
		t.Fatalf("NewSmartClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func TestGetMissingKey(t *testing.T) {
	client, _ := newTestClient(t)

	data, fresh, exist := client.Get(context.Background(), "missing")
	if exist {
		t.Fatalf("exist = %v, want false", exist)
	}
	if fresh {
		t.Fatalf("fresh = %v, want false", fresh)
	}
	if data != nil {
		t.Fatalf("data = %v, want nil", data)
	}
}

func TestSetThenGet(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	if err := client.Set(ctx, "key", []byte("value"), time.Hour, 24*time.Hour); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	data, fresh, exist := client.Get(ctx, "key")
	if !exist {
		t.Fatal("exist = false, want true")
	}
	if !fresh {
		t.Fatal("fresh = false, want true")
	}
	if string(data) != "value" {
		t.Fatalf("data = %q, want value", data)
	}
}

func TestFreshExpiresBeforeData(t *testing.T) {
	client, mr := newTestClient(t)
	ctx := context.Background()

	if err := client.Set(ctx, "key", []byte("value"), time.Hour, 24*time.Hour); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	mr.FastForward(2 * time.Hour)

	data, fresh, exist := client.Get(ctx, "key")
	if !exist {
		t.Fatal("exist = false, want true (data TTL not expired)")
	}
	if fresh {
		t.Fatal("fresh = true, want false (fresh TTL expired)")
	}
	if string(data) != "value" {
		t.Fatalf("data = %q, want value", data)
	}
}

func TestBothKeysExpire(t *testing.T) {
	client, mr := newTestClient(t)
	ctx := context.Background()

	if err := client.Set(ctx, "key", []byte("value"), time.Hour, 24*time.Hour); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	mr.FastForward(25 * time.Hour)

	_, fresh, exist := client.Get(ctx, "key")
	if fresh {
		t.Fatal("fresh = true, want false")
	}
	if exist {
		t.Fatal("exist = true, want false after data TTL expired")
	}
}
