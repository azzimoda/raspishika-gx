package main

import (
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func main() {
	// Initialize

	err := config.Init()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize config")
		return
	}

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open DB")
		return
	}
	defer db.Close()

	container, err := repository.NewContainer(db)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create container")
		return
	}

	services, err := service.NewServices(container)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create services")
		return
	}
	defer services.Stop()

	// Check services

	if err := services.HealthCheck(); err != nil {
		log.Fatal().Err(err).Msg("Health check failed")
		return
	}

	// Check migrations

	if info, err := database.GetMigrationsInfo(db, viper.GetString(config.KeyDBMigrationDir)); err != nil {
		log.Error().Err(err).Msg("Failed to get migrations info")
		return
	} else {
		log.Info().Any("migrations", info).Send()
	}

	log.Info().Msg("Health check passed")
}
