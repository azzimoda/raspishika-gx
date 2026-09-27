package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func rtEnabled(t *testing.T) {
	if os.Getenv("RT_SMOKE") != "1" {
		t.Skip("set RT_SMOKE=1 to run real justray/proxifly smoke test")
	}
}

func TestJustrayRuntimeSmoke(t *testing.T) {
	rtEnabled(t)
	viper.Set("JUSTRAY_PROXY_ADDR", "127.0.0.1:10808")
	viper.Set("PROXY_SOURCE_URL", "https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json")
	svc, _ := newProxyService()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	proxy, err := svc.FirstAvailable(ctx)
	if err != nil {
		t.Fatalf("FirstAvailable: %v", err)
	}
	t.Logf("selected proxy: %s", proxy)
	if proxy != "127.0.0.1:10808" {
		t.Fatalf("expected justray 127.0.0.1:10808, got %s", proxy)
	}
}

func TestJustrayFallbackSmoke(t *testing.T) {
	rtEnabled(t)
	viper.Set("JUSTRAY_PROXY_ADDR", "127.0.0.1:1")
	viper.Set("PROXY_SOURCE_URL", "https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json")
	svc, _ := newProxyService()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	proxy, err := svc.FirstAvailable(ctx)
	if err != nil {
		t.Fatalf("FirstAvailable: %v", err)
	}
	t.Logf("fallback selected proxy: %s", proxy)
	if proxy == "127.0.0.1:1" {
		t.Fatal("expected fallback, got justray")
	}
}
