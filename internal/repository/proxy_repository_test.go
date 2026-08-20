package repository

import (
	"testing"
	"time"
)

const sampleProxies = `[
	{"protocol":"socks5","ip":"1.2.3.4","port":1080,"geolocation":{"country":"US"}},
	{"protocol":"socks5","ip":"5.6.7.8","port":1081,"geolocation":{"country":"DE"}},
	{"protocol":"socks5","ip":"9.9.9.9","port":1082,"geolocation":{"country":"RU"}},
	{"protocol":"http","ip":"7.7.7.7","port":8080,"geolocation":{"country":"US"}},
	{"protocol":"socks5","ip":"","port":1083,"geolocation":{"country":"US"}},
	{"protocol":"socks5","ip":"4.4.4.4","port":0,"geolocation":{"country":"US"}}
]`

func TestUpdateFromJSON(t *testing.T) {
	r := NewProxyRepository()
	if err := r.UpdateFromJSON([]byte(sampleProxies)); err != nil {
		t.Fatalf("UpdateFromJSON() error: %v", err)
	}

	all := r.All()
	want := []string{"1.2.3.4:1080", "5.6.7.8:1081"}
	if len(all) != len(want) {
		t.Fatalf("got %d proxies, want %d: %v", len(all), len(want), all)
	}
	for i, w := range want {
		if all[i] != w {
			t.Fatalf("proxies[%d] = %q, want %q", i, all[i], w)
		}
	}
	if r.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt should be set after a successful update")
	}
}

func TestUpdateFromJSONKeepsOldListOnError(t *testing.T) {
	r := NewProxyRepository()
	if err := r.UpdateFromJSON([]byte(sampleProxies)); err != nil {
		t.Fatalf("initial UpdateFromJSON() error: %v", err)
	}
	old := r.All()

	if err := r.UpdateFromJSON([]byte("not json{")); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
	if err := r.UpdateFromJSON([]byte(`[{"protocol":"http","ip":"1.1.1.1","port":8080}]`)); err == nil {
		t.Fatal("expected error when no usable proxies")
	}

	all := r.All()
	if len(all) != len(old) {
		t.Fatalf("list changed after failed update: got %v, want %v", all, old)
	}
}

func TestNextWrapAround(t *testing.T) {
	r := NewProxyRepository()
	if idx, proxy := r.Next(); proxy != "" || idx != 0 {
		t.Fatalf("empty repo Next() = (%d, %q), want (0, \"\")", idx, proxy)
	}
	if err := r.UpdateFromJSON([]byte(sampleProxies)); err != nil {
		t.Fatalf("UpdateFromJSON() error: %v", err)
	}

	n := len(r.All())
	seen := make(map[string]bool)
	for i := 0; i < n; i++ {
		_, proxy := r.Next()
		if proxy == "" || seen[proxy] {
			t.Fatalf("unexpected proxy on iteration %d: %q", i, proxy)
		}
		seen[proxy] = true
	}

	// Wraps around and revisits the first proxy.
	first := r.All()[0]
	idx, proxy := r.Next()
	if proxy != first {
		t.Fatalf("after wrap-around Next() = (%d, %q), want first proxy %q", idx, proxy, first)
	}
}

func TestUpdatedAtTTL(t *testing.T) {
	r := NewProxyRepository()
	if !r.UpdatedAt().IsZero() {
		t.Fatal("UpdatedAt should be zero for a fresh repo")
	}
	if err := r.UpdateFromJSON([]byte(sampleProxies)); err != nil {
		t.Fatalf("UpdateFromJSON() error: %v", err)
	}
	if time.Since(r.UpdatedAt()) > time.Second {
		t.Fatalf("UpdatedAt too old: %v", time.Since(r.UpdatedAt()))
	}
}
