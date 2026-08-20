package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/app"
	"github.com/azzimoda/raspishika-gx/internal/fakescraper"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/rs/zerolog/log"
)

var fakeScraper = fakescraper.FakeScraper{}

type fakeScraperAPI struct{}

func (fakeScraperAPI) GetDepartments(context.Context) ([]model.Department, error) {
	return fakeScraper.ScrapeDepartments()
}
func (fakeScraperAPI) GetGroup(ctx context.Context, name string) (*model.Group, error) {
	for _, gs := range fakescraper.FakeGroups {
		for _, g := range gs {
			if string(g.GroupName) == name {
				return &g, nil
			}
		}
	}
	return nil, errors.New("not found")
}
func (fakeScraperAPI) GetGroups(ctx context.Context, departmentName string) ([]model.Group, error) {
	return fakeScraper.ScrapeDepartmentGroups(&model.Department{Name: departmentName})
}

func (fakeScraperAPI) GetTeacher(ctx context.Context, nameOrID string) (*model.Teacher, error) {
	log.Debug().Str("target", nameOrID).Msg("Getting teacher by name or ID...")

	for _, t := range fakescraper.FakeTeachers {
		log.Trace().Str("name", t.Name).Str("id", t.TeacherID).Str("target", nameOrID).Send()
		if t.Name == nameOrID || t.TeacherID == nameOrID {
			log.Trace().Msg("Teacher found")
			return &t, nil
		}
	}
	log.Warn().Msg("Teacher not found")
	return nil, errors.New("not found")
}
func (fakeScraperAPI) SearchTeachers(context.Context, string) ([]model.Teacher, error) {
	return fakeScraper.ScrapeTeachers()
}
func (fakeScraperAPI) GetTeachers(context.Context) ([]model.Teacher, error) {
	return fakeScraper.ScrapeTeachers()
}
func (f fakeScraperAPI) GetSchedule(ctx context.Context, params *apiclient.GetScheduleParams) (
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
		panic("invalid schedule config")
	}

	scheduleData, ok := fakescraper.FakeSchedules[key]
	if ok {
		log.Trace().Msg("Schedule found")
		return &scheduleData, nil
	}
	log.Warn().Msg("Schedule not found")
	return nil, fmt.Errorf("no such group/teacher")

}

func main() {

	botApp, err := app.NewWithScraper(fakeScraperAPI{})
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create app")
	}
	if err := botApp.Run(); err != nil {
		log.Fatal().Err(err).Msg("App exited with error")
	}
}
