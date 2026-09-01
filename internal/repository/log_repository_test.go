package repository

import (
	"context"
	"testing"
	"time"

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
			"group" TEXT,
			daily_sending_time TEXT,
			pair_sending BOOLEAN NOT NULL DEFAULT 0,
			update_notification BOOLEAN NOT NULL DEFAULT 0
		);
		CREATE TABLE update_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			group_or_teacher TEXT,
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

	// In-period, empty group_or_teacher — counted.
	insertLog(1, nil)
	// In-period, non-empty group_or_teacher — not counted.
	insertLog(2, "Б-123")
	// Out-of-period, empty group_or_teacher — not counted.
	if err := db.Exec(`
		INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
		VALUES (3, '', datetime('now', '-2 hours'))
	`).Error; err != nil {
		t.Fatalf("failed to insert out-of-period log: %v", err)
	}
	// Out-of-period, NULL group_or_teacher — must NOT be counted (precedence fix).
	if err := db.Exec(`
		INSERT INTO update_logs (chat_id, group_or_teacher, created_at)
		VALUES (4, NULL, datetime('now', '-2 hours'))
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
