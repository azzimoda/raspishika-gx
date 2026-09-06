package mainbot

import (
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestRussianWeekday(t *testing.T) {
	want := map[time.Weekday]string{
		time.Sunday:    "воскресенье",
		time.Monday:    "понедельник",
		time.Tuesday:   "вторник",
		time.Wednesday: "среда",
		time.Thursday:  "четверг",
		time.Friday:    "пятница",
		time.Saturday:  "суббота",
	}
	for wd, name := range want {
		if got := model.RussianWeekday(wd); got != name {
			t.Errorf("RussianWeekday(%v) = %q, want %q", wd, got, name)
		}
	}
}

func TestDayMarker(t *testing.T) {
	now := time.Now()
	today := now.Format("02.01.2006")
	tomorrow := now.AddDate(0, 0, 1).Format("02.01.2006")
	yesterday := now.AddDate(0, 0, -1).Format("02.01.2006")

	tests := []struct {
		name string
		day  model.ScheduleDay
		want string
	}{
		{"today", model.ScheduleDay{Date: today, Weekday: "понедельник"}, " (сегодня)"},
		{"today iso format", model.ScheduleDay{Date: now.Format("2006-01-02"), Weekday: "понедельник"}, " (сегодня)"},
		{"tomorrow", model.ScheduleDay{Date: tomorrow, Weekday: "вторник"}, " (завтра)"},
		{"other", model.ScheduleDay{Date: yesterday, Weekday: "среда"}, ""},
		{"unparseable date falls back to weekday", model.ScheduleDay{Date: "не-дата", Weekday: model.RussianWeekday(now.Weekday())}, " (сегодня)"},
		{"unparseable date falls back to weekday tomorrow", model.ScheduleDay{Date: "не-дата", Weekday: model.RussianWeekday(now.AddDate(0, 0, 1).Weekday())}, " (завтра)"},
		{"unparseable date other weekday", model.ScheduleDay{Date: "", Weekday: "какой-то-день"}, ""},
	}

	for _, tt := range tests {
		if got := dayMarker(tt.day); got != tt.want {
			t.Errorf("%s: dayMarker() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestFormatDayDate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"dotted", "01.09.2026", "01.09.2026"},
		{"iso", "2026-09-01", "01.09.2026"},
		{"single digit day", "2026-09-05", "05.09.2026"},
		{"unparseable falls back", "не-дата", "не-дата"},
		{"empty falls back", "", ""},
	}

	for _, tt := range tests {
		day := model.ScheduleDay{Date: tt.in}
		if got := formatDayDate(day); got != tt.want {
			t.Errorf("%s: formatDayDate(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestParseDayDate(t *testing.T) {
	want := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		in   string
		want time.Time
		ok   bool
	}{
		{"dotted", "01.09.2026", want, true},
		{"iso", "2026-09-01", want, true},
		{"empty", "", time.Time{}, false},
		{"garbage", "не-дата", time.Time{}, false},
	}

	for _, tt := range tests {
		got, ok := parseDayDate(tt.in)
		if ok != tt.ok {
			t.Errorf("%s: parseDayDate() ok = %v, want %v", tt.name, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		gy, gm, gd := got.Date()
		wy, wm, wd := tt.want.Date()
		if gy != wy || gm != wm || gd != wd {
			t.Errorf("%s: parseDayDate() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
