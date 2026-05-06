package adminbot

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"github.com/azzimoda/raspishika-gx/pkg/config"
)

func (h *handler) meddlewareThatAllowsAccessOnlyForAdmin(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			log.Debug().Msg("Admin bot: not message ignored")
			return
		}
		if update.Message.From.ID != viper.GetInt64(config.KeyAdminID) {
			log.Debug().Int64("chatID", update.Message.From.ID).Msg("Admin bot: not admin restricted")
			return
		}
		log.Debug().Msg("Admin bot: update allowed")
		next(ctx, b, update)
	}
}
