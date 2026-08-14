package scraper

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"golang.org/x/text/encoding/charmap"
)

func TestScheduleURLGroup(t *testing.T) {
	conf := model.ScheduleConfig{
		Group: &model.Group{DepartmentID: "15", GroupID: "205", Year: 2026, DepartmentName: "Отделение СОНХ"},
	}

	want := "https://coworking.tyuiu.ru/shs/all_t/sh.php?action=group&union=0&sid=15&gr=205&year=2026&vr=1"
	if got := ScheduleURL(conf, nil); got != want {
		t.Fatalf("ScheduleURL() = %q, want %q", got, want)
	}
}

func TestScheduleURLExtramuralGroup(t *testing.T) {
	conf := model.ScheduleConfig{
		Group: &model.Group{DepartmentID: "15", GroupID: "205", Year: 2026, DepartmentName: "Отделение заочного обучения"},
	}

	want := "https://coworking.tyuiu.ru/shs/all_t/shz.php?action=group&union=0&sid=15&gr=205&year=2026&vr=1"
	if got := ScheduleURL(conf, nil); got != want {
		t.Fatalf("ScheduleURL() = %q, want %q", got, want)
	}
}

func TestScheduleURLTeacher(t *testing.T) {
	conf := model.ScheduleConfig{Teacher: &model.Teacher{TeacherID: "123", Name: "Иванов"}}
	departmentIDs := []string{"15", "17"}
	year := time.Now().Year()

	want := "https://coworking.tyuiu.ru/shs/all_t/sh.php?action=prep&prep=123&vr=1&count=2" +
		"&shed[0]=15&union[0]=0&year[0]=" + strconv.Itoa(year) +
		"&shed[1]=17&union[1]=0&year[1]=" + strconv.Itoa(year)
	if got := ScheduleURL(conf, departmentIDs); got != want {
		t.Fatalf("ScheduleURL() = %q, want %q", got, want)
	}
}

func TestScheduleURLNoConfig(t *testing.T) {
	if got := ScheduleURL(model.ScheduleConfig{}, nil); got != "" {
		t.Fatalf("ScheduleURL() = %q, want empty", got)
	}
}

func TestWindows1251ToUTF8(t *testing.T) {
	const want = "Привет, мир!"
	encoded, err := charmap.Windows1251.NewEncoder().String(want)
	if err != nil {
		t.Fatalf("failed to encode fixture: %v", err)
	}

	got, err := windows1251ToUTF8(encoded)
	if err != nil {
		t.Fatalf("windows1251ToUTF8() error: %v", err)
	}
	if got != want {
		t.Fatalf("windows1251ToUTF8() = %q, want %q", got, want)
	}
}

func TestParseDepartments(t *testing.T) {
	const html = `<html><body>
		<ul class="mod-menu">
			<li class="col-lg col-md-6"><a href="/page1.html">Отделение СОНХ</a></li>
			<li class="col-lg col-md-6"><a href="/page2.html?x=1&amp;y=2">Заочное отделение</a></li>
			<li class="col-lg col-md-6"><a href="/page3.html">Не то</a></li>
		</ul>
	</body></html>`

	departments, err := parseDepartments(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parseDepartments() error: %v", err)
	}

	if len(departments) != 2 {
		t.Fatalf("parseDepartments() = %+v, want 2 departments", departments)
	}

	want := []model.Department{
		{Name: "Отделение СОНХ", URL: "https://mnokol.tyuiu.ru/page1.html"},
		{Name: "Заочное отделение", URL: "https://mnokol.tyuiu.ru/page2.html?x=1&y=2"},
	}
	for i, w := range want {
		if departments[i] != w {
			t.Errorf("department %d = %+v, want %+v", i, departments[i], w)
		}
	}
}

func TestHTTPGetRequestWithHeaders(t *testing.T) {
	var gotUA, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotReferer = r.Header.Get("Referer")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := httpGetRequestWithHeaders(srv.URL, map[string]string{
		"User-Agent": "test-agent",
		"Referer":    "https://coworking.tyuiu.ru/shs/all_t/",
	})
	if err != nil {
		t.Fatalf("httpGetRequestWithHeaders() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if gotUA != "test-agent" {
		t.Fatalf("User-Agent = %q, want test-agent", gotUA)
	}
	if gotReferer != "https://coworking.tyuiu.ru/shs/all_t/" {
		t.Fatalf("Referer = %q", gotReferer)
	}
}

func TestHTTPGetRequestRetryOnServerError(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := httpGetRequestRetryingWithRandomHeaders(srv.URL, 2)
	if err != nil {
		t.Fatalf("httpGetRequestRetryingWithRandomHeaders() error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (retry on 500)", calls)
	}
}

func TestHTTPGetRequestRetryExhausted(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := httpGetRequestRetryingWithRandomHeaders(srv.URL, 2)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}
