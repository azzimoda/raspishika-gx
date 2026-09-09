package messenger

import (
	"testing"

	botutil "github.com/azzimoda/raspishika-gx/internal/bot/util"
	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/go-telegram/bot/models"
)

// minimal ScheduleButtons for the markup mapper tests.
func testButtons(idx int) *ScheduleButtons {
	return &ScheduleButtons{
		Value:      "ИСПт-22-(9)-2",
		Days:       []model.ScheduleDay{{Date: "2026-09-01", Weekday: "вторник"}, {Date: "2026-09-02", Weekday: "среда"}},
		CurrentIdx: idx,
		LinkURL:    "https://coworking.tyuiu.ru",
	}
}

func TestScheduleMarkupWeek(t *testing.T) {
	got := scheduleMarkup(testButtons(-1))
	want := botutil.WeekScheduleMarkupFromValue("ИСПт-22-(9)-2", testButtons(-1).Days, testButtons(-1).LinkURL)
	if !equalKeyboard(got, want) {
		t.Fatalf("week markup mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestScheduleMarkupDay(t *testing.T) {
	got := scheduleMarkup(testButtons(1))
	want := botutil.DayScheduleMarkup("ИСПт-22-(9)-2", testButtons(1).Days, 1, testButtons(1).LinkURL)
	if !equalKeyboard(got, want) {
		t.Fatalf("day markup mismatch:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestScheduleMarkupFromOpts(t *testing.T) {
	markup, ok := scheduleMarkupFromOpts(nil)
	if ok || markup.InlineKeyboard != nil {
		t.Fatalf("no options must not produce markup: %+v", markup)
	}

	markup, ok = scheduleMarkupFromOpts([]SendOptions{{Buttons: testButtons(-1)}})
	if !ok || len(markup.InlineKeyboard) == 0 {
		t.Fatalf("buttons option must produce markup: %+v", markup)
	}

	markup, ok = scheduleMarkupFromOpts([]SendOptions{{}, {Buttons: testButtons(-1)}})
	if !ok || len(markup.InlineKeyboard) == 0 {
		t.Fatalf("buttons option after an empty one must still produce markup: %+v", markup)
	}
}

func equalKeyboard(a, b models.InlineKeyboardMarkup) bool {
	if len(a.InlineKeyboard) != len(b.InlineKeyboard) {
		return false
	}
	for i := range a.InlineKeyboard {
		if len(a.InlineKeyboard[i]) != len(b.InlineKeyboard[i]) {
			return false
		}
		for j := range a.InlineKeyboard[i] {
			x, y := a.InlineKeyboard[i][j], b.InlineKeyboard[i][j]
			if x.Text != y.Text || x.CallbackData != y.CallbackData || x.URL != y.URL {
				return false
			}
		}
	}
	return true
}
