package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

// JUSTRAY_PROXY_ADDR= is the documented way to run with the free-proxy source
// only. Without AllowEmptyEnv an explicitly empty variable resolves to the
// default, so justray was still used and the setting had no effect.
func TestAllowEmptyEnv(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.SetDefault(KeyJustrayProxyAddr, "127.0.0.1:10808")
	viper.AutomaticEnv() // as config.Init does; the env branch is skipped otherwise
	viper.AllowEmptyEnv(true)
	t.Setenv("JUSTRAY_PROXY_ADDR", "")

	if got := viper.GetString(KeyJustrayProxyAddr); got != "" {
		t.Fatalf("JUSTRAY_PROXY_ADDR= resolved to %q, want an empty string", got)
	}
}

func TestAllowEmptyEnvKeepsUnsetDefault(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.SetDefault(KeyJustrayProxyAddr, "127.0.0.1:10808")
	viper.AutomaticEnv()
	viper.AllowEmptyEnv(true)

	if got := viper.GetString(KeyJustrayProxyAddr); got != "127.0.0.1:10808" {
		t.Fatalf("unset variable resolved to %q, want the default", got)
	}
}

// The rotator probes justray from the host. Pointing it at the container
// address (host.docker.internal) makes every probe fail on DNS, and the
// rotator then rotates a healthy node every cooldown. Pin the loopback default
// so that mistake cannot come back unnoticed.
func TestJustrayProbeProxyDefaultsToLoopback(t *testing.T) {
	if prev, ok := os.LookupEnv("JUSTRAY_PROBE_PROXY"); ok {
		t.Setenv("JUSTRAY_PROBE_PROXY", prev) // restored at cleanup
		if err := os.Unsetenv("JUSTRAY_PROBE_PROXY"); err != nil {
			t.Fatalf("unset JUSTRAY_PROBE_PROXY: %v", err)
		}
	}
	viper.Reset()
	t.Cleanup(viper.Reset)

	Init()

	if got := viper.GetString(KeyJustrayProbeProxy); got != "127.0.0.1:10808" {
		t.Fatalf("JUSTRAY_PROBE_PROXY defaults to %q, want the loopback in-bound 127.0.0.1:10808", got)
	}
}
