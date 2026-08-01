**Date:** 2026-08-02
**Status:** Accepted (record-of-decision). Design SETTLED via a `/solution-brief`-style researcher study + multi-perspective debate pass; confidence **MEDIUM**. This memo authorizes but does not land a backlog slice implementing the resolution — that slice is gated on this memo being committed first.
**Supersedes:** None. Extends, but does not reopen, the F4-C defer-liveness family defined by `2026-07-30-defer-liveness-release-gate.md` and the 2026-07-23 dual-mechanism reconciliation. It records a divergence those two memos did not anticipate and resolves it without narrowing either committed gate.
**See also:**
- `researches/decisions/2026-07-30-defer-liveness-release-gate.md` (F4-C predecessor — established the "two independent surfaces" topology this memo extends)
- `researches/decisions/2026-07-23-release-defer-dual-mechanism-reconciliation.md` (union fail-closed defense-in-depth; "both must pass independently")
- `internal/cli/release_gate.go` (`checkDeferLiveness`, `evaluateDeferRecurrence`, `VH_HARNESS_DEFER_OVERRIDE_IDS`)
- `internal/memory/claims/claim.go` (`loadLivenessCards` — silencer-immune pool)
- `.opencode/scripts/check-defer-triggers.js` (JS release DEFER gate, manifest-authority)
- `.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md` (manifest provenance scoping policy)

# F4-C defer-liveness provenance-scope divergence — widen the manifest to the Go gate's breadth (option b), keep the Go gate unchanged, ship a runbook complement

## Framing

The F4-C predecessor (`2026-07-30-defer-liveness-release-gate.md:42-44`) committed two **independent** enforcement surfaces around the same defer-liveness pool, deliberately: a doctor/tag-time Go gate that reads `.local/coordinator/tasks/` directly, and a manifest-authority JS evaluator that reads the committed disposition record. The two were designed to disagree on vocabulary and failure mode, not to silently disagree on **which cards exist**.

The v0.19.0 release ceremony on 2026-08-02 surfaced an unanticipated interaction between those two surfaces: a card can be **invisible to the JS gate** (GREEN / `disclose`) and simultaneously a **blocker under the Go gate** (FAIL → G0 + G0c RED). The operator disposed of the blocker destructively (`rm` of the gitignored transport cards — the sanctioned retire path per AGENTS.md, but destructive nonetheless) to unblock the tag, rather than the non-destructive escape hatch the gate already provides.

A researcher study + multi-perspective debate pass converged on the disposition recorded here. This memo is the gating durable artifact: the backlog slice implementing the resolution must NOT land before this memo is committed.

## The confirmed divergence (settled)

The two defer-liveness enforcement surfaces read **different data sources** with **different scopes**, and the difference is **structural**, not configurable.

### JS release DEFER gate — manifest-authority, manifest-scoped

- **Host:** `mainRelease`, `.opencode/scripts/check-defer-triggers.js:835-1067`.
- **Input:** the committed manifest ONLY, read via `git show HEAD:<path>` (`.opencode/scripts/check-defer-triggers.js:879`). It does not read the worktree or `.local/`.
- **Disposition matrix:** `.opencode/scripts/check-defer-triggers.js:1008-1035`, applied with **NO source filter**. The `source_ref` field (`.opencode/scripts/check-defer-triggers.js:698-700`) is documented as "provenance text; never dereferenced."
- **Release-prep / promoter:** `mainReleasePrep:1204-1280` and `mainPromoter:1071-1132` likewise apply **NO source filter**.
- **Consequence:** the "exclusion" of non-`review-defer` cards from the JS gate is **BY CONSTRUCTION** — such cards are never authored into the manifest — not a runtime predicate. A card absent from the manifest is mechanically `disclose` / GREEN.

### Go defer-liveness gate — `.local/`-direct, all-source, silencer-immune

- **Host:** `checkDeferLiveness`, `internal/cli/release_gate.go:131`; recurrence predicate `evaluateDeferRecurrence`, `internal/cli/release_gate.go:415-493`.
- **Input:** EVERY file under `.local/coordinator/tasks/`, via `loadLivenessCards`, `internal/memory/claims/claim.go:458-502`. The loader is **deliberately silencer-immune** (`internal/memory/claims/claim.go:434-440`: "NO prefix(defer-/errata-) filter and NO extension(.json) filter").
- **Structural constraint:** the `LivenessCard` struct (`internal/memory/claims/claim.go:479-498`) has **NO `Source` field** → it cannot filter by provenance even if it wanted to.
- **Disposition-satisfaction** (`internal/memory/claims/claim.go:450-460`) recognizes only three states: closed status (`StatusIsClosed`), manifest entry (`manifestIDs`), or operator override (`overrideIDSet`).

### The resulting window

A non-`review-defer` OPEN card whose `path_touched(...)` trigger re-fires in the release arc is:

- **invisible to the JS gate** (not in the manifest → GREEN / `disclose`), AND
- **a blocker under the Go gate** when a release is imminent (`releaseImminent` at `internal/cli/release_gate.go:239` → FAIL → G0 + G0c RED).

The two surfaces disagree on whether the card exists for the purposes of release — the exact silent disagreement the predecessor's "independent second surface" language was meant to forbid in spirit even though neither prior memo named this provenance axis.

## The incident that surfaced it

The v0.19.0 release ceremony (tagged + pushed `v0.19.0` on 2026-08-02). Two OPEN transport cards in `.local/coordinator/tasks/` whose `path_touched(.opencode/scripts/check-defer-triggers.js)` trigger re-fired in the `v0.18.0..HEAD` arc (the script was touched 3×: `09fe9eb`, `5056596`, `f29babf`):

1. `defer-rename-check-defer-triggers-esm-ext` — provenance `source:p2-followup`.
2. `defer-promotion-flow-is-work-already-done-check` — provenance `source:external-study`.

Both were JS-green / Go-blocking mid-ceremony. The operator unblocked the tag via destructive `rm` of the gitignored transport files (the sanctioned retire path per AGENTS.md, but destructive) rather than the non-destructive escape hatch documented below. No claim was lost — both were genuinely work-already-done — but the **recovery path** took the destructive branch when a non-destructive branch existed, and the **invisibility** (JS-green) was the surprise that made the destructive branch feel forced.

## Intent — both surfaces are intentional; the friction is the unanticipated interaction

Neither surface is a bug.

- **Manifest provenance scoping is intentional.** Documented agent policy at `.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md:597-601`, and pinned by the render test `internal/cli/harness_dogfood_render_test.go:411`, which asserts the literal string `"source:p2-followup exclusion"`. The manifest was designed to carry only release-relevant dispositions, and "release-relevant" was operationalized as "the sources the release-prep enumerator authorizes into the manifest."
- **Go all-source / silencer-immune treatment is intentional.** Predecessor memo `2026-07-30-defer-liveness-release-gate.md:44` and `:56-60` ("independent second surface"), plus the code comments at `internal/cli/release_gate.go:380-383` and `internal/memory/claims/claim.go:434-450`: the pool must read **every** file so "a card cannot escape the gate by being named without the `defer-` prefix or by being `.md` instead of `.json`." This is the silencer-immunity contract; it is load-bearing for the F4 family.
- **What was NOT anticipated:** a non-`review-defer` provenance (`external-study`) firing into the release arc and blocking the ceremony. The two surfaces were each designed in isolation around their own correctness criteria; no prior memo established that their **card universes** must agree.

## The external-study enumeration gap

The provenance policy text (`.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md:597-601`) enumerates ONLY `review-defer` and `p2-followup`. `external-study` — the provenance of one of the two blocking cards in the v0.19.0 incident — is **not enumerated**. This was an open design question; this memo resolves it:

> **`external-study` provenance disposition:** release-relevant — but "release-relevant" means **INCLUDED in release prep with an explicit disposition** (frequently a non-blocking `disclose`), NOT an automatic blocker.

The default disposition for an `external-study` card is therefore `disclose` (acknowledge the study's existence in the release record without blocking), escalating to `block` only when the card's content actually contradicts the release.

## The existing non-destructive escape hatch (must be documented)

`VH_HARNESS_DEFER_OVERRIDE_IDS` is a non-destructive operator override that satisfies the Go gate **without deleting transport cards**:

- **Declaration:** `internal/cli/release_gate.go:530-538`.
- **Surfacing:** emitted in the FAIL detail at `internal/cli/release_gate.go:293` whenever the gate refuses, so the operator is told the escape hatch exists at the moment it is needed.
- **Effect:** a card listed in `VH_HARNESS_DEFER_OVERRIDE_IDS` is treated as disposition-satisfied (`overrideIDSet` at `internal/memory/claims/claim.go:450-460`) for the duration of the release. The transport card stays on disk for later curation; only the ceremony is unblocked.

The operator used destructive `rm` during the v0.19.0 ceremony instead. This memo + the runbook complement (below) exist to prevent that recurrence: the override is the canonical ceremony-unblock; `rm` is the canonical retire path for a card whose disposition is genuinely "done" and that you intend to forget.

## Options considered

Three options were on the table. All three are recorded here; the chosen one is marked.

### (a) Make the Go test gate apply the same provenance exclusion as the release gate. — REJECTED

- **Changes:** parse `source:` from `owner_notes` in `loadLivenessCards`; add a `Source` field to `LivenessCard`; filter non-`review-defer` cards out of `evaluateDeferRecurrence`; update recurrence tests.
- **Risk: HIGH — weakens the silencer-immunity guardrail.** Contradicts the documented contract at `internal/memory/claims/claim.go:434-450` and inverts the read-state-directly discipline established by the predecessor memo (`2026-07-30-defer-liveness-release-gate.md:18`). It also makes "what is release-relevant" depend on a free-text provenance string the design never trusted for gating — the same class of trust that produced the erratum-v0120 decay the F4 family exists to prevent.
- **Verdict: REJECTED.**

### (b) Widen the release manifest scope so both surfaces treat all sources uniformly. — CHOSEN

- **Changes:** NO JS code change. The release-prep evaluator already processes every manifest record uniformly (no source filter at `.opencode/scripts/check-defer-triggers.js:1008-1035`), and the release-prep enumerator already surfaces cards from all sources. The change is to the **releaser / release-readiness agent policy**: every firing card the release-prep enumerator surfaces gets an explicit, committed disposition (frequently non-blocking `disclose`), regardless of provenance. The render test `internal/cli/harness_dogfood_render_test.go:411` (which currently pins `"source:p2-followup exclusion"`) is updated to assert the widened scope.
- **Risk: LOW-MEDIUM — tightens the release gate** (more cards now require an explicit disposition). No guardrail is weakened; consistency is achieved by raising the manifest surface to the Go gate's breadth, not by lowering the Go gate to the manifest's subset.
- **Verdict: CHOSEN, confidence MEDIUM.** Evidence proves the failure mode (v0.19.0 incident) but does not quantify future non-`review-defer` card volume or the ceremony burden the widening imposes.

### (c) Doc-only — document the intentional split + a runbook note. — NOT chosen alone (adopted as COMPLEMENT)

- **Changes:** docs/runbook only.
- **Risk:** NONE to code, NONE to guardrails.
- **Verdict: NOT chosen alone** — it preserves the surprise trap (JS-green / Go-blocking). Adopted as a **COMPLEMENT** to (b): document the `VH_HARNESS_DEFER_OVERRIDE_IDS` non-destructive escape hatch (`internal/cli/release_gate.go:530-538`) and the rule "do not `rm` transport cards to clear the gate" in the releaser runbook.

## Decision (settled)

**Option (b):** widen the release manifest policy so every firing card surfaced by release-prep gets an explicit, committed disposition, while keeping the Go all-live defer-liveness gate **UNCHANGED**. Ship option (c)'s runbook clarification as a **COMPLEMENT** — document the `VH_HARNESS_DEFER_OVERRIDE_IDS` non-destructive escape hatch and the "do not `rm` transport cards to clear the gate" rule.

**`external-study` provenance disposition:** release-relevant — meaning INCLUDED in release prep with an explicit disposition (often non-blocking `disclose`), NOT automatic blocker.

**Confidence: MEDIUM.** Evidence proves the failure mode but does not quantify future non-`review-defer` card volume or ceremony burden.

## Decisive reasoning for (b)

The all-live, silencer-immune Go gate must not be narrowed → **(a) is ruled out.** Once Go cannot be narrowed, the only safe direction for consistency is **OUTWARD**: raise the manifest surface to the Go gate's breadth, rather than lower the Go gate to the manifest's subset. The intentional "two independent surfaces" topology (predecessor memo `2026-07-30-defer-liveness-release-gate.md:56-60`) argues for **keeping the broader Go backstop while improving the disposition surface around it** — not for opaque disagreement between the two.

The v0.19.0 incident is **dispositive against (c) alone.** The override fixes the **RECOVERY** path (how to unblock the tag when the Go gate fires); it does nothing for the **INVISIBILITY** (the JS gate reporting GREEN while the Go gate is RED). (c) preserves the surprise trap that made the destructive `rm` feel forced in the first place. Adopting (c) as a complement to (b) is correct: (b) removes the surprise, (c) documents the recovery.

## What would change the resolution

Two conditions would flip the choice to (c):

1. A measured release history showing that non-`review-defer` Go blockers are **systematically false-positive process debt** (the cards are always work-already-done, never genuine live debt), AND
2. An override-first runbook that **reliably** prevents deletion (operators reach for `VH_HARNESS_DEFER_OVERRIDE_IDS` first, not `rm`).

No fact would make **(a)** preferable without a replacement design that preserves silencer-immunity while proving provenance cannot suppress genuine live debt — i.e. without first solving the problem the F4 family exists to solve. (a) is therefore not a future option under the current design; it would require reopening the F4-C contract.

## Authority-line summary

| Surface | Classification | Reads | Source filter | Acts at |
|---|---|---|---|---|
| Go defer-liveness gate (`checkDeferLiveness`) | **GATE-SHAPED CONVERSION** (reads `.local/` directly) | every file under `.local/coordinator/tasks/` | **NONE** (silencer-immune by construction; `LivenessCard` has no `Source` field) | tag time (G0 + G0c) |
| JS release DEFER gate (`mainRelease`) | **GATE-SHAPED CONVERSION** over the committed record | committed manifest via `git show HEAD:<path>` | **NONE in evaluator** (exclusion is by-construction: non-authored cards are absent) | tag time (release ceremony) |
| `VH_HARNESS_DEFER_OVERRIDE_IDS` | **OPERATOR OVERRIDE** (non-destructive) | env var | n/a — names specific card IDs to satisfy | ceremony-unblock |
| `rm` of gitignored transport card | **OPERATOR RETIRE** (destructive, sanctioned) | filesystem | n/a — deletes the card | genuine "done, forget it" |

Under option (b), the manifest-scope row rises to match the Go gate's breadth; the Go row is unchanged.

## Downstream tracking

This memo authorizes but does not land the following slices. They are recorded here so the coordinator can schedule them.

- **Backlog slice (gated on this memo — do not land the slice before this memo is committed):** implement option (b). Update the releaser / release-readiness agent policy (`.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md`) so every firing card the release-prep enumerator surfaces gets an explicit, committed disposition. Update the render test `internal/cli/harness_dogfood_render_test.go:411` to assert the widened scope (replacing the `"source:p2-followup exclusion"` assertion). **The Go gate (`internal/cli/release_gate.go`, `internal/memory/claims/claim.go`) is UNTOUCHED.** Separately, ship the (c) runbook complement in the same slice: document `VH_HARNESS_DEFER_OVERRIDE_IDS` and the "do not `rm` transport cards to clear the gate" rule.
- **Separate lower-priority slice (NOT gated on this memo — independently schedulable):** the `.js → .mjs` rename of `.opencode/scripts/check-defer-triggers.js`. ESM `import` under `.js` is node-version-fragile in the Go scratch tests that copy the script without the governing `package.json`. This is a **robustness improvement, NOT a ceremony fix** — under option (b) the rename does not change ceremony behavior. The two blocking cards in the v0.19.0 incident were NOT caused by the `.js` extension; they were caused by the provenance-scope divergence this memo resolves.

## Non-goals

- Do NOT narrow the Go defer-liveness gate. Silencer-immunity is load-bearing for the F4 family.
- Do NOT merge the JS manifest vocabulary into the Go gate's vocabulary, or vice versa. The two surfaces were designed to check different things (predecessor: `2026-07-30-defer-liveness-release-gate.md:31-38`); this memo aligns their **card universe**, not their semantics.
- Do NOT auto-promote every `external-study` card to a `block` disposition. `external-study` is release-relevant (must be dispositioned), not automatically blocking.
- Do NOT treat this memo as authorization to land the (b) slice in the same commit. The (b) slice is a separate coordinator/operator scheduling decision.

## Evidence / Provenance

| Evidence | Source | Verifying command |
|---|---|---|
| JS gate is manifest-authority, manifest-scoped | `.opencode/scripts/check-defer-triggers.js:835-1067` (reads manifest via `git show HEAD:<path>` at `:879`) | `git show HEAD:.opencode/scripts/check-defer-triggers.js` |
| JS evaluator applies no source filter (exclusion is by-construction) | `.opencode/scripts/check-defer-triggers.js:698-700` (`source_ref` "never dereferenced"), `:1008-1035` (disposition matrix), `mainReleasePrep:1204-1280`, `mainPromoter:1071-1132` | `git show HEAD:.opencode/scripts/check-defer-triggers.js` |
| Go gate reads every `.local/coordinator/tasks/` file, silencer-immune | `internal/memory/claims/claim.go:434-440` (no prefix/extension filter), `:458-502` (`loadLivenessCards`), `:479-498` (`LivenessCard` has no `Source` field) | `git show HEAD:internal/memory/claims/claim.go` |
| Go recurrence predicate + releaseImminent escalation | `internal/cli/release_gate.go:131` (`checkDeferLiveness`), `:239` (`releaseImminent`), `:415-493` (`evaluateDeferRecurrence`), `:380-383` (silencer-immunity comment) | `git show HEAD:internal/cli/release_gate.go` |
| Go disposition-satisfaction recognizes only closed / manifest / override | `internal/memory/claims/claim.go:450-460` | `git show HEAD:internal/memory/claims/claim.go` |
| Non-destructive operator escape hatch exists and is surfaced | `internal/cli/release_gate.go:530-538` (declaration), `:293` (FAIL-detail surfacing) | `git show HEAD:internal/cli/release_gate.go` |
| Manifest provenance scoping is intentional + render-test-pinned | `.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md:597-601`; `internal/cli/harness_dogfood_render_test.go:411` (asserts `"source:p2-followup exclusion"`) | `git show HEAD:.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md`; `git show HEAD:internal/cli/harness_dogfood_render_test.go` |
| `external-study` provenance not enumerated in policy text | `.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md:597-601` (enumerates only `review-defer` + `p2-followup`) | `git show HEAD:.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md` |
| Predecessor "two independent surfaces" topology this memo extends | `researches/decisions/2026-07-30-defer-liveness-release-gate.md:44`, `:56-60` | `git show HEAD:researches/decisions/2026-07-30-defer-liveness-release-gate.md` |
| v0.19.0 incident (the trigger for this memo) | two OPEN transport cards in `.local/coordinator/tasks/`; script touched 3× in `v0.18.0..HEAD` (`09fe9eb`, `5056596`, `f29babf`); tag `v0.19.0` pushed 2026-08-02 | `git log --oneline v0.18.0..v0.19.0 -- .opencode/scripts/check-defer-triggers.js`; `git tag --list v0.19.0` |
| Design options + tradeoff matrix | researcher study + multi-perspective debate pass, 2026-08-02 | n/a (read-only reasoning; this memo is its durable record) |
