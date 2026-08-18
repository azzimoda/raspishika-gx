package model

import (
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

type BroadcastLog struct {
	ID        int64     `gorm:"primaryKey;column:id"`
	TaskID    int64     `gorm:"column:broadcast_task_log_id"` // [BroadcastTaskLog.ID]
	ChatID    int64     `gorm:"column:chat_id"`               // [Chat.ID]
	Group     GroupName `gorm:"column:group"`                 // [Group.GroupName]
	Error     *string   `gorm:"column:error"`                 // Empty value means update handler succeeded.
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (b *BroadcastLog) IsOk() bool { return refutil.DerefOrTypeDefault(b.Error) == "" }
