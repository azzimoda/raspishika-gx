package model

import "time"

type BroadcastTaskLog struct {
	ID        int64         `json:"id"`
	Kind      BroadcastKind `db:"kind"`
	Groups    int           `db:"groups"`
	Elapsed   int64         `db:"elapsed"` // Milliseconds
	CreatedAt time.Time     `db:"created_at"`
}

type BroadcastKind string

const (
	BAny    BroadcastKind = ""
	BDaily  BroadcastKind = "daily"
	BPair   BroadcastKind = "pair"
	BChange BroadcastKind = "update"
)
