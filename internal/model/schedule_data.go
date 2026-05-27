package model

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

type PairKind string

const (
	PairKindEmpty        PairKind = "empty"
	PairKindVacation     PairKind = "vacation"
	PairKindEvent        PairKind = "event"
	PairKindSession      PairKind = "session"
	PairKindIGA          PairKind = "iga"
	PairKindSubject      PairKind = "subject"
	PairKindPractice     PairKind = "practice"
	PairKindExam         PairKind = "exam"
	PairKindConsultation PairKind = "consultation"
)

type ScheduleData struct {
	Config ScheduleConfig `json:"config"`
	Days   []ScheduleDay  `json:"days"`
}

// Today returns the first day in the schedule which commonly represents the current day.
func (s *ScheduleData) Today() ScheduleDay { return s.Days[0] }

// Tomorrow returns a day which should represent the next day after current.
// If now is Sunday, it returns the first day, because Sunday is not included in the schedule,
// otherwise it returns the second day.
func (s *ScheduleData) Tomorrow(currentTime time.Time) ScheduleDay {
	if time.Now().Weekday() == time.Sunday {
		return s.Days[0]
	} else {
		return s.Days[1]
	}
}

type ScheduleDay struct {
	Date     Date     `json:"date"`
	Weekday  Weekday  `json:"week_day"`
	WeekKind WeekKind `json:"week_kind"`
	Pairs    []Pair   `json:"pairs"`
}

// CommonKind returns the common kind of all pairs in the day.
// If there are no pairs, it returns PairKindEmpty. If pairs have different kinds, it returns an empty string.
func (s *ScheduleDay) CommonKind() PairKind {
	if len(s.Pairs) == 0 {
		return PairKindEmpty
	}

	kind := s.Pairs[0].Kind
	for _, pair := range s.Pairs {
		if pair.Kind != kind {
			return ""
		}
	}
	return kind
}

func (s *ScheduleDay) IsEqual(other *ScheduleDay) bool { return reflect.DeepEqual(s, other) }
func (s *ScheduleDay) IsEmpty() bool                   { return s.CommonKind() == PairKindEmpty }

// CurrentPair returns the current pair at the given time.
// If the time is before the first pair, it returns the first pair.
// If the time is after the last pair, it returns the last pair.
// Otherwise, it returns the pair that is currently in progress.
//
// Returns an error if the pair start or end time cannot be parsed.
func (s ScheduleDay) CurrentPair(t time.Time) (*Pair, error) {
	log.Trace().Time("time", t).Str("timeStr", t.String()).Msg("CurrentPair")
	year, month, day := t.Date()
	for _, pair := range s.Pairs {
		startTime, err := time.Parse("15:04", strings.TrimSpace(pair.StartTime))
		if err != nil {
			return nil, fmt.Errorf("failed to parse pair start time: %w", err)
		}

		endTime, err := time.Parse("15:04", strings.TrimSpace(pair.EndTime))
		if err != nil {
			return nil, fmt.Errorf("failed to parse pair end time: %w", err)
		}

		// Adjust date.
		startTime = time.Date(year, month, day, startTime.Hour(), startTime.Minute(), 0, 0, t.Location())
		endTime = time.Date(year, month, day, endTime.Hour(), endTime.Minute(), 0, 0, t.Location())

		// Adjust start time.
		switch pair.Number {
		case 1:
			// This pair goes first; it needs a little shifting to be catched.
			startTime = startTime.Add(-1 * time.Minute)
		case 2, 3, 5, 6, 7:
			// These pairs go after 10-minute breaks.
			startTime = startTime.Add(-10 * time.Minute)
		case 4:
			// This pair goes after the big break, which is 45 minutes long.
			startTime = startTime.Add(-45 * time.Minute)
		}

		if t.After(startTime) && t.Before(endTime) {
			return &pair, nil
		}
	}

	return nil, fmt.Errorf("all pairs passed")
}

func (s *ScheduleDay) HTML() string {
	text := s.DateHTML() + ": "

	if kind := s.CommonKind(); kind != "" {
		log.Trace().Msgf("Detected common kind: %s", kind)
		if kind == PairKindEmpty {
			text += "Нет пар"
		} else {
			text += s.Pairs[0].Label
		}
		return text
	}

	for _, pair := range s.Pairs {
		if pair.Kind == PairKindEmpty {
			continue
		}
		text += "\n\n" + pair.HTML()
	}
	return text
}
func (s *ScheduleDay) DateHTML() string { return fmt.Sprintf("📅 %s, %s", s.Weekday, s.Date) }
func (s *ScheduleDay) DynamicFormatHTML(t time.Time) string {
	text := s.DateHTML() + ": "

	if kind := s.CommonKind(); kind != "" {
		log.Trace().Msgf("Detected common kind: %s", kind)
		if kind == PairKindEmpty {
			text += "Нет пар"
		} else {
			text += s.Pairs[0].Label
		}
		return text
	}

	for _, pair := range s.Pairs {
		if pair.Kind == PairKindEmpty {
			continue
		}
		text += "\n\n"
		if pair.IsBefore(t) {
			log.Trace().Msg("Before")
			text += fmt.Sprintf("<s>%s</s>", pair.HTML())
		} else {
			log.Trace().Msg("After")
			text += pair.HTML()
		}
	}
	return text
}

type Pair struct {
	Kind       PairKind `json:"kind"`
	Number     int      `json:"number"`
	StartTime  string   `json:"start_time"`
	EndTime    string   `json:"end_time"`
	Label      string   `json:"label"`
	Title      string   `json:"title"`
	Discipline string   `json:"discipline"`
	Teacher    *string  `json:"teacher"`
	Group      *string  `json:"group"`
	Subgroup   string   `json:"subgroup"`
	Classroom  string   `json:"classroom"`
	Replaced   bool     `json:"replaced"`
}

func (p *Pair) IsEmpty() bool {
	switch p.Kind {
	case PairKindEmpty, PairKindEvent, PairKindSession:
		return true
	default:
		return false
	}
}
func (p *Pair) IsEqual(other *Pair) bool { return reflect.DeepEqual(p, other) }
func (p *Pair) IsBefore(t time.Time) bool {
	endTime, err := time.Parse("15:04", p.EndTime)
	endTime = time.Date(t.Year(), t.Month(), t.Day(), endTime.Hour(), endTime.Minute(), 0, 0, t.Location())
	return err == nil && endTime.Before(t)
}

// HTML returns formatted pair string.
func (p *Pair) HTML() string {
	teacher := func() string { return refutil.DerefOrTypeDefault(p.Teacher) }

	switch p.Kind {
	case PairKindSubject:
		return fmt.Sprintf("%s\n    <b>%s</b>\n    %s", p.TimeSlotClassroomString(), p.Discipline, teacher())
	case PairKindExam, PairKindConsultation:
		return fmt.Sprintf("%s\n    <i>%s</i>\n    <b>%s</b>\n    %s",
			p.TimeSlotClassroomString(), p.Label, p.Discipline, teacher())
	default:
		return fmt.Sprintf("%s — %s", p.TimeSlotString(), p.Label)
	}
}

// TimeSlotString returns formatted time slot string.
func (p *Pair) TimeSlotString() string {
	return fmt.Sprintf("%d | %s - %s", p.Number, p.StartTime, p.EndTime)
}

// TimeSlotClassroomString returns formatted time slot string with classroom.
func (p *Pair) TimeSlotClassroomString() string {
	return fmt.Sprintf("%d | %s - %s | %s", p.Number, p.StartTime, p.EndTime, p.Classroom)
}
