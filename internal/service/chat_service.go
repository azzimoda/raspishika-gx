package service

import (
	"context"
	"fmt"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/rs/zerolog/log"
)

func NewChatService(repo repository.ChatRepository) *ChatService { return &ChatService{repo: repo} }

type ChatService struct {
	repo repository.ChatRepository
}

func (s *ChatService) CreateChat(ctx context.Context, chat *model.Chat) error {
	return s.repo.CreateChat(ctx, chat)
}
func (s *ChatService) CreateOrUpdateChat(ctx context.Context, chat *model.Chat) (created bool, err error) {
	return s.repo.CreateOrUpdateChat(ctx, chat)
}

func (s *ChatService) UpdateChat(ctx context.Context, chat *model.Chat) error {
	return s.repo.UpdateChat(ctx, chat)
}

func (s *ChatService) GetChat(ctx context.Context, id int64) (*model.Chat, error) {
	return s.repo.GetChat(ctx, id)
}
func (s *ChatService) GetChatByChatID(ctx context.Context, chatID model.ChatID) (*model.Chat, error) {
	return s.repo.GetChatByChatID(ctx, chatID)
}
func (s *ChatService) GetChatByUsernameOrChatID(ctx context.Context, usernameOrChatID string) (*model.Chat, error) {
	return s.repo.GetChatByUsernameOrChatID(ctx, usernameOrChatID)
}

func (s *ChatService) GetAllChats(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetAllChats(ctx)
}
func (s *ChatService) GetPrivateChats(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetPrivateChats(ctx)
}
func (s *ChatService) GetNewChats(ctx context.Context, duration time.Duration) ([]*model.Chat, error) {
	return s.repo.GetNewChats(ctx, duration)
}
func (s *ChatService) GetChatsByGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	return s.repo.GetChatsByGroup(ctx, group)
}
func (s *ChatService) GetChatsByDepartment(ctx context.Context, dept string) ([]*model.Chat, error) {
	return s.repo.GetChatsByDepartment(ctx, dept)
}
func (s *ChatService) GetChatsByWatchedGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	return s.repo.GetChatsByWatchedGroup(ctx, group)
}
func (s *ChatService) GetActiveChats(ctx context.Context, dur time.Duration) ([]*model.Chat, error) {
	return s.repo.GetActiveChats(ctx, dur)
}
func (s *ChatService) GetGroupChats(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetGroupChats(ctx)
}
func (s *ChatService) CountActiveChats(ctx context.Context, dur time.Duration) (int, error) {
	return s.repo.CountActiveChats(ctx, dur)
}
func (s *ChatService) GetChatsByDailyTime(ctx context.Context, time string) ([]*model.Chat, error) {
	return s.repo.GetChatsWithDailyTime(ctx, time)
}
func (s *ChatService) GetChatsWithPairNotification(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetChatsWithPairNotification(ctx)
}
func (s *ChatService) GetChatsWithChangeAlert(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetChatsWithChangeAlert(ctx)
}
func (s *ChatService) GetChatsWithDarkMode(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetChatsWithDarkMode(ctx)
}

func (s *ChatService) GetRecentTeachers(ctx context.Context, chatID int64) ([]*model.RecentTeacher, error) {
	return s.repo.GetRecentTeachers(ctx, chatID)
}

func (s *ChatService) CountAllChats(ctx context.Context) (int, error) {
	return s.repo.CountAllChats(ctx)
}

func (s *ChatService) DeleteChat(ctx context.Context, id int64) error {
	log.Debug().Int64("id", id).Msg("Deleting chat...")
	return s.repo.DeleteChat(ctx, id)
}

func (s *ChatService) AddChatRecentTeacher(ctx context.Context, recentTeacher *model.RecentTeacher) error {
	return s.repo.AddRecentTeacher(ctx, recentTeacher)
}

func (s *ChatService) HealthCheck() error {
	if _, err := s.GetAllChats(context.Background()); err != nil {
		return fmt.Errorf("failed to get all chats: %w", err)
	}
	return nil
}
