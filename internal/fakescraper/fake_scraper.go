package fakescraper

import (
	"fmt"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/rs/zerolog/log"
)

// FakeSchedule returns the demo schedule for the given group name or teacher
// name with the day dates realigned to the current calendar days.
func FakeSchedule(key string) (model.ScheduleData, bool) {
	schedule, ok := FakeSchedules[key]
	if !ok {
		return model.ScheduleData{}, false
	}
	return withCurrentDates(schedule), true
}

func withCurrentDates(schedule model.ScheduleData) model.ScheduleData {
	days := make([]model.ScheduleDay, len(schedule.Days))
	copy(days, schedule.Days)

	now := time.Now()
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if date.Weekday() == time.Sunday {
		date = date.AddDate(0, 0, 1)
	}

	for i := range days {
		days[i].Date = date.Format("2006-01-02")
		days[i].Weekday = model.RussianWeekday(date.Weekday())
		days[i].WeekKind = weekKindForDate(date)
		date = date.AddDate(0, 0, 1)
		if date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
		}
	}

	schedule.Days = days
	return schedule
}

func weekKindForDate(date time.Time) string {
	if _, week := date.ISOWeek(); week%2 == 1 {
		return "нечетная"
	}
	return "чётная"
}

func NewFakeScraper() *FakeScraper { return new(FakeScraper) }

// FakeScraper serves static demo data for the fake API.
type FakeScraper struct{}

// CheckVacation returns a fixed demo vacation status ([false]).
func (s *FakeScraper) CheckVacation() (bool, error) {

	log.Debug().Msg("CheckVacation")
	return false, nil
}

// ScrapeDepartments returns a fixed list of demo departments.
func (s *FakeScraper) ScrapeDepartments() ([]model.Department, error) {

	log.Debug().Msg("ScrapeDepartments")
	return FakeDepartments, nil
}

// ScrapeDepartmentGroups returns fixed demo groups for the given department.
func (s *FakeScraper) ScrapeDepartmentGroups(department *model.Department) ([]model.Group, error) {

	log.Debug().Str("department", department.Name).Msg("ScrapeDepartmentGroups")
	for d, gs := range FakeGroups {
		if strings.EqualFold(department.Name, d) {
			return gs, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrNoDepartment, department.Name)
}

// ScrapeTeachers returns a fixed list of demo teachers.
func (s *FakeScraper) ScrapeTeachers() ([]model.Teacher, error) {

	log.Debug().Msg("ScrapeTeachers")
	return FakeTeachers, nil
}

// ScrapeSchedule returns a fixed demo schedule for the requested group or teacher.
func (s *FakeScraper) ScrapeSchedule(url string, conf model.ScheduleConfig) (*model.ScheduleData, error) {

	log.Debug().Any("conf", conf).Msg("Scraping schedule...")
	var key string
	if conf.Group != nil {
		key = string(conf.Group.GroupName)
	} else if conf.Teacher != nil {
		key = conf.Teacher.Name
	} else {
		// A query with neither a group nor a teacher is a caller bug, but
		// panicking here took down the whole process: the demo API serves this
		// from an HTTP handler, so a single malformed request would restart the
		// container. Return a sentinel like every other failure instead.
		return nil, fmt.Errorf("%w: neither group nor teacher", ErrInvalidScheduleQuery)
	}

	schedule, ok := FakeSchedule(key)
	if ok {
		log.Trace().Msg("Schedule found")
		return &schedule, nil
	}
	log.Trace().Msg("Schedule not found")
	return nil, fmt.Errorf("%w: %q", ErrNoSchedule, key)
}
