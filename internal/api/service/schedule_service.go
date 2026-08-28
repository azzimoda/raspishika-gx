// Package service implements the caching layer between the HTTP handlers
// and the scraped data.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/api/scraper"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/pkg/redisdb"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/schollz/closestmatch"
)

var (
	// ErrNoDepartment is returned when a department is not found.
	ErrNoDepartment = errors.New("department not found")
	// ErrNoGroup is returned when a group is not found.
	ErrNoGroup = errors.New("group not found")
	// ErrNoTeacher is returned when a teacher is not found.
	ErrNoTeacher = errors.New("teacher not found")
)

// Scraper abstracts the data source used by the service.
type Scraper interface {
	CheckVacation() (bool, error)
	ScrapeDepartments() ([]model.Department, error)
	ScrapeDepartmentGroups(*model.Department) ([]model.Group, error)
	ScrapeTeachers() ([]model.Teacher, error)
	ScrapeSchedule(string, model.ScheduleConfig) (*model.ScheduleData, error)
}

// NewScheduleService creates a service that caches scraped data in Redis.
func NewScheduleService(scraper Scraper, rdb *redisdb.SmartClient) *ScheduleService {
	return &ScheduleService{scraper: scraper, rdb: rdb}
}

// ScheduleService fetches data through a Scraper and caches the results
// in Redis to avoid repeated scraping.
type ScheduleService struct {
	scraper Scraper
	rdb     *redisdb.SmartClient
}

const (
	// VacationTTL is how long vacation flag data is kept in the cache.
	VacationTTL = 6 * time.Hour

	// MetaFreshTTL is how long metadata (departments, groups, teachers) is
	// considered fresh.
	MetaFreshTTL = 24 * time.Hour
	// MetaDataTTL is how long metadata data is kept after it becomes stale.
	MetaDataTTL = 7 * 24 * time.Hour

	// ScheduleFreshTTL is how long a schedule is considered fresh.
	ScheduleFreshTTL = 30 * time.Minute
	// ScheduleDataTTL is how long schedule data is kept after it becomes stale.
	ScheduleDataTTL = 36 * time.Hour
)

// ErrServiceUnavailable is returned when the college site is unavailable,
// i.e. it is in vacation mode.
var ErrServiceUnavailable = errors.New("service unavailable")

func (s *ScheduleService) CheckVacation(ctx context.Context) (bool, error) {

	log.Debug().Msg("ScheduleService.CheckVacation")

	// Check cache

	const vacationKey = "vacation"

	isVacation, err := s.rdb.RedisClient.Get(ctx, vacationKey).Bool()
	if err == redis.Nil {
		log.Debug().Msg("Vacation cache miss")
	} else if err != nil {
		log.Error().Err(err).Msg("Failed to check vacation cache")
		return false, err
	} else {
		log.Debug().Bool("isVacationOld", isVacation).Msg("Vacation cache hit")
		return isVacation, nil
	}

	// Fetch

	isVacation, err = s.scraper.CheckVacation()
	if err != nil {
		log.Error().Err(err).Msg("Failed to check vacation")
		return false, err
	}

	// Set cache

	if err := s.rdb.RedisClient.Set(ctx, vacationKey, isVacation, VacationTTL).Err(); err != nil {
		log.Error().Err(err).Msg("Failed to set vacation cache")
	}

	return isVacation, nil
}

func (s *ScheduleService) GetDepartmentByName(ctx context.Context, name string) (*model.Department, error) {

	log.Debug().Str("name", string(name)).Msg("GetDepartmentByName")

	name = strings.ToLower(name)
	key := "department:" + string(name)

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, key)
	var departmentCached model.Department
	if exist {
		if err := json.Unmarshal(data, &departmentCached); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal department cache JSON")
			fresh, exist = false, false
		}

	}

	if fresh && departmentCached != (model.Department{}) {
		return &departmentCached, nil
	}

	// Fetch

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	departments, err := s.GetDepartments(ctx)
	if err != nil {
		if exist {
			return &departmentCached, nil
		}
		return nil, err
	}
	var departmentNew *model.Department = nil
	for _, d := range departments {
		if strings.ToLower(string(d.Name)) == name {
			departmentNew = &d
			break
		}
	}
	if departmentNew == nil {
		if exist {
			log.Warn().Str("departmentName", string(name)).Msg("Department not found, using old cache")
			return &departmentCached, nil
		}
		log.Error().Str("departmentName", string(name)).Msg("Department not found, no old cache")
		return nil, ErrNoDepartment
	}

	// Set cache

	if data, err := json.Marshal(departmentNew); err != nil {
		log.Error().Err(err).Any("department", departmentNew).Msg("Failed to serialize department to JSON")
	} else {
		if err := s.rdb.Set(ctx, key, data, MetaFreshTTL, MetaDataTTL); err != nil {
			log.Error().Err(err).Str("departmentName", departmentNew.Name).
				Msg("Failed to set department redis cache")
		}
	}

	return departmentNew, nil
}

// GetDepartments returns all departments, using the cache when possible.
func (s *ScheduleService) GetDepartments(ctx context.Context) ([]model.Department, error) {

	log.Debug().Msg("GetDepartments")

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, "departments")
	var departmentsOld []model.Department
	if exist {
		if err := json.Unmarshal(data, &departmentsOld); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal departments cache JSON")
			fresh, exist = false, false
		}
	}

	log.Trace().Bool("fresh", fresh).Any("departmentsOld", departmentsOld).
		Msg("Checking if departments cache is fresh and isn't nil")
	if fresh && departmentsOld != nil {
		return departmentsOld, nil
	}

	// Scrape

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	departmentsNew, err := s.scraper.ScrapeDepartments()
	if err != nil {
		if exist {
			log.Warn().Err(err).Msg("Failed to scrape departments, using old cache")
			return departmentsOld, nil
		}
		log.Error().Err(err).Msg("Failed to scrape departments, no old cache")
		return nil, err
	}

	// Set cache

	if data, err := json.Marshal(departmentsNew); err != nil {
		log.Error().Err(err).Msg("Failed to serialize departments to JSON")
	} else {
		if err := s.rdb.Set(ctx, "departments", data, MetaFreshTTL, MetaDataTTL); err != nil {
			log.Error().Err(err).Msg("Failed to set departments redis cache")
		}
	}

	return departmentsNew, nil
}

// GetGroupByName returns a group by its name, validating and normalizing it.
func (s *ScheduleService) GetGroupByName(ctx context.Context, name model.GroupName) (*model.Group, error) {

	log.Debug().Str("name", string(name)).Msg("GetGroupByName")

	var err error
	name, err = name.ValidateFormat()
	if err != nil {
		return nil, err
	}

	name = model.GroupName(strings.ToLower(string(name)))
	key := "group:" + string(name)

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, key)
	var groupCached model.Group
	if exist {
		if err := json.Unmarshal(data, &groupCached); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal group cache JSON")
			fresh, exist = false, false
		}

	}

	if fresh && groupCached != (model.Group{}) {
		return &groupCached, nil
	}

	// Fetch

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	groups, err := s.GetGroups(ctx)
	if err != nil {
		if exist {
			return &groupCached, nil
		}
		return nil, err
	}
	var groupNew *model.Group = nil
	for _, g := range groups {
		if strings.ToLower(string(g.GroupName)) == string(name) {
			groupNew = &g
			break
		}
	}
	if groupNew == nil {
		if exist {
			log.Warn().Str("groupName", string(name)).Msg("Group not found, using old cache")
			return &groupCached, nil
		}
		log.Error().Str("groupName", string(name)).Msg("Group not found, no old cache")
		return nil, ErrNoGroup
	}

	// Set cache

	if data, err := json.Marshal(groupNew); err != nil {
		log.Error().Err(err).Any("group", groupNew).Msg("Failed to serialize group to JSON")
	} else {
		if err := s.rdb.Set(ctx, key, data, MetaFreshTTL, MetaDataTTL); err != nil {
			log.Error().Err(err).Str("groupName", string(groupNew.GroupName)).
				Msg("Failed to set group redis cache")
		}
	}

	return groupNew, nil
}

// GetGroups returns all groups of all departments.
func (s *ScheduleService) GetGroups(ctx context.Context) ([]model.Group, error) {

	log.Debug().Msg("GetGroups")

	departments, err := s.GetDepartments(ctx)
	if err != nil {
		return nil, err
	}

	var groupsAll []model.Group
	var errs []error
	for _, d := range departments {
		groups, err := s.GetGroupsByDepartment(ctx, d.Name)
		if err != nil {
			log.Error().Err(err).Str("departments", d.Name).Msg("Failed to get departments")
			errs = append(errs, err)
			continue
		}
		groupsAll = append(groupsAll, groups...)
	}
	if len(groupsAll) == 0 {
		return nil, errors.Join(errs...)
	}
	return groupsAll, nil
}

// GetGroupsByDepartment returns the groups of a single department.
// It returns ErrNoDepartment if the department does not exist.
func (s *ScheduleService) GetGroupsByDepartment(ctx context.Context, departmentName string) ([]model.Group, error) {

	log.Debug().Msg("GetGroupsByDepartment")

	departmentName = strings.ToLower(departmentName)
	key := "groups:" + departmentName

	// Check department

	if departments, err := s.GetDepartments(ctx); err != nil {
		return nil, fmt.Errorf("failed to check department name: %w", err)
	} else {
		found := false
		for _, d := range departments {
			if strings.ToLower(d.Name) == departmentName {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrNoDepartment
		}
	}

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, key)
	var groupsCached []model.Group
	if exist {
		if err := json.Unmarshal(data, &groupsCached); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal departments cache JSON")
			fresh, exist = false, false
		}
	}

	if fresh && groupsCached != nil {
		return groupsCached, nil
	}

	// Scrape

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	department, err := s.GetDepartmentByName(ctx, departmentName)
	if err != nil {
		if exist {
			return groupsCached, nil
		}
		return nil, err
	}

	groupsNew, err := s.scraper.ScrapeDepartmentGroups(department)
	if err != nil {
		if exist {
			return groupsCached, nil
		}
		return nil, err
	}

	// Set cache

	if data, err := json.Marshal(groupsNew); err != nil {
		log.Error().Err(err).Msg("Failed to serialize departments to JSON")
	} else {
		if err := s.rdb.Set(ctx, key, data, MetaFreshTTL, MetaDataTTL); err != nil {
			log.Error().Err(err).Msg("Failed to set departments redis cache")
		}
	}

	return groupsNew, nil
}

// GetTeacherByName returns a teacher by name or college internal ID.
// It returns ErrNoTeacher if the teacher does not exist.
func (s *ScheduleService) GetTeacherByNameOrID(ctx context.Context, nameOrID string) (*model.Teacher, error) {

	log.Debug().Str("name", nameOrID).Msg("GetTeacherByNameOrID")

	nameOrID = strings.ToLower(nameOrID)
	key := "teacher:" + nameOrID

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, key)
	var teacherCached model.Teacher
	if exist {
		if err := json.Unmarshal(data, &teacherCached); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal teacher cache JSON")
			fresh, exist = false, false
		}
	}

	if fresh && teacherCached != (model.Teacher{}) {
		return &teacherCached, nil
	}

	// Fetch

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	teachers, err := s.GetTeachers(ctx)
	if err != nil {
		if exist {
			return &teacherCached, nil
		}
		return nil, err
	}
	var teacherNew *model.Teacher = nil

	var f func(t *model.Teacher) bool
	if _, err := strconv.ParseInt(nameOrID, 10, 64); err == nil {
		log.Trace().Msg("Searching by ID...")
		f = func(t *model.Teacher) bool { return t.TeacherID == nameOrID }
	} else {
		log.Trace().Msg("Searching by name...")
		f = func(t *model.Teacher) bool { return strings.EqualFold(t.Name, nameOrID) }
	}
	for _, t := range teachers {
		if f(&t) {
			teacherNew = &t
			break
		}
	}
	if teacherNew == nil {
		if exist {
			log.Warn().Str("name", string(nameOrID)).Msg("Teacher not found, using old cache")
			return &teacherCached, nil
		}
		log.Error().Str("name", string(nameOrID)).Msg("Teacher not found, no old cache")
		return nil, ErrNoTeacher
	}

	// Set cache

	if data, err := json.Marshal(teacherNew); err != nil {
		log.Error().Err(err).Any("teacher", teacherNew).Msg("Failed to serialize teacher to JSON")
	} else {
		if err := s.rdb.Set(ctx, key, data, MetaFreshTTL, MetaDataTTL); err != nil {
			log.Error().Err(err).Str("teacherName", string(teacherNew.Name)).
				Msg("Failed to set teacher redis cache")
		}
	}

	return teacherNew, nil
}

func (s *ScheduleService) SearchTeachers(ctx context.Context, query string) ([]model.Teacher, error) {
	teachers, err := s.GetTeachers(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(teachers))
	for i, t := range teachers {
		names[i] = t.Name
	}

	matchedNames := matchStrings(names, query, 5)
	log.Trace().Any("matchedNames", matchedNames).Send()

	matchedTeachers := make([]model.Teacher, 0, len(matchedNames))
	for _, n := range matchedNames {
		teacher, err := s.GetTeacherByNameOrID(ctx, n)
		if err != nil {
			log.Error().Err(err).Str("name", n).Msg("Failed to get teache by name for search")
			continue
		}
		matchedTeachers = append(matchedTeachers, *teacher)
	}
	log.Trace().Any("matchedTeachers", matchedTeachers).Send()
	return matchedTeachers, nil
}
func matchStrings(strs []string, target string, n int) []string {
	for _, s := range strs {
		if strings.EqualFold(s, target) {
			return []string{s}
		}
	}
	return closestmatch.New(strs, []int{2, 3, 4}).ClosestN(target, n)
}

// GetTeachers returns all teachers, using the cache when possible.
func (s *ScheduleService) GetTeachers(ctx context.Context) ([]model.Teacher, error) {

	log.Debug().Msg("GetTeachers")

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, "teachers")
	var teachersOld []model.Teacher
	if exist {
		if err := json.Unmarshal(data, &teachersOld); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal departments cache JSON")
			fresh, exist = false, false
		}
	}

	if fresh && teachersOld != nil {
		return teachersOld, nil
	}

	// Scrape

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	teachersNew, err := s.scraper.ScrapeTeachers()
	if err != nil {
		if exist {
			log.Warn().Err(err).Msg("Failed to scrape teachers, using old cache")
			return teachersOld, nil
		}
		log.Error().Err(err).Msg("Failed to scrape teachers, no old cache")
		return nil, err
	}

	// Set cache

	if data, err := json.Marshal(teachersNew); err != nil {
		log.Error().Err(err).Msg("Failed to serialize teachers to JSON")
	} else {
		if err := s.rdb.Set(ctx, "teachers", data, MetaFreshTTL, MetaDataTTL); err != nil {
			log.Error().Err(err).Msg("Failed to set teachers redis cache")
		}
	}

	return teachersNew, nil
}

// GetSchedule returns the schedule for the given config, using the cache when possible.
func (s *ScheduleService) GetSchedule(
	ctx context.Context, conf model.ScheduleConfig,
) (schedule *model.ScheduleData, err error) {

	log.Debug().Any("config", conf).Msg("GetSchedule")

	key := "schedule:"
	if conf.Group != nil {
		key += string(conf.Group.GroupName)
	} else if conf.Teacher != nil {
		key += conf.Teacher.Name
	} // NOTE: else is unreachable

	// Check cache

	data, fresh, exist := s.rdb.Get(ctx, key)
	var scheduleOld model.ScheduleData
	if exist {
		if err := json.Unmarshal(data, &scheduleOld); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal schedule cache JSON")
			fresh, exist = false, false
		}
		scheduleOld.IsOld = true

		if fresh {
			scheduleOld.IsOld = false
			return &scheduleOld, nil
		}
	}

	// Scrape

	if isVacation, err := s.CheckVacation(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to check vacation status; trying to fetch data anyway...")
	} else if isVacation {
		return nil, ErrServiceUnavailable
	}

	departments, err := s.GetDepartments(ctx)
	if err != nil {
		if exist {
			log.Warn().Err(err).Msg("Failed to get departments for schedule URL, using old cache")
			return &scheduleOld, nil
		}
		log.Error().Err(err).Msg("Failed to get departments for schedule URL, no old cache")
		return nil, err
	}

	scheduleNew, err := s.scraper.ScrapeSchedule(scraper.ScheduleURL(conf, departments), conf)
	if err != nil {
		if exist {
			log.Warn().Err(err).Msg("Failed to scrape schedule, using old cache")
			return &scheduleOld, nil
		}
		log.Error().Err(err).Msg("Failed to scrape schedule, no old cache")
		return nil, err
	}

	// Set cache

	if data, err := json.Marshal(scheduleNew); err != nil {
		log.Error().Err(err).Msg("Failed to serialize schedule to JSON")
	} else {
		if err := s.rdb.Set(ctx, key, data, ScheduleFreshTTL, ScheduleDataTTL); err != nil {
			log.Error().Err(err).Msg("Failed to set schedule redis cache")
		}
	}

	return scheduleNew, nil
}
