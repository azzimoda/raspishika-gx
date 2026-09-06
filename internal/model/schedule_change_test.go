package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestSynchronize(t *testing.T) {
	schedule1 := ScheduleData{
		Days: []ScheduleDay{
			{Date: "2025-01-01"},
			{Date: "2027-01-02"},
			{Date: "2025-01-03"},
			{Date: "2025-01-04"},
			{Date: "2025-01-05"},
			{Date: "2025-01-06"},
			{Date: "2025-01-07"},
		},
	}
	schedule2 := ScheduleData{
		Days: []ScheduleDay{
			{Date: "2027-01-02"},
			{Date: "2025-01-03"},
			{Date: "2025-01-04"},
			{Date: "2025-01-05"},
			{Date: "2025-01-06"},
			{Date: "2025-01-07"},
			{Date: "2025-01-08"},
		},
	}
	testCases := []struct {
		name     string
		old, new ScheduleData
	}{
		{"empty", ScheduleData{}, ScheduleData{}},
		{"same", schedule1, schedule1},
		{"different", schedule1, schedule2},
	}
	for _, tt := range testCases {
		gotOld, gotNew := Synchronize(tt.old, tt.new)
		t.Run(tt.name, func(t *testing.T) {
			if !reflect.DeepEqual(gotOld, gotNew) {
				t.Errorf("Old and new schedules expected to be equas after synchronization, got %v and %v", gotOld, gotNew)
			} else {
				t.Log("Ok")
			}
		})
	}
}

func TestSchedule(t *testing.T) {
	pairEmpty := Pair{Kind: PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"}
	pair1 := Pair{
		Kind:       PairKindSubject,
		Number:     1,
		StartTime:  "8:00",
		EndTime:    "9:35",
		Discipline: "Pair 1",
		Teacher:    "Teacher 1",
		Classroom:  "1(1)",
	}
	pair2 := Pair{
		Kind:       PairKindSubject,
		Number:     2,
		StartTime:  "8:00",
		EndTime:    "9:35",
		Discipline: "Pair 2",
		Teacher:    "Teacher 2",
		Classroom:  "2(2)",
	}
	empty2 := Pair{Kind: PairKindEmpty, Number: 2, StartTime: "8:00", EndTime: "9:35"}

	testCases := []struct {
		name     string
		old, new ScheduleDay
		want     []Diff
	}{
		{name: "empty -> empty",
			old:  ScheduleDay{Pairs: []Pair{pairEmpty}},
			new:  ScheduleDay{Pairs: []Pair{pairEmpty}},
			want: nil,
		},
		{name: "pair1 -> pair1",
			old:  ScheduleDay{Pairs: []Pair{pair1}},
			new:  ScheduleDay{Pairs: []Pair{pair1}},
			want: nil,
		},
		{name: "empty -> pair1",
			old: ScheduleDay{Pairs: []Pair{pairEmpty}},
			new: ScheduleDay{Pairs: []Pair{pair1}},
			want: []Diff{{
				OldDay:  new(ScheduleDay{Pairs: []Pair{pairEmpty}}),
				NewDay:  new(ScheduleDay{Pairs: []Pair{pair1}}),
				OldPair: pairEmpty, NewPair: pair1,
			}},
		},
		{name: "pair1 -> pair2",
			old: ScheduleDay{Pairs: []Pair{pair1}},
			new: ScheduleDay{Pairs: []Pair{pair2}},
			want: []Diff{{
				OldDay:  new(ScheduleDay{Pairs: []Pair{pair1}}),
				NewDay:  new(ScheduleDay{Pairs: []Pair{pair2}}),
				OldPair: pair1, NewPair: pairEmpty,
			}, {
				OldDay:  new(ScheduleDay{Pairs: []Pair{pair1}}),
				NewDay:  new(ScheduleDay{Pairs: []Pair{pair2}}),
				OldPair: empty2, NewPair: pair2,
			}},
		},
		{name: "pair2 -> pair1",
			old: ScheduleDay{Pairs: []Pair{pair2}},
			new: ScheduleDay{Pairs: []Pair{pair1}},
			want: []Diff{{
				OldDay:  new(ScheduleDay{Pairs: []Pair{pair2}}),
				NewDay:  new(ScheduleDay{Pairs: []Pair{pair1}}),
				OldPair: pairEmpty, NewPair: pair1,
			}, {
				OldDay:  new(ScheduleDay{Pairs: []Pair{pair2}}),
				NewDay:  new(ScheduleDay{Pairs: []Pair{pair1}}),
				OldPair: pair2, NewPair: empty2,
			}},
		},
	}
	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScheduleChange(
				ScheduleData{Days: []ScheduleDay{tt.old}},
				ScheduleData{Days: []ScheduleDay{tt.new}},
			)
			got := s.Diffs()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unexpected diffs:\nwant %#v,\n got %#v", tt.want, got)
			}
		})
	}
}

func TestScheduleChangeVariablePairCounts(t *testing.T) {
	pair1 := Pair{Kind: PairKindSubject, Number: 1, StartTime: "08:00", EndTime: "09:30", Discipline: "Математика"}
	pair2 := Pair{Kind: PairKindSubject, Number: 2, StartTime: "09:45", EndTime: "11:15", Discipline: "Физика"}
	pair3 := Pair{Kind: PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:00", Discipline: "Химия"}
	type outcome struct {
		number         int
		added, removed bool
	}
	tests := []struct {
		name               string
		oldPairs, newPairs []Pair
		want               []outcome
	}{
		{name: "first lesson added", newPairs: []Pair{pair1}, want: []outcome{{1, true, false}}},
		{name: "last lesson removed", oldPairs: []Pair{pair1}, want: []outcome{{1, false, true}}},
		{name: "middle lesson removed", oldPairs: []Pair{pair1, pair2, pair3}, newPairs: []Pair{pair1, pair3}, want: []outcome{{2, false, true}}},
		{name: "new slot added", oldPairs: []Pair{pair1}, newPairs: []Pair{pair1, pair3}, want: []outcome{{3, true, false}}},
		{name: "reordered source rows", oldPairs: []Pair{pair1, pair2, pair3}, newPairs: []Pair{pair3, pair1, pair2}},
		{name: "both empty"},
		{name: "empty slot metadata", oldPairs: []Pair{{Kind: PairKindEmpty, Number: 1, StartTime: "08:00"}}, newPairs: []Pair{{Kind: PairKindEmpty, Number: 1, StartTime: "08:15"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			change := NewScheduleChange(
				ScheduleData{Days: []ScheduleDay{{Date: "01.09.2026", Pairs: tt.oldPairs}}},
				ScheduleData{Days: []ScheduleDay{{Date: "01.09.2026", Pairs: tt.newPairs}}},
			)
			var got []outcome
			for _, diff := range change.Diffs() {
				got = append(got, outcome{diff.Number(), diff.IsAdded(), diff.IsCancelled()})
				if diff.OldPair.Number != diff.NewPair.Number {
					t.Fatal("placeholder did not preserve slot identity")
				}
				if diff.IsAdded() && diff.OldPair.Kind != PairKindEmpty {
					t.Fatal("added lesson has no empty old placeholder")
				}
				if diff.IsCancelled() && diff.NewPair.Kind != PairKindEmpty {
					t.Fatal("removed lesson has no empty new placeholder")
				}
				if diff.HTML() == "?" {
					t.Fatal("same-date change was treated as movement to another day")
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("changes = %#v, want %#v", got, tt.want)
			}
			html := change.HTML()
			if len(tt.want) == 0 && html != "Изменения не обнаружены" {
				t.Fatalf("unexpected empty report: %s", html)
			}
			for _, want := range tt.want {
				if want.added && !strings.Contains(html, "Добавлено:") {
					t.Fatalf("addition missing from report: %s", html)
				}
				if want.removed && !strings.Contains(html, "Снято:") {
					t.Fatalf("cancellation missing from report: %s", html)
				}
			}
		})
	}
}

func TestScheduleChangeAlignsExactDatesAcrossRollingWindow(t *testing.T) {
	pair := Pair{Kind: PairKindSubject, Number: 1, StartTime: "08:00", EndTime: "09:30", Discipline: "Математика", Classroom: "101"}
	replacement := pair
	replacement.Classroom = "215"
	old := ScheduleData{Days: []ScheduleDay{
		{Date: "01.09.2026", Pairs: []Pair{pair}},
		{Date: "02.09.2026", Pairs: []Pair{pair}},
		{Date: "03.09.2026", Pairs: []Pair{pair}},
		{Date: "04.09.2026", Pairs: []Pair{pair}},
	}}
	new := ScheduleData{Days: []ScheduleDay{
		{Date: "04.09.2026", Pairs: []Pair{pair}},
		{Date: "03.09.2026", Pairs: []Pair{replacement}},
		{Date: "05.09.2026", Pairs: []Pair{replacement}},
	}}
	// Direct construction must be as safe as NewScheduleChange.
	change := &ScheduleChange{Old: old, New: new}
	diffs := change.Diffs()
	if len(diffs) != 1 || diffs[0].Day().Date != "03.09.2026" || !diffs[0].IsClassroomChanged() {
		t.Fatalf("incorrect day alignment: %+v", diffs)
	}
	if diffs[0].OldPair.Classroom != "101" || diffs[0].NewPair.Classroom != "215" {
		t.Fatal("wrong lesson compared")
	}
	if diffs[0].HTML() == "?" {
		t.Fatal("classroom update was treated as movement")
	}
	if len(old.Days) != 4 || len(new.Days) != 3 || old.Days[0].Date != "01.09.2026" || new.Days[0].Date != "04.09.2026" {
		t.Fatal("comparison mutated source schedules")
	}
	if len(NewScheduleChange(old, new).Diffs()) != 1 {
		t.Fatal("constructor and direct construction disagree")
	}
	// Entirely disjoint horizons do not compare unrelated lessons by index.
	if got := NewScheduleChange(ScheduleData{Days: old.Days[:1]}, ScheduleData{Days: new.Days[2:]}).Diffs(); len(got) != 0 {
		t.Fatalf("unrelated dates generated changes: %+v", got)
	}
}

func TestScheduleChangeMissingSchedulesAndGroupMismatch(t *testing.T) {
	day := ScheduleDay{Date: "01.09.2026", Pairs: []Pair{{Kind: PairKindSubject, Number: 1}}}
	for _, change := range []*ScheduleChange{
		NewScheduleChange(ScheduleData{}, ScheduleData{Days: []ScheduleDay{day}}),
		NewScheduleChange(ScheduleData{Days: []ScheduleDay{day}}, ScheduleData{}),
	} {
		if len(change.Diffs()) != 0 {
			t.Fatal("missing horizon generated spurious lesson changes")
		}
	}
	groupA := &Group{GroupID: "1", DepartmentID: "1"}
	groupB := &Group{GroupID: "2", DepartmentID: "1"}
	if NewScheduleChange(ScheduleData{Config: GroupScheduleConfig(groupA, false)}, ScheduleData{Config: GroupScheduleConfig(groupB, false)}) != nil {
		t.Fatal("different study groups accepted for comparison")
	}
}
