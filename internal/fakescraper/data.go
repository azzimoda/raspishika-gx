package fakescraper

import "github.com/azzimoda/raspishika-gx/internal/model"

var FakeDepartments = []model.Department{
	{Name: "Отделение 1"},
	{Name: "Отделение 2"},
}

// [model.Department.Name] => [][model.Group]
var FakeGroups = map[string][]model.Group{
	FakeDepartments[0].Name: {
		// Группа без расписания (пустые пары)
		{GroupID: "0", DepartmentID: "0", GroupName: "ГБРф-26-(11)-1", DepartmentName: FakeDepartments[0].Name, Year: 2026},
		// Группа с расписанием (есть пары)
		{GroupID: "1", DepartmentID: "0", GroupName: "ГСРф-26-(11)-1", DepartmentName: FakeDepartments[0].Name, Year: 2026},
	},
	FakeDepartments[1].Name: {
		// Группа с пактиок (учебная + производственная практика)
		{GroupID: "2", DepartmentID: "1", GroupName: "ГСПф-26-(11)-1", DepartmentName: FakeDepartments[1].Name, Year: 2026},
		// Группа с зачётами
		{GroupID: "3", DepartmentID: "1", GroupName: "ГСЗф-26-(11)-1", DepartmentName: FakeDepartments[1].Name, Year: 2026},
		// Группа всегда возвращающая ошибку
		{GroupID: "4", DepartmentID: "1", GroupName: "ГВОф-26-(11)-1", DepartmentName: FakeDepartments[1].Name, Year: 2026},
	},
}

var FakeTeachers = []model.Teacher{
	{TeacherID: "0", Name: "Иванов Иван Иванович"},
	{TeacherID: "1", Name: "Петров Пётр Петрович"},
}

// [model.Group.GroupName] or [model.Teacher.Name] => [model.ScheduleData]
var FakeSchedules = map[string]model.ScheduleData{
	string(FakeGroups["Отделение 1"][0].GroupName): {
		Config: model.ScheduleConfig{Group: &FakeGroups["Отделение 1"][0]},
		Days: []model.ScheduleDay{
			{
				Date: "2026-09-01", Weekday: "вторник", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-02", Weekday: "среда", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-03", Weekday: "четверг", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-04", Weekday: "пятница", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-05", Weekday: "суббота", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-07", Weekday: "понедельник", WeekKind: "четная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
		},
	},
	string(FakeGroups["Отделение 1"][1].GroupName): {
		Config: model.ScheduleConfig{Group: &FakeGroups["Отделение 1"][1]},
		Days: []model.ScheduleDay{
			{
				Date: "2026-09-01", Weekday: "вторник", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35", Discipline: "Математика", Teacher: FakeTeachers[1].Name, Replaced: true},
					{Kind: model.PairKindSubject, Number: 2, StartTime: "9:45", EndTime: "11:20", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:05", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 4, StartTime: "13:45", EndTime: "15:20", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 5, StartTime: "15:30", EndTime: "17:05", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 6, StartTime: "17:15", EndTime: "18:50", Replaced: true},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-02", Weekday: "среда", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35", Discipline: "Математика", Teacher: FakeTeachers[1].Name, Replaced: true},
					{Kind: model.PairKindSubject, Number: 2, StartTime: "9:45", EndTime: "11:20", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:05", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 4, StartTime: "13:45", EndTime: "15:20", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 5, StartTime: "15:30", EndTime: "17:05", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 6, StartTime: "17:15", EndTime: "18:50", Replaced: true},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-03", Weekday: "четверг", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35", Discipline: "Математика", Teacher: FakeTeachers[1].Name, Replaced: true},
					{Kind: model.PairKindSubject, Number: 2, StartTime: "9:45", EndTime: "11:20", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:05", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 4, StartTime: "13:45", EndTime: "15:20", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 5, StartTime: "15:30", EndTime: "17:05", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 6, StartTime: "17:15", EndTime: "18:50", Replaced: true},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-04", Weekday: "пятница", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35", Discipline: "Математика", Teacher: FakeTeachers[1].Name, Replaced: true},
					{Kind: model.PairKindSubject, Number: 2, StartTime: "9:45", EndTime: "11:20", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:05", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 4, StartTime: "13:45", EndTime: "15:20", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 5, StartTime: "15:30", EndTime: "17:05", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 6, StartTime: "17:15", EndTime: "18:50", Replaced: true},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-05", Weekday: "суббота", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35", Discipline: "Математика", Teacher: FakeTeachers[1].Name, Replaced: true},
					{Kind: model.PairKindSubject, Number: 2, StartTime: "9:45", EndTime: "11:20", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:05", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 4, StartTime: "13:45", EndTime: "15:20", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 5, StartTime: "15:30", EndTime: "17:05", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 6, StartTime: "17:15", EndTime: "18:50", Replaced: true},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-07", Weekday: "понеделник", WeekKind: "четная",
				Pairs: []model.Pair{
					{Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35", Discipline: "Математика", Teacher: FakeTeachers[1].Name, Replaced: true},
					{Kind: model.PairKindSubject, Number: 2, StartTime: "9:45", EndTime: "11:20", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 3, StartTime: "11:30", EndTime: "13:05", Discipline: "Математика", Teacher: FakeTeachers[0].Name},
					{Kind: model.PairKindSubject, Number: 4, StartTime: "13:45", EndTime: "15:20", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 5, StartTime: "15:30", EndTime: "17:05", Discipline: "Математика", Teacher: FakeTeachers[1].Name},
					{Kind: model.PairKindSubject, Number: 6, StartTime: "17:15", EndTime: "18:50", Replaced: true},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
		},
	},
	string(FakeGroups["Отделение 2"][0].GroupName): {
		Config: model.ScheduleConfig{Group: &FakeGroups["Отделение 2"][0]},
		Days: []model.ScheduleDay{
			{
				Date: "2026-09-01", Weekday: "вторник", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-02", Weekday: "среда", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-03", Weekday: "четверг", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-04", Weekday: "пятница", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-05", Weekday: "суббота", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-07", Weekday: "понедельник", WeekKind: "четная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
		},
	},
	string(FakeGroups["Отделение 2"][1].GroupName): {
		Config: model.ScheduleConfig{Group: &FakeGroups["Отделение 2"][1]},
		Days: []model.ScheduleDay{
			{
				Date: "2026-09-01", Weekday: "вторник", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-02", Weekday: "среда", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-03", Weekday: "четверг", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-04", Weekday: "пятница", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-05", Weekday: "суббота", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-07", Weekday: "понедельник", WeekKind: "четная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
		},
	},
	// string(FakeGroups["Отделение 2"][2].GroupName): {},
	FakeTeachers[0].Name: {
		Config: model.ScheduleConfig{Teacher: &FakeTeachers[0]},
		Days: []model.ScheduleDay{
			{
				Date: "2026-09-01", Weekday: "вторник", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-02", Weekday: "среда", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-03", Weekday: "четверг", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-04", Weekday: "пятница", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-05", Weekday: "суббота", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-07", Weekday: "понедельник", WeekKind: "четная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
		},
	},
	FakeTeachers[1].Name: {
		Config: model.ScheduleConfig{Teacher: &FakeTeachers[1]},
		Days: []model.ScheduleDay{
			{
				Date: "2026-09-01", Weekday: "вторник", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-02", Weekday: "среда", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-03", Weekday: "четверг", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-04", Weekday: "пятница", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-05", Weekday: "суббота", WeekKind: "нечетная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
			{
				Date: "2026-09-07", Weekday: "понедельник", WeekKind: "четная",
				Pairs: []model.Pair{
					{Kind: model.PairKindEmpty, Number: 1, StartTime: "8:00", EndTime: "9:35"},
					{Kind: model.PairKindEmpty, Number: 2, StartTime: "9:45", EndTime: "11:20"},
					{Kind: model.PairKindEmpty, Number: 3, StartTime: "11:30", EndTime: "13:05"},
					{Kind: model.PairKindEmpty, Number: 4, StartTime: "13:45", EndTime: "15:20"},
					{Kind: model.PairKindEmpty, Number: 5, StartTime: "15:30", EndTime: "17:05"},
					{Kind: model.PairKindEmpty, Number: 6, StartTime: "17:15", EndTime: "18:50"},
					{Kind: model.PairKindEmpty, Number: 7, StartTime: "19:00", EndTime: "20:35"},
				},
			},
		},
	},
} // TODO: Make schedules different.
