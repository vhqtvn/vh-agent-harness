package claims

// claim_test.go — tests for the §4.1 claim/verifier closure kernel.
//
// These tests prove the kernel's contract (see claim.go package doc):
//   - Derive reads BOTH sides (Side A cards + Side B notes) into a single
//     typed Registry.
//   - git-tag classification of notes (released vs about-to-release).
//   - referenced-version extraction catches multi-reference scope strings.
//   - malformed cards are surfaced (NOT swallowed) — the fail-closed grounding
//     the release gate relies on.
//   - Derive is NON-AUTHORITATIVE: read-only, no writes/persistence (snapshot
//     the dir tree before/after and assert no new/modified files).
//   - the Records projection carries the S1 claim schema in each body and every
//     record validates against the substrate contract.
//
// The gate-level behavior (FAIL-on-contradiction, PASS, SKIP, fail-closed
// through the registry path) is exercised in internal/cli/release_gate_test.go,
// which consumes this kernel unchanged.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/record"
)

// gitInit creates a real throwaway git repo in dir so note-tag classification
// works. (The release_gate tests have their own copy in package cli.)
func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", dir}, args...)
		if err := exec.Command("git", full...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
}

func gitTag(t *testing.T, dir, version string) {
	t.Helper()
	if err := exec.Command("git", "-C", dir, "tag", version).Run(); err != nil {
		t.Fatalf("git tag %s: %v", version, err)
	}
}

func gitCommitStub(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".stub"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "add", "-A").Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run(); err != nil {
		t.Fatalf("git commit: %v", err)
	}
}

func writeNote(t *testing.T, dir, version string) {
	t.Helper()
	d := filepath.Join(dir, "templates", "migrations")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, version+".md"), []byte("# "+version+"\n"), 0o644); err != nil {
		t.Fatalf("write note %s: %v", version, err)
	}
}

func writeCard(t *testing.T, dir, name, taskID, title, status string, filesInScope, roughScope []string) {
	t.Helper()
	d := filepath.Join(dir, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
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
	if err := os.WriteFile(filepath.Join(d, name), raw, 0o644); err != nil {
		t.Fatalf("write card %s: %v", name, err)
	}
}

// TestDerive_BothSides projects a note + a card into a Registry and checks the
// typed projection carries exactly the inputs.
func TestDerive_BothSides(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.2.0")
	writeCard(t, dir, "defer-v020-x.json", "defer-v020-x", "Defer x", "draft",
		[]string{"templates/migrations/v0.2.0.md"}, nil)

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: unexpected error: %v", err)
	}
	if !reg.TasksPresent {
		t.Errorf("TasksPresent: want true")
	}
	if !reg.NotesPresent {
		t.Errorf("NotesPresent: want true")
	}
	if len(reg.Notes) != 1 || reg.Notes[0].Version != "v0.2.0" {
		t.Errorf("Notes = %#v, want one v0.2.0", reg.Notes)
	}
	if len(reg.Cards) != 1 || reg.Cards[0].TaskID != "defer-v020-x" {
		t.Errorf("Cards = %#v, want one defer-v020-x", reg.Cards)
	}
	if len(reg.CardErrors) != 0 {
		t.Errorf("CardErrors = %#v, want none", reg.CardErrors)
	}
	// One note + one card → two substrate records, all validating.
	if len(reg.Records) != 2 {
		t.Fatalf("Records: want 2, got %d", len(reg.Records))
	}
	for i, r := range reg.Records {
		if err := r.Validate(); err != nil {
			t.Errorf("Records[%d] failed substrate Validate: %v", i, err)
		}
	}
}

// TestDerive_GitTagClassification proves a tagged note is released and an
// untagged note is about-to-release.
func TestDerive_GitTagClassification(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.1.0")
	writeNote(t, dir, "v0.2.0")
	gitCommitStub(t, dir)
	gitTag(t, dir, "v0.1.0") // released; v0.2.0 stays about-to-release

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	byVer := map[string]bool{}
	released := map[string]bool{}
	for _, n := range reg.Notes {
		byVer[n.Version] = true
		released[n.Version] = n.Released
	}
	if !released["v0.1.0"] {
		t.Errorf("v0.1.0: want released=true (tagged)")
	}
	if released["v0.2.0"] {
		t.Errorf("v0.2.0: want released=false (untagged)")
	}
}

// TestDerive_ReferencedVersions_MultiRef proves a single scope string naming two
// notes yields BOTH versions (the AG4 multi-reference fix, now in the kernel).
func TestDerive_ReferencedVersions_MultiRef(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.2.0")
	writeCard(t, dir, "defer-multiref.json", "defer-multiref", "multi", "draft",
		nil, []string{"Fix templates/migrations/v0.1.0.md and templates/migrations/v0.2.0.md"})

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(reg.Cards) != 1 {
		t.Fatalf("Cards: want 1, got %d", len(reg.Cards))
	}
	got := append([]string(nil), reg.Cards[0].ReferencedVersions...)
	sort.Strings(got)
	want := []string{"v0.1.0", "v0.2.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ReferencedVersions = %v, want %v", got, want)
	}
}

// TestDerive_MalformedCardSurfaced proves a malformed card is collected in
// CardErrors (NOT swallowed) — the fail-closed grounding the gate relies on.
func TestDerive_MalformedCardSurfaced(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.1.0")
	d := filepath.Join(dir, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "defer-broken.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatalf("write broken card: %v", err)
	}

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: unexpected dir-level error: %v", err)
	}
	if !reg.TasksPresent {
		t.Errorf("TasksPresent: want true (dir exists)")
	}
	if len(reg.CardErrors) != 1 {
		t.Fatalf("CardErrors: want 1, got %d", len(reg.CardErrors))
	}
	if !strings.HasSuffix(reg.CardErrors[0].Path, "defer-broken.json") {
		t.Errorf("CardErrors[0].Path = %q, want defer-broken.json suffix", reg.CardErrors[0].Path)
	}
}

// TestDerive_MissingTasksDir proves an absent tasks dir is a clean no-op
// (TasksPresent=false, no error) — the gate's Side A SKIP grounding.
func TestDerive_MissingTasksDir(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.1.0")

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if reg.TasksPresent {
		t.Errorf("TasksPresent: want false (no tasks dir)")
	}
}

// TestDerive_MissingNotesDir proves an absent/empty migrations dir is a clean
// no-op (NotesPresent=false, no error) — the gate's Side B SKIP grounding.
func TestDerive_MissingNotesDir(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeCard(t, dir, "defer-x.json", "defer-x", "x", "draft", nil, nil)

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if reg.NotesPresent {
		t.Errorf("NotesPresent: want false (no notes dir)")
	}
}

// TestDerive_NonAuthoritative_ReadOnly proves Derive has READ side-effects only:
// it creates no files and modifies none. We snapshot the full dir tree
// (relative paths + sizes) before and after and assert equality.
func TestDerive_NonAuthoritative_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.1.0")
	writeCard(t, dir, "defer-x.json", "defer-x", "x", "draft",
		[]string{"templates/migrations/v0.1.0.md"}, nil)

	snapshot := func() map[string]int64 {
		out := map[string]int64{}
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			out[rel] = info.Size()
			return nil
		})
		return out
	}
	before := snapshot()

	if _, err := Derive(dir); err != nil {
		t.Fatalf("Derive: %v", err)
	}

	after := snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Errorf("Derive mutated the tree (non-authoritative invariant broken):\nbefore=%v\nafter=%v", before, after)
	}
	// Explicitly assert NO records.jsonl was created anywhere under dir.
	var jsonlFound []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".jsonl") {
			jsonlFound = append(jsonlFound, path)
		}
		return nil
	})
	if len(jsonlFound) > 0 {
		t.Errorf("Derive must not persist records; found jsonl files: %v", jsonlFound)
	}
}

// TestDerive_RecordsCarryS1 proves each substrate Record's body parses back to
// the S1 Claim schema with the expected provenance and scene fields set.
func TestDerive_RecordsCarryS1(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	gitCommitStub(t, dir)
	writeNote(t, dir, "v0.1.0")
	gitTag(t, dir, "v0.1.0") // released
	writeCard(t, dir, "defer-closed.json", "defer-closed", "done", "completed",
		[]string{"templates/migrations/v0.1.0.md"}, nil)

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	var noteRec, cardRec *record.Record
	for i := range reg.Records {
		r := reg.Records[i]
		if r.ID == "released_note:v0.1.0" {
			noteRec = &reg.Records[i]
		}
		if r.ID == "defer_disposition:defer-closed" {
			cardRec = &reg.Records[i]
		}
	}
	if noteRec == nil || cardRec == nil {
		t.Fatalf("missing expected records: noteRec=%v cardRec=%v", noteRec != nil, cardRec != nil)
	}

	// Note record: S1 body + canon provenance + correct scene.
	var nc Claim
	if err := json.Unmarshal([]byte(noteRec.Body), &nc); err != nil {
		t.Fatalf("note body parse: %v", err)
	}
	if nc.ClaimID != "released_note:v0.1.0" {
		t.Errorf("note ClaimID = %q", nc.ClaimID)
	}
	if nc.Kind != KindReleasedNote {
		t.Errorf("note Kind = %q, want %q", nc.Kind, KindReleasedNote)
	}
	if nc.Disposition != "released" {
		t.Errorf("note Disposition = %q, want released", nc.Disposition)
	}
	if nc.SourceRef == "" || !strings.HasSuffix(nc.SourceRef, "v0.1.0.md") {
		t.Errorf("note SourceRef = %q (canon provenance)", nc.SourceRef)
	}
	if noteRec.Scene == nil || *noteRec.Scene != string(KindReleasedNote) {
		t.Errorf("note Scene = %v, want %q", noteRec.Scene, KindReleasedNote)
	}
	if noteRec.Type != record.TypeInstruction {
		t.Errorf("note Type = %q, want instruction", noteRec.Type)
	}

	// Card record: S1 body + transport provenance + closed disposition.
	var cc Claim
	if err := json.Unmarshal([]byte(cardRec.Body), &cc); err != nil {
		t.Fatalf("card body parse: %v", err)
	}
	if cc.ClaimID != "defer_disposition:defer-closed" {
		t.Errorf("card ClaimID = %q", cc.ClaimID)
	}
	if cc.Kind != KindDeferDisposition {
		t.Errorf("card Kind = %q, want %q", cc.Kind, KindDeferDisposition)
	}
	if cc.Disposition != "closed" {
		t.Errorf("card Disposition = %q, want closed", cc.Disposition)
	}
	if cc.SourceRef == "" || !strings.HasSuffix(cc.SourceRef, "defer-closed.json") {
		t.Errorf("card SourceRef = %q (transport provenance)", cc.SourceRef)
	}
	if !strings.Contains(cc.VerifierRef, "defer_liveness") {
		t.Errorf("card VerifierRef = %q, want to name the gate", cc.VerifierRef)
	}
}
