package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/api/handler"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
}

type stubService struct{}

func (stubService) GetDepartments(context.Context) ([]model.Department, error) {
	return []model.Department{{Name: "Отделение СОНХ"}}, nil
}

func (stubService) GetGroupByName(context.Context, model.GroupName) (*model.Group, error) {
	return &model.Group{GroupName: "ИСПт-22-(9)-2"}, nil
}

func (stubService) GetGroups(context.Context) ([]model.Group, error) {
	return []model.Group{{GroupName: "ИСПт-22-(9)-2"}}, nil
}

func (stubService) GetGroupsByDepartment(context.Context, string) ([]model.Group, error) {
	return []model.Group{{GroupName: "ИСПт-22-(9)-2"}}, nil
}

func (stubService) GetTeacherByNameOrID(context.Context, string) (*model.Teacher, error) {
	return &model.Teacher{Name: "Иванов"}, nil
}

func (stubService) SearchTeachers(context.Context, string) ([]model.Teacher, error) {
	return []model.Teacher{{Name: "Иванов"}}, nil
}

func (stubService) GetTeachers(context.Context) ([]model.Teacher, error) {
	return []model.Teacher{{Name: "Иванов"}}, nil
}

func (stubService) GetSchedule(context.Context, model.ScheduleConfig) (*model.ScheduleData, error) {
	return &model.ScheduleData{}, nil
}

func newTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return Init(handler.NewHandler(stubService{}))
}

func doRequest(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestRoutes(t *testing.T) {
	r := newTestEngine(t)

	tests := []struct {
		path string
		want int
	}{
		{path: "/api/v1/departments", want: http.StatusOK},
		{path: "/api/v1/groups", want: http.StatusOK},
		{path: "/api/v1/teachers", want: http.StatusOK},
		{path: "/api/v1/schedule?group=ИСПт-22-(9)-2", want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := doRequest(t, r, tt.path)
			if rec.Code != tt.want {
				t.Fatalf("GET %s = %d, want %d; body: %s", tt.path, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestSwaggerRoute(t *testing.T) {
	r := newTestEngine(t)

	rec := doRequest(t, r, "/swagger/index.html")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /swagger/index.html = %d, want 200", rec.Code)
	}
}

func TestUnknownRoute(t *testing.T) {
	r := newTestEngine(t)

	rec := doRequest(t, r, "/api/v1/unknown")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/unknown = %d, want 404", rec.Code)
	}
}
