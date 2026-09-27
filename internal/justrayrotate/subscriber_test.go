package justrayrotate

import (
	"reflect"
	"testing"
)

const fixture = `[
  {
    "id": "92339444",
    "name": "Alibard VPN",
    "nodes": [
      {"id": "ru1", "name": "\ud83c\uddf7\ud83c\uddfa \u0420\u043e\u0441\u0441\u0438\u044f (\u041c\u0421\u041a)", "protocol": "vless", "server": "youta.ggisopi.su", "port": 443, "probed": true, "alive": true, "ms": 288},
      {"id": "se1", "name": "Sweden", "protocol": "vless", "server": "govpoel.ggisopi.su", "port": 443, "probed": true, "alive": true, "ms": 455},
      {"id": "nl1", "name": "Netherlands", "protocol": "vless", "server": "ivan.ggisopi.su", "port": 443, "probed": true, "alive": true, "ms": 479},
      {"id": "it1", "name": "Italy", "protocol": "vless", "server": "kask2.ggisopi.su", "port": 443, "probed": true, "alive": false},
      {"id": "mobile", "name": "For mobile operators", "protocol": "vless", "server": "rkn-sosatb.com", "port": 443, "probed": true, "alive": true, "ms": 512}
    ]
  }
]`

// ruFlag is the 🇷🇺 regional-indicator pair.
const ruFlag = "\U0001F1F7\U0001F1FA"

func TestParseSubscriptions(t *testing.T) {
	subs, err := ParseSubscriptions([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 sub, got %d", len(subs))
	}
	if got := subs[0].ID; got != "92339444" {
		t.Fatalf("sub id = %q", got)
	}
	nodes := subs[0].Nodes
	if len(nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(nodes))
	}

	nl := nodes[2]
	if !nl.IsAlive() || nl.Latency() != 479 || !nl.HasMS() {
		t.Fatalf("nl expected alive/479: %+v", nl)
	}
	it := nodes[3]
	if it.IsAlive() {
		t.Fatalf("it expected dead: %+v", it)
	}
}

func TestFlatten(t *testing.T) {
	subs, err := ParseSubscriptions([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	nodes := Flatten(subs)
	if len(nodes) != 5 {
		t.Fatalf("flatten = %d nodes", len(nodes))
	}
}

func TestUnexportedNoAlive(t *testing.T) {
	// Nodes whose probe result is missing (unprobed) must not report alive.
	data := `[{"id":"s","name":"x","nodes":[{"id":"n1","name":"N","probed":true}]}]`
	subs, err := ParseSubscriptions([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if subs[0].Nodes[0].IsAlive() {
		t.Fatal("unprobed node must not be alive")
	}
}

func TestPickerEligible(t *testing.T) {
	subs, err := ParseSubscriptions([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	nodes := Flatten(subs)

	p := NewPicker(ruFlag, "Россия", "mobile operators")
	eligible := p.Eligible(nodes, "")

	got := make([]string, 0, len(eligible))
	for _, n := range eligible {
		got = append(got, n.ID)
	}
	want := []string{"se1", "nl1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("eligible = %v, want %v", got, want)
	}
}

func TestPickerNextRoundRobin(t *testing.T) {
	subs, err := ParseSubscriptions([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	nodes := Flatten(subs)

	p := NewPicker(ruFlag, "Россия", "mobile operators")

	first, ok := p.Next(nodes, "")
	if !ok || first.ID != "se1" {
		t.Fatalf("first = %+v ok=%v, want se1", first, ok)
	}
	second, ok := p.Next(nodes, "")
	if !ok || second.ID != "nl1" {
		t.Fatalf("second = %+v ok=%v, want nl1", second, ok)
	}
	// Round-robin wraps back to the start.
	third, ok := p.Next(nodes, "")
	if !ok || third.ID != "se1" {
		t.Fatalf("third = %+v ok=%v, want se1", third, ok)
	}
}

func TestPickerNextSkipsCurrent(t *testing.T) {
	subs, err := ParseSubscriptions([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	nodes := Flatten(subs)

	p := NewPicker(ruFlag, "Россия", "mobile operators")
	next, ok := p.Next(nodes, "nl1")
	if !ok || next.ID != "se1" {
		t.Fatalf("next = %+v ok=%v, want se1 (skip current nl1)", next, ok)
	}
}

func TestPickerNoEligible(t *testing.T) {
	subs, err := ParseSubscriptions([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	// Exclude everything non-eligible.
	p := NewPicker(ruFlag, "Россия", "Sweden", "Netherlands", "mobile operators")
	nodes := Flatten(subs)
	if _, ok := p.Next(nodes, ""); ok {
		t.Fatal("expected no eligible node")
	}
}
