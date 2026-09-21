package main

import (
	"github.com/azzimoda/raspishika-gx/internal/app"
	"github.com/azzimoda/raspishika-gx/internal/fakescraper"
	"github.com/rs/zerolog/log"
)

func main() {

	botApp, err := app.NewWithScraper(fakescraper.ScraperAPI{})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create app")
	}
	if err := botApp.Run(); err != nil {
		log.Fatal().Err(err).Msg("App exited with error")
	}
}
