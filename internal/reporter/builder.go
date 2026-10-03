package reporter

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/avast/retry-go/v5"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// NewReportBuilder returns a new report builder with the given bot and recipient chat ID.
func NewReportBuilder(b *bot.Bot, recipientChatID int64) ReportBuilder {

	rb := EmptyReportBuilder()
	rb.bot = b
	rb.recipientChatID = recipientChatID
	return rb
}

// EmptyReportBuilder returns a new report builder with the default format function.
func EmptyReportBuilder() ReportBuilder {

	return ReportBuilder{debugValues: make(map[string]any), formatFunc: defaultFormatFunc}
}

// ReportData carries everything a FormatFunc needs to render a report.
type ReportData struct {
	Msg         string
	DebugValues map[string]any
	Err         error
	Platform    model.Platform
}

// FormatFunc is a function that formats a report message into a [bot.SendRichMessageParams].
type FormatFunc func(ReportData) *bot.SendRichMessageParams

// ReportBuilder is a struct that holds the state of a report builder.
type ReportBuilder struct {
	bot             *bot.Bot
	recipientChatID int64

	formatFunc FormatFunc

	isTemp      bool
	error       error
	platform    model.Platform
	debugValues map[string]any
}

// WithFormatFunc sets the format function for the report builder.
func (rc ReportBuilder) WithFormatFunc(f FormatFunc) ReportBuilder {

	rc.formatFunc = f
	return rc
}

// Platform sets the platform label for the report. Empty values are ignored so
// a zero-valued default in AppReporter does not wipe an explicit per-chat value
// set by the caller.
func (r ReportBuilder) Platform(p model.Platform) ReportBuilder {
	if p != "" {
		r.platform = p
	}
	return r
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
	return rc.send(ReportData{Msg: "", DebugValues: rc.debugValues, Err: rc.error, Platform: rc.platform},
		rc.formatFunc(ReportData{Msg: "", DebugValues: rc.debugValues, Err: rc.error, Platform: rc.platform}), 2)
}

// Msgf sends a report message with the given format string and arguments.
func (rc ReportBuilder) Msgf(format string, a ...any) (*Report, error) {
	msg := fmt.Sprintf(format, a...)
	data := ReportData{Msg: msg, DebugValues: rc.debugValues, Err: rc.error, Platform: rc.platform}
	return rc.send(data, rc.formatFunc(data), 2)
}

// Msg sends a report message with the given string.
func (rc ReportBuilder) Msg(msg string) (*Report, error) {
	data := ReportData{Msg: msg, DebugValues: rc.debugValues, Err: rc.error, Platform: rc.platform}
	return rc.send(data, rc.formatFunc(data), 2)
}

// MsgRich sends a report message built from structured rich message blocks
// (headings, dividers, tables, details, ...). Unlike [ReportBuilder.Msg] it
// bypasses the format function and sends the given blocks as-is, so the caller
// controls the rich message composition.
func (rc ReportBuilder) MsgRich(msg string, blocks []models.InputRichBlock) (*Report, error) {
	return rc.MsgRichWithMarkup(msg, blocks, nil)
}

// MsgRichWithMarkup is like [ReportBuilder.MsgRich] but attaches the given
// reply markup (e.g. an inline keyboard) to the sent message.
func (rc ReportBuilder) MsgRichWithMarkup(msg string, blocks []models.InputRichBlock, markup models.ReplyMarkup) (*Report, error) {

	data := ReportData{Msg: msg, DebugValues: rc.debugValues, Err: rc.error, Platform: rc.platform}

	if data.Err != nil {
		blocks = append([]models.InputRichBlock{errorRichBlock(data.Err.Error())}, blocks...)
	}

	params := &bot.SendRichMessageParams{
		RichMessage: models.InputRichMessage{Blocks: blocks},
		ReplyMarkup: markup,
	}
	return rc.send(data, params, 3)
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
// this logger: 2 for Msg/Msgf/Send, 3 for MsgRichWithMarkup.
func (rc ReportBuilder) send(data ReportData, params *bot.SendRichMessageParams, callerSkipFrames int) (*Report, error) {

	log.Trace().Msg("Sending report...")

	{
		var logEvent *zerolog.Event
		if data.Err != nil {
			logEvent = log.Error().Err(data.Err)
		} else {
			logEvent = log.Debug()
		}
		logEvent = logEvent.CallerSkipFrame(callerSkipFrames)
		for key, value := range data.DebugValues {
			logEvent.Any(key, value)
		}
		logEvent.Msgf("Report: %s", data.Msg)
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
		log.Error().Err(err).Str("msg", data.Msg).Int("blocks", len(params.RichMessage.Blocks)).Msg("Failed to send report message")
	}

	return &Report{ReportBuilder: rc, Message: message}, err
}

// Report is a struct that holds the state of a report.
type Report struct {
	ReportBuilder
	Message *models.Message
}

// DeleteMessage removes the report message from the chat.
func (r *Report) DeleteMessage() (isDeleted bool, err error) {

	isDeleted, err = r.bot.DeleteMessage(context.Background(), &bot.DeleteMessageParams{
		ChatID:    r.recipientChatID,
		MessageID: r.Message.ID,
	})
	log.Trace().Msgf("The report message is deleted")
	return
}

func defaultFormatFunc(data ReportData) *bot.SendRichMessageParams {

	var html strings.Builder

	// Platform
	if data.Platform != "" {
		fmt.Fprintf(&html, "<p><b>Platform:</b> %s</p>\n", data.Platform.Label())
	}

	// Chat
	chatID := extract[model.ChatID]("chatID", data.DebugValues)
	delete(data.DebugValues, "chatID")
	fullName := extract[string]("fullName", data.DebugValues)
	delete(data.DebugValues, "fullName")
	username := extract[string]("username", data.DebugValues)
	delete(data.DebugValues, "username")
	if chatID != 0 {
		var parts []string
		if fullName != "" {
			parts = append(parts, fullName)
		}
		if username != "" {
			parts = append(parts, "@"+username)
		}
		parts = append(parts, fmt.Sprintf("<code>%d</code>", int64(chatID)))
		fmt.Fprintf(&html, "<p><b>Chat:</b> %s</p>\n", strings.Join(parts, " / "))
	}

	// Group
	groupName := extract[model.GroupName]("group", data.DebugValues)
	delete(data.DebugValues, "group")
	if groupName != "" {
		// Group name is rendered by the app-level formatter with live lookup;
		// the fallback formatter only has the raw value.
		fmt.Fprintf(&html, "<p><b>Group:</b> %s</p>", groupName)
	}

	// Other debug
	if len(data.DebugValues) > 0 {

		keys := make([]string, 0, len(data.DebugValues))
		for k := range data.DebugValues {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		html.WriteString(`<table bordered striped>`)
		for _, name := range keys {
			fmt.Fprintf(&html, `<tr><td>%s</td><td><code>%+v</code></td></tr>`, name, data.DebugValues[name])
		}
		html.WriteString(`</table>`)
	}

	// Error
	if data.Err != nil {
		fmt.Fprintf(&html, "<blockquote><b>Error:</b><br><code>%s</code></blockquote>\n", data.Err.Error())
	}

	// Message text
	fmt.Fprintf(&html, "<blockquote>%s</blockquote>", data.Msg)

	return &bot.SendRichMessageParams{RichMessage: models.InputRichMessage{HTML: html.String()}}
}

func extract[T any](key string, values map[string]any) T {
	var zero T
	anyValue, ok := values[key]
	if !ok {
		return zero
	}
	value, ok := anyValue.(T)
	if !ok {
		return zero
	}
	return value
}
