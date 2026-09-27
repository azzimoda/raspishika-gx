package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/azzimoda/go-tg-proxy/botservice"
	"github.com/azzimoda/go-tg-proxy/proxyutil"
	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/messenger"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	vkclient "github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbot "github.com/azzimoda/raspishika-gx/internal/vkbot/main"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/go-telegram/bot"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// vkShutdownTimeout bounds the whole graceful shutdown of the VK bot app.
const vkShutdownTimeout = 30 * time.Second

// NewVKApp builds the VK community bot app using the given scraper API client.
// When scraperAPI is nil, the default API client from configuration is used.
// Groupcha activities are reported to the Telegram admin bot when
// ADMIN_BOT_TOKEN and ADMIN_ID are configured, like the Telegram bot does.
func NewVKApp(scraperAPI service.APIClient) (*VKApp, error) {
	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	db, err := database.Open(config.DBConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	container := repository.NewContainer(db, model.PlatformVK)

	ctx, cancel := context.WithCancel(context.Background())

	if scraperAPI == nil {
		scraperAddr := fmt.Sprintf("%s:%s",
			viper.GetString(config.KeyScraperHost), viper.GetString(config.KeyScraperPort))
		scraperAPI = apiclient.New(scraperAddr)
	}
	services, err := service.NewServices(ctx, container, scraperAPI)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create services: %w", err)
	}

	groupID := viper.GetInt64(config.KeyVKGroupID)
	vkClient, err := vkclient.New(
		viper.GetString(config.KeyVKToken),
		groupID,
		viper.GetString(config.KeyVKAPIVersion),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create VK client: %w", err)
	}
	vkBot := vkbot.New(vkClient, services)

	appReporter := &AppReporter{Services: services}

	var adminReporterBot *botservice.BotService
	if viper.GetString(config.KeyAdminBotToken) != "" && viper.GetInt64(config.KeyAdminID) != 0 {
		adminReporterBot = botservice.NewBotService(
			func(p string, _ func()) (*bot.Bot, error) {
				httpClient, err := proxyutil.NewHTTPProxyClient(p)
				if err != nil {
					return nil, err
				}
				return bot.New(viper.GetString(config.KeyAdminBotToken),
					bot.WithHTTPClient(10*time.Second, httpClient),
					bot.WithCheckInitTimeout(10*time.Second),
				)
			},
			services.Proxy,
			botservice.WithSendOnly(),
		)
		appReporter.getBotUsername = func() string { return adminReporterBot.Username() }
	}

	broadcast := service.NewBroadcastService(messenger.NewVK(vkClient), services, appReporter)

	jobPoller := service.NewBroadcastJobPoller(broadcast, container.Job, model.PlatformVK)

	a := &VKApp{
		Ctx:              ctx,
		Cancel:           cancel,
		DB:               db,
		Services:         services,
		VKClient:         vkClient,
		VKGroupID:        groupID,
		VKBot:            vkBot,
		Broadcast:        broadcast,
		JobPoller:        jobPoller,
		AdminReporterBot: adminReporterBot,
		AppReporter:      appReporter,
	}
	return a, nil
}

// VKApp runs the VK community schedule bot: Bots Long Poll message handling,
// broadcast crons and the broadcast job worker, all scoped to model.PlatformVK.
type VKApp struct {
	Ctx              context.Context
	Cancel           context.CancelFunc
	DB               *gorm.DB
	Services         *service.Services
	VKClient         *vkclient.Client
	VKGroupID        int64
	VKBot            *vkbot.Bot
	Broadcast        *service.BroadcastService
	JobPoller        *service.BroadcastJobPoller
	AdminReporterBot *botservice.BotService
	*AppReporter
}

func (a *VKApp) Run() error {
	defer a.Cancel()

	log.Info().Msg("Starting VK app...")
	ctx, cancel := signal.NotifyContext(a.Ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var runErr error

	if err := a.Services.HealthCheck(); err != nil {
		runErr = fmt.Errorf("health check failed: %w", err)
	} else {
		a.Broadcast.Run(ctx, service.BroadcastConfig{
			Daily:            viper.GetBool("daily_broadcast"),
			PairNotification: viper.GetBool("pair_notification"),
			ChangeAlert:      viper.GetBool("change_alert"),
		})

		runErr = a.runBots(ctx, cancel)
	}

	return errors.Join(runErr, a.Stop())
}

// runBots starts the VK Long Poll loop, the broadcast job worker and the
// send-only admin bot, then waits for the shutdown signal. Returns nil on a
// graceful shutdown; a failing Long Poll loop returns its error.
func (a *VKApp) runBots(ctx context.Context, cancel context.CancelFunc) error {
	g, gctx := errgroup.WithContext(ctx)

	if a.AdminReporterBot != nil {
		g.Go(func() error {
			a.AdminReporterBot.Start(gctx)
			return nil
		})
	}
	g.Go(func() error {
		a.JobPoller.Run(gctx)
		return nil
	})
	g.Go(func() error {
		err := a.VKClient.Run(gctx, func(ctx context.Context, msg vkclient.Message) error {
			return a.VKBot.Handle(ctx, msg)
		})
		if gctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		cancel()
		return err
	})

	if !a.waitForReporterReady(gctx) {
		cancel()
		return g.Wait()
	}

	if a.AdminReporterBot != nil && a.AdminReporterBot.Bot != nil {
		adminID := viper.GetInt64(config.KeyAdminID)
		a.AppReporter.Reporter = reporter.NewReporter(a.AdminReporterBot.Bot, adminID)
	}
	a.Report().Msg(fmt.Sprintf(`Started on VK community <a href="https://vk.me/club%d">%d</a>`, a.VKGroupID, a.VKGroupID))

	<-gctx.Done()
	cancel()
	return g.Wait()
}

// waitForReporterReady blocks until the send-only admin bot is built so the
// startup report can be delivered. Returns false if the context is cancelled
// while waiting.
func (a *VKApp) waitForReporterReady(ctx context.Context) bool {
	if a.AdminReporterBot == nil {
		return true
	}
	for a.AdminReporterBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return false
		default:
		}
		log.Debug().Msg("Waiting for admin reporter bot...")
		time.Sleep(5 * time.Second)
	}
	log.Info().Msg("Admin reporter bot started")
	return true
}

func (a *VKApp) Stop() error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), vkShutdownTimeout)
	defer cancel()

	a.Broadcast.Stop(shutdownCtx)

	if a.AdminReporterBot != nil {
		a.AdminReporterBot.Stop()
	}

	errServices := a.Services.Stop()
	sqlDB, err := a.DB.DB()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to get database handle: %w", err), errServices)
	}
	errDB := sqlDB.Close()
	return errors.Join(errDB, errServices)
}
