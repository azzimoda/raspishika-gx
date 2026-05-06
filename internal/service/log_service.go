package service

import (
	"context"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
)

func NewLogService(repo repository.LogRepository) *LogService { return &LogService{repo: repo} }

type LogService struct{ repo repository.LogRepository }

func (s *LogService) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	return s.repo.LogUpdate(ctx, log)
}
func (s *LogService) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	return s.repo.LogBroadcast(ctx, log)
}
