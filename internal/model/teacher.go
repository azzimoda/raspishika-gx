package model

import (
	"regexp"
	"time"
)

type TeacherID string

func (i TeacherID) String() string { return string(i) }

type TeacherName string

var teacherNameRegex = regexp.MustCompile(`[^\w\p{Cyrillic}\d_]+`)

func (n TeacherName) String() string { return string(n) }

// Safe removes all non-alphabetic characters (latin and cyrillic letters)
func (n TeacherName) Safe() string { return teacherNameRegex.ReplaceAllString(string(n), "") }

type Teacher struct {
	ID        int64       `db:"id"         json:"id"`
	TeacherID TeacherID   `db:"teacher_id" json:"teacher_id"`
	Name      TeacherName `db:"name"       json:"name"`
	CreatedAt time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt time.Time   `db:"updated_at" json:"updated_at"`
}
