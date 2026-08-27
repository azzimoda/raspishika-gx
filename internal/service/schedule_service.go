package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/azzimoda/raspishika-gx/internal/apiclient"
	"github.com/azzimoda/raspishika-gx/internal/browser"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/azzimoda/raspishika-gx/pkg/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type APIClient interface {
	GetDepartments(context.Context) ([]model.Department, error)
	GetGroup(ctx context.Context, name string) (*model.Group, error)
	GetGroups(ctx context.Context, departmentName string) ([]model.Group, error)

	GetTeacher(ctx context.Context, nameOrID string) (*model.Teacher, error)
	SearchTeachers(context.Context, string) ([]model.Teacher, error)
	GetTeachers(context.Context) ([]model.Teacher, error)
	GetSchedule(context.Context, *apiclient.GetScheduleParams) (schedule *model.ScheduleData, err error)
}

func NewScheduleService(
	scraperAPI APIClient,
	browser *browser.ChromedpBrowser,
	scheduleRepo repository.ScheduleRepository,
) *ScheduleService {
	return &ScheduleService{
		scraper:  scraperAPI,
		browser:  browser,
		schedule: scheduleRepo,
		sf:       new(singleflight.Group),
	}
}

type ScheduleService struct {
	scraper  APIClient
	browser  *browser.ChromedpBrowser
	schedule repository.ScheduleRepository
	sf       *singleflight.Group
}

// GetSchedule returns the schedule for the given config and uses cache if available.
func (s *ScheduleService) GetSchedule(
	ctx context.Context, conf model.ScheduleConfig,
) (schedule *model.ScheduleData, err error) {
	schedule, err = s.scraper.GetSchedule(ctx, scheduleConfigToAPIParams(&conf))
	if err != nil {
		return
	}
	schedule.Config = conf
	return
}

// GetSchedules returns the schedules for the given configs and uses cache if available.
func (s *ScheduleService) GetSchedules(
	ctx context.Context, confs []model.ScheduleConfig,
) ([]*model.ScheduleData, error) {
	schedules := make([]*model.ScheduleData, len(confs))
	errs := make([]error, len(confs))

	for i, conf := range confs {
		schedule, err := s.GetSchedule(ctx, conf)
		if err != nil {
			errs[i] = fmt.Errorf("failed to get schedule: %w", err)
		} else {
			schedules[i] = schedule
		}
	}
	if len(errs) > 0 {
		return schedules, fmt.Errorf("errors occurred: %w", errors.Join(errs...))
	}
	return schedules, nil
}

// GetScheduleCache checks if the schedule cache is actual for the given key.
// Returns the raw schedule and a boolean indicating if the cache is actual.
//
// When cache exists and is actual, returns the raw schedule and true.
//
// When cache exists and is expired, returns raw schedule and false.
//
// When cache does not exist, returns nil and false.
func (s *ScheduleService) GetScheduleCache(ctx context.Context, key string) (rawSchedule *model.ScheduleData, ok bool) {

	if scheduleCache, err := s.schedule.GetByKey(ctx, key); err == nil {
		schedule, errUnmarshal := scheduleCache.Unmarshal()
		if scheduleCache.IsActual(viper.GetDuration(config.KeyCacheScheduleTTL)) {
			return schedule, errUnmarshal == nil
		}
		log.Trace().Msg("Cache expired for schedule key")
		return schedule, false
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Trace().Err(err).Msg("No cache found for schedule key")
	} else {
		log.Warn().Err(err).Msg("Failed to check schedule cache from DB")
	}
	return nil, false
}
func (s *ScheduleService) UpdateScheduleCache(
	ctx context.Context, conf model.ScheduleConfig,
) (*model.ScheduleData, error) {
	// Fetch schedule

	key := conf.ScheduleKey()
	result, err, _ := s.sf.Do(key, func() (any, error) {
		sch, err := s.scraper.GetSchedule(ctx, scheduleConfigToAPIParams(&conf))
		return sch, err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scrape schedule: %w", err)
	}
	rawSchedule := result.(*model.ScheduleData)

	// Save cache

	scheduleCache, err := model.NewSchedule(key, *rawSchedule)
	if err != nil {
		return rawSchedule, fmt.Errorf("cache not updated: %w", err)
	}

	if err := s.schedule.CreateOrUpdate(ctx, scheduleCache); err != nil {
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

		oldRawSchedule, ok := s.GetScheduleCache(ctx, conf.ScheduleKey())
		if !ok || oldRawSchedule == nil {
			log.Warn().Err(err).Str("key", conf.ScheduleKey()).Msg("No change for the schedule config")
			if _, err := s.UpdateScheduleCache(ctx, conf); err != nil {
				errs = append(errs, fmt.Errorf("failed to update schedule cache: %w", err))
			}
			continue
		}

		newSchedule, err := s.UpdateScheduleCache(ctx, conf)
		if err != nil {
			log.Error().Err(err).Str("key", conf.ScheduleKey()).Msg("Failed to update schedule cache")
			errs = append(errs, fmt.Errorf("failed to update schedule cache: %w", err))
			continue
		}

		change := model.NewScheduleChange(*oldRawSchedule, *newSchedule)
		if change == nil {
			log.Warn().Msg("Unexpected different schedule configs; supposed to be unreachable")
			continue
		}

		diffs := change.Diffs()
		if len(diffs) > 0 {
			log.Debug().Any("conf", conf).Msg("Schedule change detected")
			changes[gn] = change
		}
	}

	return changes, errors.Join(errs...)
}

func (s *ScheduleService) PrepareScheduleImage(
	ctx context.Context, schedule *model.ScheduleData,
) (fileName string, bytes []byte, err error) {
	log.Trace().Msg("Preparing schedule image...")

	template := getScheduleTemplate(schedule.Config.IsDark)
	fileName, bytes, err = s.screenshot(schedule.Config, schedule.HTML(template))
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate schedule image: %w", err)
	}
	return fileName, bytes, nil
}

func (s *ScheduleService) GetGroupByName(ctx context.Context, name model.GroupName) (*model.Group, error) {
	return s.scraper.GetGroup(ctx, string(name))
}
func (s *ScheduleService) GetGroupsByDepartmentName(ctx context.Context, name string) ([]model.Group, error) {
	return s.scraper.GetGroups(ctx, name)
}
func (s *ScheduleService) ValidateGroupName(ctx context.Context, name model.GroupName) (model.GroupName, error) {
	g, err := s.scraper.GetGroup(ctx, string(name))
	if err != nil {
		return name, err
	}
	return g.GroupName, nil
}

func (s *ScheduleService) GetDepartments(ctx context.Context) ([]model.Department, error) {
	return s.scraper.GetDepartments(ctx)
}

func (s *ScheduleService) GetTeacherByNameOrID(ctx context.Context, nameOrID string) (*model.Teacher, error) {
	return s.scraper.GetTeacher(ctx, nameOrID)
}

func (s *ScheduleService) FindTeachersByName(ctx context.Context, name string) ([]model.Teacher, error) {
	return s.scraper.SearchTeachers(ctx, name)
}

func (s *ScheduleService) IsVacation(ctx context.Context) (bool, error) {
	if _, err := s.GetDepartments(ctx); err != nil {
		if errors.Is(err, apiclient.ErrServiceUnavailable) {
			return true, nil
		}
		return false, fmt.Errorf("failed to check vacation status: %w", err)
	}
	return false, nil
}

func (s *ScheduleService) screenshot(conf model.ScheduleConfig, html string) (string, []byte, error) {
	log.Debug().Msg("Taking screenshot...")

	imageData, err := s.browser.ScreenshotHTML(html)
	if err != nil {
		return "", nil, fmt.Errorf("failed to take screenshot: %w", err)
	}
	log.Trace().Msg("Taken screenshot")

	imageFileName := path.Join(viper.GetString(config.KeyScreenshotDir), scheduleScreenshotFileName(conf))
	if err := os.MkdirAll(viper.GetString(config.KeyScreenshotDir), 0755); err != nil {
		return imageFileName, nil, fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	if err := os.WriteFile(imageFileName, imageData, 0644); err != nil {
		return imageFileName, nil, fmt.Errorf("failed to write screenshot to file: %w", err)
	}
	log.Trace().Msg("Saved image file")

	return imageFileName, imageData, nil
}

func (s *ScheduleService) HealthCheck() error {
	departments, err := s.GetDepartments(context.Background())
	if err != nil || len(departments) == 0 {
		return fmt.Errorf("departments: %w", err)
	}
	if _, err := s.GetGroupsByDepartmentName(context.Background(), departments[0].Name); err != nil {
		return fmt.Errorf("groups: %w", err)
	}
	return nil
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

func scheduleConfigToAPIParams(conf *model.ScheduleConfig) *apiclient.GetScheduleParams {
	params := new(apiclient.GetScheduleParams)
	if conf.Group != nil {
		params.Group = string(conf.Group.GroupName)
	}
	if conf.Teacher != nil {
		params.Teacher = conf.Teacher.TeacherID
	}
	return params
}
