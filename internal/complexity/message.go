package complexity

import (
	"fmt"
	"strings"
)

// SnapshotAdvisoryMessage renders the advisory message for ONE nominated signal
// in the repo-snapshot projection. It carries the projection identity, the
// normalized path, the observed value, the threshold, and advisory wording. It
// MUST NOT contain FAIL/rejection/auto-split language — the signal is advisory
// evidence, never authority (staged advisory hybrid; WARN never becomes FAIL).
//
// rank/total let the caller express "candidate 3 of 7" for presentation; pass
// 0/0 to omit the rank suffix.
func SnapshotAdvisoryMessage(s Signal, rank, total int) string {
	var b strings.Builder
	b.WriteString(string(s.Projection))
	b.WriteString(": ")
	b.WriteString(s.Path)
	b.WriteString(" is ")
	b.WriteString(fmt.Sprintf("%d", s.Metric.Observed))
	b.WriteString(" lines (threshold ")
	b.WriteString(fmt.Sprintf("%d", s.Metric.Threshold))
	b.WriteString("); review cohesion and consider recording a disposition (accept-as-cohesive or split-defer)")
	if rank > 0 && total > 0 {
		b.WriteString(fmt.Sprintf(" [candidate %d of %d]", rank, total))
	}
	return b.String()
}

// PostEditAdvisoryMessage renders the advisory message for the event-time
// (post-edit) projection. Same WARN-only contract; the wording reflects that an
// edit just touched the file.
func PostEditAdvisoryMessage(s Signal) string {
	var b strings.Builder
	b.WriteString(string(s.Projection))
	b.WriteString(": ")
	b.WriteString(s.Path)
	b.WriteString(" is ")
	b.WriteString(fmt.Sprintf("%d", s.Metric.Observed))
	b.WriteString(" lines (threshold ")
	b.WriteString(fmt.Sprintf("%d", s.Metric.Threshold))
	b.WriteString(") after this edit; consider extracting helpers or reviewing the boundary's cohesion")
	return b.String()
}

// DoctorSection renders the full complexity advisory section for the doctor
// named-check output. It presents at most maxCandidates nominated signals in
// canonical order, preserving the full count so the operator sees the true
// scope. The section is advisory (WARN); it never uses FAIL/rejection language
// and carries no transition authority.
//
// enabled=false produces a short skip note instead.
func DoctorSection(nominated []Signal, maxCandidates int, enabled bool) string {
	if !enabled {
		return "complexity policy disabled (enabled: false); no advisory."
	}
	if len(nominated) == 0 {
		return "no files exceed the configured complexity thresholds."
	}
	SortSignals(nominated)
	shown, total := TruncatePresentation(nominated, maxCandidates)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("complexity advisory: %d candidate(s) above threshold (showing %d).\n", total, len(shown)))
	for i, s := range shown {
		b.WriteString("  ")
		b.WriteString(SnapshotAdvisoryMessage(s, i+1, total))
		b.WriteString("\n")
	}
	if total > len(shown) {
		b.WriteString(fmt.Sprintf("  ...and %d more (raise doctor.max_candidates to see them).\n", total-len(shown)))
	}
	b.WriteString("advisory only: record a disposition in .opencode/repo-configs/complexity-dispositions.yml (accept-as-cohesive or split-defer).")
	return b.String()
}
