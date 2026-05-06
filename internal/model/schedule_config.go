package model

import "fmt"

func GroupScheduleConfig(group *Group, isDark bool) ScheduleConfig {
	return ScheduleConfig{Group: group, IsDark: isDark}
}
func TeacherScheduleConfig(teacher *Teacher, isDark bool) ScheduleConfig {
	return ScheduleConfig{Teacher: teacher, IsDark: isDark}
}

type ScheduleConfig struct {
	Group   *Group   `json:"group"`
	Teacher *Teacher `json:"teacher"`
	IsDark  bool     `json:"is_dark"`
}

func (cs *ScheduleConfig) String() string {
	mode := "light"
	if cs.IsDark {
		mode = "dark"
	}

	if cs.Group != nil {
		return fmt.Sprintf("schedule_%s_%s_%s", cs.Group.DepartmentName, cs.Group.GroupName, mode)
	} else if cs.Teacher != nil {
		return fmt.Sprintf("teacher_%s_%s", cs.Teacher.Name.Safe(), mode)
	}
	return ""
}

func (sc *ScheduleConfig) FormatHTML() string {
	switch {
	case sc.Group != nil:
		return fmt.Sprintf("Расписание группы — <i>%s</i>", sc.Group.GroupName)
	case sc.Teacher != nil:
		return fmt.Sprintf("Расписание преподавателя — <i>%s</i>", sc.Teacher.Name)
	default:
		return "?"
	}
}

func (s *ScheduleConfig) IsEqual(other *ScheduleConfig) bool {
	if s.Group != nil && other.Group != nil {
		return s.Group.ID == other.Group.ID
	} else if s.Teacher != nil && other.Teacher != nil {
		return s.Teacher.ID == other.Teacher.ID
	} else if (*s == ScheduleConfig{}) && (*other == ScheduleConfig{}) {
		return true
	}
	return false
}
