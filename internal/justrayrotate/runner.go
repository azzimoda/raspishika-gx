package justrayrotate

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DefaultExecTimeout bounds a single justray CLI invocation.
//
// The rotator's context is the process-wide signal context and is also the key of
// the select driving the run loop, so an unbounded child process froze the whole
// daemon: no further probes, no rotations, no clean exit. Restart policies do not
// help either, because a hang is not a failure. A stuck binary is now a failed
// command, which the run loop paces like any other error.
const DefaultExecTimeout = 30 * time.Second

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
type CLIRunner struct {
	// Bin is the justray executable. Empty means "justray" resolved via PATH.
	Bin string
	// Timeout bounds each invocation. Zero means DefaultExecTimeout.
	Timeout time.Duration
}

func (r CLIRunner) bin() string {
	if r.Bin == "" {
		return "justray"
	}
	return r.Bin
}

func (r CLIRunner) timeout() time.Duration {
	if r.Timeout <= 0 {
		return DefaultExecTimeout
	}
	return r.Timeout
}

// withTimeout bounds a command by the exec timeout while still honouring the
// caller's cancellation, so stopping the rotator stops a command in flight.
func (r CLIRunner) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, r.timeout())
}

// List runs `justray subscription list --json`.
func (r CLIRunner) List(ctx context.Context) ([]Sub, error) {
	cmdCtx, cancel := r.withTimeout(ctx)
	defer cancel()
	out, err := exec.CommandContext(cmdCtx, r.bin(), "subscription", "list", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("justray subscription list: %w", err)
	}
	subs, err := ParseSubscriptions(out)
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// Up connects in proxy mode to the given node.
func (r CLIRunner) Up(ctx context.Context, id string) error {
	cmdCtx, cancel := r.withTimeout(ctx)
	defer cancel()
	return exec.CommandContext(cmdCtx, r.bin(), "up", "--proxy", id).Run()
}

// Down disconnects justray.
func (r CLIRunner) Down(ctx context.Context) error {
	cmdCtx, cancel := r.withTimeout(ctx)
	defer cancel()
	return exec.CommandContext(cmdCtx, r.bin(), "down").Run()
}
