package mainbot

import (
	"context"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	keyChat      contextKey = "chat"
	keyError     contextKey = "error"
	keyNoLogFlag contextKey = "default_handler"
)

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

func (h *handler) ReportChat(chat *model.Chat) reporter.ReportBuilder {
	r := h.Report()
	if chat == nil {
		return r
	}
	return r.Debug("chatID", chat.TgChatID).Debug("username", chat.UserName).
		Debug("group", refutil.DerefOrTypeDefault(chat.GroupName))
}
