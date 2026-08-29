package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestChatDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:chat_repo_test?mode=memory&cache=shared"), &gorm.Config{
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
			department TEXT,
			"group" TEXT
		)
	`).Error; err != nil {
		t.Fatalf("failed to create chats table: %v", err)
	}
	return db
}

func TestNormalizeDepartmentName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Отделение АиЭС", "АиЭС"},
		{" Отделение НГО ", "НГО"},
		{"Отделение СОНХ", "СОНХ"},
		{"Отделение ПО", "Политехническое"},
		{"ПО", "Политехническое"},
		{"Политехническое", "Политехническое"},
		{"Отделение МПН", "МПН"},
		{"МПН Осипенко", "МПН Осипенко"},
		{"МПН Энергетиков", "МПН Энергетиков"},
		{"НГО", "НГО"},
		{"Заочное обучение", "Заочное обучение"},
		{"", "unknown"},
		{"unknown", "unknown"},
	}
	for _, c := range cases {
		if got := normalizeDepartmentName(c.in); got != c.want {
			t.Errorf("normalizeDepartmentName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMergeNameCounts(t *testing.T) {
	in := []NameCount{
		{Name: "Отделение АиЭС", Count: 3},
		{Name: "АиЭС", Count: 2},
		{Name: "Отделение ПО", Count: 4},
		{Name: "Политехническое", Count: 1},
		{Name: "unknown", Count: 1},
	}
	got := mergeNameCounts(in)
	want := []NameCount{
		{Name: "АиЭС", Count: 5},
		{Name: "Политехническое", Count: 5},
		{Name: "unknown", Count: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows[%d] = %+v, want %+v (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestGetChatCountByDepartment(t *testing.T) {
	db := openTestChatDB(t)
	rows := []struct{ department string }{
		{"Отделение АиЭС"},
		{"АиЭС"},
		{"Отделение ПО"},
		{"Политехническое"},
		{""},
		{""},
	}
	for _, r := range rows {
		if err := db.Exec(`INSERT INTO chats (department) VALUES (?)`, r.department).Error; err != nil {
			t.Fatalf("failed to insert row: %v", err)
		}
	}

	repo := &chatRepository{db: db}
	got, err := repo.GetChatCountByDepartment(context.Background())
	if err != nil {
		t.Fatalf("GetChatCountByDepartment() error: %v", err)
	}
	want := []NameCount{
		{Name: "unknown", Count: 2},
		{Name: "АиЭС", Count: 2},
		{Name: "Политехническое", Count: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rows[%d] = %+v, want %+v (all: %v)", i, got[i], want[i], got)
		}
	}
}
