package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func init() {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
}

func testPair(number int, kind PairKind, discipline, start, end string) Pair {
	return Pair{
		Kind:       kind,
		Number:     number,
		StartTime:  start,
		EndTime:    end,
		Discipline: discipline,
	}
}

func TestSchedule_Today(t *testing.T) {
	s := &ScheduleData{Days: []ScheduleDay{{Date: "01.09.2026"}, {Date: "02.09.2026"}}}
	if got := s.Today(); got.Date != "01.09.2026" {
		t.Fatalf("Today() = %+v, want first day", got)
	}
}

func TestSchedule_Tomorrow(t *testing.T) {
	s := &ScheduleData{Days: []ScheduleDay{{Date: "01.09.2026"}, {Date: "02.09.2026"}}}

	sunday := time.Date(2026, time.August, 2, 12, 0, 0, 0, time.UTC)
	if sunday.Weekday() != time.Sunday {
		t.Fatalf("test date is not a Sunday: %s", sunday.Weekday())
	}
	if got := s.Tomorrow(sunday); got.Date != "01.09.2026" {
		t.Fatalf("Tomorrow(Sunday) = %+v, want first day", got)
	}

	monday := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	if got := s.Tomorrow(monday); got.Date != "02.09.2026" {
		t.Fatalf("Tomorrow(Monday) = %+v, want second day", got)
	}
}

func TestSchedule_HTML(t *testing.T) {
	group := &Group{GroupName: "ИСПт-22-(9)-2", DepartmentName: "Отделение"}
	s := &ScheduleData{
		Config: ScheduleConfig{Group: group},
		Days: []ScheduleDay{
			{
				Date:     "01.09.2026",
				Weekday:  "вторник",
				WeekKind: "нечетная",
				Pairs:    []Pair{testPair(1, PairKindSubject, "Математика", "8:00", "9:35")},
			},
			{
				Date:     "02.09.2026",
				Weekday:  "среда",
				WeekKind: "четная",
				Pairs:    []Pair{},
			},
		},
	}

	html := s.HTML("HEADER\nTABLE_HEAD\nTABLE_BODY\nTIMESTAMP")

	if !strings.Contains(html, "Расписание группы ИСПт-22-(9)-2 — Отделение") {
		t.Errorf("HTML missing group header: %s", html)
	}
	if !strings.Contains(html, "<th>01.09.2026") {
		t.Errorf("HTML missing day header: %s", html)
	}
	if !strings.Contains(html, "Математика") {
		t.Errorf("HTML missing discipline: %s", html)
	}
	if !strings.Contains(html, `<td class="empty"><span></span></td>`) {
		t.Errorf("HTML missing empty pair cell: %s", html)
	}
}

func TestSchedule_HTML_Teacher(t *testing.T) {
	s := &ScheduleData{
		Config: ScheduleConfig{Teacher: &Teacher{Name: "Иванов"}},
		Days:   []ScheduleDay{{Pairs: []Pair{testPair(1, PairKindSubject, "Математика", "8:00", "9:35")}}},
	}

	html := s.HTML("HEADER TABLE_BODY")
	if !strings.Contains(html, "Расписание преподавателя — Иванов") {
		t.Errorf("HTML missing teacher header: %s", html)
	}
}

func TestScheduleDay_CommonKind(t *testing.T) {
	tests := []struct {
		name string
		day  ScheduleDay
		want PairKind
	}{
		{
			name: "no pairs",
			day:  ScheduleDay{Pairs: []Pair{}},
			want: PairKindEmpty,
		},
		{
			name: "all same kind",
			day: ScheduleDay{Pairs: []Pair{
				testPair(1, PairKindSubject, "Математика", "8:00", "9:35"),
				testPair(2, PairKindSubject, "Физика", "9:45", "11:20"),
			}},
			want: PairKindSubject,
		},
		{
			name: "mixed kinds",
			day: ScheduleDay{Pairs: []Pair{
				testPair(1, PairKindSubject, "Математика", "8:00", "9:35"),
				testPair(2, PairKindExam, "Физика", "9:45", "11:20"),
			}},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.day.CommonKind(); got != tt.want {
				t.Fatalf("CommonKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScheduleDay_IsEmpty_and_IsEqual(t *testing.T) {
	empty := ScheduleDay{Pairs: []Pair{testPair(1, PairKindEmpty, "", "8:00", "9:35")}}
	if !empty.IsEmpty() {
		t.Error("day with only empty pairs should be empty")
	}

	subject := ScheduleDay{Pairs: []Pair{testPair(1, PairKindSubject, "Математика", "8:00", "9:35")}}
	if subject.IsEmpty() {
		t.Error("day with subject pair should not be empty")
	}
	if !subject.IsEqual(&subject) {
		t.Error("IsEqual should return true for the same day")
	}
	if subject.IsEqual(&empty) {
		t.Error("IsEqual should return false for different days")
	}
}

func TestScheduleDayCurrentPair(t *testing.T) {
	day := ScheduleDay{
		Pairs: []Pair{
			testPair(1, PairKindSubject, "П1", "8:00", "9:35"),
			testPair(2, PairKindSubject, "П2", "9:45", "11:20"),
			testPair(3, PairKindSubject, "П3", "11:30", "13:05"),
			testPair(4, PairKindSubject, "П4", "13:45", "15:20"),
			testPair(5, PairKindSubject, "П5", "15:30", "17:05"),
			testPair(6, PairKindSubject, "П6", "17:15", "18:50"),
			testPair(7, PairKindSubject, "П7", "19:00", "20:35"),
		},
	}

	at := func(hour, min int) time.Time {
		return time.Date(2026, time.September, 1, hour, min, 0, 0, time.Local)
	}

	tests := []struct {
		name    string
		time    time.Time
		wantNum int
		wantErr bool
	}{
		{name: "during pair 1", time: at(8, 0), wantNum: 1},
		{name: "during pair 2", time: at(10, 0), wantNum: 2},
		{name: "during pair 3", time: at(12, 0), wantNum: 3},
		{name: "during pair 4", time: at(14, 0), wantNum: 4},
		{name: "during pair 5", time: at(16, 0), wantNum: 5},
		{name: "during pair 6", time: at(18, 0), wantNum: 6},
		{name: "during pair 7", time: at(20, 0), wantNum: 7},
		{name: "before first pair", time: at(7, 0), wantErr: true},
		{name: "after last pair", time: at(21, 0), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair, err := day.CurrentPair(tt.time)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("CurrentPair(%s) = %+v, want error", tt.time, pair)
				}
				return
			}
			if err != nil {
				t.Fatalf("CurrentPair(%s) unexpected error: %v", tt.time, err)
			}
			if pair.Number != tt.wantNum {
				t.Fatalf("CurrentPair(%s) = pair %d, want pair %d", tt.time, pair.Number, tt.wantNum)
			}
		})
	}
}

func TestScheduleDayCurrentPairInvalidTime(t *testing.T) {
	day := ScheduleDay{Pairs: []Pair{{Number: 1, StartTime: "abc", EndTime: "9:35"}}}
	_, err := day.CurrentPair(time.Date(2026, time.September, 1, 10, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("CurrentPair should fail on unparseable start time")
	}
}

func TestPair_IsEmpty(t *testing.T) {
	tests := []struct {
		kind PairKind
		want bool
	}{
		{kind: PairKindEmpty, want: true},
		{kind: PairKindEvent, want: true},
		{kind: PairKindSession, want: true},
		{kind: PairKindSubject, want: false},
		{kind: PairKindExam, want: false},
		{kind: PairKindConsultation, want: false},
	}

	for _, tt := range tests {
		p := Pair{Kind: tt.kind}
		if got := p.IsEmpty(); got != tt.want {
			t.Errorf("Pair{Kind: %q}.IsEmpty() = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestPair_IsEqual(t *testing.T) {
	a := testPair(1, PairKindSubject, "Математика", "8:00", "9:35")
	b := testPair(1, PairKindSubject, "Математика", "8:00", "9:35")
	if !a.IsEqual(&b) {
		t.Error("IsEqual should return true for equal pairs")
	}
	b.Discipline = "Физика"
	if a.IsEqual(&b) {
		t.Error("IsEqual should return false for different pairs")
	}
}

func TestPair_IsPassedAt(t *testing.T) {
	p := testPair(1, PairKindSubject, "Математика", "8:00", "9:45")
	at := func(hour, min int) time.Time {
		return time.Date(2026, time.September, 1, hour, min, 0, 0, time.Local)
	}

	if !p.IsPassedAt(at(10, 0)) {
		t.Error("pair should be passed after end time")
	}
	if p.IsPassedAt(at(9, 0)) {
		t.Error("pair should not be passed before end time")
	}
	if p.IsPassedAt(at(9, 45)) {
		t.Error("pair should not be passed exactly at end time")
	}
}

func TestPair_IsPassedAt_invalidTime(t *testing.T) {
	p := Pair{EndTime: "abc"}
	if p.IsPassedAt(time.Now()) {
		t.Error("IsPassedAt should return false for unparseable end time")
	}
}

func TestPair_HTML(t *testing.T) {
	tests := []struct {
		name string
		pair Pair
		want string
	}{
		{
			name: "subject",
			pair: Pair{
				Kind: PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:45",
				Discipline: "Математика", Teacher: "Иванов", Classroom: "215",
			},
			want: "1 | 8:00 - 9:45 | 215\n    <b>Математика</b>\n    Иванов",
		},
		{
			name: "exam",
			pair: Pair{
				Kind: PairKindExam, Number: 1, StartTime: "8:00", EndTime: "9:45",
				Label: "Экзамен", Discipline: "Физика", Teacher: "Петров", Classroom: "101",
			},
			want: "1 | 8:00 - 9:45 | 101\n    <i>Экзамен</i>\n    <b>Физика</b>\n    Петров",
		},
		{
			name: "event",
			pair: Pair{
				Kind: PairKindEvent, Number: 2, StartTime: "10:00", EndTime: "11:00", Label: "Линейка",
			},
			want: "2 | 10:00 - 11:00 — Линейка",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pair.HTML(); got != tt.want {
				t.Fatalf("HTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSchedule_JSON(t *testing.T) {
	group := Group{
		GroupID:        "-1",
		DepartmentID:   "-1",
		GroupName:      "Test",
		DepartmentName: "Test",
		Year:           2026,
	}
	teacherName := "Иванов Иван Иванович"
	schedule := RawSchedule{
		Config: GroupScheduleConfig(&group, false),
		Rows: []RawScheduleRow{
			{Number: 1, TimeRange: "8:00-9:35", Days: []RawScheduleDay{
				{
					Date:     "01.01.2026",
					WeekDay:  "Понедельник",
					WeekKind: "Четная",
					Pair: Pair{
						Kind:       PairKindEmpty,
						Number:     1,
						StartTime:  "8:00",
						EndTime:    "9:35",
						Label:      "",
						Title:      "",
						Discipline: "Информатика",
						Teacher:    teacherName,
						Classroom:  "404(404)",
						Replaced:   false,
					},
				},
			}},
		},
	}

	// Try to marshal schedule to JSON
	jsonBytes, err := schedule.JSON()
	if err != nil {
		t.Errorf("Failed to marshal schedule to JSON: %v", err)
	}

	// Try to unmarshal JSON back to schedule
	var unmarshaledSchedule RawSchedule
	err = json.Unmarshal(jsonBytes, &unmarshaledSchedule)
	if err != nil {
		t.Errorf("Failed to unmarshal JSON back to schedule: %v", err)
	}

	// Compare original and unmarshaled schedules
	if !reflect.DeepEqual(schedule, unmarshaledSchedule) {
		t.Logf("Config groups:\n%+v\n%+v", schedule.Config.Group, unmarshaledSchedule.Config.Group)
		t.Errorf("Original and unmarshaled schedules are not equal:\n%#v\n%#v", schedule, unmarshaledSchedule)
	}
	if newJSON, err := unmarshaledSchedule.JSON(); err != nil || !reflect.DeepEqual(jsonBytes, newJSON) {
		t.Errorf("Failed to marshal unmarshaled schedule to JSON again: %v", err)
	}
}
