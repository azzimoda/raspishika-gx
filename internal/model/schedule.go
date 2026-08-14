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
	ID        int64     `db:"id"`
	CacheKey  string    `db:"cache_key"`
	Data      string    `db:"data"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (s *Schedule) IsActual(ttl time.Duration) bool { return s.UpdatedAt.Add(ttl).After(time.Now()) }

func (s *Schedule) Unmarshal() (*ScheduleData, error) {
	var scheduleData ScheduleData
	err := json.Unmarshal([]byte(s.Data), &scheduleData)
	return &scheduleData, err
}
