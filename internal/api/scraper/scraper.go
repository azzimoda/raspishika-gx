// Package scraper fetches and parses schedule data from the college website.
package scraper

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/SyNdicateFoundation/legitagent"
	"github.com/avast/retry-go/v5"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/corpix/uarand"
	"github.com/mxschmitt/playwright-go"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

// BrowserProvider abstracts a Playwright browser that can open pages.
type BrowserProvider interface {
	WithPage(func(page playwright.Page) error) error
}

// New creates a Scraper backed by the given browser provider.
func New(b BrowserProvider) *Scraper { return &Scraper{browser: b} }

// Scraper retrieves data from the college website.
type Scraper struct{ browser BrowserProvider }

// ErrServiceUnavailable is returned when the college site is unavailable,
// i.e. it is in vacation mode.
var ErrServiceUnavailable = errors.New("service unavailable")

// CheckVacation checks if the college site is in vacation mode.
func (s *Scraper) CheckVacation() (bool, error) {

	log.Debug().Msg("Checking vacation status...")

	const schedulesPageURL = "https://coworking.tyuiu.ru/shs/index.php"

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

// ScrapeDepartments scrapes the list of departments from the website.
func (s *Scraper) ScrapeDepartments() ([]model.Department, error) {

	log.Debug().Msg("Scraping departments...")

	if isVacation, err := s.CheckVacation(); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	const getDepartmentsRetryAttempts = 5
	const departmentSelectionPageURL = "https://mnokol.tyuiu.ru/site/index.php?option=com_content&view=article&id=1582&Itemid=247"

	resp, err := httpGetRequestRetryingWithRandomHeaders(departmentSelectionPageURL, getDepartmentsRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed loading departments page: %w", err)
	}
	defer resp.Body.Close()

	return parseDepartments(resp.Body)
}
func parseDepartments(r io.Reader) ([]model.Department, error) {

	const baseDepartmentPageURL = "https://mnokol.tyuiu.ru"

	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse departments page: %w", err)
	}

	var departments []model.Department
	doc.Find("ul.mod-menu li.col-lg.col-md-6 a").Each(func(i int, s *goquery.Selection) {
		name := s.Text()
		if !strings.Contains(strings.ToLower(name), "отделение") && !strings.Contains(strings.ToLower(name), "заоч") {
			return
		}

		departments = append(departments, model.Department{
			Name: name,
			URL:  baseDepartmentPageURL + strings.ReplaceAll(s.AttrOr("href", ""), "&amp;", "&"),
		})
	})
	return departments, nil
}

// ScrapeDepartmentGroups scrapes the list of groups for a department.
func (s *Scraper) ScrapeDepartmentGroups(department *model.Department) ([]model.Group, error) {

	log.Debug().Any("department", department).Msg("Scraping department groups...")

	if isVacation, err := s.CheckVacation(); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	groups := make([]model.Group, 0)
	err := s.browser.WithPage(func(p playwright.Page) error {
		gs, err := parseDepartmentGroups(p, department)
		if err != nil {
			return err
		}
		groups = append(groups, gs...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return groups, nil
}
func parseDepartmentGroups(p playwright.Page, department *model.Department) ([]model.Group, error) {

	log.Trace().Msg("Navigating to department page")
	if _, err := p.Goto(department.URL); err != nil {
		return nil, fmt.Errorf("failed to navigate to department page: %w", err)
	}

	frameLocator := p.FrameLocator("div.com-content-article__body iframe")
	if err := frameLocator.Locator("#groups").WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(60_000),
	}); err != nil {
		return nil, fmt.Errorf("failed to wait for groups iframe: %w", err)
	}

	options, err := frameLocator.Locator("#groups option").EvaluateAll(
		`els => els.map(el => ({ text: el.textContent.trim(), value: el.value, sid: el.getAttribute("sid"), year: el.getAttribute("year") }))`)
	if err != nil {
		return nil, fmt.Errorf("failed to get groups options: %w", err)
	}

	var groups []model.Group
	for _, opt := range options.([]any) {
		opt := opt.(map[string]any)
		if !(validateOptionValue(opt["value"]) &&
			validateOptionValue(opt["text"]) &&
			validateOptionValue(opt["sid"]) &&
			validateOptionValue(opt["year"])) {
			log.Trace().Msg("Option is invalid")

			continue
		}

		year, err := strconv.ParseInt(opt["year"].(string), 10, 64)
		if err != nil {
			continue
		}

		groups = append(groups, model.Group{
			GroupID:        opt["value"].(string),
			DepartmentID:   opt["sid"].(string),
			GroupName:      model.GroupName(opt["text"].(string)),
			Year:           int(year),
			DepartmentName: department.Name,
		})
	}
	return groups, nil
}

// ScrapeTeachers scrapes the list of all teachers.
func (s *Scraper) ScrapeTeachers() (teachers []model.Teacher, err error) {

	log.Debug().Msg("Scraping teachers...")

	if isVacation, err := s.CheckVacation(); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	const teachersPageURL = "https://mnokol.tyuiu.ru/site/index.php?option=com_content&view=article&id=1247&Itemid=304"

	err = s.browser.WithPage(func(p playwright.Page) error {
		if _, err := p.Goto(teachersPageURL); err != nil {
			return fmt.Errorf("failed to goto teachers page: %w", err)
		}

		iframeLocator := p.FrameLocator("div.com-content-article__body iframe")
		selectLocator := iframeLocator.Locator("#preps")
		if err := selectLocator.WaitFor(playwright.LocatorWaitForOptions{}); err != nil {
			return fmt.Errorf("failed to wait for teacher select: %w", err)
		}

		options, err := iframeLocator.Locator("#preps option").EvaluateAll(
			`els => els.map(el => ({ text: el.textContent.trim(), value: el.value }))`)
		if err != nil {
			return fmt.Errorf("failed to get teacher options: %w", err)
		}

		for _, opt := range options.([]any) {
			opt := opt.(map[string]any)
			if opt["value"] == nil || opt["text"] == nil {
				continue
			}

			teachers = append(teachers, model.Teacher{
				TeacherID: opt["value"].(string),
				Name:      strings.TrimSpace(opt["text"].(string)),
			})
		}
		return nil
	})
	return
}

// ScrapeSchedule fetches the page at url and parses the schedule for conf.
func (s *Scraper) ScrapeSchedule(url string, conf model.ScheduleConfig) (*model.ScheduleData, error) {

	log.Debug().Msg("Scraping schedule...")

	if isVacation, err := s.CheckVacation(); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	const attempts = 5

	var html string
	if conf.Group != nil {
		resp, err := httpGetRequestRetryingWithRandomHeaders(url, attempts)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("request failed with status %s", resp.Status)
		}
		defer resp.Body.Close()

		bytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		fixedEncoding, err := encodeWindows1251ToUTF8(string(bytes))
		if err != nil {
			return nil, fmt.Errorf("encoding conversion failed: %w", err)
		}
		html = fixedEncoding
	} else if conf.Teacher != nil {
		// TODO: Try to bypass the restrictions and remove the browser entrirely later.
		var err error
		html, err = s.getScheduleHTMLWithBrowser(url)
		if err != nil {
			return nil, fmt.Errorf("failed to get schedule HTML with browser: %w", err)
		}
	} else {
		return nil, fmt.Errorf("invalid schedule config: %+v", conf)
	}

	return parseSchedule(html, conf)
}
func (s *Scraper) getScheduleHTMLWithBrowser(url string) (string, error) {
	const retries = 5

	log.Trace().Msg("")

	browser := s.browser
	var html string

	err := retry.New(
		retry.Attempts(retries),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.FullJitterBackoffDelay),
	).Do(func() error {
		headers := generateHeaders()
		if errBrowser := browser.WithPage(func(p playwright.Page) error {
			if err := p.SetExtraHTTPHeaders(headers); err != nil {
				return fmt.Errorf("failed to set extra HTTP headers: %w", err)
			}
			if _, err := p.Goto(url); err != nil {
				return fmt.Errorf("failed to goto '%s': %w", url, err)
			}
			tableLocator := p.Locator("#main_table")
			if err := tableLocator.WaitFor(playwright.LocatorWaitForOptions{}); err != nil {
				return fmt.Errorf("failed to wait for table: %w", err)
			}
			time.Sleep(1 * time.Second)

			var errContent error
			html, errContent = p.Content()
			if errContent != nil {
				return fmt.Errorf("failed to get page content: %w", errContent)
			}
			return nil
		}); errBrowser != nil {
			log.Error().Err(errBrowser).Str("url", url).Any("headers", headers).Msg("Failed to fetch schedule page")
			return errBrowser
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch schedule page with browser after %d retries: %w", retries, err)
	}

	return html, nil
}

// ScheduleURL builds the URL of the schedule page for the given config
// and department IDs. For a group the department IDs are not needed,
// for a teacher they are used to build the "shed" query arguments.
func ScheduleURL(conf model.ScheduleConfig, departmentIDs []string) string {
	switch {
	case conf.Group != nil:
		zFlag := "" // Заочное обучение?
		if strings.Contains(strings.ToLower(conf.Group.DepartmentName), "заоч") {
			zFlag = "z"
		}
		return fmt.Sprintf(
			"https://coworking.tyuiu.ru/shs/all_t/sh%s.php?action=group&union=0&sid=%s&gr=%s&year=%d&vr=1",
			zFlag, conf.Group.DepartmentID, conf.Group.GroupID, conf.Group.Year)
	case conf.Teacher != nil:
		var departmentArgs strings.Builder
		for i, id := range departmentIDs {
			fmt.Fprintf(
				&departmentArgs,
				"&shed[%d]=%s&union[%d]=0&year[%d]=%d",
				i, id, i, i, time.Now().Year(),
				// Note: Here I use current year because I cannot fetch it from DB.
				// Commonly it shouldn't give trouble,
				// because the year in their DB changes at the end of the each semester.
			)
		}
		return fmt.Sprintf(
			"https://coworking.tyuiu.ru/shs/all_t/sh.php?action=prep&prep=%s&vr=1&count=%d%s",
			conf.Teacher.TeacherID, len(departmentIDs), departmentArgs.String())
	default:
		// Error: invalid config
		return ""
	}
}

func httpGetRequestRetryingWithRandomHeaders(url string, attempts int) (*http.Response, error) {
	var resp *http.Response
	err := retry.New(
		retry.Attempts(uint(attempts)),
		retry.Delay(time.Second),
		retry.DelayType(retry.FullJitterBackoffDelay),
		retry.OnRetry(func(attempt uint, err error) {
			log.Error().Err(err).Str("url", url).Msgf("retry attempt %d", attempt)
		}),
	).Do(func() (errReq error) {
		headers := generateHeaders()
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

func validateOptionValue(value any) bool {
	if value == nil {
		return false
	}
	if s, ok := value.(string); !ok {
		return false
	} else {
		return s != ""
	}
}
