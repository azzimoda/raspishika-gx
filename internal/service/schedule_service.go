package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/schollz/closestmatch"
	"github.com/spf13/viper"
	"golang.org/x/sync/singleflight"

	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/internal/scraper"
	"github.com/azzimoda/raspishika-gx/pkg/config"
)

func NewScheduleService(
	browserService *browser.BrowserService,
	scraper *scraper.ScraperService,
	scheduleRepo repository.ScheduleRepository,
	groupRepo repository.GroupRepository,
) *ScheduleService {
	return &ScheduleService{
		browser:  browserService,
		scraper:  scraper,
		schedule: scheduleRepo,
		groups:   groupRepo,
		sf:       new(singleflight.Group),
	}
}

type ScheduleService struct {
	browser  *browser.BrowserService
	scraper  *scraper.ScraperService
	schedule repository.ScheduleRepository
	groups   repository.GroupRepository
	sf       *singleflight.Group
}

// GetSchedule returns the schedule for the given config and uses cache if available.
func (s *ScheduleService) GetSchedule(ctx context.Context, conf model.ScheduleConfig) (*model.RawSchedule, error) {
	key := conf.ScheduleKey()
	if rawSchedule, ok := s.GetScheduleCache(key); ok {
		log.Debug().Str("cacheKey", key).Msg("Cache hit")
		return rawSchedule, nil
	}
	log.Debug().Str("cacheKey", key).Msg("Cache miss")
	return s.UpdateScheduleCache(ctx, s.browser, conf)
}

// GetSchedules returns the schedules for the given configs and uses cache if available.
//
// If errors occur on any of the configs, they are accumulated and returned as a single error. Successfully
// processed configs are returned along with any errors that occurred during processing.
func (s *ScheduleService) GetSchedules(ctx context.Context, confs []model.ScheduleConfig) ([]*model.RawSchedule, error) {
	var (
		rawSchedules []*model.RawSchedule
		errs         []error
	)
	for _, conf := range confs {
		rawSchedule, err := s.GetSchedule(ctx, conf)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get schedule: %w", err))
		} else {
			rawSchedules = append(rawSchedules, rawSchedule)
		}
	}
	if len(errs) > 0 {
		return rawSchedules, fmt.Errorf("errors occurred: %w", errors.Join(errs...))
	}
	return rawSchedules, nil
}

// GetScheduleCache checks if the schedule cache is actual for the given key.
// Returns the raw schedule and a boolean indicating if the cache is actual.
//
// When cache exists and is actual, returns the raw schedule and true.
//
// When cache exists and is expired, returns raw schedule and false.
//
// When cache does not exist, returns nil and false.
func (s *ScheduleService) GetScheduleCache(key string) (rawSchedule *model.RawSchedule, ok bool) {
	if scheduleCache, err := s.schedule.GetByKey(context.Background(), key); err == nil {
		rawSchedule, errUnmarshal := scheduleCache.Unmarshal()
		if scheduleCache.IsActual(viper.GetDuration(config.KeyCacheScheduleTTL)) {
			return rawSchedule, errUnmarshal == nil
		}
		log.Trace().Msg("Cache expired for schedule key")
		return rawSchedule, false
	} else if errors.Is(err, sql.ErrNoRows) {
		log.Trace().Err(err).Msg("No cache found for schedule key")
	} else {
		log.Warn().Err(err).Msg("Failed to check schedule cache from DB")
	}
	return nil, false
}
func (s *ScheduleService) UpdateScheduleCache(
	ctx context.Context,
	browser *browser.BrowserService,
	conf model.ScheduleConfig,
) (*model.RawSchedule, error) {
	// Fetch schedule
	key := conf.ScheduleKey()
	result, err, _ := s.sf.Do(key, func() (any, error) { return s.scrapeSchedule(ctx, conf) })
	if err != nil {
		return nil, fmt.Errorf("failed to scrape schedule: %w", err)
	}
	rawSchedule := result.(*model.RawSchedule)

	// Save cache
	scheduleCache, err := model.NewSchedule(key, *rawSchedule)
	if err != nil {
		return rawSchedule, fmt.Errorf("cache not updated: %w", err)
	}

	if err := s.schedule.CreateOrUpdate(context.Background(), scheduleCache); err != nil {
		return rawSchedule, fmt.Errorf("cache not updated: %w", err)
	}
	return rawSchedule, nil
}
func (s *ScheduleService) GetChanges(
	ctx context.Context,
	groupNames []model.GroupName,
) (map[model.GroupName]*model.ScheduleChange, error) {
	changes := make(map[model.GroupName]*model.ScheduleChange)
	var errs []error

	for _, gn := range groupNames {
		group, err := s.GetGroupByName(ctx, gn)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get group: %w", err))
			continue
		}

		conf := model.GroupScheduleConfig(group, false)

		oldRawSchedule, ok := s.GetScheduleCache(conf.ScheduleKey())
		if !ok || oldRawSchedule == nil {
			log.Warn().Err(err).Str("key", conf.ScheduleKey()).Msg("No change for the schedule config")
			if _, err := s.UpdateScheduleCache(ctx, s.browser, conf); err != nil {
				errs = append(errs, fmt.Errorf("failed to update schedule cache: %w", err))
			}
			continue
		}

		newRawSchedule, err := s.UpdateScheduleCache(ctx, s.browser, conf)
		if err != nil {
			log.Error().Err(err).Str("key", conf.ScheduleKey()).Msg("Failed to update schedule cache")
			errs = append(errs, fmt.Errorf("failed to update schedule cache: %w", err))
			continue
		}

		change := model.NewScheduleChange(oldRawSchedule.Transform(), newRawSchedule.Transform())
		diffs := change.Diffs()
		if len(diffs) > 0 {
			log.Debug().Any("conf", conf).Msg("Schedule change detected")
			changes[gn] = change
		}
	}

	return changes, errors.Join(errs...)
}

func (s *ScheduleService) PrepareScheduleImage(
	ctx context.Context,
	rawSchedule *model.RawSchedule,
) (fileName string, bytes []byte, err error) {
	if rawSchedule == nil {
		return "", nil, fmt.Errorf("schedule is nil")
	}

	fileName, bytes, err = s.htmlToImage(rawSchedule.Config, rawSchedule.HTML(getScheduleTemplate(rawSchedule.Config.IsDark)))
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate schedule image: %w", err)
	}
	return fileName, bytes, nil
}

func (s *ScheduleService) EnsureGroups(ctx context.Context) (updated bool, err error) {
	if err := s.EnsureDepartments(ctx); err != nil {
		return false, fmt.Errorf("failed to ensure departments: %w", err)
	}

	actualGroupsCount := 0
	if actualGroups, err := s.groups.GetAllActualGroups(ctx); err == nil {
		actualGroupsCount = len(actualGroups)
	} else {
		log.Debug().Err(err).Msg("Failed to get actual groups from repository, fallback to 0")
	}

	outdatedGroups, err := s.groups.GetOutdatedActualGroups(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check outdated groups: %w", err)
	}

	if actualGroupsCount == 0 || len(outdatedGroups) > 0 {
		if err := s.UpdateGroups(ctx); err != nil {
			return false, fmt.Errorf("failed to udpate groups: %w", err)
		}
		return true, nil
	}

	log.Trace().Msg("Group data is up to date")
	return false, nil
}
func (s *ScheduleService) UpdateGroups(ctx context.Context) error {
	log.Info().Msg("Updating group data...")
	newGroups, err := s.scraper.ScrapeGroups()
	if err != nil {
		return fmt.Errorf("failed to scrape groups: %w", err)
	}

	if err := s.groups.UpdateGroups(ctx, newGroups); err != nil {
		return fmt.Errorf("failed to update groups: %w", err)
	}
	return nil
}
func (s *ScheduleService) GetGroupByName(ctx context.Context, name model.GroupName) (*model.Group, error) {
	return s.groups.GetGroupByName(ctx, name)
}
func (s *ScheduleService) GetGroupsByDepartmentName(ctx context.Context, name string) ([]*model.Group, error) {
	return s.groups.GetDepartmentActualGroups(ctx, name)
}
func (s *ScheduleService) ValidateGroupName(ctx context.Context, name model.GroupName) (model.GroupName, error) {
	return s.groups.ValidateName(ctx, name)
}
func (s *ScheduleService) DeleteAllGroups(ctx context.Context) error {
	log.Warn().Msg("Deleting all groups...")
	return s.groups.DeleteAllGroups(ctx)
}

func (s *ScheduleService) EnsureDepartments(ctx context.Context) error {
	deps, err := s.groups.GetAllDepartments(ctx)
	if err != nil {
		return fmt.Errorf("failed to get departments: %w", err)
	}

	outdatedDeps, err := s.groups.GetOutdatedDepartments(ctx)
	if err != nil {
		return fmt.Errorf("failed to get outdated departments: %w", err)
	}

	if len(deps) == 0 || len(outdatedDeps) > 0 {
		return s.UpdateDepartments(ctx)
	}
	return nil
}
func (s *ScheduleService) UpdateDepartments(ctx context.Context) error {
	log.Info().Msg("Updating departments data...")
	newDeps, err := s.scraper.ScrapeDepartments()
	if err != nil {
		return fmt.Errorf("failed to scrape departments: %w", err)
	}

	for _, d := range newDeps {
		if err := s.groups.InsertOrUpdateDepartment(ctx, &d); err != nil {
			log.Error().Err(err).Any("department", d).Msg("Failed to insert or update department")
		}
	}
	return nil
}
func (s *ScheduleService) GetDepartments(ctx context.Context) ([]model.Department, error) {
	return s.groups.GetAllDepartments(ctx)
}

func (s *ScheduleService) EnsureTeachers(ctx context.Context) error {
	teachers, err := s.groups.GetAllTeachers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get teachers: %w", err)
	}

	outdatedTeachers, err := s.groups.GetOutdatedTeachers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get outdated teachers: %w", err)
	}

	if len(teachers) == 0 || len(outdatedTeachers) > 0 {
		return s.UpdateTeachers(ctx)
	}
	return nil
}
func (s *ScheduleService) UpdateTeachers(ctx context.Context) error {
	log.Info().Msg("Updating teachers data...")
	newTeachers, err := s.scraper.ScrapeTeachers()
	if err != nil {
		return fmt.Errorf("failed to scrape teachers: %w", err)
	}

	return s.groups.UpdateTeachers(ctx, newTeachers)
}
func (s *ScheduleService) GetTeacherByTeacherID(ctx context.Context, teacherID model.TeacherID) (*model.Teacher, error) {
	return s.groups.GetTeacherByID(ctx, teacherID)
}
func (s *ScheduleService) FindTeachersByName(ctx context.Context, name string) ([]*model.Teacher, error) {
	log.Debug().Msg("Finding teachers by name...")

	teachers, err := s.groups.GetAllTeachers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get teachers: %w", err)
	}
	log.Trace().Int("teachers", len(teachers)).Send()

	names := make([]string, len(teachers))
	for i, t := range teachers {
		names[i] = t.Name.String()
	}

	matchedNames := matchStrings(names, name, 5)
	log.Trace().Any("matchedNames", matchedNames).Send()

	matchedTeachers := make([]*model.Teacher, len(matchedNames))
	for i, n := range matchedNames {
		for _, t := range teachers {
			if t.Name.String() == n {
				matchedTeachers[i] = t
				break
			}
		}
	}
	log.Trace().Any("matchedTeachers", matchedTeachers).Send()
	return matchedTeachers, nil
}
func (s *ScheduleService) GetChatRecentTeachers(ctx context.Context, chatID int64) ([]*model.Teacher, error) {
	return s.groups.GetChatRecentTeachers(ctx, chatID)
}

func (s *ScheduleService) htmlToImage(conf model.ScheduleConfig, html string) (string, []byte, error) {
	imageFileName := path.Join(viper.GetString(config.KeyScreenshotDir), scheduleScreenshotFileName(conf))
	if err := s.browser.TakeScreenshotHTML(html, imageFileName); err != nil {
		return "", nil, fmt.Errorf("failed to take screenshot: %w", err)
	}

	imageData, err := os.ReadFile(imageFileName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read screenshot: %w", err)
	}
	return imageFileName, imageData, nil
}

func (s *ScheduleService) scrapeSchedule(
	ctx context.Context,
	conf model.ScheduleConfig,
) (*model.RawSchedule, error) {
	departmentIDs, err := s.groups.GetAllDepartmentIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get department IDs: %w", err)
	}

	url := scraper.ScheduleURL(conf, departmentIDs)
	if conf.Group != nil {
		return s.scraper.ScrapeSchedule(url, conf)
	} else if conf.Teacher != nil {
		return s.scraper.ScrapeScheduleWithBrowser(url, conf)
	} else {
		return nil, fmt.Errorf("invalid schedule config")
	}
}

func getScheduleTemplate(is_dark bool) string {
	fileKey := config.KeyScheduleTemplateFile
	if is_dark {
		fileKey = config.KeyScheduleTemplateDarkFile
	}
	key := config.KeyScheduleTemplate
	if is_dark {
		key = config.KeyScheduleTemplateDark
	}

	templateFile := viper.GetString(fileKey)
	bytes, err := os.ReadFile(templateFile)
	template := ""
	if err != nil {
		log.Error().Err(err).Str("templateFile", templateFile).Msg("Failed to load template file")
	} else {
		template = string(bytes)
		viper.Set(key, template)
	}

	if err != nil {
		return viper.GetString(key)
	}
	return template
}

func scheduleScreenshotFileName(conf model.ScheduleConfig) string { return conf.ImageKey() + ".png" }

// matchStrings returns the closest matches for a given target string from a list of strings.
func matchStrings(strs []string, target string, n int) []string {
	for _, s := range strs {
		if strings.EqualFold(s, target) {
			return []string{s}
		}
	}
	return closestmatch.New(strs, []int{2, 3, 4}).ClosestN(target, n)
}
