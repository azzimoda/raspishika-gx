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

	MsgGroupRemoved = "Группа %s больше не существует на сайте колледжа.\n\nНастройки сброшены — выберите новую группу через /settings"
)

// ScheduleLinkLabel is the text of the inline link button opening the college
// schedule page.
const ScheduleLinkLabel = "Сайт"

// CollegeScheduleURL is the fallback schedule page used when a specific
// group or teacher page cannot be built.
const CollegeScheduleURL = "https://coworking.tyuiu.ru/shs/index.php"

func SendErrorMessage(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) error {
	err := SendTempMessage(ctx, b, 7*time.Second, params)
	if err != nil {
		log.Error().Err(err).Any("params", params).Msg("Failed to send error message")
	}
	return err
}

const sendRetryAttempts = 3

// IsNetworkError reports whether err is a transport-level error (proxy/network),
// as opposed to a Telegram business error that should not be retried.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, bot.ErrorNotFound),
		errors.Is(err, bot.ErrorConflict),
		errors.Is(err, bot.ErrorForbidden),
		errors.Is(err, bot.ErrorBadRequest),
		errors.Is(err, bot.ErrorUnauthorized),
		bot.IsTooManyRequestsError(err),
		bot.IsMigrateError(err):
		return false
	default:
		return true
	}
}

// SendMessageWithRetry sends a message, retrying on network errors so replies are not
// lost when a proxy dies mid-flight.
func SendMessageWithRetry(ctx context.Context, b *bot.Bot, params *bot.SendMessageParams) (*models.Message, error) {
	var lastErr error
	for attempt := range sendRetryAttempts {
		msg, err := b.SendMessage(ctx, params)
		if err == nil {
			return msg, nil
		}
		lastErr = err
		if !IsNetworkError(err) {
			return nil, err
		}
		log.Warn().Err(err).Int("attempt", attempt+1).Msg("Network error sending message, retrying...")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return nil, lastErr
}

// SendTempMessage sends a temporary message that will be automatically deleted after the specified duration.
func SendTempMessage(ctx context.Context, b *bot.Bot, dur time.Duration, params *bot.SendMessageParams) error {
	msg, err := SendMessageWithRetry(ctx, b, params)
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
	if i < 0 || i >= len(c.Args) {
		return ""
	}
	return c.Args[i]
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
	ok, err := b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: message.Chat.ID, MessageID: message.ID})
	if err != nil {
		log.Warn().Err(err).Msg("Message not deleted with error")
	} else if !ok {
		log.Warn().Msg("Message not deleted")
	}
	return ok, err
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
	linkURL string,
	isOld bool,
) error {

	log.Debug().Msg("Sending week schedule message...")

	var errs []error

	if _, err := SendMessageWithRetry(ctx, b, &bot.SendMessageParams{
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

	replyMarkup := WeekScheduleMarkup(conf, linkURL)
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

// SchedulePageURL builds the URL of the college schedule page for conf.
// For a group the departments are not needed;
// for a teacher they are used to build the "shed" query arguments.
func SchedulePageURL(conf model.ScheduleConfig, departments []model.Department) string {
	return model.ScheduleURL(conf, departments)
}

// DepartmentsGetter fetches the list of departments needed to build
// a teacher schedule page URL.
type DepartmentsGetter interface {
	GetDepartments(context.Context) ([]model.Department, error)
}

// TeacherSchedulePageURL builds the schedule page URL for a teacher,
// falling back to CollegeScheduleURL when departments cannot be fetched.
func TeacherSchedulePageURL(ctx context.Context, deps DepartmentsGetter, teacher *model.Teacher) string {
	departments, err := deps.GetDepartments(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get departments for teacher schedule URL, using fallback")
		return CollegeScheduleURL
	}
	return SchedulePageURL(model.TeacherScheduleConfig(teacher, false), departments)
}

// WeekScheduleMarkup returns the keyboard for a week schedule photo.
func WeekScheduleMarkup(conf model.ScheduleConfig, linkURL string) models.InlineKeyboardMarkup {

	value := ""
	switch {
	case conf.Group != nil:
		value = string(conf.Group.GroupName)
	case conf.Teacher != nil:
		value = conf.Teacher.TeacherID
	default:
		return models.InlineKeyboardMarkup{}
	}
	return UpdateScheduleMarkup(UpdateKindWeek, value, linkURL)
}

// UpdateScheduleMarkup returns the keyboard with a link to the college
// schedule page (when linkURL is non-empty) and the update button.
func UpdateScheduleMarkup(kind UpdateKind, value, linkURL string) models.InlineKeyboardMarkup {
	row := make([]models.InlineKeyboardButton, 0, 2)
	if linkURL != "" {
		row = append(row, ScheduleLinkButton(linkURL))
	}
	row = append(row, UpdateInlineButton(kind, value))
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{row}}
}

// LinkOnlyMarkup returns the keyboard with a single link button
// to the college schedule page.
func LinkOnlyMarkup(linkURL string) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{ScheduleLinkButton(linkURL)}},
	}
}

// ScheduleLinkButton returns an inline link button opening the college
// schedule page.
func ScheduleLinkButton(linkURL string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: ScheduleLinkLabel, URL: linkURL}
}

// callbackTimestampLayout formats the cache-busting timestamp appended to
// update button callback data to prevent edit errors when the content is unchanged.
const callbackTimestampLayout = "20060102150405"

func UpdateInlineButton(kind UpdateKind, value string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{
		Text: "Обновить",
		CallbackData: NewCallbackCommand(
			kind.CallbackCommand(),
			value,
			time.Now().Format(callbackTimestampLayout),
		).String(),
	}
}

// Deprecated: use ScheduleService.IsVacation instead.
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
