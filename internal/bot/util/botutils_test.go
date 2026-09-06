package botutil

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const testLinkURL = "https://coworking.tyuiu.ru/shs/all_t/sh.php"

func TestSimpleUpdateMarkupWithLink(t *testing.T) {
	markup := SimpleUpdateMarkup(UpdateKindToday, "ИСПт-22-(9)-2", testLinkURL)

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

func TestSimpleUpdateMarkupWithoutLink(t *testing.T) {
	markup := SimpleUpdateMarkup(UpdateKindToday, "ИСПт-22-(9)-2", "")

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

	days := []model.ScheduleDay{
		{Date: "2026-09-01", Weekday: "вторник"},
		{Date: "2026-09-02", Weekday: "среда"},
		{Date: "2026-09-03", Weekday: "четверг"},
		{Date: "2026-09-04", Weekday: "пятница"},
		{Date: "2026-09-05", Weekday: "суббота"},
	}

	groupConf := model.ScheduleConfig{
		Group: &model.Group{GroupID: "205", GroupName: "ИСПт-22-(9)-2", DepartmentID: "15", Year: 2026},
	}
	markup := WeekScheduleMarkup(groupConf, testLinkURL, days)

	teacherConf := model.ScheduleConfig{
		Teacher: &model.Teacher{TeacherID: "205", Name: "Иванов Иван Иванович"},
	}
	teacherMarkup := WeekScheduleMarkup(teacherConf, testLinkURL, days)

	// Keyboard has one row for the day buttons (up to 6 per row) plus the
	// bottom link/update row.
	dayRows := (len(days) + 5) / 6
	if len(markup.InlineKeyboard) != dayRows+1 {
		t.Errorf("group keyboard rows = %d, want %d", len(markup.InlineKeyboard), dayRows+1)
	}

	for _, tt := range []struct {
		name  string
		ikm   models.InlineKeyboardMarkup
		value string
	}{
		{"group", markup, "ИСПт-22-(9)-2"},
		{"teacher", teacherMarkup, "205"},
	} {
		bottom := tt.ikm.InlineKeyboard[len(tt.ikm.InlineKeyboard)-1]
		update := bottom[len(bottom)-1]
		want := fmt.Sprintf("update_week\n%s\n", tt.value)
		if !strings.HasPrefix(update.CallbackData, want) {
			t.Errorf("%s: update callback = %q, want prefix %q", tt.name, update.CallbackData, want)
		}

		// First day button should carry the day index 0 and the value.
		firstDay := tt.ikm.InlineKeyboard[0][0]
		wantDay := fmt.Sprintf("update_day\n%s\n0\n", tt.value)
		if !strings.HasPrefix(firstDay.CallbackData, wantDay) {
			t.Errorf("%s: day callback = %q, want prefix %q", tt.name, firstDay.CallbackData, wantDay)
		}
	}
}

func TestWeekScheduleMarkupNoConfig(t *testing.T) {
	markup := WeekScheduleMarkup(model.ScheduleConfig{}, testLinkURL, nil)
	if markup.InlineKeyboard != nil {
		t.Fatalf("want empty keyboard for config without group or teacher, got %+v", markup.InlineKeyboard)
	}
}

func TestDayScheduleMarkup(t *testing.T) {
	days := []model.ScheduleDay{
		{Date: "2026-09-07", Weekday: "понедельник"},
		{Date: "2026-09-08", Weekday: "вторник"},
		{Date: "2026-09-09", Weekday: "среда"},
		{Date: "2026-09-10", Weekday: "четверг"},
		{Date: "2026-09-11", Weekday: "пятница"},
	}
	markup := DayScheduleMarkup("ИСПт-22-(9)-2", days, 2, testLinkURL)

	if len(markup.InlineKeyboard) != 3 {
		t.Fatalf("want 3 rows (days, back, bottom), got %d", len(markup.InlineKeyboard))
	}

	dayRow := markup.InlineKeyboard[0]
	if len(dayRow) != len(days) {
		t.Fatalf("day row = %d buttons, want %d", len(dayRow), len(days))
	}
	wantLabels := []string{"Пн", "Вт", "[Ср]", "Чт", "Пт"}
	for i, btn := range dayRow {
		if btn.Text != wantLabels[i] {
			t.Errorf("day %d button text = %q, want %q", i, btn.Text, wantLabels[i])
		}
		wantDay := fmt.Sprintf("update_day\nИСПт-22-(9)-2\n%d\n", i)
		if !strings.HasPrefix(btn.CallbackData, wantDay) {
			t.Errorf("day %d callback = %q, want prefix %q", i, btn.CallbackData, wantDay)
		}
	}

	back := markup.InlineKeyboard[1][0]
	if back.Text != "Неделя" || !strings.HasPrefix(back.CallbackData, "open_week\nИСПт-22-(9)-2\n") {
		t.Errorf("back button = %+v, want open_week", back)
	}
}

func TestDayScheduleMarkupCurrentDayBrackets(t *testing.T) {
	days := []model.ScheduleDay{
		{Date: "2026-09-07", Weekday: "понедельник"},
		{Date: "2026-09-08", Weekday: "вторник"},
		{Date: "2026-09-09", Weekday: "среда"},
	}
	for _, tt := range []struct {
		name string
		idx  int
		want []string
	}{
		{"first", 0, []string{"[Пн]", "Вт", "Ср"}},
		{"middle", 1, []string{"Пн", "[Вт]", "Ср"}},
		{"last", 2, []string{"Пн", "Вт", "[Ср]"}},
	} {
		markup := DayScheduleMarkup("205", days, tt.idx, "")
		if len(markup.InlineKeyboard) != 3 {
			t.Fatalf("%s: rows = %d, want 3", tt.name, len(markup.InlineKeyboard))
		}
		dayRow := markup.InlineKeyboard[0]
		if len(dayRow) != len(days) {
			t.Fatalf("%s: day row = %d buttons, want %d", tt.name, len(dayRow), len(days))
		}
		for i, btn := range dayRow {
			if btn.Text != tt.want[i] {
				t.Errorf("%s: day %d text = %q, want %q", tt.name, i, btn.Text, tt.want[i])
			}
		}
	}
}

func TestDayScheduleMarkupChunks(t *testing.T) {
	days := make([]model.ScheduleDay, 0, 8)
	weekdays := []string{"понедельник", "вторник", "среда", "четверг", "пятница", "суббота", "понедельник", "вторник"}
	for i, wd := range weekdays {
		days = append(days, model.ScheduleDay{Date: fmt.Sprintf("2026-09-%02d", 14+i), Weekday: wd})
	}

	markup := DayScheduleMarkup("205", days, 6, "")
	if len(markup.InlineKeyboard) != 4 {
		t.Fatalf("rows = %d, want 4 (2 day rows + back + bottom)", len(markup.InlineKeyboard))
	}
	first, second := markup.InlineKeyboard[0], markup.InlineKeyboard[1]
	if len(first) != 6 || len(second) != 2 {
		t.Fatalf("day rows = %d and %d buttons, want 6 and 2", len(first), len(second))
	}
	if second[0].Text != "[Пн]" {
		t.Errorf("chunked current day label = %q, want %q", second[0].Text, "[Пн]")
	}
	wantFirst := fmt.Sprintf("update_day\n205\n5\n")
	if !strings.HasPrefix(first[5].CallbackData, wantFirst) {
		t.Errorf("first chunk last callback = %q, want prefix %q", first[5].CallbackData, wantFirst)
	}
}

func TestWeekdayAbbr(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want string
	}{
		{"понедельник", "Пн"},
		{"Понедельник", "Пн"},
		{"ВТОРНИК", "Вт"},
		{"среда", "Ср"},
		{"Четверг", "Чт"},
		{"пятница", "Пт"},
		{"Суббота", "Сб"},
		{"воскресенье", "Вс"},
		{"", ""},
		{"unknown", "U"},
	} {
		if got := weekdayAbbr(tt.in); got != tt.want {
			t.Errorf("weekdayAbbr(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsMessageNotModified(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"not modified", fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: message is not modified"), true},
		{"not modified different case", fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: Message Is Not Modified"), true},
		{"other bad request", fmt.Errorf("%w, %s", bot.ErrorBadRequest, "Bad Request: chat not found"), false},
		{"network error", errors.New("proxy connect failed"), false},
		{"nil", nil, false},
	} {
		if got := IsMessageNotModified(tt.err); got != tt.want {
			t.Errorf("%s: IsMessageNotModified() = %v, want %v", tt.name, got, tt.want)
		}
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
		{UpdateKindDay, CallbackCommandUpdateDay},
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
