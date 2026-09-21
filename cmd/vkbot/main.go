package main

import (
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/app"
)

func main() {
	vkApp, err := app.NewVKApp(nil)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create VK app")
	}
	if err := vkApp.Run(); err != nil {
		log.Fatal().Err(err).Msg("VK app exited with error")
	}
}
