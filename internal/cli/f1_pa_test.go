package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// f1_pa_test.go — Slice 4 (P-a counter-evidence) tests. Covers the 8 required
// cases + the producer↔validator property guard + purity + the standalone
// coverage gate + the deepCopyPA field-preservation regression (the Slice-3
// deepCopyR3/Selection trap, now applied to the extended F1PAProbe DTO).

// --- helpers ---------------------------------------------------------------

// paTargetSummaries builds small R1/R3 summaries with the given conclusion and
// option IDs, so coverage tests can vary the target set without rebuilding the
// full canonical fixture.
func paTargetSummaries(conclusionIDs, optionIDs []string) (*F1R1JoinSummary, *F1R3ForkSummary) {
	r1 := &F1R1JoinSummary{}
	for _, id := range conclusionIDs {
		r1.Conclusions = append(r1.Conclusions, F1R1Conclusion{ConclusionID: id, PropertyID: "P-" + id})
	}
	r3 := &F1R3ForkSummary{TriggerRecognized: true, Disposition: F1R3DispositionPending}
	for _, id := range optionIDs {
		r3.Options = append(r3.Options, F1R3Option{OptionID: id, Mode: F1R3ModeContinueRepair, Mechanism: "m-" + id, SupportRefs: []string{"R1C1"}})
	}
	if len(optionIDs) == 0 {
		r3 = nil
	}
	return r1, r3
}

func paSummaryFromProbes(probes ...F1PAProbe) *F1PAProbeSummary {
	return &F1PAProbeSummary{Probes: probes}
}

// --- the 8 required Slice-4 cases ------------------------------------------

// Case 1: every target requires probe coverage.
func TestPACoverage_EveryTargetRequiresCoverage(t *testing.T) {
	r1, r3 := paTargetSummaries([]string{"R1C1"}, []string{"opt-continue", "opt-redesign"})
	// Only R1C1 and opt-redesign are covered; opt-continue is uncovered.
	pa := paSummaryFromProbes(
		F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound, EvidenceRefs: []string{"ref-1"}},
		F1PAProbe{ProbeID: "PA-P3", TargetRef: "opt-redesign", Result: F1PAResultUnavailable, Limitation: "lim"},
	)
	errs := ValidatePACoverage(r1, r3, pa)
	if !hasErrContaining(errs, `target "opt-continue" has no coverage-satisfying probe`) {
		t.Fatalf("uncovered target opt-continue must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 2: invalid enum fails.
func TestPA_InvalidResultEnumFails(t *testing.T) {
	pa := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: "definitely"})
	if errs := validatePASummary("e", pa); !hasErrContaining(errs, "unknown result") {
		t.Fatalf("unknown result must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 3: found without refs fails.
func TestPA_FoundWithoutRefsFails(t *testing.T) {
	pa := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound})
	if errs := validatePASummary("e", pa); !hasErrContaining(errs, "result=found requires >=1 non-empty evidence_ref") {
		t.Fatalf("found without refs must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
	// Blank/empty refs do not count (fabricated/locator-free evidence invalid).
	paBlank := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound, EvidenceRefs: []string{"  ", ""}})
	if errs := validatePASummary("e", paBlank); !hasErrContaining(errs, "result=found requires >=1 non-empty evidence_ref") {
		t.Fatalf("found with only blank refs must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 4: not_found_in_checked_scope without scope/method fails.
func TestPA_NotFoundWithoutScopeOrMethodFails(t *testing.T) {
	// Missing both.
	pa := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotFoundInCheckedScope})
	errs := validatePASummary("e", pa)
	if !hasErrContaining(errs, "requires a non-empty method") || !hasErrContaining(errs, "requires >=1 non-empty checked_scope") {
		t.Fatalf("not_found without method+scope must fail on both; got:\n  %s", strings.Join(errs, "\n  "))
	}
	// Method present, scope missing.
	paNoScope := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotFoundInCheckedScope, Method: "grep"})
	if errs := validatePASummary("e", paNoScope); !hasErrContaining(errs, "requires >=1 non-empty checked_scope") {
		t.Fatalf("not_found without scope must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
	// Scope present, method missing.
	paNoMethod := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotFoundInCheckedScope, CheckedScope: []string{"src/"}})
	if errs := validatePASummary("e", paNoMethod); !hasErrContaining(errs, "requires a non-empty method") {
		t.Fatalf("not_found without method must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 5: unavailable without limitation fails.
func TestPA_UnavailableWithoutLimitationFails(t *testing.T) {
	pa := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultUnavailable})
	if errs := validatePASummary("e", pa); !hasErrContaining(errs, "result=unavailable requires a non-empty limitation") {
		t.Fatalf("unavailable without limitation must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 6: fabricated evidence can't satisfy.
// Structural interpretation: empty/blank refs do not count as evidence. A
// well-formed-but-fabricated locator passes the structural check (truth is the
// federated verifier's job, memo L276 — NOT a structural gate). This test pins
// the STRUCTURAL baseline honestly: the gate rejects locator-FREE evidence,
// not locator-FORGED evidence.
func TestPA_FabricatedEvidenceCannotSatisfy(t *testing.T) {
	r1, _ := paTargetSummaries([]string{"R1C1"}, nil)
	// found with NO refs (locator-free) — gate rejects.
	paFree := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound})
	if errs := ValidatePACoverage(r1, nil, paFree); !hasErrContaining(errs, "result=found requires >=1 non-empty evidence_ref") {
		t.Fatalf("locator-free found must be rejected by the gate; got:\n  %s", strings.Join(errs, "\n  "))
	}
	// found with a present (well-formed) locator — gate ACCEPTS structurally.
	// This is the honesty caveat: a forged-but-well-formed locator is not
	// detectable structurally. Truth is federated.
	paFormed := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound, EvidenceRefs: []string{"file://evidence/1"}})
	if errs := ValidatePACoverage(r1, nil, paFormed); len(errs) != 0 {
		t.Fatalf("well-formed locator passes structurally (truth is federated); got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 7: no bounded-absence -> global-absence conversion.
// not_found_in_checked_scope is BOUNDED absence: it satisfies coverage (a probe
// was attempted within a declared scope) but is NEVER treated as global
// absence. This test pins both halves: (a) a target covered ONLY by
// not_found_in_checked_scope IS covered (the result is coverage-satisfying),
// and (b) the result REQUIRES a checked_scope (so the absence is explicitly
// bounded, not a global claim).
func TestPA_NoBoundedAbsenceToGlobalAbsenceConversion(t *testing.T) {
	r1, _ := paTargetSummaries([]string{"R1C1"}, nil)
	// (a) A target covered only by not_found_in_checked_scope IS covered.
	pa := paSummaryFromProbes(F1PAProbe{
		ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotFoundInCheckedScope,
		Method: "grep", CheckedScope: []string{"src/cli/"},
	})
	if errs := ValidatePACoverage(r1, nil, pa); len(errs) != 0 {
		t.Fatalf("bounded not_found_in_checked_scope MUST satisfy coverage (never converted to global absence); got:\n  %s", strings.Join(errs, "\n  "))
	}
	// (b) A not_found_in_checked_scope WITHOUT a checked_scope is rejected —
	// the absence must be bounded, never a bare global claim.
	paUnbounded := paSummaryFromProbes(F1PAProbe{
		ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotFoundInCheckedScope, Method: "grep",
	})
	if errs := ValidatePACoverage(r1, nil, paUnbounded); !hasErrContaining(errs, "requires >=1 non-empty checked_scope") {
		t.Fatalf("unbounded not_found must be rejected (no global-absence conversion); got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Case 8: weakest-claim survives serialization.
func TestPA_WeakestClaimSurvivesSerialization(t *testing.T) {
	env := canonicalF1Fixture()
	// The canonical fixture already carries WeakestClaim on PA-P1; verify it
	// survives a canonical-bytes round-trip (the digest projection path).
	b, err := env.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	// Re-hydrate the entries projection and confirm a weakest_claim survives.
	proj := struct {
		Entries []F1FamilyEntry `json:"entries"`
	}{}
	if err := json.Unmarshal(b, &proj); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range proj.Entries {
		if e.PA != nil {
			for _, p := range e.PA.Probes {
				if strings.Contains(strings.ToLower(p.WeakestClaim), "weakest") || p.WeakestClaim != "" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("weakest_claim must survive canonical serialization round-trip")
	}
	// Direct check: set an explicit weakest claim and confirm it round-trips.
	env.Entries[2].PA.Probes[0].WeakestClaim = "EXPLICIT-WEAKEST-CLAIM"
	b2, err := env.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b2), "EXPLICIT-WEAKEST-CLAIM") {
		t.Fatal("explicit weakest_claim must appear in canonical bytes")
	}
}

// --- producer↔validator property guard -------------------------------------

// TestGeneratePAProbes_ProducerOutputPassesValidator is the P-a analog of
// TestJoinR1_ProducerOutputPassesValidator: for well-formed inputs across
// every result enum, the producer output MUST pass validatePASummary. If a
// future change makes the producer emit validator-failing output, this fails.
func TestGeneratePAProbes_ProducerOutputPassesValidator(t *testing.T) {
	cases := []struct {
		name   string
		inputs []PAProbeInput
	}{
		{
			name: "found with refs",
			inputs: []PAProbeInput{{TargetRef: "R1C1", Result: F1PAResultFound,
				EvidenceRefs: []string{"ref-1"}, FalsificationQuestion: "q?", WeakestClaim: "wc"}},
		},
		{
			name: "not_found_in_checked_scope bounded",
			inputs: []PAProbeInput{{TargetRef: "R1C2", Result: F1PAResultNotFoundInCheckedScope,
				Method: "grep", CheckedScope: []string{"src/"}}},
		},
		{
			name: "unavailable with limitation",
			inputs: []PAProbeInput{{TargetRef: "opt-x", Result: F1PAResultUnavailable,
				Limitation: "no archive"}},
		},
		{
			name:   "not_run (no requirement, but does not satisfy coverage)",
			inputs: []PAProbeInput{{TargetRef: "R1C3", Result: F1PAResultNotRun}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summary, perrs := GeneratePAProbes(tc.inputs)
			if len(perrs) != 0 {
				t.Fatalf("producer returned errors for well-formed %q input: %v", tc.name, perrs)
			}
			if verrs := validatePASummary("e", summary); len(verrs) != 0 {
				t.Fatalf("producer output for %q must pass validatePASummary; got:\n  %s", tc.name, strings.Join(verrs, "\n  "))
			}
		})
	}
}

// Producer rejects inconsistent inputs (does not silently emit validator-
// failing output).
func TestGeneratePAProbes_RejectsInconsistentInputs(t *testing.T) {
	_, perrs := GeneratePAProbes([]PAProbeInput{
		{TargetRef: "R1C1", Result: F1PAResultFound},       // no refs
		{TargetRef: "R1C2", Result: F1PAResultUnavailable}, // no limitation
		{TargetRef: "", Result: F1PAResultFound},           // empty target
		{TargetRef: "R1C3", Result: "bogus"},               // unknown enum
	})
	if len(perrs) != 4 {
		t.Fatalf("expected 4 producer errors, got %d: %v", len(perrs), perrs)
	}
}

// Producer purity: never mutates caller's slices.
func TestGeneratePAProbes_DoesNotMutateInputs(t *testing.T) {
	inputs := []PAProbeInput{
		{TargetRef: "Z", Result: F1PAResultFound, EvidenceRefs: []string{"r2", "r1"}, CheckedScope: []string{"s2", "s1"}},
		{TargetRef: "A", Result: F1PAResultNotFoundInCheckedScope, Method: "grep", CheckedScope: []string{"x"}},
	}
	// Snapshot caller order before the call.
	wantRefs := append([]string{}, inputs[0].EvidenceRefs...)
	wantScope := append([]string{}, inputs[0].CheckedScope...)
	wantOrder := []string{inputs[0].TargetRef, inputs[1].TargetRef}
	_, _ = GeneratePAProbes(inputs)
	if len(inputs[0].EvidenceRefs) != len(wantRefs) || inputs[0].EvidenceRefs[0] != wantRefs[0] {
		t.Fatalf("producer mutated caller EvidenceRefs: got %v want %v", inputs[0].EvidenceRefs, wantRefs)
	}
	if len(inputs[0].CheckedScope) != len(wantScope) || inputs[0].CheckedScope[0] != wantScope[0] {
		t.Fatalf("producer mutated caller CheckedScope: got %v want %v", inputs[0].CheckedScope, wantScope)
	}
	if inputs[0].TargetRef != wantOrder[0] || inputs[1].TargetRef != wantOrder[1] {
		t.Fatalf("producer mutated caller input order: got %v want %v", []string{inputs[0].TargetRef, inputs[1].TargetRef}, wantOrder)
	}
}

// --- standalone gate -------------------------------------------------------

func TestValidatePACoverage_AllCoveredPasses(t *testing.T) {
	r1, r3 := paTargetSummaries([]string{"R1C1"}, []string{"opt-continue", "opt-redesign"})
	pa := paSummaryFromProbes(
		F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound, EvidenceRefs: []string{"r1"}},
		F1PAProbe{ProbeID: "PA-P2", TargetRef: "opt-continue", Result: F1PAResultNotFoundInCheckedScope, Method: "grep", CheckedScope: []string{"src/"}},
		F1PAProbe{ProbeID: "PA-P3", TargetRef: "opt-redesign", Result: F1PAResultUnavailable, Limitation: "lim"},
	)
	if errs := ValidatePACoverage(r1, r3, pa); len(errs) != 0 {
		t.Fatalf("all-covered must pass; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// not_run does NOT satisfy coverage (memo L206).
func TestValidatePACoverage_NotRunDoesNotSatisfyCoverage(t *testing.T) {
	r1, _ := paTargetSummaries([]string{"R1C1"}, nil)
	pa := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotRun})
	if errs := ValidatePACoverage(r1, nil, pa); !hasErrContaining(errs, `target "R1C1" has no coverage-satisfying probe`) {
		t.Fatalf("not_run must NOT satisfy coverage; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Gate rejects a dishonest probe even when coverage is otherwise complete
// (defense-in-depth: the gate cannot assume the envelope validator ran).
func TestValidatePACoverage_GateRejectsDishonestProbe(t *testing.T) {
	r1, _ := paTargetSummaries([]string{"R1C1"}, nil)
	// Target IS covered by a valid probe, but a SECOND probe is dishonest
	// (found with no refs). The gate must reject the dishonest probe.
	pa := paSummaryFromProbes(
		F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultFound, EvidenceRefs: []string{"r1"}},
		F1PAProbe{ProbeID: "PA-P2", TargetRef: "R1C1", Result: F1PAResultFound}, // no refs -> dishonest
	)
	if errs := ValidatePACoverage(r1, nil, pa); !hasErrContaining(errs, "result=found requires >=1 non-empty evidence_ref") {
		t.Fatalf("gate must reject dishonest probe even when target is covered; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// No targets -> nothing to cover -> gate passes (no per-probe dishonesty).
func TestValidatePACoverage_NoTargetsPasses(t *testing.T) {
	if errs := ValidatePACoverage(nil, nil, nil); len(errs) != 0 {
		t.Fatalf("no targets must pass; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Gate rejects an UNKNOWN result at its own boundary (closed-vocabulary
// defense-in-depth — the standalone gate cannot assume the envelope validator's
// enum check ran upstream). Without this, paCoverageSatisfies-as-allowlist
// would mark the target "uncovered" but never explain WHY; the explicit enum
// check produces a clear "unknown result" error.
func TestValidatePACoverage_GateRejectsUnknownResult(t *testing.T) {
	r1, _ := paTargetSummaries([]string{"R1C1"}, nil)
	pa := paSummaryFromProbes(F1PAProbe{ProbeID: "PA-P1", TargetRef: "R1C1", Result: "bogus"})
	errs := ValidatePACoverage(r1, nil, pa)
	if !hasErrContaining(errs, "unknown result") {
		t.Fatalf("gate must reject unknown result explicitly; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// paCoverageSatisfies is an EXPLICIT ALLOWLIST (not `!= not_run`): an unknown
// result is NOT coverage-satisfying. This is the backstop behind the gate's
// explicit enum check.
func TestPaCoverageSatisfies_IsExplicitAllowlist(t *testing.T) {
	for _, ok := range []string{F1PAResultFound, F1PAResultNotFoundInCheckedScope, F1PAResultUnavailable} {
		if !paCoverageSatisfies(ok) {
			t.Errorf("paCoverageSatisfies(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{F1PAResultNotRun, "bogus", "", "found "} {
		if paCoverageSatisfies(bad) {
			t.Errorf("paCoverageSatisfies(%q) = true, want false (allowlist must reject)", bad)
		}
	}
}

// --- envelope-level coverage wiring (direct test) -------------------------

// TestValidateF1Envelope_PACoverageRejection exercises the envelope-level
// coverage wiring end-to-end: a required envelope with an uncovered target
// MUST fail ValidateF1Envelope (not just the standalone gate). The shared
// helper validatePACoverageSet is covered by the gate tests above; this pins
// that validateEnvelopePACoverage is actually wired into validateF1EnvelopeContent.
func TestValidateF1Envelope_PACoverageRejection(t *testing.T) {
	env := canonicalF1Fixture()
	// Drop the probe covering opt-continue, leaving it uncovered.
	env.Entries[2].PA.Probes = filterProbe(env.Entries[2].PA.Probes, "PA-P2")
	// Re-derive the digest so the only failure is coverage (not a stale digest).
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, `target "opt-continue" has no coverage-satisfying probe`) {
		t.Fatalf("envelope validator must report uncovered target opt-continue; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// filterProbe returns a copy of probes with the named probe removed (test
// helper; does not mutate the input).
func filterProbe(probes []F1PAProbe, dropID string) []F1PAProbe {
	out := make([]F1PAProbe, 0, len(probes))
	for _, p := range probes {
		if p.ProbeID != dropID {
			out = append(out, p)
		}
	}
	return out
}

// --- deepCopyPA field-preservation regression (the Slice-3 trap) -----------

// TestDeepCopyPA_PreservesAllFields pins that deepCopyPA copies EVERY
// F1PAProbe field (the Slice-3 deepCopyR3/Selection trap, now applied to the
// extended P-a DTO). If a future field is added to F1PAProbe but not to
// deepCopyPA, this test fails.
func TestDeepCopyPA_PreservesAllFields(t *testing.T) {
	orig := &F1PAProbeSummary{Probes: []F1PAProbe{{
		ProbeID: "PA-P1", TargetRef: "R1C1",
		FalsificationQuestion: "q?", Result: F1PAResultFound,
		Method: "grep", CheckedScope: []string{"s1", "s2"},
		EvidenceRefs: []string{"r1", "r2"}, Limitation: "lim",
		WeakestClaim: "wc", Confidence: "high",
	}}}
	cp := deepCopyPA(orig)
	// Mutate the copy's slice fields; the original must be unaffected
	// (independent backing arrays).
	cp.Probes[0].CheckedScope[0] = "MUTATED"
	cp.Probes[0].EvidenceRefs[0] = "MUTATED"
	if orig.Probes[0].CheckedScope[0] == "MUTATED" || orig.Probes[0].EvidenceRefs[0] == "MUTATED" {
		t.Fatal("deepCopyPA shared slice backing arrays with the source (not a deep copy)")
	}
	// Every scalar field is preserved on the copy.
	got := cp.Probes[0]
	if got.ProbeID != "PA-P1" || got.TargetRef != "R1C1" || got.FalsificationQuestion != "q?" ||
		got.Result != F1PAResultFound || got.Method != "grep" || got.Limitation != "lim" ||
		got.WeakestClaim != "wc" || got.Confidence != "high" {
		t.Fatalf("deepCopyPA dropped a scalar field: %+v", got)
	}
	// Slice fields are preserved (length + content).
	if len(got.CheckedScope) != 2 || len(got.EvidenceRefs) != 2 {
		t.Fatalf("deepCopyPA dropped slice content: CheckedScope=%v EvidenceRefs=%v", got.CheckedScope, got.EvidenceRefs)
	}
}
