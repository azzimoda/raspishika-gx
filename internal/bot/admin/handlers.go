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

	var html strings.Builder

	html.WriteString("<h3>General Statistics</h3>\n")
	fmt.Fprintf(&html, "<p>for last %s (%s)</p>\n", periodStr, dur)

	html.WriteString("<h4>Chats</h4>\n")
	fmt.Fprintf(&html, "<p>Total: %d<br>Private/Group: %d / %d<br>Active/Semiactive/Inactive: %d / %d / %d<br>New reigstered: %d</p>\n",
		chat.ChatsTotal,
		chat.ChatsPrivate, chat.ChatsTotal-chat.ChatsPrivate,
		chat.ChatsActive, chat.ChatsSemiactive, chat.ChatsInactive,
		chat.ChatsNew)

	if len(chat.ChatsNewGrouped) > 0 {
		html.WriteString("<details><summary>New chats grouped by year</summary>\n")
		html.WriteString("<table bordered><tr><th>Year</th><th>Count</th></tr>\n")
		for year, count := range chat.ChatsNewGrouped {
			yearStr := "none"
			if year != 0 {
				yearStr = strconv.Itoa(year)
			}
			fmt.Fprintf(&html, "<tr><td>%s</td><td>%d</td></tr>\n", yearStr, count)
		}
		html.WriteString("</table></details>\n")
	}

	html.WriteString("<h4>Groups</h4>\n")
	fmt.Fprintf(&html, "<p>Groups: %d<br>CpG: %.2f</p>\n", chat.GroupsTotal, chat.ChatsPerGroup)

	html.WriteString("<h4>Updates</h4>\n")
	fmt.Fprintf(&html, "<p>Updates: %d<br>Success: %d (%.1f%%)</p>\n",
		logs.UpdatesTotal, logs.UpdatesSuccess, (float64(logs.UpdatesSuccess) / (float64(logs.UpdatesTotal) + 0.001) * 100))

	html.WriteString("<h4>Broadcast</h4>\n")
	fmt.Fprintf(&html, "<p>Broadcast Tasks/Sends: %d / %d<br>Success: %d (%.1f%%)</p>\n",
		logs.BroadcastTasks, logs.BroadcastLogs,
		logs.BroadcastSuccess, (float64(logs.BroadcastSuccess) / (float64(logs.BroadcastLogs) + 0.001) * 100))

	html.WriteString("<h4>Requests</h4>\n")
	fmt.Fprintf(&html, "<p>Actual/Potential requests: %d / %d</p>\n", logs.RequestsActual, logs.RequestsPotential)

	return html.String()
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

	var html strings.Builder

	html.WriteString("<h3>Config Report</h3>\n")

	fmt.Fprintf(&html, "<p>Total Chats: %d</p>\n", stats.ChatsTotal)
	fmt.Fprintf(&html, "<p>Total Configured: %d (%.1f%%)</p>\n",
		stats.ConfiguredGroupsTotal, (float64(stats.ConfiguredGroupsTotal)+0.1)/float64(stats.ChatsTotal)*100)
	fmt.Fprintf(&html, "<p>Unique Group: %d</p>\n", stats.ConfiguredGroupsUnique)
	fmt.Fprintf(&html, "<p>Daily/Pair/Change: %d / %d / %d</p>\n", stats.DailyEnabled, stats.PairEnabled, stats.ChangeEnabled)
	fmt.Fprintf(&html, "<p>Dark theme: %d (%.1f%%)</p>\n",
		stats.DarkEnabled, (float64(stats.DarkEnabled)+0.1)/float64(stats.ChatsTotal)*100)

	if len(stats.ChatCountByTime) > 0 {
		fmt.Fprintf(&html, "<details><summary>Chat Count by Time</summary>\n")
		html.WriteString("<table bordered><tr><th>Time</th><th>Count</th></tr>\n")
		for _, c := range stats.ChatCountByTime {
			fmt.Fprintf(&html, "<tr><td><code>%s</code></td><td><code>%d</code></td></tr>\n", c.Time, c.Count)
		}
		fmt.Fprintf(&html, "</table></details>\n")
	}

	return html.String()
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
