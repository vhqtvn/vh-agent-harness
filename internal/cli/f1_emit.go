package cli

// f1_emit.go — the F1→F2 emit-boundary producer (Slice 5 of the F1 synthesis-
// family). ValidatedF1Emit is the candidate/informational artifact F1 emits
// for F2 to consume (memo L216-227). It carries the canonical envelope (deep-
// copied at emit time so the producer's later mutations do not leak into the
// snapshot), the binding semantic digest, and the validation disposition
// (always "complete" — partial is never emitted as complete).
//
// This is a CANDIDATE / INFORMATIONAL artifact with NO TRANSITION AUTHORITY —
// consistent with the model-output-is-candidate invariant. Emitting it does
// not transition any repo state, persist anything, or grant F2 the right to
// mutate canonical content. The F2 consumer interface lives in
// f1_f2_consumer.go.
//
// Design authority: researches/decisions/2026-07-25-f1-synthesis-family-and-
// s2a-topology.md (amended, commit 15ddd54), L216-242 (F1→F2 emit-boundary
// contract + digest-binding).
//
// This file holds NO report writes, transitions, network, F2 rendering, or
// claims DB. It proves structural completeness, NOT that the evidence or
// conclusions are true (the same honesty ceiling as behavioral-closure).

import (
	"fmt"
)

// ValidatedF1Emit is the F1→F2 emit-boundary artifact. It is a CANDIDATE /
// INFORMATIONAL artifact with NO transition authority.
//
//   - CanonicalEnvelope is a DEEP COPY of the producer's envelope made at emit
//     time. The producer's later mutations do not leak into the emitted
//     snapshot (immutability). F2 reads canonical content through this copy.
//   - SemanticDigest is the sha256 of the canonical projection at emit time. It
//     BINDS the canonical content: a changed canonical field produces a
//     different digest and requires a NEW emit (a new synthesis_cycle_id +
//     re-derived digest), NEVER an in-place F2 correction (memo L241-242).
//   - ValidationDisposition is always F1ValidationComplete here — EmitF1
//     refuses to emit an incomplete envelope (memo L237: F2 must not "emit
//     partial as complete"). The field is carried explicitly so a consumer can
//     assert the disposition without re-validating.
type ValidatedF1Emit struct {
	CanonicalEnvelope     *F1SynthesisEnvelope
	SemanticDigest        string
	ValidationDisposition string
}

// EmitF1 is the F1 producer→emit function. It validates the envelope's
// canonical content (AssignF1Validation: the 9 F1 preconditions from memo
// L223-227 — one entry per family, applicability explicit, each entry
// validates independently, cross-refs resolve, R1 joins complete, every R1
// conclusion + R3 option has P-a coverage, a triggered R3 has materially-
// distinct options, conflicts/gaps explicit, digest covers canonical fields),
// assigns the fail-closed disposition, and — ONLY when disposition==complete —
// returns a ValidatedF1Emit carrying a DEEP COPY of the envelope + the binding
// digest.
//
// An incomplete envelope (any structural error, a missing family entry, an
// uncovered P-a target, a stale digest, ...) CANNOT be emitted as complete:
// EmitF1 returns (nil, errs). Partial fragments never become a complete emit.
//
// Pure w.r.t. canonical content and external side effects: it mutates ONLY
// env.Validation (the assessment field, never canonical content), and performs
// no filesystem, network, transition, or persistence. The emit is a candidate
// the caller (or F2) may consume; it does not itself act. The coordinator has
// NO transition authority (memo L252-254).
func EmitF1(env *F1SynthesisEnvelope) (*ValidatedF1Emit, []string) {
	if env == nil {
		return nil, []string{"envelope is nil"}
	}
	// AssignF1Validation validates canonical content and assigns the
	// disposition from the error count (complete iff zero errors). It mutates
	// ONLY env.Validation (assessment), never canonical content.
	errs := AssignF1Validation(env)
	if len(errs) > 0 {
		// Partial cannot be emitted as complete (memo L237). Return the errors
		// so the caller sees exactly which precondition failed; no emit.
		return nil, errs
	}
	// Complete: the digest on env was already verified by AssignF1Validation
	// (check #6 in validateF1EnvelopeContent re-derives and compares). Carry it
	// as the binding digest.
	return &ValidatedF1Emit{
		CanonicalEnvelope:     deepCopyEnvelope(env),
		SemanticDigest:        env.SemanticDigest,
		ValidationDisposition: F1ValidationComplete,
	}, nil
}

// CanonicalBytes returns the deterministic canonical projection of the emitted
// envelope (content only; F2 view metadata, the digest, and the validation
// assessment are excluded). This is the lossless rep F2 may persist (memo
// L229): a round-trip through CanonicalBytes -> parse -> consume reproduces the
// canonical content the digest binds. The returned bytes are computed from the
// emit's deep-copied snapshot, so they are stable regardless of later producer
// mutations.
func (e *ValidatedF1Emit) CanonicalBytes() ([]byte, error) {
	if e == nil || e.CanonicalEnvelope == nil {
		return nil, fmt.Errorf("f1 emit: no canonical envelope")
	}
	return e.CanonicalEnvelope.CanonicalBytes()
}

// deepCopyEnvelope returns a fully independent deep copy of env (every slice
// field + nested summary reallocated). Used by EmitF1 so the emitted snapshot
// is immutable: the producer's later mutations of env do not leak into the
// ValidatedF1Emit's canonical envelope, and an F2 consumer reading through the
// view cannot mutate the emit's binding content. Reuses the per-family
// deep-copy helpers (deepCopyR1 / deepCopyR3Fork / deepCopyPA) so a new DTO
// field added to a family summary is preserved exactly when its family's
// deep-copy helper is kept in sync (the Slice-3 deepCopy trap discipline).
func deepCopyEnvelope(env *F1SynthesisEnvelope) *F1SynthesisEnvelope {
	if env == nil {
		return nil
	}
	out := &F1SynthesisEnvelope{
		SchemaVersion:    env.SchemaVersion,
		SynthesisCycleID: env.SynthesisCycleID,
		Applicability:    env.Applicability,
		Entries:          make([]F1FamilyEntry, len(env.Entries)),
		SemanticDigest:   env.SemanticDigest,
		Validation: F1ValidationInfo{
			Disposition: env.Validation.Disposition,
			Errors:      copyStrings(env.Validation.Errors),
		},
	}
	if env.F2View != nil {
		out.F2View = &F2ViewMetadata{
			StorageLocator:    env.F2View.StorageLocator,
			WriteTimestamp:    env.F2View.WriteTimestamp,
			ViewModelVersion:  env.F2View.ViewModelVersion,
			RendererVersion:   env.F2View.RendererVersion,
			AttachmentMetaRef: env.F2View.AttachmentMetaRef,
		}
	}
	for i, e := range env.Entries {
		out.Entries[i] = F1FamilyEntry{
			Family:     e.Family,
			Triggered:  e.Triggered,
			EntryID:    e.EntryID,
			SourceRefs: copyStrings(e.SourceRefs),
			R1:         deepCopyR1(e.R1),
			R3:         deepCopyR3Fork(e.R3),
			PA:         deepCopyPA(e.PA),
		}
	}
	return out
}
