package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/azzimoda/go-tg-proxy/botservice"
	"github.com/azzimoda/go-tg-proxy/proxyutil"
	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	mainbot "github.com/azzimoda/raspishika-gx/internal/bot/main"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/messenger"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/logger"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// New creates the app with the default API client from configuration.
func New() (*App, error) {
	return NewWithScraper(nil)
}

// NewWithScraper creates the app using the given scraper API client.
// If scraperAPI is nil, the default API client from configuration is used.
func NewWithScraper(scraperAPI service.APIClient) (*App, error) {

	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

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
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	container := repository.NewContainer(db, model.PlatformTelegram)

	appReporter := &AppReporter{}

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
	appReporter.Services = services

	mainBot := botservice.NewBotService(
		func(p string, onActivity func()) (*bot.Bot, error) {
			return mainbot.New(services, p, appReporter, onActivity)
		},
		services.Proxy,
	)
	broadcast := service.NewBroadcastService(messenger.NewTelegram(func() *bot.Bot { return mainBot.Bot }), services, appReporter)
	jobPoller := service.NewBroadcastJobPoller(broadcast, container.Job, model.PlatformTelegram)

	adminReporterBot := botservice.NewBotService(
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

	a := &App{
		Ctx:              ctx,
		Cancel:           cancel,
		DB:               db,
		Services:         services,
		Broadcast:        broadcast,
		JobPoller:        jobPoller,
		MainBot:          mainBot,
		AdminReporterBot: adminReporterBot,
		AppReporter:      appReporter,
	}

	mainBot.OnRestart(a.OnMainBotRestart)

	return a, nil
}

type App struct {
	Ctx              context.Context
	Cancel           context.CancelFunc
	DB               *gorm.DB
	Services         *service.Services
	Broadcast        *service.BroadcastService
	JobPoller        *service.BroadcastJobPoller
	MainBot          *botservice.BotService
	AdminReporterBot *botservice.BotService
	*AppReporter
}

// shutdownTimeout bounds the whole graceful shutdown of the app.
const shutdownTimeout = 30 * time.Second

func (a *App) Run() error {

	defer a.Cancel()

	log.Info().Msg("Starting app...")
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

		if err := a.MainBot.HealthCheck(); err != nil {
			runErr = fmt.Errorf("main bot health check failed: %w", err)
		} else {
			runErr = a.runBots(ctx, cancel)
		}
	}

	return errors.Join(runErr, a.Stop())
}

// runBots starts the main bot and the broadcast job worker, waits for the
// shutdown signal and joins the bot goroutines. Returns nil unless a bot
// goroutine fails.
func (a *App) runBots(ctx context.Context, cancel context.CancelFunc) error {

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		a.MainBot.Start(gctx)
		return nil
	})
	g.Go(func() error {
		a.AdminReporterBot.Start(gctx)
		return nil
	})
	g.Go(func() error {
		a.JobPoller.Run(gctx)
		return nil
	})

	if !a.waitForMainBotReady(gctx) {
		cancel()
		return g.Wait()
	}

	if a.AdminReporterBot.Bot != nil {
		adminID := viper.GetInt64(config.KeyAdminID)
		a.AppReporter.Reporter = reporter.NewReporter(a.AdminReporterBot.Bot, adminID)
		a.Report().Msg("Started on bot @" + mainbot.GetMe(a.MainBot.Bot).Username)
	} else {
		log.Warn().Msg("Admin reporter bot unavailable; operational reports go to logs only")
		a.Report().Msg("Started on bot @" + mainbot.GetMe(a.MainBot.Bot).Username)
	}

	<-gctx.Done()
	cancel()
	return g.Wait()
}

// waitForMainBotReady blocks until the main bot is built. Returns false if the
// context is cancelled while waiting.
func (a *App) waitForMainBotReady(ctx context.Context) bool {

	for a.MainBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return false
		default:
		}
		log.Debug().Msg("Waiting for main bot...")
		time.Sleep(5 * time.Second)
	}
	time.Sleep(1 * time.Second)
	log.Info().Msg("Main bot started")
	return true
}

func (a *App) Stop() error {

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	a.Broadcast.Stop(shutdownCtx)

	a.MainBot.Stop()
	a.AdminReporterBot.Stop()

	errServices := a.Services.Stop()
	sqlDB, err := a.DB.DB()
	if err != nil {
		return errors.Join(fmt.Errorf("failed to get database handle: %w", err), errServices)
	}
	errDB := sqlDB.Close()
	return errors.Join(errDB, errServices)
}

func (a *App) OnMainBotRestart(ctx context.Context) {

	a.Report().Msg("Main bot is restarting...")

	for a.MainBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return
		default:
		}
		log.Debug().Msg("Waiting for main bot...")
		time.Sleep(5 * time.Second)
	}

	a.Report().Msg("Main bot has just restarted")
}

type AppReporter struct {
	reporter.Reporter
	Services       *service.Services
	getBotUsername func() string
}

// NewAppReporter builds a lazy reporter for a non-main app process (currently
// the admin bot). It formats reports like the main app, but stays quiet until
// the bot connects.
func NewAppReporter(services *service.Services, getBotUsername func() string) *AppReporter {
	return &AppReporter{Services: services, getBotUsername: getBotUsername}
}

// Report returns a report builder that stays quiet until the bot connects —
// before that there is nothing to deliver the report through. Once connected,
// reports go to the configured recipient.
func (r *AppReporter) Report() reporter.ReportBuilder {

	format := NewFormatter(r.getBotUsername, r.Services)
	if r.Reporter == nil {
		return reporter.EmptyReportBuilder().WithFormatFunc(format)
	}
	return r.Reporter.Report().WithFormatFunc(format)
}

// NewFormatter builds the rich HTML formatter that renders bot reports (debug
// values, errors, the message and chat/group context). getBotUsername returns
// the username of the bot that serves the admin deep links (the admin bot) once
// it is connected, so the formatter points the "Get chat" buttons back at it.
func NewFormatter(getBotUsername func() string, services *service.Services) reporter.FormatFunc {

	return func(msg string, debugValues map[string]any, err error) *bot.SendRichMessageParams {

		log.Trace().Str("msg", msg).Any("debugValues", debugValues).Msg("formatReport called")

		debugValues = maps.Clone(debugValues)

		var html strings.Builder
		var buttons [][]models.InlineKeyboardButton

		var botUsername string
		if getBotUsername != nil {
			botUsername = getBotUsername()
		}

		// Chat
		chatID := extract[model.ChatID]("chatID", debugValues)
		delete(debugValues, "chatID")
		fullName := extract[string]("fullName", debugValues) // TODO: Set it in App.Report()
		delete(debugValues, "fullName")
		username := extract[string]("username", debugValues)
		delete(debugValues, "username")
		if chatID != 0 && botUsername != "" {
			fmt.Fprintf(&html, "<p><b>Chat:</b> %s / @%s / <code>%d</code></p>\n", fullName, username, chatID)

			cmd := botutil.NewStartCommand("chat", strconv.FormatInt(int64(chatID), 10))
			url := MakeStartURL(botUsername, cmd)
			buttons = append(buttons, []models.InlineKeyboardButton{{Text: "Get chat", URL: url}})
		}

		// Group
		groupName := extract[model.GroupName]("group", debugValues)
		delete(debugValues, "group")
		if groupName != "" && services != nil && services.Schedule != nil {
			groupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			group, err := services.Schedule.GetGroupByName(groupCtx, model.GroupName(groupName))
			if err != nil {
				log.Warn().Err(err).Msg("failed to get group by name")
			} else {
				fmt.Fprintf(&html, "<p><b>Group:</b> %s — %s</p>\n", group.GroupName, group.DepartmentName)
			}
		}

		// Other debug
		if len(debugValues) > 0 {
			keys := make([]string, 0, len(debugValues))
			for k := range debugValues {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			html.WriteString("<table border striped><caption>Debug</caption>")
			for _, k := range keys {
				fmt.Fprintf(&html, "<tr><td><b>%s:</b></td><td><code>%v</code></td></tr>", k, debugValues[k])
			}
			html.WriteString("</table>\n")
		}

		// Error
		if err != nil {
			fmt.Fprintf(&html, "<blockquote><b>Error:</b><br><code>%s</code></blockquote>\n", err.Error())
		}

		// Message text
		fmt.Fprintf(&html, "<p>%s</p>", msg)

		params := bot.SendRichMessageParams{RichMessage: models.InputRichMessage{HTML: html.String()}}
		if len(buttons) > 0 {
			params.ReplyMarkup = models.InlineKeyboardMarkup{InlineKeyboard: buttons}
		}
		return &params
	}
}

func extract[T any](key string, values map[string]any) T {
	var zero T
	anyValue, ok := values[key]
	if !ok {
		return zero
	}
	value, ok := anyValue.(T)
	if !ok {
		return zero
	}
	return value
}

func MakeStartURL(botUsername string, cmd *botutil.StartCommand) string {
	return "https://t.me/" + botUsername + "/?start=" + cmd.String()
}
