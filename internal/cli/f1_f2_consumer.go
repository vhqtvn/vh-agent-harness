package cli

// f1_f2_consumer.go — the F2 consumer interface over a ValidatedF1Emit (Slice 5
// of the F1 synthesis-family). This defines the CONTRACT F2 must satisfy when
// it consumes an F1 emit: read canonical content through a digest-bound view,
// attach F2-derived view metadata (the allow-list below), and never mutate
// canonical content under the same digest. F2 rendering/persistence mechanics
// (the committed Markdown projection, the streak renderer, the decision table)
// are LEFT TO THE F2 TRACK — this file defines only the consumer-side boundary.
//
// Design authority: researches/decisions/2026-07-25-f1-synthesis-family-and-
// s2a-topology.md (amended, commit 15ddd54), L216-242 (F1→F2 emit-boundary
// contract + digest-binding) + L130 (the shared-envelope canonical-vs-derived
// boundary).
//
// F2 MAY (memo L229-232): persist canonical bytes / a lossless rep; write the
// committed Markdown projection; render streak + decision-table; add storage/
// timestamp/renderer/view metadata; attach captured-or-verified media metadata;
// verify source digest matches.
//
// F2 MUST NOT (memo L234-237): join evidence; merge/split properties; generate
// alternatives; generate counter-evidence; add conclusions; infer gaps;
// reinterpret bounded absence as global absence; reconstruct missing F1 from
// prose; treat unverified media as evidence; or emit partial as complete.

import (
	"errors"
	"fmt"
	"sort"
)

// f2DerivedFieldAllowList is the closed set of F2-derived fields F2 may attach
// to a view WITHOUT entering the semantic digest (memo L130: the shared-
// envelope F2-derived surface = "storage locator, write timestamp, view-model/
// renderer version, verified-media attachment metadata"). These map 1:1 to the
// F2ViewMetadata struct fields. Adding or changing any of them does NOT change
// the digest; changing a canonical field DOES (and requires a new emit).
//
// This list is the citable allow-list the F1/F2 doctor audit names in its
// finding messages (doctor_f1.go auditF2DerivedAllowList). The allow-list
// itself is enforced structurally by re-parsing the raw f2_view object with
// json.Decoder.DisallowUnknownFields against F2ViewMetadata — keyed by the
// struct's JSON tags, not by this parallel slice, so there is a single source
// of truth (the struct) and no drift risk between the list and the struct.
var f2DerivedFieldAllowList = []string{
	"storage_locator",     // where F2 persisted the projection
	"write_timestamp",     // when F2 wrote it
	"view_model_version",  // view-model version (the projection schema version)
	"renderer_version",    // renderer version (the rendering code version)
	"attachment_meta_ref", // verified-media attachment metadata reference
}

// F2EnvelopeView is the F2 consumer's digest-bound view of a ValidatedF1Emit.
// It carries the canonical envelope (read-only by contract), the binding
// digest, the projection references (synthesis_cycle_id + entry_ids) F2 must
// retain, and the F2-derived view metadata (the allow-list above).
//
// The view offers NO operation that joins evidence, merges/splits properties,
// generates alternatives or counter-evidence, adds conclusions, or infers gaps
// (memo L234-237). The only mutation path is SetF2View, which replaces F2-
// derived metadata (excluded from the digest). There is no API path to add an
// R1 conclusion, an R3 option, or a P-a probe through this view — those are F1
// producer acts, never F2 consumer acts.
type F2EnvelopeView struct {
	emit   *ValidatedF1Emit
	f2View F2ViewMetadata
}

// ConsumeF1Emit builds an F2EnvelopeView from an emit + initial F2-derived
// view metadata. The emit must be non-nil and carry validation_disposition=
// complete (a partial emit is not consumable as complete — memo L237). The
// view holds the emit by reference; the emit's canonical envelope is already a
// deep-copied snapshot, so the producer's later mutations do not affect the
// view. The f2View is the F2-derived metadata to attach (it NEVER enters the
// semantic digest).
//
// Returns an error (not a panic) when the emit is nil or incomplete so a
// caller can distinguish "nothing to consume" from a programming bug.
func ConsumeF1Emit(emit *ValidatedF1Emit, f2View F2ViewMetadata) (*F2EnvelopeView, error) {
	if emit == nil {
		return nil, errors.New("f2: cannot consume a nil emit")
	}
	if emit.ValidationDisposition != F1ValidationComplete {
		return nil, fmt.Errorf("f2: cannot consume an emit with validation_disposition=%q (only complete is consumable; partial must not be emitted as complete)", emit.ValidationDisposition)
	}
	if emit.CanonicalEnvelope == nil {
		return nil, errors.New("f2: emit carries no canonical envelope")
	}
	return &F2EnvelopeView{emit: emit, f2View: f2View}, nil
}

// CanonicalEnvelope returns a FRESH deep copy of the canonical envelope. The
// consumer reads canonical content through this copy; any mutation the consumer
// makes to the returned copy does NOT leak into the view's binding content (the
// emit's snapshot is untouched) and does NOT re-bind the digest. This is the
// read-only contract made structural: there is no API path to mutate the emit's
// canonical content through the view.
func (v *F2EnvelopeView) CanonicalEnvelope() *F1SynthesisEnvelope {
	if v == nil || v.emit == nil {
		return nil
	}
	return deepCopyEnvelope(v.emit.CanonicalEnvelope)
}

// CanonicalDigest returns the binding semantic digest (immutable; F2 cannot
// change it through the view). A changed canonical field would produce a
// different digest; VerifyCanonical detects such drift.
func (v *F2EnvelopeView) CanonicalDigest() string {
	if v == nil || v.emit == nil {
		return ""
	}
	return v.emit.SemanticDigest
}

// SynthesisCycleID returns the projection reference F2 must retain on every
// persisted/rendered artifact (memo L239-242).
func (v *F2EnvelopeView) SynthesisCycleID() string {
	if v == nil || v.emit == nil || v.emit.CanonicalEnvelope == nil {
		return ""
	}
	return v.emit.CanonicalEnvelope.SynthesisCycleID
}

// EntryIDs returns the entry_ids the view's projection must retain (memo
// L239-242). Sorted for deterministic output. Returns nil when the envelope
// carries no entries.
func (v *F2EnvelopeView) EntryIDs() []string {
	if v == nil || v.emit == nil || v.emit.CanonicalEnvelope == nil {
		return nil
	}
	ids := make([]string, 0, len(v.emit.CanonicalEnvelope.Entries))
	for _, e := range v.emit.CanonicalEnvelope.Entries {
		if e.EntryID != "" {
			ids = append(ids, e.EntryID)
		}
	}
	sort.Strings(ids)
	return ids
}

// SetF2View replaces the F2-derived view metadata. This is the ONLY mutation
// path the view offers, and these fields NEVER enter the semantic digest
// (changing them does NOT change CanonicalDigest — verify with VerifyCanonical
// before/after). Returns the view for chaining. There is intentionally no
// setter for canonical content: canonical changes require a new F1 emit.
func (v *F2EnvelopeView) SetF2View(meta F2ViewMetadata) *F2EnvelopeView {
	if v == nil {
		return v
	}
	v.f2View = meta
	return v
}

// F2View returns the attached F2-derived view metadata.
func (v *F2EnvelopeView) F2View() F2ViewMetadata {
	if v == nil {
		return F2ViewMetadata{}
	}
	return v.f2View
}

// VerifyCanonical re-derives the semantic digest from the emit's canonical
// envelope and compares it to the binding digest. A mismatch means the
// canonical content drifted under the same digest — an F1/F2 boundary
// violation (memo L241-242: a changed canonical field requires a new F1 emit,
// not an in-place F2 correction). Returns nil if the digest still binds; a
// descriptive error otherwise.
//
// This is how the "F2 cannot replace/recalculate semantic content under the
// same digest" contract (memo L130) is made DETECTABLE: structural, not
// preventive (Go has no const), but any drift surfaces here and at the F1/F2
// doctor audit (doctor_f1.go checkF1F2Consistency). It proves structural
// consistency, NOT truth (the same honesty ceiling as behavioral-closure).
func (v *F2EnvelopeView) VerifyCanonical() error {
	if v == nil || v.emit == nil || v.emit.CanonicalEnvelope == nil {
		return errors.New("f2: view has no canonical envelope to verify")
	}
	got, err := v.emit.CanonicalEnvelope.ComputeDigest()
	if err != nil {
		return fmt.Errorf("f2: digest re-derivation failed: %v", err)
	}
	if got != v.emit.SemanticDigest {
		return fmt.Errorf("f2: canonical content drifted under the binding digest (stored %q != recomputed %q — a changed canonical field requires a new F1 emit, not an in-place F2 correction)", v.emit.SemanticDigest, got)
	}
	return nil
}
