package messenger

import (
	"bytes"
	"context"
	"errors"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// errBotNotConnected marks a transient restart: the bot is temporarily nil
// while botservice reconnects it on a (possibly different) proxy. The send is
// retried but this is not reported as a proxy failure.
var errBotNotConnected = errors.New("telegram bot is not connected")

// Option configures a Telegram messenger.
type Option func(*Telegram)

// WithNetworkFailureHook registers a callback invoked when a send chain
// exhausted all retries after at least one genuine network error (broken pipe,
// timeout, refused...). proxy is the proxy the failed attempt ran through, or
// empty when unknown. The hook is not called for business errors or while the
// bot was merely reconnecting.
func WithNetworkFailureHook(hook func(err error, proxy string)) Option {
	return func(t *Telegram) { t.onNetworkError = hook }
}

// WithProxyAccessor sets a function returning the proxy the currently running
// bot is built with. It is resolved right after a network error, so the caller
// can capture and ban the exact proxy that failed.
func WithProxyAccessor(getProxy func() string) Option {
	return func(t *Telegram) { t.getProxy = getProxy }
}

// Telegram adapts a lazily-connected Telegram bot to the Messenger interface.
// The accessor is resolved on every call so the adapter can be built before
// the bot connects (bots connect asynchronously after acquiring a proxy).
type Telegram struct {
	getBot         func() *bot.Bot
	getProxy       func() string
	onNetworkError func(error, string)
}

// NewTelegram wraps a bot accessor. getBot must return the Telegram bot once
// it is connected and nil before that.
func NewTelegram(getBot func() *bot.Bot, opts ...Option) *Telegram {
	t := &Telegram{getBot: getBot}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

func (t *Telegram) client() (*bot.Bot, error) {
	if t == nil || t.getBot == nil {
		return nil, errors.New("telegram bot accessor is not set")
	}
	if b := t.getBot(); b != nil {
		return b, nil
	}
	return nil, errBotNotConnected
}

// sendWithRetry re-runs send until it succeeds. The send closure re-resolves
// the bot on each attempt so a proxy switch between attempts is picked up. The
// failure hook fires only when at least one attempt failed with a genuine
// network error (a reconnect-only chain or a business error does not count),
// carrying the proxy captured at the moment of that failure.
func sendWithRetry[T any](
	ctx context.Context,
	getProxy func() string,
	onFailure func(error, string),
	send func() (T, error),
) (T, error) {
	var sawNetworkError bool
	var failingProxy string
	v, err := botutil.RetryNetwork(ctx, func(netErr error) {
		if !errors.Is(netErr, errBotNotConnected) {
			sawNetworkError = true
			if getProxy != nil {
				failingProxy = getProxy()
			}
		}
	}, send)
	if err != nil && sawNetworkError && onFailure != nil {
		onFailure(err, failingProxy)
	}
	return v, err
}

func (t *Telegram) SendMessagePeer(ctx context.Context, peerID int64, text string, opts ...SendOptions) (int, error) {
	return sendWithRetry(ctx, t.getProxy, t.onNetworkError, func() (int, error) {
		b, err := t.client()
		if err != nil {
			return 0, err
		}
		params := &bot.SendMessageParams{
			ChatID:    peerID,
			Text:      text,
			ParseMode: models.ParseModeHTML,
		}
		if markup, ok := scheduleMarkupFromOpts(opts); ok {
			params.ReplyMarkup = markup
		}
		msg, err := b.SendMessage(ctx, params)
		if err != nil {
			return 0, err
		}
		return msg.ID, nil
	})
}

func (t *Telegram) SendPhotoPeer(ctx context.Context, peerID int64, filename string, data []byte, caption string, opts ...SendOptions) error {
	_, err := sendWithRetry(ctx, t.getProxy, t.onNetworkError, func() (struct{}, error) {
		b, err := t.client()
		if err != nil {
			return struct{}{}, err
		}
		params := &bot.SendPhotoParams{
			ChatID:    peerID,
			Photo:     &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)},
			Caption:   caption,
			ParseMode: models.ParseModeHTML,
		}
		if markup, ok := scheduleMarkupFromOpts(opts); ok {
			params.ReplyMarkup = markup
		}
		_, err = b.SendPhoto(ctx, params)
		return struct{}{}, err
	})
	return err
}

func scheduleMarkupFromOpts(opts []SendOptions) (models.InlineKeyboardMarkup, bool) {
	for _, o := range opts {
		if o.Buttons != nil {
			return scheduleMarkup(o.Buttons), true
		}
	}
	return models.InlineKeyboardMarkup{}, false
}

// scheduleMarkup renders ScheduleButtons as the same inline keyboard used by
// the interactive views: the week photo layout (day-jump buttons + link +
// refresh) when no day is highlighted, or the day-text layout otherwise.
func scheduleMarkup(b *ScheduleButtons) models.InlineKeyboardMarkup {
	if b.CurrentIdx >= 0 {
		return botutil.DayScheduleMarkup(b.Value, b.Days, b.CurrentIdx, b.LinkURL)
	}
	return botutil.WeekScheduleMarkupFromValue(b.Value, b.Days, b.LinkURL)
}

func (t *Telegram) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	_, err := sendWithRetry(ctx, t.getProxy, t.onNetworkError, func() (struct{}, error) {
		b, err := t.client()
		if err != nil {
			return struct{}{}, err
		}
		_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
		return struct{}{}, err
	})
	return err
}

func (t *Telegram) IsForbidden(err error) bool {
	return errors.Is(err, bot.ErrorForbidden)
}

func (t *Telegram) IsAdmin(ctx context.Context, peerID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	b, err := t.client()
	if err != nil {
		return false, err
	}
	member, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: peerID, UserID: userID})
	if err != nil {
		return false, err
	}
	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator:
		return true, nil
	}
	return false, nil
}
