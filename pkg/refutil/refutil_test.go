package refutil_test

import (
	"testing"

	"github.com/azzimoda/raspishika-gx/pkg/refutil"
)

func TestDerefOrTypeDefault_string(t *testing.T) {
	tests := []struct {
		name string
		ptr  *string
		want string
	}{
		{name: "nil pointer returns zero value", ptr: nil, want: ""},
		{name: "non-nil pointer returns dereferenced value", ptr: new("hello"), want: "hello"},
		{name: "empty string pointer returns empty string", ptr: new(""), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refutil.DerefOrTypeDefault(tt.ptr); got != tt.want {
				t.Errorf("DerefOrTypeDefault(%v) = %v, want %v", tt.ptr, got, tt.want)
			}
		})
	}
}

func TestDerefOrTypeDefault_int(t *testing.T) {
	tests := []struct {
		name string
		ptr  *int
		want int
	}{
		{name: "nil pointer returns zero value", ptr: nil, want: 0},
		{name: "non-nil pointer returns dereferenced value", ptr: new(42), want: 42},
		{name: "empty string pointer returns empty string", ptr: new(-67), want: -67},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := refutil.DerefOrTypeDefault(tt.ptr); got != tt.want {
				t.Errorf("DerefOrTypeDefault(%v) = %v, want %v", tt.ptr, got, tt.want)
			}
		})
	}
}
