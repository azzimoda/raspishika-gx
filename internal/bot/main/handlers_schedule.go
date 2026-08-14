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
	log.Debug().Msg("Handling command week...")

	threadID := update.Message.MessageThreadID
	chatID := update.Message.Chat.ID
	stop := sendChatActionTyping(ctx, b, chatID, threadID)
	defer stop()

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
	log.Trace().Str("name", string(group.GroupName)).Msg("Got group")

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
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
		})
		return
	}
	log.Trace().Msg("Prepared schedule image")

	err = botutil.SendWeekScheduleMessages(ctx, b, threadID, chat, conf, imageFilename, imageData, schedule.IsOld)
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
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
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

	setGroupOrTeacherAndCached(ctx, string(*chat.GroupName), schedule.IsOld)

	tomorrow := schedule.Tomorrow(time.Now())

	text := tomorrow.HTML()
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup:     botutil.UpdateScheduleMarkup("tomorrow", string(group.GroupName)),
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
			ReplyMarkup:     botutil.UpdateScheduleMarkup("today", string(group.GroupName)),
		})
		addHandlerCtxErr(ctx, err)
		return
	}
	// Otherwise, send today's schedule

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
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

	setGroupOrTeacherAndCached(ctx, string(*chat.GroupName), schedule.IsOld)

	today := schedule.Today()
	text := today.DynamicFormatHTML(time.Now())
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		ParseMode:       models.ParseModeHTML,
		Text:            text,
		ReplyMarkup:     botutil.UpdateScheduleMarkup("today", string(group.GroupName)),
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
		log.Error().Err(err).Msg("Invalid group name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            "Такой группы не существует",
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
		})
		return
	}

	group, err := h.Schedule.GetGroupByName(ctx, groupName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get group by name")
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgCouldNotLoadSchedule,
		})
		return
	}

	conf := model.GroupScheduleConfig(group, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
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

	setGroupOrTeacherAndCached(ctx, string(groupName), schedule.IsOld)

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
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

	err = botutil.SendWeekScheduleMessages(ctx, b, threadID, chat, conf, imageFilename, imageData, schedule.IsOld)
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled quick group")
}
