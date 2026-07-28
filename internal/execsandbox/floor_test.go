package execsandbox

import "testing"

// TestApplyFloor locks the load-bearing containment property: the effective
// mode is the stricter of requested and floor, and a caller can NEVER run below
// the floor. This is the unit the binary-side clamp relies on; the CLI test in
// the cli package covers the cobra flag-resolution + clamp integration.
func TestApplyFloor(t *testing.T) {
	cases := []struct {
		name      string
		requested SandboxMode
		floor     SandboxMode
		want      SandboxMode
	}{
		// No floor (ModeOff) — caller's requested mode is honored exactly.
		{"no floor: off honored", ModeOff, ModeOff, ModeOff},
		{"no floor: best-effort honored", ModeBestEffort, ModeOff, ModeBestEffort},
		{"no floor: strict honored", ModeStrict, ModeOff, ModeStrict},

		// best-effort floor — cannot go below best-effort.
		{"best-effort floor: off upgraded", ModeOff, ModeBestEffort, ModeBestEffort},
		{"best-effort floor: best-effort kept", ModeBestEffort, ModeBestEffort, ModeBestEffort},
		{"best-effort floor: strict kept (caller asked stricter)", ModeStrict, ModeBestEffort, ModeStrict},

		// strict floor — the crux case: off and best-effort are BOTH upgraded to
		// strict. The caller can never run below strict.
		{"strict floor: off upgraded (P5 bypass denied)", ModeOff, ModeStrict, ModeStrict},
		{"strict floor: best-effort upgraded", ModeBestEffort, ModeStrict, ModeStrict},
		{"strict floor: strict kept", ModeStrict, ModeStrict, ModeStrict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyFloor(tc.requested, tc.floor)
			if got != tc.want {
				t.Errorf("ApplyFloor(requested=%q, floor=%q) = %q, want %q", tc.requested, tc.floor, got, tc.want)
			}
		})
	}
}

// TestParseMinMode locks the fail-closed boundary for the floor value. Empty
// (key-absent) and the three documented modes map cleanly; any other value is
// an error so a typo cannot silently disable the floor.
func TestParseMinMode(t *testing.T) {
	cases := []struct {
		name    string
		val     string
		want    SandboxMode
		wantErr bool
	}{
		{"empty → off (no floor, key-absent)", "", ModeOff, false},
		{"off", "off", ModeOff, false},
		{"best-effort", "best-effort", ModeBestEffort, false},
		{"strict", "strict", ModeStrict, false},
		// Fail-closed: typos/unknowns are errors, never a silent off.
		{"typo 'strcit' → error", "strcit", "", true},
		{"uppercase 'Strict' → error (case-sensitive)", "Strict", "", true},
		{"garbage 'maybe' → error", "maybe", "", true},
		{"numeric '2' → error", "2", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseMinMode(tc.val)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseMinMode(%q) returned nil error, want non-nil (fail-closed)", tc.val)
				}
				if got != "" {
					t.Errorf("ParseMinMode(%q) error case returned non-empty mode %q; want empty to avoid misuse", tc.val, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMinMode(%q) returned unexpected error: %v", tc.val, err)
			}
			if got != tc.want {
				t.Errorf("ParseMinMode(%q) = %q, want %q", tc.val, got, tc.want)
			}
		})
	}
}
