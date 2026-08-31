package repository

import "gorm.io/gorm"

func NewContainer(db *gorm.DB) *Container {
	return &Container{
		Chat:     NewChatRepository(db),
		Schedule: NewScheduleRepository(db),
		Log:      NewLogRepository(db),
	}
}

type Container struct {
	Chat     ChatRepository
	Schedule ScheduleRepository
	Log      LogRepository
}
