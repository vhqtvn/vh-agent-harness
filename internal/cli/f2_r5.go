package cli

// f2_r5.go — R5 operator-synthesis durable binding (Slice 6 of the F2
// rendering/persistence family).
//
// DESIGN AUTHORITY: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), R5 contract L155-182 + C4
// resolution (L325-332).
//
// PROPERTY-IDENTITY VERDICT (HYBRID): R5 is a NEW UNION property — NOT a P2-B
// extension. P2-B's findings_delta binds AGENT closeouts. R5 binds
// OPERATOR-AUTHORED synthesis. Different subject, different cadence, different
// failure mechanism (memo L157-165).
//
// BINDING MECHANISM (memo L167-173): an operator marks/promotes authored
// synthesis → F1 admits it through the settled F1 mechanism → F1 emits a
// complete ValidatedF1Emit retaining the operator source locator and canonical
// bytes → F2 persists the emit pair and renders an addressable R5 section.
//
// F2 DOES NOT INGEST RAW CHAT (memo L171-173, L175-182). The R5 binding
// function accepts ONLY:
//   - a *ValidatedF1Emit (F1's already-validated output);
//   - a *F2R5SourceDescriptor (a typed struct whose ONLY field is the entry ID
//     that identifies the operator-source entry).
//
// ALL source data (locators, refs) is DERIVED from F1's emit — specifically
// from the identified entry's SourceRefs. There are NO caller-provided string
// fields for locators or content. A caller cannot inject raw chat prose as a
// locator because the locator is never a parameter — it is always derived from
// F1's declared SourceRefs. This is the end-to-end "raw prose rejected"
// guarantee: not just the function signature rejects strings, but every value
// in the binding traces to F1's canonical data.
//
// PROHIBITED (memo L175-182):
//   - accept an arbitrary chat excerpt as a second F2 input;
//   - summarize the operator's message anew;
//   - infer which chat passages constitute synthesis;
//   - merge several operator messages;
//   - reconstruct missing canonical text from transcript prose;
//   - treat the memo-only findings_delta syntax as an already-shipped runtime seam.
//
// R5 BINDING IS INFORM-ONLY (memo L374: "R5 binding — INFORM — Makes validated
// operator synthesis durable; does not decide or transition").

import (
	"fmt"
	"strings"
)

// --- R5 source descriptor (typed input — entry ID only) ---------------------

// F2R5SourceDescriptor is the ONLY typed input for R5 binding construction.
// It carries a SINGLE field: the entry ID that identifies the operator-source
// entry in the F1 emit.
//
// There are NO locator or content fields. ALL source data (locators, refs) is
// DERIVED from the identified entry's SourceRefs inside BuildF2R5Binding. This
// prevents a caller from injecting arbitrary prose as a locator — the locator
// is never a parameter; it always comes from F1's declared data.
//
// The "raw prose rejected" guarantee is therefore END-TO-END, not just
// signature-level: every value in the resulting F2R5Binding traces to F1's
// canonical emit (entry ID validated against the emit; locators derived from
// the entry's SourceRefs; cycle + digest from the emit).
type F2R5SourceDescriptor struct {
	// SourceEntryID must resolve to an entry in the F1 emit's envelope.
	// This is the ONLY caller-provided field. Everything else is F1-derived.
	SourceEntryID string
}

// --- R5 binding (the durable output — all fields F1-derived) ----------------

// F2R5Binding is the durable binding of an operator-source entry to a synthesis
// cycle + digest. Every field is either F1-derived (from the emit) or F2
// bookkeeping (cycle + digest binding). There are no caller-authored string
// fields — locators come from the entry's SourceRefs, not from a parameter.
//
// The binding is carried in the F2 canonical sidecar as F2-derived metadata
// (alongside F2ViewMetadata). It is rendered as an addressable section in the
// MD projection.
type F2R5Binding struct {
	// SourceEntryID is the canonical entry_id that represents the operator-
	// authored synthesis. Validated to exist in the emit.
	SourceEntryID string `json:"source_entry_id"`

	// SourceLocators are the F1-declared SourceRefs of the identified entry.
	// DERIVED from the emit — never caller-provided. These are the structural
	// references F1 retained for the operator-authored source.
	SourceLocators []string `json:"source_locators"`

	// BoundCycleID is the synthesis cycle the binding is durably attached to.
	// From the emit's SynthesisCycleID.
	BoundCycleID string `json:"bound_cycle_id"`

	// BoundDigest is the semantic digest of the emit the binding is bound to.
	// From the emit's SemanticDigest.
	BoundDigest string `json:"bound_digest"`
}

// --- R5 binding constructor -------------------------------------------------

// BuildF2R5Binding constructs an R5 binding from F1's emit + a typed source
// descriptor. This is the ONLY entry point for R5 binding construction.
//
// BEHAVIOR:
//   - source == nil → returns (nil, nil): no R5 binding. Missing operator
//     source CANNOT be reconstructed from narrative (memo L180).
//   - source.SourceEntryID does not resolve to an entry in the emit → returns
//     (nil, error): unresolved source entry.
//   - the resolved entry has no SourceRefs → returns (nil, error): F2 cannot
//     bind without F1-declared source locators (a binding with no locators
//     would be a fabricated source identity).
//   - valid source → returns a binding with locators DERIVED from the entry's
//     SourceRefs + the emit's cycle + digest.
//
// END-TO-END "RAW PROSE REJECTED": the descriptor carries ONLY an entry ID.
// Locators are derived from the entry's SourceRefs (F1-declared data). A caller
// cannot inject raw chat as a locator because there is no locator parameter.
func BuildF2R5Binding(emit *ValidatedF1Emit, source *F2R5SourceDescriptor) (*F2R5Binding, error) {
	// source == nil → no binding. Missing operator source cannot be
	// reconstructed from narrative (memo L180).
	if source == nil {
		return nil, nil
	}

	if emit == nil {
		return nil, fmt.Errorf("f2 r5: emit is nil (cannot bind without a validated emit)")
	}
	if emit.CanonicalEnvelope == nil {
		return nil, fmt.Errorf("f2 r5: emit carries no canonical envelope")
	}

	// The source entry must resolve to a real entry in the emit. DERIVE the
	// entry's SourceRefs from the emit — do NOT accept caller-provided locators.
	env := emit.CanonicalEnvelope
	var entrySourceRefs []string
	entryFound := false
	for _, entry := range env.Entries {
		if entry.EntryID == source.SourceEntryID {
			entryFound = true
			entrySourceRefs = entry.SourceRefs
			break
		}
	}
	if !entryFound {
		return nil, fmt.Errorf(
			"f2 r5: source entry %q does not resolve to any entry in the F1 emit (entries: %v) — F2 cannot bind to a non-existent canonical entry",
			source.SourceEntryID, collectEntryIDs(env))
	}

	// The entry must have F1-declared SourceRefs. Without them, F2 cannot
	// establish a source identity (a binding with no locators would be a
	// fabricated source — F2 does NOT invent locators).
	if len(entrySourceRefs) == 0 {
		return nil, fmt.Errorf(
			"f2 r5: source entry %q has no SourceRefs in the F1 emit — F2 cannot bind without F1-declared source locators (a binding requires F1 to have retained the operator source locator)",
			source.SourceEntryID)
	}

	// Construct the binding with locators DERIVED from the entry's SourceRefs.
	return &F2R5Binding{
		SourceEntryID:  source.SourceEntryID,
		SourceLocators: append([]string(nil), entrySourceRefs...), // defensive copy
		BoundCycleID:   env.SynthesisCycleID,
		BoundDigest:    emit.SemanticDigest,
	}, nil
}

// collectEntryIDs returns the entry IDs from an envelope (for diagnostics).
func collectEntryIDs(env *F1SynthesisEnvelope) []string {
	if env == nil {
		return nil
	}
	ids := make([]string, 0, len(env.Entries))
	for _, entry := range env.Entries {
		ids = append(ids, entry.EntryID)
	}
	return ids
}

// --- R5 durable-path validation gate (defense-in-depth) --------------------

// ValidateF2R5BindingAgainstEnvelope re-derives ALL binding fields from the
// canonical envelope and REJECTS any divergence. This is the durable-path
// defense-in-depth gate: even if a caller hand-constructs an F2R5Binding with
// arbitrary strings in ANY field, this gate catches it before the binding
// reaches persistence or rendering.
//
// The validation covers ALL exported fields on F2R5Binding:
//  1. SourceEntryID must resolve to an entry in the envelope;
//  2. SourceLocators must EXACTLY match the entry's SourceRefs;
//  3. BoundCycleID must equal the envelope's SynthesisCycleID;
//  4. BoundDigest must equal the envelope's SemanticDigest.
//
// Returns nil if the binding is fully consistent with the canonical envelope.
// Returns an error naming the divergence if not.
//
// This is INFORM/artifact-integrity: it may refuse to persist/render an
// inconsistent binding; it does NOT repair, normalize, or silently update.
func ValidateF2R5BindingAgainstEnvelope(binding *F2R5Binding, env *F1SynthesisEnvelope) error {
	if binding == nil {
		return nil // nil binding = no R5 data; nothing to validate
	}
	if env == nil {
		return fmt.Errorf("f2 r5 validate: envelope is nil")
	}

	// Find the entry by SourceEntryID.
	var entrySourceRefs []string
	entryFound := false
	for _, entry := range env.Entries {
		if entry.EntryID == binding.SourceEntryID {
			entryFound = true
			entrySourceRefs = entry.SourceRefs
			break
		}
	}
	if !entryFound {
		return fmt.Errorf(
			"f2 r5 validate: binding source entry %q does not resolve to any entry in the canonical envelope — a hand-constructed or stale binding cannot be persisted/rendered",
			binding.SourceEntryID)
	}

	// The binding's SourceLocators must EXACTLY match the entry's SourceRefs.
	if len(binding.SourceLocators) != len(entrySourceRefs) {
		return fmt.Errorf(
			"f2 r5 validate: binding carries %d source locators but entry %q declares %d SourceRefs — locator count mismatch (a hand-constructed binding cannot inject or omit locators)",
			len(binding.SourceLocators), binding.SourceEntryID, len(entrySourceRefs))
	}
	for i, loc := range binding.SourceLocators {
		if loc != entrySourceRefs[i] {
			return fmt.Errorf(
				"f2 r5 validate: binding source locator[%d] %q does not match entry %q SourceRef[%d] %q — a hand-constructed binding cannot substitute arbitrary strings for F1-declared locators",
				i, loc, binding.SourceEntryID, i, entrySourceRefs[i])
		}
	}

	// BoundCycleID must equal the envelope's SynthesisCycleID.
	if binding.BoundCycleID != env.SynthesisCycleID {
		return fmt.Errorf(
			"f2 r5 validate: binding BoundCycleID %q does not match envelope SynthesisCycleID %q — a hand-constructed binding cannot substitute an arbitrary cycle ID",
			binding.BoundCycleID, env.SynthesisCycleID)
	}

	// BoundDigest must equal the envelope's SemanticDigest.
	if binding.BoundDigest != env.SemanticDigest {
		return fmt.Errorf(
			"f2 r5 validate: binding BoundDigest %q does not match envelope SemanticDigest %q — a hand-constructed binding cannot substitute an arbitrary digest",
			binding.BoundDigest, env.SemanticDigest)
	}

	return nil
}

// --- R5 rendering (pure, deterministic) -------------------------------------

// renderF2R5Binding renders the R5 operator-synthesis binding section in the
// MD projection. If the binding is nil, the section renders a bounded "(no
// operator-source synthesis bound)" notice — it does NOT fabricate a binding
// or infer one from narrative.
//
// STRUCTURAL MARKERS: the section uses fenced HTML comments
// (f2-r5-binding:begin / :end) so the doctor (Slice 9) can locate it.
//
// The section is ADDRESSABLE: it carries a stable heading and the source entry
// ID + locators, so a reader can find the binding deterministically.
func renderF2R5Binding(b *strings.Builder, binding *F2R5Binding) {
	b.WriteString("<!-- f2-r5-binding:begin -->\n")
	b.WriteString("## R5 — Operator-Synthesis Durable Binding\n\n")
	b.WriteString("> Durable binding of operator-authored synthesis to this cycle + digest. F2 consumes only what F1 emitted — no raw chat, no model summarization.\n\n")

	if binding == nil {
		// Missing operator source → bounded absence. F2 does NOT infer a
		// binding from narrative or reconstruct one from prose (memo L180).
		b.WriteString("- (no operator-source synthesis bound to this cycle)\n\n")
		b.WriteString("<!-- f2-r5-binding:end -->\n\n")
		return
	}

	fmt.Fprintf(b, "- **Source entry:** `%s`\n", binding.SourceEntryID)
	if len(binding.SourceLocators) > 0 {
		fmt.Fprintf(b, "- **Source locators:** %s\n", f2RenderStringList(binding.SourceLocators))
	} else {
		b.WriteString("- **Source locators:** (none declared in canonical entry)\n")
	}
	fmt.Fprintf(b, "- **Bound to cycle:** `%s`\n", binding.BoundCycleID)
	fmt.Fprintf(b, "- **Bound to digest:** `%s`\n", binding.BoundDigest)
	b.WriteString("\n")

	b.WriteString("<!-- f2-r5-binding:end -->\n\n")
}
