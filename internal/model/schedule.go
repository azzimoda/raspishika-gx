package model

import (
	"encoding/json"
	"fmt"
	"time"
)

func NewSchedule(cacheKey string, rawSchedule ScheduleData) (*Schedule, error) {
	jsonData, err := json.Marshal(rawSchedule)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal raw schedule: %w", err)
	}
	return &Schedule{CacheKey: cacheKey, Data: string(jsonData)}, nil
}

type Schedule struct {
	ID        int64     `gorm:"primaryKey;column:id"`
	CacheKey  string    `gorm:"column:cache_key"`
	Data      string    `gorm:"column:data"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (s *Schedule) IsActual(ttl time.Duration) bool { return s.UpdatedAt.Add(ttl).After(time.Now()) }

func (s *Schedule) Unmarshal() (*ScheduleData, error) {
	var scheduleData ScheduleData
	err := json.Unmarshal([]byte(s.Data), &scheduleData)
	return &scheduleData, err
}
