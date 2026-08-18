package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

type ScheduleRepository interface {
	Create(context.Context, *model.Schedule) error
	CreateOrUpdate(context.Context, *model.Schedule) error
	GetByKey(context.Context, string) (*model.Schedule, error)
	Update(context.Context, *model.Schedule) error
}

func NewScheduleRepository(db *gorm.DB) ScheduleRepository { return &scheduleRepository{db: db} }

type scheduleRepository struct{ db *gorm.DB }

func (r *scheduleRepository) Create(ctx context.Context, schedule *model.Schedule) error {
	return r.db.WithContext(ctx).Create(schedule).Error
}
func (r *scheduleRepository) CreateOrUpdate(ctx context.Context, schedule *model.Schedule) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "cache_key"}},
			DoUpdates: clause.Assignments(map[string]any{"data": schedule.Data, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}),
		}).
		Create(schedule).Error
}
func (r *scheduleRepository) GetByKey(ctx context.Context, key string) (*model.Schedule, error) {
	var schedule model.Schedule
	err := r.db.WithContext(ctx).Where("cache_key = ?", key).First(&schedule).Error
	return &schedule, err
}
func (r *scheduleRepository) Update(ctx context.Context, schedule *model.Schedule) error {
	return r.db.WithContext(ctx).
		Model(&model.Schedule{}).
		Where("cache_key = ?", schedule.CacheKey).
		Updates(map[string]any{"data": schedule.Data, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).
		Error
}
