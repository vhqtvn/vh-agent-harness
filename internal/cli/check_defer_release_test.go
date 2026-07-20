package cli

// Shared release-mode evaluator helpers + the PROMOTER-MODE byte-identical
// guard for check-defer-triggers.js.
//
// The evaluator (templates/core/.opencode/scripts/check-defer-triggers.js) has
// TWO modes:
//   - PROMOTER mode (default, no --mode flag): human-readable, exit 0, never
//     blocking. This is the commit-time DEFER check that reads .local/ and
//     MUST stay unchanged. TestCheckDefer_PromoterModeUnchanged pins it.
//   - RELEASE mode (--mode=release): manifest-authority ONLY. Reads the
//     committed manifest at .vh-agent-harness/release-defer-dispositions.json
//     and emits structured JSON. The legacy .local/-scan release path has been
//     RETIRED; manifest authority is the sole release-authority model. The
//     manifest-mode release tests live in check_defer_release_manifest_test.go.
//
// The helpers below (releaseCardNotes, setupReleaseEvalRepo, writeReleaseCard)
// are shared with check_defer_release_manifest_test.go. They copy the TEMPLATE
// script (source of truth, independent of whether `make update` has run) into
// an isolated scratch git repo with controlled task cards, mirroring the
// hermetic pattern of TestCommitGate_BacklogSplitPreflight.

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// releaseCardNotes builds the owner_notes provenance block for a review-defer
// candidate. Pass trigger="" to omit the trigger line (malformed card).
func releaseCardNotes(source, trigger, studied string) []string {
	notes := []string{}
	if source != "" {
		notes = append(notes, source)
	}
	if trigger != "" {
		notes = append(notes, "trigger:"+trigger)
	}
	if studied != "" {
		notes = append(notes, "studied:"+studied)
	}
	return notes
}

// setupReleaseEvalRepo creates an isolated scratch git repo with a prior tag
// (v0.1.0) and controlled post-tag changes, plus a copy of the TEMPLATE
// check-defer-triggers.js at <scratch>/.opencode/scripts/. The repo's HEAD has:
//   - fileA.go and dir/fileC.go CHANGED since v0.1.0 (in the release arc)
//   - fileB.go UNCHANGED since v0.1.0 (NOT in the release arc)
//
// Returns (scratchDir, scriptPath, tasksDir). The tasks dir exists but is empty;
// individual tests write cards into it.
func setupReleaseEvalRepo(t *testing.T) (scratch, scriptPath, tasksDir string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not on PATH: %v", err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not on PATH: %v", err)
	}

	root := findModuleRoot(t)
	src := filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.js")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read template script %s: %v", src, err)
	}

	scratch = t.TempDir()
	// Copy script to <scratch>/.opencode/scripts/ so the script's
	// __dirname-based repoRoot() (path.resolve(__dirname, "..", ".."))
	// resolves to <scratch>, keeping the run hermetic.
	scriptPath = filepath.Join(scratch, ".opencode", "scripts", "check-defer-triggers.js")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	if err := os.WriteFile(scriptPath, body, 0o644); err != nil {
		t.Fatalf("write scratch script: %v", err)
	}

	tasksDir = filepath.Join(scratch, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks dir: %v", err)
	}

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	git("config", "commit.gpgsign", "false")

	writeAndStage := func(rel, content string) {
		t.Helper()
		full := filepath.Join(scratch, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// Initial commit + tag v0.1.0.
	writeAndStage("fileA.go", "package main\n")
	writeAndStage("fileB.go", "package main\n")
	writeAndStage("dir/fileC.go", "package dir\n")
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	git("tag", "v0.1.0")

	// Post-tag changes (these are in the arc v0.1.0..HEAD).
	writeAndStage("fileA.go", "package main\n// changed in arc\n")
	writeAndStage("dir/fileC.go", "package dir\n// changed in arc\n")
	git("add", "-A")
	git("commit", "-q", "-m", "changes for release")

	return scratch, scriptPath, tasksDir
}

// writeReleaseCard writes a task-card JSON file into tasksDir with the given
// task_id, lifecycle status, and owner_notes. The card carries only the fields
// the evaluator reads (task_id, status, owner_notes); other schema fields are
// omitted for test simplicity.
func writeReleaseCard(t *testing.T, tasksDir, filename, id, status string, notes []string) {
	t.Helper()
	card := map[string]interface{}{
		"schema_version": 1,
		"task_id":        id,
		"status":         status,
		"owner_notes":    notes,
	}
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card %s: %v", filename, err)
	}
	full := filepath.Join(tasksDir, filename)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatalf("write card %s: %v", filename, err)
	}
}

// TestCheckDefer_PromoterModeUnchanged — the DEFAULT mode (no --mode flag) must
// remain exactly as before: human-readable, exit 0, never blocking. This is the
// backward-compatibility guard: the release-mode simplification MUST NOT change
// promoter behavior. Reuses the existing TestCheckDeferTriggersScriptLoads
// patterns.
func TestCheckDefer_PromoterModeUnchanged(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not on PATH: %v", err)
	}
	_, script, tasksDir := setupReleaseEvalRepo(t)
	// Write a card that WOULD be a blocker in release mode.
	writeReleaseCard(t, tasksDir, "defer-fired.json", "defer-fired",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))

	// Run WITHOUT --mode=release (default promoter mode).
	cmd := exec.Command(nodeBin, script, "--tasks", tasksDir, "--since", "v0.1.0")
	cmd.Dir = filepath.Dir(filepath.Dir(filepath.Dir(script)))
	var outb, errb strings.Builder
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	timer := time.AfterFunc(30*time.Second, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("node spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	stdout := outb.String()
	// Promoter mode MUST exit 0 (never blocking).
	if exitCode != 0 {
		t.Fatalf("promoter mode must exit 0 (never blocking); got %d\n%s", exitCode, stdout)
	}
	// Promoter mode MUST be human-readable (the distinctive banner), NOT JSON.
	if !strings.Contains(stdout, "check-defer-triggers report") {
		t.Errorf("promoter mode must emit the human-readable banner; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Promoter-use-only") && !strings.Contains(stdout, "promoter-use-only") {
		t.Errorf("promoter mode must state 'promoter-use-only'; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "never blocking") {
		t.Errorf("promoter mode must state 'never blocking'; got:\n%s", stdout)
	}
	// Must NOT be JSON (no leading {).
	trimmed := strings.TrimLeft(stdout, " \t\n\r")
	if strings.HasPrefix(trimmed, "{") {
		t.Errorf("promoter mode must NOT emit JSON; got JSON-like output:\n%s", stdout)
	}
}
