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
	if args == "" {
		args = "1d"
	}

	duration, ok := parsePeriod(args)
	if !ok {
		duration = 24 * time.Hour
	}

	generalStats, err := h.Stats.GetGeneralStats(ctx, duration)
	if err != nil {
		h.Report().Err(err).Msg("Failed to collect log statistics")
		return
	}

	_, err = h.Report().Msg(buildGeneralReportText(generalStats, duration, args))
	if err != nil {
		h.Report().Err(err).Msg("Failed to send statistics report")
	}
}
func buildGeneralReportText(stats *service.GeneralStatsData, dur time.Duration, periodStr string) string {
	chat := stats.ChatStatsData
	logs := stats.LogStatsData
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

	return fmt.Sprintf(`STATISTICS FOR LAST %s (%s)

Total: %d
Private/Group: %d / %d
Active/Semiactive/Inactive: %d / %d / %d
New reigstered: %d

%s

Groups: %d
CpG: %.2f

Updates: %d
Success: %d (%.1f%%)

Broadcast Tasks/Sends: %d / %d
Success: %d (%.1f%%)
Daily/Pair/Change: %d / %d / %d
`,
		periodStr, dur,
		chat.ChatsTotal,
		chat.ChatsPrivate, chat.ChatsTotal-chat.ChatsPrivate,
		chat.ChatsActive, chat.ChatsSemiactive, chat.ChatsInactive,
		chat.ChatsNew,
		newChatsGroupedStr.String(),

		chat.GroupsTotal, chat.ChatsPerGroup,

		logs.UpdatesTotal,
		logs.UpdatesSuccess, (float64(logs.UpdatesSuccess) / (float64(logs.UpdatesTotal) + 0.001) * 100),

		logs.BroadcastTasks, logs.BroadcastLogs,
		logs.BroadcastSuccess, (float64(logs.BroadcastSuccess) / (float64(logs.BroadcastLogs) + 0.001) * 100),
		logs.BroadcastDaily, logs.BroadcastPair, logs.BroadcastChange,
	)
}

func (h *handler) handleCmdConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	stats, err := h.Stats.GetConfigStats(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get config stats")
		return
	}
	_, err = h.Report().Msg(buildConfigReportTextHTML(stats))
	if err != nil {
		log.Error().Err(err).Msg("Failed to send config report")
	}
}
func buildConfigReportTextHTML(stats *service.ConfigStatsData) string {
	var sb strings.Builder
	sb.WriteString("Config Report:\n\n")
	fmt.Fprintf(&sb, "Total Chats: %d\n", stats.ChatsTotal)
	fmt.Fprintf(&sb, "Total Configured: %d (%.1f%%)\n",
		stats.ConfiguredGroupsTotal, (float64(stats.ConfiguredGroupsTotal)+0.1)/float64(stats.ChatsTotal)*100)
	fmt.Fprintf(&sb, "Unique Group: %d\n", stats.ConfiguredGroupsUnique)
	fmt.Fprintf(&sb, "Daily/Pair/Change: %d / %d / %d\n", stats.DailyEnabled, stats.PairEnabled, stats.ChangeEnabled)
	fmt.Fprintf(&sb, "Dark theme: %d (%.1f%%)\n",
		stats.DarkEnabled, (float64(stats.DarkEnabled)+0.1)/float64(stats.ChatsTotal)*100)
	fmt.Fprintf(&sb, "\nConfigured daily times:\n<pre>\n")
	for _, c := range stats.ChatCountByTime {
		fmt.Fprintf(&sb, "%s => %d\n", c.Time, c.Count)
	}
	fmt.Fprintf(&sb, "</pre>\n")
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
