package cli

// f2_pc_test.go — tests for the P-c headline-layer salience (Slice 4).
//
// The P-c section is the FIRST DISCLOSURE LAYER of the F2 MD projection. It
// surfaces decision-relevant salience (decision frame, disposition, counter-
// evidence, weakest claim, unresolved gaps, binding metadata) BEFORE the
// detailed envelope sections. Its purpose (memo L237-238): prevent a clean
// headline from contradicting, burying, or omitting an inconclusive, failed,
// or materially-qualified result.
//
// Design authority: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), P-c contract L235-264.
//
// Key contract points tested here:
//   - all 6 required sub-sections present IN ORDER;
//   - P-c section appears before the detailed envelope sections;
//   - displayed disposition == canonical disposition (never upgraded);
//   - counter-evidence and weakest claim in the FIRST layer;
//   - P-a result enum preserved EXACTLY (never paraphrased);
//   - all values trace to canonical entry IDs (no model-authored summary).

import (
	"strings"
	"testing"
)

// --- Structural completeness ------------------------------------------------

// TestF2PCHeadline_AllSixRequiredSectionsInOrder proves the P-c headline
// section contains all 6 required sub-sections (memo L243-248) in the
// prescribed order: Decision Frame → Current Disposition → Counter-evidence →
// Weakest Claim → Unresolved Gaps → Canonical Binding Metadata.
func TestF2PCHeadline_AllSixRequiredSectionsInOrder(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// Extract the P-c section between the structural markers.
	begin := strings.Index(body, "<!-- f2-pc-headline:begin -->")
	end := strings.Index(body, "<!-- f2-pc-headline:end -->")
	if begin < 0 {
		t.Fatalf("P-c headline begin marker not found")
	}
	if end < 0 {
		t.Fatalf("P-c headline end marker not found")
	}
	if end < begin {
		t.Fatalf("P-c headline end marker appears before begin marker")
	}
	pcSection := body[begin:end]

	// All 6 sub-sections must be present IN ORDER.
	required := []string{
		"### Decision Frame",
		"### Current Disposition",
		"### Counter-evidence",
		"### Weakest Claim",
		"### Unresolved Gaps",
		"### Canonical Binding Metadata",
	}
	for i, want := range required {
		idx := strings.Index(pcSection, want)
		if idx < 0 {
			t.Errorf("P-c sub-section %d (%q) missing from headline layer", i+1, want)
			continue
		}
		if i > 0 {
			prevIdx := strings.Index(pcSection, required[i-1])
			if idx < prevIdx {
				t.Errorf("P-c sub-section %q appears before %q (order violation — memo L243-248 requires this order)", want, required[i-1])
			}
		}
	}
}

// TestF2PCHeadline_SectionPrecedesDetailedEntries proves the P-c headline
// appears BEFORE the detailed "Canonical Envelope (projected)" section. This
// is the "first disclosure layer" contract (memo L240-241): counter-evidence
// and weakest claim must surface before the details, not be buried in them.
func TestF2PCHeadline_SectionPrecedesDetailedEntries(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	pcIdx := strings.Index(body, "<!-- f2-pc-headline:begin -->")
	detailIdx := strings.Index(body, "## Canonical Envelope (projected)")
	if pcIdx < 0 || detailIdx < 0 {
		t.Fatalf("missing P-c section or detailed section")
	}
	if pcIdx > detailIdx {
		t.Fatalf("P-c headline appears AFTER the detailed envelope section — must be the first disclosure layer (memo L240-241)")
	}
}

// --- Displayed disposition == canonical disposition -------------------------

// TestF2PCHeadline_DispositionMatchesCanonical proves the P-c headline's
// displayed disposition exactly matches the canonical R3 disposition (memo
// L255). The canonical fixture carries disposition=pending; the headline must
// say pending — never "approved," "resolved," or any renderer-authored value.
func TestF2PCHeadline_DispositionMatchesCanonical(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// The canonical fixture has R3 disposition = pending.
	if !strings.Contains(body, "**R3 disposition:** `pending`") {
		t.Errorf("P-c headline does not display the canonical R3 disposition 'pending' verbatim (memo L255: displayed disposition == canonical)")
	}
	// The R3 disposition line must NOT be upgraded to a cleaner value.
	// (Note: the envelope's Validation disposition is legitimately 'complete'
	// — that is a different field from the R3 fork disposition.)
	r3DispLine := "**R3 disposition:** `"
	for _, forbidden := range []string{"approved", "resolved"} {
		if strings.Contains(body, r3DispLine+forbidden+"`") {
			t.Errorf("P-c headline upgraded R3 disposition to %q — displayed disposition must match canonical, not be renderer-authored", forbidden)
		}
	}
}

// TestF2PCHeadline_PendingDispositionNotCleanHeadline proves the INCONCLUSIVE
// contract: a pending R3 disposition must NOT be replaced by a clean
// headline. The operator must see "pending" — not a summary that implies the
// decision is settled.
func TestF2PCHeadline_PendingDispositionNotCleanHeadline(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	// The canonical fixture has R3 Disposition = F1R3DispositionPending.
	// Verify the envelope actually carries pending.
	env := ingest.CanonicalEnvelope
	var r3 *F1R3ForkSummary
	for _, e := range env.Entries {
		if e.Family == F1FamilyR3RedesignFork && e.R3 != nil {
			r3 = e.R3
		}
	}
	if r3 == nil {
		t.Fatal("fixture has no R3 entry")
	}
	if r3.Disposition != F1R3DispositionPending {
		t.Fatalf("fixture R3 disposition = %q, expected pending for this test", r3.Disposition)
	}

	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// The headline MUST surface "pending" — no clean substitute.
	if !strings.Contains(body, "`pending`") {
		t.Errorf("headline does not surface the pending disposition — an inconclusive result was buried (memo L237-238)")
	}
}

// --- Counter-evidence + weakest claim in the first layer -------------------

// TestF2PCHeadline_CounterEvidenceAndWeakestClaimPresent proves both
// counter-evidence AND weakest claim appear in the P-c first layer (memo
// L250-251: "Counter-evidence and weakest claim MUST be in the first
// disclosure layer, not merely linked or appendixed").
func TestF2PCHeadline_CounterEvidenceAndWeakestClaimPresent(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// Extract the P-c section.
	begin := strings.Index(body, "<!-- f2-pc-headline:begin -->")
	end := strings.Index(body, "<!-- f2-pc-headline:end -->")
	if begin < 0 || end < 0 {
		t.Fatalf("P-c section markers not found")
	}
	pcSection := body[begin:end]

	// Counter-evidence sub-section must contain the P-a probes.
	if !strings.Contains(pcSection, "Probe `PA-P1`") {
		t.Errorf("counter-evidence section missing probe PA-P1")
	}
	if !strings.Contains(pcSection, "Probe `PA-P2`") {
		t.Errorf("counter-evidence section missing probe PA-P2")
	}
	if !strings.Contains(pcSection, "Probe `PA-P3`") {
		t.Errorf("counter-evidence section missing probe PA-P3")
	}

	// Weakest claim sub-section must contain the fixture's weakest claim.
	// The canonical fixture has PA-P1.WeakestClaim = "R1C1 rests on a single
	// ancestry-bearing source".
	if !strings.Contains(pcSection, "R1C1 rests on a single ancestry-bearing source") {
		t.Errorf("weakest claim section missing PA-P1's weakest claim from canonical fixture")
	}
}

// --- P-a result enum preserved EXACTLY -------------------------------------

// TestF2PCHeadline_ProbeResultEnumPreservedExactly proves the P-c headline
// renders the P-a result enum EXACTLY (memo L295-301): found,
// not_found_in_checked_scope, unavailable are never paraphrased or collapsed.
// not_found_in_checked_scope NEVER renders as "none exists."
func TestF2PCHeadline_ProbeResultEnumPreservedExactly(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// All three result enums from the fixture must appear EXACTLY.
	checks := []string{
		"`found`",
		"`not_found_in_checked_scope`",
		"`unavailable`",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("P-a result enum %s not rendered exactly in P-c headline (memo L295-301: result enum preserved EXACTLY)", want)
		}
	}

	// The bounded-absence enum must NEVER be paraphrased as global absence.
	if strings.Contains(strings.ToLower(body), "none exists") {
		t.Errorf("'none exists' found — not_found_in_checked_scope must NEVER render as 'none exists' (memo L298)")
	}
}

// --- No model-authored summary (all values trace to canonical entries) -----

// TestF2PCHeadline_AllValuesTraceToCanonical proves every value in the P-c
// headline section is a verbatim projection of a canonical envelope field.
// The renderer walks the envelope struct and emits values via fmt.Fprintf —
// no model call, no narrative generation. This test verifies the structural
// guarantee by checking that specific envelope values appear VERBATIM.
func TestF2PCHeadline_AllValuesTraceToCanonical(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// Extract the P-c section.
	begin := strings.Index(body, "<!-- f2-pc-headline:begin -->")
	end := strings.Index(body, "<!-- f2-pc-headline:end -->")
	if begin < 0 || end < 0 {
		t.Fatalf("P-c section markers not found")
	}
	pcSection := body[begin:end]

	env := ingest.CanonicalEnvelope

	// R3 option IDs and modes must appear verbatim.
	for _, entry := range env.Entries {
		if entry.R3 != nil {
			for _, opt := range entry.R3.Options {
				if !strings.Contains(pcSection, opt.OptionID) {
					t.Errorf("R3 option ID %q from canonical envelope not found in P-c headline", opt.OptionID)
				}
			}
		}
	}

	// P-a probe IDs and results must appear verbatim.
	for _, entry := range env.Entries {
		if entry.PA != nil {
			for _, p := range entry.PA.Probes {
				if !strings.Contains(pcSection, p.ProbeID) {
					t.Errorf("P-a probe ID %q from canonical envelope not found in P-c headline", p.ProbeID)
				}
				if !strings.Contains(pcSection, p.Result) {
					t.Errorf("P-a probe %q result %q not found verbatim in P-c headline", p.ProbeID, p.Result)
				}
			}
		}
	}

	// Cycle ID must appear in the binding metadata.
	if !strings.Contains(pcSection, env.SynthesisCycleID) {
		t.Errorf("cycle ID %q not found in P-c canonical binding metadata", env.SynthesisCycleID)
	}
}

// --- P-c markers present for structural parsing ----------------------------

// TestF2PCHeadline_StructuralMarkersPresent proves the P-c section carries
// the fenced HTML comment markers (f2-pc-headline:begin / :end) so the doctor
// (Slice 9) can reliably locate the section and verify its sub-structure.
func TestF2PCHeadline_StructuralMarkersPresent(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)
	if !strings.Contains(body, "<!-- f2-pc-headline:begin -->") {
		t.Errorf("P-c begin marker missing")
	}
	if !strings.Contains(body, "<!-- f2-pc-headline:end -->") {
		t.Errorf("P-c end marker missing")
	}
}

// --- Byte-stable rerun (P-c section deterministic) -------------------------

// TestF2PCHeadline_DeterministicRerun proves the P-c section is deterministic:
// two renders of the same sidecar produce identical bytes (including the P-c
// section). This is required for the byte-level collision check and the
// doctor's projection-equivalence check.
func TestF2PCHeadline_DeterministicRerun(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	first, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}
	second, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("P-c deterministic rerun violated: two renders produced different bytes")
	}
}

// --- Absence handling: weakest claim and gaps absent fields present --------

// TestF2PCHeadline_AbsenceFieldsPresentNotOmitted proves the P-c section
// always renders all 6 sub-sections even when some canonical fields are
// absent. The fixture has no R1 Gaps, so the "Unresolved Gaps" sub-section
// must say "(none declared in canonical R1 conclusions)" — NOT be omitted.
// A missing sub-section would be a structural violation (memo L257-258).
func TestF2PCHeadline_AbsenceFieldsPresentNotOmitted(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// The canonical fixture has NO R1 gaps. The sub-section must still exist
	// and carry the bounded-absence notice.
	if !strings.Contains(body, "### Unresolved Gaps") {
		t.Errorf("Unresolved Gaps sub-section missing — must always be present even when no gaps declared")
	}
	// The bounded-absence notice must not claim global absence.
	if strings.Contains(strings.ToLower(body), "none exists") {
		t.Errorf("'none exists' language found — bounded absence must not become global absence")
	}
}
