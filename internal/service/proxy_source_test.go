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

	t.Run("fallback error keeps justray only", func(t *testing.T) {
		src := newJustrayFirstSource("127.0.0.1:10808", fakeSource{err: errors.New("boom")})
		addrs, err := src.Fetch(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"127.0.0.1:10808"}; !reflect.DeepEqual(addrs, want) {
			t.Fatalf("got %v, want %v", addrs, want)
		}
	})

	var _ proxy.Source = justrayFirstSource{}
}
