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
	tempKey      reportKey = "temp"
	errKey       reportKey = "err"
	chatIDKey    reportKey = "chat_id"
	usernameKey  reportKey = "username"
	groupNameKey reportKey = "group_name"
	debugKey     reportKey = "debug"
)

func NewReportBuilder(bot *bot.Bot, recipientChatID int64) ReportBuilder {
	b := EmptyReportBuilder()
	b.bot = bot
	b.recipientChatID = recipientChatID
	return b
}
func EmptyReportBuilder() ReportBuilder { return ReportBuilder{debugValues: make(map[string]any)} }

type ReportBuilder struct {
	bot             *bot.Bot
	recipientChatID int64

	isTemp      bool
	error       error
	chat        *model.Chat
	debugValues map[string]any
}

// Err adds error to report message.
func (r ReportBuilder) Err(err error) ReportBuilder {
	r.error = err
	return r
}

// Chat sets the chat ID, username, and group name of the chat, whose message caused the error.
//
// If the chatOrID is an int64, the chat ID is set to the value, and the username and group name are set to "??".
//
// If the chatOrID is a *model.Chat, the chat ID, username, and group name are set to the corresponding values from the chat.
func (r ReportBuilder) Chat(chatOrID any) ReportBuilder {
	if tgChatID, ok := chatOrID.(int64); ok {
		r.chat = new(model.Chat{TgChatID: model.ChatID(tgChatID)})
	} else if chat, ok := chatOrID.(*model.Chat); ok {
		r.chat = chat
	} else {
		log.Error().Type("type", chatOrID).Any("arg", chatOrID).Msg("Wrong type of chat argument")
	}
	return r
}

// Debug sets a debug object with the given name and value.
func (r ReportBuilder) Debug(name string, value any) ReportBuilder {
	r.debugValues[name] = value
	return r
}

func (rc ReportBuilder) Send() (*Report, error) { return rc.Msg("") }
func (rc ReportBuilder) Msgf(format string, a ...any) (*Report, error) {
	return rc.Msg(fmt.Sprintf(format, a...))
}
func (rc ReportBuilder) Msg(msg string) (*Report, error) {
	log.Trace().Msg("Sending report...")

	{
		var logEvent *zerolog.Event
		if rc.error != nil {
			logEvent = log.Error().Err(rc.error)
		} else {
			logEvent = log.Debug()
		}
		logEvent = logEvent.CallerSkipFrame(1)
		for key, value := range rc.debugValues {
			logEvent.Any(key, value)
		}
		logEvent.Msgf("Report: %s", msg)
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
	if rc.chat != nil {
		fmt.Fprintf(&msgText, "<code>/chat %d</code> @%s\n", rc.chat.TgChatID, rc.chat.UserName)
		fmt.Fprintf(&msgText, "Group: <code>%s</code>\n", refutil.DerefOrTypeDefault(rc.chat.GroupName))
	}

	// Error
	if rc.error != nil {
		fmt.Fprintf(&msgText, "Error:\n<pre>%s</pre>\n", rc.error.Error())
	}

	// Debug objects
	if len(rc.debugValues) > 0 {
		msgText.WriteString("\n<pre>")
		for name, value := range rc.debugValues {
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
