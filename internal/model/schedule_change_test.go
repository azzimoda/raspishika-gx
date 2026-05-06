package model

import (
	"reflect"
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
	teacher1 := "Teacher 1"
	teacher2 := "Teacher 2"

	pairEmpty := Pair{Kind: PairKindEmpty}
	pair1 := Pair{
		Kind:       PairKindSubject,
		Number:     1,
		StartTime:  "8:00",
		EndTime:    "9:35",
		Discipline: "Pair 1",
		Teacher:    &teacher1,
		Classroom:  "1(1)",
	}
	pair2 := Pair{
		Kind:       PairKindSubject,
		Number:     2,
		StartTime:  "8:00",
		EndTime:    "9:35",
		Discipline: "Pair 2",
		Teacher:    &teacher2,
		Classroom:  "2(2)",
	}

	testCases := []struct {
		name     string
		old, new ScheduleDay
		want     []Diff
	}{
		{"empty -> empty",
			ScheduleDay{Pairs: []Pair{pairEmpty}},
			ScheduleDay{Pairs: []Pair{pairEmpty}},
			nil,
		},
		{"pair1 -> pair1",
			ScheduleDay{Pairs: []Pair{pair1}},
			ScheduleDay{Pairs: []Pair{pair1}},
			nil,
		},
		{"empty -> pair1",
			ScheduleDay{Pairs: []Pair{pairEmpty}},
			ScheduleDay{Pairs: []Pair{pair1}},
			[]Diff{{OldPair: pairEmpty, NewPair: pair1}},
		},
		{"pair1 -> pair2",
			ScheduleDay{Pairs: []Pair{pair1}},
			ScheduleDay{Pairs: []Pair{pair2}},
			[]Diff{{OldPair: pair1, NewPair: pair2}},
		},
		{"pair2 -> pair1",
			ScheduleDay{Pairs: []Pair{pair2}},
			ScheduleDay{Pairs: []Pair{pair1}},
			[]Diff{{OldPair: pair2, NewPair: pair1}},
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
				t.Errorf("Expected %#v, got %#v", tt.want, got)
			}
		})
	}
}
