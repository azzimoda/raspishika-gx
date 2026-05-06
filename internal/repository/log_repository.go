package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

type LogRepository interface {
	LogUpdate(context.Context, model.UpdateLog) error
	GetUpdateLogsByChatID(context.Context, int64) ([]model.UpdateLog, error)
	GetUpdateLogsByPeriod(ctx context.Context, start, end time.Time) ([]model.UpdateLog, error)

	LogBroadcast(context.Context, model.BroadcastLog) error
	GetBroadcastLogsByKind(context.Context, model.BroadcastLogKind, time.Duration) ([]model.BroadcastLog, error)
}

func NewLogRepository(db *sqlx.DB) LogRepository { return &logRepository{db: db} }

type logRepository struct{ db *sqlx.DB }

func (r *logRepository) LogUpdate(ctx context.Context, log model.UpdateLog) error {
	_, err := r.db.NamedExecContext(ctx, `
			INSERT INTO update_logs (chat_id, kind, message_id, data, handling_time, error)
			VALUES (:chat_id, :kind, :message_id, :data, :handling_time, :error)
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

func (r *logRepository) LogBroadcast(ctx context.Context, log model.BroadcastLog) error {
	_, err := r.db.NamedExecContext(ctx, `
			INSERT INTO sending_logs (kind, chats, groups, elapsed, fails, errors)
			VALUES (:kind, :chats, :groups, :elapsed, :fails, :errors)
		`, log)
	return err
}
func (r *logRepository) GetBroadcastLogsByKind(
	ctx context.Context,
	kind model.BroadcastLogKind,
	dur time.Duration,
) ([]model.BroadcastLog, error) {
	query := `SELECT * FROM sending_logs WHERE created_at >= ?`
	switch kind {
	case model.BLogDaily, model.BLogPair, model.BLogChange:
		query += fmt.Sprintf(` AND kind = '%s'`, kind)
	}

	var logs []model.BroadcastLog
	if err := r.db.SelectContext(ctx, &logs, query, time.Now().Add(-dur)); err != nil {
		return nil, fmt.Errorf("failed to get sending logs for duration: %w", err)
	}
	return logs, nil
}
