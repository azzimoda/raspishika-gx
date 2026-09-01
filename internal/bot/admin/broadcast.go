package adminbot

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// activeAudiencePeriod is the window in which a chat must have sent an update
// to count as "active" for the active audience filter.
const activeAudiencePeriod = 90 * 24 * time.Hour // 3 months

// broadcastStep identifies the current stage of the broadcast wizard.
type broadcastStep int

const (
	broadcastStepAudience broadcastStep = iota
	broadcastStepSpec
	broadcastStepText
	broadcastStepConfirm
)

// audienceKind is the target filter selected in the wizard.
type audienceKind int

const (
	audienceAll audienceKind = iota
	audiencePrivate
	audienceGroupChats
	audienceByGroup
	audienceByDepartment
	audienceActive
)

// broadcastFlow holds the in-memory state of a single broadcast wizard run.
// Since the admin bot only serves ADMIN_ID, a single session on the handler is
// enough.
type broadcastFlow struct {
	adminChatID int64
	step        broadcastStep
	audience    audienceKind
	spec        string // group or department name for audienceByGroup/audienceByDepartment
	text        string // plain text from the message
	html        string // text converted from entities for sending
	menuMessage *models.Message
}

func (h *handler) handleCmdBroadcast(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.flowMu.Lock()
	if h.flow != nil {
		h.flowMu.Unlock()
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "A broadcast is already in progress. Finish or /cancel it.",
		})
		return
	}
	h.flow = &broadcastFlow{adminChatID: update.Message.Chat.ID, step: broadcastStepAudience}
	flow := h.flow
	h.flowMu.Unlock()

	h.sendAudienceMenu(ctx, b, flow)
}

// handleCmdCancel aborts an in-progress broadcast wizard.
func (h *handler) handleCmdCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	chatID := update.Message.Chat.ID
	h.flowMu.Lock()
	if h.flow == nil {
		h.flowMu.Unlock()
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "No active broadcast."})
		return
	}
	h.flow = nil
	h.flowMu.Unlock()
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: "Broadcast cancelled."})
}

func (h *handler) sendAudienceMenu(ctx context.Context, b *bot.Bot, flow *broadcastFlow) {
	kb := [][]models.InlineKeyboardButton{
		{{Text: "Everyone", CallbackData: botutil.CallbackCommandBroadcastAll}},
		{{Text: "Private chats", CallbackData: botutil.CallbackCommandBroadcastPriv}},
		{{Text: "Groups", CallbackData: botutil.CallbackCommandBroadcastGroupChats}},
		{{Text: "By group", CallbackData: botutil.CallbackCommandBroadcastByGroup}},
		{{Text: "By department", CallbackData: botutil.CallbackCommandBroadcastDept}},
		{{Text: "Active", CallbackData: botutil.CallbackCommandBroadcastActive}},
		{{Text: "Cancel", CallbackData: botutil.CallbackCommandBroadcastCancel}},
	}
	mp := &bot.SendMessageParams{
		ChatID:      flow.adminChatID,
		Text:        "Who should receive the broadcast?",
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	}
	if m, err := b.SendMessage(ctx, mp); err == nil {
		flow.menuMessage = m
	}
}

func (h *handler) handleBroadcastAudience(ctx context.Context, b *bot.Bot, update *models.Update) {
	data := update.CallbackQuery.Data

	// Determine the transition without mutating shared state yet.
	var audience audienceKind
	var specPrompt string
	switch data {
	case botutil.CallbackCommandBroadcastAll:
		audience = audienceAll
	case botutil.CallbackCommandBroadcastPriv:
		audience = audiencePrivate
	case botutil.CallbackCommandBroadcastGroupChats:
		audience = audienceGroupChats
	case botutil.CallbackCommandBroadcastByGroup:
		audience = audienceByGroup
		specPrompt = "Enter the group name:"
	case botutil.CallbackCommandBroadcastDept:
		audience = audienceByDepartment
		specPrompt = "Enter the department name:"
	case botutil.CallbackCommandBroadcastActive:
		audience = audienceActive
	default:
		return
	}

	h.flowMu.Lock()
	flow := h.flow
	if flow == nil {
		h.flowMu.Unlock()
		return
	}
	flow.audience = audience
	if specPrompt != "" {
		flow.step = broadcastStepSpec
	} else {
		flow.step = broadcastStepText
	}
	h.flowMu.Unlock()

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
	if specPrompt != "" {
		h.sendPrompt(ctx, b, flow, specPrompt)
	} else {
		h.sendPrompt(ctx, b, flow, "Enter the broadcast text (formatting is preserved):")
	}
}

// sendPrompt clears the previous menu and asks the admin to type input.
func (h *handler) sendPrompt(ctx context.Context, b *bot.Bot, flow *broadcastFlow, prompt string) {
	if flow.menuMessage != nil {
		b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: flow.menuMessage.Chat.ID, MessageID: flow.menuMessage.ID})
		flow.menuMessage = nil
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: flow.adminChatID, Text: prompt})
}

// handleBroadcastText consumes a plain-text message while the wizard waits for
// a group/department name or the broadcast text.
func (h *handler) handleBroadcastText(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.flowMu.Lock()
	flow := h.flow
	if flow == nil {
		h.flowMu.Unlock()
		return
	}
	switch flow.step {
	case broadcastStepSpec:
		spec := strings.TrimSpace(update.Message.Text)
		if spec == "" {
			h.flowMu.Unlock()
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: flow.adminChatID, Text: "Name cannot be empty. Please try again:"})
			return
		}
		flow.spec = spec
		flow.step = broadcastStepText
		nextPrompt := "Enter the broadcast text (formatting is preserved):"
		h.flowMu.Unlock()
		h.sendPrompt(ctx, b, flow, nextPrompt)
	case broadcastStepText:
		if update.Message.Text == "" {
			h.flowMu.Unlock()
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: flow.adminChatID, Text: "Message cannot be empty. Please enter the text:"})
			return
		}
		flow.text = update.Message.Text
		flow.html = botutil.EntitiesToHTML(update.Message.Text, update.Message.Entities)
		flow.step = broadcastStepConfirm
		h.flowMu.Unlock()
		h.sendConfirm(ctx, b, flow)
	default:
		h.flowMu.Unlock()
	}
}

// sendConfirm shows a preview of the audience and text together with a
// confirm/cancel inline keyboard.
func (h *handler) sendConfirm(ctx context.Context, b *bot.Bot, flow *broadcastFlow) {
	chats, err := h.resolveAudience(context.Background(), flow)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: flow.adminChatID, Text: fmt.Sprintf("Failed to count recipients: %v", err)})
		h.resetFlow()
		return
	}
	preview := fmt.Sprintf(
		"Audience: <b>%s</b>\nRecipients: <b>%d</b>\n\nMessage preview:\n%s\n\nSend?",
		html.EscapeString(flow.audienceLabel()), len(chats), html.EscapeString(flow.text),
	)
	kb := [][]models.InlineKeyboardButton{
		{{Text: "Send", CallbackData: botutil.CallbackCommandBroadcastConfirm}},
		{{Text: "Edit text", CallbackData: botutil.CallbackCommandBroadcastEdit}},
		{{Text: "Cancel", CallbackData: botutil.CallbackCommandBroadcastCancel}},
	}
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      flow.adminChatID,
		Text:        preview,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: kb},
	})
}

// resolveAudience returns the chats matching the flow's chosen audience.
func (h *handler) resolveAudience(ctx context.Context, flow *broadcastFlow) ([]*model.Chat, error) {
	switch flow.audience {
	case audienceAll:
		return h.Chat.GetAllChats(ctx)
	case audiencePrivate:
		return h.Chat.GetPrivateChats(ctx)
	case audienceGroupChats:
		return h.Chat.GetGroupChats(ctx)
	case audienceByGroup:
		return h.Chat.GetChatsByGroup(ctx, model.GroupName(flow.spec))
	case audienceByDepartment:
		return h.Chat.GetChatsByDepartment(ctx, flow.spec)
	case audienceActive:
		return h.Chat.GetActiveChats(ctx, activeAudiencePeriod)
	default:
		return nil, fmt.Errorf("unknown audience")
	}
}

func (flow *broadcastFlow) audienceLabel() string {
	switch flow.audience {
	case audienceAll:
		return "everyone"
	case audiencePrivate:
		return "private chats"
	case audienceGroupChats:
		return "groups"
	case audienceByGroup:
		return fmt.Sprintf("group %q", flow.spec)
	case audienceByDepartment:
		return fmt.Sprintf("department %q", flow.spec)
	case audienceActive:
		return "active"
	default:
		return "unknown"
	}
}

func (h *handler) handleBroadcastConfirm(ctx context.Context, b *bot.Bot, update *models.Update) {
	// Lock for the whole confirm: the flow must be claimed atomically so two
	// concurrent "Send" callbacks cannot both pass the nil check and start a
	// duplicate broadcast. The lock is only contended briefly between updates.
	h.flowMu.Lock()
	flow := h.flow
	if flow == nil {
		h.flowMu.Unlock()
		return
	}

	// finishLocked sends a final message, clears the wizard, and releases the lock.
	finishLocked := func(msg string) {
		h.flow = nil
		h.flowMu.Unlock()
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: flow.adminChatID, Text: msg})
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})

	chats, err := h.resolveAudience(context.Background(), flow)
	if err != nil {
		finishLocked(fmt.Sprintf("Failed to get recipients: %v", err))
		return
	}
	if len(chats) == 0 {
		finishLocked("No recipients for the selected audience.")
		return
	}
	if flow.html == "" {
		finishLocked("Message text is empty.")
		return
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: flow.adminChatID,
		Text:   fmt.Sprintf("Starting broadcast to %d chats...", len(chats)),
	})
	html := flow.html
	adminChatID := flow.adminChatID
	h.flow = nil
	h.flowMu.Unlock()

	// BroadcastText runs the send in a service-tracked background job and
	// reports completion via the service reporter, so it never blocks here.
	if err := h.broadcast.BroadcastText(ctx, chats, html); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminChatID, Text: fmt.Sprintf("Failed to start broadcast: %v", err)})
	}
}

func (h *handler) handleBroadcastEdit(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.flowMu.Lock()
	flow := h.flow
	if flow == nil {
		h.flowMu.Unlock()
		return
	}
	flow.step = broadcastStepText
	h.flowMu.Unlock()

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: flow.adminChatID, Text: "Enter the new broadcast text:"})
}

func (h *handler) handleBroadcastCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	h.flowMu.Lock()
	flow := h.flow
	if flow == nil {
		h.flowMu.Unlock()
		return
	}
	adminChatID := flow.adminChatID
	h.flow = nil
	h.flowMu.Unlock()

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: adminChatID, Text: "Broadcast cancelled."})
}

func (h *handler) resetFlow() {
	h.flowMu.Lock()
	h.flow = nil
	h.flowMu.Unlock()
}
