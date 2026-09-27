package vkclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/SevereCloud/vksdk/v3/api"
	"github.com/SevereCloud/vksdk/v3/longpoll-bot"
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

// The SDK posts photo uploads through vk.Client with no context. Without a
// timeout here a stalled upload blocks the synchronous Long Poll event loop
// forever, so the deadline is load-bearing rather than cosmetic.
func TestNewBoundsHTTPClient(t *testing.T) {
	c, err := New("tok", 42, "")
	if err != nil {
		t.Fatal(err)
	}
	if c.vk.Client == nil {
		t.Fatal("vk.Client must be set, otherwise SDK falls back to the untimed http.DefaultClient")
	}
	if c.vk.Client.Timeout <= 0 {
		t.Fatalf("vk.Client.Timeout = %s, want a positive deadline", c.vk.Client.Timeout)
	}
	if c.vk.Client.Timeout != vkHTTPTimeout {
		t.Fatalf("vk.Client.Timeout = %s, want %s", c.vk.Client.Timeout, vkHTTPTimeout)
	}
	// A nil Transport keeps the shared default transport, so connection pooling
	// across calls is preserved.
	if c.vk.Client.Transport != nil {
		t.Fatal("vk.Client.Transport should stay nil to reuse http.DefaultTransport")
	}
}

// The cases here use *api.Error because that is what vksdk puts in the error
// chain (api/api.go returns &response.Error). A value api.Error never reaches
// these functions in production.
func TestIsForbidden(t *testing.T) {
	for _, code := range []int{901, 902, 917} {
		if !IsForbidden(&api.Error{Code: api.ErrorType(code)}) {
			t.Errorf("code %d should be treated as forbidden", code)
		}
	}
	for _, input := range []error{
		nil,
		errors.New("boom"),
		&api.Error{Code: 9},
		&api.Error{Code: api.ErrorType(1000)},
	} {
		if IsForbidden(input) {
			t.Errorf("input %#v must not be forbidden", input)
		}
	}
	// Wrapped and formatted forms must still be recognised, since the VK
	// broadcast path inspects errors coming back through fmt.Errorf chains.
	wrapped := fmt.Errorf("send VK message: %w", &api.Error{Code: 917})
	if !IsForbidden(wrapped) {
		t.Fatal("wrapped *api.Error not detected")
	}
}

func TestIsFatalInitError(t *testing.T) {
	for _, code := range []int{5, 15, 27, 100} {
		if !isFatalInitError(&api.Error{Code: api.ErrorType(code)}) {
			t.Errorf("code %d should be fatal", code)
		}
	}
	for _, input := range []error{
		nil,
		errors.New("boom"),
		&api.Error{Code: 901},
		&api.Error{Code: api.ErrorType(6)},
	} {
		if isFatalInitError(input) {
			t.Errorf("input %#v must not be fatal", input)
		}
	}
	if !isFatalInitError(fmt.Errorf("init long poll: %w", &api.Error{Code: 5})) {
		t.Fatal("wrapped fatal *api.Error not detected")
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

// Reconnecting from "now" silently dropped every event that arrived in the gap,
// so a reconnected session must resume from the previous cursor.
func TestResumeFromCarriesCursor(t *testing.T) {
	lp := &longpoll.LongPoll{Ts: "fresh-from-server"}
	resumeFrom(lp, "42")
	if lp.Ts != "42" {
		t.Fatalf("ts = %q, want the previous cursor %q", lp.Ts, "42")
	}

	// No previous session yet: keep whatever the server handed us.
	first := &longpoll.LongPoll{Ts: "fresh-from-server"}
	resumeFrom(first, "")
	if first.Ts != "fresh-from-server" {
		t.Fatalf("ts = %q, want the server cursor untouched", first.Ts)
	}
}

// TestLongPollBackoffGrowsWhileSessionsDieInstantly is the regression: the delay
// used to be reset right after the session initialised, so sessions that failed
// immediately always waited one second, hammering VK once per second for the
// whole outage.
func TestLongPollBackoffGrowsWhileSessionsDieInstantly(t *testing.T) {
	b := newLongPollBackoff()
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	for i, w := range want {
		got := b.sessionFailed(0)
		if got != w {
			t.Fatalf("attempt %d: wait = %v, want %v", i+1, got, w)
		}
	}
	// Repeated reconnects must not stay at the cap forever either.
	for range 10 {
		b.sessionFailed(0)
	}
	if b.wait > maxLongPollBackoff {
		t.Fatalf("wait = %v, want at most %v", b.wait, maxLongPollBackoff)
	}
	if got := b.sessionFailed(0); got != maxLongPollBackoff {
		t.Fatalf("wait at the cap = %v, want %v", got, maxLongPollBackoff)
	}
}

// TestLongPollBackoffResetsAfterAWorkingSession keeps a healthy session from
// inheriting the delay an outage built up.
func TestLongPollBackoffResetsAfterAWorkingSession(t *testing.T) {
	b := newLongPollBackoff()
	for range 5 {
		b.sessionFailed(0)
	}
	if b.wait == time.Second {
		t.Fatal("backoff never grew; the test premise is broken")
	}
	// A session that outlived the wait it replaces is proof the connection works.
	if got := b.sessionFailed(45 * time.Second); got != time.Second {
		t.Fatalf("wait after a long healthy session = %v, want %v", got, time.Second)
	}
}

// TestLongPollBackoffKeepsGrowingForFlashSessions is the boundary: a session
// shorter than the current wait proves nothing.
func TestLongPollBackoffKeepsGrowingForFlashSessions(t *testing.T) {
	b := newLongPollBackoff()
	first := b.sessionFailed(0)
	if first != time.Second {
		t.Fatalf("first wait = %v, want %v", first, time.Second)
	}
	// 900ms is just under the 1s wait, so this session is not proof of life.
	if got := b.sessionFailed(900 * time.Millisecond); got != 2*time.Second {
		t.Fatalf("wait after a 900ms session = %v, want 2s", got)
	}
}
