package cli

// doctor_rewrite_parity.go — the REWRITE-PARITY structural-consistency gate
// (the 25th doctor check).
//
// This is the SAFETY LAYER acting as an INDEPENDENT AUDIT (defense-in-depth).
// It scans the same durable closeout/checkpoint surfaces as the behavioral-
// closure check for ```rewrite-parity fenced contract blocks and validates
// their STRUCTURAL consistency — schema, enums, prior-surface shape, behavior
// shape. It is NOT the sole authority: the commit-gate (Stage 1) and the
// closeout transition (Stage 2) are the primary enforcement surfaces. doctor
// catches contracts that landed in committed artifacts via paths that bypassed
// those gates (manual writes, stale checkpoints, escape-hatch commits).
//
// THE GATE IS STRUCTURAL (mirrors the "honesty ceiling" of behavioral-closure
// and F3): it can verify the contract is well-formed JSON in the known schema
// with valid enums and the required fields present. It CANNOT verify the
// design is good, the cited evidence is truthful, or a structurally-complete
// contract is substantively correct. A PASS says "the contract is structurally
// well-formed" — NEVER "the rewrite achieved parity."
//
// WHY DOCTOR (and not solely the commit-gate / closeout): the commit-gate
// fires only when --rewrite-parity-contract is passed; the closeout transition
// fires only for status=completed closeouts. doctor scans EVERY committed
// artifact independently, so a contract that landed in a checkpoint or a
// manually-written report is still audited. This is the same defense-in-depth
// rationale as the behavioral-closure doctor check.
//
// THE GATE (structural audit):
//   - No ```rewrite-parity block in any scanned artifact => PASS (the pilot
//     does NOT require every closeout to carry one; the contract is opt-in.)
//   - Block present + structurally valid                     => PASS
//   - Block present + malformed JSON / bad schema / bad enum => FAIL
//     (fail-closed on garbage, mirroring behavioral-closure)
//
// SCAN SURFACES (same as behavioral-closure):
//   - .local/coordinator/reports/**/*.md  (local closeout reports; transport)
//   - docs/checkpoints/*.md               (committed durable closeouts)

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// rewriteParityRe matches a fenced code block whose info string begins with
// "rewrite-parity" and captures the block body. Same pattern shape as
// behavioralClosureRe (double-quoted because the pattern contains literal
// backticks).
var rewriteParityRe = regexp.MustCompile("(?s)```rewrite-parity[ \\t]*\\n(.*?)\\n```")

// rewriteParityValidModes is the closed enum for the contract mode.
var rewriteParityValidModes = map[string]bool{
	"deletion_replacement":      true,
	"modification_only_rewrite": true,
}

// rewriteParityValidResults is the closed enum for behavior result.status.
var rewriteParityValidResults = map[string]bool{
	"planned":          true,
	"proven":           true,
	"failed":           true,
	"skipped":          true,
	"not-demonstrable": true,
}

// checkRewriteParity is the 25th doctor check. See the file-level comment for
// the full gate contract. It is READ-ONLY: it never mutates an artifact and
// never shells out.
func checkRewriteParity(target string) checkResult {
	const name = "rewrite-parity"

	surfaces := []string{
		filepath.Join(".local", "coordinator", "reports"),
		filepath.Join("docs", "checkpoints"),
	}

	type finding struct{ source, reason string }
	var findings []finding
	mdFiles := 0

	for _, surf := range surfaces {
		root := filepath.Join(target, surf)
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			mdFiles++
			rel, relErr := filepath.Rel(target, path)
			if relErr != nil {
				rel = path
			}
			data, err := os.ReadFile(path)
			if err != nil {
				findings = append(findings, finding{rel, "unreadable artifact: " + err.Error()})
				return nil
			}
			for _, reason := range analyzeRewriteParityBlocks(string(data)) {
				findings = append(findings, finding{rel, reason})
			}
			return nil
		})
	}

	if mdFiles == 0 {
		return checkResult{name: name, tier: tierSkip,
			detail: "no closeout/checkpoint artifacts to scan (no rewrite-parity contracts yet)"}
	}
	if len(findings) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d artifact(s) scanned; every rewrite-parity contract is structurally well-formed", mdFiles)}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].source < findings[j].source })
	var b strings.Builder
	fmt.Fprintf(&b, "%d malformed/inconsistent rewrite-parity contract(s):", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "\n  - %s: %s", f.source, f.reason)
	}
	b.WriteString("\nA rewrite-parity contract is a fenced ```rewrite-parity block carrying versioned JSON (version, applies, mode, prior_surface, behaviors). Fix the schema, or remove the block if it does not apply. (The audit checks structural consistency; it does not prove parity.)")
	return checkResult{name: name, tier: tierFail, detail: b.String()}
}

// analyzeRewriteParityBlocks parses every ```rewrite-parity fenced block in
// body and returns a reason string for each block that is malformed or
// structurally inconsistent. A well-formed block contributes no reason. This
// is the pure parsing core, separated so tests can drive it without touching
// the filesystem. It mirrors the structural core of the python/JS validators
// (rewrite-parity-validate.py / .js); the three aim for structural-rule-
// equivalence against one frozen v1 schema. This Go mirror covers the rules
// via inline test cases; only the JS mirror is pinned to all 9 golden fixtures
// (cross-language fixture-driver parity is a tracked follow-up).
func analyzeRewriteParityBlocks(body string) []string {
	blocks := rewriteParityRe.FindAllStringSubmatch(body, -1)
	var reasons []string
	for blockIdx, m := range blocks {
		pfx := fmt.Sprintf("rewrite-parity block #%d", blockIdx+1)
		var contract map[string]interface{}
		if err := json.Unmarshal([]byte(m[1]), &contract); err != nil {
			reasons = append(reasons, fmt.Sprintf("%s: JSON parse error: %v", pfx, err))
			continue
		}
		reasons = append(reasons, validateRewriteParityStructureGo(contract, pfx)...)
	}
	return reasons
}

// validateRewriteParityStructureGo is the Go mirror of the structural core
// (validateRewriteParityStructural in the JS validator, validate_structural in
// the python reference). It validates schema, enums, and field shapes. It does
// NOT validate stage-specific rules (cross-check, completion) — those belong to
// the commit-gate and closeout transition respectively.
func validateRewriteParityStructureGo(contract map[string]interface{}, pfx string) []string {
	var errs []string

	if v, ok := contract["version"]; !ok || v != float64(1) {
		errs = append(errs, fmt.Sprintf("%s: version must be 1 (got %v)", pfx, contract["version"]))
	}
	if !neStrGo(contract["applies"]) {
		errs = append(errs, fmt.Sprintf("%s: applies must be a non-empty string", pfx))
	}
	mode, _ := contract["mode"].(string)
	if !rewriteParityValidModes[mode] {
		errs = append(errs, fmt.Sprintf("%s: mode must be one of [deletion_replacement modification_only_rewrite] (got %v)", pfx, contract["mode"]))
	}

	ps, ok := contract["prior_surface"].(map[string]interface{})
	if !ok {
		errs = append(errs, fmt.Sprintf("%s: prior_surface must be an object", pfx))
	} else {
		if !neStrGo(ps["id"]) {
			errs = append(errs, fmt.Sprintf("%s: prior_surface.id must be a non-empty string", pfx))
		}
		if !neStrGo(ps["revision"]) {
			errs = append(errs, fmt.Sprintf("%s: prior_surface.revision must be a non-empty string", pfx))
		}
		paths, _ := ps["paths"].([]interface{})
		if len(paths) == 0 {
			errs = append(errs, fmt.Sprintf("%s: prior_surface.paths must be a non-empty array", pfx))
		} else {
			for i, p := range paths {
				if !neStrGo(p) {
					errs = append(errs, fmt.Sprintf("%s: prior_surface.paths[%d] must be a non-empty string", pfx, i))
				}
			}
		}
		if _, ok := ps["inventory_complete"].(bool); !ok {
			errs = append(errs, fmt.Sprintf("%s: prior_surface.inventory_complete must be a boolean", pfx))
		}
	}

	behaviors, _ := contract["behaviors"].([]interface{})
	if len(behaviors) == 0 {
		errs = append(errs, fmt.Sprintf("%s: behaviors must be a non-empty array", pfx))
	}
	seen := make(map[string]bool)
	for i, bRaw := range behaviors {
		bpfx := fmt.Sprintf("%s.behaviors[%d]", pfx, i)
		b, ok := bRaw.(map[string]interface{})
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: must be an object", bpfx))
			continue
		}
		bid, _ := b["id"].(string)
		if !neStrGo(bid) {
			errs = append(errs, fmt.Sprintf("%s.id: must be a non-empty string", bpfx))
		} else if seen[bid] {
			errs = append(errs, fmt.Sprintf("%s.id: %q is duplicated within this contract", bpfx, bid))
		} else {
			seen[bid] = true
		}
		if !neStrGo(b["description"]) {
			errs = append(errs, fmt.Sprintf("%s.description: must be a non-empty string", bpfx))
		}
		pe, _ := b["prior_evidence"].([]interface{})
		if len(pe) == 0 {
			errs = append(errs, fmt.Sprintf("%s.prior_evidence: must be a non-empty array", bpfx))
		} else {
			for j, e := range pe {
				if !neStrGo(e) {
					errs = append(errs, fmt.Sprintf("%s.prior_evidence[%d]: must be a non-empty string", bpfx, j))
				}
			}
		}
		ver, ok := b["verifier"].(map[string]interface{})
		if !ok {
			errs = append(errs, fmt.Sprintf("%s.verifier: must be an object", bpfx))
		} else {
			if !neStrGo(ver["kind"]) {
				errs = append(errs, fmt.Sprintf("%s.verifier.kind: must be a non-empty string", bpfx))
			}
			if !neStrGo(ver["locator"]) {
				errs = append(errs, fmt.Sprintf("%s.verifier.locator: must be a non-empty string", bpfx))
			}
		}
		res, ok := b["result"].(map[string]interface{})
		if !ok {
			errs = append(errs, fmt.Sprintf("%s.result: must be an object", bpfx))
		} else {
			rStatus, _ := res["status"].(string)
			if !rewriteParityValidResults[rStatus] {
				errs = append(errs, fmt.Sprintf("%s.result.status: must be one of [planned proven failed skipped not-demonstrable] (got %v)", bpfx, res["status"]))
			}
			for _, k := range []string{"receipt", "note"} {
				v, present := res[k]
				if present && v != nil && !neStrGo(v) {
					errs = append(errs, fmt.Sprintf("%s.result.%s: must be a non-empty string when present", bpfx, k))
				}
			}
		}
	}
	return errs
}

// neStrGo reports whether v is a non-empty (after trim) string. Mirrors the
// python _ne_str / JS neStr helpers.
func neStrGo(v interface{}) bool {
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}
