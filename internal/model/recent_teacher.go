package model

import "time"

type RecentTeacher struct {
	ID        int       `db:"id"`
	ChatID    int       `db:"chat_id"`
	TeacherID int       `db:"teacher_id"`
	CreatedAt time.Time `db:"created_at"`
}
