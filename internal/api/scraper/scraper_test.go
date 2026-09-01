package scraper

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"golang.org/x/text/encoding/charmap"
)

func TestWindows1251ToUTF8(t *testing.T) {
	const want = "Привет, мир!"
	encoded, err := charmap.Windows1251.NewEncoder().String(want)
	if err != nil {
		t.Fatalf("failed to encode fixture: %v", err)
	}

	got, err := encodeWindows1251ToUTF8(encoded)
	if err != nil {
		t.Fatalf("windows1251ToUTF8() error: %v", err)
	}
	if got != want {
		t.Fatalf("windows1251ToUTF8() = %q, want %q", got, want)
	}
}

func TestParseDepartmentRows(t *testing.T) {
	rows := [][]string{
		{"28728", "26", "22", "10", "1", "р"},
		{"28728", "26", "22", "1Д", "1", "р"},
		{"", "", "", "", "", "", "not a group"},
		{"otd", "1", "2", "3", "4", "otd", "АиЭС"},
	}

	name, groupRows := parseDepartmentRows(rows)
	if name != "АиЭС" {
		t.Fatalf("name = %q, want %q", name, "АиЭС")
	}
	if len(groupRows) != 2 {
		t.Fatalf("groupRows = %d, want 2 (%+v)", len(groupRows), groupRows)
	}
}

func TestParseDepartmentRowsKeepsGroupRowCarryingMarker(t *testing.T) {
	// The college attaches the department marker (otd/<name>) to its first
	// group row rather than to a separate header row. That row is a real group
	// and must be kept, otherwise the oldest group of each department is lost.
	rows := [][]string{
		{"926", "КИПр", "25", "9", "1", "otd", "АиЭС"},
		{"991", "КИПр", "26", "9", "1"},
		{"992", "КИПр", "26", "11", "1"},
	}

	name, groupRows := parseDepartmentRows(rows)
	if name != "АиЭС" {
		t.Fatalf("name = %q, want %q", name, "АиЭС")
	}
	if len(groupRows) != 3 {
		t.Fatalf("groupRows = %d, want 3 (%+v)", len(groupRows), groupRows)
	}
	if got, want := groupTitle(groupRows[0]), "КИПр-25-(9)-1"; got != want {
		t.Fatalf("first group title = %q, want %q (group row dropped)", got, want)
	}
}

func TestGroupTitle(t *testing.T) {
	got := groupTitle([]string{"28728", "26", "22", "10", "1", "р"})
	if want := "26-22-(10)-1"; got != want {
		t.Fatalf("groupTitle() = %q, want %q", got, want)
	}
}

func TestScrapeGroupsNormalizesLookalikes(t *testing.T) {
	_, groupRows := parseDepartmentRows([][]string{
		{"otd", "1", "2", "3", "4", "otd", "Отделение"},
		{"28728", "CЭЗт", "25", "9", "1", "р"},
	})
	if len(groupRows) != 1 {
		t.Fatalf("groupRows = %d, want 1", len(groupRows))
	}
	title := groupTitle(groupRows[0])
	if got, want := model.NormalizeCyrillicLookalikes(title), "СЭЗт-25-(9)-1"; got != want {
		t.Fatalf("normalized groupTitle = %q, want %q", got, want)
	}
}

func TestScrapeDepartments(t *testing.T) {
	old := groupsFunctURL
	t.Cleanup(func() { groupsFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			t.Errorf("Content-Type = %q", ct)
		}
		if xrw := r.Header.Get("X-Requested-With"); xrw != "XMLHttpRequest" {
			t.Errorf("X-Requested-With = %q", xrw)
		}
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))

		switch form.Get("action") {
		case "load_info":
			fmt.Fprint(w, `[{"un":0,"id":"28728","year":"2026","err":0},{"un":1,"id":"28727","year":"2026","err":0}]`)
		case "load":
			if form.Get("act") != "list_groups" {
				t.Errorf("act = %q, want list_groups", form.Get("act"))
			}
			name := "Другое отделение"
			if form.Get("vd") == "28728" {
				name = "АиЭС"
			}
			fmt.Fprintf(w, `[["otd","x","","","","otd","%s"],["28728","26","22","10","1","р"]]`, name)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	groupsFunctURL = srv.URL + "/shs/gr_all/funct.php"

	got, err := New().ScrapeDepartments()
	if err != nil {
		t.Fatalf("ScrapeDepartments() error: %v", err)
	}

	want := []model.Department{
		{ID: "28728", Name: "АиЭС", Year: 2026},
		{ID: "28727", Name: "Другое отделение", Year: 2026},
	}
	if len(got) != len(want) {
		t.Fatalf("ScrapeDepartments() = %+v, want %d departments", got, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("department %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestScrapeDepartmentsAllErrored(t *testing.T) {
	old := groupsFunctURL
	t.Cleanup(func() { groupsFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"un":0,"id":"28728","year":"2026","err":1}]`)
	}))
	defer srv.Close()
	groupsFunctURL = srv.URL + "/shs/gr_all/funct.php"

	if _, err := New().ScrapeDepartments(); err == nil {
		t.Fatal("ScrapeDepartments() error = nil, want an error")
	}
}

func TestScrapeDepartmentGroups(t *testing.T) {
	old := groupsFunctURL
	t.Cleanup(func() { groupsFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		if form.Get("vd") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `[
			["otd","x","","","","otd","АиЭС"],
			["28728","26","22","10","1","р"],
			["28728","26","22","1Д","1","р"]
		]`)
	}))
	defer srv.Close()
	groupsFunctURL = srv.URL + "/shs/gr_all/funct.php"

	department := &model.Department{ID: "28728"}
	got, err := New().ScrapeDepartmentGroups(department)
	if err != nil {
		t.Fatalf("ScrapeDepartmentGroups() error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ScrapeDepartmentGroups() = %+v, want 2 groups", got)
	}
	want := model.Group{
		GroupID:        "28728",
		DepartmentID:   "28728",
		GroupName:      "26-22-(10)-1",
		DepartmentName: "АиЭС",
	}
	if got[0] != want {
		t.Errorf("group 0 = %+v, want %+v", got[0], want)
	}
}

func TestScrapeDepartmentGroupsKeepsGroupWithMarker(t *testing.T) {
	// Mirrors the real college response: the department marker (otd/<name>)
	// rides on the first group row, which must be kept.
	old := groupsFunctURL
	t.Cleanup(func() { groupsFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		if form.Get("vd") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `[
			["926","КИПр","25","9","1","otd","АиЭС"],
			["991","КИПр","26","9","1"],
			["992","КИПр","26","11","1"]
		]`)
	}))
	defer srv.Close()
	groupsFunctURL = srv.URL + "/shs/gr_all/funct.php"

	got, err := New().ScrapeDepartmentGroups(&model.Department{ID: "28728"})
	if err != nil {
		t.Fatalf("ScrapeDepartmentGroups() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ScrapeDepartmentGroups() = %+v, want 3 groups (oldest group dropped)", got)
	}
	if got[0].GroupName != "КИПр-25-(9)-1" {
		t.Fatalf("oldest group = %q, want %q", got[0].GroupName, "КИПр-25-(9)-1")
	}
	if got[0].DepartmentName != "АиЭС" {
		t.Fatalf("department name = %q, want %q", got[0].DepartmentName, "АиЭС")
	}
}

func TestScrapeDepartmentGroupsResolvesMissingID(t *testing.T) {
	old := groupsFunctURL
	t.Cleanup(func() { groupsFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		switch form.Get("action") {
		case "load_info":
			fmt.Fprint(w, `[{"un":0,"id":"28728","year":"2026","err":0}]`)
		case "load":
			if form.Get("vd") == "28728" {
				fmt.Fprint(w, `[["otd","x","","","","otd","АиЭС"],["28728","26","22","10","1","р"]]`)
				return
			}
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	groupsFunctURL = srv.URL + "/shs/gr_all/funct.php"

	got, err := New().ScrapeDepartmentGroups(&model.Department{Name: "АиЭС"})
	if err != nil {
		t.Fatalf("ScrapeDepartmentGroups() error: %v", err)
	}
	if len(got) != 1 || got[0].DepartmentID != "28728" || got[0].Year != 2026 {
		t.Fatalf("ScrapeDepartmentGroups() = %+v, want resolved ID 28728 and year 2026", got)
	}
}

func TestScrapeTeachers(t *testing.T) {
	old := prepFunctURL
	t.Cleanup(func() { prepFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(b))
		if form.Get("action") != "load" || form.Get("act") != "list_prepods" ||
			form.Get("otd") != "0" || form.Get("bs") != "0" {
			t.Errorf("form = %v, want action=load act=list_prepods otd=0 bs=0", form)
		}
		fmt.Fprint(w, `[
			["273","Абайдулина Анна Игоревна"],
			["274","Вакансия"],
			["275",""],
			[""]
		]`)
	}))
	defer srv.Close()
	prepFunctURL = srv.URL + "/shs/prep_n/funct.php"

	got, err := New().ScrapeTeachers()
	if err != nil {
		t.Fatalf("ScrapeTeachers() error: %v", err)
	}

	want := []model.Teacher{{TeacherID: "273", Name: "Абайдулина Анна Игоревна"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ScrapeTeachers() = %+v, want %+v", got, want)
	}
}

func TestScrapeTeachersServerError(t *testing.T) {
	old := prepFunctURL
	t.Cleanup(func() { prepFunctURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `"e"`)
	}))
	defer srv.Close()
	prepFunctURL = srv.URL + "/shs/prep_n/funct.php"

	if _, err := New().ScrapeTeachers(); err == nil {
		t.Fatal("ScrapeTeachers() error = nil, want an error")
	}
}

// teacherSkeletonHTML is a minimal schedule page whose lesson cells are
// filled by getUrok calls, like the site serves for teachers (windows-1251).
const teacherSkeletonHTML = `<html><head><script src="shed.js"></script></head><body>
<table id="main_table">
	<tr>
		<td>№</td>
		<td>Время</td>
		<td class="head_table">01.09.2026<br>Понедельник<br>нечетная</td>
	</tr>
	<tr class="para_num">
		<td>1</td>
		<td>08:30-10:05</td>
		<td class=urok id='ur119'></td>
	</tr>
</table>
<script>getUrok('273','1','1788202800','1','9');</script>
</body></html>`

// encode1251 converts a UTF-8 string to its windows-1251 byte sequence,
// mimicking the encoding the college site actually serves.
func encode1251(t *testing.T, s string) []byte {
	t.Helper()
	enc, err := charmap.Windows1251.NewEncoder().String(s)
	if err != nil {
		t.Fatalf("failed to encode fixture: %v", err)
	}
	return []byte(enc)
}

func TestScrapeScheduleTeacherMergesCells(t *testing.T) {
	urokJSON := `{
		"1":["urok","Русский язык","<br><div style=font-size:10px>ЭРЭр-26-(9)-1","210<\/br>(10)","zamena"],
		"6":["urok","Математика","<br><div style=font-size:10px>ЭРЭр-26-(9)-1","310<\/br>(11)",""]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "Model.php"):
			if task := r.URL.Query().Get("task"); task != "get_urok" {
				t.Errorf("task = %q, want get_urok", task)
			}
			w.Write([]byte(urokJSON))
		default:
			w.Write(encode1251(t, teacherSkeletonHTML))
		}
	}))
	defer srv.Close()

	conf := model.ScheduleConfig{Teacher: &model.Teacher{TeacherID: "273", Name: "Абайдулина"}}
	got, err := New().ScrapeSchedule(srv.URL+"/shs/all_t/sh.php?action=prep", conf)
	if err != nil {
		t.Fatalf("ScrapeSchedule() error: %v", err)
	}

	if len(got.Days) != 1 {
		t.Fatalf("ScrapeSchedule() = %+v, want 1 day", got.Days)
	}
	day := got.Days[0]
	if day.Date != "01.09.2026" || day.Weekday != "Понедельник" {
		t.Errorf("day = %+v, want 01.09.2026 Понедельник", day)
	}
	if len(day.Pairs) != 1 {
		t.Fatalf("pairs = %+v, want 1 pair", day.Pairs)
	}
	pair := day.Pairs[0]
	if pair.Number != 1 {
		t.Errorf("pair.Number = %d, want 1", pair.Number)
	}
	if pair.StartTime != "08:30" || pair.EndTime != "10:05" {
		t.Errorf("pair times = %s-%s, want 08:30-10:05", pair.StartTime, pair.EndTime)
	}
	if !pair.Replaced {
		t.Error("pair.Replaced = false, want true (zamena class)")
	}
	if pair.Discipline != "Русский язык" {
		t.Errorf("pair.Discipline = %q, want %q", pair.Discipline, "Русский язык")
	}
	if pair.Group != "ЭРЭр-26-(9)-1" {
		t.Errorf("pair.Group = %q, want %q", pair.Group, "ЭРЭr-26-(9)-1")
	}
	if !strings.Contains(pair.Classroom, "210") {
		t.Errorf("pair.Classroom = %q, want to contain 210", pair.Classroom)
	}
}

func TestScrapeScheduleGroup(t *testing.T) {
	const html = `<html><body>
			<table id="main_table">
				<tr>
					<td>№</td>
					<td>Время</td>
					<td>01.09.2026<br>Понедельник<br>нечетная</td>
				</tr>
				<tr class="para_num">
					<td>1</td>
					<td>08:30-10:05</td>
					<td class="urok"><div class=disc>Математика</div><div class=cabs>101</div></td>
				</tr>
			</table>
		</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(encode1251(t, html))
	}))
	defer srv.Close()

	conf := model.ScheduleConfig{Group: &model.Group{GroupID: "205"}}
	got, err := New().ScrapeSchedule(srv.URL, conf)
	if err != nil {
		t.Fatalf("ScrapeSchedule() error: %v", err)
	}

	pair := got.Days[0].Pairs[0]
	if pair.Discipline != "Математика" {
		t.Errorf("pair.Discipline = %q, want %q", pair.Discipline, "Математика")
	}
	if pair.Classroom != "101" {
		t.Errorf("pair.Classroom = %q, want 101", pair.Classroom)
	}
}

func TestScrapeScheduleInvalidConfig(t *testing.T) {
	_, err := New().ScrapeSchedule("https://coworking.tyuiu.ru/shs/all_t/sh.php", model.ScheduleConfig{})
	if err == nil {
		t.Fatal("ScrapeSchedule() error = nil, want an error")
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

func TestScraper_CheckVacation(t *testing.T) {
	old := schedulesPageURL
	t.Cleanup(func() { schedulesPageURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	schedulesPageURL = srv.URL + "/shs/index.php"

	s := New()
	got, err := s.CheckVacation()
	if err != nil {
		t.Fatalf("CheckVacation() error: %v", err)
	}
	if !got {
		t.Fatalf("CheckVacation() = %v, want true (403 means vacation)", got)
	}
}

func TestScraper_CheckVacationNotOnVacation(t *testing.T) {
	old := schedulesPageURL
	t.Cleanup(func() { schedulesPageURL = old })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	schedulesPageURL = srv.URL + "/shs/index.php"

	s := New()
	got, err := s.CheckVacation()
	if err != nil {
		t.Fatalf("CheckVacation() error: %v", err)
	}
	if got {
		t.Fatalf("CheckVacation() = %v, want false on 200", got)
	}
}
