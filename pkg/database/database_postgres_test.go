package database

import (
	"os"
	"strings"
	"testing"

	"github.com/azzimoda/raspishika-gx/pkg/testutil"
)

// TestPostgresMigrateUp applies the postgres migration set against a live
// server. It is disabled by default so `go test ./...` needs no external
// services; set TEST_POSTGRES_DSN to a reachable PostgreSQL DSN to enable, e.g.
//
//	TEST_POSTGRES_DSN="host=127.0.0.1 port=5455 user=postgres password=raspishika dbname=raspishika sslmode=disable" go test ./pkg/database/
func TestPostgresMigrateUp(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping postgres migration test")
	}
	testutil.MoveToProjectRoot()
	cfg := Config{Driver: "postgres", MigrationsDir: "migrations"}
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

	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("failed to open/migrate database: %v", err)
	}

	version := struct{ Version int64 }{}
	if err := db.Raw("SELECT MAX(version_id) AS version FROM goose_db_version").Scan(&version).Error; err != nil {
		t.Fatalf("failed to read goose version: %v", err)
	}
	if version.Version != 5 {
		t.Fatalf("expected 5 postgres migrations to be applied, got version %d", version.Version)
	}

	var hasPlatform int
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name='chats' AND column_name='platform'`).
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
		t.Fatal("duplicate (platform, peer_id) was accepted")
	}

	var hasJobs int
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name='broadcast_jobs'`).
		Scan(&hasJobs).Error; err != nil {
		t.Fatalf("failed to inspect tables: %v", err)
	}
	if hasJobs != 1 {
		t.Fatal("broadcast_jobs table was not created")
	}
}
