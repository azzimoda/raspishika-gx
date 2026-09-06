package mainbot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

const MsgScheduleUpdated = "Расписание обновлено"

func (h *handler) handleCQUpdateWeek(ctx context.Context, b *bot.Bot, update *models.Update) {

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	darkMode := false
	if ok {
		darkMode = chat.DarkMode
	} else {
		log.Warn().Msg("Failed to get chat to get color mode, using light mode")
		addHandlerCtxErr(ctx, ErrNoChatContext)
	}

	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	value := command.Arg(0)

	isTeacher := command.Command == botutil.CallbackCommandUpdateTeacher
	if command.Command == botutil.CallbackCommandUpdateWeek {
		isTeacher = isTeacherID(value)
	}

	var conf model.ScheduleConfig
	var linkURL string
	if isTeacher {
		teacher, err := h.Schedule.GetTeacherByNameOrID(ctx, value)
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

		conf = model.TeacherScheduleConfig(teacher, darkMode)
		linkURL = botutil.TeacherSchedulePageURL(ctx, h.Schedule, teacher)
	} else {
		group, err := h.Schedule.GetGroupByName(ctx, model.GroupName(value))
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

		conf = model.GroupScheduleConfig(group, darkMode)
		linkURL = botutil.SchedulePageURL(conf, nil)
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

	_, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	message := update.CallbackQuery.Message.Message
	_, err = b.EditMessageMedia(ctx, &bot.EditMessageMediaParams{
		ChatID:    message.Chat.ID,
		MessageID: message.ID,
		Media: &models.InputMediaPhoto{
			Media:           "attach://image.png",
			MediaAttachment: bytes.NewReader(imageData),
		},
		ReplyMarkup: botutil.WeekScheduleMarkup(conf, linkURL, schedule.Days),
	})
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotSendSchedule,
		})
		return
	}

	_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackQueryID,
		Text:            MsgScheduleUpdated,
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ update week")
}

// isTeacherID reports whether the callback value is a numeric teacher ID;
// group names always contain letters and are never purely numeric.
func isTeacherID(s string) bool {

	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (h *handler) handleCQUpdateTomorrow(ctx context.Context, b *bot.Bot, update *models.Update) {

	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	groupName := model.GroupName(command.Arg(0))

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: callbackQueryID,
				Text:            fmt.Sprintf(botutil.MsgGroupRemoved, groupName),
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

	conf := model.GroupScheduleConfig(group, false)
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

	setGroupOrTeacherAndCached(ctx, string(groupName), schedule.IsOld)

	tomorrow := schedule.Tomorrow(time.Now())

	idx := 1
	if time.Now().Weekday() == time.Sunday {
		idx = 0
	}

	message := update.CallbackQuery.Message.Message
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      message.Chat.ID,
		MessageID:   message.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        formatDayHTML(conf.Name(), tomorrow),
		ReplyMarkup: dayMarkup(conf, schedule.Days, idx, botutil.SchedulePageURL(conf, nil)),
	})
	if err != nil {
		if botutil.IsMessageNotModified(err) {
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

	_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackQueryID,
		Text:            MsgScheduleUpdated,
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ update tomorrow")
}

func (h *handler) handleCQUpdateToday(ctx context.Context, b *bot.Bot, update *models.Update) {

	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	groupName := model.GroupName(command.Arg(0))

	message := update.CallbackQuery.Message.Message
	// If today is Sunday, send a special message
	if time.Now().Weekday() == time.Sunday {
		_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      message.Chat.ID,
			MessageID:   message.ID,
			ParseMode:   models.ParseModeHTML,
			Text:        "Сегодня воскресенье, отдыхайте!",
			ReplyMarkup: botutil.SimpleUpdateMarkup(botutil.UpdateKindToday, string(groupName), botutil.CollegeScheduleURL),
		})
		if err != nil {
			if botutil.IsMessageNotModified(err) {
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

		_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            MsgScheduleUpdated,
		})
		addHandlerCtxErr(ctx, err)
		return
	}
	// Otherwise, send today's schedule

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}
		if errors.Is(err, apiclient.ErrNotFound) {
			b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
				CallbackQueryID: callbackQueryID,
				Text:            fmt.Sprintf(botutil.MsgGroupRemoved, groupName),
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

	conf := model.GroupScheduleConfig(group, false)
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

	setGroupOrTeacherAndCached(ctx, string(groupName), schedule.IsOld)

	today := schedule.Today()

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      message.Chat.ID,
		MessageID:   message.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        formatDayDynamicHTML(conf.Name(), today, time.Now()),
		ReplyMarkup: dayMarkup(conf, schedule.Days, 0, botutil.SchedulePageURL(conf, nil)),
	})
	if err != nil {
		if botutil.IsMessageNotModified(err) {
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

	_, err = b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: callbackQueryID,
		Text:            MsgScheduleUpdated,
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ update today")
}
