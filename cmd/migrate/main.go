// Command migrate applies the pending database migrations once and exits.
//
// The bot, the admin bot and the VK bot all share one SQLite file. Each of them
// used to run goose on startup, so a deployment that started them together could
// have two or three processes applying the same migration at the same time: the
// busy timeout serialises the writes but not goose's version bookkeeping, and the
// process that loses fails on statements the winner already committed.
//
// Compose runs this as a one-shot service and the other services wait for it to
// exit successfully, so the migrations are applied exactly once per deploy.
package main

import (
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func main() {
	config.Init()
	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	// Always migrate: that is this command's only job, whatever the services
	// have been configured to do.
	cfg := config.DBConfig()
	cfg.AutoMigrate = true

	log.Info().
		Str("driver", cfg.Driver).
		Str("file", cfg.File).
		Str("migrations", cfg.MigrationsDir).
		Msg("Applying database migrations")

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to apply database migrations")
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get database handle")
	}
	if err := sqlDB.Close(); err != nil {
		log.Fatal().Err(err).Msg("Failed to close database")
	}

	log.Info().Msg("Database migrations applied")
}
