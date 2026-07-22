# Decision: DCP (External Context-Pruning Engine) Ownership Layer

**Date:** 2026-07-23
**Status:** Accepted (record-of-decision; fixes the F2-only-in-chat verdict). No code,
no DCP configuration/capability/overlay, and no unrelated docs change in this slice.
This memo persists the verified verdict of an in-chat ownership debate so it no longer
exists only in chat (the F2 "fact staleness / claim crossed a lossy boundary" risk).
**Supersedes:** none.
**See also:**
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(owner of the unstable §4.2 premise-recheck protocol — cited, not redesigned here).
[`../../docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`](../../docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md)
(the failure-mode case study; its stable scope is used with the §1.1/§1.5 known-error
exclusions — see Evidence).
[`./2026-07-05-commit-gate-shared-file-coupling.md`](./2026-07-05-commit-gate-shared-file-coupling.md)
(house-style template this record follows).

> **"DCP" nomenclature note.** "DCP" denotes a *Dynamic Context Pruning / external
> context-compaction engine* — a component that could replace or augment OpenCode's
> native session compaction. The term is carried from the in-chat debate this record
> persists; it is not yet referenced by any committed doc (verified — see Evidence),
> which is itself the F2 motivation for persisting the verdict here.

## 1. Framing

The question: *where should an external context-pruning engine live in this harness's
ownership model, if anywhere?* Four positions were on the table:

- **P1 — ship harness-authored config for an external DCP engine** (a config seed /
  prompt override / optional adapter) as a **bounded, evidence-gated experiment
  candidate**, not an unconditional capability.
- **P2 — clean-room reimplementation** of the external engine's pruning behavior as a
  harness-owned component.
- **P3 — native OpenCode compaction/pruning plus the harness's continuity/recovery
  policy** (the current shipped baseline).
- **P4 — a later companion**, contingent on a concrete, maintainer-accepted upstream
  seam and demonstrable maintainer willingness.

The decisive framing is the **ownership-layer split** the harness already enforces
everywhere: `templates/core/` ships only generic, domain-free, embedded outputs; a
vendor-specific runtime artifact (an engine config pointed at one specific external
binary/library) is **not** a generic embedded core output and therefore **cannot be
landed via the core corpus**. Any DCP surface can only live behind an *optional* overlay
or `.local` adapter, and even rendering it is **insufficient** without plugin presence /
health / version detection, native fallback, and safe inert-deselection behavior. This
reframes the whole debate from "do we ship the engine?" to "is there a capability-model
slot for an *optional, opt-in adapter* whose rendering does not imply runtime safety?"

A second decisive framing is the difference between *authoring a policy layer* (which
the harness has now proven it can do correctly) and *proving a runtime integration
safe*. `ba68c76` shows authored harness policy layers (a deny-first `read_only`
HarnessPolicy that resolves correctly under opencode's `findLast` evaluator) are
technically feasible. That is distinct from — and does not by itself establish —
runtime DCP integration being safe: a policy layer gates tool calls; a context engine
rewrites the very transcript the policy is evaluated against.

## 2. Decision

**P3 is the baseline. P1 is a bounded evidence-gated experiment candidate only. P4 is a
later companion conditional on a concrete upstream seam + maintainer willingness. P2 is
dormant.** Specifically:

- **P3 — ACCEPTED (current baseline).** Native OpenCode compaction plus the harness
  continuity/recovery policy (session memory, checkpoints, handoffs, workstream memory)
  remains the shipped context-management surface.
- **P1 — NOT ACTIVATED.** The exact current P1 declaration is **none** (verified — see
  Evidence). Shipping harness-authored config for a possibly-not-installed external
  engine is permissible *only* as an opt-in overlay / `.local` optional adapter, and
  only behind the pre-activation contract named in §3 (`context-pruning-external-engine-
  escalation-spec.md`). Rendering such config without the plugin it targets is
  insufficient and is refused by default.
- **P4 — NOT ACTIVATED (later companion).** Contingent on (a) a concrete, maintainer-
  accepted upstream compaction/extension seam and (b) demonstrable maintainer
  willingness. As of this record, OpenCode exposes **no** blocking compaction hook
  (verified — see Evidence); the only observed such seam is in a sibling, proprietary
  runtime (Claude Code), which is out of this harness's control.
- **P2 — DORMANT.** A clean-room reimplementation is not pursued. The prior research's
  detailed source study of the external engine's expression makes an *informal*
  clean-room claim weak (§3, SQ1). P2 can only revive under a named SQ5 trigger, and
  only behind source-independent controls and counsel review.

**Confidence and what would flip it:**

- **P3 baseline — HIGH confidence.** Flips *away from P3* only on: a measured P3
  correctness failure against the not-yet-authored behavioral spec
  (`context-pruning-engine-behavioral-spec.md`), or a required deterministic engine
  semantics that is unavailable through native *or* external seams.
- **P1 stays not-activated — MEDIUM confidence.** Flips *toward activation* only if:
  the behavioral spec + a source-independent clean-room spec both exist AND an
  independent P1 experiment passes every gate in the pre-activation contract. Flips
  *away* (toward "never") if counsel flags the concrete ship form unshippable.
- **P4 activates — LOW confidence.** Flips *toward* only if OpenCode ships a
  maintainer-accepted blocking compaction seam (currently absent) and a maintainer
  agrees to the integration shape.
- **P2 revives — LOW–MEDIUM that informal clean-room suffices; HIGH that it does not
  without controls.** Flips *toward* only if an SQ5 trigger fires AND clean-room
  controls + counsel review pass. A legal concern **alone does not trigger P2**.

## 3. Mechanism

The decision is enforced by the **ownership + capability model that already ships**,
not by new DCP code. Each position maps to an existing seam.

### P1 reframed to the `ae5b30d` CoreOutputs mechanism (SQ2)

`ae5b30d` (`feat(core): add capability-owned CoreOutputs filtering for media-perception`)
introduces `CapabilityManifest.CoreOutputs` — the list of *core-corpus LIVE output
paths* a capability owns and gates by selection. The verified limits of this mechanism,
stated precisely:

- **CoreOutputs gates only actual generic embedded core outputs.** A `CoreOutputs`
  entry is a *source-relative* path into `templates/core/` (forward-slash,
  suffix-stripped LIVE form). It controls whether the renderer stages that embedded
  file. It is a render/ownership gate, not a runtime-execution gate.
- **CoreOutputs cannot declare a vendor-specific `.opencode/dcp.jsonc` overlay file.**
  Such a file is not a generic embedded core output; it is vendor-specific expression.
  The harness's domain-free invariant (`templates/core/` carries no brand/domain
  literals) forbids it from the core corpus regardless.
- **The exact current P1 declaration is none.** Only `core/media-perception` declares
  `CoreOutputs` (two files). No capability declares any DCP-related path, and
  `.opencode/dcp.jsonc` does not exist anywhere in the repo (verified — see Evidence).

Therefore: **shipping config for a possibly-not-installed external engine fits the
capability model only as an overlay / `.local` optional adapter**, and even then
rendering is **insufficient**. A rendered config that points at a binary the consumer
has not installed is inert-by-default, and correct operation additionally requires:

- **plugin presence / health / version detection** — rendering must not imply the
  engine is present, healthy, or version-compatible;
- **native fallback** — when the engine is absent/healthy-false, behavior degrades to
  P3 native compaction, never to a broken state;
- **deselection / inactive-residue / inert-disable / retirement behavior** —
  CoreOutputs deselection leaves the file on disk untouched as *inactive residue*
  (exempt from managed-drift and unexpected-drift failures). **That residue may be
  live**: if an installed plugin (or a consumer) reads a deselected config file, it can
  still act on it. A safe disable must therefore be *inert* (the config must be written
  so a present engine ignores it), and retirement must account for a previously-active
  residue being read by an installed consumer. This is the load-bearing subtlety:
  residue-exempt-from-drift ≠ residue-inert-to-installed-readers.

### `ba68c76` — authored policy layers are feasible; runtime integration safety is not proven (SQ2 corollary)

`ba68c76` (`feat(permconfig): add read_only HarnessPolicy for RO specialists`) lands a
first-class deny-first `read_only` policy with a canonical read-only allow inventory
that resolves correctly under opencode's **`findLast`** (last-match-wins) evaluator.
This proves authored **harness policy layers** are technically feasible — we can write
rules against the *real* evaluator and validate them with a test matrix.

This is deliberately distinguished from proving **runtime DCP integration safe**. A
policy layer gates tool calls over a transcript the harness controls. A context engine
*rewrites that transcript*. "We can author a correct permission table" does not imply
"an external process that edits our conversation history is safe to wire in." The
`ba68c76` evidence lowers the cost of the *policy* half of any future P1/P4 adapter
(e.g. gating an engine-invoke tool behind a deny-first RO-safe allow) but says nothing
about the *engine* half.

### SQ1 — AGPL analysis (non-legal-advice; this is risk framing, not a legal opinion)

The external engine's source is AGPL-class licensed (this repo itself is **MIT**,
verified — see Evidence; AGPL copyleft tension with a MIT-licensed core is real). The
distinction that governs the harness's options:

- **Independently authored config / prompt override** is expression *we* author. A
  config seed or a prompt that tells our own compaction how to behave is not a copy of
  the engine's expression.
- **Copied expression** (code, prompts lifted verbatim, data structures reproduced from
  the engine's source) is the concern. AGPL's strongest reach is over copied/derived
  expression and network-served derivatives.

The prior research's **detailed source study of the engine's expression** (the reverse-
engineering corpus under `refs/`) has a corrosive consequence for P2: once the engine's
*expression* has been studied in detail, an *informal* "I just reimplemented it from
memory" clean-room claim is **weak** — clean-room hygiene is exactly the control that
breaks down when the reimplementer has read the original. Therefore, were P2 ever to
revive, it would require, at minimum:

- a **source-independent behavioral spec** (`docs/ai/context-pruning-engine-behavioral-
  spec.md`) authored *without* reference to the engine's source expression;
- **clean-room controls** — a wall between any author who has studied the source and any
  author who writes the reimplementation;
- **provenance records** showing the reimplementation derives from the spec and the
  black-box corpus, not from memory of the source;
- a **black-box corpus** (input/output observations of the engine treated as an opaque
  box) that the spec is derived from; and
- **counsel review** of the concrete ship form and the clean-room process.

A concrete ship form — whether P1 adapter or P2 reimplementation — **still requires
counsel**. This record asserts the risk framing only.

## 4. Tradeoffs

- **(+)** P3 reuses the shipped, tested native surface; no new runtime risk, no license
  exposure, no hook coupling.
- **(+)** The verdict is enforced by the *existing* ownership/capability model — no new
  gate is needed to keep DCP out of `templates/core/`.
- **(+)** P1/P2/P4 are not killed, only gated: each has a named, checkable activation
  path (the pre-activation contract for P1/P4; the SQ5 triggers for P2), so the decision
  is reversible as evidence accumulates rather than a permanent "never."
- **(−)** P3 inherits native compaction's limits — most sharply that the native
  compactor **cannot use tools during summary generation** (verified: the summary LLM
  call is made with `tools: []`). Any mechanism that needs the compactor itself to call
  a verification tool mid-summary is unimplementable on P3.
- **(−)** P2 dormancy means the deterministic/auditable-engine-semantics demand, if it
  ever materializes, must be met through P4 (an upstream seam) or a controlled P2
  revival — neither is available today.
- **(−)** CoreOutputs residue-may-be-live subtlety (§3): a future P1 adapter must
  engineer inert-disable and retirement, not rely on deselection alone.

## 5. Deferred

- **`docs/ai/context-pruning-engine-behavioral-spec.md` — required-before-triggers,
  not yet authored.** This is the **source-independent behavioral spec** that every P2
  trigger requires to exist *before* it can fire, and the baseline against which a P3
  correctness failure would be measured. It must be independently authored (no reference
  to the external engine's source expression) and is a precondition for *any* P1
  experiment, P2 revival, or P3-failure claim.
- **`docs/ai/context-pruning-external-engine-escalation-spec.md` — P1/P4 pre-activation
  contract, not yet authored.** A P1 experiment or P4 companion may not activate until
  this contract exists and is satisfied. Required gates:
  1. **ship-form / counsel matrix** — the concrete render/ship form and its counsel
     disposition (P1 adapter vs P4 upstream seam differ);
  2. **overlay boundary** — the config lives as overlay/`.local` optional adapter,
     never in `templates/core/`;
  3. **compatibility and native coexistence** — pin/version matrix + demonstrated
     native fallback to P3;
  4. **health/version** — plugin presence/health/version detection at runtime;
  5. **lifecycle / deselection-residue / inert disable** — safe inert deselection and
     retirement accounting for residue that an installed reader may still act on;
  6. **fallback / rollback** — a documented path back to P3;
  7. **measured benefit** — an evidence-gated result showing the engine beats P3 on the
     behavioral-spec baseline before the adapter is anything but experimental.

## 6. Open questions

### SQ3 — failure containment / blast radius

Ranked by current blast radius:

- **P3 — lowest.** Native compaction is already shipped and tested; failures are
  bounded to the session transcript and recovered via the existing continuity surface.
- **P1 / P4 — require opt-in and the full containment stack:** explicit opt-in (no
  default-on), a **pin/version matrix** (engine ↔ runtime compatibility declared and
  checked), **runtime health** detection, **native fallback** to P3, **safe inert
  deselection** (not just residue-exempt deselection), and **rollback**. Blast radius is
  the live transcript the engine rewrites, so containment is only as good as the
  fallback.
- **P2 — highest maintenance / hook exposure.** A harness-owned reimplementation must
  track the external engine's evolving behavior, maintains a clean-room wall forever,
  and couples the harness to pruning-semantics churn. This is why it is dormant.

### SQ4 — sensitivity analysis for the unstable §4.2 premise-recheck protocol

The §4.2 protocol (facts-as-cache-entries, re-derive cheap premises at boundaries) is
owned by
[`2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md),
which records it as **softened, not mechanically closed** — a protocol habit, not a
gate. This record does **not** redesign that protocol. The sensitivity question is:
*which landing shape of §4.2 favors P3/thin-P1, and which would make deterministic
engine semantics material?*

- **Landing shapes favoring P3 / thin P1.** If §4.2 lands as *protocol discipline*
  (re-derive cheap premises via a grep or a `doctor` run before acting, as the owner
  record describes), the demand is satisfiable on native compaction: the re-derivation
  happens *outside* the summary step, as ordinary tool calls the agent makes before a
  load-bearing action. No deterministic pruning engine is required. A thin P1 (an
  independently-authored prompt/config that nudges our own summarizer) is the most that
  is warranted.
- **Landing shapes that make engine semantics material.** If a future design requires
  *deterministic, reproducible pruning decisions* that must be auditable/replayable
  across runs (e.g. "the same transcript must compact to the same summary for a regulator-grade
  audit"), then native compaction cannot supply it: **native compaction cannot use tools
  during summary generation** (verified — the summary call is `tools: []`), and it is a
  probabilistic LLM summary, not a deterministic reduction. *Any* mechanism that needs
  the compactor to call a verification tool mid-summary, or to be replayable, is
  unimplementable on P3. That is the precise seam at which deterministic engine
  semantics become material — and the point at which P4 (an upstream seam that exposes a
  deterministic reducer) or a controlled P2 revival would re-enter consideration. As of
  this record, no such requirement has been demonstrated, so the verdict holds at P3.

### SQ5 — named, checkable P2 revisit triggers

Each trigger is **independently checkable** and **cannot fire** until
`docs/ai/context-pruning-engine-behavioral-spec.md` **exists** as the source-independent
behavioral baseline (independently authored; no reference to the external engine's
source expression). The spec's existence is a hard precondition for *every* trigger
below — it is what a "failure" or "breakage" is measured against.

- **`Upstream compatibility breakage`** — condition: *two consecutive supported OpenCode
  upgrades break the external engine while P3 fails the defined workload baseline.*
  (Two consecutive, not one, to filter ordinary churn; "supported" = upgrades the
  harness itself targets; "while P3 fails" ties it to a measured workload baseline, not
  to the engine's convenience.)
- **Measured P3 correctness failure** — condition: *P3 native compaction is shown
  incorrect against the behavioral spec on a defined workload* (e.g. a load-bearing
  premise is dropped by the native summary that the spec requires preserved), with a
  reproducible corpus.
- **Required deterministic engine semantics unavailable through native/external seams**
  — condition: *a requirement is accepted for deterministic, auditable/replayable
  pruning that neither native compaction (probabilistic, `tools: []`) nor an external
  seam can satisfy* (this is the SQ4 material-semantics seam).
- **Cross-runtime / audit requirement** — condition: *a requirement is accepted for a
  pruning transcript that is identical across runs/runtimes for audit purposes*, which
  P3's probabilistic summary structurally cannot provide.

**For every trigger above, the artifact `docs/ai/context-pruning-engine-behavioral-spec.md`
must exist BEFORE it can fire**, and the spec must carry independently-authored
contents subject to the clean-room restrictions in §3 (SQ1). **A legal concern alone
does not trigger P2** — a license question motivates care (counsel, clean-room) but does
not, by itself, justify a reimplementation; only a measured correctness/determinism/
breakage failure does.

## 7. Evidence / Provenance

Verified this slice by direct read of the cited artifacts, `git show --stat` of the two
commits, and targeted grep of the reference checkouts. All claims below are grounded in
commands run during this slice; nothing is asserted from the prior in-chat debate
without re-verification.

| Claim | Verifying artifact / command | Verified |
| --- | --- | --- |
| Repo `LICENSE` is MIT (not AGPL) → AGPL tension is external | `LICENSE:1` ("MIT License") | yes |
| `.opencode/dcp.jsonc` does not exist anywhere in repo | `glob .opencode/dcp.jsonc` → No files found | yes |
| Only `core/media-perception` declares CoreOutputs; exactly 2 paths | `internal/resolver/catalog.go:286-302` | yes |
| No capability declares a DCP-related CoreOutput (exact current P1 declaration = none) | `git grep -n CoreOutputs -- *.go *.yml *.yaml *.jsonc *.json *.tmpl` (only media-perception) | yes |
| CoreOutputs keys are source-relative LIVE embedded-core paths (gates only generic embedded core outputs) | `internal/resolver/core_outputs.go:20-26,33-51`; `internal/resolver/manifest.go:80-101` | yes |
| CoreOutputs deselection leaves residue (untouched, drift-exempt) — may be live if an installed reader acts on it | `core_outputs.go:14-18,46-50`; `manifest.go:84-88` (residue-exempt ≠ inert-to-installed-readers, inferred from "left untouched") | yes |
| `ae5b30d` = capability-owned CoreOutputs filtering (resolver/substrate/CLI + tests) | `git show --stat ae5b30d` (21 files, +1703/-97) | yes |
| `ba68c76` = deny-first `read_only` HarnessPolicy resolving under `findLast` | `git show --stat ba68c76` (6 files, +1266/-65: model/tables/emit/test) | yes |
| OpenCode native compaction summary uses `tools: []` (cannot use tools during summary generation) | `refs/opencode/packages/core/src/session/compaction.ts:195-203` (`tools: []` in the `LLM.request`) | yes |
| OpenCode compaction publishes events, no blocking PreCompact hook in the summary path | `refs/opencode/packages/core/src/session/compaction.ts:186,215` (`Compaction.Started`/`Ended` events; no interception hook in `compactAfterOverflow`) | yes |
| PreCompact/PostCompact blocking-hook seam exists only in a sibling (Claude Code), not OpenCode | `strings refs/cc-re/extracted/native/package/claude` → "Compaction blocked by PreCompact hook", PreCompact/PostCompact | yes |
| Claude Code binary embeds an AGPL/GPL/etc. license-detection regex (AGPL is a live operational concern) | `strings refs/cc-re/extracted/native/package/claude` → `agpl[.-]*...AGPL-*[0-9]*...` | yes |
| Reverse-engineering / source-study corpus exists under `refs/` | `refs/cc-re/extracted/{native,package}/` (claude binary + npm package) | yes |
| §4.2 premise-recheck protocol is owned by the 2026-07-22 record (cited, not redesigned) | `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` §Mechanism §4.2 + Honest limitation #2 | yes |
| Case-study scope is stable for F1–F7 / §4 mechanisms; §1.1 & §1.5 are known errors excluded from reliance | `docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md` Revisions §2026-07-23 + §3/§4; `2026-07-22-...md` Source corrections §1.1/§1.5 | yes |
| Neither context-pruning spec exists yet (`behavioral-spec`, `escalation-spec`) | `glob docs/ai/**/*.md` → only `template-authoring.md`, `plugin-auto-injection.md`, `derive-infer-vocabulary.md` | yes |

**House style:** this memo follows the `2026-07-05-commit-gate-shared-file-coupling.md`
convention (bolded-metadata header as frontmatter; Framing → Decision → Mechanism →
Tradeoffs → Deferred → Open questions → Evidence sections), matching the
`2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` house-style note.

**Out of scope, deliberately not done in this slice:** no implementation, no change to
DCP configuration / capabilities / overlays, no authoring of either `docs/ai/` spec
(those are named as *deferred* preconditions, not produced here), no edit to the
§4.2-protocol owner record, no edit to the case study, and no unrelated docs cleanup.
No claim above depends on case-study §1.1 ("orchestrator") or §1.5 ("3/3 modalities"),
both of which carry recorded corrections.
