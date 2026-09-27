package justrayrotate

import "testing"

func aliveNode(id, name string) Node {
	alive := true
	return Node{ID: id, Name: name, Alive: &alive}
}

func deadNode(id, name string) Node {
	alive := false
	return Node{ID: id, Name: name, Alive: &alive}
}

// TestPickerEmptyList covers the degenerate subscription: nothing to rotate to
// must report false rather than a zero node, which the rotator would then try
// to use.
func TestPickerEmptyList(t *testing.T) {
	p := NewPicker()
	if node, ok := p.Next(nil, "current"); ok || node.ID != "" {
		t.Fatalf("Next(nil, current) = (%+v, %v), want (zero, false)", node, ok)
	}
	if node, ok := p.Next([]Node{}, ""); ok || node.ID != "" {
		t.Fatalf("Next([], \"\") = (%+v, %v), want (zero, false)", node, ok)
	}
	if got := p.Eligible(nil, ""); len(got) != 0 {
		t.Fatalf("Eligible(nil, \"\") = %+v, want none", got)
	}
}

// TestPickerSingleNode covers a one-node subscription in both directions: a
// node that is not the current one is selectable, and a node that is the current
// one is skipped, which leaves nothing to rotate to.
func TestPickerSingleNode(t *testing.T) {
	t.Run("different node is selectable", func(t *testing.T) {
		p := NewPicker()
		node, ok := p.Next([]Node{aliveNode("a", "🇸🇪 Швеция")}, "b")
		if !ok || node.ID != "a" {
			t.Fatalf("Next = (%+v, %v), want node a", node, ok)
		}
		// Repeated calls keep returning the only candidate, and the picker
		// must not get stuck refusing to rotate.
		for range 3 {
			if again, ok := p.Next([]Node{aliveNode("a", "🇸🇪 Швеция")}, "b"); !ok || again.ID != "a" {
				t.Fatalf("repeat Next = (%+v, %v), want node a", again, ok)
			}
		}
	})

	t.Run("current node is skipped", func(t *testing.T) {
		p := NewPicker()
		if node, ok := p.Next([]Node{aliveNode("a", "🇸🇪 Швеция")}, "a"); ok {
			t.Fatalf("Next = (%+v, true), want false when the only node is the current one", node)
		}
	})
}

// TestPickerSkipsUnusableNodes covers the eligibility rules: dead nodes, the
// current node, nodes without an id and excluded names never come back.
func TestPickerSkipsUnusableNodes(t *testing.T) {
	nodes := []Node{
		aliveNode("", "no id"),
		deadNode("dead", "🇫🇮 Финляндия"),
		aliveNode("ru", "🇷🇺 Россия"),
		aliveNode("cur", "🇸🇪 Швеция"),
		aliveNode("keep", "🇩🇪 Германия"),
	}
	p := NewPicker("🇷🇺", "Россия")
	got := p.Eligible(nodes, "cur")
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("Eligible = %+v, want only 'keep'", got)
	}
	node, ok := p.Next(nodes, "cur")
	if !ok || node.ID != "keep" {
		t.Fatalf("Next = (%+v, %v), want node keep", node, ok)
	}
}

// TestPickerRoundRobin covers the rotation order over several nodes.
func TestPickerRoundRobin(t *testing.T) {
	nodes := []Node{
		aliveNode("a", "🇸🇪 Швеция"),
		aliveNode("b", "🇩🇪 Германия"),
		aliveNode("c", "🇫🇮 Финляндия"),
	}
	p := NewPicker()
	var order []string
	for range len(nodes) {
		node, ok := p.Next(nodes, "current")
		if !ok {
			t.Fatalf("Next reported no node on iteration %d", len(order))
		}
		order = append(order, node.ID)
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Fatalf("order = %v, want [a b c]", order)
	}
	// The next round continues from where it stopped.
	if node, _ := p.Next(nodes, "current"); node.ID != "a" {
		t.Fatalf("after a full cycle Next = %q, want a", node.ID)
	}
}

// TestPickerForgetsDeadSelection keeps round-robin from stalling when the node
// it last picked has since died: it is no longer eligible, and the picker has to
// fall back to the head of the list rather than report nothing.
func TestPickerForgetsDeadSelection(t *testing.T) {
	p := NewPicker()
	nodes := []Node{aliveNode("a", "🇸🇪 Швеция"), aliveNode("b", "🇩🇪 Германия")}
	if node, ok := p.Next(nodes, "current"); !ok || node.ID != "a" {
		t.Fatalf("first Next = (%+v, %v), want node a", node, ok)
	}
	shrunk := []Node{deadNode("a", "🇸🇪 Швеция"), aliveNode("b", "🇩🇪 Германия")}
	node, ok := p.Next(shrunk, "current")
	if !ok || node.ID != "b" {
		t.Fatalf("Next after the last pick died = (%+v, %v), want node b", node, ok)
	}
}
