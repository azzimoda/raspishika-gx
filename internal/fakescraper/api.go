package fakescraper

import (
	"context"
	"errors"
	"fmt"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/rs/zerolog/log"
)

// Sentinel errors for the fake scraper's failure modes. They let a test assert on
// the reason a call failed with errors.Is, and let the fake report failures with
// the same errors the real client understands, so a bot under test cannot tell
// the demo dataset from the real API.
var (
	// ErrNoDepartment is returned for a department that is not in the fake data.
	ErrNoDepartment = errors.New("no such department")
	// ErrNoSchedule is returned for a group or teacher that has no fake schedule.
	ErrNoSchedule = errors.New("no such group/teacher")
	// ErrInvalidScheduleQuery is returned when neither a group nor a teacher was
	// asked for.
	ErrInvalidScheduleQuery = errors.New("invalid schedule config")
)

// ScraperAPI implements the service API client over the hardcoded fake data.
// it lets the demo binaries (fakebot, fakevkbot) run against the same offline
// dataset without a real scraper.
type ScraperAPI struct{}

var fakeScraper = FakeScraper{}

func (ScraperAPI) GetDepartments(context.Context) ([]model.Department, error) {
	return fakeScraper.ScrapeDepartments()
}

func (ScraperAPI) GetGroup(ctx context.Context, name string) (*model.Group, error) {
	for _, gs := range FakeGroups {
		for _, g := range gs {
			if string(g.GroupName) == name {
				return &g, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: группа %s", apiclient.ErrNotFound, name)
}

func (ScraperAPI) GetGroups(ctx context.Context, departmentName string) ([]model.Group, error) {
	return fakeScraper.ScrapeDepartmentGroups(&model.Department{Name: departmentName})
}

func (ScraperAPI) GetTeacher(ctx context.Context, nameOrID string) (*model.Teacher, error) {
	log.Debug().Str("target", nameOrID).Msg("Getting teacher by name or ID...")

	for _, t := range FakeTeachers {
		log.Trace().Str("name", t.Name).Str("id", t.TeacherID).Str("target", nameOrID).Send()
		if t.Name == nameOrID || t.TeacherID == nameOrID {
			log.Trace().Msg("Teacher found")
			return &t, nil
		}
	}
	log.Warn().Msg("Teacher not found")
	return nil, fmt.Errorf("%w: преподаватель %s", apiclient.ErrNotFound, nameOrID)
}

func (ScraperAPI) SearchTeachers(context.Context, string) ([]model.Teacher, error) {
	return fakeScraper.ScrapeTeachers()
}

func (ScraperAPI) GetTeachers(context.Context) ([]model.Teacher, error) {
	return fakeScraper.ScrapeTeachers()
}

func (f ScraperAPI) GetSchedule(ctx context.Context, params *apiclient.GetScheduleParams) (
	schedule *model.ScheduleData,
	err error,
) {
	log.Debug().Any("params", params).Msg("Getting schedule...")

	var key string
	if params.Group != "" {
		key = params.Group
		log.Trace().Str("name", key).Msg("Group schedule")
	} else if params.Teacher != "" {
		teacher, err := f.GetTeacher(ctx, params.Teacher)
		if err != nil {
			log.Warn().Msg("Teacher not found by ID")
			return nil, err
		}
		key = teacher.Name
		log.Trace().Str("name", key).Msg("Teacher schedule")
	} else {
		return nil, fmt.Errorf("%w: neither group nor teacher", ErrInvalidScheduleQuery)
	}

	scheduleData, ok := FakeSchedule(key)
	if ok {
		log.Trace().Msg("Schedule found")
		return &scheduleData, nil
	}
	log.Warn().Msg("Schedule not found")
	return nil, fmt.Errorf("%w: расписание не найдено для %s", apiclient.ErrServiceUnavailable, key)
}
