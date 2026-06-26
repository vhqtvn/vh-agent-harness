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

// TestIsMutableByPlatform is the Slice-4 "project_owned is never plain-mutable"
// guard. Only platform_managed may be touched by an ungated platform render.
func TestIsMutableByPlatform(t *testing.T) {
	cases := map[Class]bool{
		ClassPlatformManaged:   true,
		ClassPlatformArmed:     false, // armed path only, not a plain render
		ClassOverlayExtension:  false, // merge-only, never overwrite
		ClassProjectOwned:      false, // NEVER touched by platform update
		ClassExternalGenerated: false, // provider-version-pin, drift-checked
		ClassLocalOnly:         false, // not on the platform update path
	}
	for c, want := range cases {
		if got := IsMutableByPlatform(c); got != want {
			t.Errorf("IsMutableByPlatform(%s) = %v, want %v", c, got, want)
		}
	}
}
