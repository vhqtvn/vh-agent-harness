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
//   - LIVE-repo cleanliness is NOT asserted by these tests. It is the authority
//     of doctor checks #12 (`checkDeferLiveness`) AND #13
//     (`checkStagedErrataContent`) at the G0c release gate
//     (scripts/release-tag.sh), NOT a `go test` canary. A go-test canary over
//     the real repo would duplicate the authoritative G0c check at the wrong
//     layer, turning the universal dev test red during release-prep windows
//     (#12: whenever an open defer card's path_touched target is touched by a
//     legitimate commit; #13: whenever a staged errata card is mid-flight
//     before `release inject-errata` runs). `go test ./...` stays HERMETIC
//     (scratch-repo fixtures only); the live F4-C / defer-liveness refusal
//     AND the staged-errata-content refusal both surface at doctor/G0c.
//
// symptom_signature stability is parked; these tests key cards by task_id only.

import (
	"encoding/json"
	"fmt"
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

// writeAdoptionMarker writes a canonical coordinator-adoption marker at
// <dir>/.vh-agent-harness/coordinator-adoption.json (defer-003). The committed
// marker is the opt-in that makes checkDeferLiveness authoritative. Tests that
// exercise the gate's card/contradiction logic must call this so they fall
// under matrix row 3 (tasks present + marker valid) rather than row 2 (tasks
// present + marker absent → tierSkip).
func writeAdoptionMarker(t *testing.T, dir string) {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir marker dir: %v", err)
	}
	body := `{"version":1,"adopted":true}` + "\n"
	if err := os.WriteFile(filepath.Join(d, "coordinator-adoption.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adoption marker: %v", err)
	}
}

// writeCorruptAdoptionMarker writes an unparseable marker body (matrix row 5).
func writeCorruptAdoptionMarker(t *testing.T, dir string) {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir marker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "coordinator-adoption.json"), []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt marker: %v", err)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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
	writeAdoptionMarker(t, dir)
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

// --- defer-003: coordinator-adoption state matrix (all 5 rows, test-mandated) ---
//
// The committed marker at .vh-agent-harness/coordinator-adoption.json is the
// opt-in that makes checkDeferLiveness authoritative. These five tests pin the
// exact state matrix the gate applies; row 4 is the crux (THE FIX — adopted
// transport dir lost).

// Row 1: tasks absent + marker absent → tierSkip (greenfield / never adopted).
func TestDeferLivenessGate_Matrix_GreenfieldTasksAbsentMarkerAbsent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	// No tasks dir, no marker.

	r := checkDeferLiveness(dir)
	if r.tier != tierSkip {
		t.Fatalf("row 1 (greenfield): want tierSkip, got %s: %s", r.tier, r.detail)
	}
}

// Row 2: tasks present + marker absent → tierSkip (do NOT retroactively infer
// adoption from losable transport).
func TestDeferLivenessGate_Matrix_TasksPresentMarkerAbsent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	// A stray open errata card that WOULD be a release blocker under row 3 — but
	// without the adoption marker the gate stays inactive (SKIP).
	writeTaskCardObj(t, dir, "errata-v0120.json",
		"errata-v0120", "Erratum", "draft", nil,
		[]string{"Add erratum to templates/migrations/v0.13.0.md"})

	r := checkDeferLiveness(dir)
	if r.tier != tierSkip {
		t.Fatalf("row 2 (tasks present, no marker): want tierSkip, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "not adopted") {
		t.Errorf("row 2 detail should explain the not-adopted SKIP, got %q", r.detail)
	}
}

// Row 3: tasks present + marker valid → the existing gate runs. This is the
// regression-safety proof that CardError→tierFail still holds under the new
// adoption-gated path: an unparseable card FAILs (not SKIPs) once adopted.
func TestDeferLivenessGate_Matrix_TasksPresentMarkerValid_CardErrorStillFails(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeAdoptionMarker(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	writeTaskCard(t, dir, "defer-broken.json", "{broken")

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("row 3 (adopted, malformed card): want tierFail (CardError regression-safety), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "defer-broken") {
		t.Errorf("row 3 FAIL should name the malformed card; got %q", r.detail)
	}
}

// Row 4 (THE FIX / crux): tasks absent + marker valid → tierFail. An adopted
// repo that lost its whole transport dir FAILs instead of silently SKIPping.
// This is the behavioral-closure crux path.
func TestDeferLivenessGate_Matrix_TasksAbsentMarkerValid_Fails(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeAdoptionMarker(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	// No tasks dir — the adopted transport dir is gone.

	r := checkDeferLiveness(dir)
	if r.tier != tierFail {
		t.Fatalf("row 4 (THE FIX: adopted transport lost): want tierFail, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "tasks") || !strings.Contains(r.detail, "absent") {
		t.Errorf("row 4 FAIL should explain the adopted-transport-absent failure; got %q", r.detail)
	}
}

// Row 5: marker corrupt/unreadable (either dir state) → tierFail (fail-closed).
// Exercised with the transport dir PRESENT (a card exists) so the only reason
// for the FAIL is the corrupt marker.
func TestDeferLivenessGate_Matrix_CorruptMarkerFails(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeCorruptAdoptionMarker(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	writeTaskCardObj(t, dir, "defer-clean.json",
		"defer-clean", "Defer: benign", "completed",
		[]string{"internal/foo.go"}, nil)

	r := checkDeferLiveness(dir)
	if r.tier != tierFail {
		t.Fatalf("row 5 (corrupt marker, dir present): want tierFail, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "corrupt") {
		t.Errorf("row 5 FAIL should flag the corrupt marker; got %q", r.detail)
	}
}

// Row 5 (variant): corrupt marker with the transport dir ABSENT — still FAILs
// (corrupt dominates regardless of transport state).
func TestDeferLivenessGate_Matrix_CorruptMarkerFails_TasksAbsent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeCorruptAdoptionMarker(t, dir)
	writeMigrationNote(t, dir, "v0.1.0")
	// No tasks dir.

	r := checkDeferLiveness(dir)
	if r.tier != tierFail {
		t.Fatalf("row 5 (corrupt marker, dir absent): want tierFail, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "corrupt") {
		t.Errorf("row 5 FAIL should flag the corrupt marker; got %q", r.detail)
	}
}

// --- doctor check #13: staged-errata-content (the THIRD failure mode) ---

// sampleErratumBody is a minimal corrective body an erratum file carries after
// its title + status blockquote. Used by the #13 tests as the content that must
// appear in the about-to-release note for a staged card to pass.
const sampleErratumBody = `## What was wrong

The release note falsely claimed the feature worked.

### 1. Specific defect

**Claimed:** X happens.
**Actual behavior:** Y happens.`

// writeErratumFile writes an erratum markdown file at <dir>/<relPath> with a
// standard title + status blockquote + the given corrective body. relPath is
// repo-relative (e.g. "docs/migration-errata/v0.12.0.md").
func writeErratumFile(t *testing.T, dir, relPath, version, body string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir erratum dir: %v", err)
	}
	content := fmt.Sprintf("# Erratum: %s — sample false claim\n\n> **Status:** staged for injection into the next release note.\n\n%s\n", version, body)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write erratum %s: %v", relPath, err)
	}
}

// writeStagedErrataCard writes a GENUINELY HEALTHY staged errata card: status
// "staged", a staged_path pointing at relPath (repo-relative), AND every core
// field the card-validity semantics require (task_type, coordination_mode,
// primary_lane, nonempty files_in_scope/rough_scope/success_criteria/
// validation_plan). It mirrors the real errata-v0290-*.json card shape.
//
// History: this fixture once wrote files_in_scope/rough_scope as EMPTY lists
// with no task_type/coordination_mode/primary_lane — under the staged-errata
// health semantics (defer-release-injector-degraded-eligibility) those fixtures
// are DEGRADED (a non-draft staged card needs nonempty DoR lists + core enum
// fields), so every test that means "a healthy staged card" must use this
// upgraded shape. Tests that mean "a degraded staged card" use
// writeDegradedStagedErrataCard (release_inject_test.go).
func writeStagedErrataCard(t *testing.T, dir, name, taskID, title, stagedPath string) {
	t.Helper()
	obj := map[string]any{
		"schema_version":    1,
		"task_id":           taskID,
		"title":             title,
		"status":            "staged",
		"staged_path":       stagedPath,
		"task_type":         "implementation",
		"coordination_mode": "short",
		"primary_lane":      "release",
		"files_in_scope":    []string{stagedPath},
		"rough_scope":       []string{"Staged erratum transport: the release ceremony's inject-errata step reads this card, injects the staged_path body into the next release note, and flips this card staged→completed."},
		"success_criteria":  []string{"Next release note carries an '## Errata' section with this staged corrective body injected"},
		"validation_plan":   []string{"Release ceremony inject-errata step reads this card and injects the staged_path body into the next release note"},
		"history": []any{map[string]any{
			"at":           "2026-11-07T06:38:29Z",
			"event":        "erratum_staged",
			"session_name": "erratum-stage",
			"status":       "staged",
			"note":         "Staged by execution session (test fixture mirroring the canonical errata-card shape).",
		}},
		"created_at": "2026-11-07T06:38:29Z",
		"updated_at": "2026-11-07T06:38:29Z",
	}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal card %s: %v", name, err)
	}
	writeTaskCard(t, dir, name, string(raw))
}

// writeMigrationNoteBody writes a migration note at
// <dir>/templates/migrations/<version>.md with an arbitrary body (not just the
// version title), so a test can seed a note that already contains erratum text.
func writeMigrationNoteBody(t *testing.T, dir, version, body string) {
	t.Helper()
	d := filepath.Join(dir, "templates", "migrations")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, version+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write note %s: %v", version, err)
	}
}

// TestStagedErrataContent_FailWhenContentMissing: a staged errata card whose
// correction body is ABSENT from the about-to-release (untagged) note → FAIL.
// This is the core third-failure-mode contract: the release ceremony must stop.
func TestStagedErrataContent_FailWhenContentMissing(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nNo errata here.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")

	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for staged errata whose content is missing from the note, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "errata-v0120-sample") {
		t.Errorf("FAIL should name the offending card; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "v0.15.0") {
		t.Errorf("FAIL should name the checked about-to-release note; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "inject-errata") {
		t.Errorf("FAIL should point at the recovery command; got %q", r.detail)
	}
}

// TestStagedErrataContent_PassWhenContentPresent: a staged errata card whose
// correction body IS present in the about-to-release note → PASS. Proves the
// content check is a real signature match, not card status alone.
func TestStagedErrataContent_PassWhenContentPresent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	// The note CONTAINS the erratum body under a section heading.
	noteBody := "# v0.15.0\n\n## Errata for v0.12.0\n\n" + sampleErratumBody + "\n"
	writeMigrationNoteBody(t, dir, "v0.15.0", noteBody)

	r := checkStagedErrataContent(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS for staged errata whose content is present in the note, got %s: %s", r.tier, r.detail)
	}
}

// TestStagedErrataContent_SkipWhenNoAboutToReleaseNote: a staged errata card but
// NO about-to-release (untagged) note exists → SKIP. This is the current
// steady-state of the real repo (no v0.15.0 note yet) and proves the check does
// not false-flag a repo with no imminent release.
func TestStagedErrataContent_SkipWhenNoAboutToReleaseNote(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Only a RELEASED note (tagged) exists — nothing about-to-release.
	writeMigrationNoteBody(t, dir, "v0.14.0", "# v0.14.0\n")
	gitCommitStub(t, dir)
	gitTag(t, dir, "v0.14.0")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")

	r := checkStagedErrataContent(dir)
	// Assert the positive SKIP contract directly. Do NOT intercept our own
	// expected outcome with t.Skipf — that would make the test pass trivially
	// even if the implementation returned tierSkip for the wrong reason.
	if r.tier != tierSkip {
		t.Fatalf("want SKIP (no about-to-release note), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "about-to-release") {
		t.Errorf("SKIP detail should explain why (no about-to-release note); got %q", r.detail)
	}
}

// TestStagedErrataContent_PassWhenNoStagedCards: an about-to-release note exists
// but no staged errata cards exist (all flipped to completed by inject) → PASS.
// This is the post-inject steady state.
func TestStagedErrataContent_PassWhenNoStagedCards(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n")
	// A COMPLETED errata card (not staged) — inject already flipped it.
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	// Overwrite the card status to completed (writeStagedErrataCard writes staged;
	// flip it here to simulate post-inject).
	writeTaskCardObj(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "completed",
		nil, nil)

	r := checkStagedErrataContent(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS (no staged cards), got %s: %s", r.tier, r.detail)
	}
}

// TestStagedErrataContent_FailWhenStagedPathUnreadable: a staged card whose
// staged_path points at a missing file → FAIL (fail-closed). A staged card with
// no staged erratum text is itself a release blocker.
func TestStagedErrataContent_FailWhenStagedPathUnreadable(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n")
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	// NOTE: no erratum file written — staged_path is unreadable.

	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for staged card with unreadable staged_path, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "unreadable") {
		t.Errorf("FAIL should flag the unreadable staged_path; got %q", r.detail)
	}
}

// TestStagedErrataContent_FailWhenHighestNoteLacksContent: with MULTIPLE
// about-to-release (untagged) notes, check #13 must target the SAME
// highest-version note that `release inject-errata` injects into. A staged
// erratum whose body lives ONLY in an older (lower-version) untagged note
// must FAIL — the note that will actually ship lacks the correction. This is
// the regression test for the round-2 review finding (target-note selection
// mismatch between #13 and inject-errata).
func TestStagedErrataContent_FailWhenHighestNoteLacksContent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	// Two untagged notes. v0.15.0 contains the erratum body; v0.16.0 (the
	// highest) does NOT. inject-errata would write into v0.16.0; #13 must
	// therefore check v0.16.0 and FAIL because the body is absent there.
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\n## Errata for v0.12.0\n\n"+sampleErratumBody+"\n")
	writeMigrationNoteBody(t, dir, "v0.16.0", "# v0.16.0\n\nNo errata here.\n")

	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL when highest untagged note lacks content, got %s: %s", r.tier, r.detail)
	}
	// The FAIL must name the HIGHEST note (v0.16.0), not the stale one (v0.15.0).
	if !strings.Contains(r.detail, "v0.16.0") {
		t.Errorf("FAIL should name the highest about-to-release note v0.16.0; got %q", r.detail)
	}
	if strings.Contains(r.detail, "v0.15.0") {
		t.Errorf("FAIL should NOT reference the stale lower note v0.15.0; got %q", r.detail)
	}
}

// TestStagedErrataContent_LiveRepoIsClean was REMOVED. It ran
// checkStagedErrataContent against the live repo, duplicating doctor #13 at the
// go-test layer — the same hermetic/live split that removed the #12 canaries
// (commit c588c7e) applies here. The 6 hermetic fixtures above (Fail/PASS/SKIP
// variants) cover every logic branch; the live-state signal belongs to doctor
// #13 at G0c, which can legitimately be red during the staged→inject window of
// release-prep. `go test ./...` stays hermetic.

// --- shape-invalid staged errata cards (defer-release-injector-degraded-eligibility, disposal b′) ---
//
// A staged errata card that does not carry EXACTLY the staged-errata shape
// (the key allowlist enumerated from the two real staged cards — see
// internal/taskcard/staged_validation.go) must FAIL check #13 BEFORE content
// presence is accepted — even when its corrective body IS in the note. Card
// shape and content presence are independent; the refusal names the
// offending card, its offending fields, and the recovery path (clean the
// card or sanctioned withdrawal from staging). writeShapeInvalidStagedErrataCard
// (release_inject_test.go) is the canonical shape fixture: an
// otherwise-complete staged card with an empty success_criteria list.

// TestStagedErrataContent_FailOnShapeInvalidCard_BodyAbsent: a shape-invalid
// staged card with its corrective body ABSENT from the note → FAIL naming the
// offending card + offending field + recovery guidance (the shape refusal
// fires before the content check would).
func TestStagedErrataContent_FailOnShapeInvalidCard_BodyAbsent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nNo errata here.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeShapeInvalidStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")

	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for shape-invalid staged card (body absent), got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{"errata-v0120-sample", "success_criteria", "INVALID", "clean the card", "withdraw"} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail should mention %q; got %q", want, r.detail)
		}
	}
}

// TestStagedErrataContent_FailOnShapeInvalidCard_BodyPresent: a shape-invalid
// staged card whose corrective body IS present in the note must STILL FAIL —
// content presence cannot make a shape-invalid card eligible.
func TestStagedErrataContent_FailOnShapeInvalidCard_BodyPresent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeShapeInvalidStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	noteBody := "# v0.15.0\n\n## Errata for v0.12.0\n\n" + sampleErratumBody + "\n"
	writeMigrationNoteBody(t, dir, "v0.15.0", noteBody)

	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for shape-invalid staged card even with body present, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "INVALID") || !strings.Contains(r.detail, "success_criteria") {
		t.Errorf("FAIL should name the shape-invalid card + offending fields; got %q", r.detail)
	}
}

// TestStagedErrataContent_HealthyBodyPresentPasses: after the fixture upgrade
// (writeStagedErrataCard is now the full staged-errata shape), the healthy +
// body-present case still PASSes — proving the shape gate does not
// over-reject healthy staged cards. (Also covered by PassWhenContentPresent
// above; this arm pins the pairing explicitly.)
func TestStagedErrataContent_HealthyBodyPresentPasses(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	noteBody := "# v0.15.0\n\n## Errata for v0.12.0\n\n" + sampleErratumBody + "\n"
	writeMigrationNoteBody(t, dir, "v0.15.0", noteBody)

	r := checkStagedErrataContent(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS for healthy staged card with body present, got %s: %s", r.tier, r.detail)
	}
}

// writeStagedErrataCardWithExecutionEraField writes the full staged-errata
// card shape plus ONE execution-era field (last_review) — a benign, fully
// valid review object. Under the shape contract even a VALID execution-era
// value refuses: staged errata cards never execute, so execution-era state
// has no business on one.
func writeStagedErrataCardWithExecutionEraField(t *testing.T, dir, name, taskID, title, stagedPath string) {
	t.Helper()
	obj := map[string]any{
		"schema_version":    1,
		"task_id":           taskID,
		"title":             title,
		"status":            "staged",
		"staged_path":       stagedPath,
		"task_type":         "implementation",
		"coordination_mode": "short",
		"primary_lane":      "release",
		"files_in_scope":    []string{stagedPath},
		"rough_scope":       []string{"Staged erratum transport carrying a last_review block (execution-era state)."},
		"success_criteria":  []string{"Next release note carries the staged corrective body"},
		"validation_plan":   []string{"Release ceremony inject-errata step reads this card"},
		"history": []any{map[string]any{
			"at": "2026-11-07T06:38:29Z", "event": "erratum_staged",
			"session_name": "erratum-stage", "status": "staged", "note": "n",
		}},
		"created_at": "2026-11-07T06:38:29Z",
		"updated_at": "2026-11-07T06:38:29Z",
		"last_review": map[string]any{
			"path":        ".local/coordinator/reviews/fixture-review.md",
			"reviewed_at": "2026-11-12T00:00:00Z",
			"title":       "Coordinator review",
			"status":      "reported",
		},
	}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal card %s: %v", name, err)
	}
	writeTaskCard(t, dir, name, string(raw))
}

// TestStagedErrataContent_ExecutionEraFieldRefused: the release-surface pin
// for the fail-closed core. An otherwise-HEALTHY staged card carrying a
// VALID last_review block (any execution-era field would do) is uncheckable
// under the shape contract. Check #13 must FAIL naming last_review even with
// the corrective body present, and the injector must REFUSE the batch with
// zero writes. Against the OLD dirty implementation (the JS-normalizer
// port) this card was verified HEALTHY (tmp/red-shape-probe.txt) — a
// genuine RED receipt.
func TestStagedErrataContent_ExecutionEraFieldRefused(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCardWithExecutionEraField(t, dir, "errata-v0120-exec-era.json",
		"errata-v0120-exec-era", "Erratum: sample false claim in v0.12.0",
		"docs/migration-errata/v0.12.0.md")
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\n## Errata for v0.12.0\n\n"+sampleErratumBody+"\n")

	// Check #13 FAILs on the execution-era field even though the corrective
	// body is present in the note.
	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for execution-era field on a staged card, got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{"errata-v0120-exec-era", "last_review", "unexpected field"} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail should mention %q; got %q", want, r.detail)
		}
	}

	// The injector refuses the same card with ZERO writes.
	before := snapshotTree(t, dir)
	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err == nil {
		t.Fatalf("inject-errata must refuse the execution-era-field card; got success:\n%s", out)
	}
	if !strings.Contains(out, "errata-v0120-exec-era") || !strings.Contains(out, "last_review") {
		t.Errorf("refusal must name the offending card + field; got:\n%s", out)
	}
	assertTreeUnchanged(t, dir, before)
}

// realErrataCardFixtures are byte-faithful COPIES of the two real staged
// errata cards (.local/coordinator/tasks/errata-v0290-hub-token-
// normalization.json and errata-v0290-preserved-file-skew.json) — the
// verified template the shape allowlist was enumerated from. The values are
// copied as FIXTURE DATA — tests never read .local at test time. This
// proves the shape gate passes the exact card shapes the real release
// ceremony will select.
var realErrataCardFixtures = map[string]struct {
	cardName  string
	stagedPth string
	body      string
}{
	"hub-token-normalization": {
		cardName:  "errata-v0290-hub-token-normalization.json",
		stagedPth: "docs/migration-errata/v0.29.0-hub-token-normalization.md",
		body: `{
  "schema_version": 1,
  "task_id": "errata-v0290-hub-token-normalization",
  "title": "Erratum: v0.29.0 — hub-token normalization in state-lib.js (migration notice for the next release)",
  "status": "staged",
  "staged_path": "docs/migration-errata/v0.29.0-hub-token-normalization.md",
  "files_in_scope": [
    "docs/migration-errata/v0.29.0-hub-token-normalization.md"
  ],
  "rough_scope": [
    "Staged erratum transport for the committed hub-token-normalization migration notice (state-lib.js:5554 i++ → i += 1, commit 4469f3c; grammar-only intent, hub origin-hash advance at 0b56ab6, stale-preserve consequence, safeguard-first reconciliation, customization routes, link-integrity rider). The release ceremony's inject-errata step reads this card, injects the staged_path body into the next release note as an '## Errata for v0.29.0' section, and flips this card staged→completed."
  ],
  "history": [
    {
      "at": "2026-11-11T18:16:10Z",
      "event": "erratum_staged",
      "session_name": "erratum-stage-hub-token",
      "status": "staged",
      "note": "Staged by execution session for task-2026-11-11t18-10-20 (stage migration notice + erratum for state-lib.js hub-token normalization 4469f3c ahead of next release). staged_path points at the committed notice file per the canonical errata-card schema (internal/memory/claims DeferCard.staged_path; writeStagedErrataCard test helper mirrors this shape). Committed file left untouched; forward-notice framing (change landed post-v0.29.0 at 4469f3c, ships next release) carried in the file's status blockquote."
    }
  ],
  "created_at": "2026-11-11T18:16:10Z",
  "updated_at": "2026-11-11T18:16:10Z",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "release",
  "success_criteria": [
    "Next release note carries an '## Errata for v0.29.0' section with the staged hub-token-normalization corrective body injected",
    "Card reaches completed state via the release ceremony's inject-errata step"
  ],
  "validation_plan": [
    "Release ceremony inject-errata step reads this card and injects the staged_path body into the next release note",
    "Operator confirms the errata section appears in the published release note"
  ]
}`,
	},
	"preserved-file-skew": {
		cardName:  "errata-v0290-preserved-file-skew.json",
		stagedPth: "docs/migration-errata/v0.29.0.md",
		body: `{
  "schema_version": 1,
  "task_id": "errata-v0290-preserved-file-skew",
  "title": "Erratum: v0.29.0 — preserved-file skew: update re-render and closeout-gate claims",
  "status": "staged",
  "staged_path": "docs/migration-errata/v0.29.0.md",
  "files_in_scope": [
    "docs/migration-errata/v0.29.0.md"
  ],
  "rough_scope": [
    "Staged erratum transport for the committed v0.29.0 erratum (preserved-file skew: per-file update re-render and JS-only closeout-gate claims). The release ceremony's inject-errata step reads this card, injects the staged_path corrective body into the next release note as an '## Errata for v0.29.0' section, and flips this card staged→completed."
  ],
  "history": [
    {
      "at": "2026-11-07T06:38:29Z",
      "event": "erratum_staged",
      "session_name": "erratum-stage",
      "status": "staged",
      "note": "Staged by execution session for task-2026-11-07t04-39-27 (release-ceremony inject erratum into next release note). staged_path points at the committed erratum file per the canonical errata-card schema (internal/memory/claims DeferCard.staged_path; writeStagedErrataCard test helper mirrors this shape). Committed file left untouched."
    }
  ],
  "created_at": "2026-11-07T06:38:29Z",
  "updated_at": "2026-11-07T06:38:29Z",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "release",
  "success_criteria": [
    "Next release note carries an '## Errata for v0.29.0' section with the staged preserved-file-skew corrective body injected",
    "Card reaches completed state via the release ceremony's inject-errata step"
  ],
  "validation_plan": [
    "Release ceremony inject-errata step reads this card and injects the staged_path body into the next release note",
    "Operator confirms the errata section appears in the published release note"
  ]
}`,
	},
}

// TestStagedErrataContent_RealErrataCardShapePasses: the exact SHAPE of BOTH
// real errata-v0290 cards (fixture values copied, never read from .local)
// passes the shape gate — both at check #13 and through the injector — so
// the next release ceremony is not blocked by this slice.
func TestStagedErrataContent_RealErrataCardShapePasses(t *testing.T) {
	for name, fx := range realErrataCardFixtures {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			writeErratumFile(t, dir, fx.stagedPth, "v0.29.0", sampleErratumBody)
			writeTaskCard(t, dir, fx.cardName, fx.body)
			noteBody := "# v0.30.0\n\n## Errata for v0.29.0\n\n" + sampleErratumBody + "\n"
			writeMigrationNoteBody(t, dir, "v0.30.0", noteBody)

			// Check #13 passes with the body present.
			r := checkStagedErrataContent(dir)
			if r.tier != tierPass {
				t.Fatalf("want PASS for the real errata card shape with body present, got %s: %s", r.tier, r.detail)
			}

			// The injector accepts the card too (idempotent flip path, body present).
			out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
			if err != nil {
				t.Fatalf("inject-errata should accept the real errata card shape, got %v:\n%s", err, out)
			}
			if status := readCardStatus(t, filepath.Join(dir, ".local", "coordinator", "tasks", fx.cardName)); status != "completed" {
				t.Errorf("real-shape card should flip to completed, got %q", status)
			}
		})
	}
}

// restageProducedCardFixture is the byte-faithful output of the REAL
// sanctioned producer (round-5 FIX 2): derived by actually running
// restageCoordinationTask — the rendered .opencode/scripts/state-lib.js,
// imported from a hermetic scratch under repo tmp/ (OPENCODE_LOCAL_COORDINATOR_
// ROOT isolated; scratch deleted after the run) — over a healthy ready
// errata-class card created through saveCoordinationTask. The canonical
// write merges defaultCoordinationTaskPayload (state-lib.js:992) into every
// saved card (state-lib.js:1757), so the output carries the nine
// execution-era keys and the other canonical-default keys at their
// canonical EMPTY states. Pre-FIX-2 the shape allowlist refused this card
// as unexpected fields — sanctioned-path staged cards were structurally
// un-injectable (RED receipt tmp/red-round5-fix2.txt). Twin of
// internalRestageProducedCard (internal/taskcard/staged_validation_test.go).
const restageProducedCardFixture = `{
  "schema_version": 1,
  "task_id": "errata-v0300-restage-fixture",
  "title": "Erratum: v0.30.0 — restage-produced fixture erratum (round-5 shape contract)",
  "task_type": "implementation",
  "coordination_mode": "short",
  "primary_lane": "release",
  "research_question": "",
  "source_policy": null,
  "source_allowlist": [],
  "desired_artifact_type": null,
  "target_artifact_path": null,
  "rough_scope": [
    "Fixture erratum produced by the real restageCoordinationTask for the round-5 shape contract integration test."
  ],
  "open_questions": [],
  "ready_criteria": [],
  "files_in_scope": [
    "docs/migration-errata/v0.30.0-restage-fixture.md"
  ],
  "constraints": [],
  "non_goals": [],
  "success_criteria": [
    "Gate and injector accept the real restage-produced card shape",
    "Canonical-empty execution-era keys pass; any content refuses"
  ],
  "validation_plan": [
    "go test ./internal/taskcard/ -run TestCheckStagedErrataShape (restage-fixture PASS arm)",
    "go test ./internal/cli/ -run 'TestStagedErrataContent|TestReleaseInjectErrata' (restage-fixture PASS arms)"
  ],
  "report_envelope": "minimal",
  "backlog_id": null,
  "workstream_slug": null,
  "dependencies": [],
  "owner_notes": [],
  "status": "staged",
  "session_aliases": [],
  "active_session_alias": null,
  "claimed_at": null,
  "report_paths": [],
  "review_paths": [],
  "latest_report": null,
  "f3_design_readiness": null,
  "decision_record": null,
  "next_action": "Awaiting release inject-errata: the release ceremony reads staged_path, injects the corrective body into the next release note, and flips this card to completed.",
  "predicted_impact": null,
  "measured_outcome": null,
  "last_review": null,
  "history": [
    {
      "at": "2026-11-19T06:54:44Z",
      "event": "task_created",
      "session_name": null,
      "status": "ready",
      "note": "Created local coordination task card."
    },
    {
      "at": "2026-11-19T06:54:44Z",
      "event": "task_restaged",
      "session_name": null,
      "status": "staged",
      "note": "Round-5 fixture: real restage output pinned as the committed Go test fixture."
    }
  ],
  "created_at": "2026-11-19T06:54:44Z",
  "updated_at": "2026-11-19T06:54:44Z",
  "staged_path": "docs/migration-errata/v0.30.0-restage-fixture.md"
}
`

// TestStagedErrataContent_RestageProducedCardPasses (round-5 FIX 2): a card
// produced by the REAL sanctioned restage op — canonical-empty execution-era
// keys and all — is ACCEPTED by check #13 (shape PASS through the real
// producer, not a hand-shaped exemplar). With its corrective body present in
// the about-to-release note, the check PASSes end to end.
func TestStagedErrataContent_RestageProducedCardPasses(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.30.0-restage-fixture.md", "v0.30.0", sampleErratumBody)
	writeTaskCard(t, dir, "errata-v0300-restage-fixture.json", restageProducedCardFixture)
	noteBody := "# v0.31.0\n\n## Errata for v0.30.0\n\n" + sampleErratumBody + "\n"
	writeMigrationNoteBody(t, dir, "v0.31.0", noteBody)

	r := checkStagedErrataContent(dir)
	if r.tier != tierPass {
		t.Fatalf("want PASS for the real restage-produced card with body present, got %s: %s", r.tier, r.detail)
	}
}

// TestStagedErrataContent_WhitespacePaddedStagedPathRefused (round-5 FIX 1):
// a staged_path whose TrimSpace form differs from the raw value is refused
// by the shape preflight BEFORE check #13 accepts content — both release
// consumers TrimSpace before filepath.IsAbs/Join, so " /outside/x.md" and
// kin would become out-of-repo reads at consume time. All four padded forms
// (POSIX-absolute, traversal, Windows drive-letter, UNC) FAIL the check with
// the consumed-representation refusal. RED receipt (guard temporarily
// reverted): tmp/red-round5-fix1.txt.
func TestStagedErrataContent_WhitespacePaddedStagedPathRefused(t *testing.T) {
	for _, p := range []string{
		" /outside/x.md",        // whitespace-prefixed POSIX-absolute
		" ../outside.md",        // whitespace-prefixed parent traversal
		" C:/x.md",              // whitespace-prefixed Windows drive-letter
		" \\\\srv\\share\\x.md", // whitespace-prefixed UNC
	} {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nNo errata here.\n")
			writeStagedErrataCard(t, dir, "errata-v0120-padded.json",
				"errata-v0120-padded", "Erratum: padded staged_path claim", p)

			r := checkStagedErrataContent(dir)
			if r.tier != tierFail {
				t.Fatalf("want FAIL for whitespace-padded staged_path %q, got %s: %s", p, r.tier, r.detail)
			}
			for _, want := range []string{"errata-v0120-padded", "staged_path", "whitespace-padded", "consumed representation must equal the stored one"} {
				if !strings.Contains(r.detail, want) {
					t.Errorf("FAIL detail should mention %q; got %q", want, r.detail)
				}
			}
		})
	}
}

// TestStagedErrataContent_FailOnRegistryUndecodableErrataCard (C-F1 gate
// arm): a staged errata card the CLAIM REGISTRY cannot decode
// (files_in_scope object → reg.CardErrors, never in reg.Cards) coexists with
// a healthy staged card whose corrective body IS present in the note.
// Pre-fix, #13 selects only the healthy card and PASSes — the undecodable
// possibly-staged card is invisible to both the selection and the health
// preflight. Post-fix #13 FAILs naming the undecodable card file; content
// presence of the OTHER card cannot rescue the batch.
func TestStagedErrataContent_FailOnRegistryUndecodableErrataCard(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-b-healthy.json",
		"errata-healthy", "Erratum: healthy staged claim", "docs/migration-errata/v0.12.0.md")
	writeDecodeFailingStagedErrataCard(t, dir, "errata-a-degraded.json",
		"errata-degraded", "Erratum: registry-undecodable staged claim", "docs/migration-errata/v0.13.0.md")
	// The HEALTHY card's body IS present — must not rescue the batch.
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\n## Errata for v0.12.0\n\n"+sampleErratumBody+"\n")

	r := checkStagedErrataContent(dir)
	if r.tier != tierFail {
		t.Fatalf("want FAIL for a registry-undecodable errata card in the batch, got %s: %s", r.tier, r.detail)
	}
	for _, want := range []string{"errata-a-degraded", "clean the card", "withdraw"} {
		if !strings.Contains(r.detail, want) {
			t.Errorf("FAIL detail should mention %q; got %q", want, r.detail)
		}
	}
}

// --- F4-C release-diff-recurrence predicate (doctor #12, second class) ---
//
// These tests prove the F1-D1 gap fix: an OPEN liveness card whose
// path_touched target re-fires in the release diff (PRIOR_TAG..HEAD) is REFUSED
// when a release is imminent (an about-to-release note exists), and CLEARS once
// a disposition lands (closed status / manifest entry / operator override). They
// also prove silencer-immunity (a card without the defer- prefix or a .md card
// blocks) and the advisory-only behavior of fog/cold findings.

// gitCommitFile writes a file at relpath with the given content, adds it, and
// commits it so the recurrence predicate can compute a meaningful
// PRIOR_TAG..HEAD diff.
func gitCommitFile(t *testing.T, dir, relpath, content, msg string) {
	t.Helper()
	full := filepath.Join(dir, relpath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", relpath, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", relpath, err)
	}
	exec.Command("git", "-C", dir, "config", "user.email", "t@t").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "t").Run()
	if err := exec.Command("git", "-C", dir, "add", relpath).Run(); err != nil {
		t.Fatalf("git add %s: %v", relpath, err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "-q", "-m", msg).Run(); err != nil {
		t.Fatalf("git commit %s: %v", relpath, err)
	}
}

// writeManifest writes a minimal release-defer-dispositions manifest carrying the
// given defer_ids as records[]. Doctor #12 reads this for manifest_entry(c); it
// does NOT validate the handshake (that is the release-mode evaluator's job).
func writeManifest(t *testing.T, dir string, deferIDs []string) {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir manifest dir: %v", err)
	}
	records := make([]map[string]any, 0, len(deferIDs))
	for _, id := range deferIDs {
		records = append(records, map[string]any{"defer_id": id})
	}
	obj := map[string]any{"schema_version": 1, "records": records}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "release-defer-dispositions.json"), raw, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// writeLivenessCard writes a coordinator task card under
// <dir>/.local/coordinator/tasks/<name> carrying an owner_notes trigger block.
// Used by the F4-C recurrence tests where the card's path_touched target is the
// signal. name may carry ANY extension/prefix (the silencer-immunity contract).
func writeLivenessCard(t *testing.T, dir, name, taskID, status string, ownerNotes []string) {
	t.Helper()
	obj := map[string]any{
		"schema_version": 1,
		"task_id":        taskID,
		"title":          "recurrence-test-card",
		"status":         status,
		"owner_notes":    ownerNotes,
	}
	raw, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("marshal card %s: %v", name, err)
	}
	writeTaskCard(t, dir, name, string(raw))
}

// writeRawLivenessCard writes an arbitrary body (e.g. markdown) as a liveness
// card file. Used to prove silencer-immunity for non-JSON cards.
func writeRawLivenessCard(t *testing.T, dir, name, body string) {
	t.Helper()
	writeTaskCard(t, dir, name, body)
}

// recurrenceFixture builds a scratch git repo with: a prior tag (v1.0.0), a
// committed change to targetPath AFTER the tag (so targetPath is in the
// v1.0.0..HEAD diff), a valid adoption marker, and an about-to-release
// (untagged) migration note v1.1.0 (so releaseImminent == true). Returns dir.
func recurrenceFixture(t *testing.T, targetPath string) string {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("VH_HARNESS_DEFER_DIFF_SINCE", "") // force self-derived prior tag
	t.Setenv("VH_HARNESS_DEFER_OVERRIDE_IDS", "")
	writeAdoptionMarker(t, dir)
	gitCommitFile(t, dir, targetPath, "baseline\n", "baseline")
	gitTag(t, dir, "v1.0.0")
	gitCommitFile(t, dir, targetPath, "changed\n", "modify target")
	writeMigrationNote(t, dir, "v1.1.0") // untagged → about-to-release
	return dir
}

// TestDeferRecurrence_RefusesFiredOpenCard is the BEHAVIORAL-CLOSURE CRUX
// (REFUSE half): an imminent release whose diff fires an OPEN .local/coordinator/
// tasks/ defer with no disposition is REFUSED (FAIL) and the offending card is
// named. The card deliberately carries NO defer- prefix (silencer-immunity: a
// card cannot escape the gate by being named without the prefix).
func TestDeferRecurrence_RefusesFiredOpenCard(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeLivenessCard(t, dir, "task-no-prefix.json", "task-no-prefix", "draft",
		[]string{"source:review-defer", "trigger:path_touched(" + target + ")", "studied:2026-07-30"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("CRUX want FAIL for fired open card with no disposition, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "task-no-prefix") {
		t.Errorf("FAIL should name the offending card; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "release-diff-recurrence") {
		t.Errorf("FAIL should flag the recurrence class; got %q", r.detail)
	}
}

// TestDeferRecurrence_ClearsViaClosedStatus is the CRUX CLEAR half (a): once the
// fired card's status moves into the closed set (completed), the refuse clears.
func TestDeferRecurrence_ClearsViaClosedStatus(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeLivenessCard(t, dir, "task-clearable.json", "task-clearable", "completed",
		[]string{"trigger:path_touched(" + target + ")"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("CRUX want PASS after closed status, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferRecurrence_ClearsViaManifestEntry is the CRUX CLEAR half (b): an
// otherwise-blocking open card clears when a manifest record for its task_id
// exists (the releaser's committed disposition).
func TestDeferRecurrence_ClearsViaManifestEntry(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeLivenessCard(t, dir, "task-manifest.json", "task-manifest", "draft",
		[]string{"trigger:path_touched(" + target + ")"})
	writeManifest(t, dir, []string{"task-manifest"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("CRUX want PASS after manifest entry, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferRecurrence_ClearsViaOperatorOverride is the CRUX CLEAR half (c): an
// otherwise-blocking open card clears when the operator supplies a live-intent
// override (VH_HARNESS_DEFER_OVERRIDE_IDS env var).
func TestDeferRecurrence_ClearsViaOperatorOverride(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeLivenessCard(t, dir, "task-override.json", "task-override", "draft",
		[]string{"trigger:path_touched(" + target + ")"})
	t.Setenv("VH_HARNESS_DEFER_OVERRIDE_IDS", "task-override")

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("CRUX want PASS after operator override, got %s: %s", r.tier, r.detail)
	}
}

// TestDeferRecurrence_MDCardBlocks proves silencer-immunity for non-JSON cards:
// a .md file (not parseable as JSON) carrying a path_touched target in free text
// is treated as an OPEN card and BLOCKS when its target fires.
func TestDeferRecurrence_MDCardBlocks(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeRawLivenessCard(t, dir, "scratch-notes.md",
		"# scratch\n\nTrigger: path_touched("+target+")\n")

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierFail {
		t.Fatalf("want FAIL for fired .md card (silencer-immunity), got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "scratch-notes") {
		t.Errorf("FAIL should name the .md card (filename-derived id); got %q", r.detail)
	}
}

// TestDeferRecurrence_NonGrammarTriggerIsCold proves a card whose ONLY target
// is non-grammar (a glob or directory operand) is COLD — INFORM only, never
// blocks. v1 exact-match cannot resolve glob/dir path_touched semantics.
func TestDeferRecurrence_NonGrammarTriggerIsCold(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeLivenessCard(t, dir, "task-glob.json", "task-glob", "draft",
		[]string{"trigger:path_touched(internal/cli/*)"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS for cold (glob) card — INFORM only, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "cold card") {
		t.Errorf("PASS detail should INFORM about the cold card; got %q", r.detail)
	}
}

// TestDeferRecurrence_FogCardIsInform proves a card with NO path_touched target
// (fog) is INFORM only — never blocks — even when a release is imminent.
func TestDeferRecurrence_FogCardIsInform(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := recurrenceFixture(t, target)
	writeLivenessCard(t, dir, "task-fog.json", "task-fog", "draft",
		[]string{"source:review-defer", "studied:2026-07-30"}) // no trigger:path_touched

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS for fog card — INFORM only, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "fog card") {
		t.Errorf("PASS detail should INFORM about the fog card; got %q", r.detail)
	}
}

// TestDeferRecurrence_DormantWhenNoReleaseImminent proves the predicate is
// DORMANT (advisory only, no FAIL) when no release is imminent (no about-to-
// release note), even with a fired open card. This keeps doctor HEALTHY during
// ordinary development; the BLOCK activates only at tag time.
func TestDeferRecurrence_DormantWhenNoReleaseImminent(t *testing.T) {
	target := "internal/cli/release_gate.go"
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv("VH_HARNESS_DEFER_DIFF_SINCE", "")
	t.Setenv("VH_HARNESS_DEFER_OVERRIDE_IDS", "")
	writeAdoptionMarker(t, dir)
	gitCommitFile(t, dir, target, "baseline\n", "baseline")
	gitTag(t, dir, "v1.0.0")
	gitCommitFile(t, dir, target, "changed\n", "modify target")
	// A RELEASED note (tagged) — no about-to-release → releaseImminent == false.
	writeMigrationNote(t, dir, "v1.0.0")
	writeLivenessCard(t, dir, "task-dormant.json", "task-dormant", "draft",
		[]string{"trigger:path_touched(" + target + ")"})

	r := checkDeferLiveness(dir)
	if r.tier == tierSkip {
		t.Skipf("check unavailable in env: %s", r.detail)
	}
	if r.tier != tierPass {
		t.Fatalf("want PASS (dormant) when no release imminent, got %s: %s", r.tier, r.detail)
	}
}
