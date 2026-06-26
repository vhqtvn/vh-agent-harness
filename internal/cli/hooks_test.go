package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// recordingHookRunner records the leaves fireHook asks it to run. It is the CLI-
// level no-bypass proof surface: if a hook gate denies, recordingHookRunner.leaves
// must stay empty. It never spawns bash.
type recordingHookRunner struct {
	leaves []string
	err    error // if set, Run returns exit 1 + err
}

func (r *recordingHookRunner) Run(_ context.Context, leaf string, _ []string) (int, error) {
	r.leaves = append(r.leaves, filepath.Base(leaf))
	if r.err != nil {
		return 1, r.err
	}
	return 0, nil
}

// writeRunShapeCLI writes a run-shape.yml under root/.vh-agent-harness/ with the
// given lifecycle hooks and creates each leaf file on disk (so os.Stat sees it).
func writeRunShapeCLI(t *testing.T, root string, hooks map[string]string) {
	t.Helper()
	dir := filepath.Join(root, ".vh-agent-harness")
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	var b strings.Builder
	b.WriteString("run_shape_version: \"0.1\"\nlifecycle:\n")
	for k, v := range hooks {
		b.WriteString("  " + k + ": " + v + "\n")
		if v != "" {
			leafPath := filepath.Join(dir, v)
			if err := os.MkdirAll(filepath.Dir(leafPath), 0o755); err != nil {
				t.Fatalf("mkdir leaf: %v", err)
			}
			if err := os.WriteFile(leafPath, []byte("#!/usr/bin/env bash\necho "+k+"\n"), 0o755); err != nil {
				t.Fatalf("write leaf %s: %v", v, err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "run-shape.yml"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

// --- VERB WIRING (criterion: hooks wrap the verb body) ----------------------

// TestUp_FiresPreAndPostHooks — up fires pre_up, then backend.Up, then post_up.
func TestUp_FiresPreAndPostHooks(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	writeRunShapeCLI(t, root, map[string]string{
		"pre_up":  "scripts/pre-up.sh",
		"post_up": "scripts/post-up.sh",
	})

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{}
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runUp(cmd, nil); err != nil {
			t.Fatalf("runUp: %v", err)
		}
		if len(rec.log) != 1 || rec.log[0] != "Up" {
			t.Errorf("backend should see exactly Up; got %v", rec.log)
		}
		wantLeaves := []string{"pre-up.sh", "post-up.sh"}
		if len(rr.leaves) != 2 {
			t.Fatalf("expected 2 hook firings; got %v", rr.leaves)
		}
		for i, w := range wantLeaves {
			if rr.leaves[i] != w {
				t.Errorf("hook %d = %q, want %q (order: pre_up before post_up)", i, rr.leaves[i], w)
			}
		}
	})
}

// TestDown_FiresPreAndPostHooks — down fires pre_down, backend.Down, post_down.
func TestDown_FiresPreAndPostHooks(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	writeRunShapeCLI(t, root, map[string]string{
		"pre_down":  "scripts/pre-down.sh",
		"post_down": "scripts/post-down.sh",
	})

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{}
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runDown(cmd, nil); err != nil {
			t.Fatalf("runDown: %v", err)
		}
		if len(rec.log) != 1 || rec.log[0] != "Down" {
			t.Errorf("backend should see exactly Down; got %v", rec.log)
		}
		if len(rr.leaves) != 2 || rr.leaves[0] != "pre-down.sh" || rr.leaves[1] != "post-down.sh" {
			t.Errorf("expected pre-down.sh then post-down.sh; got %v", rr.leaves)
		}
	})
}

// TestExec_FiresPreAndPostExec — exec fires pre_exec, gate, backend.Exec, post_exec.
func TestExec_FiresPreAndPostExec(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	writeRunShapeCLI(t, root, map[string]string{
		"pre_exec":  "scripts/pre-exec.sh",
		"post_exec": "scripts/post-exec.sh",
	})

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{} // same seam gates exec AND hooks
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runExec(cmd, []string{"echo", "hi"}); err != nil {
			t.Fatalf("runExec: %v", err)
		}
		if len(rec.log) != 1 || !strings.HasPrefix(rec.log[0], "Exec:echo hi") {
			t.Errorf("backend should see Exec:echo hi; got %v", rec.log)
		}
		if len(rr.leaves) != 2 || rr.leaves[0] != "pre-exec.sh" || rr.leaves[1] != "post-exec.sh" {
			t.Errorf("expected pre-exec.sh then post-exec.sh; got %v", rr.leaves)
		}
	})
}

// --- NO-BYPASS AT THE VERB LEVEL --------------------------------------------

// TestUp_PreUpDeniedFailsVerb — when the (same) gate denies, pre_up is blocked,
// runUp errors, and backend.Up is NEVER reached. Hooks are not a side door.
func TestUp_PreUpDeniedFailsVerb(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	writeRunShapeCLI(t, root, map[string]string{"pre_up": "scripts/pre-up.sh"})

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{} // denies BOTH the hook leaf and any exec
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runUp(cmd, nil)
		if err == nil {
			t.Fatalf("pre_up denial must fail the verb")
		}
		if !strings.Contains(err.Error(), "pre_up hook") {
			t.Errorf("error should blame pre_up hook; got %q", err.Error())
		}
		if len(rec.log) != 0 {
			t.Errorf("GATE BYPASS: backend.Up reached despite pre_up denial: %v", rec.log)
		}
		if len(rr.leaves) != 0 {
			t.Errorf("GATE BYPASS: hook leaf ran despite denial: %v", rr.leaves)
		}
	})
}

// TestExec_PreExecDeniedFailsVerb — pre_exec denial blocks the verb; the user-
// command gate is never reached AND backend.Exec never runs.
func TestExec_PreExecDeniedFailsVerb(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	writeRunShapeCLI(t, root, map[string]string{"pre_exec": "scripts/pre-exec.sh"})

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{}
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runExec(cmd, []string{"echo", "hi"})
		if err == nil {
			t.Fatalf("pre_exec denial must fail the verb")
		}
		if !strings.Contains(err.Error(), "pre_exec hook") {
			t.Errorf("error should blame pre_exec hook; got %q", err.Error())
		}
		if len(rec.log) != 0 {
			t.Errorf("backend.Exec reached despite pre_exec denial: %v", rec.log)
		}
		if len(rr.leaves) != 0 {
			t.Errorf("hook leaf ran despite denial: %v", rr.leaves)
		}
	})
}

// --- PATH-POINTER-ONLY AT THE VERB LEVEL (criterion 1) ----------------------

// TestUp_InlineShellYAMLRejected — a run-shape carrying inline shell in pre_up is
// rejected at load; runUp errors and the backend is never touched.
func TestUp_InlineShellYAMLRejected(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	dir := filepath.Join(root, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "run_shape_version: \"0.1\"\nlifecycle:\n  pre_up: \"echo bad; rm -rf /\"\n"
	if err := os.WriteFile(filepath.Join(dir, "run-shape.yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{}
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runUp(cmd, nil)
		if err == nil {
			t.Fatalf("inline-shell run-shape must be rejected")
		}
		if !strings.Contains(err.Error(), "inline shell") {
			t.Errorf("error should mention inline shell; got %q", err.Error())
		}
		if len(rec.log) != 0 {
			t.Errorf("backend touched despite bad run-shape: %v", rec.log)
		}
	})
}

// --- SLICE 1–4 PRESERVATION (absent run-shape = no-op) ----------------------

// TestUp_NoRunShapeIsNoop — a repo with NO run-shape file fires no hooks and
// behaves byte-identically to pre-Slice-5 (the existing deny/allow exec tests
// rely on this: their repos have manifests only, no run-shape).
func TestUp_NoRunShapeIsNoop(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{}
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runUp(cmd, nil); err != nil {
			t.Fatalf("up with no run-shape must succeed; got %v", err)
		}
		if len(rec.log) != 1 || rec.log[0] != "Up" {
			t.Errorf("backend should see Up once; got %v", rec.log)
		}
		if len(rr.leaves) != 0 {
			t.Errorf("no run-shape => no hook firings; got %v", rr.leaves)
		}
	})
}

// TestExec_NoRunShapePreservesDenyPath — with no run-shape, pre/post_exec are
// no-ops and the existing exec deny-path semantics are intact (the user-command
// gate denies and reaches the same error text the slice-4 tests assert).
func TestExec_NoRunShapePreservesDenyPath(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{}
	rr := &recordingHookRunner{}
	hookDeps.runner = rr
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runExec(cmd, []string{"echo", "hi"})
		if err == nil || !strings.Contains(err.Error(), "denied by permission hook") {
			t.Errorf("no-run-shape exec deny path should match slice-4 text; got %v", err)
		}
		if len(rec.log) != 0 || len(rr.leaves) != 0 {
			t.Errorf("nothing should run on deny; backend=%v hooks=%v", rec.log, rr.leaves)
		}
	})
}
