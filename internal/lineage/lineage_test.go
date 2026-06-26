package lineage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestLineage_RoundTrip verifies Write then Read returns an equal Lineage.
func TestLineage_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	orig := Seed("embedded-corpus://agent-harness", map[string]string{
		"project_name":    "Demo",
		"project_slug":    "demo",
		"coordinator_dir": "coordinator",
	}, "test")

	if err := orig.Write(tmp); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err := os.Stat(FilePath(tmp)); err != nil {
		t.Fatalf("lineage file not written: %v", err)
	}

	got, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Normalize the volatile timestamp before deep-equal; it legitimately moves
	// with wall-clock and is not part of the lineage contract.
	orig.Render.LastSuccessfulRenderAt = got.Render.LastSuccessfulRenderAt
	if !reflect.DeepEqual(orig, got) {
		t.Fatalf("round-trip mismatch:\nwant %+v\ngot  %+v", orig, got)
	}
}

// TestLineage_SeedRecordsRawAnswerValues confirms Seed captures the raw
// install-identity answers into AnswersRef.Values (so doctor/update can recover
// them for a faithful re-render) and that the capture is a defensive copy —
// mutating the source map after Seed must not change the recorded values.
func TestLineage_SeedRecordsRawAnswerValues(t *testing.T) {
	answers := map[string]string{
		"project_name":    "My Project",
		"project_slug":    "my-project",
		"coordinator_dir": "coordinator",
	}
	lin := Seed("embedded-corpus://agent-harness", answers, "test")
	if lin.Answers.Values["project_name"] != "My Project" {
		t.Errorf("Values[project_name]: got %q want %q", lin.Answers.Values["project_name"], "My Project")
	}
	if lin.Answers.Values["project_slug"] != "my-project" {
		t.Errorf("Values[project_slug]: got %q want %q", lin.Answers.Values["project_slug"], "my-project")
	}
	if lin.Answers.Values["coordinator_dir"] != "coordinator" {
		t.Errorf("Values[coordinator_dir]: got %q want %q", lin.Answers.Values["coordinator_dir"], "coordinator")
	}
	// Defensive copy: mutating the caller's map after Seed must not leak in.
	answers["project_name"] = "MUTATED"
	if lin.Answers.Values["project_name"] != "My Project" {
		t.Errorf("Values aliases the caller's map (not a defensive copy): got %q want %q",
			lin.Answers.Values["project_name"], "My Project")
	}
}

// TestLineage_WriteProducesDeterministicSelectedKeys confirms the selected_keys
// list is sorted + de-duplicated on write (identical lineage values produce
// identical selected_keys bytes).
func TestLineage_WriteProducesDeterministicSelectedKeys(t *testing.T) {
	l := &Lineage{
		LineageVersion: LineageVersion,
		Answers: AnswersRef{
			Digest:       "sha256:deadbeef",
			SelectedKeys: []string{"zeta", "alpha", "alpha", "mu"},
		},
	}
	tmp := t.TempDir()
	if err := l.Write(tmp); err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := []string{"alpha", "mu", "zeta"}
	if !reflect.DeepEqual(l.Answers.SelectedKeys, want) {
		t.Fatalf("selected_keys not sorted/deduped: got %v want %v", l.Answers.SelectedKeys, want)
	}
}

// TestLineage_MissingReturnsNil mirrors manifest.Find's "absent vs broken"
// contract: a missing file is a normal state (returns nil, nil), not an error.
func TestLineage_MissingReturnsNil(t *testing.T) {
	tmp := t.TempDir()
	got, err := Read(tmp)
	if err != nil {
		t.Fatalf("Read on missing file should be nil,nil; got err=%v", err)
	}
	if got != nil {
		t.Fatalf("Read on missing file should be nil; got %+v", got)
	}
}

// TestLineage_UnparseableReturnsError: a present-but-broken file is an error
// (distinguishes "absent" from "corrupt").
func TestLineage_UnparseableReturnsError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(FilePath(tmp), []byte("::: not ::: yaml :::"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(tmp); err == nil {
		t.Fatal("Read on unparseable file should error")
	}
}

// TestLineage_DigestOf_Deterministic confirms the digest is order-independent
// and changes when answers change.
func TestLineage_DigestOf_Deterministic(t *testing.T) {
	a := map[string]string{"project_name": "Demo", "project_slug": "demo"}
	b := map[string]string{"project_slug": "demo", "project_name": "Demo"} // reordered
	if DigestOf(a) != DigestOf(b) {
		t.Fatalf("digest must be order-independent:\n  %q\n  %q", DigestOf(a), DigestOf(b))
	}
	c := map[string]string{"project_name": "Other", "project_slug": "demo"} // value change
	if DigestOf(a) == DigestOf(c) {
		t.Fatal("digest must change when a value changes")
	}
	d := map[string]string{"project_name": "Demo"} // key set change
	if DigestOf(a) == DigestOf(d) {
		t.Fatal("digest must change when a key is removed")
	}
	if !strings.HasPrefix(DigestOf(a), "sha256:") {
		t.Fatalf("digest should be sha256-prefixed: %q", DigestOf(a))
	}
}

// TestLineage_UpdateID_DeterministicAndStable confirms content-addressing.
func TestLineage_UpdateID_DeterministicAndStable(t *testing.T) {
	d := "sha256:abc"
	if UpdateID(d, "v1") != UpdateID(d, "v1") {
		t.Fatal("UpdateID must be deterministic for identical inputs")
	}
	if UpdateID(d, "v1") == UpdateID(d, "v2") {
		t.Fatal("UpdateID must change when the vh-agent-harness version changes")
	}
	if len(UpdateID(d, "v1")) != 16 {
		t.Fatalf("UpdateID should be 16 hex chars; got %d", len(UpdateID(d, "v1")))
	}
}

// TestAssertLineageOnly_AcceptsSeededLineage: the real seeded lineage must pass
// the authority-hygiene guard (only S1 facts present).
func TestAssertLineageOnly_AcceptsSeededLineage(t *testing.T) {
	tmp := t.TempDir()
	l := Seed("embedded-corpus://agent-harness", map[string]string{
		"project_name": "Demo",
	}, "test")
	if err := l.Write(tmp); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(FilePath(tmp))
	if err != nil {
		t.Fatal(err)
	}
	if err := AssertLineageOnly(raw); err != nil {
		t.Fatalf("seeded lineage must satisfy AssertLineageOnly: %v", err)
	}
}

// TestAssertLineageOnly_RejectsS2S3S4Leaks is the authority-hygiene test for
// closeout #4. A lineage file carrying S2/S3/S4 facts must be rejected, and the
// error must name the leaked surface so the failure is self-explaining. This
// proves a lineage file (parsed) cannot answer "which profile?" (S3), "is this
// file safe to overwrite?" (S2), or "which services run?" (S4).
func TestAssertLineageOnly_RejectsS2S3S4Leaks(t *testing.T) {
	poisoned := []byte(`lineage_version: "1"
template: {source: x}
copier: {version: ""}
answers: {digest: sha256:x}
render: {last_successful_update_id: x}
# ---- forbidden authority leaks below ----
profile: minimal            # S3 - profile selection
modules: [a, b]             # S3 - feature surface
ownership_overrides: {}     # S2 - ownership classification
classes: [managed]          # S2 - ownership classes
services: {web: {}}         # S4 - runtime services
runtime: {backend: host}    # S4 - runtime shape
hooks: {pre: []}            # S4 - lifecycle hooks
`)
	err := AssertLineageOnly(poisoned)
	if err == nil {
		t.Fatal("AssertLineageOnly must reject a lineage file with S2/S3/S4 leaks")
	}
	for _, surface := range []string{"S2", "S3", "S4"} {
		if !strings.Contains(err.Error(), surface) {
			t.Errorf("error should name the leaked surface %q; got: %v", surface, err)
		}
	}
	for _, leaked := range []string{"profile", "ownership_overrides", "services"} {
		if !strings.Contains(err.Error(), leaked) {
			t.Errorf("error should name the leaked key %q; got: %v", leaked, err)
		}
	}
}

// TestAssertLineageOnly_RejectsUnknownKey: a key that is neither allowed nor in
// the known-forbidden map is still rejected (defensive: only the documented S1
// facts are permitted).
func TestAssertLineageOnly_RejectsUnknownKey(t *testing.T) {
	unknown := []byte(`lineage_version: "1"
surprise_field: 42
`)
	err := AssertLineageOnly(unknown)
	if err == nil {
		t.Fatal("AssertLineageOnly must reject unknown keys")
	}
	if !strings.Contains(err.Error(), "surprise_field") {
		t.Errorf("error should name the unknown key; got: %v", err)
	}
}
