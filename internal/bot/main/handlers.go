package mainbot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
)

var ErrNoChatContext error = errors.New("failed to get chat from context")

func newHandler(s *service.Services, reporter reporter.Reporter) *handler {
	return &handler{Services: s, Reporter: reporter}
}

type handler struct {
	*service.Services
	reporter.Reporter
}

func (h *handler) registerHandlers(b *bot.Bot) {
	// Commands
	{
		registerCommandHandler(b, "start", h.handleCmdStart, h.checkRegularAccess)
		registerCommandHandler(b, "help", h.handleCmdHelp, h.checkRegularAccess)
		registerCommandHandler(b, "stop", h.handleCmdStop, h.checkConfigAccess)
		registerCommandHandler(b, "week", h.handleCmdWeek, h.checkRegularAccess)
		registerCommandHandler(b, "tomorrow", h.handleCmdTomorrow, h.checkRegularAccess)
		registerCommandHandler(b, "today", h.handleCmdToday, h.checkRegularAccess)
		registerCommandHandler(b, "teacher", h.handleCmdTeacher, h.checkRegularAccess)
		registerCommandHandler(b, "settings", h.handleCmdSettings, h.checkConfigAccess)
		registerCommandHandler(b, "access", h.handleCmdAccess, h.checkConfigAccess)
	}

	// Text commands
	{
		registerTextHandler(b, "неделя", h.handleCmdWeek, h.checkRegularAccess)
		registerTextHandler(b, "завтра", h.handleCmdTomorrow, h.checkRegularAccess)
		registerTextHandler(b, "сегодня", h.handleCmdToday, h.checkRegularAccess)
		registerTextHandler(b, "преподаватель", h.handleCmdTeacher, h.checkRegularAccess)
		registerTextHandler(b, "отмена", h.handleCmdCancel, h.checkRegularAccess)
	}

	// States
	{
		h.registerChatStateHandler(b, model.ChatStateSelectingGroup, h.handleTextGroup, h.checkConfigAccess)
		h.registerChatStateHandler(b, model.ChatStateSelectingTime, h.handleTextTime, h.checkConfigAccess)
		h.registerChatStateHandler(b, model.ChatStateSelectingTeacher, h.handleTextTeacherName, h.checkRegularAccess)
	}

	// Quick group text
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}

		chat, err := h.Chat.GetChatByChatID(context.Background(), model.ChatID(update.Message.Chat.ID))
		if err != nil {
			log.Error().Err(err).Msg("Failed to get chat by chat ID")
			return false
		}
		if state, _ := chat.GetState(); state == model.ChatStateSelectingGroup {
			return false
		}

		// Check the group name
		groupName := model.GroupName(update.Message.Text)
		if _, err := h.Schedule.ValidateGroupName(context.Background(), groupName); err == nil {
			return true
		} else {
			return false
		}
	}, h.handleTextQuickGroup, h.checkRegularAccess)

	// Callback queries
	{
		// Regular callbacks
		registerRegularCallbackHandler := func(callbackCommand string, handler bot.HandlerFunc) {
			b.RegisterHandlerRegexp(bot.HandlerTypeCallbackQueryData, callbackDataRegexp(callbackCommand),
				handler, h.checkRegularAccess)
		}

		registerRegularCallbackHandler(botutil.CallbackCommandDelete, h.handleCQDelete)
		registerRegularCallbackHandler(botutil.CallbackCommandSelectTeacher, h.handleCQTeacher)
		registerRegularCallbackHandler(botutil.CallbackCommandUpdateGroup, h.handleCQUpdateGroup)
		registerRegularCallbackHandler(botutil.CallbackCommandUpdateTeacher, h.handleCQUpdateTeacher)
		registerRegularCallbackHandler(botutil.CallbackCommandUpdateTomorrow, h.handleCQUpdateTomorrow)
		registerRegularCallbackHandler(botutil.CallbackCommandUpdateToday, h.handleCQUpdateToday)

		// Config callbacks
		registerConfigCallbackHandler := func(callbackCommand string, handler bot.HandlerFunc) {
			b.RegisterHandlerRegexp(bot.HandlerTypeCallbackQueryData, callbackDataRegexp(callbackCommand),
				handler, h.checkConfigAccess)
		}

		registerConfigCallbackHandler(botutil.CallbackCommandDeleteConfig, h.handleCQDelete)
		registerConfigCallbackHandler(botutil.CallbackCommandSelectDepartment, h.handleCQSelectDepartment)
		registerConfigCallbackHandler(botutil.CallbackCommandConfigGroup, h.handleCQConfigGroup)
		registerConfigCallbackHandler(botutil.CallbackCommandConfigDailyTime, h.handleCQConfigDailyTime)
		registerConfigCallbackHandler(botutil.CallbackCommandDailyOff, h.handleCQDailyOff)
		registerConfigCallbackHandler(botutil.CallbackCommandConfigReminder, h.handleCQConfigReminder)
		registerConfigCallbackHandler(botutil.CallbackCommandConfigChange, h.handleCQConfigChange)
		registerConfigCallbackHandler(botutil.CallbackCommandConfigDarkMode, h.handleCQConfigDarkMode)
		registerConfigCallbackHandler(botutil.CallbackCommandSetAccess, h.handleCQSetAccess)
	}
}

func registerCommandHandler(b *bot.Bot, pattern string, f bot.HandlerFunc, m ...bot.Middleware) string {
	return b.RegisterHandlerMatchFunc(commandMatchFunc(pattern, GetMe(b).Username), f, m...)
}
func commandMatchFunc(pattern string, username string) bot.MatchFunc {
	re := regexp.MustCompile(fmt.Sprintf(`^/%s(@\w+)?(\s[\s\S]+)?$`, pattern))
	return func(update *models.Update) bool {
		return matchUpdatePatternUsername(update, username, re)
	}
}
func matchUpdatePatternUsername(update *models.Update, username string, re *regexp.Regexp) bool {
	if update.Message == nil || update.Message.Text == "" {
		return false // Not a text message.
	}

	text := update.Message.Text

	if !re.MatchString(text) {
		return false
	}
	submatches := re.FindStringSubmatch(text)
	if submatches == nil {
		return true
	}

	if update.Message.Chat.Type == models.ChatTypeGroup || update.Message.Chat.Type == models.ChatTypeSupergroup {
		if submatches[1] == "" {
			// In group chats command without username are sent to last accessed bot.
			// That means, if this bot is not the last accessed, it won't receive that message.
			// Otherwise, it will receive it and should handle it.
			return true
		}
		return submatches[1] == "@"+username
	}

	// In private chat any username is allowed.
	return true
}

func registerTextHandler(b *bot.Bot, pattern string, f bot.HandlerFunc, m ...bot.Middleware) string {
	re := regexp.MustCompile(fmt.Sprintf("(?i)^%s$", pattern))
	return b.RegisterHandlerRegexp(bot.HandlerTypeMessageText, re, f, m...)
}

func (h *handler) registerChatStateHandler(b *bot.Bot, state model.ChatState, f bot.HandlerFunc, m ...bot.Middleware) string {
	matchFunc := func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}

		chat, err := h.Chat.GetChatByChatID(context.Background(), model.ChatID(update.Message.Chat.ID))
		chatState := model.ChatStateDefault
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get chat to match chat state; interpreting as default")
		} else {
			chatState, _ = chat.GetState()
		}

		log.Trace().Any("chat", chat).Any("state", chatState).Any("target", state).Msg("Matching chat state...")
		return chatState == state
	}
	return b.RegisterHandlerMatchFunc(matchFunc, f, m...)
}

func callbackDataRegexp(s string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf("^%s(\n.*)*$", s))
}

func (h *handler) handleDefault(ctx context.Context, b *bot.Bot, update *models.Update) {
	setNoLogFlag(ctx) // Do not log default handler

	if update.Message != nil {
		log.Debug().Str("text", update.Message.Text).Msg("Unhandled message")
	} else if update.CallbackQuery != nil {
		log.Debug().Str("data", update.CallbackQuery.Data).Msg("Unhandled callback query")
	} else {
		log.Debug().Any("update", update).Msg("Unhandled update type")
	}
}

// handleVacationCommand handles command specially when today is vacation, and return true if it is.
func (h *handler) handleVacationCommand(ctx context.Context, b *bot.Bot, update *models.Update, kind string) bool {
	if !viper.GetBool(config.KeyHandleVacation) {
		return false
	}

	now := time.Now()
	isVacation := false
	switch kind {
	case "week":
		isVacation = IsVacation(now) && IsVacation(now.AddDate(0, 0, 6))
	case "today":
		isVacation = IsVacation(now)
	case "tomorrow":
		isVacation = IsVacation(now.AddDate(0, 0, 1))
	}

	if !isVacation {
		return false
	}
	log.Info().Msg("Today is vacation, handling...")

	setNoLogFlag(ctx)

	dur := time.Until(time.Date(now.Year(), time.September, 1, 0, 0, 0, 0, now.Location()))
	days := int(dur.Hours()) / 24
	text := fmt.Sprintf("До конца каникул осталось %d дней", days)
	if m := update.Message; m != nil {
		err := botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
			ChatID:          m.Chat.ID,
			MessageThreadID: m.MessageThreadID,
			Text:            text,
		})
		addHandlerCtxErr(ctx, err)

		_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: m.Chat.ID, MessageID: m.ID})
		addHandlerCtxErr(ctx, err)
	} else if cq := update.CallbackQuery; cq != nil {
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cq.ID,
			Text:            text,
			CacheTime:       3600, // 1 hour
		})
		addHandlerCtxErr(ctx, err)
	}

	return true
}

func setNoLogFlag(ctx context.Context) {
	noLogFlag, ok := ctx.Value(keyNoLogFlag).(*bool)
	if ok {
		*noLogFlag = true
	} else {
		log.Warn().Msg("Failed to set no-log handler flag")
	}
}

var me *models.User
var meOnce sync.Once

func GetMe(b *bot.Bot) *models.User {
	meOnce.Do(func() {
		if err := retry.New(retry.Attempts(5), retry.Delay(100*time.Millisecond)).Do(func() (err error) {
			me, err = b.GetMe(context.Background())
			return
		}); err != nil {
			log.Error().Err(err).Msg("Failed to get me")
		}
	})
	return me
}
