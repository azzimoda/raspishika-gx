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
	// CountScheduleRequestsByPeriod returns the number of schedule requests
	// (update logs with a group or teacher set) split by cache usage.
	CountScheduleRequestsByPeriod(ctx context.Context, start, end time.Time) (cached, uncached int, err error)
	// CountDistinctChatsByPeriod returns the number of distinct chats that made
	// at least one request within the period (DAU).
	CountDistinctChatsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	// GetUpdateLogCountByKind returns update log counts grouped by update kind
	// ("message", "callback_query", ...).
	GetUpdateLogCountByKind(ctx context.Context, start, end time.Time) (map[string]int, error)
	// GetUpdateLatencyStatsByPeriod returns avg/p95/max handler latency (ms).
	// Count is zero when there are no logs; percentile fields may be nil.
	GetUpdateLatencyStatsByPeriod(ctx context.Context, start, end time.Time) (*LatencyStats, error)
	// GetRequestsCountByHour returns the number of requests per hour of day.
	GetRequestsCountByHour(ctx context.Context, start, end time.Time) ([]TimeCount, error)
	// GetTopRequestedSchedules returns the most requested groups/teachers.
	GetTopRequestedSchedules(ctx context.Context, start, end time.Time, limit int) ([]NameCount, error)
}
type BroadcastLogRepository interface {
	LogBroadcastTask(context.Context, *model.BroadcastTaskLog) error
	UpdateBroadcastTaskLog(context.Context, *model.BroadcastTaskLog) error
	LogBroadcast(context.Context, model.BroadcastLog) error
	CountBroadcastTaskLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountSuccessfulBroadcastLogsByPeriod(ctx context.Context, start, end time.Time) (int, error)
	CountBroadcastLogsByPeriodAndKind(ctx context.Context, kind model.BroadcastKind, start, end time.Time) (int, error)
	// GetBroadcastTaskStatsByKind returns broadcast task stats grouped by kind
	// (tasks count, total groups sent, average elapsed ms).
	GetBroadcastTaskStatsByKind(ctx context.Context, start, end time.Time) ([]BroadcastTaskKindStats, error)
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

// NameCount is a generic name/count aggregation row.
type NameCount struct {
	Name  string
	Count int
}

// LatencyStats holds update handler latency aggregation (milliseconds).
// Count is the number of logs with elapsed set; AvgMs/P95Ms/MaxMs are zero when
// there are no logs.
type LatencyStats struct {
	Count int
	AvgMs int
	P95Ms int
	MaxMs int
}

// BroadcastTaskKindStats is a broadcast task aggregation row for one kind.
type BroadcastTaskKindStats struct {
	Kind         model.BroadcastKind
	Tasks        int
	Groups       int
	AvgElapsedMs int
}

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
		FROM broadcast_logs bl JOIN broadcast_task_logs btl ON bl.broadcast_task_log_id = btl.id
		WHERE bl.created_at BETWEEN ? AND ? AND btl.kind = ?
	`
	var count int64
	if err := r.db.WithContext(ctx).Raw(query, start, end, kind).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count broadcast logs by kind: %w", err)
	}
	return int(count), nil
}

// CountActualRequests counts requests the bot actually made: manual schedule
// requests that were not served from cache (cached = 0) plus all broadcast
// group schedule requests.
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

// CountPotentialRequests counts how many requests to the college site users
// could have made without the bot: every manual schedule request (cached or
// not), excluding broadcasts.
func (r *logRepository) CountPotentialRequests(ctx context.Context, start, end time.Time) (int, error) {
	const query = `
		SELECT COUNT(*) FROM update_logs
		WHERE group_or_teacher IS NOT NULL AND group_or_teacher != ''
			AND created_at BETWEEN ? AND ?
	`
	var count int64
	if err := r.db.WithContext(ctx).Raw(query, start, end).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count potential requests: %w", err)
	}
	return int(count), nil
}

func (r *logRepository) CountScheduleRequestsByPeriod(ctx context.Context, start, end time.Time) (cached, uncached int, err error) {
	const query = `
		SELECT cached, COUNT(*) AS count FROM update_logs
		WHERE group_or_teacher IS NOT NULL AND group_or_teacher != ''
			AND created_at BETWEEN ? AND ?
		GROUP BY cached
	`
	var rows []struct {
		Cached bool
		Count  int
	}
	if err = r.db.WithContext(ctx).Raw(query, start, end).Scan(&rows).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to count schedule requests by cache: %w", err)
	}
	for _, row := range rows {
		if row.Cached {
			cached = row.Count
		} else {
			uncached = row.Count
		}
	}
	return cached, uncached, nil
}

func (r *logRepository) CountDistinctChatsByPeriod(ctx context.Context, start, end time.Time) (int, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.UpdateLog{}).
		Where("created_at BETWEEN ? AND ?", start, end).
		Distinct("chat_id").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count distinct chats: %w", err)
	}
	return int(count), nil
}

func (r *logRepository) GetUpdateLogCountByKind(ctx context.Context, start, end time.Time) (map[string]int, error) {
	const query = `
		SELECT kind, COUNT(*) AS count FROM update_logs
		WHERE created_at BETWEEN ? AND ?
		GROUP BY kind
	`
	var rows []struct {
		Kind  string
		Count int
	}
	if err := r.db.WithContext(ctx).Raw(query, start, end).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to count update logs by kind: %w", err)
	}
	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.Kind] = row.Count
	}
	return result, nil
}

func (r *logRepository) GetUpdateLatencyStatsByPeriod(ctx context.Context, start, end time.Time) (*LatencyStats, error) {
	const baseQuery = `
		SELECT COUNT(*) AS count,
			CAST(AVG(elapsed) AS INTEGER) AS avg_ms,
			MAX(elapsed) AS max_ms
		FROM update_logs
		WHERE elapsed IS NOT NULL AND created_at BETWEEN ? AND ?
	`
	stats := &LatencyStats{}
	if err := r.db.WithContext(ctx).Raw(baseQuery, start, end).Scan(stats).Error; err != nil {
		return nil, fmt.Errorf("failed to get update latency stats: %w", err)
	}

	if stats.Count <= 0 {
		return stats, nil
	}

	if err := r.db.WithContext(ctx).Raw(`
		SELECT elapsed FROM update_logs
		WHERE elapsed IS NOT NULL AND created_at BETWEEN ? AND ?
		ORDER BY elapsed LIMIT 1 OFFSET ?
	`, start, end, int(float64(stats.Count)*0.95)).Scan(&stats.P95Ms).Error; err != nil {
		return nil, fmt.Errorf("failed to get p95 update latency: %w", err)
	}
	return stats, nil
}

func (r *logRepository) GetRequestsCountByHour(ctx context.Context, start, end time.Time) ([]TimeCount, error) {
	const query = `
		SELECT strftime('%H', created_at) AS time, COUNT(*) AS count FROM update_logs
		WHERE created_at BETWEEN ? AND ?
		GROUP BY strftime('%H', created_at)
		ORDER BY time
	`
	var result []TimeCount
	if err := r.db.WithContext(ctx).Raw(query, start, end).Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("failed to count requests by hour: %w", err)
	}
	return result, nil
}

func (r *logRepository) GetTopRequestedSchedules(ctx context.Context, start, end time.Time, limit int) ([]NameCount, error) {
	const query = `
		SELECT group_or_teacher AS name, COUNT(*) AS count FROM update_logs
		WHERE group_or_teacher IS NOT NULL AND group_or_teacher != ''
			AND created_at BETWEEN ? AND ?
		GROUP BY group_or_teacher
		ORDER BY count DESC, name ASC
		LIMIT ?
	`
	var rows []NameCount
	if err := r.db.WithContext(ctx).Raw(query, start, end, limit).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get top requested schedules: %w", err)
	}
	return rows, nil
}

func (r *logRepository) GetBroadcastTaskStatsByKind(ctx context.Context, start, end time.Time) ([]BroadcastTaskKindStats, error) {
	const query = `
		SELECT kind, COUNT(*) AS tasks, COALESCE(SUM(groups), 0) AS groups,
			CAST(AVG(elapsed) AS INTEGER) AS avg_elapsed_ms
		FROM broadcast_task_logs
		WHERE created_at BETWEEN ? AND ?
		GROUP BY kind
		ORDER BY kind
	`
	var rows []BroadcastTaskKindStats
	if err := r.db.WithContext(ctx).Raw(query, start, end).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get broadcast task stats by kind: %w", err)
	}
	return rows, nil
}
