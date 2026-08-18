package model

import "time"

type RecentTeacher struct {
	ID          int64     `gorm:"primaryKey;column:id"`
	ChatID      int64     `gorm:"column:chat_id"`
	TeacherID   string    `gorm:"column:teacher_id"`
	TeacherName string    `gorm:"column:teacher_name"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}
