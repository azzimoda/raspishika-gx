package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

// The .env warning is gated on container detection, so the detection itself is
// pinned: a false negative reintroduces the per-start warning in production, a
// false positive silences a real hint for anyone developing on the host.
func TestRunningInContainer(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "dockerenv")
	prev := dockerEnvPath
	dockerEnvPath = marker
	t.Cleanup(func() { dockerEnvPath = prev })

	if runningInContainer() {
		t.Fatal("runningInContainer() = true with no marker file, want false")
	}

	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if !runningInContainer() {
		t.Fatal("runningInContainer() = false with a marker file present, want true")
	}

	// Podman marks the container with $container instead of the file.
	if err := os.Remove(marker); err != nil {
		t.Fatalf("remove marker: %v", err)
	}
	t.Setenv("container", "podman")
	if !runningInContainer() {
		t.Fatal("runningInContainer() = false with $container set, want true")
	}
}

// A missing .env is expected in a container, where compose injects the
// configuration through `environment:`. Warning about it on every start was
// noise that also made `docker compose logs | grep -i error` useless as a smoke
// check, so the warning is gated on not being containerized.
func TestInitEnvWarningOnlyOutsideContainer(t *testing.T) {
	cases := []struct {
		name      string
		container bool
		wantWarn  bool
	}{
		{name: "container", container: true, wantWarn: false},
		{name: "host", container: false, wantWarn: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "dockerenv")
			if tc.container {
				if err := os.WriteFile(marker, nil, 0o600); err != nil {
					t.Fatalf("write marker: %v", err)
				}
			}
			prevPath := dockerEnvPath
			dockerEnvPath = marker
			t.Cleanup(func() { dockerEnvPath = prevPath })

			var buf bytes.Buffer
			prevLogger := log.Logger
			log.Logger = zerolog.New(&buf)
			t.Cleanup(func() { log.Logger = prevLogger })

			viper.Reset()
			t.Cleanup(viper.Reset)

			// The working directory is pkg/config, which has no .env, so the
			// load always fails and the warning branch is the one exercised.
			Init()

			if gotWarn := strings.Contains(buf.String(), ".env file not found"); gotWarn != tc.wantWarn {
				t.Fatalf("warning logged = %v, want %v; log output: %s", gotWarn, tc.wantWarn, buf.String())
			}
		})
	}
}
