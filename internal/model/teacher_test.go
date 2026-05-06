package model_test

import (
	"testing"

	"github.com/azzimoda/raspishika-gx/internal/model"
)

func TestTeacherName_Safe(t *testing.T) {
	testCases := []struct {
		input string
		want  string
	}{
		{input: "", want: ""},
		{input: "Name", want: "Name"},
		{input: "Surname Name", want: "SurnameName"},
		{input: "Surname Name Fathername", want: "SurnameNameFathername"},
		{input: "Surname N.F.", want: "SurnameNF"},
		{input: "Имя", want: "Имя"},
		{input: "Фамилия Имя", want: "ФамилияИмя"},
		{input: "Фамилия Имя Отчество", want: "ФамилияИмяОтчество"},
		{input: "Фамилия И.О.", want: "ФамилияИО"},
	}
	for _, tt := range testCases {
		name := tt.input
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			if got := model.TeacherName(tt.input).Safe(); got != tt.want {
				t.Errorf("TeacherName(%q).Safe() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
