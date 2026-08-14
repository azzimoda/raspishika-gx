package model

import "regexp"

// Teacher represents a teacher of the college.
type Teacher struct {
	TeacherID string `json:"teacher_id" example:"205"`
	Name      string `json:"name" example:"Иванов Иван Иванович"`
}

var teacherNameRegex = regexp.MustCompile(`[^\w\p{Cyrillic}\d_]+`)

// Safe removes all non-alphabetic characters (latin and cyrillic letters)
func (t *Teacher) SafeName() string { return teacherNameRegex.ReplaceAllString(string(t.Name), "") }
