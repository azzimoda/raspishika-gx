package mainbot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

type contextKey string

const (
	keyChat      contextKey = "chat"
	keyError     contextKey = "error"
	keyNoLogFlag contextKey = "default_handler"
)

func sendWeekScheduleMessages(
	ctx context.Context,
	b *bot.Bot,
	messageThreadID int,
	chat *model.Chat,
	conf model.ScheduleConfig,
	imageFilename string,
	imageData []byte,
) error {
	var errs []error

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: messageThreadID,
		Text:            conf.FormatHTML() + ":",
		ParseMode:       models.ParseModeHTML,
		ReplyMarkup:     mainMenuMarkup(chat.IsPrivate()),
	}); err != nil {
		errs = append(errs, err)
	}

	if _, err := b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: messageThreadID,
		Action:          models.ChatActionUploadPhoto,
	}); err != nil {
		errs = append(errs, err)
	}

	replyMarkup := weekScheduleMarkup(conf)
	if err := sendSchedulePhoto(ctx, b, chat, messageThreadID, imageFilename, imageData, replyMarkup); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// mainMenuMarkup returns the main menu keyboard for the given chat type.
func mainMenuMarkup(isPrivate bool) models.ReplyMarkup {
	if isPrivate {
		return models.ReplyKeyboardMarkup{
			Keyboard: [][]models.KeyboardButton{
				{{Text: "Неделя"}},
				{{Text: "Сегодня"}, {Text: "Завтра"}, {Text: "Преподаватель"}},
			},
			ResizeKeyboard: true,
		}
	} else {
		return models.ReplyKeyboardRemove{RemoveKeyboard: true}
	}
}

func sendSchedulePhoto(
	ctx context.Context,
	b *bot.Bot,
	chat *model.Chat,
	messageThreadID int,
	imageFilename string,
	imageData []byte,
	replyMarkup models.ReplyMarkup,
) error {
	log.Trace().Any("tgChatID", chat.TgChatID).Str("filename", imageFilename).Msg("Sending schedule photo...")
	_, err := b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: messageThreadID,
		Photo:           &models.InputFileUpload{Filename: imageFilename, Data: bytes.NewReader(imageData)},
		ReplyMarkup:     replyMarkup,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to send schedule photo")
		err2 := botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID: chat.TgChatID,
			Text:   botutil.ErrMsgCouldNotSendSchedule,
		})
		return errors.Join(err, err2)
	}
	return err
}

func weekScheduleMarkup(conf model.ScheduleConfig) models.ReplyMarkup {
	if conf.Group != nil {
		return updateScheduleMarkup("group", string(conf.Group.GroupName))
	} else if conf.Teacher != nil {
		return updateScheduleMarkup("teacher", conf.Teacher.TeacherID.String())
	} else {
		return nil
	}
}
func updateScheduleMarkup(kind, value string) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{updateInlineButton(kind, value)}},
	}
}
func updateInlineButton(kind, value string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{
		Text: "Обновить",
		CallbackData: fmt.Sprintf("update_%s\n%s\n%s",
			kind, value,
			time.Now().Format("20060102150405"),
			// NOTE: Time is added to prevent editing message error when the content is the same.
		),
	}
}

func teacherMenuMarkup(teachers []*model.Teacher) models.InlineKeyboardMarkup {
	keyboard := make([][]models.InlineKeyboardButton, 0)
	for _, teacher := range teachers {
		keyboard = append(keyboard, []models.InlineKeyboardButton{{
			Text:         teacher.Name.String(),
			CallbackData: fmt.Sprintf("%s\n%s", CallbackCommandSelectTeacher, teacher.TeacherID),
		}})
	}
	keyboard = append(keyboard, []models.InlineKeyboardButton{{Text: "Отмена", CallbackData: CallbackCommandDelete}})
	return models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

func accessMenuMarkup(accessLevel model.ChatAccessLevel) models.InlineKeyboardMarkup {
	keyboard := [][]models.InlineKeyboardButton{
		{},
		{{Text: "Закрыть", CallbackData: CallbackCommandDeleteConfig}},
	}
	for i := range 3 {
		text := fmt.Sprint(i)
		if i == int(accessLevel) {
			text = fmt.Sprintf("[%d]", i)
		}
		keyboard[0] = append(keyboard[0], models.InlineKeyboardButton{
			Text:         text,
			CallbackData: fmt.Sprintf("%s\n%d", CallbackCommandSetAccess, i),
		})
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}

// addHandlerCtxErr adds an error to the handler error context.
func addHandlerCtxErr(ctx context.Context, err error) {
	handlerErrs, ok := ctx.Value(keyError).(*[]error)
	if ok {
		if err != nil {
			*handlerErrs = append(*handlerErrs, err)
		}
	} else {
		log.Warn().Err(err).Msg("Error context not found")
	}
}

func shortenText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength-2] + "…"
	}
	return text
}

// TODO: Remove this function
func unimplementedHandler() { log.Error().CallerSkipFrame(1).Msg("Handler is unimplemented") }

func sendChatActionTyping(ctx context.Context, b *bot.Bot, chatID any, messageThreadID int) {
	// TODO: Implement repeated chat action sending with cancel context or timeout
	if _, err := b.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID:          chatID,
		MessageThreadID: messageThreadID,
		Action:          models.ChatActionTyping,
	}); err != nil {
		addHandlerCtxErr(ctx, err)
		log.Error().Err(err).Msg("Failed to send chat action")
	}
}
