package cli

import (
	"context"
	"reflect"
	"strings"
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

// TestExec_F1WrappedGitMutationDenied is the F1 regression matrix for the A1 Go
// backstop. The bypass: `vh-agent-harness exec git <global-flag> <mutation>`
// (e.g. `exec git --no-pager commit`) reached evaluateGate's harness branch
// where the JS adjacency regex (`git-mutation-bypass`) cannot match a flag
// between `git` and the verb, so the mutation slipped past the commit-gate.
// denyExecGitMutationPayload now denies such payloads at the Go binary BEFORE
// the JS bridge even runs. This test pins that:
//
//   - DENY cases return an error naming the "git mutation guard" and NEVER
//     reach the backend (rec.log empty) NOR the gate (h.got empty — the A1
//     guard returns before evaluateGate is called).
//   - ALLOW control cases (read-only git + globals, and non-git mutations like
//     mkdir/pytest/`bash -c '...'`) MUST still reach the backend, proving A1
//     is git-mutation-scoped only and does NOT over-deny the legitimate exec
//     surface. Nested-shell git (`bash -c 'git …'`) is explicitly OUT OF SCOPE
//     for A1 and stays governed by the JS forbidden-pattern scan.
func TestExec_F1WrappedGitMutationDenied(t *testing.T) {
	type tc struct {
		name     string
		args     []string
		wantDeny bool
		verbSub  string // substring the denial reason must contain (deny only)
	}
	cases := []tc{
		// --- F1 DENY: git mutation routed past a global flag (the bypass). ---
		{name: "git --no-pager commit", args: []string{"git", "--no-pager", "commit"}, wantDeny: true, verbSub: "commit"},
		{name: "git -C /x push", args: []string{"git", "-C", "/x", "push"}, wantDeny: true, verbSub: "push"},
		{name: "git --git-dir=/x commit", args: []string{"git", "--git-dir=/x", "commit"}, wantDeny: true, verbSub: "commit"},
		{name: "git commit (adjacency regression)", args: []string{"git", "commit"}, wantDeny: true, verbSub: "commit"},
		{name: "git --no-pager push", args: []string{"git", "--no-pager", "push"}, wantDeny: true, verbSub: "push"},
		{name: "git -c x commit (config-flag consume-and-continue)", args: []string{"git", "-c", "x=y", "commit"}, wantDeny: true, verbSub: "commit"},
		{name: "git --work-tree=/x reset", args: []string{"git", "--work-tree=/x", "reset"}, wantDeny: true, verbSub: "reset"},

		// --- ALLOW: read-only git + global flags (A1 must NOT over-deny). ---
		{name: "git --no-pager status (readonly + global flag)", args: []string{"git", "--no-pager", "status"}, wantDeny: false},
		{name: "git -C /repo status (readonly + path-bearing flag)", args: []string{"git", "-C", "/repo", "status"}, wantDeny: false},
		{name: "git --no-pager log (readonly)", args: []string{"git", "--no-pager", "log"}, wantDeny: false},

		// --- ALLOW: non-git mutations (the legitimate exec surface). ---
		{name: "mkdir tmp/x (non-git mutation)", args: []string{"mkdir", "tmp/x"}, wantDeny: false},
		{name: "pytest (non-git mutation)", args: []string{"pytest"}, wantDeny: false},
		{name: "npm test (non-git mutation)", args: []string{"npm", "test"}, wantDeny: false},
		{name: "bash -c echo hi (nested shell OUT OF SCOPE)", args: []string{"bash", "-c", "echo hi"}, wantDeny: false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
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
				err := runExec(cmd, c.args)
				if c.wantDeny {
					if err == nil {
						t.Fatalf("runExec(%v): expected denial error, got nil", c.args)
					}
					if !strings.Contains(err.Error(), "git mutation guard") {
						t.Errorf("runExec(%v) err = %q; must name the git mutation guard", c.args, err.Error())
					}
					if !strings.Contains(err.Error(), c.verbSub) {
						t.Errorf("runExec(%v) err = %q; must name the routed verb %q", c.args, err.Error(), c.verbSub)
					}
					if len(rec.log) != 0 {
						t.Errorf("A1 deny must block BEFORE the backend; got backend log %v", rec.log)
					}
					if len(h.got) != 0 {
						t.Errorf("A1 deny must block BEFORE the JS gate; gate saw %v", h.got)
					}
				} else {
					if err != nil {
						t.Fatalf("runExec(%v): A1 must NOT over-deny; got %v", c.args, err)
					}
					if len(rec.log) != 1 {
						t.Errorf("runExec(%v): backend should have been reached; got log %v", c.args, rec.log)
					}
				}
			})
		})
	}
}
