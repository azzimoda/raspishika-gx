package mainbot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

// resolveScheduleTarget builds the schedule config and the schedule-page link
// for a callback value (a group name or a numeric teacher ID).
func (h *handler) resolveScheduleTarget(ctx context.Context, value string, isTeacher, darkMode bool) (model.ScheduleConfig, string, error) {
	if isTeacher {
		teacher, err := h.Schedule.GetTeacherByNameOrID(ctx, value)
		if err != nil {
			return model.ScheduleConfig{}, "", err
		}
		return model.TeacherScheduleConfig(teacher, darkMode), botutil.TeacherSchedulePageURL(ctx, h.Schedule, teacher), nil
	}

	group, err := h.Schedule.GetGroupByName(ctx, model.GroupName(value))
	if err != nil {
		return model.ScheduleConfig{}, "", err
	}
	conf := model.GroupScheduleConfig(group, darkMode)
	return conf, botutil.SchedulePageURL(conf, nil), nil
}

// chatDarkMode returns the dark mode preference of the chat from context,
// falling back to light mode when the chat is absent.
func (h *handler) chatDarkMode(ctx context.Context) bool {
	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		log.Warn().Msg("Failed to get chat to get color mode, using light mode")
		addHandlerCtxErr(ctx, ErrNoChatContext)
		return false
	}
	return chat.DarkMode
}

// handleCQUpdateDay is the callback handler for opening or navigating between
// schedule days. When the source message is a week photo it sends a new text
// message (and deletes the photo); when the source is a day text message it
// edits that message in place.
func (h *handler) handleCQUpdateDay(ctx context.Context, b *bot.Bot, update *models.Update) {
	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	value := command.Arg(0)

	idx, err := strconv.Atoi(command.Arg(1))
	if err != nil || idx < 0 {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	conf, linkURL, err := h.resolveScheduleTarget(ctx, value, isTeacherID(value), false)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: callbackQueryID,
				Text:            fmt.Sprintf(botutil.MsgGroupRemoved, value),
			})
			return
		}
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	total := len(schedule.Days)
	if total == 0 {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}
	if idx >= total {
		idx = total - 1
	}

	setGroupOrTeacherAndCached(ctx, conf.Name(), schedule.IsOld)

	text := formatDayHTML(conf.Name(), schedule.Days[idx])
	if isTodayDay(schedule.Days[idx]) {
		text = formatDayDynamicHTML(conf.Name(), schedule.Days[idx], time.Now())
	}
	replyMarkup := dayMarkup(conf, schedule.Days, idx, linkURL)

	message := update.CallbackQuery.Message.Message
	if len(message.Photo) > 0 {
		// The source is a week photo: send the day as a new message, then delete
		// the photo so that the formats do not accumulate.
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			ParseMode:       models.ParseModeHTML,
			Text:            text,
			ReplyMarkup:     replyMarkup,
		})
		if err == nil {
			botutil.DeleteMessage(ctx, b, message)
		}
	} else {
		// The source is a day text message: edit in place.
		_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      message.Chat.ID,
			MessageID:   message.ID,
			ParseMode:   models.ParseModeHTML,
			Text:        text,
			ReplyMarkup: replyMarkup,
		})
	}

	if err != nil {
		if botutil.IsMessageNotModified(err) {
			// The tapped day is already shown; treat it as a no-op.
			_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackQueryID})
			addHandlerCtxErr(ctx, err)
			return
		}
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotSendSchedule,
		})
		return
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: callbackQueryID})
	log.Info().Msg("Handled CQ update day")
}

// handleCQOpenWeek is the callback handler for the "Неделя" button on a day
// text message. It shows the week schedule as a new photo and deletes the day
// message so that the formats do not accumulate.
func (h *handler) handleCQOpenWeek(ctx context.Context, b *bot.Bot, update *models.Update) {
	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	value := command.Arg(0)

	conf, linkURL, err := h.resolveScheduleTarget(ctx, value, isTeacherID(value), h.chatDarkMode(ctx))
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: callbackQueryID,
				Text:            fmt.Sprintf(botutil.MsgGroupRemoved, value),
			})
			return
		}
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	setGroupOrTeacherAndCached(ctx, conf.Name(), schedule.IsOld)

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	message := update.CallbackQuery.Message.Message
	_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Photo:           &models.InputFileUpload{Filename: imageFilename, Data: bytes.NewReader(imageData)},
		ReplyMarkup:     botutil.WeekScheduleMarkup(conf, linkURL, schedule.Days),
	})
	if err == nil {
		botutil.DeleteMessage(ctx, b, message)
	}

	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotSendSchedule,
		})
		return
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackQueryID,
		Text:            MsgScheduleUpdated,
	})
	log.Info().Msg("Handled CQ open week")
}
