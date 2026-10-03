package vkbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/reporter"
	"github.com/azzimoda/raspishika-gx/internal/vkbot/client"
	vkbotutil "github.com/azzimoda/raspishika-gx/internal/vkbot/util"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

type sentMessage struct {
	peerID   int64
	text     string
	keyboard *vkbotutil.Keyboard
}

type fakeMessenger struct {
	messages    []sentMessage
	photos      int
	photoErr    error
	adminErr    error
	adminChecks int
	admins      map[int64]bool
}

func (f *fakeMessenger) SendMessage(_ context.Context, peerID int64, text string, keyboard *vkbotutil.Keyboard) (int, error) {
	f.messages = append(f.messages, sentMessage{peerID, text, keyboard})
	return 0, nil
}

func (f *fakeMessenger) SendPhoto(_ context.Context, _ int64, _ string, _ []byte, _ string, _ *vkbotutil.Keyboard) (int, error) {
	f.photos++
	return 0, f.photoErr
}

func (f *fakeMessenger) IsAdmin(_ context.Context, _, userID int64) (bool, error) {
	f.adminChecks++
	return f.admins[userID], f.adminErr
}

type fakeChats struct {
	records                   map[model.ChatID]*model.Chat
	updates, creates, deletes int
	resets                    int
	recent                    []*model.RecentTeacher
	createErr                 error
}

func (f *fakeChats) ResetGroupSettings(_ context.Context, chat *model.Chat) error {
	f.resets++
	f.updates++
	chat.GroupName = nil
	chat.DepartmentName = nil
	chat.PairSending = false
	chat.ChangeAlert = false
	chat.DailySendingTime = nil
	chat.State = model.ChatStateDefault
	if stored, ok := f.records[model.ChatID(chat.ID)]; ok {
		*stored = *chat
	}
	return nil
}

func (f *fakeChats) GetChatByChatID(_ context.Context, id model.ChatID) (*model.Chat, error) {
	chat, ok := f.records[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	clone := *chat
	return &clone, nil
}

func (f *fakeChats) CreateChat(_ context.Context, chat *model.Chat) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.creates++
	chat.ID = int64(100 + f.creates)
	copy := *chat
	f.records[chat.PeerID] = &copy
	return nil
}

func (f *fakeChats) UpdateChat(_ context.Context, chat *model.Chat) error {
	f.updates++
	copy := *chat
	f.records[chat.PeerID] = &copy
	return nil
}

func (f *fakeChats) DeleteChat(_ context.Context, id int64) error {
	f.deletes++
	for peer, chat := range f.records {
		if chat.ID == id {
			delete(f.records, peer)
		}
	}
	f.recent = nil
	return nil
}

func (f *fakeChats) GetRecentTeachers(context.Context, int64) ([]*model.RecentTeacher, error) {
	return f.recent, nil
}

func (f *fakeChats) AddChatRecentTeacher(_ context.Context, teacher *model.RecentTeacher) error {
	f.recent = append(f.recent, teacher)
	return nil
}

func (f *fakeChats) CountAllChats(context.Context) (int, error) {
	return len(f.records), nil
}

type fakeSchedules struct {
	group       *model.Group
	departments []model.Department
	groups      []model.Group
	teachers    []model.Teacher
	schedule    *model.ScheduleData
	imageErr    error
	err         error
	requested   model.ScheduleConfig
	lookups     int
	vacation    bool
	vacationErr error
}

func (f *fakeSchedules) IsVacation(context.Context) (bool, error) {
	return f.vacation, f.vacationErr
}

func (f *fakeSchedules) GetDepartments(context.Context) ([]model.Department, error) {
	return f.departments, nil
}

func (f *fakeSchedules) GetGroupsByDepartmentName(context.Context, string) ([]model.Group, error) {
	return f.groups, nil
}

func (f *fakeSchedules) GetGroupByName(context.Context, model.GroupName) (*model.Group, error) {
	f.lookups++
	return f.group, nil
}

func (f *fakeSchedules) GetTeacherByNameOrID(_ context.Context, id string) (*model.Teacher, error) {
	for _, teacher := range f.teachers {
		if teacher.TeacherID == id {
			return &teacher, nil
		}
	}
	return nil, nil
}

func (f *fakeSchedules) FindTeachersByName(context.Context, string) ([]model.Teacher, error) {
	return f.teachers, nil
}

func (f *fakeSchedules) GetSchedule(_ context.Context, config model.ScheduleConfig) (*model.ScheduleData, error) {
	f.requested = config
	if f.err != nil {
		return nil, f.err
	}
	if f.schedule == nil {
		return nil, nil
	}
	copy := *f.schedule
	return &copy, nil
}

func (f *fakeSchedules) PrepareScheduleImage(context.Context, *model.ScheduleData) (string, []byte, error) {
	return "schedule.png", []byte("png"), f.imageErr
}

type fakeStats struct {
	entries []model.UpdateLog
	err     error
}

func (f *fakeStats) LogUpdate(_ context.Context, entry model.UpdateLog) error {
	f.entries = append(f.entries, entry)
	return f.err
}

type stubReporter struct{}

func (stubReporter) Report() reporter.ReportBuilder { return reporter.EmptyReportBuilder() }

func testBot() (*Bot, *fakeMessenger, *fakeChats, *fakeSchedules) {
	m := &fakeMessenger{admins: map[int64]bool{10: true}}
	c := &fakeChats{records: map[model.ChatID]*model.Chat{}}
	s := &fakeSchedules{group: &model.Group{GroupName: "ИСПт-22-(9)-2", DepartmentName: "СОНХ", DepartmentID: "1", GroupID: "2", Year: 2026}}
	b := &Bot{
		api:       m,
		chats:     c,
		schedules: s,
		now:       func() time.Time { return time.Date(2026, 9, 5, 12, 0, 0, 0, time.FixedZone("UTC+5", 5*3600)) },
		sessions:  make(map[sessionKey]session),
	}
	return b, m, c, s
}

func withStats(b *Bot) *fakeStats {
	st := &fakeStats{}
	b.stats = st
	return st
}

func incoming(peerID, userID int64, command string) vkclient.Message {
	return vkclient.Message{ID: 1, PeerID: peerID, FromID: userID, Text: command}
}

func addChat(c *fakeChats, peerID int64, access model.ChatAccessLevel) {
	group := model.GroupName("ИСПт-22-(9)-2")
	c.records[model.ChatID(peerID)] = &model.Chat{ID: peerID, PeerID: model.ChatID(peerID), GroupName: &group, Access: access}
}

func run(t *testing.T, b *Bot, msg vkclient.Message) {
	t.Helper()
	if err := b.Handle(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
}

func TestParseCommandsAndPayload(t *testing.T) {
	cases := []struct {
		msg          vkclient.Message
		command, arg string
		ok           bool
	}{
		{vkclient.Message{Text: "/week испт 22 9 2"}, "week", "испт 22 9 2", true},
		{vkclient.Message{Text: "[club42|Бот], /today"}, "today", "", true},
		{vkclient.Message{Text: "Завтра"}, "tomorrow", "", true},
		{vkclient.Message{Text: "подпись", Payload: `{"command":"groups\n123\n2"}`}, "groups", "123\n2", true},
		{vkclient.Message{Text: "подпись", Payload: `{"command":"day\nИСПт-22-(9)-2\n3"}`}, "day", "ИСПт-22-(9)-2\n3", true},
		{vkclient.Message{Text: "dark", Payload: `{broken`}, "", "", false},
		{vkclient.Message{Text: "Настройки"}, "settings", "", true},
		{vkclient.Message{Text: "преподаватель Иванов И.И."}, "teacher", "Иванов И.И.", true},
		{vkclient.Message{Text: "Завтра в 9 собираемся у главного корпуса"}, "", "", false},
		{vkclient.Message{Text: "Настройки хочу изменить"}, "", "", false},
		{vkclient.Message{Text: "вторник собрание"}, "", "", false},
	}
	for _, tc := range cases {
		command, arg, ok := parseCommand(tc.msg)
		if command != tc.command || arg != tc.arg || ok != tc.ok {
			t.Errorf("%+v => (%q,%q,%v)", tc.msg, command, arg, ok)
		}
	}
}

func TestConfigurationChecksCurrentVKPermissionBeforeMutation(t *testing.T) {
	for _, command := range []string{"/setgroup испт 22 9 2", "/daily 07:30", "/pair", "/change", "/dark", "/stop", "/setaccess 0"} {
		t.Run(command, func(t *testing.T) {
			b, m, c, s := testBot()
			addChat(c, 2000000001, model.ChatAccessConfigAdmin)
			run(t, b, incoming(2000000001, 11, command))
			if c.updates != 0 || c.deletes != 0 || s.lookups != 0 {
				t.Fatal("denied command changed state or performed lookup")
			}
			if m.adminChecks == 0 || len(m.messages) != 1 {
				t.Fatal("permission was not checked and explained")
			}
		})
	}
	// Even the open-access mode never delegates ownership of the access menu.
	b, m, c, _ := testBot()
	addChat(c, 2000000001, model.ChatAccessAll)
	run(t, b, incoming(2000000001, 11, "/setaccess 2"))
	if c.updates != 0 || m.adminChecks != 1 {
		t.Fatal("non-admin changed access")
	}
}

func TestFailedPermissionLookupFailsClosed(t *testing.T) {
	b, m, c, _ := testBot()
	addChat(c, 2000000001, model.ChatAccessConfigAdmin)
	m.adminErr = errors.New("VK unavailable")
	run(t, b, incoming(2000000001, 10, "/dark"))
	if c.updates != 0 || c.records[2000000001].DarkMode {
		t.Fatal("setting changed without verified admin")
	}
	if !strings.Contains(m.messages[0].text, "проверить права") {
		t.Fatal("missing verification failure explanation")
	}
}

func TestConversationChatterAndOutgoingMessagesAreIgnored(t *testing.T) {
	b, m, c, _ := testBot()
	addChat(c, 2000000001, model.ChatAccessAdminOnly)
	run(t, b, incoming(2000000001, 11, "Обсудим это после пары"))
	outgoing := incoming(2000000001, 10, "/start")
	outgoing.Out = true
	run(t, b, outgoing)
	run(t, b, incoming(2000000001, -42, "/start"))
	if c.updates != 0 || m.adminChecks != 0 || len(m.messages) != 0 {
		t.Fatal("bot reacted to unrelated/outgoing content")
	}
}

func TestNewConversationsDefaultToAdminConfiguration(t *testing.T) {
	b, _, c, _ := testBot()
	run(t, b, incoming(2000000005, 11, "/start"))
	chat := c.records[2000000005]
	if chat == nil || chat.Access != model.ChatAccessConfigAdmin || chat.IsPrivate() {
		t.Fatal("unsafe conversation defaults")
	}
}

func TestSessionsAreBoundToUserAndRecheckPermissions(t *testing.T) {
	b, m, c, _ := testBot()
	addChat(c, 2000000001, model.ChatAccessConfigAdmin)
	run(t, b, incoming(2000000001, 10, "/daily"))
	updates := c.updates
	run(t, b, incoming(2000000001, 11, "08:00"))
	if c.updates != updates || c.records[2000000001].DailySendingTime != nil {
		t.Fatal("another user consumed admin prompt")
	}
	m.admins[10] = false
	run(t, b, incoming(2000000001, 10, "08:00"))
	if c.records[2000000001].DailySendingTime != nil {
		t.Fatal("revoked admin consumed old prompt")
	}
	m.admins[10] = true
	run(t, b, incoming(2000000001, 10, "08:00"))
	if got := c.records[2000000001].DailySendingTime; got == nil || *got != "08:00" {
		t.Fatal("admin time not saved")
	}
}

func TestDailyTimeValidationAndDisable(t *testing.T) {
	for _, bad := range []string{"7:30", "24:00", "12:60", "00:0", "tomorrow"} {
		b, _, c, _ := testBot()
		addChat(c, 10, model.ChatAccessAll)
		run(t, b, incoming(10, 10, "/daily "+bad))
		if c.records[10].DailySendingTime != nil {
			t.Errorf("accepted %q", bad)
		}
	}
	b, _, c, _ := testBot()
	addChat(c, 10, model.ChatAccessAll)
	run(t, b, incoming(10, 10, "/daily 00:00"))
	if c.records[10].DailySendingTime == nil {
		t.Fatal("midnight rejected")
	}
	run(t, b, incoming(10, 10, "/daily off"))
	if c.records[10].DailySendingTime != nil {
		t.Fatal("daily not disabled")
	}
}

func TestPaginationEveryItemReachableWithinVKLimits(t *testing.T) {
	items := make([]vkbotutil.MenuItem, 31)
	for i := range items {
		items[i] = vkbotutil.MenuItem{Label: strings.Repeat("я", 50), Command: fmt.Sprintf("item\n%d", i)}
	}
	seen := map[string]bool{}
	for page := 0; page < (len(items)+vkbotutil.PageSize-1)/vkbotutil.PageSize; page++ {
		keyboard := vkbotutil.PagedKeyboard(items, page, "page", "settings")
		if !keyboard.Inline || len(keyboard.Buttons) > 6 {
			t.Fatal("too many inline rows")
		}
		count := 0
		for _, row := range keyboard.Buttons {
			if len(row) > 4 {
				t.Fatal("too many buttons in row")
			}
			for _, button := range row {
				count++
				if len([]rune(button.Action.Label)) > 40 || len(button.Action.Payload) > 255 {
					t.Fatal("VK label/payload limit exceeded")
				}
				var payload struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal([]byte(button.Action.Payload), &payload); err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(payload.Command, "item\n") {
					seen[payload.Command] = true
				}
			}
		}
		if count > 10 {
			t.Fatalf("page %d has %d inline buttons", page, count)
		}
	}
	if len(seen) != len(items) {
		t.Fatalf("only %d of %d items reachable", len(seen), len(items))
	}
}

func TestTodayUsesDateAndIncludesEverySubject(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 10, model.ChatAccessAll)
	s.schedule = &model.ScheduleData{Days: []model.ScheduleDay{
		{Date: "04.09.2026", Pairs: []model.Pair{{Kind: model.PairKindSubject, Discipline: "Вчера"}}},
		{Date: "05.09.2026", Weekday: "суббота", Pairs: []model.Pair{
			{Kind: model.PairKindSubject, Number: 1, StartTime: "08:00", EndTime: "09:30", Discipline: "Математика", Teacher: "<a href='https://t.me/ivanov'>Иванов И.И.</a>"},
			{Kind: model.PairKindSubject, Number: 2, StartTime: "09:40", EndTime: "11:10", Discipline: "Физика"},
		}},
	}}
	run(t, b, incoming(10, 10, "/today"))
	text := m.messages[len(m.messages)-1].text
	if !strings.Contains(text, "Математика") || !strings.Contains(text, "Физика") || strings.Contains(text, "Вчера") {
		t.Fatal(text)
	}
	if strings.Contains(text, "<a href") {
		t.Fatal("TG HTML leaked into VK text: " + text)
	}
	run(t, b, incoming(10, 10, "/tomorrow"))
	if !strings.Contains(m.messages[len(m.messages)-1].text, "Воскресенье") {
		t.Fatal("Sunday returned a different day's lessons")
	}
}

func TestEmptyAndShortSchedulesDoNotPanic(t *testing.T) {
	for _, days := range [][]model.ScheduleDay{nil, {{Date: "05.09.2026"}}} {
		b, _, c, s := testBot()
		addChat(c, 10, model.ChatAccessAll)
		s.schedule = &model.ScheduleData{Days: days}
		run(t, b, incoming(10, 10, "/tomorrow"))
	}
}

func TestPhotoFailureFallsBackToText(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 10, model.ChatAccessAll)
	m.photoErr = errors.New("upload unavailable")
	s.schedule = &model.ScheduleData{Days: []model.ScheduleDay{{Date: "05.09.2026", Pairs: []model.Pair{{Kind: model.PairKindSubject, Discipline: "Математика"}}}}}
	run(t, b, incoming(10, 10, "/week"))
	if m.photos != 1 || len(m.messages) != 1 || !strings.Contains(m.messages[0].text, "Математика") {
		t.Fatal("missing text fallback")
	}
}

func TestTeacherSelectionAndRecentHistory(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 10, model.ChatAccessAll)
	s.teachers = []model.Teacher{{TeacherID: "7", Name: "Иванов И.И."}, {TeacherID: "8", Name: "Иванова А.А."}}
	s.schedule = &model.ScheduleData{Days: []model.ScheduleDay{{Date: "05.09.2026"}}}
	run(t, b, incoming(10, 10, "/teacher Иван"))
	if m.messages[0].keyboard == nil {
		t.Fatal("teacher choices missing")
	}
	msg := incoming(10, 10, "Иванова А.А.")
	msg.Payload = `{"command":"showteacher\n8"}`
	run(t, b, msg)
	if s.requested.Teacher == nil || s.requested.Teacher.TeacherID != "8" || len(c.recent) != 1 || c.recent[0].TeacherID != "8" {
		t.Fatal("wrong teacher or missing recent history")
	}
}

func TestDayClickOpensDayNavigation(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 10, model.ChatAccessAll)
	s.group = &model.Group{GroupName: "ИСПт-22-(9)-2", DepartmentName: "СОНХ", DepartmentID: "1", GroupID: "2", Year: 2026}
	s.schedule = &model.ScheduleData{Days: []model.ScheduleDay{
		{Date: "07.09.2026", Weekday: "понедельник", Pairs: []model.Pair{{Kind: model.PairKindSubject, Discipline: "Информатика"}}},
		{Date: "08.09.2026", Weekday: "вторник", Pairs: []model.Pair{{Kind: model.PairKindSubject, Discipline: "Математика"}}},
	}}
	msg := incoming(10, 10, "")
	msg.Payload = `{"command":"day\nИСПт-22-(9)-2\n0"}`
	run(t, b, msg)
	last := m.messages[len(m.messages)-1]
	if !strings.Contains(last.text, "Информатика") || strings.Contains(last.text, "Математика") {
		t.Fatal("wrong day opened: " + last.text)
	}
	if last.keyboard == nil || !last.keyboard.Inline {
		t.Fatal("day view lacks inline navigation")
	}
}

func TestExplicitGroupConfigurationAndQuickLookupRemainDistinct(t *testing.T) {
	b, _, c, s := testBot()
	addChat(c, 10, model.ChatAccessAll)
	s.group = &model.Group{GroupName: "СЭЗт-23-(9)-1", DepartmentName: "Новое отделение", DepartmentID: "2", GroupID: "7", Year: 2026}
	s.schedule = &model.ScheduleData{Days: []model.ScheduleDay{{Date: "05.09.2026"}}}
	run(t, b, incoming(10, 10, "сэзт 23 9 1"))
	if got := *c.records[10].GroupName; got != "ИСПт-22-(9)-2" {
		t.Fatal("quick lookup overwrote configured group", got)
	}
	run(t, b, incoming(10, 10, "/setgroup сэзт 23 9 1"))
	if got := *c.records[10].GroupName; got != "СЭЗт-23-(9)-1" || c.records[10].DepartmentName == nil || *c.records[10].DepartmentName != "Новое отделение" {
		t.Fatal("explicit selection did not save canonical group/department")
	}
	run(t, b, incoming(10, 10, "/pair"))
	run(t, b, incoming(10, 10, "/change"))
	run(t, b, incoming(10, 10, "/dark"))
	if !c.records[10].PairSending || !c.records[10].ChangeAlert || !c.records[10].DarkMode {
		t.Fatal("settings toggles not saved")
	}
}

func TestStopRemovesSettingsAndSessions(t *testing.T) {
	b, m, c, _ := testBot()
	addChat(c, 10, model.ChatAccessAll)
	run(t, b, incoming(10, 10, "/daily"))
	run(t, b, incoming(10, 10, "/stop"))
	if c.records[10] != nil || b.getSession(incoming(10, 10, "")).kind != "" {
		t.Fatal("stop retained chat/session")
	}
	keyboard := m.messages[len(m.messages)-1].keyboard
	if keyboard == nil || len(keyboard.Buttons) != 0 {
		t.Fatal("stop did not clear keyboard")
	}
}

// TestExpiredStoredGroupResetsTheChat is the VK counterpart of the Telegram
// bot's resetChatForExpiredGroup. Without it the chat keeps a group that has
// left the schedule, so every later request repeats the same refusal and the
// broadcasts keep running against a dead group.
func TestExpiredStoredGroupResetsTheChat(t *testing.T) {
	cases := []struct {
		name string
		msg  vkclient.Message
	}{
		{"typed command", incoming(42, 10, "/week")},
		{"keyboard button", vkclient.Message{ID: 1, PeerID: 42, FromID: 10, Payload: `{"command":"day\nИСПт-22-(9)-2\n1"}`}},
		{"button without a group", vkclient.Message{ID: 1, PeerID: 42, FromID: 10, Payload: `{"command":"day"}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, m, c, s := testBot()
			addChat(c, 42, model.ChatAccessAll)
			s.group = nil // the group has left the schedule

			run(t, b, tc.msg)

			if c.resets != 1 {
				t.Fatalf("ResetGroupSettings calls = %d, want 1", c.resets)
			}
			chat, err := c.GetChatByChatID(context.Background(), 42)
			if err != nil {
				t.Fatalf("GetChatByChatID: %v", err)
			}
			if chat.GroupName != nil {
				t.Fatalf("stored group = %q, want it cleared", *chat.GroupName)
			}
			if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].text, "была удалена") {
				t.Fatalf("last message = %q, want the group-removed notice", m.messages[len(m.messages)-1].text)
			}
		})
	}
}

// TestUnknownTypedGroupKeepsSettings guards the other side: a name the user
// typed is not the chat's stored group, so a typo must not wipe their settings.
func TestUnknownTypedGroupKeepsSettings(t *testing.T) {
	b, _, c, s := testBot()
	addChat(c, 42, model.ChatAccessAll)
	s.group = nil

	run(t, b, incoming(42, 10, "/week АиЭС-22(9)-1"))

	if c.resets != 0 {
		t.Fatalf("ResetGroupSettings calls = %d, want 0 for a name the user typed", c.resets)
	}
	chat, err := c.GetChatByChatID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetChatByChatID: %v", err)
	}
	if chat.GroupName == nil {
		t.Fatal("stored group was cleared for an unknown name the user typed")
	}
}

// TestSettingsMenuAnswersVacation covers HANDLE_VACATION parity: during the
// holidays the Telegram bot answers with the vacation notice instead of the
// menu, and so must the VK bot.
func TestSettingsMenuAnswersVacation(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 42, model.ChatAccessAll)
	s.vacation = true
	viper.Set(config.KeyHandleVacation, true)
	t.Cleanup(func() { viper.Set(config.KeyHandleVacation, true) })

	run(t, b, incoming(42, 10, "/settings"))

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}
	text := m.messages[0].text
	if !strings.Contains(text, "каникул") {
		t.Fatalf("message = %q, want the vacation notice", text)
	}
	if strings.Contains(text, "Настройки") {
		t.Fatalf("message = %q, want the settings menu replaced by the notice", text)
	}
	if strings.Contains(text, "-") && strings.Contains(text, "осталось -") {
		t.Fatalf("message = %q, want a non-negative day count", text)
	}
}

// TestSettingsMenuDuringVacationWhenFlagOff keeps the flag in charge: an
// operator who disables HANDLE_VACATION still gets the menu.
func TestSettingsMenuDuringVacationWhenFlagOff(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 42, model.ChatAccessAll)
	s.vacation = true
	viper.Set(config.KeyHandleVacation, false)
	t.Cleanup(func() { viper.Set(config.KeyHandleVacation, true) })

	run(t, b, incoming(42, 10, "/settings"))

	if len(m.messages) != 1 || !strings.Contains(m.messages[0].text, "Настройки") {
		t.Fatalf("messages = %+v, want the settings menu", m.messages)
	}
}

// TestTeacherSearchAnswersVacation mirrors the Telegram teacher command, which
// is not gated by HANDLE_VACATION.
func TestTeacherSearchAnswersVacation(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 42, model.ChatAccessAll)
	s.vacation = true
	viper.Set(config.KeyHandleVacation, false)
	t.Cleanup(func() { viper.Set(config.KeyHandleVacation, true) })

	run(t, b, incoming(42, 10, "/teacher Иванов"))

	if len(m.messages) != 1 || !strings.Contains(m.messages[0].text, "каникул") {
		t.Fatalf("messages = %+v, want the vacation notice", m.messages)
	}
	if s.lookups != 0 {
		t.Fatalf("teacher lookups = %d, want 0 during vacation", s.lookups)
	}
}

// TestVacationCheckFailureDoesNotLockOut keeps a transient API error from
// locking users out of their settings: the Telegram bot logs it and carries on.
func TestVacationCheckFailureDoesNotLockOut(t *testing.T) {
	b, m, c, s := testBot()
	addChat(c, 42, model.ChatAccessAll)
	s.vacationErr = errors.New("boom")
	viper.Set(config.KeyHandleVacation, true)
	t.Cleanup(func() { viper.Set(config.KeyHandleVacation, true) })

	run(t, b, incoming(42, 10, "/settings"))

	if len(m.messages) != 1 || !strings.Contains(m.messages[0].text, "Настройки") {
		t.Fatalf("messages = %+v, want the settings menu despite the failed check", m.messages)
	}
}

func TestDaysUntilSeptember(t *testing.T) {
	cases := []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), 58},
		{time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), 1},
		{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), 0},
		// Past the end of the holidays the count must not go negative.
		{time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), 0},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), 0},
	}
	for _, tc := range cases {
		if got := daysUntilSeptember(tc.now); got != tc.want {
			t.Errorf("daysUntilSeptember(%s) = %d, want %d", tc.now, got, tc.want)
		}
	}
}

func TestReportableFiltersBenignErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil is not reportable", nil, false},
		{"not found is not reportable", apiclient.ErrNotFound, false},
		{"service unavailable is not reportable", apiclient.ErrServiceUnavailable, false},
		{"other error is reportable", errors.New("boom"), true},
		{"wrapped not found is not reportable", fmt.Errorf("wrap: %w", apiclient.ErrNotFound), false},
		{"wrapped other is reportable", fmt.Errorf("wrap: %w", errors.New("boom")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reportable(tc.err); got != tc.want {
				t.Errorf("reportable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestStopCommandReportsAndDeletesChat(t *testing.T) {
	b, m, c, _ := testBot()
	withStats(b)
	chatID := model.ChatID(2000000001)
	group := model.GroupName("ИСПт-22-(9)-2")
	c.records[chatID] = &model.Chat{ID: 1, PeerID: chatID, GroupName: &group, Access: model.ChatAccessAll}

	run(t, b, incoming(int64(chatID), 10, "/stop"))

	if c.deletes != 1 {
		t.Fatalf("deletes = %d, want 1", c.deletes)
	}
	found := false
	for _, msg := range m.messages {
		if msg.peerID == int64(chatID) && strings.Contains(msg.text, "Рассылки остановлены") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("stop confirmation not sent, messages = %+v", m.messages)
	}
}
