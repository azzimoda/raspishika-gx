package database

import (
	"path/filepath"
	"testing"

	"github.com/azzimoda/raspishika-gx/pkg/testutil"
)

// TestSQLiteMigrateUp applies the SQLite migration set to a fresh file and
// asserts the multi-platform schema shaped by 00005/00006.
func TestSQLiteMigrateUp(t *testing.T) {
	testutil.MoveToProjectRoot()
	file := filepath.Join(t.TempDir(), "migrated.db")

	db, err := Open(Config{File: file, MigrationsDir: "migrations", Driver: "sqlite3"})
	if err != nil {
		t.Fatalf("failed to open/migrate database: %v", err)
	}

	version := struct{ Version int64 }{}
	if err := db.Raw("SELECT MAX(version_id) AS version FROM goose_db_version").Scan(&version).Error; err != nil {
		t.Fatalf("failed to read goose version: %v", err)
	}
	if version.Version != 7 {
		t.Fatalf("expected 7 SQLite migrations to be applied, got version %d", version.Version)
	}

	var hasPlatform int
	if err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info('chats') WHERE name='platform'`).
		Scan(&hasPlatform).Error; err != nil {
		t.Fatalf("failed to inspect chats columns: %v", err)
	}
	if hasPlatform != 1 {
		t.Fatal("chats table has no platform column")
	}

	if err := db.Exec(`INSERT INTO chats (tg_chat_id, platform) VALUES (?, ?)`, int64(12345), "telegram").Error; err != nil {
		t.Fatalf("failed to insert telegram chat: %v", err)
	}
	if err := db.Exec(`INSERT INTO chats (tg_chat_id, platform) VALUES (?, ?)`, int64(12345), "vk").Error; err != nil {
		t.Fatalf("failed to insert vk chat with colliding peer id: %v", err)
	}
	if err := db.Exec(`INSERT INTO chats (tg_chat_id, platform) VALUES (?, ?)`, int64(12345), "vk").Error; err == nil {
		t.Fatal("duplicate (platform, tg_chat_id) was accepted")
	}

	var hasJobs int
	if err := db.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='broadcast_jobs'`).
		Scan(&hasJobs).Error; err != nil {
		t.Fatalf("failed to inspect tables: %v", err)
	}
	if hasJobs != 1 {
		t.Fatal("broadcast_jobs table was not created")
	}
}
