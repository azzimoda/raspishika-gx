package model

import (
	"fmt"
	"strconv"
	"strings"
)

// scheduleBaseURL is the base of the schedule pages.
const scheduleBaseURL = "https://coworking.tyuiu.ru/shs/all_t/sh%s.php"

// ScheduleURL builds the URL of the schedule page for the given config
// and departments. For a group the departments are not needed,
// for a teacher they are used to build the "shed" query arguments.
// Note: the teacher schedule query must use literal square brackets
// ("shed[0]=..."), percent-encoded ones result in HTTP 403.
func ScheduleURL(conf ScheduleConfig, departments []Department) string {
	switch {
	case conf.Group != nil:
		zFlag := "" // Заочное обучение?
		if strings.Contains(strings.ToLower(conf.Group.DepartmentName), "заоч") {
			zFlag = "z"
		}
		return fmt.Sprintf(
			scheduleBaseURL+"?action=group&union=0&sid=%s&gr=%s&year=%d&vr=1",
			zFlag, conf.Group.DepartmentID, conf.Group.GroupID, conf.Group.Year)
	case conf.Teacher != nil:
		var b strings.Builder
		b.WriteString("https://coworking.tyuiu.ru/shs/all_t/sh.php?action=prep&prep=")
		b.WriteString(conf.Teacher.TeacherID)
		b.WriteString("&vr=1&count=")
		b.WriteString(strconv.Itoa(len(departments)))
		for i, d := range departments {
			fmt.Fprintf(&b, "&shed[%d]=%s&union[%d]=0&year[%d]=%d", i, d.ID, i, i, d.Year)
		}
		return b.String()
	default:
		// Error: invalid config
		return ""
	}
}
