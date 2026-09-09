package mainbot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const subscriptionsPerPage = 8

func subscriptionMenu(chat *model.Chat, subscriptions []model.ScheduleSubscription, page int) (string, models.InlineKeyboardMarkup) {
	lastPage := max(0, (len(subscriptions)-1)/subscriptionsPerPage)
	page = max(0, min(page, lastPage))
	start := page * subscriptionsPerPage
	end := min(start+subscriptionsPerPage, len(subscriptions))
	var text strings.Builder
	text.WriteString("<b>Группы для ежедневной рассылки</b>\n")
	rows := make([][]models.InlineKeyboardButton, 0)
	if len(subscriptions) == 0 {
		text.WriteString("\nГруппы пока не выбраны.\n")
	}
	for _, subscription := range subscriptions[start:end] {
		primary := chat.GroupName != nil && *chat.GroupName == subscription.GroupName
		fmt.Fprintf(&text, "\n• %s", html.EscapeString(string(subscription.GroupName)))
		if primary {
			text.WriteString(" — основная")
		} else {
			rows = append(rows, []models.InlineKeyboardButton{{
				Text:         "Удалить " + string(subscription.GroupName),
				CallbackData: fmt.Sprintf("%s\n%d\n%d", botutil.CallbackCommandRemoveSubscription, subscription.ID, page),
			}})
		}
	}
	if lastPage > 0 {
		fmt.Fprintf(&text, "\n\nСтраница %d из %d", page+1, lastPage+1)
		var navigation []models.InlineKeyboardButton
		if page > 0 {
			navigation = append(navigation, models.InlineKeyboardButton{Text: "←", CallbackData: fmt.Sprintf("%s\n%d", botutil.CallbackCommandConfigSubscriptions, page-1)})
		}
		if page < lastPage {
			navigation = append(navigation, models.InlineKeyboardButton{Text: "→", CallbackData: fmt.Sprintf("%s\n%d", botutil.CallbackCommandConfigSubscriptions, page+1)})
		}
		rows = append(rows, navigation)
	}
	if chat.DailySendingTime == nil {
		text.WriteString("\n\nРассылка выключена. Установите время, чтобы получать расписания всех подключённых групп.")
	} else {
		fmt.Fprintf(&text, "\n\nВсе расписания приходят в %s по Екатеринбургу.", html.EscapeString(*chat.DailySendingTime))
	}
	text.WriteString("\nОсновная группа используется для команд и напоминаний.")
	rows = append(rows,
		[]models.InlineKeyboardButton{{Text: "Добавить группу", CallbackData: botutil.CallbackCommandAddGroup}},
		[]models.InlineKeyboardButton{{Text: "Изменить основную группу", CallbackData: botutil.CallbackCommandConfigGroup}},
		[]models.InlineKeyboardButton{{Text: "Установить время", CallbackData: botutil.CallbackCommandConfigDailyTime}},
	)
	if chat.DailySendingTime != nil {
		rows = append(rows, []models.InlineKeyboardButton{{Text: "Выключить рассылку", CallbackData: botutil.CallbackCommandDailyOff}})
	}
	rows = append(rows, []models.InlineKeyboardButton{{Text: "Назад к настройкам", CallbackData: botutil.CallbackCommandSettings}})
	return text.String(), models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (h *handler) sendSubscriptionsMenu(ctx context.Context, b *bot.Bot, chatID int64, threadID int) error {
	chat, err := h.Chat.GetChat(ctx, chatID)
	if err != nil {
		return err
	}
	subscriptions, err := h.Chat.GetScheduleSubscriptions(ctx, chat.ID)
	if err != nil {
		return err
	}
	text, markup := subscriptionMenu(chat, subscriptions, 0)
	_, err = botutil.SendMessageWithRetry(ctx, b, &bot.SendMessageParams{
		ChatID: chat.PeerID, MessageThreadID: threadID, Text: text,
		ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
	})
	return err
}

func (h *handler) editSubscriptionsMenu(ctx context.Context, b *bot.Bot, update *models.Update, page int) error {
	chat, ok := getCtxChat(ctx)
	if !ok {
		return ErrNoChatContext
	}
	current, err := h.Chat.GetChat(ctx, chat.ID)
	if err != nil {
		return err
	}
	subscriptions, err := h.Chat.GetScheduleSubscriptions(ctx, current.ID)
	if err != nil {
		return err
	}
	text, markup := subscriptionMenu(current, subscriptions, page)
	message := update.CallbackQuery.Message.Message
	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID: message.Chat.ID, MessageID: message.ID, Text: text,
		ParseMode: models.ParseModeHTML, ReplyMarkup: markup,
	})
	if botutil.IsMessageNotModified(err) {
		return nil
	}
	return err
}

func (h *handler) handleCQSubscriptions(ctx context.Context, b *bot.Bot, update *models.Update) {
	if err := h.finishAddingGroup(ctx); err != nil {
		h.answerSubscriptionAction(ctx, b, update, "", err)
		return
	}
	page, _ := strconv.Atoi(botutil.ParseCallbackData(update.CallbackQuery.Data).Arg(0))
	err := h.editSubscriptionsMenu(ctx, b, update, page)
	h.answerSubscriptionAction(ctx, b, update, "", err)
}

func (h *handler) handleCQAddGroup(ctx context.Context, b *bot.Bot, update *models.Update) {
	chat, ok := getCtxChat(ctx)
	if !ok {
		h.answerSubscriptionAction(ctx, b, update, "", ErrNoChatContext)
		return
	}
	err := h.sendDepartmentSelectionMenu(ctx, b, chat, update, true)
	h.answerSubscriptionAction(ctx, b, update, "", err)
}

func (h *handler) handleCQRemoveSubscription(ctx context.Context, b *bot.Bot, update *models.Update) {
	chat, ok := getCtxChat(ctx)
	if !ok {
		h.answerSubscriptionAction(ctx, b, update, "", ErrNoChatContext)
		return
	}
	command := botutil.ParseCallbackData(update.CallbackQuery.Data)
	id, err := strconv.ParseInt(command.Arg(0), 10, 64)
	if err != nil || id <= 0 {
		h.answerSubscriptionAction(ctx, b, update, "Откройте список групп заново через /settings", nil)
		return
	}
	removed, err := h.Chat.RemoveScheduleSubscription(ctx, chat.ID, id)
	if errors.Is(err, repository.ErrPrimarySubscription) {
		h.answerSubscriptionAction(ctx, b, update, "Эта группа стала основной. Её можно заменить в настройках.", nil)
		return
	}
	if err != nil {
		h.answerSubscriptionAction(ctx, b, update, "", err)
		return
	}
	page, _ := strconv.Atoi(command.Arg(1))
	err = h.editSubscriptionsMenu(ctx, b, update, page)
	text := "Подписка уже удалена"
	if removed {
		text = "Группа удалена из рассылки"
	}
	h.answerSubscriptionAction(ctx, b, update, text, err)
}

func (h *handler) handleCQSettings(ctx context.Context, b *bot.Bot, update *models.Update) {
	if err := h.finishAddingGroup(ctx); err != nil {
		h.answerSubscriptionAction(ctx, b, update, "", err)
		return
	}
	chat, ok := getCtxChat(ctx)
	if !ok {
		h.answerSubscriptionAction(ctx, b, update, "", ErrNoChatContext)
		return
	}
	err := updateSettingsMenuMessage(ctx, b, update.CallbackQuery.Message.Message, chat)
	h.answerSubscriptionAction(ctx, b, update, "", err)
}

func (h *handler) finishAddingGroup(ctx context.Context) error {
	chat, ok := getCtxChat(ctx)
	if !ok {
		return ErrNoChatContext
	}
	if chat.State == model.ChatStateAddingGroup {
		return h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault))
	}
	return nil
}

func (h *handler) answerSubscriptionAction(ctx context.Context, b *bot.Bot, update *models.Update, text string, err error) {
	if err != nil {
		addHandlerCtxErr(ctx, err)
		text = botutil.ErrMsgTryLater
	}
	_, answerErr := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID, Text: text})
	addHandlerCtxErr(ctx, answerErr)
}
