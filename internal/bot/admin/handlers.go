package adminbot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go/v5"
	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
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
}

func (*handler) handleDefault(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Any("update", update).Msg("Unhandled update")
}

func (h *handler) handleCmdStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, argsStr := botutil.ParseCommand(update.Message.Text)
	if argsStr == "" {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: "Welcome back, Master!"})
		return
	}

	args := botutil.ParseStartCommand(argsStr)
	switch args.Arg(0) {
	case "chat":
		usernameOrChatID := args.Arg(1)

		chat, err := h.Chat.GetChatByUsernameOrChatID(ctx, usernameOrChatID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				h.Report().Msg("Chat not found")
			} else {
				h.Report().Err(err).Msg("Error getting chat")
			}
			return
		}
		h.reportChat(chat).Msg("Chat found")
	case "group":
		groupName := args.Arg(1)

		group, err := h.Schedule.GetGroupByName(ctx, model.GroupName(groupName))
		if err != nil {
			h.Report().Err(err).Msg("Error getting group")
			return
		}

		h.Report().Debug("name", group.GroupName).Debug("department", group.DepartmentName).
			Debug("gr", group.GroupID).Debug("sid", group.DepartmentID).
			Msg("Group found") // TODO: Add data about chats with this group.
	}
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

Actual/Potential requests: %d / %d
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

		logs.RequestsActual, logs.RequestsPotential,
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

var me *models.User
var meOnce sync.Once

func GetMe(b *bot.Bot) *models.User {
	meOnce.Do(func() {
		if err := retry.New(retry.Attempts(5), retry.Delay(100*time.Millisecond)).Do(func() (err error) {
			me, err = b.GetMe(context.Background())
			return
		}); err != nil {
			log.Error().Err(err).Msg("Failed to get me")
		}
	})
	return me
}
