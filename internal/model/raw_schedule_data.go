package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Date string

func (d Date) String() string { return string(d) }

type Weekday string

func (w Weekday) String() string { return string(w) }

type WeekKind string

func (k WeekKind) String() string { return string(k) }

type TimeRange string

func (r TimeRange) String() string { return string(r) }

type RawScheduleDay struct {
	Date     Date     `json:"date"`
	WeekDay  Weekday  `json:"weekday"`
	WeekKind WeekKind `json:"week_kind"`
	Pair     Pair     `json:"pair"`
}

type RawScheduleRow struct {
	Number int `json:"number"`
	// Like "8:00-9:35".
	TimeRange TimeRange        `json:"time_range"`
	Days      []RawScheduleDay `json:"days"`
}

type RawSchedule struct {
	Config ScheduleConfig
	Rows   []RawScheduleRow
}

func (s *RawSchedule) Transform() ScheduleData {
	schedule := ScheduleData{
		Config: s.Config,
		Days:   []ScheduleDay{},
	}

	for di := range len(s.Rows[0].Days) {
		day := ScheduleDay{}

		day.Date = s.Rows[0].Days[di].Date
		day.Weekday = s.Rows[0].Days[di].WeekDay
		day.WeekKind = s.Rows[0].Days[di].WeekKind

		for ri := 0; ri < len(s.Rows); ri++ {
			pair := s.Rows[ri].Days[di].Pair

			pair.Number = s.Rows[ri].Number
			parts := strings.Split(s.Rows[ri].TimeRange.String(), "-")
			pair.StartTime = strings.TrimSpace(parts[0])
			pair.EndTime = strings.TrimSpace(parts[1])

			day.Pairs = append(day.Pairs, pair)
		}

		schedule.Days = append(schedule.Days, day)
	}

	// log.Trace().Msgf("Transformed schedule: %#v", schedule)
	return schedule
}

func (s *RawSchedule) JSON() ([]byte, error) { return json.Marshal(s) }

// HTML representation of the schedule.
func (s *RawSchedule) HTML(template string) string {
	var header string
	if s.Config.Group != nil {
		header = fmt.Sprintf("Расписание группы %s — %s", s.Config.Group.GroupName, s.Config.Group.DepartmentName)
	} else if s.Config.Teacher != nil {
		header = fmt.Sprintf("Расписание преподавателя — %s", s.Config.Teacher.Name)
	} else {
		header = "Расписание"
	}

	var tableHead strings.Builder
	for _, day := range s.Rows[0].Days {
		fmt.Fprintf(&tableHead, "<th>%s<br>%s<br>%s</th>\n", day.Date, day.WeekDay, day.WeekKind)
	}

	html := strings.NewReplacer(
		"HEADER", header,
		"TABLE_HEAD", tableHead.String(),
		"TABLE_BODY", s.generateTableBody(s.Rows),
		"TIMESTAMP", time.Now().Format(time.RFC3339),
	).Replace(template)

	return html
}

func (s *RawSchedule) generateTableBody(rows []RawScheduleRow) string {
	var tableBody strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&tableBody, `<tr>
				<td class="side_column_number">%d</td>
				<td class="side_column_time">%s</td>
				%s
			</tr>`,
			row.Number, strings.ReplaceAll(row.TimeRange.String(), "-", "<hr>"), s.generateRowPairs(row.Days))
	}
	return tableBody.String()
}

func (s *RawSchedule) generateRowPairs(days []RawScheduleDay) string {
	var rowPairs strings.Builder

	for _, day := range days {
		cssClass := day.Pair.Kind
		if day.Pair.Replaced {
			cssClass += " replaced"
		}

		switch day.Pair.Kind {
		case PairKindEvent, PairKindVacation, PairKindSession, PairKindPractice, PairKindIGA:
			fmt.Fprintf(&rowPairs, `<td class="%s"><span>%s</span></td>`, cssClass, day.Pair.Label)
		case PairKindExam, PairKindConsultation:
			fmt.Fprintf(&rowPairs, `<td class='%s'>
					<span class='title'>%s</span><br>
					<hr>
					<span class='discipline'>%s</span><br> <br>
					<span class='teacher'>%s</span><br>
					<span class='classroom'>%s</span><br>
				</td>`,
				cssClass, day.Pair.Title, day.Pair.Discipline, *day.Pair.Teacher, day.Pair.Classroom)
		case PairKindSubject:
			secondLine := ""
			if day.Pair.Group != nil {
				secondLine = fmt.Sprintf("<span class='group'>%s</span>", *day.Pair.Group)
			} else if day.Pair.Teacher != nil {
				secondLine = fmt.Sprintf("<span class='teacher'>%s</span>", *day.Pair.Teacher)
			}
			subgroupLine := ""
			if day.Pair.Subgroup != "" {
				subgroupLine = fmt.Sprintf("<br><span class='subgroup'>%s</span>", day.Pair.Subgroup)
			}

			fmt.Fprintf(&rowPairs, `<td class='%s'>
					<span class='discipline'>%s</span>%s<br>
					<br>
					%s<br>
					<span class='classroom'>%s</span><br>
				</td>`,
				cssClass, day.Pair.Discipline, subgroupLine, secondLine, day.Pair.Classroom)
		default:
			label := ""
			if day.Pair.Replaced {
				label = "Снято"
			}
			fmt.Fprintf(&rowPairs, `<td class="%s"><span>%s</span></td>`, cssClass, label)
		}
		rowPairs.WriteString("\n")
	}

	return rowPairs.String()
}
