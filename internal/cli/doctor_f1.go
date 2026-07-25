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
	"bytes"
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

// --- F1/F2 consistency audit (check #17) -----------------------------------
//
// checkF1F2Consistency is the F1→F2 emit-boundary consistency audit. It scans
// the same durable closeout surfaces as checkF1EnvelopeConsistency (#16) for
// fenced ```f1-synthesis-envelope projections that carry F2-DERIVED view
// metadata (an F2-bearing projection: the combined F1+F2 artifact the data
// model supports today). For each, it verifies the F1→F2 digest-binding
// contract (memo L216-242):
//
//  1. DIGEST-BINDING: the carried semantic_digest still matches the canonical
//     content (re-derive + compare). A mismatch means F2 drifted canonical
//     content under the same digest — the core F1/F2 boundary violation
//     (memo L130, L241-242: F2 cannot replace/recalculate semantic content
//     under the same digest; a changed canonical field requires a new emit).
//  2. BINDING REFERENCES: synthesis_cycle_id + >=1 entry_id + semantic_digest
//     are present (memo L239-242: every persisted/rendered F2 artifact retains
//     them).
//  3. DERIVED-FIELD ALLOW-LIST: the f2_view object carries ONLY the closed
//     derived-field set (f2DerivedFieldAllowList). A foreign field is an F2
//     boundary violation (F2 smuggling content into the derived surface).
//
// THE CHECK IS STRUCTURAL, NOT A TRUTH PROVER (the same honesty ceiling as
// behavioral-closure and #16). It SKIPs cleanly when NO projection carries an
// F2 view (nothing to audit — F2 rendering is a separate track and may not
// have attached view metadata yet). An F1 projection WITHOUT an f2_view is not
// F2-bearing and is not audited here (it is covered by #16).
func checkF1F2Consistency(target string) checkResult {
	const name = "f1-f2-consistency"

	surfaces := []string{
		filepath.Join(".local", "coordinator", "reports"),
		filepath.Join("docs", "checkpoints"),
	}

	type finding struct{ source, reason string }
	var findings []finding
	mdFiles := 0
	f2Views := 0

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
				// Unreadable artifact is already reported by #16; do not
				// double-report.
				return nil
			}
			body := string(data)
			analyzed, seen := analyzeF1F2Consistency(body)
			f2Views += seen
			for _, reason := range analyzed {
				findings = append(findings, finding{rel, reason})
			}
			return nil
		})
	}

	// SKIP when no F2-bearing projection exists — there is nothing to audit at
	// the F1/F2 boundary. This is distinct from mdFiles==0: artifacts may exist
	// (and be audited by #16) yet carry no f2_view (F2 has not attached view
	// metadata). The honest signal for a not-yet-adopted F2 surface is SKIP.
	if f2Views == 0 {
		return checkResult{name: name, tier: tierSkip,
			detail: fmt.Sprintf("%d closeout artifact(s) scanned; none carries an F2-bearing projection (no f2_view to audit)", mdFiles)}
	}
	if len(findings) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d F2-bearing projection(s) across %d artifact(s) audited; every F1→F2 digest-binding contract holds (structural consistency is not semantic truth)", f2Views, mdFiles)}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].source < findings[j].source })
	var b strings.Builder
	fmt.Fprintf(&b, "%d F1/F2 consistency violation(s):", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "\n  - %s: %s", f.source, f.reason)
	}
	b.WriteString("\nAn F2-bearing projection is a fenced ```f1-synthesis-envelope JSON block carrying an f2_view object. It must retain synthesis_cycle_id + entry_ids + semantic_digest, the digest must still bind the canonical content (F2 must not drift it), and the f2_view must carry only the closed derived-field allow-list (storage_locator, write_timestamp, view_model_version, renderer_version, attachment_meta_ref). (Structural consistency is not semantic truth.)")
	return checkResult{name: name, tier: tierFail, detail: b.String()}
}

// analyzeF1F2Consistency is the pure parsing core of the F1/F2 consistency
// audit (separated so tests can drive it without a filesystem). It returns the
// list of F1/F2 boundary findings AND the count of F2-bearing projections
// considered (so the check can distinguish "no F2 view anywhere" = SKIP from
// "F2 views audited" = PASS/FAIL). Malformed JSON is left to #16 and skipped
// here (no double-reporting).
func analyzeF1F2Consistency(body string) ([]string, int) {
	blocks := f1EnvelopeRe.FindAllStringSubmatch(body, -1)
	var findings []string
	seen := 0
	for n, m := range blocks {
		raw := []byte(m[1])
		var env F1SynthesisEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue // malformed JSON is reported by #16
		}
		// F2-bearing status is decided from RAW "f2_view" KEY PRESENCE, not
		// from non-empty known fields on the unmarshaled F2ViewMetadata:
		// json.Unmarshal silently drops unknown keys, so a PURE-FOREIGN f2_view
		// (the exact smuggling case the allow-list must catch) would yield an
		// empty struct and be skipped if presence were decided from non-empty
		// known fields. Key presence is the honest signal.
		if !f2ViewKeyPresent(raw) {
			continue // not an F2-bearing projection (no f2_view key at all)
		}
		seen++
		prefix := fmt.Sprintf("f1-synthesis-envelope block #%d (F2-bearing)", n+1)
		findings = append(findings, auditF2DigestBinding(prefix, &env)...)
		findings = append(findings, auditF2BindingRefs(prefix, &env)...)
		findings = append(findings, auditF2DerivedAllowList(prefix, extractF2ViewRaw(raw))...)
	}
	return findings, seen
}

// auditF2DigestBinding verifies the digest still binds the canonical content
// (F2 must not drift it under the same digest).
func auditF2DigestBinding(prefix string, env *F1SynthesisEnvelope) []string {
	var findings []string
	if env.SemanticDigest == "" {
		findings = append(findings, prefix+": carries an f2_view but no semantic_digest (every persisted/rendered F2 artifact must retain the binding digest)")
		return findings
	}
	got, derr := env.ComputeDigest()
	if derr != nil {
		findings = append(findings, fmt.Sprintf("%s: digest re-derivation failed: %v", prefix, derr))
		return findings
	}
	if got != env.SemanticDigest {
		findings = append(findings, fmt.Sprintf("%s: semantic_digest mismatch (stored %q != recomputed %q) — F2 drifted canonical content under the same digest (a changed canonical field requires a new F1 emit, not an in-place correction)", prefix, env.SemanticDigest, got))
	}
	return findings
}

// auditF2BindingRefs verifies the projection retains the binding references
// F2 must carry (synthesis_cycle_id + entry_ids).
func auditF2BindingRefs(prefix string, env *F1SynthesisEnvelope) []string {
	var findings []string
	if env.SynthesisCycleID == "" {
		findings = append(findings, prefix+": carries an f2_view but no synthesis_cycle_id (every persisted/rendered F2 artifact must retain it)")
	}
	entryIDCount := 0
	for _, e := range env.Entries {
		if e.EntryID != "" {
			entryIDCount++
		}
	}
	if entryIDCount == 0 {
		findings = append(findings, prefix+": carries an f2_view but no entry_ids (every persisted/rendered F2 artifact must retain them)")
	}
	return findings
}

// auditF2DerivedAllowList verifies the raw f2_view object carries ONLY the
// closed derived-field set. It first requires the f2_view to be a JSON OBJECT
// (leading '{' after trimming): Go's json.Decoder.Decode into a non-pointer
// struct target accepts literal null silently (no error, fields at zero value),
// which would let a non-object f2_view (null/scalar/array/bool) PASS the
// allow-list audit -- a fail-open. Requiring a leading '{' closes null and
// every non-object shape comprehensively. It then re-parses the object with
// DisallowUnknownFields against F2ViewMetadata so a foreign field (content
// smuggled into the derived surface) is a finding. Empty input is not an error
// (the f2_view key was absent, already skipped by f2ViewKeyPresent).
func auditF2DerivedAllowList(prefix string, rawF2View []byte) []string {
	trimmed := bytes.TrimSpace(rawF2View)
	if len(trimmed) == 0 {
		return nil
	}
	// The f2_view MUST be a JSON object. A non-object f2_view (null, scalar,
	// array, boolean) is not a valid derived-view surface and is rejected
	// before decoding (the DisallowUnknownFields path cannot catch it because
	// Decode accepts null into a struct silently).
	if trimmed[0] != '{' {
		snippet := string(trimmed)
		if len(snippet) > 16 {
			snippet = snippet[:16] + "..."
		}
		return []string{fmt.Sprintf("%s: f2_view must be a JSON object, got %q (a non-object f2_view -- null/scalar/array/bool -- is not a valid derived-view surface; want an object limited to the allow-list: %s)", prefix, snippet, strings.Join(f2DerivedFieldAllowList, ", "))}
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var meta F2ViewMetadata
	if err := dec.Decode(&meta); err != nil {
		return []string{fmt.Sprintf("%s: f2_view %v (only the derived-field allow-list is permitted: %s)", prefix, err, strings.Join(f2DerivedFieldAllowList, ", "))}
	}
	return nil
}

// f2ViewKeyPresent reports whether the raw block body carries an "f2_view" key
// (regardless of whether its object holds only allow-list fields, only foreign
// fields, or is empty). F2-bearing status is decided from RAW KEY PRESENCE so
// that a PURE-FOREIGN f2_view (json.Unmarshal drops its unknown keys, leaving
// an empty F2ViewMetadata) is still audited rather than silently skipped — the
// smuggling case the derived-field allow-list exists to catch.
func f2ViewKeyPresent(blockBody []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(blockBody, &m); err != nil {
		return false
	}
	_, present := m["f2_view"]
	return present
}

// extractF2ViewRaw re-parses blockBody into a raw-message map and returns the
// raw bytes of the "f2_view" key (or nil if absent). Used so the allow-list
// check can strictly validate the f2_view object in isolation, without the
// DisallowUnknownFields strictness bleeding into the rest of the envelope.
func extractF2ViewRaw(blockBody []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(blockBody, &m); err != nil {
		return nil
	}
	return m["f2_view"]
}
