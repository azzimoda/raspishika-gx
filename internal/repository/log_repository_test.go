package repository

import (
	"context"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestLogDB opens an in-memory SQLite database with the minimal schemas
// needed by the log/chats stats queries.
func openTestLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec(`
		CREATE TABLE chats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT,
			"group" TEXT,
			daily_sending_time TEXT,
			pair_sending BOOLEAN NOT NULL DEFAULT 0,
			update_notification BOOLEAN NOT NULL DEFAULT 0
		);
		CREATE TABLE update_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			group_or_teacher TEXT,
			cached BOOLEAN NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		);
		CREATE TABLE broadcast_task_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			groups INTEGER NOT NULL DEFAULT 0,
			elapsed INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		);
		CREATE TABLE broadcast_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			broadcast_task_log_id INTEGER NOT NULL,
			chat_id INTEGER NOT NULL,
			error TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT (CURRENT_TIMESTAMP)
		);
	`).Error; err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}
	return db
}

func insertChat(t *testing.T, db *gorm.DB, id int, group string, daily *string, pair, change bool) {
	t.Helper()
	var dailyOrNull any
	if daily != nil {
		dailyOrNull = *daily
	}
	var groupOrNull any
	if group != "" {
		groupOrNull = group
	}
	if err := db.Exec(`
		INSERT INTO chats (id, "group", daily_sending_time, pair_sending, update_notification)
		VALUES (?, ?, ?, ?, ?)
	`, id, groupOrNull, dailyOrNull, pair, change).Error; err != nil {
		t.Fatalf("failed to insert chat %d: %v", id, err)
	}
}

func TestCountChatActivities(t *testing.T) {
	db := openTestLogDB(t)

	daily := "09:00"
	// 1: active — has a log within the period, no group configured.
	insertChat(t, db, 1, "", nil, false, false)
	// 2: semiactive — no logs, group + daily broadcast.
	insertChat(t, db, 2, "Б-123", &daily, false, false)
	// 3: inactive — no logs, no group, no broadcasts.
	insertChat(t, db, 3, "", nil, false, false)
	// 4: semiactive — no logs, group + pair broadcast.
	insertChat(t, db, 4, "Б-456", nil, true, false)
	// 5: inactive — no logs, group configured but no broadcasts enabled.
	insertChat(t, db, 5, "Б-789", nil, false, false)
	// 6: semiactive — only old logs, group + change broadcast.
	insertChat(t, db, 6, "Б-999", nil, false, true)
	// 7: active — logs within the period, group + pair broadcast.
	insertChat(t, db, 7, "Б-111", nil, true, false)

	insertLog := func(chatID int, groupOrTeacher any, offset string) {
		var group any
		if groupOrTeacher != nil {
			group = groupOrTeacher
		}
		if err := db.Exec(`
			INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
			VALUES (?, ?, datetime('now', ?))
		`, chatID, group, offset).Error; err != nil {
			t.Fatalf("failed to insert update log: %v", err)
		}
	}
	insertLog(1, "Б-111", "-1 minute")
	insertLog(7, "Б-111", "-5 minutes")
	insertLog(6, "Б-999", "-2 days")

	repo := &chatRepository{db: db}
	now := time.Now()
	start := now.Add(-24 * time.Hour)
	end := now.Add(time.Minute)
	got, err := repo.CountChatActivitiesByPeriod(context.Background(), start, end)
	if err != nil {
		t.Fatalf("CountChatActivitiesByPeriod() error: %v", err)
	}

	want := ChatActivityCounts{Active: 2, Semiactive: 3, Inactive: 2}
	if got != want {
		t.Fatalf("countChatActivities() = %+v, want %+v", got, want)
	}
	// The three buckets must partition all chats exactly.
	if sum := got.Active + got.Semiactive + got.Inactive; sum != 7 {
		t.Fatalf("buckets do not partition all chats: sum = %d, want 7", sum)
	}
}

func TestCountChatActivitiesByPeriod(t *testing.T) {
	db := openTestLogDB(t)

	daily := "09:00"
	insertChat(t, db, 1, "", nil, false, false)
	insertChat(t, db, 2, "Б-123", &daily, false, false)
	insertChat(t, db, 3, "", nil, false, false)

	now := time.Now()
	start := now.Add(-24 * time.Hour)
	end := now.Add(time.Minute)
	if err := db.Exec(`
		INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
		VALUES (1, 'Б-111', ?), (3, 'Б-111', ?)
	`, now.Add(-time.Hour), now.Add(-48*time.Hour)).Error; err != nil {
		t.Fatalf("failed to insert update_logs: %v", err)
	}

	repo := &chatRepository{db: db}
	got, err := repo.CountChatActivitiesByPeriod(context.Background(), start, end)
	if err != nil {
		t.Fatalf("CountChatActivitiesByPeriod() error: %v", err)
	}
	want := ChatActivityCounts{Active: 1, Semiactive: 1, Inactive: 1}
	if got != want {
		t.Fatalf("CountChatActivitiesByPeriod() = %+v, want %+v", got, want)
	}
}

func TestCountChatActivitiesByPeriodPlatformScoped(t *testing.T) {
	db := openTestLogDB(t)

	insertPlatformChat := func(id int, platform string, group string, pair bool) {
		t.Helper()
		var groupOrNull any
		if group != "" {
			groupOrNull = group
		}
		if err := db.Exec(`
			INSERT INTO chats (id, platform, "group", daily_sending_time, pair_sending, update_notification)
			VALUES (?, ?, ?, NULL, ?, 0)
		`, id, platform, groupOrNull, pair).Error; err != nil {
			t.Fatalf("failed to insert platform chat %d: %v", id, err)
		}
	}

	// Telegram chats: one active (has a log), one semiactive (group + broadcast).
	insertPlatformChat(1, "telegram", "Б-123", false)
	insertPlatformChat(2, "telegram", "Б-456", true)
	// VK chat with the same shape that must NOT leak into the Telegram scope.
	insertPlatformChat(3, "vk", "Б-456", true)

	now := time.Now()
	start := now.Add(-24 * time.Hour)
	end := now.Add(time.Minute)
	if err := db.Exec(`
		INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
		VALUES (1, 'Б-123', ?)
	`, now.Add(-time.Hour)).Error; err != nil {
		t.Fatalf("failed to insert update_log: %v", err)
	}

	repo := &chatRepository{db: db, platform: model.PlatformTelegram}
	got, err := repo.CountChatActivitiesByPeriod(context.Background(), start, end)
	if err != nil {
		t.Fatalf("CountChatActivitiesByPeriod() error: %v", err)
	}
	want := ChatActivityCounts{Active: 1, Semiactive: 1, Inactive: 0}
	if got != want {
		t.Fatalf("CountChatActivitiesByPeriod() = %+v, want %+v", got, want)
	}
}

func TestCountPotentialRequests(t *testing.T) {
	db := openTestLogDB(t)

	now := time.Now().UTC()
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	insertLog := func(chatID, group any) {
		if err := db.Exec(`
			INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
			VALUES (?, ?, datetime('now', ?))
		`, chatID, group, "-30 minutes").Error; err != nil {
			t.Fatalf("failed to insert in-period log: %v", err)
		}
	}

	// In-period, non-empty group_or_teacher — counted (a manual schedule request).
	insertLog(1, "Б-123")
	// In-period, empty group_or_teacher — not counted (not a schedule request).
	insertLog(2, "")
	// In-period, NULL group_or_teacher — not counted.
	insertLog(3, nil)
	// Out-of-period, non-empty group_or_teacher — not counted.
	if err := db.Exec(`
		INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
		VALUES (4, 'Б-456', datetime('now', '-2 hours'))
	`).Error; err != nil {
		t.Fatalf("failed to insert out-of-period log: %v", err)
	}

	repo := &logRepository{db: db}
	got, err := repo.CountPotentialRequests(context.Background(), start, end)
	if err != nil {
		t.Fatalf("CountPotentialRequests() error: %v", err)
	}
	if got != 1 {
		t.Fatalf("CountPotentialRequests() = %d, want 1", got)
	}
}

func TestCountActualRequests(t *testing.T) {
	db := openTestLogDB(t)

	now := time.Now().UTC()
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)

	// Manual requests.
	insertLogs := [][]any{
		// In-period, group set, not cached — counted.
		{1, "Б-123", 0, "-30 minutes"},
		// In-period, group set, cached — not counted (served from cache).
		{2, "Б-456", 1, "-30 minutes"},
		// In-period, no group — not counted.
		{3, "", 0, "-30 minutes"},
		// Out-of-period, group set, not cached — not counted.
		{4, "Б-789", 0, "-2 hours"},
	}
	for _, l := range insertLogs {
		if err := db.Exec(`
			INSERT INTO update_logs (chat_id, group_or_teacher, cached, created_at)
			VALUES (?, ?, ?, datetime('now', ?))
		`, l...).Error; err != nil {
			t.Fatalf("failed to insert update log %v: %v", l, err)
		}
	}

	// Broadcasts: two in-period tasks (3+2 groups), one out-of-period (5 groups).
	insertBroadcast := func(kind string, groups int, offset string) {
		if err := db.Exec(`
			INSERT INTO broadcast_task_logs (kind, groups, created_at)
			VALUES (?, ?, datetime('now', ?))
		`, kind, groups, offset).Error; err != nil {
			t.Fatalf("failed to insert broadcast task log: %v", err)
		}
	}
	insertBroadcast("daily", 3, "-30 minutes")
	insertBroadcast("pair", 2, "-30 minutes")
	insertBroadcast("daily", 5, "-2 hours")

	repo := &logRepository{db: db}
	got, err := repo.CountActualRequests(context.Background(), start, end)
	if err != nil {
		t.Fatalf("CountActualRequests() error: %v", err)
	}
	// 1 (manual uncached) + 5 (broadcast groups) = 6.
	if got != 6 {
		t.Fatalf("CountActualRequests() = %d, want 6", got)
	}
}

func TestCountBroadcastLogsByPeriodAndKind(t *testing.T) {
	db := openTestLogDB(t)

	now := time.Now().UTC()
	start := now.Add(-2 * time.Hour)
	end := now.Add(time.Hour)

	// Task A: mass broadcast, created in period.
	// Use raw sqlDB for inserts: GORM db.Exec does not reliably report errors
	// or set last_insert_rowid() under a single-connection pool.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	// Verify schema using sqlDB
	var colCount int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('broadcast_task_logs')`).Scan(&colCount)
	t.Logf("broadcast_task_logs columns: %d", colCount)

	insertTask := func(kind string, groups int, createdAt time.Time) int64 {
		res, err := sqlDB.Exec(`INSERT INTO broadcast_task_logs (kind, groups, elapsed, created_at) VALUES (?, ?, 0, ?)`,
			kind, groups, createdAt)
		if err != nil {
			t.Fatalf("insert task %s exec: %v", kind, err)
		}
		rows, _ := res.RowsAffected()
		var id int64
		sqlDB.QueryRow(`SELECT last_insert_rowid()`).Scan(&id)
		t.Logf("insertTask %s: rows=%d id=%d createdAt=%v", kind, rows, id, createdAt)
		return id
	}
	taskAID := insertTask("mass", 3, now.Add(-time.Hour))
	taskBID := insertTask("daily", 2, now.Add(-30*time.Minute))
	taskCID := insertTask("daily", 5, now.Add(-3*time.Hour))
	boundary := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC)
	taskDID := insertTask("daily", 1, boundary)

	// Verify tasks using sqlDB
	var verifyTasks []struct {
		ID      int64
		Kind    string
		Created string
	}
	rows, _ := sqlDB.Query(`SELECT id, kind, created_at FROM broadcast_task_logs ORDER BY id`)
	for rows.Next() {
		var r struct {
			ID      int64
			Kind    string
			Created string
		}
		rows.Scan(&r.ID, &r.Kind, &r.Created)
		verifyTasks = append(verifyTasks, r)
	}
	rows.Close()
	for _, dt := range verifyTasks {
		t.Logf("  VERIFY TASK: id=%d kind=%s created=%q", dt.ID, dt.Kind, dt.Created)
	}

	// Verify logs using sqlDB — use individual count queries to avoid
	// any connection-pool / cursor issues with sqlDB.Query().
	var logA1, logB1, logC1, logD1 int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs WHERE broadcast_task_log_id = ?`, taskAID).Scan(&logA1)
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs WHERE broadcast_task_log_id = ?`, taskBID).Scan(&logB1)
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs WHERE broadcast_task_log_id = ?`, taskCID).Scan(&logC1)
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs WHERE broadcast_task_log_id = ?`, taskDID).Scan(&logD1)
	t.Logf("  log counts by task: taskA=%d taskB=%d taskC=%d taskD=%d", logA1, logB1, logC1, logD1)
	var totalLogsInDB int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs`).Scan(&totalLogsInDB)
	t.Logf("  total logs in DB: %d", totalLogsInDB)
	var sampleLog struct {
		ID     int64
		TaskID int64
	}
	sqlDB.QueryRow(`SELECT id, broadcast_task_log_id FROM broadcast_logs LIMIT 1`).Scan(&sampleLog.ID, &sampleLog.TaskID)
	t.Logf("  sample log: id=%d task_id=%d", sampleLog.ID, sampleLog.TaskID)
	// Also count daily via raw sqlDB
	var rawDaily int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs bl JOIN broadcast_task_logs btl ON bl.broadcast_task_log_id = btl.id WHERE btl.kind = 'daily' AND btl.created_at BETWEEN ? AND ?`, start, end).Scan(&rawDaily)
	t.Logf("rawDB daily count (in period): %d", rawDaily)
	// Count without period filter
	var rawDailyAll int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs bl JOIN broadcast_task_logs btl ON bl.broadcast_task_log_id = btl.id WHERE btl.kind = 'daily'`).Scan(&rawDailyAll)
	t.Logf("rawDB daily count (all): %d", rawDailyAll)
	// Show daily task details
	var dailyTaskDetails []struct {
		ID      int64
		Created string
	}
	sqlDB.QueryRow(`SELECT id, created_at FROM broadcast_task_logs WHERE kind = 'daily' ORDER BY id`).Scan(&dailyTaskDetails)
	for _, dt := range dailyTaskDetails {
		t.Logf("  DAILY TASK: id=%d created=%q start=%q end=%q", dt.ID, dt.Created, start, end)
	}

	insertLog := func(taskID, chatID int64) {
		res, err := sqlDB.Exec(`INSERT INTO broadcast_logs (broadcast_task_log_id, chat_id, error) VALUES (?, ?, '')`, taskID, chatID)
		if err != nil {
			t.Fatalf("insert log task=%d chat=%d: %v", taskID, chatID, err)
		}
		rows, _ := res.RowsAffected()
		var cnt int
		sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs`).Scan(&cnt)
		t.Logf("insertLog task=%d chat=%d: rows=%d totalInDB=%d", taskID, chatID, rows, cnt)
	}
	insertLog(taskAID, 1)
	insertLog(taskAID, 2)
	insertLog(taskAID, 3)
	insertLog(taskBID, 4)
	insertLog(taskBID, 5)
	insertLog(taskCID, 6)
	nextDay := boundary.Add(2 * time.Hour)
	if _, err := sqlDB.Exec(`INSERT INTO broadcast_logs (broadcast_task_log_id, chat_id, error, created_at) VALUES (?, 7, '', ?)`,
		taskDID, nextDay); err != nil {
		t.Fatalf("failed to insert log D: %v", err)
	}

	var totalLogs int
	sqlDB.QueryRow(`SELECT COUNT(*) FROM broadcast_logs`).Scan(&totalLogs)
	t.Logf("total_logs=%d (expect 7)", totalLogs)

	// Show task created_at values using sqlDB directly
	var taskTimes2 []struct {
		Kind    string
		Created string
	}
	rows3, _ := sqlDB.Query(`SELECT kind, created_at FROM broadcast_task_logs ORDER BY id`)
	for rows3.Next() {
		var r struct {
			Kind    string
			Created string
		}
		rows3.Scan(&r.Kind, &r.Created)
		taskTimes2 = append(taskTimes2, r)
	}
	rows3.Close()
	for _, tt := range taskTimes2 {
		t.Logf("  task: kind=%s created=%s", tt.Kind, tt.Created)
	}
	// Show which daily logs should be in period using sqlDB directly
	var dailyLogs []struct {
		TaskID      int64
		TaskCreated string
	}
	rows2, _ := sqlDB.Query(`
		SELECT bl.broadcast_task_log_id, btl.created_at
		FROM broadcast_logs bl
		JOIN broadcast_task_logs btl ON bl.broadcast_task_log_id = btl.id
		WHERE btl.kind = 'daily'
	`)
	for rows2.Next() {
		var r struct {
			TaskID      int64
			TaskCreated string
		}
		rows2.Scan(&r.TaskID, &r.TaskCreated)
		dailyLogs = append(dailyLogs, r)
	}
	rows2.Close()
	for _, dl := range dailyLogs {
		t.Logf("  daily log: task_id=%d task_created=%s", dl.TaskID, dl.TaskCreated)
	}

	repo := &logRepository{db: db}
	ctx := context.Background()

	massGot, err := repo.CountBroadcastLogsByPeriodAndKind(ctx, model.BMass, start, end)
	if err != nil {
		t.Fatalf("CountBroadcastLogsByPeriodAndKind(mass) error: %v", err)
	}
	if massGot != 3 {
		t.Fatalf("mass logs = %d, want 3 (taskA has 3 logs)", massGot)
	}

	dailyGot, err := repo.CountBroadcastLogsByPeriodAndKind(ctx, model.BDaily, start, end)
	if err != nil {
		t.Fatalf("CountBroadcastLogsByPeriodAndKind(daily) error: %v", err)
	}
	// Task B (2 logs, in period) = 2. Task C is out of period. Task D (23:59:59) is also
	// outside the [now-2h, now+1h] window when now is ~21:57, so only taskB counts.
	if dailyGot != 2 {
		t.Fatalf("daily logs = %d, want 2 (taskB 2 logs; taskC and taskD are out of period)", dailyGot)
	}

	pairGot, err := repo.CountBroadcastLogsByPeriodAndKind(ctx, model.BPair, start, end)
	if err != nil {
		t.Fatalf("CountBroadcastLogsByPeriodAndKind(pair) error: %v", err)
	}
	if pairGot != 0 {
		t.Fatalf("pair logs = %d, want 0", pairGot)
	}

	anyGot, err := repo.CountBroadcastLogsByPeriodAndKind(ctx, model.BAny, start, end)
	if err != nil {
		t.Fatalf("CountBroadcastLogsByPeriodAndKind(any) error: %v", err)
	}
	// All logs from tasks in period: mass(3) + daily(2) = 5.
	// taskC (now-3h) and taskD (23:59:59) are outside [now-2h, now+1h].
	if anyGot != 5 {
		t.Fatalf("any logs = %d, want 5", anyGot)
	}
}
