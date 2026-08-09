package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestRewriteParityCrossLanguageConformance is the EXECUTABLE SHARED
// CONFORMANCE DRIVER for the rewrite-parity gate. It feeds identical contract
// inputs through all three validator mirrors — the python reference
// (rewrite-parity-validate.py), the JS mirror (rewrite-parity-validate.js), and
// the Go in-process mirror (validateRewriteParityStructureGo) — and asserts
// they classify every input identically.
//
// This is the binding that closes F3 Hazard 1 (schema-drift across the three
// implementations). Without it, a contributor could change ONE validator's
// structural rule (e.g. the python version check, the Go mode enum, the JS
// prior_surface.paths shape) without any test catching the divergence. With it,
// the three mirrors are bound to identical inputs so a one-sided rule change
// fails this test.
//
// The cases cover the structural core (the only stage all three implement):
// version edge cases (1, true, 1.0, 2, "1"), mode validity, prior_surface
// shape, behavior IDs/evidence/verifier, and inventory_complete typing. The
// precommit cross-check (Stage 1) and completion rules (Stage 2) are NOT shared
// across all three — the Go mirror is structural-only — so they are not bound
// here (precommit is exercised py↔JS in tests/scripts/; completion is JS-only).
//
// JSON does not distinguish int/float: 1 and 1.0 are both "the number 1". All
// three mirrors accept 1.0 (numerically equal to 1) and reject booleans
// (Python True==1 is special-cased; JS strict !==; Go float64 decode). The
// version:true and version:1.0 cases pin this cross-surface agreement.
func TestRewriteParityCrossLanguageConformance(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH")
	}
	repoRoot := filepath.Join("..", "..")
	pyScript := filepath.Join(repoRoot, ".opencode", "scripts", "rewrite-parity-validate.py")
	jsScript := filepath.Join(repoRoot, ".opencode", "scripts", "rewrite-parity-validate.js")
	for _, f := range []string{pyScript, jsScript} {
		if _, err := os.Stat(f); err != nil {
			t.Skipf("rendered validator unavailable: %s (run `vh-agent-harness update` first)", f)
		}
	}

	// Each case is a contract JSON string + the expected structural verdict
	// (true = accepted by all three, false = rejected by all three).
	cases := []struct {
		name   string
		json   string
		accept bool
	}{
		// --- Valid contracts (accept by all three) ---
		{
			name:   "valid deletion_replacement planned",
			json:   `{"version":1,"applies":"component X","mode":"deletion_replacement","prior_surface":{"id":"X","revision":"abc123","paths":["src/x.go"],"inventory_complete":true},"behaviors":[{"id":"b1","description":"preserves X","prior_evidence":["src/x.go:L10"],"verifier":{"kind":"go-test","locator":"go test ./src/x/..."},"result":{"status":"planned"}}]}`,
			accept: true,
		},
		{
			name:   "valid modification_only_rewrite planned",
			json:   `{"version":1,"applies":"component Y","mode":"modification_only_rewrite","prior_surface":{"id":"Y","revision":"def456","paths":["src/y.go"],"inventory_complete":false},"behaviors":[{"id":"b1","description":"preserves Y","prior_evidence":["src/y.go"],"verifier":{"kind":"go-test","locator":"go test ./src/y/..."},"result":{"status":"planned","note":"pre-commit"}}]}`,
			accept: true,
		},
		// --- version edge cases (the core cross-surface binding) ---
		{name: "version 1.0 integral float (JSON int/float equivalence)", json: `{"version":1.0,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: true},
		{name: "version true (boolean — rejected by all three)", json: `{"version":true,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "version 2 (wrong integer)", json: `{"version":2,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "version string", json: `{"version":"1","applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "version missing", json: `{"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		// --- mode ---
		{name: "bad mode", json: `{"version":1,"applies":"x","mode":"deletion","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "mode missing", json: `{"version":1,"applies":"x","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		// --- applies ---
		{name: "applies empty", json: `{"version":1,"applies":"","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		// --- prior_surface ---
		{name: "prior_surface missing", json: `{"version":1,"applies":"x","mode":"deletion_replacement","behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "prior_surface.paths empty", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":[],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "inventory_complete non-bool", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":"yes"},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "revision empty", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		// --- behaviors ---
		{name: "behaviors empty", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[]}`, accept: false},
		{name: "behavior missing verifier", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"result":{"status":"planned"}}]}`, accept: false},
		{name: "behavior duplicate ids", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}},{"id":"b","description":"d2","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"planned"}}]}`, accept: false},
		{name: "behavior bad result enum", json: `{"version":1,"applies":"x","mode":"deletion_replacement","prior_surface":{"id":"a","revision":"s","paths":["p"],"inventory_complete":false},"behaviors":[{"id":"b","description":"d","prior_evidence":["p"],"verifier":{"kind":"t","locator":"l"},"result":{"status":"maybe"}}]}`, accept: false},
		// --- malformed JSON ---
		{name: "malformed JSON", json: `{not json`, accept: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Write the contract JSON to a temp file (shared input for all three).
			tmp, err := os.CreateTemp(t.TempDir(), "contract-*.json")
			if err != nil {
				t.Fatalf("create temp: %v", err)
			}
			if _, err := tmp.WriteString(tc.json); err != nil {
				t.Fatalf("write temp: %v", err)
			}
			tmp.Close()

			pyAccept := pythonStructuralAccept(t, pyScript, tmp.Name())
			jsAccept := jsStructuralAccept(t, jsScript, tmp.Name())
			goAccept := goStructuralAccept(t, tmp.Name())

			if pyAccept != jsAccept || jsAccept != goAccept {
				t.Errorf("cross-language DISAGREEMENT on %q: py=%v js=%v go=%v (contract: %s)",
					tc.name, pyAccept, jsAccept, goAccept, tc.json)
			}
			if pyAccept != tc.accept {
				t.Errorf("expected accept=%v but got py=%v js=%v go=%v on %q (contract: %s)",
					tc.accept, pyAccept, jsAccept, goAccept, tc.name, tc.json)
			}
		})
	}
}

// pythonStructuralAccept runs the python reference validator on the contract
// file at the structural stage and returns true if it accepts (exit 0).
func pythonStructuralAccept(t *testing.T, script, contractFile string) bool {
	t.Helper()
	cmd := exec.Command("python3", script, "--contract-file", contractFile, "--stage", "structural")
	err := cmd.Run()
	return err == nil // exit 0 = valid
}

// jsStructuralAccept runs the JS mirror via node, reading the contract file,
// and returns true if validateRewriteParityStructural yields zero errors.
func jsStructuralAccept(t *testing.T, script, contractFile string) bool {
	t.Helper()
	// Resolve the script to an absolute path + file:// URL so node's dynamic
	// import works regardless of cwd. The snippet reads the contract file,
	// parses it, runs the structural validator, and exits 0/1.
	snippet := `
import { readFileSync } from "fs";
import { pathToFileURL } from "url";
const mod = await import(pathToFileURL(process.argv[1]).href);
let raw;
try { raw = readFileSync(process.argv[2], "utf8"); } catch { process.exit(1); }
let contract;
try { contract = JSON.parse(raw); } catch { process.exit(1); }
const errs = mod.validateRewriteParityStructural(contract);
process.exit(errs.length ? 1 : 0);
`
	cmd := exec.Command("node", "--input-type=module", "-e", snippet, "--", script, contractFile)
	err := cmd.Run()
	return err == nil
}

// goStructuralAccept runs the Go in-process mirror on the contract file and
// returns true if validateRewriteParityStructureGo yields zero errors.
func goStructuralAccept(t *testing.T, contractFile string) bool {
	t.Helper()
	raw, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var contract map[string]interface{}
	if err := json.Unmarshal(raw, &contract); err != nil {
		return false // malformed JSON = reject (matches py/JS)
	}
	errs := validateRewriteParityStructureGo(contract, "conformance")
	return len(errs) == 0
}
