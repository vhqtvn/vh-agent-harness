package hooks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permission"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// --- Test doubles ------------------------------------------------------------

type allowGate struct{}

func (allowGate) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Allow, "test-allow", nil
}

type denyGate struct{}

func (denyGate) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Deny, "test-deny: hook leaf blocked", nil
}

// askGate returns Ask; the CLI treats Ask as deny-by-default (no operator loop).
type askGate struct{}

func (askGate) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Ask, "test-ask", nil
}

// faultGate simulates a shell-guard bridge fault (non-nil error). ShellGuardHook
// is fail-safe and returns (Deny, "", err) on a fault; this gate models that.
type faultGate struct{}

func (faultGate) Evaluate(context.Context, []string) (permission.Action, string, error) {
	return permission.Deny, "", errors.New("node bridge crashed")
}

// recordingRunner records every leaf it is asked to run. It is the proof surface
// for no-bypass: if a gate denies, recordingRunner.runs MUST stay empty.
type recordingRunner struct {
	runs []string
	envs [][]string
	err  error // if set, Run returns exit 1 + err
}

func (r *recordingRunner) Run(_ context.Context, leaf string, env []string) (int, error) {
	r.runs = append(r.runs, leaf)
	r.envs = append(r.envs, env)
	if r.err != nil {
		return 1, r.err
	}
	return 0, nil
}

// setupProject writes a run-shape + a real leaf file under root/.vh-agent-harness/
// so os.Stat sees the leaf (otherwise it is a missing-leaf no-op).
func setupProject(t *testing.T, root string, hooks map[runshape.LifecycleHook]string) {
	t.Helper()
	dir := filepath.Join(root, runshape.DirName)
	scripts := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	var b strings.Builder
	b.WriteString("run_shape_version: \"0.1\"\nlifecycle:\n")
	for h, leaf := range hooks {
		if leaf == "" {
			continue
		}
		b.WriteString("  " + string(h) + ": " + leaf + "\n")
		leafPath := filepath.Join(dir, leaf)
		if err := os.MkdirAll(filepath.Dir(leafPath), 0o755); err != nil {
			t.Fatalf("mkdir leaf dir: %v", err)
		}
		if err := os.WriteFile(leafPath, []byte("#!/usr/bin/env bash\necho hi\n"), 0o755); err != nil {
			t.Fatalf("write leaf %s: %v", leaf, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, runshape.FileName), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write run-shape: %v", err)
	}
}

func loadRS(t *testing.T, root string) *runshape.RunShape {
	t.Helper()
	rs, err := runshape.LoadForRoot(root)
	if err != nil {
		t.Fatalf("LoadForRoot: %v", err)
	}
	return rs
}

// --- NO-BYPASS PROOF (criterion 3) ------------------------------------------

// TestFire_GateDeniedNotRun is the load-bearing no-bypass proof: when the gate
// denies, the leaf is NEVER executed. recordingRunner.runs must be empty and the
// record must be StatusDenied. The failure policy (FailVerb for pre_up) surfaces
// the denial as a non-nil error.
func TestFire_GateDeniedNotRun(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPreUp: "scripts/clean.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(denyGate{}, rr)

	rec, err := d.Fire(context.Background(), Point(runshape.HookPreUp), rs, "docker_compose", root)
	if rec.Status != StatusDenied {
		t.Errorf("status = %s, want denied", rec.Status)
	}
	if len(rr.runs) != 0 {
		t.Fatalf("GATE BYPASS: leaf was run despite deny: %v", rr.runs)
	}
	if err == nil {
		t.Fatalf("pre_up is FailVerb: a denied hook must surface a non-nil error")
	}
	var gde *GateDeniedError
	if !errors.As(err, &gde) {
		t.Errorf("error should be *GateDeniedError; got %T: %v", err, err)
	}
	if rec.GateAction != permission.Deny {
		t.Errorf("gate action = %s, want deny", rec.GateAction)
	}
}

// TestFire_GateAskNotRun — Ask is treated as deny-by-default (no operator loop);
// the leaf does not run.
func TestFire_GateAskNotRun(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPreUp: "scripts/clean.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(askGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPreUp), rs, "host-shell", root)
	if rec.Status != StatusDenied {
		t.Errorf("Ask should be fail-closed; status = %s", rec.Status)
	}
	if len(rr.runs) != 0 {
		t.Fatalf("GATE BYPASS: leaf run on Ask: %v", rr.runs)
	}
	if err == nil {
		t.Fatalf("Ask on a FailVerb point must surface an error")
	}
}

// TestFire_GateFaultNotRun — a gate fault (Deny+err) is fail-closed: no run.
func TestFire_GateFaultNotRun(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPreUp: "scripts/clean.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(faultGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPreUp), rs, "docker_compose", root)
	if rec.Status != StatusDenied {
		t.Errorf("fault should be fail-closed; status = %s", rec.Status)
	}
	if len(rr.runs) != 0 {
		t.Fatalf("GATE BYPASS: leaf run on gate fault: %v", rr.runs)
	}
	if err == nil {
		t.Fatalf("fault on a FailVerb point must surface an error")
	}
}

// TestFire_NilGateFailClosed — a Dispatcher built with a nil gate never runs a
// leaf (denyAllHook fallback). Defense in depth: nil gate != ungated.
func TestFire_NilGateFailClosed(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPreUp: "scripts/clean.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(nil, rr) // nil gate => denyAllHook
	rec, err := d.Fire(context.Background(), Point(runshape.HookPreUp), rs, "docker_compose", root)
	if rec.Status != StatusDenied {
		t.Errorf("nil gate should fail closed; status = %s", rec.Status)
	}
	if len(rr.runs) != 0 {
		t.Fatalf("GATE BYPASS: leaf run with nil gate: %v", rr.runs)
	}
	if err == nil {
		t.Fatalf("nil gate on FailVerb point must surface an error")
	}
}

// --- ALLOW + RUN (criterion 1, 5) -------------------------------------------

// TestFire_GateAllowedRuns — an allow gate runs the leaf once and records context.
func TestFire_GateAllowedRuns(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPostUp: "scripts/migrate.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(allowGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPostUp), rs, "docker_compose", root)
	if err != nil {
		t.Fatalf("allow+success should not error; got %v", err)
	}
	if rec.Status != StatusRan {
		t.Errorf("status = %s, want ran", rec.Status)
	}
	if len(rr.runs) != 1 {
		t.Fatalf("leaf should run exactly once; got %v", rr.runs)
	}
	if !strings.HasSuffix(rr.runs[0], "scripts/migrate.sh") {
		t.Errorf("ran leaf = %q, want .../scripts/migrate.sh", rr.runs[0])
	}
	// Criterion 5: lifecycle + backend recorded.
	if rec.Point != runshape.HookPostUp {
		t.Errorf("record point = %s, want post_up", rec.Point)
	}
	if rec.BackendName != "docker_compose" {
		t.Errorf("record backend = %q, want docker_compose", rec.BackendName)
	}
	if rec.Leaf != "scripts/migrate.sh" {
		t.Errorf("record leaf = %q", rec.Leaf)
	}
	if rec.GateAction != permission.Allow {
		t.Errorf("record gate action = %s, want allow", rec.GateAction)
	}
	// HARNESS_* env carries backend + verb + root.
	if len(rr.envs) != 1 {
		t.Fatalf("env not recorded")
	}
	envJoin := strings.Join(rr.envs[0], "\n")
	for _, want := range []string{"HARNESS_VERB=up", "HARNESS_BACKEND=docker_compose", "HARNESS_ROOT="} {
		if !strings.Contains(envJoin, want) {
			t.Errorf("env missing %q; got:\n%s", want, envJoin)
		}
	}
}

// --- ABSENT = NO-OP (criterion 4) -------------------------------------------

// TestFire_AbsentNoop — no pointer declared at the point => StatusNoop, nil err.
func TestFire_AbsentNoop(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPreUp: "scripts/clean.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(allowGate{}, rr)
	// post_down has no pointer declared.
	rec, err := d.Fire(context.Background(), Point(runshape.HookPostDown), rs, "docker_compose", root)
	if err != nil {
		t.Fatalf("absent point should not error; got %v", err)
	}
	if rec.Status != StatusNoop {
		t.Errorf("status = %s, want noop", rec.Status)
	}
	if len(rr.runs) != 0 {
		t.Errorf("absent point should not run anything; got %v", rr.runs)
	}
}

// TestFire_NoRunShape — a nil RunShape (no run-shape at all) is a clean no-op.
func TestFire_NoRunShape(t *testing.T) {
	root := t.TempDir()
	rr := &recordingRunner{}
	d := NewWithRunner(allowGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPreUp), nil, "host-shell", root)
	if err != nil {
		t.Fatalf("nil run-shape should not error; got %v", err)
	}
	if rec.Status != StatusNoop {
		t.Errorf("status = %s, want noop", rec.Status)
	}
}

// TestFire_MissingLeafWarnNoop — a pointer to a nonexistent leaf is a typed
// non-fatal warning, NOT a crash. The verb continues (nil error returned).
func TestFire_MissingLeafWarnNoop(t *testing.T) {
	root := t.TempDir()
	// Declare a pointer but DO NOT create the leaf file.
	dir := filepath.Join(root, runshape.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "run_shape_version: \"0.1\"\nlifecycle:\n  post_down: scripts/absent.sh\n"
	if err := os.WriteFile(filepath.Join(dir, runshape.FileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(allowGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPostDown), rs, "docker_compose", root)
	if err != nil {
		t.Fatalf("missing leaf must NOT crash the verb; got err=%v", err)
	}
	if rec.Status != StatusNoop {
		t.Errorf("missing leaf status = %s, want noop", rec.Status)
	}
	var mle *MissingLeafError
	if !errors.As(rec.Err, &mle) {
		t.Errorf("record.Err should be *MissingLeafError; got %T: %v", rec.Err, rec.Err)
	}
	if len(rr.runs) != 0 {
		t.Errorf("missing leaf should not run; got %v", rr.runs)
	}
}

// --- FAILURE POLICY (§4 table) ----------------------------------------------

// TestFire_PostDownWarnAndContinue — pre_down/post_down are warn-and-continue: a
// gate denial returns a nil verb error (the warning is logged).
func TestFire_PostDownWarnAndContinue(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPostDown: "scripts/cleanup.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{}
	d := NewWithRunner(denyGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPostDown), rs, "docker_compose", root)
	if rec.Status != StatusDenied {
		t.Errorf("status = %s, want denied", rec.Status)
	}
	if len(rr.runs) != 0 {
		t.Fatalf("denied leaf must not run: %v", rr.runs)
	}
	if err != nil {
		t.Errorf("post_down is WarnAndContinue; denial must return nil verb error; got %v", err)
	}
}

// TestFire_NonZeroExitFailVerb — a gate-allowed leaf that exits non-zero fails
// the verb on a FailVerb point (pre_up), with the exit code in the record.
func TestFire_NonZeroExitFailVerb(t *testing.T) {
	root := t.TempDir()
	setupProject(t, root, map[runshape.LifecycleHook]string{runshape.HookPreUp: "scripts/clean.sh"})
	rs := loadRS(t, root)

	rr := &recordingRunner{err: errors.New("exit status 2")}
	d := NewWithRunner(allowGate{}, rr)
	rec, err := d.Fire(context.Background(), Point(runshape.HookPreUp), rs, "docker_compose", root)
	if err == nil {
		t.Fatalf("non-zero exit on pre_up (FailVerb) must surface an error")
	}
	if rec.Status != StatusRan {
		t.Errorf("status = %s, want ran (it ran then failed)", rec.Status)
	}
	if rec.ExitCode != 1 {
		t.Errorf("exit code = %d, want 1", rec.ExitCode)
	}
}

// TestPolicyFor_FixedSet — the §4 table is encoded exactly.
func TestPolicyFor_FixedSet(t *testing.T) {
	cases := []struct {
		point Point
		want  FailurePolicy
	}{
		{runshape.HookOnFirstInstall, FailVerb},
		{runshape.HookOnUpdate, FailVerb},
		{runshape.HookPreUp, FailVerb},
		{runshape.HookPostUp, FailVerbServicesUp},
		{runshape.HookPreDown, WarnAndContinue},
		{runshape.HookPostDown, WarnAndContinue},
		{runshape.HookPreExec, FailVerb},
		{runshape.HookPostExec, WarnAndContinue},
		{runshape.HookOnUninstall, WarnAndContinue},
	}
	for _, c := range cases {
		if got := PolicyFor(c.point); got != c.want {
			t.Errorf("PolicyFor(%s) = %v, want %v", c.point, got, c.want)
		}
	}
}
