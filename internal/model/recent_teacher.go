package model

import "time"

type RecentTeacher struct {
	ID          int64     `db:"id"`
	ChatID      int64     `db:"chat_id"`
	TeacherID   string    `db:"teacher_id"`
	TeacherName string    `db:"teacher_name"`
	CreatedAt   time.Time `db:"created_at"`
}
