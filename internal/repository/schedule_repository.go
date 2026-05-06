package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

type ScheduleRepository interface {
	Create(context.Context, *model.Schedule) error
	CreateOrUpdate(context.Context, *model.Schedule) error
	GetByKey(context.Context, string) (*model.Schedule, error)
	Update(context.Context, *model.Schedule) error
}

func NewScheduleRepository(db *sqlx.DB) ScheduleRepository { return &scheduleRepository{db: db} }

type scheduleRepository struct{ db *sqlx.DB }

func (r *scheduleRepository) Create(ctx context.Context, schedule *model.Schedule) error {
	res, err := r.db.NamedExecContext(ctx, `
		INSERT INTO schedules (group_id, teacher_id, data)
		VALUES (:group_id, :teacher_id, :data)
	`, schedule)

	if id, err := res.LastInsertId(); err == nil {
		schedule.ID = id
	}

	return err
}
func (r *scheduleRepository) CreateOrUpdate(ctx context.Context, schedule *model.Schedule) error {
	res, err := r.db.NamedExecContext(ctx, `
		INSERT INTO schedules (cache_key, data)
		VALUES (:cache_key, :data)
		ON CONFLICT (cache_key) DO UPDATE SET data = :data, updated_at = CURRENT_TIMESTAMP
	`, schedule)

	if id, err := res.LastInsertId(); err == nil {
		schedule.ID = id
	}

	return err
}
func (r *scheduleRepository) GetByKey(ctx context.Context, key string) (*model.Schedule, error) {
	var schedule model.Schedule
	err := r.db.GetContext(ctx, &schedule, `SELECT * FROM schedules WHERE cache_key = ?`, key)
	return &schedule, err
}
func (r *scheduleRepository) Update(ctx context.Context, schedule *model.Schedule) error {
	_, err := r.db.NamedExecContext(ctx, `
		UPDATE schedules
		SET data = :data, updated_at = CURRENT_TIMESTAMP
		WHERE cache_key = :cache_key
	`, schedule)
	return err
}
