package cli

import (
	"strings"
	"testing"
)

// f1_r3_test.go — Slice 3 tests for the R3 repair-routing fork producer +
// gate. Covers the amended-memo contract (L150-194): trigger truth table,
// material-difference, R1 basis, P-a coverage, disposition-before-transition,
// selection-rationale, and the authority-line invariant (the gate acts; the
// coordinator never mutates lifecycle state in place).

// validR3ForkInput returns a producer input that fires the trigger and carries
// two materially-distinct options, each with an R1 basis and P-a coverage. It
// is the golden input every happy-path test starts from.
func validR3ForkInput() R3ForkInput {
	return R3ForkInput{
		RepairIntent:            F1R3RepairIntentPresent,
		StructuralReviewOutcome: F1R3StructuralReviewNonPass,
		ContinueRepair: &F1R3Option{
			OptionID:                 "opt-continue",
			Mode:                     F1R3ModeContinueRepair,
			Mechanism:                "apply the queued repair to the boundary",
			AffectedProperties:       []string{"R1P1"},
			SupportRefs:              []string{"R1C1"},
			CounterEvidenceProbeRefs: []string{"PA-P1"},
			Costs:                    []string{"eng-hours"},
			Risks:                    []string{"recurrence"},
			ReversalCost:             "low",
			CheapestValidation:       "re-run suite",
		},
		Redesign: &F1R3Option{
			OptionID:                 "opt-redesign",
			Mode:                     F1R3ModeRedesign,
			Mechanism:                "redesign the boundary so the hazard cannot recur",
			AffectedProperties:       []string{"R1P1"},
			SupportRefs:              []string{"R1C1"},
			CounterEvidenceProbeRefs: []string{"PA-P2"},
			Costs:                    []string{"eng-weeks"},
			Risks:                    []string{"schedule-slip"},
			ReversalCost:             "high",
			CheapestValidation:       "design-review + suite",
		},
	}
}

// --- trigger truth table (Slice 3 required case 1+2) ----------------------

func TestGenerateR3Fork_TriggerTruthTable(t *testing.T) {
	cases := []struct {
		name       string
		intent     string
		outcome    string
		wantTrig   bool
		wantErrSub string // non-empty => expect at least one error containing this
	}{
		{"present+non_pass fires", F1R3RepairIntentPresent, F1R3StructuralReviewNonPass, true, ""},
		{"present+pass does NOT fire", F1R3RepairIntentPresent, F1R3StructuralReviewPass, false, ""},
		{"absent+non_pass does NOT fire", F1R3RepairIntentAbsent, F1R3StructuralReviewNonPass, false, ""},
		{"absent+pass does NOT fire", F1R3RepairIntentAbsent, F1R3StructuralReviewPass, false, ""},
		{"unknown intent rejected", "maybe", F1R3StructuralReviewNonPass, false, "unknown repair_intent"},
		{"unknown outcome rejected", F1R3RepairIntentPresent, "indeterminate", false, "unknown structural_review_outcome"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := R3ForkInput{RepairIntent: c.intent, StructuralReviewOutcome: c.outcome}
			// For the firing case, supply valid options; otherwise omit them.
			if c.wantTrig {
				in = validR3ForkInput()
				in.RepairIntent = c.intent
				in.StructuralReviewOutcome = c.outcome
			}
			fork, errs := GenerateR3Fork(in)
			if fork.TriggerRecognized != c.wantTrig {
				t.Fatalf("TriggerRecognized=%v, want %v", fork.TriggerRecognized, c.wantTrig)
			}
			if c.wantErrSub != "" {
				if !errsContain(errs, c.wantErrSub) {
					t.Fatalf("expected error containing %q; got %v", c.wantErrSub, errs)
				}
			}
			// A non-firing trigger must never fabricate the fork: options stay
			// empty (this is the "non-structural/passing review doesn't
			// fabricate trigger" guarantee).
			if !c.wantTrig && len(fork.Options) > 0 {
				t.Fatalf("non-triggered fork must carry no options; got %d", len(fork.Options))
			}
		})
	}
}

// --- missing redesign fails (case 3) ---------------------------------------

func TestGenerateR3Fork_MissingRedesignFails(t *testing.T) {
	in := validR3ForkInput()
	in.Redesign = nil
	fork, errs := GenerateR3Fork(in)
	if !fork.TriggerRecognized {
		t.Fatal("trigger should be recognized")
	}
	if !errsContain(errs, "requires a redesign option") {
		t.Fatalf("missing redesign must fail; got %v", errs)
	}
}

func TestGenerateR3Fork_MissingContinueRepairFails(t *testing.T) {
	in := validR3ForkInput()
	in.ContinueRepair = nil
	_, errs := GenerateR3Fork(in)
	if !errsContain(errs, "requires a continue_repair option") {
		t.Fatalf("missing continue_repair must fail; got %v", errs)
	}
}

// --- renamed variants don't satisfy material difference (case 4) -----------

func TestIsMateriallyDistinct(t *testing.T) {
	cases := []struct {
		name         string
		continueMech string
		redesignMech string
		wantDistinct bool
	}{
		{"exact rename (case-different)", "Apply the Queued Repair", "apply the queued repair", false},
		{"exact rename (identical)", "apply the queued repair", "apply the queued repair", false},
		{"redesign adds nothing (token subset)", "apply the queued repair now", "apply the queued repair", false},
		{"delay only (timing word added)", "apply the repair", "apply the repair later", false},
		{"duration-word delay (arbitrary vocabulary)", "apply repair", "apply repair after two weeks", false},
		{"duration-word delay (months)", "apply the patch", "apply the patch in three months", false},
		{"quantity-word beyond ten (eleven weeks)", "apply repair", "apply repair after eleven weeks", false},
		{"numeric quantity (11 weeks)", "apply repair", "apply repair after 11 weeks", false},
		{"numeric quantity (99 days)", "apply repair", "apply repair after 99 days", false},
		{"clearly distinct redesign", "apply the queued repair", "redesign the boundary so the hazard cannot recur", true},
		{"distinct verb", "apply the repair", "replace the subsystem", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			co := &F1R3Option{Mechanism: c.continueMech}
			rd := &F1R3Option{Mechanism: c.redesignMech}
			if got := isMateriallyDistinct(co, rd); got != c.wantDistinct {
				t.Fatalf("isMateriallyDistinct(%q,%q)=%v, want %v", c.continueMech, c.redesignMech, got, c.wantDistinct)
			}
		})
	}
}

func TestGenerateR3Fork_RenamedRedesignFails(t *testing.T) {
	in := validR3ForkInput()
	// Redesign merely renames the continue-repair mechanism.
	in.Redesign.Mechanism = "Apply The Queued Repair To The Boundary"
	_, errs := GenerateR3Fork(in)
	if !errsContain(errs, "not materially distinct") {
		t.Fatalf("renamed redesign must fail material-difference; got %v", errs)
	}
}

// --- missing R1 basis fails (case 5) ---------------------------------------

func TestGenerateR3Fork_MissingR1BasisFails(t *testing.T) {
	in := validR3ForkInput()
	in.ContinueRepair.SupportRefs = nil
	_, errs := GenerateR3Fork(in)
	if !errsContain(errs, "continue_repair option has no support_refs") {
		t.Fatalf("missing R1 basis must fail; got %v", errs)
	}
}

// --- missing P-a coverage fails (case 6) -----------------------------------

func TestGenerateR3Fork_MissingPACoverageFails(t *testing.T) {
	in := validR3ForkInput()
	in.Redesign.CounterEvidenceProbeRefs = nil
	_, errs := GenerateR3Fork(in)
	if !errsContain(errs, "redesign option has no counter_evidence_probe_refs") {
		t.Fatalf("missing P-a coverage must fail; got %v", errs)
	}
}

// --- selected repair without recorded redesign-rejection rationale fails (case 7) --

func TestValidateR3ForkForTransition_SelectionWithoutRationaleFails(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	// Operator selects the continue_repair but records no rationale.
	selected, err := RecordR3Disposition(fork, F1R3DispositionSelected)
	if err != nil {
		t.Fatal(err)
	}
	selected.Selection = &F1R3Selection{SelectedOptionID: "opt-continue"} // empty rationale
	errs := ValidateR3ForkForTransition(selected, true)
	if !errsContain(errs, "no redesign_rejection_rationale recorded") {
		t.Fatalf("selecting continue_repair without rationale must fail; got %v", errs)
	}
}

func TestValidateR3ForkForTransition_SelectionWithRationalePasses(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	selected, err := RecordR3Disposition(fork, F1R3DispositionSelected)
	if err != nil {
		t.Fatal(err)
	}
	selected.Selection = &F1R3Selection{
		SelectedOptionID:           "opt-continue",
		RedesignRejectionRationale: "redesign cost is out of scope for this cycle; recurrence risk accepted",
	}
	if errs := ValidateR3ForkForTransition(selected, true); len(errs) != 0 {
		t.Fatalf("selecting continue_repair WITH rationale must pass the gate; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR3ForkForTransition_SelectingRedesignNeedsNoRationale(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	selected, err := RecordR3Disposition(fork, F1R3DispositionSelected)
	if err != nil {
		t.Fatal(err)
	}
	// Selecting the redesign needs no rejection rationale.
	selected.Selection = &F1R3Selection{SelectedOptionID: "opt-redesign"}
	if errs := ValidateR3ForkForTransition(selected, true); len(errs) != 0 {
		t.Fatalf("selecting redesign (no rationale) must pass; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateR3ForkForTransition_SelectedDispositionWithoutSelectionFails(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	selected, err := RecordR3Disposition(fork, F1R3DispositionSelected)
	if err != nil {
		t.Fatal(err)
	}
	// Disposition=selected but no Selection record at all.
	errs := ValidateR3ForkForTransition(selected, true)
	if !errsContain(errs, "no selection record is present") {
		t.Fatalf("selected disposition without a selection record must fail; got %v", errs)
	}
}

func TestValidateR3ForkForTransition_WhitespaceOnlyRationaleFails(t *testing.T) {
	// A whitespace-only rationale must not satisfy the requirement (the gate
	// trims before checking emptiness).
	fork, _ := GenerateR3Fork(validR3ForkInput())
	selected, err := RecordR3Disposition(fork, F1R3DispositionSelected)
	if err != nil {
		t.Fatal(err)
	}
	selected.Selection = &F1R3Selection{SelectedOptionID: "opt-continue", RedesignRejectionRationale: "   "}
	errs := ValidateR3ForkForTransition(selected, true)
	if !errsContain(errs, "no redesign_rejection_rationale recorded") {
		t.Fatalf("whitespace-only rationale must fail; got %v", errs)
	}
}

// --- disposition-before-transition (gate core) ----------------------------

func TestValidateR3ForkForTransition_PendingDispositionBlocksTransition(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	// Producer sets disposition=pending.
	errs := ValidateR3ForkForTransition(fork, true)
	if !errsContain(errs, "disposition is still pending") {
		t.Fatalf("pending disposition must block transition; got %v", errs)
	}
}

func TestValidateR3ForkForTransition_NonTriggeredForkBlocksTransition(t *testing.T) {
	fork, _ := GenerateR3Fork(R3ForkInput{
		RepairIntent:            F1R3RepairIntentAbsent,
		StructuralReviewOutcome: F1R3StructuralReviewNonPass,
	})
	errs := ValidateR3ForkForTransition(fork, true)
	if !errsContain(errs, "trigger is not recognized") {
		t.Fatalf("transition on a non-triggered fork must fail; got %v", errs)
	}
}

func TestValidateR3ForkForTransition_UnknownDispositionRejected(t *testing.T) {
	// B-F1 regression: the standalone gate must reject an unknown disposition
	// itself (it takes no envelope, so it cannot assume ValidateF1Envelope ran
	// upstream). "garbage" must not slip past the ==pending / ==selected
	// branches — whether or not a transition is requested.
	fork, _ := GenerateR3Fork(validR3ForkInput())
	fork.Disposition = "garbage"
	if errs := ValidateR3ForkForTransition(fork, false); !errsContain(errs, "unknown disposition") {
		t.Fatalf("unknown disposition must be rejected even without a transition; got %v", errs)
	}
	if errs := ValidateR3ForkForTransition(fork, true); !errsContain(errs, "unknown disposition") {
		t.Fatalf("unknown disposition must be rejected for a transition; got %v", errs)
	}
}
func TestValidateR3ForkForTransition_NoTransitionDoesNotRequireDisposition(t *testing.T) {
	// Merely HAVING a triggered fork (no transition requested) does not require
	// a recorded disposition — the gate only enforces disposition-before-
	// TRANSITION. This is "gate blocks only on structural predicates."
	fork, _ := GenerateR3Fork(validR3ForkInput())
	if errs := ValidateR3ForkForTransition(fork, false); len(errs) != 0 {
		t.Fatalf("a triggered fork with no transition requested must pass; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- coordinator-facing code cannot directly mutate lifecycle state (case 8) --

func TestValidateR3ForkForTransition_DoesNotMutateInput(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	before := fork.Disposition
	beforeSel := fork.Selection
	_ = ValidateR3ForkForTransition(fork, true)
	if fork.Disposition != before || fork.Selection != beforeSel {
		t.Fatalf("gate must not mutate its input; disposition=%q (want %q), selection=%v (want %v)",
			fork.Disposition, before, fork.Selection, beforeSel)
	}
}

func TestRecordR3Disposition_DoesNotMutatePrior(t *testing.T) {
	prior, _ := GenerateR3Fork(validR3ForkInput())
	out, err := RecordR3Disposition(prior, F1R3DispositionRejected)
	if err != nil {
		t.Fatal(err)
	}
	if prior.Disposition != F1R3DispositionPending {
		t.Fatalf("RecordR3Disposition must not mutate prior; prior.Disposition=%q (want pending)", prior.Disposition)
	}
	if out.Disposition != F1R3DispositionRejected {
		t.Fatalf("out.Disposition=%q, want rejected", out.Disposition)
	}
	if &out.Options == &prior.Options {
		t.Fatal("RecordR3Disposition must return an independent options slice")
	}
}

func TestRecordR3Disposition_UnknownDispositionFails(t *testing.T) {
	prior, _ := GenerateR3Fork(validR3ForkInput())
	if _, err := RecordR3Disposition(prior, "garbage"); err == nil {
		t.Fatal("unknown disposition must error")
	}
}

// --- gate blocks only on structural predicates (case 9) -------------------

func TestValidateR3ForkForTransition_LowQualityButDistinctRedesignPasses(t *testing.T) {
	// The gate validates COMPLETENESS, not redesign quality. A redesign that
	// is structurally distinct (different approach) but semantically poor
	// ("just delete everything") still passes the gate — quality is an
	// operator judgment (memo L193-194).
	in := validR3ForkInput()
	in.Redesign.Mechanism = "delete the entire subsystem and rewrite from scratch"
	selected, err := RecordR3Disposition(in2fork(t, in), F1R3DispositionSelected)
	if err != nil {
		t.Fatal(err)
	}
	selected.Selection = &F1R3Selection{SelectedOptionID: "opt-redesign"}
	if errs := ValidateR3ForkForTransition(selected, true); len(errs) != 0 {
		t.Fatalf("gate must not judge redesign quality; a distinct redesign passes; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- non-triggered fork must not carry options ----------------------------

func TestGenerateR3Fork_NonTriggeredWithOptionsFails(t *testing.T) {
	in := R3ForkInput{
		RepairIntent:            F1R3RepairIntentAbsent, // does not fire
		StructuralReviewOutcome: F1R3StructuralReviewNonPass,
		ContinueRepair:          &F1R3Option{OptionID: "x", Mode: F1R3ModeContinueRepair, Mechanism: "m"},
	}
	_, errs := GenerateR3Fork(in)
	if !errsContain(errs, "non-triggered fork must not carry options") {
		t.Fatalf("non-triggered fork with options must fail; got %v", errs)
	}
}

// --- producer↔validator property guard ------------------------------------

func TestGenerateR3Fork_ProducerOutputPassesCompleteness(t *testing.T) {
	// The producer's accepted output (zero errors) MUST pass the shared
	// completeness check the validator + gate use. This is the guard that
	// would have caught a producer/validator disagreement.
	cases := []struct {
		name string
		in   R3ForkInput
	}{
		{"triggered valid", validR3ForkInput()},
		{"not triggered", R3ForkInput{RepairIntent: F1R3RepairIntentAbsent, StructuralReviewOutcome: F1R3StructuralReviewNonPass}},
		{"not triggered pass-review", R3ForkInput{RepairIntent: F1R3RepairIntentPresent, StructuralReviewOutcome: F1R3StructuralReviewPass}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fork, errs := GenerateR3Fork(c.in)
			if len(errs) != 0 {
				t.Fatalf("producer must accept this input with zero errors; got %v", errs)
			}
			if cerrs := validateR3ForkCompleteness("r3", fork); len(cerrs) != 0 {
				t.Fatalf("producer output must pass validateR3ForkCompleteness; got:\n  %s", strings.Join(cerrs, "\n  "))
			}
		})
	}
}

// --- envelope validator surfaces fork incompleteness (doctor parse-site) ---

func TestValidateR3Summary_TriggeredForkMissingRedesignFails(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	fork.Options = fork.Options[:1] // drop the redesign option
	errs := validateR3Summary("entries[1]", fork)
	if !errsContain(errs, "no redesign option") {
		t.Fatalf("envelope validator must catch missing redesign; got %v", errs)
	}
}

// TestValidateR3ForkCompleteness_ExactlyOnePerMode pins the exactly-one
// contract: the fork is a binary repair-vs-redesign decision, so a second
// option of either mode is rejected (one continue_repair + one redesign).
func TestValidateR3ForkCompleteness_MultipleRedesignRejected(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	// Append a second, distinct redesign option.
	fork.Options = append(fork.Options, F1R3Option{
		OptionID: "opt-redesign-2", Mode: F1R3ModeRedesign,
		Mechanism:                "replace the entire subsystem with a different architecture",
		AffectedProperties:       []string{"R1P1"},
		SupportRefs:              []string{"R1C1"},
		CounterEvidenceProbeRefs: []string{"PA-P3"},
	})
	errs := validateR3ForkCompleteness("r3", fork)
	if !errsContain(errs, "multiple redesign options (want exactly one)") {
		t.Fatalf("a second redesign option must be rejected; got %v", errs)
	}
}

func TestValidateR3ForkCompleteness_MultipleContinueRepairRejected(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	// Append a second continue_repair option.
	fork.Options = append(fork.Options, F1R3Option{
		OptionID: "opt-continue-2", Mode: F1R3ModeContinueRepair,
		Mechanism:                "apply a different queued patch",
		SupportRefs:              []string{"R1C1"},
		CounterEvidenceProbeRefs: []string{"PA-P1"},
	})
	errs := validateR3ForkCompleteness("r3", fork)
	if !errsContain(errs, "multiple continue_repair options (want exactly one)") {
		t.Fatalf("a second continue_repair option must be rejected; got %v", errs)
	}
}

// --- helper ----------------------------------------------------------------

// in2fork builds the fork for an input (used where a fork value is needed).
func in2fork(t *testing.T, in R3ForkInput) *F1R3ForkSummary {
	t.Helper()
	fork, errs := GenerateR3Fork(in)
	if len(errs) != 0 {
		t.Fatalf("in2fork: producer rejected input: %v", errs)
	}
	return fork
}

func errsContain(errs []string, needle string) bool {
	for _, e := range errs {
		if strings.Contains(e, needle) {
			return true
		}
	}
	return false
}

// --- F-B1 regression: Selection preserved across the R1-cycle deep-copy ----

// TestWithNewR1Cycle_PreservesR3Selection pins the fix for the F-B1 data-
// integrity bug: WithNewR1Cycle must deep-copy R3 (via deepCopyR3Fork) so an
// operator's recorded Selection survives a cycle change. Without the fix,
// deepCopyR3 dropped Selection and the new envelope carried Disposition==selected
// with Selection==nil, producing a confusing "no selection record" error.
func TestWithNewR1Cycle_PreservesR3Selection(t *testing.T) {
	env := canonicalF1Fixture()
	// Put a selected disposition + Selection on the r3 entry.
	r3 := env.Entries[1].R3
	r3.Disposition = F1R3DispositionSelected
	r3.Selection = &F1R3Selection{
		SelectedOptionID:           "opt-continue",
		RedesignRejectionRationale: "redesign out of scope this cycle",
	}
	// Recompute digest + validation for the mutated envelope.
	env.SemanticDigest, _ = env.ComputeDigest()
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}

	// A fresh R1 join to drive a new cycle.
	newJoin := &F1R1JoinSummary{Conclusions: []F1R1Conclusion{
		{ConclusionID: "R1C1", PropertyID: "R1P1", JoinDisposition: F1R1JoinUnion},
	}}
	next := WithNewR1Cycle(env, "cycle-next", newJoin)

	// The new envelope's r3 entry must retain Selection.
	var nr3 *F1R3ForkSummary
	for i := range next.Entries {
		if next.Entries[i].Family == F1FamilyR3RedesignFork {
			nr3 = next.Entries[i].R3
		}
	}
	if nr3 == nil {
		t.Fatal("next envelope has no r3 entry")
	}
	if nr3.Disposition != F1R3DispositionSelected {
		t.Fatalf("next r3 disposition=%q, want selected (must survive cycle)", nr3.Disposition)
	}
	if nr3.Selection == nil {
		t.Fatal("F-B1 regression: WithNewR1Cycle dropped Selection (deepCopyR3 path); Selection must survive the cycle change")
	}
	if nr3.Selection.SelectedOptionID != "opt-continue" || nr3.Selection.RedesignRejectionRationale != "redesign out of scope this cycle" {
		t.Fatalf("Selection field values not preserved: %+v", nr3.Selection)
	}
	// Independence: mutating next's Selection must not affect prior.
	nr3.Selection.SelectedOptionID = "mutated"
	if env.Entries[1].R3.Selection.SelectedOptionID == "mutated" {
		t.Fatal("Selection must be an independent copy, not aliased to prior")
	}
}

// TestRecordR3Disposition_DeepCopiesSelection pins the deepCopyR3Fork Selection
// branch (F-E1): a fork carrying a Selection, passed through RecordR3Disposition,
// yields an independent Selection copy.
func TestRecordR3Disposition_DeepCopiesSelection(t *testing.T) {
	fork, _ := GenerateR3Fork(validR3ForkInput())
	fork.Disposition = F1R3DispositionSelected
	fork.Selection = &F1R3Selection{SelectedOptionID: "opt-redesign"}
	out, err := RecordR3Disposition(fork, F1R3DispositionRejected)
	if err != nil {
		t.Fatal(err)
	}
	if out.Selection == nil || out.Selection.SelectedOptionID != "opt-redesign" {
		t.Fatalf("Selection not preserved through RecordR3Disposition: %+v", out.Selection)
	}
	// Pointer inequality (independent copy, not aliased).
	out.Selection.SelectedOptionID = "mutated"
	if fork.Selection.SelectedOptionID == "mutated" {
		t.Fatal("out.Selection must be an independent copy, not aliased to prior.Selection")
	}
}
