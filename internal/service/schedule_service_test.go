package service

import (
	"context"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

// scheduleRepoStub is an in-memory repository.ScheduleRepository.
type scheduleRepoStub struct {
	stored map[string]*model.Schedule
}

func (r *scheduleRepoStub) Create(_ context.Context, schedule *model.Schedule) error {
	return r.CreateOrUpdate(context.Background(), schedule)
}

func (r *scheduleRepoStub) CreateOrUpdate(_ context.Context, schedule *model.Schedule) error {
	if r.stored == nil {
		r.stored = make(map[string]*model.Schedule)
	}
	schedule.UpdatedAt = time.Now()
	r.stored[schedule.CacheKey] = schedule
	return nil
}

func (r *scheduleRepoStub) GetByKey(_ context.Context, key string) (*model.Schedule, error) {
	schedule, ok := r.stored[key]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return schedule, nil
}

func (r *scheduleRepoStub) Update(_ context.Context, schedule *model.Schedule) error {
	return r.CreateOrUpdate(context.Background(), schedule)
}

// scheduleAPIClientStub implements service.APIClient for schedule tests.
type scheduleAPIClientStub struct {
	schedule *model.ScheduleData
	calls    int
}

func (a *scheduleAPIClientStub) GetDepartments(context.Context) ([]model.Department, error) {
	return nil, nil
}

func (a *scheduleAPIClientStub) GetGroup(context.Context, string) (*model.Group, error) {
	return nil, nil
}

func (a *scheduleAPIClientStub) GetGroups(context.Context, string) ([]model.Group, error) {
	return nil, nil
}

func (a *scheduleAPIClientStub) GetTeacher(context.Context, string) (*model.Teacher, error) {
	return nil, nil
}

func (a *scheduleAPIClientStub) SearchTeachers(context.Context, string) ([]model.Teacher, error) {
	return nil, nil
}

func (a *scheduleAPIClientStub) GetTeachers(context.Context) ([]model.Teacher, error) {
	return nil, nil
}

func (a *scheduleAPIClientStub) GetSchedule(context.Context, *apiclient.GetScheduleParams) (*model.ScheduleData, error) {
	a.calls++
	return a.schedule, nil
}

// TestGetScheduleReappliesThemeOnCacheHit guards against serving the cached
// schedule's theme to a chat with a different setting: the cache key does not
// include the theme, so a light-mode cache entry must still render dark for a
// dark-mode request (and vice versa).
func TestGetScheduleReappliesThemeOnCacheHit(t *testing.T) {
	prev := viper.Get(config.KeyCacheScheduleTTL)
	viper.Set(config.KeyCacheScheduleTTL, time.Hour)
	t.Cleanup(func() {
		if prev == nil {
			viper.Set(config.KeyCacheScheduleTTL, nil)
		} else {
			viper.Set(config.KeyCacheScheduleTTL, prev)
		}
	})

	group := &model.Group{
		GroupID:        "1",
		DepartmentID:   "15",
		GroupName:      "ИСПт-22-(9)-2",
		DepartmentName: "Отделение СОНХ",
	}
	api := &scheduleAPIClientStub{schedule: &model.ScheduleData{
		Days: []model.ScheduleDay{{Date: "2026-10-20", Weekday: "понедельник", WeekKind: "чётная"}},
	}}
	repo := &scheduleRepoStub{}
	svc := NewScheduleService(api, nil, repo)
	ctx := context.Background()

	light, err := svc.GetSchedule(ctx, model.GroupScheduleConfig(group, false))
	if err != nil {
		t.Fatalf("GetSchedule(light) error = %v", err)
	}
	if light.Config.IsDark {
		t.Fatal("GetSchedule(light) returned a dark config")
	}
	if api.calls != 1 {
		t.Fatalf("GetSchedule(light) API calls = %d, want 1", api.calls)
	}

	dark, err := svc.GetSchedule(ctx, model.GroupScheduleConfig(group, true))
	if err != nil {
		t.Fatalf("GetSchedule(dark) error = %v", err)
	}
	if !dark.Config.IsDark {
		t.Fatal("GetSchedule(dark) served from cache returned a light-theme config")
	}
	if api.calls != 1 {
		t.Fatalf("GetSchedule(dark) hit the API (%d calls), want a cache hit", api.calls)
	}
}
