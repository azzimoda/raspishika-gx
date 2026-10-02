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

// openTestPlatformScopeDB builds a chats table with every column the raw
// reporting queries group or filter on, plus an update_logs table.
func openTestPlatformScopeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:chat_platform_scope_test?mode=memory&cache=shared"), &gorm.Config{
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
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS update_logs`,
		`DROP TABLE IF EXISTS chats`,
		`CREATE TABLE chats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tg_chat_id INTEGER,
			platform TEXT,
			department TEXT,
			"group" TEXT,
			access INTEGER NOT NULL DEFAULT 0,
			daily_sending_time TEXT,
			pair_sending BOOLEAN NOT NULL DEFAULT 0,
			update_notification BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME
		)`,
		`CREATE TABLE update_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER NOT NULL,
			created_at DATETIME
		)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("failed to prepare schema (%s): %v", stmt, err)
		}
	}
	return db
}

// seedPlatformScopeChats inserts Telegram and VK rows that differ on every
// dimension the reporting queries group or filter on, so a query that ignores
// the repository's platform produces a different answer than a filtered one.
func seedPlatformScopeChats(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	rows := [][]any{
		// id, tg_chat_id, platform, department, group, access, daily, pair, update
		{1, 100, "telegram", "ПО", "Б-11(9)1", 0, "09:00", 0, 1},
		{2, 200, "telegram", "ПО", "Б-11(9)1", 1, "09:00", 0, 0},
		{3, 300, "telegram", "ПО", "АиЭС-11(9)1", 2, nil, 1, 1},
		{4, -100, "telegram", "ПО", "Группа-ТГ", 0, nil, 0, 0},
		{5, 2000000002, "telegram", "ПО", "Большой-ID", 0, nil, 0, 0},
		{11, 1000000, "vk", "ИС", "НГО-11(11)2", 1, "11:00", 0, 1},
		{12, 2000000, "vk", "ИС", "НГО-11(11)2", 2, "11:00", 0, 0},
		{13, 2000000001, "vk", "ИС", "Группа-VK", 1, nil, 0, 0},
	}
	for _, r := range rows {
		var daily any
		if r[6] != nil {
			daily = r[6]
		}
		if err := db.Exec(`
			INSERT INTO chats (id, tg_chat_id, platform, department, "group", access, daily_sending_time,
				pair_sending, update_notification, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, r[0], r[1], r[2], r[3], r[4], r[5], daily, r[7], r[8], now).Error; err != nil {
			t.Fatalf("failed to insert chat %v: %v", r[0], err)
		}
	}
}

func sumInts(vals ...int) int {
	total := 0
	for _, v := range vals {
		total += v
	}
	return total
}

// TestRawQueriesRespectPlatformScope covers every repository method built on a
// raw query. gorm's Raw drops a Where clause set on the session beforehand, so
// these used to ignore the repository's platform entirely and a VK repo
// reported Telegram rows and vice versa.
//
// The invariant is the same for all of them and needs no per-query expected
// values: the admin view (empty platform) must be the disjoint union of the two
// platform views. A query that drops the filter counts the other platform's
// rows twice, so the sum no longer matches.
func TestRawQueriesRespectPlatformScope(t *testing.T) {
	db := openTestPlatformScopeDB(t)
	now := time.Now()
	seedPlatformScopeChats(t, db, now)

	ctx := context.Background()
	start, end := now.Add(-time.Hour), now.Add(time.Hour)
	admin := NewChatRepository(db, "")
	telegram := NewChatRepository(db, model.PlatformTelegram)
	vk := NewChatRepository(db, model.PlatformVK)

	// total reduces a result to a number that adds up across platforms.
	cases := []struct {
		name  string
		total func(ChatRepository) (int, error)
	}{
		{"GetNewChatCountByYearByPeriod", func(r ChatRepository) (int, error) {
			m, err := r.GetNewChatCountByYearByPeriod(ctx, start, end)
			return sumInts(m[11]), err
		}},
		{"CountChatActivitiesByPeriod", func(r ChatRepository) (int, error) {
			c, err := r.CountChatActivitiesByPeriod(ctx, start, end)
			return sumInts(c.Active, c.Semiactive, c.Inactive), err
		}},
		{"GetGroupedCountChatCountByTime", func(r ChatRepository) (int, error) {
			rows, err := r.GetGroupedCountChatCountByTime(ctx)
			total := 0
			for _, row := range rows {
				total += row.Count
			}
			return total, err
		}},
		{"GetChatCountByDepartment", func(r ChatRepository) (int, error) {
			rows, err := r.GetChatCountByDepartment(ctx)
			total := 0
			for _, row := range rows {
				total += row.Count
			}
			return total, err
		}},
		{"GetChatsByAccessLevel", func(r ChatRepository) (int, error) {
			m, err := r.GetChatsByAccessLevel(ctx)
			total := 0
			for _, count := range m {
				total += count
			}
			return total, err
		}},
		{"GetChatCountByPlatform", func(r ChatRepository) (int, error) {
			m, err := r.GetChatCountByPlatform(ctx)
			total := 0
			for _, count := range m {
				total += count
			}
			return total, err
		}},
		{"GetTopGroupsByChatCount", func(r ChatRepository) (int, error) {
			rows, err := r.GetTopGroupsByChatCount(ctx, 100)
			total := 0
			for _, row := range rows {
				total += row.Count
			}
			return total, err
		}},
		{"CountAllConfiguredGroups", func(r ChatRepository) (int, error) {
			return r.CountAllConfiguredGroups(ctx)
		}},
		{"CountPricateChats", func(r ChatRepository) (int, error) {
			return r.CountPricateChats(ctx)
		}},
		{"GetWatchedGroupNames", func(r ChatRepository) (int, error) {
			names, err := r.GetWatchedGroupNames(ctx)
			return len(names), err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adminTotal, err := tc.total(admin)
			if err != nil {
				t.Fatalf("admin scope error: %v", err)
			}
			tgTotal, err := tc.total(telegram)
			if err != nil {
				t.Fatalf("telegram scope error: %v", err)
			}
			vkTotal, err := tc.total(vk)
			if err != nil {
				t.Fatalf("vk scope error: %v", err)
			}
			// Both platforms must contribute, otherwise the union below would
			// hold even if a scope returned nothing at all.
			if tgTotal == 0 || vkTotal == 0 {
				t.Fatalf("seed data is not exercised: telegram=%d vk=%d", tgTotal, vkTotal)
			}
			if want := tgTotal + vkTotal; adminTotal != want {
				t.Fatalf("admin scope counted %d, want %d (telegram %d + vk %d): a platform scope is leaking rows from the other platform",
					adminTotal, want, tgTotal, vkTotal)
			}
		})
	}
}

// TestGetAvgChatPerGroupPlatformScope pins the values rather than the union,
// since an average over grouped counts does not add up across platforms.
func TestGetAvgChatPerGroupPlatformScope(t *testing.T) {
	db := openTestPlatformScopeDB(t)
	seedPlatformScopeChats(t, db, time.Now())

	ctx := context.Background()
	// Telegram: groups of 3, 1, 1, 1 chats. VK: groups of 2, 1. Both: 6 groups, 8 chats.
	want := map[model.Platform]float64{
		model.PlatformTelegram: 1.25,
		model.PlatformVK:       1.5,
		"":                     8.0 / 6.0,
	}
	for platform, exp := range want {
		got, err := NewChatRepository(db, platform).GetAvgChatPerGroup(ctx)
		if err != nil {
			t.Fatalf("platform %q: error: %v", platform, err)
		}
		if got != exp {
			t.Errorf("platform %q: GetAvgChatPerGroup() = %v, want %v", platform, got, exp)
		}
	}
}

// TestPlatformScopedRawSQLHasNoDanglingWhere guards the shape of the rendered
// SQL: whereClause has to leave an empty string for the empty platform and a
// complete clause otherwise, or a query breaks or silently drops the filter.
func TestPlatformScopedRawSQLHasNoDanglingWhere(t *testing.T) {
	repo := &chatRepository{platform: model.PlatformVK}
	if cond, args := repo.platformCond(""); cond != "platform = ?" || len(args) != 1 {
		t.Fatalf("platformCond(\"\") = %q, %v; want \"platform = ?\", one arg", cond, args)
	}
	if cond, args := repo.platformCond("c"); cond != "c.platform = ?" || len(args) != 1 {
		t.Fatalf("platformCond(\"c\") = %q, %v; want \"c.platform = ?\", one arg", cond, args)
	}
	spanning := &chatRepository{platform: ""}
	if cond, args := spanning.platformCond("c"); cond != "" || args != nil {
		t.Fatalf("empty platform: platformCond(\"c\") = %q, %v; want \"\", nil", cond, args)
	}

	if got := whereClause(); got != "" {
		t.Errorf("whereClause() = %q, want empty", got)
	}
	if got := whereClause("", "a = ?"); got != "WHERE a = ?" {
		t.Errorf("whereClause(\"\", \"a = ?\") = %q, want %q", got, "WHERE a = ?")
	}
	if got := whereClause("platform = ?", "a = ?"); got != "WHERE platform = ? AND a = ?" {
		t.Errorf("whereClause with two conds = %q", got)
	}
}
