package fakescraper

import (
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestWithCurrentDates(t *testing.T) {
	template := model.ScheduleData{Days: []model.ScheduleDay{{}, {}, {}, {}, {}, {}}}

	schedule := withCurrentDates(template)

	if len(schedule.Days) != len(template.Days) {
		t.Fatalf("withCurrentDates() days = %d, want %d", len(schedule.Days), len(template.Days))
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if today.Weekday() == time.Sunday {
		today = today.AddDate(0, 0, 1)
	}

	for i, day := range schedule.Days {
		wantDate := today
		for step := 0; step < i; {
			wantDate = wantDate.AddDate(0, 0, 1)
			if wantDate.Weekday() != time.Sunday {
				step++
			}
		}

		if day.Date != wantDate.Format("2006-01-02") {
			t.Errorf("days[%d].Date = %q, want %q", i, day.Date, wantDate.Format("2006-01-02"))
		}
		if day.Weekday != model.RussianWeekday(wantDate.Weekday()) {
			t.Errorf("days[%d].Weekday = %q, want %q", i, day.Weekday, model.RussianWeekday(wantDate.Weekday()))
		}
		if wantDate.Weekday() == time.Sunday {
			t.Errorf("days[%d] falls on Sunday", i)
		}
	}
}

func TestFakeSchedule_FirstDayIsToday(t *testing.T) {
	for key := range FakeSchedules {
		schedule, ok := FakeSchedule(key)
		if !ok {
			t.Fatalf("FakeSchedule(%q) not found", key)
		}
		if len(schedule.Days) == 0 {
			t.Fatalf("FakeSchedule(%q) has no days", key)
		}

		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if today.Weekday() == time.Sunday {
			today = today.AddDate(0, 0, 1)
		}
		if schedule.Days[0].Date != today.Format("2006-01-02") {
			t.Errorf("FakeSchedule(%q) first day = %q, want today %q",
				key, schedule.Days[0].Date, today.Format("2006-01-02"))
		}
	}
}
