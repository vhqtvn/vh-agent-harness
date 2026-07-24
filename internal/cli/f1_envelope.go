package cli

// F1 synthesis-family envelope (Slice 1: vocabulary, DTOs, canonical
// serialization, semantic-digest scope).
//
// This file declares the CLOSED vocabulary and the data-transfer objects for
// the F1SynthesisEnvelope: one versioned, domain-free envelope carrying
// exactly one entry per F1 family (r1_cross_lane_join, r3_redesign_fork,
// pa_counter_evidence). It reuses the behavioral-closure pilot PRINCIPLES
// (closed vocab, pure validator, unknown-value rejection, canonical-template
// / parser agreement, structural-consistency-≠-truth) but NOT its "absent
// token passes" behavior — at an applicable F1 seam, a missing entry is
// INCOMPLETE, not passing.
//
// The pure validator lives in f1_validator.go; the doctor audit surface lives
// in doctor_f1.go. This file holds NO report writes, transitions, network, or
// F2 rendering. It proves structural consistency, NOT that evidence or
// conclusions are true.
//
// Design authority: researches/decisions/2026-07-25-f1-synthesis-family-and-
// s2a-topology.md (amended, commit 15ddd54). The canonical-vs-derived boundary
// is encoded below: the semantic digest covers the canonical projection and
// EXCLUDES F2 view metadata (storage locator, write timestamp, view-model /
// renderer version, verified-media attachment metadata).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// --- Closed vocabulary -----------------------------------------------------

// F1Family — the three named synthesis families. Omission is NOT acceptable
// when the envelope is applicable (applicability == required): an applicable
// seam with a missing family entry is incomplete.
const (
	F1FamilyR1CrossLaneJoin   = "r1_cross_lane_join"
	F1FamilyR3RedesignFork    = "r3_redesign_fork"
	F1FamilyPACounterEvidence = "pa_counter_evidence"
)

// f1ValidFamilies is the closed set of recognized family names. Unknown
// families are rejected by the validator (unknown-value rejection).
var f1ValidFamilies = map[string]struct{}{
	F1FamilyR1CrossLaneJoin:   {},
	F1FamilyR3RedesignFork:    {},
	F1FamilyPACounterEvidence: {},
}

// F1Applicability — envelope-level applicability. The task pins exactly these
// two values: an envelope is either REQUIRED (every family must appear once)
// or NOT_TRIGGERED (the whole envelope is not applicable; entries optional).
const (
	F1ApplicabilityRequired     = "required"
	F1ApplicabilityNotTriggered = "not_triggered"
)

var f1ValidApplicabilities = map[string]struct{}{
	F1ApplicabilityRequired:     {},
	F1ApplicabilityNotTriggered: {},
}

// F1Triggered — per-entry triggered state, carried by every entry. The design
// authority (amended memo L113) permits three values: the family fired
// (triggered), its trigger condition was not met (not_triggered), or it does
// not apply to this synthesis context at all (not_applicable). A
// not_triggered / not_applicable entry carries no family summary (there is
// nothing to summarize); only a triggered entry must carry its summary.
const (
	F1TriggeredTriggered     = "triggered"
	F1TriggeredNotTriggered  = "not_triggered"
	F1TriggeredNotApplicable = "not_applicable"
)

var f1ValidTriggered = map[string]struct{}{
	F1TriggeredTriggered:     {},
	F1TriggeredNotTriggered:  {},
	F1TriggeredNotApplicable: {},
}

// F1R1JoinDisposition — how lanes joined into a conclusion (Slice 2). A MERGE
// conclusion joins >=2 lanes that addressed the SAME property into one finding;
// a UNION conclusion stands alone (one lane, distinct property) and is never
// merged with another lane's conclusion. Distinct properties are always an
// independent UNION — they are never collapsed.
const (
	F1R1JoinMerge = "merge"
	F1R1JoinUnion = "union"
)

var f1ValidR1JoinDispositions = map[string]struct{}{
	F1R1JoinMerge: {},
	F1R1JoinUnion: {},
}

// F1R3OptionMode — the two materially-distinct R3 option modes. A redesign
// candidate that merely renames, delays, or subdivides the same repair is
// INVALID (enforced by the R3 producer/gate in Slice 3, not here).
const (
	F1R3ModeContinueRepair = "continue_repair"
	F1R3ModeRedesign       = "redesign"
)

var f1ValidR3Modes = map[string]struct{}{
	F1R3ModeContinueRepair: {},
	F1R3ModeRedesign:       {},
}

// F1R3Disposition — operator disposition on the R3 fork, recorded before any
// route transition (gate-shaped conversion in Slice 3).
const (
	F1R3DispositionPending  = "pending"
	F1R3DispositionSelected = "selected"
	F1R3DispositionRejected = "rejected"
	F1R3DispositionDeferred = "deferred"
)

var f1ValidR3Dispositions = map[string]struct{}{
	F1R3DispositionPending:  {},
	F1R3DispositionSelected: {},
	F1R3DispositionRejected: {},
	F1R3DispositionDeferred: {},
}

// F1PAResult — the P-a counter-evidence probe result enum. Declared here as
// closed vocabulary so the Slice 1 envelope/validator can resolve P-a entries
// structurally; the P-a producer logic lives in Slice 4.
//
//   - found                      : requires real evidence refs.
//   - not_found_in_checked_scope : requires method + checked scope; this is
//     bounded absence, NOT global absence.
//   - unavailable                : requires an explicit limitation.
//   - not_run                    : cannot satisfy coverage.
const (
	F1PAResultFound                  = "found"
	F1PAResultNotFoundInCheckedScope = "not_found_in_checked_scope"
	F1PAResultUnavailable            = "unavailable"
	F1PAResultNotRun                 = "not_run"
)

var f1ValidPAResults = map[string]struct{}{
	F1PAResultFound:                  {},
	F1PAResultNotFoundInCheckedScope: {},
	F1PAResultUnavailable:            {},
	F1PAResultNotRun:                 {},
}

// F1ValidationDisposition — whole-envelope fail-closed disposition. The
// validator sets this: any structural error forces INCOMPLETE; only a
// clean envelope is COMPLETE. A ValidatedF1Emit (Slice 5) requires COMPLETE.
const (
	F1ValidationComplete   = "complete"
	F1ValidationIncomplete = "incomplete"
)

// f1ValidValidationDispositions is the closed set for the carried validation
// disposition. The validator rejects an unknown carried value: a committed
// artifact is untrusted input, so the doctor parse site must not accept an
// arbitrary disposition string. (AssignF1Validation always writes a valid
// value, but ValidateF1Envelope must still defend the parse site.)
var f1ValidValidationDispositions = map[string]struct{}{
	F1ValidationComplete:   {},
	F1ValidationIncomplete: {},
}

// --- DTOs ------------------------------------------------------------------

// F1SynthesisEnvelope is the single versioned, domain-free envelope carrying
// one entry per F1 family. SemanticDigest and Validation are ASSESSMENTS of
// the content, not content themselves: they are EXCLUDED from the canonical
// digest projection. F2View is F2-derived metadata and is likewise EXCLUDED.
type F1SynthesisEnvelope struct {
	SchemaVersion    string           `json:"schema_version"`
	SynthesisCycleID string           `json:"synthesis_cycle_id"`
	Applicability    string           `json:"applicability"`
	Entries          []F1FamilyEntry  `json:"entries"`
	F2View           *F2ViewMetadata  `json:"f2_view,omitempty"`
	SemanticDigest   string           `json:"semantic_digest,omitempty"`
	Validation       F1ValidationInfo `json:"validation"`
}

// F2ViewMetadata carries the F2-derived-only fields. These NEVER enter the
// semantic digest: a change to view metadata does NOT constitute a change to
// canonical content (changed canonical content requires a new synthesis
// cycle, not an in-place F2 correction).
type F2ViewMetadata struct {
	StorageLocator    string `json:"storage_locator,omitempty"`
	WriteTimestamp    string `json:"write_timestamp,omitempty"`
	ViewModelVersion  string `json:"view_model_version,omitempty"`
	RendererVersion   string `json:"renderer_version,omitempty"`
	AttachmentMetaRef string `json:"attachment_meta_ref,omitempty"`
}

// F1ValidationInfo is the structural validation result carried on the
// envelope. It is assessment, not content (excluded from the digest).
type F1ValidationInfo struct {
	Disposition string   `json:"disposition"`
	Errors      []string `json:"errors,omitempty"`
}

// F1FamilyEntry is one family's slot in the envelope. Exactly one entry per
// family is required when the envelope is applicable. The per-family summary
// pointers (R1/R3/PA) carry the cross-reference-bearing fields the validator
// resolves; family-specific producer logic lives in later slices.
type F1FamilyEntry struct {
	Family     string            `json:"family"`
	Triggered  string            `json:"triggered"`
	EntryID    string            `json:"entry_id"`
	SourceRefs []string          `json:"source_refs,omitempty"`
	R1         *F1R1JoinSummary  `json:"r1,omitempty"`
	R3         *F1R3ForkSummary  `json:"r3,omitempty"`
	PA         *F1PAProbeSummary `json:"pa,omitempty"`
}

// F1R1JoinSummary is the R1 cross-lane join slot. Slice 1 carried the
// conclusion graph (stable conclusion IDs that R3 options and P-a probes
// reference); Slice 2 adds the full join/lane/ancestry/hazard mechanics.
type F1R1JoinSummary struct {
	Conclusions []F1R1Conclusion `json:"conclusions,omitempty"`
}

// F1R1Conclusion is one material joined R1 finding: the deterministic join of
// every lane that addressed PropertyID. A MERGE conclusion joins >=2 lanes on
// the same property; a UNION conclusion stands alone (one lane, distinct
// property). Sources are ancestry-bearing: two sources sharing an ancestry
// root are NOT independent and the producer collapses them. Hazards carry the
// explicit survival chain (hazard_ref -> symptom_refs -> source_refs ->
// ancestry -> contradiction/gap -> consuming R3 option IDs -> consuming P-a
// probe IDs); survival is NOT inferred.
type F1R1Conclusion struct {
	ConclusionID    string              `json:"conclusion_id"`
	PropertyID      string              `json:"property_id"`
	JoinDisposition string              `json:"join_disposition"`
	Lanes           []F1R1LaneContrib   `json:"lanes,omitempty"`
	Sources         []F1R1Source        `json:"sources,omitempty"`
	Agreements      []string            `json:"agreements,omitempty"`
	Contradictions  []F1R1Contradiction `json:"contradictions,omitempty"`
	Gaps            []F1R1Gap           `json:"gaps,omitempty"`
	Hazards         []F1R1HazardLink    `json:"hazards,omitempty"`
}

// F1R1LaneContrib is one lane's (producer-act's) contribution to a conclusion.
type F1R1LaneContrib struct {
	LaneID   string `json:"lane_id"`
	ActID    string `json:"act_id,omitempty"`
	Position string `json:"position,omitempty"`
}

// F1R1Source is a source locator with its ancestry roots. AncestryRoots name
// the primordial sources this source descends from; two sources that share an
// ancestry root (or share a locator) are NOT independent and the producer
// collapses them into one (recording the shared root). An empty Locator is
// invalid on an evidence-bearing conclusion.
type F1R1Source struct {
	Locator       string   `json:"locator"`
	AncestryRoots []string `json:"ancestry_roots,omitempty"`
}

// F1R1Contradiction records that two lanes disagree on the property. It
// carries a stable ContradictionID so an F1R1HazardLink.ContradictionRef can
// resolve to it (a hazard survives via an explicit contradiction/gap leg).
type F1R1Contradiction struct {
	ContradictionID string `json:"contradiction_id"`
	LaneA           string `json:"lane_a"`
	LaneB           string `json:"lane_b"`
	Detail          string `json:"detail"`
}

// F1R1Gap records an unresolved aspect of a property. It carries a stable
// GapID so an F1R1HazardLink.GapRef can resolve to it.
type F1R1Gap struct {
	GapID  string `json:"gap_id"`
	Aspect string `json:"aspect"`
	Detail string `json:"detail"`
}

// F1R1HazardLink is a hazard<->symptom link with the explicit survival chain.
// The chain hazard_ref -> symptom_refs -> source_refs -> ancestry ->
// contradiction/gap -> consuming R3 option IDs -> consuming P-a probe IDs must
// be intact for the hazard to survive; survival is NOT inferred. A hazard link
// whose source locators do not resolve to the conclusion's declared sources, or
// whose consuming R3/P-a refs dangle, is a structural inconsistency.
type F1R1HazardLink struct {
	HazardRef            string   `json:"hazard_ref"`
	SymptomRefs          []string `json:"symptom_refs,omitempty"`
	SourceLocators       []string `json:"source_locators,omitempty"`
	AncestryRoots        []string `json:"ancestry_roots,omitempty"`
	ContradictionRef     string   `json:"contradiction_ref,omitempty"`
	GapRef               string   `json:"gap_ref,omitempty"`
	ConsumingR3OptionIDs []string `json:"consuming_r3_option_ids,omitempty"`
	ConsumingPAProbeIDs  []string `json:"consuming_pa_probe_ids,omitempty"`
}

// F1R3ForkSummary is the R3 repair-routing fork slot. TriggerRecognized must
// be set by the producer when repair_intent==present AND
// structural_review_outcome==non_pass (the closed-set mapping is Slice 3).
// Disposition is the operator disposition recorded before route transition.
type F1R3ForkSummary struct {
	TriggerRecognized bool         `json:"trigger_recognized"`
	Options           []F1R3Option `json:"options,omitempty"`
	Disposition       string       `json:"disposition"`
}

// F1R3Option is one repair-routing option. Each carries the canonical R3
// producer fields from the amended memo (costs/risks/reversal_cost/
// cheapest_validation) — these are CANONICAL, not F2-derived. SupportRefs
// point at R1 conclusion IDs; CounterEvidenceProbeRefs point at P-a probe IDs.
type F1R3Option struct {
	OptionID                 string   `json:"option_id"`
	Mode                     string   `json:"mode"`
	Mechanism                string   `json:"mechanism"`
	AffectedProperties       []string `json:"affected_properties,omitempty"`
	SupportRefs              []string `json:"support_refs,omitempty"`
	CounterEvidenceProbeRefs []string `json:"counter_evidence_probe_refs,omitempty"`
	Costs                    []string `json:"costs,omitempty"`
	Risks                    []string `json:"risks,omitempty"`
	ReversalCost             string   `json:"reversal_cost,omitempty"`
	CheapestValidation       string   `json:"cheapest_validation,omitempty"`
}

// F1PAProbeSummary is the P-a counter-evidence slot. Slice 1 carries the
// probe graph (probe IDs + target refs the validator resolves); Slice 4 adds
// falsification-question/scope/method/limitation/weakest-claim mechanics.
type F1PAProbeSummary struct {
	Probes []F1PAProbe `json:"probes,omitempty"`
}

// F1PAProbe is one counter-evidence probe. TargetRef points at an R1
// conclusion ID or an R3 option ID. EvidenceRefs are real evidence locators
// (fabricated/locator-free evidence is invalid under any result — Slice 4).
type F1PAProbe struct {
	ProbeID      string   `json:"probe_id"`
	TargetRef    string   `json:"target_ref"`
	Result       string   `json:"result"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

// --- Canonical serialization + semantic digest -----------------------------

// f1DigestProjection is the canonical content covered by the semantic digest.
// It deliberately OMITS F2View (F2-derived), SemanticDigest (self-reference),
// and Validation (assessment, not content). A change to any field listed here
// changes the digest; a change to any omitted field does NOT. The Entries
// field carries omitempty so that a nil slice and an empty slice produce
// identical bytes (digest stability: two equivalent not_triggered envelopes
// — one with Entries==nil, one with Entries==[] — share one digest).
type f1DigestProjection struct {
	SchemaVersion    string          `json:"schema_version"`
	SynthesisCycleID string          `json:"synthesis_cycle_id"`
	Applicability    string          `json:"applicability"`
	Entries          []F1FamilyEntry `json:"entries,omitempty"`
}

// CanonicalBytes returns the deterministic JSON serialization of the
// canonical projection (content only; F2 view metadata, the digest, and the
// validation assessment are excluded). encoding/json marshals struct fields
// in declaration order and sorts map keys, so the output is deterministic for
// a given struct value. Optional slice/string fields use omitempty so that a
// nil slice and an empty slice produce identical bytes (digest stability
// across round-trips).
func (e *F1SynthesisEnvelope) CanonicalBytes() ([]byte, error) {
	proj := f1DigestProjection{
		SchemaVersion:    e.SchemaVersion,
		SynthesisCycleID: e.SynthesisCycleID,
		Applicability:    e.Applicability,
		Entries:          e.Entries,
	}
	return json.Marshal(proj)
}

// ComputeDigest returns the hex-encoded sha256 of the canonical projection.
// It does NOT mutate the envelope; callers assign the result to
// SemanticDigest. The validator re-derives independently and compares.
func (e *F1SynthesisEnvelope) ComputeDigest() (string, error) {
	b, err := e.CanonicalBytes()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
