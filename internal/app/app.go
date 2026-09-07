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
	appReporter.getBot = func() *bot.Bot { return mainBot.Bot }
	broadcast := service.NewBroadcastService(messenger.NewTelegram(func() *bot.Bot { return mainBot.Bot }), services, appReporter)
	jobPoller := service.NewBroadcastJobPoller(broadcast, container.Job, model.PlatformTelegram)

	a := &App{
		Ctx:         ctx,
		Cancel:      cancel,
		DB:          db,
		Services:    services,
		Broadcast:   broadcast,
		JobPoller:   jobPoller,
		MainBot:     mainBot,
		AppReporter: appReporter,
	}

	mainBot.OnRestart(a.OnMainBotRestart)

	return a, nil
}

type App struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	DB        *gorm.DB
	Services  *service.Services
	Broadcast *service.BroadcastService
	JobPoller *service.BroadcastJobPoller
	MainBot   *botservice.BotService
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
		a.JobPoller.Run(gctx)
		return nil
	})

	if !a.waitForMainBotReady(gctx) {
		cancel()
		return g.Wait()
	}

	// Operational reports go to ADMIN_ID from the admin bot's account: the
	// reporter below is a send-only ADMIN_BOT_TOKEN bot (no polling) running
	// through the same SOCKS5 stack as the main bot. Without the token or a
	// usable proxy reports fall back to logs only.
	rep, err := a.adminTokenReporter(gctx)
	if err != nil {
		log.Warn().Err(err).Msg("Admin reporter unavailable; operational reports go to logs only")
	} else if rep != nil {
		a.AppReporter.Reporter = rep
		a.Report().Msg("Started on bot @" + mainbot.GetMe(a.MainBot.Bot).Username)
	}

	<-gctx.Done()
	cancel()
	return g.Wait()
}

// adminTokenReporter builds the reporter that delivers operational reports to
// ADMIN_ID through ADMIN_BOT_TOKEN. The bot is never started: it only sends
// messages, so no getUpdates polling runs. Returns nil, nil when reporting is
// not configured (no ADMIN_BOT_TOKEN or ADMIN_ID).
func (a *App) adminTokenReporter(ctx context.Context) (reporter.Reporter, error) {
	token := viper.GetString(config.KeyAdminBotToken)
	adminID := viper.GetInt64(config.KeyAdminID)
	if token == "" || adminID == 0 {
		return nil, nil
	}

	proxyAddr, err := a.Services.Proxy.FirstAvailable(ctx)
	if err != nil {
		return nil, err
	}
	httpClient, err := proxyutil.NewHTTPProxyClient(proxyAddr)
	if err != nil {
		return nil, err
	}
	adminBot, err := bot.New(token, bot.WithHTTPClient(10*time.Second, httpClient))
	if err != nil {
		return nil, err
	}
	return reporter.NewReporter(adminBot, adminID), nil
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
	Services *service.Services
	getBot   func() *bot.Bot
}

// NewAppReporter builds a lazy reporter for a non-main app process (currently
// the admin bot). It formats reports like the main app, but stays quiet until
// the bot connects.
func NewAppReporter(services *service.Services, getBot func() *bot.Bot) *AppReporter {
	return &AppReporter{Services: services, getBot: getBot}
}

// Report returns a report builder that stays quiet until the bot connects —
// before that there is nothing to deliver the report through. Once connected,
// reports go to the configured recipient.
func (r *AppReporter) Report() reporter.ReportBuilder {

	format := NewFormatter(r.getBot, r.Services)
	if r.Reporter == nil {
		return reporter.EmptyReportBuilder().WithFormatFunc(format)
	}
	return r.Reporter.Report().WithFormatFunc(format)
}

// NewFormatter builds the rich HTML formatter that renders bot reports (debug
// values, errors, the message and chat/group context). getBot returns the
// connected bot once it exists, so the formatter also serves the deep links
// that point back to the bot.
func NewFormatter(getBot func() *bot.Bot, services *service.Services) reporter.FormatFunc {

	return func(msg string, debugValues map[string]any, err error) *bot.SendRichMessageParams {

		log.Trace().Str("msg", msg).Any("debugValues", debugValues).Msg("formatReport called")

		debugValues = maps.Clone(debugValues)

		var html strings.Builder
		var buttons [][]models.InlineKeyboardButton

		var botUsername string
		if getBot != nil {
			if b := getBot(); b != nil {
				if me := mainbot.GetMe(b); me != nil {
					botUsername = me.Username
				}
			}
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
