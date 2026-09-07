package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

// BroadcastJobRepository stores manual mass-broadcast jobs. Rows are collected
// per platform: an admin enqueues one row per target platform and each
// schedule-bot process claims only its own platform's rows.
type BroadcastJobRepository interface {
	// Enqueue inserts a new pending job.
	Enqueue(context.Context, *model.BroadcastJob) error
	// ClaimNext atomically claims the oldest pending job for the given platform
	// and marks it sending. It returns nil when there is nothing to claim.
	// Concurrent claimants are fenced with SELECT ... FOR UPDATE SKIP LOCKED on
	// Postgres and a guarded update on SQLite.
	ClaimNext(context.Context, model.Platform) (*model.BroadcastJob, error)
	// Finish records the terminal state (done/failed) and finished_at.
	Finish(context.Context, *model.BroadcastJob) error
	// Recent returns the newest jobs across all platforms for the admin audit.
	Recent(ctx context.Context, limit int) ([]*model.BroadcastJob, error)
}

func NewBroadcastJobRepository(db *gorm.DB) BroadcastJobRepository {
	return &broadcastJobRepository{db: db}
}

type broadcastJobRepository struct {
	db *gorm.DB
}

func (r *broadcastJobRepository) Enqueue(ctx context.Context, job *model.BroadcastJob) error {
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.Status == "" {
		job.Status = model.JobPending
	}
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *broadcastJobRepository) ClaimNext(ctx context.Context, platform model.Platform) (*model.BroadcastJob, error) {
	switch r.db.Dialector.Name() {
	case "postgres":
		job := &model.BroadcastJob{}
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where("platform = ? AND status = ?", platform, model.JobPending).
				Order("created_at ASC, id ASC").
				First(job).Error; err != nil {
				return err
			}
			claim := fmt.Sprintf("%s:%d", hostname(), os.Getpid())
			return tx.Model(&model.BroadcastJob{}).
				Where("id = ? AND status = ?", job.ID, model.JobPending).
				Updates(map[string]any{"status": model.JobSending, "claimed_by": claim}).Error
		})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to claim broadcast job: %w", err)
		}
		return job, nil
	default:
		// SQLite serializes writers, so a guarded single UPDATE is atomic enough
		// across the platforms' workers in one process.
		claim := fmt.Sprintf("%s:%d", hostname(), os.Getpid())
		var job model.BroadcastJob
		err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("platform = ? AND status = ?", platform, model.JobPending).
				Order("created_at ASC, id ASC").
				First(&job).Error; err != nil {
				return err
			}
			res := tx.Model(&model.BroadcastJob{}).
				Where("id = ? AND status = ?", job.ID, model.JobPending).
				Updates(map[string]any{"status": model.JobSending, "claimed_by": claim})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			return nil
		})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to claim broadcast job: %w", err)
		}
		return &job, nil
	}
}

func (r *broadcastJobRepository) Finish(ctx context.Context, job *model.BroadcastJob) error {
	updates := map[string]any{
		"status":      job.Status,
		"error":       job.Error,
		"finished_at": time.Now(),
	}
	return r.db.WithContext(ctx).
		Model(&model.BroadcastJob{}).
		Where("id = ?", job.ID).
		Updates(updates).Error
}

func (r *broadcastJobRepository) Recent(ctx context.Context, limit int) ([]*model.BroadcastJob, error) {
	var jobs []*model.BroadcastJob
	err := r.db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&jobs).Error
	return jobs, err
}

func hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return "unknown-host"
	}
	return host
}
