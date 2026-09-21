package vkbotutil

import (
	"strconv"
	"strings"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

// Command strings embedded into keyboard payloads. The interactive bot parses
// them back from incoming message events.
const (
	CommandDay  = "day"
	CommandWeek = "week"
)

// ScheduleLinkLabel titles the button opening the college schedule page.
const ScheduleLinkLabel = "Сайт"

// vkDaysPerRow is the maximum number of buttons VK renders on one inline row.
const vkDaysPerRow = 5

// ScheduleKeyboard renders schedule navigation as a VK keyboard mirroring the
// Telegram schedule views: day-jump buttons plus a week, link and refresh
// button. currentIdx >= 0 marks a text day view (its button is bracketed);
// -1 means a full-week photo view.
func ScheduleKeyboard(value string, days []model.ScheduleDay, currentIdx int, linkURL string) *Keyboard {
	rows := dayJumpRows(value, days, currentIdx)
	bottom := make([]Button, 0, 2)
	if linkURL != "" {
		bottom = append(bottom, LinkButton(ScheduleLinkLabel, linkURL))
	}
	if currentIdx < 0 {
		bottom = append(bottom, TextButton("Обновить", CommandWeek+"\n"+value))
		rows = append(rows, bottom)
		return &Keyboard{Inline: true, Buttons: rows}
	}
	rows = append(rows, []Button{TextButton("Неделя", CommandWeek+"\n"+value)})
	bottom = append(bottom, TextButton("Обновить", CommandDay+"\n"+value+"\n"+strconv.Itoa(currentIdx)))
	rows = append(rows, bottom)
	return &Keyboard{Inline: true, Buttons: rows}
}

func dayJumpRows(value string, days []model.ScheduleDay, currentIdx int) [][]Button {
	rows := make([][]Button, 0, len(days)/vkDaysPerRow+1)
	row := make([]Button, 0, vkDaysPerRow)
	for i, day := range days {
		label := WeekdayAbbr(day.Weekday)
		if i == currentIdx {
			label = "[" + label + "]"
		}
		row = append(row, TextButton(label, CommandDay+"\n"+value+"\n"+strconv.Itoa(i)))
		if len(row) == vkDaysPerRow {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// WeekdayAbbr returns the short Russian weekday name used on day buttons.
func WeekdayAbbr(weekday string) string {
	switch strings.ToLower(weekday) {
	case "понедельник":
		return "Пн"
	case "вторник":
		return "Вт"
	case "среда":
		return "Ср"
	case "четверг":
		return "Чт"
	case "пятница":
		return "Пт"
	case "суббота":
		return "Сб"
	case "воскресенье":
		return "Вс"
	}
	return weekday
}
