package adminbot

import (
	"testing"
	"time"
)

func TestParsePeriodSpecRelative(t *testing.T) {
	spec, ok := parsePeriodSpec("7d")
	if !ok {
		t.Fatalf("parsePeriodSpec(7d) ok = false")
	}
	if !spec.isRelative {
		t.Fatalf("expected relative")
	}
	wantDur := 7 * 24 * time.Hour
	if got := spec.end.Sub(spec.start); got != wantDur {
		t.Fatalf("window = %v, want %v", got, wantDur)
	}

	if _, ok := parsePeriodSpec("48h"); !ok {
		t.Fatalf("parsePeriodSpec(48h) ok = false")
	}
	if _, ok := parsePeriodSpec("2w"); !ok {
		t.Fatalf("parsePeriodSpec(2w) ok = false")
	}
	if _, ok := parsePeriodSpec("1m"); !ok {
		t.Fatalf("parsePeriodSpec(1m) ok = false")
	}
}

func TestParsePeriodSpecRange(t *testing.T) {
	spec, ok := parsePeriodSpec("2026-08-20..2026-08-27")
	if !ok {
		t.Fatalf("parsePeriodSpec(range) ok = false")
	}
	if spec.isRelative {
		t.Fatalf("range must not be relative")
	}
	wantStart := time.Date(2026, 8, 20, 0, 0, 0, 0, statsTZ)
	wantEnd := time.Date(2026, 8, 27, 23, 59, 59, 999000000, statsTZ)
	if !spec.start.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", spec.start, wantStart)
	}
	if !spec.end.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", spec.end, wantEnd)
	}
	if spec.label == "" {
		t.Fatalf("expected a non-empty range label")
	}
}

func TestParsePeriodSpecSinceOffset(t *testing.T) {
	spec, ok := parsePeriodSpec("48h@2026-08-20 12:00")
	if !ok {
		t.Fatalf("parsePeriodSpec(since) ok = false")
	}
	if spec.isRelative {
		t.Fatalf("since-offset must not be relative")
	}
	wantStart := time.Date(2026, 8, 20, 12, 0, 0, 0, statsTZ)
	wantEnd := time.Date(2026, 8, 22, 12, 0, 0, 0, statsTZ)
	if !spec.start.Equal(wantStart) {
		t.Fatalf("start = %v, want %v", spec.start, wantStart)
	}
	if !spec.end.Equal(wantEnd) {
		t.Fatalf("end = %v, want %v", spec.end, wantEnd)
	}
	// Both bounds must be normalized to UTC for DB comparison.
	zStart, _ := spec.start.Zone()
	zEnd, _ := spec.end.Zone()
	if zStart != "UTC" || zEnd != "UTC" {
		t.Fatalf("bounds must be UTC for DB query, got zones %q, %q", zStart, zEnd)
	}
}

func TestParsePeriodSpecInvalid(t *testing.T) {
	for _, s := range []string{"", "abc", "..", "2026-08-20..", "..2026-08-27", "abc@12:00", "0h"} {
		if _, ok := parsePeriodSpec(s); ok {
			t.Fatalf("parsePeriodSpec(%q) ok = true, want false", s)
		}
	}

	// Reversed range must be rejected.
	if _, ok := parsePeriodSpec("2026-08-27..2026-08-20"); ok {
		t.Fatalf("reversed range accepted")
	}
}
