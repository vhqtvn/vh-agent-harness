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
	"strings"
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

// --- Transient-locator admission check (narrow durable-path gate) -----------
//
// Design authority: researches/decisions/2026-07-25-f2-rendering-family-
// mechanism.md (transient-locator contract sentence). Before writing an F2
// artifact pair, persistence must refuse any repo-relative canonical
// provenance locator lexically rooted under tmp/agent-runs/. Persistence must
// not resolve, rewrite, inline, or otherwise replace that locator, and this
// rule does not classify other locator roots.
//
// This is a NARROW ADMISSION CHECK for ONE already-defined disposable root
// (tmp/agent-runs/ — the harness's canonical per-session disposable output
// root per docs/ai/shell-execution.md and AGENTS.md). It is NOT a general
// durability classifier: it does not consult .gitignore, does not stat the
// filesystem, does not classify .local/ or other tmp roots, does not classify
// absolute paths, and does not classify URLs. It refuses exactly the one root
// a transient provenance locator is most likely to point at when it leaks into
// canonical content (the dogfood origin: commit 6fec8222's first real F2 pair
// cited tmp/agent-runs/f1-build/SLICE-0-STOP.md in three canonical fields).
//
// The check is PURE and LEXICAL: no I/O, no resolution, no rewrite, no hashing,
// no inlining. The recovery from a refusal is a new F1 emit under a new
// synthesis cycle with a durable locator — never an in-place rewrite (F2 must
// NOT rewrite locators; that would cross the F1→F2 fence).

// f2TransientAgentRunsRoot is the single disposable root this gate admits. It
// is repo-relative (no leading slash): a repo-relative locator rooted here is
// transient by construction (the harness never commits tmp/agent-runs/). The
// gate rejects exactly this root.
const f2TransientAgentRunsRoot = "tmp/agent-runs"

// f2NormalizeLocator applies the minimal lexical normalization the gate uses
// to compare a locator against the disposable root:
//  1. backslash → forward slash (cross-platform: a Windows-style locator must
//     not evade the check by spelling the separator differently);
//  2. strip a single leading "./" (a relative-path prefix must not evade the
//     check).
//
// This is deliberately MINIMAL. It does NOT collapse "..", strip repeated
// "./", lower-case, trim whitespace, resolve symlinks, or canonicalize case.
// A locator that evades the check by any of those means is out of scope: this
// gate catches the common, honest mistake (a transient path cited verbatim),
// not an adversarial obfuscation.
func f2NormalizeLocator(locator string) string {
	n := strings.ReplaceAll(locator, "\\", "/")
	n = strings.TrimPrefix(n, "./")
	return n
}

// f2IsTransientAgentRunsLocator reports whether a locator, after minimal
// lexical normalization, is repo-relative and rooted under tmp/agent-runs/.
// "Rooted under" means the normalized locator is exactly tmp/agent-runs OR is
// prefixed tmp/agent-runs/ (a child path).
//
// Absolute paths and URLs do NOT match: an absolute path normalizes to a
// leading "/", and a URL keeps its scheme prefix, so neither equals nor is
// prefixed by the bare repo-relative root. The check therefore never classifies
// them (per the non-goal: no absolute-path / URL classification). It admits
// ONLY repo-relative locators rooted under the one disposable root.
func f2IsTransientAgentRunsLocator(locator string) bool {
	n := f2NormalizeLocator(locator)
	return n == f2TransientAgentRunsRoot || strings.HasPrefix(n, f2TransientAgentRunsRoot+"/")
}

// validateNoTransientProvenanceLocators walks the canonical envelope and
// refuses any provenance locator that, after minimal lexical normalization, is
// repo-relative and rooted under tmp/agent-runs/. Returns an artifact-integrity
// error naming the offending canonical field path + the locator value, or nil
// when every locator is admissible (or absent).
//
// The provenance locator fields enumerated (the locator-bearing canonical F1
// envelope fields — references pointing at where source/evidence material
// lives):
//
//   - F1FamilyEntry.SourceRefs                        (entry-level source refs)
//   - F1R1Conclusion.Sources[].Locator               (R1 source locator)
//   - F1R1Conclusion.Hazards[].SourceLocators        (R1 hazard source locators)
//   - F1PAProbe.EvidenceRefs                         (P-a evidence refs)
//
// Deliberately NOT enumerated:
//   - F1PAProbe.CheckedScope — a scope-DESCRIPTION field ("the scope that was
//     checked"), not a provenance locator. Narrow by design.
//   - F2ViewMetadata.StorageLocator / AttachmentMetaRef — F2-derived metadata,
//     excluded from canonical content; not canonical provenance.
//   - R5Binding.SourceLocators / MediaAttachments[].Locator — F2-derived
//     metadata carried on the sidecar (not the canonical envelope); R5 source
//     locators are separately constrained to match a canonical entry's
//     SourceRefs, so a transient SourceRef is caught here transitively.
//   - cross-reference ID fields (SupportRefs, CounterEvidenceProbeRefs,
//     TargetRef, AffectedProperties, ancestry roots, conclusion/option/probe
//     IDs) — these resolve WITHIN the envelope, not at the filesystem; they are
//     not file/path locators.
//
// The locator field set is FIXED to these four canonical provenance slots.
// Adding a new provenance locator field to the F1 envelope requires extending
// this walk (the same discipline as the F1 deep-copy helpers).
func validateNoTransientProvenanceLocators(env *F1SynthesisEnvelope) error {
	for i := range env.Entries {
		e := &env.Entries[i]

		// entry-level source_refs
		for j, ref := range e.SourceRefs {
			if f2IsTransientAgentRunsLocator(ref) {
				return f2TransientLocatorError(
					fmt.Sprintf("entries[%d].source_refs[%d]", i, j), ref)
			}
		}

		// r1 conclusions: sources[].locator + hazards[].source_locators
		if e.R1 != nil {
			for c := range e.R1.Conclusions {
				concl := &e.R1.Conclusions[c]
				for s := range concl.Sources {
					loc := concl.Sources[s].Locator
					if f2IsTransientAgentRunsLocator(loc) {
						return f2TransientLocatorError(
							fmt.Sprintf("entries[%d].r1.conclusions[%d].sources[%d].locator", i, c, s), loc)
					}
				}
				for h := range concl.Hazards {
					hz := &concl.Hazards[h]
					for sl := range hz.SourceLocators {
						loc := hz.SourceLocators[sl]
						if f2IsTransientAgentRunsLocator(loc) {
							return f2TransientLocatorError(
								fmt.Sprintf("entries[%d].r1.conclusions[%d].hazards[%d].source_locators[%d]", i, c, h, sl), loc)
						}
					}
				}
			}
		}

		// pa probes: evidence_refs
		if e.PA != nil {
			for p := range e.PA.Probes {
				probe := &e.PA.Probes[p]
				for er := range probe.EvidenceRefs {
					ref := probe.EvidenceRefs[er]
					if f2IsTransientAgentRunsLocator(ref) {
						return f2TransientLocatorError(
							fmt.Sprintf("entries[%d].pa.probes[%d].evidence_refs[%d]", i, p, er), ref)
					}
				}
			}
		}
	}
	return nil
}

// f2TransientLocatorError builds the artifact-integrity error for a refused
// transient provenance locator. The error names the offending canonical field
// path and the locator value so an operator can locate and correct the entry.
// It is returned pre-write: neither the JSON sidecar nor the MD projection is
// created when this fires.
func f2TransientLocatorError(fieldPath, locator string) error {
	return fmt.Errorf(
		"f2 persist: artifact-integrity refusal — canonical provenance locator %q at %s is rooted under the disposable tmp/agent-runs/ root (transient locators must not enter the durable F2 artifact; persistence does not resolve, rewrite, inline, or replace the locator — re-emit with a durable locator under a new synthesis cycle)",
		locator, fieldPath)
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

	// Transient-locator admission gate (narrow durable-path check): refuse
	// BEFORE constructing the sidecar or touching the filesystem if any
	// canonical provenance locator is repo-relative and rooted under
	// tmp/agent-runs/. Pure lexical check — no resolve/rewrite/inline/hash/stat.
	// The recovery is a new F1 emit with a durable locator, never an in-place
	// rewrite. See validateNoTransientProvenanceLocators for the field set.
	if tErr := validateNoTransientProvenanceLocators(ingest.CanonicalEnvelope); tErr != nil {
		return F2PersistNotAttempted, tErr
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
