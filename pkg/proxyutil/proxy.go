package proxyutil

import (
	"context"
	"net"
	"net/http"

	"golang.org/x/net/proxy"
)

func NewHTTPProxyClient(p string) (*http.Client, error) {
	dialer, err := proxy.SOCKS5("tcp", p, nil, proxy.Direct)
	if err != nil {
		return nil, err
	}
	httpTransport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
	}
	httpClient := &http.Client{Transport: httpTransport}
	return httpClient, nil
}
