package repository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/testutil"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func subscriptionRepository(t *testing.T) (ChatRepository, *gorm.DB, *model.Chat) {
	t.Helper()
	testutil.MoveToProjectRoot()
	db, err := database.Open(database.Config{File: filepath.Join(t.TempDir(), "subscriptions.db"), MigrationsDir: "migrations"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	repo := NewChatRepository(db, model.PlatformTelegram)
	daily := "08:00"
	chat := &model.Chat{PeerID: 101, DailySendingTime: &daily, PairSending: true, ChangeAlert: true, DarkMode: true}
	if err := repo.CreateChat(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	return repo, db, chat
}

func TestPostgresScheduleSubscriptions(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	schema := fmt.Sprintf("subscription_repo_test_%d", time.Now().UnixNano())
	for _, query := range []string{"CREATE SCHEMA " + schema, "SET LOCAL search_path TO " + schema} {
		if err := tx.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	root, err := testutil.FindProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"00001_baseline.sql", "00006_schedule_subscriptions.sql"} {
		contents, err := os.ReadFile(filepath.Join(root, "migrations/postgres", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(strings.Split(string(contents), "-- +goose Down")[0]).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewChatRepository(tx, model.PlatformTelegram)
	ctx := context.Background()
	daily := "08:00"
	chat := &model.Chat{PeerID: 101, DailySendingTime: &daily}
	if err := repo.CreateChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "B"}); err != nil {
		t.Fatal(err)
	}
	if added, err := repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "B"}); err != nil || added {
		t.Fatalf("duplicate: %v, %v", added, err)
	}
	recipients, err := repo.GetDailyRecipients(ctx, daily)
	if err != nil || len(recipients) != 2 {
		t.Fatalf("recipients: %v, %v", recipients, err)
	}
	if exists, err := repo.HasScheduleSubscription(ctx, recipients[1].Subscription); err != nil || !exists {
		t.Fatalf("exists: %v, %v", exists, err)
	}
	if _, err := repo.RemoveUnavailableGroup(ctx, chat.ID, "A"); err != nil {
		t.Fatal(err)
	}
	current, err := repo.GetChat(ctx, chat.ID)
	if err != nil || current.GroupName != nil || current.DailySendingTime == nil {
		t.Fatalf("removed primary: %+v, %v", current, err)
	}
	if removed, err := repo.RemoveScheduleSubscription(ctx, chat.ID, recipients[1].Subscription.ID); err != nil || !removed {
		t.Fatalf("remove: %v, %v", removed, err)
	}
	current, _ = repo.GetChat(ctx, chat.ID)
	if current.DailySendingTime != nil {
		t.Fatal("empty daily broadcast remains enabled")
	}
	if _, err := repo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "C"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteChat(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := tx.Model(&model.ScheduleSubscription{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("subscriptions after stop: %d, %v", count, err)
	}
}

func TestScheduleSubscriptionsPrimaryReplacementAndDuplicates(t *testing.T) {
	repo, db, chat := subscriptionRepository(t)
	ctx := context.Background()
	for i, name := range []model.GroupName{"A", "B", "C"} {
		group := model.Group{GroupName: name, DepartmentName: "Отделение"}
		if i == 0 {
			if _, err := repo.SetPrimaryGroup(ctx, chat.ID, group); err != nil {
				t.Fatal(err)
			}
		} else if added, err := repo.AddScheduleSubscription(ctx, chat.ID, group); err != nil || !added {
			t.Fatalf("add: %v, %v", added, err)
		}
		if added, err := repo.AddScheduleSubscription(ctx, chat.ID, group); err != nil || added {
			t.Fatalf("duplicate: %v, %v", added, err)
		}
	}
	subscriptions, err := repo.GetScheduleSubscriptions(ctx, chat.ID)
	if err != nil || len(subscriptions) != 3 {
		t.Fatalf("subscriptions: %v, %v", subscriptions, err)
	}
	if _, err := repo.RemoveScheduleSubscription(ctx, chat.ID, subscriptions[0].ID); !errors.Is(err, ErrPrimarySubscription) {
		t.Fatalf("primary removal: %v", err)
	}
	current, err := repo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "B", DepartmentName: "Другое отделение"})
	if err != nil || *current.GroupName != "B" || *current.DepartmentName != "Другое отделение" {
		t.Fatalf("replacement: %v, %v", current, err)
	}
	if current.DailySendingTime == nil || *current.DailySendingTime != "08:00" || !current.DarkMode || !current.PairSending || !current.ChangeAlert {
		t.Fatalf("settings changed: %+v", current)
	}
	subscriptions, _ = repo.GetScheduleSubscriptions(ctx, chat.ID)
	if len(subscriptions) != 2 || subscriptions[0].GroupName != "B" || subscriptions[1].GroupName != "C" {
		t.Fatalf("additional groups lost or duplicated: %v", subscriptions)
	}
	if err := db.Create(&model.ScheduleSubscription{ChatID: chat.ID, GroupName: "B"}).Error; err == nil {
		t.Fatal("database accepted a duplicate subscription")
	}
	if removed, err := repo.RemoveScheduleSubscription(ctx, chat.ID, subscriptions[1].ID); err != nil || !removed {
		t.Fatalf("remove: %v, %v", removed, err)
	}
	if removed, err := repo.RemoveScheduleSubscription(ctx, chat.ID, subscriptions[1].ID); err != nil || removed {
		t.Fatalf("repeated remove: %v, %v", removed, err)
	}
}

func TestScheduleSubscriptionsRemovedGroupAndStop(t *testing.T) {
	repo, db, chat := subscriptionRepository(t)
	ctx := context.Background()
	if _, err := repo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "A"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RemoveUnavailableGroup(ctx, chat.ID, "A"); err != nil {
		t.Fatal(err)
	}
	current, _ := repo.GetChat(ctx, chat.ID)
	if current.GroupName != nil || current.PairSending || current.ChangeAlert || current.DailySendingTime == nil {
		t.Fatalf("removed primary disabled other subscriptions: %+v", current)
	}
	recipients, err := repo.GetDailyRecipients(ctx, "08:00")
	if err != nil || len(recipients) != 1 || recipients[0].Subscription.GroupName != "B" {
		t.Fatalf("daily without primary: %v, %v", recipients, err)
	}
	if _, err := repo.RemoveUnavailableGroup(ctx, chat.ID, "B"); err != nil {
		t.Fatal(err)
	}
	current, _ = repo.GetChat(ctx, chat.ID)
	if current.DailySendingTime != nil {
		t.Fatal("empty daily broadcast remains enabled")
	}
	if _, err := repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "C"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteChat(ctx, chat.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&model.ScheduleSubscription{}).Where("chat_id = ?", chat.ID).Count(&count)
	if count != 0 {
		t.Fatal("/stop left subscriptions in the database")
	}
	if _, err := repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "D"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("subscription revived deleted chat: %v", err)
	}
}

func TestScheduleSubscriptionsPlatformAndOwnerIsolation(t *testing.T) {
	repo, db, chat := subscriptionRepository(t)
	ctx := context.Background()
	otherRepo := NewChatRepository(db, model.PlatformVK)
	other := &model.Chat{PeerID: chat.PeerID, DailySendingTime: chat.DailySendingTime}
	if err := otherRepo.CreateChat(ctx, other); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		repo ChatRepository
		chat *model.Chat
	}{{repo, chat}, {otherRepo, other}} {
		if _, err := item.repo.AddScheduleSubscription(ctx, item.chat.ID, model.Group{GroupName: "A"}); err != nil {
			t.Fatal(err)
		}
	}
	recipients, err := repo.GetDailyRecipients(ctx, "08:00")
	if err != nil || len(recipients) != 1 || recipients[0].Chat.ID != chat.ID {
		t.Fatalf("platform leak: %v, %v", recipients, err)
	}
	if got, err := repo.GetDailyRecipients(ctx, "09:00"); err != nil || len(got) != 0 {
		t.Fatalf("wrong time: %v, %v", got, err)
	}
	if got, err := otherRepo.GetScheduleSubscriptions(ctx, chat.ID); err != nil || len(got) != 0 {
		t.Fatalf("platform leak: %v, %v", got, err)
	}
	if _, err := otherRepo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "B"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("changed another platform's chat: %v", err)
	}
	subs, _ := otherRepo.GetScheduleSubscriptions(ctx, other.ID)
	if removed, err := repo.RemoveScheduleSubscription(ctx, chat.ID, subs[0].ID); err != nil || removed {
		t.Fatalf("deleted someone else's subscription: %v, %v", removed, err)
	}
}

func TestScheduleSubscriptionsConcurrentAddAndStop(t *testing.T) {
	repo, db, chat := subscriptionRepository(t)
	ctx := context.Background()
	var workers sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		workers.Go(func() {
			<-start
			_, err := repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "A"})
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Errorf("add during stop: %v", err)
			}
		})
	}
	workers.Go(func() {
		<-start
		if err := repo.DeleteChat(ctx, chat.ID); err != nil {
			t.Errorf("stop: %v", err)
		}
	})
	close(start)
	workers.Wait()
	var count int64
	if err := db.Model(&model.ScheduleSubscription{}).Where("chat_id = ?", chat.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("orphan subscriptions: %d, %v", count, err)
	}
}
