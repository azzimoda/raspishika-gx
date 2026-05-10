package adminbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/service"
)

var startTime = time.Now() // TODO: Use to calculate uptime

func newHandler(s *service.Services, reporter reporter.Reporter) *handler {
	return &handler{Services: s, Reporter: reporter}
}

type handler struct {
	*service.Services
	reporter.Reporter
}

func (h *handler) registerHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, h.handleCmdStart)
	b.RegisterHandler(bot.HandlerTypeMessageText, "stats", bot.MatchTypeCommandStartOnly, h.handleCmdStats)
	b.RegisterHandler(bot.HandlerTypeMessageText, "config", bot.MatchTypeCommandStartOnly, h.handleCmdConfig)
	b.RegisterHandler(bot.HandlerTypeMessageText, "chat", bot.MatchTypeCommandStartOnly, h.handleCmdChat)
	b.RegisterHandler(bot.HandlerTypeMessageText, "group", bot.MatchTypeCommandStartOnly, h.handleCmdGroup)

	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		return update.Message != nil && strings.HasPrefix(update.Message.Text, "@") // message with username
	}, h.handleTextChat)
	b.RegisterHandlerMatchFunc(func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}

		_, err := h.Schedule.ValidateGroupName(context.Background(), model.GroupName(update.Message.Text))
		return err == nil // group name is valid
	}, h.handleTextGroup)
}

func (*handler) handleDefault(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Any("update", update).Msg("Unhandled update")
}

func (*handler) handleCmdStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "Welcome back, Master!"})
}

func (h *handler) handleCmdStats(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, args := botutil.ParseCommand(update.Message.Text)
	duration, ok := parsePeriod(args)
	if !ok {
		duration = 24 * time.Hour
	}

	chatStats, err := h.Chat.GetGeneralStats(ctx, duration)
	if err != nil {
		h.Report().Err(err).Msg("Failed to collect chat statistics")
	}
	logStats, err := h.Log.GetGeneralStats(ctx, duration)
	if err != nil {
		h.Report().Err(err).Msg("Failed to collect log statistics")
	}

	_, err = h.Report().Msg(buildReportText(chatStats, logStats, duration))
	if err != nil {
		h.Report().Err(err).Msg("Failed to send statistics report")
	}
}
func buildReportText(chat *service.ChatStatsData, log *service.LogStatsData, dur time.Duration) string {
	return fmt.Sprintf(`STATISTICS FOR LAST %s

Total: %d
Private/Group: %d / %d
Active/Inactive: %d / %d
New reigstered: %d

%v

Groups: %d
CpG: %.2f

Updates: %d
Success: %d (%.1f%%)

Broadcasts: %d
Daily/Pair/Change: %d / %d / %d
Fails: %d`,
		dur,
		chat.ChatsTotal,
		chat.ChatsPrivate, chat.ChatsTotal-chat.ChatsPrivate,
		chat.ChatsTotal-chat.ChatsInactive, chat.ChatsInactive,
		chat.ChatsNew,
		chat.ChatsNewGrouped,

		chat.GroupsTotal, chat.ChatsPerGroup,

		log.UpdatesTotal,
		log.UpdatesSuccess, (float64(log.UpdatesSuccess) / (float64(log.UpdatesTotal) + 0.001) * 100),

		log.BroadcastChats,
		log.BroadcastDaily, log.BroadcastPair, log.BroadcastChange,
		log.BroadcastFails,
	)
}

func (*handler) handleCmdConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Warn().Msg("Unimplemented handler")
}

func (*handler) handleCmdChat(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Warn().Msg("Unimplemented handler")
}
func (*handler) handleTextChat(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Warn().Any("update", update).Msg("Unimplemented handler")
}

func (*handler) handleCmdGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Warn().Msg("Unimplemented handler")
}
func (*handler) handleTextGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Warn().Any("update", update).Msg("Unimplemented handler")
}

var me *models.User

func GetMe(b *bot.Bot) *models.User {
	if me != nil {
		return me
	}

	var err error
	me, err = b.GetMe(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get me")
		return nil
	}
	return me
}
