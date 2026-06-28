package service

import (
	"context"
	"fmt"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
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
func (s *ChatService) GetChatsByWatchedGroup(ctx context.Context, group model.GroupName) ([]*model.Chat, error) {
	return s.repo.GetChatsByWatchedGroup(ctx, group)
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

func (s *ChatService) DeleteChat(ctx context.Context, id int64) error {
	return s.repo.DeleteChat(ctx, id)
}

func (s *ChatService) AddChatRecentTeacher(ctx context.Context, chatID int64, teacherID int64) error {
	return s.repo.AddRecentTeacher(ctx, chatID, teacherID)
}

func (s *ChatService) GetGeneralStats(ctx context.Context, duration time.Duration) (*ChatStatsData, error) {
	chatsTotal, err := s.repo.CountAllChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count all chats: %w", err)
	}
	chatsPrivate, err := s.repo.CountPricateChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count private chats: %w", err)
	}
	chatsActive, err := s.repo.CountActiveChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count active chats: %w", err)
	}
	chatsSemiactive, err := s.repo.CountSemiactiveChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count semiactive chats: %w", err)
	}
	chatsInactive, err := s.repo.CountInactiveChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count inactive chats: %w", err)
	}
	chatsNew, err := s.repo.CountNewChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count new chats: %w", err)
	}
	chatsNewGrouped, err := s.repo.GetNewChatCountByYear(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to get new chat count by group: %w", err)
	}
	chatsPerGroup, err := s.repo.GetAvgChatPerGroup(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get avg chat per group: %w", err)
	}
	groupsTotal, err := s.repo.CountAllConfiguredGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count all configured groups: %w", err)
	}

	stats := &ChatStatsData{
		ChatsTotal:      chatsTotal,
		ChatsPrivate:    chatsPrivate,
		ChatsActive:     chatsActive,
		ChatsSemiactive: chatsSemiactive,
		ChatsInactive:   chatsInactive,
		ChatsNew:        chatsNew,
		ChatsNewGrouped: chatsNewGrouped,
		ChatsPerGroup:   chatsPerGroup,
		GroupsTotal:     groupsTotal,
	}
	return stats, nil
}

func (s *ChatService) HealthCheck() error {
	if _, err := s.GetAllChats(context.Background()); err != nil {
		return fmt.Errorf("failed to get all chats: %w", err)
	}
	return nil
}

type ChatStatsData struct {
	ChatsTotal      int         `json:"chats_total"`
	ChatsPrivate    int         `json:"chats_private"`
	ChatsActive     int         `json:"chats_active"`
	ChatsSemiactive int         `json:"chats_semiactive"`
	ChatsInactive   int         `json:"chats_inactive"`
	ChatsNew        int         `json:"chats_new"`
	ChatsNewGrouped map[int]int `json:"chats_new_grouped"`
	ChatsPerGroup   float64     `json:"chat_per_group"`
	GroupsTotal     int         `json:"groups_total"`
}
