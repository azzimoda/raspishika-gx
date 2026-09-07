package messenger

import (
	"bytes"
	"context"
	"errors"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Telegram adapts a lazily-connected Telegram bot to the Messenger interface.
// The accessor is resolved on every call so the adapter can be built before
// the bot connects (bots connect asynchronously after acquiring a proxy).
type Telegram struct {
	getBot func() *bot.Bot
}

// NewTelegram wraps a bot accessor. getBot must return the Telegram bot once
// it is connected and nil before that.
func NewTelegram(getBot func() *bot.Bot) *Telegram {
	return &Telegram{getBot: getBot}
}

func (t *Telegram) client() (*bot.Bot, error) {
	if t == nil || t.getBot == nil {
		return nil, errors.New("telegram bot accessor is not set")
	}
	if b := t.getBot(); b != nil {
		return b, nil
	}
	return nil, errors.New("telegram bot is not connected")
}

func (t *Telegram) SendMessagePeer(ctx context.Context, peerID int64, text string) error {
	b, err := t.client()
	if err != nil {
		return err
	}
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    peerID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
	return err
}

func (t *Telegram) SendPhotoPeer(ctx context.Context, peerID int64, filename string, data []byte, caption string) error {
	b, err := t.client()
	if err != nil {
		return err
	}
	_, err = b.SendPhoto(ctx, &bot.SendPhotoParams{
		ChatID:    peerID,
		Photo:     &models.InputFileUpload{Filename: filename, Data: bytes.NewReader(data)},
		Caption:   caption,
		ParseMode: models.ParseModeHTML,
	})
	return err
}

func (t *Telegram) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	b, err := t.client()
	if err != nil {
		return err
	}
	_, err = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
	return err
}

func (t *Telegram) IsForbidden(err error) bool {
	return errors.Is(err, bot.ErrorForbidden)
}

func (t *Telegram) IsAdmin(ctx context.Context, peerID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	b, err := t.client()
	if err != nil {
		return false, err
	}
	member, err := b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: peerID, UserID: userID})
	if err != nil {
		return false, err
	}
	switch member.Type {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator:
		return true, nil
	}
	return false, nil
}
