package main

import (
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/app"
	"github.com/azzimoda/raspishika-gx/internal/fakescraper"
)

func main() {

	vkApp, err := app.NewVKApp(fakescraper.ScraperAPI{})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create VK app")
	}
	if err := vkApp.Run(); err != nil {
		log.Fatal().Err(err).Msg("VK app exited with error")
	}
}
