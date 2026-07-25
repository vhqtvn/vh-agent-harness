package cli

// f1_r3.go — the R3 repair-routing fork PRODUCER (Slice 3 of the F1 synthesis-
// family). Pure, deterministic, domain-free. Mirrors the R1 producer's
// authority stance: the producer INFORMS (it generates + surfaces the fork);
// the GATE (f1_r3_gate.go) ACTS (it refuses a transition that fails
// completeness or lacks a recorded disposition). The coordinator never holds
// transition authority.
//
// Design authority: researches/decisions/2026-07-25-f1-synthesis-family-and-
// s2a-topology.md (amended, commit 15ddd54), L150-194.
//
// OPERATOR-OWNED SEAMS (deferred, NOT decided here):
//   - repo-verdict → structural_review_outcome mapping (memo open-question #4).
//     The producer takes structural_review_outcome as a closed-enum INPUT; the
//     caller supplies the already-classified value. No narrative inference.
//   - operator-disposition timing / exact lifecycle transition (memo
//     open-question #6). The gate takes a `transitionRequested` flag; the exact
//     repo lifecycle point that sets it is the operator's decision.
//
// This file holds NO report writes, transitions, network, F2 rendering, or
// claims DB. It proves structural completeness, NOT that the redesign is good
// (quality is an operator judgment; the gate validates completeness only —
// memo L193-194).

import (
	"fmt"
	"strings"
)

// R3ForkInput is the producer input for the R3 repair-routing fork.
// RepairIntent and StructuralReviewOutcome are closed-enum values the caller
// supplies (the repo-verdict→enum mapping is operator-owned). When the trigger
// fires, ContinueRepair and Redesign must both be present.
type R3ForkInput struct {
	RepairIntent            string
	StructuralReviewOutcome string
	ContinueRepair          *F1R3Option
	Redesign                *F1R3Option
}

// GenerateR3Fork is the pure R3 producer. It classifies the trigger and, when
// the trigger fires, assembles a fork with a continue_repair option and a
// materially-distinct redesign option. It returns the fork summary plus a list
// of producer errors (empty == the fork is structurally complete for its
// triggered state). It NEVER mutates its inputs and NEVER shells out.
//
// Trigger: RepairIntent==present AND StructuralReviewOutcome==non_pass. When
// the trigger fires, both options are required, the redesign must be
// materially distinct from the continue-repair, and each option must carry an
// R1 basis (≥1 support_ref) and P-a coverage (≥1 counter_evidence_probe_ref).
// When the trigger does NOT fire, the fork carries no options (a non-triggered
// fork with options is inconsistent — options imply a triggered fork).
func GenerateR3Fork(input R3ForkInput) (*F1R3ForkSummary, []string) {
	var errs []string

	if _, ok := f1ValidR3RepairIntents[input.RepairIntent]; !ok {
		errs = append(errs, fmt.Sprintf("r3: unknown repair_intent %q (want one of %s)", input.RepairIntent, f1SortedKeys(f1ValidR3RepairIntents)))
	}
	if _, ok := f1ValidR3StructuralReviewOutcomes[input.StructuralReviewOutcome]; !ok {
		errs = append(errs, fmt.Sprintf("r3: unknown structural_review_outcome %q (want one of %s)", input.StructuralReviewOutcome, f1SortedKeys(f1ValidR3StructuralReviewOutcomes)))
	}

	triggered := input.RepairIntent == F1R3RepairIntentPresent && input.StructuralReviewOutcome == F1R3StructuralReviewNonPass

	if !triggered {
		if input.ContinueRepair != nil || input.Redesign != nil {
			errs = append(errs, "r3: non-triggered fork must not carry options (options imply a triggered fork)")
		}
		// Return even on enum errors so the caller sees the full picture; the
		// summary is structurally inert (not triggered, no options).
		return &F1R3ForkSummary{TriggerRecognized: false, Disposition: F1R3DispositionPending}, errs
	}

	// Triggered: require both options.
	if input.ContinueRepair == nil {
		errs = append(errs, "r3: triggered fork requires a continue_repair option")
	}
	if input.Redesign == nil {
		errs = append(errs, "r3: triggered fork requires a redesign option")
	}
	if input.ContinueRepair != nil && input.Redesign != nil {
		if input.ContinueRepair.Mode != F1R3ModeContinueRepair {
			errs = append(errs, fmt.Sprintf("r3: continue_repair option has mode %q (want %q)", input.ContinueRepair.Mode, F1R3ModeContinueRepair))
		}
		if input.Redesign.Mode != F1R3ModeRedesign {
			errs = append(errs, fmt.Sprintf("r3: redesign option has mode %q (want %q)", input.Redesign.Mode, F1R3ModeRedesign))
		}
		if !isMateriallyDistinct(input.ContinueRepair, input.Redesign) {
			errs = append(errs, "r3: redesign is not materially distinct from continue_repair (it merely renames, delays, or subdivides the same repair)")
		}
		if len(input.ContinueRepair.SupportRefs) == 0 {
			errs = append(errs, "r3: continue_repair option has no support_refs (R1 basis required)")
		}
		if len(input.Redesign.SupportRefs) == 0 {
			errs = append(errs, "r3: redesign option has no support_refs (R1 basis required)")
		}
		if len(input.ContinueRepair.CounterEvidenceProbeRefs) == 0 {
			errs = append(errs, "r3: continue_repair option has no counter_evidence_probe_refs (P-a coverage required)")
		}
		if len(input.Redesign.CounterEvidenceProbeRefs) == 0 {
			errs = append(errs, "r3: redesign option has no counter_evidence_probe_refs (P-a coverage required)")
		}
	}

	if len(errs) > 0 {
		// Still return a triggered, pending summary so the caller has a handle;
		// the errors make clear it is not structurally complete.
		return &F1R3ForkSummary{TriggerRecognized: true, Disposition: F1R3DispositionPending}, errs
	}

	return &F1R3ForkSummary{
		TriggerRecognized: true,
		Disposition:       F1R3DispositionPending,
		Options: []F1R3Option{
			*deepCopyR3Option(input.ContinueRepair),
			*deepCopyR3Option(input.Redesign),
		},
	}, nil
}

// RecordR3Disposition returns a NEW fork with the operator disposition
// recorded. It is the ONLY sanctioned way to change a fork's disposition, and
// it is immutable: prior is never mutated (deep-copied). This encodes the
// authority-line rule that disposition is recorded by an explicit operator
// action (selected/rejected/deferred) before any route transition — never by
// coordinator in-place mutation. An unknown disposition is an error.
func RecordR3Disposition(prior *F1R3ForkSummary, disposition string) (*F1R3ForkSummary, error) {
	if prior == nil {
		return nil, fmt.Errorf("RecordR3Disposition: prior fork is nil")
	}
	if _, ok := f1ValidR3Dispositions[disposition]; !ok {
		return nil, fmt.Errorf("RecordR3Disposition: unknown disposition %q (want one of %s)", disposition, f1SortedKeys(f1ValidR3Dispositions))
	}
	out := deepCopyR3Fork(prior)
	out.Disposition = disposition
	return out, nil
}

// isMateriallyDistinct is the structural material-difference guard. The amended
// memo (L157-159) makes a redesign INVALID if it "merely renames, delays, or
// subdivides the same repair." This function returns false when the redesign is
// a trivial restatement of the continue-repair. It compares the mechanisms'
// CORES: the token sets with filler (articles, conjunctions, prepositions,
// timing, durations, quantities, procedural filler) stripped. A redesign is
// NOT materially distinct when:
//   - rename:    the normalized mechanisms are equal, OR their cores are equal;
//   - subdivide: the redesign's core adds no substantive token the
//     continue-repair does not already carry (redesign-core ⊆ continue-core);
//   - delay:     the redesign's extra tokens are all filler (timing/duration),
//     so the cores are equal — "apply repair after two weeks" and "apply
//     repair" share the core {apply, repair}.
//
// This is a NECESSARY structural baseline (it catches trivial restatement
// across rename/subdivide/delay phrasings), NOT a sufficient quality judge.
// Deeper material-difference is an operator/semantic judgment; the gate
// validates completeness, not redesign quality (memo L193-194).
func isMateriallyDistinct(continueOpt, redesignOpt *F1R3Option) bool {
	if continueOpt == nil || redesignOpt == nil {
		return false
	}
	nc := normalizeMechanism(continueOpt.Mechanism)
	nr := normalizeMechanism(redesignOpt.Mechanism)
	if nc == "" || nr == "" {
		return false
	}
	if nc == nr {
		return false // exact rename
	}
	cc := coreTokenSet(nc)
	cr := coreTokenSet(nr)
	// If stripping filler leaves no substantive token on either side, the
	// mechanism is all filler — cannot establish distinctness.
	if len(cc) == 0 || len(cr) == 0 {
		return false
	}
	if tokenSetEqual(cc, cr) {
		return false // same core after stripping filler => rename/delay with filler
	}
	if tokenSetSubset(cr, cc) {
		return false // redesign core adds no substantive token => subdivide
	}
	return true
}

// r3FillerTokens are tokens stripped before comparing mechanism cores: they
// carry no substantive approach information (articles, conjunctions,
// prepositions, timing, durations, quantities, procedural filler). Stripping
// them lets a delay phrased with arbitrary duration vocabulary ("apply repair
// after two weeks") compare equal to its base ("apply repair") — both share
// the core {apply, repair}. The set is deliberately broad on the
// non-substantive side; a token that could name an action or object is NOT a
// filler token and is retained.
var r3FillerTokens = map[string]struct{}{
	// articles
	"a": {}, "an": {}, "the": {},
	// conjunctions
	"and": {}, "or": {}, "but": {}, "so": {}, "yet": {},
	// prepositions
	"to": {}, "of": {}, "in": {}, "on": {}, "at": {}, "by": {}, "for": {},
	"with": {}, "from": {}, "into": {}, "onto": {}, "upon": {},
	// timing
	"later": {}, "delay": {}, "delayed": {}, "defer": {}, "deferred": {},
	"postpone": {}, "postponed": {}, "eventually": {}, "queue": {}, "queued": {},
	"schedule": {}, "scheduled": {}, "after": {}, "then": {}, "next": {},
	"finally": {}, "subsequently": {}, "when": {}, "ready": {}, "until": {},
	"before": {}, "soon": {}, "tomorrow": {}, "today": {}, "yesterday": {},
	"now": {}, "while": {},
	// durations
	"week": {}, "weeks": {}, "day": {}, "days": {}, "month": {}, "months": {},
	"quarter": {}, "year": {}, "years": {}, "hour": {}, "hours": {},
	"minute": {}, "minutes": {}, "second": {}, "seconds": {},
	// quantities / ordinals
	"one": {}, "two": {}, "three": {}, "four": {}, "five": {}, "six": {},
	"seven": {}, "eight": {}, "nine": {}, "ten": {}, "first": {}, "third": {},
	"once": {}, "twice": {}, "some": {}, "any": {}, "all": {},
	// procedural filler
	"step": {}, "steps": {}, "phase": {}, "phases": {}, "stage": {}, "stages": {},
	"part": {}, "parts": {}, "iteratively": {}, "incrementally": {}, "via": {},
}

// normalizeMechanism lower-cases the mechanism, collapses internal whitespace
// runs to a single space, and strips the punctuation runes period/comma/
// semicolon/colon so superficial variants (case, spacing, trailing or mid
// punctuation) compare equal.
func normalizeMechanism(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	out := make([]rune, 0, len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n':
			if !prevSpace {
				out = append(out, ' ')
				prevSpace = true
			}
		case r == '.' || r == ',' || r == ';' || r == ':':
			// drop the punctuation rune wherever it occurs
			continue
		default:
			out = append(out, r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(string(out))
}

// coreTokenSet returns the substantive tokens of s: all whitespace-separated
// tokens that are NOT stripped. Three strip rules make the comparison robust
// against arbitrary-quantity delay phrasings without an unbounded word-list:
//  1. digit-only tokens ("11", "99") are stripped — they are quantities;
//  2. a token immediately preceding a duration-unit token is stripped — it is
//     the quantity of that duration ("two weeks", "eleven weeks", "few months");
//  3. duration-unit tokens themselves (week/days/months/...) and the static
//     r3FillerTokens (articles, prepositions, timing words, ...) are stripped.
//
// The surviving tokens are the substantive core (actions, objects, properties)
// that two mechanisms are compared on.
func coreTokenSet(s string) map[string]struct{} {
	fields := strings.Fields(s)
	set := map[string]struct{}{}
	for i, t := range fields {
		if isDigits(t) {
			continue // rule 1: numeric quantity
		}
		if i+1 < len(fields) {
			if _, du := r3DurationUnits[fields[i+1]]; du {
				continue // rule 2: quantity preceding a duration unit
			}
		}
		if _, du := r3DurationUnits[t]; du {
			continue // rule 3a: duration-unit token
		}
		if _, filler := r3FillerTokens[t]; filler {
			continue // rule 3b: static filler
		}
		set[t] = struct{}{}
	}
	return set
}

// r3DurationUnits are the duration-unit tokens. A token immediately preceding
// one of these is treated as its quantity and stripped (rule 2 above).
var r3DurationUnits = map[string]struct{}{
	"week": {}, "weeks": {}, "day": {}, "days": {}, "month": {}, "months": {},
	"quarter": {}, "quarters": {}, "year": {}, "years": {}, "hour": {}, "hours": {},
	"minute": {}, "minutes": {}, "second": {}, "seconds": {},
}

// isDigits reports whether s is a non-empty run of ASCII digits (a numeric
// quantity like "11" or "99").
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// tokenSetEqual reports whether a and b contain exactly the same tokens.
func tokenSetEqual(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// tokenSetSubset reports whether every token in a is also in b (a ⊆ b).
func tokenSetSubset(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// deepCopyR3Fork returns an independent deep copy of fork (options + all
// nested slices). Used by RecordR3Disposition and WithNewR1Cycle so a
// disposition/cycle change never aliases the prior fork's backing arrays.
func deepCopyR3Fork(fork *F1R3ForkSummary) *F1R3ForkSummary {
	if fork == nil {
		return nil
	}
	out := &F1R3ForkSummary{
		TriggerRecognized: fork.TriggerRecognized,
		Disposition:       fork.Disposition,
	}
	if fork.Selection != nil {
		out.Selection = &F1R3Selection{
			SelectedOptionID:           fork.Selection.SelectedOptionID,
			RedesignRejectionRationale: fork.Selection.RedesignRejectionRationale,
		}
	}
	if fork.Options != nil {
		out.Options = make([]F1R3Option, len(fork.Options))
		for i := range fork.Options {
			o := deepCopyR3Option(&fork.Options[i])
			out.Options[i] = *o
		}
	}
	return out
}

// deepCopyR3Option returns an independent deep copy of one option.
func deepCopyR3Option(o *F1R3Option) *F1R3Option {
	if o == nil {
		return nil
	}
	return &F1R3Option{
		OptionID:                 o.OptionID,
		Mode:                     o.Mode,
		Mechanism:                o.Mechanism,
		AffectedProperties:       copyStrings(o.AffectedProperties),
		SupportRefs:              copyStrings(o.SupportRefs),
		CounterEvidenceProbeRefs: copyStrings(o.CounterEvidenceProbeRefs),
		Costs:                    copyStrings(o.Costs),
		Risks:                    copyStrings(o.Risks),
		ReversalCost:             o.ReversalCost,
		CheapestValidation:       o.CheapestValidation,
	}
}
