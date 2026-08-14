package model

import (
	"errors"
	"testing"
)

func TestGroupNameValidateFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   GroupName
		want    GroupName
		wantErr error
	}{
		{
			name:  "already normalized",
			input: "ИСПт-22-(9)-2",
			want:  "ИСПт-22-(9)-2",
		},
		{
			name:  "base without parentheses",
			input: "ГРПт-23-11-1",
			want:  "ГРПт-23-(11)-1",
		},
		{
			name:  "separated by spaces",
			input: "ГРПт 24 (9) 1",
			want:  "ГРПт-24-(9)-1",
		},
		{
			name:  "no separators",
			input: "ГРПт23(9)1",
			want:  "ГРПт-23-(9)-1",
		},
		{
			name:  "base eleven",
			input: "ИСПт-25-(11)-1",
			want:  "ИСПт-25-(11)-1",
		},
		{
			name:  "min prefix length",
			input: "АБВ-26-(9)-3",
			want:  "АБВ-26-(9)-3",
		},
		{
			name:  "max prefix length",
			input: "АБВГД-27-(11)-4",
			want:  "АБВГД-27-(11)-4",
		},
		{
			name:  "latin prefix",
			input: "ABC-22-(9)-1",
			want:  "ABC-22-(9)-1",
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "prefix too short",
			input:   "АБ-22-(9)-1",
			want:    "АБ-22-(9)-1",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "prefix too long",
			input:   "АБВГДЕ-22-(9)-1",
			want:    "АБВГДЕ-22-(9)-1",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "base not 9 or 11",
			input:   "ИСПт-22-(10)-1",
			want:    "ИСПт-22-(10)-1",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "group number not a single digit",
			input:   "ИСПт-22-(9)-12",
			want:    "ИСПт-22-(9)-12",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "year longer than two digits",
			input:   "ИСПт-222-(9)-1",
			want:    "ИСПт-222-(9)-1",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "trailing whitespace",
			input:   "ИСПт-22-(9)-1 ",
			want:    "ИСПт-22-(9)-1 ",
			wantErr: ErrInvalidGroupNameFormat,
		},
		{
			name:    "non-digit group number",
			input:   "ИСПт-22-(9)-x",
			want:    "ИСПт-22-(9)-x",
			wantErr: ErrInvalidGroupNameFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.ValidateFormat()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateFormat() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("ValidateFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGroupNameParse(t *testing.T) {
	tests := []struct {
		name      string
		input     GroupName
		wantName  string
		wantYear  int
		wantBase  int
		wantN     int
		wantError error
	}{
		{
			name:     "normalized name",
			input:    "ИСПт-22-(9)-1",
			wantName: "ИСПт",
			wantYear: 22,
			wantBase: 9,
			wantN:    1,
		},
		{
			name:     "base eleven",
			input:    "ГРПт-23-11-2",
			wantName: "ГРПт",
			wantYear: 23,
			wantBase: 11,
			wantN:    2,
		},
		{
			name:      "invalid format",
			input:     "не-группа",
			wantError: ErrInvalidGroupNameFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, year, base, n, err := tt.input.Parse()
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Parse() error = %v, want %v", err, tt.wantError)
			}
			if name != tt.wantName || year != tt.wantYear || base != tt.wantBase || n != tt.wantN {
				t.Fatalf("Parse() = (%q, %d, %d, %d), want (%q, %d, %d, %d)",
					name, year, base, n, tt.wantName, tt.wantYear, tt.wantBase, tt.wantN)
			}
		})
	}
}
