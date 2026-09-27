package justrayrotate

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// startSocks5 runs a minimal SOCKS5 CONNECT proxy. The probe can only reach the
// test servers through it, which is what makes the redirect and transport
// behaviour observable end to end.
func startSocks5(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSocks5(conn)
		}
	}()
	return ln.Addr().String()
}

func serveSocks5(conn net.Conn) {
	defer conn.Close()

	// Greeting: version, method count, methods.
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return
	}
	if _, err := io.ReadFull(conn, make([]byte, int(head[1]))); err != nil {
		return
	}
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil { // no auth
		return
	}

	// Request: version, command, reserved, address type.
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[1] != 0x01 { // CONNECT only
		return
	}
	var host string
	switch req[3] {
	case 0x01: // IPv4
		raw := make([]byte, 4)
		if _, err := io.ReadFull(conn, raw); err != nil {
			return
		}
		host = net.IP(raw).String()
	case 0x03: // domain
		raw := make([]byte, 1)
		if _, err := io.ReadFull(conn, raw); err != nil {
			return
		}
		name := make([]byte, int(raw[0]))
		if _, err := io.ReadFull(conn, name); err != nil {
			return
		}
		host = string(name)
	default:
		return
	}
	rawPort := make([]byte, 2)
	if _, err := io.ReadFull(conn, rawPort); err != nil {
		return
	}
	port := int(rawPort[0])<<8 | int(rawPort[1])

	upstream, err := net.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		_, _ = conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	go func() { _, _ = io.Copy(upstream, conn) }()
	_, _ = io.Copy(conn, upstream)
}

// A probe that follows redirects ends up somewhere other than the Bot API. The
// Bot API root answers 302 to core.telegram.org, so a node able to reach
// core.telegram.org while blocked at the Bot API used to look healthy and was
// never rotated - the exact failure the rotator exists to fix.
func TestTelProbeDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer elsewhere.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer origin.Close()

	proxyAddr := startSocks5(t)

	probe, err := NewTelProbe(proxyAddr, origin.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// The 302 from the configured host still proves the host answered, so the
	// probe reports healthy.
	if err := probe(t.Context()); err != nil {
		t.Fatalf("probe through redirecting host: %v", err)
	}
	if n := redirected.Load(); n != 0 {
		t.Fatalf("probe followed the redirect and hit the other host %d time(s)", n)
	}
}

// Any HTTP status from the Bot API host means the node reaches it, matching how
// the bot's pool accepts the "not found" getMe returns for a fake token.
func TestTelProbeAcceptsErrorStatus(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()

	proxyAddr := startSocks5(t)
	probe, err := NewTelProbe(proxyAddr, api.URL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe(t.Context()); err != nil {
		t.Fatalf("404 from the API host should count as reachable: %v", err)
	}
}

// A node whose route is dead must be reported as such, or the rotator never
// rotates anything.
func TestTelProbeReportsUnreachable(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := api.URL
	api.Close() // nothing is listening now

	proxyAddr := startSocks5(t)
	probe, err := NewTelProbe(proxyAddr, addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := probe(t.Context()); err == nil {
		t.Fatal("probe reported a dead endpoint as reachable")
	}
}

// A missing proxy address used to yield a probe that always failed with
// "dial tcp: missing address", which would have looked like a permanently dead
// proxy rather than a configuration mistake.
func TestNewTelProbeRejectsEmptyAddress(t *testing.T) {
	if _, err := NewTelProbe("", DefaultProbeURL, time.Second); err == nil {
		t.Fatal("empty proxy address accepted")
	}
}

// The default must exercise the Bot API, not the host that redirects away.
func TestDefaultProbeURLIsBotAPI(t *testing.T) {
	if DefaultProbeURL != "https://api.telegram.org/bot0/getMe" {
		t.Fatalf("default probe URL = %q, want a Bot API method", DefaultProbeURL)
	}
}

func TestTelProbeRespectsContext(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer slow.Close()
	// Registered last, so it runs first and lets the handler return before
	// Close waits on it. The SOCKS5 relay keeps the client side of the
	// connection open, so the server never observes the cancellation itself.
	defer close(release)

	proxyAddr := startSocks5(t)
	probe, err := NewTelProbe(proxyAddr, slow.URL, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	if err := probe(ctx); err == nil {
		t.Fatal("probe ignored context cancellation")
	}
}
