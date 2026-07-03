package scraper

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/avast/retry-go/v5"
	"github.com/playwright-community/playwright-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/config"
)

func New(browser *browser.BrowserService) *ScraperService { return &ScraperService{browser: browser} }

type ScraperService struct{ browser *browser.BrowserService }

func (s *ScraperService) ScrapeSchedule(url model.URL, conf model.ScheduleConfig) (*model.RawSchedule, error) {
	const getScheduleRetryAttempts = 5

	resp, err := HTTPGetRequestRetryingWithRandomHeaders(url.String(), getScheduleRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %s", resp.Status)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	fixedEncoding, err := windows1251ToUTF8(string(bytes))
	if err != nil {
		return nil, fmt.Errorf("encoding conversion failed: %w", err)
	}

	if log.Logger.GetLevel() == zerolog.TraceLevel {
		saveScheduleCache(conf, fixedEncoding)
	}

	return parseSchedule(fixedEncoding, conf)
}

func (s *ScraperService) ScrapeScheduleWithBrowser(
	url model.URL,
	conf model.ScheduleConfig,
) (*model.RawSchedule, error) {
	html, err := s.getScheduleHTML(url, conf)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule HTML with browser: %w", err)
	}
	return parseSchedule(html, conf)
}

func (s *ScraperService) ScrapeGroups() ([]model.Group, error) {
	departments, err := s.ScrapeDepartments()
	if err != nil {
		return nil, err
	}

	var groups []model.Group
	for _, d := range departments {
		depGroups, err := s.scrapeDepartmentGroups(&d)
		if err != nil {
			log.Error().Any("department", d).Msg("Failed to scrape groups from the department page")
			continue
		}
		groups = append(groups, depGroups...)
	}
	log.Trace().Int("groups", len(groups)).Msg("Groups parsed")

	return groups, nil
}

const DepartmentSelectionPageURL = "https://mnokol.tyuiu.ru/site/index.php?option=com_content&view=article&id=1582&Itemid=247"
const BaseDepartmentPageURL = "https://mnokol.tyuiu.ru"

func (s *ScraperService) ScrapeDepartments() ([]model.Department, error) {
	const getDepartmentsRetryAttempts = 5

	resp, err := HTTPGetRequestRetryingWithRandomHeaders(DepartmentSelectionPageURL, getDepartmentsRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed loading departments page: %w", err)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
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
			Name: model.DepartmentName(name),
			URL:  model.URL(BaseDepartmentPageURL + strings.ReplaceAll(s.AttrOr("href", ""), "&amp;", "&")),
		})
	})
	return departments, nil
}
func (s *ScraperService) scrapeDepartmentGroups(department *model.Department) ([]model.Group, error) {
	log.Trace().Any("department", department).Msg("Scraping department groups...")

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

const TeachersPageURL = "https://mnokol.tyuiu.ru/site/index.php?option=com_content&view=article&id=1247&Itemid=304"

func (s *ScraperService) ScrapeTeachers() (teachers []model.Teacher, err error) {
	err = s.browser.WithPage(func(p playwright.Page) error {
		if _, err := p.Goto(TeachersPageURL); err != nil {
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
				TeacherID: model.TeacherID(opt["value"].(string)),
				Name:      model.TeacherName(strings.TrimSpace(opt["text"].(string))),
			})
		}
		return nil
	})
	return
}

const maxRetries = 10

func (s *ScraperService) getScheduleHTML(url model.URL, conf model.ScheduleConfig) (string, error) {
	log.Trace().Msg("")

	browser := s.browser
	var html string

	err := retry.New(
		retry.Attempts(maxRetries),
		retry.Delay(100*time.Millisecond),
		retry.DelayType(retry.FullJitterBackoffDelay),
	).Do(func() error {
		headers := GenerateHeaders()
		if errBrowser := browser.WithPage(func(p playwright.Page) error {
			if err := p.SetExtraHTTPHeaders(headers); err != nil {
				return fmt.Errorf("failed to set extra HTTP headers: %w", err)
			}
			if _, err := p.Goto(url.String()); err != nil {
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
			log.Error().Err(errBrowser).Str("url", url.String()).Any("headers", headers).
				Msg("Failed to fetch schedule page")
			return errBrowser
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch schedule page with browser after %d retries: %w", maxRetries, err)
	}

	if log.Logger.GetLevel() == zerolog.TraceLevel {
		saveScheduleCache(conf, html)
	}

	return html, nil
}

func saveScheduleCache(conf model.ScheduleConfig, html string) {
	cacheDir := viper.GetString(config.KeyCacheDir)

	// Ensure directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Warn().Err(err).Msg("Failed to create cache directory")
		return
	}

	// Save HTML to cache directory
	filename := filepath.Join(cacheDir, conf.ScheduleKey()+".html")
	if err := os.WriteFile(filename, []byte(html), 0644); err != nil {
		log.Warn().Err(err).Msg("Failed to save schedule HTML to file")
	} else {
		log.Debug().Msgf("Saved schedule HTML to %s", filename)
	}
}

// ScheduleURL returns formatted URL for group or teacher schedule page depending on the given schedule config.
// Parameter departmentIDs is used for teacher schedule page only and may be empty or nil for group.
//
// Returns empty string if config is invalid.
func ScheduleURL(config model.ScheduleConfig, departmentIDs []model.DepartmentID) model.URL {
	switch {
	case config.Group != nil:
		zFlag := "" // Заочное обучение?
		if strings.Contains(strings.ToLower(config.Group.DepartmentName.String()), "заоч") {
			zFlag = "z"
		}
		return model.URL(fmt.Sprintf(
			"https://coworking.tyuiu.ru/shs/all_t/sh%s.php?action=group&union=0&sid=%s&gr=%s&year=%d&vr=1",
			zFlag, config.Group.DepartmentID, config.Group.GroupID, config.Group.Year))
	case config.Teacher != nil:
		var departmentArgs strings.Builder
		for i, id := range departmentIDs {
			fmt.Fprintf(
				&departmentArgs,
				"&shed[%d]=%s&union[%d]=0&year[%d]=%d",
				i, id, i, i, time.Now().Year(),
				// Note: Here I use current year because I cannot fetch it from DB.
				// Commonly it shouldn't give trouble,
				// because the year in their DB changes at the end of the first semester.
			)
		}
		return model.URL(fmt.Sprintf(
			"https://coworking.tyuiu.ru/shs/all_t/sh.php?action=prep&prep=%s&vr=1&count=%d%s",
			config.Teacher.TeacherID, len(departmentIDs), departmentArgs.String()))
	default:
		// Error: invalid config
		return ""
	}
}

func windows1251ToUTF8(s string) (string, error) {
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
