package proxyutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPProxyClientDialRespectsContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept connections but never complete the SOCKS handshake, so the dial
	// blocks until the context is cancelled.
	done := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				close(done)
				return
			}
			defer conn.Close()
		}
	}()
	defer func() { ln.Close(); <-done }()

	client, err := NewHTTPProxyClient(ln.Addr().String())
	if err != nil {
		t.Fatalf("NewHTTPProxyClient() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/x", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error: %v", err)
	}

	start := time.Now()
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected an error from a hung proxy dial")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context error, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("dial took too long: %v", elapsed)
	}
}
