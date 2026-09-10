package mainbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	startMessageText = `
Привет! Со мной ты можешь легко получить расписание своей группы и любого преподавателя

Для начала нужно задать свою группу, после этого можно будет использовать команды /week, /tomorrow, /today и настроить рассылки. Прочие функции перечислены в /help

Помимо команд можно использовать кнопки клавиатуры, а также меня можно добавить в групповой чат

Подпишись на канал разработчика @mazzaLLM, где ты можешь найти последние новости и обсудить бота в комментариях`
	helpMessageText = `Основные команды:

• /week — Расписание на неделю
• /tomorrow — Расписание на завтра
• /today — Распасание на сегодня
• /teacher — Расписание преподавателя
• /settings — Меню настроек
• /stop — Удалить данные о себе и остановить рассылки
• /help — Это сообщение

Доступные настройки (/settings):

• Ежедневная рассылка: можно задать время, в которое бот будет присылать расписание на неделю каждый день
• Напоминания за 15 минут перед парами
• Уведомления об изменениях в расписании
• В групповом чате можно настроить уровен доступа участников к командам (/access)

Также:

• Бота можно добавить в групповой чат
• Можно получить расписание любой группы, просто прислав её название, например: "ИСПт-22-(9)-2" или "испт 22 9 2"

По всем вопросам пишите в комментарии или директ канала @mazzaLLM.`
)

func (h *handler) handleCmdStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling command start...")

	// The admin deep links ("Get chat"/"Get group" buttons in admin reports)
	// point at this bot and must only ever serve ADMIN_ID.
	if _, argsStr := botutil.ParseCommand(update.Message.Text); argsStr != "" {
		if update.Message.From != nil && update.Message.From.ID == h.adminID {
			h.handleAdminStart(ctx, b, update, argsStr)
			return
		}
		log.Warn().Int64("userID", update.Message.From.ID).Msg("Ignored admin deep link from a non-admin user")
		return
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            startMessageText,
	})
	addHandlerCtxErr(ctx, err)

	if err == nil {
		chat, ok := ctx.Value(keyChat).(*model.Chat)
		if !ok {
			addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
			return
		}
		if chat.GroupName == nil || string(refutil.DerefOrTypeDefault(chat.GroupName)) == "" {
			h.offerToSetGroupOnStart(ctx, b, chat, update)
		}
	}
	log.Info().Msg("Handled start")
}

// handleAdminStart responds to the admin deep links in the report messages:
// /start chat=<chatIDOrUsername> and /start group=<groupName>.
func (h *handler) handleAdminStart(ctx context.Context, _ *bot.Bot, update *models.Update, argsStr string) {
	args := botutil.ParseStartCommand(argsStr)
	switch args.Arg(0) {
	case "chat":
		chat, err := h.Chat.GetChatByUsernameOrChatID(ctx, args.Arg(1))
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				h.Report().Msg("Chat not found")
			} else {
				h.Report().Err(err).Msg("Error getting chat")
			}
			return
		}
		h.ReportChat(chat).Msg("Chat found")
	case "group":
		group, err := h.Schedule.GetGroupByName(ctx, model.GroupName(args.Arg(1)))
		if err != nil {
			h.Report().Err(err).Msg("Error getting group")
			return
		}
		h.Report().Debug("name", group.GroupName).Debug("department", group.DepartmentName).
			Debug("gr", group.GroupID).Debug("sid", group.DepartmentID).
			Msg("Group found")
	}
}
func (h *handler) offerToSetGroupOnStart(ctx context.Context, b *bot.Bot, chat *model.Chat, update *models.Update) {
	err := h.sendDepartmentSelectionMenu(ctx, b, chat, update)
	addHandlerCtxErr(ctx, err)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send department selection menu")
		botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
			ChatID:          chat.PeerID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            botutil.ErrMsgTryLater,
		})
	}
}

func (h *handler) handleCmdHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            helpMessageText,
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Handled help")
}

func (h *handler) handleCmdStop(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling command stop...")

	message := update.Message
	chatID := message.Chat.ID
	messageThreadID := message.MessageThreadID

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, MessageThreadID: messageThreadID,
			Text: "Не удалось удалить данные чата, попробуйте позже",
		})
		return
	}

	if err := h.Chat.DeleteChat(ctx, chat.ID); err != nil {
		addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID, MessageThreadID: messageThreadID,
			Text: "Не удалось удалить данные чата, попробуйте позже",
		})
		return
	}
	log.Trace().Int64("tgChatID", int64(chat.PeerID)).Msg("Chat deleted from DB")

	count, err := h.Chat.CountAllChats(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to count all chats")
	}

	h.ReportChat(chat).Msgf("User stopped the bot ☹ (×%d rests)", count)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID, MessageThreadID: messageThreadID,
		Text:        "Ваши данные удалены и рассылки остановлены. Спасибо, что пользовались ботом!",
		ReplyMarkup: models.ReplyKeyboardRemove{RemoveKeyboard: true},
	})
	addHandlerCtxErr(ctx, err)
	log.Info().Msg("Handled command stop")
}

func (h *handler) handleCmdCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	log.Debug().Msg("Handling command cancel...")

	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Произошла ошибка, попробуйте позже",
		})
		return
	}

	if err := h.Chat.UpdateChat(ctx, chat.WithState(model.ChatStateDefault)); err != nil {
		addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Произошла ошибка, попробуйте позже",
		})
		return
	}
	log.Trace().Msg("Set user's state to default")

	err := botutil.SendTempMessage(ctx, b, 10*time.Second, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            "Настройка отменена",
		// NOTE: This command is accessible during setting of group and time.
	})
	addHandlerCtxErr(ctx, err)

	log.Info().Msg("Hanlded command cancel")
}

// handleCQDelete deletes the message of the callback button
func (h *handler) handleCQDelete(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		log.Debug().Any("update", update).Msg("Message is inaccessible, cannot delete")
		return
	}

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: update.CallbackQuery.ID})

	if deleted, err := botutil.DeleteMessage(ctx, b, msg); err != nil {
		log.Warn().Err(err).Any("update", update).Msg("Failed to delete message")
		addHandlerCtxErr(ctx, err)
	} else if !deleted {
		log.Warn().Any("update", update).Msg("Message is not deleted")
	}

	log.Info().Msg("Handled CQ delete")
}
