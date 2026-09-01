// Package scraper fetches and parses schedule data from the college website.
package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SyNdicateFoundation/legitagent"
	"github.com/avast/retry-go/v5"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/corpix/uarand"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

const (
	// modelUrokURL serves per-day lesson data for teacher schedules.
	modelUrokURL = "https://coworking.tyuiu.ru/shs/all_t/Model.php"

	// retryAttempts is how many times an HTTP request is retried.
	retryAttempts = 5
)

// Package-level endpoint URLs (vars so tests can point them at httptest).
var (
	// groupsFunctURL loads the lists of departments and groups.
	groupsFunctURL = "https://coworking.tyuiu.ru/shs/gr_all/funct.php"
	// prepFunctURL loads the list of teachers.
	prepFunctURL = "https://coworking.tyuiu.ru/shs/prep_n/funct.php"
	// schedulesPageURL is the index page used to check vacation status.
	schedulesPageURL = "https://coworking.tyuiu.ru/shs/index.php"
)

// Scraper retrieves data from the college website over plain HTTP.
type Scraper struct{}

// New creates a Scraper.
func New() *Scraper { return &Scraper{} }

// CheckVacation checks if the college site is in vacation mode.
func (s *Scraper) CheckVacation() (bool, error) {

	log.Debug().Msg("Checking vacation status...")

	resp, err := httpGetRequestWithHeaders(schedulesPageURL, generateHeaders())
	if err != nil {
		return false, fmt.Errorf("failed to check vacation status: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusForbidden: // Means today is vacation
		log.Debug().Msg("College site is in vacation mode")
		return true, nil
	case http.StatusOK:
		log.Trace().Msg("College site is not in vacation mode")
		return false, nil
	default:
		log.Warn().Int("statusCode", resp.StatusCode).Msg("unexpected status code")
		return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
}

// loadInfoItem is a single entry of the action=load_info response.
type loadInfoItem struct {
	ID   string `json:"id"`
	Year string `json:"year"`
	Err  int    `json:"err"`
}

// ScrapeDepartments scrapes the list of departments from the website.
func (s *Scraper) ScrapeDepartments() ([]model.Department, error) {

	log.Debug().Msg("Scraping departments...")

	body, err := postFormRetry(groupsFunctURL, url.Values{"action": {"load_info"}}, retryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch departments: %w", err)
	}

	var items []loadInfoItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("failed to parse departments: %w", err)
	}

	var departments []model.Department
	var errs []error
	for _, item := range items {
		if item.Err != 0 {
			continue
		}
		dep, err := s.departmentByID(item)
		if err != nil {
			errs = append(errs, fmt.Errorf("department %s: %w", item.ID, err))
			continue
		}
		departments = append(departments, dep)
	}
	if len(departments) == 0 {
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		return nil, errors.New("no departments found")
	}

	sort.Slice(departments, func(i, j int) bool { return departments[i].Name < departments[j].Name })

	return departments, nil
}

// departmentByID fetches the department name for the given load_info entry.
func (s *Scraper) departmentByID(item loadInfoItem) (model.Department, error) {

	body, err := postFormRetry(groupsFunctURL, url.Values{
		"action": {"load"}, "act": {"list_groups"}, "otd": {"0"}, "vd": {item.ID},
	}, retryAttempts)
	if err != nil {
		return model.Department{}, fmt.Errorf("failed to fetch groups: %w", err)
	}

	rows, err := unmarshalRows(body)
	if err != nil {
		return model.Department{}, err
	}

	name, _ := parseDepartmentRows(rows)
	if name == "" {
		return model.Department{}, errors.New("department header row not found")
	}

	year, _ := strconv.Atoi(item.Year)
	return model.Department{ID: item.ID, Name: name, Year: year}, nil
}

// ScrapeDepartmentGroups scrapes the list of groups for a department.
func (s *Scraper) ScrapeDepartmentGroups(department *model.Department) ([]model.Group, error) {

	log.Debug().Any("department", department).Msg("Scraping department groups...")

	if department == nil {
		return nil, errors.New("department is nil")
	}

	// A department loaded from an old cache might lack the new ID/Year.
	if department.ID == "" {
		var err error
		department, err = s.resolveDepartment(department)
		if err != nil {
			return nil, err
		}
	}

	body, err := postFormRetry(groupsFunctURL, url.Values{
		"action": {"load"}, "act": {"list_groups"}, "otd": {"0"}, "vd": {department.ID},
	}, retryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch groups of department %q: %w", department.Name, err)
	}

	rows, err := unmarshalRows(body)
	if err != nil {
		return nil, err
	}

	name, groupRows := parseDepartmentRows(rows)
	if name == "" {
		name = department.Name
	}

	groups := make([]model.Group, 0, len(groupRows))
	for _, r := range groupRows {
		groups = append(groups, model.Group{
			GroupID:        r[0],
			DepartmentID:   department.ID,
			GroupName:      model.GroupName(model.NormalizeCyrillicLookalikes(groupTitle(r))),
			DepartmentName: name,
			Year:           department.Year,
		})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].GroupName < groups[j].GroupName })

	return groups, nil
}

// resolveDepartment finds the up-to-date department (with ID and Year) by name.
func (s *Scraper) resolveDepartment(department *model.Department) (*model.Department, error) {
	departments, err := s.ScrapeDepartments()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve department %q: %w", department.Name, err)
	}
	for _, d := range departments {
		if strings.EqualFold(d.Name, department.Name) {
			return &d, nil
		}
	}
	return nil, fmt.Errorf("department %q not found", department.Name)
}

func unmarshalRows(body []byte) ([][]string, error) {
	var rows [][]string
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("failed to parse groups: %w", err)
	}
	return rows, nil
}

// parseDepartmentRows splits the rows of a list_groups response into the
// department name (from the header row) and the group rows.
func parseDepartmentRows(rows [][]string) (name string, groupRows [][]string) {
	for _, r := range rows {
		if len(r) >= 7 && r[5] == "otd" && strings.TrimSpace(r[6]) != "" {
			name = strings.TrimSpace(r[6])
			continue
		}
		if len(r) >= 2 && strings.TrimSpace(r[1]) != "" {
			groupRows = append(groupRows, r)
		}
	}
	return name, groupRows
}

// groupTitle builds a group name from its row: "XXX-YY-(Z)-N".
func groupTitle(r []string) string {
	if len(r) < 5 {
		return strings.TrimSpace(r[1])
	}
	return fmt.Sprintf("%s-%s-(%s)-%s",
		strings.TrimSpace(r[1]), strings.TrimSpace(r[2]),
		strings.TrimSpace(r[3]), strings.TrimSpace(r[4]))
}

// ScrapeTeachers scrapes the list of all teachers.
func (s *Scraper) ScrapeTeachers() ([]model.Teacher, error) {

	log.Debug().Msg("Scraping teachers...")

	body, err := postFormRetry(prepFunctURL, url.Values{
		"action": {"load"}, "act": {"list_prepods"}, "otd": {"0"}, "bs": {"0"},
	}, retryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch teachers: %w", err)
	}

	if body := strings.TrimSpace(string(body)); body == `"e"` {
		return nil, errors.New("failed to fetch teachers: server returned an error")
	}

	var pairs [][]string
	if err := json.Unmarshal(body, &pairs); err != nil {
		return nil, fmt.Errorf("failed to parse teachers: %w", err)
	}

	teachers := make([]model.Teacher, 0, len(pairs))
	for _, p := range pairs {
		if len(p) < 2 {
			continue
		}
		id, name := strings.TrimSpace(p[0]), strings.TrimSpace(p[1])
		if id == "" || name == "" {
			continue
		}
		switch name {
		case "Вакансия", "Вакансия 1", "молодая", "молодая 1":
			continue
		}
		teachers = append(teachers, model.Teacher{TeacherID: id, Name: name})
	}

	return teachers, nil
}

// ScrapeSchedule fetches the page at url and parses the schedule for conf.
func (s *Scraper) ScrapeSchedule(url string, conf model.ScheduleConfig) (*model.ScheduleData, error) {

	log.Debug().Msg("Scraping schedule...")

	var html string
	var err error

	switch {
	case conf.Group != nil:
		html, err = s.fetchSchedulePage(url)
	case conf.Teacher != nil:
		html, err = s.getScheduleForTeacher(url)
	default:
		return nil, fmt.Errorf("invalid schedule config: %+v", conf)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schedule: %w", err)
	}

	return parseSchedule(html, conf)
}

// getScheduleForTeacher fetches the teacher schedule page and fills the
// lesson cells as the site JS (getUrok) does, from Model.php responses.
func (s *Scraper) getScheduleForTeacher(scheduleURL string) (string, error) {

	log.Trace().Msg("Fetching teacher schedule page")

	html, err := s.fetchSchedulePage(scheduleURL)
	if err != nil {
		return "", err
	}

	modelURL := modelEndpoint(scheduleURL)
	matches := getUrokScriptRE.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		pid, chisl, run, dn, ms := m[1], m[2], m[3], m[4], m[5]
		data, err := s.fetchUrok(modelURL, pid, chisl, run)
		if err != nil {
			return "", fmt.Errorf("failed to fetch urok data for pid=%s chisl=%s run=%s: %w", pid, chisl, run, err)
		}
		for u := 1; u <= 7; u++ {
			cell, ok := data[strconv.Itoa(u)]
			if !ok || len(cell) < 5 {
				continue
			}
			var cellHTML string
			switch cell[0] {
			case "urok":
				cellHTML = buildUrokCell(cell)
			case "ekz":
				cellHTML = buildEkzCell(cell)
			default:
				continue
			}
			id := fmt.Sprintf("ur%d%s%s", u, dn, ms)
			old := "<td class=urok id='" + id + "'></td>"
			if strings.Contains(html, old) {
				html = strings.Replace(html, old, "<td class=urok id='"+id+"'>"+cellHTML+"</td>", 1)
			}
		}
	}
	return html, nil
}

// getUrokScriptRE matches getUrok('pid','chisl','run','dn','ms'); calls
// embedded in the teacher schedule page.
var getUrokScriptRE = regexp.MustCompile(`getUrok\('([^']*)','([^']*)','([^']*)','([^']*)','([^']*)'\)`)

// modelEndpoint derives the Model.php endpoint from a schedule page URL,
// so it works against the real site and in tests alike.
func modelEndpoint(scheduleURL string) string {
	u, err := url.Parse(scheduleURL)
	if err != nil {
		return modelUrokURL
	}
	return fmt.Sprintf("%s://%s/shs/all_t/Model.php", u.Scheme, u.Host)
}

// buildUrokCell builds the HTML of a regular lesson cell, mirroring the
// getUrok function of the site's all_t/shed.js.
func buildUrokCell(row []string) string {
	return "<table class='comm3 " + row[4] + "'><tr><td><div class=disc>" +
		row[1] + "<br>" + row[2] + "</div></td><td class=cabs><div class=cab>" +
		row[3] + "</div></td></tr></table>"
}

// buildEkzCell builds the HTML of an exam/consultation cell, mirroring the
// getUrok function of the site's all_t/shed.js.
func buildEkzCell(row []string) string {
	podgrupp := ""
	if len(row) >= 6 {
		podgrupp = row[5]
	}
	return "<table class='comm3 " + row[4] + "'><tr><td class=head_ekz>" +
		row[1] + "</td><td rowspan=2 class=cabs><div class=cab>" +
		row[3] + "</div></td></tr><tr><td><div class=disc>" +
		row[2] + "</div><div class=podgrupp>" + podgrupp + "</div></td></tr></table>"
}

// fetchSchedulePage downloads the schedule page and converts it to UTF-8.
func (s *Scraper) fetchSchedulePage(scheduleURL string) (string, error) {

	resp, err := httpGetRequestRetryingWithRandomHeaders(scheduleURL, retryAttempts)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return "", fmt.Errorf("request failed with status %s", resp.Status)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	fixedEncoding, err := encodeWindows1251ToUTF8(string(bytes))
	if err != nil {
		return "", fmt.Errorf("encoding conversion failed: %w", err)
	}

	return fixedEncoding, nil
}

// fetchUrok fetches the lesson data for a single day of a teacher schedule.
func (s *Scraper) fetchUrok(modelURL, pid, chisl, run string) (map[string][]string, error) {

	urokURL := fmt.Sprintf("%s?task=get_urok&format=row&p=%s&c=%s&r=%s", modelURL, pid, chisl, run)

	resp, err := httpGetRequestRetryingWithRandomHeaders(urokURL, retryAttempts, "model")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %s", resp.Status)
	}
	defer resp.Body.Close()

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var data map[string][]string
	if err := json.Unmarshal(bytes, &data); err != nil {
		return nil, fmt.Errorf("failed to parse urok data: %w", err)
	}

	return data, nil
}

// minimalHeaders mimics a plain XHR and avoids the site's anti-bot rule that
// returns 403 when a Model.php request carries browser navigation headers or
// a legacy user agent. Only a modern UA and a Referer are sent.
func minimalHeaders() map[string]string {
	return map[string]string{
		"User-Agent": modernUserAgent(),
		"Referer":    "https://coworking.tyuiu.ru/shs/all_t/",
	}
}

// modernUserAgents are real, current browser strings; the college site
// rejects old/legacy user agents (e.g. Firefox 3.6) with a 403.
var modernUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:127.0) Gecko/20100101 Firefox/127.0",
}

func modernUserAgent() string {
	return modernUserAgents[rand.Intn(len(modernUserAgents))]
}

// httpGetRequestRetryingWithRandomHeaders retries an HTTP GET until it succeeds
// or the attempts run out. By default full random browser headers are sent;
// pass "model" as headerKind for the minimal header set that the Model.php
// endpoint accepts.
func httpGetRequestRetryingWithRandomHeaders(url string, attempts int, headerKinds ...string) (*http.Response, error) {
	headersFor := generateHeaders
	if len(headerKinds) > 0 && headerKinds[0] == "model" {
		headersFor = minimalHeaders
	}

	var resp *http.Response
	err := retry.New(
		retry.Attempts(uint(attempts)),
		retry.Delay(time.Second),
		retry.DelayType(retry.FullJitterBackoffDelay),
		retry.OnRetry(func(attempt uint, err error) {
			log.Error().Err(err).Str("url", url).Msgf("retry attempt %d", attempt)
		}),
	).Do(func() (errReq error) {
		headers := headersFor()
		resp, errReq = httpGetRequestWithHeaders(url, headers)
		if errReq != nil {
			e := log.Error().Err(errReq).Str("url", url).Any("headers", headers)
			if resp != nil {
				e = e.Str("status", resp.Status)
			}
			e.Msg("HTTP GET request failed")
			return errReq
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return nil
		case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests:
			errReq = retry.Unrecoverable(fmt.Errorf("unexpected status: %s", resp.Status))
		default:
			errReq = fmt.Errorf("unexpected status: %s", resp.Status)
		}

		log.Error().Err(errReq).Str("url", url).Any("headers", headers).Str("status", resp.Status).
			Msg("HTTP GET request failed")
		return errReq
	})
	return resp, err
}

// postFormRetry POSTs the form until it succeeds or the attempts run out.
func postFormRetry(url string, form url.Values, attempts int) ([]byte, error) {
	var body []byte
	err := retry.New(
		retry.Attempts(uint(attempts)),
		retry.Delay(time.Second),
		retry.DelayType(retry.FullJitterBackoffDelay),
		retry.OnRetry(func(attempt uint, err error) {
			log.Error().Err(err).Str("url", url).Msgf("retry attempt %d", attempt)
		}),
	).Do(func() (errReq error) {
		b, errReq := postForm(url, form)
		if errReq != nil {
			log.Error().Err(errReq).Str("url", url).Msg("HTTP POST request failed")
			return errReq
		}
		body = b
		return nil
	})
	return body, err
}

// postForm sends a POST form to the given url and returns the response body.
func postForm(url string, form url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Add("X-Requested-With", "XMLHttpRequest")
	for key, value := range generateHeaders() {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return io.ReadAll(resp.Body)
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests:
		return nil, retry.Unrecoverable(fmt.Errorf("unexpected status: %s", resp.Status))
	default:
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}
}

var laGenerator = legitagent.NewGenerator()

func generateHeaders() map[string]string {
	agent, err := laGenerator.Generate()
	if err != nil {
		// Fallback to uarand
		log.Warn().Err(err).Msg("Failed to generate legit agent, falling back to uarand")
		return map[string]string{"User-Agent": uarand.GetRandom(), "Referer": "https://coworking.tyuiu.ru/shs/all_t/"}
	}
	defer laGenerator.ReleaseAgent(agent)

	headers := map[string]string{"User-Agent": agent.UserAgent, "Referer": "https://coworking.tyuiu.ru/shs/all_t/"}
	for k := range agent.Headers {
		headers[k] = agent.Headers.Get(k)
	}
	return headers
}

func httpGetRequestWithHeaders(url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func encodeWindows1251ToUTF8(s string) (string, error) {

	decoder := charmap.Windows1251.NewDecoder()
	reader := transform.NewReader(strings.NewReader(s), decoder)
	result, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(result), nil
}
