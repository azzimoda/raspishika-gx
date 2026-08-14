package scraper

import (
	"errors"
	"strings"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func mustParseSchedule(t *testing.T, html string, conf model.ScheduleConfig) *model.ScheduleData {
	t.Helper()
	schedule, err := parseSchedule(html, conf)
	if err != nil {
		t.Fatalf("parseSchedule() error: %v", err)
	}
	return schedule
}

const headerRow = `<tr><td></td><td></td>
	<td>01.09.2026<br>вторник<br>нечетная</td>
	<td>02.09.2026<br>среда<br>четная</td>
</tr>`

func TestParseScheduleSubjectGroup(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{GroupName: "ИСПт-22-(9)-2"}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td><span class="disc">Математика</span><span class="prep">Иванов</span><span class="cabs">215</span><span class="podgrupp">1</span></td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)

	if len(schedule.Days) != 2 {
		t.Fatalf("days = %d, want 2", len(schedule.Days))
	}
	day := schedule.Days[0]
	if day.Date != "01.09.2026" || day.Weekday != "вторник" || day.WeekKind != "нечетная" {
		t.Fatalf("unexpected day meta: %+v", day)
	}
	if len(day.Pairs) != 1 {
		t.Fatalf("pairs = %d, want 1", len(day.Pairs))
	}

	pair := day.Pairs[0]
	want := model.Pair{
		Kind: model.PairKindSubject, Number: 1, StartTime: "8:00", EndTime: "9:35",
		Discipline: "Математика", Teacher: "Иванов", Classroom: "215", Subgroup: "1",
	}
	if !pair.IsEqual(&want) {
		t.Fatalf("pair = %+v, want %+v", pair, want)
	}

	if got := schedule.Days[1].Pairs[0].Kind; got != model.PairKindEmpty {
		t.Fatalf("empty day pair kind = %q, want empty", got)
	}
}

func TestParseScheduleSubjectTeacher(t *testing.T) {
	conf := model.ScheduleConfig{Teacher: &model.Teacher{Name: "Иванов"}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td><span class="disc">Математика<div>ИСПт-22-(9)-2</div></span><span class="prep">Иванов</span><span class="cabs">215</span></td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)
	pair := schedule.Days[0].Pairs[0]

	if pair.Discipline != "Математика" {
		t.Errorf("discipline = %q, want Математика", pair.Discipline)
	}
	if pair.Group != "ИСПт-22-(9)-2" {
		t.Errorf("group = %q, want ИСПт-22-(9)-2", pair.Group)
	}
}

func TestParseScheduleExam(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td class="ekzamen"><span class="head_ekz">Экзамен</span><span class="prep">Петров</span><span class="cabs">101</span></td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)
	pair := schedule.Days[0].Pairs[0]

	want := model.Pair{
		Kind: model.PairKindExam, Number: 1, StartTime: "8:00", EndTime: "9:35",
		Title: "Экзамен", Teacher: "Петров", Classroom: "101",
	}
	if !pair.IsEqual(&want) {
		t.Fatalf("pair = %+v, want %+v", pair, want)
	}
}

func TestParseScheduleConsultation(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td><table class="consultation"><tr><td>Консультация</td></tr></table><span class="prep">Петров</span><span class="cabs">101</span></td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)
	pair := schedule.Days[0].Pairs[0]
	if pair.Kind != model.PairKindConsultation {
		t.Fatalf("kind = %q, want consultation", pair.Kind)
	}
}

func TestParseScheduleVacation(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td class="head_urok_kanik">Каникулы</td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)
	pair := schedule.Days[0].Pairs[0]
	if pair.Kind != model.PairKindVacation {
		t.Fatalf("kind = %q, want vacation", pair.Kind)
	}
	if pair.Label != "Каникулы" {
		t.Fatalf("label = %q, want Каникулы", pair.Label)
	}
}

func TestParseScheduleReplaced(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td><table class="zamena"><tr><td><span class="disc">Математика</span><span class="prep">Иванов</span><span class="cabs">215</span></td></tr></table></td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)
	pair := schedule.Days[0].Pairs[0]
	if !pair.Replaced {
		t.Fatal("replaced = false, want true")
	}
	if pair.Kind != model.PairKindSubject {
		t.Fatalf("kind = %q, want subject", pair.Kind)
	}
}

func TestParseScheduleCancelled(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>1</td>
			<td>8:00-9:35</td>
			<td><span class="disc">Занятие снято</span></td>
			<td></td>
		</tr>
	</table>`

	schedule := mustParseSchedule(t, html, conf)
	pair := schedule.Days[0].Pairs[0]
	if pair.Kind != model.PairKindEmpty {
		t.Fatalf("kind = %q, want empty", pair.Kind)
	}
}

func TestParseSchedulePanicsOnInvalidNumber(t *testing.T) {
	conf := model.ScheduleConfig{Group: &model.Group{}}
	html := `<table id="main_table">` + headerRow + `
		<tr class="para_num">
			<td>abc</td>
			<td>8:00-9:35</td>
			<td></td>
			<td></td>
		</tr>
	</table>`

	_, err := parseSchedule(html, conf)
	if !errors.Is(err, ErrParserPanicked) {
		t.Fatalf("error = %v, want ErrParserPanicked", err)
	}
}

func TestParseScheduleTableNotFound(t *testing.T) {
	_, err := parseSchedule(`<html><body><table id="other_table"></table></body></html>`, model.ScheduleConfig{})
	if err == nil || !strings.Contains(err.Error(), "table element not found") {
		t.Fatalf("error = %v, want table element not found", err)
	}
}
