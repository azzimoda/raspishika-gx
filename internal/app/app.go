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
	"time"

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
	"gorm.io/gorm"
)

func New() (*App, error) {

	config.Init()

	logger.Init(viper.GetString(config.KeyLogLevel), viper.GetString(config.KeyLogDir))

	db, err := database.Open(viper.GetString(config.KeyDBFile), viper.GetString(config.KeyDBMigrationDir))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	container := repository.NewContainer(db)

	appReporter := new(AppReporter)

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
	MainBot   *service.BotService
	AdminBot  *service.BotService
	*AppReporter
}

func (a *App) Run() error {

	defer a.Cancel()

	log.Info().Msg("Starting app...")
	ctx, cancel := signal.NotifyContext(a.Ctx, os.Interrupt)
	defer cancel()

	// Services
	if err := a.Services.HealthCheck(); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	a.Broadcast.Run(ctx, service.BroadcastConfig{
		Daily:            viper.GetBool("daily_broadcast"),
		PairNotification: viper.GetBool("pair_notification"),
		ChangeAlert:      viper.GetBool("change_alert"),
	})

	// Bots
	if err := a.MainBot.HealthCheck(); err != nil {
		return fmt.Errorf("main bot health check failed: %w", err)
	}
	go a.MainBot.Start(ctx)

	if viper.GetInt("admin_id") != 0 {
		if err := a.AdminBot.HealthCheck(); err != nil {
			log.Error().Err(err).Msg("Admin bot health check failed")
		} else {
			shouldReturn := a.StartAdminBot(ctx)
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

func (a *App) StartAdminBot(ctx context.Context) bool {

	go a.AdminBot.Start(ctx)

	for a.AdminBot.Bot == nil || a.MainBot.Bot == nil {
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

	a.AppReporter.Reporter = reporter.NewReporter(a.AdminBot.Bot, viper.GetInt64(config.KeyAdminID))
	a.Report().Msg("Started on bot @" + mainbot.GetMe(a.MainBot.Bot).Username)
	return false
}

func (a *App) Stop() error {

	a.Broadcast.Stop()
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
		fmt.Fprintf(&html, "<b>Chat:</b> %s / @%s / <code>%d</code>\n", fullName, username, chatID)

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
			fmt.Fprintf(&html, "<b>Group:</b> %s — %s\n", group.GroupName, group.DepartmentName)
		}
	}

	// Other debug
	if len(debugValues) > 0 {
		fmt.Fprintf(&html, "<b>Other debug:</b>\n")
		keys := make([]string, 0, len(debugValues))
		for k := range debugValues {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		html.WriteString("<table border striped>")
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
	fmt.Fprintf(&html, "%s", msg)

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
