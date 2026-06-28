package service

import (
	"context"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

func NewLogService(repo repository.LogRepository) *LogService { return &LogService{repo: repo} }

type LogService struct{ repo repository.LogRepository }

func (s *LogService) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	return s.repo.LogUpdate(ctx, log)
}
func (s *LogService) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	return s.repo.LogBroadcast(ctx, log)
}

func (s *LogService) GetGeneralStats(ctx context.Context, dur time.Duration) (*LogStatsData, error) {
	now := time.Now()

	updates, err := s.repo.GetUpdateLogsByPeriod(ctx, now.Add(-dur), now)
	if err != nil {
		return nil, err
	}
	updatesTotal := len(updates)
	updatesSuccess := 0
	for _, u := range updates {
		if refutil.DerefOrTypeDefault(u.Error) == "" {
			updatesSuccess += 1
		}
	}

	broadcasts, err := s.repo.GetBroadcastLogsByKind(ctx, model.BLogAny, dur)
	if err != nil {
		return nil, err
	}
	broadcastChats := 0
	broadcastDaily := 0
	broadcastPair := 0
	broadcastChange := 0
	broadcastFails := 0
	for _, b := range broadcasts {
		broadcastChats += b.Chats
		broadcastFails += b.Fails

		if b.Kind == model.BLogDaily {
			broadcastDaily += b.Chats
		}
		if b.Kind == model.BLogPair {
			broadcastPair += b.Chats
		}
		if b.Kind == model.BLogChange {
			broadcastChange += b.Chats
		}
	}

	stats := &LogStatsData{
		UpdatesTotal:   updatesTotal,
		UpdatesSuccess: updatesSuccess,

		BroadcastChats:  broadcastChats,
		BroadcastFails:  broadcastFails,
		BroadcastDaily:  broadcastDaily,
		BroadcastPair:   broadcastPair,
		BroadcastChange: broadcastChange,
	}
	return stats, nil
}

type LogStatsData struct {
	UpdatesTotal   int `json:"updates_total"`
	UpdatesSuccess int `json:"updates_success"`

	BroadcastChats  int `json:"broadcast_chats"`
	BroadcastFails  int `json:"broadcast_success"`
	BroadcastDaily  int `json:"broadcast_daily"`
	BroadcastPair   int `json:"broadcast_Pair"`
	BroadcastChange int `json:"broadcast_change"`
}

func (s *LogService) HealthCheck() error {
	if _, err := s.GetGeneralStats(context.Background(), time.Hour); err != nil {
		return err
	}
	return nil
}
