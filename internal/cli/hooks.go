package cli

import (
	"context"
	"fmt"

	"github.com/vhqtvn/vh-agent-harness/internal/hooks"
	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// hookDeps is the test seam for the hook dispatcher. Tests inject a recording
// Runner so they can assert which leaves fired (and that a gate-denied leaf did
// NOT fire) without spawning bash. Production leaves runner nil so the dispatcher
// uses its default os runner.
//
// The GATE is NOT seam-injected here: fireHook reuses activeHook(harnessRoot) —
// the SAME permission policy pack that gates `exec` (controlled by the existing
// runtimeCmdDeps.hook seam). This is the literal "hooks run through the same
// shell-guard policy pack as exec" guarantee: one seam, one gate, for both.
var hookDeps = struct {
	runner hooks.Runner // nil => dispatcher uses its default os runner
}{
	runner: nil,
}

// resetHookDeps restores the hook-dispatcher seam after a test. Called from
// resetRuntimeDeps so every runtime/hook test resets both seams together.
func resetHookDeps() {
	hookDeps.runner = nil
}

// fireHook loads the run-shape for harnessRoot, builds a dispatcher bound to the
// SAME gate that gates exec (activeHook), and fires point. It returns the
// dispatch record and the policy-mapped verb error. Absent run-shape / absent
// point / missing leaf are clean no-ops (nil error). A malformed run-shape is a
// fail-verb signal (it is not silently ignored).
//
// harnessRoot is the project root (loadedManifest.dir) containing both
// .opencode/ (manifest) and .vh-agent-harness/ (run-shape).
func fireHook(ctx context.Context, point hooks.Point, backendName, harnessRoot string) (*hooks.DispatchRecord, error) {
	rs, err := runshape.LoadForRoot(harnessRoot)
	if err != nil {
		// A malformed/invalid run-shape is fail-verb: never silently ignore a bad
		// pointer or an unknown lifecycle key.
		return nil, fmt.Errorf("load run-shape: %w", err)
	}
	gate := activeHook(harnessRoot) // SAME policy pack that gates exec
	d := hooks.NewWithRunner(gate, hookDeps.runner)
	return d.Fire(ctx, point, rs, backendName, harnessRoot)
}

// Compile-time anchor: keep the permission import meaningful and ensure the gate
// type the dispatcher consumes is the same Hook interface exec uses.
var _ permission.Hook = (permission.Hook)(nil)
