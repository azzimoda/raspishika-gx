package justrayrotate

import (
	"encoding/json"
	"fmt"
)

// Status is the connection state reported by `justray status --json`.
//
// It carries no node id, so the active node is identified by its server and port,
// which are unique across the node list.
type Status struct {
	Connected bool   `json:"connected"`
	Mode      string `json:"mode"`
	Node      string `json:"node"`
	Server    string `json:"server"`
	Port      int    `json:"port"`
	Protocol  string `json:"protocol"`
	Uptime    int64  `json:"uptime"`
}

// ParseStatus decodes `justray status --json` output.
func ParseStatus(data []byte) (Status, error) {
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return Status{}, fmt.Errorf("parse justray status: %w", err)
	}
	return s, nil
}

// CurrentNode returns the node in nodes that justray is connected to, and whether
// one was found. A disconnected daemon has no current node, and neither does a
// node list that no longer contains the active one.
func CurrentNode(nodes []Node, st Status) (Node, bool) {
	if !st.Connected || st.Server == "" || st.Port == 0 {
		return Node{}, false
	}
	for _, n := range nodes {
		if n.Server == st.Server && n.Port == st.Port {
			return n, true
		}
	}
	return Node{}, false
}
