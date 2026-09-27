package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/testutil"
)

// TestPostgresCountChatActivitiesByPeriod runs CountChatActivitiesByPeriod
// against a live PostgreSQL server to catch dialect regressions (the query uses
// IS TRUE/IS FALSE on boolean columns instead of SQLite-style `= 1`). It is
// disabled by default; enable with TEST_POSTGRES_DSN, e.g.
//
//	TEST_POSTGRES_DSN="host=127.0.0.1 port=5455 user=postgres password=raspishika dbname=raspishika sslmode=disable" go test ./internal/repository/
func TestPostgresCountChatActivitiesByPeriod(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping postgres repository test")
	}
	testutil.MoveToProjectRoot()
	cfg := database.Config{Driver: "postgres", MigrationsDir: "migrations", AutoMigrate: true}
	for _, kv := range strings.Fields(dsn) {
		key, value, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		switch key {
		case "host":
			cfg.Host = value
		case "port":
			cfg.Port = value
		case "user":
			cfg.User = value
		case "password":
			cfg.Password = value
		case "dbname":
			cfg.Name = value
		case "sslmode":
			cfg.SSLMode = value
		default:
			t.Fatalf("unknown DSN key %q", key)
		}
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.User == "" || cfg.Name == "" {
		t.Skipf("incomplete DSN %q", dsn)
	}

	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("failed to open/migrate database: %v", err)
	}

	const platform = "vt" // VK test scope, so rows never collide with the app
	t.Cleanup(func() {
		db.Exec(`DELETE FROM update_logs WHERE chat_id IN (SELECT id FROM chats WHERE platform = ?)`, platform)
		db.Exec(`DELETE FROM chats WHERE platform = ?`, platform)
	})

	now := time.Now()
	start, end := now.Add(-24*time.Hour), now

	chats := []model.Chat{
		// active: has an update log inside the period
		{PeerID: model.ChatID(1_000_001), Platform: platform, GroupName: strPtr("АиЭС")},
		// semiactive: no logs, group + broadcast enabled
		{PeerID: model.ChatID(1_000_002), Platform: platform, GroupName: strPtr("НГО")},
		// inactive: no logs and no broadcast
		{PeerID: model.ChatID(1_000_003), Platform: platform},
	}
	if err := db.Create(&chats).Error; err != nil {
		t.Fatalf("failed to insert chats: %v", err)
	}

	// Pair notification (semiactive chat) and an update log (active chat).
	if err := db.Model(model.Chat{}).
		Where("tg_chat_id IN ?", []model.ChatID{chats[0].PeerID, chats[1].PeerID}).
		Update("pair_sending", true).Error; err != nil {
		t.Fatalf("failed to enable pair_sending: %v", err)
	}
	if err := db.Exec(`INSERT INTO update_logs (chat_id, kind, message_id, data, elapsed, created_at) VALUES (?, ?, 0, '', 0, ?)`,
		chats[0].ID, "test", start.Add(time.Hour)).Error; err != nil {
		t.Fatalf("failed to insert update log: %v", err)
	}

	repo := NewChatRepository(db, model.Platform(platform))
	got, err := repo.CountChatActivitiesByPeriod(context.Background(), start, end)
	if err != nil {
		t.Fatalf("CountChatActivitiesByPeriod() error on postgres: %v", err)
	}
	want := ChatActivityCounts{Active: 1, Semiactive: 1, Inactive: 1}
	if got != want {
		t.Fatalf("CountChatActivitiesByPeriod() = %+v, want %+v", got, want)
	}
}

func strPtr(s string) *model.GroupName { g := model.GroupName(s); return &g }
