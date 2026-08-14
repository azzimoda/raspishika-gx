// Package model defines the data structures of the raspishika domain:
// departments, groups, teachers and schedules.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// PairKind describes the type of a pair (lecture, exam, vacation, etc.).
type PairKind string

// Pair kinds.
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

// Schedule is the final schedule for a group or teacher.
type ScheduleData struct {
	Config ScheduleConfig `json:"config"`
	Days   []ScheduleDay  `json:"days"`
	IsOld  bool
}

func (s ScheduleData) WithConfig(conf ScheduleConfig) ScheduleData {
	s.Config = conf
	return s
}

// Today returns the first day in the schedule which commonly represents the current day.
func (s *ScheduleData) Today() ScheduleDay { return s.Days[0] }

// Tomorrow returns a day which should represent the next day after current.
// If now is Sunday, it returns the first day, because Sunday is not included in the schedule,
// otherwise it returns the second day.
func (s *ScheduleData) Tomorrow(currentTime time.Time) ScheduleDay {
	if currentTime.Weekday() == time.Sunday {
		return s.Days[0]
	}
	return s.Days[1]
}

func (s *ScheduleData) JSON() ([]byte, error) { return json.Marshal(s) }

// HTML fills the given HTML template with the schedule contents.
// Supported placeholders: HEADER, TABLE_HEAD, TABLE_BODY and TIMESTAMP.
func (s *ScheduleData) HTML(template string) string {
	var header string
	if s.Config.Group != nil {
		header = fmt.Sprintf("Расписание группы %s — %s", s.Config.Group.GroupName, s.Config.Group.DepartmentName)
	} else if s.Config.Teacher != nil {
		header = fmt.Sprintf("Расписание преподавателя — %s", s.Config.Teacher.Name)
	} else {
		header = "Расписание"
	}

	var tableHead strings.Builder
	for _, day := range s.Days {
		fmt.Fprintf(&tableHead, "<th>%s<br>%s<br>%s</th>\n", day.Date, day.Weekday, day.WeekKind)
	}

	html := strings.NewReplacer(
		"HEADER", header,
		"TABLE_HEAD", tableHead.String(),
		"TABLE_BODY", s.generateTableBody(),
		"TIMESTAMP", time.Now().Format(time.RFC3339),
	).Replace(template)

	return html
}
func (s *ScheduleData) generateTableBody() string {
	pairNumbers := make(map[int]struct{})
	timeRanges := make(map[int]string)
	for _, day := range s.Days {
		for _, pair := range day.Pairs {
			pairNumbers[pair.Number] = struct{}{}
			if _, ok := timeRanges[pair.Number]; !ok {
				timeRanges[pair.Number] = fmt.Sprintf("%s-%s", pair.StartTime, pair.EndTime)
			}
		}
	}

	sortedNumbers := make([]int, 0, len(pairNumbers))
	for n := range pairNumbers {
		sortedNumbers = append(sortedNumbers, n)
	}
	sort.Ints(sortedNumbers)

	var tableBody strings.Builder
	for _, num := range sortedNumbers {
		fmt.Fprintf(&tableBody, `<tr>
				<td class="side_column_number">%d</td>
				<td class="side_column_time">%s</td>
				%s
			</tr>`,
			num, strings.ReplaceAll(timeRanges[num], "-", "<hr>"), s.generateRowPairs(num))
	}
	return tableBody.String()
}
func (s *ScheduleData) generateRowPairs(pairNum int) string {
	var rowPairs strings.Builder

	for _, day := range s.Days {
		var pair *Pair
		for i := range day.Pairs {
			if day.Pairs[i].Number == pairNum {
				pair = &day.Pairs[i]
				break
			}
		}

		if pair == nil {
			fmt.Fprintf(&rowPairs, `<td class="%s"><span></span></td>`, PairKindEmpty)
			rowPairs.WriteString("\n")
			continue
		}

		cssClass := string(pair.Kind)
		if pair.Replaced {
			cssClass += " replaced"
		}

		switch pair.Kind {
		case PairKindEvent, PairKindVacation, PairKindSession, PairKindPractice, PairKindIGA:
			fmt.Fprintf(&rowPairs, `<td class="%s"><span>%s</span></td>`, cssClass, pair.Label)
		case PairKindExam, PairKindConsultation:
			teacher := ""
			if pair.Teacher != "" {
				teacher = pair.Teacher
			}
			fmt.Fprintf(&rowPairs, `
				<td class='%s'>
					<span class='title'>%s</span><br>
					<hr>
					<span class='discipline'>%s</span><br> <br>
					<span class='teacher'>%s</span><br>
					<span class='classroom'>%s</span><br>
				</td>`,
				cssClass, pair.Title, pair.Discipline, teacher, pair.Classroom)
		case PairKindSubject:
			secondLine := ""
			if pair.Group != "" {
				secondLine = fmt.Sprintf("<span class='group'>%s</span>", pair.Group)
			} else if pair.Teacher != "" {
				secondLine = fmt.Sprintf("<span class='teacher'>%s</span>", pair.Teacher)
			}
			subgroupLine := ""
			if pair.Subgroup != "" {
				subgroupLine = fmt.Sprintf("<br><span class='subgroup'>%s</span>", pair.Subgroup)
			}

			fmt.Fprintf(&rowPairs, `<td class='%s'>
					<span class='discipline'>%s</span>%s<br>
					<br>
					%s<br>
					<span class='classroom'>%s</span><br>
				</td>`,
				cssClass, pair.Discipline, subgroupLine, secondLine, pair.Classroom)
		default:
			label := ""
			if pair.Replaced {
				label = "Снято"
			}
			fmt.Fprintf(&rowPairs, `<td class="%s"><span>%s</span></td>`, cssClass, label)
		}
		rowPairs.WriteString("\n")
	}

	return rowPairs.String()
}

// ScheduleDay is a single day of a schedule.
type ScheduleDay struct {
	Date     string `json:"date" example:"2026-09-01"`
	Weekday  string `json:"week_day" example:"вторник"`
	WeekKind string `json:"week_kind" example:"нечетная"`
	Pairs    []Pair `json:"pairs"`
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

// IsEqual reports whether the day equals other, comparing all fields.
func (s *ScheduleDay) IsEqual(other *ScheduleDay) bool { return reflect.DeepEqual(s, other) }

// IsEmpty reports whether the day has no active pairs.
func (s *ScheduleDay) IsEmpty() bool { return s.CommonKind() == PairKindEmpty }

var ErrAllPairsPassed = errors.New("all pairs passed")

// CurrentPair returns the pair that is currently in progress at the given time.
// Pair start times are shifted backwards to cover the breaks between pairs,
// so the current pair is picked even during a break.
//
// Returns an error if the time does not fall into any pair,
// or if a pair start or end time cannot be parsed.
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

	return nil, ErrAllPairsPassed
}

// HTML formats the whole day as human-readable HTML.
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

// DateHTML formats the date header of a day.
func (s *ScheduleDay) DateHTML() string { return fmt.Sprintf("📅 %s, %s", s.Weekday, s.Date) }

// DynamicFormatHTML formats the day like HTML, but marks pairs
// that have already passed at time t as struck-through.
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
		if pair.IsPassedAt(t) {
			log.Trace().Msg("Before")
			text += fmt.Sprintf("<s>%s</s>", pair.HTML())
		} else {
			log.Trace().Msg("After")
			text += pair.HTML()
		}
	}
	return text
}

// Pair is a single pair (lesson) of a schedule day.
type Pair struct {
	Kind       PairKind `json:"kind" example:"subject"`
	Number     int      `json:"number" example:"1"`
	StartTime  string   `json:"start_time" example:"8:00"`
	EndTime    string   `json:"end_time" example:"9:45"`
	Label      string   `json:"label" example:"Математика"`
	Title      string   `json:"title" example:"Консультация"`
	Discipline string   `json:"discipline" example:"Математика"`
	Teacher    string   `json:"teacher" example:"Иванов Иван Иванович"`
	Group      string   `json:"group" example:"ИСПт-22-(9)-2"`
	Subgroup   string   `json:"subgroup" example:"1"`
	Classroom  string   `json:"classroom" example:"215"`
	Replaced   bool     `json:"replaced"`
}

// IsEmpty reports whether the pair represents a gap in the schedule,
// e.g. an empty slot, an event or a session.
func (p *Pair) IsEmpty() bool {
	switch p.Kind {
	case PairKindEmpty, PairKindEvent, PairKindSession:
		return true
	default:
		return false
	}
}

// IsEqual reports whether the pair equals other, comparing all fields.
func (p *Pair) IsEqual(other *Pair) bool { return reflect.DeepEqual(p, other) }

// IsPassedAt reports whether the pair has already ended before time t.
func (p *Pair) IsPassedAt(t time.Time) bool {
	endTime, err := time.Parse("15:04", p.EndTime)
	endTime = time.Date(t.Year(), t.Month(), t.Day(), endTime.Hour(), endTime.Minute(), 0, 0, t.Location())
	return err == nil && endTime.Before(t)
}

// HTML returns formatted pair string.
func (p *Pair) HTML() string {
	switch p.Kind {
	case PairKindSubject:
		return fmt.Sprintf("%s\n    <b>%s</b>\n    %s", p.timeSlotClassroomString(), p.Discipline, p.Teacher)
	case PairKindExam, PairKindConsultation:
		return fmt.Sprintf("%s\n    <i>%s</i>\n    <b>%s</b>\n    %s",
			p.timeSlotClassroomString(), p.Label, p.Discipline, p.Teacher)
	default:
		return fmt.Sprintf("%s — %s", p.timeSlotString(), p.Label)
	}
}
func (p *Pair) timeSlotString() string {
	return fmt.Sprintf("%d | %s - %s", p.Number, p.StartTime, p.EndTime)
}
func (p *Pair) timeSlotClassroomString() string {
	return fmt.Sprintf("%d | %s - %s | %s", p.Number, p.StartTime, p.EndTime, p.Classroom)
}
