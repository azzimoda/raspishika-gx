package model

import (
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

// UpdateLog represents a log entry for a Telegram API update.
type UpdateLog struct {
	ID        int64     `db:"id"`
	ChatID    int64     `db:"chat_id"`    // [Chat.ID]
	Kind      string    `db:"kind"`       // Variants: "message", "callback_query".
	MessageID int       `db:"message_id"` // ID of User's message or of the message of inline keyboard of the callback query.
	Data      string    `db:"data"`       // Message text or callback query data.
	Elapsed   int       `db:"elapsed"`    // Milliseconds
	Error     *string   `db:"error"`      // Empty value means update handler succeeded.
	CreatedAt time.Time `db:"created_at"`
}

func (ul *UpdateLog) IsOk() bool { return refutil.DerefOrTypeDefault(ul.Error) == "" }
