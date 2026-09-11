package proxyfail

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/azzimoda/go-tg-proxy/proxy"
)

func TestFailTrackerBanAndExpiry(t *testing.T) {
	tracker := NewFailTracker(50 * time.Millisecond)
	addr := "169.58.97.115:1080"

	if tracker.IsBanned(addr) {
		t.Fatalf("fresh tracker must not ban %s", addr)
	}
	if !tracker.Ban(addr) {
		t.Fatalf("first ban must report a new ban")
	}
	if tracker.Ban(addr) {
		t.Fatalf("redundant ban must not report a new one")
	}
	if !tracker.IsBanned(addr) {
		t.Fatalf("%s must be banned", addr)
	}

	time.Sleep(60 * time.Millisecond)
	if tracker.IsBanned(addr) {
		t.Fatalf("ban must expire after cooldown")
	}
}

func TestFailTrackerReset(t *testing.T) {
	tracker := NewFailTracker(time.Minute)
	tracker.SetActive("1.2.3.4:1080")
	tracker.Ban("1.2.3.4:1080")
	if tracker.Active() != "1.2.3.4:1080" {
		t.Fatalf("active proxy not recorded")
	}
	tracker.Reset("1.2.3.4:1080")
	if tracker.IsBanned("1.2.3.4:1080") {
		t.Fatalf("reset must remove the ban")
	}
	if tracker.Active() != "" {
		t.Fatalf("reset must clear the active proxy")
	}
}

type fakeChecker struct {
	latency time.Duration
	err     error
}

func (c fakeChecker) CheckLatency(context.Context, string) (time.Duration, error) {
	return c.latency, c.err
}

func TestBanCheckerBlocksBanned(t *testing.T) {
	tracker := NewFailTracker(time.Minute)
	tracker.Ban("169.58.97.115:1080")

	checker := BanChecker{Checker: fakeChecker{latency: time.Millisecond}, Tracker: tracker}
	_, err := checker.CheckLatency(context.Background(), "169.58.97.115:1080")
	if !errors.Is(err, proxy.ErrProxyUnavailable) {
		t.Fatalf("banned proxy must be reported unavailable, got %v", err)
	}
}

func TestBanCheckerDelegates(t *testing.T) {
	tracker := NewFailTracker(time.Minute)

	checker := BanChecker{Checker: fakeChecker{latency: 5 * time.Millisecond, err: errors.New("inner failure")}, Tracker: tracker}
	_, err := checker.CheckLatency(context.Background(), "10.0.0.1:1080")
	if err == nil || err.Error() != "inner failure" {
		t.Fatalf("live proxy must be delegated to the inner checker, got %v", err)
	}
}

func TestBanCheckerNilTracker(t *testing.T) {
	checker := BanChecker{Checker: fakeChecker{latency: 5 * time.Millisecond}, Tracker: nil}
	if lat, err := checker.CheckLatency(context.Background(), "10.0.0.1:1080"); err != nil || lat != 5*time.Millisecond {
		t.Fatalf("nil tracker must not affect checks, got %v %v", lat, err)
	}
}
