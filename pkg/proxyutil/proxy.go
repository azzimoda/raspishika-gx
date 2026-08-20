package proxyutil

import (
	"context"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/proxy"
)

const dialTimeout = 5 * time.Second

func NewHTTPProxyClient(p string) (*http.Client, error) {
	// The forward dialer bounds the TCP connect to the proxy itself even when
	// the request context has no deadline (e.g. the getUpdates long poll).
	dialer, err := proxy.SOCKS5("tcp", p, nil, &net.Dialer{Timeout: dialTimeout})
	if err != nil {
		return nil, err
	}
	ctxDialer, _ := dialer.(proxy.ContextDialer)
	httpTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if ctxDialer != nil {
				return ctxDialer.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
		TLSHandshakeTimeout: dialTimeout,
	}
	httpClient := &http.Client{Transport: httpTransport}
	return httpClient, nil
}
