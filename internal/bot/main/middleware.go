package mainbot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

var ErrUnknownUpdateType = errors.New("unknown update type")

// ensureChat middlware creates or updates chat in database before handling udpate and adds it to the context.
func (h *handler) ensureChat(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		chat, err := h.createOrUpdateChat(b, update)
		if errors.Is(err, ErrUnknownUpdateType) {
			next(ctx, b, update)
			return
		} else if err != nil {
			h.Report().Err(err).Msg("Failed to create or update chat")
			return
		}

		ctx = context.WithValue(ctx, keyChat, chat)
		next(ctx, b, update)
	}
}
func (h *handler) createOrUpdateChat(b *bot.Bot, update *models.Update) (*model.Chat, error) {
	var chatID model.ChatID
	var username model.UserName
	if update.Message != nil {
		chatID = model.ChatID(update.Message.Chat.ID)
		username = model.UserName(update.Message.Chat.Username)
	} else if update.CallbackQuery != nil {
		chatID = model.ChatID(update.CallbackQuery.Message.Message.Chat.ID)
		username = model.UserName(update.CallbackQuery.Message.Message.Chat.Username)
	} else {
		return nil, ErrUnknownUpdateType
	}

	chat := new(model.Chat{TgChatID: chatID, UserName: new(model.UserName(username))})
	created, err := h.Chat.CreateOrUpdateChat(context.Background(), chat)
	if err != nil {
		return nil, fmt.Errorf("failed to create or update chat: %w", err)
	}
	if created {
		go h.sendNewChatReport(chat, b)
	}

	return chat, nil
}

// sendNewChatReport middlware sends a report to the admin chat when a new user chat is registered.
// It also sends a message to the admin chat if the user chat has a group configured.
func (h *handler) sendNewChatReport(chat *model.Chat, b *bot.Bot) {
	report, sentErr := h.Report().Chat(chat).Msg("New chat registered")
	if sentErr != nil {
		log.Warn().Err(sentErr).Msg("Failed to send new chat report")
		return
	}

	msg := report.Message
	var err error
	for range 5 {
		time.Sleep(20 * time.Second)

		if chat, err = h.Chat.GetChatByChatID(context.Background(), chat.TgChatID); err == nil && chat.GroupName != nil {
			b.DeleteMessage(context.Background(), &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
			h.Report().Chat(chat).Msgf("Chat configured group %s", *chat.GroupName)
			break
		}
	}
}

// ignoreOldMessage middleware ignores messages sent more that 10 minutes ago.
func (h *handler) ignoreOldMessage(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil || time.Unix(int64(update.Message.Date), 0).After(time.Now().Add(-10*time.Minute)) {
			next(ctx, b, update)
			return
		}
		log.Trace().Msg("Old message ignored")
	}
}

// ignoreInaccessibleMessageCQ middleware filters out callback queries with inaccessible messages.
func (h *handler) ignoreInaccessibleMessageCQ(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery == nil || update.CallbackQuery.Message.Message != nil {
			next(ctx, b, update)
			return
		}
	}
}

// callbackSF is a single flight group for handling callback queries
// and preventing them from being handled multiple times simultaneously for one message.
var callbackSF = singleflight.Group{}

// syncCQSingleFlight middleware ensures that a callback query is handled only once for one message.
func (h *handler) syncCQSingleFlight(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.CallbackQuery == nil {
			next(ctx, b, update)
			return
		}

		key := fmt.Sprint(update.CallbackQuery.Message.Message.ID)
		_, err, shared := callbackSF.Do(key, func() (any, error) {
			next(ctx, b, update)
			return nil, nil
		})
		if err != nil {
			log.Error().Err(err).Str("message_id", key).Msg("Failed to handle a callback query in single flight")
		}
		if shared {
			log.Trace().Str("message_id", key).Msg("Prevented a callback query from being handled multiple times")
		}
	}
}

// logUpdate middleware logs incoming updates.
func (h *handler) logUpdate(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, bot *bot.Bot, update *models.Update) {
		log.Trace().Any("update", update).Msg("Received update")

		// Prepare

		var handlerErrs []error
		ctx = context.WithValue(ctx, keyError, &handlerErrs)

		var noLogFlag bool
		ctx = context.WithValue(ctx, keyNoLogFlag, &noLogFlag)

		// Call the handler

		startTime := time.Now()
		next(ctx, bot, update)
		elapsedTime := time.Since(startTime)

		// Log the update

		log.Trace().Any("update", update).Msg("Update processed")

		if noLogFlag {
			return
		}

		chat, ok := ctx.Value(keyChat).(*model.Chat)
		if !ok {
			h.Report().Msg("Failed to get chat from context")
			return
		}

		updateKind := "unknown"
		messageID := 0
		updateData := ""

		logEvent := log.Info().Dur("elapsed_time", elapsedTime)
		if update.Message != nil {
			message := update.Message
			updateKind = "message"
			updateData = message.Text
			logEvent.
				Int64("chat_id", message.Chat.ID).
				Str("username", message.From.Username).
				Str("first_name", message.From.FirstName).
				Str("last_name", message.From.LastName).
				Str("text", botutil.ShortenText(message.Text, 100)).
				Msg("Message handled")
		} else if update.CallbackQuery != nil {
			updateKind = "callback_query"
			messageID = update.CallbackQuery.Message.Message.ID
			updateData = update.CallbackQuery.Data
			logEvent.
				Int("message_id", update.CallbackQuery.Message.Message.ID).
				Int64("chat_id", update.CallbackQuery.Message.Message.Chat.ID).
				Str("username", update.CallbackQuery.From.Username).
				Str("first_name", update.CallbackQuery.From.FirstName).
				Str("last_name", update.CallbackQuery.From.LastName).
				Str("data", update.CallbackQuery.Data).
				Msg("Callback query handled")
		} else {
			logEvent.Msg("Unknown update type")
		}

		var handlerErr error
		handlerErrStr := ""
		if len(handlerErrs) > 0 {
			log.Trace().Errs("errs", handlerErrs).Send()
			handlerErr = errors.Join(handlerErrs...)
			handlerErrStr = handlerErr.Error()
			h.Report().Err(handlerErr).Chat(chat).
				Debug("update_type", updateKind).
				Debug("update_data", updateData).
				Msg("Handler error")
		}

		h.Log.LogUpdate(ctx, model.UpdateLog{
			ChatID:       chat.ID,
			Kind:         updateKind,
			MessageID:    messageID,
			Data:         updateData,
			HandlingTime: int(elapsedTime.Milliseconds()),
			Error:        &handlerErrStr,
		})
	}
}

// checkRegularAccess middleware checks if the user has access to use regular commands.
func (h *handler) checkRegularAccess(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		chat, ok := ctx.Value(keyChat).(*model.Chat)
		if !ok {
			h.Report().Msg("Failed to get chat from context")
			next(ctx, b, update)
			return
		}

		isAdmin, err := isAdmin(ctx, b, update)
		if err != nil {
			h.Report().Err(err).Chat(chat).Msg("Failed to get chat member")
			next(ctx, b, update)
			return
		}

		// User can use a regular command only if chat access is not ChatAccessAdminOnly, or the user is admin
		logEvent := log.Trace().Bool("admin", isAdmin).Int("access", int(chat.Access))
		if chat.Access != model.ChatAccessAdminOnly || isAdmin {
			next(ctx, b, update)
			return
		}
		logEvent.Msg("User is not allowed to use regular commands")
	}
}

// checkConfigAccess middleware checks if the user has access to use config commands.
func (h *handler) checkConfigAccess(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		chat, ok := ctx.Value(keyChat).(*model.Chat)
		if !ok {
			h.Report().Msg("Failed to get chat from context")
			return
		}

		isAdmin, err := isAdmin(ctx, b, update)
		if err != nil {
			h.Report().Err(err).Chat(chat).Msg("Failed to get chat member")
			next(ctx, b, update)
			return
		}

		// User can use a config command only if chat access is ChatAccessAll, or the user is admin.
		logEvent := log.Trace().Bool("admin", isAdmin).Int("access", int(chat.Access))
		if chat.Access == model.ChatAccessAll || isAdmin {
			next(ctx, b, update)
			return
		} else {
			logEvent.Msg("User is not allowed to use config commands")
			if update.CallbackQuery != nil {
				b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
					CallbackQueryID: update.CallbackQuery.ID,
					Text:            "Доступ запрещен",
				})
			}
		}
	}
}

// isAdmin checks if the user is an admin in the chat.
func isAdmin(ctx context.Context, b *bot.Bot, update *models.Update) (bool, error) {
	var chatID, userID int64
	if update.Message != nil {
		chatID = update.Message.Chat.ID
		userID = update.Message.From.ID
	} else if update.CallbackQuery != nil {
		chatID = update.CallbackQuery.Message.Message.Chat.ID
		userID = update.CallbackQuery.From.ID
	}

	if chatID > 0 {
		// Chat is private, user is admin by default
		return true, nil
	}

	chatMember, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get chat member; retrying...")
		// Retry once
		chatMember, err = b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
		if err != nil {
			return false, fmt.Errorf("failed to get chat member with retry: %w", err)
		}
	}

	return chatMember.Type == models.ChatMemberTypeAdministrator || chatMember.Type == models.ChatMemberTypeOwner, nil
}
