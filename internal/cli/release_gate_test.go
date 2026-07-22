package cli

// release_gate_test.go — tests for the §4.3 generic defer-liveness release gate.
//
// These tests prove the gate's contract (see release_gate.go):
//   - FAIL on a constructed contradiction (open errata card OR open defer card
//     referencing an existing migration note) — the release blocker.
//   - PASS when clean (closed cards, or open cards that target no present note).
//   - SKIP when either side is absent (no tasks dir, no migration notes, no git).
//   - the errata SUBSET: an open errata card fails and a staged errata card
//     passes (the exact behavior of the former erratum_gate_test.go, now proven
//     as a fixture of this generic gate rather than a parallel mechanism).
//   - LIVE: the real repo (the actual release blocker) is clean today.
//
// symptom_signature stability is parked; these tests key cards by task_id only.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTaskCard writes a coordinator task card body under
// <dir>/.local/coordinator/tasks/<name>.json. body may be either a raw JSON
// string or a value marshaled by the caller.
func writeTaskCard(t *testing.T, dir, name, body string) {
	t.Helper()
	d := filepath.Join(dir, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write card %s: %v", name, err)
	}
}

// writeTaskCardObj marshals a deferLivenessCard-shaped map to a full card file.
// Kept minimal: only the fields the gate reads are required; the coordinator
// card schema has many more fields (owner_notes, history, …) the gate ignores.
func writeTaskCardObj(t *testing.T, dir, name, taskID, title, status string, filesInScope, roughScope []string) {
	t.Helper()
	obj := map[string]any{
		"schema_version": 1,
		"task_id":        taskID,
		"title":          title,
		"status":         status,
		"files_in_scope": filesInScope,
		"rough_scope":    roughScope,
	}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal card %s: %v", name, err)
	}
	writeTaskCard(t, dir, name, string(raw))
}

// writeMigrationNote writes an empty migration note body at
// <dir>/templates/migrations/<version>.md.
func writeMigrationNote(t *testing.T, dir, version string) {
	t.Helper()
	d := filepath.Join(dir, "templates", "migrations")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, version+".md"), []byte("# "+version+"\n"), 0o644); err != nil {
		t.Fatalf("write note %s: %v", version, err)
	}
}

// gitTag creates a lightweight git tag <version> in dir so the note is
// classified released=true (immutable shipped artifact).
func gitTag(t *testing.T, dir, version string) {
	t.Helper()
	if err := exec.Command("git", "-C", dir, "tag", version).Run(); err != nil {
		t.Fatalf("git tag %s: %v", version, err)
	}
}

// gitCommitStub writes a placeholder file and commits it so a subsequent
// `git tag` has a HEAD to point at (a lightweight tag without commits fails on
// most git versions). Only the Tests that exercise released-vs-about-to-release
// classification need this; the rest rely on note EXISTENCE alone, which does
// not require any commit.
func gitCommitStub(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".stub"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	exec.Command("git", "-C", dir, "config", "user.email", "t@t").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "t").Run()
	if err := exec.Command("git", "-C", dir, "add", "-A").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

// --- errata subset (the former erratum_gate_test.go, now a fixture) ---

// TestDeferLivenessGate_FailOnOpenErrataCard: an OPEN errata card alongside a
// present migration note → FAIL. This is the exact release blocker the former
// erratum gate enforced (draft errata card → fail), now exercised as a subset
// of the generic gate rather than a separate mechanism.
func TestDeferLivenessGate_FailOnOpenErrataCard(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.12.0")
	writeTaskCardObj(t, dir, "errata-v0120-fake-claim.json",
		"errata-v0120-fake-claim", "Erratum: false claim shipped in v0.12.0", "draft",
		nil, []string{"Add erratum to templates/migrations/v0.13.0.md"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL for open errata card, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "errata-v0120-fake-claim") {
		t.Errorf("FAIL should name the errata card; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "errata card") {
		t.Errorf("FAIL should flag the errata-subset nature; got %q", r.detail)
	}
}

// TestDeferLivenessGate_PassWhenErrataStaged: a STAGED errata card (correction
// queued for next release) alongside a present note → PASS. Mirrors the former
// erratum gate passing on a non-draft errata card; "staged" is in the closed set.
func TestDeferLivenessGate_PassWhenErrataStaged(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.12.0")
	writeTaskCardObj(t, dir, "errata-v0120-fake-claim.json",
		"errata-v0120-fake-claim", "Erratum: false claim shipped in v0.12.0", "staged",
		nil, []string{"Add erratum to templates/migrations/v0.13.0.md"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS for staged errata card, got %s: %s", r.tier, r.detail)
	}
}

// --- generic defer-card generalization ---

// TestDeferLivenessGate_FailOnOpenDeferCard: an OPEN defer card whose
// files_in_scope references a migration note that EXISTS on disk → FAIL. This is
// the generalization beyond the errata subset: a generic open defer contradicting
// a released/about-to-release claim.
func TestDeferLivenessGate_FailOnOpenDeferCard(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.2.0")
	writeTaskCardObj(t, dir, "defer-v020-watchout.json",
		"defer-v020-watchout", "Defer: supersede v0.2.0 migration watchout", "draft",
		[]string{"templates/migrations/v0.2.0.md"}, nil)

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL for open defer card targeting existing note, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "defer-v020-watchout") {
		t.Errorf("FAIL should name the defer card; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "v0.2.0") {
		t.Errorf("FAIL should name the referenced note version; got %q", r.detail)
	}
}

// TestDeferLivenessGate_FailOnDeferCardViaRoughScope: same as above but the note
// reference lives in rough_scope (free-form prose), not files_in_scope. Proves
// the regex scan catches prose references, not just structured file lists.
func TestDeferLivenessGate_FailOnDeferCardViaRoughScope(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.14.0")
	writeTaskCardObj(t, dir, "defer-release-docdrift.json",
		"defer-release-docdrift", "Defer: doc drift in release notes", "ready",
		nil, []string{"Correct the claim in templates/migrations/v0.14.0.md before next cut"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL for open defer card referencing note via rough_scope, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferLivenessGate_PassWhenDeferTargetsAbsentNote: an OPEN defer card that
// references a migration note which does NOT exist on disk → PASS. The card is
// open but its contradicted claim is not present in any shipped/about-to-ship
// note, so there is no release-blocking contradiction.
func TestDeferLivenessGate_PassWhenDeferTargetsAbsentNote(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Only v0.1.0 exists; the card targets v0.9.9 (absent).
	writeMigrationNote(t, dir, "v0.1.0")
	writeTaskCardObj(t, dir, "defer-future.json",
		"defer-future", "Defer: something in a not-yet-existing note", "draft",
		[]string{"templates/migrations/v0.9.9.md"}, nil)

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS for open defer targeting an absent note, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferLivenessGate_PassWhenDeferTargetsNoNote: an OPEN defer card that
// references no migration note at all (e.g. a pure code-level defer) → PASS.
func TestDeferLivenessGate_PassWhenDeferTargetsNoNote(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	writeTaskCardObj(t, dir, "defer-code-only.json",
		"defer-code-only", "Defer: internal substrate refactor", "draft",
		[]string{"internal/substrate/renderer.go"}, nil)

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS for open defer targeting no note, got %s: %s", r.tier, r.detail)
	}
}

// --- closed-state behavior ---

// TestDeferLivenessGate_PassWhenAllCardsClosed: completed / cancelled / staged
// cards (including one targeting an existing note) → PASS. The closed set is
// honored for every member.
func TestDeferLivenessGate_PassWhenAllCardsClosed(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.2.0")
	for _, st := range []string{"completed", "cancelled", "staged"} {
		writeTaskCardObj(t, dir, "defer-closed-"+st+".json",
			"defer-closed-"+st, "Defer: closed card ("+st+")", st,
			[]string{"templates/migrations/v0.2.0.md"}, nil)
	}
	// Also a closed errata card targeting the note.
	writeTaskCardObj(t, dir, "errata-closed.json",
		"errata-closed", "Erratum: resolved", "completed", nil,
		[]string{"templates/migrations/v0.2.0.md"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS when all cards closed, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferLivenessGate_ReportsReleasedVsAboutToRelease: a released note (git
// tag present) vs an about-to-release note (no tag) both surface in the PASS
// detail counts, exercising the git-tag classification seam.
func TestDeferLivenessGate_ReportsReleasedVsAboutToRelease(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	writeMigrationNote(t, dir, "v0.2.0")
	gitCommitStub(t, dir)
	gitTag(t, dir, "v0.1.0") // released
	// v0.2.0 has no tag → about-to-release
	// One OPEN defer card that does NOT target a present note, so the gate PASSES.
	writeTaskCardObj(t, dir, "defer-unrelated.json",
		"defer-unrelated", "Defer: code-only", "draft",
		[]string{"internal/foo.go"}, nil)

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "1 released") {
		t.Errorf("PASS detail should count 1 released note; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "1 about-to-release") {
		t.Errorf("PASS detail should count 1 about-to-release note; got %q", r.detail)
	}
}

// --- SKIP paths (the gate cannot run honestly) ---

// TestDeferLivenessGate_SkipWhenNoTasksDir: no .local/coordinator/tasks/ → SKIP.
func TestDeferLivenessGate_SkipWhenNoTasksDir(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")

	r := checkDeferLiveness(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP when tasks dir absent, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferLivenessGate_SkipWhenNoNotes: tasks present but no migration notes →
// SKIP (no Side B to contradict).
func TestDeferLivenessGate_SkipWhenNoNotes(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeTaskCardObj(t, dir, "errata-v0120.json",
		"errata-v0120", "Erratum", "draft", nil, nil)

	r := checkDeferLiveness(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP when no notes present, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferLivenessGate_SkipWhenNotGitTree: target is not a git work tree → SKIP.
func TestDeferLivenessGate_SkipWhenNotGitTree(t *testing.T) {
	dir := t.TempDir() // no git init
	writeMigrationNote(t, dir, "v0.1.0")
	writeTaskCardObj(t, dir, "errata-v0120.json",
		"errata-v0120", "Erratum", "draft", nil, nil)

	r := checkDeferLiveness(dir)
	if r.tier != tierSkip {
		t.Fatalf("want SKIP when not a git work tree, got %s: %s", r.tier, r.detail)
	}
}

// --- FAIL-CLOSED on malformed/unreadable cards ---

// TestDeferLivenessGate_FailOnMalformedCardBeforeContradiction is the exact
// regression for the fail-open path the gate once had. A lexically-earlier
// malformed defer card (defer-00-...) is scanned BEFORE a later valid open
// contradiction (defer-01-...). The gate must NOT SKIP on the malformed card
// and thereby miss the contradiction; it FAILs (fail-closed) and names the
// offending card. os.ReadDir yields filename order, so defer-00 precedes
// defer-01 in the scan.
func TestDeferLivenessGate_FailOnMalformedCardBeforeContradiction(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.2.0")
	// Malformed card, lexically first.
	writeTaskCard(t, dir, "defer-00-malformed.json", "{not valid json")
	// Valid open contradiction, lexically second — the card the old gate would
	// have skipped past.
	writeTaskCardObj(t, dir, "defer-01-claim.json",
		"defer-01-claim", "Defer: open contradiction in v0.2.0", "draft",
		[]string{"templates/migrations/v0.2.0.md"}, nil)

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (fail-closed: malformed card before a valid contradiction), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "defer-00-malformed") {
		t.Errorf("FAIL should name the malformed card; got %q", r.detail)
	}
}

// TestDeferLivenessGate_FailOnMalformedCardAlone: a single unreadable defer card
// with no other contradiction → still FAIL. An unparseable card cannot be
// classified open-or-closed, so the gate treats it as a hard blocker rather than
// SKIP, even when no open contradiction accompanies it.
func TestDeferLivenessGate_FailOnMalformedCardAlone(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	writeTaskCard(t, dir, "defer-broken.json", "{broken")

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (fail-closed on unparseable card), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "defer-broken") {
		t.Errorf("FAIL should name the malformed card; got %q", r.detail)
	}
}

// TestDeferLivenessGate_FailOnMultiReferenceSecondNote proves the AG4 fix: a
// single scope string naming TWO migration notes — v0.1.0.md (absent on disk)
// AND v0.2.0.md (present) — must be evaluated against BOTH. The old first-match
// code checked only v0.1.0, found it absent, and PASSed, masking the open
// contradiction. The gate now extracts every match and FAILs.
func TestDeferLivenessGate_FailOnMultiReferenceSecondNote(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Only v0.2.0 exists; v0.1.0 is absent.
	writeMigrationNote(t, dir, "v0.2.0")
	writeTaskCardObj(t, dir, "defer-multiref.json",
		"defer-multiref", "Defer: supersede across v0.1.0 and v0.2.0", "draft",
		nil, []string{"Fix templates/migrations/v0.1.0.md and templates/migrations/v0.2.0.md"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL (second referenced note v0.2.0 exists), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "defer-multiref") {
		t.Errorf("FAIL should name the defer card; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "v0.2.0") {
		t.Errorf("FAIL should name the existing referenced note v0.2.0; got %q", r.detail)
	}
}

// --- LIVE gate: the real release blocker over this repo ---

// TestDeferLivenessGate_LiveRepoIsClean runs the gate against the real repo
// (via the test CWD's git toplevel). This is the actual release blocker: if a
// real open defer/errata card ever contradicts a real released/about-to-release
// migration note, go test ./... fails HERE. It passes today because the only
// card targeting a released note (errata-v0120) is staged (closed), and the
// other open defers target code/internal surfaces, not migration notes.
func TestDeferLivenessGate_LiveRepoIsClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available; cannot locate repo root for live gate")
	}
	repoRoot, err := repoRootFromCwd(t)
	if err != nil {
		t.Skipf("could not locate git working-tree root: %v", err)
	}
	r := checkDeferLiveness(repoRoot)
	if r.tier == tierSkip {
		t.Skipf("live gate unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("live repo must be clean under the defer-liveness gate, got %s: %s", r.tier, r.detail)
	}
}
