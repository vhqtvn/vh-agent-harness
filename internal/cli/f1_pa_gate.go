package cli

// f1_pa_gate.go — the P-a TARGET-COVERAGE gate (the ACTS half of memo L274:
// "require-P-a-target-coverage = GATE-SHAPED CONVERSION (envelope / lifecycle
// completeness validator)"). Pure and standalone: given the R1, R3, and PA
// summaries, it returns the list of coverage errors (empty == covered).
//
// SCOPE — what this gate checks and what it deliberately defers:
//   - CHECKS: (1) per-probe result-honesty (validatePAProbeRequirements —
//     found needs real refs; not_found_in_checked_scope needs method+scope;
//     unavailable needs a limitation); (2) target COVERAGE — every material
//     R1 conclusion + every declared R3 option has >=1 coverage-satisfying
//     probe (result != not_run).
//   - DEFERS to the envelope validator: cross-family ref RESOLUTION (a probe
//     TargetRef must resolve to a real R1 conclusion / R3 option — that is
//     resolveF1CrossRefs in f1_validator.go) and digest VALIDITY (the
//     envelope validator's digest re-derivation). This gate is envelope-
//     agnostic by design: it takes the summaries, not the whole envelope.
//
// OPERATOR-OWNED BOUNDARY (memo open-question #5, L307-311): the baseline
// coverage rule IS decided — not_run fails coverage; unavailable satisfies
// coverage structurally (with a limitation) but cannot support proven. The
// NOT-decided extension — whether high-risk release seams ALSO block on
// `unavailable` — is operator-owned and is NOT enforced here. Callers that
// need the high-risk-seam policy must layer it on top (and that decision
// belongs to a policy owner, not this structural gate).
//
// Structural, not truth: this gate proves coverage COMPLETENESS and per-probe
// HONESTY, NOT that the evidence is actually true (memo L276:
// determine-whether-evidence-actually-true is a federated verifier
// responsibility, NOT a structural gate).

import "fmt"

// ValidatePACoverage is the standalone require-P-a-target-coverage gate. It
// takes the R1, R3, and PA summaries directly (no envelope) and returns the
// coverage + per-probe-honesty errors. Empty return == every material target
// is covered by an honest probe. The gate mutates nothing.
func ValidatePACoverage(r1 *F1R1JoinSummary, r3 *F1R3ForkSummary, pa *F1PAProbeSummary) []string {
	var errs []string
	// 0. Closed-vocabulary rejection (defense-in-depth). The standalone gate
	//    cannot assume the envelope validator's enum check ran upstream
	//    (F1PAResult is a closed enum; an unknown Result must be rejected at
	//    the gate's own boundary, mirroring the R3 gate's unconditional
	//    disposition-enum check). paCoverageSatisfies is an explicit allowlist
	//    so an unknown result is NOT silently treated as coverage-satisfying,
	//    but this explicit check produces a clear "unknown result" error
	//    rather than a confusing "uncovered target" error.
	if pa != nil {
		for i := range pa.Probes {
			p := &pa.Probes[i]
			if _, ok := f1ValidPAResults[p.Result]; !ok {
				errs = append(errs, fmt.Sprintf("pa-gate.pa.probes[%d]: unknown result %q (want one of %s)", i, p.Result, f1SortedKeys(f1ValidPAResults)))
			}
		}
	}
	// 1. Per-probe honesty (rejects dishonest probes regardless of coverage).
	errs = append(errs, validatePAProbeRequirements("pa-gate", pa)...)
	// 2. Target coverage (rejects uncovered targets).
	targets := paTargetSet(r1, r3)
	if len(targets) == 0 {
		// No material conclusions/options: nothing to cover. Still report any
		// per-probe dishonesty from (0)/(1).
		return errs
	}
	errs = append(errs, validatePACoverageSet("pa-gate-coverage", targets, pa)...)
	return errs
}
