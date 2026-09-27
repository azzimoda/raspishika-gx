// Package justrayrotate rotates justray nodes through a provider's node list
// when the local justray proxy stops reaching Telegram, keeping bots on the
// local proxy instead of the free-proxy fallback.
package justrayrotate

import "encoding/json"

// Sub is a sing-box subscription as reported by `justray subscription list --json`,
// flattened to the fields the rotator needs.
type Sub struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Nodes []Node `json:"nodes"`
}

// Node is a single provider node. Alive is only present for probed nodes;
// unprobed (or dead) nodes omit it, so it is a pointer.
type Node struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Probed   bool   `json:"probed"`
	Alive    *bool  `json:"alive"`
	MS       *int   `json:"ms"`
}

// IsAlive reports whether the node passed its last probe with a latency.
func (n Node) IsAlive() bool { return n.Alive != nil && *n.Alive }

// HasMS reports whether the node probe produced a latency measurement.
func (n Node) HasMS() bool { return n.MS != nil }

// Latency returns the node latency in milliseconds (0 when unmeasured).
func (n Node) Latency() int {
	if n.MS == nil {
		return 0
	}
	return *n.MS
}

// ParseSubscriptions decodes `justray subscription list --json` output. The
// `network`, `transport`, `tls` and auth fields of each node are dropped — the
// rotator only needs identity, topology and probe results.
func ParseSubscriptions(data []byte) ([]Sub, error) {
	var subs []Sub
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// Flatten returns all nodes across subscriptions in the given order.
func Flatten(subs []Sub) []Node {
	var out []Node
	for _, s := range subs {
		out = append(out, s.Nodes...)
	}
	return out
}
