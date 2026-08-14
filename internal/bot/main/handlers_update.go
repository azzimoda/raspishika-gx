package mainbot

import (
	"bytes"
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

const MsgScheduleUpdated = "Расписание обновлено"

func (h *handler) handleCQUpdateGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	chat, ok := ctx.Value(keyChat).(*model.Chat)
	darkMode := false
	if ok {
		darkMode = chat.DarkMode
	} else {
		addHandlerCtxErr(ctx, ErrNoChatContext)
	}

	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	groupName := model.GroupName(command.Arg(0))

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	conf := model.GroupScheduleConfig(group, darkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	setGroupOrTeacherAndCached(ctx, string(groupName), schedule.IsOld)

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
		ReplyMarkup: botutil.UpdateScheduleMarkup("group", string(groupName)),
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

	log.Info().Msg("Handled CQ update group")
}

func (h *handler) handleCQUpdateTeacher(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling CQ update teacher...")

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	darkMode := false
	if ok {
		darkMode = chat.DarkMode
	} else {
		addHandlerCtxErr(ctx, ErrNoChatContext)
	}

	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	teacherID := command.Arg(0)

	teacher, err := h.Schedule.GetTeacherByNameOrID(ctx, teacherID)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	conf := model.TeacherScheduleConfig(teacher, darkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}
	log.Trace().Bool("old", schedule.IsOld).Msg("Got teacher schedule")

	setGroupOrTeacherAndCached(ctx, string(schedule.Config.Teacher.Name), schedule.IsOld)

	_, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}
	log.Trace().Msg("Prepared schedule image")

	message := update.CallbackQuery.Message.Message
	_, err = b.EditMessageMedia(ctx, &bot.EditMessageMediaParams{
		ChatID:    message.Chat.ID,
		MessageID: message.ID,
		Media: &models.InputMediaPhoto{
			Media:           "attach://image.png",
			MediaAttachment: bytes.NewReader(imageData),
		},
		ReplyMarkup: botutil.UpdateScheduleMarkup("teacher", string(teacherID)),
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

	log.Info().Msg("Handled CQ update teacher")
}

func (h *handler) handleCQUpdateTomorrow(ctx context.Context, b *bot.Bot, update *models.Update) {
	callbackQueryID := update.CallbackQuery.ID
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	groupName := model.GroupName(command.Arg(0))

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
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
		addHandlerCtxErr(ctx, err)
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
			CallbackQueryID: callbackQueryID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	setGroupOrTeacherAndCached(ctx, string(groupName), schedule.IsOld)

	tomorrow := schedule.Tomorrow(time.Now())

	message := update.CallbackQuery.Message.Message
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      message.Chat.ID,
		MessageID:   message.ID,
		ParseMode:   models.ParseModeHTML,
		Text:        tomorrow.HTML(),
		ReplyMarkup: botutil.UpdateScheduleMarkup("tomorrow", string(groupName)),
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
			ReplyMarkup: botutil.UpdateScheduleMarkup("today", string(groupName)),
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
		return
	}
	// Otherwise, send today's schedule

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
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
		Text:        today.DynamicFormatHTML(time.Now()),
		ReplyMarkup: botutil.UpdateScheduleMarkup("today", string(groupName)),
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

	log.Info().Msg("Handled CQ update today")
}
