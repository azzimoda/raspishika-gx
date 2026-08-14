package model

import "testing"

func TestRawSchedule_Transform(t *testing.T) {
	raw := RawSchedule{
		Config: ScheduleConfig{},
		Rows: []RawScheduleRow{
			{
				Number:    1,
				TimeRange: "8:00 - 9:35",
				Days: []RawScheduleDay{
					{Date: "01.09.2026", WeekDay: "вторник", WeekKind: "нечетная", Pair: Pair{Discipline: "Математика"}},
					{Date: "02.09.2026", WeekDay: "среда", WeekKind: "четная", Pair: Pair{Discipline: "Физика"}},
				},
			},
			{
				Number:    2,
				TimeRange: "9:45-11:20",
				Days: []RawScheduleDay{
					{Date: "01.09.2026", WeekDay: "вторник", WeekKind: "нечетная", Pair: Pair{Discipline: "Русский"}},
					{Date: "02.09.2026", WeekDay: "среда", WeekKind: "четная", Pair: Pair{Discipline: "Химия"}},
				},
			},
		},
	}

	schedule := raw.Transform()

	if len(schedule.Days) != 2 {
		t.Fatalf("Transform() produced %d days, want 2", len(schedule.Days))
	}

	first := schedule.Days[0]
	if first.Date != "01.09.2026" || first.Weekday != "вторник" || first.WeekKind != "нечетная" {
		t.Errorf("unexpected first day meta: %+v", first)
	}
	if len(first.Pairs) != 2 {
		t.Fatalf("first day has %d pairs, want 2", len(first.Pairs))
	}

	wantPairs := []Pair{
		{Discipline: "Математика", Number: 1, StartTime: "8:00", EndTime: "9:35"},
		{Discipline: "Русский", Number: 2, StartTime: "9:45", EndTime: "11:20"},
	}
	for i, want := range wantPairs {
		got := first.Pairs[i]
		if got.Number != want.Number || got.StartTime != want.StartTime ||
			got.EndTime != want.EndTime || got.Discipline != want.Discipline {
			t.Errorf("pair %d = %+v, want %+v", i, got, want)
		}
	}

	second := schedule.Days[1]
	if second.Date != "02.09.2026" || second.Pairs[0].Discipline != "Физика" || second.Pairs[1].Discipline != "Химия" {
		t.Errorf("unexpected second day: %+v", second)
	}
}
