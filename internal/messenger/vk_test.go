package messenger

import (
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestKeyboardFromOpts(t *testing.T) {
	if keyboardFromOpts(nil) != nil {
		t.Fatal("nil opts produced a keyboard")
	}
	days := []model.ScheduleDay{{Weekday: "понедельник"}, {Weekday: "вторник"}}
	opts := []SendOptions{{Buttons: &ScheduleButtons{
		Value:      "ИСПт-22-(9)-2",
		Days:       days,
		CurrentIdx: -1,
		LinkURL:    "https://coworking.tyuiu.ru/",
	}}}
	keyboard := keyboardFromOpts(opts)
	if keyboard == nil || !keyboard.Inline {
		t.Fatal("week view must render an inline keyboard")
	}
	if keyboard.Buttons[len(keyboard.Buttons)-1][len(keyboard.Buttons[len(keyboard.Buttons)-1])-1].Action.Label != "Обновить" {
		t.Fatal("week view lacks refresh button")
	}
}

func TestKeyboardFromOptsMarksCurrentDay(t *testing.T) {
	days := []model.ScheduleDay{{Weekday: "понедельник"}, {Weekday: "вторник"}}
	opts := []SendOptions{{Buttons: &ScheduleButtons{
		Value:      "ИСПт-22-(9)-2",
		Days:       days,
		CurrentIdx: 1,
	}}}
	keyboard := keyboardFromOpts(opts)
	var found bool
	for _, row := range keyboard.Buttons {
		for _, button := range row {
			if button.Action.Label == "[Вт]" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("current day button is not bracketed")
	}
}
