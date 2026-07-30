package substrate

import (
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// TestPlanOutcome_ManagedOverwriteRoutingMatchesPredicate is the F1 coupling
// guard (contract-invariant-audit pilot run, finding F1, class C5).
//
// The seam apply path declares the wholesale-overwrite class-set TWICE:
//
//   - the LIVE per-class switch in planOutcome (internal/substrate/apply.go),
//     which routes classes to its managed-* outcomes; and
//   - the ownership.IsOverwritableBySeamApply predicate
//     (internal/ownership/ordering.go), which the switch does NOT call — it
//     documents the same class-set it routes.
//
// The two authorities AGREE today, but before this test that agreement was
// review-only on the switch side: ownership.TestIsOverwritableBySeamApply pins
// the PREDICATE (a change to the predicate's truth-set without reconciling the
// switch fails that test), but a change to the planOutcome switch — e.g. routing
// a THIRD class to ActionManagedOverwrite/ActionManagedNoop — would NOT fail it.
// Switch-side drift was covered only by substrate apply tests plus review.
//
// This test mechanically couples the two: across ALL six armed classes it
// asserts that planOutcome reaches a managed-overwrite action
// (ActionManagedOverwrite or ActionManagedNoop) IF AND ONLY IF
// ownership.IsOverwritableBySeamApply(class) is true. A divergence between the
// switch routing and the predicate class-set now fails this test instead of
// silently recreating the dual-authority drift hazard.
//
// This is a static-coupling property: it proves the two declarations agree, not
// that a live write executed (no live tree is mutated here). The overwritable
// classes carry DRIFTED live content so managedUpToDate is false and the routing
// is the load-bearing ActionManagedOverwrite rather than the up-to-date noop
// (both count as "reached the managed-overwrite route"; drift makes the exercise
// realistic). platform_armed reaches planArmed, which has no registered schema
// for the synthetic test path and so errors — that is still "not a
// managed-overwrite action", consistent with IsOverwritableBySeamApply being
// false for platform_armed.
func TestPlanOutcome_ManagedOverwriteRoutingMatchesPredicate(t *testing.T) {
	live := t.TempDir()
	staging := t.TempDir()

	// One synthetic path per class, classified via exact-path platform defaults.
	// Resolve builds the effective map so the classifier sees real S2 entries.
	defaults := ownership.ModuleDefaults{}
	for _, c := range ownership.AllClasses() {
		rel := couplingRel(c)
		defaults[rel] = ownership.PathRule{Class: c, Provenance: "core.coupling"}
		// Staged content for every class (read by the overwrite/armed routes).
		writeFile(t, staging, rel, "STAGED "+string(c))
	}
	eff, err := ownership.Resolve(defaults, nil)
	if err != nil {
		t.Fatalf("ownership.Resolve: %v", err)
	}
	opts := ApplyOptions{
		ProjectRoot: live,
		StagingDir:  staging,
		Classifier:  NewClassifier(eff, nil),
	}

	for _, c := range ownership.AllClasses() {
		rel := couplingRel(c)
		// Make the overwritable classes' live content DIFFER from staged so
		// managedUpToDate is false and the routing is ActionManagedOverwrite
		// (the overwrite route), not the up-to-date noop.
		if ownership.IsOverwritableBySeamApply(c) {
			writeFile(t, live, rel, "LIVE DIFFERENT "+string(c))
		}
		outcome, pErr := planOutcome(opts, rel)
		reachedManagedOverwrite := pErr == nil &&
			(outcome.Action == ActionManagedOverwrite || outcome.Action == ActionManagedNoop)
		if reachedManagedOverwrite != ownership.IsOverwritableBySeamApply(c) {
			t.Errorf("coupling divergence for class %q: planOutcome reached a managed-"+
				"overwrite action (action=%q, err=%v) but IsOverwritableBySeamApply=%v; "+
				"the planOutcome switch and ownership.IsOverwritableBySeamApply must route "+
				"the same class-set to the managed-overwrite outcomes (audit F1)",
				c, outcome.Action, pErr, ownership.IsOverwritableBySeamApply(c))
		}
	}
}

// couplingRel maps a class to a unique synthetic repo-relative path for the F1
// coupling test. The path is deliberately not a schema'd filename, so
// platform_armed reaches planArmed's "no registered schema" error branch
// (acceptable: that is not a managed-overwrite action).
func couplingRel(c ownership.Class) string {
	return "coupling/" + string(c) + ".txt"
}
