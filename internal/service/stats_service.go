package service

import (
	"context"
	"fmt"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
)

func NewStatsService(logRepo repository.LogRepository, chatRepo repository.ChatRepository) *StatsService {
	return &StatsService{
		logRepo:  logRepo,
		chatRepo: chatRepo,
	}
}

type StatsService struct {
	logRepo  repository.LogRepository
	chatRepo repository.ChatRepository
}

func (s *StatsService) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	return s.logRepo.LogUpdate(ctx, log)
}
func (s *StatsService) LogBroadcastTask(ctx context.Context, log *model.BroadcastTaskLog) error {
	return s.logRepo.LogBroadcastTask(ctx, log)
}
func (s *StatsService) UpdateBroadcastTaskLog(ctx context.Context, log *model.BroadcastTaskLog) error {
	return s.logRepo.UpdateBroadcastTaskLog(ctx, log)
}
func (s *StatsService) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	return s.logRepo.LogBroadcast(ctx, log)
}

func (s *StatsService) GetChatStats(ctx context.Context, duration time.Duration) (*ChatStatsData, error) {
	chatsTotal, err := s.chatRepo.CountAllChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count all chats: %w", err)
	}
	chatsPrivate, err := s.chatRepo.CountPricateChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count private chats: %w", err)
	}
	chatsActive, err := s.chatRepo.CountActiveChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count active chats: %w", err)
	}
	chatsSemiactive, err := s.chatRepo.CountSemiactiveChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count semiactive chats: %w", err)
	}
	chatsInactive, err := s.chatRepo.CountInactiveChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count inactive chats: %w", err)
	}
	chatsNew, err := s.chatRepo.CountNewChats(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to count new chats: %w", err)
	}
	chatsNewGrouped, err := s.chatRepo.GetNewChatCountByYear(ctx, duration)
	if err != nil {
		return nil, fmt.Errorf("failed to get new chat count by group: %w", err)
	}
	chatsPerGroup, err := s.chatRepo.GetAvgChatPerGroup(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get avg chat per group: %w", err)
	}
	groupsTotal, err := s.chatRepo.CountAllConfiguredGroups(ctx)
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

func (s *StatsService) GetConfigStats(ctx context.Context) (*ConfigStatsData, error) {
	chatsTotal, err := s.chatRepo.CountAllChats(ctx)
	if err != nil {
		return nil, err
	}
	chatsWithConfiguredGroup, err := s.chatRepo.CountChatsWithConfiguredGroup(ctx)
	if err != nil {
		return nil, err
	}
	uniqueConfiguredGroups, err := s.chatRepo.CountUniqueConfiguredGroups(ctx)
	if err != nil {
		return nil, err
	}
	dailyEnabled, err := s.chatRepo.CountDailyEnabled(ctx)
	if err != nil {
		return nil, err
	}
	pairEnabled, err := s.chatRepo.CountPairEnabled(ctx)
	if err != nil {
		return nil, err
	}
	changeEnabled, err := s.chatRepo.CountChangeEnabled(ctx)
	if err != nil {
		return nil, err
	}
	darkEnabled, err := s.chatRepo.CountDarkEnabled(ctx)
	if err != nil {
		return nil, err
	}
	chatCountByTime, err := s.chatRepo.GetGroupedCountChatCountByTime(ctx)
	if err != nil {
		return nil, err
	}

	return &ConfigStatsData{
		ChatsTotal:             chatsTotal,
		ConfiguredGroupsTotal:  chatsWithConfiguredGroup,
		ConfiguredGroupsUnique: uniqueConfiguredGroups,
		DailyEnabled:           dailyEnabled,
		PairEnabled:            pairEnabled,
		ChangeEnabled:          changeEnabled,
		DarkEnabled:            darkEnabled,
		ChatCountByTime:        chatCountByTime,
	}, nil
}

type ConfigStatsData struct {
	ChatsTotal             int
	ConfiguredGroupsTotal  int
	ConfiguredGroupsUnique int
	DailyEnabled           int
	PairEnabled            int
	ChangeEnabled          int
	DarkEnabled            int
	ChatCountByTime        []repository.TimeCount
}

func (s *StatsService) GetLogStats(ctx context.Context, dur time.Duration) (*LogStatsData, error) {
	now := time.Now()

	updatesTotal, err := s.logRepo.CountUpdateLogsByPeriod(ctx, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	updatesSuccess, err := s.logRepo.CountSuccessfulUpdateLogsByPeriod(ctx, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}

	broadcastTasks, err := s.logRepo.CountBroadcastTaskLogsByPeriod(ctx, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	broadcastLogs, err := s.logRepo.CountBroadcastLogsByPeriod(ctx, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	broadcastSuccess, err := s.logRepo.CountSuccessfulBroadcastLogsByPeriod(ctx, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	broadcastDaily, err := s.logRepo.CountBroadcastLogsByPeriodAndKind(ctx, model.BDaily, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	broadcastPair, err := s.logRepo.CountBroadcastLogsByPeriodAndKind(ctx, model.BPair, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	broadcastChange, err := s.logRepo.CountBroadcastLogsByPeriodAndKind(ctx, model.BChange, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}

	stats := &LogStatsData{
		UpdatesTotal:   updatesTotal,
		UpdatesSuccess: updatesSuccess,

		BroadcastTasks:   broadcastTasks,
		BroadcastLogs:    broadcastLogs,
		BroadcastSuccess: broadcastSuccess,
		BroadcastDaily:   broadcastDaily,
		BroadcastPair:    broadcastPair,
		BroadcastChange:  broadcastChange,
	}
	return stats, nil
}

type LogStatsData struct {
	UpdatesTotal   int
	UpdatesSuccess int

	BroadcastTasks   int
	BroadcastLogs    int
	BroadcastSuccess int
	BroadcastDaily   int
	BroadcastPair    int
	BroadcastChange  int
}

func (s *StatsService) GetGeneralStats(ctx context.Context, duration time.Duration) (*GeneralStatsData, error) {
	chatStats, err := s.GetChatStats(ctx, duration)
	if err != nil {
		return nil, err
	}
	logStats, err := s.GetLogStats(ctx, duration)
	if err != nil {
		return nil, err
	}
	return &GeneralStatsData{ChatStatsData: chatStats, LogStatsData: logStats}, nil
}

type GeneralStatsData struct {
	*ChatStatsData
	*LogStatsData
}

func (s *StatsService) HealthCheck() error {
	if _, err := s.GetGeneralStats(context.Background(), time.Hour); err != nil {
		return err
	}
	return nil
}
