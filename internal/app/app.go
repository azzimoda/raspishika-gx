package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"gorm.io/gorm"

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
	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	container := repository.NewContainer(db)

	appReporter := new(appReporter)

	ctx, cancel := context.WithCancel(context.Background())

	services, err := service.NewServices(ctx, container)
	if err != nil {
		cancel()
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
		ctx:         ctx,
		cancel:      cancel,
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
	ctx       context.Context
	cancel    context.CancelFunc
	db        *gorm.DB
	services  *service.Services
	broadcast *service.BroadcastService
	mainBot   *service.BotService
	adminBot  *service.BotService
	*appReporter
}

func (a *App) Run() error {
	defer a.cancel()

	log.Info().Msg("Starting app...")
	ctx, cancel := signal.NotifyContext(a.ctx, os.Interrupt)
	defer cancel()

	// Services
	if err := a.services.HealthCheck(); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	a.broadcast.Run(ctx, service.BroadcastConfig{
		Daily:            viper.GetBool("daily_broadcast"),
		PairNotification: viper.GetBool("pair_notification"),
		ChangeAlert:      viper.GetBool("change_alert"),
	})

	// Bots
	if err := a.mainBot.HealthCheck(); err != nil {
		return fmt.Errorf("main bot health check failed: %w", err)
	}
	go a.mainBot.Start(ctx)

	if viper.GetInt("admin_id") != 0 {
		if err := a.adminBot.HealthCheck(); err != nil {
			log.Error().Err(err).Msg("Admin bot health check failed")
		} else {
			shouldReturn := a.startAdminBot(ctx)
			if shouldReturn {
				return a.Stop()
			}
		}
	} else {
		log.Debug().Msg("Admin bot is disabled")
	}

	<-ctx.Done()

	return a.Stop()
}

func (a *App) startAdminBot(ctx context.Context) bool {
	go a.adminBot.Start(ctx)
	for a.adminBot.Bot == nil || a.mainBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return true
		default:
		}
		log.Debug().Msg("Wating for bots...")
		time.Sleep(5 * time.Second)
	}
	time.Sleep(1 * time.Second)
	log.Info().Msg("All bots started")

	a.appReporter.Reporter = reporter.NewReporter(a.adminBot.Bot, viper.GetInt64(config.KeyAdminID))
	a.Report().Msg("Started on bot @" + mainbot.GetMe(a.mainBot.Bot).Username)
	return false
}

func (a *App) Stop() error {
	a.broadcast.Stop()
	errServices := a.services.Stop()
	sqlDB, err := a.db.DB()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to get database handle: %w", err), errServices)
	}
	errDB := sqlDB.Close()
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
