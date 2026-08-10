package cli

// pause_new_work_test.go verifies the Go half of the repo-scoped pause on new
// work (memo-4): the sentinel read contract mirrored from the JS module, and
// the engage/status/disengage CLI verbs. The behavioral crux (engaged refuses
// covered new work while in-flight work is untouched) is cross-verified in the
// JS black-box suite (tests/scripts/pause-new-work.test.js), which drives the
// real state-lib.js + bgshell + plugin paths. These Go tests pin the sentinel
// contract and the operator UX in isolation.
//
// These tests use runWithCwd (process-global cwd), so they MUST NOT use
// t.Parallel().

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupPauseRepo creates a temp harness-style repo (with a run-shape marker so
// pauseProjectRoot finds it) and returns its root.
func setupPauseRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rsDir := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(rsDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rsDir, err)
	}
	rsPath := filepath.Join(rsDir, "run-shape.yml")
	if err := os.WriteFile(rsPath, []byte("runtime:\n  backend: host-shell\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", rsPath, err)
	}
	return dir
}

func TestPauseReadState_AbsentIsDisengaged(t *testing.T) {
	dir := setupPauseRepo(t)
	runWithCwd(t, dir, func() {
		root, err := pauseProjectRoot()
		if err != nil {
			t.Fatalf("pauseProjectRoot: %v", err)
		}
		if root != dir {
			t.Fatalf("root: want %s, got %s", dir, root)
		}
		engaged, degraded, meta, readErr := pauseReadState(root)
		if readErr != nil {
			t.Fatalf("readErr: %v", readErr)
		}
		if engaged {
			t.Errorf("absent sentinel: want engaged=false (disengaged), got true")
		}
		if degraded {
			t.Errorf("absent sentinel: want degraded=false, got true")
		}
		if meta != nil {
			t.Errorf("absent sentinel: want nil meta, got %v", meta)
		}
	})
}

func TestPauseReadState_PresentValidIsEngaged(t *testing.T) {
	dir := setupPauseRepo(t)
	runWithCwd(t, dir, func() {
		root, _ := pauseProjectRoot()
		sp := pauseSentinelPath(root)
		if err := os.MkdirAll(filepath.Dir(sp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sp, []byte(`{"engaged_at":"2026-08-10T00:00:00Z","reason":"maintenance"}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		engaged, degraded, meta, readErr := pauseReadState(root)
		if readErr != nil {
			t.Fatalf("readErr: %v", readErr)
		}
		if !engaged {
			t.Errorf("valid sentinel: want engaged=true, got false")
		}
		if degraded {
			t.Errorf("valid sentinel: want degraded=false, got true")
		}
		if meta == nil || meta["reason"] != "maintenance" {
			t.Errorf("valid sentinel meta: want reason=maintenance, got %v", meta)
		}
	})
}

func TestPauseReadState_MalformedIsEngagedNotDegraded(t *testing.T) {
	dir := setupPauseRepo(t)
	runWithCwd(t, dir, func() {
		root, _ := pauseProjectRoot()
		sp := pauseSentinelPath(root)
		if err := os.MkdirAll(filepath.Dir(sp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sp, []byte("{not json"), 0o644); err != nil {
			t.Fatal(err)
		}
		engaged, degraded, _, readErr := pauseReadState(root)
		if readErr != nil {
			t.Fatalf("readErr: %v", readErr)
		}
		if !engaged {
			t.Errorf("malformed sentinel: want engaged=true (fail-safe), got false")
		}
		if degraded {
			t.Errorf("malformed sentinel: want degraded=false (content is advisory), got true")
		}
	})
}

func TestPauseReadState_EmptyIsEngagedNotDegraded(t *testing.T) {
	dir := setupPauseRepo(t)
	runWithCwd(t, dir, func() {
		root, _ := pauseProjectRoot()
		sp := pauseSentinelPath(root)
		if err := os.MkdirAll(filepath.Dir(sp), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sp, []byte("   \n"), 0o644); err != nil {
			t.Fatal(err)
		}
		engaged, degraded, _, _ := pauseReadState(root)
		if !engaged {
			t.Errorf("empty sentinel: want engaged=true (fail-safe), got false")
		}
		if degraded {
			t.Errorf("empty sentinel: want degraded=false, got true")
		}
	})
}

func TestPauseStateRoot_OverrideEnv(t *testing.T) {
	t.Setenv("OPENCODE_STATE_ROOT", "/custom/state/root")
	if got := pauseStateRoot("anyrepo"); got != "/custom/state/root" {
		t.Errorf("override: want /custom/state/root, got %s", got)
	}
}

func TestPauseStateRoot_DefaultUnderRepo(t *testing.T) {
	t.Setenv("OPENCODE_STATE_ROOT", "")
	if got := pauseStateRoot("/r"); got != filepath.Join("/r", ".opencode", "state") {
		t.Errorf("default: want /r/.opencode/state, got %s", got)
	}
}

func TestPauseEngageThenStatusThenDisengage(t *testing.T) {
	dir := setupPauseRepo(t)
	runWithCwd(t, dir, func() {
		root, _ := pauseProjectRoot()

		// engage
		engCmd, engBuf := newOutCmd()
		if err := runPauseEngage(engCmd, []string{"maintenance", "window"}); err != nil {
			t.Fatalf("engage: %v", err)
		}
		if !strings.Contains(engBuf.String(), "engaged") {
			t.Errorf("engage output: want 'engaged', got %q", engBuf.String())
		}
		if _, err := os.Stat(pauseSentinelPath(root)); err != nil {
			t.Fatalf("sentinel not written: %v", err)
		}

		// status -> engaged
		stCmd, stBuf := newOutCmd()
		if err := runPauseStatus(stCmd, nil); err != nil {
			t.Fatalf("status: %v", err)
		}
		if !strings.Contains(stBuf.String(), "engaged") || !strings.Contains(stBuf.String(), "maintenance window") {
			t.Errorf("status output: want engaged + reason, got %q", stBuf.String())
		}

		// disengage
		disCmd, disBuf := newOutCmd()
		if err := runPauseDisengage(disCmd, nil); err != nil {
			t.Fatalf("disengage: %v", err)
		}
		if !strings.Contains(disBuf.String(), "disengaged") {
			t.Errorf("disengage output: want disengaged, got %q", disBuf.String())
		}
		if _, err := os.Stat(pauseSentinelPath(root)); !os.IsNotExist(err) {
			t.Fatalf("sentinel not removed after disengage (err=%v)", err)
		}

		// status -> disengaged
		stCmd2, stBuf2 := newOutCmd()
		if err := runPauseStatus(stCmd2, nil); err != nil {
			t.Fatalf("status after disengage: %v", err)
		}
		if !strings.Contains(stBuf2.String(), "disengaged") {
			t.Errorf("status after disengage: want disengaged, got %q", stBuf2.String())
		}
	})
}

func TestPauseDisengage_AbsentIsNoop(t *testing.T) {
	dir := setupPauseRepo(t)
	runWithCwd(t, dir, func() {
		cmd, buf := newOutCmd()
		if err := runPauseDisengage(cmd, nil); err != nil {
			t.Fatalf("disengage absent: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "disengaged") || !strings.Contains(out, "already absent") {
			t.Errorf("disengage-absent output: want disengaged + already absent, got %q", out)
		}
	})
}

func TestPauseStatus_NoInstallIsReachable(t *testing.T) {
	// status must remain reachable even with no harness install (invariant 4).
	dir := t.TempDir()
	runWithCwd(t, dir, func() {
		cmd, buf := newOutCmd()
		if err := runPauseStatus(cmd, nil); err != nil {
			t.Fatalf("status no-install: %v", err)
		}
		if !strings.Contains(buf.String(), "disengaged") {
			t.Errorf("status no-install: want disengaged, got %q", buf.String())
		}
	})
}

func TestPauseEngage_NoInstallErrors(t *testing.T) {
	// engage without an install must error (cannot resolve where to write).
	dir := t.TempDir()
	runWithCwd(t, dir, func() {
		cmd, _ := newOutCmd()
		err := runPauseEngage(cmd, nil)
		if err == nil {
			t.Fatal("engage no-install: want error, got nil")
		}
	})
}
