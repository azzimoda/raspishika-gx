package config

import (
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
