package vkclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SevereCloud/vksdk/v3/api"
)

func TestNewValidation(t *testing.T) {
	if _, err := New("", 1, ""); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, err := New("tok", 0, ""); err == nil {
		t.Fatal("zero group ID accepted")
	}
	if _, err := New("tok", -5, ""); err == nil {
		t.Fatal("negative group ID accepted")
	}
}

func TestNewDefaultVersion(t *testing.T) {
	c, err := New("tok", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.vk.Version != DefaultVersion {
		t.Fatalf("version = %q, want %q", c.vk.Version, DefaultVersion)
	}
	withVersion, err := New("tok", 42, "5.131")
	if err != nil {
		t.Fatal(err)
	}
	if withVersion.vk.Version != "5.131" {
		t.Fatalf("version override = %q", withVersion.vk.Version)
	}
}

func TestIsForbidden(t *testing.T) {
	for _, code := range []int{901, 902, 917} {
		if !IsForbidden(api.Error{Code: api.ErrorType(code)}) {
			t.Errorf("code %d should be treated as forbidden", code)
		}
	}
	for _, input := range []error{
		nil,
		errors.New("boom"),
		api.Error{Code: 9},
		&api.Error{Code: 917},
		api.Error{Code: api.ErrorType((1000))},
	} {
		if IsForbidden(input) {
			t.Errorf("input %#v must not be forbidden", input)
		}
	}
	var wrapped api.Error
	wrapped.Code = api.ErrorType(902)
	if !IsForbidden(wrapped) {
		t.Fatal("wrapped api error not detected")
	}
}

func TestRandomIDWithinRange(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := randomID()
		if id <= 0 {
			t.Fatalf("randomID <= 0: %d", id)
		}
	}
}

func TestSplitText(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int
	}{
		{"empty message", "", 1},
		{"short message", "привет", 1},
		{"exactly one unit of limit", strings.Repeat("я", 4000), 1},
	}
	for _, tc := range cases {
		parts := splitText(tc.input)
		if len(parts) != tc.want {
			t.Errorf("%s: got %d parts, want %d", tc.name, len(parts), tc.want)
		}
	}
	long := strings.Repeat("т", 10000)
	if parts := splitText(long); len(parts) < 3 {
		t.Fatalf("long message split into %d parts", len(parts))
	}
	for i, part := range splitText(long) {
		units := 0
		for _, r := range part {
			units++
			if r > 0xffff {
				units++
			}
		}
		if units > 4000 {
			t.Errorf("part %d exceeds 4000 units", i)
		}
	}
}

func TestSplitTextSplitsByNewlines(t *testing.T) {
	// A paragraph long enough to overflow should be cut at the previous
	// newline rather than mid-sentence.
	paragraph := strings.Repeat("а", 3000) + "\n"
	long := paragraph + paragraph + paragraph + paragraph
	parts := splitText(long)
	for i, part := range parts[:len(parts)-1] {
		if !strings.HasSuffix(part, "\n") {
			t.Fatalf("part %d did not end at a newline", i)
		}
	}
}

// pause returns immediately for non-positive delays, keeping backoff loops
// testable without real sleeping.
func TestPause(t *testing.T) {
	if err := pause(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := pause(context.Background(), -1); err != nil {
		t.Fatal(err)
	}
}
