# Context-Pruning External-Engine Activation Escalation Spec

> **What this is.** The **pre-activation contract** for any future P1
> (harness-authored config for an external context-pruning engine) or P4
> (a later companion contingent on an upstream seam) activation. It enumerates
> the gates a P1 experiment or P4 companion MUST satisfy before it may
> activate. **A P1 experiment or P4 companion may NOT activate until this
> contract exists AND every one of its gates is satisfied** — the contract is a
> *precondition* for activation, not a consequence of it.
>
> **Why it exists.** Named as the "P1/P4 pre-activation contract, not yet
> authored" in `researches/decisions/2026-07-23-dcp-ownership-layer.md` §5
> (commit `6c13bcaf24ea42ae99defb8922dc9280eb78296e`). The decision record
> fixes the verdict that **P1 and P4 are NOT-ACTIVATED** (§2) and holds that
> verdict *behind* this contract: the gates below are the checkable conditions
> under which the NOT-ACTIVATED verdict could be revisited toward activation.
>
> **Scope.** This document is the *escalation contract* — it states what must be
> true before activation, not how the engine is built or whether building it is
> wise. It is paired with the sibling behavioral oracle
> [`context-pruning-engine-behavioral-spec.md`](./context-pruning-engine-behavioral-spec.md),
> which is the baseline gate 7 measures against. This spec does **not**
> redesign the §4.2 premise-recheck protocol (owned by
> `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
> and cited by the DCP record, not separately listed as one of the §5 gates).
>
> **Status of P1/P4 as of this spec.** NOT-ACTIVATED. Authoring this contract
> does NOT activate anything. It establishes the bar a future activation
> proposal must clear.

## How this contract is read

The seven gates below are transcribed verbatim-in-substance from memo §5 and
elaborated with the mechanism grounding from memo §3 so each is independently
checkable. They are **conjunctive**: every gate must be satisfied for a P1/P4
activation to proceed. A single unsatisfied gate holds the NOT-ACTIVATED
verdict. The gates are ordered for readability, not for sequencing — they may
be worked in parallel, but all must be green before activation.

Each gate states:
- **Requirement** — what the gate demands, transcribed from §5.
- **Satisfied when** — the observable, checkable condition that clears the gate.
- **Not satisfied when** — the observable failure that holds it.
- **Mechanism grounding** — where the memo §3 substantiates the requirement
  (so the gate is not a bare assertion).

The gates apply to **both** P1 and P4 unless a gate notes a P1-vs-P4
divergence. Where they diverge, the divergence is named (memo §5 gate 1 makes
the P1-vs-P4 split explicit).

---

## Gate 1 — Ship-form / counsel matrix

- **Requirement (§5).** The concrete render/ship form and its counsel
  disposition. P1 adapter and P4 upstream seam **differ** in ship form, so the
  matrix is per-activation-path, not a single answer.
- **Satisfied when.** A named, concrete ship form exists for the activation
  path being proposed (P1 overlay/`.local` adapter config, or a P4 upstream-
  seam integration), AND its counsel disposition is recorded. The memo
  (§3 SQ1) asserts risk framing only and explicitly defers legal opinion: a
  concrete ship form — whether P1 adapter or P2 reimplementation — *still
  requires counsel*. A clean counsel disposition (counsel-reviewed and cleared,
  or counsel-flagged-unshippable recorded as a hold) is therefore a hard gate.
- **Not satisfied when.** No concrete ship form is named; or counsel has not
  reviewed the concrete form (an abstract "config seed" without a rendered
  artifact is not a concrete ship form); or counsel flagged the form
  unshippable and the flag is unresolved.
- **Mechanism grounding (§3).** SQ1 establishes the license-risk framing: an
  independently-authored config/prompt override is *our* expression (lower
  risk), copied expression is the concern. The gate is the counsel review of
  the **concrete** form, which is per-ship-form by construction.

## Gate 2 — Overlay boundary

- **Requirement (§5).** The config lives as an overlay / `.local` optional
  adapter, **never** in `templates/core/`.
- **Satisfied when.** The shipped config is expressed as an overlay-pack output
  or a `.local` adapter, AND no part of the activation renders a vendor-
  specific file into `templates/core/`. The harness's domain-free invariant
  (`templates/core/` carries no brand/domain literals) is preserved.
- **Not satisfied when.** The activation attempts to land vendor-specific
  config in the core corpus; or relies on `CapabilityManifest.CoreOutputs` to
  stage a vendor-specific overlay file (CoreOutputs gates only generic embedded
  core outputs — a vendor config is not one and is forbidden from `templates/core/`
  by the domain-free invariant regardless).
- **Mechanism grounding (§3).** The `ae5b30d` CoreOutputs mechanism gates only
  *actual generic embedded core outputs* (source-relative paths into
  `templates/core/`). It cannot declare a vendor-specific overlay file, and the
  exact current P1 declaration is none. Any DCP surface can therefore only
  live behind an optional overlay or `.local` adapter.

## Gate 3 — Compatibility and native coexistence

- **Requirement (§5).** A pin/version matrix **plus** demonstrated native
  fallback to P3.
- **Satisfied when.** An engine ↔ runtime compatibility matrix is declared
  (which engine versions are compatible with which harness/runtime versions)
  AND checked at activation; AND native fallback to P3 (memo §2 ACCEPTED
  baseline) is demonstrated — when the engine is absent or unhealthy, behavior
  degrades to native compaction, never to a broken state.
- **Not satisfied when.** No compatibility matrix is declared; or the matrix is
  declared but not checked; or native fallback is asserted but not
  demonstrated (a coexistence claim without a fallback demonstration does not
  clear the gate).
- **Mechanism grounding (§3).** Rendering config that points at a binary the
  consumer has not installed is inert-by-default, and correct operation
  additionally requires plugin presence/health/version detection (gate 4) AND
  native fallback. SQ3 ranks P1/P4 blast radius as requiring "the full
  containment stack," of which the pin/version matrix and native fallback are
  load-bearing members.

## Gate 4 — Health / version detection

- **Requirement (§5).** Plugin presence / health / version detection at
  runtime.
- **Satisfied when.** The runtime detects whether the engine plugin is present,
  healthy, and version-compatible before relying on it, and surfaces the
  detection result (not a silent assumption of presence).
- **Not satisfied when.** The activation assumes the engine is present without
  detection; or detects presence but not health; or detects health but not
  version compatibility; or the detection result is not surfaced into the
  fallback decision (gate 3) and the inert-disable decision (gate 5).
- **Mechanism grounding (§3).** The memo's reframing (§1, §3) makes rendering
  *insufficient* without plugin presence/health/version detection: "rendering
  must not imply the engine is present, healthy, or version-compatible." This
  is the runtime-execution companion to gate 2's render-boundary requirement.

## Gate 5 — Lifecycle / deselection-residue / inert disable

- **Requirement (§5).** Safe inert deselection and retirement, accounting for
  residue that an installed reader may still act on.
- **Satisfied when.** Deselection disables the config **inertly** — a present
  engine that reads a deselected config file ignores it (the config is written
  so an installed engine does not act on it), not merely drift-exempt; AND
  retirement accounts for a previously-active residue being read by an
  installed consumer (retirement is not "leave it untouched and hope").
- **Not satisfied when.** Deselection relies on CoreOutputs residue-exemption
  alone (residue-exempt-from-drift ≠ residue-inert-to-installed-readers); or
  the disable leaves a config an installed engine would still act on; or
  retirement has no story for previously-active residue.
- **Mechanism grounding (§3).** This is the load-bearing subtlety the memo
  calls out explicitly: CoreOutputs deselection leaves the file on disk
  untouched as *inactive residue* (exempt from managed-drift and
  unexpected-drift failures), **but that residue may be live** — if an
  installed plugin or consumer reads a deselected config file, it can still act
  on it. A safe disable must be *inert*; retirement must account for a
  previously-active residue being read. This gate exists precisely because
  residue-exempt ≠ residue-inert.

## Gate 6 — Fallback / rollback

- **Requirement (§5).** A documented path back to P3.
- **Satisfied when.** A rollback procedure exists that returns the harness to
  the P3 native-compaction baseline, documented and demonstrable (not merely
  asserted). Rollback is the containment backstop that makes blast radius
  acceptable (SQ3: "containment is only as good as the fallback").
- **Not satisfied when.** No rollback procedure is documented; or it is
  documented but not demonstrable; or rollback depends on state that the
  engine itself corrupted (a rollback that assumes a clean transcript the
  engine rewrote is not a real rollback).
- **Mechanism grounding (§3, §6-SQ3).** The live transcript the engine rewrites
  is the blast radius; containment is only as good as the fallback. P3 is the
  ACCEPTED baseline (§2), so "back to P3" is the canonical rollback target.

## Gate 7 — Measured benefit

- **Requirement (§5).** An evidence-gated result showing the engine beats P3 on
  the behavioral-spec baseline **before** the adapter is anything but
  experimental.
- **Satisfied when.** A measured result exists, on the workload fixtures of the
  sibling behavioral spec
  ([`context-pruning-engine-behavioral-spec.md`](./context-pruning-engine-behavioral-spec.md)),
  showing the engine outperforms P3 native compaction on the §6 insufficiency
  signals (i.e. the engine clears signals P3 fails, without introducing new
  failures) — AND the adapter remains experimental (not default-on) until that
  result is recorded.
- **Not satisfied when.** No measured result exists against the behavioral
  spec; or the result shows parity/worse rather than a beat; or the adapter is
  promoted past experimental before the result is recorded. This gate has a
  hard dependency on the sibling behavioral spec existing as the measurement
  baseline (the two specs are co-required).
- **Mechanism grounding (§2, §5).** P1 stays not-activated at MEDIUM confidence
  and flips toward activation "only if: the behavioral spec + a
  source-independent clean-room spec both exist AND an independent P1
  experiment passes every gate in the pre-activation contract." Gate 7 is the
  measured-benefit member of that conjunction; the behavioral spec is the
  baseline it is measured against.

---

## What this contract does NOT do

- **It does not activate P1 or P4.** Authoring and satisfying this contract is
  necessary, not sufficient, for activation. The memo §2 confidence statements
  name additional preconditions (the behavioral spec's existence for any P1
  experiment; a maintainer-accepted upstream seam for P4; an independent P1
  experiment passing every gate).
- **It does not enumerate the §4.2 premise-recheck boundary as a gate.** §5
  does not list it separately; it is owned by
  `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
  and cited (not redesigned) by the DCP record. No premise-recheck gate and no
  migration-note dependency is invented here.
- **It does not address P2 (clean-room reimplementation).** P2 is dormant and
  gated behind the SQ5 triggers and source-independent controls, not behind
  this P1/P4 contract. See the memo §2 (P2 DORMANT), §3 SQ1 (clean-room
  controls), and §6 SQ5 (named triggers).
- **It does not assert counsel clearance.** Gate 1 requires a counsel
  disposition; the memo asserts risk framing only and defers legal opinion.
  This contract names the gate; it does not pre-clear it.

## Provenance and constraints honored

- **Faithful transcription.** The seven gates are reproduced in substance from
  `researches/decisions/2026-07-23-dcp-ownership-layer.md` §5 (commit
  `6c13bcaf24ea42ae99defb8922dc9280eb78296e`). No gate is invented or dropped.
  The gate count (7) is verified by direct read of the committed memo.
- **Mechanism grounding.** Each gate's elaboration cites the memo §3
  mechanism (the `ae5b30d` CoreOutputs limits, the `ba68c76` policy-layer
  distinction, the residue-may-be-live subtlety, SQ3 containment ranking) so
  the contract is durable workflow guidance, not a bare transcription.
- **NOT-ACTIVATED preserved.** This spec does not change the P1/P4 verdict.
  P1 and P4 remain NOT-ACTIVATED (memo §2) until this contract exists AND is
  satisfied; this document establishes the bar, it does not clear it.
- **Source-independent.** Authored from the harness decision record and the
  sibling behavioral spec. No reference to any external context-pruning
  engine's implementation, code, prompts, or architecture.
- **See also.**
  `researches/decisions/2026-07-23-dcp-ownership-layer.md` (§5 Deferred names
  this artifact; §2 Decision fixes NOT-ACTIVATED; §3 Mechanism grounds the
  gates),
  [`./context-pruning-engine-behavioral-spec.md`](./context-pruning-engine-behavioral-spec.md)
  (gate 7 measurement baseline; co-required),
  `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
  (§4.2 premise-recheck protocol owner — cited, not a gate here).
