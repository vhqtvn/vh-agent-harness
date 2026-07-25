package cli

// f2_streak.go — R1-derived operator synthesis streak scanner (Slice 8 of the
// F2 rendering/persistence family).
//
// DESIGN AUTHORITY: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), "R1-derived operator
// synthesis streak" (L268-286).
//
// The streak is a DERIVED VIEW produced by deterministically scanning the
// immutable canonical sidecars at docs/checkpoints/f2/*.canonical.json. It
// is NOT a new canonical property: R1 is declared ONCE in F1 and DERIVED into
// F2 (memo L109-112). The scanner formats what F1 already declared; it never
// infers a relationship, creates a hazard link, collapses a contradiction into
// agreement, derives global absence from bounded absence, replaces canonical
// ancestry, or authors a second conclusion.
//
// ORDERING (memo L281-282): canonical chronology + a stable tie-breaker
// (synthesis_cycle_id), NEVER filesystem mtime. No shared mutable index. No
// INDEX.md is written or read; the scanner re-derives the streak on every call.
//
// SCOPE (memo L284-285): render ALL valid committed F2 cycles in the checked
// repo scope. No "last N" policy. If scale later requires a bound, it MUST be
// explicit in renderer config AND printed AND deterministic AND described as a
// view filter — never a silent truncation.
//
// DIGEST GATE: each sidecar is re-derived from its canonical envelope and
// compared to F2ViewMetadata.SourceSemanticDigest. A sidecar whose carried
// digest does not match the recomputed digest is EXCLUDED from the streak and
// reported as a diagnostic. The streak never displays content whose canonical
// binding is broken.
//
// HONESTY CEILING (memo L362-382): a successfully-scanned streak is a faithful
// display of the canonical content of each included cycle; it is NOT thereby
// proven to describe conclusions that are actually true. Scanning is structural
// display, not semantic verification.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// F2StreakDiagnosticKind names the reason a sidecar was excluded or warned.
type F2StreakDiagnosticKind string

const (
	// F2StreakExcludedCorruptJSON: the file could not be parsed as JSON / as
	// an F2CanonicalSidecar. Excluded from the streak.
	F2StreakExcludedCorruptJSON F2StreakDiagnosticKind = "excluded_corrupt_json"
	// F2StreakExcludedBadDigest: the carried SourceSemanticDigest does not
	// match the digest recomputed from the canonical envelope. Excluded.
	F2StreakExcludedBadDigest F2StreakDiagnosticKind = "excluded_bad_digest"
	// F2StreakExcludedMissingEnvelope: the sidecar's CanonicalEnvelope is nil.
	// Excluded.
	F2StreakExcludedMissingEnvelope F2StreakDiagnosticKind = "excluded_missing_envelope"
	// F2StreakExcludedMissingMetadata: the sidecar's F2ViewMetadata is empty
	// (zero SynthesisCycleID). Excluded.
	F2StreakExcludedMissingMetadata F2StreakDiagnosticKind = "excluded_missing_metadata"
	// F2StreakExcludedEmptyCycleID: the canonical envelope's SynthesisCycleID
	// is empty. Excluded (a cycle with no identity cannot be ordered).
	F2StreakExcludedEmptyCycleID F2StreakDiagnosticKind = "excluded_empty_cycle_id"
	// F2StreakWarnNoR1Entry: the cycle has no r1_cross_lane_join entry. The
	// cycle is still INCLUDED (it appears in chronological position with a
	// bounded-absence notice) — it is not removed from the streak.
	F2StreakWarnNoR1Entry F2StreakDiagnosticKind = "warn_no_r1_entry"
)

// F2StreakScanDiagnostic records one per-sidecar diagnostic during a streak
// scan. Excluded diagnostics name a cycle that was removed from the view;
// warn diagnostics name a cycle that remains but carried an advisory.
type F2StreakScanDiagnostic struct {
	// Path is the file path the diagnostic refers to.
	Path string `json:"path"`
	// CycleID is the cycle identity if it could be read; "" otherwise.
	CycleID string `json:"cycle_id,omitempty"`
	// Kind is the diagnostic category (see F2StreakDiagnosticKind constants).
	Kind F2StreakDiagnosticKind `json:"kind"`
	// Detail is a human-readable explanation of the diagnostic.
	Detail string `json:"detail"`
}

// F2R1StreakCycleView is one cycle's projection within the R1 streak. It
// carries only the canonical fields the streak displays, copied verbatim from
// the sidecar — no new relationships, no inferred fields.
type F2R1StreakCycleView struct {
	// SynthesisCycleID is the canonical cycle identity (the sort key).
	SynthesisCycleID string `json:"synthesis_cycle_id"`
	// EntryIDs are the sorted canonical entry_id values F2 retains.
	EntryIDs []string `json:"entry_ids"`
	// SourceSemanticDigest is the binding digest (recomputed + matched).
	SourceSemanticDigest string `json:"source_semantic_digest"`
	// SchemaVersion is the F1 envelope schema version (carried, not invented).
	SchemaVersion string `json:"schema_version"`
	// ProjectionVersion is the MD projection format version.
	ProjectionVersion string `json:"projection_version"`
	// RendererVersion is the MD renderer code version.
	RendererVersion string `json:"renderer_version"`
	// WriteTimestamp is the RFC3339 UTC timestamp when F2 wrote the pair.
	WriteTimestamp string `json:"write_timestamp"`
	// CanonicalSidecarPath is the path to the canonical.json member.
	CanonicalSidecarPath string `json:"canonical_sidecar_path"`
	// ReciprocalLocator is the relative path to the .md projection member.
	ReciprocalLocator string `json:"reciprocal_locator"`
	// R1EntryID is the entry_id of the r1_cross_lane_join entry, or "" when
	// the cycle declared no R1 entry (bounded absence).
	R1EntryID string `json:"r1_entry_id,omitempty"`
	// R1Conclusions is the verbatim conclusions slice from the R1 entry.
	// nil when the cycle declared no R1 entry.
	R1Conclusions []F1R1Conclusion `json:"r1_conclusions,omitempty"`
	// HasR1Entry reports whether the cycle declared an r1_cross_lane_join
	// entry. When false, the cycle appears in the streak at its chronological
	// position with a bounded-absence notice (it is NOT removed).
	HasR1Entry bool `json:"has_r1_entry"`
}

// F2R1StreakView is the whole R1 streak: a chronologically-ordered list of
// per-cycle views plus the renderer version that produced it. It is a pure
// snapshot of the scan at call time — no shared mutable state.
type F2R1StreakView struct {
	// Cycles is the chronologically-ordered (by SynthesisCycleID) list of
	// valid cycles. Each cycle carries its R1 conclusions verbatim.
	Cycles []F2R1StreakCycleView `json:"cycles"`
	// RendererVersion is the streak renderer code version (printed in output).
	RendererVersion string `json:"renderer_version"`
	// ExcludedCount is the number of sidecars excluded (corrupt/bad-digest/
	// missing-envelope/missing-metadata/empty-cycle). Excluded cycles do not
	// appear in Cycles; they appear in the diagnostics slice.
	ExcludedCount int `json:"excluded_count"`
}

// F2StreakRendererVersion identifies the streak renderer code version.
const F2StreakRendererVersion = "1"

// ScanF2R1Streak scans a directory of canonical sidecars
// (docs/checkpoints/f2/*.canonical.json) and produces the R1-derived operator
// synthesis streak.
//
// Pure: no mutation of the filesystem, no shared mutable index, no model calls.
// The streak is re-derived on every call from the immutable sidecars present
// at scan time.
//
// Ordering is canonical chronology: cycles are sorted by SynthesisCycleID
// (stable lexicographic), NEVER by filesystem mtime (memo L281-282).
//
// Digest gate (memo L272-275 — "the semantic digest"): each sidecar's carried
// SourceSemanticDigest is re-derived from its CanonicalEnvelope and compared.
// A mismatch excludes the cycle from the streak (reported as a diagnostic).
// The streak never displays content whose canonical binding is broken.
//
// F2 "must NOT" fence (memo L277-278): the scanner never infers a streak
// relationship, creates a hazard link, collapses a contradiction into
// agreement, derives global absence from bounded absence, replaces canonical
// ancestry, or authors a conclusion. Every byte of R1 content in the view is
// a verbatim copy of the canonical envelope's R1 conclusions.
func ScanF2R1Streak(dir string) (*F2R1StreakView, []F2StreakScanDiagnostic, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("f2 streak: cannot read directory %q: %w", dir, err)
	}

	var (
		cycles      []F2R1StreakCycleView
		diagnostics []F2StreakScanDiagnostic
		excluded    int
	)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".canonical.json") {
			continue
		}
		path := filepath.Join(dir, name)

		view, diags, excluded := scanF2SidecarForStreak(path)
		diagnostics = append(diagnostics, diags...)
		if excluded {
			continue
		}
		cycles = append(cycles, *view)
	}

	// Stable chronological ordering by SynthesisCycleID (lexicographic).
	// sort.SliceStable is deterministic for equal keys; combined with the
	// unique-cycle-per-sidecar invariant this gives a total order.
	sort.SliceStable(cycles, func(i, j int) bool {
		return cycles[i].SynthesisCycleID < cycles[j].SynthesisCycleID
	})

	// Count excluded sidecars for the view's ExcludedCount.
	for _, d := range diagnostics {
		if strings.HasPrefix(string(d.Kind), "excluded_") {
			excluded++
		}
	}

	return &F2R1StreakView{
		Cycles:          cycles,
		RendererVersion: F2StreakRendererVersion,
		ExcludedCount:   excluded,
	}, diagnostics, nil
}

// scanF2SidecarForStreak reads one sidecar, verifies its digest, and returns
// either a populated cycle view (included) or a diagnostic (excluded). A
// cycle with no R1 entry is INCLUDED with a warn diagnostic — it is not
// removed from the streak.
func scanF2SidecarForStreak(path string) (*F2R1StreakCycleView, []F2StreakScanDiagnostic, bool) {
	sidecar, err := ReadF2CanonicalSidecar(path)
	if err != nil {
		return nil, []F2StreakScanDiagnostic{{
			Path:   path,
			Kind:   F2StreakExcludedCorruptJSON,
			Detail: fmt.Sprintf("could not parse sidecar: %v", err),
		}}, true
	}

	env := sidecar.CanonicalEnvelope
	if env == nil {
		return nil, []F2StreakScanDiagnostic{{
			Path:   path,
			Kind:   F2StreakExcludedMissingEnvelope,
			Detail: "sidecar CanonicalEnvelope is nil",
		}}, true
	}

	if env.SynthesisCycleID == "" {
		return nil, []F2StreakScanDiagnostic{{
			Path:   path,
			Kind:   F2StreakExcludedEmptyCycleID,
			Detail: "canonical envelope SynthesisCycleID is empty",
		}}, true
	}

	meta := sidecar.F2ViewMetadata
	if meta.SynthesisCycleID == "" {
		return nil, []F2StreakScanDiagnostic{{
			Path:    path,
			CycleID: env.SynthesisCycleID,
			Kind:    F2StreakExcludedMissingMetadata,
			Detail:  "F2ViewMetadata is empty (no SynthesisCycleID)",
		}}, true
	}

	// Digest gate: re-derive from the canonical envelope and compare.
	recomputed, dErr := env.ComputeDigest()
	if dErr != nil {
		return nil, []F2StreakScanDiagnostic{{
			Path:    path,
			CycleID: env.SynthesisCycleID,
			Kind:    F2StreakExcludedBadDigest,
			Detail:  fmt.Sprintf("cannot recompute digest: %v", dErr),
		}}, true
	}
	if recomputed != meta.SourceSemanticDigest {
		return nil, []F2StreakScanDiagnostic{{
			Path:    path,
			CycleID: env.SynthesisCycleID,
			Kind:    F2StreakExcludedBadDigest,
			Detail: fmt.Sprintf("digest mismatch: recomputed %q != carried %q",
				recomputed, meta.SourceSemanticDigest),
		}}, true
	}

	// Extract the R1 entry by family name (does NOT assume entry order).
	var (
		r1Entry   *F1FamilyEntry
		r1EntryID string
		r1Concl   []F1R1Conclusion
		hasR1     bool
	)
	for i := range env.Entries {
		e := &env.Entries[i]
		if e.Family == F1FamilyR1CrossLaneJoin {
			r1Entry = e
			r1EntryID = e.EntryID
			hasR1 = true
			if e.R1 != nil {
				r1Concl = e.R1.Conclusions
			}
			break
		}
	}

	var diags []F2StreakScanDiagnostic
	if !hasR1 {
		// Warn but INCLUDE: the cycle appears at its chronological position
		// with a bounded-absence notice. It is not removed from the streak.
		diags = append(diags, F2StreakScanDiagnostic{
			Path:    path,
			CycleID: env.SynthesisCycleID,
			Kind:    F2StreakWarnNoR1Entry,
			Detail:  "cycle declares no r1_cross_lane_join entry (bounded absence, not removed)",
		})
	}

	// Copy entry IDs verbatim (already sorted in F2ArtifactViewMeta).
	entryIDs := make([]string, len(meta.EntryIDs))
	copy(entryIDs, meta.EntryIDs)

	// Deep-copy R1 conclusions so the view owns its own memory (the sidecar
	// read may be GC'd after the scan returns).
	var conclusions []F1R1Conclusion
	if r1Concl != nil {
		conclusions = make([]F1R1Conclusion, len(r1Concl))
		copy(conclusions, r1Concl)
		// Also copy the slice fields inside each conclusion (defensive deep
		// copy — the view must not share backing arrays with the sidecar).
		for i := range conclusions {
			conclusions[i].Lanes = copyLaneSlice(r1Concl[i].Lanes)
			conclusions[i].Sources = copySourceSlice(r1Concl[i].Sources)
			conclusions[i].Agreements = copyStringSlice(r1Concl[i].Agreements)
			conclusions[i].Contradictions = copyContradictionSlice(r1Concl[i].Contradictions)
			conclusions[i].Gaps = copyGapSlice(r1Concl[i].Gaps)
			conclusions[i].Hazards = copyHazardSlice(r1Concl[i].Hazards)
		}
	}

	view := &F2R1StreakCycleView{
		SynthesisCycleID:     env.SynthesisCycleID,
		EntryIDs:             entryIDs,
		SourceSemanticDigest: meta.SourceSemanticDigest,
		SchemaVersion:        meta.SchemaVersion,
		ProjectionVersion:    meta.ProjectionVersion,
		RendererVersion:      meta.RendererVersion,
		WriteTimestamp:       meta.WriteTimestamp,
		CanonicalSidecarPath: path,
		ReciprocalLocator:    meta.ReciprocalLocator,
		R1EntryID:            r1EntryID,
		R1Conclusions:        conclusions,
		HasR1Entry:           hasR1,
	}
	_ = r1Entry // r1Entry captured for clarity; view carries its fields.
	return view, diags, false
}

// copyStringSlice returns a defensive copy of a string slice.
func copyStringSlice(ss []string) []string {
	if ss == nil {
		return nil
	}
	out := make([]string, len(ss))
	copy(out, ss)
	return out
}

// copyLaneSlice returns a defensive copy of a lane-contrib slice.
func copyLaneSlice(ls []F1R1LaneContrib) []F1R1LaneContrib {
	if ls == nil {
		return nil
	}
	out := make([]F1R1LaneContrib, len(ls))
	copy(out, ls)
	return out
}

// copySourceSlice returns a defensive copy of a source slice (including the
// AncestryRoots sub-slice).
func copySourceSlice(ss []F1R1Source) []F1R1Source {
	if ss == nil {
		return nil
	}
	out := make([]F1R1Source, len(ss))
	for i := range ss {
		out[i].Locator = ss[i].Locator
		out[i].AncestryRoots = copyStringSlice(ss[i].AncestryRoots)
	}
	return out
}

// copyContradictionSlice returns a defensive copy of a contradiction slice.
func copyContradictionSlice(cs []F1R1Contradiction) []F1R1Contradiction {
	if cs == nil {
		return nil
	}
	out := make([]F1R1Contradiction, len(cs))
	copy(out, cs)
	return out
}

// copyGapSlice returns a defensive copy of a gap slice.
func copyGapSlice(gs []F1R1Gap) []F1R1Gap {
	if gs == nil {
		return nil
	}
	out := make([]F1R1Gap, len(gs))
	copy(out, gs)
	return out
}

// copyHazardSlice returns a defensive copy of a hazard-link slice (including
// all its sub-slices — symptom_refs, source_locators, ancestry_roots,
// consuming_r3/pa option/probe IDs).
func copyHazardSlice(hs []F1R1HazardLink) []F1R1HazardLink {
	if hs == nil {
		return nil
	}
	out := make([]F1R1HazardLink, len(hs))
	for i := range hs {
		out[i].HazardRef = hs[i].HazardRef
		out[i].SymptomRefs = copyStringSlice(hs[i].SymptomRefs)
		out[i].SourceLocators = copyStringSlice(hs[i].SourceLocators)
		out[i].AncestryRoots = copyStringSlice(hs[i].AncestryRoots)
		out[i].ContradictionRef = hs[i].ContradictionRef
		out[i].GapRef = hs[i].GapRef
		out[i].ConsumingR3OptionIDs = copyStringSlice(hs[i].ConsumingR3OptionIDs)
		out[i].ConsumingPAProbeIDs = copyStringSlice(hs[i].ConsumingPAProbeIDs)
	}
	return out
}

// RenderF2R1Streak produces the deterministic Markdown bytes for the R1
// streak view. Pure: no filesystem access, no model calls, no narrative
// generation. Every byte is derivable from the view by this same code path.
//
// The streak output:
//  1. A standing notice identifying the streak as derived/non-authoritative.
//  2. The renderer version (printed, deterministic).
//  3. The excluded-count (how many sidecars were dropped for bad digest/corrupt
//     bytes — so the operator knows the view is not silently complete).
//  4. A per-cycle section in chronological order, each rendering the cycle's R1
//     conclusions VERBATIM (all fields: disposition, lanes, sources,
//     agreements, contradictions, gaps, hazard links — exactly as declared).
//
// NO NEW RELATIONSHIPS: the streak renders each cycle independently. There is
// no cross-cycle "this conclusion follows from that one" linkage text. Cycles
// appear in chronological order; their conclusions appear verbatim. Any
// inference that the cycles form a narrative belongs to the reader, not F2.
func RenderF2R1Streak(view *F2R1StreakView) ([]byte, error) {
	if view == nil {
		return nil, fmt.Errorf("f2 streak render: view is nil")
	}

	var b strings.Builder
	b.WriteString("## F2 R1-derived operator synthesis streak\n\n")
	b.WriteString("> **Derived, informational, non-authoritative.** This streak is a\n")
	b.WriteString("> deterministic projection of the canonical F2 sidecars. It carries no\n")
	b.WriteString("> canonical meaning beyond what each cycle's digest-bound F1 emit\n")
	b.WriteString("> declared. F2 formats; it does NOT infer streak continuity, create hazard\n")
	b.WriteString("> links across cycles, collapse contradictions into agreement, derive\n")
	b.WriteString("> global absence from bounded absence, replace canonical ancestry, or\n")
	b.WriteString("> author a conclusion.\n\n")

	fmt.Fprintf(&b, "- **Streak renderer version:** `%s`\n", view.RendererVersion)
	fmt.Fprintf(&b, "- **Cycles in view:** %d\n", len(view.Cycles))
	fmt.Fprintf(&b, "- **Excluded sidecars:** %d (bad digest / corrupt / missing envelope)\n", view.ExcludedCount)
	b.WriteString("\n")

	if len(view.Cycles) == 0 {
		b.WriteString("<!-- f2-r1-streak:begin -->\n")
		b.WriteString("(no valid F2 cycles in checked scope)\n")
		b.WriteString("<!-- f2-r1-streak:end -->\n")
		return []byte(b.String()), nil
	}

	b.WriteString("<!-- f2-r1-streak:begin -->\n\n")
	for _, cycle := range view.Cycles {
		renderF2StreakCycle(&b, cycle)
	}
	b.WriteString("<!-- f2-r1-streak:end -->\n")

	return []byte(b.String()), nil
}

// renderF2StreakCycle renders one cycle's section in the streak. The cycle's
// R1 conclusions are rendered verbatim via renderF2R1Summary (the same
// deterministic code path the per-cycle MD projection uses).
func renderF2StreakCycle(b *strings.Builder, cycle F2R1StreakCycleView) {
	fmt.Fprintf(b, "### Cycle `%s`\n\n", cycle.SynthesisCycleID)
	fmt.Fprintf(b, "- **Source semantic digest:** `%s`\n", cycle.SourceSemanticDigest)
	fmt.Fprintf(b, "- **Schema version:** `%s`\n", cycle.SchemaVersion)
	fmt.Fprintf(b, "- **Projection version:** `%s` / renderer `%s`\n", cycle.ProjectionVersion, cycle.RendererVersion)
	fmt.Fprintf(b, "- **Write timestamp:** `%s`\n", cycle.WriteTimestamp)
	if len(cycle.EntryIDs) > 0 {
		fmt.Fprintf(b, "- **Entry IDs:** %s\n", f2RenderStringList(cycle.EntryIDs))
	}
	fmt.Fprintf(b, "- **Canonical sidecar:** `%s`\n", cycle.CanonicalSidecarPath)
	if cycle.ReciprocalLocator != "" {
		fmt.Fprintf(b, "- **Reciprocal locator (MD):** `%s`\n", cycle.ReciprocalLocator)
	}
	b.WriteString("\n")

	if !cycle.HasR1Entry {
		b.WriteString("##### R1 conclusions\n\n")
		b.WriteString("(no r1_cross_lane_join entry declared in this cycle — bounded absence)\n\n")
		return
	}

	fmt.Fprintf(b, "##### R1 entry `%s`\n\n", cycle.R1EntryID)
	// Reuse the per-cycle renderer so the streak's R1 rendering is byte-
	// identical to the per-cycle MD projection's R1 rendering. This guarantees
	// the streak introduces NO new formatting, NO new fields, NO new
	// relationships — only the same deterministic projection, ordered.
	renderF2R1Summary(b, &F1R1JoinSummary{Conclusions: cycle.R1Conclusions})
}

// SerializeF2R1StreakView returns the deterministic JSON serialization of the
// streak view. Exported so the doctor (Slice 9) and tests can round-trip the
// view. encoding/json marshals struct fields in declaration order and sorts
// map keys, so the output is deterministic for a given struct value.
func SerializeF2R1StreakView(view *F2R1StreakView) ([]byte, error) {
	return json.MarshalIndent(view, "", "  ")
}
