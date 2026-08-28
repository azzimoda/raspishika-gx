package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSearchTeachers(t *testing.T) {
	var gotPath, gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"teacher_id":"205","name":"Иванов Иван Иванович"}]`))
	}))
	defer ts.Close()

	addr := strings.TrimPrefix(ts.URL, "http://")
	c := New(addr)

	teachers, err := c.SearchTeachers(context.Background(), "Иванов")
	if err != nil {
		t.Fatalf("SearchTeachers() error: %v", err)
	}
	if gotPath != "/api/v1/teachers/search" {
		t.Fatalf("request path = %q, want %q", gotPath, "/api/v1/teachers/search")
	}
	if gotQuery != "Иванов" {
		t.Fatalf("query = %q, want %q", gotQuery, "Иванов")
	}
	if len(teachers) != 1 {
		t.Fatalf("got %d teachers, want 1", len(teachers))
	}
	if teachers[0].TeacherID != "205" || teachers[0].Name != "Иванов Иван Иванович" {
		t.Fatalf("unexpected teacher: %+v", teachers[0])
	}
}
