package service

import (
	"context"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
)

func NewChatService(repo repository.ChatRepository) *ChatService { return &ChatService{repo: repo} }

type ChatService struct {
	repo repository.ChatRepository
}

func (s *ChatService) Create(ctx context.Context, chat *model.Chat) error {
	return s.repo.Create(ctx, chat)
}
func (s *ChatService) CreateOrUpdate(ctx context.Context, chat *model.Chat) (created bool, err error) {
	return s.repo.CreateOrUpdate(ctx, chat)
}

func (s *ChatService) Update(ctx context.Context, chat *model.Chat) error {
	return s.repo.Update(ctx, chat)
}

func (s *ChatService) Get(ctx context.Context, id int64) (*model.Chat, error) {
	return s.repo.Get(ctx, id)
}
func (s *ChatService) GetByChatID(ctx context.Context, chatID model.ChatID) (*model.Chat, error) {
	return s.repo.GetByChatID(ctx, chatID)
}

func (s *ChatService) All(ctx context.Context) ([]*model.Chat, error) { return s.repo.GetAll(ctx) }
func (s *ChatService) AllPrivate(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetAllPrivate(ctx)
}
func (s *ChatService) AllNew(ctx context.Context, duration time.Duration) ([]*model.Chat, error) {
	return s.repo.GetAllNew(ctx, duration)
}
func (s *ChatService) AllByGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	return s.repo.GetAllByGroup(ctx, group)
}
func (s *ChatService) AllByWatchedGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	return s.repo.GetAllByWatchedGroup(ctx, group)
}
func (s *ChatService) AllByDailyTime(ctx context.Context, time string) ([]*model.Chat, error) {
	return s.repo.GetAllByDailyTime(ctx, time)
}
func (s *ChatService) AllWithPairNotification(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetAllWithPairNotification(ctx)
}
func (s *ChatService) AllWithChangeAlert(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetAllWithChangeAlert(ctx)
}
func (s *ChatService) AllWithDarkMode(ctx context.Context) ([]*model.Chat, error) {
	return s.repo.GetAllWithDarkMode(ctx)
}

func (s *ChatService) Delete(ctx context.Context, id int64) error { return s.repo.Delete(ctx, id) }

func (s *ChatService) AddChatRecentTeacher(ctx context.Context, chatID int64, teacherID int64) error {
	return s.repo.AddRecentTeacher(ctx, chatID, teacherID)
}
