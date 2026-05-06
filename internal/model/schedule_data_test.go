package model_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestSchedule_JSON(t *testing.T) {
	group := model.Group{
		GroupID:        "-1",
		DepartmentID:   "-1",
		GroupName:      "Test",
		DepartmentName: "Test",
		Year:           2026,
		CreatedAt:      time.Now(),
	}
	teacherName := "Иванов Иван Иванович"
	schedule := model.RawSchedule{
		Config: model.GroupScheduleConfig(&group, false),
		Rows: []model.RawScheduleRow{
			{Number: 1, TimeRange: "8:00-9:35", Days: []model.RawScheduleDay{
				{
					Date:     "01.01.2026",
					WeekDay:  "Понедельник",
					WeekKind: "Четная",
					Pair: model.Pair{
						Kind:       model.PairKindEmpty,
						Number:     1,
						StartTime:  "8:00",
						EndTime:    "9:35",
						Label:      "",
						Title:      "",
						Discipline: "Информатика",
						Teacher:    &teacherName,
						Classroom:  "404(404)",
						Replaced:   false,
					},
				},
			}},
		},
	}

	// Try to marshal schedule to JSON
	jsonBytes, err := schedule.JSON()
	if err != nil {
		t.Errorf("Failed to marshal schedule to JSON: %v", err)
	}

	// Try to unmarshal JSON back to schedule
	var unmarshaledSchedule model.RawSchedule
	err = json.Unmarshal(jsonBytes, &unmarshaledSchedule)
	if err != nil {
		t.Errorf("Failed to unmarshal JSON back to schedule: %v", err)
	}

	// Compare original and unmarshaled schedules
	if !reflect.DeepEqual(schedule, unmarshaledSchedule) {
		t.Errorf("Original and unmarshaled schedules are not equal:\n%+v\n%+v", schedule, unmarshaledSchedule)
	}
	if newJSON, err := unmarshaledSchedule.JSON(); err != nil || !reflect.DeepEqual(jsonBytes, newJSON) {
		t.Errorf("Failed to marshal unmarshaled schedule to JSON again: %v", err)
	}

	t.Log("Schedule marshaled and unmarshaled successfully")
}
