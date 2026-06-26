// Package hooks implements the thin lifecycle-hook dispatcher (Slice 5).
//
// It fires project-owned shell leaves at the FIXED lifecycle points declared in
// .vh-agent-harness/run-shape.yml. Three load-bearing guarantees, all enforced
// here and proven by tests:
//
//  1. NO BYPASS: every hook leaf is gate-checked through the SAME permission.Hook
//     (the shell-guard policy pack) that gates `exec`. A gate-denied leaf is
//     NEVER executed (fail-closed). Hooks are not a side door around the gate.
//  2. FIXED POINTS + PATH POINTERS: the dispatcher fires only runshape.LifecycleHook
//     values that survived Load (path pointers under scripts/, never inline shell,
//     never unknown keys).
//  3. ABSENT = NO-OP: a point with no declared hook, or a pointer to a missing
//     leaf, is a clean no-op (missing leaf = typed non-fatal warning; never a crash).
//
// Each dispatch records the lifecycle point + active backend name + gate verdict
// for auditability (DispatchRecord).
package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// Point is a fixed lifecycle point. It aliases runshape.LifecycleHook so the
// dispatcher and the reader share ONE fixed set (no parallel enum to drift).
type Point = runshape.LifecycleHook

// FailurePolicy mirrors the run-shape spec §4: how a gate denial or non-zero hook exit
// affects the wrapping verb.
type FailurePolicy int

const (
	// FailVerb: a denial/non-zero exit fails the wrapping verb. Used by
	// on_first_install, on_update, pre_up, pre_exec.
	FailVerb FailurePolicy = iota
	// FailVerbServicesUp: like FailVerb but the services are already up (post_up).
	// Returned as a distinct policy so a caller can surface the nuance; it fails
	// the verb exactly like FailVerb.
	FailVerbServicesUp
	// WarnAndContinue: a denial/non-zero exit logs a warning and the verb
	// continues. Used by pre_down, post_down, post_exec, on_uninstall.
	WarnAndContinue
)

// PolicyFor returns the the run-shape spec §4 failure policy for a lifecycle point.
func PolicyFor(p Point) FailurePolicy {
	switch p {
	case runshape.HookPostUp:
		return FailVerbServicesUp
	case runshape.HookPreDown, runshape.HookPostDown, runshape.HookPostExec, runshape.HookOnUninstall:
		return WarnAndContinue
	default:
		// on_first_install, on_update, pre_up, pre_exec
		return FailVerb
	}
}

// DispatchStatus is the outcome category of a single dispatch.
type DispatchStatus int

const (
	// StatusNoop: no hook declared at this point, or the leaf is missing on disk.
	StatusNoop DispatchStatus = iota
	// StatusDenied: the gate denied/asked/errored; the leaf was NOT executed.
	StatusDenied
	// StatusRan: the gate allowed and the leaf was executed (exit code in record).
	StatusRan
)

// String renders the status for logs/diagnostics.
func (s DispatchStatus) String() string {
	switch s {
	case StatusNoop:
		return "noop"
	case StatusDenied:
		return "denied"
	case StatusRan:
		return "ran"
	default:
		return fmt.Sprintf("status(%d)", int(s))
	}
}

// DispatchRecord captures what fired at a lifecycle point, for auditability. It
// records the lifecycle point, the active backend name, the resolved leaf path,
// the gate verdict, and (if run) the exit code / error. The CLI returns/logs
// these so a caller can inspect exactly which hooks fired.
type DispatchRecord struct {
	Point       Point
	BackendName string
	Leaf        string // relative leaf path as declared ("" if absent/no-op)
	Status      DispatchStatus
	GateAction  permission.Action
	GateReason  string
	ExitCode    int
	Err         error
}

// Runner executes a gate-allowed hook leaf. The default osRunner runs
// `bash <leaf>` on the host with the HARNESS_* env. Tests inject a recording
// runner so they never spawn a shell — and so they can assert a gate-denied leaf
// is NOT run.
type Runner interface {
	Run(ctx context.Context, leaf string, env []string) (exitCode int, err error)
}

// osRunner runs `bash <leaf>` via os/exec. It is the production Runner.
type osRunner struct{}

func (osRunner) Run(ctx context.Context, leaf string, env []string) (int, error) {
	cmd := exec.CommandContext(ctx, "bash", leaf)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), fmt.Errorf("hook leaf %s exited %d: %s", leaf, exitErr.ExitCode(), strings.TrimSpace(string(out)))
		}
		return -1, fmt.Errorf("hook leaf %s spawn: %w (output: %s)", leaf, err, strings.TrimSpace(string(out)))
	}
	return 0, nil
}

// denyAllHook is the fail-closed fallback when a Dispatcher is constructed with a
// nil gate. The CLI always injects activeHook (non-nil), so this is defense in
// depth: a nil gate can NEVER mean "run ungated".
type denyAllHook struct{}

func (denyAllHook) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Deny, "no permission gate wired for hooks (nil gate); failing closed", nil
}

// Dispatcher fires lifecycle hooks at fixed points. Every hook leaf is
// gate-checked through the SAME permission.Hook that gates `exec`; a denied leaf
// is NEVER executed. Absent/missing leaves are clean no-ops.
type Dispatcher struct {
	gate   permission.Hook // NEVER nil (denyAllHook fallback)
	runner Runner          // runs gate-allowed leaves
}

// New returns a Dispatcher bound to gate (which MUST be the same permission
// policy pack that gates exec) using the default os runner. A nil gate is
// fail-closed via denyAllHook — hooks are never run ungated.
func New(gate permission.Hook) *Dispatcher {
	if gate == nil {
		gate = denyAllHook{}
	}
	return &Dispatcher{gate: gate, runner: osRunner{}}
}

// NewWithRunner is like New but runs allowed leaves via runner. A nil runner
// falls back to the default os runner. This is the test seam: unit tests inject a
// recording runner to assert gate-denied leaves are not executed.
func NewWithRunner(gate permission.Hook, runner Runner) *Dispatcher {
	d := New(gate)
	if runner != nil {
		d.runner = runner
	}
	return d
}

// Fire dispatches the hook for point, if declared in rs. It returns a non-nil
// DispatchRecord and an error only when the failure policy says the wrapping verb
// must fail. Behavior:
//
//   - no pointer declared at point -> StatusNoop, nil error (clean no-op);
//   - pointer declared but leaf missing on disk -> StatusNoop + typed warning
//     (MissingLeafError in record.Err), nil error (DO NOT crash);
//   - gate denies/asks/faults -> StatusDenied, leaf NOT executed; error per policy;
//   - gate allows -> run leaf; non-zero exit -> error per policy.
//
// WarnAndContinue points (post_down/post_exec/on_uninstall/pre_down) always
// return a nil error (the denial/failure is logged as a warning).
func (d *Dispatcher) Fire(ctx context.Context, point Point, rs *runshape.RunShape, backendName, harnessRoot string) (*DispatchRecord, error) {
	rec := &DispatchRecord{Point: point, BackendName: backendName}
	if rs == nil {
		return rec, nil // no run-shape => nothing to fire
	}
	leaf, declared := rs.Lifecycle[point]
	if !declared || leaf == "" {
		return rec, nil // absent = no-op
	}
	rec.Leaf = leaf

	absLeaf := filepath.Join(harnessRoot, runshape.DirName, leaf)
	if _, err := os.Stat(absLeaf); err != nil {
		// Missing leaf: typed NON-FATAL warning. Do NOT crash the verb. We log and
		// return a no-op record carrying the typed error for callers that care.
		rec.Status = StatusNoop
		rec.Err = &MissingLeafError{Point: point, Leaf: leaf}
		fmt.Fprintf(os.Stderr, "warning: lifecycle %s leaf %s missing on disk (%v); continuing\n", point, leaf, err)
		return rec, nil
	}

	// GATE-CHECK: the SAME policy pack that gates exec. The gate sees the resolved
	// leaf command `bash <absLeaf>`. Deny/Ask/fault => fail-closed (leaf NOT run).
	action, reason, gerr := d.gate.Evaluate(ctx, []string{"bash", absLeaf})
	rec.GateAction = action
	rec.GateReason = reason
	if action != permission.Allow {
		rec.Status = StatusDenied
		denied := &GateDeniedError{Point: point, Leaf: leaf, Action: action, Reason: reason, Err: gerr}
		rec.Err = denied
		fmt.Fprintf(os.Stderr, "lifecycle %s leaf %s denied by gate (%s): %s\n", point, leaf, action, reason)
		return rec, d.policyError(point, denied)
	}

	// Gate allowed: run the leaf with the stable HARNESS_* env (the run-shape spec §4).
	env := []string{
		"HARNESS_VERB=" + verbFor(point),
		"HARNESS_ROOT=" + harnessRoot,
		"HARNESS_BACKEND=" + backendName,
	}
	code, runErr := d.runner.Run(ctx, absLeaf, env)
	rec.Status = StatusRan
	rec.ExitCode = code
	rec.Err = runErr
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "lifecycle %s leaf %s failed: %v\n", point, leaf, runErr)
		return rec, d.policyError(point, runErr)
	}
	return rec, nil
}

// policyError maps a hook failure to the verb-level error per the §4 policy. For
// WarnAndContinue points it returns nil (the failure is already logged); for
// FailVerb points it returns the failure so the wrapping verb fails.
func (d *Dispatcher) policyError(point Point, err error) error {
	if err == nil {
		return nil
	}
	if PolicyFor(point) == WarnAndContinue {
		return nil
	}
	return err
}

// verbFor derives the wrapping verb name for the HARNESS_VERB env var from the
// lifecycle point (pre_up/post_up wrap `up`, etc.).
func verbFor(p Point) string {
	switch p {
	case runshape.HookPreUp, runshape.HookPostUp:
		return "up"
	case runshape.HookPreDown, runshape.HookPostDown:
		return "down"
	case runshape.HookPreExec, runshape.HookPostExec:
		return "exec"
	case runshape.HookOnFirstInstall:
		return "install"
	case runshape.HookOnUpdate:
		return "update"
	case runshape.HookOnUninstall:
		return "uninstall"
	}
	return string(p)
}

// --- Typed errors -----------------------------------------------------------

// MissingLeafError indicates a declared hook pointer resolves to a file that does
// not exist on disk. It is NON-FATAL: the dispatcher treats it as a no-op with a
// warning rather than crashing the verb.
type MissingLeafError struct {
	Point Point
	Leaf  string
}

func (e *MissingLeafError) Error() string {
	return fmt.Sprintf("hooks: lifecycle %s leaf %s missing on disk", e.Point, e.Leaf)
}

// GateDeniedError indicates the shell-guard gate denied (or could not decide on,
// or faulted on) a hook leaf, so the leaf was NOT executed. It carries the gate's
// action + reason. This is the fail-closed proof: a denied hook does not run.
type GateDeniedError struct {
	Point  Point
	Leaf   string
	Action permission.Action
	Reason string
	Err    error
}

func (e *GateDeniedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("hooks: lifecycle %s leaf %s gate %s (fault: %v)", e.Point, e.Leaf, e.Action, e.Err)
	}
	return fmt.Sprintf("hooks: lifecycle %s leaf %s gate %s: %s", e.Point, e.Leaf, e.Action, e.Reason)
}
