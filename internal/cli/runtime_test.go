package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runtime"
)

// recordingBackend is a Backend double that records every verb and can return a
// canned error. Used to prove exec/shell NEVER reach the backend when the
// permission hook denies.
type recordingBackend struct {
	name string
	log  []string
	err  error
}

func (r *recordingBackend) Name() string { return r.name }

// Capabilities satisfies the slice-2 Backend interface. The recording double
// does not model a capability surface, so it returns an empty matrix (the real
// backends' matrices are exercised in internal/runtime/capability_test.go).
func (r *recordingBackend) Capabilities() runtime.CapabilityMatrix {
	return runtime.CapabilityMatrix{Backend: r.name}
}
func (r *recordingBackend) Up(context.Context) error {
	r.log = append(r.log, "Up")
	return r.err
}
func (r *recordingBackend) Down(context.Context) error {
	r.log = append(r.log, "Down")
	return r.err
}
func (r *recordingBackend) Exec(_ context.Context, cmd []string, _ runtime.ExecOpts) error {
	r.log = append(r.log, "Exec:"+strings.Join(cmd, " "))
	return r.err
}
func (r *recordingBackend) Logs(_ context.Context, svc string, follow bool) error {
	r.log = append(r.log, fmt.Sprintf("Logs:%s:%v", svc, follow))
	return r.err
}
func (r *recordingBackend) Ps(context.Context) ([]runtime.ServiceStatus, error) {
	r.log = append(r.log, "Ps")
	return nil, r.err
}
func (r *recordingBackend) Healthcheck(context.Context) error {
	r.log = append(r.log, "Healthcheck")
	return r.err
}

// denyHook is a Hook double that always denies, for testing the deny path.
type denyHook struct{}

func (denyHook) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Deny, "test-deny: command blocked", nil
}

// allowHook is a Hook double that always allows. Used where a test needs to
// prove the allow-path reaches the backend WITHOUT depending on the wired
// ShellGuardHook default (which would spawn node and deny against a temp root
// that has no eval.js). This is the explicit test seam that replaces the old
// "hook=nil => NoOp(Allow)" path that slice 4b removed.
type allowHook struct{}

func (allowHook) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Allow, "test-allow", nil
}

// resetRuntimeDeps restores the package-level seams after a test mutates them.
func resetRuntimeDeps(t *testing.T) {
	t.Helper()
	runtimeCmdDeps.backendFor = defaultBackendFor
	runtimeCmdDeps.hook = nil
	resetHookCache()
	resetHookDeps() // also clear the hook-dispatcher runner seam
}

// writeFixtureManifest writes a minimal manifest under root with the given
// runtime backend (+ optional default_service) so loadManifest finds it.
func writeFixtureManifest(t *testing.T, root, backend, defaultService string) {
	t.Helper()
	m := &manifest.Manifest{
		SchemaVersion:     manifest.SchemaVersion,
		Project:           manifest.Project{Name: "demo", Slug: "demo"},
		EnabledComponents: []string{},
		Files:             map[string]manifest.File{},
	}
	m.Runtime.Backend = backend
	m.Runtime.DefaultService = defaultService
	if err := m.Write(root); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// TestExec_HookRunsBeforeBackend is the slice-4a safety proof: when the
// permission hook denies, the backend is NEVER invoked and the command returns a
// non-nil error carrying the denial reason. This proves the gate runs BEFORE the
// backend even though the wired default is NoOp (Allow).
func TestExec_HookRunsBeforeBackend(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{}
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runExec(cmd, []string{"echo", "hello"})
		if err == nil {
			t.Fatalf("exec with deny hook: expected error, got nil")
		}
		if !strings.Contains(err.Error(), "denied by permission hook") {
			t.Errorf("error should mention denial; got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "test-deny") {
			t.Errorf("error should carry the hook reason; got %q", err.Error())
		}
		if len(rec.log) != 0 {
			t.Errorf("backend was invoked despite deny hook: %v", rec.log)
		}
	})
}

// TestExec_AllowReachesBackend verifies the wired-default path (NoOp Allow)
// actually reaches the backend.
func TestExec_AllowReachesBackend(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = allowHook{} // explicit allow seam; avoids the wired ShellGuardHook (would deny against this temp root with no eval.js)
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runExec(cmd, []string{"echo", "hi"}); err != nil {
			t.Fatalf("exec allow: %v", err)
		}
		if len(rec.log) != 1 || !strings.HasPrefix(rec.log[0], "Exec:echo hi") {
			t.Errorf("backend exec not recorded: %v", rec.log)
		}
	})
}

// TestShell_HookDenies mirrors the deny-path test for the shell verb.
func TestShell_HookDenies(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")

	rec := &recordingBackend{name: "docker_compose"}
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return rec, nil }
	runtimeCmdDeps.hook = denyHook{}
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runShell(cmd, nil)
		if err == nil {
			t.Fatalf("shell with deny hook: expected error, got nil")
		}
		if len(rec.log) != 0 {
			t.Errorf("backend was invoked despite deny hook: %v", rec.log)
		}
	})
}

// TestBackendSelection_UnknownBackend verifies an unknown manifest backend
// errors clearly (no silent defaulting, no fallback).
func TestBackendSelection_UnknownBackend(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "kubernetes", "")
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runUp(cmd, nil)
		if err == nil {
			t.Fatalf("unknown backend: expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unknown runtime.backend") || !strings.Contains(err.Error(), "kubernetes") {
			t.Errorf("error should name the unknown backend; got %q", err.Error())
		}
	})
}

// TestBackendSelection_EmptyBackend verifies a missing backend field errors with
// install guidance rather than guessing docker_compose silently.
func TestBackendSelection_EmptyBackend(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "", "")
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runUp(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "runtime.backend") {
			t.Errorf("empty backend should error with guidance; got %v", err)
		}
	})
}

// TestFailWithGuidance_NoSilentFallback verifies the live docker_compose path
// fails loudly when the daemon is unreachable and does NOT degrade to bare.
// The backend's exported Reachable field is overridden to fail; since the probe
// runs before any docker call, no real daemon is contacted.
func TestFailWithGuidance_NoSilentFallback(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "docker_compose", "dev")
	if err := os.WriteFile(filepath.Join(root, "docker-compose.yml"),
		[]byte("services:\n  dev:\n    image: busybox\n    command: sleep infinity\n"), 0o644); err != nil {
		t.Fatalf("write compose file: %v", err)
	}

	dc := runtime.NewDockerCompose(runtime.DockerComposeConfig{
		ComposeFile: "docker-compose.yml", ProjectName: "demo", DefaultService: "dev", Dir: root,
	})
	dc.Reachable = func(context.Context) error { return fmt.Errorf("connection refused (test stub)") }
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return dc, nil }
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runUp(cmd, nil)
		if err == nil {
			t.Fatalf("up with unreachable daemon: expected guidance error, got nil")
		}
		for _, want := range []string{"docker_compose", "No fallback configured", "connection refused"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("guidance error missing %q; got %q", want, err.Error())
			}
		}
	})
}

// TestBare_SelectionSucceeds verifies selecting runtime.backend=bare yields a
// bare backend whose Up verb succeeds. The "no isolation" warning text itself
// is asserted in internal/runtime/bare_test.go (where the stderr seam lives).
func TestBare_SelectionSucceeds(t *testing.T) {
	root := t.TempDir()
	writeFixtureManifest(t, root, "bare", "")

	b := runtime.NewBare(runtime.BareConfig{Dir: root})
	runtimeCmdDeps.backendFor = func(*loadedManifest) (runtime.Backend, error) { return b, nil }
	runtimeCmdDeps.hook = nil
	defer resetRuntimeDeps(t)

	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		if err := runUp(cmd, nil); err != nil {
			t.Fatalf("bare up: %v", err)
		}
	})
}

// TestResolveBackend_NoManifest verifies the runtime verbs error clearly when no
// manifest is installed.
func TestResolveBackend_NoManifest(t *testing.T) {
	root := t.TempDir()
	defer resetRuntimeDeps(t)
	runWithCwd(t, root, func() {
		cmd, _ := newOutCmd()
		err := runUp(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "install") {
			t.Errorf("no-manifest error should mention install; got %v", err)
		}
	})
}
