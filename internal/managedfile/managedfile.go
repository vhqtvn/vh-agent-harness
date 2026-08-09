// Package managedfile owns the SHARED typed disposition model for the
// platform_managed ownership class's origin-hash three-way preservation.
//
// Both the update path (internal/substrate Apply) and the lint path
// (internal/cli doctor.checkManagedDrift) must agree on whether a
// platform_managed file is in a SANCTIONED PRESERVED state that update
// deliberately does NOT overwrite. Before this package existed, each path
// re-derived that determination independently (Apply routed three distinct
// reasons to one generic ActionManagedDiverged action and carried the reason
// only in a human-readable Note string that is explicitly "never a correctness
// signal"; doctor hand-rolled its own origin comparison for the consumer-EDIT
// case and never recognized the consumer-DELETE case at all). That left the two
// free to drift apart — exactly the closure gap this taxonomy closes.
//
// The model is deliberately NARROW: it classifies only the PRESERVED reasons.
// The writable outcomes (overwrite / noop on the update path; in-sync / drifted
// / missing on the lint path) are each caller's own concern — they legitimately
// differ (Apply writes; doctor compares) and unifying them would couple
// unrelated logic. What MUST be identical is the preserved determination, so
// that is the single shared decision ClassifyPreserved encodes.
//
// The classifier is IO-free (it takes a LiveState the caller observed) so it is
// pure and unit-testable without touching the filesystem. Both callers perform
// the same small set of observations (stat + read + hash) and pass the result
// here, guaranteeing identical decisions from identical inputs.
package managedfile

// PreservedReason names WHY a platform_managed file (with a recorded origin) is
// in a sanctioned state update deliberately does NOT overwrite. It is the
// machine-readable correctness signal that, before this type existed, lived
// only inside a generic diverged action's Note string (which is diagnostics-
// only and must never be scanned for correctness).
//
// The zero value "" means NOT preserved: the file is unedited, bootstrap (no
// recorded origin), genuine drift, platform-regenerated, or a directory/stat
// weirdness. The caller applies its own writable/drift logic in that case.
type PreservedReason string

const (
	// ConsumerEdit: the live file is present and readable, has a recorded
	// origin, its live hash diverges from that origin, AND it does not already
	// equal the current staged corpus (i.e. a genuine consumer hand-edit, not a
	// partial-failure self-heal). Update preserves it (ActionManagedDiverged);
	// doctor surfaces it as non-failing consumer-preserved.
	ConsumerEdit PreservedReason = "consumer-edit"

	// ConsumerDelete: the live file is absent AND a prior origin is recorded
	// (the platform previously rendered it; the consumer deleted it). Update
	// respects the deletion (never re-seeds); doctor surfaces it as non-failing
	// consumer-preserved — and crucially does NOT accompany it with the false
	// "an ordinary update will restore it" claim that belongs only to genuine
	// missing drift.
	ConsumerDelete PreservedReason = "consumer-delete"

	// Unreadable: the live file is stat-able as a regular file but its bytes
	// cannot be read, so a consumer edit cannot be ruled out. Update preserves
	// it (the safe, never-clobber choice — the write only needs WRITE perm and
	// would otherwise silently clobber a possible edit); doctor treats it as
	// preserved where it observes the same condition.
	Unreadable PreservedReason = "unreadable"

	// UnknownBaseline (F6): the live file EXISTS at a platform_managed path but
	// there is NO recorded origin for it (hadOrigin == false). This is the
	// adoption-migration case: a pre-feature install (or a pre-seam tree) carries
	// on-disk managed files whose bytes we cannot attribute to either the
	// platform (a genuine unedited render) or the consumer (a hand-edit). Because
	// we cannot PROVE the live bytes are platform-origin, overwriting them risks
	// silently destroying consumer edits — exactly the F6 data-loss gate. Update
	// PRESERVES the live bytes (never clobbers) and reports them in a single
	// deterministic batched stall; the path stays preserved until a future
	// `accept-platform` recovery operation (Slice 4 / F2) explicitly adopts the
	// platform bytes. An ABSENT live file with no origin is NOT UnknownBaseline
	// — it is a safe bootstrap (there are no existing bytes to lose).
	UnknownBaseline PreservedReason = "unknown-baseline"
)

// LiveState captures what the caller observed about a live managed file's
// existence and readability. It is the IO-free input to ClassifyPreserved so the
// classifier is pure and unit-testable without a filesystem.
//
// Construction contract (both callers build this identically before calling
// ClassifyPreserved):
//   - Absent: set when os.Stat reports IsNotExist (the consumer deleted it).
//   - IsRegular: set when os.Stat succeeded and the entry is NOT a directory
//     (a directory at a managed path is NOT a preserved state — the caller's
//     write path reports the blocked write).
//   - Readable: set when the bytes were read successfully; Hash is the
//     origin-hash digest of those bytes (originhash.Digest format).
//
// A zero LiveState (no field set) represents "present-but-not-a-regular-file or
// a stat error other than NotExist" — NOT a preserved state; the caller falls
// through to its writable/drift path.
type LiveState struct {
	Absent    bool   // os.IsNotExist — consumer deleted the file
	IsRegular bool   // os.Stat succeeded and entry is a regular file (not a dir)
	Readable  bool   // bytes were read successfully
	Hash      string // originhash.Digest of the live bytes (when Readable)
}

// ClassifyPreserved returns the sanctioned preserved reason for a
// platform_managed path, or "" when the path is NOT in a preserved state. It is
// the SINGLE decision both substrate.Apply (update) and cli.doctor (lint) call
// so the two never disagree on preserved-vs-genuine.
//
// Inputs:
//   - regenerated: the path is platform-regenerated (ApplyOptions.
//     RegeneratedPlatformPaths). Regenerated paths are NEVER preserved — the
//     platform overwrites them on every apply to stay byte-in-sync with its
//     canonical emission, so a consumer edit/divergence is genuine drift, not
//     a preserved state.
//   - hadOrigin / origin: the recorded origin hash for this path (from the
//     origin-hash store). hadOrigin false means bootstrap / pre-feature (no
//     recorded origin) → never preserved (the file is treated as unedited and
//     overwritten/seeded).
//   - live: the observed LiveState (see the struct doc for the construction
//     contract).
//   - stagedHash: the origin-hash digest of the current staged corpus copy
//     ("" when the staged copy could not be read). Used ONLY to distinguish a
//     genuine consumer edit (ConsumerEdit) from a partial-failure self-heal
//     (live already equals what the platform would write → NOT preserved, so
//     the caller advances the origin and reconciles the interrupted
//     generation). When "" the self-heal branch cannot be confirmed, so a
//     diverged live file is classified ConsumerEdit (the safe, never-clobber
//     choice).
//
// Decision tree (mirrors substrate.Apply's planOutcome origin-hash branch
// exactly — see the apply.go comment block; the two MUST stay in lockstep):
//
//	regenerated                          -> ""            (never preserved)
//	!hadOrigin + live.Absent             -> ""            (bootstrap: no existing bytes to lose)
//	!hadOrigin + !live.IsRegular         -> ""            (dir/stat weirdness: caller reports)
//	!hadOrigin + live.IsRegular          -> UnknownBaseline (F6 migration: existing bytes, no origin proof)
//	hadOrigin + live.Absent              -> ConsumerDelete
//	hadOrigin + !live.IsRegular          -> ""            (dir/stat weirdness)
//	hadOrigin + !live.Readable           -> Unreadable
//	hadOrigin + live.Hash == origin      -> ""            (unedited)
//	hadOrigin + live.Hash == stagedHash  -> ""            (self-heal: live==staged)
//	hadOrigin + otherwise                -> ConsumerEdit
//
// The empty-stagedHash self-heal guard: when stagedHash is "" (staged read
// failed) AND live diverges from origin, the safe choice is ConsumerEdit
// (preserve, never clobber) — consistent with Apply's treatment of a staged-hash
// read failure as diverged.
//
// F6 (UnknownBaseline): the !hadOrigin branch is the adoption-migration gate.
// Record absence MUST NEVER authorize overwriting existing bytes: a pre-feature
// install carries on-disk managed files whose provenance is unknowable (could be
// genuine platform bytes, could be consumer edits). The only safe disposition is
// preserve/stall — never overwrite. An ABSENT live file (no existing bytes) is a
// safe bootstrap. A regenerated path bypasses this entirely (the regenerated
// exemption applies regardless of origin state — the canonical emission must
// stay in sync).
func ClassifyPreserved(regenerated, hadOrigin bool, origin string, live LiveState, stagedHash string) PreservedReason {
	if regenerated {
		return ""
	}
	if !hadOrigin {
		// F6 adoption-migration gate: no recorded origin for this path.
		// An ABSENT live file is a safe bootstrap (no bytes to lose). A
		// directory/stat-weirdness falls through too (caller reports). An
		// existing REGULAR FILE is unknown-baseline → preserve/stall.
		if live.Absent || !live.IsRegular {
			return ""
		}
		return UnknownBaseline
	}
	if live.Absent {
		return ConsumerDelete
	}
	if !live.IsRegular {
		return ""
	}
	if !live.Readable {
		return Unreadable
	}
	if live.Hash == origin {
		return "" // unedited — caller overwrites/noops
	}
	if stagedHash != "" && live.Hash == stagedHash {
		return "" // partial-failure self-heal — live already == staged
	}
	return ConsumerEdit
}
