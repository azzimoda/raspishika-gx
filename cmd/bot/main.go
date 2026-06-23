package main

import (
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/app"
)

func main() {
	app, err := app.New()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create app")
	}
	if err := app.Run(); err != nil {
		log.Fatal().Err(err).Msg("App exited with error")
	}
}
