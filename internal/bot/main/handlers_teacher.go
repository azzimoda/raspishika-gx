package mainbot

import (
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

func (h *handler) handleCmdTeacher(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	threadID := update.Message.MessageThreadID

	stop := sendChatActionTyping(ctx, b, chatID, threadID)
	defer stop()

	botutil.DeleteMessage(ctx, b, update.Message)

	if status, err := h.Schedule.IsVacation(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to check vacation status; trying to process anyway...")
	} else if status {
		sendVacationAnswer(ctx, b, update, false)
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
	_ = chat

	recentTeachers, err := h.Chat.GetRecentTeachers(ctx, chat.ID)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		log.Warn().Err(err).Msg("Failed to get chat's recent teachers, fallback to none")
		recentTeachers = []*model.RecentTeacher{}
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateSelectingTeacher)); err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chatID,
			MessageThreadID: threadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	teachers := make([]model.Teacher, len(recentTeachers))
	for i, t := range recentTeachers {
		teachers[i] = model.Teacher{TeacherID: t.TeacherID, Name: t.TeacherName}
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            "Пришлите полное имя преподавателя или его часть",
		ReplyMarkup:     teacherMenuMarkup(teachers, ""),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled command teacher")
}

func (h *handler) handleTextTeacherName(ctx context.Context, b *bot.Bot, update *models.Update) {
	botutil.DeleteMessage(ctx, b, update.Message)

	teachers, err := h.Schedule.FindTeachersByName(ctx, update.Message.Text)
	if err != nil {
		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
			ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.CollegeScheduleURL),
		})
		return
	}

	if len(teachers) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Не удалось найти преподавателя, попробуйте ещё раз",
			ReplyMarkup:     teacherMenuMarkup(nil, botutil.CollegeScheduleURL),
		})
		addHandlerCtxErr(ctx, err)
		return
	} else if len(teachers) == 1 {
		chat, ok := ctx.Value(keyChat).(*model.Chat)
		if ok {
			if err := h.sendTeacherSchedule(ctx, b, update.Message, chat, &teachers[0]); err != nil {
				addHandlerCtxErr(ctx, err)
				botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
					ChatID:          update.Message.Chat.ID,
					MessageThreadID: update.Message.MessageThreadID,
					Text:            botutil.ErrMsgTryLater,
					ReplyMarkup:     botutil.LinkOnlyMarkup(botutil.TeacherSchedulePageURL(ctx, h.Schedule, &teachers[0])),
				})
			}
			return
		} else {
			addHandlerCtxErr(ctx, ErrNoChatContext)
			// Failed to send schedule right now, send list with one teacher to let user select it manually
		}
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            "Выберите преподавателя из списка или попробуйте снова",
		ReplyMarkup:     teacherMenuMarkup(teachers, ""),
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled text teacher name")
}

func (h *handler) handleCQTeacher(ctx context.Context, b *bot.Bot, update *models.Update) {

	log.Debug().Msg("Handling CQ teacher...")

	message := update.CallbackQuery.Message.Message
	_, err := botutil.DeleteMessage(ctx, b, message)
	addHandlerCtxErr(ctx, err)

	stop := sendChatActionTyping(ctx, b, message.Chat.ID, message.MessageThreadID)
	defer stop()

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

	teacher, err := h.Schedule.GetTeacherByNameOrID(ctx, command.Arg(0))
	if err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			sendVacationAnswer(ctx, b, update, false)
			return
		}

		addHandlerCtxErr(ctx, err)
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          message.Chat.ID,
			MessageThreadID: message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
		return
	}

	err = h.sendTeacherSchedule(ctx, b, message, chat, teacher)
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled CQ teacher")
}

func (h *handler) sendTeacherSchedule(
	ctx context.Context,
	b *bot.Bot,
	message *models.Message,
	chat *model.Chat,
	teacher *model.Teacher,
) error {
	log.Debug().Msg("Sending teacher schedule...")

	if err := h.Chat.AddChatRecentTeacher(ctx, &model.RecentTeacher{
		ChatID: chat.ID, TeacherID: teacher.TeacherID, TeacherName: teacher.Name,
	}); err != nil {
		addHandlerCtxErr(ctx, err)
		h.ReportChat(chat).Err(err).Msg("Failed to add recent teacher for chat")
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		return err
	}

	conf := model.TeacherScheduleConfig(teacher, chat.DarkMode)
	schedule, err := h.Schedule.GetSchedule(ctx, conf)
	if err != nil {
		return err
	}
	log.Trace().Bool("old", schedule.IsOld).Msg("Got teacher schedule")

	setGroupOrTeacherAndCached(ctx, string(teacher.Name), schedule.IsOld)

	imageFilename, imageData, err := h.Schedule.PrepareScheduleImage(ctx, schedule)
	if err != nil {
		return err
	}
	log.Trace().Msg("Prepared schedule image")

	return botutil.SendWeekSchedule(
		ctx, b, message.MessageThreadID, chat, conf, schedule.Days, imageFilename, imageData,
		botutil.TeacherSchedulePageURL(ctx, h.Schedule, teacher), schedule.IsOld)
}

func teacherMenuMarkup(teachers []model.Teacher, linkURL string) models.InlineKeyboardMarkup {
	keyboard := make([][]models.InlineKeyboardButton, 0)
	for _, t := range teachers {
		keyboard = append(keyboard, []models.InlineKeyboardButton{{
			Text:         t.Name,
			CallbackData: botutil.NewCallbackCommand(botutil.CallbackCommandSelectTeacher, t.TeacherID).String(),
		}})
	}
	bottomRow := []models.InlineKeyboardButton{{Text: "Отмена", CallbackData: botutil.CallbackCommandDelete}}
	if linkURL != "" {
		bottomRow = append([]models.InlineKeyboardButton{botutil.ScheduleLinkButton(linkURL)}, bottomRow...)
	}
	keyboard = append(keyboard, bottomRow)
	return models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}
