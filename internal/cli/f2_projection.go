package cli

// f2_projection.go — deterministic Markdown projection + pair-level
// persistence coordination (Slice 3 of the F2 rendering/persistence family).
//
// DESIGN AUTHORITY: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), Decision 1 (L52-89) +
// ingest steps 4-5 (L135-139).
//
// ARCHITECTURE (Decision 1):
//   docs/checkpoints/f2/<synthesis_cycle_id>.canonical.json   # Slice 2
//   docs/checkpoints/f2/<synthesis_cycle_id>.md               # THIS FILE
//
// The Markdown projection is "how F2 displays it" — derived ONLY from the
// canonical sidecar, never an independent source of semantic content. It is
// regenerable: given the canonical sidecar, the MD can always be reproduced
// byte-for-byte by the same deterministic code path.
//
// STANDING NOTICE (memo L71-73): the MD self-identifies as "Derived,
// informational, and non-authoritative. Canonical meaning remains in the
// digest-bound F1 emit."
//
// NO FREE-FORM MODEL SUMMARIZATION (memo L261-264): the renderer is pure
// deterministic code that walks the canonical envelope struct and emits
// formatted Markdown. It NEVER calls a model, generates narrative, or produces
// an independent summary. Every byte is derivable from the canonical sidecar
// by the same deterministic code path. This is the load-bearing fence for the
// rendering layer: F2 formats; it does NOT synthesize.
//
// PAIR COORDINATION (memo L135-139, the collision contract for the PAIR):
//   - neither canonical.json nor .md exists  → write both;
//   - both exist and both match the ingest     → idempotent no-op;
//   - either exists with different content     → refuse (new cycle required);
//   - only one exists                          → report an incomplete pair
//     (do NOT auto-complete — the operator investigates why one is missing).
//
// F2 RENDERING IS INFORM-ONLY (memo L373: "Markdown projection — INFORM —
// Deterministic display only"). It may refuse to produce/overwrite an artifact
// (artifact integrity); it cannot block another system transition.
//
// HONESTY CEILING (memo L362-382): a successfully-rendered projection is a
// faithful, deterministic display of the canonical content; it is NOT thereby
// proven to describe conclusions that are actually true. Rendering is
// structural display, not semantic verification.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// --- MD path ---------------------------------------------------------------

// F2MarkdownProjectionPath returns the MD projection file path for a cycle
// within a directory. Exported so the doctor (Slice 9) and the streak scanner
// (Slice 8) use the same path convention as the renderer.
func F2MarkdownProjectionPath(dir, cycleID string) string {
	return filepath.Join(dir, cycleID+".md")
}

// f2ViewMetadataRe matches a fenced code block whose info string begins with
// "f2-view-metadata" and captures the block body (JSON of F2ArtifactViewMeta).
// Used by the pair-consistency check and the doctor (Slice 9) to extract the
// projection's view metadata without re-rendering the full file.
//
// Deliberately a double-quoted string (the pattern contains literal backticks),
// matching the F1 pattern (f1EnvelopeRe in doctor_f1.go).
var f2ViewMetadataRe = regexp.MustCompile("(?s)```f2-view-metadata[ \\t]*\\n(.*?)\\n```")

// --- MD rendering (pure, deterministic) -------------------------------------

// RenderF2MarkdownProjection produces the deterministic Markdown bytes for the
// MD projection of a canonical sidecar. Pure: no filesystem access, no model
// calls, no narrative generation. Every byte is derivable from the sidecar by
// this same code path.
//
// The projection contains:
//  1. A standing notice identifying the MD as derived/non-authoritative.
//  2. A fenced f2-view-metadata JSON block (the F2ArtifactViewMeta for this
//     projection, with its reciprocal locator pointing back to the canonical
//     sidecar — distinct from the canonical sidecar's own reciprocal locator
//     which points forward to the MD).
//  3. A faithful, deterministic rendering of every canonical envelope field:
//     schema version, cycle ID, applicability, digest, validation, and every
//     entry (R1/R3/P-a) with all its sub-fields.
//
// Two calls with the same sidecar + dir produce identical bytes (deterministic
// rendering — required for byte-stable reruns and collision detection). The
// renderer walks slices in declaration order and uses no map iteration on data
// fields, so Go's non-deterministic map ordering cannot affect the output.
func RenderF2MarkdownProjection(sidecar *F2CanonicalSidecar, dir string) ([]byte, error) {
	if sidecar == nil {
		return nil, fmt.Errorf("f2 render: sidecar is nil")
	}
	if sidecar.CanonicalEnvelope == nil {
		return nil, fmt.Errorf("f2 render: canonical envelope is nil")
	}

	env := sidecar.CanonicalEnvelope
	cycle := sidecar.F2ViewMetadata.SynthesisCycleID
	if cycle == "" {
		return nil, fmt.Errorf("f2 render: sidecar carries no synthesis_cycle_id")
	}

	// R5 binding validation gate (defense-in-depth): if the sidecar carries an
	// R5 binding, its SourceLocators must EXACTLY match the canonical entry's
	// SourceRefs. A hand-constructed sidecar with a tampered binding is rejected
	// here — it never reaches the rendered MD projection.
	if sidecar.R5Binding != nil {
		if vErr := ValidateF2R5BindingAgainstEnvelope(sidecar.R5Binding, env); vErr != nil {
			return nil, fmt.Errorf("f2 render: R5 binding validation failed (render-path gate): %w", vErr)
		}
	}

	// P-b media attachment validation gate (defense-in-depth): if the sidecar
	// carries media attachments, each is structurally validated against the
	// canonical envelope. A hand-constructed sidecar with tampered attachments
	// is rejected here — they never reach the rendered MD projection.
	for i := range sidecar.MediaAttachments {
		if vErr := ValidateF2MediaAttachmentAgainstEnvelope(&sidecar.MediaAttachments[i], env); vErr != nil {
			return nil, fmt.Errorf("f2 render: media attachment[%d] validation failed (render-path gate): %w", i, vErr)
		}
	}

	canonPath := F2CanonicalSidecarPath(dir, cycle)

	// Build the MD-specific view metadata: a copy of the canonical sidecar's
	// metadata, with the reciprocal locator repointed to the canonical sidecar
	// (the canonical's own reciprocal locator points forward to this MD; the
	// MD's reciprocal locator points back to the canonical — they are a pair).
	mdMeta := sidecar.F2ViewMetadata
	mdMeta.ReciprocalLocator = canonPath

	metaJSON, err := json.MarshalIndent(mdMeta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("f2 render: cannot serialize view metadata: %w", err)
	}

	var b strings.Builder

	// --- Title ---
	fmt.Fprintf(&b, "# F2 Projection — Synthesis Cycle `%s`\n\n", cycle)

	// --- Standing notice (exact wording from memo L71-73) ---
	fmt.Fprintf(&b, "> **Derived, informational, and non-authoritative.** Canonical meaning remains in the digest-bound F1 emit at `%s`.\n\n", canonPath)

	// --- F2 view metadata block (fenced JSON for doctor parsing) ---
	b.WriteString("## F2 View Metadata\n\n")
	fmt.Fprintf(&b, "```f2-view-metadata\n%s\n```\n\n", string(metaJSON))

	// --- P-c headline (FIRST DISCLOSURE LAYER — memo L240-264) ---
	// The P-c section surfaces decision-relevant salience BEFORE the detailed
	// envelope projection. Counter-evidence and weakest claim MUST appear in
	// this first layer, not buried in the details below.
	renderF2PCHeadline(&b, env, mdMeta)

	// --- P-a decision-request table (memo L289-301) ---
	// Structured per-option decision matrix from canonical R3 option records +
	// P-a probes. A second salience layer after the headline, before the
	// detailed envelope projection.
	renderF2PATable(&b, env)

	// --- R5 operator-synthesis durable binding (memo L155-182) ---
	// Addressable binding section. nil binding → bounded "(no operator-source
	// synthesis bound)" notice (F2 does NOT infer a binding from narrative).
	renderF2R5Binding(&b, sidecar.R5Binding)

	// --- P-b evidence-grade media provenance (memo L184-233) ---
	// Domain-free capability-class slot. Empty → bounded "(no evidence-grade
	// media attachments)" notice. All provenance is structurally present but
	// content truth is NOT verified (honesty ceiling).
	renderF2MediaAttachments(&b, sidecar.MediaAttachments)

	// --- Canonical envelope (projected) ---
	b.WriteString("## Canonical Envelope (projected)\n\n")
	fmt.Fprintf(&b, "- **Schema version:** `%s`\n", env.SchemaVersion)
	fmt.Fprintf(&b, "- **Synthesis cycle ID:** `%s`\n", env.SynthesisCycleID)
	fmt.Fprintf(&b, "- **Applicability:** `%s`\n", env.Applicability)
	if env.SemanticDigest != "" {
		fmt.Fprintf(&b, "- **Semantic digest:** `%s`\n", env.SemanticDigest)
	}
	fmt.Fprintf(&b, "- **Validation disposition:** `%s`\n", env.Validation.Disposition)
	b.WriteString("\n")

	// --- Entries ---
	if len(env.Entries) > 0 {
		b.WriteString("### Entries\n\n")
		for _, entry := range env.Entries {
			renderF2EntrySection(&b, entry)
		}
	}

	return []byte(b.String()), nil
}

// renderF2PCHeadline renders the P-c headline-layer salience section (memo
// L235-264). This is the FIRST DISCLOSURE LAYER of the MD projection: it
// surfaces decision-relevant salience before the detailed envelope sections.
//
// The section contains, IN ORDER (memo L243-248):
//  1. Decision frame (R3 trigger + options);
//  2. Current disposition/verdict (R3 disposition, VERBATIM from canonical);
//  3. Counter-evidence (all P-a probes with their result enum preserved EXACTLY);
//  4. Weakest claim (P-a probes' WeakestClaim field);
//  5. Unresolved gaps (R1 conclusions' Gaps field);
//  6. Canonical binding metadata (cycle, entries, digest).
//
// Counter-evidence and weakest claim MUST be in this first layer, not merely
// linked or appendiced (memo L250-251).
//
// NO SEMANTIC SUMMARIZATION (memo L261-264): every value is a verbatim
// projection of a canonical field. The renderer NEVER asks a model to
// "summarize for the headline." All values trace to canonical entry IDs.
//
// DISPLAYED DISPOSITION == CANONICAL DISPOSITION (memo L255): the R3
// disposition is rendered exactly as carried — a pending decision stays
// pending, never upgraded to "approved" or "resolved."
//
// STRUCTURAL MARKERS: the section uses fenced HTML comments
// (f2-pc-headline:begin / :end) so the doctor (Slice 9) can reliably locate
// the P-c layer and verify its required sub-sections.
func renderF2PCHeadline(b *strings.Builder, env *F1SynthesisEnvelope, meta F2ArtifactViewMeta) {
	// Find the R3, P-a, and R1 entries by family name (do NOT assume order).
	var r3Entry, paEntry, r1Entry *F1FamilyEntry
	for i := range env.Entries {
		switch env.Entries[i].Family {
		case F1FamilyR3RedesignFork:
			r3Entry = &env.Entries[i]
		case F1FamilyPACounterEvidence:
			paEntry = &env.Entries[i]
		case F1FamilyR1CrossLaneJoin:
			r1Entry = &env.Entries[i]
		}
	}

	b.WriteString("<!-- f2-pc-headline:begin -->\n")
	b.WriteString("## P-c Headline — Decision Salience Layer\n\n")
	b.WriteString("> First disclosure layer. Counter-evidence and weakest claim surface here, before the detailed sections. Values are deterministic projections of canonical entries — no model summarization.\n\n")

	// --- 1. Decision frame ---
	b.WriteString("### Decision Frame\n")
	if r3Entry != nil && r3Entry.R3 != nil {
		r3 := r3Entry.R3
		fmt.Fprintf(b, "- **Trigger recognized:** `%t`\n", r3.TriggerRecognized)
		if len(r3.Options) > 0 {
			b.WriteString("- **Options:**")
			for i, opt := range r3.Options {
				if i == 0 {
					fmt.Fprintf(b, " `%s` (%s)", opt.OptionID, opt.Mode)
				} else {
					fmt.Fprintf(b, ", `%s` (%s)", opt.OptionID, opt.Mode)
				}
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("- No R3 redesign fork in this synthesis cycle.\n")
	}
	b.WriteString("\n")

	// --- 2. Current disposition/verdict (VERBATIM from canonical R3) ---
	b.WriteString("### Current Disposition\n")
	if r3Entry != nil && r3Entry.R3 != nil {
		// Displayed disposition == canonical disposition (memo L255).
		// NEVER upgrade, downgrade, or paraphrase.
		fmt.Fprintf(b, "- **R3 disposition:** `%s`\n", r3Entry.R3.Disposition)
		if r3Entry.R3.Selection != nil {
			fmt.Fprintf(b, "- **Selected option:** `%s`\n", r3Entry.R3.Selection.SelectedOptionID)
		}
	} else {
		b.WriteString("- No R3 disposition (no redesign fork triggered).\n")
	}
	b.WriteString("\n")

	// --- 3. Counter-evidence (all P-a probes, result enum preserved EXACTLY) ---
	b.WriteString("### Counter-evidence\n")
	if paEntry != nil && paEntry.PA != nil && len(paEntry.PA.Probes) > 0 {
		for _, p := range paEntry.PA.Probes {
			// The result enum is rendered EXACTLY — never collapsed or
			// paraphrased (memo L295-301). not_found_in_checked_scope NEVER
			// renders as "none exists."
			fmt.Fprintf(b, "- Probe `%s` (target `%s`): `%s`", p.ProbeID, p.TargetRef, p.Result)
			if len(p.EvidenceRefs) > 0 {
				fmt.Fprintf(b, " — evidence: %s", f2RenderStringList(p.EvidenceRefs))
			}
			if p.Limitation != "" {
				fmt.Fprintf(b, " — limitation: %s", p.Limitation)
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("- No counter-evidence probes in this synthesis cycle.\n")
	}
	b.WriteString("\n")

	// --- 4. Weakest claim (P-a probes' WeakestClaim field) ---
	b.WriteString("### Weakest Claim\n")
	weakestFound := false
	if paEntry != nil && paEntry.PA != nil {
		for _, p := range paEntry.PA.Probes {
			if p.WeakestClaim != "" {
				fmt.Fprintf(b, "- Probe `%s`: %s\n", p.ProbeID, p.WeakestClaim)
				weakestFound = true
			}
		}
	}
	if !weakestFound {
		b.WriteString("- (none declared in canonical P-a probes)\n")
	}
	b.WriteString("\n")

	// --- 5. Unresolved gaps (R1 conclusions' Gaps field) ---
	b.WriteString("### Unresolved Gaps\n")
	gapsFound := false
	if r1Entry != nil && r1Entry.R1 != nil {
		for _, c := range r1Entry.R1.Conclusions {
			for _, g := range c.Gaps {
				fmt.Fprintf(b, "- Conclusion `%s` gap `%s` (%s): %s\n", c.ConclusionID, g.GapID, g.Aspect, g.Detail)
				gapsFound = true
			}
		}
	}
	if !gapsFound {
		b.WriteString("- (none declared in canonical R1 conclusions)\n")
	}
	b.WriteString("\n")

	// --- 6. Canonical binding metadata ---
	b.WriteString("### Canonical Binding Metadata\n")
	fmt.Fprintf(b, "- **Cycle:** `%s`\n", meta.SynthesisCycleID)
	if len(meta.EntryIDs) > 0 {
		fmt.Fprintf(b, "- **Entry IDs:** %s\n", f2RenderStringList(meta.EntryIDs))
	}
	if meta.SourceSemanticDigest != "" {
		fmt.Fprintf(b, "- **Source digest:** `%s`\n", meta.SourceSemanticDigest)
	}
	b.WriteString("\n")

	b.WriteString("<!-- f2-pc-headline:end -->\n\n")
}

// renderF2PATable renders the P-a decision-request table (memo L289-301).
// This is a structured per-option decision matrix rendered from canonical R3
// option records + P-a probes. It is a SALIENCE LAYER (second, after the P-c
// headline) that organizes the same canonical data into a per-option view.
//
// Required columns (memo L291-292): Option | Costs | Evidence against |
// Weakest claim | Reversal cost.
//
// CANONICAL SOURCE MAPPING (C1 build gate, resolved):
//   - Option         : F1R3Option.OptionID + Mode
//   - Costs          : F1R3Option.Costs (canonical R3 field — NOT invented)
//   - Evidence against: P-a probes referenced by the option's
//     CounterEvidenceProbeRefs, result enum preserved EXACTLY
//   - Weakest claim   : those same probes' WeakestClaim field
//   - Reversal cost   : F1R3Option.ReversalCost (canonical R3 field)
//
// PROBE-RESULT SEMANTICS PRESERVED EXACTLY (memo L295-301):
//   - found / not_found_in_checked_scope / unavailable / not_run.
//   - not_found_in_checked_scope NEVER renders as "none exists."
//   - unavailable stays distinct from a negative result.
//   - not_run stays visibly unperformed.
//   - Blank cells must not silently collapse these states.
//
// BOUNDED ABSENCE (memo L335): if a canonical field is absent (empty Costs,
// empty ReversalCost, no CounterEvidenceProbeRefs, etc.), the cell renders a
// bounded-absence marker — NEVER a fabricated value and NEVER a global
// "none exists" claim.
//
// MISSING CANONICAL SOURCE (memo L335): if no R3 entry exists at all, the
// entire section renders an incomplete-surface diagnostic — NOT a fabricated
// table with invented rows.
//
// STRUCTURAL MARKERS: the section uses fenced HTML comments
// (f2-pa-table:begin / :end) so the doctor (Slice 9) can locate it.
//
// NO SEMANTIC SUMMARIZATION: every value is a verbatim projection. The
// renderer NEVER generates alternatives, joins evidence, or produces an
// independent conclusion.
func renderF2PATable(b *strings.Builder, env *F1SynthesisEnvelope) {
	// Find the R3 and P-a entries by family name (do NOT assume order).
	var r3Entry, paEntry *F1FamilyEntry
	for i := range env.Entries {
		switch env.Entries[i].Family {
		case F1FamilyR3RedesignFork:
			r3Entry = &env.Entries[i]
		case F1FamilyPACounterEvidence:
			paEntry = &env.Entries[i]
		}
	}

	b.WriteString("<!-- f2-pa-table:begin -->\n")
	b.WriteString("## P-a Decision-Request Table\n\n")
	b.WriteString("> Deterministic per-option decision matrix from canonical R3 option records + P-a probes. Probe-result semantics preserved EXACTLY. No model summarization.\n\n")

	// Missing canonical source → incomplete-surface diagnostic (not a
	// fabricated table with invented rows — memo L335).
	if r3Entry == nil || r3Entry.R3 == nil || len(r3Entry.R3.Options) == 0 {
		b.WriteString("- (no R3 redesign fork with options in canonical emit — decision-request table not applicable)\n\n")
		b.WriteString("<!-- f2-pa-table:end -->\n\n")
		return
	}

	// Build a probe lookup index from the P-a entry (if present) so the
	// option's CounterEvidenceProbeRefs can be resolved to probe records.
	// This is a pure rendering helper — no new state is introduced.
	probeIndex := make(map[string]*F1PAProbe)
	if paEntry != nil && paEntry.PA != nil {
		for i := range paEntry.PA.Probes {
			probeIndex[paEntry.PA.Probes[i].ProbeID] = &paEntry.PA.Probes[i]
		}
	}

	// Render the table header.
	b.WriteString("| Option | Costs | Evidence against | Weakest claim | Reversal cost |\n")
	b.WriteString("|--------|-------|-----------------|---------------|---------------|\n")

	for _, opt := range r3Entry.R3.Options {
		optionCell := fmt.Sprintf("`%s` (%s)", opt.OptionID, opt.Mode)
		costsCell := f2PATableCostsCell(opt.Costs)
		evidenceCell := f2PATableEvidenceCell(opt.CounterEvidenceProbeRefs, probeIndex)
		weakestCell := f2PATableWeakestCell(opt.CounterEvidenceProbeRefs, probeIndex)
		reversalCell := f2PATableReversalCell(opt.ReversalCost)

		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
			optionCell, costsCell, evidenceCell, weakestCell, reversalCell)
	}
	b.WriteString("\n")

	b.WriteString("<!-- f2-pa-table:end -->\n\n")
}

// f2PATableCostsCell renders the Costs column from the canonical R3 option's
// Costs field. Bounded absence if empty — NEVER fabricated.
func f2PATableCostsCell(costs []string) string {
	if len(costs) == 0 {
		return "(no costs declared in canonical R3 option)"
	}
	return f2RenderStringList(costs)
}

// f2PATableReversalCell renders the Reversal-cost column from the canonical R3
// option's ReversalCost field. Bounded absence if empty — NEVER fabricated.
func f2PATableReversalCell(reversalCost string) string {
	if reversalCost == "" {
		return "(no reversal cost declared in canonical R3 option)"
	}
	return "`" + reversalCost + "`"
}

// f2PATableEvidenceCell renders the Evidence-against column from the P-a probes
// referenced by the option's CounterEvidenceProbeRefs. The probe-result enum is
// preserved EXACTLY (memo L295-301):
//   - found                     : includes evidence refs;
//   - not_found_in_checked_scope: includes checked scope, NEVER "none exists";
//   - unavailable               : includes limitation, stays distinct from negative;
//   - not_run                   : stays visibly unperformed.
//
// Bounded absence if no probes are bound. Unresolved probe refs render an
// incomplete-surface diagnostic (not a fabricated cell).
func f2PATableEvidenceCell(probeRefs []string, probeIndex map[string]*F1PAProbe) string {
	if len(probeRefs) == 0 {
		return "(no counter-evidence probe bound to this option)"
	}
	parts := make([]string, 0, len(probeRefs))
	for _, ref := range probeRefs {
		probe, ok := probeIndex[ref]
		if !ok {
			parts = append(parts, fmt.Sprintf("probe `%s` referenced but not found in canonical P-a entry", ref))
			continue
		}
		// The result enum is rendered EXACTLY — never collapsed or
		// paraphrased (memo L295-301).
		switch probe.Result {
		case F1PAResultFound:
			if len(probe.EvidenceRefs) > 0 {
				parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (evidence: %s)",
					probe.ProbeID, probe.Result, f2RenderStringList(probe.EvidenceRefs)))
			} else {
				parts = append(parts, fmt.Sprintf("probe `%s`: `%s`", probe.ProbeID, probe.Result))
			}
		case F1PAResultNotFoundInCheckedScope:
			// NEVER renders as "none exists" (memo L298). This is
			// BOUNDED absence — the scope that was checked.
			if len(probe.CheckedScope) > 0 {
				parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (checked scope: %s)",
					probe.ProbeID, probe.Result, f2RenderStringList(probe.CheckedScope)))
			} else {
				parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (scope not declared in canonical probe)",
					probe.ProbeID, probe.Result))
			}
		case F1PAResultUnavailable:
			// Stays distinct from a negative result (memo L299).
			if probe.Limitation != "" {
				parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (limitation: %s)",
					probe.ProbeID, probe.Result, probe.Limitation))
			} else {
				parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (limitation not declared in canonical probe)",
					probe.ProbeID, probe.Result))
			}
		case F1PAResultNotRun:
			// Stays visibly unperformed (memo L300).
			parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (not performed)", probe.ProbeID, probe.Result))
		default:
			// Unknown enum — render it verbatim (the F1 validator should
			// have caught this, but F2 does not reinterpret).
			parts = append(parts, fmt.Sprintf("probe `%s`: `%s` (unknown result enum)", probe.ProbeID, probe.Result))
		}
	}
	return strings.Join(parts, "; ")
}

// f2PATableWeakestCell renders the Weakest-claim column from the P-a probes
// referenced by the option's CounterEvidenceProbeRefs. Only probes that carry a
// non-empty WeakestClaim are listed. Bounded absence if none declare one.
func f2PATableWeakestCell(probeRefs []string, probeIndex map[string]*F1PAProbe) string {
	if len(probeRefs) == 0 {
		return "(no weakest claim — no probe bound to this option)"
	}
	parts := make([]string, 0, len(probeRefs))
	for _, ref := range probeRefs {
		probe, ok := probeIndex[ref]
		if !ok {
			continue // unresolved probe already diagnosed in the evidence cell
		}
		if probe.WeakestClaim != "" {
			parts = append(parts, fmt.Sprintf("probe `%s`: %s", probe.ProbeID, probe.WeakestClaim))
		}
	}
	if len(parts) == 0 {
		return "(no weakest claim declared by bound probes)"
	}
	return strings.Join(parts, "; ")
}

// renderF2EntrySection renders one family entry. Deterministic: walks the
// entry struct in field-declaration order. If the family is not triggered,
// only the header + triggered status is rendered (no family summary to
// project — a not_triggered/not_applicable entry carries none).
func renderF2EntrySection(b *strings.Builder, entry F1FamilyEntry) {
	fmt.Fprintf(b, "#### Family `%s` — Entry `%s`\n\n", entry.Family, entry.EntryID)
	fmt.Fprintf(b, "- **Triggered:** `%s`\n", entry.Triggered)
	if len(entry.SourceRefs) > 0 {
		fmt.Fprintf(b, "- **Source refs:** %s\n", f2RenderStringList(entry.SourceRefs))
	}
	b.WriteString("\n")

	switch entry.Family {
	case F1FamilyR1CrossLaneJoin:
		if entry.R1 != nil {
			renderF2R1Summary(b, entry.R1)
		}
	case F1FamilyR3RedesignFork:
		if entry.R3 != nil {
			renderF2R3Summary(b, entry.R3)
		}
	case F1FamilyPACounterEvidence:
		if entry.PA != nil {
			renderF2PASummary(b, entry.PA)
		}
	}
	b.WriteString("\n")
}

// renderF2R1Summary renders the R1 cross-lane join conclusions. Each
// conclusion carries the full join graph: disposition, lanes, sources,
// agreements, contradictions, gaps, and hazard links — all as declared by F1
// (F2 never infers a relationship or creates a new link).
func renderF2R1Summary(b *strings.Builder, r1 *F1R1JoinSummary) {
	if len(r1.Conclusions) == 0 {
		b.WriteString("(no R1 conclusions)\n\n")
		return
	}
	b.WriteString("##### R1 Conclusions\n\n")
	for _, c := range r1.Conclusions {
		fmt.Fprintf(b, "###### Conclusion `%s` — Property `%s`\n\n", c.ConclusionID, c.PropertyID)
		fmt.Fprintf(b, "- **Join disposition:** `%s`\n", c.JoinDisposition)

		if len(c.Lanes) > 0 {
			b.WriteString("- **Lanes:**\n")
			for _, lane := range c.Lanes {
				fmt.Fprintf(b, "  - `%s`", lane.LaneID)
				if lane.ActID != "" {
					fmt.Fprintf(b, " (act: `%s`", lane.ActID)
					if lane.Position != "" {
						fmt.Fprintf(b, ", position: `%s`", lane.Position)
					}
					b.WriteString(")")
				} else if lane.Position != "" {
					fmt.Fprintf(b, " (position: `%s`)", lane.Position)
				}
				b.WriteString("\n")
			}
		}

		if len(c.Sources) > 0 {
			b.WriteString("- **Sources:**\n")
			for _, src := range c.Sources {
				fmt.Fprintf(b, "  - `%s`", src.Locator)
				if len(src.AncestryRoots) > 0 {
					fmt.Fprintf(b, " (ancestry: %s)", f2RenderStringList(src.AncestryRoots))
				}
				b.WriteString("\n")
			}
		}

		if len(c.Agreements) > 0 {
			fmt.Fprintf(b, "- **Agreements refs:** %s\n", f2RenderStringList(c.Agreements))
		}

		if len(c.Contradictions) > 0 {
			b.WriteString("- **Contradictions:**\n")
			for _, con := range c.Contradictions {
				fmt.Fprintf(b, "  - `%s` (`%s` vs `%s`): %s\n", con.ContradictionID, con.LaneA, con.LaneB, con.Detail)
			}
		}

		if len(c.Gaps) > 0 {
			b.WriteString("- **Gaps:**\n")
			for _, g := range c.Gaps {
				fmt.Fprintf(b, "  - `%s` (%s): %s\n", g.GapID, g.Aspect, g.Detail)
			}
		}

		if len(c.Hazards) > 0 {
			b.WriteString("- **Hazard links:**\n")
			for _, h := range c.Hazards {
				renderF2HazardLink(b, h)
			}
		}

		b.WriteString("\n")
	}
}

// renderF2HazardLink renders one hazard<->symptom link with its explicit
// survival chain (memo: survival is NOT inferred — every leg is declared).
func renderF2HazardLink(b *strings.Builder, h F1R1HazardLink) {
	fmt.Fprintf(b, "  - Hazard `%s`", h.HazardRef)
	if len(h.SymptomRefs) > 0 {
		fmt.Fprintf(b, " → symptoms %s", f2RenderStringList(h.SymptomRefs))
	}
	if len(h.SourceLocators) > 0 {
		fmt.Fprintf(b, " → sources %s", f2RenderStringList(h.SourceLocators))
	}
	if len(h.AncestryRoots) > 0 {
		fmt.Fprintf(b, " → ancestry %s", f2RenderStringList(h.AncestryRoots))
	}
	if h.ContradictionRef != "" {
		fmt.Fprintf(b, " → contradiction `%s`", h.ContradictionRef)
	}
	if h.GapRef != "" {
		fmt.Fprintf(b, " → gap `%s`", h.GapRef)
	}
	if len(h.ConsumingR3OptionIDs) > 0 {
		fmt.Fprintf(b, " → consuming R3 options %s", f2RenderStringList(h.ConsumingR3OptionIDs))
	}
	if len(h.ConsumingPAProbeIDs) > 0 {
		fmt.Fprintf(b, " → consuming P-a probes %s", f2RenderStringList(h.ConsumingPAProbeIDs))
	}
	b.WriteString("\n")
}

// renderF2R3Summary renders the R3 redesign fork: trigger, options (each with
// all canonical fields including costs/risks/reversal_cost/cheapest_validation
// per the C1 resolution), disposition, and selection.
func renderF2R3Summary(b *strings.Builder, r3 *F1R3ForkSummary) {
	fmt.Fprintf(b, "- **Trigger recognized:** `%t`\n\n", r3.TriggerRecognized)

	if len(r3.Options) > 0 {
		b.WriteString("##### R3 Options\n\n")
		for _, opt := range r3.Options {
			fmt.Fprintf(b, "###### Option `%s` — Mode `%s`\n\n", opt.OptionID, opt.Mode)
			fmt.Fprintf(b, "- **Mechanism:** %s\n", opt.Mechanism)
			if len(opt.AffectedProperties) > 0 {
				fmt.Fprintf(b, "- **Affected properties:** %s\n", f2RenderStringList(opt.AffectedProperties))
			}
			if len(opt.SupportRefs) > 0 {
				fmt.Fprintf(b, "- **Support refs (R1 conclusions):** %s\n", f2RenderStringList(opt.SupportRefs))
			}
			if len(opt.CounterEvidenceProbeRefs) > 0 {
				fmt.Fprintf(b, "- **Counter-evidence probe refs (P-a):** %s\n", f2RenderStringList(opt.CounterEvidenceProbeRefs))
			}
			if len(opt.Costs) > 0 {
				fmt.Fprintf(b, "- **Costs:** %s\n", f2RenderStringList(opt.Costs))
			}
			if len(opt.Risks) > 0 {
				fmt.Fprintf(b, "- **Risks:** %s\n", f2RenderStringList(opt.Risks))
			}
			if opt.ReversalCost != "" {
				fmt.Fprintf(b, "- **Reversal cost:** %s\n", opt.ReversalCost)
			}
			if opt.CheapestValidation != "" {
				fmt.Fprintf(b, "- **Cheapest validation:** %s\n", opt.CheapestValidation)
			}
			b.WriteString("\n")
		}
	}

	fmt.Fprintf(b, "- **Disposition:** `%s`\n", r3.Disposition)
	if r3.Selection != nil {
		fmt.Fprintf(b, "- **Selected option:** `%s`\n", r3.Selection.SelectedOptionID)
		if r3.Selection.RedesignRejectionRationale != "" {
			fmt.Fprintf(b, "- **Redesign rejection rationale:** %s\n", r3.Selection.RedesignRejectionRationale)
		}
	}
}

// renderF2PASummary renders the P-a counter-evidence probes. The Result enum
// is preserved EXACTLY (found / not_found_in_checked_scope / unavailable /
// not_run) — it is NEVER collapsed, paraphrased, or reinterpreted.
// not_found_in_checked_scope NEVER renders as "none exists" (memo L298-301).
func renderF2PASummary(b *strings.Builder, pa *F1PAProbeSummary) {
	if len(pa.Probes) == 0 {
		b.WriteString("(no P-a probes)\n\n")
		return
	}
	b.WriteString("##### P-a Probes\n\n")
	for _, p := range pa.Probes {
		fmt.Fprintf(b, "###### Probe `%s` — Target `%s`\n\n", p.ProbeID, p.TargetRef)
		// The Result enum is rendered EXACTLY — no reinterpretation.
		fmt.Fprintf(b, "- **Result:** `%s`\n", p.Result)
		if p.FalsificationQuestion != "" {
			fmt.Fprintf(b, "- **Falsification question:** %s\n", p.FalsificationQuestion)
		}
		if p.Method != "" {
			fmt.Fprintf(b, "- **Method:** %s\n", p.Method)
		}
		if len(p.CheckedScope) > 0 {
			fmt.Fprintf(b, "- **Checked scope:** %s\n", f2RenderStringList(p.CheckedScope))
		}
		if len(p.EvidenceRefs) > 0 {
			fmt.Fprintf(b, "- **Evidence refs:** %s\n", f2RenderStringList(p.EvidenceRefs))
		}
		if p.Limitation != "" {
			fmt.Fprintf(b, "- **Limitation:** %s\n", p.Limitation)
		}
		if p.WeakestClaim != "" {
			fmt.Fprintf(b, "- **Weakest claim:** %s\n", p.WeakestClaim)
		}
		if p.Confidence != "" {
			fmt.Fprintf(b, "- **Confidence:** %s\n", p.Confidence)
		}
		b.WriteString("\n")
	}
}

// f2RenderStringList formats a string slice as a comma-separated list of
// backtick-quoted values. Returns "(none)" for an empty slice (should not
// appear in practice — callers guard with len > 0).
func f2RenderStringList(ss []string) string {
	if len(ss) == 0 {
		return "(none)"
	}
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = "`" + s + "`"
	}
	return strings.Join(quoted, ", ")
}

// --- Pair-level persistence coordination -----------------------------------

// F2PairOutcome reports what PersistF2Pair did.
type F2PairOutcome int

const (
	// F2PairNotAttempted is the zero value (should not appear in practice).
	F2PairNotAttempted F2PairOutcome = iota

	// F2PairWritten means neither file existed and both were freshly written.
	F2PairWritten

	// F2PairIdempotent means both files existed and both matched the ingest's
	// canonical content. No file was modified (original bytes preserved).
	F2PairIdempotent

	// F2PairRefused means at least one file existed with DIFFERENT canonical
	// content. Neither file was modified. A new synthesis cycle is required.
	F2PairRefused

	// F2PairIncompleteCanonicalOnly means only the canonical sidecar existed
	// (and it matched the ingest). The MD is missing. F2 did NOT auto-complete
	// the pair — the operator investigates why the MD is absent.
	F2PairIncompleteCanonicalOnly

	// F2PairIncompleteMDOnly means only the MD existed (and it matched the
	// ingest). The canonical sidecar is missing. F2 did NOT auto-complete.
	F2PairIncompleteMDOnly
)

// String returns a human-readable outcome name for diagnostics and tests.
func (o F2PairOutcome) String() string {
	switch o {
	case F2PairWritten:
		return "written"
	case F2PairIdempotent:
		return "idempotent"
	case F2PairRefused:
		return "refused"
	case F2PairIncompleteCanonicalOnly:
		return "incomplete_canonical_only"
	case F2PairIncompleteMDOnly:
		return "incomplete_md_only"
	default:
		return "not_attempted"
	}
}

// PersistF2Pair writes BOTH the canonical sidecar and the MD projection for the
// ingest result's synthesis cycle, enforcing the pair-level collision contract
// (memo L137-139):
//
//   - neither canonical.json nor .md exists → write both (F2PairWritten);
//   - both exist and both match               → idempotent no-op (F2PairIdempotent);
//   - either exists with different content    → refuse, do NOT overwrite
//     (F2PairRefused). A new synthesis cycle is required.
//   - only one exists (and it matches)        → report an incomplete pair
//     (F2PairIncompleteCanonicalOnly or F2PairIncompleteMDOnly). F2 does NOT
//     auto-complete the pair.
//
// The canonical collision key is the full envelope content
// (f2CanonicalContentFingerprint). The MD collision check is a byte-level
// re-render comparison: the stored canonical sidecar is re-rendered into the
// expected MD bytes (using the sidecar's own timestamp) and compared
// byte-for-byte against the stored MD bytes. This enforces the memo's "byte-
// identical pair" contract (L137) and detects ANY tampering of the MD prose,
// not just a changed digest. For the MD-only incomplete case (no canonical to
// re-render from), the source_semantic_digest from the metadata block is used
// as the collision key instead.
//
// `now` is injected (not time.Now()) so tests are deterministic. Both files
// share the same write timestamp (the pair is atomic in intent: both written
// in the same call with the same `now`).
//
// Returns (outcome, nil) on a handled result (written or idempotent), or
// (outcome, error) for a refused overwrite or an incomplete-pair detection
// (the error describes the issue), or (F2PairNotAttempted, error) on an I/O
// or serialization failure.
//
// F2 NEVER repairs, normalizes, or silently updates. The only recovery from a
// refused overwrite or an incomplete pair is a new F1 emit under a new cycle
// ID (for refused) or operator investigation (for incomplete).
func PersistF2Pair(ingest *F2IngestResult, dir string, now time.Time) (F2PairOutcome, error) {
	if ingest == nil {
		return F2PairNotAttempted, fmt.Errorf("f2 pair: ingest result is nil (nothing to persist)")
	}
	if ingest.CanonicalEnvelope == nil {
		return F2PairNotAttempted, fmt.Errorf("f2 pair: ingest result carries no canonical envelope")
	}
	if ingest.SynthesisCycleID == "" {
		return F2PairNotAttempted, fmt.Errorf("f2 pair: ingest result carries no synthesis_cycle_id")
	}

	// R5 binding validation gate (defense-in-depth): if the ingest carries an
	// R5 binding, its SourceLocators must EXACTLY match the canonical entry's
	// SourceRefs. A hand-constructed binding with arbitrary strings is rejected
	// here — it never reaches the durable pair.
	if ingest.R5Binding != nil {
		if vErr := ValidateF2R5BindingAgainstEnvelope(ingest.R5Binding, ingest.CanonicalEnvelope); vErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: R5 binding validation failed (durable-path gate): %w", vErr)
		}
	}

	// P-b media attachment validation gate (defense-in-depth): if the ingest
	// carries media attachments, each is structurally validated against the
	// canonical envelope. A hand-constructed attachment with arbitrary strings
	// is rejected here — it never reaches the durable pair.
	for i := range ingest.MediaAttachments {
		if vErr := ValidateF2MediaAttachmentAgainstEnvelope(&ingest.MediaAttachments[i], ingest.CanonicalEnvelope); vErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: media attachment[%d] validation failed (durable-path gate): %w", i, vErr)
		}
	}

	// Build the sidecar + serialize both artifacts (pure, before any I/O).
	sidecar := buildF2CanonicalSidecar(ingest, dir, now)
	canonBytes, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot serialize canonical sidecar: %w", err)
	}
	mdBytes, err := RenderF2MarkdownProjection(sidecar, dir)
	if err != nil {
		return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot render MD projection: %w", err)
	}

	newFP, err := f2CanonicalContentFingerprint(sidecar.CanonicalEnvelope)
	if err != nil {
		return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot compute canonical fingerprint: %w", err)
	}

	cycle := ingest.SynthesisCycleID
	canonPath := F2CanonicalSidecarPath(dir, cycle)
	mdPath := F2MarkdownProjectionPath(dir, cycle)

	canonExists := f2FileExists(canonPath)
	mdExists := f2FileExists(mdPath)

	// --- Case 1: neither exists → write both ---
	if !canonExists && !mdExists {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot create pair directory %q: %w", dir, mkErr)
		}
		// Write canonical first (O_EXCL — atomic create).
		if err := f2AtomicWrite(canonPath, canonBytes); err != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: canonical write failed: %w", err)
		}
		// Write MD (O_EXCL). If this fails, the canonical was already written
		// — the pair is now incomplete (CanonicalOnly). Report it honestly.
		if err := f2AtomicWrite(mdPath, mdBytes); err != nil {
			return F2PairIncompleteCanonicalOnly, fmt.Errorf(
				"f2 pair: canonical sidecar written at %q but MD write at %q failed (pair is now incomplete — investigate before retrying): %w",
				canonPath, mdPath, err)
		}
		return F2PairWritten, nil
	}

	// --- Case 2: both exist → check both for idempotency / refusal ---
	if canonExists && mdExists {
		// Check canonical content.
		existingCanon, cErr := readF2SidecarOrReject(canonPath)
		if cErr != nil {
			return F2PairRefused, cErr
		}
		existingFP, fpErr := f2CanonicalContentFingerprint(existingCanon.CanonicalEnvelope)
		if fpErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot fingerprint existing canonical at %q: %w", canonPath, fpErr)
		}
		if !bytes.Equal(existingFP, newFP) {
			return F2PairRefused, fmt.Errorf(
				"f2 pair: canonical content for cycle %q differs from the existing sidecar at %q (immutability: a changed canonical field requires a new F1 emit + synthesis cycle)",
				cycle, canonPath)
		}
		// Check MD content: re-render the MD from the STORED canonical sidecar
		// (using its own timestamp) and compare byte-for-byte against the
		// stored MD bytes. This is the "byte-identical pair" contract (memo
		// L137: "both exist byte-identical → idempotent no-op"). It detects
		// ANY tampering of the MD prose outside the metadata block — not just
		// a changed digest. A digest-only check would miss prose edits that
		// leave source_semantic_digest intact.
		//
		// The re-render uses the STORED sidecar (not the new one) so the
		// timestamp in the metadata block matches: two pairs written at
		// different times for the same canonical content are idempotent
		// (the canonical content match governs, not the timestamp).
		expectedMD, rErr := RenderF2MarkdownProjection(existingCanon, dir)
		if rErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot re-render MD from stored canonical at %q: %w", canonPath, rErr)
		}
		storedMD, sErr := os.ReadFile(mdPath)
		if sErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot read stored MD at %q: %w", mdPath, sErr)
		}
		if !bytes.Equal(expectedMD, storedMD) {
			return F2PairRefused, fmt.Errorf(
				"f2 pair: MD projection for cycle %q at %q does not match a deterministic re-render from the canonical sidecar (the MD was tampered or drifted — immutability: a changed projection requires investigation; F2 does not repair in place; a new F1 emit + synthesis cycle is required if the projection must change)",
				cycle, mdPath)
		}
		return F2PairIdempotent, nil
	}

	// --- Case 3: only canonical exists → check + report incomplete pair ---
	if canonExists && !mdExists {
		existingCanon, cErr := readF2SidecarOrReject(canonPath)
		if cErr != nil {
			return F2PairRefused, cErr
		}
		existingFP, fpErr := f2CanonicalContentFingerprint(existingCanon.CanonicalEnvelope)
		if fpErr != nil {
			return F2PairNotAttempted, fmt.Errorf("f2 pair: cannot fingerprint existing canonical at %q: %w", canonPath, fpErr)
		}
		if !bytes.Equal(existingFP, newFP) {
			return F2PairRefused, fmt.Errorf(
				"f2 pair: canonical content for cycle %q differs from the existing sidecar at %q (immutability: a changed canonical field requires a new F1 emit + synthesis cycle)",
				cycle, canonPath)
		}
		// Canonical matches but MD is missing → incomplete pair. Do NOT
		// auto-complete — the operator investigates why the MD is absent.
		return F2PairIncompleteCanonicalOnly, fmt.Errorf(
			"f2 pair: incomplete pair for cycle %q — canonical sidecar exists at %q but MD projection is missing at %q (investigate before completing; F2 does not auto-complete an incomplete pair)",
			cycle, canonPath, mdPath)
	}

	// --- Case 4: only MD exists → check + report incomplete pair ---
	// (canonExists == false, mdExists == true)
	storedDigest, dErr := extractSourceDigestFromMD(mdPath)
	if dErr != nil {
		return F2PairRefused, fmt.Errorf("f2 pair: cannot extract digest from existing MD at %q (refusing to overwrite): %w", mdPath, dErr)
	}
	if storedDigest != ingest.SemanticDigest {
		return F2PairRefused, fmt.Errorf(
			"f2 pair: MD projection for cycle %q carries digest %q but ingest digest is %q (immutability: content differs — a new F1 emit + synthesis cycle is required)",
			cycle, storedDigest, ingest.SemanticDigest)
	}
	// MD matches but canonical is missing → incomplete pair. Do NOT
	// auto-complete.
	return F2PairIncompleteMDOnly, fmt.Errorf(
		"f2 pair: incomplete pair for cycle %q — MD projection exists at %q but canonical sidecar is missing at %q (investigate before completing; F2 does not auto-complete an incomplete pair)",
		cycle, mdPath, canonPath)
}

// --- Pair helpers ----------------------------------------------------------

// f2FileExists reports whether a path exists (file or otherwise). Used by the
// pair coordinator to assess the on-disk pair state.
func f2FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// f2AtomicWrite creates a file exclusively (O_EXCL) and writes the given bytes.
// If the file already exists, it returns an error (the caller decides what to
// do — typically a collision check). This is the same atomic-create pattern as
// Slice 2's PersistF2CanonicalSidecar.
func f2AtomicWrite(path string, data []byte) error {
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, wErr := fd.Write(data); wErr != nil {
		fd.Close()
		return wErr
	}
	if cErr := fd.Close(); cErr != nil {
		return cErr
	}
	return nil
}

// readF2SidecarOrReject reads and parses a canonical sidecar. If the file is
// unreadable or not valid JSON, it returns a refusal error (the caller reports
// F2PairRefused — immutability forbids overwriting a corrupt artifact).
func readF2SidecarOrReject(path string) (*F2CanonicalSidecar, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("f2 pair: cannot read existing canonical sidecar at %q (refusing to overwrite an unreadable artifact): %w", path, err)
	}
	var sidecar F2CanonicalSidecar
	if jErr := json.Unmarshal(raw, &sidecar); jErr != nil {
		return nil, fmt.Errorf("f2 pair: existing canonical sidecar at %q is not valid JSON (refusing to overwrite a corrupt artifact — investigate or use a new cycle): %w", path, jErr)
	}
	return &sidecar, nil
}

// extractSourceDigestFromMD reads an MD projection file, parses its
// f2-view-metadata fenced block, and returns the source_semantic_digest value.
// This is the MD collision key: if it matches the ingest's digest, the MD was
// rendered from the same canonical content. Returns an error if the file is
// unreadable, the metadata block is absent, or the digest field is empty.
func extractSourceDigestFromMD(mdPath string) (string, error) {
	raw, err := os.ReadFile(mdPath)
	if err != nil {
		return "", fmt.Errorf("cannot read MD projection at %q: %w", mdPath, err)
	}
	matches := f2ViewMetadataRe.FindStringSubmatch(string(raw))
	if len(matches) < 2 {
		return "", fmt.Errorf("no f2-view-metadata fenced block found in MD at %q", mdPath)
	}
	var meta F2ArtifactViewMeta
	if jErr := json.Unmarshal([]byte(matches[1]), &meta); jErr != nil {
		return "", fmt.Errorf("cannot parse f2-view-metadata JSON in MD at %q: %w", mdPath, jErr)
	}
	if meta.SourceSemanticDigest == "" {
		return "", fmt.Errorf("f2-view-metadata in MD at %q carries no source_semantic_digest", mdPath)
	}
	return meta.SourceSemanticDigest, nil
}

// ExtractF2ViewMetadataFromMDBytes parses an MD projection's raw bytes and
// returns the F2ArtifactViewMeta from its fenced metadata block. Exported so
// the doctor (Slice 9) can inspect a projection's metadata without re-rendering.
// Returns an error if the metadata block is absent or unparseable.
func ExtractF2ViewMetadataFromMDBytes(mdBytes []byte) (*F2ArtifactViewMeta, error) {
	matches := f2ViewMetadataRe.FindStringSubmatch(string(mdBytes))
	if len(matches) < 2 {
		return nil, fmt.Errorf("no f2-view-metadata fenced block found")
	}
	var meta F2ArtifactViewMeta
	if err := json.Unmarshal([]byte(matches[1]), &meta); err != nil {
		return nil, fmt.Errorf("cannot parse f2-view-metadata JSON: %w", err)
	}
	return &meta, nil
}
