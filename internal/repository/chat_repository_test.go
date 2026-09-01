package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openTestChatDBFull builds a chats table with the columns needed by the list
// queries (tg_chat_id, department, group), plus an update_logs table for the
// activity queries.
func openTestChatDBFull(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:chat_repo_spec_test?mode=memory&cache=shared"), &gorm.Config{
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
	db.Exec(`DROP TABLE IF EXISTS update_logs`)
	db.Exec(`DROP TABLE IF EXISTS chats`)
	if err := db.Exec(`
		CREATE TABLE chats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tg_chat_id INTEGER,
			department TEXT,
			"group" TEXT,
			created_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("failed to create chats table: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE update_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id INTEGER,
			created_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("failed to create update_logs table: %v", err)
	}
	return db
}

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

// openTestChatDBUnique builds a chats table that mirrors the production schema
// (00001_baseline.sql), including the UNIQUE constraint on tg_chat_id.
func openTestChatDBUnique(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:chat_repo_unique_test?mode=memory&cache=shared"), &gorm.Config{
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
	if err := db.Exec(`DROP TABLE IF EXISTS chats`).Error; err != nil {
		t.Fatalf("failed to drop chats table: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE chats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tg_chat_id INTEGER NOT NULL UNIQUE,
			username TEXT,
			state TEXT DEFAULT 'default',
			department TEXT,
			"group" TEXT,
			daily_sending_time TEXT,
			pair_sending BOOLEAN NOT NULL DEFAULT 0,
			update_notification BOOLEAN NOT NULL DEFAULT 0,
			dark_mode BOOLEAN NOT NULL DEFAULT 0,
			access INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`).Error; err != nil {
		t.Fatalf("failed to create chats table: %v", err)
	}
	return db
}

func TestCreateOrUpdateChatConcurrentCreate(t *testing.T) {
	db := openTestChatDBUnique(t)
	repo := &chatRepository{db: db}

	const baseChatID = int64(1327362040)
	const goroutines = 16
	const iterations = 5
	const username = "some_user"

	for iter := 0; iter < iterations; iter++ {
		chatID := model.ChatID(baseChatID + int64(iter))
		var createdCount int
		seen := make(map[int64]struct{}, goroutines)
		var mu sync.Mutex
		var wg sync.WaitGroup
		start := make(chan struct{})

		wg.Add(goroutines)
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				<-start
				chat := &model.Chat{TgChatID: chatID, UserName: new(username)}
				created, err := repo.CreateOrUpdateChat(context.Background(), chat)
				if err != nil {
					t.Errorf("CreateOrUpdateChat() error: %v", err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				if created {
					createdCount++
				}
				seen[chat.ID] = struct{}{}
			}()
		}
		close(start)
		wg.Wait()

		if createdCount != 1 {
			t.Fatalf("iteration %d: createdCount = %d, want 1", iter, createdCount)
		}
		if len(seen) != 1 {
			t.Fatalf("iteration %d: %d distinct chat rows created, want 1", iter, len(seen))
		}
	}
}

func TestCreateOrUpdateChatExisting(t *testing.T) {
	db := openTestChatDBUnique(t)
	repo := &chatRepository{db: db}

	username := "first_name"
	chat := &model.Chat{TgChatID: model.ChatID(100), UserName: new(username)}
	created, err := repo.CreateOrUpdateChat(context.Background(), chat)
	if err != nil {
		t.Fatalf("first CreateOrUpdateChat() error: %v", err)
	}
	if !created || chat.ID == 0 {
		t.Fatalf("first call: created = %v, id = %d; want created=true and non-zero id", created, chat.ID)
	}
	firstID := chat.ID

	// Second call for the same chat must reuse the existing row.
	updatedUsername := "second_name"
	again := &model.Chat{TgChatID: model.ChatID(100), UserName: new(updatedUsername)}
	created, err = repo.CreateOrUpdateChat(context.Background(), again)
	if err != nil {
		t.Fatalf("second CreateOrUpdateChat() error: %v", err)
	}
	if created {
		t.Fatal("second call: created = true, want false")
	}
	if again.ID != firstID {
		t.Fatalf("second call: id = %d, want %d (same row)", again.ID, firstID)
	}
	if again.UserName == nil || *again.UserName != updatedUsername {
		t.Fatalf("second call: username = %v, want %q", again.UserName, updatedUsername)
	}
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

func TestGetChatsByDepartment(t *testing.T) {
	db := openTestChatDBFull(t)
	for _, r := range []struct{ dept string }{
		{"АиЭС"}, {"АиЭС"}, {"НГО"},
	} {
		if err := db.Exec(`INSERT INTO chats (department) VALUES (?)`, r.dept).Error; err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}
	repo := &chatRepository{db: db}
	got, err := repo.GetChatsByDepartment(context.Background(), "АиЭС")
	if err != nil {
		t.Fatalf("GetChatsByDepartment() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d chats, want 2", len(got))
	}
}

func TestGetGroupChatsAndPrivate(t *testing.T) {
	db := openTestChatDBFull(t)
	for _, id := range []int64{10, 20, -5, -7} {
		if err := db.Exec(`INSERT INTO chats (tg_chat_id) VALUES (?)`, id).Error; err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}
	repo := &chatRepository{db: db}

	groups, err := repo.GetGroupChats(context.Background())
	if err != nil {
		t.Fatalf("GetGroupChats() error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d group chats, want 2", len(groups))
	}

	priv, err := repo.GetPrivateChats(context.Background())
	if err != nil {
		t.Fatalf("GetPrivateChats() error: %v", err)
	}
	if len(priv) != 2 {
		t.Fatalf("got %d private chats, want 2", len(priv))
	}
}

func TestGetActiveChatsAndCount(t *testing.T) {
	db := openTestChatDBFull(t)
	// chats 1,2,3
	if err := db.Exec(`INSERT INTO chats (id, tg_chat_id) VALUES (1, 10), (2, 20), (3, 30)`).Error; err != nil {
		t.Fatalf("failed to insert chats: %v", err)
	}
	// chat 1 has a recent update log (active), chat 2 has an old one (inactive)
	now := time.Now()
	if err := db.Exec(`INSERT INTO update_logs (chat_id, created_at) VALUES (1, ?), (2, ?)`,
		now.Add(-time.Hour), now.Add(-7*24*time.Hour)).Error; err != nil {
		t.Fatalf("failed to insert update_logs: %v", err)
	}

	repo := &chatRepository{db: db}

	active, err := repo.GetActiveChats(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("GetActiveChats() error: %v", err)
	}
	if len(active) != 1 || active[0].ID != 1 {
		t.Fatalf("got active chats %+v, want just chat 1", active)
	}

	count, err := repo.CountActiveChats(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("CountActiveChats() error: %v", err)
	}
	if count != 1 {
		t.Fatalf("got active count %d, want 1", count)
	}
}

func TestCountNewChatsByPeriod(t *testing.T) {
	db := openTestChatDBFull(t)
	startT := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	endT := time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC)
	in := "2026-08-25T12:00:00Z"
	before := "2026-08-10T12:00:00Z"
	after := "2026-09-01T12:00:00Z"
	if err := db.Exec(`INSERT INTO chats (id, tg_chat_id, created_at) VALUES (1, 10, ?), (2, 20, ?), (3, 30, ?)`,
		in, before, after).Error; err != nil {
		t.Fatalf("failed to insert chats: %v", err)
	}

	repo := &chatRepository{db: db}
	got, err := repo.CountNewChatsByPeriod(context.Background(), startT, endT)
	if err != nil {
		t.Fatalf("CountNewChatsByPeriod() error: %v", err)
	}
	if got != 1 {
		t.Fatalf("CountNewChatsByPeriod() = %d, want 1", got)
	}
}

func TestGetNewChatCountByYearByPeriod(t *testing.T) {
	db := openTestChatDBFull(t)
	startT := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	endT := time.Date(2026, 8, 27, 23, 59, 59, 0, time.UTC)
	in := "2026-08-25T12:00:00Z"
	before := "2026-08-10T12:00:00Z"
	if err := db.Exec(`INSERT INTO chats (id, tg_chat_id, "group", created_at) VALUES
		(1, 10, 'ИСПт-24-(9)-1', ?), (2, 20, 'ГРПт-24-11-2', ?), (3, 30, 'Б-123', ?), (4, 40, '', ?)`,
		in, in, before, in).Error; err != nil {
		t.Fatalf("failed to insert chats: %v", err)
	}

	repo := &chatRepository{db: db}
	got, err := repo.GetNewChatCountByYearByPeriod(context.Background(), startT, endT)
	if err != nil {
		t.Fatalf("GetNewChatCountByYearByPeriod() error: %v", err)
	}
	// Only chats within the window: ids 1 (ИСПт-24 -> 24), 2 (ГРПт-24 -> 24)
	// and 4 ('' -> year 0). Chat 3's group parses to an error and is skipped;
	// it is outside the window anyway.
	want := map[int]int{24: 2, 0: 1}
	if len(got) != len(want) {
		t.Fatalf("GetNewChatCountByYearByPeriod() = %v, want %v", got, want)
	}
	for y, c := range want {
		if got[y] != c {
			t.Fatalf("GetNewChatCountByYearByPeriod()[%d] = %d, want %d", y, got[y], c)
		}
	}
}
