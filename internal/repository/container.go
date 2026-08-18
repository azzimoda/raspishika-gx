package repository

import "gorm.io/gorm"

func NewContainer(db *gorm.DB) *Container {
	return &Container{
		Proxy:    NewProxyRepository(),
		Chat:     NewChatRepository(db),
		Schedule: NewScheduleRepository(db),
		Log:      NewLogRepository(db),
	}
}

type Container struct {
	Proxy    ProxyRepository
	Chat     ChatRepository
	Schedule ScheduleRepository
	Log      LogRepository
}
