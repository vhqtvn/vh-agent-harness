package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/record"
)

// strPtr mirrors the helper in the record package tests but is local to avoid
// importing test helpers across packages.
func strPtr(s string) *string { return &s }

// newRec is a convenience constructor for tests: it fills the required fields
// and returns a valid record with sensible defaults.
func newRec(id string, t record.Type, p record.Priority, body string, updatedAt time.Time) record.Record {
	return record.Record{
		ID:         id,
		Type:       t,
		Priority:   p,
		Scope:      record.ScopeSession,
		CreatedAt:  updatedAt,
		UpdatedAt:  updatedAt,
		Body:       body,
		SessionKey: strPtr("build-memory"),
		Workstream: strPtr("typed-memory-layer"),
		SourceRef:  strPtr("docs/checkpoints/x.md"),
	}
}

// TestAppendProducesOneJSONLinePerRecord verifies that appending N records
// yields exactly N non-empty JSONL lines and that re-reading decodes the same
// records.
func TestAppendProducesOneJSONLinePerRecord(t *testing.T) {
	root := t.TempDir()
	const alias = "session-a"

	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		rec := newRec("rec-"+itoa(i), record.TypeEpisodic, record.PriorityNormal, "body "+itoa(i), ts.Add(time.Duration(i)*time.Minute))
		if err := AppendSession(root, alias, rec); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	// Count lines on disk to prove the file is strict JSONL.
	path := SessionRecordsPath(root, alias)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	trimmed := bytes.TrimRight(data, "\n")
	lines := bytes.Split(trimmed, []byte("\n"))
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d (%q)", len(lines), data)
	}

	got, stats, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 records read, got %d", len(got))
	}
	if stats.LinesScanned != 4 || stats.Malformed != 0 {
		t.Fatalf("stats wrong: %+v", stats)
	}
}

// TestAppendWorkstreamAndRead mirrors the session append test for the
// workstream path, asserting the second locked path also works symmetrically.
func TestAppendWorkstreamAndRead(t *testing.T) {
	root := t.TempDir()
	const slug = "memory-layer"

	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	rec := newRec("ws-1", record.TypeInstruction, record.PriorityHigh, "ship slice 2", ts)
	rec.Scope = record.ScopeWorkstream
	if err := AppendWorkstream(root, slug, rec); err != nil {
		t.Fatalf("append: %v", err)
	}

	// The workstream file lives directly under the slug directory, not under
	// a memory/ subdir. Verify the path helper agrees with reality.
	path := WorkstreamRecordsPath(root, slug)
	if got, want := path, filepath.Join(root, StateDir, WorkstreamsDir, slug, RecordsFile); got != want {
		t.Fatalf("path mismatch: got %s want %s", got, want)
	}
	got, _, err := ReadWorkstream(root, slug, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ws-1" {
		t.Fatalf("unexpected read: %+v", got)
	}
}

// TestAppendValidatesBeforeWrite asserts that an invalid record is rejected
// AND leaves the file untouched (no partial line written).
func TestAppendValidatesBeforeWrite(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	bad := newRec("", record.TypePersona, record.PriorityNormal, "no id", time.Now())
	err := AppendSession(root, alias, bad)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if _, statErr := os.Stat(SessionRecordsPath(root, alias)); !os.IsNotExist(statErr) {
		t.Fatalf("file should not exist after a rejected append, got err=%v", statErr)
	}
}

// TestAppendRejectsOversizedLine guards the storage self-consistency
// invariant: a record whose encoded JSONL line exceeds the reader's
// maxLineBytes cap must be rejected on the write path before any file or
// directory is created. Session and workstream append share the single
// appendTo helper, so exercising the session path covers both entry points.
func TestAppendRejectsOversizedLine(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	// Body of maxLineBytes alone already makes the full encoded line (Body
	// plus the JSON field names and the other record fields) exceed the cap.
	rec := newRec("too-big", record.TypePersona, record.PriorityNormal, strings.Repeat("x", maxLineBytes), time.Now())
	err := AppendSession(root, alias, rec)
	if err == nil {
		t.Fatal("expected oversized-line error")
	}
	if _, statErr := os.Stat(SessionRecordsPath(root, alias)); !os.IsNotExist(statErr) {
		t.Fatalf("file should not exist after a rejected append, got err=%v", statErr)
	}
}

// TestAppendFsyncsDirOnCreateOnly is the regression test for the create-path
// parent-directory fsync (D1 item 3: "fsync parent dir on create"). It proves:
//
//   - On the FIRST append to a brand-new records.jsonl, fsyncDir fires exactly
//     once (the create path actually executes — the data file's parent
//     directory is fsync'd so the directory entry referencing the new file
//     survives a crash).
//   - On a SECOND append to the SAME (now-existing) file, fsyncDir does NOT
//     fire again (the create path is correctly gated on the stat-before-write
//     create flag, so subsequent appends avoid a redundant dir fsync).
//
// Why this is a real regression test (fails on OLD code, passes on NEW):
// The prior implementation stat'd the file AFTER the write to detect creation.
// Because write(2) to an O_APPEND regular file advances the inode size
// synchronously before returning, and marshalLine always emits at least 3
// bytes (json.Encoder.Encode yields at minimum "{}\n"), that post-write stat
// always observed size > 0. That made `fi.Size() == 0` permanently false, so
// the fsyncDir call was unreachable dead code and the parent directory was
// NEVER fsync'd on create. With that code, dirSyncCount stays 0 on BOTH
// appends and this test FAILS at the very first assertion. The fix stat's
// BEFORE the write (under the combined lock), so a zero-size file reliably
// means "this open created it"; the first append observes size == 0, sets
// created = true, and the gated fsyncDir fires — count reaches 1, test PASSES.
//
// fsyncDir is a package-level var swapped here with an atomic-counting
// observer that still calls through to the real implementation (so the
// tempdir's directory entry is genuinely fsync'd — harmless, and keeps the
// test honest about side effects). The original is restored via defer so no
// other test is affected. No t.Parallel: package-level var swap is safe only
// under Go's default sequential test execution.
func TestAppendFsyncsDirOnCreateOnly(t *testing.T) {
	origDirSync := fsyncDir
	defer func() { fsyncDir = origDirSync }()

	var dirSyncCount int32
	fsyncDir = func(dir string) error {
		atomic.AddInt32(&dirSyncCount, 1)
		return origDirSync(dir)
	}

	root := t.TempDir()
	const alias = "dirsync"
	ts := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)

	// First append: creates records.jsonl → fsyncDir MUST fire exactly once.
	if err := AppendSession(root, alias, newRec("first", record.TypePersona, record.PriorityNormal, "one", ts)); err != nil {
		t.Fatalf("first append: %v", err)
	}
	if got := atomic.LoadInt32(&dirSyncCount); got != 1 {
		t.Fatalf("after first append (create path): expected dirSyncCount=1, got %d — the create-path dir fsync did not fire (dead code)", got)
	}

	// Second append to the SAME file: file exists → fsyncDir MUST NOT fire.
	if err := AppendSession(root, alias, newRec("second", record.TypePersona, record.PriorityNormal, "two", ts.Add(time.Second))); err != nil {
		t.Fatalf("second append: %v", err)
	}
	if got := atomic.LoadInt32(&dirSyncCount); got != 1 {
		t.Fatalf("after second append (no create): expected dirSyncCount=1 (unchanged), got %d — dir fsync fired on a non-create append", got)
	}
}

// TestLastWriteWinsByUpdatedAt verifies that appending a record with an
// existing ID and a newer UpdatedAt supersedes the older line on read.
func TestLastWriteWinsByUpdatedAt(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	t0 := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)

	v1 := newRec("dup", record.TypeEpisodic, record.PriorityNormal, "first", t0)
	v2 := newRec("dup", record.TypeEpisodic, record.PriorityNormal, "second", t1)
	v2.Body = "second-body"

	if err := AppendSession(root, alias, v1); err != nil {
		t.Fatalf("append v1: %v", err)
	}
	if err := AppendSession(root, alias, v2); err != nil {
		t.Fatalf("append v2: %v", err)
	}

	got, stats, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 deduped record, got %d", len(got))
	}
	if got[0].Body != "second-body" {
		t.Fatalf("expected newer body to win, got %q", got[0].Body)
	}
	if stats.LinesScanned != 2 {
		t.Fatalf("expected 2 lines scanned, got %d", stats.LinesScanned)
	}
}

// TestLastWriteWinsFileOrderTie verifies that when two lines for the same ID
// share an identical UpdatedAt, the later line in the file wins (true
// last-write-wins, not first-seen).
func TestLastWriteWinsFileOrderTie(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	r1 := newRec("dup", record.TypePersona, record.PriorityLow, "older-line-pos", ts)
	r1.Body = "first-in-file"
	r2 := newRec("dup", record.TypePersona, record.PriorityLow, "newer-line-pos", ts)
	r2.Body = "last-in-file"

	if err := AppendSession(root, alias, r1); err != nil {
		t.Fatal(err)
	}
	if err := AppendSession(root, alias, r2); err != nil {
		t.Fatal(err)
	}
	got, _, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].Body != "last-in-file" {
		t.Fatalf("expected last-in-file to win on tie, got %+v", got)
	}
}

// TestReaderFilters exercises every Query filter: scope, session_key,
// workstream, type, priority, and both keyword fields.
func TestReaderFilters(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	recs := []record.Record{
		func() record.Record {
			r := newRec("a", record.TypePersona, record.PriorityHigh, "alpha body", ts)
			r.SessionKey = strPtr("sess-A")
			r.Workstream = nil
			r.Scope = record.ScopeSession
			return r
		}(),
		func() record.Record {
			r := newRec("b", record.TypeEpisodic, record.PriorityNormal, "beta body", ts)
			r.SessionKey = strPtr("sess-B")
			r.Workstream = strPtr("ws-B")
			r.Scope = record.ScopeWorkstream
			r.SourceRef = strPtr("cmd://grep/foo")
			return r
		}(),
	}
	for _, r := range recs {
		if err := AppendSession(root, alias, r); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	cases := []struct {
		name   string
		q      Query
		wantID string
	}{
		{"by type persona", Query{Type: record.TypePersona}, "a"},
		{"by type episodic", Query{Type: record.TypeEpisodic}, "b"},
		{"by priority high", Query{Priority: record.PriorityHigh}, "a"},
		{"by scope session", Query{Scope: record.ScopeSession}, "a"},
		{"by scope workstream", Query{Scope: record.ScopeWorkstream}, "b"},
		{"by session_key", Query{SessionKey: "sess-B"}, "b"},
		{"by workstream", Query{Workstream: "ws-B"}, "b"},
		{"by body keyword", Query{BodyKeyword: "ALPHA"}, "a"},
		{"by source_ref keyword", Query{SourceRefKeyword: "grep"}, "b"},
		{"no filter returns both", Query{}, "a"}, // 'a' first by sort (high priority)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := ReadSession(root, alias, tc.q)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if tc.wantID == "" {
				if len(got) != 0 {
					t.Fatalf("expected no matches, got %+v", got)
				}
				return
			}
			// For the "no filter" case there are 2 records; just check the
			// expected one is present.
			found := false
			for _, r := range got {
				if r.ID == tc.wantID {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected to find %s in results: %+v", tc.wantID, got)
			}
		})
	}

	// Negative: a filter matching nothing yields an empty slice.
	t.Run("filter matches nothing", func(t *testing.T) {
		got, _, err := ReadSession(root, alias, Query{Workstream: "no-such-ws"})
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected empty result, got %+v", got)
		}
	})
}

// TestReaderSortOrder verifies the primary (Priority desc) and secondary
// (UpdatedAt desc) sort keys.
func TestReaderSortOrder(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	base := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	// Insert out of order to prove the sort is the reader's doing.
	recs := []record.Record{
		newRec("low-new", record.TypePersona, record.PriorityLow, "x", base.Add(2*time.Hour)),
		newRec("crit-old", record.TypePersona, record.PriorityCritical, "x", base),
		newRec("high-mid", record.TypePersona, record.PriorityHigh, "x", base.Add(time.Hour)),
		newRec("crit-older", record.TypePersona, record.PriorityCritical, "x", base.Add(-time.Hour)),
	}
	for _, r := range recs {
		if err := AppendSession(root, alias, r); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, _, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Expected: crit-old (newer critical) > crit-older (older critical) >
	// high-mid > low-new.
	wantOrder := []string{"crit-old", "crit-older", "high-mid", "low-new"}
	if len(got) != len(wantOrder) {
		t.Fatalf("expected %d records, got %d (%+v)", len(wantOrder), len(got), got)
	}
	for i, want := range wantOrder {
		if got[i].ID != want {
			t.Fatalf("position %d: want %s got %s; full=%+v", i, want, got[i].ID, got)
		}
	}
}

// TestReaderCapsAtMaxRecords asserts that a scan with more records than
// MaxRecords returns at most MaxRecords (after sort, so the top-priority
// records survive the cap).
func TestReaderCapsAtMaxRecords(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	base := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	// Write MaxRecords + 5 low-priority records, then one critical record.
	for i := 0; i < MaxRecords+5; i++ {
		r := newRec("low-"+itoa(i), record.TypePersona, record.PriorityLow, "bulk", base.Add(time.Duration(i)*time.Second))
		if err := AppendSession(root, alias, r); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	crit := newRec("crit-top", record.TypePersona, record.PriorityCritical, "top", base.Add(time.Hour))
	if err := AppendSession(root, alias, crit); err != nil {
		t.Fatalf("append crit: %v", err)
	}

	got, _, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != MaxRecords {
		t.Fatalf("expected cap at %d, got %d", MaxRecords, len(got))
	}
	// The critical record must survive at position 0 because sort happens
	// before the cap.
	if got[0].ID != "crit-top" {
		t.Fatalf("expected critical record at index 0, got %s", got[0].ID)
	}
}

// TestReaderHonorsExplicitLimit verifies that a smaller Query.Limit is
// honored, and that it is clamped to MaxRecords when larger.
func TestReaderHonorsExplicitLimit(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	base := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		r := newRec("r-"+itoa(i), record.TypePersona, record.PriorityNormal, "b", base.Add(time.Duration(i)*time.Second))
		if err := AppendSession(root, alias, r); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, _, err := ReadSession(root, alias, Query{Limit: 2})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected limit 2, got %d", len(got))
	}

	got2, _, err := ReadSession(root, alias, Query{Limit: MaxRecords + 100})
	if err != nil {
		t.Fatalf("read oversized limit: %v", err)
	}
	if len(got2) != 5 {
		t.Fatalf("expected clamp to actual count 5, got %d", len(got2))
	}
}

// TestReaderFaultToleranceMalformedLine verifies that a malformed line in the
// middle of the file is skipped and counted, while valid lines around it are
// still returned.
func TestReaderFaultToleranceMalformedLine(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	if err := AppendSession(root, alias, newRec("good-1", record.TypePersona, record.PriorityNormal, "one", ts)); err != nil {
		t.Fatal(err)
	}

	// Manually append a malformed line and a second good line.
	path := SessionRecordsPath(root, alias)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{this is not valid json\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if err := AppendSession(root, alias, newRec("good-2", record.TypePersona, record.PriorityNormal, "two", ts.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}

	got, stats, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("expected no fatal error, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid records, got %d (%+v)", len(got), got)
	}
	if stats.Malformed != 1 {
		t.Fatalf("expected 1 malformed line counted, got %d", stats.Malformed)
	}
	if stats.LinesScanned != 3 {
		t.Fatalf("expected 3 lines scanned, got %d", stats.LinesScanned)
	}
}

// TestReaderSkipsOverlongLine verifies the "never fatal" contract for a line
// whose length exceeds maxLineBytes. The package doc promises that malformed
// lines are "skipped, counted in Stats, never fatal"; an over-long line must
// be treated exactly like any other malformed line (skip + count) rather than
// aborting the whole scan, which a naive bufio.Scanner would do via ErrTooLong.
//
// The records.jsonl is written directly as raw bytes (os.WriteFile), bypassing
// the B1 writer guard, to model an externally-edited, corrupted, or
// version-skew file that the writer guard cannot prevent. The middle line is a
// single non-JSON blob larger than maxLineBytes, forcing the reader's
// length/over-long path before any JSON decoding happens.
func TestReaderSkipsOverlongLine(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	good1 := newRec("good-1", record.TypePersona, record.PriorityNormal, "first", ts)
	good2 := newRec("good-2", record.TypePersona, record.PriorityNormal, "third", ts.Add(time.Hour))

	line1, err := marshalLine(good1)
	if err != nil {
		t.Fatalf("marshal good-1: %v", err)
	}
	line3, err := marshalLine(good2)
	if err != nil {
		t.Fatalf("marshal good-2: %v", err)
	}

	// Middle line: one line (terminated by '\n') whose length strictly exceeds
	// maxLineBytes, so the reader must detect it by length alone and skip it.
	overlong := append([]byte(nil), bytes.Repeat([]byte("x"), maxLineBytes+64)...)
	overlong = append(overlong, '\n')

	content := make([]byte, 0, len(line1)+len(overlong)+len(line3))
	content = append(content, line1...)
	content = append(content, overlong...)
	content = append(content, line3...)

	path := SessionRecordsPath(root, alias)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, stats, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("expected no fatal error for an over-long line, got %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected the two valid records (good-1, good-2), got %d: %+v", len(got), got)
	}
	gotIDs := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !gotIDs["good-1"] || !gotIDs["good-2"] {
		t.Fatalf("expected good-1 and good-2 (not the over-long middle), got %+v", got)
	}
	if stats.Malformed < 1 {
		t.Fatalf("expected the over-long line counted as malformed (>=1), got %d", stats.Malformed)
	}
}

// TestReaderFaultToleranceBadEnumSkipped verifies that a line that decodes as
// JSON but carries an unknown enum is skipped (because Record.UnmarshalJSON
// rejects it) — protecting the result set from corruption.
func TestReaderFaultToleranceBadEnumSkipped(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	ts := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)

	if err := AppendSession(root, alias, newRec("good", record.TypePersona, record.PriorityNormal, "ok", ts)); err != nil {
		t.Fatal(err)
	}
	// Append a line with an unknown type directly.
	path := SessionRecordsPath(root, alias)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"id":"bad","type":"unknown-type","priority":"normal","scope":"session","created_at":"2026-07-07T10:00:00Z","updated_at":"2026-07-07T10:00:00Z","body":"x"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, stats, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("expected only the good record, got %+v", got)
	}
	if stats.Malformed != 1 {
		t.Fatalf("expected 1 malformed, got %d", stats.Malformed)
	}
}

// TestMissingFileReturnsEmptyNotError verifies the fault-tolerance contract:
// reading a non-existent file yields an empty slice and a nil error.
func TestMissingFileReturnsEmptyNotError(t *testing.T) {
	root := t.TempDir()
	got, stats, err := ReadSession(root, "never-existed", Query{})
	if err != nil {
		t.Fatalf("expected nil error on missing file, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %+v", got)
	}
	if stats.LinesScanned != 0 || stats.Malformed != 0 {
		t.Fatalf("expected zero stats, got %+v", stats)
	}
}

// TestFlatMemoryFilesUntouched is a guard test for the additive-only
// contract: appending and reading records must NOT create or modify the
// pre-existing flat memory files (brief.md, resolved-context.md,
// open-questions.md, decision-log.md) under the session memory directory.
func TestFlatMemoryFilesUntouched(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	memDir := filepath.Join(root, StateDir, SessionsDir, alias, SessionMemoryDir)

	// Pre-create the canonical flat files with sentinel content so we can
	// detect any modification.
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatal(err)
	}
	flatNames := []string{"brief.md", "resolved-context.md", "open-questions.md", "decision-log.md"}
	sentinels := map[string]string{}
	for _, name := range flatNames {
		content := "sentinel-" + name
		sentinels[name] = content
		if err := os.WriteFile(filepath.Join(memDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rec := newRec("r1", record.TypePersona, record.PriorityNormal, "body", time.Now())
	if err := AppendSession(root, alias, rec); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, _, err := ReadSession(root, alias, Query{}); err != nil {
		t.Fatalf("read: %v", err)
	}

	// Verify each flat file's content is byte-identical to the sentinel.
	for _, name := range flatNames {
		got, err := os.ReadFile(filepath.Join(memDir, name))
		if err != nil {
			t.Fatalf("read flat file %s: %v", name, err)
		}
		if string(got) != sentinels[name] {
			t.Fatalf("flat file %s was modified: want %q got %q", name, sentinels[name], got)
		}
	}

	// Also verify the records.jsonl file IS the only new file added.
	entries, err := os.ReadDir(memDir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Name()] = true
	}
	if !seen[RecordsFile] {
		t.Errorf("expected %s to be created", RecordsFile)
	}
	for _, name := range flatNames {
		if !seen[name] {
			t.Errorf("flat file %s disappeared", name)
		}
	}
}

// TestRecordsFilePersistsNoHTMLxEscaping verifies that record bodies
// containing characters json.Marshal would HTML-escape by default (&, <, >)
// are written verbatim, because the store disables SetEscapeHTML.
func TestRecordsFilePersistsNoHTMLEscaping(t *testing.T) {
	root := t.TempDir()
	const alias = "s"
	rec := newRec("r1", record.TypeInstruction, record.PriorityNormal, "use a < b && c > d", time.Now())
	if err := AppendSession(root, alias, rec); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(SessionRecordsPath(root, alias))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `\u003c`) || strings.Contains(string(data), `\u003e`) || strings.Contains(string(data), `\u0026`) {
		t.Fatalf("body was HTML-escaped; raw line: %s", data)
	}
	got, _, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].Body != "use a < b && c > d" {
		t.Fatalf("round trip lost body: %+v", got)
	}
}

// TestConcurrentAppendSafety exercises D1's write-safety guarantee: many
// goroutines appending distinct records to the SAME session file simultaneously
// must not lose or corrupt a single line. The per-path in-process mutex
// serializes these same-process appends (POSIX flock alone would NOT, because
// each goroutine opens its own fd and gets an independent flock lock).
//
// Records are deliberately made LARGE (bodies exceed the POSIX PIPE_BUF of
// 4096 bytes) so that an O_APPEND write of a full line is NOT guaranteed atomic
// by the kernel. Without serialization, concurrent >PIPE_BUF O_APPEND writes
// could interleave at the byte level, producing malformed lines the fault-
// tolerant reader would skip and count. So this test meaningfully exercises the
// mutex: if serialization failed, we would observe Malformed > 0 and/or fewer
// than N*M records recoverable.
func TestConcurrentAppendSafety(t *testing.T) {
	root := t.TempDir()
	const alias = "concurrent"

	const (
		goroutines = 8
		perG       = 10
	)
	want := goroutines * perG

	// Body of 8 KiB strictly exceeds PIPE_BUF (4096) on Linux, defeating
	// O_APPEND atomicity and forcing reliance on the in-process mutex.
	body := strings.Repeat("x", 8192)

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	start := make(chan struct{})
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			<-start // release all goroutines as close to simultaneously as possible
			for r := 0; r < perG; r++ {
				// Distinct IDs across goroutines so dedup cannot collapse them;
				// an UpdatedAt tie would otherwise be fine because IDs differ.
				ts := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC).Add(time.Duration(g*perG+r) * time.Second)
				rec := newRec("g"+itoa(g)+"-r"+itoa(r), record.TypeEpisodic, record.PriorityNormal, body, ts)
				if err := AppendSession(root, alias, rec); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}(g)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent append failed: %v", err)
		}
	}

	got, stats, err := ReadSession(root, alias, Query{Limit: MaxRecords + want})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != want {
		t.Fatalf("expected all %d records recoverable, got %d (stats=%+v)", want, len(got), stats)
	}
	if stats.Malformed != 0 {
		t.Fatalf("expected zero malformed lines (no interleaving corruption), got %d (stats=%+v)", stats.Malformed, stats)
	}
	if stats.LinesScanned != want {
		t.Fatalf("expected %d lines scanned, got %d", want, stats.LinesScanned)
	}

	// Verify every distinct ID is present.
	seen := make(map[string]bool, want)
	for _, r := range got {
		seen[r.ID] = true
	}
	for g := 0; g < goroutines; g++ {
		for r := 0; r < perG; r++ {
			id := "g" + itoa(g) + "-r" + itoa(r)
			if !seen[id] {
				t.Fatalf("record %s not recovered after concurrent append", id)
			}
		}
	}
}

// TestLockReleasedAfterAppendReturns proves the append critical section's locks
// (in-process mutex + cross-process flock) are released on the success path: a
// second append to the SAME file returns promptly instead of deadlocking. A
// failure here surfaces as a test-suite timeout (the goroutine never completes).
func TestLockReleasedAfterAppendReturns(t *testing.T) {
	root := t.TempDir()
	const alias = "release-on-success"
	ts := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)

	if err := AppendSession(root, alias, newRec("first", record.TypePersona, record.PriorityNormal, "one", ts)); err != nil {
		t.Fatalf("first append: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- AppendSession(root, alias, newRec("second", record.TypePersona, record.PriorityNormal, "two", ts.Add(time.Second)))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second append (release-on-success): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second append deadlocked: append lock not released on success path")
	}

	got, _, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records after two appends, got %d", len(got))
	}
}

// TestAppendAfterRejectedAppendSucceeds proves that an append rejected on the
// pre-lock error path (here, the B1 oversized-line guard, which returns before
// any lock is acquired) does not leave a lock held: a subsequent valid append
// to the SAME file returns promptly. This is the D1-suggested release-path
// check, adjusted for this slice's lock placement (the size guard precedes the
// lock, so it cannot leak a lock — the test confirms that explicitly).
//
// NOTE on the post-lock error path: errors that occur AFTER flockEx succeeds
// (write / fsync data / fsync dir) are released by the deferred funlock(f) +
// f.Close() pair, which runs LIFO on every return after the fd is opened.
// Injecting a genuine post-lock write/fsync failure against a t.TempDir() is
// not reliably portable (FIFO/pipe fsync semantics vary, disk-full is
// environmental), so the post-lock release is verified by structural reasoning
// (the defer chain) rather than a synthetic I/O-failure test. The concurrent
// test above additionally exercises the lock under heavy contention.
func TestAppendAfterRejectedAppendSucceeds(t *testing.T) {
	root := t.TempDir()
	const alias = "reject-then-ok"
	ts := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)

	// Rejected pre-lock: oversized line returns before any lock is acquired
	// and before the file is created.
	bad := newRec("too-big", record.TypePersona, record.PriorityNormal, strings.Repeat("x", maxLineBytes), ts)
	if err := AppendSession(root, alias, bad); err == nil {
		t.Fatal("expected oversized-line error")
	}
	if _, statErr := os.Stat(SessionRecordsPath(root, alias)); !os.IsNotExist(statErr) {
		t.Fatalf("file should not exist after a rejected append, got err=%v", statErr)
	}

	// A subsequent valid append to the SAME file must succeed promptly,
	// proving no lock was left held by the rejected append.
	done := make(chan error, 1)
	go func() {
		done <- AppendSession(root, alias, newRec("ok", record.TypePersona, record.PriorityNormal, "valid", ts))
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("valid append after rejected append: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("valid append after rejected append deadlocked: a pre-lock error leaked a lock")
	}

	got, _, err := ReadSession(root, alias, Query{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ok" {
		t.Fatalf("expected only the valid record, got %+v", got)
	}
}

// itoa is a tiny local int→string helper to avoid importing strconv just for
// test index labels.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
