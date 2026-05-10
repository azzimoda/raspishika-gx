package mainbot

import (
	"context"
	"fmt"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

const startMessageText = `
Привет! Со мной ты можешь легко получить расписание своей группы и любого преподавателя

Для начала нужно задать свою группу, после этого можно будет использовать команды /week, /tomorrow, /today и настроить рассылки. Прочие функции перечислены в /help

Помимо команд можно использовать кнопки клавиатуры, а также меня можно добавить в групповой чат

Подпишись на канал разработчика @mazzaLLM, где ты можешь найти последние новости и обсудить бота в комментариях`

const helpMessageText = `Основные команды:

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

func (h *handler) handleCmdStart(ctx context.Context, b *bot.Bot, update *models.Update) {
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
		} else if chat.GroupName == nil || string(refutil.DerefOrTypeDefault(chat.GroupName)) == "" {
			err := h.sendDepartmentSelectionMenu(ctx, b, chat, update.Message.MessageThreadID)
			addHandlerCtxErr(ctx, err)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to send department selection menu")
				botutil.SendErrorMessage(ctx, b, &bot.SendMessageParams{
					ChatID:          chat.TgChatID,
					MessageThreadID: update.Message.MessageThreadID,
					Text:            botutil.ErrMsgTryLater, // TODO: Move those error message constants into this package
				})
				return
			}
		}
	}
}
func (h *handler) handleCmdHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            helpMessageText,
	})
	addHandlerCtxErr(ctx, err)
}
func (h *handler) handleCmdStop(ctx context.Context, b *bot.Bot, update *models.Update) {
	chat, ok := ctx.Value(keyChat).(*model.Chat)
	if !ok {
		addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Не удалось удалить данные чата, попробуйте позже",
		})
		return
	}

	if err := h.Chat.DeleteChat(ctx, chat.ID); err != nil {
		addHandlerCtxErr(ctx, fmt.Errorf("failed to get chat from handler context"))
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:          update.Message.Chat.ID,
			MessageThreadID: update.Message.MessageThreadID,
			Text:            "Не удалось удалить данные чата, попробуйте позже",
		})
		return
	}

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            "Ваши данные удалены и рассылки остановлены. Спасибо, что пользовались ботом!",
		ReplyMarkup:     models.ReplyKeyboardRemove{RemoveKeyboard: true},
	})
	addHandlerCtxErr(ctx, err)
}

func (h *handler) handleCmdCancel(ctx context.Context, b *bot.Bot, update *models.Update) {
	unimplementedHandler()
}

// handleCQDelete deletes the message of the callback button
func (h *handler) handleCQDelete(ctx context.Context, b *bot.Bot, update *models.Update) {
	msg := update.CallbackQuery.Message.Message
	if msg == nil {
		log.Debug().Any("update", update).Msg("Message is inaccessible")
		return
	}

	if deleted, err := botutil.DeleteMessage(ctx, b, msg); err != nil {
		log.Warn().Err(err).Any("update", update).Msg("Failed to delete message")
		addHandlerCtxErr(ctx, err)
	} else if !deleted {
		log.Debug().Any("update", update).Msg("Message is not deleted")
	}
}
