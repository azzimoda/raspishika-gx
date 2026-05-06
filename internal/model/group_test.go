package model_test

import (
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestGroupName_ValidateFormat(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		groupName model.GroupName
		want      model.GroupName
		wantErr   bool
	}{
		{"valid group name must be validated", "ИСПт-22-(9)-2", "ИСПт-22-(9)-2", false},
		{"name without parens be validated", "ИСПт-22-9-2", "ИСПт-22-(9)-2", false},
		{"name without dashes be validated", "ИСПт 22 (11) 2", "ИСПт-22-(11)-2", false},
		{"name without dashes and parens be validated", "ИСПт 22 11 2", "ИСПт-22-(11)-2", false},
		{"name without spaces and parens be validated", "ИСПт2292", "ИСПт-22-(9)-2", false},

		{"invalid case must valudate format but not case", "испт-22-(11)-2", "испт-22-(11)-2", false},
		{"invalid case without spaces and parens must valudate format but not case", "испт2292", "испт-22-(9)-2", false},

		{"must validate CЭЗт-25-(9)-1 (C is latin capital letter)", "CЭЗт-25-(9)-1", "CЭЗт-25-(9)-1", false},

		{"empty string must cause error", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := tt.groupName.ValidateFormat()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("ValidateGroupName() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("ValidateGroupName() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("ValidateGroupName() = %v, want %v", got, tt.want)
			}
		})
	}
}
