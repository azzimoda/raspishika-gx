package botutil

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot/models"
)

const testLinkURL = "https://coworking.tyuiu.ru/shs/all_t/sh.php"

func TestUpdateScheduleMarkupWithLink(t *testing.T) {
	markup := UpdateScheduleMarkup(UpdateKindToday, "ИСПт-22-(9)-2", testLinkURL)

	row := markup.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("want 2 buttons (link + update), got %d", len(row))
	}
	if row[0].Text != ScheduleLinkLabel || row[0].URL != testLinkURL {
		t.Fatalf("link button = %+v, want Text=%q URL=%q", row[0], ScheduleLinkLabel, testLinkURL)
	}
	if row[0].CallbackData != "" {
		t.Fatalf("link button must not have callback data, got %q", row[0].CallbackData)
	}
	if row[1].Text != "Обновить" || row[1].URL != "" || row[1].CallbackData == "" {
		t.Fatalf("update button = %+v, want Text=%q with callback data", row[1], "Обновить")
	}
}

func TestUpdateScheduleMarkupWithoutLink(t *testing.T) {
	markup := UpdateScheduleMarkup(UpdateKindToday, "ИСПт-22-(9)-2", "")

	row := markup.InlineKeyboard[0]
	if len(row) != 1 {
		t.Fatalf("want 1 button (update only), got %d", len(row))
	}
	if row[0].Text != "Обновить" {
		t.Fatalf("button = %+v, want Text=%q", row[0], "Обновить")
	}
}

func TestLinkOnlyMarkup(t *testing.T) {
	markup := LinkOnlyMarkup(testLinkURL)

	row := markup.InlineKeyboard[0]
	if len(row) != 1 {
		t.Fatalf("want 1 button, got %d", len(row))
	}
	if row[0].Text != ScheduleLinkLabel || row[0].URL != testLinkURL {
		t.Fatalf("button = %+v, want Text=%q URL=%q", row[0], ScheduleLinkLabel, testLinkURL)
	}
}

func TestWeekScheduleMarkup(t *testing.T) {

	groupConf := model.ScheduleConfig{
		Group: &model.Group{GroupID: "205", GroupName: "ИСПт-22-(9)-2", DepartmentID: "15", Year: 2026},
	}
	markup := WeekScheduleMarkup(groupConf, testLinkURL)

	teacherConf := model.ScheduleConfig{
		Teacher: &model.Teacher{TeacherID: "205", Name: "Иванов Иван Иванович"},
	}
	teacherMarkup := WeekScheduleMarkup(teacherConf, testLinkURL)

	for _, tt := range []struct {
		name  string
		ikm   models.InlineKeyboardMarkup
		value string
	}{
		{"group", markup, "ИСПт-22-(9)-2"},
		{"teacher", teacherMarkup, "205"},
	} {
		row := tt.ikm.InlineKeyboard[0]
		update := row[len(row)-1]
		want := fmt.Sprintf("update_week\n%s\n", tt.value)
		if !strings.HasPrefix(update.CallbackData, want) {
			t.Errorf("%s: update callback = %q, want prefix %q", tt.name, update.CallbackData, want)
		}
	}
}

func TestWeekScheduleMarkupNoConfig(t *testing.T) {
	markup := WeekScheduleMarkup(model.ScheduleConfig{}, testLinkURL)
	if markup.InlineKeyboard != nil {
		t.Fatalf("want empty keyboard for config without group or teacher, got %+v", markup.InlineKeyboard)
	}
}

func TestUpdateKindCallbackCommand(t *testing.T) {
	for _, tt := range []struct {
		kind UpdateKind
		want string
	}{
		{UpdateKindWeek, CallbackCommandUpdateWeek},
		{UpdateKindToday, CallbackCommandUpdateToday},
		{UpdateKindTomorrow, CallbackCommandUpdateTomorrow},
	} {
		if got := tt.kind.CallbackCommand(); got != tt.want {
			t.Errorf("%s: CallbackCommand() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestCallbackCommandRoundTrip(t *testing.T) {
	cmd := NewCallbackCommand("update_week", "ИСПт-22-(9)-2", "20260829120000")
	parsed := ParseCallbackData(cmd.String())
	if parsed.Command != cmd.Command {
		t.Fatalf("parsed command %q, want %q", parsed.Command, cmd.Command)
	}
	if len(parsed.Args) != len(cmd.Args) {
		t.Fatalf("parsed args %v, want %v", parsed.Args, cmd.Args)
	}
	for i := range cmd.Args {
		if parsed.Args[i] != cmd.Args[i] {
			t.Fatalf("parsed arg %d = %q, want %q", i, parsed.Args[i], cmd.Args[i])
		}
	}
}

func TestUpdateInlineButtonCallbackData(t *testing.T) {
	btn := UpdateInlineButton(UpdateKindToday, "ИСПт-22-(9)-2")
	parsed := ParseCallbackData(btn.CallbackData)
	wantCmd := CallbackCommandUpdateToday
	if parsed.Command != wantCmd {
		t.Errorf("command = %q, want %q", parsed.Command, wantCmd)
	}
	if parsed.Arg(0) != "ИСПт-22-(9)-2" {
		t.Errorf("arg0 = %q, want group name", parsed.Arg(0))
	}
	if parsed.Arg(1) == "" {
		t.Errorf("arg1 (timestamp) must be present, got %q", parsed.Arg(1))
	}
}

func Test_firstSunday(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		m    time.Month
		y    int
		want time.Time
	}{
		{"2026-01>4", 1, 2026, time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)},
		{"2026-02>1", 2, 2026, time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)},
		{"2026-03>1", 3, 2026, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)},
		{"2026-04>5", 4, 2026, time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC)},
		{"2026-05>3", 5, 2026, time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)},
		{"2026-06>7", 6, 2026, time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)},
		{"2026-07>5", 7, 2026, time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)},
		{"2026-08>2", 8, 2026, time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)},
		{"2026-09>6", 9, 2026, time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)},
		{"2026-10>4", 10, 2026, time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)},
		{"2026-11>1", 11, 2026, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{"2026-12>6", 12, 2026, time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstSunday(tt.m, tt.y)
			if got != tt.want {
				t.Errorf("firstSunday() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsVacation(t *testing.T) {
	tests := []struct {
		// Named input parameters for target function.
		t    time.Time
		want bool
	}{
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC), true}, // First Sunday
		{time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), true},

		{time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), true},
		{time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), true},

		{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC), false},

		{time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), false},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.t.Format(time.DateOnly), func(t *testing.T) {
			got := IsVacation(tt.t)
			if got != tt.want {
				t.Errorf("IsVacation() = %v, want %v", got, tt.want)
			}
		})
	}
}
