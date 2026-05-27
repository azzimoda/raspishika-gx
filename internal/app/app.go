package app

import (
	"context"
	"errors"
	"fmt"
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

	return &App{
		db:          db,
		services:    services,
		broadcast:   broadcast,
		mainBot:     mainBot,
		adminBot:    adminBot,
		appReporter: appReporter,
	}, nil
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
		for a.adminBot.Bot == nil {
			log.Debug().Msg("Wating for admin bot...")
			time.Sleep(1 * time.Second)
		}
		time.Sleep(1 * time.Second)
		log.Info().Msg("Admin bot started")
		a.appReporter.Reporter = reporter.NewReporter(a.adminBot.Bot, viper.GetInt64(config.KeyAdminID))
		a.Report().Msg("Started on bot @" + mainbot.GetMe(a.mainBot.Bot).Username)
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

type appReporter struct{ reporter.Reporter }

func (r appReporter) Report() reporter.ReportBuilder {
	if r.Reporter == nil {
		return reporter.EmptyReportConfig()
	}
	return r.Reporter.Report()
}
