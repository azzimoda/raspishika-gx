package service

import (
	"context"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func (s *ChatService) GetScheduleSubscriptions(ctx context.Context, chatID int64) ([]model.ScheduleSubscription, error) {
	return s.repo.GetScheduleSubscriptions(ctx, chatID)
}

func (s *ChatService) AddScheduleSubscription(ctx context.Context, chatID int64, group model.Group) (bool, error) {
	return s.repo.AddScheduleSubscription(ctx, chatID, group)
}

func (s *ChatService) SetPrimaryGroup(ctx context.Context, chatID int64, group model.Group) (*model.Chat, error) {
	return s.repo.SetPrimaryGroup(ctx, chatID, group)
}

func (s *ChatService) RemoveScheduleSubscription(ctx context.Context, chatID, subscriptionID int64) (bool, error) {
	return s.repo.RemoveScheduleSubscription(ctx, chatID, subscriptionID)
}

func (s *ChatService) RemoveUnavailableGroup(ctx context.Context, chatID int64, group model.GroupName) (bool, error) {
	return s.repo.RemoveUnavailableGroup(ctx, chatID, group)
}

func (s *ChatService) GetDailyRecipients(ctx context.Context, dailyTime string) ([]model.DailyRecipient, error) {
	return s.repo.GetDailyRecipients(ctx, dailyTime)
}

func (s *ChatService) HasScheduleSubscription(ctx context.Context, subscription model.ScheduleSubscription) (bool, error) {
	return s.repo.HasScheduleSubscription(ctx, subscription)
}
