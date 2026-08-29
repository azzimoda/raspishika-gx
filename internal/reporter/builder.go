package reporter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avast/retry-go/v5"
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
func (rc ReportBuilder) Send() (*Report, error) {
	return rc.send("", rc.formatFunc("", rc.debugValues, rc.error), 3)
}

// Msgf sends a report message with the given format string and arguments.
func (rc ReportBuilder) Msgf(format string, a ...any) (*Report, error) {
	msg := fmt.Sprintf(format, a...)
	return rc.send(msg, rc.formatFunc(msg, rc.debugValues, rc.error), 3)
}

// Msg sends a report message with the given string.
func (rc ReportBuilder) Msg(msg string) (*Report, error) {
	return rc.send(msg, rc.formatFunc(msg, rc.debugValues, rc.error), 2)
}

// MsgRich sends a report message built from structured rich message blocks
// (headings, dividers, tables, details, ...). Unlike [ReportBuilder.Msg] it
// bypasses the format function and sends the given blocks as-is, so the caller
// controls the rich message composition.
func (rc ReportBuilder) MsgRich(msg string, blocks []models.InputRichBlock) (*Report, error) {

	if rc.error != nil {
		blocks = append([]models.InputRichBlock{errorRichBlock(rc.error.Error())}, blocks...)
	}

	params := &bot.SendRichMessageParams{RichMessage: models.InputRichMessage{Blocks: blocks}}
	return rc.send(msg, params, 2)
}

// errorRichBlock builds a blockquote rich block with the given error message.
func errorRichBlock(errText string) models.InputRichBlock {
	return models.InputRichBlock{
		Type: models.RichBlockTypeBlockQuotation,
		InputRichBlockBlockQuotation: &models.InputRichBlockBlockQuotation{
			Type: models.RichBlockTypeBlockQuotation,
			Blocks: []models.InputRichBlock{
				{
					Type:                    models.RichBlockTypeParagraph,
					InputRichBlockParagraph: &models.InputRichBlockParagraph{Text: models.RichText{PlainText: errText}},
				},
			},
		},
	}
}

// send logs the report and sends the given rich message params to the report
// recipient with retries. Params must not carry a ChatID; it is set here.
// callerSkipFrames is the number of frames between the actual call site and
// this logger: 2 for Msg/MsgRich, 3 for Msgf/Send.
func (rc ReportBuilder) send(msg string, params *bot.SendRichMessageParams, callerSkipFrames int) (*Report, error) {

	log.Trace().Msg("Sending report...")

	{
		var logEvent *zerolog.Event
		if rc.error != nil {
			logEvent = log.Error().Err(rc.error)
		} else {
			logEvent = log.Debug()
		}
		logEvent = logEvent.CallerSkipFrame(callerSkipFrames)
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

	params.ChatID = rc.recipientChatID

	// Send the message
	var message *models.Message
	err := retry.New(
		retry.Attempts(5), retry.Delay(500*time.Millisecond), retry.DelayType(retry.FullJitterBackoffDelay),
		retry.OnRetry(func(attempt uint, err error) {
			log.Error().Err(err).Msgf("Failed to send report; retry attempt %d", attempt)
		}),
	).Do(func() error {
		var err error
		message, err = rc.bot.SendRichMessage(context.Background(), params)
		return err
	})
	if err != nil {
		rc.bot.SendMessage(context.Background(), &bot.SendMessageParams{
			ChatID:    rc.recipientChatID,
			Text:      fmt.Sprintf("Failed to send report:\n<pre>%s</pre>", err),
			ParseMode: models.ParseModeHTML,
		})
		log.Error().Err(err).Str("msg", msg).Int("blocks", len(params.RichMessage.Blocks)).Msg("Failed to send report message")
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

		html.WriteString(`<table bordered striped>`)
		for _, name := range keys {
			fmt.Fprintf(&html, `<tr><td>%s</td><td><code>%+v</code></td></tr>`, name, debugValues[name])
		}
		html.WriteString(`</table>`)
	}

	// Message text
	fmt.Fprintf(&html, "<blockquote>%s</blockquote>", msg)

	return &bot.SendRichMessageParams{RichMessage: models.InputRichMessage{HTML: html.String()}}
}
