package adminbot

import (
	"context"
	"fmt"
	"strconv"
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
		return
	}
	logStats, err := h.Log.GetGeneralStats(ctx, duration)
	if err != nil {
		h.Report().Err(err).Msg("Failed to collect log statistics")
		return
	}

	_, err = h.Report().Msg(buildGeneralReportText(chatStats, logStats, duration))
	if err != nil {
		h.Report().Err(err).Msg("Failed to send statistics report")
	}
}
func buildGeneralReportText(chat *service.ChatStatsData, log *service.LogStatsData, dur time.Duration) string {
	var newChatsGroupedStr strings.Builder
	if chat.ChatsNewGrouped != nil {
		for year, count := range chat.ChatsNewGrouped {
			yearStr := "none"
			if year != 0 {
				yearStr = strconv.Itoa(year)
			}
			fmt.Fprintf(&newChatsGroupedStr, "%s => %d\n", yearStr, count)
		}
	}

	return fmt.Sprintf(`STATISTICS FOR LAST %s

Total: %d
Private/Group: %d / %d
Active/Semiactive/Inactive: %d / %d / %d
New reigstered: %d

%s

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
		chat.ChatsActive, chat.ChatsSemiactive, chat.ChatsInactive,
		chat.ChatsNew,
		newChatsGroupedStr.String(),

		chat.GroupsTotal, chat.ChatsPerGroup,

		log.UpdatesTotal,
		log.UpdatesSuccess, (float64(log.UpdatesSuccess) / (float64(log.UpdatesTotal) + 0.001) * 100),

		log.BroadcastChats,
		log.BroadcastDaily, log.BroadcastPair, log.BroadcastChange,
		log.BroadcastFails,
	)
}

func (h *handler) handleCmdConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats, err := h.Chat.GetConfigStats(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get config stats")
		return
	}
	_, err = h.Report().Msg(buildConfigReportText(stats))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send config report")
	}
}
func buildConfigReportText(stats *service.ConfigStatsData) string {
	var sb strings.Builder
	sb.WriteString("Config Report:\n\n")
	fmt.Fprintf(&sb, "Total Chats: %d\n", stats.ChatsTotal)
	fmt.Fprintf(&sb, "Total Configured: %d\n", stats.ConfiguredGroupsTotal)
	fmt.Fprintf(&sb, "Unique Configured: %d\n", stats.ConfiguredGroupsUnique)
	fmt.Fprintf(&sb, "Daily: %d\n", stats.DailyEnabled)
	fmt.Fprintf(&sb, "Pair: %d\n", stats.PairEnabled)
	fmt.Fprintf(&sb, "Change: %d\n", stats.ChangeEnabled)
	fmt.Fprintf(&sb, "Dark: %d\n", stats.DarkEnabled)
	fmt.Fprintf(&sb, "\n")
	for time, count := range stats.ChatCountByTime {
		fmt.Fprintf(&sb, "%s => %d\n", time, count)
	}
	return sb.String()
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
