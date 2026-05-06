package model

import "time"

// UpdateLog represents a log entry for a Telegram API update.
type UpdateLog struct {
	ID           int64     `db:"id"`
	ChatID       int64     `db:"chat_id"` // [Chat.ID]
	Kind         string    `db:"kind"`    // Variants: "message", "callback_query"
	MessageID    int       `db:"message_id"`
	Data         string    `db:"data"`
	HandlingTime int       `db:"handling_time"` // Milliseconds
	Error        *string   `db:"error"`
	CreatedAt    time.Time `db:"created_at"`
}

func (ul *UpdateLog) IsOk() bool {
	return ul.Error == nil || *ul.Error == ""
}
