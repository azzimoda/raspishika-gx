package model

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func NewScheduleChange(old, new ScheduleData) *ScheduleChange {
	if !old.Config.IsEqual(&new.Config) {
		return nil
	}
	old, new = Synchronize(old, new)
	return &ScheduleChange{old, new}
}
func Synchronize(old, new ScheduleData) (ScheduleData, ScheduleData) {
	// The college returns a rolling window. Dates leaving or entering that
	// window are not lesson cancellations or additions; compare common dates.
	// Match their identity rather than assuming exactly one day has passed.
	newByDate := make(map[string]ScheduleDay, len(new.Days))
	for _, day := range new.Days {
		newByDate[day.Date] = day
	}
	var oldDays, newDays []ScheduleDay
	for _, oldDay := range old.Days {
		if newDay, ok := newByDate[oldDay.Date]; ok {
			oldDays = append(oldDays, oldDay)
			newDays = append(newDays, newDay)
		}
	}
	old.Days, new.Days = oldDays, newDays
	return old, new
}

type ScheduleChange struct {
	Old ScheduleData `json:"old"`
	New ScheduleData `json:"new"`
}

func (s *ScheduleChange) Diffs() []Diff {
	var absDiffs []Diff
	// ScheduleChange can also be decoded from JSON or constructed directly.
	old, new := Synchronize(s.Old, s.New)
	for d := range old.Days {
		oldDay, newDay := old.Days[d], new.Days[d]
		oldPairs := make(map[int]Pair, len(oldDay.Pairs))
		newPairs := make(map[int]Pair, len(newDay.Pairs))
		numbers := make(map[int]struct{}, len(oldDay.Pairs)+len(newDay.Pairs))
		for _, pair := range oldDay.Pairs {
			oldPairs[pair.Number] = pair
			numbers[pair.Number] = struct{}{}
		}
		for _, pair := range newDay.Pairs {
			newPairs[pair.Number] = pair
			numbers[pair.Number] = struct{}{}
		}
		ordered := make([]int, 0, len(numbers))
		for number := range numbers {
			ordered = append(ordered, number)
		}
		sort.Ints(ordered)
		for _, number := range ordered {
			oldPair, hadOld := oldPairs[number]
			newPair, hasNew := newPairs[number]
			if !hadOld {
				oldPair = emptyPairSlot(newPair)
			}
			if !hasNew {
				newPair = emptyPairSlot(oldPair)
			}
			// Empty slots can differ in metadata, but neither contains a lesson.
			// Avoid creating a Diff whose Number method has no active pair.
			if oldPair.IsEmpty() && newPair.IsEmpty() {
				continue
			}
			if !reflect.DeepEqual(oldPair, newPair) {
				absDiffs = append(absDiffs, Diff{OldDay: &oldDay, NewDay: &newDay, OldPair: oldPair, NewPair: newPair})
			}
		}
	}

	return absDiffs
}

func emptyPairSlot(pair Pair) Pair {
	return Pair{Kind: PairKindEmpty, Number: pair.Number, StartTime: pair.StartTime, EndTime: pair.EndTime}
}

func (s *ScheduleChange) HTML() string {
	diffs := s.Diffs()
	if len(diffs) == 0 {
		return "Изменения не обнаружены"
	}

	// Sort diffs by date and pair number
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Day().Date == diffs[j].Day().Date {
			return diffs[i].Number() < diffs[j].Number()
		}

		date1, ok1 := parseScheduleDate(diffs[i].Day().Date)
		date2, ok2 := parseScheduleDate(diffs[j].Day().Date)
		if ok1 && ok2 {
			return date1.Before(date2)
		}
		if ok1 != ok2 {
			return ok1
		}
		return diffs[i].Day().Date < diffs[j].Day().Date
	})

	// Build result string
	var text strings.Builder
	switch {
	case s.New.Config.Group != nil:
		fmt.Fprintf(&text, "Изменения в расписании группы %s:", s.New.Config.Group.GroupName)
	case s.New.Config.Teacher != nil:
		fmt.Fprintf(&text, "Изменения в расписании преподавателя %s:", s.New.Config.Teacher.Name)
	default:
		text.WriteString("Изменения в расписании:")
	}

	var currentDate string
	for _, diff := range diffs {
		date := diff.Day().Date
		if currentDate != date {
			currentDate = date
			fmt.Fprintf(&text, "\n\n%s: ", diff.Day().DateHTML())
		}
		fmt.Fprintf(&text, "\n\n%s", diff.HTML())
	}

	return text.String()
}

func parseScheduleDate(value string) (time.Time, bool) {
	for _, layout := range []string{"02.01.2006", "2006-01-02"} {
		if date, err := time.Parse(layout, value); err == nil {
			return date, true
		}
	}
	return time.Time{}, false
}

type Diff struct {
	OldDay  *ScheduleDay `json:"old_day"`
	OldPair Pair         `json:"old_pair"`
	NewDay  *ScheduleDay `json:"new_day"`
	NewPair Pair         `json:"new_pair"`
}

func (d *Diff) Day() *ScheduleDay {
	if d.NewDay != nil {
		return d.NewDay
	} else if d.OldDay != nil {
		return d.OldDay
	} else {
		panic("Diff must have at least one day instanse")
	}
}
func (d *Diff) Number() int {
	if !d.NewPair.IsEmpty() {
		return d.NewPair.Number
	} else if !d.OldPair.IsEmpty() {
		return d.OldPair.Number
	} else {
		log.Panic().Msg("Diff must have at least one not empty pair instanse")
		panic("")
	}
}

// HTML representation of the difference.
func (d *Diff) HTML() (result string) {
	if d.OldDay != nil && d.NewDay != nil && d.OldDay.Date != d.NewDay.Date {
		// Pair moved to other day
		log.Warn().Msg("Schedule difference case not yet implemented: Pair moved to other day")
		// NOTE: This case not suposed to implemented.

		result = "?"
	} else {
		result = d.pairChangeHTML()
	}
	return result
}

func (d *Diff) IsFullChange() bool {
	return d.IsClassroomChanged() && d.IsDisciplineChanged() && d.IsTeacherChanged()
}
func (d *Diff) IsClassroomChanged() bool  { return d.OldPair.Classroom != d.NewPair.Classroom }
func (d *Diff) IsDisciplineChanged() bool { return d.OldPair.Discipline != d.NewPair.Discipline }
func (d *Diff) IsTeacherChanged() bool {
	return d.OldPair.Teacher != d.NewPair.Teacher
}
func (d *Diff) IsMovedInDay() bool { return d.OldPair.Number != d.NewPair.Number }
func (d *Diff) IsCancelled() bool  { return d.NewPair.IsEmpty() }
func (d *Diff) IsAdded() bool      { return d.OldPair.IsEmpty() }

// pairChangeHTML returns formatted pair change string.
func (d *Diff) pairChangeHTML() string {
	text := ""

	if d.IsAdded() {
		text = fmt.Sprintf("<i>Добавлено:</i>\n%s", d.NewPair.HTML())
	} else if d.IsCancelled() {
		text = fmt.Sprintf("<i>Снято:</i>\n<s>%s</s>", d.OldPair.HTML())
	} else if d.IsFullChange() {
		text = fmt.Sprintf("<i>Замена:</i>\n<s>%s</s>\n%s", d.OldPair.HTML(), d.NewPair.HTML())
	} else {
		switch d.NewPair.Kind {
		case PairKindSubject:
			text = "<i>Замена :</i>\n" + d.NewPair.timeSlotString()

			if d.IsClassroomChanged() {
				text += fmt.Sprintf(" | <s>%s</s> <b><i>%s</i></b>", d.OldPair.Classroom, d.NewPair.Classroom)
			} else {
				text += fmt.Sprintf(" | %s", d.NewPair.Classroom)
			}

			if d.IsDisciplineChanged() {
				text += fmt.Sprintf("\n    <s>%s</s> <b><i>%s</i></b>", d.OldPair.Discipline, d.NewPair.Discipline)
			} else {
				text += fmt.Sprintf("\n    %s", d.NewPair.Discipline)
			}

			if d.IsTeacherChanged() {
				text += fmt.Sprintf("\n    <s>%s</s> <b><i>%s</i></b>",
					d.OldPair.Teacher, d.NewPair.Teacher)
			} else {
				text += fmt.Sprintf("\n    %s", d.NewPair.Teacher)
			}

		default:
			text = fmt.Sprintf("<i>Замена:</i>\n<s>%s</s>\n<b><i>%s</i></b>", d.OldPair.HTML(), d.NewPair.HTML())
		}
	}

	return text
}
