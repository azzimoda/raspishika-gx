package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/redisdb"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
}

type mockScraper struct {
	departments    []model.Department
	departmentsErr error
	groupsByDept   map[string][]model.Group
	groupsErr      error
	teachers       []model.Teacher
	teachersErr    error
	schedule       *model.ScheduleData
	scheduleErr    error

	vacation    bool
	vacationErr error

	departmentsCalls int
	groupsCalls      int
	teachersCalls    int
	scheduleCalls    int
	scheduleURL      string
}

func (m *mockScraper) ScrapeDepartments() ([]model.Department, error) {
	m.departmentsCalls++
	return m.departments, m.departmentsErr
}

func (m *mockScraper) ScrapeDepartmentGroups(department *model.Department) ([]model.Group, error) {
	m.groupsCalls++
	if m.groupsErr != nil {
		return nil, m.groupsErr
	}
	return m.groupsByDept[department.Name], nil
}

func (m *mockScraper) ScrapeTeachers() ([]model.Teacher, error) {
	m.teachersCalls++
	return m.teachers, m.teachersErr
}

func (m *mockScraper) ScrapeSchedule(url string, conf model.ScheduleConfig) (*model.ScheduleData, error) {
	m.scheduleCalls++
	m.scheduleURL = url
	return m.schedule, m.scheduleErr
}

func (m *mockScraper) CheckVacation() (bool, error) {
	return m.vacation, m.vacationErr
}

func newTestService(t *testing.T, sc Scraper) (*ScheduleService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	cache, err := redisdb.New(&redis.Options{Addr: mr.Addr()})
	if err != nil {
		t.Fatalf("NewSmartClient: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return NewScheduleService(sc, cache), mr
}

func testData() (*mockScraper, []model.Department, map[string][]model.Group) {
	departments := []model.Department{
		{Name: "Отделение СОНХ"},
		{Name: "Отделение ИТ"},
	}
	groupsByDept := map[string][]model.Group{
		"отделение сонх": {
			{GroupID: "1", DepartmentID: "10", GroupName: "ИСПт-22-(9)-2", DepartmentName: "Отделение СОНХ", Year: 2026},
		},
		"отделение ит": {
			{GroupID: "2", DepartmentID: "11", GroupName: "ИСПт-22-(11)-1", DepartmentName: "Отделение ИТ", Year: 2026},
		},
	}
	return &mockScraper{departments: departments, groupsByDept: groupsByDept}, departments, groupsByDept
}

func TestGetDepartments(t *testing.T) {
	ctx := context.Background()

	t.Run("scrapes and caches", func(t *testing.T) {
		mock, departments, _ := testData()
		svc, _ := newTestService(t, mock)

		got, err := svc.GetDepartments(ctx)
		if err != nil {
			t.Fatalf("GetDepartments() error: %v", err)
		}
		if !reflect.DeepEqual(got, departments) {
			t.Fatalf("GetDepartments() = %+v, want %+v", got, departments)
		}
		if mock.departmentsCalls != 1 {
			t.Fatalf("scrape calls = %d, want 1", mock.departmentsCalls)
		}

		got2, err := svc.GetDepartments(ctx)
		if err != nil {
			t.Fatalf("second GetDepartments() error: %v", err)
		}
		if mock.departmentsCalls != 1 {
			t.Fatalf("second call should hit cache, scrape calls = %d", mock.departmentsCalls)
		}
		if !reflect.DeepEqual(got, got2) {
			t.Fatal("cached result differs from scraped result")
		}
	})

	t.Run("error with no cache", func(t *testing.T) {
		mock := &mockScraper{departmentsErr: errors.New("scrape failed")}
		svc, _ := newTestService(t, mock)

		_, err := svc.GetDepartments(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("error with stale cache returns old data", func(t *testing.T) {
		mock, departments, _ := testData()
		svc, mr := newTestService(t, mock)

		if _, err := svc.GetDepartments(ctx); err != nil {
			t.Fatalf("prime cache: %v", err)
		}
		mr.Del("departments:fresh")
		mock.departmentsErr = errors.New("scrape failed")

		got, err := svc.GetDepartments(ctx)
		if err != nil {
			t.Fatalf("expected old cache on scrape error, got %v", err)
		}
		if !reflect.DeepEqual(got, departments) {
			t.Fatalf("got %+v, want stale cache %+v", got, departments)
		}
	})
}

func TestGetTeachers(t *testing.T) {
	ctx := context.Background()
	teachers := []model.Teacher{{TeacherID: "1", Name: "Иванов"}, {TeacherID: "2", Name: "Петров"}}

	t.Run("scrapes and caches", func(t *testing.T) {
		mock := &mockScraper{teachers: teachers}
		svc, _ := newTestService(t, mock)

		got, err := svc.GetTeachers(ctx)
		if err != nil {
			t.Fatalf("GetTeachers() error: %v", err)
		}
		if !reflect.DeepEqual(got, teachers) {
			t.Fatalf("got %+v, want %+v", got, teachers)
		}

		if _, err := svc.GetTeachers(ctx); err != nil {
			t.Fatalf("second call error: %v", err)
		}
		if mock.teachersCalls != 1 {
			t.Fatalf("second call should hit cache, scrape calls = %d", mock.teachersCalls)
		}
	})

	t.Run("error with stale cache", func(t *testing.T) {
		mock := &mockScraper{teachers: teachers}
		svc, mr := newTestService(t, mock)

		if _, err := svc.GetTeachers(ctx); err != nil {
			t.Fatalf("prime cache: %v", err)
		}
		mr.Del("teachers:fresh")
		mock.teachersErr = errors.New("scrape failed")

		got, err := svc.GetTeachers(ctx)
		if err != nil {
			t.Fatalf("expected old cache, got %v", err)
		}
		if !reflect.DeepEqual(got, teachers) {
			t.Fatalf("got %+v, want stale cache %+v", got, teachers)
		}
	})

	t.Run("error with no cache", func(t *testing.T) {
		mock := &mockScraper{teachersErr: errors.New("scrape failed")}
		svc, _ := newTestService(t, mock)

		_, err := svc.GetTeachers(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestGetTeacherByName(t *testing.T) {
	ctx := context.Background()

	t.Run("found and cached", func(t *testing.T) {
		mock := &mockScraper{teachers: []model.Teacher{{TeacherID: "1", Name: "Иванов"}}}
		svc, _ := newTestService(t, mock)

		teacher, err := svc.GetTeacherByNameOrID(ctx, "иванов")
		if err != nil {
			t.Fatalf("GetTeacherByNameOrID() error: %v", err)
		}
		if teacher == nil || teacher.Name != "Иванов" {
			t.Fatalf("teacher = %+v, want Иванов", teacher)
		}

		teacher2, err := svc.GetTeacherByNameOrID(ctx, "Иванов")
		if err != nil {
			t.Fatalf("second GetTeacherByNameOrID() error: %v", err)
		}
		if teacher2.TeacherID != teacher.TeacherID {
			t.Fatal("cached teacher differs")
		}
		if mock.teachersCalls != 1 {
			t.Fatalf("second call should hit cache, scrape calls = %d", mock.teachersCalls)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock := &mockScraper{teachers: []model.Teacher{{TeacherID: "1", Name: "Иванов"}}}
		svc, _ := newTestService(t, mock)

		_, err := svc.GetTeacherByNameOrID(ctx, "Сидоров")
		if !errors.Is(err, ErrNoTeacher) {
			t.Fatalf("error = %v, want ErrNoTeacher", err)
		}
	})
}

func TestGetSchedule(t *testing.T) {
	ctx := context.Background()
	schedule := &model.ScheduleData{Days: []model.ScheduleDay{{Date: "01.09.2026"}}, IsOld: true}
	group := &model.Group{
		GroupID: "1", DepartmentID: "10", GroupName: "ИСПт-22-(9)-2",
		DepartmentName: "Отделение СОНХ", Year: 2026,
	}

	t.Run("group schedule scrapes and caches", func(t *testing.T) {
		mock, _, _ := testData()
		mock.schedule = schedule
		svc, _ := newTestService(t, mock)

		got, err := svc.GetSchedule(ctx, model.ScheduleConfig{Group: group})
		if err != nil {
			t.Fatalf("GetSchedule() error: %v", err)
		}
		if !reflect.DeepEqual(got, schedule) {
			t.Fatalf("got %+v, want %+v", got, schedule)
		}
		if mock.scheduleURL == "" {
			t.Fatal("scraper was not called with a URL")
		}
		if !strings.Contains(mock.scheduleURL, "gr=1&year=2026") {
			t.Fatalf("unexpected schedule URL: %s", mock.scheduleURL)
		}

		if _, err := svc.GetSchedule(ctx, model.ScheduleConfig{Group: group}); err != nil {
			t.Fatalf("second call error: %v", err)
		}
		if mock.scheduleCalls != 1 {
			t.Fatalf("second call should hit cache, scrape calls = %d", mock.scheduleCalls)
		}
	})

	t.Run("teacher schedule builds prep URL", func(t *testing.T) {
		mock, _, _ := testData()
		mock.schedule = schedule
		svc, _ := newTestService(t, mock)

		teacher := &model.Teacher{TeacherID: "123", Name: "Иванов"}
		if _, err := svc.GetSchedule(ctx, model.ScheduleConfig{Teacher: teacher}); err != nil {
			t.Fatalf("GetSchedule() error: %v", err)
		}
		if !strings.Contains(mock.scheduleURL, "action=prep&prep=123") {
			t.Fatalf("unexpected schedule URL: %s", mock.scheduleURL)
		}
	})

	t.Run("scrape error with stale cache", func(t *testing.T) {
		mock, _, _ := testData()
		mock.schedule = schedule
		svc, mr := newTestService(t, mock)

		if _, err := svc.GetSchedule(ctx, model.ScheduleConfig{Group: group}); err != nil {
			t.Fatalf("prime cache: %v", err)
		}
		mr.Del("schedule:ИСПт-22-(9)-2:fresh")
		mock.scheduleErr = errors.New("scrape failed")

		got, err := svc.GetSchedule(ctx, model.ScheduleConfig{Group: group})
		if err != nil {
			t.Fatalf("expected old cache, got %v", err)
		}
		if !reflect.DeepEqual(got, schedule) {
			t.Fatalf("got %+v, want stale cache %+v", got, schedule)
		}
	})

	t.Run("scrape error with no cache", func(t *testing.T) {
		mock, _, _ := testData()
		mock.scheduleErr = errors.New("scrape failed")
		svc, _ := newTestService(t, mock)

		_, err := svc.GetSchedule(ctx, model.ScheduleConfig{Group: group})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestTTLFreshExpiresBeforeData(t *testing.T) {
	ctx := context.Background()
	mock, _, _ := testData()
	svc, mr := newTestService(t, mock)

	if _, err := svc.GetDepartments(ctx); err != nil {
		t.Fatalf("prime cache: %v", err)
	}

	mr.FastForward(25 * time.Hour)

	data, fresh, exist := svc.rdb.Get(ctx, "departments")
	if !exist {
		t.Fatal("data key should still exist after fresh TTL expiry")
	}
	if fresh {
		t.Fatal("fresh marker should have expired")
	}
	if len(data) == 0 {
		t.Fatal("expected cached data bytes")
	}
}

func TestGetGroupByNameNormalizesLookalikes(t *testing.T) {
	ctx := context.Background()

	// The college emits the group name with a Latin "C"; the user types the
	// Cyrillic "С". They must match.
	departments := []model.Department{{Name: "Отделение СЭЗ"}}
	groupsByDept := map[string][]model.Group{
		"Отделение СЭЗ": {
			{GroupID: "5", DepartmentID: "20", GroupName: "CЭЗт-25-(9)-1", DepartmentName: "Отделение СЭЗ", Year: 2026},
		},
	}
	mock := &mockScraper{departments: departments, groupsByDept: groupsByDept}
	svc, _ := newTestService(t, mock)

	got, err := svc.GetGroupByName(ctx, model.GroupName("СЭЗт-25-(9)-1"))
	if err != nil {
		t.Fatalf("GetGroupByName() error: %v", err)
	}
	if got.GroupID != "5" {
		t.Fatalf("GetGroupByName().GroupID = %q, want %q", got.GroupID, "5")
	}
}
