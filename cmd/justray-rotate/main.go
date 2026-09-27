package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/internal/justrayrotate"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
)

func main() {
	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	proxyAddr := viper.GetString(config.KeyJustrayProxyAddr)
	probeURL := viper.GetString(config.KeyJustrayProbeURL)

	exclude := defaultExcludes()
	if extra := viper.GetString(config.KeyJustrayExclude); extra != "" {
		exclude = append(exclude, splitCSV(extra)...)
	}

	picker := justrayrotate.NewPicker(exclude...)
	runner := justrayrotate.CLIRunner{Bin: viper.GetString(config.KeyJustrayBin)}

	rotator := justrayrotate.NewRotator(justrayrotate.RotatorConfig{
		Probe:            justrayrotate.NewTelProbe(proxyAddr, probeURL, justrayrotate.DefaultProbeTimeout),
		CheckInterval:    viper.GetDuration(config.KeyJustrayCheckInterval),
		FailureThreshold: viper.GetInt(config.KeyJustrayFailureThreshold),
		Cooldown:         viper.GetDuration(config.KeyJustrayCooldown),
		MaxRotations:     viper.GetInt(config.KeyJustrayMaxRotations),
		LongBackoff:      viper.GetDuration(config.KeyJustrayLongBackoff),
	}, runner, picker)

	log.Info().
		Str("proxy", proxyAddr).
		Strs("exclude", exclude).
		Msg("Starting justray node rotator")

	if err := rotator.Run(ctx); err != nil {
		log.Fatal().Err(err).Msg("Rotator exited with error")
	}
	log.Info().Msg("Rotator stopped")
}

// defaultExcludes returns the substrings that make a node ineligible: Russian
// nodes (regional flag or name) plus the "For mobile operators" group, which
// routes via a Roskomnadzor-specific server.
func defaultExcludes() []string {
	return []string{"🇷🇺", "Россия", "моб. операторов"}
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
