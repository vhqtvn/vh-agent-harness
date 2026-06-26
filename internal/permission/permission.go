package permission

import (
	"context"
	"fmt"
	"io"
	"os"
)

// stderr is the writer NoOpHook warns to. It defaults to os.Stderr; tests swap
// it to a buffer to assert the loud warning without leaking to real stderr.
// Swapping this var is test-only and does not change the public Hook API.
var stderr io.Writer = os.Stderr

// Action is the permission-gate verdict for a candidate command.
type Action int

// Possible Action values, in priority order: Deny stops immediately, Ask means
// "prompt the operator" (treated as deny-by-default when no operator loop is
// attached), Allow means "run it".
const (
	// Allow permits the command to proceed to the runtime backend.
	Allow Action = iota
	// Deny blocks the command; the backend is never invoked.
	Deny
	// Ask means the hook could not decide and wants operator confirmation. With
	// no operator loop wired (slice 4a), Ask is treated as deny-by-default by
	// the CLI layer.
	Ask
)

// String returns the canonical lowercase name of the action.
func (a Action) String() string {
	switch a {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	case Ask:
		return "ask"
	default:
		return fmt.Sprintf("unknown(%d)", int(a))
	}
}

// Hook evaluates a candidate command and returns a verdict plus a human-readable
// reason. A non-nil error means the hook itself failed (e.g. the shell-guard
// bridge crashed); callers should treat that as deny-by-default for safety.
//
// Slice 4a defines the interface; slice 4b ships the node-bridge implementation.
type Hook interface {
	Evaluate(ctx context.Context, cmd []string) (Action, string, error)
}

// noopWarning is the loud, on-every-eval reminder that command checking is not
// active. It is printed to stderr so it is never swallowed by stdout capture.
const noopWarning = "WARNING: permission hook is a slice-4a stub — no command checking is active. Do not use in production. Slice 4b will wire the shell-guard bridge."

// noopReason is the Allow reason returned by NoOpHook.
const noopReason = "permission hook not wired (slice 4a stub)"

// NoOpHook is the slice-4a placeholder Hook. It ALWAYS returns Allow so the
// exec path is wired end-to-end, but it prints the loud noopWarning to stderr
// on every evaluation so the safety gap is explicit, never silent.
//
// Slice 4b replaces this with a real shell-guard node-bridge.
type NoOpHook struct{}

// Evaluate logs the noop warning and returns (Allow, noopReason, nil).
func (NoOpHook) Evaluate(context.Context, []string) (Action, string, error) {
	fmt.Fprintln(stderr, noopWarning)
	return Allow, noopReason, nil
}
