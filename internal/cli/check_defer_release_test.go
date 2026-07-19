package cli

// Release-mode tests for check-defer-triggers.js.
//
// These test the STRICT release-evaluation mode (--mode=release) of the
// promoter-use-only predicate checker. The existing promoter mode (no --mode
// flag) is covered by TestCheckDeferTriggersScriptLoads in
// check_defer_triggers_test.go and MUST stay unchanged (exit 0, human-readable,
// never blocking). This file pins the release-mode contract:
//
//   - selects ONLY source:review-defer candidates (excludes source:p2-followup)
//   - emits structured JSON with deterministic classification + sorted IDs
//   - returns NONZERO for blocker or evaluator-error classifications
//   - returns 0 for clear (no candidates / all resolved) or advisory (unfired)
//   - REJECTS unsupported trigger grammar (||, non-predicate terms, directory
//     operands, empty args) as evaluator-error — NEVER silently unfired
//   - tasks dir ABSENT or EMPTY → clear (pass); unreadable → fail closed
//   - JSON output is valid JSON for unusual task IDs / trigger strings
//
// The tests copy the TEMPLATE script (source of truth, independent of whether
// `make update` has run) into an isolated scratch git repo with controlled
// task cards, mirroring the hermetic pattern of TestCommitGate_BacklogSplitPreflight.

import (
	"encoding/json"
	"errors"
	"fmt"
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

// releaseResult is the parsed JSON envelope the release-mode evaluator emits.
// Only the fields the tests assert are typed; findings carry raw interface{} for
// per-test flexibility.
type releaseResult struct {
	Mode            string                   `json:"mode"`
	Since           string                   `json:"since"`
	TasksDirState   string                   `json:"tasks_dir_state"`
	ArcPaths        []string                 `json:"arc_paths"`
	Classification  string                   `json:"classification"`
	Findings        []map[string]interface{} `json:"findings"`
	BlockingIDs     []string                 `json:"blocking_ids"`
	AdvisoryIDs     []string                 `json:"advisory_ids"`
	EvaluatorErrIDs []string                 `json:"evaluator_error_ids"`
	ResolvedIDs     []string                 `json:"resolved_ids"`
	Error           *string                  `json:"error"`
}

// runReleaseEval runs the evaluator in release mode with the given extra args
// (typically --since <ref>). Returns (exitCode, parsedResult, stdout, stderr).
// The cwd is set to the scratch repo root derived from the script path.
func runReleaseEval(t *testing.T, script, tasksDir string, extraArgs ...string) (int, releaseResult, string, string) {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node not on PATH: %v", err)
	}
	args := []string{script, "--mode=release", "--tasks", tasksDir}
	args = append(args, extraArgs...)
	cmd := exec.Command(nodeBin, args...)
	// repoRoot = <script-dir>/../.. = <scratch>. script is at
	// <scratch>/.opencode/scripts/check-defer-triggers.js.
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
	var result releaseResult
	stdout := outb.String()
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("release-mode output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
			exitCode, err, stdout, errb.String())
	}
	return exitCode, result, stdout, errb.String()
}

// TestCheckDefer_ReleaseMode_TasksDirAbsent — the absent-tasks-dir policy:
// when the tasks directory does NOT exist, the evaluator MUST classify as clear
// (pass). Absence is not a mandatory surface (local candidates are transport,
// not truth). This is the OPERATOR-CONFIRMED policy from the settled decisions.
func TestCheckDefer_ReleaseMode_TasksDirAbsent(t *testing.T) {
	_, script, _ := setupReleaseEvalRepo(t)
	absent := filepath.Join(t.TempDir(), "does-not-exist")
	exitCode, result, _, _ := runReleaseEval(t, script, absent, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("absent tasks dir must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "clear" {
		t.Errorf("absent tasks dir → classification clear; got %q", result.Classification)
	}
	if result.TasksDirState != "absent" {
		t.Errorf("absent tasks dir → tasks_dir_state absent; got %q", result.TasksDirState)
	}
	if len(result.Findings) != 0 {
		t.Errorf("absent tasks dir → 0 findings; got %d", len(result.Findings))
	}
}

// TestCheckDefer_ReleaseMode_TasksDirEmpty — an existing but EMPTY tasks dir
// MUST classify as clear (pass). No candidates = nothing to block on.
func TestCheckDefer_ReleaseMode_TasksDirEmpty(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("empty tasks dir must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "clear" {
		t.Errorf("empty tasks dir → classification clear; got %q", result.Classification)
	}
	if result.TasksDirState != "empty" {
		t.Errorf("empty tasks dir → tasks_dir_state empty; got %q", result.TasksDirState)
	}
}

// TestCheckDefer_ReleaseMode_UnfiredUnresolved — a candidate whose
// path_touched(<file-not-in-arc>) trigger does NOT fire is advisory (unfired
// warning). Exit 0 (advisory does not block).
func TestCheckDefer_ReleaseMode_UnfiredUnresolved(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-unfired.json", "defer-unfired",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileB.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("unfired advisory must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "advisory" {
		t.Errorf("unfired unresolved → classification advisory; got %q", result.Classification)
	}
	if len(result.AdvisoryIDs) != 1 || result.AdvisoryIDs[0] != "defer-unfired" {
		t.Errorf("advisory_ids = [defer-unfired]; got %v", result.AdvisoryIDs)
	}
	if len(result.BlockingIDs) != 0 {
		t.Errorf("unfired → 0 blocking_ids; got %v", result.BlockingIDs)
	}
}

// TestCheckDefer_ReleaseMode_FiredUnresolvedReleaseRelevant — the core BLOCKER
// case: path_touched(<file-in-arc>) fires, the card is unresolved → blocker,
// nonzero exit.
func TestCheckDefer_ReleaseMode_FiredUnresolvedReleaseRelevant(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-fired.json", "defer-fired",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("fired-unresolved-release-relevant must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("fired-unresolved-release-relevant → classification blocker; got %q", result.Classification)
	}
	if len(result.BlockingIDs) != 1 || result.BlockingIDs[0] != "defer-fired" {
		t.Errorf("blocking_ids = [defer-fired]; got %v", result.BlockingIDs)
	}
}

// TestCheckDefer_ReleaseMode_FiredUnresolvedDeepPath — a path under a
// subdirectory (dir/fileC.go) that IS in the arc must also fire (exact path
// match, not directory-prefix).
func TestCheckDefer_ReleaseMode_FiredUnresolvedDeepPath(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-deep.json", "defer-deep",
		"working", releaseCardNotes("source:review-defer", "path_touched(dir/fileC.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("fired deep-path must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("fired deep-path → blocker; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_FiredAfterTag — after_tag(<existing-tag>) fires
// → blocker (after_tag firing means the candidate is active for this release).
func TestCheckDefer_ReleaseMode_FiredAfterTag(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-aftertag.json", "defer-aftertag",
		"draft", releaseCardNotes("source:review-defer", "after_tag(v0.1.0)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("fired after_tag must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("fired after_tag → blocker; got %q", result.Classification)
	}
	if len(result.BlockingIDs) != 1 || result.BlockingIDs[0] != "defer-aftertag" {
		t.Errorf("blocking_ids = [defer-aftertag]; got %v", result.BlockingIDs)
	}
}

// TestCheckDefer_ReleaseMode_AfterTagNotFired — after_tag(<nonexistent>) does
// not fire → advisory (exit 0).
func TestCheckDefer_ReleaseMode_AfterTagNotFired(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-noaftertag.json", "defer-noaftertag",
		"draft", releaseCardNotes("source:review-defer", "after_tag(v9.9.9)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("unfired after_tag must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "advisory" {
		t.Errorf("unfired after_tag → advisory; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_FiredButCompleted — a fired trigger on a card
// whose lifecycle status is "completed" is RESOLVED → clear (informational).
func TestCheckDefer_ReleaseMode_FiredButCompleted(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-done.json", "defer-done",
		"completed", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("resolved (completed) must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "clear" {
		t.Errorf("resolved (completed) → clear; got %q", result.Classification)
	}
	if len(result.ResolvedIDs) != 1 || result.ResolvedIDs[0] != "defer-done" {
		t.Errorf("resolved_ids = [defer-done]; got %v", result.ResolvedIDs)
	}
}

// TestCheckDefer_ReleaseMode_FiredButCancelled — cancelled is also resolved.
func TestCheckDefer_ReleaseMode_FiredButCancelled(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-cancelled.json", "defer-cancelled",
		"cancelled", releaseCardNotes("source:review-defer", "after_tag(v0.1.0)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("resolved (cancelled) must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "clear" {
		t.Errorf("resolved (cancelled) → clear; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_SourceP2FollowupExcluded — a source:p2-followup
// candidate is OUTSIDE v1 release scope and MUST NOT appear in findings at all.
func TestCheckDefer_ReleaseMode_SourceP2FollowupExcluded(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "p2-followup.json", "p2-followup",
		"draft", releaseCardNotes("source:p2-followup", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("source:p2-followup is excluded → must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "clear" {
		t.Errorf("excluded p2-followup → clear (no candidates); got %q", result.Classification)
	}
	if len(result.Findings) != 0 {
		t.Errorf("p2-followup must NOT appear in findings; got %d findings", len(result.Findings))
	}
}

// TestCheckDefer_ReleaseMode_NoSourceExcluded — a card with no source: line at
// all is not a review-defer candidate and is excluded.
func TestCheckDefer_ReleaseMode_NoSourceExcluded(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "no-source.json", "no-source",
		"draft", []string{"trigger:path_touched(fileA.go)", "studied:2026-07-01"})
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("no source: → excluded → PASS (exit 0); got %d", exitCode)
	}
	if len(result.Findings) != 0 {
		t.Errorf("card without source: must not appear in findings; got %d", len(result.Findings))
	}
}

// TestCheckDefer_ReleaseMode_UnknownStatus — an unrecognized lifecycle status
// is evaluator-error (fail closed; NOT implicit pass).
func TestCheckDefer_ReleaseMode_UnknownStatus(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-weird.json", "defer-weird",
		"frozen", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("unknown lifecycle status must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("unknown status → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_UnsupportedGrammar — a trigger line using `||`
// separators (a real existing card pattern) is evaluator-error, NOT silently
// unfired. This is the key safety property: the evaluator never silently
// bypasses by treating broken grammar as "not met."
func TestCheckDefer_ReleaseMode_UnsupportedGrammar(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	// This mirrors .local/coordinator/tasks/defer-p1-lineage-002-001.json.
	writeReleaseCard(t, tasksDir, "defer-broken.json", "defer-broken",
		"draft", releaseCardNotes("source:review-defer",
			"path_touched(internal/renderstate/)||path_touched(internal/cli/seam.go)||feature:prune-phase",
			"2026-07-15"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("unsupported trigger grammar must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("unsupported grammar → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_DirectoryOperand — a path_touched arg ending with
// `/` is a directory operand (no directory-prefix matching in v1) and must be
// evaluator-error.
func TestCheckDefer_ReleaseMode_DirectoryOperand(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-dir.json", "defer-dir",
		"draft", releaseCardNotes("source:review-defer", "path_touched(internal/renderstate/)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("directory operand must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("directory operand → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_NonPredicateTerm — a trigger term that does not
// match any known predicate (e.g. feature:prune-phase) is evaluator-error.
func TestCheckDefer_ReleaseMode_NonPredicateTerm(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-nonpred.json", "defer-nonpred",
		"draft", releaseCardNotes("source:review-defer", "feature:prune-phase", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("non-predicate term must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("non-predicate term → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_MissingTrigger — a review-defer card without a
// trigger: line is malformed (missing required provenance) → evaluator-error.
func TestCheckDefer_ReleaseMode_MissingTrigger(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-notrigger.json", "defer-notrigger",
		"draft", []string{"source:review-defer", "studied:2026-07-01"})
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("missing trigger must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("missing trigger → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_MissingStudied — a review-defer card without a
// studied: line is malformed → evaluator-error.
func TestCheckDefer_ReleaseMode_MissingStudied(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-nostudied.json", "defer-nostudied",
		"draft", []string{"source:review-defer", "trigger:path_touched(fileA.go)"})
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("missing studied must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("missing studied → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_MalformedJSON — a task file that is not valid JSON
// is evaluator-error (fail closed).
func TestCheckDefer_ReleaseMode_MalformedJSON(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	// Write a file that LOOKS like a task card (so the evaluator picks it up)
	// but contains broken JSON.
	full := filepath.Join(tasksDir, "defer-brokenjson.json")
	if err := os.WriteFile(full, []byte("{ this is not valid json,"), 0o644); err != nil {
		t.Fatalf("write broken card: %v", err)
	}
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("malformed JSON must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("malformed JSON → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_TriggerAnyOR — trigger:any(...) uses OR semantics.
// Here one inner path is in the arc (fires) and one is not; the OR fires →
// blocker (unresolved).
func TestCheckDefer_ReleaseMode_TriggerAnyOR(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-any.json", "defer-any",
		"draft", releaseCardNotes("source:review-defer",
			"any(path_touched(fileB.go),path_touched(fileA.go))", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("trigger:any with one firing member must REFUSE (nonzero); got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("trigger:any firing → blocker; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_TriggerAnyNoneFired — trigger:any(...) where no
// inner predicate fires → advisory.
func TestCheckDefer_ReleaseMode_TriggerAnyNoneFired(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-anynone.json", "defer-anynone",
		"draft", releaseCardNotes("source:review-defer",
			"any(path_touched(fileB.go),after_tag(v9.9.9))", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("trigger:any none fired → advisory (exit 0); got %d", exitCode)
	}
	if result.Classification != "advisory" {
		t.Errorf("trigger:any none fired → advisory; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_MultipleTriggerLinesAND — multiple ordinary
// trigger: lines use AND semantics. Both must fire for the candidate to fire.
func TestCheckDefer_ReleaseMode_MultipleTriggerLinesAND(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	// Two trigger lines: path_touched(fileA.go) [fires] + after_tag(v9.9.9) [no fire].
	// AND → does not fire → advisory.
	notes := []string{
		"source:review-defer",
		"trigger:path_touched(fileA.go)",
		"trigger:after_tag(v9.9.9)",
		"studied:2026-07-01",
	}
	writeReleaseCard(t, tasksDir, "defer-and.json", "defer-and", "draft", notes)
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("AND with one not-fired → advisory (exit 0); got %d", exitCode)
	}
	if result.Classification != "advisory" {
		t.Errorf("AND one-not-fired → advisory; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_DeterministicSorting — multiple blockers must
// appear in the blocking_ids array in SORTED order (deterministic output).
func TestCheckDefer_ReleaseMode_DeterministicSorting(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	// Create three blockers with IDs that are NOT in alphabetical file-write order.
	writeReleaseCard(t, tasksDir, "c.json", "defer-ccc",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	writeReleaseCard(t, tasksDir, "a.json", "defer-aaa",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	writeReleaseCard(t, tasksDir, "b.json", "defer-bbb",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("multiple blockers must REFUSE; got exit 0")
	}
	want := []string{"defer-aaa", "defer-bbb", "defer-ccc"}
	if len(result.BlockingIDs) != 3 {
		t.Fatalf("expected 3 blocking_ids; got %v", result.BlockingIDs)
	}
	for i, w := range want {
		if result.BlockingIDs[i] != w {
			t.Errorf("blocking_ids[%d] = %q, want %q (sorted); full=%v", i, result.BlockingIDs[i], w, result.BlockingIDs)
		}
	}
	// Findings must also be sorted by id.
	for i, w := range want {
		if got := result.Findings[i]["id"]; got != w {
			t.Errorf("findings[%d].id = %v, want %q (sorted)", i, got, w)
		}
	}
}

// TestCheckDefer_ReleaseMode_JSONSafety — task IDs and trigger strings with
// characters that would break naive JSON escaping (quotes, backslashes, control
// chars) MUST produce valid JSON. The parsed result round-trips the exact ID.
func TestCheckDefer_ReleaseMode_JSONSafety(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	weirdID := `defer-"quote"-back\slash`
	writeReleaseCard(t, tasksDir, "weird.json", weirdID,
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("weird-ID blocker must REFUSE; got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("weird ID → blocker; got %q", result.Classification)
	}
	if len(result.BlockingIDs) != 1 || result.BlockingIDs[0] != weirdID {
		t.Errorf("blocking_ids must round-trip exact weird ID %q; got %v", weirdID, result.BlockingIDs)
	}
}

// TestCheckDefer_ReleaseMode_MixedFindings — a mix of resolved, advisory, and
// blocker candidates produces the correct top-level classification (blocker
// wins) and correct per-category ID lists.
func TestCheckDefer_ReleaseMode_MixedFindings(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	// Resolved (completed) + fired → clear per-card.
	writeReleaseCard(t, tasksDir, "resolved.json", "defer-resolved",
		"completed", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	// Unfired advisory.
	writeReleaseCard(t, tasksDir, "advisory.json", "defer-advisory",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileB.go)", "2026-07-01"))
	// Blocker.
	writeReleaseCard(t, tasksDir, "blocker.json", "defer-blocker",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("has a blocker → REFUSE; got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("mixed with a blocker → blocker; got %q", result.Classification)
	}
	if !containsStr(result.BlockingIDs, "defer-blocker") {
		t.Errorf("defer-blocker must be in blocking_ids; got %v", result.BlockingIDs)
	}
	if !containsStr(result.AdvisoryIDs, "defer-advisory") {
		t.Errorf("defer-advisory must be in advisory_ids; got %v", result.AdvisoryIDs)
	}
	if !containsStr(result.ResolvedIDs, "defer-resolved") {
		t.Errorf("defer-resolved must be in resolved_ids; got %v", result.ResolvedIDs)
	}
}

// TestCheckDefer_ReleaseMode_EvaluatorErrorPrecedence — if any card is
// evaluator-error, the top-level classification is evaluator-error (fail
// closed), even if there are also advisory cards.
func TestCheckDefer_ReleaseMode_EvaluatorErrorPrecedence(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "adv.json", "defer-adv",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileB.go)", "2026-07-01"))
	writeReleaseCard(t, tasksDir, "err.json", "defer-err",
		"draft", releaseCardNotes("source:review-defer", "feature:unknown", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode == 0 {
		t.Fatalf("evaluator-error present → REFUSE; got exit 0")
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("evaluator-error precedence → evaluator-error; got %q", result.Classification)
	}
}

// TestCheckDefer_ReleaseMode_NoSinceArg — without --since, the evaluator
// auto-resolves (latest tag or HEAD~N fallback). The scratch repo has v0.1.0,
// so auto-since = v0.1.0 and the evaluation is the same as --since v0.1.0.
func TestCheckDefer_ReleaseMode_NoSinceArg(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	writeReleaseCard(t, tasksDir, "defer-fired.json", "defer-fired",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir /* no --since */)
	if exitCode == 0 {
		t.Fatalf("fired-unresolved must REFUSE even without --since; got exit 0")
	}
	if result.Classification != "blocker" {
		t.Errorf("fired-unresolved → blocker; got %q", result.Classification)
	}
	if result.Since != "v0.1.0" {
		t.Errorf("auto-since should resolve to v0.1.0; got %q", result.Since)
	}
}

// TestCheckDefer_ReleaseMode_ArcPathsInOutput — the release arc's changed paths
// are surfaced in the output (for diagnostics + so G7/wrapper can show what
// was evaluated). This pins that the evaluator actually computed the arc.
func TestCheckDefer_ReleaseMode_ArcPathsInOutput(t *testing.T) {
	_, script, tasksDir := setupReleaseEvalRepo(t)
	exitCode, result, stdoutRaw, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.0")
	if exitCode != 0 {
		t.Fatalf("empty tasks dir → exit 0; got %d", exitCode)
	}
	// arc_paths should contain fileA.go and dir/fileC.go (both changed since v0.1.0).
	for _, want := range []string{"fileA.go", "dir/fileC.go"} {
		if !strings.Contains(stdoutRaw, fmt.Sprintf("%q", want)) {
			t.Errorf("arc_paths must contain %s; stdout:\n%s", want, stdoutRaw)
		}
	}
	// fileB.go was NOT changed since v0.1.0 and must not be in the arc.
	if strings.Contains(stdoutRaw, "fileB.go") {
		t.Errorf("fileB.go must NOT be in the arc; stdout:\n%s", stdoutRaw)
	}
	_ = result
}

// TestCheckDefer_PromoterModeUnchanged — the DEFAULT mode (no --mode flag) must
// remain exactly as before: human-readable, exit 0, never blocking. This is the
// backward-compatibility guard: adding release mode MUST NOT change promoter
// behavior. Reuses the existing TestCheckDeferTriggersScriptLoads patterns.
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

// =============================================================================
// FAIL-OPEN DEFECT COVERAGE — null/indeterminate release arc (tier1_b/F1 fix)
// =============================================================================
//
// The release-safety gate MUST fail closed when the release arc cannot be
// deterministically computed. The canonical fail-open path (no prior tag →
// HEAD~32 fallback → isSafeRef rejects `~` → changedPathsSince returns null →
// fired path_touched candidate silently downgraded to advisory → exit 0 →
// wrapper proceeds to `git tag`) is closed by mapping null changedPaths to
// evaluator-error BEFORE candidate evaluation in mainRelease.
//
// These tests also pin the empty-arc-vs-null-arc distinction: an empty Set
// (deterministic, zero changed paths) is NOT null and evaluates normally.

// TestCheckDefer_ReleaseMode_NoPriorTag_HeadFallback_FailClosed — the fail-open
// defect: a repo with NO prior tag, a fired path_touched review-DEFER. The
// evaluator falls back to HEAD~32, isSafeRef rejects `~`, changedPathsSince
// returns null. Before the fix this silently downgraded the fired candidate to
// advisory (exit 0); after the fix it MUST be evaluator-error (exit 2).
//
// This is the hermetic evaluator-level proof that invariant #2 (nonzero exit
// on evaluator-error / fail-closed) holds even when the release arc is null.
func TestCheckDefer_ReleaseMode_NoPriorTag_HeadFallback_FailClosed(t *testing.T) {
	scratch, script, tasksDir := setupReleaseEvalRepo(t)
	// Delete the prior tag to simulate the no-prior-tag scenario.
	// resolveSince will then fall back to HEAD~32, which isSafeRef rejects.
	cmd := exec.Command("git", "-C", scratch, "tag", "-d", "v0.1.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag -d v0.1.0: %v\n%s", err, out)
	}
	// A path_touched card referencing fileA.go (which exists in the repo).
	// Before the fix this would be classified advisory (no-git-diff-data);
	// after the fix the null arc short-circuits to evaluator-error BEFORE
	// this candidate is ever evaluated.
	writeReleaseCard(t, tasksDir, "defer-failopen.json", "defer-failopen",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	// No --since: resolveSince auto-resolves (no tag → HEAD~32 fallback).
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir /* no --since */)
	if exitCode != 2 {
		t.Fatalf("null arc (HEAD~32 fallback) must be evaluator-error exit 2; got %d", exitCode)
	}
	if result.Classification != "evaluator-error" {
		t.Errorf("null arc → classification evaluator-error; got %q", result.Classification)
	}
	// The candidate must NOT be classified as advisory (the fail-open path).
	if containsStr(result.AdvisoryIDs, "defer-failopen") {
		t.Errorf("null arc must NOT downgrade to advisory (fail-open); advisory_ids=%v", result.AdvisoryIDs)
	}
	// arc_paths must be empty (the arc was indeterminate).
	if len(result.ArcPaths) != 0 {
		t.Errorf("null arc → arc_paths must be empty; got %v", result.ArcPaths)
	}
	// The since field must show the HEAD~32 fallback (proving the path was taken).
	if result.Since != "HEAD~32" {
		t.Errorf("no-tag fallback → since=HEAD~32; got %q", result.Since)
	}
}

// TestCheckDefer_ReleaseMode_EmptyArcDeterministic_Passes — an empty-but-
// deterministic arc (prior tag at HEAD, zero changed paths) MUST still pass.
// This pins the empty-arc-vs-null-arc distinction: empty Set = no files in
// scope = no path_touched can fire = advisory/clear; null = indeterminate =
// evaluator-error. The fix must NOT collapse empty-arc-pass into null-arc-error.
func TestCheckDefer_ReleaseMode_EmptyArcDeterministic_Passes(t *testing.T) {
	scratch, script, tasksDir := setupReleaseEvalRepo(t)
	// Create a new tag at HEAD so the release arc from v0.1.5 to HEAD is
	// EMPTY (deterministic — git ran successfully, zero changed paths) but
	// NOT null. changedPathsSince returns an empty Set (truthy), so the null
	// check does not fire.
	cmd := exec.Command("git", "-C", scratch, "tag", "v0.1.5")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag v0.1.5: %v\n%s", err, out)
	}
	// A path_touched card: fileA.go is NOT in the empty arc → unfired → advisory.
	writeReleaseCard(t, tasksDir, "defer-emptyarc.json", "defer-emptyarc",
		"draft", releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-07-01"))
	exitCode, result, _, _ := runReleaseEval(t, script, tasksDir, "--since", "v0.1.5")
	if exitCode != 0 {
		t.Fatalf("empty deterministic arc must PASS (exit 0); got %d", exitCode)
	}
	if result.Classification != "advisory" {
		t.Errorf("empty arc + unfired → advisory (exit 0); got %q (exit %d)",
			result.Classification, exitCode)
	}
	// arc_paths must be empty (deterministic, zero changed paths).
	if len(result.ArcPaths) != 0 {
		t.Errorf("empty arc → arc_paths must be empty; got %v", result.ArcPaths)
	}
	// The candidate must be advisory (unfired), NOT evaluator-error.
	if !containsStr(result.AdvisoryIDs, "defer-emptyarc") {
		t.Errorf("empty arc + unfired → advisory_ids must contain defer-emptyarc; got %v", result.AdvisoryIDs)
	}
	if containsStr(result.EvaluatorErrIDs, "defer-emptyarc") {
		t.Errorf("empty arc must NOT be evaluator-error; got %v", result.EvaluatorErrIDs)
	}
}
