package adminbot

import (
	"context"

	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func (h *handler) meddlewareThatAllowsAccessOnlyForAdmin(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		var userID int64
		switch {
		case update.Message != nil:
			userID = update.Message.From.ID
		case update.CallbackQuery != nil:
			userID = update.CallbackQuery.From.ID
		default:
			log.Debug().Msg("Admin bot: not a message or callback ignored")
			return
		}
		if userID != viper.GetInt64(config.KeyAdminID) {
			log.Debug().Int64("userID", userID).Msg("Admin bot: not admin restricted")
			return
		}
		log.Debug().Msg("Admin bot: update allowed")
		next(ctx, b, update)
	}
}
