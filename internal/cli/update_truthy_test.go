package cli

import "testing"

// Tests for truthyString (internal/cli/update.go) — the canonical env-value
// truthiness test reused across the CLI's env-driven bypass switches (e.g.
// RUN_FROM_AGENT). This boundary table locks the exact accepted set against
// future drift so a refactor cannot silently narrow or widen it.
//
// truthyString lowercases the whitespace-trimmed input and matches one of
// {"1","true","yes","on"}; everything else (including "", "0", and the
// "y" that defaultUpdateConfirm accepts — defaultUpdateConfirm compares
// only against "y" case-insensitively, never "t") is false. This mirrors
// the contract in the function's doc comment exactly.
//
// This does NOT duplicate the RUN_FROM_AGENT="1" integration test in
// update_guard_test.go — that one exercises the guard's env bypass end to end;
// this targets the truthyString unit directly, independent of the other seams.
func TestTruthyString_Boundary(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// --- canonical truthy set (lowercase) ---
		{name: "one", in: "1", want: true},
		{name: "true_lc", in: "true", want: true},
		{name: "yes_lc", in: "yes", want: true},
		{name: "on_lc", in: "on", want: true},

		// --- case-insensitivity (upper + mixed case) ---
		{name: "true_uc", in: "TRUE", want: true},
		{name: "true_tc", in: "True", want: true},
		{name: "true_mixed", in: "TrUe", want: true},
		{name: "yes_uc", in: "YES", want: true},
		{name: "yes_mixed", in: "yEs", want: true},
		{name: "on_uc", in: "ON", want: true},

		// --- whitespace trimming (spaces, tabs, newlines, CR) ---
		{name: "yes_padded_spaces", in: "  yes  ", want: true},
		{name: "on_tab_newline", in: "\ton\n", want: true},
		{name: "true_crlf_padded", in: "  TRUE\r\n", want: true},

		// --- falsy: empty / whitespace-only / zero ---
		{name: "empty", in: "", want: false},
		{name: "single_space", in: " ", want: false},
		{name: "only_spaces", in: "   ", want: false},
		{name: "only_tab", in: "\t", want: false},
		{name: "zero", in: "0", want: false},

		// --- falsy: explicit-negation words are NOT honored (no opt-out set) ---
		{name: "no", in: "no", want: false},
		{name: "off", in: "off", want: false},
		{name: "false", in: "false", want: false},
		{name: "NO_uc", in: "NO", want: false},
		{name: "FALSE_uc", in: "FALSE", want: false},

		// --- falsy: garbage, prefixes, and abbreviations ---
		{name: "nope", in: "nope", want: false},
		{name: "banana", in: "banana", want: false},
		{name: "y_abbrev", in: "y", want: false},
		{name: "Y_abbrev", in: "Y", want: false},
		{name: "t_abbrev", in: "t", want: false},
		{name: "two", in: "2", want: false},
		{name: "trueish", in: "trueish", want: false},
		{name: "yes_with_arg", in: "yes please", want: false},
		{name: "leading_junk", in: "x1", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truthyString(tc.in); got != tc.want {
				t.Errorf("truthyString(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
