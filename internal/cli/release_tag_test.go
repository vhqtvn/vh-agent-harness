package cli

// Release-tag wrapper tests for the DEFER enforcement gate.
//
// scripts/release-tag.sh is the sanctioned release-tag wrapper (PROJECT-LOCAL,
// not templates/core). After Slice 2 it invokes the deterministic release-DEFER
// evaluator (.opencode/scripts/check-defer-triggers.js --mode=release) BEFORE
// the `git tag -a` mutation. On blocker or evaluator-error classification the
// wrapper REFUSES: emits exactly one JSON object with the 5 established fields
// (ok, tag, commit, pushed, error), returns nonzero, and never reaches the
// push path.
//
// These tests black-box the wrapper inside a fully-isolated temp git repo with
// a controlled task-card landscape, mirroring the hermetic pattern of
// TestCommitGate_BacklogSplitPreflight and TestCheckDefer_ReleaseMode_*. The
// evaluator template is copied from templates/core/ and the {{COORDINATOR_DIR}}
// token is rendered to "coordinator" (what `vh-agent-harness update` produces),
// so the tests do NOT depend on `make update` having run.

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

// setupReleaseTagRepo creates an isolated scratch git repo with:
//   - scripts/release-tag.sh copied from the repo source (project-local)
//   - .opencode/scripts/check-defer-triggers.js copied from the TEMPLATE with
//     {{COORDINATOR_DIR}} rendered to "coordinator" (simulating the rendered
//     artifact that `vh-agent-harness update` produces)
//   - .local/coordinator/tasks/ ready for test-written cards
//   - a prior tag v0.1.0 + post-tag changes to fileA.go and dir/fileC.go
//     (so the release arc v0.1.0..HEAD contains exactly those two paths;
//     fileB.go is UNCHANGED and therefore NOT in the arc)
//   - a valid annotated-tag message file at <scratch>/msg.txt
//
// Returns (scratch, wrapperPath, tasksDir, msgFile).
func setupReleaseTagRepo(t *testing.T) (scratch, wrapper, tasksDir, msgFile string) {
	t.Helper()
	for _, bin := range []string{"bash", "git", "node"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH: %v", bin, err)
		}
	}
	root := findModuleRoot(t)

	// Copy the project-local wrapper (source = shipped).
	wrapperSrc := filepath.Join(root, "scripts", "release-tag.sh")
	wrapperBody, err := os.ReadFile(wrapperSrc)
	if err != nil {
		t.Fatalf("read wrapper source %s: %v", wrapperSrc, err)
	}
	// Copy the TEMPLATE evaluator and render the coordinator token (what
	// `vh-agent-harness update` produces for .opencode/scripts/).
	evalSrc := filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.js")
	evalBody, err := os.ReadFile(evalSrc)
	if err != nil {
		t.Fatalf("read evaluator template %s: %v", evalSrc, err)
	}
	renderedEval := strings.ReplaceAll(string(evalBody), "{{COORDINATOR_DIR}}", "coordinator")

	scratch = t.TempDir()
	// Write wrapper at <scratch>/scripts/release-tag.sh.
	wrapper = filepath.Join(scratch, "scripts", "release-tag.sh")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(wrapper, wrapperBody, 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	// Write rendered evaluator at <scratch>/.opencode/scripts/.
	evalDst := filepath.Join(scratch, ".opencode", "scripts", "check-defer-triggers.js")
	if err := os.MkdirAll(filepath.Dir(evalDst), 0o755); err != nil {
		t.Fatalf("mkdir .opencode/scripts: %v", err)
	}
	if err := os.WriteFile(evalDst, []byte(renderedEval), 0o644); err != nil {
		t.Fatalf("write evaluator: %v", err)
	}
	// Create .local/coordinator/tasks/ (the rendered default tasks dir).
	tasksDir = filepath.Join(scratch, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks dir: %v", err)
	}

	// git init + identity + initial commit + tag v0.1.0 + post-tag changes.
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

	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(scratch, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	writeFile("fileA.go", "package main\n")
	writeFile("fileB.go", "package main\n")
	writeFile("dir/fileC.go", "package dir\n")
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	git("tag", "v0.1.0")
	// Post-tag changes (in the arc v0.1.0..HEAD): fileA.go + dir/fileC.go.
	writeFile("fileA.go", "package main\n// changed in arc\n")
	writeFile("dir/fileC.go", "package dir\n// changed in arc\n")
	git("add", "-A")
	git("commit", "-q", "-m", "changes for release")

	// Annotated-tag message file.
	msgFile = filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	return scratch, wrapper, tasksDir, msgFile
}

// releaseTagResult is the parsed JSON the wrapper emits. Pointer types for
// nullable fields (tag, commit, error) so null round-trips correctly.
type releaseTagResult struct {
	OK     bool    `json:"ok"`
	Tag    *string `json:"tag"`
	Commit *string `json:"commit"`
	Pushed bool    `json:"pushed"`
	Error  *string `json:"error"`
}

// runReleaseTag invokes scripts/release-tag.sh with the given version and
// optional env overrides. Returns (exitCode, parsedResult, stdout, stderr).
// RELEASE_TAG_MESSAGE_FILE defaults to msgFile unless the caller overrides it.
func runReleaseTag(t *testing.T, wrapper, msgFile, version string, env map[string]string) (int, releaseTagResult, string, string) {
	t.Helper()
	cmd := exec.Command("bash", wrapper, version)
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper)) // <scratch>
	cmd.Env = os.Environ()
	hasMsgFile := false
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
		if k == "RELEASE_TAG_MESSAGE_FILE" {
			hasMsgFile = true
		}
	}
	if !hasMsgFile {
		cmd.Env = append(cmd.Env, "RELEASE_TAG_MESSAGE_FILE="+msgFile)
	}
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
			t.Fatalf("bash spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	stdout := outb.String()
	var result releaseTagResult
	if stdout != "" {
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("wrapper output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
				exitCode, err, stdout, errb.String())
		}
	}
	return exitCode, result, stdout, errb.String()
}

// tagExists checks whether the given tag exists in the scratch repo.
func tagExists(t *testing.T, scratch, tag string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", scratch, "tag", "-l", tag)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	return strings.TrimSpace(string(out)) == tag
}

// =============================================================================
// HAPPY PATHS — the tag MUST be created.
// =============================================================================

func TestReleaseTag_NoReviewDeferCandidates_AllowsTag(t *testing.T) {
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)
	// tasks dir exists but is empty — no candidates.
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("no DEFER candidates must allow the tag (exit 0); got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must exist after a successful release")
	}
}

func TestReleaseTag_TasksDirAbsent_AllowsTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Remove the tasks dir entirely to simulate absence (operator-confirmed
	// policy: absence is not a mandatory surface — local candidates are
	// transport, not truth).
	if err := os.RemoveAll(tasksDir); err != nil {
		t.Fatalf("remove tasks dir: %v", err)
	}
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("absent tasks dir must allow the tag (exit 0); got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must exist")
	}
}

func TestReleaseTag_UnfiredCandidate_AllowsTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Unfired: path_touched references fileB.go which is NOT in the release arc.
	writeReleaseCard(t, tasksDir, "card1.json", "T-unfired", "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileB.go)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("unfired DEFER must NOT block (advisory only); got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must exist after a successful release")
	}
}

func TestReleaseTag_FiredUnrelated_AllowsTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// after_tag references a tag that does NOT exist → unfired → advisory.
	writeReleaseCard(t, tasksDir, "card1.json", "T-aftertag-nofired", "draft",
		releaseCardNotes("source:review-defer", "after_tag(v9.9.9)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("after_tag not fired must NOT block; got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must exist after a successful release")
	}
}

func TestReleaseTag_ResolvedCompleted_AllowsTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Fired (fileA.go IS in the arc) but completed → resolved → clear.
	writeReleaseCard(t, tasksDir, "card1.json", "T-completed", "completed",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("completed DEFER must NOT block; got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must exist after a successful release")
	}
}

func TestReleaseTag_ResolvedCancelled_AllowsTag(t *testing.T) {
	_, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	writeReleaseCard(t, tasksDir, "card1.json", "T-cancelled", "cancelled",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("cancelled DEFER must NOT block; got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
}

func TestReleaseTag_P2FollowupExcluded_AllowsTag(t *testing.T) {
	_, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// source:p2-followup is EXCLUDED from v1 release blocking even if fired
	// and unresolved.
	writeReleaseCard(t, tasksDir, "card1.json", "T-p2", "draft",
		releaseCardNotes("source:p2-followup", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("source:p2-followup must be excluded; got exit %d", exitCode)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
}

// =============================================================================
// REFUSAL PATHS — the tag MUST NOT be created; nonzero exit; push path NOT reached.
// =============================================================================

func TestReleaseTag_FiredUnresolvedReleaseRelevant_BlocksTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// path_touched(fileA.go) FIRES (fileA.go is in the arc) + unresolved → BLOCKER.
	writeReleaseCard(t, tasksDir, "card1.json", "T-blocker", "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, result, stdout, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("fired unresolved release-relevant DEFER must REFUSE (nonzero); got exit 0\n%s", stdout)
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after a refusal")
	}
	if result.Pushed {
		t.Errorf("pushed must be false after a DEFER refusal (push path not reached)")
	}
	// The error field must carry the classification + blocking task ID.
	if result.Error == nil {
		t.Fatalf("error must not be nil on refusal")
	}
	if !strings.Contains(*result.Error, "blocker") {
		t.Errorf("error must mention blocker classification; got %q", *result.Error)
	}
	if !strings.Contains(*result.Error, "T-blocker") {
		t.Errorf("error must carry the blocking task ID T-blocker; got %q", *result.Error)
	}
}

func TestReleaseTag_FiredDeepPath_BlocksTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// dir/fileC.go is in the arc (deep path). Exact match must fire.
	writeReleaseCard(t, tasksDir, "card1.json", "T-deep", "draft",
		releaseCardNotes("source:review-defer", "path_touched(dir/fileC.go)", "2026-01-01"))
	exitCode, _, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("fired deep-path DEFER must REFUSE; got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
}

func TestReleaseTag_AfterTagFired_BlocksTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// after_tag(v0.1.0) fires (v0.1.0 exists) + unresolved → BLOCKER.
	writeReleaseCard(t, tasksDir, "card1.json", "T-aftertag", "draft",
		releaseCardNotes("source:review-defer", "after_tag(v0.1.0)", "2026-01-01"))
	exitCode, _, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("fired after_tag DEFER must REFUSE; got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
}

func TestReleaseTag_UnknownStatus_Refuses(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	writeReleaseCard(t, tasksDir, "card1.json", "T-unknown", "bogus-status",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("unknown status must REFUSE (evaluator-error); got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after evaluator-error refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "evaluator-error") {
		t.Errorf("error must mention evaluator-error; got %v", result.Error)
	}
}

func TestReleaseTag_UnsupportedGrammar_Refuses(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Mirrors the real defer-p1-lineage-002-001 card: `||`-separated predicates
	// with a directory operand and a non-predicate term. The strict evaluator
	// MUST reject this as evaluator-error (never silently unfired).
	writeReleaseCard(t, tasksDir, "card1.json", "T-grammar", "draft",
		releaseCardNotes("source:review-defer",
			"path_touched(internal/foo/)||path_touched(internal/bar.go)||feature:prune", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("unsupported grammar must REFUSE (evaluator-error); got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after evaluator-error refusal")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "evaluator-error") {
		t.Errorf("error must mention evaluator-error; got %v", result.Error)
	}
}

func TestReleaseTag_MalformedJSON_Refuses(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	full := filepath.Join(tasksDir, "card1.json")
	if err := os.WriteFile(full, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write malformed card: %v", err)
	}
	exitCode, _, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("malformed card JSON must REFUSE (evaluator-error); got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after evaluator-error refusal")
	}
}

func TestReleaseTag_RefusalDoesNotReachPushPath(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Set up a bare remote so we can detect whether `git push` was attempted.
	remote := filepath.Join(t.TempDir(), "remote.git")
	rcmd := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	rcmd("init", "--bare", "-q", remote)
	rcmd("-C", scratch, "remote", "add", "origin", remote)

	// Fired-unresolved-release-relevant → BLOCKER.
	writeReleaseCard(t, tasksDir, "card1.json", "T-block", "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))

	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0",
		map[string]string{"RELEASE_TAG_PUSH": "1"})
	if exitCode == 0 {
		t.Fatalf("blocker must REFUSE even with push enabled; got exit 0")
	}
	if result.Pushed {
		t.Errorf("pushed must be false (push path not reached)")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist locally after refusal")
	}
	// The bare remote must have ZERO tags — proving the push path never ran.
	out, err := exec.Command("git", "-C", remote, "tag", "-l").Output()
	if err != nil {
		t.Fatalf("git -C remote tag -l: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("remote must have NO tags (push path not reached); got %q", string(out))
	}
}

// TestReleaseTag_NoPriorTag_FailClosed_RefusesTag — the end-to-end proof that
// the fail-open defect (tier1_b/F1) is closed at the wrapper level. In a repo
// with NO prior tag, the wrapper invokes the evaluator with --mode=release only
// (no --since). The evaluator falls back to HEAD~32, isSafeRef rejects `~`,
// changedPathsSince returns null, and (after the fix) the evaluator classifies
// this as evaluator-error (exit 2) BEFORE candidate evaluation. The wrapper
// sees the nonzero exit and REFUSES: emits the 5-field JSON with ok=false,
// returns nonzero, does NOT create the tag, and does NOT reach the push path.
//
// Before the fix, the fired path_touched candidate was silently downgraded to
// advisory (exit 0) and the wrapper proceeded to `git tag -a` — the worst
// defect class for a release gate.
func TestReleaseTag_NoPriorTag_FailClosed_RefusesTag(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Delete the prior tag to simulate the no-prior-tag fail-open scenario.
	// The wrapper will invoke the evaluator with --mode=release only (no
	// --since), triggering the HEAD~32 fallback → null arc → evaluator-error.
	cmd := exec.Command("git", "-C", scratch, "tag", "-d", "v0.1.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git tag -d v0.1.0: %v\n%s", err, out)
	}
	// Set up a bare remote so we can prove the push path is never reached.
	remote := filepath.Join(t.TempDir(), "remote.git")
	rcmd := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	rcmd("init", "--bare", "-q", remote)
	rcmd("-C", scratch, "remote", "add", "origin", remote)

	// A fired DEFER card: fileA.go exists in the repo. Before the fix the null
	// arc downgraded this to advisory (no-git-diff-data); after the fix the
	// null arc short-circuits to evaluator-error before this is evaluated.
	writeReleaseCard(t, tasksDir, "card1.json", "T-failopen", "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))

	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0",
		map[string]string{"RELEASE_TAG_PUSH": "1"})
	if exitCode == 0 {
		t.Fatalf("no-prior-tag fail-open path must REFUSE (nonzero); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false on evaluator-error refusal")
	}
	// The tag MUST NOT be created — the gate closed before `git tag -a`.
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT exist after no-prior-tag refusal")
	}
	// Push MUST NOT be reached.
	if result.Pushed {
		t.Errorf("pushed must be false (push path not reached)")
	}
	// The bare remote must have ZERO tags — proving the push path never ran.
	out, err := exec.Command("git", "-C", remote, "tag", "-l").Output()
	if err != nil {
		t.Fatalf("git -C remote tag -l: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("remote must have NO tags (push path not reached); got %q", string(out))
	}
	// The error field must carry the evaluator-error classification.
	if result.Error == nil {
		t.Fatalf("error must not be nil on refusal")
	}
	if !strings.Contains(*result.Error, "evaluator-error") {
		t.Errorf("error must mention evaluator-error classification; got %q", *result.Error)
	}
}

// =============================================================================
// ORDERING / ESCAPING / DETERMINISM
// =============================================================================

func TestReleaseTag_MultipleBlockers_DeterministicOrder(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// Three blockers with IDs that must appear sorted in the error message.
	writeReleaseCard(t, tasksDir, "c-zeta.json", "T-zeta", "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	writeReleaseCard(t, tasksDir, "c-alpha.json", "T-alpha", "draft",
		releaseCardNotes("source:review-defer", "path_touched(dir/fileC.go)", "2026-01-01"))
	writeReleaseCard(t, tasksDir, "c-mike.json", "T-mike", "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("multiple blockers must REFUSE; got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
	if result.Error == nil {
		t.Fatalf("error must not be nil")
	}
	errStr := *result.Error
	alphaIdx := strings.Index(errStr, "T-alpha")
	mikeIdx := strings.Index(errStr, "T-mike")
	zetaIdx := strings.Index(errStr, "T-zeta")
	if alphaIdx < 0 || mikeIdx < 0 || zetaIdx < 0 {
		t.Fatalf("error must list all three blocking IDs; got %q", errStr)
	}
	if !(alphaIdx < mikeIdx && mikeIdx < zetaIdx) {
		t.Errorf("blocking IDs must appear in sorted order (T-alpha < T-mike < T-zeta); got %q", errStr)
	}
}

func TestReleaseTag_EscapableCharsInID_ValidJSON(t *testing.T) {
	scratch, wrapper, tasksDir, msgFile := setupReleaseTagRepo(t)
	// ID with double-quote and backslash — must be safely escaped in the JSON.
	writeReleaseCard(t, tasksDir, "card1.json", `T-q"and\b`, "draft",
		releaseCardNotes("source:review-defer", "path_touched(fileA.go)", "2026-01-01"))
	exitCode, _, stdout, _ := runReleaseTag(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("blocker with escapable ID must REFUSE; got exit 0")
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT exist after refusal")
	}
	// runReleaseTag already parsed stdout as JSON — proving structural validity.
	// Re-parse into a raw map to confirm beyond the typed struct.
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("re-parse failed (escaping broken?): %v\nstdout:\n%s", err, stdout)
	}
	errField, ok := raw["error"]
	if !ok {
		t.Fatalf("error field must be present; got %v", raw)
	}
	errStr, ok := errField.(string)
	if !ok {
		t.Fatalf("error field must be a string; got %T", errField)
	}
	if !strings.Contains(errStr, `T-q"and\b`) {
		t.Errorf("error must carry the raw ID characters; got %q", errStr)
	}
}

// =============================================================================
// NON-DEFER VALIDATION INTACT (regression guard)
// =============================================================================

func TestReleaseTag_ExistingValidationIntact(t *testing.T) {
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)

	t.Run("missing_version", func(t *testing.T) {
		exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "", nil)
		if exitCode == 0 {
			t.Fatalf("missing version must REFUSE; got exit 0")
		}
		if result.OK {
			t.Errorf("expected ok=false")
		}
	})

	t.Run("bad_semver", func(t *testing.T) {
		exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "not-a-version", nil)
		if exitCode == 0 {
			t.Fatalf("bad semver must REFUSE; got exit 0")
		}
		if result.OK {
			t.Errorf("expected ok=false")
		}
		if result.Tag == nil || *result.Tag != "not-a-version" {
			t.Errorf("bad-semver refusal must echo the version in tag; got %v", result.Tag)
		}
	})

	t.Run("tag_already_exists", func(t *testing.T) {
		// v0.1.0 already exists in the scratch repo.
		_ = scratch
		exitCode, result, _, _ := runReleaseTag(t, wrapper, msgFile, "v0.1.0", nil)
		if exitCode == 0 {
			t.Fatalf("pre-existing tag must REFUSE; got exit 0")
		}
		if result.OK {
			t.Errorf("expected ok=false")
		}
	})
}
