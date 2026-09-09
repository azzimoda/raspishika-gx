package mainbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/service"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/azzimoda/raspishika-gx/pkg/database"
	"github.com/azzimoda/raspishika-gx/pkg/testutil"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/spf13/viper"
)

type subscriptionTelegramStub struct {
	messages []string
	markups  []models.InlineKeyboardMarkup
}

func (s *subscriptionTelegramStub) Do(req *http.Request) (*http.Response, error) {
	method := path.Base(req.URL.Path)
	if err := req.ParseMultipartForm(1 << 20); err != nil && method != "getMe" {
		return nil, err
	}
	if req.MultipartForm != nil {
		defer req.MultipartForm.RemoveAll()
	}
	s.messages = append(s.messages, req.FormValue("text"))
	if raw := req.FormValue("reply_markup"); raw != "" {
		var markup models.InlineKeyboardMarkup
		if err := json.Unmarshal([]byte(raw), &markup); err != nil {
			return nil, err
		}
		s.markups = append(s.markups, markup)
	}
	result := `true`
	switch method {
	case "getMe":
		result = `{"id":999,"is_bot":true,"first_name":"Test","username":"test_bot"}`
	case "getChatMember":
		result = `{"status":"member","user":{"id":101,"first_name":"Test","is_bot":false}}`
	case "sendMessage", "editMessageText":
		result = `{"message_id":1,"date":1,"chat":{"id":101,"type":"private"}}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`)), Header: make(http.Header)}, nil
}

type subscriptionAPIStub struct{ service.APIClient }

func (subscriptionAPIStub) GetDepartments(context.Context) ([]model.Department, error) {
	return []model.Department{{Name: "Отделение"}}, nil
}
func (subscriptionAPIStub) GetGroups(context.Context, string) ([]model.Group, error) {
	return []model.Group{{GroupName: "ИСПт-22-(9)-1", DepartmentName: "Отделение"}}, nil
}
func (subscriptionAPIStub) GetGroup(_ context.Context, name string) (*model.Group, error) {
	name = strings.ReplaceAll(name, "испт", "ИСПт")
	normalized, err := model.GroupName(name).ValidateFormat()
	if err != nil {
		return nil, err
	}
	return &model.Group{GroupName: normalized, DepartmentName: "Отделение"}, nil
}

func TestSubscriptionSelectionAndRemovalThroughHandlers(t *testing.T) {
	root, err := testutil.FindProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(database.Config{File: filepath.Join(t.TempDir(), "bot.db"), MigrationsDir: filepath.Join(root, "migrations")})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	repo := repository.NewChatRepository(db, model.PlatformTelegram)
	chat := &model.Chat{PeerID: 101}
	ctx := context.Background()
	if err := repo.CreateChat(ctx, chat); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetPrimaryGroup(ctx, chat.ID, model.Group{GroupName: "ИСПт-22-(9)-2"}); err != nil {
		t.Fatal(err)
	}
	oldTTL := viper.Get(config.KeyChatStateTTL)
	viper.Set(config.KeyChatStateTTL, time.Hour)
	defer viper.Set(config.KeyChatStateTTL, oldTTL)
	client := &subscriptionTelegramStub{}
	b, err := bot.New("999:test", bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, client), bot.WithNotAsyncHandlers())
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{Services: &service.Services{Chat: service.NewChatService(repo), Schedule: service.NewScheduleService(subscriptionAPIStub{}, nil, nil)}}
	h.registerHandlers(b)
	process := func(data, text string) {
		t.Helper()
		current, err := repo.GetChat(ctx, chat.ID)
		if err != nil {
			t.Fatal(err)
		}
		message := &models.Message{ID: 1, Chat: models.Chat{ID: int64(current.PeerID), Type: models.ChatTypePrivate}, From: &models.User{ID: 101}, Text: text}
		update := &models.Update{Message: message}
		if data != "" {
			update = &models.Update{CallbackQuery: &models.CallbackQuery{ID: "test", Data: data, From: models.User{ID: 101}, Message: models.MaybeInaccessibleMessage{Message: message}}}
		}
		var handlerErrors []error
		callCtx := context.WithValue(context.WithValue(ctx, keyChat, current), keyError, &handlerErrors)
		b.ProcessUpdate(callCtx, update)
		if len(handlerErrors) != 0 {
			t.Fatalf("handler errors: %v", handlerErrors)
		}
	}
	process(botutil.CallbackCommandAddGroup, "")
	current, _ := repo.GetChat(ctx, chat.ID)
	if current.State != model.ChatStateAddingGroup {
		t.Fatalf("state = %s", current.State)
	}
	departmentButtonFound := false
	for _, markup := range client.markups {
		for _, row := range markup.InlineKeyboard {
			for _, button := range row {
				if button.CallbackData == botutil.CallbackCommandSubscriptionDepartment+"\nОтделение" {
					departmentButtonFound = true
				}
			}
		}
	}
	if !departmentButtonFound {
		t.Fatal("department menu lost the addition mode")
	}
	process(botutil.CallbackCommandSubscriptionDepartment+"\nОтделение", "")
	process("", "испт 22 9 1")
	current, _ = repo.GetChat(ctx, chat.ID)
	subs, _ := repo.GetScheduleSubscriptions(ctx, chat.ID)
	if len(subs) != 2 || *current.GroupName != "ИСПт-22-(9)-2" || current.State != model.ChatStateDefault {
		t.Fatalf("adding replaced primary or failed: %+v %v", current, subs)
	}
	process(botutil.CallbackCommandAddGroup, "")
	process("", "ИСПт-22-(9)-1")
	subs, _ = repo.GetScheduleSubscriptions(ctx, chat.ID)
	if len(subs) != 2 {
		t.Fatal("duplicate added")
	}
	process(botutil.CallbackCommandConfigDailyTime, "")
	process("", "08:00")
	current, _ = repo.GetChat(ctx, chat.ID)
	if current.DailySendingTime == nil || *current.DailySendingTime != "08:00" {
		t.Fatal("shared time not saved")
	}
	process(fmt.Sprintf("%s\n%d", botutil.CallbackCommandRemoveSubscription, subs[1].ID), "")
	subs, _ = repo.GetScheduleSubscriptions(ctx, chat.ID)
	if len(subs) != 1 || subs[0].GroupName != "ИСПт-22-(9)-2" {
		t.Fatalf("wrong subscription removed: %v", subs)
	}
	process(botutil.CallbackCommandAddGroup, "")
	process(botutil.CallbackCommandConfigSubscriptions, "")
	current, _ = repo.GetChat(ctx, chat.ID)
	if current.State != model.ChatStateDefault {
		t.Fatal("cancel left the chat in addition mode")
	}
	// Новые кнопки используют действующие ограничения настройки группового чата.
	current.PeerID = -101
	if err := db.Model(&model.Chat{}).Where("id = ?", chat.ID).Updates(map[string]any{"tg_chat_id": -101, "access": model.ChatAccessConfigAdmin}).Error; err != nil {
		t.Fatal(err)
	}
	process(botutil.CallbackCommandAddGroup, "")
	current, _ = repo.GetChat(ctx, chat.ID)
	if current.State != model.ChatStateDefault {
		t.Fatal("member changed restricted group settings")
	}
}

func TestSubscriptionMenuPaginationAndPrimaryProtection(t *testing.T) {
	primary := model.GroupName("Группа 0")
	chat := &model.Chat{GroupName: &primary}
	subs := make([]model.ScheduleSubscription, 10)
	for i := range subs {
		subs[i] = model.ScheduleSubscription{ID: int64(i + 1), GroupName: model.GroupName(fmt.Sprintf("Группа %d", i))}
	}
	text, markup := subscriptionMenu(chat, subs, 0)
	if !strings.Contains(text, "основная") || !strings.Contains(text, "Рассылка выключена") {
		t.Fatal(text)
	}
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if strings.HasPrefix(button.CallbackData, botutil.CallbackCommandRemoveSubscription+"\n1\n") {
				t.Fatal("primary is removable")
			}
			if len(button.CallbackData) > 64 {
				t.Fatal("callback data too long")
			}
		}
	}
	text, _ = subscriptionMenu(chat, subs, 999)
	if !strings.Contains(text, "Страница 2 из 2") || !strings.Contains(text, "Группа 9") || strings.Contains(text, "Группа 0") {
		t.Fatal(text)
	}
	text, _ = subscriptionMenu(chat, nil, -1)
	if !strings.Contains(text, "пока не выбраны") {
		t.Fatal(text)
	}
}
