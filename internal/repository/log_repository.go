package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

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

func NewLogRepository(db *sqlx.DB) LogRepository { return &logRepository{db: db} }

type logRepository struct{ db *sqlx.DB }

func (r *logRepository) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	_, err := r.db.NamedExecContext(ctx, `
			INSERT INTO update_logs (chat_id, kind, message_id, data, elapsed, error)
			VALUES (:chat_id, :kind, :message_id, :data, :elapsed, :error)
		`, log)
	return err
}
func (r *logRepository) GetUpdateLogsByChatID(ctx context.Context, chatID int64) ([]model.UpdateLog, error) {
	var logs []model.UpdateLog
	err := r.db.SelectContext(ctx, &logs, `SELECT * FROM update_logs WHERE chat_id = ?`, chatID)
	return logs, err
}
func (r *logRepository) GetUpdateLogsByPeriod(ctx context.Context, start, end time.Time) ([]model.UpdateLog, error) {
	var logs []model.UpdateLog
	err := r.db.SelectContext(ctx, &logs, `
			SELECT * FROM update_logs WHERE created_at >= ? AND created_at <= ?
		`, start, end)
	return logs, err

}
func (r *logRepository) CountUpdateLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM update_logs WHERE created_at BETWEEN ? AND ?`
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end); err != nil {
		return 0, fmt.Errorf("failed to count update logs: %w", err)
	}
	return count, nil
}
func (r *logRepository) CountSuccessfulUpdateLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM update_logs WHERE created_at BETWEEN ? AND ? AND (error IS NULL OR error = '')`
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end); err != nil {
		return 0, fmt.Errorf("failed to count successful update logs: %w", err)
	}
	return count, nil
}

func (r *logRepository) LogBroadcastTask(ctx context.Context, taskLog *model.BroadcastTaskLog) error {
	if taskLog == nil {
		return nil
	}

	res, err := r.db.NamedExecContext(ctx, `
			INSERT INTO broadcast_task_logs (kind, elapsed) VALUES (:kind, :elapsed)
		`, *taskLog)
	if err != nil {
		return err
	}

	// Update taskLog.ID and taskLog.CreatedAt
	taskLog.ID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	taskLog.CreatedAt = time.Now()
	return nil
}
func (r *logRepository) UpdateBroadcastTaskLog(ctx context.Context, taskLog *model.BroadcastTaskLog) error {
	if taskLog == nil {
		return nil
	}

	_, err := r.db.NamedExecContext(ctx, `UPDATE broadcast_task_logs SET elapsed = :elapsed WHERE id = :id`, *taskLog)
	return err
}
func (r *logRepository) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	_, err := r.db.NamedExecContext(ctx, `
			INSERT INTO broadcast_logs (broadcast_task_log_id, chat_id, error) VALUES (:broadcast_task_log_id, :chat_id, :error)
		`, log)
	return err
}
func (r *logRepository) CountBroadcastTaskLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM broadcast_task_logs WHERE created_at BETWEEN ? AND ?`
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end); err != nil {
		return 0, fmt.Errorf("failed to count broadcast task logs: %w", err)
	}
	return count, nil
}
func (r *logRepository) CountBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM broadcast_logs WHERE created_at BETWEEN ? AND ?`
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end); err != nil {
		return 0, fmt.Errorf("failed to count broadcast logs: %w", err)
	}
	return count, nil
}
func (r *logRepository) CountSuccessfulBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM broadcast_logs WHERE created_at BETWEEN ? AND ? AND (error IS NULL OR error = '')`
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end); err != nil {
		return 0, fmt.Errorf("failed to count successful broadcast logs: %w", err)
	}
	return count, nil
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
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end, kind); err != nil {
		return 0, fmt.Errorf("failed to count broadcast logs by kind: %w", err)
	}
	return count, nil
}

func (r *logRepository) CountActualRequests(ctx context.Context, start, end time.Time) (int, error) {
	const queryUpdates = `
		SELECT COUNT(*) FROM update_logs
		WHERE group_or_teacher IS NOT NULL AND group_or_teacher != '' AND cached = 0 AND created_at BETWEEN ? AND ?`
	var countUpdates int
	if err := r.db.GetContext(ctx, &countUpdates, queryUpdates, start, end); err != nil {
		return 0, fmt.Errorf("failed to count actual update requests: %w", err)
	}

	const queryBroadcasts = `SELECT SUM(groups) FROM broadcast_task_logs WHERE created_at BETWEEN ? AND ?`
	var countBroadcasts int
	if err := r.db.GetContext(ctx, &countBroadcasts, queryBroadcasts, start, end); err != nil {
		if strings.Contains(err.Error(), "converting NULL to int is unsupported") {
			countBroadcasts = 0
		} else {
			return 0, fmt.Errorf("failed to count broadcast logs: %w", err)
		}
	}

	return countUpdates + countBroadcasts, nil
}
func (r *logRepository) CountPotentialRequests(ctx context.Context, start, end time.Time) (int, error) {
	const query = `SELECT COUNT(*) FROM update_logs WHERE "group" IS NULL OR "group" = '' AND created_at BETWEEN ? AND ?`
	var count int
	if err := r.db.GetContext(ctx, &count, query, start, end); err != nil {
		return 0, fmt.Errorf("failed to count potential requests: %w", err)
	}
	return count, nil
}
