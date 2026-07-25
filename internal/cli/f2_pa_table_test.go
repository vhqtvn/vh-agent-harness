package cli

// f2_pa_table_test.go — tests for the P-a decision-request table (Slice 5).
//
// The P-a table is a structured per-option decision matrix rendered from
// canonical R3 option records + P-a probes. It is the second salience layer
// (after the P-c headline). Design authority: researches/decisions/2026-07-25-
// f2-rendering-family-mechanism.md, P-a-derived table contract L289-301 +
// C1 build gate resolution.
//
// Key contract points tested here:
//   - golden rendering for all 4 P-a result enums (found / not_found_in_checked_scope
//     / unavailable / not_run), each preserved EXACTLY;
//   - bounded absence stays bounded (never fabricated, never global "none exists");
//   - costs/reversal cost from canonical R3 option fields (not invented);
//   - missing canonical source → incomplete-surface diagnostic (not a fabricated table);
//   - structural markers present;
//   - deterministic rerun;
//   - all values trace to canonical entries;
//   - unresolved probe ref → incomplete-surface diagnostic.

import (
	"strings"
	"testing"
	"time"
)

// --- Helpers for local P-a table fixtures -----------------------------------

// paTableFixture builds a local F1SynthesisEnvelope with R3 options + P-a probes
// covering all 4 result enums. This does NOT go through EmitF1 — it constructs
// the envelope directly for focused P-a table testing. The F1 validator is not
// re-run here (these tests exercise the RENDERER, not the ingest gate; the
// ingest gate's tests are in f2_ingest_test.go).
func paTableFixture() *F1SynthesisEnvelope {
	return &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "pa-test-cycle",
		Applicability:    "required",
		Entries: []F1FamilyEntry{
			{
				Family:     F1FamilyR3RedesignFork,
				Triggered:  "true",
				EntryID:    "entry-r3",
				SourceRefs: []string{"src-r3"},
				R3: &F1R3ForkSummary{
					TriggerRecognized: true,
					Disposition:       "pending",
					Options: []F1R3Option{
						{
							OptionID:                 "opt-a",
							Mode:                     "continue_repair",
							Mechanism:                "patch the immediate defect",
							Costs:                    []string{"eng-hours"},
							Risks:                    []string{"recurrence"},
							ReversalCost:             "low",
							CheapestValidation:       "re-run suite",
							CounterEvidenceProbeRefs: []string{"PA-FOUND", "PA-NF"},
						},
						{
							OptionID:                 "opt-b",
							Mode:                     "redesign",
							Mechanism:                "restructure the abstraction",
							Costs:                    []string{"eng-weeks"},
							Risks:                    []string{"schedule-slip"},
							ReversalCost:             "high",
							CheapestValidation:       "design-review + suite",
							CounterEvidenceProbeRefs: []string{"PA-UNAVAIL", "PA-NOTRUN"},
						},
					},
				},
			},
			{
				Family:     F1FamilyPACounterEvidence,
				Triggered:  "true",
				EntryID:    "entry-pa",
				SourceRefs: []string{"src-pa"},
				PA: &F1PAProbeSummary{
					Probes: []F1PAProbe{
						{
							ProbeID:      "PA-FOUND",
							TargetRef:    "opt-a",
							Result:       F1PAResultFound,
							EvidenceRefs: []string{"pa-ref-found"},
							WeakestClaim: "the found evidence is narrow",
						},
						{
							ProbeID:      "PA-NF",
							TargetRef:    "opt-a",
							Result:       F1PAResultNotFoundInCheckedScope,
							Method:       "repo grep",
							CheckedScope: []string{"internal/cli/f1_r3.go"},
							WeakestClaim: "scope was too narrow to rule out",
						},
						{
							ProbeID:    "PA-UNAVAIL",
							TargetRef:  "opt-b",
							Result:     F1PAResultUnavailable,
							Limitation: "no design archive indexed",
						},
						{
							ProbeID:   "PA-NOTRUN",
							TargetRef: "opt-b",
							Result:    F1PAResultNotRun,
						},
					},
				},
			},
		},
	}
}

// paTableFixtureNoR3 builds a local envelope with NO R3 entry — to test the
// missing-canonical-source → incomplete-surface diagnostic.
func paTableFixtureNoR3() *F1SynthesisEnvelope {
	return &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "no-r3-cycle",
		Applicability:    "required",
		Entries: []F1FamilyEntry{
			{
				Family:    F1FamilyR1CrossLaneJoin,
				Triggered: "true",
				EntryID:   "entry-r1",
				R1: &F1R1JoinSummary{
					Conclusions: []F1R1Conclusion{
						{ConclusionID: "R1C1", PropertyID: "R1P1", JoinDisposition: "union"},
					},
				},
			},
		},
	}
}

// paTableFixtureBoundedAbsence builds a local envelope with an R3 option that
// has NO costs, NO reversal cost, and NO bound probes — to test bounded-
// absence markers.
func paTableFixtureBoundedAbsence() *F1SynthesisEnvelope {
	return &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "absence-cycle",
		Applicability:    "required",
		Entries: []F1FamilyEntry{
			{
				Family:    F1FamilyR3RedesignFork,
				Triggered: "true",
				EntryID:   "entry-r3",
				R3: &F1R3ForkSummary{
					TriggerRecognized: true,
					Disposition:       "pending",
					Options: []F1R3Option{
						{
							OptionID:  "opt-bare",
							Mode:      "continue_repair",
							Mechanism: "no fields populated",
							// Costs, Risks, ReversalCost, CheapestValidation all zero-value.
							// CounterEvidenceProbeRefs empty.
						},
					},
				},
			},
		},
	}
}

// paTableFixtureUnresolvedRef builds a local envelope where an R3 option
// references a probe ID that does NOT exist in the P-a entry — to test the
// unresolved-ref incomplete-surface diagnostic.
func paTableFixtureUnresolvedRef() *F1SynthesisEnvelope {
	return &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "unresolved-cycle",
		Applicability:    "required",
		Entries: []F1FamilyEntry{
			{
				Family:    F1FamilyR3RedesignFork,
				Triggered: "true",
				EntryID:   "entry-r3",
				R3: &F1R3ForkSummary{
					TriggerRecognized: true,
					Disposition:       "pending",
					Options: []F1R3Option{
						{
							OptionID:                 "opt-x",
							Mode:                     "redesign",
							Mechanism:                "test unresolved ref",
							Costs:                    []string{"c1"},
							ReversalCost:             "medium",
							CounterEvidenceProbeRefs: []string{"NO-SUCH-PROBE"},
						},
					},
				},
			},
			{
				Family:    F1FamilyPACounterEvidence,
				Triggered: "true",
				EntryID:   "entry-pa",
				PA: &F1PAProbeSummary{
					Probes: []F1PAProbe{
						{ProbeID: "PA-OTHER", TargetRef: "opt-x", Result: F1PAResultFound, EvidenceRefs: []string{"r1"}},
					},
				},
			},
		},
	}
}

// renderPATableString renders the P-a table section from an env into a string
// for test inspection.
func renderPATableString(env *F1SynthesisEnvelope) string {
	var b strings.Builder
	renderF2PATable(&b, env)
	return b.String()
}

// renderFullMDFromEnv renders the full MD projection from an env (wrapping it
// in a sidecar) for structural-position tests.
func renderFullMDFromEnv(t *testing.T, env *F1SynthesisEnvelope) string {
	t.Helper()
	sidecar := &F2CanonicalSidecar{
		Kind:              F2CanonicalSidecarKind,
		CanonicalEnvelope: env,
		F2ViewMetadata: F2ArtifactViewMeta{
			SynthesisCycleID:               env.SynthesisCycleID,
			CanonicalRepresentationVersion: F2CanonicalRepresentationVersion,
			SchemaVersion:                  env.SchemaVersion,
			ProjectionVersion:              F2ProjectionVersion,
			RendererVersion:                F2RendererVersion,
			WriteTimestamp:                 fixedTime.Format(time.RFC3339),
		},
	}
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	return string(md)
}

// --- Golden: all 4 result enums preserved EXACTLY ---------------------------

// TestF2PATable_AllFourResultEnumsPreservedExactly proves that all 4 P-a result
// enums (found / not_found_in_checked_scope / unavailable / not_run) render
// with their EXACT string value — never collapsed, paraphrased, or reinterpreted
// (memo L295-301).
func TestF2PATable_AllFourResultEnumsPreservedExactly(t *testing.T) {
	body := renderPATableString(paTableFixture())

	requiredEnums := []string{
		"`" + F1PAResultFound + "`",
		"`" + F1PAResultNotFoundInCheckedScope + "`",
		"`" + F1PAResultUnavailable + "`",
		"`" + F1PAResultNotRun + "`",
	}
	for _, want := range requiredEnums {
		if !strings.Contains(body, want) {
			t.Errorf("P-a table must contain exact result enum %s — not found in:\n%s", want, body)
		}
	}

	// The bounded-absence qualifier for not_run ("not performed") must appear
	// alongside the not_run enum (memo L300: "not_run stays visibly unperformed").
	if !strings.Contains(body, "(not performed)") {
		t.Errorf("not_run must render as visibly unperformed — \"(not performed)\" not found in:\n%s", body)
	}

	// unavailable must stay distinct from a negative result — it must carry
	// its limitation, not be treated as "none exists" (memo L299).
	if !strings.Contains(body, "no design archive indexed") {
		t.Errorf("unavailable probe must carry its limitation — not found in:\n%s", body)
	}
}

// --- Bounded absence stays bounded ------------------------------------------

// TestF2PATable_BoundedAbsenceNeverFabricated proves that an R3 option with no
// costs, no reversal cost, and no bound probes renders bounded-absence markers
// — NEVER fabricated values and NEVER a global "none exists" claim.
func TestF2PATable_BoundedAbsenceNeverFabricated(t *testing.T) {
	body := renderPATableString(paTableFixtureBoundedAbsence())

	// Costs cell: bounded absence.
	if !strings.Contains(body, "(no costs declared in canonical R3 option)") {
		t.Errorf("expected bounded-absence marker for empty costs — not found in:\n%s", body)
	}

	// Reversal cost cell: bounded absence.
	if !strings.Contains(body, "(no reversal cost declared in canonical R3 option)") {
		t.Errorf("expected bounded-absence marker for empty reversal cost — not found in:\n%s", body)
	}

	// Evidence against cell: bounded absence (no probe bound).
	if !strings.Contains(body, "(no counter-evidence probe bound to this option)") {
		t.Errorf("expected bounded-absence marker for no bound probes — not found in:\n%s", body)
	}

	// Weakest claim cell: bounded absence (no probe bound).
	if !strings.Contains(body, "(no weakest claim — no probe bound to this option)") {
		t.Errorf("expected bounded-absence marker for weakest claim with no bound probes — not found in:\n%s", body)
	}

	// The string "none exists" must NEVER appear — bounded absence is NOT
	// global absence (memo L298).
	if strings.Contains(body, "none exists") {
		t.Errorf("bounded absence must NEVER render as \"none exists\" — found in:\n%s", body)
	}
}

// --- not_found_in_checked_scope NEVER renders as "none exists" ---------------

// TestF2PATable_NotFoundInCheckedScopeNeverNoneExists proves that the
// not_found_in_checked_scope enum NEVER renders as "none exists" (memo L298).
// This is the load-bearing bounded-absence guarantee: the scope that was
// checked is shown, and the result is NOT reinterpreted as global absence.
func TestF2PATable_NotFoundInCheckedScopeNeverNoneExists(t *testing.T) {
	body := renderPATableString(paTableFixture())

	if strings.Contains(body, "none exists") {
		t.Errorf("not_found_in_checked_scope must NEVER render as \"none exists\" — found in:\n%s", body)
	}

	// The checked scope must appear alongside the enum.
	if !strings.Contains(body, "checked scope:") {
		t.Errorf("not_found_in_checked_scope must show the checked scope — not found in:\n%s", body)
	}
}

// --- Costs and reversal from canonical R3 fields ----------------------------

// TestF2PATable_CostsAndReversalFromCanonicalR3 proves the Costs and Reversal-
// cost cells come from the canonical R3 option fields (Costs / ReversalCost),
// not invented by the renderer.
func TestF2PATable_CostsAndReversalFromCanonicalR3(t *testing.T) {
	body := renderPATableString(paTableFixture())

	// opt-a: Costs = ["eng-hours"], ReversalCost = "low".
	if !strings.Contains(body, "`eng-hours`") {
		t.Errorf("expected canonical cost \"eng-hours\" in the costs cell — not found in:\n%s", body)
	}
	if !strings.Contains(body, "`low`") {
		t.Errorf("expected canonical reversal cost \"low\" in the reversal cell — not found in:\n%s", body)
	}

	// opt-b: Costs = ["eng-weeks"], ReversalCost = "high".
	if !strings.Contains(body, "`eng-weeks`") {
		t.Errorf("expected canonical cost \"eng-weeks\" in the costs cell — not found in:\n%s", body)
	}
	if !strings.Contains(body, "`high`") {
		t.Errorf("expected canonical reversal cost \"high\" in the reversal cell — not found in:\n%s", body)
	}
}

// --- Missing canonical source → incomplete-surface diagnostic ---------------

// TestF2PATable_MissingCanonicalSourceNotFabricated proves that when no R3
// entry exists, the section renders an incomplete-surface diagnostic — NOT a
// fabricated table with invented rows (memo L335).
func TestF2PATable_MissingCanonicalSourceNotFabricated(t *testing.T) {
	body := renderPATableString(paTableFixtureNoR3())

	// The diagnostic marker must appear.
	if !strings.Contains(body, "decision-request table not applicable") {
		t.Errorf("expected incomplete-surface diagnostic for missing R3 — not found in:\n%s", body)
	}

	// A fabricated table row must NOT appear (no Option column values).
	if strings.Contains(body, "| `opt-") {
		t.Errorf("missing canonical source must NOT produce fabricated table rows — found in:\n%s", body)
	}

	// The table header row must NOT appear (no fabricated table).
	if strings.Contains(body, "| Option | Costs |") {
		t.Errorf("missing canonical source must NOT produce a fabricated table header — found in:\n%s", body)
	}
}

// --- Structural markers present ---------------------------------------------

// TestF2PATable_StructuralMarkersPresent proves the begin/end structural
// markers are present so the doctor (Slice 9) can locate the section.
func TestF2PATable_StructuralMarkersPresent(t *testing.T) {
	body := renderPATableString(paTableFixture())

	if !strings.Contains(body, "<!-- f2-pa-table:begin -->") {
		t.Errorf("begin marker not found in:\n%s", body)
	}
	if !strings.Contains(body, "<!-- f2-pa-table:end -->") {
		t.Errorf("end marker not found in:\n%s", body)
	}
}

// --- Deterministic rerun ----------------------------------------------------

// TestF2PATable_DeterministicRerun proves the P-a table renders identically on
// repeated calls with the same input (byte-stable — required for collision
// detection and idempotency).
func TestF2PATable_DeterministicRerun(t *testing.T) {
	env := paTableFixture()
	first := renderPATableString(env)
	second := renderPATableString(env)
	if first != second {
		t.Errorf("P-a table is not deterministic — first and second renders differ:\n--- FIRST ---\n%s\n--- SECOND ---\n%s", first, second)
	}
}

// --- Values trace to canonical entries --------------------------------------

// TestF2PATable_ValuesTraceToCanonicalEntries proves every value in the P-a
// table is a verbatim projection of a canonical field — no model-authored
// summary, no invented content (memo L261-264, L294).
func TestF2PATable_ValuesTraceToCanonicalEntries(t *testing.T) {
	env := paTableFixture()
	body := renderPATableString(env)

	// Every option ID + mode must appear (from R3 option records).
	for _, opt := range env.Entries[0].R3.Options {
		needle := "`" + opt.OptionID + "`"
		if !strings.Contains(body, needle) {
			t.Errorf("canonical option ID %s must appear in the table — not found", needle)
		}
		needleMode := "(" + opt.Mode + ")"
		if !strings.Contains(body, needleMode) {
			t.Errorf("canonical option mode %s must appear in the table — not found", needleMode)
		}
	}

	// Every probe ID referenced by CounterEvidenceProbeRefs must appear.
	for _, opt := range env.Entries[0].R3.Options {
		for _, ref := range opt.CounterEvidenceProbeRefs {
			needle := "`" + ref + "`"
			if !strings.Contains(body, needle) {
				t.Errorf("canonical probe ref %s must appear in the table — not found", needle)
			}
		}
	}

	// Weakest claims from canonical probes must appear verbatim.
	for _, p := range env.Entries[1].PA.Probes {
		if p.WeakestClaim != "" {
			if !strings.Contains(body, p.WeakestClaim) {
				t.Errorf("canonical weakest claim %q must appear verbatim — not found", p.WeakestClaim)
			}
		}
	}
}

// --- Unresolved probe ref → incomplete-surface diagnostic -------------------

// TestF2PATable_UnresolvedProbeRefDiagnosed proves that when an R3 option
// references a probe ID that does NOT exist in the P-a entry, the evidence
// cell renders an incomplete-surface diagnostic — NOT a fabricated cell.
func TestF2PATable_UnresolvedProbeRefDiagnosed(t *testing.T) {
	body := renderPATableString(paTableFixtureUnresolvedRef())

	if !strings.Contains(body, "referenced but not found in canonical P-a entry") {
		t.Errorf("expected incomplete-surface diagnostic for unresolved probe ref — not found in:\n%s", body)
	}
}

// --- Position: after P-c headline, before detailed envelope -----------------

// TestF2PATable_PositionAfterPCHeadlineBeforeDetails proves the P-a table
// appears after the P-c headline section and before the detailed canonical
// envelope projection — a second salience layer.
func TestF2PATable_PositionAfterPCHeadlineBeforeDetails(t *testing.T) {
	body := renderFullMDFromEnv(t, paTableFixture())

	pcIdx := strings.Index(body, "<!-- f2-pc-headline:begin -->")
	paIdx := strings.Index(body, "<!-- f2-pa-table:begin -->")
	detailsIdx := strings.Index(body, "## Canonical Envelope (projected)")

	if pcIdx < 0 {
		t.Fatalf("P-c headline section not found")
	}
	if paIdx < 0 {
		t.Fatalf("P-a table section not found")
	}
	if detailsIdx < 0 {
		t.Fatalf("detailed envelope section not found")
	}

	if paIdx < pcIdx {
		t.Errorf("P-a table must appear AFTER P-c headline (paIdx=%d < pcIdx=%d)", paIdx, pcIdx)
	}
	if detailsIdx < paIdx {
		t.Errorf("P-a table must appear BEFORE detailed envelope (detailsIdx=%d < paIdx=%d)", detailsIdx, paIdx)
	}
}

// --- Shared fixture integration ---------------------------------------------

// TestF2PATable_SharedFixtureRenders proves the shared canonical fixture
// (canonicalF1Fixture) renders a valid P-a table through the full pipeline
// (ingest → sidecar → render). The shared fixture has opt-continue (bound to
// PA-P2: not_found_in_checked_scope) and opt-redesign (bound to PA-P3:
// unavailable).
func TestF2PATable_SharedFixtureRenders(t *testing.T) {
	ingest := f2IngestFromFixture(t)
	sidecar := buildF2CanonicalSidecar(ingest, "docs/checkpoints/f2", fixedTime)
	md, err := RenderF2MarkdownProjection(sidecar, "docs/checkpoints/f2")
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	body := string(md)

	// Both option IDs must appear.
	if !strings.Contains(body, "`opt-continue`") {
		t.Errorf("opt-continue must appear in P-a table — not found")
	}
	if !strings.Contains(body, "`opt-redesign`") {
		t.Errorf("opt-redesign must appear in P-a table — not found")
	}

	// PA-P2 (not_found_in_checked_scope) must render with its exact enum.
	if !strings.Contains(body, "`not_found_in_checked_scope`") {
		t.Errorf("PA-P2 result enum not_found_in_checked_scope must appear — not found")
	}

	// PA-P3 (unavailable) must render with its exact enum.
	if !strings.Contains(body, "`unavailable`") {
		t.Errorf("PA-P3 result enum unavailable must appear — not found")
	}

	// Canonical costs must appear.
	if !strings.Contains(body, "`eng-hours`") {
		t.Errorf("canonical cost eng-hours must appear — not found")
	}
	if !strings.Contains(body, "`eng-weeks`") {
		t.Errorf("canonical cost eng-weeks must appear — not found")
	}

	// Canonical reversal costs must appear.
	if !strings.Contains(body, "`low`") {
		t.Errorf("canonical reversal cost low must appear — not found")
	}
	if !strings.Contains(body, "`high`") {
		t.Errorf("canonical reversal cost high must appear — not found")
	}
}
