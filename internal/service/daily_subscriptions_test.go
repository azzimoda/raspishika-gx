package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/spf13/viper"
)

type dailyAPIStub struct {
	APIClient
	missing  model.GroupName
	failing  model.GroupName
	requests []string
}

func (a *dailyAPIStub) GetDepartments(context.Context) ([]model.Department, error) {
	return []model.Department{{Name: "Отделение"}}, nil
}

func (a *dailyAPIStub) GetGroup(_ context.Context, name string) (*model.Group, error) {
	if model.GroupName(name) == a.missing {
		return nil, apiclient.ErrNotFound
	}
	return &model.Group{GroupName: model.GroupName(name), DepartmentName: "Отделение"}, nil
}

func (a *dailyAPIStub) GetSchedule(_ context.Context, params *apiclient.GetScheduleParams) (*model.ScheduleData, error) {
	a.requests = append(a.requests, params.Group)
	if model.GroupName(params.Group) == a.failing {
		return nil, errors.New("расписание временно недоступно")
	}
	return &model.ScheduleData{Days: []model.ScheduleDay{{Date: "09.09.2026"}}}, nil
}

type dailyBrowserStub struct {
	renders     int
	afterRender func()
}

func (b *dailyBrowserStub) ScreenshotHTML(string) ([]byte, error) {
	b.renders++
	if b.afterRender != nil {
		b.afterRender()
	}
	return []byte("schedule-image"), nil
}

func dailyFixture(t *testing.T, groups ...model.GroupName) (*BroadcastService, repository.ChatRepository, *model.Chat, *broadcastMessengerStub, *broadcastLogsStub, *dailyAPIStub, *dailyBrowserStub) {
	t.Helper()
	db := openTestChatDB(t)
	repo := repository.NewChatRepository(db, model.PlatformTelegram)
	daily := "08:00"
	chat := &model.Chat{PeerID: 100, DailySendingTime: &daily, DarkMode: true}
	if err := repo.CreateChat(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	for i, name := range groups {
		group := model.Group{GroupName: name, DepartmentName: "Отделение"}
		if i == 0 {
			var err error
			chat, err = repo.SetPrimaryGroup(context.Background(), chat.ID, group)
			if err != nil {
				t.Fatal(err)
			}
		} else if _, err := repo.AddScheduleSubscription(context.Background(), chat.ID, group); err != nil {
			t.Fatal(err)
		}
	}
	template := filepath.Join(t.TempDir(), "schedule.html")
	if err := os.WriteFile(template, []byte("{{.}}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{config.KeyScheduleTemplateFile, config.KeyScheduleTemplateDarkFile, config.KeyScreenshotDir} {
		old := viper.Get(key)
		t.Cleanup(func() { viper.Set(key, old) })
		if key == config.KeyScreenshotDir {
			viper.Set(key, t.TempDir())
		} else {
			viper.Set(key, template)
		}
	}
	api, renderer := &dailyAPIStub{}, &dailyBrowserStub{}
	messenger, logs := &broadcastMessengerStub{}, &broadcastLogsStub{}
	schedules := NewScheduleService(api, nil, nil)
	schedules.browser = renderer
	s := NewBroadcastService(messenger, &Services{Chat: NewChatService(repo), Stats: NewStatsService(logs, repo), Schedule: schedules}, nil)
	t.Cleanup(func() { s.Stop(context.Background()) })
	return s, repo, chat, messenger, logs, api, renderer
}

func TestDailyBroadcastMultipleGroups(t *testing.T) {
	s, _, chat, messenger, logs, api, renderer := dailyFixture(t, "A", "B", "C")
	s.handleDailyBroadcast(context.Background(), time.Date(2026, 9, 9, 8, 0, 0, 0, time.Local))
	if len(messenger.deliveries) != 3 || len(logs.logs) != 3 || renderer.renders != 3 || len(api.requests) != 3 {
		t.Fatalf("deliveries=%d logs=%d renders=%d requests=%v", len(messenger.deliveries), len(logs.logs), renderer.renders, api.requests)
	}
	for i, group := range []string{"A", "B", "C"} {
		delivery := messenger.deliveries[i]
		if delivery.peerID != int64(chat.PeerID) || delivery.buttons == nil || delivery.buttons.Value != group || !strings.Contains(delivery.text, "<i>"+group+"</i>") {
			t.Fatalf("wrong group or buttons: %+v", delivery)
		}
		if string(logs.logs[i].Group) != group || logs.logs[i].ChatID != chat.ID || logs.logs[i].Error != nil {
			t.Fatalf("wrong delivery log: %+v", logs.logs[i])
		}
	}
}

func TestDailyBroadcastPartialFailureAndRemovedPrimary(t *testing.T) {
	s, repo, chat, messenger, _, api, _ := dailyFixture(t, "A", "B", "C")
	api.missing, api.failing = "A", "B"
	s.handleDailyBroadcast(context.Background(), time.Date(2026, 9, 9, 8, 0, 0, 0, time.Local))
	if len(messenger.deliveries) != 2 || messenger.deliveries[1].buttons.Value != "C" {
		t.Fatalf("other groups were not delivered: %+v", messenger.deliveries)
	}
	current, err := repo.GetChat(context.Background(), chat.ID)
	if err != nil || current.GroupName != nil || current.DailySendingTime == nil {
		t.Fatalf("daily settings lost: %+v, %v", current, err)
	}
	subs, _ := repo.GetScheduleSubscriptions(context.Background(), chat.ID)
	if len(subs) != 2 {
		t.Fatalf("transient failure removed subscription: %v", subs)
	}
}

func TestDailyBroadcastRechecksAfterRendering(t *testing.T) {
	for _, action := range []string{"remove", "remove_readd", "stop", "time", "off"} {
		t.Run(action, func(t *testing.T) {
			s, repo, chat, messenger, _, _, renderer := dailyFixture(t, "A", "B")
			ctx := context.Background()
			recipients, err := repo.GetDailyRecipients(ctx, "08:00")
			if err != nil {
				t.Fatal(err)
			}
			recipient := recipients[1]
			renderer.afterRender = func() {
				switch action {
				case "remove":
					_, err = repo.RemoveScheduleSubscription(ctx, chat.ID, recipient.Subscription.ID)
				case "remove_readd":
					_, err = repo.RemoveScheduleSubscription(ctx, chat.ID, recipient.Subscription.ID)
					if err == nil {
						_, err = repo.AddScheduleSubscription(ctx, chat.ID, model.Group{GroupName: "B"})
					}
				case "stop":
					err = repo.DeleteChat(ctx, chat.ID)
				case "time":
					value := "09:00"
					chat.DailySendingTime = &value
					err = repo.UpdateChat(ctx, chat)
				case "off":
					chat.DailySendingTime = nil
					err = repo.UpdateChat(ctx, chat)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			schedule := &model.ScheduleData{Config: model.GroupScheduleConfig(&model.Group{GroupName: "B"}, false)}
			if err := s.sendDaily(ctx, 1, []*model.ScheduleData{schedule}, map[model.GroupName][]model.DailyRecipient{"B": {recipient}}); err != nil {
				t.Fatal(err)
			}
			if len(messenger.deliveries) != 0 {
				t.Fatal("sent a canceled subscription")
			}
		})
	}
}

func TestDailyBroadcastReusesImage(t *testing.T) {
	s, repo, _, messenger, _, api, renderer := dailyFixture(t, "A")
	daily := "08:00"
	other := &model.Chat{PeerID: 200, DailySendingTime: &daily, DarkMode: true}
	ctx := context.Background()
	if err := repo.CreateChat(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetPrimaryGroup(ctx, other.ID, model.Group{GroupName: "A"}); err != nil {
		t.Fatal(err)
	}
	s.handleDailyBroadcast(ctx, time.Date(2026, 9, 9, 8, 0, 0, 0, time.Local))
	if len(messenger.deliveries) != 2 || renderer.renders != 1 || len(api.requests) != 1 {
		t.Fatalf("deliveries=%d renders=%d requests=%v", len(messenger.deliveries), renderer.renders, api.requests)
	}
}

func TestDailyBroadcastForbiddenStopsAllGroups(t *testing.T) {
	s, repo, chat, messenger, logs, _, _ := dailyFixture(t, "A", "B")
	messenger.sendError = errTestForbidden
	s.handleDailyBroadcast(context.Background(), time.Date(2026, 9, 9, 8, 0, 0, 0, time.Local))
	current, err := repo.GetChat(context.Background(), chat.ID)
	if err != nil || current.DailySendingTime != nil || len(messenger.deliveries) != 1 || len(logs.logs) != 1 {
		t.Fatalf("forbidden: %+v, %v, sends=%d logs=%d", current, err, len(messenger.deliveries), len(logs.logs))
	}
	subs, err := repo.GetScheduleSubscriptions(context.Background(), chat.ID)
	if err != nil || len(subs) != 2 {
		t.Fatalf("selected groups lost: %v, %v", subs, err)
	}
}

func TestDailyBroadcastAdditionalGroupSurvivesPrimaryChange(t *testing.T) {
	s, repo, chat, messenger, _, _, _ := dailyFixture(t, "A", "B")
	ctx := context.Background()
	recipients, err := repo.GetDailyRecipients(ctx, "08:00")
	if err != nil {
		t.Fatal(err)
	}
	grouped, confs := s.prepareDailyBroadcast(ctx, recipients)
	schedules, err := s.Schedule.GetSchedules(ctx, confs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "C"}); err != nil {
		t.Fatal(err)
	}
	if err := s.sendDaily(ctx, 1, schedules, grouped); err != nil {
		t.Fatal(err)
	}
	if len(messenger.deliveries) != 1 || messenger.deliveries[0].buttons.Value != "B" {
		t.Fatalf("wrong deliveries after primary change: %v", messenger.deliveries)
	}
}
