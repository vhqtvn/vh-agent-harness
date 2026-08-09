package cli

import (
	"strings"
	"testing"
)

// TestCheckRewriteParity exercises the rewrite-parity structural-consistency
// gate across both scan surfaces. Mirrors TestCheckBehavioralClosure.
func TestCheckRewriteParity(t *testing.T) {
	validPlanned := "```rewrite-parity\n" +
		`{"version":1,"applies":"comp-x","mode":"deletion_replacement",` +
		`"prior_surface":{"id":"comp-x","revision":"abc123","paths":["src/x/a.go"],` +
		`"inventory_complete":true},"behaviors":[{"id":"x-1","description":"exports",` +
		`"prior_evidence":["src/x/a.go:L1"],"verifier":{"kind":"go-test",` +
		`"locator":"go test ./src/x/..."},"result":{"status":"planned"}}]}` + "\n```\n"

	validCompletion := "```rewrite-parity\n" +
		`{"version":1,"applies":"comp-x","mode":"deletion_replacement",` +
		`"prior_surface":{"id":"comp-x","revision":"abc123","paths":["src/x/a.go"],` +
		`"inventory_complete":true},"behaviors":[{"id":"x-1","description":"exports",` +
		`"prior_evidence":["src/x/a.go:L1"],"verifier":{"kind":"go-test",` +
		`"locator":"go test ./src/x/..."},"result":{"status":"proven",` +
		`"receipt":"HEAD def: go test -> PASS"}}]}` + "\n```\n"

	badMode := "```rewrite-parity\n" +
		`{"version":1,"applies":"comp-x","mode":"invalid_mode",` +
		`"prior_surface":{"id":"comp-x","revision":"abc123","paths":["src/x/a.go"],` +
		`"inventory_complete":true},"behaviors":[{"id":"x-1","description":"exports",` +
		`"prior_evidence":["src/x/a.go:L1"],"verifier":{"kind":"go-test",` +
		`"locator":"go test ./src/x/..."},"result":{"status":"planned"}}]}` + "\n```\n"

	badResultEnum := "```rewrite-parity\n" +
		`{"version":1,"applies":"comp-x","mode":"deletion_replacement",` +
		`"prior_surface":{"id":"comp-x","revision":"abc123","paths":["src/x/a.go"],` +
		`"inventory_complete":true},"behaviors":[{"id":"x-1","description":"exports",` +
		`"prior_evidence":["src/x/a.go:L1"],"verifier":{"kind":"go-test",` +
		`"locator":"go test ./src/x/..."},"result":{"status":"garbage"}}]}` + "\n```\n"

	malformedJSON := "```rewrite-parity\n{this is not valid json at all\n```\n"

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
			name: "report without a rewrite-parity block passes (opt-in)",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-08-09T00-00-00-closeout.md": "# Closeout\n\nDone.\n",
			},
			wantTier: tierPass,
		},
		{
			name: "checkpoint without a block passes (opt-in)",
			files: map[string]string{
				"docs/checkpoints/2026-08-09-slice.md": "# Checkpoint\n\nNotes.\n",
			},
			wantTier: tierPass,
		},
		{
			name: "valid planned contract passes",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-08-09T00-00-00-closeout.md": "# Closeout\n" + validPlanned,
			},
			wantTier: tierPass,
		},
		{
			name: "valid completion contract passes",
			files: map[string]string{
				"docs/checkpoints/2026-08-09-slice.md": "# Checkpoint\n" + validCompletion,
			},
			wantTier: tierPass,
		},
		{
			name: "malformed JSON fails",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-08-09T00-00-00-closeout.md": malformedJSON,
			},
			wantTier:  tierFail,
			wantInDet: "JSON parse error",
		},
		{
			name: "bad mode enum fails",
			files: map[string]string{
				"docs/checkpoints/2026-08-09-slice.md": badMode,
			},
			wantTier:  tierFail,
			wantInDet: "mode must be one of",
		},
		{
			name: "bad result enum fails",
			files: map[string]string{
				".local/coordinator/reports/eval-001/2026-08-09T00-00-00-closeout.md": badResultEnum,
			},
			wantTier:  tierFail,
			wantInDet: "result.status: must be one of",
		},
		{
			name: "good artifact + bad artifact in the same scan fails and names only the bad one",
			files: map[string]string{
				".local/coordinator/reports/a/2026-08-09T00-00-00-closeout.md": validPlanned,
				".local/coordinator/reports/b/2026-08-09T00-00-00-closeout.md": malformedJSON,
			},
			wantTier:  tierFail,
			wantInDet: "JSON parse error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for rel, body := range tc.files {
				writeCloseoutArtifact(t, dir, rel, body)
			}
			r := checkRewriteParity(dir)
			if r.tier != tc.wantTier {
				t.Fatalf("tier: got %s, want %s (detail: %s)", r.tier, tc.wantTier, r.detail)
			}
			if tc.wantInDet != "" && !strings.Contains(r.detail, tc.wantInDet) {
				t.Errorf("detail %q does not contain expected substring %q", r.detail, tc.wantInDet)
			}
		})
	}
}

// TestAnalyzeRewriteParityBlocksPure drives the pure parsing core directly
// (no filesystem), pinning the structural schema and the fail-closed-on-garbage
// invariant at the unit level.
func TestAnalyzeRewriteParityBlocksPure(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantReasons int
	}{
		{"no block", "plain prose\n", 0},
		{"valid minimal contract", "```rewrite-parity\n" +
			`{"version":1,"applies":"x","mode":"deletion_replacement",` +
			`"prior_surface":{"id":"x","revision":"r","paths":["p.go"],"inventory_complete":false},` +
			`"behaviors":[{"id":"b","description":"d","prior_evidence":["e"],` +
			`"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}` + "\n```\n", 0},
		{"malformed json", "```rewrite-parity\n{bad\n```\n", 1},
		{"bad version", "```rewrite-parity\n" +
			`{"version":2,"applies":"x","mode":"deletion_replacement",` +
			`"prior_surface":{"id":"x","revision":"r","paths":["p.go"],"inventory_complete":false},` +
			`"behaviors":[{"id":"b","description":"d","prior_evidence":["e"],` +
			`"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}` + "\n```\n", 1},
		{"missing verifier", "```rewrite-parity\n" +
			`{"version":1,"applies":"x","mode":"deletion_replacement",` +
			`"prior_surface":{"id":"x","revision":"r","paths":["p.go"],"inventory_complete":false},` +
			`"behaviors":[{"id":"b","description":"d","prior_evidence":["e"],` +
			`"verifier":{},"result":{"status":"planned"}}]}` + "\n```\n", 2},
		{"duplicate behavior ids", "```rewrite-parity\n" +
			`{"version":1,"applies":"x","mode":"deletion_replacement",` +
			`"prior_surface":{"id":"x","revision":"r","paths":["p.go"],"inventory_complete":false},` +
			`"behaviors":[{"id":"b","description":"d","prior_evidence":["e"],` +
			`"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}},` +
			`{"id":"b","description":"d2","prior_evidence":["e2"],` +
			`"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}` + "\n```\n", 1},
		{"two bad blocks both reported", "```rewrite-parity\n{bad1\n```\nmid\n```rewrite-parity\n{bad2\n```\n", 2},
		{"modification_only_rewrite valid", "```rewrite-parity\n" +
			`{"version":1,"applies":"x","mode":"modification_only_rewrite",` +
			`"prior_surface":{"id":"x","revision":"r","paths":["p.go"],"inventory_complete":true},` +
			`"behaviors":[{"id":"b","description":"d","prior_evidence":["e"],` +
			`"verifier":{"kind":"t","locator":"l"},"result":{"status":"proven","receipt":"HEAD x: pass"}}]}` + "\n```\n", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := analyzeRewriteParityBlocks(tc.body)
			if len(got) != tc.wantReasons {
				t.Fatalf("reasons: got %d (%v), want %d", len(got), got, tc.wantReasons)
			}
		})
	}
}
