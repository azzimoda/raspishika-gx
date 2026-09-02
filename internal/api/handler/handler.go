// Package handler contains the HTTP handlers and the API responses.
package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	_ "github.com/azzimoda/raspishika-gx/docs"
	"github.com/azzimoda/raspishika-gx/internal/api/service"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// Service is the data source used by the handlers.
type Service interface {
	GetDepartments(context.Context) ([]model.Department, error)
	GetGroupByName(context.Context, model.GroupName) (*model.Group, error)
	GetGroups(context.Context) ([]model.Group, error)
	GetGroupsByDepartment(ctx context.Context, departmentID string) ([]model.Group, error)
	GetTeacherByNameOrID(context.Context, string) (*model.Teacher, error)
	SearchTeachers(context.Context, string) ([]model.Teacher, error)
	GetTeachers(context.Context) ([]model.Teacher, error)
	GetSchedule(context.Context, model.ScheduleConfig) (sch *model.ScheduleData, err error)
}

// NewHandler creates a Handler with the given service.
func NewHandler(service Service) *Handler { return &Handler{service: service} }

// Handler processes incoming API requests.
type Handler struct{ service Service }

// GetDepartments returns the list of all existing departments.
//
// @Summary     Get departments
// @Description Returns list of all existing departments
// @Tags        departments
// @Produce     json
// @Success     200  {object}  []model.Department
// @Failure     503  {object}  model.ErrorResponse
// @Router      /departments [get]
func (h *Handler) GetDepartments(c *gin.Context) {
	ctx := c.Request.Context()

	departments, err := h.service.GetDepartments(ctx)
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to get departments")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		log.Error().Err(err).Msg("Failed to get departments")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}
	log.Info().Any("departmentsCount", len(departments)).Msg("Got departments")
	c.JSON(http.StatusOK, &departments)
}

// GetGroups returns the list of all existing groups, optionally filtered by department.
//
// @Summary     Get groups
// @Description Returns list of all existing groups, optionally filtered by department
// @Tags        groups
// @Produce     json
// @Param       department  query  string  false  "Department name"
// @Success     200  {object}  []model.Group
// @Success     404  {object}  model.ErrorResponse
// @Failure     503  {object}  model.ErrorResponse
// @Router      /groups [get]
func (h *Handler) GetGroups(c *gin.Context) {
	ctx := c.Request.Context()

	department := c.Query("department")

	var groups []model.Group
	var err error
	if department == "" {
		groups, err = h.service.GetGroups(ctx)
	} else {
		groups, err = h.service.GetGroupsByDepartment(ctx, department)
	}
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to get departments")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		if errors.Is(err, service.ErrNoDepartment) && department != "" {
			log.Warn().Str("department", department).Msg("Department not found")
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: fmt.Sprintf("no department %q", department),
			})
			return
		}

		log.Error().Err(err).Str("department", department).Msg("Failed to get groups by department")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	log.Info().Str("department", department).Any("groupsCount", len(groups)).Msg("Got groups")
	c.JSON(http.StatusOK, groups)
}

// GetGroup returns a group by name.
//
// @Summary     Get group
// @Description Returns a group by name
// @Tags        groups
// @Produce     json
// @Param       name  path  string  true  "Group name"
// @Success     200  {object}  model.Group
// @Success     404  {object}  model.ErrorResponse
// @Failure     503  {object}  model.ErrorResponse
// @Router      /groups/{name} [get]
func (h *Handler) GetGroup(c *gin.Context) {
	ctx := c.Request.Context()

	groupNameStr := c.Param("name")
	groupName, err := model.GroupName(groupNameStr).ValidateFormat()
	if err != nil {
		log.Warn().Msg("Bad request to get group")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
		return
	}

	group, err := h.service.GetGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to get departments")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		if errors.Is(err, service.ErrNoGroup) {
			log.Warn().Str("name", groupNameStr).Msg("Group not found")
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: fmt.Sprintf("no group %q", groupNameStr),
			})
			return
		}

		log.Error().Err(err).Str("name", groupNameStr).Msg("Failed to get group")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	log.Info().Str("name", string(group.GroupName)).Msg("Got group")
	c.JSON(http.StatusOK, group)
}

// GetTeachers returns the list of all existing teachers.
//
// @Summary     Get teachers
// @Description Returns list of all existing teachers
// @Tags        teachers
// @Produce     json
// @Success     200  {object}  []model.Teacher
// @Failure     503  {object}  model.ErrorResponse
// @Router      /teachers [get]
func (h *Handler) GetTeachers(c *gin.Context) {
	ctx := c.Request.Context()

	teachers, err := h.service.GetTeachers(ctx)
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to get teachers")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		log.Error().Err(err).Msg("Failed to get teachers")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	log.Info().Msg("Got teachers")
	c.JSON(http.StatusOK, teachers)
}

// SearchTeachers returns a list of teachers matched by part of name.
//
// @Summary     Search teachers
// @Description Returns a list of teachers matched by part of name
// @Tags        teachers
// @Produce     json
// @Param       q  query  string  true  "Teacher name part"
// @Success     200  {object}  []model.Teacher
// @Success     400  {object}  model.ErrorResponse
// @Failure     503  {object}  model.ErrorResponse
// @Router      /teachers/search [get]
func (h *Handler) SearchTeachers(c *gin.Context) {
	ctx := c.Request.Context()

	query := c.Query("q")

	teachers, err := h.service.SearchTeachers(ctx, query)
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to search teachers")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		log.Error().Err(err).Msg("Failed to search teachers")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	log.Info().Msg("Searched teachers")
	c.JSON(http.StatusOK, teachers)
}

// GetTeacher returns a teacher by name or college internal ID.
//
// @Summary     Get teacher
// @Description Returns a teacher by name or college internal ID
// @Tags        teachers
// @Produce     json
// @Param       name_or_id  path  string  true  "Teacher name or college internal ID"
// @Success     200  {object}  model.Teacher
// @Success     404  {object}  model.ErrorResponse
// @Failure     503  {object}  model.ErrorResponse
// @Router      /teachers/{name_or_id} [get]
func (h *Handler) GetTeacher(c *gin.Context) {
	ctx := c.Request.Context()

	nameOrID := c.Param("name_or_id")

	teacher, err := h.service.GetTeacherByNameOrID(ctx, nameOrID)
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to get teacher")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		if errors.Is(err, service.ErrNoTeacher) {
			log.Warn().Str("nameOrID", nameOrID).Msg("Teacher not found")
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error: fmt.Sprintf("no teacher %q", nameOrID),
			})
			return
		}

		log.Error().Err(err).Str("nameOrID", nameOrID).Msg("Failed to get teacher")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	log.Info().Str("nameOrID", nameOrID).Msg("Got teacher")
	c.JSON(http.StatusOK, teacher)
}

// GetSchedule returns the schedule of a group or a teacher.
//
// @Summary     Get schedule
// @Description Returns schedule of a group or a teacher
// @Tags        schedule
// @Produce     json
// @Param       group    query  string  false  "Group name"
// @Param       teacher  query  string  false  "Teacher name or college internal ID"
// @Success     200  {object}  model.ScheduleData
// @Success     400  {object}  model.ErrorResponse
// @Success     404  {object}  model.ErrorResponse
// @Failure     503  {object}  model.ErrorResponse
// @Router      /schedule [get]
func (h *Handler) GetSchedule(c *gin.Context) {
	ctx := c.Request.Context()

	groupName := c.Query("group")
	teacherNameOrID := c.Query("teacher")

	var conf model.ScheduleConfig
	if groupName != "" {
		validatedGroupName, err := model.GroupName(groupName).ValidateFormat()
		if err != nil {
			log.Warn().Str("group", groupName).Msg("Invalid group name to get schedule")
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: err.Error()})
			return
		}

		group, err := h.service.GetGroupByName(ctx, validatedGroupName)
		if err != nil {
			if errors.Is(err, service.ErrServiceUnavailable) {
				log.Error().Err(err).Msg("Failed to get group for schedule")
				c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
				return
			}

			if errors.Is(err, service.ErrNoGroup) {
				log.Warn().Str("group", groupName).Msg("Group not found")
				c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "group not found"})
				return
			}

			log.Error().Err(err).Str("group", groupName).Msg("Failed to get group schedule")
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
			return
		}

		conf = model.ScheduleConfig{Group: group}
	} else if teacherNameOrID != "" {
		teacher, err := h.service.GetTeacherByNameOrID(ctx, teacherNameOrID)
		if err != nil {
			if errors.Is(err, service.ErrServiceUnavailable) {
				log.Error().Err(err).Msg("Failed to get teacher for schedule")
				c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
				return
			}

			if errors.Is(err, service.ErrNoTeacher) {
				log.Warn().Str("teacher", teacherNameOrID).Msg("Teacher not found")
				c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "teacher not found"})
				return
			}

			log.Error().Err(err).Str("teacher", teacherNameOrID).Msg("Failed to get teacher schedule")
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
			return
		}

		conf = model.ScheduleConfig{Teacher: teacher}
	} else {
		log.Warn().Msg("Bad request to get schedule")
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "expected either group or teacher"})
		return
	}

	schedule, err := h.service.GetSchedule(ctx, conf)
	if err != nil {
		if errors.Is(err, service.ErrServiceUnavailable) {
			log.Error().Err(err).Msg("Failed to get schedule")
			c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{Error: err.Error()})
			return
		}

		log.Error().Err(err).Any("config", conf).Msg("Failed to get schedule")
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: err.Error()})
		return
	}

	log.Info().Any("imageKey", conf.ImageKey()).Msg("Got schedule")
	c.JSON(http.StatusOK, schedule)
}
