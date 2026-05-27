package reporter

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/model" // TODO: Remove this dependency to internal package.
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

type reportKey string

const (
	tempKey     reportKey = "temp"
	errKey      reportKey = "err"
	chatIDKey   reportKey = "chat_id"
	usernameKey reportKey = "username"
	debugKey    reportKey = "debug"
)

func NewReportBuilder(bot *bot.Bot, recipientChatID int64) ReportBuilder {
	return ReportBuilder{bot: bot, recipientChatID: recipientChatID, Context: defaultContext()}
}
func EmptyReportConfig() ReportBuilder { return ReportBuilder{Context: defaultContext()} }

func defaultContext() context.Context {
	ctx := context.Background()

	ctx = context.WithValue(ctx, tempKey, false)
	ctx = context.WithValue(ctx, errKey, nil)
	ctx = context.WithValue(ctx, chatIDKey, model.ChatID(0))
	ctx = context.WithValue(ctx, usernameKey, model.UserName(""))
	ctx = context.WithValue(ctx, debugKey, make(map[string]any))

	return ctx
}

type ReportBuilder struct {
	bot             *bot.Bot
	recipientChatID int64
	context.Context
}

// Err adds error to report message.
func (r ReportBuilder) Err(err error) ReportBuilder { return r.withValue(errKey, err) }

// Chat sets the chat, whose message caused the error. It can be either a Chat object or a chat ID.
func (r ReportBuilder) Chat(chatOrID any) ReportBuilder {
	if tgChatID, ok := chatOrID.(int64); ok {
		r = r.withValue(chatIDKey, tgChatID)
	} else if chat, ok := chatOrID.(*model.Chat); ok {
		r = r.withValue(chatIDKey, chat.TgChatID).withValue(usernameKey, refutil.DerefOrTypeDefault(chat.UserName))
	} else {
		log.Error().Type("type", chatOrID).Any("arg", chatOrID).Msg("Wrong type of chat argument")
	}
	return r
}

// Debug sets a debug object with the given name and value.
func (r ReportBuilder) Debug(name string, value any) ReportBuilder {
	debugValues, ok := r.Value(debugKey).(map[string]any)
	if !ok {
		debugValues = make(map[string]any)
		debugValues[name] = value
		r.Context = context.WithValue(r.Context, debugKey, debugValues)
	} else {
		debugValues[name] = value
	}
	return r
}

func (r ReportBuilder) withValue(key any, value any) ReportBuilder {
	r.Context = context.WithValue(r.Context, key, value)
	return r
}

func (rc ReportBuilder) Send() (*Report, error) { return rc.Msg("") }
func (rc ReportBuilder) Msgf(format string, a ...any) (*Report, error) {
	return rc.Msg(fmt.Sprintf(format, a...))
}
func (rc ReportBuilder) Msg(msg string) (*Report, error) {
	log.Trace().Msg("Sending report...")

	reportErr, ok := rc.Value(errKey).(error)
	if !ok {
		reportErr = nil
	}
	chatID := rc.Value(chatIDKey).(model.ChatID)
	username := rc.Value(usernameKey).(model.UserName)
	debugObjects := rc.Value(debugKey).(map[string]any)

	{
		var logEvent *zerolog.Event
		if reportErr != nil {
			logEvent = log.Error().Err(reportErr)
		} else {
			logEvent = log.Debug()
		}
		logEvent = logEvent.CallerSkipFrame(1)
		for key, value := range debugObjects {
			logEvent.Any(key, value)
		}
		logEvent.Any("chatID", chatID).Any("username", username).Msgf("Report: %s", msg)
	}

	if rc.bot == nil {
		if log.Logger.GetLevel() == zerolog.TraceLevel {
			log.Warn().Msg("ReportConfig.Send: bot is nil")
		}
		return nil, fmt.Errorf("bot is nil")
	}

	// Assemble the message text.
	var msgText strings.Builder

	// Chat
	if chatID != 0 {
		fmt.Fprintf(&msgText, "<code>/chat %d</code> @%s\n", chatID, username)
	}

	// Error
	if reportErr != nil {
		fmt.Fprintf(&msgText, "Error:\n<pre>%s</pre>\n", reportErr.Error())
	}

	// Debug objects
	if len(debugObjects) > 0 {
		msgText.WriteString("\n<pre>")
		for name, value := range debugObjects {
			fmt.Fprintf(&msgText, "%s = %+v\n", name, value)
		}
		msgText.WriteString("</pre>\n")
	}

	// Message text
	msgText.WriteString(msg)

	// Send the message.
	message, err := rc.bot.SendMessage(context.Background(), &bot.SendMessageParams{
		ChatID:    rc.recipientChatID,
		Text:      msgText.String(),
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		rc.bot.SendMessage(context.Background(), &bot.SendMessageParams{
			ChatID:    rc.recipientChatID,
			Text:      fmt.Sprintf("Failed to send report:\n<pre>%s</pre>", err),
			ParseMode: models.ParseModeHTML,
		})
		log.Error().Err(err).Str("text", msgText.String()).Msg("Failed to send report message")
	}

	return &Report{rc, message}, err
}

type Report struct {
	ReportBuilder
	Message *models.Message
}

func (r *Report) RemoveMessage() (isDeleted bool, err error) {
	isDeleted, err = r.bot.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
		ChatID:    r.recipientChatID,
		MessageID: r.Message.ID,
	})
	log.Trace().Msgf("The report message is deleted")
	return
}
