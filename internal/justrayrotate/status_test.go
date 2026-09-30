package justrayrotate

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// Verbatim output of `justray status --json` (1.6.6) on a connected daemon in
// proxy mode. The full shape is only reported while connected: a disconnected
// daemon reports last_node and drops node/server/port, see
// TestParseStatusDisconnected.
const connectedStatusJSON = `{
  "connected": true,
  "mode": "proxy",
  "node": "Финляндия",
  "server": "finla.ali-bard.ru",
  "port": 443,
  "proxy_port": 10808,
  "protocol": "vless",
  "uptime": 753
}`

func TestParseStatus(t *testing.T) {
	st, err := ParseStatus([]byte(connectedStatusJSON))
	if err != nil {
		t.Fatal(err)
	}
	if !st.Connected {
		t.Error("Connected = false, want true")
	}
	if st.Mode != "proxy" {
		t.Errorf("Mode = %q, want proxy", st.Mode)
	}
	if st.Server != "finla.ali-bard.ru" || st.Port != 443 {
		t.Errorf("endpoint = %s:%d, want finla.ali-bard.ru:443", st.Server, st.Port)
	}
	if st.Uptime != 753 {
		t.Errorf("Uptime = %d, want 753", st.Uptime)
	}
}

// A disconnected daemon reports the last used node by name and no endpoint, so
// there is no node to match against the subscription and CurrentNode reports
// "unknown". That is the wanted outcome: with nothing connected, any eligible
// node is fair game, and the previous one must not be excluded for free.
func TestParseStatusDisconnected(t *testing.T) {
	raw := `{
  "connected": false,
  "last_node": "Финляндия"
}`
	st, err := ParseStatus([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if st.Connected {
		t.Error("Connected = true, want false")
	}
	if st.Server != "" || st.Port != 0 {
		t.Errorf("endpoint = %s:%d, want an empty endpoint", st.Server, st.Port)
	}

	nodes := []Node{
		{ID: "a", Server: "finla.ali-bard.ru", Port: 443, Name: "🇫🇮 Финляндия"},
	}
	if node, ok := CurrentNode(nodes, st); ok {
		t.Errorf("CurrentNode matched %q on a disconnected daemon, want no match", node.ID)
	}
}

func TestParseStatusRejectsGarbage(t *testing.T) {
	if _, err := ParseStatus([]byte("not json")); err == nil {
		t.Fatal("expected an error for non-JSON output")
	}
}

func TestCurrentNode(t *testing.T) {
	nodes := []Node{
		{ID: "a", Server: "ru.example", Port: 443},
		{ID: "b", Server: "se.example", Port: 443},
		{ID: "c", Server: "nl.example", Port: 8443},
	}

	tests := []struct {
		name   string
		status Status
		wantID string
	}{
		{
			name:   "connected to a listed node",
			status: Status{Connected: true, Server: "se.example", Port: 443},
			wantID: "b",
		},
		{
			name:   "disconnected has no current node",
			status: Status{Connected: false, Server: "se.example", Port: 443},
			wantID: "",
		},
		{
			name:   "node is gone from the list",
			status: Status{Connected: true, Server: "gone.example", Port: 443},
			wantID: "",
		},
		{
			name:   "port distinguishes two nodes on one server",
			status: Status{Connected: true, Server: "nl.example", Port: 8443},
			wantID: "c",
		},
		{
			name:   "wrong port on a known server",
			status: Status{Connected: true, Server: "se.example", Port: 8443},
			wantID: "",
		},
		{
			name:   "empty status",
			status: Status{},
			wantID: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, ok := CurrentNode(nodes, tt.status)
			if tt.wantID == "" {
				if ok {
					t.Fatalf("got node %q, want none", node.ID)
				}
				return
			}
			if !ok {
				t.Fatalf("no current node found, want %q", tt.wantID)
			}
			if node.ID != tt.wantID {
				t.Fatalf("current node = %q, want %q", node.ID, tt.wantID)
			}
		})
	}
}

// The active node is what the probe just found unusable, so picking it again
// makes the rotation a no-op. On the first rotation after a restart the picker
// has no history, so only the real status can keep that from happening.
func TestFirstRotationSkipsTheActiveNode(t *testing.T) {
	nodes := []Node{
		{ID: "active", Name: "Sweden", Server: "se.example", Port: 443, Probed: true, Alive: boolp(true)},
		{ID: "other", Name: "Netherlands", Server: "nl.example", Port: 443, Probed: true, Alive: boolp(true)},
	}
	runner := &fakeRunner{
		subs:   []Sub{{ID: "s1", Nodes: nodes}},
		status: Status{Connected: true, Server: "se.example", Port: 443},
	}
	r, err := NewRotator(RotatorConfig{Probe: probeFail}, runner, NewPicker())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.rotate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls := runner.upCalls(); len(calls) != 1 || calls[0] != "other" {
		t.Fatalf("rotated to %v, want [other]", calls)
	}
}

// An unknown current node (daemon down, older justray, node removed) must not
// stop the rotator: it degrades to the picker's own round-robin.
func TestRotationProceedsWhenStatusUnavailable(t *testing.T) {
	nodes := []Node{
		{ID: "a", Name: "Sweden", Server: "se.example", Port: 443, Probed: true, Alive: boolp(true)},
		{ID: "b", Name: "Netherlands", Server: "nl.example", Port: 443, Probed: true, Alive: boolp(true)},
	}
	for _, tc := range []struct {
		name      string
		status    Status
		statusErr error
	}{
		{name: "status errors", statusErr: errors.New("justray status: exit status 1")},
		{name: "disconnected", status: Status{Connected: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{
				subs:      []Sub{{ID: "s1", Nodes: nodes}},
				status:    tc.status,
				statusErr: tc.statusErr,
			}
			r, err := NewRotator(RotatorConfig{Probe: probeFail}, runner, NewPicker())
			if err != nil {
				t.Fatal(err)
			}
			if err := r.rotate(context.Background()); err != nil {
				t.Fatal(err)
			}
			if calls := runner.upCalls(); len(calls) != 1 {
				t.Fatalf("rotated to %v, want exactly one rotation", calls)
			}
		})
	}
}

func TestCLIRunnerStatus(t *testing.T) {
	// Guard against the JSON tags drifting from justray's actual output shape.
	var st Status
	if err := json.Unmarshal([]byte(connectedStatusJSON), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Connected || st.Node != "Финляндия" || st.Server != "finla.ali-bard.ru" || st.Port != 443 {
		t.Fatalf("decoded %+v", st)
	}
}

func boolp(b bool) *bool { return &b }
