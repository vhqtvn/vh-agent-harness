package cli

import (
	"context"
	"reflect"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// recordingPermHook records the argv it was asked to evaluate and always allows,
// so a test can assert WHAT runExec hands the shell-guard gate.
type recordingPermHook struct{ got []string }

func (h *recordingPermHook) Evaluate(_ context.Context, cmd []string) (permission.Action, string, error) {
	h.got = append([]string(nil), cmd...)
	return permission.Allow, "", nil
}

// TestExec_GateSeesWrappedCommand is the regression guard for the shell-guard
// double-gate bug: `vh-agent-harness exec <mutating>` (mkdir/pytest/npm/bash -c)
// was denied because runExec re-evaluated the BARE payload against the gate's
// raw-command read-only allowlist. The fix evaluates the command AS INVOKED —
// wrapped in `vh-agent-harness exec` — so the gate's harness branch trusts the
// exec boundary (payload still scanned by forbidden-patterns + commit-gate).
// This pins that the gate receives the wrapped argv, not the bare payload.
func TestExec_GateSeesWrappedCommand(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	h := &recordingPermHook{}
	runtimeCmdDeps.hook = h
	defer resetRuntimeDeps(t)

	execFl.service, execFl.workdir, execFl.tty = "", "", false
	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runExec(cmd, []string{"mkdir", "-p", "docs/x"}); err != nil {
			t.Fatalf("runExec(mkdir) must be allowed through the wrapped gate, got: %v", err)
		}
	})

	want := []string{"vh-agent-harness", "exec", "mkdir", "-p", "docs/x"}
	if !reflect.DeepEqual(h.got, want) {
		t.Errorf("gate saw %v, want wrapped %v (bare payload would hit the read-only allowlist)", h.got, want)
	}
	// And the backend actually received the un-wrapped payload to run.
	if len(rec.log) != 1 || rec.log[0] != "Exec:mkdir -p docs/x" {
		t.Errorf("backend should exec the bare payload; got %v", rec.log)
	}
}
