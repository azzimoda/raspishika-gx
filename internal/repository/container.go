package repository

import "github.com/jmoiron/sqlx"

func NewContainer(db *sqlx.DB) *Container {
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
