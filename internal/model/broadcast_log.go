package model

import (
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

type BroadcastLog struct {
	ID        int64     `db:"id"`
	TaskID    int64     `db:"broadcast_task_log_id"` // [BroadcastTaskLog.ID]
	ChatID    int64     `db:"chat_id"`               // [Chat.ID]
	Group     GroupName `db:"group"`                 // [Group.GroupName]
	Error     *string   `db:"error"`                 // Empty value means update handler succeeded.
	CreatedAt time.Time `db:"created_at"`
}

func (b *BroadcastLog) IsOk() bool { return refutil.DerefOrTypeDefault(b.Error) == "" }
