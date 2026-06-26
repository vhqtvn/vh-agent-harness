package proposals

// This file hardens the Slice-5.3 proposal ledger with dedicated unit tests.
// Until now the ledger (Append/Read JSONL under .vh-agent-harness/proposals.jsonl)
// was exercised only indirectly through the seam apply path; these tests pin its
// contract directly: JSONL record shape, append-only accumulation, insertion-
// order reads, the 1MB per-line cap, timestamp/ref stamping, and the
// external_generated path layout (the ledger is harness-created/maintained, not
// a platform template, and lives under the lineage dir).

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// sampleRecords builds n deterministic records carrying a structured enum_removed
// proposal each, so tests can assert round-trip of every Proposal field.
func sampleRecords(n int) []Record {
	out := make([]Record, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Record{
			Path:  "vh-harness-profile.yml",
			Class: "platform_armed",
			Proposals: []schema.Proposal{{
				Field:         "profile",
				Kind:          "enum_removed",
				PlatformValue: "supervised",
				ProjectValue:  "experimental",
				Envelope:      "enum: minimal | coordination | supervised",
				Hint:          "choose a value still in the platform enum",
			}},
		})
	}
	return out
}

// --- Append + Read round-trip ------------------------------------------------

// TestAppend_WritesValidJSONLRecord confirms Append writes exactly one JSON line
// per record, each line carrying the timestamp, the harness ref, the path/class,
// and the full structured proposal (every Proposal field round-trips).
func TestAppend_WritesValidJSONLRecord(t *testing.T) {
	target := t.TempDir()
	const ref = "harness/v0.9.9-test"

	written, err := Append(target, ref, sampleRecords(1))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if written != 1 {
		t.Errorf("Append returned n=%d, want 1", written)
	}

	raw, err := os.ReadFile(FilePath(target))
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("ledger line count: got %d, want 1 (one JSON line per record)", len(lines))
	}

	// Each line is a standalone valid JSON object.
	var rec Record
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("line is not valid JSON: %v\nline=%q", err, lines[0])
	}

	// Ref + timestamp stamped by Append.
	if rec.Ref != ref {
		t.Errorf("Ref: got %q, want %q", rec.Ref, ref)
	}
	if rec.Timestamp == "" {
		t.Error("Timestamp empty; Append must stamp an RFC3339 time")
	}
	if _, perr := time.Parse(time.RFC3339, rec.Timestamp); perr != nil {
		t.Errorf("Timestamp not RFC3339: %v (%q)", perr, rec.Timestamp)
	}

	// Path + class + proposal fields round-trip.
	if rec.Path != "vh-harness-profile.yml" {
		t.Errorf("Path: got %q", rec.Path)
	}
	if rec.Class != "platform_armed" {
		t.Errorf("Class: got %q", rec.Class)
	}
	if len(rec.Proposals) != 1 {
		t.Fatalf("Proposals: got %d, want 1", len(rec.Proposals))
	}
	p := rec.Proposals[0]
	for _, c := range []struct{ field, want string }{
		{"Field", "profile"},
		{"Kind", "enum_removed"},
		{"Envelope", "enum: minimal | coordination | supervised"},
		{"Hint", "choose a value still in the platform enum"},
	} {
		got := proposalStringField(p, c.field)
		if got != c.want {
			t.Errorf("Proposal.%s: got %q, want %q", c.field, got, c.want)
		}
	}
	if pv, _ := p.PlatformValue.(string); pv != "supervised" {
		t.Errorf("Proposal.PlatformValue: got %v, want supervised", p.PlatformValue)
	}
	if pv, _ := p.ProjectValue.(string); pv != "experimental" {
		t.Errorf("Proposal.ProjectValue: got %v, want experimental", p.ProjectValue)
	}
}

// proposalStringField reads a string Proposal field by name for compact asserts.
func proposalStringField(p schema.Proposal, name string) string {
	switch name {
	case "Field":
		return p.Field
	case "Kind":
		return p.Kind
	case "Envelope":
		return p.Envelope
	case "Hint":
		return p.Hint
	}
	return ""
}

// TestRead_ReturnsRecordsInOrder confirms Read returns records in insertion
// order across multiple distinct proposals in a single Append call.
func TestRead_ReturnsRecordsInOrder(t *testing.T) {
	target := t.TempDir()
	records := []Record{
		{Path: "a.yml", Class: "platform_armed", Proposals: []schema.Proposal{{Field: "a", Kind: "enum_removed", ProjectValue: "x"}}},
		{Path: "b.yml", Class: "platform_armed", Proposals: []schema.Proposal{{Field: "b", Kind: "enum_removed", ProjectValue: "y"}}},
		{Path: "c.yml", Class: "platform_armed", Proposals: []schema.Proposal{{Field: "c", Kind: "enum_removed", ProjectValue: "z"}}},
	}
	if _, err := Append(target, "harness/v1", records); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := Read(target)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(records) {
		t.Fatalf("Read count: got %d, want %d", len(got), len(records))
	}
	for i, want := range records {
		if got[i].Path != want.Path {
			t.Errorf("Read[%d].Path: got %q, want %q (insertion order)", i, got[i].Path, want.Path)
		}
	}
}

// --- Append-only accumulation ----------------------------------------------

// TestAppend_AppendOnlyAccumulates confirms repeated Append calls accumulate:
// prior entries are preserved across updates (the ledger is append-only across
// the harness lifetime; trimming is an explicit operator action).
func TestAppend_AppendOnlyAccumulates(t *testing.T) {
	target := t.TempDir()
	if _, err := Append(target, "harness/v1", sampleRecords(2)); err != nil {
		t.Fatal(err)
	}
	// A later update appends more conflicts; the first two must survive.
	if _, err := Append(target, "harness/v2", sampleRecords(3)); err != nil {
		t.Fatal(err)
	}
	got, err := Read(target)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("after two appends: got %d records, want 5 (append-only)", len(got))
	}
	// The ref of later records reflects the second append.
	if got[4].Ref != "harness/v2" {
		t.Errorf("later record ref: got %q, want harness/v2", got[4].Ref)
	}
}

// --- 1MB line cap -----------------------------------------------------------

// TestRead_LineCapRejectsOversizeRecord confirms a record whose JSON line
// exceeds the 1MB per-line scanner cap is rejected (Read returns an error),
// while records parsed before the oversize line are still returned. Append
// itself does not enforce the cap — the limit lives in the Read scanner buffer.
func TestRead_LineCapRejectsOversizeRecord(t *testing.T) {
	target := t.TempDir()
	// Build a record whose marshaled JSON line is comfortably over 1MB.
	huge := strings.Repeat("x", 1_100_000)
	oversize := []Record{{
		Path: "big.yml",
		Proposals: []schema.Proposal{{
			Field: "profile",
			Kind:  "enum_removed",
			Hint:  huge,
		}},
	}}
	if _, err := Append(target, "harness/v1", oversize); err != nil {
		t.Fatalf("Append oversize: %v", err)
	}
	got, err := Read(target)
	if err == nil {
		t.Fatal("Read: want error for >1MB line, got nil")
	}
	// The error must be the bufio too-long error (the cap firing).
	if !strings.Contains(err.Error(), "too long") && err != bufio.ErrTooLong {
		// Allow either the wrapped message or the sentinel; both are acceptable.
		if !strings.Contains(err.Error(), "scan ledger") {
			t.Errorf("error should be the line-cap/scan error; got %v", err)
		}
	}
	// No record returned for a single oversize line.
	if len(got) != 0 {
		t.Errorf("oversize-only ledger: got %d records, want 0", len(got))
	}
}

// TestRead_LineCapKeepsRecordsBeforeOversize confirms records on lines BEFORE an
// oversize line are still returned (Read parses left-to-right and surfaces the
// cap error only for the offending line).
func TestRead_LineCapKeepsRecordsBeforeOversize(t *testing.T) {
	target := t.TempDir()
	// First a normal record, then an oversize one.
	if _, err := Append(target, "harness/v1", sampleRecords(1)); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("y", 1_100_000)
	if _, err := Append(target, "harness/v1", []Record{{
		Path:      "big.yml",
		Proposals: []schema.Proposal{{Field: "f", Kind: "enum_removed", Hint: huge}},
	}}); err != nil {
		t.Fatal(err)
	}
	got, err := Read(target)
	if err == nil {
		t.Fatal("Read: want error for the oversize line, got nil")
	}
	if len(got) != 1 {
		t.Errorf("records before oversize line: got %d, want 1", len(got))
	}
	if got[0].Path != "vh-harness-profile.yml" {
		t.Errorf("first record path: got %q", got[0].Path)
	}
}

// --- timestamp + ref stamping ----------------------------------------------

// TestAppend_StampsTimestampAndRef confirms Append fills in Timestamp + Ref when
// a record omits them, but preserves caller-supplied values when present (so a
// replay/back-fill can carry its own provenance).
func TestAppend_StampsTimestampAndRef(t *testing.T) {
	target := t.TempDir()
	records := []Record{
		{Path: "blank.yml", Class: "platform_armed", Proposals: []schema.Proposal{{Field: "f", Kind: "enum_removed"}}},
		{Path: "preset.yml", Class: "platform_armed", Timestamp: "2025-01-01T00:00:00Z", Ref: "harness/backfill", Proposals: []schema.Proposal{{Field: "f", Kind: "enum_removed"}}},
	}
	if _, err := Append(target, "harness/v1", records); err != nil {
		t.Fatal(err)
	}
	got, err := Read(target)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	// Blank record stamped with the append ref + a fresh RFC3339 timestamp.
	if got[0].Ref != "harness/v1" {
		t.Errorf("blank record Ref: got %q, want harness/v1 (stamped)", got[0].Ref)
	}
	if got[0].Timestamp == "" {
		t.Error("blank record Timestamp not stamped")
	}
	// Preset record preserved caller values (not overwritten).
	if got[1].Ref != "harness/backfill" {
		t.Errorf("preset record Ref overwritten: got %q, want harness/backfill", got[1].Ref)
	}
	if got[1].Timestamp != "2025-01-01T00:00:00Z" {
		t.Errorf("preset record Timestamp overwritten: got %q", got[1].Timestamp)
	}
}

// --- no-op + missing-ledger edge cases -------------------------------------

// TestAppend_EmptyRecordsIsNoOp confirms an empty records slice writes nothing
// and creates no ledger file.
func TestAppend_EmptyRecordsIsNoOp(t *testing.T) {
	target := t.TempDir()
	n, err := Append(target, "harness/v1", nil)
	if err != nil {
		t.Fatalf("Append(nil): %v", err)
	}
	if n != 0 {
		t.Errorf("Append(nil) returned %d, want 0", n)
	}
	if _, statErr := os.Stat(FilePath(target)); !os.IsNotExist(statErr) {
		t.Errorf("empty Append must not create the ledger; statErr=%v", statErr)
	}
}

// TestRead_MissingLedgerReturnsNil confirms a missing ledger returns (nil, nil)
// — the common, healthy case (no conflicts recorded yet).
func TestRead_MissingLedgerReturnsNil(t *testing.T) {
	target := t.TempDir()
	got, err := Read(target)
	if err != nil {
		t.Errorf("Read on missing ledger: want nil err, got %v", err)
	}
	if got != nil {
		t.Errorf("Read on missing ledger: want nil records, got %d", len(got))
	}
}

// TestRead_ToleratesTrailingBlankLine confirms Read skips a trailing blank line
// (Append always terminates the last record with '\n', leaving a final empty
// line that a strict line-splitter would misread as a record).
func TestRead_ToleratesTrailingBlankLine(t *testing.T) {
	target := t.TempDir()
	if _, err := Append(target, "harness/v1", sampleRecords(1)); err != nil {
		t.Fatal(err)
	}
	got, err := Read(target)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("trailing blank line mishandled: got %d records, want 1", len(got))
	}
}

// --- external_generated path layout ----------------------------------------

// TestFilePath_LivesUnderLineageDir confirms FilePath returns the ledger under
// the lineage dir (<target>/.vh-agent-harness/proposals.jsonl). The ledger is
// external_generated: created and maintained by the harness at apply time, never
// a platform template, never rendered or owned by a pack.
func TestFilePath_LivesUnderLineageDir(t *testing.T) {
	target := t.TempDir()
	got := FilePath(target)
	want := filepath.Join(target, lineage.DirName, FileName)
	if got != want {
		t.Errorf("FilePath: got %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, filepath.Join(lineage.DirName, FileName)) {
		t.Errorf("FilePath must end with %s/%s; got %q", lineage.DirName, FileName, got)
	}
	if FileName != "proposals.jsonl" {
		t.Errorf("FileName constant drifted: got %q", FileName)
	}
}

// TestAppend_CreatesLineageDirIfMissing confirms Append creates the lineage dir
// when it does not exist (the external_generated ledger is born on the first
// conflict surface, not pre-seeded by install).
func TestAppend_CreatesLineageDirIfMissing(t *testing.T) {
	target := t.TempDir()
	// Confirm the lineage dir is absent before the first append.
	if _, err := os.Stat(filepath.Join(target, lineage.DirName)); !os.IsNotExist(err) {
		t.Fatalf("lineage dir should be absent before first append: %v", err)
	}
	if _, err := Append(target, "harness/v1", sampleRecords(1)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// The lineage dir + ledger now exist.
	if _, err := os.Stat(filepath.Join(target, lineage.DirName)); err != nil {
		t.Errorf("lineage dir not created by Append: %v", err)
	}
	if _, err := os.Stat(FilePath(target)); err != nil {
		t.Errorf("ledger not created by Append: %v", err)
	}
}
