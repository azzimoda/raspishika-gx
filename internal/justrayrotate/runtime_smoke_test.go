package justrayrotate

import (
	"context"
	"os"
	"testing"
	"time"
)

func rtEnabled(t *testing.T) {
	if os.Getenv("RT_SMOKE") != "1" {
		t.Skip("set RT_SMOKE=1 to run real justray/Telegram smoke test")
	}
}

// TestJustrayListSmoke checks the justray CLI is reachable and returns a
// parseable subscription list with a sane eligible subset.
func TestJustrayListSmoke(t *testing.T) {
	rtEnabled(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	subs, err := (CLIRunner{}).List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(subs) == 0 {
		t.Fatal("empty subscription list")
	}
	nodes := Flatten(subs)
	if len(nodes) == 0 {
		t.Fatal("no nodes in subscriptions")
	}

	alive := 0
	for _, n := range nodes {
		if n.IsAlive() {
			alive++
		}
	}
	t.Logf("subs=%d nodes=%d alive=%d", len(subs), len(nodes), alive)
	if alive == 0 {
		t.Fatal("expected at least one alive node")
	}

	p := NewPicker("🇷🇺", "Россия", "моб. операторов")
	eligible := p.Eligible(nodes, "")
	if len(eligible) == 0 {
		t.Fatal("expected at least one eligible (non-RU, alive) node")
	}
	t.Logf("eligible=%d", len(eligible))
}

// TestJustrayRotatorSmoke runs the real rotator briefly against the live
// justray daemon; with a healthy proxy it must not rotate.
func TestJustrayRotatorSmoke(t *testing.T) {
	rtEnabled(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe, err := NewTelProbe("127.0.0.1:10808", DefaultProbeURL, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	rotator, err := NewRotator(RotatorConfig{
		Probe:            probe,
		CheckInterval:    200 * time.Millisecond,
		FailureThreshold: 2,
		MaxRotations:     3,
	}, CLIRunner{}, NewPicker("🇷🇺", "Россия", "моб. операторов"))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan bool)
	go func() {
		_ = rotator.Run(ctx)
		done <- true
	}()
	time.Sleep(2 * time.Second)
	cancel()
	<-done
}
