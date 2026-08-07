package cli

// Shared release-mode evaluator helpers + the PROMOTER-MODE byte-identical
// guard for check-defer-triggers.mjs.
//
// The evaluator (templates/core/.opencode/scripts/check-defer-triggers.mjs) has
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
// check-defer-triggers.mjs at <scratch>/.opencode/scripts/. The repo's HEAD has:
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
	src := filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs")
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read template script %s: %v", src, err)
	}

	scratch = t.TempDir()
	// Copy script to <scratch>/.opencode/scripts/ so the script's
	// __dirname-based repoRoot() (path.resolve(__dirname, "..", ".."))
	// resolves to <scratch>, keeping the run hermetic.
	scriptPath = filepath.Join(scratch, ".opencode", "scripts", "check-defer-triggers.mjs")
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

// TestCheckDefer_PromoterMode_MalformedOrJoinTrigger guards a stealth-park
// defect in parsePredicate. The greedy regex /^path_touched\((.+)\)$/ used to
// swallow `a)||path_touched(b` for the malformed trigger
// `path_touched(a)||path_touched(b)`, classify it as a VALID path_touched with a
// garbage arg, and the card silently parked as "valid-not-met"
// (note: not-touched-since-ref) — indistinguishable from a genuine future-watch.
// `||` is not an OR operator anywhere (the real OR is `any(...)` via
// extractTriggers), so such a trigger can NEVER fire and must be visibly flagged.
//
// After the fix, parsePredicate returns {kind:"malformed"} for an arg carrying
// `||` or an unbalanced paren, and the promoter detail surfaces
// `malformed-predicate` — NOT `not-touched-since-ref`. This test also pins that
// a well-formed single `path_touched(x)` and a well-formed
// `any(path_touched(x),path_touched(y))` still parse and evaluate correctly, so
// the malformed guard does not reject legitimate triggers.
func TestCheckDefer_PromoterMode_MalformedOrJoinTrigger(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not on PATH: %v", err)
	}
	_, script, tasksDir := setupReleaseEvalRepo(t)

	// Card 1: malformed `||`-joined trigger. fileA.go IS in the release arc, so
	// the old buggy path classified this as a valid path_touched and reported
	// not-touched-since-ref (a silent park). The fix must report malformed.
	writeReleaseCard(t, tasksDir, "defer-malformed.json", "defer-malformed",
		"draft", releaseCardNotes("source:review-defer",
			"path_touched(fileA.go)||path_touched(fileB.go)", "2026-07-31"))
	// Card 2: well-formed single path_touched on a path IN the arc.
	writeReleaseCard(t, tasksDir, "defer-single.json", "defer-single",
		"draft", releaseCardNotes("source:review-defer",
			"path_touched(fileA.go)", "2026-07-31"))
	// Card 3: well-formed any() OR over one IN-arc and one OUT-of-arc path.
	writeReleaseCard(t, tasksDir, "defer-any.json", "defer-any",
		"draft", releaseCardNotes("source:review-defer",
			"any(path_touched(fileA.go),path_touched(fileB.go))", "2026-07-31"))

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
	if exitCode != 0 {
		t.Fatalf("promoter mode must exit 0 (never blocking); got %d\n%s", exitCode, stdout)
	}

	// CRUX: the malformed trigger must surface malformed-predicate, and must NOT
	// be silently parked as not-touched-since-ref (the defect).
	buggyLine := "path_touched(fileA.go)||path_touched(fileB.go) (not-touched-since-ref)"
	fixedLine := "path_touched(fileA.go)||path_touched(fileB.go) (malformed-predicate)"
	if strings.Contains(stdout, buggyLine) {
		t.Fatalf("malformed ||-joined trigger was silently parked as not-touched-since-ref "+
			"(stealth-park defect regressed). Promoter output:\n%s", stdout)
	}
	if !strings.Contains(stdout, fixedLine) {
		t.Fatalf("malformed ||-joined trigger must surface malformed-predicate; got:\n%s", stdout)
	}

	// Well-formed single path_touched must still evaluate (fileA.go is in the arc).
	if !strings.Contains(stdout, "[READY] defer-single") {
		t.Errorf("well-formed single path_touched(fileA.go) must evaluate to READY (touched); got:\n%s", stdout)
	}
	// Well-formed any() must still split + evaluate both inner predicates:
	// fileA.go touched (OR fires → READY), fileB.go cleanly not-touched.
	if !strings.Contains(stdout, "[READY] defer-any") {
		t.Errorf("well-formed any() over an in-arc path must be READY; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "path_touched(fileB.go) (not-touched-since-ref)") {
		t.Errorf("well-formed any() must evaluate the second inner predicate cleanly "+
			"(not-touched-since-ref), not malformed; got:\n%s", stdout)
	}
}

// TestCheckDefer_PromoterMode_MalformedAfterTagTrigger is the after_tag-arm
// witness for the same parsePredicate malformed guard that the path_touched
// test above exercises. parsePredicate handles after_tag identically to
// path_touched: greedy ^after_tag\((.+)\)$ capture, then the shared
// isMalformedArg helper. For an arg carrying `||` or an unbalanced paren it
// returns {kind:"malformed"}, and evaluatePredicate surfaces
// `malformed-predicate` — NOT the buggy silent-park note.
//
// NB: the silent-park note DIFFERS by arm. The path_touched arm parked as
// `not-touched-since-ref` (the garbage arg missed the Set.has lookup). The
// after_tag arm parks as `tag-missing` because tagExists's isSafeRef rejects
// the garbage arg. Either way the card silently parks as "valid-not-met"
// (indistinguishable from a genuine future-watch) instead of being visibly
// flagged malformed. This witness pins the after_tag arm so neither arm can
// regress without a test going red. It also pins that a well-formed
// after_tag(<existing-tag>) still evaluates to READY, proving the malformed
// guard does not over-reject legitimate after_tag triggers.
func TestCheckDefer_PromoterMode_MalformedAfterTagTrigger(t *testing.T) {
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not on PATH: %v", err)
	}
	_, script, tasksDir := setupReleaseEvalRepo(t)

	// Card 1: malformed `||`-joined after_tag trigger. The greedy regex would
	// capture `v0.1.0)||after_tag(v0.2.0` as the arg; without the malformed
	// guard, tagExists's isSafeRef rejects that garbage and the card silently
	// parks as tag-missing. The fix must report malformed-predicate.
	writeReleaseCard(t, tasksDir, "defer-after-malformed.json", "defer-after-malformed",
		"draft", releaseCardNotes("source:review-defer",
			"after_tag(v0.1.0)||after_tag(v0.2.0)", "2026-07-31"))
	// Card 2: well-formed after_tag whose tag EXISTS in the scratch repo
	// (setupReleaseEvalRepo creates v0.1.0). This must evaluate to READY so the
	// malformed guard is shown not to reject a legitimate after_tag.
	writeReleaseCard(t, tasksDir, "defer-after-ok.json", "defer-after-ok",
		"draft", releaseCardNotes("source:review-defer",
			"after_tag(v0.1.0)", "2026-07-31"))

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
	if exitCode != 0 {
		t.Fatalf("promoter mode must exit 0 (never blocking); got %d\n%s", exitCode, stdout)
	}

	// CRUX: the malformed after_tag trigger must surface malformed-predicate,
	// and must NOT be silently parked as tag-missing (the after_tag arm's
	// silent-park note, analogous to path_touched's not-touched-since-ref).
	buggyLine := "after_tag(v0.1.0)||after_tag(v0.2.0) (tag-missing)"
	fixedLine := "after_tag(v0.1.0)||after_tag(v0.2.0) (malformed-predicate)"
	if strings.Contains(stdout, buggyLine) {
		t.Fatalf("malformed ||-joined after_tag trigger was silently parked as tag-missing "+
			"(after_tag-arm stealth-park defect regressed). Promoter output:\n%s", stdout)
	}
	if !strings.Contains(stdout, fixedLine) {
		t.Fatalf("malformed ||-joined after_tag trigger must surface malformed-predicate; got:\n%s", stdout)
	}

	// Well-formed after_tag(<existing-tag>) must still evaluate to READY
	// (tag-exists), proving the malformed guard does not over-reject legitimate
	// after_tag triggers.
	if !strings.Contains(stdout, "[READY] defer-after-ok") {
		t.Errorf("well-formed after_tag(v0.1.0) must evaluate to READY (tag exists); got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "after_tag(v0.1.0) (tag-exists)") {
		t.Errorf("well-formed after_tag(v0.1.0) detail must show tag-exists; got:\n%s", stdout)
	}
}

// =============================================================================
// Release-prep (F4-C mechanical enumerator) three-provenance fixture
// =============================================================================
//
// RELEASE-PREP MODE (--mode=release-prep) is distinct from the release
// (manifest-authority) mode modeled by manifestResult in
// check_defer_release_manifest_test.go. Release-prep is the F4-C mechanical
// enumerator: it scans .local/<tasks>/ WITHOUT a provenance filter, finds OPEN
// defer cards whose path_touched target re-fires in the release arc, and emits
// the missing_disposition / draft_stub_records / advisory envelope. It reads
// the committed manifest's defer_id set to decide which firing cards are already
// disposed (satisfied), but it does NOT apply the disposition matrix (that is
// release mode's job).

// releasePrepResult is the parsed JSON envelope the release-prep evaluator
// emits. Only the fields asserted by the three-provenance fixture are typed.
type releasePrepResult struct {
	Mode               string                   `json:"mode"`
	Classification     string                   `json:"classification"`
	DiffSince          string                   `json:"diff_since"`
	MissingDisposition []map[string]interface{} `json:"missing_disposition"`
	Advisory           *struct {
		FiredTotal int `json:"fired_total"`
	} `json:"advisory"`
}

// runReleasePrepEval runs the evaluator in release-prep (F4-C enumerator) mode
// and returns (exitCode, parsedEnvelope). cwd is the scratch repo root derived
// from the script path so the script's __dirname-based repoRoot() resolves to
// <scratch>, keeping the run hermetic. Mirrors runReleaseEvalManifest but for
// the release-prep mode + its envelope shape.
func runReleasePrepEval(t *testing.T, script, tasksDir, since string) (int, releasePrepResult) {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node not on PATH: %v", err)
	}
	cmd := exec.Command(nodeBin, script,
		"--mode=release-prep", "--tasks", tasksDir, "--since", since)
	cmd.Dir = filepath.Dir(filepath.Dir(filepath.Dir(script))) // <scratch>
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
	var result releasePrepResult
	stdout := outb.String()
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("release-prep output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
			exitCode, err, stdout, errb.String())
	}
	return exitCode, result
}

// releasePrepMissingHasID reports whether id appears among the parsed
// missing_disposition entries' defer_id values.
func releasePrepMissingHasID(result releasePrepResult, id string) bool {
	for _, m := range result.MissingDisposition {
		if v, ok := m["defer_id"].(string); ok && v == id {
			return true
		}
	}
	return false
}

// TestCheckDefer_ReleasePrep_ThreeProvenanceDispositions proves the WIDENED
// release-prep manifest scope end-to-end across all three DEFER provenance
// classes: source:review-defer, source:external-study, and source:p2-followup.
//
// The release-prep enumerator (mainReleasePrep in check-defer-triggers.mjs)
// scans task cards WITHOUT a provenance filter — every OPEN card whose
// path_touched target re-fires in the release arc surfaces in
// missing_disposition, regardless of provenance class, and needs an explicit
// committed manifest disposition. This is the behavioral closure for the
// widened scope (every firing card gets an explicit disposition, not only
// source:review-defer-tagged ones) that P2-REL-001's option (b) widened and that
// harness_dogfood_render_test.go asserts only structurally. A wrapper-level test
// (release_tag_manifest_test.go) cannot prove the .local surfacing path because
// the wrapper ceremony never reads task cards; only this evaluator-level fixture
// exercises the actual enumerator.
//
// CRUX (behavioral closure): all three provenance classes surface in
// missing_disposition when undisposed (phase 1), then ALL THREE clear once each
// has a committed manifest record (phase 2).
func TestCheckDefer_ReleasePrep_ThreeProvenanceDispositions(t *testing.T) {
	// The three DEFER provenance classes. Each card carries the SAME firing
	// path_touched target (fileA.go, which setupReleaseEvalRepo changes since
	// v0.1.0) so the ONLY varying axis is the provenance class — proving the
	// enumerator surfaces EVERY provenance, not just review-defer.
	provenances := []struct {
		name    string
		source  string // owner_notes provenance tag
		deferID string
	}{
		{"review-defer", "source:review-defer", "fixture-review-defer"},
		{"external-study", "source:external-study", "fixture-external-study"},
		{"p2-followup", "source:p2-followup", "fixture-p2-followup"},
	}

	scratch, script, tasksDir := setupReleaseEvalRepo(t)
	for _, p := range provenances {
		notes := releaseCardNotes(p.source, "path_touched(fileA.go)", "2026-08-02")
		writeReleaseCard(t, tasksDir, p.deferID+".json", p.deferID, "draft", notes)
	}

	// Phase 1 — NO manifest. All three provenance classes must surface in
	// missing_disposition (the widened scope), so release-prep BLOCKS (exit 1).
	code1, result1 := runReleasePrepEval(t, script, tasksDir, "v0.1.0")
	if code1 != 1 {
		t.Fatalf("three undisposed firing cards must BLOCK (exit 1); got %d", code1)
	}
	if result1.Classification != "blocker" {
		t.Fatalf("phase 1 classification = %q, want blocker", result1.Classification)
	}
	firedGot := -1
	if result1.Advisory != nil {
		firedGot = result1.Advisory.FiredTotal
	}
	if firedGot != len(provenances) {
		t.Fatalf("advisory.fired_total = %d, want %d (all three provenances must fire)",
			firedGot, len(provenances))
	}
	for _, p := range provenances {
		if !releasePrepMissingHasID(result1, p.deferID) {
			t.Errorf("%s card %q must surface in missing_disposition (widened scope — "+
				"every provenance class is enumerated, not only review-defer); got %+v",
				p.name, p.deferID, result1.MissingDisposition)
		}
	}

	// Phase 2 — commit a schema-valid manifest record for each of the three
	// IDs, reusing the canonical seed-shape builder (seededNoDiscloseInvalid →
	// disposition=disclose, as the settled design mandates). NB: release-prep
	// satisfies a card by defer_id PRESENCE in the committed manifest
	// (committedManifestDeferIds reads only defer_id, not the disposition
	// value); disclose is chosen per the design, but any schema-valid
	// disposition would satisfy the enumerator. The committed defer_id set
	// satisfies each card, so release-prep classifies CLEAR and none of the
	// three remains in missing_disposition.
	records := make([]manifestRecordSpec, 0, len(provenances))
	for _, p := range provenances {
		records = append(records, seededNoDiscloseInvalid(p.deferID))
	}
	spec := manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records:             records,
	}
	manifestBytes := buildManifestBytes(t, scratch, spec)
	commitReleaseManifest(t, scratch, manifestBytes, "")

	code2, result2 := runReleasePrepEval(t, script, tasksDir, "v0.1.0")
	if code2 != 0 {
		t.Fatalf("with committed dispositions for all three provenances, release-prep "+
			"must PASS (exit 0); got %d", code2)
	}
	if result2.Classification != "clear" {
		t.Fatalf("phase 2 classification = %q, want clear", result2.Classification)
	}
	for _, p := range provenances {
		if releasePrepMissingHasID(result2, p.deferID) {
			t.Errorf("%s card %q must NOT remain in missing_disposition after the "+
				"manifest record; got %+v", p.name, p.deferID, result2.MissingDisposition)
		}
	}
}
