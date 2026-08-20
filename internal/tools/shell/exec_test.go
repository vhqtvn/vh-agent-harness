package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runQuick runs one command via the internal runner with defaults.
func runQuick(t *testing.T, cfg Config, command string, timeoutMs int64) Outcome {
	t.Helper()
	cfg.normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return run(ctx, &cfg, command, timeoutMs, "")
}

// TestOutcomeFactsOrthogonal asserts the four cause classifications are
// DISTINCT: exit (0 and non-zero), in-shell command-not-found (127 =
// exit, NOT spawn error), external signal, timeout, and spawn error —
// each classified as exactly its own cause with its own fact fields.
func TestOutcomeFactsOrthogonal(t *testing.T) {
	t.Run("exit zero", func(t *testing.T) {
		out := runQuick(t, Config{}, "printf hello", 5000)
		if out.Cause != CauseExit || out.ExitCode != 0 {
			t.Fatalf("got cause=%q exitCode=%d, want exit/0", out.Cause, out.ExitCode)
		}
		if out.Stdout != "hello" {
			t.Fatalf("stdout = %q, want %q", out.Stdout, "hello")
		}
		if out.TimedOut || out.Signal != "" || out.SpawnError != "" {
			t.Fatalf("foreign facts set on clean exit: %+v", out)
		}
	})

	t.Run("exit non-zero", func(t *testing.T) {
		out := runQuick(t, Config{}, "exit 42", 5000)
		if out.Cause != CauseExit || out.ExitCode != 42 {
			t.Fatalf("got cause=%q exitCode=%d, want exit/42", out.Cause, out.ExitCode)
		}
		if out.TimedOut || out.Signal != "" || out.SpawnError != "" {
			t.Fatalf("foreign facts set on exit 42: %+v", out)
		}
	})

	t.Run("in-shell command-not-found is exit 127, not spawn error", func(t *testing.T) {
		out := runQuick(t, Config{}, "definitely-missing-binary-xyz-12345", 5000)
		if out.Cause != CauseExit || out.ExitCode != 127 {
			t.Fatalf("got cause=%q exitCode=%d, want exit/127 (bash convention)", out.Cause, out.ExitCode)
		}
		if out.SpawnError != "" {
			t.Fatalf("command-not-found inside the shell must NOT be a spawn error: %q", out.SpawnError)
		}
		if !strings.Contains(out.Stderr, "not found") && !strings.Contains(out.Stderr, "No such file") {
			t.Fatalf("expected not-found diagnostic on stderr, got %q", out.Stderr)
		}
	})

	t.Run("external signal", func(t *testing.T) {
		out := runQuick(t, Config{}, "kill -TERM $$", 5000)
		if out.Cause != CauseSignal {
			t.Fatalf("cause = %q, want signal (got exitCode=%d stderr=%q)", out.Cause, out.ExitCode, out.Stderr)
		}
		if out.Signal != "terminated" {
			t.Fatalf("signal = %q, want terminated", out.Signal)
		}
		if out.ExitCode != -1 {
			t.Fatalf("exitCode = %d, want -1 when signaled", out.ExitCode)
		}
		if out.TimedOut || out.SpawnError != "" {
			t.Fatalf("foreign facts set on signal death: %+v", out)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		start := time.Now()
		out := runQuick(t, Config{}, "sleep 5", 300)
		elapsed := time.Since(start)
		if out.Cause != CauseTimeout || !out.TimedOut {
			t.Fatalf("got cause=%q timedOut=%v, want timeout/true", out.Cause, out.TimedOut)
		}
		if elapsed > 3*time.Second {
			t.Fatalf("timeout took %v; the 5s sleep must be killed at ~300ms", elapsed)
		}
		if out.ExitCode != 0 || out.Signal != "" || out.SpawnError != "" {
			t.Fatalf("foreign facts set on timeout: %+v", out)
		}
	})

	t.Run("spawn error missing shell binary", func(t *testing.T) {
		cfg := Config{ShellPath: "/nonexistent/shell-binary-xyz"}
		out := runQuick(t, cfg, "true", 5000)
		if out.Cause != CauseError {
			t.Fatalf("cause = %q, want error (spawn failure)", out.Cause)
		}
		if !strings.Contains(out.SpawnError, "no such file") && !strings.Contains(out.SpawnError, "not exist") {
			t.Fatalf("spawnError = %q, want a not-found diagnostic", out.SpawnError)
		}
		if out.TimedOut || out.Signal != "" || out.ExitCode != 0 {
			t.Fatalf("foreign facts set on spawn error: %+v", out)
		}
	})
}

// TestTimeoutKillsProcessGroup is the group-kill proof: a background
// child (sleep 30) spawned by the timed-out command must be dead after
// run returns — Setpgid + negative-signal teardown, not just the direct
// child.
func TestTimeoutKillsProcessGroup(t *testing.T) {
	out := runQuick(t, Config{}, "sleep 30 & echo bgpid=$!; sleep 30", 400)
	if out.Cause != CauseTimeout || !out.TimedOut {
		t.Fatalf("cause=%q timedOut=%v, want timeout", out.Cause, out.TimedOut)
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(out.Stdout), "bgpid=%d", &pid); err != nil {
		t.Fatalf("cannot parse background pid from stdout %q: %v", out.Stdout, err)
	}
	if pid <= 1 {
		t.Fatalf("refusing pid %d as a background-child proof", pid)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return // reaped: the group kill reached the background child
		}
		if err != nil {
			t.Fatalf("liveness probe on pid %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("background sleep (pid %d) survived the timeout teardown; group kill failed", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestOutcomeJSONShape pins the serialized field order/shape (the
// model-facing content contract; fixed struct order, no maps).
func TestOutcomeJSONShape(t *testing.T) {
	out := runQuick(t, Config{}, "printf hi", 5000)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(mustJSON(t, out)), &raw); err != nil {
		t.Fatalf("outcome is not valid JSON: %v", err)
	}
	for _, k := range []string{"cause", "command", "exitCode", "stdout", "stderr", "durationMs", "effectiveTimeoutMs", "sandbox"} {
		if _, ok := raw[k]; !ok {
			t.Fatalf("outcome JSON missing field %q: %s", k, mustJSON(t, out))
		}
	}
	if string(raw["cause"]) != `"exit"` || string(raw["sandbox"]) != `"none"` {
		t.Fatalf("cause/sandbox wrong: %s", mustJSON(t, out))
	}
	// omitempty discipline: timeout/signal/spawn facts absent on a clean exit.
	for _, k := range []string{"signal", "timedOut", "spawnError", "truncated", "droppedBytes"} {
		if _, ok := raw[k]; ok {
			t.Fatalf("field %q must be omitted on a clean exit: %s", k, mustJSON(t, out))
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestLimitedWriterBoundedMemory pins the in-memory cap invariant: the
// writer always accepts writes (pipe keeps draining) but stores only
// limit bytes and counts the drop exactly. Per the documented
// invariant (one copier goroutine per stream) writes come from ONE
// goroutine; the acceptance-always property is what keeps the exec pipe
// draining under overflow.
func TestLimitedWriterBoundedMemory(t *testing.T) {
	w := newLimitedWriter(8)
	chunk := []byte("0123456789") // 10 bytes
	for j := 0; j < 300; j++ {
		if n, err := w.Write(chunk); n != len(chunk) || err != nil {
			t.Fatalf("Write = %d, %v; want full acceptance", n, err)
		}
	}
	if w.buf.Len() != 8 {
		t.Fatalf("stored %d bytes, want the 8-byte cap", w.buf.Len())
	}
	if w.dropped != 300*10-8 {
		t.Fatalf("dropped = %d, want %d", w.dropped, 300*10-8)
	}
}

// TestStderrCapturedSeparately documents the separate-streams decision
// (interleaving order is scheduler-dependent, so streams are captured
// independently for deterministic presentation).
func TestStderrCapturedSeparately(t *testing.T) {
	out := runQuick(t, Config{}, "printf out; printf err 1>&2", 5000)
	if out.Stdout != "out" || out.Stderr != "err" {
		t.Fatalf("stdout=%q stderr=%q, want out/err", out.Stdout, out.Stderr)
	}
}
