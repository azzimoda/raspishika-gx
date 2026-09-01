package adminbot

import (
	"context"
	"errors"
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
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

func newHandler(s *service.Services, reporter reporter.Reporter, broadcast *service.BroadcastService) *handler {
	return &handler{Services: s, Reporter: reporter, broadcast: broadcast}
}

type handler struct {
	*service.Services
	reporter.Reporter
	broadcast *service.BroadcastService
	flowMu    sync.Mutex
	flow      *broadcastFlow
}

func (h *handler) registerHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "start", bot.MatchTypeCommandStartOnly, h.handleCmdStart)
	b.RegisterHandler(bot.HandlerTypeMessageText, "dashboard", bot.MatchTypeCommandStartOnly, h.handleCmdDashboard)
	b.RegisterHandler(bot.HandlerTypeMessageText, "broadcast", bot.MatchTypeCommandStartOnly, h.handleCmdBroadcast)
	b.RegisterHandler(bot.HandlerTypeMessageText, "cancel", bot.MatchTypeCommandStartOnly, h.handleCmdCancel)

	// Broadcast wizard callbacks
	registerBroadcastCallback := func(command string, f bot.HandlerFunc) {
		b.RegisterHandlerRegexp(bot.HandlerTypeCallbackQueryData, callbackDataRegexp(command), f)
	}
	registerBroadcastCallback(botutil.CallbackCommandBroadcastAll, h.handleBroadcastAudience)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastPriv, h.handleBroadcastAudience)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastGroupChats, h.handleBroadcastAudience)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastByGroup, h.handleBroadcastAudience)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastDept, h.handleBroadcastAudience)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastActive, h.handleBroadcastAudience)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastConfirm, h.handleBroadcastConfirm)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastEdit, h.handleBroadcastEdit)
	registerBroadcastCallback(botutil.CallbackCommandBroadcastCancel, h.handleBroadcastCancel)

	// While a broadcast wizard is active, ordinary text messages feed it.
	b.RegisterHandlerMatchFunc(h.broadcastTextMatch, h.handleBroadcastText)
}

// broadcastTextMatch matches a plain message only while a broadcast wizard is
// waiting for a spec or text input.
func (h *handler) broadcastTextMatch(update *models.Update) bool {
	if update.Message == nil || update.Message.Text == "" {
		return false
	}
	h.flowMu.Lock()
	defer h.flowMu.Unlock()
	if h.flow == nil {
		return false
	}
	return h.flow.step == broadcastStepSpec || h.flow.step == broadcastStepText
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

func (h *handler) handleCmdDashboard(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, args := botutil.ParseCommand(update.Message.Text)

	spec, ok := parsePeriodSpec(args)
	if !ok {
		spec, _ = parsePeriodSpec("1d")
	}

	g, gctx := errgroup.WithContext(ctx)
	var generalStats *service.GeneralStatsData
	var configStats *service.ConfigStatsData
	g.Go(func() (err error) {
		generalStats, err = h.Stats.GetGeneralStats(gctx, spec.start, spec.end)
		return err
	})
	g.Go(func() (err error) {
		configStats, err = h.Stats.GetConfigStats(gctx)
		return err
	})
	if err := g.Wait(); err != nil {
		h.Report().Err(err).Msg("Failed to collect statistics")
		return
	}

	blocks := buildDashboard(generalStats, configStats, spec)
	_, err := h.Report().MsgRich("Dashboard", blocks)
	if err != nil {
		h.Report().Err(err).Msg("Failed to send dashboard")
	}
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
