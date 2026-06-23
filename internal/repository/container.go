package repository

import "github.com/jmoiron/sqlx"

func NewContainer(db *sqlx.DB) (*Container, error) {
	proxyRepo, err := NewProxyRepository()
	if err != nil {
		return nil, err
	}

	return &Container{
		Proxy:    proxyRepo,
		Chat:     NewChatRepository(db),
		Group:    NewGroupRepository(db),
		Schedule: NewScheduleRepository(db),
		Log:      NewLogRepository(db),
	}, nil
}

type Container struct {
	Proxy    ProxyRepository
	Chat     ChatRepository
	Group    GroupRepository
	Schedule ScheduleRepository
	Log      LogRepository
}
