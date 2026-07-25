package cli

// f2_pb.go — P-b evidence-grade media provenance slot (Slice 7 of the F2
// rendering/persistence family).
//
// DESIGN AUTHORITY: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), P-b contract L184-233.
//
// P-b is a DOMAIN-FREE CAPABILITY-CLASS SLOT (memo L186-212). It carries
// declared provenance for a media attachment that is eligible for evidence-
// grade display. The harness defines the locator shape, the capability class,
// the provenance fields, the closed structural value set, and the
// rendering/validation. The harness must NOT define vendor/transport/
// proprietary-service/model-name-as-contract (memo L208-212).
//
// VERBATIM OPERATOR REQUIREMENT (memo L214-215): capability_status ≠
// evidence-grade. Existing media-perception values (available | unavailable |
// uncertain) describe perception CAPABILITY STATUS. They do NOT mean
// true | false | authenticated. They must NOT be reused as evidence-grade or
// content-truth values. The evidence_grade field is a CLOSED SET: captured |
// verified. capability_status is a separate optional field carried alongside.
//
// FABRICATED-CHART REFUSAL (memo L217-225): a media attachment is eligible for
// evidence-grade display ONLY when ALL of:
//   - the locator is structurally valid (kind ∈ {path, url}, value non-empty);
//   - evidence_grade is captured or verified;
//   - the required provenance fields are present;
//   - the attachment is associated with a canonical F1 entry (EntryID resolves);
//   - source digest/cycle binding is retained (BoundCycleID + BoundDigest).
//
// If any is absent: F2 does NOT display the attachment as evidence-grade media.
// An invalid attachment is REJECTED at construction and at every durable path
// (persist canonical, persist pair, render MD) — the same defense-in-depth
// pattern as R5. This does NOT reject unrelated F1 entries or the canonical
// envelope: only the invalid attachment is refused.
//
// HONESTY CEILING (memo L227-233): the P-b validator establishes "required
// provenance metadata is structurally present and declares the source as
// captured or verified." It may NOT establish "the chart's numbers are real,
// accurate, unbiased, or authentically produced." Plausible fabricated metadata
// remains beyond this structural gate. This limitation appears here AND in the
// doctor documentation (Slice 9). This is the F2 specialization of the F1
// memo's fabricated-chart hard rule.
//
// P-b IS INFORM-ONLY (memo L375-377): carries declared provenance; does not
// prove media truth. Attachment refusal is INFORM / artifact integrity: omits
// structurally unverified attachment; does not reject unrelated synthesis or
// block workflow state.

import (
	"fmt"
	"strings"
)

// --- Closed value sets (memo L192, L195, L214-215) --------------------------

// F2MediaLocatorKindPath and F2MediaLocatorKindURL are the closed set of
// locator kinds (memo L192). A locator with a kind outside this set is
// structurally invalid and rejected.
const (
	F2MediaLocatorKindPath = "path"
	F2MediaLocatorKindURL  = "url"
)

// F2MediaEvidenceGradeCaptured and F2MediaEvidenceGradeVerified are the closed
// set of evidence_grade values (memo L195).
//
// VERBATIM OPERATOR REQUIREMENT (memo L214-215): capability_status values
// (available | unavailable | uncertain) are NOT evidence-grade values and must
// NOT be reused here. A descriptor with evidence_grade="available" is rejected
// — "available" is a capability status, not an evidence grade.
const (
	F2MediaEvidenceGradeCaptured = "captured"
	F2MediaEvidenceGradeVerified = "verified"
)

// --- Media attachment types (domain-free capability-class slot) --------------

// F2MediaLocator identifies where the media lives. The kind is constrained to
// {path, url} (memo L192). The value is the path or URL string.
type F2MediaLocator struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// F2MediaProvenance carries the structural provenance of a media attachment
// (memo L196-202). All fields except ContentDigest are REQUIRED for evidence-
// grade eligibility. ContentDigest is optional ("if available" — memo L201).
//
// HONESTY CEILING: the presence of these fields establishes that provenance
// metadata was DECLARED. It does NOT establish that the declared provenance is
// truthful or that the media content is authentic. Plausible fabricated
// metadata passes this structural gate (memo L227-233).
type F2MediaProvenance struct {
	SourceLocator               string `json:"source_locator"`
	CaptureOrVerificationMethod string `json:"capture_or_verification_method"`
	ObservedOrVerifiedAt        string `json:"observed_or_verified_at"`
	ProducerOrVerifierClass     string `json:"producer_or_verifier_class"`
	ContentDigest               string `json:"content_digest,omitempty"`
}

// F2MediaAttachmentDescriptor is the typed input for media attachment
// construction. It carries the operator-provided media metadata. ALL media
// data is caller-provided as TYPED STRUCT FIELDS — there is no string/prose
// parameter. The descriptor does NOT carry BoundCycleID or BoundDigest: those
// are DERIVED from the emit inside the constructor (preventing hand-construction
// of arbitrary binding values).
//
// The "no raw chat" guarantee is at the TYPE level: a caller cannot pass a
// string blob and have it interpreted as a media attachment. Each field is a
// typed, named struct member. Free-form provenance strings are accepted
// (the honesty ceiling explicitly acknowledges that plausible fabricated
// metadata is beyond the structural gate — memo L227-233).
type F2MediaAttachmentDescriptor struct {
	AttachmentID     string
	EntryID          string
	Locator          F2MediaLocator
	CapabilityClass  string
	ModalityHint     string
	EvidenceGrade    string
	Provenance       F2MediaProvenance
	CapabilityStatus string
	Limitations      string
}

// F2MediaAttachment is the durable media attachment (memo L188-206). It
// carries the descriptor fields PLUS the digest/cycle binding (BoundCycleID +
// BoundDigest) derived from the emit. These binding fields are EXPORTED (for
// JSON serialization) but are NEVER caller-provided — they are derived inside
// BuildF2MediaAttachment and re-validated at every durable path by
// ValidateF2MediaAttachmentAgainstEnvelope.
//
// VERBATIM OPERATOR REQUIREMENT (memo L214-215): EvidenceGrade is a CLOSED SET
// (captured | verified). CapabilityStatus is a SEPARATE optional field — it
// carries perception-capability values (available | unavailable | uncertain)
// and must NOT be conflated with evidence grade.
type F2MediaAttachment struct {
	AttachmentID     string            `json:"attachment_id"`
	EntryID          string            `json:"entry_id"`
	Locator          F2MediaLocator    `json:"locator"`
	CapabilityClass  string            `json:"capability_class"`
	ModalityHint     string            `json:"modality_hint,omitempty"`
	EvidenceGrade    string            `json:"evidence_grade"`
	Provenance       F2MediaProvenance `json:"provenance"`
	CapabilityStatus string            `json:"capability_status,omitempty"`
	Limitations      string            `json:"limitations,omitempty"`
	BoundCycleID     string            `json:"bound_cycle_id"`
	BoundDigest      string            `json:"bound_digest"`
}

// --- Media attachment constructor -------------------------------------------

// BuildF2MediaAttachment constructs a media attachment from F1's emit + a typed
// descriptor. This is the ONLY entry point for attachment construction.
//
// BEHAVIOR:
//   - descriptor == nil → returns (nil, nil): no attachment. Missing media
//     cannot be reconstructed from narrative (memo L180, L223-224).
//   - descriptor fields validated structurally (locator kind/value, evidence
//     grade, provenance presence, entry resolution).
//   - BoundCycleID + BoundDigest DERIVED from the emit (never from the
//     descriptor — preventing hand-construction of arbitrary binding values).
//   - valid descriptor → returns a fully-bound attachment.
//
// RAW PROSE REJECTED AT THE TYPE LEVEL: the descriptor is a typed struct with
// named fields — there is no string parameter that could carry raw chat as a
// media identity. Free-form provenance strings are accepted (the honesty
// ceiling acknowledges plausible fabricated metadata is beyond the structural
// gate — memo L227-233).
func BuildF2MediaAttachment(emit *ValidatedF1Emit, descriptor *F2MediaAttachmentDescriptor) (*F2MediaAttachment, error) {
	if descriptor == nil {
		return nil, nil
	}

	if emit == nil {
		return nil, fmt.Errorf("f2 pb: emit is nil (cannot bind media without a validated emit)")
	}
	if emit.CanonicalEnvelope == nil {
		return nil, fmt.Errorf("f2 pb: emit carries no canonical envelope")
	}

	env := emit.CanonicalEnvelope

	// Structural validation of descriptor fields (fabricated-chart refusal
	// gate — memo L217-225).

	// AttachmentID must be non-empty (addressability).
	if descriptor.AttachmentID == "" {
		return nil, fmt.Errorf("f2 pb: descriptor attachment_id is empty (cannot construct an unaddressable attachment)")
	}

	// EntryID must resolve to a canonical entry.
	entryFound := false
	for _, entry := range env.Entries {
		if entry.EntryID == descriptor.EntryID {
			entryFound = true
			break
		}
	}
	if !entryFound {
		return nil, fmt.Errorf(
			"f2 pb: descriptor entry_id %q does not resolve to any entry in the F1 emit (entries: %v) — an attachment must be associated with a canonical F1 entry",
			descriptor.EntryID, collectEntryIDs(env))
	}

	// Locator kind must be in the closed set {path, url}.
	if descriptor.Locator.Kind != F2MediaLocatorKindPath && descriptor.Locator.Kind != F2MediaLocatorKindURL {
		return nil, fmt.Errorf(
			"f2 pb: descriptor locator.kind %q is not in {path, url} — structurally invalid locator",
			descriptor.Locator.Kind)
	}
	if descriptor.Locator.Value == "" {
		return nil, fmt.Errorf("f2 pb: descriptor locator.value is empty — structurally invalid locator")
	}

	// EvidenceGrade must be in the closed set {captured, verified}.
	// VERBATIM OPERATOR REQUIREMENT: capability_status values (available |
	// unavailable | uncertain) are NOT accepted here (memo L214-215).
	if descriptor.EvidenceGrade != F2MediaEvidenceGradeCaptured && descriptor.EvidenceGrade != F2MediaEvidenceGradeVerified {
		return nil, fmt.Errorf(
			"f2 pb: descriptor evidence_grade %q is not in {captured, verified} — capability_status values (available/unavailable/uncertain) are NOT evidence grades and must not be reused as such",
			descriptor.EvidenceGrade)
	}

	// Required provenance fields must be present (memo L196-200).
	// ContentDigest is optional ("if available" — memo L201).
	if descriptor.Provenance.SourceLocator == "" {
		return nil, fmt.Errorf("f2 pb: descriptor provenance.source_locator is empty — required provenance field absent")
	}
	if descriptor.Provenance.CaptureOrVerificationMethod == "" {
		return nil, fmt.Errorf("f2 pb: descriptor provenance.capture_or_verification_method is empty — required provenance field absent")
	}
	if descriptor.Provenance.ObservedOrVerifiedAt == "" {
		return nil, fmt.Errorf("f2 pb: descriptor provenance.observed_or_verified_at is empty — required provenance field absent")
	}
	if descriptor.Provenance.ProducerOrVerifierClass == "" {
		return nil, fmt.Errorf("f2 pb: descriptor provenance.producer_or_verifier_class is empty — required provenance field absent")
	}

	// Construct the attachment with cycle/digest binding DERIVED from the emit.
	return &F2MediaAttachment{
		AttachmentID:     descriptor.AttachmentID,
		EntryID:          descriptor.EntryID,
		Locator:          descriptor.Locator,
		CapabilityClass:  descriptor.CapabilityClass,
		ModalityHint:     descriptor.ModalityHint,
		EvidenceGrade:    descriptor.EvidenceGrade,
		Provenance:       descriptor.Provenance,
		CapabilityStatus: descriptor.CapabilityStatus,
		Limitations:      descriptor.Limitations,
		BoundCycleID:     env.SynthesisCycleID,
		BoundDigest:      emit.SemanticDigest,
	}, nil
}

// --- Media attachment durable-path validation gate (defense-in-depth) --------

// ValidateF2MediaAttachmentAgainstEnvelope re-validates ALL exported fields on
// a media attachment against the canonical envelope and REJECTS any divergence.
// This is the durable-path defense-in-depth gate: even if a caller hand-
// constructs an F2MediaAttachment with arbitrary strings in ANY field, this
// gate catches it before the attachment reaches persistence or rendering.
//
// The validation covers the FABRICATED-CHART REFUSAL conditions (memo L217-225)
// PLUS the digest/cycle binding:
//  1. EntryID must resolve to an entry in the envelope;
//  2. Locator.Kind must be in {path, url}, Locator.Value non-empty;
//  3. EvidenceGrade must be in {captured, verified} (capability_status values
//     NOT accepted — memo L214-215);
//  4. Required provenance fields must be present;
//  5. BoundCycleID must equal the envelope's SynthesisCycleID;
//  6. BoundDigest must equal the envelope's SemanticDigest.
//
// HONESTY CEILING (memo L227-233): this validator establishes "required
// provenance metadata is structurally present and declares the source as
// captured or verified." It does NOT establish "the chart's numbers are real,
// accurate, unbiased, or authentically produced." Plausible fabricated metadata
// remains beyond this structural gate.
//
// Returns nil if the attachment is fully consistent. Returns an error naming
// the divergence if not.
//
// This is INFORM/artifact-integrity: it may refuse to persist/render an
// inconsistent attachment; it does NOT repair, normalize, or silently update.
func ValidateF2MediaAttachmentAgainstEnvelope(att *F2MediaAttachment, env *F1SynthesisEnvelope) error {
	if att == nil {
		return nil // nil attachment = no P-b data; nothing to validate
	}
	if env == nil {
		return fmt.Errorf("f2 pb validate: envelope is nil")
	}

	// 0. AttachmentID must be non-empty (addressability — mirrors the
	// constructor check at f2_pb.go:187-190). A hand-constructed attachment
	// with an empty AttachmentID would otherwise pass the durable gates.
	if att.AttachmentID == "" {
		return fmt.Errorf("f2 pb validate: attachment has an empty attachment_id — an unaddressable attachment cannot be persisted/rendered")
	}

	// 1. EntryID must resolve to a canonical entry.
	entryFound := false
	for _, entry := range env.Entries {
		if entry.EntryID == att.EntryID {
			entryFound = true
			break
		}
	}
	if !entryFound {
		return fmt.Errorf(
			"f2 pb validate: attachment %q entry_id %q does not resolve to any entry in the canonical envelope — a hand-constructed or stale attachment cannot be persisted/rendered",
			att.AttachmentID, att.EntryID)
	}

	// 2. Locator structural validity.
	if att.Locator.Kind != F2MediaLocatorKindPath && att.Locator.Kind != F2MediaLocatorKindURL {
		return fmt.Errorf(
			"f2 pb validate: attachment %q locator.kind %q is not in {path, url}",
			att.AttachmentID, att.Locator.Kind)
	}
	if att.Locator.Value == "" {
		return fmt.Errorf("f2 pb validate: attachment %q locator.value is empty", att.AttachmentID)
	}

	// 3. EvidenceGrade must be in {captured, verified}.
	// VERBATIM OPERATOR REQUIREMENT: capability_status values NOT accepted.
	if att.EvidenceGrade != F2MediaEvidenceGradeCaptured && att.EvidenceGrade != F2MediaEvidenceGradeVerified {
		return fmt.Errorf(
			"f2 pb validate: attachment %q evidence_grade %q is not in {captured, verified} — capability_status values are NOT evidence grades",
			att.AttachmentID, att.EvidenceGrade)
	}

	// 4. Required provenance fields present.
	if att.Provenance.SourceLocator == "" {
		return fmt.Errorf("f2 pb validate: attachment %q provenance.source_locator is empty", att.AttachmentID)
	}
	if att.Provenance.CaptureOrVerificationMethod == "" {
		return fmt.Errorf("f2 pb validate: attachment %q provenance.capture_or_verification_method is empty", att.AttachmentID)
	}
	if att.Provenance.ObservedOrVerifiedAt == "" {
		return fmt.Errorf("f2 pb validate: attachment %q provenance.observed_or_verified_at is empty", att.AttachmentID)
	}
	if att.Provenance.ProducerOrVerifierClass == "" {
		return fmt.Errorf("f2 pb validate: attachment %q provenance.producer_or_verifier_class is empty", att.AttachmentID)
	}

	// 5. BoundCycleID must equal the envelope's SynthesisCycleID.
	if att.BoundCycleID != env.SynthesisCycleID {
		return fmt.Errorf(
			"f2 pb validate: attachment %q BoundCycleID %q does not match envelope SynthesisCycleID %q — a hand-constructed attachment cannot substitute an arbitrary cycle ID",
			att.AttachmentID, att.BoundCycleID, env.SynthesisCycleID)
	}

	// 6. BoundDigest must equal the envelope's SemanticDigest.
	if att.BoundDigest != env.SemanticDigest {
		return fmt.Errorf(
			"f2 pb validate: attachment %q BoundDigest does not match envelope SemanticDigest — a hand-constructed attachment cannot substitute an arbitrary digest",
			att.AttachmentID)
	}

	return nil
}

// --- Media attachment rendering (pure, deterministic) -----------------------

// renderF2MediaAttachments renders the P-b evidence-grade media provenance
// section in the MD projection. If the attachment list is empty/nil, the
// section renders a bounded "(no evidence-grade media attachments)" notice —
// it does NOT fabricate an attachment or infer one from narrative.
//
// STRUCTURAL MARKERS: the section uses fenced HTML comments
// (f2-pb-media:begin / :end) so the doctor (Slice 9) can locate it.
//
// HONESTY CEILING (rendered in the section notice): the displayed provenance
// is structurally present — it is NOT verified for content truth. The MD
// projection carries this caveat so a reader does not mistake structural
// presence for semantic truth.
func renderF2MediaAttachments(b *strings.Builder, attachments []F2MediaAttachment) {
	b.WriteString("<!-- f2-pb-media:begin -->\n")
	b.WriteString("## P-b — Evidence-Grade Media Provenance\n\n")
	b.WriteString("> Media attachments eligible for evidence-grade display. Provenance is structurally present — **content truth is NOT verified** (plausible fabricated metadata passes this gate). capability_status ≠ evidence-grade.\n\n")

	if len(attachments) == 0 {
		b.WriteString("- (no evidence-grade media attachments bound to this cycle)\n\n")
		b.WriteString("<!-- f2-pb-media:end -->\n\n")
		return
	}

	for _, att := range attachments {
		fmt.Fprintf(b, "### Attachment `%s`\n\n", att.AttachmentID)
		fmt.Fprintf(b, "- **Entry:** `%s`\n", att.EntryID)
		fmt.Fprintf(b, "- **Locator:** `%s` `%s`\n", att.Locator.Kind, att.Locator.Value)
		if att.CapabilityClass != "" {
			fmt.Fprintf(b, "- **Capability class:** `%s`\n", att.CapabilityClass)
		}
		if att.ModalityHint != "" {
			fmt.Fprintf(b, "- **Modality hint:** %s\n", att.ModalityHint)
		}
		// EvidenceGrade rendered EXACTLY (captured | verified — never
		// paraphrased or conflated with capability_status).
		fmt.Fprintf(b, "- **Evidence grade:** `%s`\n", att.EvidenceGrade)

		// Provenance (structurally present — content truth NOT verified).
		b.WriteString("- **Provenance:**\n")
		fmt.Fprintf(b, "  - Source locator: `%s`\n", att.Provenance.SourceLocator)
		fmt.Fprintf(b, "  - Capture/verification method: %s\n", att.Provenance.CaptureOrVerificationMethod)
		fmt.Fprintf(b, "  - Observed/verified at: `%s`\n", att.Provenance.ObservedOrVerifiedAt)
		fmt.Fprintf(b, "  - Producer/verifier class: `%s`\n", att.Provenance.ProducerOrVerifierClass)
		if att.Provenance.ContentDigest != "" {
			fmt.Fprintf(b, "  - Content digest: `%s`\n", att.Provenance.ContentDigest)
		}

		// CapabilityStatus is OPTIONAL and SEPARATE from evidence grade.
		// Rendered only if present; clearly labeled as capability status,
		// NOT evidence grade.
		if att.CapabilityStatus != "" {
			fmt.Fprintf(b, "- **Capability status:** `%s` (perception capability — NOT evidence grade)\n", att.CapabilityStatus)
		}
		if att.Limitations != "" {
			fmt.Fprintf(b, "- **Limitations:** %s\n", att.Limitations)
		}

		// Binding metadata.
		fmt.Fprintf(b, "- **Bound to cycle:** `%s`\n", att.BoundCycleID)
		fmt.Fprintf(b, "- **Bound to digest:** `%s`\n", att.BoundDigest)
		b.WriteString("\n")
	}

	b.WriteString("<!-- f2-pb-media:end -->\n\n")
}
