package model_test

import (
	"reflect"
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestScheduleConfig_WithDarkMode(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for receiver constructor.
		cisDark bool
		// Named input parameters for target function.
		isDark bool
		want   model.ScheduleConfig
	}{
		{
			name:    "light mode",
			cisDark: true,
			isDark:  false,
			want:    model.ScheduleConfig{Group: nil, IsDark: false},
		},
		{
			name:    "dark mode",
			cisDark: false,
			isDark:  true,
			want:    model.ScheduleConfig{Group: nil, IsDark: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := model.GroupScheduleConfig(nil, tt.cisDark)
			got := cs.WithDarkMode(tt.isDark)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("WithDarkMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
