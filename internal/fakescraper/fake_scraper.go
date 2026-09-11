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

// withCurrentDates realigns the static demo days so the first day of the
// schedule is the current calendar day, like the real college site serves the
// current week. Sundays (when the college is closed) are skipped; weekday names
// and dates are recomputed from the template pairs.
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
		date = date.AddDate(0, 0, 1)
		if date.Weekday() == time.Sunday {
			date = date.AddDate(0, 0, 1)
		}
	}

	schedule.Days = days
	return schedule
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
	return nil, fmt.Errorf("no department %q", department.Name)
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
		panic("invalid schedule config")
	}

	schedule, ok := FakeSchedule(key)
	if ok {
		log.Trace().Msg("Schedule found")
		return &schedule, nil
	}
	log.Trace().Msg("Schedule not found")
	return nil, fmt.Errorf("no such group/teacher")
}
