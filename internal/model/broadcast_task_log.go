package model

import "time"

type BroadcastTaskLog struct {
	ID        int64         `gorm:"primaryKey;column:id" json:"id"`
	Kind      BroadcastKind `gorm:"column:kind"`
	Groups    int           `gorm:"column:groups"`
	Elapsed   int64         `gorm:"column:elapsed"` // Milliseconds
	CreatedAt time.Time     `gorm:"column:created_at"`
}

type BroadcastKind string

const (
	BAny    BroadcastKind = ""
	BDaily  BroadcastKind = "daily"
	BPair   BroadcastKind = "pair"
	BChange BroadcastKind = "update"
)
