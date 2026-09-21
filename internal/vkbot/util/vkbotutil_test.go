package vkbotutil

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestTextButtonJSON(t *testing.T) {
	button := TextButton("Сегодня", "today")
	if button.Action.Type != "text" || button.Action.Label != "Сегодня" || button.Action.Link != "" {
		t.Fatalf("unexpected button: %+v", button)
	}
	var payload struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(button.Action.Payload), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Command != "today" {
		t.Fatalf("payload command = %q", payload.Command)
	}
}

func TestLinkButton(t *testing.T) {
	button := LinkButton(ScheduleLinkLabel, "https://coworking.tyuiu.ru/")
	if button.Action.Type != "open_link" || button.Action.Link == "" {
		t.Fatalf("unexpected link button: %+v", button)
	}
}

func TestKeyboardJSONEncodesInline(t *testing.T) {
	keyboard := &Keyboard{Inline: true, Buttons: [][]Button{{TextButton("Завтра", "tomorrow")}}}
	jsonString, err := keyboard.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonString, `"inline":true`) {
		t.Fatalf("inline flag missing: %s", jsonString)
	}
	if !strings.Contains(jsonString, `"buttons"`) {
		t.Fatalf("buttons missing: %s", jsonString)
	}
}

func TestNilKeyboardJSONIsEmpty(t *testing.T) {
	jsonString, err := (*Keyboard)(nil).JSON()
	if err != nil || jsonString != "" {
		t.Fatalf("(%q, %v)", jsonString, err)
	}
}

func TestPageNumber(t *testing.T) {
	if got := PageNumber("2"); got != 2 {
		t.Fatalf("PageNumber(2) = %d", got)
	}
	if got := PageNumber("abc"); got != 0 {
		t.Fatalf("PageNumber(abc) = %d", got)
	}
	if got := PageNumber("-1"); got != 0 {
		t.Fatalf("PageNumber(-1) = %d", got)
	}
}

func TestPagedKeyboardClampsPage(t *testing.T) {
	items := make([]MenuItem, 6)
	for i := range items {
		items[i] = MenuItem{Label: "x", Command: "item"}
	}
	// 6 items, 6 per page: one page, so page 5 must clamp back.
	keyboard := PagedKeyboard(items, 5, "page", "back")
	commands := collectCommands(keyboard)
	foundNav := false
	for _, c := range commands {
		if strings.HasPrefix(c, "page\n") {
			foundNav = true
		}
	}
	if foundNav {
		t.Fatal("clamping to the only page must not leave navigation buttons")
	}
	if len(keyboard.Buttons) != 4 {
		t.Fatalf("expected 3 item rows + back row, got %d", len(keyboard.Buttons))
	}
}

func TestShortLabel(t *testing.T) {
	long := strings.Repeat("ж", 60)
	if got := shortLabel(long); !strings.HasSuffix(got, "…") || len([]rune(got)) > 40 {
		t.Fatalf("long label not truncated: %q (%d runes)", got, len([]rune(got)))
	}
	if shortLabel("коротко") != "коротко" {
		t.Fatal("short label changed")
	}
}

func TestWeekdayAbbr(t *testing.T) {
	cases := map[string]string{
		"понедельник": "Пн", "вторник": "Вт", "среда": "Ср", "четверг": "Чт",
		"пятница": "Пт", "суббота": "Сб", "воскресенье": "Вс", "Пятница": "Пт", "": "",
	}
	for input, want := range cases {
		if got := WeekdayAbbr(input); got != want {
			t.Errorf("WeekdayAbbr(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestScheduleKeyboardFlatLayouts(t *testing.T) {
	days := []model.ScheduleDay{
		{Weekday: "понедельник"}, {Weekday: "вторник"}, {Weekday: "среда"}, {Weekday: "четверг"},
		{Weekday: "пятница"}, {Weekday: "суббота"}, {Weekday: "воскресенье"},
	}
	t.Run("week", func(t *testing.T) {
		keyboard := ScheduleKeyboard("ИСПт-22-(9)-2", days, -1, "https://coworking.tyuiu.ru/")
		commands := collectCommands(keyboard)
		hasWeek := false
		for _, c := range commands {
			if c == "week\nИСПт-22-(9)-2" {
				hasWeek = true
			}
		}
		if !hasWeek {
			t.Fatalf("week view missing refresh command: %v", commands)
		}
		if !hasLink(keyboard) {
			t.Fatal("week view missing link button")
		}
	})
	t.Run("day", func(t *testing.T) {
		keyboard := ScheduleKeyboard("иванов", days, 2, "")
		commands := collectCommands(keyboard)
		hasDay := false
		for _, c := range commands {
			if c == "day\nиванов\n2" {
				hasDay = true
			}
		}
		if !hasDay {
			t.Fatalf("day view missing refresh command: %v", commands)
		}
		hasWeek := false
		for _, c := range commands {
			if c == "week\nиванов" {
				hasWeek = true
			}
		}
		if !hasWeek {
			t.Fatalf("day view missing week button: %v", commands)
		}
	})
}

func collectCommands(keyboard *Keyboard) []string {
	var commands []string
	for _, row := range keyboard.Buttons {
		for _, button := range row {
			if button.Action.Payload == "" {
				continue
			}
			var payload struct {
				Command string `json:"command"`
			}
			if json.Unmarshal([]byte(button.Action.Payload), &payload) == nil {
				commands = append(commands, payload.Command)
			}
		}
	}
	return commands
}

func hasLink(keyboard *Keyboard) bool {
	for _, row := range keyboard.Buttons {
		for _, button := range row {
			if button.Action.Type == "open_link" && button.Action.Link != "" {
				return true
			}
		}
	}
	return false
}
