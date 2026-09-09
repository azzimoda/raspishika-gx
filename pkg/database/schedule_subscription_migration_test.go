package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/testutil"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestScheduleSubscriptionMigrationPreservesSettings(t *testing.T) {
	root, err := testutil.FindProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var opener gorm.Dialector
			migration := "00007_schedule_subscriptions.sql"
			if dialect == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not set")
				}
				opener = postgres.Open(dsn)
				migration = "postgres/00006_schedule_subscriptions.sql"
			} else {
				opener = sqlite.Open(filepath.Join(t.TempDir(), "migration.db"))
			}
			db, err := gorm.Open(opener, &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			tx, err := sqlDB.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			exec := func(query string) {
				t.Helper()
				if _, err := tx.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if dialect == "postgres" {
				// Временная схема откатывается вместе с транзакцией теста.
				schema := fmt.Sprintf("subscription_test_%d", time.Now().UnixNano())
				exec("CREATE SCHEMA " + schema)
				exec("SET LOCAL search_path TO " + schema)
			}
			exec(`CREATE TABLE chats (id INTEGER PRIMARY KEY, "group" TEXT, department TEXT, daily_sending_time TEXT, platform TEXT)`)
			exec(`INSERT INTO chats VALUES (1, 'A', 'Отделение', '08:00', 'telegram'), (2, 'B', NULL, NULL, 'vk'), (3, NULL, NULL, NULL, 'telegram'), (4, '', NULL, NULL, 'telegram')`)
			contents, err := os.ReadFile(filepath.Join(root, "migrations", migration))
			if err != nil {
				t.Fatal(err)
			}
			exec(strings.Split(string(contents), "-- +goose Down")[0])
			var count int
			if err := tx.QueryRow("SELECT COUNT(*) FROM schedule_subscriptions").Scan(&count); err != nil || count != 2 {
				t.Fatalf("backfill count = %d, error = %v", count, err)
			}
			var name, department string
			var daily sql.NullString
			if err := tx.QueryRow(`SELECT s.group_name, s.department_name, c.daily_sending_time FROM schedule_subscriptions s JOIN chats c ON c.id = s.chat_id WHERE c.id = 1`).Scan(&name, &department, &daily); err != nil {
				t.Fatal(err)
			}
			if name != "A" || department != "Отделение" || !daily.Valid || daily.String != "08:00" {
				t.Fatalf("enabled settings changed: %s %s %v", name, department, daily)
			}
			if err := tx.QueryRow(`SELECT c.daily_sending_time FROM schedule_subscriptions s JOIN chats c ON c.id = s.chat_id WHERE c.id = 2`).Scan(&daily); err != nil {
				t.Fatal(err)
			}
			if daily.Valid {
				t.Fatal("migration enabled a disabled broadcast")
			}
			exec("SAVEPOINT duplicate_subscription")
			if _, err := tx.Exec("INSERT INTO schedule_subscriptions (chat_id, group_name) VALUES (1, 'A')"); err == nil {
				t.Fatal("duplicate accepted")
			}
			exec("ROLLBACK TO SAVEPOINT duplicate_subscription")
		})
	}
}
