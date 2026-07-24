package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// f1_envelope_test.go — Slice 1 tests for the F1 synthesis-envelope vocabulary,
// pure validator, canonical serialization / digest, and the doctor audit.
//
// These tests cover exactly the Slice 1 contract:
//   - canonical fixture parses and validates structurally consistent;
//   - the canonical-template example and the parser/validator agree;
//   - unknown family / result / applicability enums fail;
//   - missing or duplicate family fails;
//   - broken cross-entry references fail;
//   - the digest changes when a canonical field changes;
//   - F2-only metadata does NOT affect the digest;
//   - a structurally-valid fixture does NOT claim semantic truth.

// canonicalF1Fixture returns a structurally-consistent envelope with all three
// families present, cross-references resolved, and the semantic digest
// computed + assigned. It is the golden fixture: every "happy path" test
// starts from a copy of it.
func canonicalF1Fixture() *F1SynthesisEnvelope {
	env := &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "cycle-001",
		Applicability:    F1ApplicabilityRequired,
		Entries: []F1FamilyEntry{
			{
				Family:    F1FamilyR1CrossLaneJoin,
				Triggered: F1TriggeredTriggered,
				EntryID:   "entry-r1",
				R1: &F1R1JoinSummary{Conclusions: []F1R1Conclusion{
					{ConclusionID: "R1C1", PropertyID: "R1P1", JoinDisposition: F1R1JoinUnion,
						Lanes:   []F1R1LaneContrib{{LaneID: "lane-a"}},
						Sources: []F1R1Source{{Locator: "src-a"}}},
				}},
			},
			{
				Family:    F1FamilyR3RedesignFork,
				Triggered: F1TriggeredTriggered,
				EntryID:   "entry-r3",
				R3: &F1R3ForkSummary{
					TriggerRecognized: true,
					Disposition:       F1R3DispositionPending,
					Options: []F1R3Option{
						{
							OptionID: "opt-continue", Mode: F1R3ModeContinueRepair, Mechanism: "apply the queued repair",
							AffectedProperties:       []string{"R1P1"},
							SupportRefs:              []string{"R1C1"},
							CounterEvidenceProbeRefs: []string{"PA-P1"},
							Costs:                    []string{"eng-hours"},
							Risks:                    []string{"recurrence"},
							ReversalCost:             "low",
							CheapestValidation:       "re-run suite",
						},
						{
							OptionID: "opt-redesign", Mode: F1R3ModeRedesign, Mechanism: "redesign the boundary so the hazard cannot recur",
							AffectedProperties:       []string{"R1P1"},
							SupportRefs:              []string{"R1C1"},
							CounterEvidenceProbeRefs: []string{"PA-P2"},
							Costs:                    []string{"eng-weeks"},
							Risks:                    []string{"schedule-slip"},
							ReversalCost:             "high",
							CheapestValidation:       "design-review + suite",
						},
					},
				},
			},
			{
				Family:    F1FamilyPACounterEvidence,
				Triggered: F1TriggeredTriggered,
				EntryID:   "entry-pa",
				PA: &F1PAProbeSummary{Probes: []F1PAProbe{
					{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotRun},
					{ProbeID: "PA-P2", TargetRef: "opt-redesign", Result: F1PAResultNotRun},
				}},
			},
		},
	}
	d, err := env.ComputeDigest()
	if err != nil {
		panic("canonical fixture digest: " + err.Error())
	}
	env.SemanticDigest = d
	// Reflect a post-AssignF1Validation state: the golden fixture is complete
	// (zero errors). Tests that mutate the envelope to be invalid and call
	// ValidateF1Envelope directly keep this carried disposition; the validator
	// checks the enum is known (not that it matches the error state — that
	// consistency is AssignF1Validation's job, which overwrites this).
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}
	return env
}

// canonicalF1FixtureBlock returns the canonical fixture rendered as a fenced
// ```f1-synthesis-envelope JSON block (the form the doctor scanner reads).
func canonicalF1FixtureBlock() string {
	b, err := canonicalF1Fixture().CanonicalBytes()
	if err != nil {
		panic("canonical fixture canonical bytes: " + err.Error())
	}
	// Re-hydrate to include semantic_digest in the rendered block (the digest
	// projection omits it by design; the artifact must still carry it).
	env := canonicalF1Fixture()
	_ = json.Unmarshal(b, env)
	full, err := json.Marshal(env)
	if err != nil {
		panic("canonical fixture full marshal: " + err.Error())
	}
	return "```f1-synthesis-envelope\n" + string(full) + "\n```"
}

func TestValidateF1Envelope_CanonicalFixture(t *testing.T) {
	env := canonicalF1Fixture()
	if errs := ValidateF1Envelope(env); len(errs) != 0 {
		t.Fatalf("canonical fixture must be structurally consistent; got errors:\n  %s", strings.Join(errs, "\n  "))
	}
}

// Template/parser agreement: the canonical fenced block parses under the same
// rules the validator enforces. This mirrors behavioral-closure's
// template/parser-agreement guarantee.
func TestAnalyzeF1EnvelopeBlocks_CanonicalTemplateAgrees(t *testing.T) {
	if reasons := analyzeF1EnvelopeBlocks(canonicalF1FixtureBlock()); len(reasons) != 0 {
		t.Fatalf("canonical fenced block must parse + validate clean; got reasons:\n  %s", strings.Join(reasons, "\n  "))
	}
}

func TestValidateF1Envelope_UnknownFamilyFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].Family = "r9_made_up"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown family") {
		t.Fatalf("unknown family must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_UnknownApplicabilityFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Applicability = "maybe"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown applicability") {
		t.Fatalf("unknown applicability must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_UnknownPAResultFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[2].PA.Probes[0].Result = "definitely"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown result") {
		t.Fatalf("unknown P-a result must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_MissingFamilyFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Drop the pa entry entirely.
	env.Entries = append(env.Entries[:2], env.Entries[3:]...)
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "pa_counter_evidence\" is missing") {
		t.Fatalf("missing family must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_DuplicateFamilyFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Clone the r1 entry so two r1 entries exist.
	env.Entries = append(env.Entries, env.Entries[0])
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "duplicate family") {
		t.Fatalf("duplicate family must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_BrokenR3SupportRefFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[1].R3.Options[0].SupportRefs = []string{"NO-SUCH-CONCLUSION"}
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any r1 conclusion_id") {
		t.Fatalf("dangling r3 support_ref must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_BrokenPAProbeRefFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[1].R3.Options[0].CounterEvidenceProbeRefs = []string{"NO-SUCH-PROBE"}
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any pa probe_id") {
		t.Fatalf("dangling r3 counter_evidence_probe_ref must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_BrokenPATargetRefFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[2].PA.Probes[0].TargetRef = "NO-SUCH-TARGET"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "does not resolve to any r1 conclusion_id or r3 option_id") {
		t.Fatalf("dangling pa target_ref must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestF1Digest_ChangesOnCanonicalFieldChange(t *testing.T) {
	env := canonicalF1Fixture()
	d1 := env.SemanticDigest
	// Mutate a CANONICAL content field (R1 conclusion property).
	env.Entries[0].R1.Conclusions[0].PropertyID = "R1P1-CHANGED"
	d2, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 == d2 {
		t.Fatalf("digest must change when a canonical field changes; both = %q", d1)
	}
}

func TestF1Digest_F2ViewMetadataDoesNotAffectDigest(t *testing.T) {
	env := canonicalF1Fixture()
	d1 := env.SemanticDigest
	// Add F2-derived view metadata (must be excluded from the digest).
	env.F2View = &F2ViewMetadata{
		StorageLocator:    "s3://bucket/projection-001.json",
		WriteTimestamp:    "2026-07-25T00:00:00Z",
		ViewModelVersion:  "2",
		RendererVersion:   "1",
		AttachmentMetaRef: "att://meta-001",
	}
	d2, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("F2 view metadata must NOT affect the digest; d1=%q d2=%q", d1, d2)
	}
}

// A structurally-valid fixture does NOT claim semantic truth: the validator's
// silence is the only claim, and the carried disposition means STRUCTURALLY
// complete — never that evidence/conclusions are true. This test pins that
// contract: a clean envelope yields complete with zero errors and no
// truth-bearing semantic field exists on the envelope.
func TestValidateF1Envelope_StructurallyValidDoesNotClaimTruth(t *testing.T) {
	env := canonicalF1Fixture()
	errs := AssignF1Validation(env)
	if len(errs) != 0 {
		t.Fatalf("canonical fixture must be structurally consistent; got errors:\n  %s", strings.Join(errs, "\n  "))
	}
	if env.Validation.Disposition != F1ValidationComplete {
		t.Fatalf("disposition = %q, want complete", env.Validation.Disposition)
	}
	if len(env.Validation.Errors) != 0 {
		t.Fatalf("complete envelope must carry zero errors; got %v", env.Validation.Errors)
	}
	// No field on the envelope asserts semantic truth. complete means
	// STRUCTURALLY complete only (proven truth is the federated verifier's
	// job). This assertion exists so a future field rename cannot quietly
	// introduce a truth claim into the structural validator's output.
	for _, bad := range []string{"true", "verified", "proven"} {
		if strings.Contains(strings.ToLower(env.Validation.Disposition), bad) {
			t.Fatalf("disposition %q must not carry a truth claim (%q)", env.Validation.Disposition, bad)
		}
	}
}

func TestValidateF1Envelope_IncompleteDispositionFailClosed(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].Family = "garbage" // force at least one error
	errs := AssignF1Validation(env)
	if len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
	if env.Validation.Disposition != F1ValidationIncomplete {
		t.Fatalf("disposition = %q, want incomplete (fail-closed)", env.Validation.Disposition)
	}
}

// TestAssignF1Validation_ZeroValueValidationReachesComplete is the regression
// guard for the committer-caught contract drift: a producer building a valid
// envelope (content + digest, Validation at its zero value per the documented
// contract) must reach disposition=complete via a SINGLE AssignF1Validation
// call. The carried-disposition enum check belongs ONLY on the doctor
// parse-site path (ValidateF1Envelope), never on the producer path — the
// producer is about to overwrite the disposition, so rejecting the not-yet-
// assigned zero value would be circular (validation determines disposition).
func TestAssignF1Validation_ZeroValueValidationReachesComplete(t *testing.T) {
	// Start from the golden fixture (valid content + computed digest) and
	// reset Validation to its zero value — the natural producer state before
	// AssignF1Validation is called.
	env := canonicalF1Fixture()
	env.Validation = F1ValidationInfo{} // zero value: Disposition=""

	errs := AssignF1Validation(env)
	if len(errs) != 0 {
		t.Fatalf("zero-value Validation on valid content must yield zero errors on the producer path; got:\n  %s", strings.Join(errs, "\n  "))
	}
	if env.Validation.Disposition != F1ValidationComplete {
		t.Fatalf("producer single-call disposition = %q, want complete", env.Validation.Disposition)
	}
}

// TestValidateF1Envelope_DoctorPathRejectsZeroValueDisposition confirms the
// complement: the doctor parse-site path (ValidateF1Envelope on a committed
// artifact) DOES reject a zero-value / unknown disposition — committed
// artifacts must carry a valid disposition (they are untrusted input). This
// is what distinguishes the two paths and keeps the parse site honest.
func TestValidateF1Envelope_DoctorPathRejectsZeroValueDisposition(t *testing.T) {
	env := canonicalF1Fixture()
	env.Validation = F1ValidationInfo{} // zero value: Disposition=""
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown validation disposition") {
		t.Fatalf("doctor path must reject zero-value disposition; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_EmptyDigestFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.SemanticDigest = "" // drop the digest
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "semantic_digest is empty") {
		t.Fatalf("empty digest must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_StaleDigestFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Mutate canonical content WITHOUT re-deriving the digest.
	env.Entries[0].R1.Conclusions[0].PropertyID = "R1P1-CHANGED"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "semantic_digest mismatch") {
		t.Fatalf("stale digest must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_DuplicateEntryIDFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[2].EntryID = env.Entries[0].EntryID // duplicate entry-r1
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "duplicate entry_id") {
		t.Fatalf("duplicate entry_id must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_NotTriggeredAllowsAbsentEntries(t *testing.T) {
	// A not_triggered envelope is N/A as a whole; entries are optional. An
	// empty entry list is structurally fine (there is no applicable seam to
	// require coverage for). The digest is computed over the (minimal)
	// canonical projection.
	env := &F1SynthesisEnvelope{
		SchemaVersion:    "1",
		SynthesisCycleID: "cycle-002",
		Applicability:    F1ApplicabilityNotTriggered,
		Entries:          nil,
		Validation:       F1ValidationInfo{Disposition: F1ValidationComplete},
	}
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	if errs := ValidateF1Envelope(env); len(errs) != 0 {
		t.Fatalf("not_triggered envelope with no entries must be consistent; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- doctor audit surface --------------------------------------------------

func TestCheckF1EnvelopeConsistency_NoArtifactsSkips(t *testing.T) {
	dir := t.TempDir()
	r := checkF1EnvelopeConsistency(dir)
	if r.tier != tierSkip {
		t.Fatalf("tier = %q, want SKIP when no artifacts carry a projection", r.tier)
	}
}

// TestCheckF1EnvelopeConsistency_ArtifactWithoutProjectionSkips is the
// regression guard for the committer-caught contract drift: when closeout
// .md artifacts EXIST but none carries an f1-synthesis-envelope projection,
// the check must SKIP ("nothing to audit"), not return a vacuous PASS. This
// matches the documented contract at doctor_f1.go (file comment), doctor.go
// (registration), and README.agent.md. A vacuous PASS would read as a
// positive endorsement where there is nothing to audit.
func TestCheckF1EnvelopeConsistency_ArtifactWithoutProjectionSkips(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "docs", "checkpoints")
	if err := os.MkdirAll(cp, 0o755); err != nil {
		t.Fatal(err)
	}
	// A checkpoint with NO f1-synthesis-envelope block.
	if err := os.WriteFile(filepath.Join(cp, "plain.md"), []byte("# plain checkpoint\n\nNo F1 projection here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkF1EnvelopeConsistency(dir)
	if r.tier != tierSkip {
		t.Fatalf("tier = %q, want SKIP when artifacts exist but none carries a projection; detail=%s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "none carries an f1-synthesis-envelope projection") {
		t.Fatalf("SKIP detail must explain no projection carried; got %q", r.detail)
	}
}

func TestCheckF1EnvelopeConsistency_ConsistentBlockPasses(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "docs", "checkpoints")
	if err := os.MkdirAll(cp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cp, "snap.md"), []byte("# snap\n\n"+canonicalF1FixtureBlock()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkF1EnvelopeConsistency(dir)
	if r.tier != tierPass {
		t.Fatalf("tier = %q, want PASS; detail=%s", r.tier, r.detail)
	}
}

func TestCheckF1EnvelopeConsistency_InconsistentBlockFails(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "docs", "checkpoints")
	if err := os.MkdirAll(cp, 0o755); err != nil {
		t.Fatal(err)
	}
	// Build a block with a dangling cross-reference (pa target_ref that
	// resolves to nothing).
	env := canonicalF1Fixture()
	env.Entries[2].PA.Probes[0].TargetRef = "NO-SUCH-TARGET"
	// Do NOT re-derive: the digest now also mismatches, compounding the
	// finding — but the dangling-ref reason is the one we assert on.
	full, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	block := "```f1-synthesis-envelope\n" + string(full) + "\n```"
	if err := os.WriteFile(filepath.Join(cp, "bad.md"), []byte("# bad\n\n"+block+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkF1EnvelopeConsistency(dir)
	if r.tier != tierFail {
		t.Fatalf("tier = %q, want FAIL; detail=%s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "does not resolve to any r1 conclusion_id or r3 option_id") {
		t.Fatalf("FAIL detail must name the dangling ref; got:\n%s", r.detail)
	}
}

func TestAnalyzeF1EnvelopeBlocks_MalformedJSONIsFinding(t *testing.T) {
	body := "```f1-synthesis-envelope\n{not valid json\n```\n"
	reasons := analyzeF1EnvelopeBlocks(body)
	if len(reasons) != 1 {
		t.Fatalf("got %d reasons, want 1 for malformed JSON: %v", len(reasons), reasons)
	}
	if !strings.Contains(reasons[0], "JSON parse error") {
		t.Fatalf("reason must mention JSON parse error; got %q", reasons[0])
	}
}

// --- F1: carried validation disposition is a closed enum -------------------

func TestValidateF1Envelope_UnknownValidationDispositionFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Validation = F1ValidationInfo{Disposition: "definitely"}
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown validation disposition") {
		t.Fatalf("unknown carried disposition must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- F6: required envelope identity fields ---------------------------------

func TestValidateF1Envelope_EmptySchemaVersionFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.SchemaVersion = ""
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "schema_version is empty") {
		t.Fatalf("empty schema_version must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_EmptySynthesisCycleIDFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.SynthesisCycleID = ""
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "synthesis_cycle_id is empty") {
		t.Fatalf("empty synthesis_cycle_id must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- F3: family↔summary binding --------------------------------------------

func TestValidateF1Envelope_ForeignSummaryFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Move the R3 summary onto the r1 entry — a foreign summary.
	env.Entries[0].R3 = env.Entries[1].R3
	env.Entries[1].R3 = nil
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "r1 entry must not carry an r3 summary") {
		t.Fatalf("foreign r3 summary on an r1 entry must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_TriggeredWithoutMatchingSummaryFails(t *testing.T) {
	env := canonicalF1Fixture()
	// r3 entry is triggered but drop its summary.
	env.Entries[1].R3 = nil
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "r3 entry is triggered but its r3 summary is missing") {
		t.Fatalf("triggered r3 entry without summary must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- F5: remaining unknown-enum coverage -----------------------------------

func TestValidateF1Envelope_UnknownTriggeredFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[0].Triggered = "perhaps"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown triggered") {
		t.Fatalf("unknown triggered must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_UnknownR3ModeFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[1].R3.Options[0].Mode = "rewrite"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown mode") {
		t.Fatalf("unknown r3 mode must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_UnknownR3DispositionFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[1].R3.Disposition = "approved"
	if errs := ValidateF1Envelope(env); !hasErrContaining(errs, "unknown disposition") {
		t.Fatalf("unknown r3 disposition must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- F2 regression: nil vs empty Entries produce identical digests ---------

func TestF1Digest_NilVsEmptyEntriesStable(t *testing.T) {
	mk := func(entries []F1FamilyEntry) *F1SynthesisEnvelope {
		return &F1SynthesisEnvelope{
			SchemaVersion:    "1",
			SynthesisCycleID: "cycle-nil-vs-empty",
			Applicability:    F1ApplicabilityNotTriggered,
			Entries:          entries,
		}
	}
	dNil, err := mk(nil).ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	dEmpty, err := mk([]F1FamilyEntry{}).ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if dNil != dEmpty {
		t.Fatalf("nil-Entries and empty-Entries must share one digest; nil=%q empty=%q", dNil, dEmpty)
	}
}

// --- F7: not_applicable is a recognized per-entry disposition --------------

func TestValidateF1Envelope_NotApplicableEntryPasses(t *testing.T) {
	// The design authority (memo L113) permits an entry to carry a
	// not_applicable disposition — the family does not apply to this
	// synthesis context. A not_applicable entry carries no family summary and
	// nothing else may reference it. Build an envelope where the r3 fork is
	// not_applicable (no structural review occurred): no r3 summary, and the
	// pa probe that targeted the (now-absent) r3 option is dropped.
	env := canonicalF1Fixture()
	env.Entries[1].Triggered = F1TriggeredNotApplicable
	env.Entries[1].R3 = nil
	env.Entries[2].PA.Probes = []F1PAProbe{
		{ProbeID: "PA-P1", TargetRef: "R1C1", Result: F1PAResultNotRun},
	}
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}
	if errs := ValidateF1Envelope(env); len(errs) != 0 {
		t.Fatalf("a not_applicable r3 entry must be structurally valid; got errors:\n  %s", strings.Join(errs, "\n  "))
	}
}

func TestValidateF1Envelope_NotApplicableAccepted(t *testing.T) {
	// Regression guard: pin that not_applicable is ACCEPTED. If the vocabulary
	// were reverted to {triggered, not_triggered} only, this envelope would
	// fail with "unknown triggered". A different family (r1) is set
	// not_applicable here to vary the shape from NotApplicableEntryPasses
	// (which sets r3). The envelope is structurally valid end-to-end.
	env := canonicalF1Fixture()
	env.Entries[0].Triggered = F1TriggeredNotApplicable
	env.Entries[0].R1 = nil // not_applicable => no summary
	// r1 now has no conclusions and pa has no probes, so clear the r3
	// support/probe refs that pointed at them (this envelope is about the
	// not_applicable acceptance, not cross-ref resolution).
	env.Entries[2].PA.Probes = nil
	for i := range env.Entries[1].R3.Options {
		env.Entries[1].R3.Options[i].SupportRefs = nil
		env.Entries[1].R3.Options[i].CounterEvidenceProbeRefs = nil
	}
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}
	if errs := ValidateF1Envelope(env); len(errs) != 0 {
		t.Fatalf("not_applicable entry must be accepted with zero errors; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- prohibitive summary binding: non-triggered entries MUST NOT carry summaries ---

// TestValidateF1Envelope_NotTriggeredEntryWithSummaryFails pins the
// prohibitive reading: a not_triggered entry MUST NOT carry its matching
// family summary. A not_triggered family produced no synthesis content; a
// summary present on it is misleading state. This resolves the
// prescription-vs-permission ambiguity the committer flagged (code+doc+tests
// now agree on PROHIBITIVE per the DTO doc at f1_envelope.go F1Triggered).
func TestValidateF1Envelope_NotTriggeredEntryWithSummaryFails(t *testing.T) {
	env := canonicalF1Fixture()
	// Mark the r1 entry not_triggered but LEAVE its r1 summary in place —
	// this is the misleading state the prohibitive binding rejects.
	env.Entries[0].Triggered = F1TriggeredNotTriggered
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}
	errs := ValidateF1Envelope(env)
	if !hasErrContaining(errs, "r1 entry is not_triggered but carries an r1 summary") {
		t.Fatalf("a not_triggered entry carrying its summary must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// TestValidateF1Envelope_NotApplicableEntryWithSummaryFails is the
// not_applicable counterpart: a not_applicable entry MUST NOT carry its
// matching summary either (same prohibitive rule, different disposition).
func TestValidateF1Envelope_NotApplicableEntryWithSummaryFails(t *testing.T) {
	env := canonicalF1Fixture()
	env.Entries[2].Triggered = F1TriggeredNotApplicable
	// LEAVE the pa summary in place — the misleading state.
	d, err := env.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	env.SemanticDigest = d
	env.Validation = F1ValidationInfo{Disposition: F1ValidationComplete}
	errs := ValidateF1Envelope(env)
	if !hasErrContaining(errs, "pa entry is not_applicable but carries a pa summary") {
		t.Fatalf("a not_applicable entry carrying its summary must fail; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// --- helper ----------------------------------------------------------------

func hasErrContaining(errs []string, needle string) bool {
	for _, e := range errs {
		if strings.Contains(e, needle) {
			return true
		}
	}
	return false
}
