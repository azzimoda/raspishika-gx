package mainbot

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

func (h *handler) handleCmdWeek(ctx context.Context, b *bot.Bot, update *models.Update) {
	threadID := update.Message.MessageThreadID
	chatID := update.Message.Chat.ID
	sendChatActionTyping(ctx, b, chatID, threadID)

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	if chat.GroupName == nil {
		// Offer to set group
		log.Warn().Int64("chatID", chat.TgChatID.Int64()).Msg("Group name is not set")
		h.sendDepartmentSelectionMenu(ctx, b, chat, threadID)
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, *chat.GroupName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	rawSchedule, err := h.Schedule.Get(ctx, conf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, conf, rawSchedule)
	if err != nil {
		log.Error().Err(err).Msg("Failed to prepare schedule image")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	err = sendWeekScheduleMessages(ctx, b, threadID, chat, conf, imageFilename, imageData)
	addHandlerCtxErr(ctx, err)
}

func (h *handler) handleCmdTomorrow(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID
	sendChatActionTyping(ctx, b, chatID, threadID)

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	if chat.GroupName == nil {
		// Offer to set group
		log.Warn().Int64("chatID", chat.TgChatID.Int64()).Msg("Group name is not set")
		h.sendDepartmentSelectionMenu(ctx, b, chat, threadID)
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, *chat.GroupName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	rawSchedule, err := h.Schedule.Get(ctx, conf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	schedule := rawSchedule.Transform()
	tomorrow := schedule.Tomorrow(time.Now())

	text := tomorrow.HTML()
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup:     updateScheduleMarkup("tomorrow", string(group.GroupName)),
	})
	addHandlerCtxErr(ctx, err)
}

func (h *handler) handleCmdToday(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID
	sendChatActionTyping(ctx, b, chatID, threadID)

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	if chat.GroupName == nil {
		// Offer to set group
		log.Debug().Int64("chatID", chat.TgChatID.Int64()).Msg("Group name is not set")
		addHandlerCtxErr(ctx, h.sendDepartmentSelectionMenu(ctx, b, chat, threadID))
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, *chat.GroupName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	// If today is Sunday, send a special message
	if time.Now().Weekday() == time.Sunday {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            "Сегодня воскресенье, отдыхайте!",
			ReplyMarkup:     updateScheduleMarkup("today", string(group.GroupName)),
		})
		addHandlerCtxErr(ctx, err)
		return
	}
	// Otherwise, send today's schedule

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	rawSchedule, err := h.Schedule.Get(ctx, conf)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get schedule")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	schedule := rawSchedule.Transform()
	today := schedule.Today()
	text := today.DynamicFormatHTML(time.Now())
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup:     updateScheduleMarkup("today", string(group.GroupName)),
	})
	addHandlerCtxErr(ctx, err)
}
