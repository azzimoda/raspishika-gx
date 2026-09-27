package vkbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	vkclient "github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	"github.com/rs/zerolog/log"
)

// vacationText and configVacationText mirror the Telegram bot's answers, so a
// user switching platforms reads the same thing.
const (
	vacationText        = "До конца каникул осталось %d день/дня/дней"
	configVacationText  = "Не могу настроить группу во время каникул, подождите до начала семестра"
	groupRemovedMessage = "Группа %s была удалена из расписания, настройки группы сброшены."
)

// sendVacationAnswer reports the days left until the semester starts.
func (b *Bot) sendVacationAnswer(ctx context.Context, peerID int64, isConfig bool) error {
	now := b.now()
	text := fmt.Sprintf(vacationText, daysUntilSeptember(now))
	if isConfig {
		return b.send(ctx, peerID, configVacationText+"\n"+text, mainKeyboard())
	}
	return b.send(ctx, peerID, text, mainKeyboard())
}

// daysUntilSeptember counts whole days from now to 1 September of the current
// year, the day the college's summer holidays end. It never goes negative:
// after that date the semester has already started, and "-4 дней" would be
// nonsense in a message a user reads.
func daysUntilSeptember(now time.Time) int {
	until := time.Date(now.Year(), time.September, 1, 0, 0, 0, 0, now.Location())
	return max(int(until.Sub(now).Hours())/24, 0)
}

// vacationActive reports whether the schedule site is on vacation. A failure to
// tell is not treated as vacation: the Telegram bot logs it and carries on, so
// a transient API error must not lock users out of their settings.
func (b *Bot) vacationActive(ctx context.Context) bool {
	if b.schedules == nil {
		return false
	}
	vacation, err := b.schedules.IsVacation(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check vacation status; processing anyway")
		return false
	}
	return vacation
}

// resetChatForExpiredGroup clears the stored group of a chat whose group has
// left the schedule and tells the user, matching the Telegram bot. Without it
// the chat keeps pointing at a group that no longer exists, so every later
// request repeats the same "not found" answer and the broadcasts keep running
// against a dead group.
//
// It only runs for a group the chat had stored. A name the user just typed is
// not reset: they may simply have mistyped it, and clearing their settings
// would be rude.
func (b *Bot) resetChatForExpiredGroup(ctx context.Context, chat *model.Chat, msg vkclient.Message, name string) error {
	if err := b.chats.ResetGroupSettings(ctx, chat); err != nil {
		// The user still has to learn the group is gone, so report the failure
		// and say it anyway.
		log.Error().Err(err).Msg("Failed to reset group settings for an expired group")
	}
	return b.send(ctx, msg.PeerID, fmt.Sprintf(groupRemovedMessage, name), mainKeyboard())
}

// storedGroupIsGone reports whether the lookup used the chat's stored group,
// which is the only case that justifies resetting the chat.
func storedGroupIsGone(chat *model.Chat, requested string) bool {
	return chat.GroupName != nil &&
		strings.EqualFold(strings.TrimSpace(requested), string(*chat.GroupName))
}
