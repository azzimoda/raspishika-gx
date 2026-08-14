package model

import (
	"encoding/json"
	"strings"
)

// RawSchedule is the schedule exactly as parsed from the scraped HTML page,
// before it is transformed into a Schedule.
type RawSchedule struct {
	Config ScheduleConfig   `json:"config"`
	Rows   []RawScheduleRow `json:"rows"`
}

// RawScheduleRow is a single pair slot (e.g. pair 1) for all days of the week.
type RawScheduleRow struct {
	Number int `json:"number"`
	// Like "8:00-9:35".
	TimeRange string           `json:"time_range"`
	Days      []RawScheduleDay `json:"days"`
}

// RawScheduleDay is the pair of one row for one specific day.
type RawScheduleDay struct {
	// Like "02.01.2006"
	Date     string `json:"date"`
	WeekDay  string `json:"weekday"`
	WeekKind string `json:"week_kind"`
	Pair     Pair   `json:"pair"`
}

func (s *RawSchedule) JSON() ([]byte, error) { return json.Marshal(s) }

// Transform converts the raw schedule into the final Schedule,
// assigning each pair its number and splitting the time range
// into separate start and end times.
func (s *RawSchedule) Transform() ScheduleData {
	schedule := ScheduleData{
		Config: s.Config,
		Days:   []ScheduleDay{},
	}

	for dayIdx := range len(s.Rows[0].Days) {
		day := ScheduleDay{
			Date:     s.Rows[0].Days[dayIdx].Date,
			Weekday:  s.Rows[0].Days[dayIdx].WeekDay,
			WeekKind: s.Rows[0].Days[dayIdx].WeekKind,
		}

		for rowIdx := 0; rowIdx < len(s.Rows); rowIdx++ {
			pair := s.Rows[rowIdx].Days[dayIdx].Pair

			pair.Number = s.Rows[rowIdx].Number
			parts := strings.Split(s.Rows[rowIdx].TimeRange, "-")
			pair.StartTime = strings.TrimSpace(parts[0])
			pair.EndTime = strings.TrimSpace(parts[1])

			day.Pairs = append(day.Pairs, pair)
		}

		schedule.Days = append(schedule.Days, day)
	}

	return schedule
}

func (s RawSchedule) WithConfig(conf ScheduleConfig) RawSchedule {
	s.Config = conf
	return s
}
