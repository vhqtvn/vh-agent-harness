package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCloseoutArtifact writes a markdown artifact at the repo-relative path rel
// under target, creating intermediate dirs. Used to seed behavioral-closure
// scan fixtures on the two scan surfaces (.local/coordinator/reports and
// docs/checkpoints).
func writeCloseoutArtifact(t *testing.T, target, rel, body string) {
	t.Helper()
	full := filepath.Join(target, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// TestCheckBehavioralClosure exercises the behavioral-closure structural gate
// across both scan surfaces. The DoD-required negative (proven + skipped FAILs)
// and positive (inconclusive + not-demonstrable PASSes) are the "consistent" /
// "inconsistent" anchor cases.
func TestCheckBehavioralClosure(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantTier  string
		wantInDet string
	}{
		{
			name:     "no artifacts at all skips",
			files:    map[string]string{},
			wantTier: tierSkip,
		},
		{
			name: "report without a declaration passes (absent token = pass)",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-07-24T00-00-00-closeout.md": "# Closeout\n\nDone.\n",
			},
			wantTier: tierPass,
		},
		{
			name: "checkpoint without a declaration passes (absent token = pass)",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "# Checkpoint\n\nNotes.\n",
			},
			wantTier: tierPass,
		},
		{
			name: "consistent proven declaration passes",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-07-24T00-00-00-closeout.md": "# Closeout\n\n```behavioral-closure\nverdict: proven\npath: cmd/vh-agent-harness\nverifier: go test ./...\ncommand: go test ./...\nresult: proven\n```\n",
			},
			wantTier: tierPass,
		},
		{
			// Regression: the canonical declaration shipped in
			// templates/core/docs/coordination/CLOSEOUT_TEMPLATE.md annotates
			// every enum value with an inline " # ..." comment. A closeout
			// copied VERBATIM from the template MUST pass the gate — the
			// producer-facing example and the consumer validator must agree.
			// (commit-review F1/F2: previously this failed as an unknown
			// verdict because the parser did not strip inline comments.)
			name: "canonical template declaration (inline comments) passes verbatim",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closure\nverdict: proven              # proven | inconclusive | failed | abandoned\npath: <load-bearing path>    # the codepath whose execution proves the behavior\nverifier: <test/command>     # the named seam that exercises it\ncommand: <the command>       # the exact command that exercises it\nresult: proven               # proven | skipped | not-demonstrable (the crux outcome)\n```\n",
			},
			wantTier: tierPass,
		},
		{
			name: "consistent inconclusive with not-demonstrable passes",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "# Checkpoint\n\n```behavioral-closure\nverdict: inconclusive\npath: cmd/vh-agent-harness\nverifier: go test ./...\ncommand: go test ./...\nresult: not-demonstrable\n```\n",
			},
			wantTier: tierPass,
		},
		{
			name: "failed verdict with skipped result passes (only proven is gated)",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closure\nverdict: failed\npath: cmd/vh-agent-harness\nresult: skipped\n```\n",
			},
			wantTier: tierPass,
		},
		{
			name: "inconsistent proven verdict with skipped result fails",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-07-24T00-00-00-closeout.md": "# Closeout\n\n```behavioral-closure\nverdict: proven\npath: cmd/vh-agent-harness\nverifier: go test ./...\ncommand: go test ./...\nresult: skipped\n```\n",
			},
			wantTier:  tierFail,
			wantInDet: `verdict: proven but crux result is "skipped"`,
		},
		{
			name: "proven verdict with absent result fails",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closure\nverdict: proven\npath: cmd/vh-agent-harness\n```\n",
			},
			wantTier:  tierFail,
			wantInDet: "(absent)",
		},
		{
			name: "unknown verdict enum fails (garbage)",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closure\nverdict: maybe\nresult: proven\n```\n",
			},
			wantTier:  tierFail,
			wantInDet: "unknown verdict",
		},
		{
			name: "unknown result enum fails (garbage)",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closure\nverdict: inconclusive\nresult: done\n```\n",
			},
			wantTier:  tierFail,
			wantInDet: "unknown crux result",
		},
		{
			name: "declaration without verdict field fails (malformed)",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closure\npath: cmd/vh-agent-harness\nresult: proven\n```\n",
			},
			wantTier:  tierFail,
			wantInDet: "no 'verdict:' field",
		},
		{
			name: "a different info string (behavioral-closures) is not matched",
			files: map[string]string{
				"docs/checkpoints/2026-07-24-slice.md": "```behavioral-closures\nverdict: proven\nresult: skipped\n```\n",
			},
			wantTier: tierPass,
		},
		// --- M-x interaction-reachability receipt (ship-review four structural tests) ---
		{
			// Test 1: a COMPLETE interaction-reachability receipt (declared
			// interaction-touching + all six fields + outcome evidence) is
			// ACCEPTED. This is the positive case — the gate enforces the
			// receipt is present and consistent, and a complete honest receipt
			// passes (structural completeness).
			name: "M-x complete interaction-reachability receipt passes",
			files: map[string]string{
				"docs/checkpoints/2026-08-13-slice.md": "```behavioral-closure\n" +
					"verdict: proven\n" +
					"path: web/src/pane.ts\n" +
					"verifier: playwright e2e\n" +
					"command: npx playwright test pane-focus\n" +
					"result: proven\n" +
					"interaction_touching: true\n" +
					"interaction_action: user clicks the pane-0 region\n" +
					"interaction_target: focus moves to pane-0 handler\n" +
					"interaction_environment: real Chromium browser (not jsdom)\n" +
					"interaction_verifier: npx playwright test pane-focus --real-click\n" +
					"interaction_tree: a1b2c3d\n" +
					"interaction_outcome: focus moved to pane-0 and the active class applied\n" +
					"interaction_evidence: outcome\n" +
					"```\n",
			},
			wantTier: tierPass,
		},
		{
			// Test 2: a MISSING receipt is REJECTED when interaction_touching
			// is declared. The author claims interaction-touching but provides
			// no receipt fields — the gate FAILs (the receipt is required, not
			// advisory).
			name: "M-x missing receipt fails when interaction_touching declared",
			files: map[string]string{
				"docs/checkpoints/2026-08-13-slice.md": "```behavioral-closure\n" +
					"verdict: proven\n" +
					"path: web/src/pane.ts\n" +
					"result: proven\n" +
					"interaction_touching: true\n" +
					"```\n",
			},
			wantTier:  tierFail,
			wantInDet: "interaction-reachability receipt is incomplete",
		},
		{
			// Test 3: MECHANISM-only evidence (e.g. "called setActive(), it
			// returned") is classified skipped, NOT proven. The author declares
			// interaction_evidence: mechanism AND result: proven — the gate
			// FAILs because mechanism-asserting evidence cannot support proven
			// (condition 2 downgrade). The receipt fields are all present; the
			// FAILURE is the mechanism-as-proven inconsistency.
			name: "M-x mechanism-only evidence cannot be proven (downgrade to skipped)",
			files: map[string]string{
				"docs/checkpoints/2026-08-13-slice.md": "```behavioral-closure\n" +
					"verdict: proven\n" +
					"path: web/src/pane.ts\n" +
					"result: proven\n" +
					"interaction_touching: true\n" +
					"interaction_action: called handler.setActive()\n" +
					"interaction_target: focus moves to pane-0\n" +
					"interaction_environment: jsdom unit test\n" +
					"interaction_verifier: jest pane.test.ts\n" +
					"interaction_tree: a1b2c3d\n" +
					"interaction_outcome: setActive() returned without error\n" +
					"interaction_evidence: mechanism\n" +
					"```\n",
			},
			wantTier:  tierFail,
			wantInDet: "mechanism-asserting evidence",
		},
		{
			// Test 4: ORDINARY non-interaction behavioral-closure cruxes
			// retain existing behavior (no regression). A closeout with NO
			// interaction_touching field at all behaves exactly as before —
			// the interaction-reachability checks are additive and do not fire.
			name: "M-x non-interaction crux has no regression (interaction_touching absent)",
			files: map[string]string{
				"docs/checkpoints/2026-08-13-slice.md": "```behavioral-closure\n" +
					"verdict: proven\n" +
					"path: internal/cli/doctor_behavioral_closure.go\n" +
					"verifier: go test ./internal/cli/\n" +
					"command: go test ./internal/cli/ -run BehavioralClosure\n" +
					"result: proven\n" +
					"```\n",
			},
			wantTier: tierPass,
		},
		{
			// Supplemental: interaction_touching: false is an explicit
			// opt-out — no receipt required. A non-interaction closeout that
			// declares interaction_touching: false passes just like an absent
			// declaration.
			name: "M-x interaction_touching false is explicit opt-out (no receipt required)",
			files: map[string]string{
				"docs/checkpoints/2026-08-13-slice.md": "```behavioral-closure\n" +
					"verdict: proven\n" +
					"path: internal/cli/doctor.go\n" +
					"result: proven\n" +
					"interaction_touching: false\n" +
					"```\n",
			},
			wantTier: tierPass,
		},
		{
			// Supplemental: mechanism evidence with result: skipped is
			// CONSISTENT — the author honestly downgraded. This passes,
			// confirming the gate accepts an honest mechanism receipt that
			// does not claim proven.
			name: "M-x mechanism evidence with result skipped is consistent (passes)",
			files: map[string]string{
				"docs/checkpoints/2026-08-13-slice.md": "```behavioral-closure\n" +
					"verdict: inconclusive\n" +
					"path: web/src/pane.ts\n" +
					"result: skipped\n" +
					"interaction_touching: true\n" +
					"interaction_action: called handler.setActive()\n" +
					"interaction_target: focus moves to pane-0\n" +
					"interaction_environment: jsdom unit test\n" +
					"interaction_verifier: jest pane.test.ts\n" +
					"interaction_tree: a1b2c3d\n" +
					"interaction_outcome: setActive() returned without error\n" +
					"interaction_evidence: mechanism\n" +
					"```\n",
			},
			wantTier: tierPass,
		},
		{
			name: "good artifact + bad artifact in the same scan fails and names only the bad one",
			files: map[string]string{
				".local/coordinator/reports/a/2026-07-24T00-00-00-closeout.md": "```behavioral-closure\nverdict: proven\nresult: proven\n```\n",
				".local/coordinator/reports/b/2026-07-24T00-00-00-closeout.md": "```behavioral-closure\nverdict: proven\nresult: skipped\n```\n",
			},
			wantTier:  tierFail,
			wantInDet: "skipped",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, body := range tc.files {
				writeCloseoutArtifact(t, dir, rel, body)
			}
			r := checkBehavioralClosure(dir)
			if r.tier != tc.wantTier {
				t.Fatalf("tier: got %s, want %s (detail: %s)", r.tier, tc.wantTier, r.detail)
			}
			if tc.wantInDet != "" && !strings.Contains(r.detail, tc.wantInDet) {
				t.Errorf("detail %q does not contain expected substring %q", r.detail, tc.wantInDet)
			}
		})
	}
}

// TestAnalyzeBehavioralClosureBlocksPure drives the pure parsing core directly
// (no filesystem), pinning the consistency rule and the fail-closed-on-garbage
// invariant at the unit level.
func TestAnalyzeBehavioralClosureBlocksPure(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantReasons int
	}{
		{"no block", "plain prose\n", 0},
		{"proven+proven consistent", "```behavioral-closure\nverdict: proven\nresult: proven\n```\n", 0},
		{"proven+skipped inconsistent", "```behavioral-closure\nverdict: proven\nresult: skipped\n```\n", 1},
		{"two bad blocks both reported", "```behavioral-closure\nverdict: proven\nresult: skipped\n```\nmid\n```behavioral-closure\nverdict: wat\n```\n", 2},
		{"result with embedded colons parses", "```behavioral-closure\nverdict: inconclusive\ncommand: a: b: c\nresult: not-demonstrable\n```\n", 0},
		// Regression: the canonical template example (inline # comments) parses clean.
		{"canonical template example with inline comments", "```behavioral-closure\nverdict: proven              # proven | inconclusive | failed | abandoned\npath: <load-bearing path>    # the codepath whose execution proves the behavior\nverifier: <test/command>     # the named seam that exercises it\ncommand: <the command>       # the exact command that exercises it\nresult: proven               # proven | skipped | not-demonstrable (the crux outcome)\n```\n", 0},
		// M-x interaction-reachability receipt — pure parsing pins (ship-review).
		{"M-x complete receipt (outcome evidence) consistent", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: true\ninteraction_action: real click\ninteraction_target: focus\ntarget\ninteraction_environment: real browser\ninteraction_verifier: playwright\ntest\ninteraction_tree: sha123\ninteraction_outcome: focus moved\ninteraction_evidence: outcome\n```\n", 0},
		{"M-x missing receipt fields fails", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: true\n```\n", 2},
		{"M-x mechanism+proven fails (downgrade)", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: true\ninteraction_action: a\ninteraction_target: b\ninteraction_environment: c\ninteraction_verifier: d\ninteraction_tree: e\ninteraction_outcome: f\ninteraction_evidence: mechanism\n```\n", 1},
		{"M-x mechanism+skipped consistent", "```behavioral-closure\nverdict: inconclusive\nresult: skipped\ninteraction_touching: true\ninteraction_action: a\ninteraction_target: b\ninteraction_environment: c\ninteraction_verifier: d\ninteraction_tree: e\ninteraction_outcome: f\ninteraction_evidence: mechanism\n```\n", 0},
		{"M-x non-interaction absent touching no regression", "```behavioral-closure\nverdict: proven\nresult: proven\n```\n", 0},
		{"M-x touching false explicit opt-out", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: false\n```\n", 0},
		{"M-x garbage touching value fails", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: maybe\n```\n", 1},
		{"M-x missing evidence field fails", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: true\ninteraction_action: a\ninteraction_target: b\ninteraction_environment: c\ninteraction_verifier: d\ninteraction_tree: e\ninteraction_outcome: f\n```\n", 1},
		{"M-x garbage evidence value fails", "```behavioral-closure\nverdict: proven\nresult: proven\ninteraction_touching: true\ninteraction_action: a\ninteraction_target: b\ninteraction_environment: c\ninteraction_verifier: d\ninteraction_tree: e\ninteraction_outcome: f\ninteraction_evidence: maybe\n```\n", 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := analyzeBehavioralClosureBlocks(tc.body)
			if len(got) != tc.wantReasons {
				t.Fatalf("reasons: got %d (%v), want %d", len(got), got, tc.wantReasons)
			}
		})
	}
}

// TestStripInlineComment pins the " # ..." comment convention so a value
// annotated inline (as the shipped template does) still reduces to its bare
// enum, while a '#' not preceded by whitespace is preserved.
func TestStripInlineComment(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"proven", "proven"},
		{"proven              # proven | inconclusive | failed | abandoned", "proven"},
		{"not-demonstrable               # proven | skipped | not-demonstrable (the crux outcome)", "not-demonstrable"},
		{"src/pkg#tag", "src/pkg#tag"}, // '#' not preceded by whitespace -> preserved
		{"https://h.test/x#frag", "https://h.test/x#frag"},
		{"  spaced  ", "spaced"},           // plain trim still applies
		{"a b\t# trailing comment", "a b"}, // tab before '#' is also a comment
		{"#onlycomment", "#onlycomment"},   // value starting with '#' is preserved (i==0 fallthrough)
		{"", ""},
	}
	for _, tc := range tests {
		if got := stripInlineComment(tc.in); got != tc.want {
			t.Errorf("stripInlineComment(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
