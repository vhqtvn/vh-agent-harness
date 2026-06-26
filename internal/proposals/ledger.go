// Package proposals owns the Slice-5.3 proposal ledger: an append-only JSONL
// record of every armed-file conflict (schema reconcile Proposal) the seam apply
// path surfaced but did NOT auto-resolve. When substrate.Apply plans an armed
// file and the reconciler returns OutcomePropose (e.g. the platform withdrew an
// enum value the project still uses — an enum_removed proposal), the live
// instance is left untouched and a structured Proposal is emitted. seamApply
// appends each such Proposal to .vh-agent-harness/proposals.jsonl so an operator
// has a durable, reviewable record of every needs-decision conflict, surfaced by
// `vh-agent-harness proposals`.
//
// The ledger is external_generated / runtime: it is created and maintained by
// the harness at apply time and is NOT a platform template (never rendered, never
// owned by a pack). It is append-only across updates so the history of conflict
// surfaces is preserved; trimming is an explicit operator action.
//
// Slice 6 (HELD beyond v0) will add the D2-C downgrade proposal path (acting on
// a proposal to downgrade protection). This package only RECORDS and LISTS
// proposals; it never resolves them.
package proposals

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/schema"
)

// FileName is the ledger file name inside the lineage dir.
const FileName = "proposals.jsonl"

// FilePath returns the absolute path to the ledger under a target directory.
func FilePath(target string) string {
	return filepath.Join(target, lineage.DirName, FileName)
}

// Record is one ledger entry: a single armed-file conflict surfaced at apply
// time, carrying the path, its ownership class, the harness ref that produced
// it, an RFC3339 timestamp, and the structured Proposals the reconciler emitted.
type Record struct {
	// Timestamp is the RFC3339 time the record was appended.
	Timestamp string `json:"timestamp"`
	// Path is the repo-relative live path of the armed file (e.g.
	// "vh-harness-profile.yml").
	Path string `json:"path"`
	// Class is the armed ownership class of the path (always platform_armed for
	// proposal-bearing outcomes today).
	Class string `json:"class"`
	// Ref is the harness template ref that produced the conflict (lineage ref).
	Ref string `json:"ref,omitempty"`
	// Proposals is the structured reconcile proposals (e.g. enum_removed).
	// Non-empty for every record (seamApply only records ArmedProposal outcomes).
	Proposals []schema.Proposal `json:"proposals"`
}

// Append writes records to the ledger, one JSON line per record. It creates the
// lineage dir if needed and opens the file append-only so prior entries are
// preserved across updates. Each record is stamped with Timestamp + Ref. An
// empty records slice is a no-op (returns 0, nil). Returns the number of records
// appended.
func Append(target, ref string, records []Record) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(filepath.Join(target, lineage.DirName), 0o755); err != nil {
		return 0, fmt.Errorf("proposals: mkdir: %w", err)
	}
	f, err := os.OpenFile(FilePath(target), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("proposals: open ledger: %w", err)
	}
	defer f.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	written := 0
	for _, rec := range records {
		if rec.Timestamp == "" {
			rec.Timestamp = now
		}
		if rec.Ref == "" {
			rec.Ref = ref
		}
		line, err := json.Marshal(rec)
		if err != nil {
			return written, fmt.Errorf("proposals: marshal record for %q: %w", rec.Path, err)
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			return written, fmt.Errorf("proposals: write ledger: %w", err)
		}
		written++
	}
	return written, nil
}

// Read loads every record from the ledger in insertion order. A missing ledger
// returns (nil, nil) (no conflicts recorded yet — the common, healthy case); a
// present-but-unparseable line returns an error naming the offending line so an
// operator can inspect it. Blank lines are skipped.
func Read(target string) ([]Record, error) {
	f, err := os.Open(FilePath(target))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("proposals: open ledger: %w", err)
	}
	defer f.Close()
	var out []Record
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // raise per-line cap for big proposal sets
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return out, fmt.Errorf("proposals: parse line %d: %w", lineNo, err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("proposals: scan ledger: %w", err)
	}
	return out, nil
}
