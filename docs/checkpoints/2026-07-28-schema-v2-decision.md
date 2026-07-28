# Decision Record — schema_v2 Panel Structural Capacity (Track-1 S1)

**Date:** 2026-07-28
**Slice:** Track-1 S1 — `schema_version` 1→2 (additive `panel` block).
**Status:** **LANDED** as its own reviewed commit (see Landing addendum §9).
**Scope:** `templates/core/.opencode/config/review-tiers.json`
(rendered mirror `.opencode/config/review-tiers.json` regenerated via
`make update`). No source-code change; no `commit-reviewer*.md` change
(attribution is coordinator-derived — see §9).

> **Provenance.** This record was drafted under gitignored `tmp/` while a
> concurrent commit-gate lane was reverting hunks in the shared
> `review-tiers.json` to land its own S1+S2 work. It moves here, unchanged in
> its decision content, when the schema_v2 slice lands as its own reviewed
> commit. The Landing addendum (§9) records the attribution decision, the
> re-verification, and the closeout.

---

## 1. What the slice is

A **single additive change**: bump `schema_version` 1→2 in `review-tiers.json`
and add an optional `panel` section. It is **additive, backward-compatible, and
domain-free**.

### 1.1 The `panel` block (additions)

- **`roles`** — generic role IDs (no model names):
  - `r1_redundant` — redundant generalist block-finder (the DEFAULT role for
    every leaf).
  - `r1_backup_secondary` — secondary backup block-finder (redundant slot,
    model-agnostic; preserves block-finding when a primary leaf is unavailable).
  - `r2_contract_data` — contract/data-integrity specialist lens; authoritative
    owner for `contract_drift` and `data_integrity` (the loss-weakest class).
  - `r3_security` — security specialist lens.
  - `r4_blind_spot` — blind-spot / spec-conformance lens. **RESERVED — no leaf
    assigned in the default `leaf_roles`** (the 5-leaf reshape is NOT approved;
    see non-goals + the S3/Branch-B resolution).
- **`lenses`** — `block_finder` (generalist, all finding classes) ships as the
  default lens.
- **`leaf_roles`** — every default leaf (`commit-reviewer-a..d`) is
  `r1_redundant` / `block_finder`. A leaf assumes a specialized role ONLY when
  its entry is changed from this default.
- **`class_ownership`** — maps every finding class to an owning role. The
  DEFAULT routes every class to `r1_redundant` (current behavior), with `*` as
  the fallback owner.
- **`quorum`** — rule `owner_or_redundant`: a class-C BLOCK is **upheld** if
  ≥1 owner-role lens for C raises it OR ≥1 `r1_redundant` leaf raises it. A
  non-owner, non-redundant lens **cannot outvote the owner on the owner's
  class** (no cross-lens outvote). The existing evidence-verifiability rule
  still applies. **Under the shipped defaults this reduces EXACTLY to current
  behavior** (a class-C BLOCK is upheld if any leaf raises it with evidence).
- **`escalation`** — cross-lens conflict disposition. When owner-lenses
  disagree across classes on the same finding, record an
  `escalation_disclosures[]` entry. **DISCLOSURE ONLY** (like `axis_conflict`):
  surfaces the conflict verbatim and NEVER changes the collapsed verdict
  (still blocked > split > approve). The authority line is unchanged — lenses
  propose, the coordinator + commit-gate dispose.
- **Leaf attribution** — the OPTIONAL `role` / `lens` / `class_owner` output
  fields an earlier session added to `commit-reviewer*.md` are NOT required:
  attribution is **coordinator-derived** at merge time from `leaf_roles` (see
  §9). The lane's revert of those prompt fields is therefore CLEAN.

### 1.2 Domain-free guarantee

Core ships **generic role/lens IDs only** — no model names, no project
specifics. **Model identity stays in per-repo `.local/config/agent-model/<leaf>`
(gitignored).** A role's ranked primary/fallback model chain is expressed by
assigning multiple leaves to the same role and relying on tier/leaf ordering;
each leaf resolves its own model. This keeps model identity in one place and
the fallback chain as leaf ordering, not a parallel model-id list. (This is the
mechanism that makes a slot model-independent by construction.)

---

## 2. Review verdict (operator + commit-review)

**Verdict: APPROVED — additive / backward-compatible / domain-free confirmed.**

- **Operator review:** the slice adds structural capacity only; it does not
  change behavior under defaults. Approved to land as its own reviewed commit
  once the file conflict clears.
- **commit-review findings:**
  - **F1 (BLOCK-graded, then FIXED):** GPT-brand literals appeared in
    `templates/core/` (the domain-free core). **FIXED by de-branding** —
    `r1_backup_non_gpt`→`r1_backup_secondary`, "Non-GPT backup"→"Secondary
    backup", "GPT-loss-weakest"→"loss-weakest", "GPT-independence"→"model-
    independence". **Zero behavioral impact**: the `r1_backup_secondary` role
    is unreferenced in the default `leaf_roles` (no leaf holds it), so the
    rename changes no runtime assignment. De-branded terms confirmed present in
    the rendered config.
  - **C2 (advisory, ACCEPTED as deferred):** tighten the `quorum` prose.
    Accepted — will be tightened when roles actually specialize (future S2+);
    the current prose is correct under defaults.
  - **C3 (advisory, ACCEPTED as deferred):** add fixtures exercising the
    per-class quorum once roles specialize. Accepted — fixtures are not
    warranted while every leaf is `r1_redundant` (the quorum reduces to "any
    leaf raises it"); they become meaningful when a leaf takes
    `r2_contract_data` / `r3_security`.

---

## 3. Decision rationale

- **Structural capacity, not behavior.** The slice lands the schema/config
  shape needed to route specialized prompts and weigh findings by class
  ownership — WITHOUT hardcoding domain rules and WITHOUT changing today's
  behavior. Defaults reduce to the pre-panel uniform 4-leaf redundant-block-
  finder exactly.
- **Prep for O3 (not O3 itself).** The schema gives O3 a place to land a leaf
  in `r2_contract_data` and to route `contract_drift`/`data_integrity` to it.
  **But O3 is build-as-experiment, gated on its own A/B** (see the slice-plan
  reconciliation log): the R2 lens finding-survival uplift is UNMEASURED (the
  A/B harness preflight-blocks, never run). This slice does not assert O3 works;
  it only removes the structural blocker.
- **Per-class quorum is the integration primitive.** `owner_or_redundant`
  preserves the "lens finds; coordinator + gate act" authority line: an owner
  lens cannot be outvoted on its own class, and a redundant block-finder can
  always uphold. No cross-lens outvote prevents a loud-imprecise lens from
  dropping a quiet-precise owner's finding.
- **Disclosure-only escalation is the safe failure mode.** Cross-lens conflict
  surfaces verbatim without ever flipping the collapsed verdict — so a
  misconfigured class-ownership map degrades to "more disclosure," never to
  "wrong block/approve."

---

## 4. Status (pre-landing)

- **Implemented + verified** by the implementing lane: `go test ./...`,
  `gofmt`, `go vet` pass.
- The schema_v2 change was **held** while the concurrent commit-gate lane
  landed its S1+S2 work in the shared `review-tiers.json` (lane landing at
  `4bc821e`, panel-free).
- **Landing plan (executed — see §9):** once the lane cleared, the slice
  commits as its OWN reviewed commit (run `/commit-review` on the exact
  `review-tiers.json` + template diff first), then this memo moves to
  `docs/checkpoints/`.

---

## 5. Verification

| Claim | Verifying command / output | Verified |
|---|---|---|
| `schema_version` is 2 with an additive `panel` section | read of `.opencode/config/review-tiers.json`: `schema_version: 2`, `panel:` block present | yes |
| Default `leaf_roles` assigns every leaf `r1_redundant`/`block_finder` | read of `review-tiers.json` `leaf_roles`: a/b/c/d all `r1_redundant` + `block_finder` | yes |
| Default `class_ownership` routes every class to `r1_redundant` | read of `review-tiers.json` `class_ownership`: security/data_integrity/contract_drift/correctness/`*` all `r1_redundant` | yes |
| `quorum.rule = owner_or_redundant`; reduces to current under defaults | read of `review-tiers.json` `quorum` comment + `rule`/`owner_predicate`/`redundant_role` | yes (structure; behavioral reduction is by-design + test-asserted by lane) |
| `escalation` is disclose-only → `escalation_disclosures[]` | read of `review-tiers.json` `escalation` comment + `field: escalation_disclosures` | yes |
| Domain-free: no model names in core roles/lenses | read of `roles`/`lenses`: only generic IDs (`r1_redundant`, `block_finder`, …); model identity documented as `.local/config/agent-model/<leaf>` | yes |
| F1 de-branding landed (no brand literals in role/lens IDs) | read shows `r1_backup_secondary`, "Secondary backup", "loss-weakest", "model-agnostic" in place | yes (terms present); residual-literal re-scan re-run at landing — see §9 |
| F1 fix has zero behavioral impact (`r1_backup_secondary` unreferenced in default `leaf_roles`) | read of `leaf_roles`: no leaf holds `r1_backup_secondary` | yes |
| Leaf attribution is coordinator-derived (no leaf-emitted fields required) | design: coordinator reads `leaf_roles[<leaf>].role` + `class_ownership[<class>]` at merge — see §9 | yes (design) |
| Build + tests pass | `go test ./...` / `gofmt` / `go vet` (re-run at landing — see §9) | yes |

---

## 6. Behavioral closure

```behavioral-closure
crux: This slice is additive structural capacity that reduces to current
  behavior under defaults. The load-bearing path is "existing 4-leaf default
  parses are unaffected AND the new `panel` block parses AND build/tests pass."
verifier: go test ./... + gofmt + go vet + doctor (auto-classifier shape-valid)
  + make update (clean render) + jq parse, plus existing coordinator-round
  fixtures still merging under default config.
result: proven
verdict: proven
note: Re-verified at landing (see §9). go test ./... PASS, gofmt clean, go vet
  clean, doctor HEALTHY (auto-classifier PASS — review-tiers.json shape-valid;
  dev-stale-embed PASS), make update reconciled (review-tiers.json
  managed-unchanged after render, proving source==rendered), jq parse valid.
  The panel block is unread under defaults (tier_cascade.py ignores the unknown
  `panel` key; no code path consults panel fields when every leaf is
  r1_redundant), so the no-regression outcome is demonstrated by the passing
  suite + the structural fact that defaults leave the panel inert. The slice's
  behavioral claim is "no change by design" (defaults reduce to current), not a
  new behavior to prove.
```

---

## 7. Non-goals (out of scope for this slice)

- **NO behavior change.** Defaults reduce to current; the slice adds capacity,
  not routing.
- **NO 5-leaf R4 activation.** `r4_blind_spot` is RESERVED with no leaf. The
  S3 probe returned **Branch B** (0/39 real-wrongly-dropped; the blind-spot gap
  is an illusion) — R4 is NOT warranted and is NOT activated here.
- **NO O3 declaration.** The schema PREPARES for O3 (a leaf in
  `r2_contract_data`), but O3 is build-as-experiment, gated on its own
  finding-survival A/B (uplift currently unmeasured).
- **NO commit-gate changes.** `commit-gate.sh` and active blocking policy are
  untouched; the quorum rule and escalation are disclosure/merge-layer, not
  gate logic.
- **NO model identity in core.** Model names stay in per-repo
  `.local/config/agent-model/`.
- **NO leaf-emitted attribution surface.** Attribution is coordinator-derived
  (see §9); the `commit-reviewer*.md` revert is left clean.

---

## 8. Persist-guard compliance

This memo contains **aggregate/structural descriptions only** — generic role
and lens IDs, schema field names, and review-verdict prose. No model names, no
consumer-repo names/paths, no verbatim finding text, no code snippets, no
`/home/` absolute paths (`.local/config/agent-model/` is referenced
repo-relative). The de-branding removed the only brand literals that touched
the domain-free core.

---

## 9. Landing addendum (2026-07-28)

The commit-gate lane landed at `4bc821e` (panel-free), clearing the shared
`review-tiers.json` for this slice. The schema_v2 reapply was reapplied onto
the post-lane tree and lands here as its own reviewed commit.

### 9.1 Attribution decision — coordinator-derived (clean revert)

The lane's landing **reverted** the optional `role` / `lens` / `class_owner`
output fields an earlier session had added to `commit-reviewer*.md`. The
question for this slice: does the per-class-quorum coordinator-merge **require**
leaves to **emit** those fields?

**Decision: NO — attribution is coordinator-derived. The lane's revert is
CLEAN. This slice touches `review-tiers.json` only (no `commit-reviewer*.md`).**

**Rationale.** The coordinator dispatches the leaves and owns the merge. At
merge time it already knows which leaf produced which finding, so it can
**derive** each leaf's role from `panel.leaf_roles[<leaf>].role` and the class
owner from `panel.class_ownership[<class>]`, then apply the `quorum` rule.
Leaves continue to emit only findings (with `class` as today). This:

- keeps a **single source of truth** (`leaf_roles` in config) — no second
  per-leaf attribution surface that would have to be kept in sync with config
  (dual-derivation hazard);
- matches the operator-confirmed config direction ("config-driven via
  `leaf_roles`", "model identity in ONE place");
- makes the lane's `commit-reviewer*.md` revert clean — leaf prompts need no
  role tagging.

Leaf-emitted attribution would be required only if leaves ran in a context that
could not be correlated back to their config entry — which is not the case (the
coordinator dispatches them by name and owns the merge). The optional output
fields remain a *possible* future optimization (a leaf self-declaring its role
to short-circuit a config lookup), but they are not needed for the quorum to
function and are deliberately not re-introduced here.

### 9.2 Re-verification at landing

Re-run on the post-lane tree (HEAD `e8aafc4`, after the concurrent researches
commit advanced HEAD past the lane landing):

| Check | Result |
|---|---|
| `go test ./...` | PASS (all packages; `internal/cli`, `internal/permission`, `tests/integration` ran uncached) |
| `gofmt -l .` | clean (no output) |
| `go vet ./...` | clean |
| `bin/vh-agent-harness doctor` | HEALTHY — 0 problems; `auto-classifier PASS` (review-tiers.json shape-valid); `dev-stale-embed PASS` (embedded corpus matches checkout); `behavioral-closure PASS` |
| `make update` | reconciled 213 files — review-tiers.json `managed-unchanged` after render (proves source==rendered); no new diff introduced onto the rendered config |
| `jq` parse (source + rendered) | valid JSON; `schema_version: 2`; `panel` present; 5 roles; 4 leaves all `r1_redundant`/`block_finder`; every class → `r1_redundant`; `quorum.rule=owner_or_redundant`; `escalation.disposition=disclose_only` |
| brand-literal re-scan (`rg -ni 'gpt\|non-gpt\|openai\|claude\|anthropic\|gemini'` over both review-tiers.json) | NO MATCHES — domain-free confirmed; F1 de-branding holds |
| source == rendered | confirmed via `make update` leaving the rendered file `managed-unchanged` |

### 9.3 Backward-compat (load-bearing claim)

Under the shipped defaults the `panel` block is **structurally inert**: every
leaf is `r1_redundant`, every class owns to `r1_redundant`, and `quorum =
owner_or_redundant` reduces to "a class-C BLOCK is upheld if any leaf raises it
with evidence" — exactly the pre-panel behavior. `tier_cascade.py` ignores the
unknown `panel` key (schema_v2 is structural capacity, not enforcement). No
behavior change by default; the passing suite (including existing coordinator-
round merge fixtures) is the outcome observation for no-regression.

### 9.4 Files in this commit

- `templates/core/.opencode/config/review-tiers.json` (source: schema_version
  1→2 + additive `panel` block)
- `.opencode/config/review-tiers.json` (rendered mirror, regenerated via
  `make update`)
- `docs/checkpoints/2026-07-28-schema-v2-decision.md` (this record, moved from
  gitignored `tmp/`)

No `commit-reviewer*.md` change (coordinator-derived attribution; clean
revert). No source-code change. Concurrent sessions' work (the researches
addendum at `e8aafc4`; an unrelated in-flight `complexity-policy.yml` toggle)
is explicitly **not** bundled — the gated-commit private-index excludes it and
only this slice's explicit file list is staged.
