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
	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	adminbot "github.com/azzimoda/raspishika-gx/internal/bot/admin"
	mainbot "github.com/azzimoda/raspishika-gx/internal/bot/main"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
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

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	container := repository.NewContainer(db)

	appReporter := new(AppReporter)

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

	mainBot := botservice.NewBotService(
		func(p string, onActivity func()) (*bot.Bot, error) {
			return mainbot.New(services, p, appReporter, onActivity)
		},
		services.Proxy,
	)
	broadcast := service.NewBroadcastService(mainBot, services, appReporter)
	adminBot := botservice.NewBotService(
		func(p string, onActivity func()) (*bot.Bot, error) {
			return adminbot.New(services, p, appReporter, broadcast, onActivity)
		},
		services.Proxy,
	)

	a := &App{
		Ctx:         ctx,
		Cancel:      cancel,
		DB:          db,
		Services:    services,
		Broadcast:   broadcast,
		MainBot:     mainBot,
		AdminBot:    adminBot,
		AppReporter: appReporter,
	}
	appReporter.App = a

	mainBot.OnRestart(a.OnMainBotRestart)
	adminBot.OnRestart(a.OnAdminBotRestart)

	return a, nil
}

type App struct {
	Ctx       context.Context
	Cancel    context.CancelFunc
	DB        *gorm.DB
	Services  *service.Services
	Broadcast *service.BroadcastService
	MainBot   *botservice.BotService
	AdminBot  *botservice.BotService
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

// runBots starts the bots, waits for the shutdown signal and joins the bot
// goroutines. It returns nil unless a bot goroutine fails.
func (a *App) runBots(ctx context.Context, cancel context.CancelFunc) error {

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		a.MainBot.Start(gctx)
		return nil
	})

	adminEnabled := viper.GetInt("admin_id") != 0
	if adminEnabled {
		if err := a.AdminBot.HealthCheck(); err != nil {
			log.Error().Err(err).Msg("Admin bot health check failed")
			adminEnabled = false
		} else {
			g.Go(func() error {
				a.AdminBot.Start(gctx)
				return nil
			})
		}
	} else {
		log.Debug().Msg("Admin bot is disabled")
	}

	if adminEnabled {
		if !a.waitForBotsReady(gctx) {
			cancel()
		} else {
			a.AppReporter.Reporter = reporter.NewReporter(a.AdminBot.Bot, viper.GetInt64(config.KeyAdminID))
			a.Report().Msg("Started on bot @" + mainbot.GetMe(a.MainBot.Bot).Username)
			<-gctx.Done()
			cancel()
		}
	} else {
		<-gctx.Done()
		cancel()
	}

	return g.Wait()
}

// waitForBotsReady blocks until both bots are built. Returns false if the
// context is cancelled while waiting.
func (a *App) waitForBotsReady(ctx context.Context) bool {

	for a.AdminBot.Bot == nil || a.MainBot.Bot == nil {
		select {
		case <-ctx.Done():
			log.Warn().Msg("Context cancelled!")
			return false
		default:
		}
		log.Debug().Msg("Waiting for bots...")
		time.Sleep(5 * time.Second)
	}
	time.Sleep(1 * time.Second)
	log.Info().Msg("All bots started")
	return true
}

func (a *App) Stop() error {

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	a.Broadcast.Stop(shutdownCtx)

	a.MainBot.Stop()
	if a.AdminBot != nil {
		a.AdminBot.Stop()
	}

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
		log.Debug().Msg("Wating for main bot...")
		time.Sleep(5 * time.Second)
	}

	a.Report().Msg("Main bot has just restarted")
}

func (a *App) OnAdminBotRestart(ctx context.Context) {

	if a.AdminBot.Bot != nil {
		a.Report().Msg("Admin bot is restarting...")
		time.Sleep(3 * time.Second)
	}

	for a.AdminBot.Bot == nil {
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

type AppReporter struct {
	App *App
	reporter.Reporter
}

func (r *AppReporter) Report() reporter.ReportBuilder {

	if r.Reporter == nil {
		return reporter.EmptyReportBuilder().WithFormatFunc(r.App.formatReport)
	}
	return r.Reporter.Report().WithFormatFunc(r.App.formatReport)
}
func (a *App) formatReport(msg string, debugValues map[string]any, err error) *bot.SendRichMessageParams {

	log.Trace().Str("msg", msg).Any("debugValues", debugValues).Msg("formatReport called")

	debugValues = maps.Clone(debugValues)

	ctx := context.Background()

	var html strings.Builder
	var buttons [][]models.InlineKeyboardButton

	// Chat
	chatID := extract[model.ChatID]("chatID", debugValues)
	delete(debugValues, "chatID")
	fullName := extract[string]("fullName", debugValues) // TODO: Set it in App.Report()
	delete(debugValues, "fullName")
	username := extract[string]("username", debugValues)
	delete(debugValues, "username")
	if chatID != 0 && a.AdminBot.Username() != "" {
		fmt.Fprintf(&html, "<p><b>Chat:</b> %s / @%s / <code>%d</code></p>\n", fullName, username, chatID)

		cmd := botutil.NewStartCommand("chat", strconv.FormatInt(int64(chatID), 10))
		url := MakeStartURL(a.AdminBot.Username(), cmd)
		buttons = append(buttons, []models.InlineKeyboardButton{{Text: "Get chat", URL: url}})
	}

	// Group
	groupName := extract[model.GroupName]("group", debugValues)
	delete(debugValues, "group")
	if groupName != "" {
		groupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		group, err := a.Services.Schedule.GetGroupByName(groupCtx, model.GroupName(groupName))
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
