package cli

// f1_r3_gate.go — the R3 repair-routing fork GATE (Slice 3). This is the
// GATE-SHAPED CONVERSION the amended memo (L187-194) requires: a repair-routing
// / task-lifecycle VALIDATOR. THE GATE (not the coordinator) applies the
// refusal.
//
// SCOPE — what this gate checks (fork-level completeness + transition):
//   - trigger-recognized (when a transition is requested);
//   - both-options-exist (a continue_repair AND a materially-distinct redesign
//     option, when triggered);
//   - both-have-P-a-COVERAGE (each option carries ≥1 counter_evidence_probe
//     link + ≥1 R1 support basis — a COUNT check);
//   - disposition-recorded-before-transition (non-pending when transitioning);
//   - selection-rationale (when disposition==selected).
//
// SCOPE — what this gate does NOT re-check (it is fork-only; it takes no
// envelope): cross-family ref RESOLUTION (option.SupportRefs → R1 conclusion
// IDs, option.CounterEvidenceProbeRefs → P-a probe IDs) and digest VALIDITY.
// Those are enforced by the envelope structural validator: resolveF1CrossRefs
// (f1_validator.go) resolves every inter-family ref, and ValidateF1Envelope's
// digest re-derivation binds the canonical content. The gate assumes the
// envelope has been (or will be) run through ValidateF1Envelope for ref/digest
// validity; it does not duplicate that envelope-aware work. This split is
// deliberate: the gate is a standalone fork check callable without an envelope,
// while ref/digest validity is inherently envelope-scoped.
//
// Authority line (memo L270-272):
//   - require-fork-before-repair-route     = GATE-SHAPED CONVERSION (this file)
//   - record-disposition-before-transition = GATE-SHAPED CONVERSION (this file)
//
// The gate is a PURE function: it returns a list of structural errors and
// mutates NOTHING. The lifecycle owner (the caller at the transition point)
// applies the refusal by acting on a non-empty error list. There is no
// in-place disposition setter and no state mutation here — that is the whole
// point of "the gate acts, the coordinator informs."
//
// OPERATOR-OWNED SEAM (memo open-question #6, L312-314): the EXACT repo
// lifecycle transition where pending→selected/rejected/deferred is operator-
// owned. This gate takes a `transitionRequested` flag as input; the caller
// (the lifecycle owner) sets it. Wiring the gate into a specific repo state
// machine would decide an operator-owned question, so it is deferred
// (stub-with-contract: the gate is the contract; the transition point is the
// operator's).
//
// The gate validates COMPLETENESS, not redesign quality (memo L193-194).

import (
	"fmt"
	"strings"
)

// ValidateR3ForkForTransition is the R3 transition gate. It returns the list
// of fork-level structural errors that block a repair-route transition. Empty
// == the transition may proceed (assuming the envelope has also passed
// ValidateF1Envelope for cross-family ref resolution + digest validity — see
// the file-level SCOPE note). It is pure: it never mutates fork and never
// shells out.
//
// Checks:
//   - fork-completeness (validateR3ForkCompleteness): when triggered, both a
//     continue_repair and a materially-distinct redesign option exist, each
//     with an R1 basis and P-a coverage link;
//   - selection-rationale (validateR3Selection): when disposition==selected;
//   - if transitionRequested: the trigger must be recognized AND the operator
//     disposition must be non-pending (selected/rejected/deferred) — a
//     transition may not proceed on a pending disposition.
func ValidateR3ForkForTransition(fork *F1R3ForkSummary, transitionRequested bool) []string {
	var errs []string
	if fork == nil {
		if transitionRequested {
			return []string{"r3 transition requested but no r3 fork is present (require-fork-before-repair-route)"}
		}
		return nil
	}
	errs = append(errs, validateR3ForkCompleteness("r3", fork)...)
	// Disposition enum membership — the gate is standalone (it takes no
	// envelope), so it validates the carried disposition itself rather than
	// assuming ValidateF1Envelope ran upstream. This rejects an unknown
	// disposition (e.g. deserialized "garbage") that would otherwise slip
	// past the ==pending / ==selected branches below.
	if _, ok := f1ValidR3Dispositions[fork.Disposition]; !ok {
		errs = append(errs, fmt.Sprintf("r3: unknown disposition %q (want one of %s)", fork.Disposition, f1SortedKeys(f1ValidR3Dispositions)))
	}
	if fork.Disposition == F1R3DispositionSelected {
		errs = append(errs, validateR3Selection("r3", fork)...)
	}
	if transitionRequested {
		if !fork.TriggerRecognized {
			errs = append(errs, "r3 transition requested but the fork trigger is not recognized")
		}
		if fork.Disposition == F1R3DispositionPending {
			errs = append(errs, "r3 transition requested but operator disposition is still pending (record selected/rejected/deferred before transition)")
		}
	}
	return errs
}

// validateR3ForkCompleteness checks the fork is structurally complete for its
// triggered state. It is shared by the envelope structural validator
// (validateR3Summary) and the transition gate so a committed projection and a
// transition decision see the SAME completeness rule. Pure.
//
//   - Triggered: requires EXACTLY ONE continue_repair option AND EXACTLY ONE
//     redesign option (the fork is a binary repair-vs-redesign decision: one
//     repair under review, one materially-distinct redesign alternative), the
//     redesign materially distinct from the continue-repair, and each option
//     carrying an R1 basis (≥1 support_ref) and P-a coverage (≥1
//     counter_evidence_probe_ref). A second option of either mode is rejected.
//   - Not triggered: the fork must carry NO options (options imply a trigger).
//
// prefix labels the error source (the envelope validator passes the entry path
// like "entries[1].r3"; the gate passes "r3").
func validateR3ForkCompleteness(prefix string, r3 *F1R3ForkSummary) []string {
	if r3 == nil {
		return nil
	}
	p := prefix
	var errs []string
	if !r3.TriggerRecognized {
		if len(r3.Options) > 0 {
			errs = append(errs, p+": non-triggered fork carries options (options imply a triggered fork)")
		}
		return errs
	}
	var continueOpt, redesignOpt *F1R3Option
	for i := range r3.Options {
		o := &r3.Options[i]
		switch o.Mode {
		case F1R3ModeContinueRepair:
			if continueOpt != nil {
				errs = append(errs, p+": multiple continue_repair options (want exactly one)")
			}
			continueOpt = o
		case F1R3ModeRedesign:
			if redesignOpt != nil {
				errs = append(errs, p+": multiple redesign options (want exactly one)")
			}
			redesignOpt = o
		}
		if len(o.SupportRefs) == 0 {
			errs = append(errs, fmt.Sprintf("%s: option %q has no support_refs (R1 basis required)", p, o.OptionID))
		}
		if len(o.CounterEvidenceProbeRefs) == 0 {
			errs = append(errs, fmt.Sprintf("%s: option %q has no counter_evidence_probe_refs (P-a coverage required)", p, o.OptionID))
		}
	}
	if continueOpt == nil {
		errs = append(errs, p+": triggered fork has no continue_repair option")
	}
	if redesignOpt == nil {
		errs = append(errs, p+": triggered fork has no redesign option")
	}
	if continueOpt != nil && redesignOpt != nil && !isMateriallyDistinct(continueOpt, redesignOpt) {
		errs = append(errs, p+": redesign is not materially distinct from continue_repair (it merely renames, delays, or subdivides the same repair)")
	}
	return errs
}

// validateR3Selection checks the operator's recorded selection when
// Disposition==selected. The Selection must be present, its SelectedOptionID
// must resolve to a declared option, and — if the selected option is the
// continue_repair — a non-empty RedesignRejectionRationale is required (the
// operator engaged the redesign and stated why it was not taken). Selecting the
// redesign needs no rejection rationale. Pure.
func validateR3Selection(prefix string, r3 *F1R3ForkSummary) []string {
	p := prefix
	if r3.Selection == nil {
		return []string{p + ": disposition is selected but no selection record is present"}
	}
	var continueOpt *F1R3Option
	optionByID := map[string]*F1R3Option{}
	for i := range r3.Options {
		o := &r3.Options[i]
		optionByID[o.OptionID] = o
		if o.Mode == F1R3ModeContinueRepair {
			continueOpt = o
		}
	}
	chosen, ok := optionByID[r3.Selection.SelectedOptionID]
	if !ok {
		return []string{fmt.Sprintf("%s: selection.selected_option_id %q does not resolve to any declared option", p, r3.Selection.SelectedOptionID)}
	}
	if continueOpt != nil && chosen.OptionID == continueOpt.OptionID && strings.TrimSpace(r3.Selection.RedesignRejectionRationale) == "" {
		return []string{p + ": continue_repair selected but no redesign_rejection_rationale recorded (engage the redesign and state why it was not taken)"}
	}
	return nil
}
