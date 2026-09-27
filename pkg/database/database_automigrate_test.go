package database

import (
	"path/filepath"
	"testing"

	"github.com/azzimoda/raspishika-gx/pkg/testutil"
)

// bot, adminbot and vkbot share one SQLite file and each opened it with
// migrations enabled, so a deploy starting them together had several processes
// run goose over the same pending migration. The busy timeout serialises the
// writes but not goose's version bookkeeping, and the loser fails on statements
// the winner already committed.
//
// Compose therefore migrates once in a dedicated service and opens the database
// with AutoMigrate off everywhere else.
func TestOpenSkipsMigrationsWhenDisabled(t *testing.T) {
	testutil.MoveToProjectRoot()
	file := filepath.Join(t.TempDir(), "unmigrated.db")

	db, err := Open(Config{
		File:          file,
		MigrationsDir: "migrations",
		Driver:        "sqlite3",
		AutoMigrate:   false,
	})
	if err != nil {
		t.Fatalf("open without migrations: %v", err)
	}

	var tables int
	if err := db.Raw(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goose_db_version'",
	).Scan(&tables).Error; err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("migrations ran even though AutoMigrate was disabled")
	}
}

func TestMigrationsDir(t *testing.T) {
	with := Config{MigrationsDir: "migrations", AutoMigrate: true}
	if got := with.migrationsDir(); got != "migrations" {
		t.Errorf("migrationsDir() = %q, want migrations", got)
	}

	without := Config{MigrationsDir: "migrations", AutoMigrate: false}
	if got := without.migrationsDir(); got != "" {
		t.Errorf("migrationsDir() = %q, want empty when disabled", got)
	}

	// An unset directory stays unset either way.
	if got := (Config{AutoMigrate: true}).migrationsDir(); got != "" {
		t.Errorf("migrationsDir() = %q, want empty when unconfigured", got)
	}
}
