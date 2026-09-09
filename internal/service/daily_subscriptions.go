package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"sort"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/model"
)

func (s *BroadcastService) currentDailyRecipient(ctx context.Context, queued model.DailyRecipient) (*model.Chat, error) {
	chat, err := s.currentRecipient(ctx, &queued.Chat, model.BMass)
	if err != nil || chat == nil {
		return nil, err
	}
	if chat.DailySendingTime == nil || queued.Chat.DailySendingTime == nil || *chat.DailySendingTime != *queued.Chat.DailySendingTime {
		return nil, nil
	}
	if queued.Subscription.ChatID != chat.ID {
		return nil, nil
	}
	exists, err := s.Chat.HasScheduleSubscription(ctx, queued.Subscription)
	if err != nil || !exists {
		return nil, err
	}
	return chat, nil
}

func (s *BroadcastService) prepareDailyBroadcast(ctx context.Context, recipients []model.DailyRecipient) (map[model.GroupName][]model.DailyRecipient, []model.ScheduleConfig) {
	grouped := make(map[model.GroupName][]model.DailyRecipient)
	for _, recipient := range recipients {
		name := recipient.Subscription.GroupName
		grouped[name] = append(grouped[name], recipient)
	}
	names := make([]model.GroupName, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	confs := make([]model.ScheduleConfig, 0, len(names))
	for _, name := range names {
		if ctx.Err() != nil {
			break
		}
		group, err := s.Schedule.GetGroupByName(ctx, name)
		if err != nil {
			if errors.Is(err, apiclient.ErrNotFound) {
				for _, recipient := range grouped[name] {
					s.removeUnavailableDailyGroup(ctx, recipient)
				}
			} else {
				s.reportError(err, "Не удалось загрузить группу для ежедневной рассылки")
			}
			continue
		}
		if group != nil {
			confs = append(confs, model.GroupScheduleConfig(group, false))
		}
	}
	return grouped, confs
}

func (s *BroadcastService) removeUnavailableDailyGroup(ctx context.Context, recipient model.DailyRecipient) {
	chat, err := s.currentDailyRecipient(ctx, recipient)
	if err != nil {
		s.reportError(err, "Не удалось проверить подписку на исчезнувшую группу")
		return
	}
	if chat == nil {
		return
	}
	group := recipient.Subscription.GroupName
	removed, err := s.Chat.RemoveUnavailableGroup(ctx, chat.ID, group)
	if err != nil {
		s.reportError(err, "Не удалось удалить подписку на исчезнувшую группу")
		return
	}
	if !removed {
		return
	}
	current, err := s.currentRecipient(ctx, chat, model.BMass)
	if err != nil || current == nil {
		return
	}
	text := fmt.Sprintf("Группа %s больше не существует на сайте колледжа и удалена из рассылки. Остальные группы сохранены.", html.EscapeString(string(group)))
	if chat.GroupName != nil && *chat.GroupName == group {
		text += "\nВыберите новую основную группу через /settings."
	}
	_, err = s.Messenger.SendMessagePeer(ctx, int64(current.PeerID), text)
	if s.Messenger.IsForbidden(err) {
		s.handleForbidden(ctx, err, current)
	} else if err != nil {
		s.reportError(err, "Не удалось сообщить об исчезнувшей группе")
	}
}
