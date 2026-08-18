package model

import (
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

// UpdateLog represents a log entry for a Telegram API update.
type UpdateLog struct {
	ID             int64     `gorm:"primaryKey;column:id"`
	ChatID         int64     `gorm:"column:chat_id"`          // [Chat.ID]
	Kind           string    `gorm:"column:kind"`             // Variants: "message", "callback_query".
	MessageID      int       `gorm:"column:message_id"`       // ID of User's message or of the message of inline keyboard of the callback query.
	Data           string    `gorm:"column:data"`             // Message text or callback query data.
	IsCached       bool      `gorm:"column:cached"`           // true if schedule sent from cache.
	GroupOrTeacher string    `gorm:"column:group_or_teacher"` // [Group.GroupName] or [Teacher.Name] or empty.
	Elapsed        int       `gorm:"column:elapsed"`          // Milliseconds
	Error          *string   `gorm:"column:error"`            // Empty value means update handler succeeded.
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (ul *UpdateLog) IsOk() bool { return refutil.DerefOrTypeDefault(ul.Error) == "" }
