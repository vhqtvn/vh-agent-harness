// Package store implements the append-only JSONL persistence layer for typed
// memory records defined by internal/memory/record.
//
// Records are stored as strict JSON Lines (one complete JSON object per line,
// UTF-8, "\n"-terminated) at two locked repo-relative locations:
//
//   - session:    .opencode/state/sessions/<alias>/memory/records.jsonl
//   - workstream: .opencode/state/workstreams/<slug>/records.jsonl
//
// This layer is purely ADDITIVE. The pre-existing flat memory files (brief.md,
// resolved-context.md, open-questions.md, decision-log.md, and workstream
// memory files) remain canonical and are NEVER read, written, migrated, or
// otherwise touched here.
//
// Write model (this slice): append-only. Adding a record appends exactly one
// JSON line. Updates are expressed by appending a new line with the same ID
// and a newer UpdatedAt; readers treat the latest UpdatedAt per ID as current
// (last-write-wins). The append marshals the whole line into memory and issues
// a single O_APPEND write so that normal operation never leaves a partial or
// malformed line in the file.
//
// Concurrency: this slice does NOT implement advisory locking, fsync, or the
// tmp-file-then-rename rewrite discipline. Concurrent writers may interleave
// whole lines safely on most platforms (O_APPEND writes are atomic up to the
// pipe buffer size on POSIX) but the layer provides no fsync durability and no
// cross-process lock. That hardening is deferred to a later slice.
//
// Read model: bounded linear scan. There is no database, no index, no cache.
// A reader scans the relevant records.jsonl once, skips malformed lines
// (counting them in Stats, never fatal), deduplicates by ID keeping the
// latest UpdatedAt, applies the requested filters, sorts by Priority then
// UpdatedAt (both descending), and caps the result at MaxRecords.
//
// All public functions resolve paths relative to a caller-supplied repo root
// string (no hardcoded absolute home directories) following the same pattern
// used by internal/lineage.
package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/memory/record"
)

// StateDir is the repo-relative root of the harness state tree.
const StateDir = ".opencode/state"

// SessionsDir is the directory under StateDir holding per-session state.
const SessionsDir = "sessions"

// WorkstreamsDir is the directory under StateDir holding per-workstream state.
const WorkstreamsDir = "workstreams"

// SessionMemoryDir is the per-session memory directory name.
const SessionMemoryDir = "memory"

// RecordsFile is the filename of the append-only JSONL record log within each
// session memory directory or workstream directory.
const RecordsFile = "records.jsonl"

// MaxRecords is the default upper bound on the number of records returned by a
// single Read query. A Query may override this via Query.Limit, but a zero
// Query.Limit falls back to MaxRecords. The bound keeps linear-scan retrieval
// predictable and bounded for callers (including future prompt builders).
const MaxRecords = 200

// maxLineBytes is the per-line bound for both the reader and the writer. The
// reader's bufio.Reader is sized to exactly this cap (see readFile) so a line
// whose length exceeds it is detected as malformed without unbounded buffering;
// the writer rejects any encoded line longer than this so self-authored files
// never contain a line the reader would skip. 1 MiB is generous for a memory
// atom (Body text plus provenance) while still rejecting runaway lines.
const maxLineBytes = 1 << 20

// Query selects and filters records during a Read.
//
// All filter fields are optional. A zero-valued filter field means "match
// any". When multiple filters are set they are AND-combined: a record must
// satisfy every non-zero filter to be retained.
//
// Keyword filters (BodyKeyword, SourceRefKeyword) perform case-insensitive
// substring matching against the respective field. Empty keyword = match any.
type Query struct {
	// Scope restricts results to the given record.Scope. Zero = any.
	Scope record.Scope
	// SessionKey restricts results to records whose SessionKey equals this
	// value. Zero = any. Compared exactly (case-sensitive).
	SessionKey string
	// Workstream restricts results to records whose Workstream pointer equals
	// this value. Zero = any. Compared exactly (case-sensitive).
	Workstream string
	// Type restricts results to the given record.Type. Zero = any.
	Type record.Type
	// Priority restricts results to the given record.Priority. Zero = any.
	Priority record.Priority
	// BodyKeyword, when non-empty, retains only records whose Body contains
	// this case-insensitive substring.
	BodyKeyword string
	// SourceRefKeyword, when non-empty, retains only records whose SourceRef
	// pointer (when set) contains this case-insensitive substring. Records
	// with a nil SourceRef never match a non-empty SourceRefKeyword.
	SourceRefKeyword string
	// Limit caps the number of returned records. Zero (or negative) means
	// MaxRecords. Values above MaxRecords are clamped to MaxRecords so a
	// caller cannot accidentally unbound a scan.
	Limit int
}

// Stats reports per-scan accounting for diagnostics. It lets callers observe
// data-quality issues (e.g. malformed lines) without forcing the scan itself
// to fail.
type Stats struct {
	// LinesScanned is the count of non-empty lines examined during the scan,
	// including any that failed to decode.
	LinesScanned int
	// Malformed is the count of non-empty lines that could not be decoded as
	// valid record JSON (or that decoded but failed enum/value validation).
	// These lines are skipped and never cause the scan to fail.
	Malformed int
}

// SessionRecordsPath returns the absolute path to the records.jsonl file for
// the given session alias under root. It performs no filesystem checks.
func SessionRecordsPath(root, alias string) string {
	return filepath.Join(root, StateDir, SessionsDir, alias, SessionMemoryDir, RecordsFile)
}

// WorkstreamRecordsPath returns the absolute path to the records.jsonl file
// for the given workstream slug under root. It performs no filesystem checks.
func WorkstreamRecordsPath(root, slug string) string {
	return filepath.Join(root, StateDir, WorkstreamsDir, slug, RecordsFile)
}

// AppendSession validates rec and appends it as one JSON line to the session
// records.jsonl for alias, creating the file and parent directories as
// needed.
//
// Returns an error if rec fails validation or if the append write fails.
// Missing parent directories are created with 0o755 perms; the records file is
// created with 0o644 perms when absent.
func AppendSession(root, alias string, rec record.Record) error {
	return appendTo(SessionRecordsPath(root, alias), rec)
}

// AppendWorkstream validates rec and appends it as one JSON line to the
// workstream records.jsonl for slug, creating the file and parent directories
// as needed. See AppendSession for semantics.
func AppendWorkstream(root, slug string, rec record.Record) error {
	return appendTo(WorkstreamRecordsPath(root, slug), rec)
}

// ReadSession scans the session records.jsonl for alias, applies q, and
// returns the matching records plus scan stats. A missing file is not an
// error: it yields an empty slice and a zero-valued Stats.
func ReadSession(root, alias string, q Query) ([]record.Record, Stats, error) {
	return readFile(SessionRecordsPath(root, alias), q)
}

// ReadWorkstream scans the workstream records.jsonl for slug, applies q, and
// returns the matching records plus scan stats. A missing file is not an
// error: it yields an empty slice and a zero-valued Stats.
func ReadWorkstream(root, slug string, q Query) ([]record.Record, Stats, error) {
	return readFile(WorkstreamRecordsPath(root, slug), q)
}

// appendTo is the shared append implementation. It marshals rec to a single
// JSON line (with a trailing newline), creates parent directories if needed,
// and issues one O_APPEND|O_CREATE|O_WRONLY write of the full line so that
// normal operation cannot leave a partial line in the file.
func appendTo(path string, rec record.Record) error {
	if err := rec.Validate(); err != nil {
		return fmt.Errorf("memory store: %w", err)
	}

	line, err := marshalLine(rec)
	if err != nil {
		return fmt.Errorf("memory store: encode record %q: %w", rec.ID, err)
	}

	// Guard the read/write line-size invariant: the reader caps each scan
	// line at maxLineBytes and treats a longer line as malformed (skipped and
	// counted in Stats, never fatal — see readFile). Reject such a record
	// before any filesystem side effect so this layer never writes a line the
	// reader would skip, keeping self-authored files fully readable.
	if len(line) > maxLineBytes {
		return fmt.Errorf("memory store: record %q encoded line (%d bytes) exceeds max %d", rec.ID, len(line), maxLineBytes)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memory store: create dirs for %s: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("memory store: open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("memory store: write %s: %w", path, err)
	}
	return nil
}

// marshalLine encodes rec as compact JSON with HTML escaping disabled (so
// record bodies containing characters like &, <, > survive verbatim) and
// appends the trailing newline required by the JSON Lines format.
func marshalLine(rec record.Record) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return nil, err
	}
	// json.Encoder.Encode already appends a trailing "\n", so buf now holds
	// exactly one complete JSONL line.
	return buf.Bytes(), nil
}

// readFile is the shared linear-scan implementation. See the package doc for
// the full pipeline (scan → skip malformed → dedup by ID last-write-wins →
// filter → sort → cap).
func readFile(path string, q Query) ([]record.Record, Stats, error) {
	var stats Stats

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing file is the normal empty-store case: not an error.
			return []record.Record{}, stats, nil
		}
		return nil, stats, fmt.Errorf("memory store: open %s: %w", path, err)
	}
	defer f.Close()

	// latest holds the winning record per ID (max UpdatedAt; ties broken by
	// later position in the file, i.e. true last-write-wins). Iteration order
	// is preserved by order for stable, deterministic output.
	latest := make(map[string]record.Record)
	order := make([]string, 0)

	// Fault-tolerant line scan honoring the package-doc contract that every
	// malformed line is "skipped, counted in Stats, never fatal" — including
	// a line that exceeds maxLineBytes. A bufio.Scanner would surface such a
	// line as a fatal ErrTooLong that aborts the whole scan; instead we drive
	// a bufio.Reader whose buffer is capped at exactly maxLineBytes and treat
	// a line that fills the buffer before its '\n' (bufio.ErrBufferFull) as
	// just another malformed line: count it, drain through the next '\n' (or
	// EOF) without buffering the whole line, and keep scanning. Memory stays
	// bounded at maxLineBytes regardless of how long a bad line is. The only
	// fatal path is a true I/O error mid-scan; line content/length never is.
	reader := bufio.NewReaderSize(f, maxLineBytes)
scanLoop:
	for {
		chunk, rerr := reader.ReadSlice('\n')
		switch rerr {
		case bufio.ErrBufferFull:
			// Over-long line: it exceeded maxLineBytes before a '\n'. Count
			// it as a scanned malformed line, then drain the remainder of the
			// same line through the next '\n' (or EOF) without buffering it.
			stats.LinesScanned++
			stats.Malformed++
			for {
				_, derr := reader.ReadSlice('\n')
				switch derr {
				case nil:
					// Reached the line terminator: over-long line drained.
				case bufio.ErrBufferFull:
					// Still inside the same over-long line; keep draining.
					continue
				case io.EOF:
					// The over-long line ran to EOF with no terminator.
					break scanLoop
				default:
					return nil, stats, fmt.Errorf("memory store: scan %s: %w", path, derr)
				}
				break
			}
			continue
		case io.EOF:
			// End of input. chunk holds a trailing line only if the file did
			// not end with '\n'; an empty chunk means we are done. (An
			// over-long line surfaces as ErrBufferFull first, so any chunk
			// reaching here is bounded: len(chunk) <= maxLineBytes.)
			if len(chunk) == 0 {
				break scanLoop
			}
			// Fall through to process the final, unterminated line.
		case nil:
			// A complete '\n'-terminated line; process it below.
		default:
			// True I/O error mid-scan: the only fatal path.
			return nil, stats, fmt.Errorf("memory store: scan %s: %w", path, rerr)
		}

		// Process one complete line. chunk includes the trailing '\n' when
		// present (ReadSlice keeps the delimiter); strip it, then skip blanks
		// (e.g. a trailing newline at EOF) just like before.
		raw := bytes.TrimRight(chunk, "\n")
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		stats.LinesScanned++

		var rec record.Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			stats.Malformed++
			continue
		}
		// Decode succeeds for known enums (Record.UnmarshalJSON rejects
		// unknown Type/Priority/Scope). A line that decoded but is otherwise
		// semantically broken (e.g. missing ID, zero time) is treated as
		// malformed and skipped rather than poisoning the result set.
		if err := rec.Validate(); err != nil {
			stats.Malformed++
			continue
		}

		_, seen := latest[rec.ID]
		if !seen {
			// First time we encounter this ID: record its insertion order so
			// the final output is deterministic and stable before sorting.
			order = append(order, rec.ID)
			latest[rec.ID] = rec
			continue
		}
		// Last-write-wins by UpdatedAt. On an exact tie the later line in the
		// file wins (>= comparison), which matches the "append a newer line to
		// update" contract; an older line never regresses a newer one.
		prev := latest[rec.ID]
		if !rec.UpdatedAt.Before(prev.UpdatedAt) {
			latest[rec.ID] = rec
		}
	}

	// Materialize, filter, and preserve first-seen ordering before sorting.
	out := make([]record.Record, 0, len(order))
	for _, id := range order {
		rec := latest[id]
		if matches(rec, q) {
			out = append(out, rec)
		}
	}

	sortRecords(out)

	if limit := effectiveLimit(q.Limit); limit < len(out) {
		out = out[:limit]
	}
	return out, stats, nil
}

// matches reports whether rec satisfies every non-zero filter in q. Empty
// string / zero enum filters are wildcards. Keyword filters are
// case-insensitive substrings.
func matches(rec record.Record, q Query) bool {
	if q.Scope != "" && rec.Scope != q.Scope {
		return false
	}
	if q.Type != "" && rec.Type != q.Type {
		return false
	}
	if q.Priority != "" && rec.Priority != q.Priority {
		return false
	}
	if q.SessionKey != "" {
		if rec.SessionKey == nil || *rec.SessionKey != q.SessionKey {
			return false
		}
	}
	if q.Workstream != "" {
		if rec.Workstream == nil || *rec.Workstream != q.Workstream {
			return false
		}
	}
	if q.BodyKeyword != "" {
		if !strings.Contains(strings.ToLower(rec.Body), strings.ToLower(q.BodyKeyword)) {
			return false
		}
	}
	if q.SourceRefKeyword != "" {
		if rec.SourceRef == nil {
			return false
		}
		if !strings.Contains(strings.ToLower(*rec.SourceRef), strings.ToLower(q.SourceRefKeyword)) {
			return false
		}
	}
	return true
}

// sortRecords sorts in place: primary by Priority rank descending (critical
// first), secondary by UpdatedAt descending (newest first). The sort is not
// stable, but the two keys fully determine order for distinct records; ties
// on both keys have no meaningful preference.
func sortRecords(recs []record.Record) {
	sort.Slice(recs, func(i, j int) bool {
		ri, rj := recs[i].Priority.Rank(), recs[j].Priority.Rank()
		if ri != rj {
			return ri > rj
		}
		return recs[i].UpdatedAt.After(recs[j].UpdatedAt)
	})
}

// effectiveLimit resolves a caller-supplied limit to a bounded cap. Zero or
// negative → MaxRecords. Above MaxRecords → clamped to MaxRecords.
func effectiveLimit(limit int) int {
	if limit <= 0 {
		return MaxRecords
	}
	if limit > MaxRecords {
		return MaxRecords
	}
	return limit
}
