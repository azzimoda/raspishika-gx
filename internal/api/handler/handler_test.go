package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/api/service"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
}

type mockService struct {
	getDepartments       func(ctx context.Context) ([]model.Department, error)
	getGroupByName       func(ctx context.Context, name model.GroupName) (*model.Group, error)
	getGroups            func(ctx context.Context) ([]model.Group, error)
	getGroupsByDept      func(ctx context.Context, departmentID string) ([]model.Group, error)
	searchTeachers       func(ctx context.Context, query string) ([]model.Teacher, error)
	getTeacherByNameOrID func(ctx context.Context, nameOrID string) (*model.Teacher, error)
	getTeachers          func(ctx context.Context) ([]model.Teacher, error)
	getSchedule          func(ctx context.Context, conf model.ScheduleConfig) (*model.ScheduleData, error)

	lastGroupName    model.GroupName
	lastDepartment   string
	lastTeacherName  string
	lastScheduleConf model.ScheduleConfig
}

func (m *mockService) GetDepartments(ctx context.Context) ([]model.Department, error) {
	if m.getDepartments != nil {
		return m.getDepartments(ctx)
	}
	return nil, nil
}

func (m *mockService) GetGroupByName(ctx context.Context, name model.GroupName) (*model.Group, error) {
	m.lastGroupName = name
	if m.getGroupByName != nil {
		return m.getGroupByName(ctx, name)
	}
	return nil, nil
}

func (m *mockService) GetGroups(ctx context.Context) ([]model.Group, error) {
	if m.getGroups != nil {
		return m.getGroups(ctx)
	}
	return nil, nil
}

func (m *mockService) GetGroupsByDepartment(ctx context.Context, departmentID string) ([]model.Group, error) {
	m.lastDepartment = departmentID
	if m.getGroupsByDept != nil {
		return m.getGroupsByDept(ctx, departmentID)
	}
	return nil, nil
}

func (m *mockService) GetTeacherByNameOrID(ctx context.Context, nameOrID string) (*model.Teacher, error) {
	m.lastTeacherName = nameOrID
	if m.getTeacherByNameOrID != nil {
		return m.getTeacherByNameOrID(ctx, nameOrID)
	}
	return nil, nil
}

func (m *mockService) SearchTeachers(ctx context.Context, nameOrID string) ([]model.Teacher, error) {
	if m.getTeachers != nil {
		return m.getTeachers(ctx)
	}
	return nil, nil
}

func (m *mockService) GetTeachers(ctx context.Context) ([]model.Teacher, error) {
	if m.getTeachers != nil {
		return m.getTeachers(ctx)
	}
	return nil, nil
}

func (m *mockService) GetSchedule(ctx context.Context, conf model.ScheduleConfig) (*model.ScheduleData, error) {
	m.lastScheduleConf = conf
	if m.getSchedule != nil {
		return m.getSchedule(ctx, conf)
	}
	return nil, nil
}

func setupRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	api.GET("/departments", h.GetDepartments)
	api.GET("/groups", h.GetGroups)
	api.GET("/teachers", h.GetTeachers)
	api.GET("/schedule", h.GetSchedule)
	return r
}

func doRequest(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode response %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestGetDepartments(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockService{getDepartments: func(context.Context) ([]model.Department, error) {
			return []model.Department{{Name: "Отделение СОНХ"}}, nil
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/departments")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		got := decodeBody[[]model.Department](t, rec)
		if len(got) != 1 || got[0].Name != "Отделение СОНХ" {
			t.Fatalf("body = %+v", got)
		}
	})

	t.Run("service error", func(t *testing.T) {
		mock := &mockService{getDepartments: func(context.Context) ([]model.Department, error) {
			return nil, errors.New("boom")
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/departments")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})

	t.Run("service unavailable", func(t *testing.T) {
		mock := &mockService{getDepartments: func(context.Context) ([]model.Department, error) {
			return nil, service.ErrServiceUnavailable
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/departments")

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestGetGroups(t *testing.T) {
	t.Run("all groups", func(t *testing.T) {
		mock := &mockService{getGroups: func(context.Context) ([]model.Group, error) {
			return []model.Group{{GroupName: "ИСПт-22-(9)-2"}}, nil
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/groups")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		got := decodeBody[[]model.Group](t, rec)
		if len(got) != 1 || got[0].GroupName != "ИСПт-22-(9)-2" {
			t.Fatalf("body = %+v", got)
		}
	})

	t.Run("by department", func(t *testing.T) {
		mock := &mockService{getGroupsByDept: func(context.Context, string) ([]model.Group, error) {
			return []model.Group{{GroupName: "ИСПт-22-(9)-2"}}, nil
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/groups?department=Отделение%20СОНХ")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		if mock.lastDepartment != "Отделение СОНХ" {
			t.Fatalf("department = %q", mock.lastDepartment)
		}
	})

	t.Run("department not found", func(t *testing.T) {
		mock := &mockService{getGroupsByDept: func(context.Context, string) ([]model.Group, error) {
			return nil, service.ErrNoDepartment
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/groups?department=Нет")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("service error", func(t *testing.T) {
		mock := &mockService{getGroups: func(context.Context) ([]model.Group, error) {
			return nil, errors.New("boom")
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/groups")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})
}

func TestGetTeachers(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockService{getTeachers: func(context.Context) ([]model.Teacher, error) {
			return []model.Teacher{{Name: "Иванов"}}, nil
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/teachers")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		got := decodeBody[[]model.Teacher](t, rec)
		if len(got) != 1 || got[0].Name != "Иванов" {
			t.Fatalf("body = %+v", got)
		}
	})

	t.Run("service error", func(t *testing.T) {
		mock := &mockService{getTeachers: func(context.Context) ([]model.Teacher, error) {
			return nil, errors.New("boom")
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/teachers")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", rec.Code)
		}
	})

	t.Run("service unavailable", func(t *testing.T) {
		mock := &mockService{getTeachers: func(context.Context) ([]model.Teacher, error) {
			return nil, service.ErrServiceUnavailable
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/teachers")

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestGetSchedule(t *testing.T) {
	group := &model.Group{GroupID: "1", GroupName: "ИСПт-22-(9)-2"}
	teacher := &model.Teacher{TeacherID: "123", Name: "Иванов"}
	schedule := &model.ScheduleData{Days: []model.ScheduleDay{{Date: "01.09.2026"}}}

	t.Run("no query params", func(t *testing.T) {
		mock := &mockService{}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid group name", func(t *testing.T) {
		mock := &mockService{}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?group=не-группа")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
		}
		got := decodeBody[map[string]string](t, rec)
		if got["error"] != "string does not match the group name format: 'не-группа'" {
			t.Fatalf("error message = %q", got["error"])
		}
	})

	t.Run("group not found", func(t *testing.T) {
		mock := &mockService{getGroupByName: func(context.Context, model.GroupName) (*model.Group, error) {
			return nil, service.ErrNoGroup
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?group=ИСПт-22-(9)-2")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("group schedule", func(t *testing.T) {
		mock := &mockService{
			getGroupByName: func(context.Context, model.GroupName) (*model.Group, error) {
				return group, nil
			},
			getSchedule: func(context.Context, model.ScheduleConfig) (*model.ScheduleData, error) {
				return schedule, nil
			},
		}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?group=ИСПт-22-(9)-2")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		if mock.lastGroupName != "ИСПт-22-(9)-2" {
			t.Fatalf("group name = %q", mock.lastGroupName)
		}
		if mock.lastScheduleConf.Group != group {
			t.Fatal("GetSchedule was not called with the resolved group")
		}
	})

	t.Run("teacher not found", func(t *testing.T) {
		mock := &mockService{getTeacherByNameOrID: func(context.Context, string) (*model.Teacher, error) {
			return nil, service.ErrNoTeacher
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?teacher=Иванов")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("teacher schedule", func(t *testing.T) {
		mock := &mockService{
			getTeacherByNameOrID: func(context.Context, string) (*model.Teacher, error) {
				return teacher, nil
			},
			getSchedule: func(context.Context, model.ScheduleConfig) (*model.ScheduleData, error) {
				return schedule, nil
			},
		}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?teacher=Иванов")

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
		}
		if mock.lastTeacherName != "Иванов" {
			t.Fatalf("teacher name = %q", mock.lastTeacherName)
		}
		if mock.lastScheduleConf.Teacher != teacher {
			t.Fatal("GetSchedule was not called with the resolved teacher")
		}
	})

	t.Run("schedule service error", func(t *testing.T) {
		mock := &mockService{
			getGroupByName: func(context.Context, model.GroupName) (*model.Group, error) {
				return group, nil
			},
			getSchedule: func(context.Context, model.ScheduleConfig) (*model.ScheduleData, error) {
				return nil, errors.New("boom")
			},
		}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?group=ИСПт-22-(9)-2")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("group lookup unavailable", func(t *testing.T) {
		mock := &mockService{getGroupByName: func(context.Context, model.GroupName) (*model.Group, error) {
			return nil, service.ErrServiceUnavailable
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?group=ИСПт-22-(9)-2")

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("teacher lookup unavailable", func(t *testing.T) {
		mock := &mockService{getTeacherByNameOrID: func(context.Context, string) (*model.Teacher, error) {
			return nil, service.ErrServiceUnavailable
		}}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?teacher=Иванов")

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("schedule fetch unavailable", func(t *testing.T) {
		mock := &mockService{
			getGroupByName: func(context.Context, model.GroupName) (*model.Group, error) {
				return group, nil
			},
			getSchedule: func(context.Context, model.ScheduleConfig) (*model.ScheduleData, error) {
				return nil, service.ErrServiceUnavailable
			},
		}
		h := NewHandler(mock)
		rec := doRequest(t, setupRouter(h), "/api/v1/schedule?group=ИСПт-22-(9)-2")

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503; body: %s", rec.Code, rec.Body.String())
		}
	})
}
