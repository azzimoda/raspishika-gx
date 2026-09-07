package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/azzimoda/go-tg-proxy/botservice"
	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/app"
	adminbot "github.com/azzimoda/raspishika-gx/internal/bot/admin"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// shutdownTimeout bounds the whole graceful shutdown of the admin bot.
const shutdownTimeout = 30 * time.Second

func main() {

	config.Init()
	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	adminID := viper.GetInt64(config.KeyAdminID)
	if adminID == 0 || viper.GetString(config.KeyAdminBotToken) == "" {
		log.Fatal().Msg("ADMIN_ID and ADMIN_BOT_TOKEN are required to run the admin bot")
	}

	db, err := database.Open(database.Config{
		Driver:        viper.GetString(config.KeyDBDriver),
		File:          viper.GetString(config.KeyDBFile),
		MigrationsDir: viper.GetString(config.KeyDBMigrationDir),
		Host:          viper.GetString(config.KeyDBHost),
		Port:          viper.GetString(config.KeyDBPort),
		User:          viper.GetString(config.KeyDBUser),
		Password:      viper.GetString(config.KeyDBPassword),
		Name:          viper.GetString(config.KeyDBName),
		SSLMode:       viper.GetString(config.KeyDBSSLMode),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open database")
	}

	// The admin bot monitors all platforms, so its repositories are not scoped
	// to a single platform.
	container := repository.NewContainer(db, "")

	scraperAddr := fmt.Sprintf("%s:%s",
		viper.GetString(config.KeyScraperHost), viper.GetString(config.KeyScraperPort))
	services, err := service.NewAdminServices(context.Background(), container, apiclient.New(scraperAddr))
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create services")
	}

	log.Info().Msg("Starting admin bot...")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Enqueues broadcast jobs instead of sending messages directly: the
	// per-platform schedule bots pick them up and deliver them.
	jobs := service.NewBroadcastJobs(container.Job, nil)

	var adminBot *bot.Bot
	appReporter := app.NewAppReporter(services, func() *bot.Bot { return adminBot })
	botService := botservice.NewBotService(
		func(proxy string, onActivity func()) (*bot.Bot, error) {
			adminBot, err = adminbot.New(services, proxy, appReporter, jobs, onActivity)
			return adminBot, err
		},
		services.Proxy,
	)
	botService.OnRestart(func(context.Context) {
		if adminBot != nil {
			appReporter.Reporter = reporter.NewReporter(adminBot, adminID)
		}
	})

	runErr := run(botService, ctx, cancel)

	services.Stop()

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get database handle")
	}
	if err := sqlDB.Close(); err != nil {
		log.Error().Err(err).Msg("Failed to close database")
	}

	if runErr != nil {
		log.Fatal().Err(runErr).Msg("Admin bot exited with error")
	}
}

// run starts the bot service, waits for the shutdown signal, then stops the
// bot. Returns an error when the bot goroutine fails.
func run(botService *botservice.BotService, ctx context.Context, cancel context.CancelFunc) error {

	if err := botService.HealthCheck(); err != nil {
		return fmt.Errorf("bot health check failed: %w", err)
	}

	botService.Start(ctx)

	for botService.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return errors.New("context cancelled while waiting for the admin bot")
		default:
		}
		log.Debug().Msg("Waiting for admin bot...")
		time.Sleep(5 * time.Second)
	}

	<-ctx.Done()
	botService.Stop()
	return nil
}
