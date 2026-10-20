package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/azzimoda/go-tg-proxy/proxy"
)

type fakeSource struct {
	addrs []string
	err   error
}

func (f fakeSource) Fetch(context.Context) ([]string, error) { return f.addrs, f.err }

func TestJustrayFirstSource(t *testing.T) {
	ctx := context.Background()

	t.Run("empty addr returns fallback as-is", func(t *testing.T) {
		fallback := fakeSource{addrs: []string{"a:1", "b:2"}}
		got := newJustrayFirstSource("", fallback)
		if _, ok := got.(fakeSource); !ok {
			t.Fatalf("expected fallback source, got %T", got)
		}
	})

	t.Run("justray first, fallback appended", func(t *testing.T) {
		src := newJustrayFirstSource("127.0.0.1:10808", fakeSource{addrs: []string{"a:1", "b:2"}})
		addrs, err := src.Fetch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"127.0.0.1:10808", "a:1", "b:2"}
		if !reflect.DeepEqual(addrs, want) {
			t.Fatalf("got %v, want %v", addrs, want)
		}
	})

	t.Run("dedupe justray from fallback", func(t *testing.T) {
		src := newJustrayFirstSource("127.0.0.1:10808", fakeSource{addrs: []string{"127.0.0.1:10808", "a:1"}})
		addrs, err := src.Fetch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"127.0.0.1:10808", "a:1"}
		if !reflect.DeepEqual(addrs, want) {
			t.Fatalf("got %v, want %v", addrs, want)
		}
	})

	// A failed fallback with justray configured degrades to a justray-only
	// list rather than failing: the checker validates justray and the revalidate
	// loop retries the fallback on every cycle.
	t.Run("fallback error with justray degrades to justray-only", func(t *testing.T) {
		src := newJustrayFirstSource("127.0.0.1:10808", fakeSource{err: errors.New("boom")})
		addrs, err := src.Fetch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"127.0.0.1:10808"}
		if !reflect.DeepEqual(addrs, want) {
			t.Fatalf("got %v, want %v", addrs, want)
		}
	})

	// Without justray, a failed fallback is still propagated so a transient
	// upstream failure retries on the next call instead of caching an empty list.
	t.Run("fallback error without justray is propagated", func(t *testing.T) {
		src := newJustrayFirstSource("", fakeSource{err: errors.New("boom")})
		addrs, err := src.Fetch(ctx)
		if err == nil {
			t.Fatal("expected an error so the pool does not cache a degraded list")
		}
		if addrs != nil {
			t.Fatalf("addrs = %v, want nil alongside the error", addrs)
		}
	})

	var _ proxy.Source = justrayFirstSource{}
}
