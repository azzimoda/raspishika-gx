package mainbot

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

func (h *handler) handleCmdTeacher(ctx context.Context, b *bot.Bot, update *models.Update) {
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
	_ = chat

	recentTeachers, err := h.Schedule.GetChatRecentTeachers(ctx, chat.ID)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		log.Warn().Err(err).Msg("Failed to get chat's recent teachers, fallback to none")
		recentTeachers = []*model.Teacher{}
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateSelectingTeacher)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            "Пришлите полное имя преподавателя или его часть",
		ReplyMarkup:     botutil.TeacherMenuMarkup(recentTeachers),
	})
	addHandlerCtxErr(ctx, err)
}

func (h *handler) handleTextTeacherName(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := botutil.DeleteMessage(ctx, b, update.Message)
	addHandlerCtxErr(ctx, err)

	if err := h.Schedule.EnsureTeachers(ctx); err != nil {
		addHandlerCtxErr(ctx, err)
		h.Report().Err(err).Msg("Failed to ensure teachers, using cached")
	}

	teachers, err := h.Schedule.FindTeachersByName(ctx, update.Message.Text)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	if len(teachers) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Не удалось найти преподавателя, попробуйте ещё раз",
			ReplyMarkup:     botutil.TeacherMenuMarkup(nil), // Empty list
		})
		addHandlerCtxErr(ctx, err)
		return
	} else if len(teachers) == 1 {
		chat, ok := ctx.Value(keyChat).(*model.Chat)
		if ok {
			if err := h.sendTeacherSchedule(ctx, b, update.Message, chat, teachers[0]); err != nil {
				addHandlerCtxErr(ctx, err)
				botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
					ChatID:          update.Message.Chat.ID,
					MessageThreadID: update.Message.MessageThreadID,
					Text:            botutil.ErrMsgTryLater,
				})
			}
			return
		} else {
			addHandlerCtxErr(ctx, ErrNoChatContext)
			// Failed to change chat's status, send list with one teacher to let user select it manually
		}
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            "Выберите преподавателя из списка или попробуйте снова",
		ReplyMarkup:     botutil.TeacherMenuMarkup(teachers),
	})
	addHandlerCtxErr(ctx, err)
	// TODO: Send list of matching teachers
}

func (h *handler) handleCQTeacher(ctx context.Context, b *bot.Bot, update *models.Update) {
	message := update.CallbackQuery.Message.Message
	_, err := botutil.DeleteMessage(ctx, b, message)
	addHandlerCtxErr(ctx, err)

	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, ErrNoChatContext)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	teacher, err := h.Schedule.GetTeacherByTeacherID(ctx, model.TeacherID(command.Arg(0)))
	if err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	err = h.sendTeacherSchedule(ctx, b, message, chat, teacher)
	addHandlerCtxErr(ctx, err)
}

func (h *handler) sendTeacherSchedule(
	ctx context.Context,
	b *bot.Bot,
	message *models.Message,
	chat *model.Chat,
	teacher *model.Teacher,
) error {
	if err := h.Chat.AddChatRecentTeacher(ctx, chat.ID, teacher.ID); err != nil {
		addHandlerCtxErr(ctx, err)
		h.Report().Err(err).Chat(chat).Msg("Failed to add recent teacher for chat")
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		return err
	}

	sendChatActionTyping(ctx, b, message.Chat.ID, message.MessageThreadID)

	conf := model.TeacherScheduleConfig(teacher, chat.DarkMode)
	rawSchedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		return err
	}

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, conf, rawSchedule)
	if err != nil {
		return err
	}

	return botutil.SendWeekScheduleMessages(ctx, b, message.MessageThreadID, chat, conf, imageFilename, imageData)
}
