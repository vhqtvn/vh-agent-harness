package ownership

import (
	"errors"
	"strings"
	"testing"
)

// TestAllClasses_CanonicalOrder confirms AllClasses lists the six armed classes
// in the documented canonical order (lattice-first, off-lattice-last).
func TestAllClasses_CanonicalOrder(t *testing.T) {
	got := AllClasses()
	want := []Class{
		ClassPlatformManaged,
		ClassPlatformArmed,
		ClassOverlayExtension,
		ClassProjectOwned,
		ClassExternalGenerated,
		ClassLocalOnly,
	}
	if len(got) != len(want) {
		t.Fatalf("AllClasses length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, c := range got {
		if c != want[i] {
			t.Errorf("AllClasses[%d] = %q, want %q", i, c, want[i])
		}
	}
}

// TestClass_IsValid covers the valid set and rejects garbage.
func TestClass_IsValid(t *testing.T) {
	for _, c := range AllClasses() {
		if !c.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", c)
		}
	}
	for _, bad := range []string{"", "managed", "platform", "PLATFORM_MANAGED", "project-owned", "weird"} {
		if Class(bad).IsValid() {
			t.Errorf("%q.IsValid() = true, want false", bad)
		}
	}
}

// TestClass_IsHandOverridable confirms the on-lattice vs off-lattice split.
func TestClass_IsHandOverridable(t *testing.T) {
	onLattice := []Class{ClassPlatformManaged, ClassPlatformArmed, ClassOverlayExtension, ClassProjectOwned}
	offLattice := []Class{ClassExternalGenerated, ClassLocalOnly}
	for _, c := range onLattice {
		if !c.IsHandOverridable() {
			t.Errorf("%q.IsHandOverridable() = false, want true (on-lattice)", c)
		}
	}
	for _, c := range offLattice {
		if c.IsHandOverridable() {
			t.Errorf("%q.IsHandOverridable() = true, want false (off-lattice)", c)
		}
	}
}

// TestParseClass validates the input-validation chokepoint.
func TestParseClass(t *testing.T) {
	c, err := ParseClass("project_owned")
	if err != nil || c != ClassProjectOwned {
		t.Fatalf("ParseClass(project_owned) = %q, %v; want project_owned, nil", c, err)
	}
	_, err = ParseClass("nope")
	if err == nil {
		t.Fatal("ParseClass(nope) should error")
	}
	// Invalid literal must be detectable as InvalidClassError.
	var ice *InvalidClassError
	if !errors.As(err, &ice) {
		t.Fatalf("ParseClass(nope) err must be *InvalidClassError; got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should name the bad literal; got: %v", err)
	}
}

// TestCompare_OnLattice covers every on-lattice pair direction and is the
// executable form of the raise/reject transition table.
func TestCompare_OnLattice(t *testing.T) {
	onLattice := []Class{ClassPlatformManaged, ClassPlatformArmed, ClassOverlayExtension, ClassProjectOwned}
	for _, from := range onLattice {
		for _, to := range onLattice {
			d, err := Compare(from, to)
			if err != nil {
				t.Errorf("Compare(%s,%s) unexpected err: %v", from, to, err)
				continue
			}
			rf, rt := rankOf(from), rankOf(to)
			want := DecisionEqual
			switch {
			case rt > rf:
				want = DecisionRaise
			case rt < rf:
				want = DecisionLower
			}
			if d != want {
				t.Errorf("Compare(%s,%s) = %s, want %s", from, to, d, want)
			}
		}
	}
}

// rankOf is a test-local mirror of protectionRank for expectation computation.
func rankOf(c Class) int { r, _ := rank(c); return r }

// TestCompare_OffLattice confirms Compare fails closed whenever either operand
// is off-lattice (external_generated / local_only), never silently ranking.
func TestCompare_OffLattice(t *testing.T) {
	offLattice := []Class{ClassExternalGenerated, ClassLocalOnly}
	onLattice := []Class{ClassPlatformManaged, ClassPlatformArmed, ClassOverlayExtension, ClassProjectOwned}
	// off -> on, on -> off, off -> off must all error (NotHandOverridableError).
	for _, from := range offLattice {
		for _, to := range onLattice {
			if _, err := Compare(from, to); err == nil {
				t.Errorf("Compare(%s,%s): expected NotHandOverridableError, got nil", from, to)
			}
		}
	}
	for _, from := range onLattice {
		for _, to := range offLattice {
			if _, err := Compare(from, to); err == nil {
				t.Errorf("Compare(%s,%s): expected NotHandOverridableError, got nil", from, to)
			}
		}
	}
	for _, from := range offLattice {
		for _, to := range offLattice {
			if _, err := Compare(from, to); err == nil {
				t.Errorf("Compare(%s,%s): expected NotHandOverridableError, got nil", from, to)
			}
		}
	}
}

// TestCompare_InvalidOperand returns InvalidClassError for garbage operands.
func TestCompare_InvalidOperand(t *testing.T) {
	if _, err := Compare(Class("nope"), ClassProjectOwned); err == nil {
		t.Fatal("Compare(invalid, ...) should error")
	}
	if _, err := Compare(ClassPlatformManaged, Class("nope")); err == nil {
		t.Fatal("Compare(..., invalid) should error")
	}
}

// TestCompare_BriefRaiseRejectTable is the explicit acceptance gate from the
// Slice 4 brief: the named accepted (raise) and rejected (downgrade) examples.
func TestCompare_BriefRaiseRejectTable(t *testing.T) {
	t.Run("accepted_raises", func(t *testing.T) {
		cases := []struct{ from, to Class }{
			{ClassPlatformManaged, ClassPlatformArmed}, // accepted (raise)
			{ClassPlatformManaged, ClassProjectOwned},  // accepted (raise)
			{ClassOverlayExtension, ClassProjectOwned}, // accepted (raise)
		}
		for _, c := range cases {
			d, err := Compare(c.from, c.to)
			if err != nil {
				t.Errorf("Compare(%s,%s) err = %v, want nil", c.from, c.to, err)
				continue
			}
			if d != DecisionRaise {
				t.Errorf("Compare(%s,%s) = %s, want raise", c.from, c.to, d)
			}
		}
	})
	t.Run("rejected_downgrades", func(t *testing.T) {
		cases := []struct{ from, to Class }{
			{ClassProjectOwned, ClassPlatformManaged},  // rejected (downgrade)
			{ClassPlatformArmed, ClassPlatformManaged}, // rejected (downgrade)
		}
		for _, c := range cases {
			d, err := Compare(c.from, c.to)
			if err != nil {
				t.Errorf("Compare(%s,%s) err = %v, want nil (decision=lower, not an error)", c.from, c.to, err)
				continue
			}
			if d != DecisionLower {
				t.Errorf("Compare(%s,%s) = %s, want lower", c.from, c.to, d)
			}
		}
	})
}

// TestIsMutableByGenericRender is the Slice-4 "project_owned is never plain-mutable"
// guard. Only platform_managed may be touched by an ungated platform render.
func TestIsMutableByGenericRender(t *testing.T) {
	cases := map[Class]bool{
		ClassPlatformManaged:   true,
		ClassPlatformArmed:     false, // armed path only, not a plain render
		ClassOverlayExtension:  false, // merge-only, never overwrite
		ClassProjectOwned:      false, // NEVER touched by platform update
		ClassExternalGenerated: false, // provider-version-pin, drift-checked
		ClassLocalOnly:         false, // not on the platform update path
	}
	for c, want := range cases {
		if got := IsMutableByGenericRender(c); got != want {
			t.Errorf("IsMutableByGenericRender(%s) = %v, want %v", c, got, want)
		}
	}
}

// TestIsOverwritableBySeamApply pins the wholesale-overwrite class set for a seam
// apply: exactly platform_managed and overlay_extension. This is the same set the
// seam apply switch (internal/substrate/apply.go planOutcome) routes to its
// managed-* outcomes. The switch is the LIVE authority and does NOT call this
// predicate. This test pins the PREDICATE side: changing the predicate's
// return-set without reconciling it with the switch fails here. The switch side
// is not mechanically coupled to this test — its routing is covered by the
// substrate apply tests plus review; coupling the two (shared fixture, apply-side
// assertion, or wiring the predicate) is a tracked follow-up when planOutcome's
// class routing is next changed.
func TestIsOverwritableBySeamApply(t *testing.T) {
	cases := map[Class]bool{
		ClassPlatformManaged:   true,  // generic force-overwrite
		ClassPlatformArmed:     false, // reconcile/proposal path, never wholesale overwrite
		ClassOverlayExtension:  true,  // overlay-system overwrite when the pack is active
		ClassProjectOwned:      false, // preserved when present, seeded once when absent
		ClassExternalGenerated: false, // provider/project-owned, preserved/seeded
		ClassLocalOnly:         false, // not on the platform update path
	}
	for c, want := range cases {
		if got := IsOverwritableBySeamApply(c); got != want {
			t.Errorf("IsOverwritableBySeamApply(%s) = %v, want %v", c, got, want)
		}
	}
}

// TestGenericMutableIsSubsetOfSeamOverwritable pins the DISAMBIGUATION between
// the two exported overwrite/mutability predicates and the structural
// relationship that makes them confusable (audit finding F2): they are NOT
// synonyms, and one is a strict subset of the other.
//
//   - IsMutableByGenericRender guards the GENERIC (overlay-unaware) render/force-
//     overwrite: true for {platform_managed} only.
//   - IsOverwritableBySeamApply guards the SEAM-APPLY wholesale overwrite: true
//     for {platform_managed, overlay_extension}.
//
// The subset invariant IsMutableByGenericRender(c) => IsOverwritableBySeamApply(c)
// holds for every class: everything a generic ungated render may overwrite, a seam
// apply may also overwrite — but the seam apply additionally overwrites
// overlay_extension (by overlay-system authority). This was previously an
// unenforced, assumption-only invariant (audit F2 "Related C3"); pinning it here
// means a future edit to either predicate that breaks the relationship fails the
// test rather than silently recreating the confusable-pair hazard. It does NOT
// change either predicate's truth table.
func TestGenericMutableIsSubsetOfSeamOverwritable(t *testing.T) {
	for _, c := range AllClasses() {
		mutable := IsMutableByGenericRender(c)
		overwritable := IsOverwritableBySeamApply(c)
		if mutable && !overwritable {
			t.Errorf("disambiguation violation: IsMutableByGenericRender(%s)=true but IsOverwritableBySeamApply(%s)=false (generic-render-mutable must be a subset of seam-apply-overwritable)", c, c)
		}
	}
	// Sanity: the two predicates are genuinely distinct (the subset is strict),
	// so the rename exists for a reason — they are not interchangeable.
	if !IsOverwritableBySeamApply(ClassOverlayExtension) {
		t.Errorf("IsOverwritableBySeamApply(overlay_extension) must be true; the predicates would be identical otherwise")
	}
	if IsMutableByGenericRender(ClassOverlayExtension) {
		t.Errorf("IsMutableByGenericRender(overlay_extension) must be false; the predicates would be identical otherwise")
	}
}
