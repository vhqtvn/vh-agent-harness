package renderstate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DestinationState labels the on-disk state of a definite orphan's destination
// relative to the last recorded render.
type DestinationState string

const (
	// DestMissing means the destination is gone (operator removed it). The
	// record is RETIRED from the manifest — there is nothing to preserve and
	// nothing to report.
	DestMissing DestinationState = "missing"
	// DestUnchanged means the destination is still on disk and byte-identical to
	// the last recorded render (the orphan is pristine).
	DestUnchanged DestinationState = "unchanged"
	// DestModified means the destination is still on disk but differs from the
	// last recorded render (operator hand-edited it, or another source rewrote
	// it). Still reported_preserved — the platform never deletes it.
	DestModified DestinationState = "modified"
)

// OrphanAction labels what the report-only detection did for one finding. v1
// never deletes; every finding is reported_preserved.
type OrphanAction string

const (
	// ActionReportedPreserved: the orphan was surfaced for visibility and left
	// on disk untouched.
	ActionReportedPreserved OrphanAction = "reported_preserved"
)

// Reason labels why a record is a definite orphan. v1 only emits source_missing.
type Reason string

const (
	// ReasonSourceMissing: the producing source can no longer be read from the
	// producer pack (pack gone OR source file gone within the pack). This is the
	// only definite-orphan reason in v1.
	ReasonSourceMissing Reason = "source_missing"
)

// OrphanFinding is one preserved orphan surfaced by report-only detection. It is
// the structured shape attached to substrate.ApplyReport so the dry-run and
// normal-update reports can render it consistently.
type OrphanFinding struct {
	// DestinationPath is the normalized repo-relative live path of the orphaned
	// rendered file (e.g. ".opencode/skills/tdd-loop/SKILL.md").
	DestinationPath string
	// SkillDir is the containing skill directory (e.g.
	// ".opencode/skills/tdd-loop"), derived from the destination for operator
	// readability.
	SkillDir string
	// ProducerKind echoes the record's producer kind (v1: overlay_skill).
	ProducerKind ProducerKind
	// OverlayPack is the pack that produced the now-orphaned file.
	OverlayPack string
	// SourceRelativePath is the pack-FS-relative source that is now missing.
	SourceRelativePath string
	// Reason is why this is a definite orphan (source_missing).
	Reason Reason
	// DestinationState is the on-disk state of the destination relative to the
	// last render (unchanged / modified).
	DestinationState DestinationState
	// Action is what the report-only detection did (reported_preserved).
	Action OrphanAction
}

// SourceState is the tri-state result of probing whether the producing source
// for a record still exists, INDEPENDENT of whether the pack is currently
// selected. It is the typed correctness signal that separates a DEFINITE orphan
// (confirmed source-missing) from an unreadable/transient probe (cannot
// classify) and from a still-present source (not an orphan).
type SourceState string

const (
	// SourcePresent: the producing source can still be read → not an orphan
	// (covers a deselected-but-extant pack).
	SourcePresent SourceState = "present"
	// SourceMissing: the source is CONFIRMED absent — the file is gone from its
	// pack (fs.ErrNotExist) or the whole pack is gone. This is the only
	// definite-orphan condition.
	SourceMissing SourceState = "missing"
	// SourceIndeterminate: the probe could not classify the source — a
	// permission error, an unreadable/malformed pack, or a transient I/O error.
	// This is NEVER classified as a definite orphan: Compare warns and skips the
	// record (a transient error must not produce a false-positive orphan), and
	// NextManifest does NOT retain it as a stale orphan.
	SourceIndeterminate SourceState = "indeterminate"
)

// SourceChecker reports the source state for a record. The seam supplies a
// concrete implementation that opens the pack by name and probes the
// source-relative path inside the pack FS, distinguishing a confirmed-absent
// source (fs.ErrNotExist / pack gone) from an unreadable one (permission /
// transient). This is the provenance contract: a definite orphan requires the
// source to be CONFIRMED missing, not merely unreadable or deselected.
type SourceChecker interface {
	// CheckSource returns the source state for rec. Only SourceMissing makes rec
	// a definite-orphan candidate; SourceIndeterminate must be skipped (warned),
	// never flagged.
	CheckSource(rec Record) SourceState
}

// skillDirFromDestination derives the containing skill directory from a rendered
// skill file path. For a destination under ".opencode/skills/<name>/..." it
// returns ".opencode/skills/<name>". For an unexpected shape it returns the
// destination's parent directory.
func skillDirFromDestination(dest string) string {
	const prefix = ".opencode/skills/"
	rest := strings.TrimPrefix(dest, prefix)
	if len(rest) < len(dest) {
		// dest was under .opencode/skills/; skill dir = prefix + first segment.
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			return prefix + rest[:i]
		}
		return dest
	}
	return filepath.ToSlash(filepath.Dir(filepath.Clean(dest)))
}

// diskDigest reads the destination file under projectRoot and returns its
// digest, or "" if the file is missing/unreadable.
func diskDigest(projectRoot, dest string) string {
	live := filepath.Join(projectRoot, filepath.FromSlash(dest))
	data, err := os.ReadFile(live)
	if err != nil {
		return ""
	}
	return Digest(data)
}

// Compare evaluates the PRIOR manifest against the current render and produces
// the report-only orphan findings. A record is a definite orphan only when its
// source is CONFIRMED missing (checker.CheckSource == SourceMissing) AND its
// destination is still on disk. A record whose source still exists is never an
// orphan, even if its pack is currently deselected (the source may simply not
// have been rendered this run). A record whose source is INDETERMINATE
// (unreadable / transient / permission) is WARNED and skipped — it must never be
// classified as a definite orphan on a transient error. A record whose source is
// missing but whose destination is also gone is retired silently (nothing to
// preserve). checker must be non-nil. warn receives one line per indeterminate
// record; pass io.Discard (or nil) to suppress, or a buffer in tests to assert.
func Compare(prior *Manifest, current []Record, checker SourceChecker, projectRoot string, warn io.Writer) []OrphanFinding {
	if prior == nil || checker == nil {
		return nil
	}
	// currentByDest lets us skip records that were freshly rendered this run
	// (source present by construction). The orphan candidates are prior records
	// NOT reproduced by the current render.
	currentByDest := make(map[string]bool, len(current))
	for _, c := range current {
		currentByDest[NormalizeDestination(c.DestinationPath)] = true
	}

	// De-duplicate prior records by destination defensively (Validate would have
	// rejected duplicates on Read, but a hand-authored manifest could reappear).
	// The destination key is NORMALIZED on every lookup so a hand-authored or
	// corrupted prior manifest with a non-canonical path (trailing slash, "./"
	// prefix, backslashes) cannot silently miss a current-render match and slip
	// through as a false orphan.
	seen := make(map[string]bool, len(prior.Entries))
	var findings []OrphanFinding
	for _, rec := range prior.Entries {
		dest := NormalizeDestination(rec.DestinationPath)
		if seen[dest] {
			continue
		}
		seen[dest] = true
		// A freshly rendered record is not an orphan regardless of source state.
		if currentByDest[dest] {
			continue
		}
		// Tri-state source probe. Only CONFIRMED missing → definite-orphan
		// candidate. Indeterminate → warn + skip (never a false-positive orphan).
		switch checker.CheckSource(rec) {
		case SourcePresent:
			continue
		case SourceIndeterminate:
			if warn != nil {
				fmt.Fprintf(warn, "renderstate: warning: source state indeterminate for %q (pack %q, source %q); orphan detection skipped for this record (not classified as an orphan)\n",
					dest, rec.OverlayPack, rec.SourceRelativePath)
			}
			continue
		case SourceMissing:
			// fall through to the orphan-candidate checks below.
		default:
			// An unexpected/unknown SourceState (e.g. a future value a stale
			// checker returns) must NEVER false-positive as a definite orphan.
			// Treat it as indeterminate: warn + skip (fail-safe).
			if warn != nil {
				fmt.Fprintf(warn, "renderstate: warning: source state unknown for %q (pack %q, source %q); orphan detection skipped for this record (not classified as an orphan)\n",
					dest, rec.OverlayPack, rec.SourceRelativePath)
			}
			continue
		}
		// Source missing. Report only if the destination is still on disk.
		dig := diskDigest(projectRoot, dest)
		if dig == "" {
			// Destination gone → retire silently.
			continue
		}
		state := DestModified
		if dig == rec.RenderedDigest {
			state = DestUnchanged
		}
		findings = append(findings, OrphanFinding{
			DestinationPath:    dest,
			SkillDir:           skillDirFromDestination(dest),
			ProducerKind:       rec.ProducerKind,
			OverlayPack:        rec.OverlayPack,
			SourceRelativePath: rec.SourceRelativePath,
			Reason:             ReasonSourceMissing,
			DestinationState:   state,
			Action:             ActionReportedPreserved,
		})
	}
	return findings
}

// NextManifest builds the manifest to persist after a non-dry-run apply (the
// caller gates: it reaches NextManifest only when no currently-rendered,
// manifest-tracked overlay-skill destination reports WriteFailed; non-skill write
// failures do not gate, and substrate.Apply return semantics are unchanged). It is the
// union of the freshly rendered records and the stale prior records that must be
// RETAINED so their orphans keep reporting across runs (decision: retain a record
// whose source is missing while its destination remains present; retire only when
// the destination is gone or the source returns). A fresh record always wins over
// a stale one with the same destination (the source returned), and a stale record
// whose destination is gone is dropped. renderID stamps the new manifest's
// successful_render_id. checker and projectRoot are used only to decide stale
// retention; both may be nil when retention is not being computed (then only the
// current records are persisted).
func NextManifest(prior *Manifest, current []Record, checker SourceChecker, projectRoot, renderID string) *Manifest {
	next := New(renderID)
	byDest := make(map[string]Record, len(current)+len(prior.GetEntries()))
	// 1. Seed with stale prior records whose source is missing AND destination
	//    still present (keep reporting the orphan). Source-present prior records
	//    are dropped: either the current render reproduces them (fresh record
	//    wins) or the source exists but the unit was not rendered this run
	//    (deselected → no longer tracked, correct).
	if prior != nil {
		for _, rec := range prior.Entries {
			// Retain a prior record as a stale orphan ONLY when its source is
			// CONFIRMED missing (not merely indeterminate — a transient probe
			// must not promote a record to tracked-orphan state) AND its
			// destination is still present (keep reporting the orphan).
			if checker != nil && checker.CheckSource(rec) == SourceMissing {
				if diskDigest(projectRoot, rec.DestinationPath) != "" {
					byDest[NormalizeDestination(rec.DestinationPath)] = rec
				}
			}
		}
	}
	// 2. Overlay the freshly rendered records (source returned wins). Keyed by
	//    the normalized destination so a fresh record always displaces a stale
	//    prior record for the same logical path even if the prior copy carried a
	//    non-canonical form.
	for _, rec := range current {
		byDest[NormalizeDestination(rec.DestinationPath)] = rec
	}
	for _, rec := range byDest {
		next.Entries = append(next.Entries, rec)
	}
	return next
}

// GetEntries returns m.Entries, tolerating a nil manifest so callers can write
// NextManifest(nil, current, ...) for the no-prior-manifest bootstrap.
func (m *Manifest) GetEntries() []Record {
	if m == nil {
		return nil
	}
	return m.Entries
}

// SkillDirOf wraps skillDirFromDestination for external callers that want the
// readable grouping without re-deriving it.
func SkillDirOf(dest string) string {
	return skillDirFromDestination(dest)
}
