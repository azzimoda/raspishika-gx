package justrayrotate

import "strings"

// Picker selects the next node to rotate to. Round-robin among live,
// non-excluded nodes, always skipping the node just tried so a broken node is
// not retried immediately.
type Picker struct {
	// last is the most recently selected node id, used for round-robin
	// progression and to avoid selecting it again right away.
	last string
	// exclude lists node-name substrings (e.g. "🇷🇺", "Россия") that must never
	// be selected.
	exclude []string
}

// NewPicker builds a picker excluding nodes whose name contains any of the
// given substrings.
func NewPicker(exclude ...string) *Picker {
	return &Picker{exclude: exclude}
}

// Eligible returns live nodes that are not excluded and not the current one,
// in the source list order.
func (p *Picker) Eligible(nodes []Node, current string) []Node {
	var out []Node
	for _, n := range nodes {
		if n.ID == "" || n.ID == current || !n.IsAlive() || p.excluded(n.Name) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Next returns the round-robin successor of the last picked node among the
// eligible ones. It reports false when there is nothing to rotate to.
func (p *Picker) Next(nodes []Node, current string) (Node, bool) {
	eligible := p.Eligible(nodes, current)
	if len(eligible) == 0 {
		return Node{}, false
	}

	start := 0
	if p.last != "" {
		for i, n := range eligible {
			if n.ID == p.last {
				start = (i + 1) % len(eligible)
				break
			}
		}
	}

	next := eligible[start]
	p.last = next.ID
	return next, true
}

func (p *Picker) excluded(name string) bool {
	for _, e := range p.exclude {
		if e != "" && strings.Contains(name, e) {
			return true
		}
	}
	return false
}
