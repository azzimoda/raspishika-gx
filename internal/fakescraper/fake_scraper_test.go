package fakescraper

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
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
		wantKind := weekKindForDate(wantDate)
		if day.WeekKind != wantKind {
			t.Errorf("days[%d].WeekKind = %q, want %q (ISO week %d)", i, day.WeekKind, wantKind, func() int {
				_, w := wantDate.ISOWeek()
				return w
			}())
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

// TestFakeFailuresAreSentinels covers the failure modes a bot has to react to
// differently. In particular ErrNotFound means "the group left the schedule" and
// makes a bot clear the user's settings, so a plain ad-hoc error there would
// leave a real group looking deleted; anything else must not be confused for it.
func TestFakeFailuresAreSentinels(t *testing.T) {
	api := ScraperAPI{}
	ctx := context.Background()

	t.Run("unknown group", func(t *testing.T) {
		_, err := api.GetGroup(ctx, "Нет-такой-22")
		if !errors.Is(err, apiclient.ErrNotFound) {
			t.Fatalf("GetGroup = %v, want apiclient.ErrNotFound", err)
		}
	})

	t.Run("unknown teacher", func(t *testing.T) {
		_, err := api.GetTeacher(ctx, "Нет-такого-преподавателя")
		if !errors.Is(err, apiclient.ErrNotFound) {
			t.Fatalf("GetTeacher = %v, want apiclient.ErrNotFound", err)
		}
	})

	t.Run("group without a schedule", func(t *testing.T) {
		_, err := api.GetSchedule(ctx, &apiclient.GetScheduleParams{Group: "Нет-такой-22"})
		if !errors.Is(err, apiclient.ErrServiceUnavailable) {
			t.Fatalf("GetSchedule = %v, want apiclient.ErrServiceUnavailable", err)
		}
		// A missing schedule is not a deleted group: resetting a chat on this
		// would throw away settings over a scrape miss.
		if errors.Is(err, apiclient.ErrNotFound) {
			t.Fatalf("GetSchedule reported ErrNotFound for a missing schedule")
		}
	})

	t.Run("unknown department", func(t *testing.T) {
		scraper := FakeScraper{}
		_, err := scraper.ScrapeDepartmentGroups(&model.Department{Name: "Нет-такой"})
		if !errors.Is(err, ErrNoDepartment) {
			t.Fatalf("ScrapeDepartmentGroups = %v, want ErrNoDepartment", err)
		}
	})
}

// TestFakeScheduleQueryWithoutGroupOrTeacher keeps a malformed request from
// taking the process down. Both entry points used to panic, which in the demo
// stack means one bad request restarts the container, and in a test means the
// whole test binary dies instead of one case failing.
func TestFakeScheduleQueryWithoutGroupOrTeacher(t *testing.T) {
	conf := model.ScheduleConfig{}

	scraper := FakeScraper{}
	if _, err := scraper.ScrapeSchedule("url", conf); !errors.Is(err, ErrInvalidScheduleQuery) {
		t.Fatalf("ScrapeSchedule = %v, want ErrInvalidScheduleQuery", err)
	}
	_, err := (ScraperAPI{}).GetSchedule(context.Background(), &apiclient.GetScheduleParams{})
	if !errors.Is(err, ErrInvalidScheduleQuery) {
		t.Fatalf("ScraperAPI.GetSchedule = %v, want ErrInvalidScheduleQuery", err)
	}
}
