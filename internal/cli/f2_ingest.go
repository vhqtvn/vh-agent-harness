package cli

// f2_ingest.go — the F2 consumer-side INGEST GATE (Slice 1 of the F2 rendering
// / persistence family). This is the pure consumer: it takes a ValidatedF1Emit
// and decides whether it is safe to persist as a canonical F2 artifact. It
// implements the first three steps of the F2 deterministic ingest sequence
// (F2 memo L126-140); steps 4-6 (pair construction, collision handling, doctor
// audit) live in later slices (f2_persist.go, doctor_f2.go).
//
// DESIGN AUTHORITY: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), L120-151 (the F1→F2
// consumption contract) + L302-337 (C1/C4 resolutions verified in Slice 0).
//
// THE F2 INGEST IS A CANDIDATE-ACCEPTANCE GATE, NOT A TRANSITION AUTHORITY. It
// may refuse to produce an F2 artifact (artifact integrity); it cannot block
// another system transition (F2 memo authority-line audit, L370: "F2 input
// validation — INFORM / artifact integrity — May refuse production of an
// invalid F2 artifact; cannot block another transition").
//
// THE LOAD-BEARING FENCE (F2 memo L146-151, "F2 must NOT"): F2 never joins
// evidence, merges/splits properties, generates alternatives, generates
// counter-evidence, adds conclusions, infers gaps, reinterprets bounded
// absence as global absence, reconstructs a missing F1 entry from prose,
// treats unverified media as evidence, or emits partial as complete. The
// ingest gate enforces the last two structurally: it accepts ONLY a typed
// *ValidatedF1Emit (no prose parse path), and ONLY one whose
// validation_disposition == complete (partial is never ingested as complete).
//
// HONESTY CEILING (F2 memo L362-382): structural consistency, NOT semantic
// truth. A successfully-ingested emit is internally consistent and digest-
// bound; it is NOT thereby proven to describe conclusions that are actually
// true. Proving truth is the federated verifier's job, not an F2 ingest gate.

import (
	"fmt"
	"sort"
)

// F2IngestResult carries a successfully-ingested F1 emit ready for F2
// persistence + rendering. It is the pure output of the ingest gate (no
// filesystem, no side effects). The CanonicalEnvelope is a DEEP COPY made at
// ingest time so the caller cannot mutate the binding content under the
// captured digest (the same immutability discipline ValidatedF1Emit uses).
type F2IngestResult struct {
	// CanonicalEnvelope is a fresh deep copy of the emit's canonical envelope.
	// Mutations the caller makes to this copy do NOT re-bind the digest and do
	// NOT leak into the emit's snapshot (verified by deepCopyEnvelope).
	CanonicalEnvelope *F1SynthesisEnvelope

	// SemanticDigest is the binding digest captured from the emit. A changed
	// canonical field would produce a different digest; the ingest gate
	// re-derives and compares before capturing (step 3 of the ingest sequence).
	SemanticDigest string

	// SynthesisCycleID is the canonical cycle identity F2 must retain on every
	// persisted/rendered artifact (F2 memo L66-69, L347-350).
	SynthesisCycleID string

	// SchemaVersion is the canonical representation/schema version F2 retains
	// (F2 memo L66-69). Carried from the envelope, not invented by F2.
	SchemaVersion string

	// EntryIDs are the sorted canonical entry_id values F2 must retain (F2
	// memo L66-69, L347-350). Sorted for deterministic output.
	EntryIDs []string

	// ResolvedRefs is the cross-reference graph F2 verified at ingest. It
	// records that every R3 SupportRef / CounterEvidenceProbeRef, every P-a
	// TargetRef, and every R1 hazard consuming-ref resolves to a declared
	// canonical ID. This is the structural proof downstream rendering (P-a
	// table, R5 binding, R1 streak) relies on: every rendered ref traces to a
	// canonical entry.
	ResolvedRefs F2ResolvedRefGraph
}

// F2ResolvedRefGraph records the canonical ID sets F2 verified at ingest. It
// is the resolved cross-reference graph: every inter-family reference in the
// envelope resolves to a member of one of these sets. Downstream F2 rendering
// uses these sets to trace every rendered field back to a canonical source
// (the F2 memo's field-mapping requirement, verified in Slice 0).
type F2ResolvedRefGraph struct {
	// EntryIDs are the declared family entry_id values (one per family when
	// applicable). Sorted.
	EntryIDs []string

	// R1ConclusionIDs are the declared r1 conclusion_id values. Sorted.
	// R3 SupportRefs and P-a TargetRefs may resolve here.
	R1ConclusionIDs []string

	// R3OptionIDs are the declared r3 option_id values. Sorted. P-a TargetRefs
	// may resolve here.
	R3OptionIDs []string

	// PAProbeIDs are the declared pa probe_id values. Sorted. R3
	// CounterEvidenceProbeRefs may resolve here.
	PAProbeIDs []string
}

// IngestF1EmitForF2 is the F2 consumer-side ingest gate. It implements steps
// 1-3 of the F2 deterministic ingest sequence (F2 memo L126-140):
//
//  1. PARSE without semantic normalization. The input is a typed
//     *ValidatedF1Emit — there is NO prose parse path. F2 accepts exactly the
//     F1→F2 emit-boundary artifact (memo L122-125): no second narrative input,
//     no chat excerpt, no inferred history, no reconstructed entry, no
//     model-generated summary, no manually edited rendering field. The type
//     system enforces this: a caller CANNOT pass a string/prose blob.
//
//  2. STRUCTURALLY VALIDATE the envelope. F2 does NOT trust F1's emit claim
//     (defense-in-depth): it re-runs ValidateF1Envelope on the canonical
//     envelope independently. This catches a tampered emit whose
//     validation_disposition says "complete" but whose canonical content was
//     mutated after F1's emit (the digest would no longer bind). It also
//     requires the carried disposition == complete (partial is never ingested
//     as complete — memo L237, L150).
//
//  3. RECOMPUTE AND COMPARE THE SEMANTIC DIGEST. ValidateF1Envelope step 6
//     does this (re-derive + compare); the ingest gate surfaces it as an
//     explicit, named failure so a digest mismatch is never silently absorbed.
//     No silent normalization, no digest update, no repair — a changed
//     canonical field requires a NEW F1 emit + digest (memo L132-134).
//
// Returns (result, nil) on a clean ingest, or (nil, errs) on any failure. The
// result carries a DEEP COPY of the canonical envelope (immutability). Pure:
// no filesystem, no network, no transition, no persistence.
//
// This function offers NO operation that joins evidence, generates content,
// reconstructs an entry from prose, or repairs a digest. The only path from a
// failed ingest to a successful one is a corrected/re-validated F1 emit.
func IngestF1EmitForF2(emit *ValidatedF1Emit) (*F2IngestResult, []string) {
	// Step 1 precondition: the input must be a non-nil ValidatedF1Emit carrying
	// a canonical envelope. A nil emit is "nothing to consume" (not a panic);
	// a nil canonical envelope is a malformed emit.
	if emit == nil {
		return nil, []string{"f2 ingest: emit is nil (nothing to consume)"}
	}
	if emit.CanonicalEnvelope == nil {
		return nil, []string{"f2 ingest: emit carries no canonical envelope"}
	}

	// Step 2a: the carried validation_disposition MUST be complete. F2 never
	// ingests a partial emit as complete (memo L150, L237). This is checked
	// BEFORE re-validation so a partial emit is rejected for the right reason
	// (its own disposition), not for a downstream content error.
	if emit.ValidationDisposition != F1ValidationComplete {
		return nil, []string{fmt.Sprintf(
			"f2 ingest: validation_disposition=%q (only complete is consumable; partial must not be emitted/ingested as complete)",
			emit.ValidationDisposition)}
	}

	// Step 2b: re-validate the canonical content INDEPENDENTLY (defense-in-
	// depth). F2 does not trust F1's emit claim; it re-runs the full structural
	// validator on the canonical envelope. This catches a tampered emit (the
	// canonical content was mutated after F1's emit, so the digest no longer
	// binds) and any structural inconsistency. ValidateF1Envelope is the
	// doctor parse-site path: it checks the carried disposition is a known enum
	// too (committed-artifact defense).
	env := emit.CanonicalEnvelope
	errs := ValidateF1Envelope(env)
	if len(errs) > 0 {
		// Prefix each error so the F2 ingest surface is distinguishable from a
		// raw F1 validation call. The underlying errors are F1's (F2 does not
		// invent new structural rules; it re-applies F1's).
		out := make([]string, 0, len(errs))
		for _, e := range errs {
			out = append(out, "f2 ingest: re-validation failed: "+e)
		}
		return nil, out
	}

	// Step 3: recompute + compare the semantic digest, AND reconcile the emit's
	// claimed digest against the envelope's verified digest.
	//
	// ValidateF1Envelope step 6 already re-derived the digest from the canonical
	// content and compared it to env.SemanticDigest — so env.SemanticDigest is
	// verified to bind the content. But F2 captures the binding digest from the
	// EMIT (emit.SemanticDigest), not from the envelope. A tampered/hand-
	// constructed emit whose CanonicalEnvelope validates (its own digest binds
	// its content) but whose SemanticDigest is set to a different value would
	// otherwise pass and F2 would carry a SPOOFED binding digest that does not
	// bind the shipped envelope — breaking the F2 defense-in-depth digest-
	// binding contract (memo L130-134: recompute == supplied, no silent update,
	// no repair). Require emit.SemanticDigest == env.SemanticDigest so the
	// emit's claimed digest transitively binds the content (via the envelope's
	// already-verified digest). This is the named, non-silent failure the memo
	// requires for a digest mismatch.
	if emit.SemanticDigest != env.SemanticDigest {
		return nil, []string{fmt.Sprintf(
			"f2 ingest: emit.SemanticDigest %q != canonical envelope's semantic_digest %q (the emit's binding digest must match the envelope's verified digest; a spoofed emit digest is rejected — recompute == supplied, no silent update, no repair)",
			emit.SemanticDigest, env.SemanticDigest)}
	}
	bindingDigest := emit.SemanticDigest
	// env.SemanticDigest was verified non-empty by ValidateF1Envelope (step 6
	// rejects an empty digest), and the reconciliation above required equality,
	// so bindingDigest is non-empty here by construction. The explicit guard is
	// retained as defense-in-depth with a clear named failure.
	if bindingDigest == "" {
		return nil, []string{"f2 ingest: emit carries no semantic_digest (every persisted/rendered F2 artifact must retain the binding digest)"}
	}

	// Build the resolved cross-reference graph (the structural proof downstream
	// rendering relies on). f2ResolvedRefGraph walks the envelope and collects
	// the declared ID sets; ValidateF1Envelope already verified every inter-
	// family ref resolves to a member of one of these sets, so the graph is
	// complete by construction here.
	refs := f2ResolvedRefGraph(env)

	// Collect the entry_ids F2 must retain (sorted for deterministic output).
	entryIDs := make([]string, 0, len(env.Entries))
	for _, e := range env.Entries {
		if e.EntryID != "" {
			entryIDs = append(entryIDs, e.EntryID)
		}
	}
	sort.Strings(entryIDs)

	return &F2IngestResult{
		CanonicalEnvelope: deepCopyEnvelope(env),
		SemanticDigest:    bindingDigest,
		SynthesisCycleID:  env.SynthesisCycleID,
		SchemaVersion:     env.SchemaVersion,
		EntryIDs:          entryIDs,
		ResolvedRefs:      refs,
	}, nil
}

// f2ResolvedRefGraph walks env and collects the declared canonical ID sets:
// entry_ids, r1 conclusion_ids, r3 option_ids, pa probe_ids. Each set is
// sorted for deterministic output. This is the resolved graph downstream F2
// rendering (P-a table, R5 binding, R1 streak) traces rendered refs through to
// prove they reach a canonical source.
//
// Family-gated (mirrors resolveF1CrossRefs): only an r1 entry's R1 summary
// contributes conclusion IDs, only an r3 entry's R3 summary contributes option
// IDs, only a pa entry's PA summary contributes probe IDs. Defense-in-depth
// alongside the family↔summary binding check.
func f2ResolvedRefGraph(env *F1SynthesisEnvelope) F2ResolvedRefGraph {
	var g F2ResolvedRefGraph
	entrySeen := map[string]struct{}{}
	r1Seen := map[string]struct{}{}
	r3Seen := map[string]struct{}{}
	paSeen := map[string]struct{}{}
	add := func(dst *[]string, seen map[string]struct{}, id string) {
		if id == "" {
			return
		}
		if _, dup := seen[id]; dup {
			return
		}
		seen[id] = struct{}{}
		*dst = append(*dst, id)
	}
	for i := range env.Entries {
		e := &env.Entries[i]
		add(&g.EntryIDs, entrySeen, e.EntryID)
		switch e.Family {
		case F1FamilyR1CrossLaneJoin:
			if e.R1 != nil {
				for j := range e.R1.Conclusions {
					add(&g.R1ConclusionIDs, r1Seen, e.R1.Conclusions[j].ConclusionID)
				}
			}
		case F1FamilyR3RedesignFork:
			if e.R3 != nil {
				for j := range e.R3.Options {
					add(&g.R3OptionIDs, r3Seen, e.R3.Options[j].OptionID)
				}
			}
		case F1FamilyPACounterEvidence:
			if e.PA != nil {
				for j := range e.PA.Probes {
					add(&g.PAProbeIDs, paSeen, e.PA.Probes[j].ProbeID)
				}
			}
		}
	}
	sort.Strings(g.EntryIDs)
	sort.Strings(g.R1ConclusionIDs)
	sort.Strings(g.R3OptionIDs)
	sort.Strings(g.PAProbeIDs)
	return g
}
