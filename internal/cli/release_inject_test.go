package cli

// release_inject_test.go — tests for `vh-agent-harness release inject-errata`.
//
// These tests prove the inject subcommand's contract:
//   - INJECT + FLIP: a staged errata card whose body is absent from the
//     about-to-release note → section appended, card flipped staged→completed.
//   - IDEMPOTENT: a note that already contains the body → no duplicate section,
//     but the card still flips.
//   - DRY-RUN: no files written.
//   - NO STAGED CARDS: clean no-op (exit 0).
//   - RED→GREEN: after inject, doctor check #13 passes (the closure).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readCardStatus reads a card file and returns its status field.
func readCardStatus(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read card %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse card %s: %v", path, err)
	}
	s, _ := doc["status"].(string)
	return s
}

// TestReleaseInjectErrata_InjectsAndFlips: the core contract — a staged errata
// card whose body is absent from the about-to-release note gets its section
// injected AND its card flipped to completed in the same slice.
func TestReleaseInjectErrata_InjectsAndFlips(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nRelease notes.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	cardName := "errata-v0120-sample.json"
	writeStagedErrataCard(t, dir, cardName,
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	cardPath := filepath.Join(dir, ".local", "coordinator", "tasks", cardName)

	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err != nil {
		t.Fatalf("inject-errata failed: %v\n--- output ---\n%s", err, out)
	}
	// The note now contains the erratum body.
	noteBytes, nerr := os.ReadFile(filepath.Join(dir, "templates", "migrations", "v0.15.0.md"))
	if nerr != nil {
		t.Fatalf("read note: %v", nerr)
	}
	noteText := string(noteBytes)
	if !strings.Contains(noteText, "## Errata for v0.12.0") {
		t.Errorf("note should contain the errata section heading; got:\n%s", noteText)
	}
	if !strings.Contains(strings.ToLower(noteText), strings.ToLower(sampleErratumBody)) {
		t.Errorf("note should contain the erratum body; got:\n%s", noteText)
	}
	// The card is flipped to completed.
	if status := readCardStatus(t, cardPath); status != "completed" {
		t.Errorf("card status should be completed, got %q", status)
	}
	if !strings.Contains(out, "injected") {
		t.Errorf("output should report injection; got:\n%s", out)
	}
}

// TestReleaseInjectErrata_RedToGreenClosure: the inject clears the #13 FAIL.
// Before inject, #13 FAILs (content missing); after inject, #13 PASSes.
func TestReleaseInjectErrata_RedToGreenClosure(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nRelease notes.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")

	// RED: #13 fails before inject.
	r := checkStagedErrataContent(dir)
	t.Logf("RED: check #13 tier before inject = %s (detail: %s)", r.tier, r.detail)
	if r.tier != tierFail {
		t.Fatalf("RED: want FAIL before inject, got %s: %s", r.tier, r.detail)
	}

	// Inject.
	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err != nil {
		t.Fatalf("inject-errata failed: %v\n--- output ---\n%s", err, out)
	}
	t.Logf("INJECT: release inject-errata ran; output:\n%s", out)

	// GREEN: #13 passes after inject (no staged cards remain).
	r2 := checkStagedErrataContent(dir)
	t.Logf("GREEN: check #13 tier after inject = %s (detail: %s)", r2.tier, r2.detail)
	if r2.tier != tierPass {
		t.Fatalf("GREEN: want PASS after inject, got %s: %s", r2.tier, r2.detail)
	}
}

// TestReleaseInjectErrata_IdempotentWhenAlreadyPresent: a note that already
// contains the body → no duplicate section, but the card still flips.
func TestReleaseInjectErrata_IdempotentWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	// The note ALREADY contains the body.
	noteBody := "# v0.15.0\n\n## Errata for v0.12.0\n\n" + sampleErratumBody + "\n"
	writeMigrationNoteBody(t, dir, "v0.15.0", noteBody)
	cardPath := filepath.Join(dir, ".local", "coordinator", "tasks", "errata-v0120-sample.json")

	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err != nil {
		t.Fatalf("inject-errata failed: %v\n--- output ---\n%s", err, out)
	}
	// The note was NOT modified (body already present).
	noteBytes, _ := os.ReadFile(filepath.Join(dir, "templates", "migrations", "v0.15.0.md"))
	if string(noteBytes) != noteBody {
		t.Errorf("note should be unchanged (idempotent); got:\n%s", string(noteBytes))
	}
	// Count section headings — must be exactly 1 (no duplicate).
	if c := strings.Count(string(noteBytes), "## Errata for v0.12.0"); c != 1 {
		t.Errorf("expected exactly 1 errata section heading, got %d", c)
	}
	// The card still flipped (idempotent on the NOTE, not the card).
	if status := readCardStatus(t, cardPath); status != "completed" {
		t.Errorf("card status should be completed (flipped even when note unchanged), got %q", status)
	}
	if !strings.Contains(out, "already present") {
		t.Errorf("output should report idempotent skip; got:\n%s", out)
	}
}

// TestReleaseInjectErrata_DryRunWritesNothing: --dry-run prints the plan but
// writes neither the note nor the card.
func TestReleaseInjectErrata_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	origNote := "# v0.15.0\n\nRelease notes.\n"
	writeMigrationNoteBody(t, dir, "v0.15.0", origNote)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	cardPath := filepath.Join(dir, ".local", "coordinator", "tasks", "errata-v0120-sample.json")

	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir, "--dry-run"})
	if err != nil {
		t.Fatalf("inject-errata --dry-run failed: %v\n--- output ---\n%s", err, out)
	}
	// Note unchanged.
	noteBytes, _ := os.ReadFile(filepath.Join(dir, "templates", "migrations", "v0.15.0.md"))
	if string(noteBytes) != origNote {
		t.Errorf("dry-run should not modify the note; got:\n%s", string(noteBytes))
	}
	// Card still staged.
	if status := readCardStatus(t, cardPath); status != "staged" {
		t.Errorf("dry-run should not flip the card, got %q", status)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("output should flag dry-run; got:\n%s", out)
	}
}

// TestReleaseInjectErrata_NoteOverrideRejectedWhenNotHighest: with two untagged
// notes (v0.15.0, v0.16.0), --note must NOT be accepted when it points at the
// lower note. This prevents an operator from injecting into a stale note,
// flipping the card to completed, and having check #13 PASS on the empty staged
// set while the shipping note (v0.16.0) lacks the correction (commit-review
// tier1b-F1 round 3).
func TestReleaseInjectErrata_NoteOverrideRejectedWhenNotHighest(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n")
	writeMigrationNoteBody(t, dir, "v0.16.0", "# v0.16.0\n")
	cardPath := filepath.Join(dir, ".local", "coordinator", "tasks", "errata-v0120-sample.json")

	// --note pointing at v0.15.0 (the lower note) must be REJECTED.
	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir, "--note", "templates/migrations/v0.15.0.md"})
	if err == nil {
		t.Fatalf("--note pointing at a non-highest note should be rejected; got success:\n%s", out)
	}
	if !strings.Contains(out, "does not resolve to the highest about-to-release note") {
		t.Errorf("error should explain --note must match the highest note; got:\n%s", out)
	}
	// Card must remain staged (no flip happened).
	if status := readCardStatus(t, cardPath); status != "staged" {
		t.Errorf("card should remain staged when --note is rejected, got %q", status)
	}
	// The higher note must be unmodified.
	highNote, _ := os.ReadFile(filepath.Join(dir, "templates", "migrations", "v0.16.0.md"))
	if !strings.HasPrefix(string(highNote), "# v0.16.0\n") {
		t.Errorf("highest note should be untouched; got:\n%s", string(highNote))
	}
}

// TestReleaseInjectErrata_NoteOverrideAcceptedWhenHighest: --note pointing at
// the highest about-to-release note IS accepted (the override is not dead — it
// just must match the note the gate targets).
func TestReleaseInjectErrata_NoteOverrideAcceptedWhenHighest(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-v0120-sample.json",
		"errata-v0120-sample", "Erratum: sample false claim in v0.12.0", "docs/migration-errata/v0.12.0.md")
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n")
	writeMigrationNoteBody(t, dir, "v0.16.0", "# v0.16.0\n")

	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir, "--note", "templates/migrations/v0.16.0.md"})
	if err != nil {
		t.Fatalf("--note pointing at the highest note should succeed; got %v:\n%s", err, out)
	}
	if !strings.Contains(out, "injected section") {
		t.Errorf("output should report injection; got:\n%s", out)
	}
}

// TestReleaseInjectErrata_NoStagedCards: no staged errata cards → clean no-op.
func TestReleaseInjectErrata_NoStagedCards(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n")

	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err != nil {
		t.Fatalf("inject-errata with no staged cards should succeed, got %v", err)
	}
	if !strings.Contains(out, "nothing to inject") {
		t.Errorf("output should report no staged cards; got:\n%s", out)
	}
}

// writeShapeInvalidStagedErrataCard writes a staged errata card that VIOLATES
// the staged-errata SHAPE (internal/taskcard/staged_validation.go):
// identical to the healthy writeStagedErrataCard shape except
// success_criteria is EMPTY — a required list must be non-empty, so the
// offending field set is exactly ["success_criteria"], deterministic for
// assertions. The staged_path + erratum body are fully valid, so pre-fix
// behavior injected these cards as if they were healthy (no shape concept at
// all). (The OLD JS-normalizer port also degraded an empty success_criteria
// at non-draft status — this arm keeps coverage but is not a red receipt.)
func writeShapeInvalidStagedErrataCard(t *testing.T, dir, name, taskID, title, stagedPath string) {
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
		"rough_scope":       []string{"Staged erratum transport with an empty success_criteria list (shape-invalid)."},
		"success_criteria":  []string{},
		"validation_plan":   []string{"Release ceremony inject-errata step reads this card and injects the staged_path body into the next release note"},
		"history": []any{map[string]any{
			"at": "2026-11-07T06:38:29Z", "event": "erratum_staged",
			"session_name": "erratum-stage", "status": "staged", "note": "n",
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

// snapshotTree captures every file under root as relpath → raw bytes, for
// byte-identical (zero-write) assertions across a refused injector run.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		snap[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snap
}

// assertTreeUnchanged fails the test when any file under root differs from the
// snapshot (byte-identical zero-write proof).
func assertTreeUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotTree(t, root)
	for p, want := range before {
		got, ok := after[p]
		if !ok {
			t.Errorf("file %s disappeared during a refused run (must be zero-write)", p)
			continue
		}
		if got != want {
			t.Errorf("file %s mutated during a refused run (must be byte-identical):\n--- before ---\n%s\n--- after ---\n%s", p, want, got)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("file %s appeared during a refused run (must be zero-write)", p)
		}
	}
}

// runMixedBatchRefusal is the shared body of the mixed healthy+shape-invalid
// batch arms: a healthy staged errata card and a SHAPE-INVALID staged errata
// card (empty success_criteria) coexist; inject-errata must REFUSE the whole
// batch (nonzero) BEFORE ANY write — note AND both cards byte-identical — and
// name the offending card id + offending field. degradedFirst controls on-disk
// ordering (cards iterate in filename order), so both orderings are proven.
func runMixedBatchRefusal(t *testing.T, degradedFirst bool, extraArgs ...string) {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nRelease notes.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeErratumFile(t, dir, "docs/migration-errata/v0.13.0.md", "v0.13.0", sampleErratumBody)

	healthyName, degradedName := "errata-b-healthy.json", "errata-a-degraded.json"
	if degradedFirst {
		healthyName, degradedName = "errata-a-healthy.json", "errata-b-degraded.json"
	}
	writeStagedErrataCard(t, dir, healthyName,
		"errata-healthy", "Erratum: healthy staged claim", "docs/migration-errata/v0.12.0.md")
	writeShapeInvalidStagedErrataCard(t, dir, degradedName,
		"errata-degraded", "Erratum: degraded staged claim", "docs/migration-errata/v0.13.0.md")

	before := snapshotTree(t, dir)
	args := append([]string{"release", "inject-errata", "--target", dir}, extraArgs...)
	out, err := executeCapture(t, args)

	// REFUSAL: nonzero exit, offending card + field named.
	if err == nil {
		t.Fatalf("mixed healthy+shape-invalid batch must be REFUSED (nonzero); got success:\n%s", out)
	}
	if !strings.Contains(out, "errata-degraded") {
		t.Errorf("refusal must name the offending card id errata-degraded; got:\n%s", out)
	}
	if !strings.Contains(out, "success_criteria") {
		t.Errorf("refusal must name the offending field success_criteria; got:\n%s", out)
	}
	// ZERO-WRITE: the note and BOTH cards are byte-identical.
	assertTreeUnchanged(t, dir, before)
}

// TestReleaseInjectErrata_MixedBatchShapeInvalidFirst_RefusesZeroWrite: a
// shape-invalid staged card (filename-sorted first) + a healthy staged card →
// whole-batch refusal before any write. Pre-fix receipt (RED): BOTH cards were
// injected and flipped with exit 0 — the injector had no shape concept at all.
func TestReleaseInjectErrata_MixedBatchShapeInvalidFirst_RefusesZeroWrite(t *testing.T) {
	runMixedBatchRefusal(t, true)
}

// TestReleaseInjectErrata_MixedBatchHealthyFirst_RefusesZeroWrite: same
// refusal with the healthy card sorted first — proving the preflight covers
// the WHOLE selected set, not just a failing prefix.
func TestReleaseInjectErrata_MixedBatchHealthyFirst_RefusesZeroWrite(t *testing.T) {
	runMixedBatchRefusal(t, false)
}

// TestReleaseInjectErrata_ShapeInvalidRefusedWhenBodyAlreadyPresent: the
// shape-invalid card's erratum body is ALREADY present in the target note —
// content presence must NOT rescue a shape-invalid card (its card shape is
// independent of the note). Still a whole-batch zero-write refusal.
func TestReleaseInjectErrata_ShapeInvalidRefusedWhenBodyAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeErratumFile(t, dir, "docs/migration-errata/v0.13.0.md", "v0.13.0", sampleErratumBody)
	writeShapeInvalidStagedErrataCard(t, dir, "errata-a-degraded.json",
		"errata-degraded", "Erratum: degraded staged claim", "docs/migration-errata/v0.13.0.md")
	// The note ALREADY contains the shape-invalid card's corrective body.
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\n## Errata for v0.13.0\n\n"+sampleErratumBody+"\n")

	before := snapshotTree(t, dir)
	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err == nil {
		t.Fatalf("shape-invalid card must be refused even when its body is already present; got success:\n%s", out)
	}
	if !strings.Contains(out, "errata-degraded") || !strings.Contains(out, "success_criteria") {
		t.Errorf("refusal must name offending card + field; got:\n%s", out)
	}
	assertTreeUnchanged(t, dir, before)
}

// TestReleaseInjectErrata_ShapeInvalidRefusedUnderDryRun: --dry-run must refuse
// a shape-invalid selection too — eligibility is a property of the batch, not
// of the write, so the ceremony's preview path fails exactly like the real path.
func TestReleaseInjectErrata_ShapeInvalidRefusedUnderDryRun(t *testing.T) {
	runMixedBatchRefusal(t, true, "--dry-run")
}

// TestReleaseInjectErrata_MixedBatchWithUnreadableStagedPath_RefusesZeroWrite:
// a shape-invalid card whose staged_path is ALSO unreadable + a healthy card.
// This is the literal pre-fix PARTIAL-WRITE receipt: today the healthy card is
// injected and flipped while the failing card merely lands in the trailing
// error list — a mixed batch partially injects. Post-fix the whole batch is
// refused before any write.
func TestReleaseInjectErrata_MixedBatchWithUnreadableStagedPath_RefusesZeroWrite(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nRelease notes.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeStagedErrataCard(t, dir, "errata-b-healthy.json",
		"errata-healthy", "Erratum: healthy staged claim", "docs/migration-errata/v0.12.0.md")
	// Shape-invalid card whose staged_path points at a file that does not exist.
	writeShapeInvalidStagedErrataCard(t, dir, "errata-a-degraded.json",
		"errata-degraded", "Erratum: degraded staged claim", "docs/migration-errata/missing-v0.13.0.md")

	before := snapshotTree(t, dir)
	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err == nil {
		t.Fatalf("mixed batch with a shape-invalid+unreadable card must be REFUSED; got success:\n%s", out)
	}
	if !strings.Contains(out, "errata-degraded") {
		t.Errorf("refusal must name the offending card; got:\n%s", out)
	}
	assertTreeUnchanged(t, dir, before)
}

// writeDecodeFailingStagedErrataCard writes a staged errata card the CLAIM
// REGISTRY (claims.Derive → loadDeferCards) cannot decode: the JSON is
// well-formed, but files_in_scope is an OBJECT where the DeferCard schema
// wants a string array, so json.Unmarshal fails and the card lands in
// reg.CardErrors instead of reg.Cards. The card is INTENDED to be a staged
// erratum (status staged, staged_path set) — the C-F1 class: invisible to
// stagedErrataCards(reg.Cards), so pre-fix neither the injector's Phase-0
// preflight nor doctor check #13 ever shape-checks it, and a mixed batch
// PARTIALLY INJECTS.
func writeDecodeFailingStagedErrataCard(t *testing.T, dir, name, taskID, title, stagedPath string) {
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
		"files_in_scope":    map[string]any{}, // registry decode failure: object, not array
		"rough_scope":       []string{"Staged erratum transport whose card fails registry decode (files_in_scope is an object, not an array)."},
		"success_criteria":  []string{"Next release note carries the staged corrective body"},
		"validation_plan":   []string{"Release ceremony inject-errata step reads this card"},
		"history": []any{map[string]any{
			"at": "2026-11-07T06:38:29Z", "event": "erratum_staged",
			"session_name": "erratum-stage", "status": "staged", "note": "n",
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

// runMixedBatchDecodeRefusal is the C-F1 body: a healthy staged errata card
// and a staged errata card that fails CLAIM-REGISTRY DECODE (files_in_scope
// object → reg.CardErrors, absent from reg.Cards) coexist. Pre-fix, the
// decode-failing card is invisible to the whole-set shape preflight (it is
// never in the selected staged set), so the healthy card PARTIALLY INJECTS
// and flips with exit 0 — the exact zero-partial-write violation. Post-fix
// the whole batch is refused before any write, the undecodable card file is
// named, and the tree stays byte-identical. decodeFirst controls on-disk
// ordering so both orderings are proven.
func runMixedBatchDecodeRefusal(t *testing.T, decodeFailingFirst bool) {
	t.Helper()
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nRelease notes.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.12.0.md", "v0.12.0", sampleErratumBody)
	writeErratumFile(t, dir, "docs/migration-errata/v0.13.0.md", "v0.13.0", sampleErratumBody)

	healthyName, undecodableName := "errata-b-healthy.json", "errata-a-degraded.json"
	if decodeFailingFirst {
		healthyName, undecodableName = "errata-a-healthy.json", "errata-b-degraded.json"
	}
	writeStagedErrataCard(t, dir, healthyName,
		"errata-healthy", "Erratum: healthy staged claim", "docs/migration-errata/v0.12.0.md")
	writeDecodeFailingStagedErrataCard(t, dir, undecodableName,
		"errata-degraded", "Erratum: registry-undecodable staged claim", "docs/migration-errata/v0.13.0.md")

	before := snapshotTree(t, dir)
	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})

	// REFUSAL: nonzero exit, undecodable card file named.
	if err == nil {
		t.Fatalf("mixed healthy+decode-failing batch must be REFUSED (nonzero); got success:\n%s", out)
	}
	if !strings.Contains(out, strings.TrimSuffix(undecodableName, ".json")) {
		t.Errorf("refusal must name the registry-undecodable card %s; got:\n%s", undecodableName, out)
	}
	// ZERO-WRITE: the note and BOTH cards are byte-identical.
	assertTreeUnchanged(t, dir, before)
}

// TestReleaseInjectErrata_MixedBatchDecodeFailureUndecodableFirst_RefusesZeroWrite
// (C-F1 RED arm): decode-failing card filename-sorted first + healthy card →
// whole-batch refusal before any write. Pre-fix receipt: BOTH cards were
// processed as if the batch were healthy-only — the healthy card injected and
// flipped with exit 0 while the undecodable card sat silently in
// reg.CardErrors.
func TestReleaseInjectErrata_MixedBatchDecodeFailureUndecodableFirst_RefusesZeroWrite(t *testing.T) {
	runMixedBatchDecodeRefusal(t, true)
}

// TestReleaseInjectErrata_MixedBatchDecodeFailureHealthyFirst_RefusesZeroWrite
// (C-F1): the healthy card sorted first — proves the registry-decode refusal
// covers the WHOLE selected batch plus the CardErrors class, not just a
// failing prefix.
func TestReleaseInjectErrata_MixedBatchDecodeFailureHealthyFirst_RefusesZeroWrite(t *testing.T) {
	runMixedBatchDecodeRefusal(t, false)
}

// TestReleaseInjectErrata_WhitespacePaddedStagedPathRefusedZeroWrite (round-5
// FIX 1): the injector preflight refuses a staged card whose staged_path
// TrimSpace form differs from its raw value — the injector itself TrimSpaces
// before filepath.IsAbs/Join (release_inject.go:289-301), so a padded
// " /outside/x.md" would be consumed as the ABSOLUTE /outside/x.md: an
// out-of-repo read. All four padded forms (POSIX-absolute, traversal,
// Windows drive-letter, UNC) refuse the batch with ZERO writes. RED receipt
// (guard temporarily reverted): tmp/red-round5-fix1.txt — pre-fix the
// preflight passed the raw relative-looking string and the injector went on
// to read the TRIMMED out-of-repo path.
func TestReleaseInjectErrata_WhitespacePaddedStagedPathRefusedZeroWrite(t *testing.T) {
	for _, p := range []string{
		" /outside/x.md",        // whitespace-prefixed POSIX-absolute
		" ../outside.md",        // whitespace-prefixed parent traversal
		"C:\\x.md ",             // whitespace-SUFFIXED Windows drive-letter
		" \\\\srv\\share\\x.md", // whitespace-prefixed UNC
	} {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			dir := t.TempDir()
			gitInit(t, dir)
			writeMigrationNoteBody(t, dir, "v0.15.0", "# v0.15.0\n\nRelease notes.\n")
			writeStagedErrataCard(t, dir, "errata-v0120-padded.json",
				"errata-v0120-padded", "Erratum: padded staged_path claim", p)

			before := snapshotTree(t, dir)
			out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
			if err == nil {
				t.Fatalf("whitespace-padded staged_path %q must be REFUSED; got success:\n%s", p, out)
			}
			for _, want := range []string{"errata-v0120-padded", "staged_path", "whitespace-padded", "consumed representation must equal the stored one"} {
				if !strings.Contains(out, want) {
					t.Errorf("refusal must mention %q; got:\n%s", want, out)
				}
			}
			assertTreeUnchanged(t, dir, before)
		})
	}
}

// TestReleaseInjectErrata_RestageProducedCardInjectsAndFlips (round-5 FIX 2):
// a card produced by the REAL sanctioned restage op — canonical-empty
// execution-era keys and all — is ACCEPTED end to end by the injector: the
// section is injected into the about-to-release note and the card flips
// staged→completed. Pre-FIX-2 the shape allowlist refused this exact card
// (unexpected-field refusals over the canonical-empty keys), making
// sanctioned-path staged cards structurally un-injectable.
func TestReleaseInjectErrata_RestageProducedCardInjectsAndFlips(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeMigrationNoteBody(t, dir, "v0.31.0", "# v0.31.0\n\nRelease notes.\n")
	writeErratumFile(t, dir, "docs/migration-errata/v0.30.0-restage-fixture.md", "v0.30.0", sampleErratumBody)
	writeTaskCard(t, dir, "errata-v0300-restage-fixture.json", restageProducedCardFixture)
	cardPath := filepath.Join(dir, ".local", "coordinator", "tasks", "errata-v0300-restage-fixture.json")

	out, err := executeCapture(t, []string{"release", "inject-errata", "--target", dir})
	if err != nil {
		t.Fatalf("inject-errata should ACCEPT the real restage-produced card, got %v\n--- output ---\n%s", err, out)
	}
	noteBytes, nerr := os.ReadFile(filepath.Join(dir, "templates", "migrations", "v0.31.0.md"))
	if nerr != nil {
		t.Fatalf("read note: %v", nerr)
	}
	if !strings.Contains(string(noteBytes), "## Errata for v0.30.0") {
		t.Errorf("note should contain the errata section; got:\n%s", string(noteBytes))
	}
	if status := readCardStatus(t, cardPath); status != "completed" {
		t.Errorf("restage-produced card should flip to completed, got %q", status)
	}
}

// TestAtomicWriteFile_FailureLeavesOriginalIntact proves that when the write
// fails (e.g. read-only destination directory), the original file content is
// preserved byte-for-byte — the truncate-before-write hazard is eliminated.
func TestAtomicWriteFile_FailureLeavesOriginalIntact(t *testing.T) {
	dir := t.TempDir()
	noteDir := filepath.Join(dir, "notes")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(noteDir, "test.md")
	original := "# original content\n"
	if err := os.WriteFile(notePath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make the directory read-only → CreateTemp fails.
	if err := os.Chmod(noteDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(noteDir, 0o755) //nolint:errcheck // restore for TempDir cleanup

	err := atomicWriteFile(notePath, []byte("# new content\n"))
	if err == nil {
		t.Fatal("expected error on read-only directory, got nil")
	}

	// Original MUST be intact.
	got, rerr := os.ReadFile(notePath)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if string(got) != original {
		t.Fatalf("original content corrupted after failed write: got %q, want %q", got, original)
	}
}

// TestAtomicWriteFile_SuccessWritesAndCleansUp proves the happy path writes
// the expected content and leaves no temp file behind.
func TestAtomicWriteFile_SuccessWritesAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	original := "# old\n"
	if err := os.WriteFile(notePath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	newContent := "# new content\nline 2\n"
	if err := atomicWriteFile(notePath, []byte(newContent)); err != nil {
		t.Fatalf("atomicWriteFile: %v", err)
	}

	got, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newContent {
		t.Fatalf("content = %q, want %q", got, newContent)
	}

	// No temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
