package model

import "time"

type BroadcastLog struct {
	ID        int              `db:"id"`
	Kind      BroadcastLogKind `db:"kind"`
	Chats     int              `db:"chats"`
	Groups    int              `db:"groups"`
	Elapsed   int              `db:"elapsed"` // milliseconds
	Fails     int              `db:"fails"`
	Errors    string           `db:"errors"`
	CreatedAt time.Time        `db:"created_at"`
}

type BroadcastLogKind string

const (
	BLogAny    BroadcastLogKind = ""
	BLogDaily  BroadcastLogKind = "daily"
	BLogPair   BroadcastLogKind = "pair"
	BLogChange BroadcastLogKind = "update"
)
