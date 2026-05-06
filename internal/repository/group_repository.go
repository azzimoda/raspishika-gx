package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func NewGroupRepository(db *sqlx.DB) GroupRepository { return &groupRepository{db} }

// GroupRepository defines the interface for group-related database operations.
// It handles student groups and teachers.
type GroupRepository interface {
	UpdateGroups(context.Context, []model.Group) error
	GetGroupByName(context.Context, model.GroupName) (*model.Group, error)
	GetAllGroups(context.Context) ([]*model.Group, error)
	GetAllActualGroups(context.Context) ([]*model.Group, error)
	GetOutdatedActualGroups(context.Context) ([]*model.Group, error)
	GetDepartmentActualGroups(context.Context, string) ([]*model.Group, error)

	ValidateNameCase(context.Context, model.GroupName) (model.GroupName, error)
	ValidateName(context.Context, model.GroupName) (model.GroupName, error)

	InsertOrUpdateDepartment(context.Context, *model.Department) error
	GetAllDepartments(context.Context) ([]model.Department, error)
	GetOutdatedDepartments(context.Context) ([]model.Department, error)
	GetAllDepartmentIDs(context.Context) ([]model.DepartmentID, error)

	UpdateTeachers(context.Context, []model.Teacher) error
	GetAllTeachers(context.Context) ([]*model.Teacher, error)
	GetOutdatedTeachers(context.Context) ([]*model.Teacher, error)
	GetChatRecentTeachers(ctx context.Context, chatID int64) ([]*model.Teacher, error)
	GetTeacherByID(context.Context, model.TeacherID) (*model.Teacher, error)
}

type groupRepository struct{ db *sqlx.DB }

func (r *groupRepository) UpdateGroups(ctx context.Context, groups []model.Group) error {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	for _, group := range groups {
		var _g model.Group
		if err := tx.GetContext(ctx, &_g, `SELECT * FROM groups WHERE group_id = ?`, group.GroupID); err != nil {
			// Insert new
			_, err = tx.NamedExecContext(ctx, `
					INSERT INTO groups (group_id, department_id, group_name, department_name, year)
					VALUES (:group_id, :department_id, :group_name, :department_name, :year)
				`, group)
			if err != nil {
				tx.Rollback()
				return err
			}
		}

		// Update existing
		group.UpdatedAt = time.Now()
		_, err = tx.NamedExecContext(ctx, `
				UPDATE groups
				SET department_id = :department_id, updated_at = :updated_at
				WHERE department_name = :department_name AND group_id = :group_id
			`, group)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}
func (r *groupRepository) GetGroupByName(ctx context.Context, name model.GroupName) (*model.Group, error) {
	var group model.Group
	err := r.db.GetContext(ctx, &group, "SELECT * FROM groups WHERE group_name = ?", name)
	return &group, err
}
func (r *groupRepository) GetAllGroups(ctx context.Context) ([]*model.Group, error) {
	var groups []*model.Group
	err := r.db.SelectContext(ctx, &groups, "SELECT * FROM groups")
	return groups, err
}
func (r *groupRepository) GetAllActualGroups(ctx context.Context) ([]*model.Group, error) {
	groups, err := r.GetAllGroups(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().AddDate(0, -8, 0) // Shift the date 8 months back, so its year equals to current cours start year
	currentYear := now.Year()
	var actualGroups []*model.Group
	for _, group := range groups {
		applicationYear := group.ApplicationYear()
		if applicationYear+4 < currentYear {
			// The group is not graduated, add it to the actual groups
			actualGroups = append(actualGroups, group)
		}
	}

	return actualGroups, nil
}
func (r *groupRepository) GetOutdatedActualGroups(ctx context.Context) ([]*model.Group, error) {
	var groups []*model.Group
	// Select groups updated more than 7 days ago.
	if err := r.db.SelectContext(
		ctx,
		&groups,
		`SELECT * FROM groups WHERE updated_at < datetime('now', '-7 days')`,
	); err != nil {
		return nil, err
	}

	now := time.Now().AddDate(0, -8, 0) // Shift the date 8 months back, so its year equals to current cours start year
	currentYear := now.Year()
	var actualGroups []*model.Group
	for _, group := range groups {
		applicationYear := group.ApplicationYear()
		if applicationYear+4 < currentYear {
			// The group is not graduated, add it to the actual groups
			actualGroups = append(actualGroups, group)
		}
	}

	return actualGroups, nil
}
func (r *groupRepository) GetDepartmentActualGroups(ctx context.Context, name string) ([]*model.Group, error) {
	var groups []*model.Group
	err := r.db.SelectContext(ctx, &groups, `SELECT * FROM groups WHERE department_name = ?`, name)
	return groups, err
}

func (r *groupRepository) ValidateNameCase(ctx context.Context, name model.GroupName) (model.GroupName, error) {
	nameLower := strings.ToLower(string(name))

	groups, err := r.GetAllGroups(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get groups from DB")
		return name, err
	}

	for _, group := range groups {
		groupNameLower := strings.ToLower(string(group.GroupName))
		if groupNameLower == nameLower {
			return group.GroupName, nil
		}
	}
	return name, errors.New("group name not found")
}
func (r *groupRepository) ValidateName(ctx context.Context, name model.GroupName) (model.GroupName, error) {
	validatedFormat, err := name.ValidateFormat()
	if err != nil {
		return name, err
	}
	return r.ValidateNameCase(ctx, validatedFormat)
}

func (r *groupRepository) InsertOrUpdateDepartment(ctx context.Context, department *model.Department) error {
	_, err := r.db.NamedExecContext(ctx, `
		INSERT INTO departments (name, url)
		VALUES (:name, :url)
		ON CONFLICT (name) DO UPDATE SET url = :url, updated_at = CURRENT_TIMESTAMP
	`, department)
	return err
}
func (r *groupRepository) GetAllDepartments(ctx context.Context) ([]model.Department, error) {
	var departments []model.Department
	err := r.db.SelectContext(ctx, &departments, `SELECT id, name, url, created_at, updated_at FROM departments`)
	return departments, err
}
func (r *groupRepository) GetOutdatedDepartments(ctx context.Context) ([]model.Department, error) {
	var departments []model.Department
	err := r.db.SelectContext(ctx, &departments, `
			SELECT * FROM departments
			WHERE updated_at < datetime('now', '-1 day')
		`)
	return departments, err
}
func (r *groupRepository) GetAllDepartmentIDs(ctx context.Context) ([]model.DepartmentID, error) {
	var ids []model.DepartmentID
	err := r.db.SelectContext(ctx, &ids, `SELECT DISTINCT department_id FROM groups`)
	return ids, err
}

func (r *groupRepository) UpdateTeachers(ctx context.Context, teachers []model.Teacher) error {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	for _, t := range teachers {
		var _t model.Teacher
		if err := tx.GetContext(ctx, &_t, `SELECT * FROM teachers WHERE teacher_id = ?`, t.TeacherID); err != nil {
			// Insert new
			_, err = tx.NamedExecContext(ctx, `INSERT INTO teachers (teacher_id, name) VALUES (:teacher_id, :name)`, t)
			if err != nil {
				tx.Rollback()
				return err
			}
		}

		// Update existing
		t.UpdatedAt = time.Now()
		_, err = tx.NamedExecContext(ctx, `
				UPDATE teachers
				SET name = :name, updated_at = :updated_at
				WHERE teacher_id = :teacher_id
			`, t)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
func (r *groupRepository) GetTeacherByID(ctx context.Context, teacherID model.TeacherID) (*model.Teacher, error) {
	var teacher model.Teacher
	err := r.db.GetContext(ctx, &teacher, `SELECT * FROM teachers WHERE teacher_id = ?`, teacherID)
	return &teacher, err
}
func (r *groupRepository) GetAllTeachers(ctx context.Context) ([]*model.Teacher, error) {
	var teachers []*model.Teacher
	err := r.db.SelectContext(ctx, &teachers, `SELECT * FROM teachers`)
	return teachers, err
}
func (r *groupRepository) GetChatRecentTeachers(ctx context.Context, chatID int64) ([]*model.Teacher, error) {
	var teachers []*model.Teacher
	err := r.db.SelectContext(ctx, &teachers, `
			SELECT t.id, t.teacher_id, t.name, t.created_at, t.updated_at
			FROM recent_teachers rt JOIN teachers t ON rt.teacher_id = t.id
			WHERE rt.chat_id = ?
		`, chatID)
	return teachers, err
}
func (r *groupRepository) GetOutdatedTeachers(ctx context.Context) ([]*model.Teacher, error) {
	var teachers []*model.Teacher
	err := r.db.SelectContext(ctx, &teachers, `
			SELECT * FROM teachers
			WHERE updated_at < datetime('now', '-1 day')
		`)
	return teachers, err
}
