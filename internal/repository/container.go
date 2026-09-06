package repository

import (
	"github.com/azzimoda/raspishika-gx/internal/model"
	"gorm.io/gorm"
)

// NewContainer builds a repository container scoped to the given platform.
// Use PlatformTelegram/PlatformVK for a bot process and an empty platform for
// admin/dashboard processes spanning all platforms.
func NewContainer(db *gorm.DB, platform model.Platform) *Container {
	return &Container{
		Chat:     NewChatRepository(db, platform),
		Schedule: NewScheduleRepository(db),
		Log:      NewLogRepository(db),
	}
}

type Container struct {
	Chat     ChatRepository
	Schedule ScheduleRepository
	Log      LogRepository
}
