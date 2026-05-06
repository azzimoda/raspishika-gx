package main

import (
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/app"
)

func main() {
	app, err := app.New()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create app")
	}
	app.Run()
}
