package model_test

import (
	"reflect"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestRawSchedule_WithConfig(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		config model.ScheduleConfig
		want   model.RawSchedule
	}{
		{
			name:   "empty",
			config: model.ScheduleConfig{},
			want:   model.RawSchedule{Config: model.ScheduleConfig{}},
		},
		{
			name:   "with group",
			config: model.ScheduleConfig{Group: &model.Group{ID: 1}},
			want:   model.RawSchedule{Config: model.ScheduleConfig{Group: &model.Group{ID: 1}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s model.RawSchedule
			got := s.WithConfig(tt.config)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WithConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}
