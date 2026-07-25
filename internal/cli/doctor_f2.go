package cli

// doctor_f2.go — the F2 pair STRUCTURAL-CONSISTENCY audit (doctor check #18).
// This is the safety-layer ACT on committed F2 canonical+MD pairs: it scans
// docs/checkpoints/f2/ for <cycle>.canonical.json + <cycle>.md pairs and FAILs
// when a pair is internally inconsistent.
//
// THE CHECK IS STRUCTURAL, NOT A TRUTH PROVER. A PASSING pair is internally
// consistent; it is NOT thereby proven to describe conclusions/media/evidence
// that are actually true. The detail wording states this explicitly.
//
// CHECKS PER PAIR (memo L362-382 — "doctor says structural consistency, not
// semantic truth"):
//  1. PAIR PRESENCE: both .canonical.json and .md exist.
//  1b. ENVELOPE CYCLE BINDING: CanonicalEnvelope.SynthesisCycleID == pair
//      filename base (the durable identity must match the on-disk filename).
//  2. DIGEST RECOMPUTE: re-derive from CanonicalEnvelope, compare to carried
//     SourceSemanticDigest. Mismatch = F2 drifted canonical content under the
//     same digest.
//  2c. METADATA CYCLE BINDING: F2ViewMetadata.SynthesisCycleID ==
//      CanonicalEnvelope.SynthesisCycleID. A two-faced sidecar where meta says
//      "bar" while env says "foo" (matching filename) is an internal
//      inconsistency — the metadata cycle is derived FROM the envelope cycle,
//      so a divergence is by definition malformed.
//  3. RECIPROCAL BINDING: canonical's ReciprocalLocator → .md, MD's metadata
//     block → canonical. The pair members must point at each other.
//  4. PAIR METADATA AGREEMENT: cycle_id, entry_ids, source_semantic_digest,
//     schema/projection/renderer versions must agree between the two
//     artifacts (ReciprocalLocator intentionally differs — each points at
//     the other).
//  5. DETERMINISTIC PROJECTION EQUIVALENCE: re-render the MD from the stored
//     canonical sidecar (using its own timestamp) and byte-compare to the
//     stored MD. A drift = the MD was hand-edited or rendered by a different
//     renderer version.
//  6. P-c REQUIRED STRUCTURE: the MD carries the f2-pc-headline structural
//     markers and the canonical envelope has the R3/P-a/R1 entries the P-c
//     headline requires (decision frame, disposition, counter-evidence,
//     weakest claim). The R3 entry must have a usable (non-nil) payload.
//  7. P-b PROVENANCE PRESENCE: if MediaAttachments are declared on the
//     sidecar, each validates against the envelope (the P-b provenance gate).
//     The detail states content truth is NOT verified.
//  8. P-a ENUM VALIDITY: every P-a probe Result in the canonical envelope is
//     a known enum (found/not_found_in_checked_scope/unavailable/not_run).
//  9. R5 BINDING CONSISTENCY: if R5Binding is declared on the sidecar, it
//     validates against the envelope (the R5 durable-path gate).
//
// SCAN SURFACE: docs/checkpoints/f2/ (the F2 artifact directory). This is
// distinct from #16/#17 which scan .md files for fenced blocks; #18 scans the
// actual FILE PAIRS (canonical.json + .md side by side).

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// logicalF2Dir is the repo-relative logical directory the F2 pair contract
// uses for ReciprocalLocator values. Production pairs persist locators in
// this repo-relative form (see f2_persist.go buildF2CanonicalSidecar +
// f2_projection.go RenderF2MarkdownProjection — both take `dir` and use
// filepath.Join(dir, cycle+".md") etc.). The doctor must compare against the
// SAME logical dir, not the absolute scan dir, or every production pair would
// spuriously FAIL the reciprocal check.
var logicalF2Dir = filepath.Join("docs", "checkpoints", "f2")

// checkF2PairConsistency is the F2 pair structural-consistency doctor check.
// It is READ-ONLY: it never mutates a pair artifact and never shells out.
func checkF2PairConsistency(target string) checkResult {
	const name = "f2-pairs"

	absDir := filepath.Join(target, logicalF2Dir)
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		// No F2 directory at all = SKIP (nothing to audit).
		return checkResult{name: name, tier: tierSkip,
			detail: "no docs/checkpoints/f2/ directory (nothing to audit)"}
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return checkResult{name: name, tier: tierFail,
			detail: "cannot read docs/checkpoints/f2/: " + err.Error()}
	}

	// Collect canonical sidecars and MDs by base name (the filename stem,
	// NOT the embedded cycle ID — a pair stored as foo.{canonical.json,md}
	// that declares cycle "bar" is itself a finding, not a pass).
	type pairKey struct{ base string }
	canonicals := make(map[pairKey]string) // base → canonical.json path
	mds := make(map[pairKey]string)        // base → .md path

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		entryName := entry.Name()
		path := filepath.Join(absDir, entryName)
		if strings.HasSuffix(entryName, ".canonical.json") {
			base := strings.TrimSuffix(entryName, ".canonical.json")
			canonicals[pairKey{base}] = path
		} else if strings.HasSuffix(entryName, ".md") {
			base := strings.TrimSuffix(entryName, ".md")
			mds[pairKey{base}] = path
		}
	}

	// Union of all cycle bases.
	allBases := make(map[pairKey]bool)
	for k := range canonicals {
		allBases[k] = true
	}
	for k := range mds {
		allBases[k] = true
	}

	if len(allBases) == 0 {
		// Directory exists but no canonical.json or .md files = SKIP.
		return checkResult{name: name, tier: tierSkip,
			detail: "docs/checkpoints/f2/ exists but contains no .canonical.json or .md pairs (nothing to audit)"}
	}

	type finding struct {
		source string
		reason string
	}
	var findings []finding
	pairsAudited := 0

	// Sort bases for deterministic output.
	sortedBases := make([]pairKey, 0, len(allBases))
	for k := range allBases {
		sortedBases = append(sortedBases, k)
	}
	sort.Slice(sortedBases, func(i, j int) bool { return sortedBases[i].base < sortedBases[j].base })

	for _, key := range sortedBases {
		canonicalPath, hasCanonical := canonicals[key]
		mdPath, hasMD := mds[key]

		if !hasCanonical {
			findings = append(findings, finding{
				source: mdPath,
				reason: fmt.Sprintf("incomplete pair: .md exists but no %s.canonical.json (canonical sidecar missing)", key.base),
			})
			continue
		}
		if !hasMD {
			findings = append(findings, finding{
				source: canonicalPath,
				reason: fmt.Sprintf("incomplete pair: .canonical.json exists but no %s.md (projection missing)", key.base),
			})
			continue
		}

		pairsAudited++
		for _, f := range auditF2Pair(canonicalPath, mdPath, logicalF2Dir, key.base) {
			findings = append(findings, finding{source: canonicalPath, reason: f})
		}
	}

	if len(findings) == 0 {
		return checkResult{name: name, tier: tierPass,
			detail: fmt.Sprintf("%d F2 pair(s) audited; every pair is structurally consistent (structural consistency is not semantic truth; content/media truth is not verified)", pairsAudited)}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].source < findings[j].source })
	var b strings.Builder
	fmt.Fprintf(&b, "%d F2 pair consistency violation(s):", len(findings))
	for _, f := range findings {
		fmt.Fprintf(&b, "\n  - %s: %s", f.source, f.reason)
	}
	b.WriteString("\nAn F2 pair is a <cycle>.canonical.json + <cycle>.md file pair. The canonical sidecar must carry a digest matching its canonical content, the MD must be the deterministic projection of the canonical sidecar, and the pair must retain cycle/entry/digest/version consistency. (Structural consistency is not semantic truth; content/media truth is not verified.)")
	return checkResult{name: name, tier: tierFail, detail: b.String()}
}

// auditF2Pair returns a list of structural-consistency findings for one pair.
// Each finding is a reason string (the caller adds the source path). An empty
// slice means the pair is structurally consistent.
//
// Parameters:
//   - canonicalPath: absolute path to the .canonical.json file (for I/O)
//   - mdPath: absolute path to the .md file (for I/O)
//   - logicalDir: repo-relative logical dir (e.g. "docs/checkpoints/f2") — the
//     basis the F2 pair contract uses for ReciprocalLocator values AND for
//     the deterministic re-render. Using the repo-relative dir (not the
//     absolute scan dir) ensures production pairs with repo-relative locators
//     pass the reciprocal check.
//   - pairBase: the filename stem shared by the pair (e.g. "cycle-001" from
//     "cycle-001.canonical.json" + "cycle-001.md"). The reciprocal locators
//     MUST point at the actual pair files (derived from pairBase), NOT from
//     the embedded cycle ID — a pair stored as foo.{canonical.json,md} that
//     declares cycle "bar" is itself a finding.
func auditF2Pair(canonicalPath, mdPath, logicalDir, pairBase string) []string {
	// 1. Parse the canonical sidecar.
	sidecar, err := ReadF2CanonicalSidecar(canonicalPath)
	if err != nil {
		return []string{"canonical sidecar unparseable: " + err.Error()}
	}
	if sidecar.Kind != F2CanonicalSidecarKind {
		return []string{fmt.Sprintf("canonical sidecar kind = %q, want %q", sidecar.Kind, F2CanonicalSidecarKind)}
	}
	env := sidecar.CanonicalEnvelope
	if env == nil {
		return []string{"canonical sidecar has nil CanonicalEnvelope"}
	}

	var findings []string

	// 1b. Envelope cycle ID must match the pair filename basename. A pair
	// stored as foo.{canonical.json,md} whose envelope declares cycle "bar"
	// is internally inconsistent (the canonical identity and the durable
	// filename disagree). Step 2c independently checks that the F2 view
	// metadata cycle matches the envelope cycle — together these three legs
	// (filename, envelope, metadata) must all agree on the cycle identity.
	if env.SynthesisCycleID != pairBase {
		findings = append(findings, fmt.Sprintf("envelope SynthesisCycleID %q != pair filename base %q (the canonical cycle identity must match the durable pair filename)", env.SynthesisCycleID, pairBase))
	}

	// 2. Digest recompute.
	if env.SemanticDigest == "" {
		findings = append(findings, "canonical envelope carries no semantic_digest")
	} else {
		got, derr := env.ComputeDigest()
		if derr != nil {
			findings = append(findings, fmt.Sprintf("digest re-derivation failed: %v", derr))
		} else if got != env.SemanticDigest {
			findings = append(findings, fmt.Sprintf("semantic_digest mismatch (envelope %q != recomputed %q) — canonical content drifted under the same digest", env.SemanticDigest, got))
		}
	}

	// 2b. Carried metadata digest matches envelope digest.
	meta := sidecar.F2ViewMetadata
	if meta.SourceSemanticDigest != "" && meta.SourceSemanticDigest != env.SemanticDigest {
		findings = append(findings, fmt.Sprintf("F2ViewMetadata.SourceSemanticDigest (%q) != envelope.SemanticDigest (%q)", meta.SourceSemanticDigest, env.SemanticDigest))
	}

	// 2c. F2ViewMetadata.SynthesisCycleID must equal CanonicalEnvelope's
	// SynthesisCycleID. This closes the two-faced sidecar gap where meta
	// says "bar" (matching the MD) while env says "foo" (matching the
	// filename) — the metadata cycle is derived FROM the envelope cycle, so
	// a divergence is by definition malformed (see F2ArtifactViewMeta
	// SynthesisCycleID doc comment: "the canonical cycle identity, from the
	// envelope"). This is a SEPARATE leg from step 1b (env cycle vs
	// pairBase) and step 4 (MD meta vs canonical meta).
	if meta.SynthesisCycleID != env.SynthesisCycleID {
		findings = append(findings, fmt.Sprintf("F2ViewMetadata.SynthesisCycleID (%q) != CanonicalEnvelope.SynthesisCycleID (%q)", meta.SynthesisCycleID, env.SynthesisCycleID))
	}

	// 3. Read + parse the MD.
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		findings = append(findings, "cannot read MD projection: "+err.Error())
		return findings
	}
	mdMeta, extractErr := ExtractF2ViewMetadataFromMDBytes(mdBytes)
	if extractErr != nil {
		findings = append(findings, "cannot extract f2-view-metadata from MD: "+extractErr.Error())
		return findings
	}

	// 4. Pair metadata agreement (cycle, digest, schema, versions).
	if mdMeta.SynthesisCycleID != meta.SynthesisCycleID {
		findings = append(findings, fmt.Sprintf("cycle mismatch: canonical %q != MD %q", meta.SynthesisCycleID, mdMeta.SynthesisCycleID))
	}
	if mdMeta.SourceSemanticDigest != meta.SourceSemanticDigest {
		findings = append(findings, fmt.Sprintf("digest mismatch: canonical %q != MD %q", meta.SourceSemanticDigest, mdMeta.SourceSemanticDigest))
	}
	if mdMeta.SchemaVersion != meta.SchemaVersion {
		findings = append(findings, fmt.Sprintf("schema_version mismatch: canonical %q != MD %q", meta.SchemaVersion, mdMeta.SchemaVersion))
	}
	if mdMeta.ProjectionVersion != meta.ProjectionVersion {
		findings = append(findings, fmt.Sprintf("projection_version mismatch: canonical %q != MD %q", meta.ProjectionVersion, mdMeta.ProjectionVersion))
	}
	if mdMeta.RendererVersion != meta.RendererVersion {
		findings = append(findings, fmt.Sprintf("renderer_version mismatch: canonical %q != MD %q", meta.RendererVersion, mdMeta.RendererVersion))
	}
	// Entry IDs must agree (sorted, so direct comparison).
	if !stringSlicesEqual(mdMeta.EntryIDs, meta.EntryIDs) {
		findings = append(findings, fmt.Sprintf("entry_ids mismatch: canonical %v != MD %v", meta.EntryIDs, mdMeta.EntryIDs))
	}

	// 4b. Reciprocal binding: canonical's ReciprocalLocator MUST point at the
	// .md member; MD's ReciprocalLocator MUST point at the canonical.json
	// member. The pair members must point at each other (memo L66-69).
	// Expected locators are derived from the PAIR's actual filename base
	// (pairBase), NOT from the embedded cycle ID — a pair stored as
	// foo.{canonical.json,md} that declares cycle "bar" is a finding.
	expectedMDLocator := filepath.Join(logicalDir, pairBase+".md")
	if meta.ReciprocalLocator != expectedMDLocator {
		findings = append(findings, fmt.Sprintf("canonical ReciprocalLocator %q != expected %q (must point at the .md member of the pair)", meta.ReciprocalLocator, expectedMDLocator))
	}
	expectedCanonicalLocator := filepath.Join(logicalDir, pairBase+".canonical.json")
	if mdMeta.ReciprocalLocator != expectedCanonicalLocator {
		findings = append(findings, fmt.Sprintf("MD ReciprocalLocator %q != expected %q (must point at the canonical.json member of the pair)", mdMeta.ReciprocalLocator, expectedCanonicalLocator))
	}

	// 5. Deterministic projection equivalence: re-render MD from the stored
	// canonical sidecar (using its own timestamp) and byte-compare. Uses the
	// same logicalDir the production renderer uses so the ReciprocalLocator
	// in the re-rendered MD matches the production shape.
	expectedMD, rerr := RenderF2MarkdownProjection(sidecar, logicalDir)
	if rerr != nil {
		findings = append(findings, fmt.Sprintf("cannot re-render MD from canonical sidecar: %v", rerr))
	} else if string(expectedMD) != string(mdBytes) {
		findings = append(findings, "MD projection is stale (stored MD bytes != deterministic re-render from the stored canonical sidecar) — the MD was hand-edited or rendered by a different renderer version")
	}

	// 6. P-c required structure: check the MD carries the P-c markers and the
	// canonical envelope has the entries the P-c headline requires (R3 for the
	// decision frame + disposition, P-a for counter-evidence + weakest claim).
	mdStr := string(mdBytes)
	if !strings.Contains(mdStr, "f2-pc-headline:begin") || !strings.Contains(mdStr, "f2-pc-headline:end") {
		findings = append(findings, "MD missing P-c headline structural markers (f2-pc-headline:begin/:end)")
	} else {
		// The P-c headline requires: R3 entry (decision frame + disposition),
		// P-a probes with WeakestClaim (counter-evidence + weakest claim).
		// Check the canonical envelope has these entries.
		if !envelopeHasFamily(env, F1FamilyR3RedesignFork) {
			findings = append(findings, "P-c headline requires an r3_redesign_fork entry (decision frame); canonical envelope has none")
		} else if !envelopeHasNonNilR3Payload(env) {
			findings = append(findings, "P-c headline requires an r3_redesign_fork entry with a usable (non-nil) R3 payload (decision frame + disposition); canonical envelope has an r3_redesign_fork entry but its R3 payload is nil")
		}
		if !envelopeHasPAWeakestClaim(env) {
			findings = append(findings, "P-c headline requires P-a probes with weakest_claim; canonical envelope has none")
		}
	}

	// 7. P-b provenance presence: if MediaAttachments are on the sidecar, each
	// must validate against the envelope (the P-b durable-path gate).
	for i, att := range sidecar.MediaAttachments {
		if verr := ValidateF2MediaAttachmentAgainstEnvelope(&att, env); verr != nil {
			findings = append(findings, fmt.Sprintf("P-b media attachment[%d] (%s) fails provenance gate: %v (content truth is NOT verified)", i, att.AttachmentID, verr))
		}
	}

	// 8. P-a enum validity: every P-a probe Result is a known enum.
	for _, e := range env.Entries {
		if e.Family != F1FamilyPACounterEvidence || e.PA == nil {
			continue
		}
		for _, probe := range e.PA.Probes {
			if !f1PAResultValid(probe.Result) {
				findings = append(findings, fmt.Sprintf("P-a probe %s has unknown result enum %q (valid: found/not_found_in_checked_scope/unavailable/not_run)", probe.ProbeID, probe.Result))
			}
		}
	}

	// 9. R5 binding consistency: if R5Binding is on the sidecar, it must
	// validate against the envelope (the R5 durable-path gate).
	if sidecar.R5Binding != nil {
		if verr := ValidateF2R5BindingAgainstEnvelope(sidecar.R5Binding, env); verr != nil {
			findings = append(findings, fmt.Sprintf("R5 binding fails durable-path gate: %v", verr))
		}
	}

	return findings
}

// envelopeHasFamily reports whether the envelope has at least one entry of
// the given family.
func envelopeHasFamily(env *F1SynthesisEnvelope, family string) bool {
	for _, e := range env.Entries {
		if e.Family == family {
			return true
		}
	}
	return false
}

// envelopeHasNonNilR3Payload reports whether the envelope has at least one
// r3_redesign_fork entry with a non-nil R3 summary payload. A nil R3 payload
// means the entry carries no decision frame or disposition — the P-c headline
// would emit placeholder absence strings, not real canonical values.
func envelopeHasNonNilR3Payload(env *F1SynthesisEnvelope) bool {
	for _, e := range env.Entries {
		if e.Family == F1FamilyR3RedesignFork && e.R3 != nil {
			return true
		}
	}
	return false
}

// envelopeHasPAWeakestClaim reports whether the envelope's P-a probes carry at
// least one non-empty WeakestClaim (the P-c headline's "Weakest Claim" section
// requires this).
func envelopeHasPAWeakestClaim(env *F1SynthesisEnvelope) bool {
	for _, e := range env.Entries {
		if e.Family != F1FamilyPACounterEvidence || e.PA == nil {
			continue
		}
		for _, probe := range e.PA.Probes {
			if probe.WeakestClaim != "" {
				return true
			}
		}
	}
	return false
}

// f1PAResultValid reports whether a P-a probe result is a known enum.
func f1PAResultValid(result string) bool {
	switch result {
	case F1PAResultFound, F1PAResultNotFoundInCheckedScope, F1PAResultUnavailable, F1PAResultNotRun:
		return true
	}
	return false
}

// stringSlicesEqual reports whether two string slices are equal (same length +
// same elements in the same order). Used for EntryIDs comparison (both are
// sorted, so direct comparison is valid).
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// (No pure in-memory analyzer: the audit reads the MD from disk as part of the
// pair check, so tests use the filesystem-based f2DoctorFixture pattern
// instead of duplicating auditF2Pair's logic in a second function that would
// drift from the real path.)
