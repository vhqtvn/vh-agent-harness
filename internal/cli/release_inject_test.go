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
