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

	// defer-005 contract: ValidUntil is omitted-for-v1 (empty by explicit
	// contract, NOT proof a verdict is unexpired). v1 claim derivation has no
	// verdict-expiry input (DeferCard carries no valid_until field, migration
	// notes have no expiry source, and the gate performs no timestamp
	// validation). The omitempty tag must drop the key from every body. Assert
	// BOTH the decoded field is empty AND the raw JSON body omits the key, so
	// a future regression that accidentally populates the field cannot slip
	// through as a silently-present-but-zero value.
	if cc.ValidUntil != "" {
		t.Errorf("card ValidUntil = %q, want empty (omitted-for-v1 contract)", cc.ValidUntil)
	}
	if nc.ValidUntil != "" {
		t.Errorf("note ValidUntil = %q, want empty (omitted-for-v1 contract)", nc.ValidUntil)
	}
	if strings.Contains(noteRec.Body, "valid_until") {
		t.Errorf("note body must not carry valid_until under the omitted-for-v1 contract; body=%s", noteRec.Body)
	}
	if strings.Contains(cardRec.Body, "valid_until") {
		t.Errorf("card body must not carry valid_until under the omitted-for-v1 contract; body=%s", cardRec.Body)
	}
}

// TestDerive_SemverSortNotLexicographic proves migration notes are sorted by
// numeric semver, not lexicographic string order. With notes v0.9.0 and
// v0.10.0, lexicographic order would place v0.10.0 BEFORE v0.9.0 (because "1"
// < "9"), making v0.9.0 the "highest" — which would be wrong. The correct
// highest is v0.10.0. This is load-bearing for release safety: doctor check
// #13 and `release inject-errata` both select about[len-1] as the shipping note.
func TestDerive_SemverSortNotLexicographic(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.9.0")
	writeNote(t, dir, "v0.10.0")
	writeNote(t, dir, "v0.8.0")
	gitCommitStub(t, dir)
	// None tagged → all about-to-release.

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(reg.Notes) != 3 {
		t.Fatalf("want 3 notes, got %d", len(reg.Notes))
	}
	// Ascending semver order: v0.8.0, v0.9.0, v0.10.0.
	want := []string{"v0.8.0", "v0.9.0", "v0.10.0"}
	for i, w := range want {
		if reg.Notes[i].Version != w {
			t.Errorf("notes[%d].Version = %q, want %q (semver sort)", i, reg.Notes[i].Version, w)
		}
	}
	// The highest (about[len-1]) MUST be v0.10.0, not v0.9.0.
	highest := reg.Notes[len(reg.Notes)-1].Version
	if highest != "v0.10.0" {
		t.Errorf("highest note = %q, want v0.10.0 (semver, not lexicographic)", highest)
	}
}

// TestSemverLess directly exercises the comparator on cross-digit-boundary cases.
func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.10.0", "v0.9.0", false}, // 10 > 9 numerically
		{"v0.9.0", "v0.10.0", true},  // 9 < 10 numerically
		{"v0.1.0", "v0.1.1", true},
		{"v1.0.0", "v0.99.99", false},
		{"v0.0.1", "v0.0.2", true},
		{"v0.0.2", "v0.0.1", false},
		{"v0.1.0", "v0.1.0", false}, // equal → not less
	}
	for _, c := range cases {
		got := semverLess(c.a, c.b)
		if got != c.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestDerive_CollisionSafeClaimIdentity (defer-004) proves claim identity is
// collision-safe: empty and duplicate task_id cards are surfaced as CardError
// (fail-closed) and EXCLUDED from record projection, so they cannot collapse
// onto one record.ID under the substrate's append/dedup (last-write-wins by ID)
// semantics. Every emitted record keeps its SourceRef, and valid unique cards
// retain the stable identity "defer_disposition:<task_id>".
func TestDerive_CollisionSafeClaimIdentity(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeNote(t, dir, "v0.1.0")
	// Two valid unique cards + one empty-task_id card + two duplicate-task_id cards.
	writeCard(t, dir, "defer-good-a.json", "defer-good-a", "good a", "draft", nil, nil)
	writeCard(t, dir, "defer-good-b.json", "defer-good-b", "good b", "completed", nil, nil)
	writeCard(t, dir, "defer-empty.json", "", "empty id", "draft", nil, nil)
	writeCard(t, dir, "defer-dup-1.json", "defer-dup", "dup one", "draft", nil, nil)
	writeCard(t, dir, "defer-dup-2.json", "defer-dup", "dup two", "draft", nil, nil)

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: unexpected dir-level error: %v", err)
	}

	// (1) Empty + duplicate IDs surface as CardError, not silently merged.
	// 1 empty + 2 duplicate = 3 offending cards.
	if len(reg.CardErrors) != 3 {
		t.Fatalf("CardErrors: want 3 (1 empty + 2 dup), got %d: %v", len(reg.CardErrors), reg.CardErrors)
	}
	// Each offending source path must be recoverable through CardError.Path.
	paths := map[string]bool{}
	for _, ce := range reg.CardErrors {
		paths[filepath.Base(ce.Path)] = true
	}
	if !paths["defer-empty.json"] {
		t.Errorf("CardErrors must name the empty-task_id source defer-empty.json; got paths %v", paths)
	}
	if !paths["defer-dup-1.json"] || !paths["defer-dup-2.json"] {
		t.Errorf("CardErrors must name BOTH duplicate sources; got paths %v", paths)
	}
	// Every CardError must also carry the error detail (not a nil Err).
	for _, ce := range reg.CardErrors {
		if ce.Err == nil {
			t.Errorf("CardError for %s has nil Err — detail lost", filepath.Base(ce.Path))
		}
	}

	// (2) Only the two valid unique cards survive into reg.Cards.
	if len(reg.Cards) != 2 {
		t.Fatalf("Cards: want 2 surviving, got %d", len(reg.Cards))
	}

	// (3) Invalid-card diagnostics do not themselves share record IDs: the
	// invalid cards contribute ZERO records (they are CardErrors only), and
	// every surviving record has a unique ID.
	ids := map[string]bool{}
	for _, r := range reg.Records {
		if ids[r.ID] {
			t.Errorf("duplicate record.ID %q — collision-safety broken", r.ID)
		}
		ids[r.ID] = true
	}
	// 1 note + 2 valid cards = 3 records. Invalid cards contribute none.
	if want := 1 + 2; len(reg.Records) != want {
		t.Errorf("Records: want %d (note + valid cards only), got %d", want, len(reg.Records))
	}

	// (4) Every emitted record retains a non-empty SourceRef (provenance preserved).
	for i, r := range reg.Records {
		if r.SourceRef == nil || *r.SourceRef == "" {
			t.Errorf("Records[%d].SourceRef empty — provenance lost", i)
		}
	}

	// (5) Valid unique cards retain the stable identity defer_disposition:<task_id>.
	if !ids["defer_disposition:defer-good-a"] {
		t.Errorf("missing record defer_disposition:defer-good-a (stable identity); ids=%v", ids)
	}
	if !ids["defer_disposition:defer-good-b"] {
		t.Errorf("missing record defer_disposition:defer-good-b (stable identity); ids=%v", ids)
	}
}

// TestDerive_TaskIDNormalized (defer-004) proves task_id is normalized
// (whitespace-trimmed) BEFORE identity derivation. A single card with surrounding
// whitespace survives with the TRIMMED stable identity.
func TestDerive_TaskIDNormalized(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeCard(t, dir, "defer-ws.json", "  defer-ws  ", "ws", "draft", nil, nil)

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(reg.CardErrors) != 0 {
		t.Fatalf("CardErrors: want 0, got %v", reg.CardErrors)
	}
	if len(reg.Cards) != 1 {
		t.Fatalf("Cards: want 1 surviving (normalized), got %d", len(reg.Cards))
	}
	if reg.Cards[0].TaskID != "defer-ws" {
		t.Errorf("surviving TaskID = %q, want trimmed \"defer-ws\"", reg.Cards[0].TaskID)
	}
	found := false
	for _, r := range reg.Records {
		if r.ID == "defer_disposition:defer-ws" {
			found = true
		}
	}
	if !found {
		t.Errorf("missing record defer_disposition:defer-ws (normalized identity); ids=%v", recordIDs(reg.Records))
	}
}

// TestDerive_TaskIDWhitespaceDuplicate (defer-004) proves two task_id values
// differing ONLY by surrounding whitespace collide AFTER normalization. Both
// participants are surfaced as CardError (ambiguous identity, fail-closed) and
// neither survives — the kernel does not silently pick one over the other.
func TestDerive_TaskIDWhitespaceDuplicate(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeCard(t, dir, "defer-ws-1.json", "  defer-ws  ", "ws one", "draft", nil, nil)
	writeCard(t, dir, "defer-ws-2.json", "\tdefer-ws\n", "ws two", "draft", nil, nil)

	reg, err := Derive(dir)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if len(reg.CardErrors) != 2 {
		t.Fatalf("CardErrors: want 2 (both collide after trim), got %d: %v", len(reg.CardErrors), reg.CardErrors)
	}
	if len(reg.Cards) != 0 {
		t.Fatalf("Cards: want 0 surviving (ambiguous identity, fail-closed), got %d", len(reg.Cards))
	}
	// No record is projected for the ambiguous identity.
	for _, r := range reg.Records {
		if r.ID == "defer_disposition:defer-ws" {
			t.Errorf("record projected for ambiguous identity %q; ids=%v", r.ID, recordIDs(reg.Records))
		}
	}
}

// recordIDs returns the IDs of a record slice (test helper).
func recordIDs(rs []record.Record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

// --- defer-003: coordinator-adoption marker validation (kernel projects, does not act) ---

// writeAdoptionMarker writes a canonical adoption marker payload at
// <dir>/.vh-agent-harness/coordinator-adoption.json.
func writeAdoptionMarker(t *testing.T, dir, body string) {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir marker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(d, "coordinator-adoption.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adoption marker: %v", err)
	}
}

const canonicalAdoptionMarker = `{"version":1,"adopted":true}` + "\n"

// TestLoadAdoptionMarker_Absent: no marker file → AdoptionMarkerAbsent with no
// detail. This is the greenfield signal (clean no-op), NOT a failure.
func TestLoadAdoptionMarker_Absent(t *testing.T) {
	dir := t.TempDir()
	st, detail := loadAdoptionMarker(dir)
	if st != AdoptionMarkerAbsent {
		t.Fatalf("state = %q, want %q", st, AdoptionMarkerAbsent)
	}
	if detail != "" {
		t.Errorf("absent detail should be empty, got %q", detail)
	}
}

// TestLoadAdoptionMarker_Valid: the canonical producer payload parses to
// AdoptionMarkerValid with no detail.
func TestLoadAdoptionMarker_Valid(t *testing.T) {
	dir := t.TempDir()
	writeAdoptionMarker(t, dir, canonicalAdoptionMarker)
	st, detail := loadAdoptionMarker(dir)
	if st != AdoptionMarkerValid {
		t.Fatalf("state = %q, want %q (detail=%q)", st, AdoptionMarkerValid, detail)
	}
	if detail != "" {
		t.Errorf("valid detail should be empty, got %q", detail)
	}
}

// TestLoadAdoptionMarker_CorruptUnparseable: an unreadable/unparseable body →
// AdoptionMarkerCorrupt with a detail naming the failure (fail-closed).
func TestLoadAdoptionMarker_CorruptUnparseable(t *testing.T) {
	dir := t.TempDir()
	writeAdoptionMarker(t, dir, "{not valid json")
	st, detail := loadAdoptionMarker(dir)
	if st != AdoptionMarkerCorrupt {
		t.Fatalf("state = %q, want %q", st, AdoptionMarkerCorrupt)
	}
	if !strings.Contains(detail, "parse adoption marker") {
		t.Errorf("corrupt detail should name the parse failure, got %q", detail)
	}
}

// TestLoadAdoptionMarker_CorruptWrongShape: a parseable but non-canonical
// payload (wrong version, or adopted=false) is corrupt, not silently trusted.
func TestLoadAdoptionMarker_CorruptWrongShape(t *testing.T) {
	for _, body := range []string{
		`{"version":2,"adopted":true}`,  // wrong version
		`{"version":1,"adopted":false}`, // not adopted
		`{"version":1}`,                 // adopted missing
		`{"adopted":true}`,              // version missing
	} {
		dir := t.TempDir()
		writeAdoptionMarker(t, dir, body)
		st, detail := loadAdoptionMarker(dir)
		if st != AdoptionMarkerCorrupt {
			t.Errorf("body %q: state = %q, want %q (detail=%q)", body, st, AdoptionMarkerCorrupt, detail)
		}
		if !strings.Contains(detail, "unexpected payload") {
			t.Errorf("body %q: detail should flag unexpected payload, got %q", body, detail)
		}
	}
}

// TestLoadAdoptionMarker_CorruptSupersetFields: a payload that carries the
// canonical fields PLUS an extra member must be corrupt, NOT silently accepted
// as Valid. json.Unmarshal would ignore unknown keys; the loader decodes with
// DisallowUnknownFields so the "exactly the canonical shape" contract holds and
// a tampered/forward-incompatible marker fails closed instead of falling through
// to the running gate (row 3) and potentially PASSing.
func TestLoadAdoptionMarker_CorruptSupersetFields(t *testing.T) {
	for _, body := range []string{
		`{"version":1,"adopted":true,"extra":1}`,            // extra numeric field
		`{"version":1,"adopted":true,"unexpected":"value"}`, // extra string field
		`{"version":1,"adopted":true,"note":"x"}`,           // plausible-looking extra
	} {
		dir := t.TempDir()
		writeAdoptionMarker(t, dir, body)
		st, detail := loadAdoptionMarker(dir)
		if st != AdoptionMarkerCorrupt {
			t.Errorf("body %q: state = %q, want %q (detail=%q)", body, st, AdoptionMarkerCorrupt, detail)
		}
		if !strings.Contains(detail, "parse adoption marker") {
			t.Errorf("body %q: detail should flag a parse failure (unknown field), got %q", body, detail)
		}
	}
}

// TestLoadAdoptionMarker_CorruptTrailingContent: a marker whose bytes contain a
// valid first JSON object FOLLOWED BY more content (a second JSON value or
// trailing junk) must be corrupt. json.Decoder.Decode reads only the first
// value; without an io.EOF check the trailing content would be ignored and the
// marker classified Valid — a fail-open vs the "exactly the canonical shape"
// contract.
func TestLoadAdoptionMarker_CorruptTrailingContent(t *testing.T) {
	for _, body := range []string{
		"{\"version\":1,\"adopted\":true}\n{\"extra\":1}", // trailing second JSON object
		"{\"version\":1,\"adopted\":true}garbage",         // trailing non-JSON junk
	} {
		dir := t.TempDir()
		writeAdoptionMarker(t, dir, body)
		st, detail := loadAdoptionMarker(dir)
		if st != AdoptionMarkerCorrupt {
			t.Errorf("body: state = %q, want %q (detail=%q)", st, AdoptionMarkerCorrupt, detail)
		}
		if !strings.Contains(detail, "trailing content") {
			t.Errorf("detail should flag trailing content, got %q", detail)
		}
	}
}

// TestDerive_ProjectsAdoptionMarker proves Derive (the single source the gate
// consumes) carries the marker state into the Registry for all three states.
// This is the authority-line seam: the kernel PROJECTS evidence; the gate acts.
func TestDerive_ProjectsAdoptionMarker(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		reg, err := Derive(dir)
		if err != nil {
			t.Fatalf("Derive: %v", err)
		}
		if reg.AdoptionMarker != AdoptionMarkerAbsent {
			t.Fatalf("AdoptionMarker = %q, want %q", reg.AdoptionMarker, AdoptionMarkerAbsent)
		}
	})
	t.Run("valid", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		writeAdoptionMarker(t, dir, canonicalAdoptionMarker)
		reg, err := Derive(dir)
		if err != nil {
			t.Fatalf("Derive: %v", err)
		}
		if reg.AdoptionMarker != AdoptionMarkerValid {
			t.Fatalf("AdoptionMarker = %q, want %q", reg.AdoptionMarker, AdoptionMarkerValid)
		}
	})
	t.Run("corrupt", func(t *testing.T) {
		dir := t.TempDir()
		gitInit(t, dir)
		writeAdoptionMarker(t, dir, "garbage")
		reg, err := Derive(dir)
		if err != nil {
			t.Fatalf("Derive: %v", err)
		}
		if reg.AdoptionMarker != AdoptionMarkerCorrupt {
			t.Fatalf("AdoptionMarker = %q, want %q", reg.AdoptionMarker, AdoptionMarkerCorrupt)
		}
		if reg.AdoptionMarkerDetail == "" {
			t.Errorf("corrupt AdoptionMarkerDetail should be non-empty")
		}
	})
}
