package mainbot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

func (h *handler) handleCmdWeek(ctx context.Context, b *bot.Bot, update *models.Update) {

	log.Debug().Msg("Handling command week...")

	threadID := update.Message.MessageThreadID
	chatID := update.Message.Chat.ID
	stop := sendChatActionTyping(ctx, b, chatID, threadID)
	defer stop()

	chat, ok := getCtxChat(ctx)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, *chat.GroupName)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			h.resetChatForExpiredGroup(ctx, b, chat, chatID, threadID)
			return
		}

		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}
	log.Trace().Str("name", string(group.GroupName)).Msg("Got group")

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.SchedulePageURL(conf, nil)),
		})
		return
	}
	log.Trace().Msg("Got schedule")

	setGroupOrTeacherAndCached(ctx, string(*chat.GroupName), schedule.IsOld)

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare schedule image")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.SchedulePageURL(conf, nil)),
		})
		return
	}
	log.Trace().Msg("Prepared schedule image")

	err = botutil.SendWeekSchedule(ctx, b, threadID, chat, conf, schedule.Days, imageFilename, imageData, botutil.SchedulePageURL(conf, nil), schedule.IsOld)
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled command week")
}

func (h *handler) handleCmdTomorrow(ctx context.Context, b *bot.Bot, update *models.Update) {

	log.Debug().Msg("Handling command tomorrow...")

	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID
	stop := sendChatActionTyping(ctx, b, chatID, threadID)
	defer stop()

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, *chat.GroupName)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			h.resetChatForExpiredGroup(ctx, b, chat, chatID, threadID)
			return
		}

		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.SchedulePageURL(conf, nil)),
		})
		return
	}

	setGroupOrTeacherAndCached(ctx, string(*chat.GroupName), schedule.IsOld)

	tomorrow := schedule.Tomorrow(time.Now())

	idx := 1
	if time.Now().Weekday() == time.Sunday {
		idx = 0
	}
	text := formatDayHTML(conf.Name(), tomorrow)
	inlineMarkup := dayMarkup(conf, schedule.Days, idx, botutil.SchedulePageURL(conf, nil))
	_, err = botutil.SendMessageWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup:     inlineMarkup,
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled command tomorrow")
}

func (h *handler) handleCmdToday(ctx context.Context, b *bot.Bot, update *models.Update) {

	log.Debug().Msg("Handling command today...")

	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID
	stop := sendChatActionTyping(ctx, b, chatID, threadID)
	defer stop()

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, *chat.GroupName)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			h.resetChatForExpiredGroup(ctx, b, chat, chatID, threadID)
			return
		}

		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	conf := model.GroupScheduleConfig(group, chat.DarkMode)

	// If today is Sunday, send a special message
	if time.Now().Weekday() == time.Sunday {
		_, err := botutil.SendMessageWithRetry(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            "Сегодня воскресенье, отдыхайте!",
			ReplyMarkup:     botutil.SimpleUpdateMarkup(botutil.UpdateKindToday, string(group.GroupName), botutil.SchedulePageURL(conf, nil)),
		})
		addHandlerCtxErr(ctx, err)
		return
	}
	// Otherwise, send today's schedule

	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.SchedulePageURL(conf, nil)),
		})
		return
	}

	setGroupOrTeacherAndCached(ctx, string(*chat.GroupName), schedule.IsOld)

	today := schedule.Today()

	text := formatDayDynamicHTML(conf.Name(), today, time.Now())
	_, err = botutil.SendMessageWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup:     dayMarkup(conf, schedule.Days, 0, botutil.SchedulePageURL(conf, nil)),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled command today")
}

func (h *handler) handleTextQuickGroup(ctx context.Context, b *bot.Bot, update *models.Update) {

	log.Debug().Msg("Handling quick group...")

	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID
	stop := sendChatActionTyping(ctx, b, chatID, threadID)
	defer stop()

	groupName := model.GroupName(update.Message.Text)
	var err error
	groupName, err = h.Schedule.ValidateGroupName(ctx, groupName)
	if err != nil { // This condition should be impossible
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		log.Error().Err(err).Msg("Invalid group name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            "Такой группы не существует",
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.SchedulePageURL(conf, nil)),
		})
		return
	}

	setGroupOrTeacherAndCached(ctx, string(groupName), schedule.IsOld)

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare schedule image")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.SchedulePageURL(conf, nil)),
		})
		return
	}

	err = botutil.SendWeekSchedule(ctx, b, threadID, chat, conf, schedule.Days, imageFilename, imageData, botutil.SchedulePageURL(conf, nil), schedule.IsOld)
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled quick group")
}

func getCtxChat(ctx context.Context) (*model.Chat, bool) {

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	return chat, ok
}

func sendVacationAnswer(ctx context.Context, b *bot.Bot, update *models.Update, isConfig bool) {

	t := time.Now()

	const configVacationText = "Не могу настроить группу во время каникул, подождите до начала семестра"

	dur := time.Until(time.Date(t.Year(), time.September, 1, 0, 0, 0, 0, t.Location()))
	days := int(dur.Hours()) / 24
	text := fmt.Sprintf("До конца каникул осталось %d день/дня/дней", days)

	if m := update.Message; m != nil {
		if isConfig {
			err := botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
				ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID, Text: configVacationText,
			})
			addHandlerCtxErr(ctx, err)
		}

		err := botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
			ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID, Text: text,
		})
		addHandlerCtxErr(ctx, err)

		b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: m.Chat.ID, MessageID: m.ID})
	} else if cq := update.CallbackQuery; cq != nil {
		_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: cq.ID, Text: text, CacheTime: 3600, // 1 hour
		})
		addHandlerCtxErr(ctx, err)

		if cq.Message.Message != nil {
			m := cq.Message.Message

			if isConfig {
				err := botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
					ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID, Text: configVacationText,
				})
				addHandlerCtxErr(ctx, err)
			}

			err = botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
				ChatID: m.Chat.ID, MessageThreadID: m.MessageThreadID, Text: text,
			})
			addHandlerCtxErr(ctx, err)
		}
	}
}

// dayMarkup returns the navigation keyboard for a text day view at the given
// index within the week's days. The value (group name or teacher ID) is
// derived from the schedule config.
func dayMarkup(conf model.ScheduleConfig, days []model.ScheduleDay, idx int, linkURL string) models.InlineKeyboardMarkup {
	value := ""
	switch {
	case conf.Group != nil:
		value = string(conf.Group.GroupName)
	case conf.Teacher != nil:
		value = conf.Teacher.TeacherID
	}
	return botutil.DayScheduleMarkup(value, days, idx, linkURL)
}

// scheduleDayDateLayouts are the date formats a schedule day may come in: the
// real scraper produces "DD.MM.YYYY" while demo data uses "YYYY-MM-DD".
var scheduleDayDateLayouts = []string{"02.01.2006", "2006-01-02"}

// parseDayDate parses a schedule day date in any known layout.
func parseDayDate(s string) (time.Time, bool) {
	for _, layout := range scheduleDayDateLayouts {
		if t, err := time.ParseInLocation(layout, s, time.Now().Location()); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// isDayOn reports whether the schedule day falls on the given calendar date.
func isDayOn(day model.ScheduleDay, t time.Time) bool {
	date, ok := parseDayDate(day.Date)
	if !ok {
		return false
	}
	y1, m1, d1 := date.Date()
	y2, m2, d2 := t.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// isTodayDay reports whether the schedule day is the current calendar day.
func isTodayDay(day model.ScheduleDay) bool { return isDayOn(day, time.Now()) }

// isTomorrowDay reports whether the schedule day is the next calendar day.
func isTomorrowDay(day model.ScheduleDay) bool { return isDayOn(day, time.Now().AddDate(0, 0, 1)) }

// dayMarker returns a short human-readable marker for the day when it
// corresponds to today or tomorrow, e.g. " (сегодня)". Otherwise it returns
// an empty string. The day is matched by calendar date; when its date cannot
// be parsed it falls back to comparing the lowercase Russian weekday names
// coming from the scraper.
func dayMarker(day model.ScheduleDay) string {
	if isTodayDay(day) {
		return " (сегодня)"
	}
	if isTomorrowDay(day) {
		return " (завтра)"
	}

	// Fallback for schedule days without a parseable date.
	if _, ok := parseDayDate(day.Date); ok {
		return ""
	}

	now := time.Now()
	if strings.EqualFold(day.Weekday, model.RussianWeekday(now.Weekday())) {
		return " (сегодня)"
	}
	if strings.EqualFold(day.Weekday, model.RussianWeekday(now.AddDate(0, 0, 1).Weekday())) {
		return " (завтра)"
	}
	return ""
}

// formatDayDate returns the schedule day's date in the "dd.mm.yyyy" format,
// falling back to the raw date string when it cannot be parsed. The scraped
// and demo data provide the date in different formats ("DD.MM.YYYY" and
// "YYYY-MM-DD"), hence the normalization.
func formatDayDate(day model.ScheduleDay) string {
	date, ok := parseDayDate(day.Date)
	if !ok {
		return day.Date
	}
	return date.Format("02.01.2006")
}

func formatDayHTML(name string, day model.ScheduleDay) string {
	text := fmt.Sprintf("📅 %s — %s, %s%s: ", name, day.Weekday, formatDayDate(day), dayMarker(day))

	if kind := day.CommonKind(); kind != "" {
		log.Trace().Msgf("Detected common kind: %s", kind)
		if kind == model.PairKindEmpty {
			text += "Нет пар"
		} else {
			text += day.Pairs[0].Label
		}
		return text
	}

	for _, pair := range day.Pairs {
		if pair.Kind == model.PairKindEmpty {
			continue
		}
		text += "\n\n" + pair.HTML()
	}

	return text
}
func formatDayDynamicHTML(name string, day model.ScheduleDay, t time.Time) string {

	text := fmt.Sprintf("📅 %s — %s, %s%s: ", name, day.Weekday, formatDayDate(day), dayMarker(day))

	if kind := day.CommonKind(); kind != "" {
		log.Trace().Msgf("Detected common kind: %s", kind)
		if kind == model.PairKindEmpty {
			text += "Нет пар"
		} else {
			text += day.Pairs[0].Label
		}
		return text
	}

	for _, pair := range day.Pairs {
		if pair.Kind == model.PairKindEmpty {
			continue
		}
		text += "\n\n"
		if pair.IsPassedAt(t) {
			log.Trace().Msg("Before")
			text += fmt.Sprintf("<s>%s</s>", pair.HTML())
		} else {
			log.Trace().Msg("After")
			text += pair.HTML()
		}
	}

	return text
}

func (h *handler) resetChatForExpiredGroup(ctx context.Context, b *bot.Bot, chat *model.Chat, chatID int64, threadID int) {
	groupName := string(*chat.GroupName)

	if err := h.Chat.ResetGroupSettings(ctx, chat); err != nil {
		log.Error().Err(err).Msg("Failed to reset group settings")
	}

	botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            fmt.Sprintf(botutil.MsgGroupRemoved, groupName),
		ReplyMarkup:     botutil.MainMenuMarkup(chat.IsPrivate()),
	})
}
