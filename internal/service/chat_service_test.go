package service

import (
	"context"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTestChatDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:chat_service_test?mode=memory&cache=shared"), &gorm.Config{
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

func TestResetGroupSettings(t *testing.T) {
	db := openTestChatDB(t)
	repo := repository.NewChatRepository(db)
	svc := NewChatService(repo)

	group := model.GroupName("ИСПт-22-(9)-2")
	dept := "АиЭС"
	time := "19:00"
	chat := &model.Chat{
		TgChatID:         12345,
		GroupName:        &group,
		DepartmentName:   &dept,
		DailySendingTime: &time,
		PairSending:      true,
		ChangeAlert:      true,
		DarkMode:         true,
		State:            model.ChatStateDefault,
	}
	if err := svc.CreateChat(context.Background(), chat); err != nil {
		t.Fatalf("CreateChat() error: %v", err)
	}

	if err := svc.ResetGroupSettings(context.Background(), chat); err != nil {
		t.Fatalf("ResetGroupSettings() error: %v", err)
	}

	got, err := svc.GetChatByChatID(context.Background(), chat.TgChatID)
	if err != nil {
		t.Fatalf("GetChatByChatID() error: %v", err)
	}

	if got.GroupName != nil {
		t.Errorf("GroupName = %v, want nil", *got.GroupName)
	}
	if got.DepartmentName != nil {
		t.Errorf("DepartmentName = %v, want nil", *got.DepartmentName)
	}
	if got.DailySendingTime != nil {
		t.Errorf("DailySendingTime = %v, want nil", *got.DailySendingTime)
	}
	if got.PairSending {
		t.Error("PairSending = true, want false")
	}
	if got.ChangeAlert {
		t.Error("ChangeAlert = true, want false")
	}
	if got.State != model.ChatStateDefault {
		t.Errorf("State = %v, want %v", got.State, model.ChatStateDefault)
	}
}
