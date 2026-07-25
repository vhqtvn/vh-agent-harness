package cli

// f2_persist.go — immutable canonical sidecar persistence (Slice 2 of the F2
// rendering / persistence family). This file implements step 4-5 of the F2
// deterministic ingest sequence for the CANONICAL sidecar only (the .md
// projection + pair-level coordination land in Slice 3; the doctor audit lands
// in Slice 9).
//
// DESIGN AUTHORITY: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (commit 605f406, amended 4029e42), Decision 1 (L52-89) +
// ingest steps 4-5 (L135-139).
//
// ARCHITECTURE (Decision 1):
//   docs/checkpoints/f2/<synthesis_cycle_id>.canonical.json   # THIS FILE
//   docs/checkpoints/f2/<synthesis_cycle_id>.md               # Slice 3
//
// The canonical sidecar is "what F1 said" — the lossless, digest-bound F1
// evidence carried into durability. It is IMMUTABLE: once written for a cycle,
// it is never silently overwritten. The collision contract (memo L137-139):
//   - neither canonical.json nor .md exists  → write (this slice: canonical
//     does not exist → write);
//   - both exist byte-identical              → idempotent no-op (this slice:
//     canonical exists and canonical content is byte-identical → no-op);
//   - either exists different                → refuse, report new cycle
//     required (this slice: canonical exists but content differs → refuse);
//   - only one exists                         → incomplete pair (pair-level;
//     detected by Slice 3 coordination + Slice 9 doctor).
//
// F2 PERSISTENCE IS INFORM-ONLY (memo L372: "Canonical JSON persistence —
// INFORM — Lossless storage of F1 output; no independent conclusion"). It may
// refuse to produce/overwrite an artifact (artifact integrity); it cannot
// block another system transition.
//
// THE LOAD-BEARING FENCE (memo L146-151): this file offers NO operation that
// joins evidence, merges/splits properties, generates content, reconstructs an
// entry from prose, repairs a digest, or emits partial as complete. The only
// mutation is writing a NEW canonical sidecar for a cycle that does not yet
// have one (or confirming idempotency for one that does).
//
// HONESTY CEILING (memo L362-382): a successfully-persisted sidecar is
// internally consistent and digest-bound; it is NOT thereby proven to describe
// conclusions that are actually true. Persistence is structural, not semantic
// verification.
//
// ---------------------------------------------------------------------------
// ROUTING NOTE (advisory from Slice 1 commit review): two F2-side entry
// surfaces exist over ValidatedF1Emit.
//   - ConsumeF1Emit / F2EnvelopeView (f1_f2_consumer.go) — F1-SIDE consumer
//     boundary. Holds the emit BY REFERENCE. Used by F1's own F2-view metadata
//     attachment and the F1/F2 doctor digest-binding audit (checkF1F2Consist-
//     ency). Does NOT deep-copy at consumption time (the emit's envelope is
//     already a deep-copied snapshot from EmitF1). Read-only access via
//     CanonicalEnvelope() returns a fresh deep copy on each call.
//   - IngestF1EmitForF2 / F2IngestResult (f2_ingest.go) — F2-SIDE ingest gate.
//     Deep-copies the envelope into the result at ingest time. Used by F2
//     PERSISTENCE + RENDERING (this file and downstream slices). The result
//     carries the resolved cross-reference graph and the binding digest.
// They do NOT collide (no symbol overlap). Use ConsumeF1Emit for F1-side view
// metadata + doctor audit; use IngestF1EmitForF2 for F2 persistence/rendering.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// --- F2 version constants (carried as F2 view metadata on every artifact) --

// F2CanonicalRepresentationVersion identifies the canonical sidecar format
// version. Bumped when the sidecar's wrapper structure changes (NOT when the
// F1 envelope schema changes — that is SchemaVersion, carried from the
// envelope). The canonical content is always the lossless F1SynthesisEnvelope.
const F2CanonicalRepresentationVersion = "1"

// F2ProjectionVersion identifies the Markdown projection format version
// (used by Slice 3's renderer). Declared here so the canonical sidecar can
// carry it as view metadata per memo L66-69 ("both artifacts retain ...
// projection/renderer version").
const F2ProjectionVersion = "1"

// F2RendererVersion identifies the Markdown renderer code version (used by
// Slice 3's renderer). Declared here for the same reason as F2ProjectionVersion.
const F2RendererVersion = "1"

// F2CanonicalSidecarKind is the discriminator value written in the sidecar's
// top-level "kind" field, so readers (doctor, streak scanner) can distinguish
// an F2 canonical sidecar from arbitrary JSON at the same path.
const F2CanonicalSidecarKind = "f2_canonical_sidecar"

// --- DTOs -------------------------------------------------------------------

// F2CanonicalSidecar is the persisted representation of a validated F1 emit.
// It wraps the lossless F1SynthesisEnvelope (the canonical, digest-bound F1
// evidence) with F2 view metadata (bookkeeping that is NOT canonical F1
// evidence and NOT digest-bound — memo L66-69).
//
// The CanonicalEnvelope is "what F1 said"; the F2ViewMetadata is "what F2
// recorded about the pair." The two are structurally separated so the doctor
// (Slice 9) can re-derive the digest from CanonicalEnvelope alone and compare
// it to F2ViewMetadata.SourceSemanticDigest.
type F2CanonicalSidecar struct {
	// Kind is always F2CanonicalSidecarKind. Lets readers identify the format.
	Kind string `json:"kind"`

	// CanonicalEnvelope is the lossless F1SynthesisEnvelope as emitted by F1.
	// This is the digest-bound canonical content. It is never mutated by F2.
	CanonicalEnvelope *F1SynthesisEnvelope `json:"canonical_envelope"`

	// F2ViewMetadata is F2 bookkeeping carried alongside the canonical content.
	// These fields NEVER enter the F1 semantic digest (memo L66-69: "F2 view
	// metadata, not canonical F1 evidence").
	F2ViewMetadata F2ArtifactViewMeta `json:"f2_view_metadata"`

	// R5Binding is the optional operator-synthesis durable binding (Slice 6).
	// nil when no operator-source was bound. F2-derived metadata — NOT part of
	// the canonical fingerprint (the collision key is the envelope content
	// alone). Carried here so the binding is durable (persisted with the pair).
	R5Binding *F2R5Binding `json:"r5_binding,omitempty"`

	// MediaAttachments are the optional P-b evidence-grade media provenance
	// slots (Slice 7). nil/empty when no attachments were declared. F2-derived
	// metadata — NOT part of the canonical fingerprint. Carried here so the
	// attachments are durable (persisted with the pair).
	MediaAttachments []F2MediaAttachment `json:"media_attachments,omitempty"`
}

// F2ArtifactViewMeta is the F2 bookkeeping carried on BOTH the canonical
// sidecar and the Markdown projection (memo L66-69). It records the pair
// identity (cycle, entries, digest), the format/renderer versions, the
// reciprocal locator back to the canonical pair, and the write timestamp.
//
// None of these fields are canonical F1 evidence. A change to any of them does
// NOT change the semantic digest; a change to a canonical field DOES (and
// requires a new F1 emit + cycle).
type F2ArtifactViewMeta struct {
	// SynthesisCycleID is the canonical cycle identity (from the envelope).
	SynthesisCycleID string `json:"synthesis_cycle_id"`

	// EntryIDs are the sorted canonical entry_id values F2 must retain.
	EntryIDs []string `json:"entry_ids"`

	// SourceSemanticDigest is the binding digest from the F1 emit. The doctor
	// re-derives the digest from CanonicalEnvelope and compares it here.
	SourceSemanticDigest string `json:"source_semantic_digest"`

	// CanonicalRepresentationVersion is the sidecar wrapper format version.
	CanonicalRepresentationVersion string `json:"canonical_representation_version"`

	// SchemaVersion is the F1 envelope schema version (carried from the
	// envelope, not invented by F2).
	SchemaVersion string `json:"schema_version"`

	// ProjectionVersion is the Markdown projection format version.
	ProjectionVersion string `json:"projection_version"`

	// RendererVersion is the Markdown renderer code version.
	RendererVersion string `json:"renderer_version"`

	// ReciprocalLocator is the relative path back to the .md projection member
	// of the pair: "<dir>/<cycle>.md". Lets a reader of the canonical sidecar
	// find the projection and vice versa.
	ReciprocalLocator string `json:"reciprocal_locator"`

	// WriteTimestamp is the RFC3339 UTC timestamp when F2 wrote this artifact.
	// It is F2 view metadata: two writes of the same canonical content at
	// different times are idempotent (the canonical content match governs, not
	// the timestamp).
	WriteTimestamp string `json:"write_timestamp"`
}

// F2PersistOutcome reports what PersistF2CanonicalSidecar did.
type F2PersistOutcome int

const (
	// F2PersistNotAttempted is the zero value (should not appear in practice).
	F2PersistNotAttempted F2PersistOutcome = iota

	// F2PersistWritten means the canonical sidecar did not exist and was
	// freshly written.
	F2PersistWritten

	// F2PersistIdempotent means the canonical sidecar already existed and its
	// canonical content is byte-identical to what would be written. No file
	// modification occurred (the original timestamp and bytes are preserved).
	F2PersistIdempotent

	// F2PersistRefused means the canonical sidecar already existed but its
	// canonical content DIFFERS from what would be written. F2 refused to
	// overwrite (immutability). A new synthesis cycle is required.
	F2PersistRefused
)

// String returns a human-readable outcome name for diagnostics and tests.
func (o F2PersistOutcome) String() string {
	switch o {
	case F2PersistWritten:
		return "written"
	case F2PersistIdempotent:
		return "idempotent"
	case F2PersistRefused:
		return "refused"
	default:
		return "not_attempted"
	}
}

// --- Serialization ----------------------------------------------------------

// SerializeF2CanonicalSidecar produces the deterministic JSON bytes for the
// full sidecar (canonical envelope + F2 view metadata). encoding/json marshals
// struct fields in declaration order; the envelope and all nested types are
// pure structs with no map fields, so the output is deterministic for a given
// struct value (verified: the only maps in the package are vocab lookup
// tables, not data fields).
//
// The output uses 2-space indentation for human readability. Two calls with
// the same sidecar value produce identical bytes (deterministic serialization
// — required for byte-stable reruns and collision detection).
func SerializeF2CanonicalSidecar(sidecar *F2CanonicalSidecar) ([]byte, error) {
	return json.MarshalIndent(sidecar, "", "  ")
}

// f2CanonicalContentFingerprint returns deterministic JSON bytes for the
// canonical envelope portion of a sidecar — the lossless F1 evidence that
// the semantic digest binds. Two sidecars with the same fingerprint carry the
// same canonical F1 content.
//
// This is the COLLISION KEY: the immutability contract compares canonical
// content, NOT the full file (which includes the write timestamp — two writes
// of the same cycle at different times must be idempotent, and the timestamp
// is F2 view metadata that does not affect canonical identity).
//
// The full envelope is compared (not just f1DigestProjection) because the
// canonical sidecar is the lossless emit: any difference in the emit (including
// F2View, SemanticDigest, or Validation carried by F1) is a different artifact
// for the same cycle, and immutability requires refusing the overwrite.
func f2CanonicalContentFingerprint(env *F1SynthesisEnvelope) ([]byte, error) {
	return json.Marshal(env)
}

// --- Sidecar construction ---------------------------------------------------

// buildF2CanonicalSidecar assembles the sidecar from an ingest result. Pure:
// no filesystem access. The reciprocal locator is derived from dir + cycle_id
// so a reader of the canonical sidecar can find the .md projection.
func buildF2CanonicalSidecar(ingest *F2IngestResult, dir string, now time.Time) *F2CanonicalSidecar {
	cycle := ingest.SynthesisCycleID
	return &F2CanonicalSidecar{
		Kind:              F2CanonicalSidecarKind,
		CanonicalEnvelope: ingest.CanonicalEnvelope,
		F2ViewMetadata: F2ArtifactViewMeta{
			SynthesisCycleID:               cycle,
			EntryIDs:                       ingest.EntryIDs,
			SourceSemanticDigest:           ingest.SemanticDigest,
			CanonicalRepresentationVersion: F2CanonicalRepresentationVersion,
			SchemaVersion:                  ingest.SchemaVersion,
			ProjectionVersion:              F2ProjectionVersion,
			RendererVersion:                F2RendererVersion,
			ReciprocalLocator:              filepath.Join(dir, cycle+".md"),
			WriteTimestamp:                 now.UTC().Format(time.RFC3339),
		},
		R5Binding:        ingest.R5Binding,
		MediaAttachments: ingest.MediaAttachments,
	}
}

// F2CanonicalSidecarPath returns the canonical sidecar file path for a cycle
// within a directory. Exported so the doctor (Slice 9) and the streak scanner
// (Slice 8) use the same path convention.
func F2CanonicalSidecarPath(dir, cycleID string) string {
	return filepath.Join(dir, cycleID+".canonical.json")
}

// --- Persistence (collision-handled) ----------------------------------------

// PersistF2CanonicalSidecar writes the canonical sidecar for the ingest
// result's synthesis cycle, enforcing the immutability collision contract
// (memo L137-139):
//
//   - canonical.json does not exist → write it (F2PersistWritten);
//   - canonical.json exists and canonical content is byte-identical →
//     idempotent no-op, the file is NOT touched (F2PersistIdempotent);
//   - canonical.json exists but canonical content differs → refuse, do NOT
//     overwrite (F2PersistRefused). A new synthesis cycle is required.
//
// The collision key is the canonical envelope content (f2CanonicalContent-
// Fingerprint), NOT the full file: two writes of the same cycle at different
// times share the same canonical content and are idempotent regardless of the
// write timestamp (which is F2 view metadata, not canonical identity).
//
// `now` is injected (not time.Now()) so tests are deterministic. Production
// callers pass time.Now().
//
// Returns (outcome, nil) on a handled result (written or idempotent), or
// (F2PersistRefused, error) when the collision refuses the overwrite, or
// (F2PersistNotAttempted, error) on an I/O or serialization failure.
//
// F2 NEVER repairs, normalizes, or silently updates. The only recovery from a
// refused overwrite is a new F1 emit under a new cycle ID.
func PersistF2CanonicalSidecar(ingest *F2IngestResult, dir string, now time.Time) (F2PersistOutcome, error) {
	if ingest == nil {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: ingest result is nil (nothing to persist)")
	}
	if ingest.CanonicalEnvelope == nil {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: ingest result carries no canonical envelope")
	}
	if ingest.SynthesisCycleID == "" {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: ingest result carries no synthesis_cycle_id (cannot derive sidecar path)")
	}

	// R5 binding validation gate (defense-in-depth): if the ingest carries an
	// R5 binding, its SourceLocators must EXACTLY match the canonical entry's
	// SourceRefs. A hand-constructed binding with arbitrary strings is rejected
	// here — it never reaches the durable artifact.
	if ingest.R5Binding != nil {
		if vErr := ValidateF2R5BindingAgainstEnvelope(ingest.R5Binding, ingest.CanonicalEnvelope); vErr != nil {
			return F2PersistNotAttempted, fmt.Errorf("f2 persist: R5 binding validation failed (durable-path gate): %w", vErr)
		}
	}

	// P-b media attachment validation gate (defense-in-depth): if the ingest
	// carries media attachments, each is structurally validated against the
	// canonical envelope. A hand-constructed attachment with arbitrary strings
	// is rejected here — it never reaches the durable artifact.
	for i := range ingest.MediaAttachments {
		if vErr := ValidateF2MediaAttachmentAgainstEnvelope(&ingest.MediaAttachments[i], ingest.CanonicalEnvelope); vErr != nil {
			return F2PersistNotAttempted, fmt.Errorf("f2 persist: media attachment[%d] validation failed (durable-path gate): %w", i, vErr)
		}
	}

	sidecar := buildF2CanonicalSidecar(ingest, dir, now)
	newBytes, err := SerializeF2CanonicalSidecar(sidecar)
	if err != nil {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: failed to serialize canonical sidecar: %w", err)
	}

	newFP, err := f2CanonicalContentFingerprint(sidecar.CanonicalEnvelope)
	if err != nil {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: failed to compute canonical fingerprint: %w", err)
	}

	path := F2CanonicalSidecarPath(dir, ingest.SynthesisCycleID)

	// Ensure the parent directory exists before attempting the atomic create.
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: cannot create canonical sidecar directory %q: %w", dir, mkErr)
	}

	// ATOMIC CREATE (O_EXCL): the file is created ONLY if it does not already
	// exist. This closes the TOCTOU window that a read-then-write sequence
	// (os.ReadFile → os.WriteFile) would leave open: two concurrent persisters
	// for the same cycle could both observe ErrNotExist and the second would
	// silently overwrite the first via O_TRUNC. O_EXCL makes the create
	// exclusive: at most one persister wins the create; the other gets
	// os.IsExist and falls through to the collision check below.
	fd, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if createErr == nil {
		// We exclusively created the file — write the sidecar bytes. No other
		// process could have the file open for writing (O_EXCL guarantees it
		// did not exist a moment ago).
		if _, wErr := fd.Write(newBytes); wErr != nil {
			fd.Close()
			return F2PersistNotAttempted, fmt.Errorf("f2 persist: cannot write canonical sidecar at %q (exclusive create succeeded but write failed): %w", path, wErr)
		}
		if cErr := fd.Close(); cErr != nil {
			return F2PersistNotAttempted, fmt.Errorf("f2 persist: cannot close canonical sidecar at %q after write: %w", path, cErr)
		}
		return F2PersistWritten, nil
	}

	// O_EXCL failed. If it is NOT "already exists", surface the error (perms,
	// read-only FS, etc.). Do NOT fall back to a non-exclusive write —
	// immutability forbids silently overwriting an artifact we could not
	// exclusively create.
	if !os.IsExist(createErr) {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: cannot exclusively create canonical sidecar at %q: %w", path, createErr)
	}

	// File exists (created by a prior persister or was already there) → apply
	// the collision check. Read the existing content.
	existing, readErr := os.ReadFile(path)
	if readErr != nil {
		// An unreadable existing file is NOT silently overwritten
		// (immutability). Surface the I/O error; the operator must
		// investigate. This is distinct from a content refusal.
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: cannot read existing canonical sidecar at %q (refusing to overwrite an unreadable artifact — investigate before retrying): %w", path, readErr)
	}

	// Parse the existing sidecar to extract its canonical envelope, then
	// fingerprint and compare.
	var existingSidecar F2CanonicalSidecar
	if jsonErr := json.Unmarshal(existing, &existingSidecar); jsonErr != nil {
		// The existing file is not valid JSON. Do NOT overwrite (immutability).
		// This is a corruption signal the doctor (Slice 9) will also catch.
		return F2PersistRefused, fmt.Errorf(
			"f2 persist: existing canonical sidecar at %q is not valid JSON (refusing to overwrite a corrupt/unparseable artifact — investigate or use a new cycle): %w",
			path, jsonErr)
	}

	existingFP, fpErr := f2CanonicalContentFingerprint(existingSidecar.CanonicalEnvelope)
	if fpErr != nil {
		return F2PersistNotAttempted, fmt.Errorf("f2 persist: cannot fingerprint existing canonical envelope at %q: %w", path, fpErr)
	}

	if bytes.Equal(existingFP, newFP) {
		// Canonical content is byte-identical → idempotent no-op. The file is
		// NOT rewritten: the original timestamp and bytes are preserved. A
		// re-run at a later time with the same canonical content is a no-op.
		return F2PersistIdempotent, nil
	}

	// Canonical content differs → refuse. The immutability contract forbids
	// silently overwriting a different canonical emit under the same cycle ID.
	// The operator must issue a new F1 emit under a new synthesis cycle.
	return F2PersistRefused, fmt.Errorf(
		"f2 persist: canonical content for cycle %q differs from the existing sidecar at %q (immutability: a changed canonical field requires a new F1 emit + synthesis cycle, not an in-place overwrite)",
		ingest.SynthesisCycleID, path)
}

// --- Read-back (for doctor + round-trip verification) -----------------------

// ReadF2CanonicalSidecar reads and parses a canonical sidecar from disk.
// Returns an error if the file does not exist or is not valid JSON. Exported
// so the doctor (Slice 9) and the streak scanner (Slice 8) can read sidecars
// through the same parse path.
//
// This is a READ operation: it never mutates the file. It proves the file is
// parseable, NOT that its content is true (the same honesty ceiling as the
// rest of F2).
func ReadF2CanonicalSidecar(path string) (*F2CanonicalSidecar, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("f2 read: cannot read canonical sidecar at %q: %w", path, err)
	}
	var sidecar F2CanonicalSidecar
	if jErr := json.Unmarshal(raw, &sidecar); jErr != nil {
		return nil, fmt.Errorf("f2 read: canonical sidecar at %q is not valid JSON: %w", path, jErr)
	}
	return &sidecar, nil
}
