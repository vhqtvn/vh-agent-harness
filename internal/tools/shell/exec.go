package shell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// Cause is the closed vocabulary of the ORTHOGONAL outcome
// classification. Exactly one cause describes every completed
// invocation; the fact fields (ExitCode, Signal, TimedOut, SpawnError)
// are never conflated with each other or with ordinary error text
// (dsh defensive-patterns rule: timeout/exit/signal/error are separate
// facts, and a result carries the one that happened).
type Cause string

const (
	// CauseExit: the command ran to completion and exited by itself
	// (any exit code, including non-zero and bash's 127 for
	// command-not-found-inside-the-shell).
	CauseExit Cause = "exit"
	// CauseSignal: the command died from a signal that did NOT come
	// from this tool's own timeout teardown (e.g. `kill -TERM $$`).
	CauseSignal Cause = "signal"
	// CauseTimeout: the effective deadline fired and the process group
	// was killed by this tool.
	CauseTimeout Cause = "timeout"
	// CauseError: the command never ran (spawn failure: missing shell,
	// unusable workdir, sandbox setup failure).
	CauseError Cause = "error"
)

// Outcome is the frozen canonical outcome of one run_shell invocation.
// It serializes deterministically (fixed struct order, no maps) and is
// the content the model sees for exit/signal causes. timeout and error
// causes surface as Pipeline-isError results instead (their typed facts
// travel in the error text; the pipeline-level Result.TimedOut is
// reserved for the def-level cap).
type Outcome struct {
	// Cause is the one-of classification; exactly one of the fact
	// fields below is meaningful for it.
	Cause Cause `json:"cause"`
	// Command is the exact command line that was dispatched.
	Command string `json:"command"`
	// ExitCode is valid for CauseExit. -1 when the process died by
	// signal (Go ProcessState convention), 0 for clean exits.
	ExitCode int `json:"exitCode"`
	// Signal names the terminating signal (e.g. "terminated") for
	// CauseSignal.
	Signal string `json:"signal,omitempty"`
	// TimedOut is the orthogonal timeout fact, set only for
	// CauseTimeout.
	TimedOut bool `json:"timedOut,omitempty"`
	// SpawnError carries the failure text for CauseError (the command
	// never executed).
	SpawnError string `json:"spawnError,omitempty"`
	// Stdout is the captured stdout, capped at MaxCapturedBytes. When
	// Truncated is true a trailing marker line describes the drop.
	Stdout string `json:"stdout"`
	// Stderr is the captured stderr, capped independently (the same
	// budget as stdout; interleaving order is scheduler-dependent, so
	// the streams are captured separately for deterministic
	// presentation).
	Stderr string `json:"stderr"`
	// Truncated is true when either stream hit its cap.
	Truncated bool `json:"truncated,omitempty"`
	// DroppedBytes is the total number of bytes seen past the caps.
	DroppedBytes int64 `json:"droppedBytes,omitempty"`
	// DurationMs is the wall-clock duration of the invocation.
	DurationMs int64 `json:"durationMs"`
	// EffectiveTimeoutMs is the deadline actually enforced (args value
	// clamped to the configured cap).
	EffectiveTimeoutMs int64 `json:"effectiveTimeoutMs"`
	// Sandbox names the confinement that wrapped the command ("none"
	// means NO confinement — see the package doc).
	Sandbox string `json:"sandbox"`

	// sandboxErr carries the typed *SandboxUnavailableError when the
	// confinement backend refused the run (fail-closed). Unexported on
	// purpose: the serialized outcome shape is frozen; the typed error
	// surfaces through execute()'s error, where callers can errors.As
	// it. The text twin travels in SpawnError.
	sandboxErr *SandboxUnavailableError
}

// TimeoutError is the typed error returned for CauseTimeout. The
// Pipeline normalizes it into an isError result whose content carries
// the timeout fact explicitly.
type TimeoutError struct {
	EffectiveTimeoutMs int64
	StdoutBytes        int
	StderrBytes        int
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("run_shell: timed out after %dms (process group killed); captured stdout=%dB stderr=%dB",
		e.EffectiveTimeoutMs, e.StdoutBytes, e.StderrBytes)
}

// truncationMarker is appended (as its own line) to a stream that hit
// its capture cap, so truncation is visible in-band, not only in the
// structured fields.
func truncationMarker(dropped int64, cap int64) string {
	return fmt.Sprintf("\n[run_shell: output truncated, %d bytes dropped (cap %dB)]", dropped, cap)
}

// limitedWriter caps in-memory capture at limit bytes while always
// accepting (and counting) everything the child writes. Accepting the
// overflow keeps exec's pipe-copy goroutine draining, so a child that
// writes gigabytes can never block on us and memory stays bounded —
// this is the whole point of the "64KB in-memory" guarantee.
//
// Invariant: exactly one goroutine (exec's copier) writes to each
// instance; stdout and stderr use separate instances. No locking needed.
type limitedWriter struct {
	buf     bytes.Buffer
	limit   int64
	dropped int64
}

func newLimitedWriter(limit int64) *limitedWriter {
	return &limitedWriter{limit: limit}
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	room := w.limit - int64(w.buf.Len())
	if room > 0 {
		n := int64(len(p))
		if n > room {
			n = room
		}
		w.buf.Write(p[:n])
	}
	if overflow := int64(len(p)) - max64(room, 0); overflow > 0 {
		w.dropped += overflow
	}
	return len(p), nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// run executes one command and returns the classified Outcome. It never
// returns a Go error: the classification lives entirely in the Outcome
// (callers map CauseTimeout/CauseError to Pipeline errors).
//
// Timeout teardown: the child is a process-group leader (Setpgid); when
// the deadline fires the whole group is SIGKILLed via a negative-pid
// signal, so grandchildren (background jobs, pipelines) cannot outlive
// the call. WaitDelay bounds the reap so a wedged stream cannot hang
// Wait past the kill.
func run(ctx context.Context, cfg *Config, command string, timeoutMs int64, workdir string) Outcome {
	start := time.Now()
	out := Outcome{
		Command:            command,
		EffectiveTimeoutMs: timeoutMs,
		Sandbox:            cfg.sandboxLabel(),
	}
	stdout := newLimitedWriter(cfg.MaxCapturedBytes)
	stderr := newLimitedWriter(cfg.MaxCapturedBytes)

	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := newCommand(cctx, cfg, command)
	cmd.Dir = workdir
	cmd.Env = buildEnv(cfg)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	applyProcessGroup(cmd) // Setpgid: child leads its own group
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = 2 * time.Second

	if cfg.Sandbox != nil {
		if err := cfg.Sandbox(cmd); err != nil {
			out.Cause = CauseError
			out.SpawnError = fmt.Sprintf("sandbox %q failed: %v", cfg.sandboxLabel(), err)
			var unavail *SandboxUnavailableError
			if errors.As(err, &unavail) {
				out.sandboxErr = unavail // typed fail-closed: execute() rethrows it
			}
			finishOutcome(&out, stdout, stderr, cfg, start)
			return out
		}
	}

	err := cmd.Run()
	finishOutcome(&out, stdout, stderr, cfg, start)

	switch {
	case err == nil:
		// Completed before the deadline (a deadline that fired during
		// teardown after a clean exit still counts as a clean exit).
		out.Cause = CauseExit
		out.ExitCode = cmd.ProcessState.ExitCode()
	case cctx.Err() == context.DeadlineExceeded:
		out.Cause = CauseTimeout
		out.TimedOut = true
	case isExitError(err):
		if sig, ok := terminatingSignal(cmd); ok {
			out.Cause = CauseSignal
			out.Signal = sig
			out.ExitCode = -1
		} else {
			out.Cause = CauseExit
			out.ExitCode = cmd.ProcessState.ExitCode()
		}
	default:
		// Spawn-class failure: the command never ran (missing shell,
		// chdir failure, ...). Never conflated with an in-shell
		// command-not-found (that is bash exit 127, CauseExit above).
		out.Cause = CauseError
		out.SpawnError = err.Error()
	}
	return out
}

// finishOutcome stamps capture results (with in-band truncation
// markers) and duration onto out.
func finishOutcome(out *Outcome, stdout, stderr *limitedWriter, cfg *Config, start time.Time) {
	out.Stdout = stdout.buf.String()
	out.Stderr = stderr.buf.String()
	if stdout.dropped > 0 {
		out.Stdout += truncationMarker(stdout.dropped, cfg.MaxCapturedBytes)
	}
	if stderr.dropped > 0 {
		out.Stderr += truncationMarker(stderr.dropped, cfg.MaxCapturedBytes)
	}
	out.Truncated = stdout.dropped > 0 || stderr.dropped > 0
	out.DroppedBytes = stdout.dropped + stderr.dropped
	out.DurationMs = time.Since(start).Milliseconds()
}

// newCommand builds the child invocation: a NON-INTERACTIVE shell that
// reads no startup files (--noprofile --norc; the explicit env carries
// no BASH_ENV), keeps no history, and sets up no job control. The
// script runs exactly as given — no injected shell options, for
// predictability.
func newCommand(ctx context.Context, cfg *Config, command string) *exec.Cmd {
	return exec.CommandContext(ctx, cfg.ShellPath, "--noprofile", "--norc", "-c", command)
}

// isExitError reports whether err is an ordinary process exit (as
// opposed to a spawn failure), i.e. *exec.ExitError.
func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
