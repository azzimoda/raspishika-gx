package justrayrotate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// hangingJustray writes a stub binary that never returns, standing in for a
// justray that wedges, e.g. waiting on a locked subscription store.
func hangingJustray(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub relies on a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "justray")
	// exec keeps the shell out of the way so the timeout kills the process that
	// is actually sleeping instead of orphaning it.
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// A stuck justray used to hang the rotator's run loop forever, because the same
// context is the signal context and the key of the select driving the loop: no
// further probes, no rotations, and no clean exit. A hang is not a failure, so
// restart policies do not rescue it either.
func TestCLIRunnerBoundsHungBinary(t *testing.T) {
	runner := CLIRunner{Bin: hangingJustray(t), Timeout: 200 * time.Millisecond}

	calls := map[string]func(context.Context) error{
		"List": func(ctx context.Context) error { _, err := runner.List(ctx); return err },
		"Up":   func(ctx context.Context) error { return runner.Up(ctx, "node") },
		"Down": runner.Down,
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			if err := call(context.Background()); err == nil {
				t.Fatalf("%s reported success from a binary that never returns", name)
			}
			if elapsed := time.Since(start); elapsed > 10*time.Second {
				t.Fatalf("%s took %s, the exec timeout is not bounding it", name, elapsed)
			}
		})
	}
}

func TestCLIRunnerTimeoutDefault(t *testing.T) {
	if got := (CLIRunner{}).timeout(); got != DefaultExecTimeout {
		t.Fatalf("default timeout = %s, want %s", got, DefaultExecTimeout)
	}
	if got := (CLIRunner{Timeout: time.Second}).timeout(); got != time.Second {
		t.Fatalf("configured timeout = %s, want 1s", got)
	}
}
