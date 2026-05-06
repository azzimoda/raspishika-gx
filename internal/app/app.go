package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	adminbot "github.com/azzimoda/raspishika-gx/internal/bot/admin"
	mainbot "github.com/azzimoda/raspishika-gx/internal/bot/main"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/azzimoda/raspishika-gx/pkg/reporter"
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

	appReporter := new(AppReporter)

	services, err := service.NewServices(container)
	if err != nil {
		return nil, fmt.Errorf("failed to create services: %w", err)
	}

	mainBot := service.NewBotService(
		func(p string) (*bot.Bot, error) { return mainbot.New(services, p, appReporter) },
		services.Proxy,
	)

	broadcast := service.NewBroadcastService(mainBot.Bot, services)

	adminBot := service.NewBotService(
		func(p string) (*bot.Bot, error) { return adminbot.New(services, p, appReporter) },
		services.Proxy,
	)

	appReporter.Reporter = reporter.NewReporter(adminBot.Bot, viper.GetInt64(config.KeyAdminID))

	return &App{
		db:        db,
		services:  services,
		broadcast: broadcast,
		mainBot:   mainBot,
		adminBot:  adminBot,
		Reporter:  appReporter,
	}, nil
}

type App struct {
	db        *sqlx.DB
	services  *service.Services
	broadcast *service.BroadcastService
	mainBot   *service.BotService
	adminBot  *service.BotService
	reporter.Reporter
}

func (a *App) Run() error {
	log.Info().Msg("Starting app...")
	ctx := context.Background()

	// Services
	a.broadcast.Run(ctx, service.BroadcastConfig{
		Daily:            viper.GetBool("daily_broadcast"),
		PairNotification: viper.GetBool("pair_notification"),
		ChangeAlert:      viper.GetBool("change_alert"),
	})

	// Bots
	go a.mainBot.Start(ctx)
	log.Info().Msg("Main bot started")

	if viper.GetInt("admin_id") != 0 {
		go a.adminBot.Start(ctx)
		log.Info().Msg("Admin bot started")
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

type AppReporter struct{ reporter.Reporter }

func (r AppReporter) Report() reporter.ReportBuilder {
	if r.Reporter == nil {
		return reporter.EmptyReportConfig()
	}
	return r.Reporter.Report()
}
