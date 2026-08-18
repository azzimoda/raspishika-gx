package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

type UpdateLogRepository interface {
	LogUpdate(context.Context, model.UpdateLog) error
	GetUpdateLogsByChatID(context.Context, int64) ([]model.UpdateLog, error)
	GetUpdateLogsByPeriod(ctx context.Context, start, end time.Time) ([]model.UpdateLog, error)
	CountUpdateLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountSuccessfulUpdateLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
}
type BroadcastLogRepository interface {
	LogBroadcastTask(context.Context, *model.BroadcastTaskLog) error
	UpdateBroadcastTaskLog(context.Context, *model.BroadcastTaskLog) error
	LogBroadcast(context.Context, model.BroadcastLog) error
	CountBroadcastTaskLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountSuccessfulBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountBroadcastLogsByPeriodAndKind(ctx context.Context, kind model.BroadcastKind, start, end time.Time) (int, error)
}
type RequestLogRepository interface {
	CountActualRequests(ctx context.Context, start, end time.Time) (int, error)
	CountPotentialRequests(ctx context.Context, start, end time.Time) (int, error)
}
type LogRepository interface {
	UpdateLogRepository
	BroadcastLogRepository
	RequestLogRepository
}

func NewLogRepository(db *gorm.DB) LogRepository { return &logRepository{db: db} }

type logRepository struct{ db *gorm.DB }

func (r *logRepository) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	return r.db.WithContext(ctx).Create(&log).Error
}
func (r *logRepository) GetUpdateLogsByChatID(ctx context.Context, chatID int64) ([]model.UpdateLog, error) {
	var logs []model.UpdateLog
	err := r.db.WithContext(ctx).Where("chat_id = ?", chatID).Find(&logs).Error
	return logs, err
}
func (r *logRepository) GetUpdateLogsByPeriod(ctx context.Context, start, end time.Time) ([]model.UpdateLog, error) {
	var logs []model.UpdateLog
	err := r.db.WithContext(ctx).
		Where("created_at >= ? AND created_at <= ?", start, end).
		Find(&logs).Error
	return logs, err

}
func (r *logRepository) CountUpdateLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.UpdateLog{}).
		Where("created_at BETWEEN ? AND ?", start, end).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count update logs: %w", err)
	}
	return int(count), nil
}
func (r *logRepository) CountSuccessfulUpdateLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.UpdateLog{}).
		Where("created_at BETWEEN ? AND ? AND (error IS NULL OR error = '')", start, end).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count successful update logs: %w", err)
	}
	return int(count), nil
}

func (r *logRepository) LogBroadcastTask(ctx context.Context, taskLog *model.BroadcastTaskLog) error {
	if taskLog == nil {
		return nil
	}

	if err := r.db.WithContext(ctx).Create(taskLog).Error; err != nil {
		return err
	}
	return nil
}
func (r *logRepository) UpdateBroadcastTaskLog(ctx context.Context, taskLog *model.BroadcastTaskLog) error {
	if taskLog == nil {
		return nil
	}

	return r.db.WithContext(ctx).
		Model(&model.BroadcastTaskLog{}).
		Where("id = ?", taskLog.ID).
		Update("elapsed", taskLog.Elapsed).Error
}
func (r *logRepository) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	return r.db.WithContext(ctx).Create(&log).Error
}
func (r *logRepository) CountBroadcastTaskLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.BroadcastTaskLog{}).
		Where("created_at BETWEEN ? AND ?", start, end).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count broadcast task logs: %w", err)
	}
	return int(count), nil
}
func (r *logRepository) CountBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.BroadcastLog{}).
		Where("created_at BETWEEN ? AND ?", start, end).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count broadcast logs: %w", err)
	}
	return int(count), nil
}
func (r *logRepository) CountSuccessfulBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.BroadcastLog{}).
		Where("created_at BETWEEN ? AND ? AND (error IS NULL OR error = '')", start, end).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count successful broadcast logs: %w", err)
	}
	return int(count), nil
}
func (r *logRepository) CountBroadcastLogsByPeriodAndKind(ctx context.Context, kind model.BroadcastKind, start, end time.Time) (int, error) {
	if kind == model.BAny {
		return r.CountBroadcastLogsByPeriod(ctx, start, end)
	}

	const query = `
		SELECT COUNT(*)
		FROM broadcast_logs BL JOIN broadcast_task_logs BTL ON BL.broadcast_task_log_id = BTL.id
		WHERE BL.created_at BETWEEN ? AND ? AND kind = ?
	`
	var count int64
	if err := r.db.WithContext(ctx).Raw(query, start, end, kind).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count broadcast logs by kind: %w", err)
	}
	return int(count), nil
}

func (r *logRepository) CountActualRequests(ctx context.Context, start, end time.Time) (int, error) {
	var countUpdates int64
	if err := r.db.WithContext(ctx).
		Model(&model.UpdateLog{}).
		Where("group_or_teacher IS NOT NULL AND group_or_teacher != '' AND cached = 0 AND created_at BETWEEN ? AND ?", start, end).
		Count(&countUpdates).Error; err != nil {
		return 0, fmt.Errorf("failed to count actual update requests: %w", err)
	}

	const queryBroadcasts = `SELECT COALESCE(SUM(groups), 0) FROM broadcast_task_logs WHERE created_at BETWEEN ? AND ?`
	var countBroadcasts int64
	if err := r.db.WithContext(ctx).Raw(queryBroadcasts, start, end).Scan(&countBroadcasts).Error; err != nil {
		return 0, fmt.Errorf("failed to count broadcast logs: %w", err)
	}

	return int(countUpdates + countBroadcasts), nil
}
func (r *logRepository) CountPotentialRequests(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM update_logs WHERE group_or_teacher IS NULL OR group_or_teacher = '' AND created_at BETWEEN ? AND ?`
	var count int64
	if err := r.db.WithContext(ctx).Raw(query, start, end).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count potential requests: %w", err)
	}
	return int(count), nil
}
