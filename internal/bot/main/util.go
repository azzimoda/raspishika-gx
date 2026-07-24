package mainbot

import (
	"context"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
)

type contextKey string

const (
	keyChat           contextKey = "chat"
	keyError          contextKey = "error"
	keyNoLogFlag      contextKey = "default_handler"
	keyGroupOrTeacher contextKey = "group_or_teacher"
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

func sendChatActionTyping(ctx context.Context, b *bot.Bot, chatID any, messageThreadID int) context.CancelFunc {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		for {
			if _, err := b.SendChatAction(ctx, &bot.SendChatActionParams{
				ChatID:          chatID,
				MessageThreadID: messageThreadID,
				Action:          models.ChatActionTyping,
			}); err != nil {
				addHandlerCtxErr(ctx, err)
				log.Error().Err(err).Msg("Failed to send chat action")
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	return cancel
}

func (h *handler) ReportChat(chat *model.Chat) reporter.ReportBuilder {
	r := h.Report()
	if chat == nil {
		return r
	}
	return r.Debug("chatID", chat.TgChatID).Debug("username", chat.UserName).
		Debug("group", refutil.DerefOrTypeDefault(chat.GroupName))
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
