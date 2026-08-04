# Decision Brief: Opt-Out Pilot Distribution Design

**Date:** 2026-08-04
**Status:** Operator-signed-off 2026-08-04 — APPROVED (vehicle, migration disclosure, build-slice gating). Design-only; build slice sign-off-gated and separate.
**Basis:** Produced by a read-only `researcher → debate → planner` solution-brief chain and captured at `tmp/agent-runs/opt-out-distribution/brief.md`.

---

## Operator sign-off
Date: 2026-08-04. The operator APPROVED all three sign-off decisions:
1. **Vehicle APPROVED** — existing `features:` map with default-true values + a finite feature→pack mapping table (NOT a generic same-name rule). Operator verified the foundation: `features map[string]bool` exists (`internal/cli/profile.go:84`, `internal/schema/harness_profile.go:63`); resolved features already flow into permission emission (`internal/permconfig/transform.go:18`); the platform-default/project-override algebra is as described. The generic-rule rejection is endorsed (a generic rule would let any stray feature key activate a pack).
2. **Migration disclosure APPROVED as specified** — silent gain is not acceptable; the full disclosure list (help-migrate note + exact opt-out YAML + dry-run-first + 3 destination paths/ownership + same-path overwrite disclosure + orphan-retention-on-opt-out + restart requirement + orphan-prune UX pending build verification) must NOT be trimmed to ship faster.
3. **Build-slice gating APPROVED** — sign-off-gated; must not race the active committers.

**Operator additions recorded:**
- **Lifecycle-canon addition (build-slice item, NOT this durability fix):** the design invents a middle tier the vocabulary was missing — "shipped default-on overlay pilot" (embedded, default-enabled, consumer-disable-able, overlay_extension-owned, strictly INFORMS-only, NOT S2). That vocabulary gap is exactly why these three landed project-local by accident: the S1/S2 vocabulary had no middle tier, so the placement fell out of the vocabulary rather than being chosen. The build slice must write this tier into the skill-lifecycle canon alongside S1/S2 so the next pilot's placement is a decision, not an accident.
- **Cost clarification (neither number load-bearing):** the 680-char figure is a historical post-commit total (commit `edd5ea5`), not a current-tree measurement; a sibling discovery audit independently measured the current all-skill total at 4,283 chars. Bounded exposure is the point, not the exact figure.
- **Payoff framing:** formal-verification exists because of vh-solara's concurrency races; resolve-first exists because of a defer pile spanning both repos; contract-invariant-audit exists because of a vh-solara state audit. After this ships, vh-solara finally has them. "Where the problem is, the tool isn't" — until this lands.

---

## Verified current state (re-verified by researcher, not blindly trusted)

- Core contains **11 skill directories, but only 10 are unconditional**; `media-perception` is capability-selected.
- Shipped embedded overlays are `auto-classifier-pilot`, `release`, and `repo-mail`.
- The three pilots (`formal-verification-pilot`, `resolve-first-pilot`, `contract-invariant-audit-pilot`) and `harness-dogfood` are currently project-local (`.vh-agent-harness/overlays/`) and NOT embedded — consumers cannot even opt in today.
- Existing `overlays:` reconciliation is additive opt-in and cannot implement durable opt-out by omission.
- `features:` is `map[string]bool`; arbitrary feature keys are accepted and preserved. Platform defaults form the base; project values override platform values. It has the necessary default-true/project-false reconciliation algebra, but does NOT currently activate overlay packs.
- Current lifecycle terminology distinguishes S1 overlay pilot from S2 core promotion and has NO established term for the proposed middle tier.

## Design answers

### 1. Vehicle: existing `features:` map with default-true values

Choose the existing `features:` map with default-true values, combined with a finite feature-to-pack mapping in central overlay selection:

```yaml
features:
  contract-invariant-audit-pilot: true
  formal-verification-pilot: true
  resolve-first-pilot: true
```

A consumer opts out individually:

```yaml
features:
  formal-verification-pilot: false
```

This reuses:
- existing platform-default/project-override feature reconciliation;
- existing embedded-overlay selection and rendering;
- existing `overlay_extension` ownership.

The mapping must be an explicit table for these shipped pilots, NOT a generic rule that arbitrary feature names activate same-named packs.

Rejected alternatives:
- `overlays_disabled:` was rejected because it adds a new negative-list schema, reconciliation rules, and precedence model without improving residue handling.
- Moving into core was rejected because it changes ownership to `platform_managed` and contradicts the unresolved S2/core-promotion hold.
- Negative capability selection was rejected as unnecessarily heavy for three independent skills.

### 2. Schema and resolution

No new top-level schema field is needed. The exact profile change is three Boolean entries in the existing `features:` map.

Verified semantics:
- `features` is `map[string]bool`; arbitrary feature keys are accepted and preserved.
- Platform defaults form the base.
- Project values override platform values.
- Therefore a new platform `true` is reconciled into an existing profile, while an explicit consumer `false` persists.
- At raw template lookup, a genuinely absent key evaluates false; the default-on result comes from profile reconciliation injecting the platform default.

Effective selection should be:
1. Reconcile platform feature defaults with consumer overrides.
2. Add the mapped pilot pack when its feature is true.
3. Do not add it through the default-feature route when false.
4. Preserve existing explicit-overlay and capability-selected routes. An explicit selection can intentionally re-add a pack; `false` disables the default, rather than becoming a global veto over contradictory explicit configuration.
5. Pass the resulting pack set through the existing shared render staging path.

**Target doctor behavior:**
Doctor should report each shipped pilot as one of:
- enabled by platform default;
- disabled by consumer feature override;
- selected through an explicit overlay or capability route.

Active overlay paths should participate in managed-drift assessment. A previously rendered but now deselected skill should be reported as an advisory orphan, not make the repository unhealthy merely because the consumer opted out.

This is target behavior: current doctor managed-drift comparison omits overlay paths.

**Dry-run behavior:**
`update --dry-run` already uses the complete render and ownership plan. It should preview each newly created or changed pilot skill as `overlay_extension`, including same-path overwrite consequences.

### 3. Migration: silent gain is NOT acceptable

The release needs:
- a `vh-agent-harness help migrate` migration note;
- exact opt-out YAML;
- an instruction to run `update --dry-run` first;
- the three destination paths and their `overlay_extension` ownership;
- disclosure that an active overlay may replace a differing same-path file;
- disclosure that opting out stops future staging and management but does not automatically delete previously emitted output;
- the separate `--prune-orphans` cleanup behavior, after its exact safety and UX are verified in the build slice;
- the OpenCode restart requirement after newly rendered skills appear.

This follows the ownership contract: the gain is permitted as active `overlay_extension`, but its ownership and overwrite plan must be visible before the live update.

### 4. S2 tension: default-on is defensible only as a distinct middle state

> **Shipped default-on overlay pilot:** embedded, enabled by default, consumer-disable-able, `overlay_extension`-owned, and strictly INFORMS-only.

It is NOT S2/core promotion.

The three skills were verified to add no commit, release, doctor, update, approval, or state-transition authority. Their append configurations add no permissions or gates.

The cited cost number (operator-stated `2,598 → 773 chars ≈ ~193 tokens`) was INCORRECT. Commit `edd5ea5` records:

| Skill | Before | After |
|---|---:|---:|
| formal-verification | 927 | 237 |
| resolve-first | 907 | 218 |
| contract-invariant-audit | 638 | 225 |
| **Total** | **2,472** | **680** |

That is:
- **1,792 characters saved**;
- approximately **448 tokens saved** using the commit's characters/4 estimate;
- approximately **170 tokens total after trimming**.

The current formal-verification description changed after that commit, so 680 characters is a verified HISTORICAL post-commit total, not a verified current-tree total. Cost supports bounded exposure but does not prove effectiveness or promotion readiness.

S2-hold still means:
- no move into `templates/core/`;
- no removal of the pilot maturity signal;
- no transition or gating authority;
- no claim of validated consumer effectiveness;
- positive real-consumer evidence remains required for core graduation.

### 5. What moves

Move:
```text
.vh-agent-harness/overlays/formal-verification-pilot/
  → templates/overlays/formal-verification-pilot/

.vh-agent-harness/overlays/resolve-first-pilot/
  → templates/overlays/resolve-first-pilot/

.vh-agent-harness/overlays/contract-invariant-audit-pilot/
  → templates/overlays/contract-invariant-audit-pilot/
```

Keep local:
```text
.vh-agent-harness/overlays/harness-dogfood/
```
`harness-dogfood` contains repository-specific release machinery and explicitly says it is not a generic promotion candidate.

Retain the `-pilot` pack suffixes. Default distribution changes reach, not maturity or S2 status.

### 6. Blast radius

On their next binary update, consumers will see:
- three new feature keys reconciled into the profile, defaulting to true unless overridden;
- these generated skill files:
```text
.opencode/skills/formal-verification/SKILL.md
.opencode/skills/resolve-first/SKILL.md
.opencode/skills/contract-invariant-audit/SKILL.md
```
- `update --dry-run` entries for their `overlay_extension` create/update plan;
- after the doctor work lands, effective enablement and overlay drift/orphan reporting.

Consumers who subsequently set a feature false will stop receiving future management for that pack, but an already-rendered file remains as preserved orphan output until deliberately pruned.

Nothing gains gate or transition authority. Default-on distribution must not alter the pilots' existing INFORMS-only contracts.

## Build-slice handoff (sign-off-gated; NOT started by this pass)

The later build should be scoped to:
- the three pack moves listed above;
- platform and example profile defaults;
- finite feature-to-pack selection in `internal/cli/profile.go`;
- shared render, doctor, inventory, and associated tests;
- embedded-overlay inventory tests;
- migration documentation under `templates/migrations/`;
- `README.agent.md`;
- removal of redundant explicit pilot selections from the dogfood profile;
- regeneration of `.opencode/` only after source changes;
- **Lifecycle-canon addition:** writing the new "shipped default-on overlay pilot" tier into the skill-lifecycle canon alongside S1/S2.

The build is **sign-off-gated** and **must not race the two active committing sessions**.

Framing confidence: not supplied at kickoff; no framing-confidence shift applies.
Debate confidence: medium, with doctor and orphan UX retained as explicit build-verification obligations.

```behavioral-closure
verdict: inconclusive
result: skipped
crux_path: tmp/agent-runs/opt-out-distribution/brief.md
note: "Research, debate, and planner passes completed and the design content was produced, but the read-only solution-brief agent could not materialize the requested tmp files; no implementation was started. Persisted to tmp/ by a follow-on researcher transcription pass."
```

## Discovery / process findings

### Recurring process gap: solution-brief's read-only profile strands its own deliverable

The `solution-brief` subagent is constrained to read-only operation and CANNOT write files — including its own `tmp/` deliverable. This has now stranded solution-brief deliverables in **at least 4 lanes** (this opt-out-distribution pass being the latest): the full design exists only in the agent's text return, so a single compaction or context-archive operation loses it entirely. This is the precise prose-only failure mode the project keeps hitting.

**Required mitigation (process rule):** every `solution-brief` routing MUST be paired with an explicit researcher persist step that transcribes the full deliverable to `tmp/` byte-faithfully. Treating the solution-brief return as the durable artifact is unsafe — it lives only in volatile context.

**Root cause for a future process/design slice:** the read-only profile that keeps solution-brief safe from source mutation also prevents it from writing disposable `tmp/` research output. Either (a) the solution-brief profile should be scoped to permit `tmp/`-only writes (read-only on source, write on repo-scoped `tmp/`), or (b) the solution-brief command should auto-delegate a researcher persist as its final step. Do NOT fix this in the current opt-out build slice — it is a separate harness-process concern.

---

## Cross-References

- `researches/decisions/skill-lifecycle-management.md` (build slice will add the new middle tier here)
- `researches/decisions/2026-08-02-formal-verification-agent-proof-skill.md`
- `researches/decisions/2026-08-02-resolve-first-defer-processing.md`
- `researches/decisions/2026-07-29-contract-invariant-audit-capability.md`
