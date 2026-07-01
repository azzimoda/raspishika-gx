package model

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
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
	if len(old.Days) == 0 || len(new.Days) == 0 {
		return old, new
	}
	if old.Days[0].Date != new.Days[0].Date {
		// Shift old schedule one day forward
		old.Days = old.Days[1:]
	}
	length := min(len(old.Days), len(new.Days))
	old.Days = old.Days[:length]
	new.Days = new.Days[:length]
	return old, new
}

type ScheduleChange struct {
	Old ScheduleData `json:"old"`
	New ScheduleData `json:"new"`
}

func (s *ScheduleChange) Diffs() []Diff {
	var absDiffs []Diff
	for d := range s.Old.Days {
		for p := range s.Old.Days[d].Pairs {
			newDay := s.New.Days[d]
			oldPair := s.Old.Days[d].Pairs[p]
			newPair := s.New.Days[d].Pairs[p]
			if !reflect.DeepEqual(oldPair, newPair) {
				absDiffs = append(absDiffs, Diff{NewDay: &newDay, OldPair: oldPair, NewPair: newPair})
			}
		}
	}

	return absDiffs
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

		date1, err := time.Parse("02.01.2006", diffs[i].Day().Date.String())
		if err != nil {
			log.Warn().Err(err).Str("timeStr", diffs[i].Day().Date.String()).Msg("Failed to parse date")
		}
		date2, err := time.Parse("02.01.2006", diffs[j].Day().Date.String())
		if err != nil {
			log.Warn().Err(err).Str("timeStr", diffs[j].Day().Date.String()).Msg("Failed to parse date")
		}

		return date1.Before(date2)
	})

	// Build result string
	var text strings.Builder
	fmt.Fprintf(&text, "Изменения в расписании группы %s:", s.New.Config.Group.GroupName)

	var currentDate Date
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
	if d.OldDay != nil && d.NewDay != nil && !d.OldDay.IsEqual(d.NewDay) {
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
	return refutil.DerefOrTypeDefault(d.OldPair.Teacher) != refutil.DerefOrTypeDefault(d.NewPair.Teacher)
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
			text = "<i>Замена :</i>\n" + d.NewPair.TimeSlotString()

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
					refutil.DerefOrTypeDefault(d.OldPair.Teacher),
					refutil.DerefOrTypeDefault(d.NewPair.Teacher),
				)
			} else {
				text += fmt.Sprintf("\n    %s", refutil.DerefOrTypeDefault(d.NewPair.Teacher))
			}

		default:
			text = fmt.Sprintf("<i>Замена:</i>\n<s>%s</s>\n<b><i>%s</i></b>", d.OldPair.HTML(), d.NewPair.HTML())
		}
	}

	return text
}
