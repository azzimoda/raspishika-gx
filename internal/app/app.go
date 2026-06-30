package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/go-telegram/bot"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	adminbot "github.com/azzimoda/raspishika-gx/internal/bot/admin"
	mainbot "github.com/azzimoda/raspishika-gx/internal/bot/main"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
)

func New() (*App, error) {
	if err := config.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	container, err := repository.NewContainer(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	appReporter := new(appReporter)

	services, err := service.NewServices(container)
	if err != nil {
		return nil, fmt.Errorf("failed to create services: %w", err)
	}

	mainBot := service.NewBotService(
		func(p string) (*bot.Bot, error) { return mainbot.New(services, p, appReporter) },
		services.Proxy,
	)
	adminBot := service.NewBotService(
		func(p string) (*bot.Bot, error) { return adminbot.New(services, p, appReporter) },
		services.Proxy,
	)

	broadcast := service.NewBroadcastService(mainBot, services, appReporter)

	a := &App{
		db:          db,
		services:    services,
		broadcast:   broadcast,
		mainBot:     mainBot,
		adminBot:    adminBot,
		appReporter: appReporter,
	}

	mainBot.OnRestart(a.onMainBotRestart)
	adminBot.OnRestart(a.onAdminBotRestart)

	return a, nil
}

type App struct {
	db        *sqlx.DB
	services  *service.Services
	broadcast *service.BroadcastService
	mainBot   *service.BotService
	adminBot  *service.BotService
	*appReporter
}

func (a *App) Run() error {
	log.Info().Msg("Starting app...")
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Services
	if err := a.services.HealthCheck(); err != nil {
		log.Fatal().Err(err).Msg("Health check failed")
		return err
	}
	a.broadcast.Run(ctx, service.BroadcastConfig{
		Daily:            viper.GetBool("daily_broadcast"),
		PairNotification: viper.GetBool("pair_notification"),
		ChangeAlert:      viper.GetBool("change_alert"),
	})

	// Bots
	if err := a.mainBot.HealthCheck(); err != nil {
		log.Fatal().Err(err).Msg("Main bot health check failed")
		return err
	}
	go a.mainBot.Start(ctx)

	if viper.GetInt("admin_id") != 0 {
		if err := a.adminBot.HealthCheck(); err != nil {
			log.Error().Err(err).Msg("Admin bot health check failed")
		} else {
			go a.adminBot.Start(ctx)
			for a.adminBot.Bot == nil || a.mainBot.Bot == nil {
				select {
				case <-ctx.Done():
					log.Warn().Msg("Context cancelled!")
					return nil
				default:
				}
				log.Debug().Msg("Wating for bots...")
				time.Sleep(5 * time.Second)
			}
			time.Sleep(1 * time.Second)
			log.Info().Msg("All bots started")
			a.appReporter.Reporter = reporter.NewReporter(a.adminBot.Bot, viper.GetInt64(config.KeyAdminID))
			a.Report().Msg("Started on bot @" + mainbot.GetMe(a.mainBot.Bot).Username)
		}
	} else {
		log.Debug().Msg("Admin bot is disabled")
	}

	<-ctx.Done()

	// TODO: Graceful shutdown

	return nil
}

func (a *App) Stop() error {
	a.broadcast.Stop()
	errServices := a.services.Stop()
	errDB := a.db.Close()
	return errors.Join(errDB, errServices)
}

func (a *App) onMainBotRestart(ctx context.Context) {
	a.Report().Msg("Main bot is restarting...")

	for a.mainBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return
		default:
		}
		log.Debug().Msg("Wating for main bot...")
		time.Sleep(5 * time.Second)
	}

	a.Report().Msg("Main bot has just restarted")
}

func (a *App) onAdminBotRestart(ctx context.Context) {
	if a.adminBot.Bot != nil {
		a.Report().Msg("Admin bot is restarting...")
		time.Sleep(3 * time.Second)
	}

	for a.adminBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return
		default:
		}
		log.Debug().Msg("Wating for admin bot...")
		time.Sleep(5 * time.Second)
	}

	a.Report().Msg("Admin bot has just restarted")
}

type appReporter struct{ reporter.Reporter }

func (r appReporter) Report() reporter.ReportBuilder {
	if r.Reporter == nil {
		return reporter.EmptyReportBuilder()
	}
	return r.Reporter.Report()
}
