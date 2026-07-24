package cli

// doctor_f1.go — the F1 synthesis-envelope STRUCTURAL-CONSISTENCY audit
// (doctor check, appended after behavioral-closure). This is the safety-layer
// ACT on committed F1 projections: it scans durable closeout artifacts for
// fenced `f1-synthesis-envelope` JSON blocks and FAILs when a committed
// projection is internally inconsistent (unknown enum, missing/duplicate
// family, dangling cross-reference, or a digest that no longer matches the
// canonical content).
//
// THE CHECK IS STRUCTURAL, NOT A TRUTH PROVER. A PASSING envelope is
// internally consistent; it is NOT thereby proven to describe conclusions
// that are actually true. Proving truth is the federated verifier's job, not
// a doctor gate (determine-whether-evidence-actually-true is NOT a structural
// gate per the authority-line audit).
//
// This check does NOT reuse behavioral-closure's absent-token-passes
// behavior for applicable F1 seams: when an envelope IS present, a missing
// family entry is INCOMPLETE and FAILs. But an artifact with NO envelope
// block contributes no finding (SKIP when nothing carries one) — that is
// "nothing to audit", not "passing an applicable seam".
//
// SCAN SURFACES (same durable closeout artifacts as behavioral-closure):
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

// f1EnvelopeRe matches a fenced code block whose info string begins with
// "f1-synthesis-envelope" and captures the block body. The body is JSON
// (deterministic serialization + sha256 digest align naturally with JSON).
// Deliberately a double-quoted string (the pattern contains literal backticks).
var f1EnvelopeRe = regexp.MustCompile("(?s)```f1-synthesis-envelope[ \\t]*\\n(.*?)\\n```")

// checkF1EnvelopeConsistency is the F1 structural-consistency doctor check.
// It is READ-ONLY: it never mutates a closeout artifact and never shells out.
func checkF1EnvelopeConsistency(target string) checkResult {
	const name = "f1-envelope"

	surfaces := []string{
		filepath.Join(".local", "coordinator", "reports"),
		filepath.Join("docs", "checkpoints"),
	}

	type finding struct{ source, reason string }
	var findings []finding
	mdFiles := 0
	projections := 0

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
				findings = append(findings, finding{rel, "unreadable closeout artifact: " + err.Error()})
				return nil
			}
			body := string(data)
			projections += countF1EnvelopeBlocks(body)
			for _, reason := range analyzeF1EnvelopeBlocks(body) {
				findings = append(findings, finding{rel, reason})
			}
			return nil
		})
	}

	// SKIP when no f1-synthesis-envelope projection was carried by ANY
	// artifact — there is nothing to audit. This is distinct from mdFiles==0
	// (no .md files at all): artifacts may exist yet carry no projection, and
	// the honest signal for a new, not-yet-adopted structural family is SKIP
	// ("nothing to audit"), not a vacuous PASS ("every projection is
	// consistent"). Code and docs agree on this across the three staged
	// contract sites (file-level comment, doctor.go registration, README).
	if projections == 0 {
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("%d closeout artifact(s) scanned; none carries an f1-synthesis-envelope projection (nothing to audit)", mdFiles)}
	}
	if len(findings) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d projection(s) across %d artifact(s) audited; every f1-synthesis-envelope projection is structurally consistent (structural consistency is not semantic truth)", projections, mdFiles)}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].source < findings[j].source })
	var b strings.Builder
	fmt.Fprintf(&b, "%d inconsistent/malformed f1-synthesis-envelope projection(s):", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "\n  - %s: %s", f.source, f.reason)
	}
	b.WriteString("\nA projection is a fenced ```f1-synthesis-envelope JSON block. It must use the closed vocabulary, carry exactly one entry per family when applicable, resolve every cross-reference, and carry a semantic_digest matching its canonical content. (Structural consistency is not semantic truth.)")
	return checkResult{name: name, tier: tierFail, detail: b.String()}
}

// countF1EnvelopeBlocks returns the number of fenced f1-synthesis-envelope
// blocks in body. Pure (no filesystem); the doctor check uses it to
// distinguish "artifacts exist but none carries a projection" (SKIP — nothing
// to audit) from "at least one projection was audited" (PASS/FAIL).
func countF1EnvelopeBlocks(body string) int {
	return len(f1EnvelopeRe.FindAllStringSubmatch(body, -1))
}

// analyzeF1EnvelopeBlocks parses every ```f1-synthesis-envelope fenced JSON
// block in body and returns a reason string for each block that fails pure
// structural validation. A consistent block contributes no reason. This is the
// pure parsing core, separated so tests can drive it without a filesystem.
// Each block is parsed independently — a malformed JSON body is itself a
// finding (the projection must round-trip cleanly).
func analyzeF1EnvelopeBlocks(body string) []string {
	blocks := f1EnvelopeRe.FindAllStringSubmatch(body, -1)
	var reasons []string
	for n, m := range blocks {
		var env F1SynthesisEnvelope
		if err := json.Unmarshal([]byte(m[1]), &env); err != nil {
			reasons = append(reasons, fmt.Sprintf("f1-synthesis-envelope block #%d: JSON parse error: %v", n+1, err))
			continue
		}
		for _, e := range ValidateF1Envelope(&env) {
			reasons = append(reasons, fmt.Sprintf("f1-synthesis-envelope block #%d: %s", n+1, e))
		}
	}
	return reasons
}
