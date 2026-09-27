package justrayrotate

import (
	"context"
	"os/exec"
)

// Runner drives the local justray binary. It is an interface so tests can fake
// node listings and node switches.
type Runner interface {
	// List returns the current subscriptions with probe results.
	List(ctx context.Context) ([]Sub, error)
	// Up connects in proxy mode to the given node.
	Up(ctx context.Context, id string) error
	// Down disconnects the current connection.
	Down(context.Context) error
}

// CLIRunner executes the justray CLI found on PATH.
type CLIRunner struct{ Bin string }

// List runs `justray subscription list --json`.
func (r CLIRunner) List(ctx context.Context) ([]Sub, error) {
	bin := r.Bin
	if bin == "" {
		bin = "justray"
	}
	out, err := exec.CommandContext(ctx, bin, "subscription", "list", "--json").Output()
	if err != nil {
		return nil, err
	}
	subs, err := ParseSubscriptions(out)
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// Up connects in proxy mode to the given node.
func (r CLIRunner) Up(ctx context.Context, id string) error {
	bin := r.Bin
	if bin == "" {
		bin = "justray"
	}
	return exec.CommandContext(ctx, bin, "up", "--proxy", id).Run()
}

// Down disconnects justray.
func (r CLIRunner) Down(ctx context.Context) error {
	bin := r.Bin
	if bin == "" {
		bin = "justray"
	}
	return exec.CommandContext(ctx, bin, "down").Run()
}
