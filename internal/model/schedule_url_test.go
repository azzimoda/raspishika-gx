package model

import "testing"

func TestScheduleURLGroup(t *testing.T) {
	conf := ScheduleConfig{
		Group: &Group{DepartmentID: "15", GroupID: "205", Year: 2026, DepartmentName: "Отделение СОНХ"},
	}

	want := "https://coworking.tyuiu.ru/shs/all_t/sh.php?action=group&union=0&sid=15&gr=205&year=2026&vr=1"
	if got := ScheduleURL(conf, nil); got != want {
		t.Fatalf("ScheduleURL() = %q, want %q", got, want)
	}
}

func TestScheduleURLExtramuralGroup(t *testing.T) {
	conf := ScheduleConfig{
		Group: &Group{DepartmentID: "15", GroupID: "205", Year: 2026, DepartmentName: "Отделение заочного обучения"},
	}

	want := "https://coworking.tyuiu.ru/shs/all_t/shz.php?action=group&union=0&sid=15&gr=205&year=2026&vr=1"
	if got := ScheduleURL(conf, nil); got != want {
		t.Fatalf("ScheduleURL() = %q, want %q", got, want)
	}
}

func TestScheduleURLTeacher(t *testing.T) {
	conf := ScheduleConfig{Teacher: &Teacher{TeacherID: "123", Name: "Иванов"}}
	departments := []Department{
		{ID: "15", Year: 2026},
		{ID: "17", Year: 2026},
	}

	want := "https://coworking.tyuiu.ru/shs/all_t/sh.php?action=prep&prep=123&vr=1&count=2" +
		"&shed[0]=15&union[0]=0&year[0]=2026" +
		"&shed[1]=17&union[1]=0&year[1]=2026"
	if got := ScheduleURL(conf, departments); got != want {
		t.Fatalf("ScheduleURL() = %q, want %q", got, want)
	}
}

func TestScheduleURLNoConfig(t *testing.T) {
	if got := ScheduleURL(ScheduleConfig{}, nil); got != "" {
		t.Fatalf("ScheduleURL() = %q, want empty", got)
	}
}
