package reporter

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// NewReportBuilder returns a new report builder with the given bot and recipient chat ID.
func NewReportBuilder(bot *bot.Bot, recipientChatID int64) ReportBuilder {
	b := EmptyReportBuilder()
	b.bot = bot
	b.recipientChatID = recipientChatID
	return b
}

// EmptyReportBuilder returns a new report builder with the default format function.
func EmptyReportBuilder() ReportBuilder {
	return ReportBuilder{debugValues: make(map[string]any), formatFunc: defaultFormatFunc}
}

// FormatFunc is a function that formats a report message into a [bot.SendRichMessageParams].
type FormatFunc func(msg string, debugValues map[string]any, err error) *bot.SendRichMessageParams

// ReportBuilder is a struct that holds the state of a report builder.
type ReportBuilder struct {
	bot             *bot.Bot
	recipientChatID int64

	formatFunc FormatFunc

	isTemp      bool
	error       error
	debugValues map[string]any
}

// WithFormatFunc sets the format function for the report builder.
func (rc ReportBuilder) WithFormatFunc(f FormatFunc) ReportBuilder {
	rc.formatFunc = f
	return rc
}

// Err adds error to report message.
func (r ReportBuilder) Err(err error) ReportBuilder {
	r.error = err
	return r
}

// Debug sets a debug object with the given name and value.
func (r ReportBuilder) Debug(name string, value any) ReportBuilder {
	r.debugValues[name] = value
	return r
}

// Send sends a report message with an empty string.
func (rc ReportBuilder) Send() (*Report, error) { return rc.Msg("") }

// Msgf sends a report message with the given format string and arguments.
func (rc ReportBuilder) Msgf(format string, a ...any) (*Report, error) {
	return rc.Msg(fmt.Sprintf(format, a...))
}

// Msg sends a report message with the given string.
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

	params := rc.formatFunc(msg, rc.debugValues, rc.error)
	params.ChatID = rc.recipientChatID

	// Send the message
	message, err := rc.bot.SendRichMessage(context.Background(), params)
	if err != nil {
		rc.bot.SendMessage(context.Background(), &bot.SendMessageParams{
			ChatID:    rc.recipientChatID,
			Text:      fmt.Sprintf("Failed to send report:\n<pre>%s</pre>", err),
			ParseMode: models.ParseModeHTML,
		})
		log.Error().Err(err).Str("text", params.RichMessage.HTML).Msg("Failed to send report message")
	}

	return &Report{rc, message}, err
}

// Report is a struct that holds the state of a report.
type Report struct {
	ReportBuilder
	Message *models.Message
}

// RemoveMessage removes the report message from the chat.
func (r *Report) RemoveMessage() (isDeleted bool, err error) {
	isDeleted, err = r.bot.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
		ChatID:    r.recipientChatID,
		MessageID: r.Message.ID,
	})
	log.Trace().Msgf("The report message is deleted")
	return
}

func defaultFormatFunc(msg string, debugValues map[string]any, err error) *bot.SendRichMessageParams {
	var html strings.Builder

	// Error
	if err != nil {
		fmt.Fprintf(&html, "Error:\n<pre>%s</pre>\n", err.Error())
	}

	// Debug objects
	if len(debugValues) > 0 {
		keys := make([]string, 0, len(debugValues))
		for k := range debugValues {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		html.WriteString(`<table bordered striped>
			<caption>Striped</caption>`)
		for _, name := range keys {
			fmt.Fprintf(&html, `<tr><td>%s</td><td><code>%+v</code></td></tr>`, name, debugValues[name])
		}
		html.WriteString(`</table>`)
	}

	// Message text
	fmt.Fprintf(&html, "<blockquote>%s</blockquote>", msg)

	return &bot.SendRichMessageParams{
		RichMessage: models.InputRichMessage{HTML: html.String()},
	}
}
