package botutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"go.yaml.in/yaml/v3"
)

const (
	ErrMsgTryLater             = "Произошла ошибка, попробуйте позже"
	ErrMsgCouldNotLoadSchedule = "Не удалось загрузить расписание, попробуйте позже"
	ErrMsgCouldNotUpdateData   = "Не удалось обновить данные, попробуйте позже"
	ErrMsgCouldNotSendSchedule = "Не удалось отправить расписание, попробуте позже"
	ErrMsgSelectGroupAgain     = "Не удалось найти группу, выберите группу ещё раз"
)

func SendErrorMessage(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) error {
	err := SendTempMessage(ctx, b, 7*time.Second, params)
	if err != nil {
		log.Error().Err(err).Any("params", params).Msg("Failed to send error message")
	}
	return err
}

// SendTempMessage sends a temporary message that will be automatically deleted after the specified duration.
func SendTempMessage(ctx context.Context, b *bot.Bot, dur time.Duration, params *bot.SendMessageParams) error {
	msg, err := b.SendMessage(ctx, params)
	if err != nil {
		log.Error().Err(err).Msg("Error sending temporary message")
		return fmt.Errorf("error sending temporary message: %w", err)
	}

	go func() {
		time.Sleep(dur)
		success, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
		if err != nil {
			log.Error().Err(err).Msg("Error deleting temporary message")
		}
		if !success {
			log.Warn().Msg("Temporary message could not be deleted")
		}
	}()

	return nil
}

// ParseCommand parses a command string into a command and arguments.
func ParseCommand(text string) (command string, args string) {
	re := regexp.MustCompile(`^/(\w+)(@\w+)?(\s([\s\S]+))?$`)
	submatches := re.FindStringSubmatch(text)
	if submatches != nil {
		command = submatches[1]
		args = submatches[4]
	} else {
		command = text
	}
	return command, args
}

// ParseCallbackData parses callback data into a CallbackCommand struct.
//
// The data is expected to be in the format of a command followed by newline-separated arguments.
func ParseCallbackData(data string) CallbackCommand {
	lines := strings.Split(data, "\n")
	command := ""
	if len(lines) >= 1 {
		command = lines[0]
	}
	args := []string{}
	if len(lines) >= 2 {
		args = lines[1:]
	}
	return CallbackCommand{Command: command, Args: args}
}

// TODO: Use it for building callback data.
func NewCallbackCommand(command string, args ...string) CallbackCommand {
	return CallbackCommand{Command: command, Args: args}
}

// CallbackCommand represents a parsed callback command with a command and arguments.
type CallbackCommand struct {
	Command string
	Args    []string
}

// Arg returns the argument at the specified index, or an empty string if out of bounds.
func (c CallbackCommand) Arg(i int) string {
	if i < len(c.Args) {
		return c.Args[i]
	}
	return ""
}

func (c CallbackCommand) String() string {
	b := new(strings.Builder)
	fmt.Fprint(b, c.Command)
	for _, s := range c.Args {
		fmt.Fprintf(b, "\n%s", s)
	}
	return b.String()
}

func DeleteMessage(ctx context.Context, b *bot.Bot, message *models.Message) (bool, error) {
	if message == nil {
		return false, nil
	}
	return b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: message.Chat.ID, MessageID: message.ID})
}

func ParseBotCommands(bytes []byte) ([]models.BotCommand, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(bytes, &node); err != nil {
		return nil, fmt.Errorf("failed to unmarshal command config file: %w", err)
	}

	if node.Kind != yaml.DocumentNode {
		return nil, fmt.Errorf("failed to parse command YAML document")
	}
	if len(node.Content) < 1 {
		return nil, fmt.Errorf("failed to parse command YAML document")
	}
	node = *node.Content[0]
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("failed to parse command YAML document")
	}

	var commands []models.BotCommand
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		value := node.Content[i+1].Value
		commands = append(commands, models.BotCommand{Command: key, Description: value})
	}
	return commands, nil
}

func SendWeekScheduleMessages(
	ctx context.Context,
	b *bot.Bot,
	messageThreadID int,
	chat *model.Chat,
	conf model.ScheduleConfig,
	imageFilename string,
	imageData []byte,
	isOld bool,
) error {
	log.Debug().Msg("Sending week schedule message...")
	var errs []error

	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: messageThreadID,
		Text:            conf.FormatHTML() + ":",
		ParseMode:       models.ParseModeHTML,
		ReplyMarkup:     MainMenuMarkup(chat.IsPrivate()),
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

	replyMarkup := WeekScheduleMarkup(conf)
	if err := sendSchedulePhoto(ctx, b, chat, messageThreadID, imageFilename, imageData, replyMarkup, isOld); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// MainMenuMarkup returns the main menu keyboard for the given chat type.
func MainMenuMarkup(isPrivate bool) models.ReplyMarkup {
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
	isOld bool,
) error {
	log.Trace().Any("tgChatID", chat.TgChatID).Str("filename", imageFilename).Msg("Sending schedule photo...")
	photoParams := &bot.SendPhotoParams{
		ChatID:          chat.TgChatID,
		MessageThreadID: messageThreadID,
		Photo:           &models.InputFileUpload{Filename: imageFilename, Data: bytes.NewReader(imageData)},
		ReplyMarkup:     replyMarkup,
	}
	if isOld {
		photoParams.Caption = "<i>Не удалось обновить расписание, информация может быть не актуальной!</i>"
		photoParams.ParseMode = models.ParseModeHTML
	}
	_, err := b.SendPhoto(ctx, photoParams)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send schedule photo")
		err2 := SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID: chat.TgChatID,
			Text:   ErrMsgCouldNotSendSchedule,
		})
		return errors.Join(err, err2)
	}
	return err
}

func WeekScheduleMarkup(conf model.ScheduleConfig) models.ReplyMarkup {
	if conf.Group != nil {
		return UpdateScheduleMarkup("group", string(conf.Group.GroupName))
	} else if conf.Teacher != nil {
		return UpdateScheduleMarkup("teacher", conf.Teacher.TeacherID)
	} else {
		return nil
	}
}
func UpdateScheduleMarkup(kind, value string) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{UpdateInlineButton(kind, value)}},
	}
}
func UpdateInlineButton(kind, value string) models.InlineKeyboardButton {
	_ = CallbackCommand{Command: "update_" + kind, Args: []string{value, time.Now().Format("20060102150405")}}
	return models.InlineKeyboardButton{
		Text: "Обновить",
		CallbackData: fmt.Sprintf("update_%s\n%s\n%s",
			kind, value,
			time.Now().Format("20060102150405"),
			// NOTE: Time is added to prevent editing message error when the content is the same.
		),
	}
}

func IsVacation(t time.Time) bool {
	if t.Month() == time.August {
		return true
	}
	if t.Month() != time.July {
		return false
	}

	// Find first sunday of July
	firstSun := firstSunday(time.July, t.Year())

	// Vacation starts from the furst Sunday of July
	return t.Equal(firstSun) || t.After(firstSun)
}

func firstSunday(m time.Month, y int) time.Time {
	date := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	weekday := date.Weekday()
	daysToAdd := (time.Sunday - weekday + 7) % 7
	if daysToAdd == 0 && weekday != time.Sunday {
		daysToAdd = 7
	}

	return date.AddDate(0, 0, int(daysToAdd))
}
