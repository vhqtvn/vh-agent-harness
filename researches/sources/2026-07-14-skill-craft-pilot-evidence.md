# Sources: Skill-Craft Pilot Evidence (tdd-loop, debugging-loop)

**Date:** 2026-07-14
**Topic:** Evidence trail for the skill S2 promotion-gate decision.
**Decision memo:**
[`../decisions/2026-07-14-skill-s2-promotion-gate.md`](../decisions/2026-07-14-skill-s2-promotion-gate.md).

This packet records the overlay-pilot evidence behind the S2 (overlay-pilot-
then-promote) discharges for the two skills in this cycle. It is intentionally a
record, not a recommendation — the decision lives in the memo.

## Confidence legend

- **HIGH** — verified against repo source, committed artifact, or reproducible
  behavior.
- **MED** — single-source claim or pilot-reported behavior; not independently
  re-verified in this slice.
- **LOW** — anecdotal; directionally useful only.

## Pilot provenance (verified)

- The pilot contributions came from **two consuming repos** — **TrueAI** and
  **VH-Solara** — responding to an adopt-question.
- **Both returned real pilot evidence** (a retrospective or forward validation
  against a real repo), not a speculative yes. This is what discharges S2's
  "real pilot" requirement.
  - **Confidence: HIGH** — recorded in the skill-craft-import session memory and
    reflected in the committed `f56b964` artifact (tdd-loop) and the committed
    `c52268a` artifact (debugging-loop, including the D2C escape the VH-Solara
    forward pilot validated).

## Finding 1 — tdd-loop / TrueAI pilot (SATISFIED)

- **Skill:** tdd-loop core skill (red→green→refactor in-loop, seam localization,
  S1 absence-contract).
- **Pilot repo:** TrueAI (retrospective pilot).
- **S2 verdict:** SATISFIED → committed `f56b964`.
- **What the pilot validated:**
  - **In-loop refactor.** The "refactor under green" step (D1A) held up under the
    TrueAI repo's real TDD loops; the pilot's "2-vs-1 outlier" framing was
    reinterpreted as alignment with canonical Beck TDD (the majority), not an
    overturn of it.
    - **Confidence: HIGH** — committed in `f56b964`.
  - **Seam discipline / localization authority-honesty (D3).** The pilot caught a
    localization seam-map citing a `billing` package that **did not exist** in the
    consumer's current repo state — an aspirational authority reference. This
    produced the D3 rule: authority references must be real + verifiable, and the
    consumer's `AGENTS.md` testing-rules section is localized in the **same slice**
    to avoid two-sources-of-truth drift.
    - **Confidence: HIGH** — the `billing` catch is the concrete incident; D3 is
      the encoded rule, committed in `f56b964`.

## Finding 2 — debugging-loop / TrueAI pilot (SATISFIED for the core discipline)

- **Skill:** debugging-loop core skill (build-red-first +
  reproduce→minimise→hypothesise→instrument→fix→regress + D2C escape).
- **Pilot repo:** TrueAI (retrospective pilot).
- **S2 verdict:** SATISFIED for the **core discipline** (not for the D2C escape —
  see Finding 3).
- **What the pilot validated:**
  - **build-red-first.** The "make it red before you fix it" entry condition held
    up as the keystone of the deterministic-red flagship.
    - **Confidence: HIGH** — landed in the held skill's step sequence.
  - **The reproduce→…→regress sequence.** The full agent-iterated loop validated
    end-to-end against the TrueAI repo.
    - **Confidence: HIGH** — landed in the held skill.
  - **step-0 absence-contract (S1 contribution).** The pilot contributed the
    step-0 absence-contract framing (state what is *not* present before
    hypothesizing what is), which landed in the held skill.
    - **Confidence: HIGH** — in the held `SKILL.md`.
  - **step-7 cleanup / 3-line post-mortem.** The pilot contributed a step-7
    "clean up + write a ≤3-line post-mortem" tail, which landed in the held skill.
    - **Confidence: HIGH** — in the held `SKILL.md`.

## Finding 3 — debugging-loop D2C escape / VH-Solara forward pilot (SATISFIED)

- **Skill:** debugging-loop core skill, **D2C (non-deterministic red) hybrid
  escape** specifically.
- **Pilot repo:** VH-Solara (forward pilot — volunteered).
- **S2 verdict:** **SATISFIED.** This was the open gate for `P2-SKILLS-002`; the
  pilot discharged it. Skill committed `c52268a`; row moved `blocked → done`.
- **Why VH-Solara is the stress-test repo:** VH-Solara runs a 6-lane
  Go + Vitest + Playwright stack with a **GPU/WebRender heat-saga** — a class of
  non-deterministic red signals that **breaks the deterministic-red keystone** of
  the debugging-loop. It is the canonical case the D2C escape was designed for:
  a red signal that is `human-observed | non-deterministic | not agent-runnable`,
  where theorizing into the noise is the failure mode the escape prevents.
- **Pilot outcome (the heat saga):** the D2C escape was validated against
  VH-Solara's Firefox/WebRender GPU/thermal defect (a `mask-image` gradient
  rendering failure under real GPU load). The pre-pilot MEDIUM-confidence-on-
  wording hold **resolved**: the escape wording held up under a real
  non-deterministic saga.
  - **All FIVE guardrails fire correctly.** (1) The `human-observed` label was
    correctly applied — the defect needs a human eye to read the rendered frame,
    so it was NOT promoted to an agent-owned deterministic red/green gate.
    (2) The agent did NOT continue theorizing after the downgrade. (3)
    `diagnostics-export` packaged the handoff bundle (environment, repro attempts,
    the next human-observation request). (4) `bgshell-job` was kept inside the
    deterministic-red attempt boundary (NON-GPU long probes only) — the GPU/
    compositor probing correctly routed to the Downgrade, not to bgshell. (5) The
    agent-owned loop ENDED after the labeled handoff (no "one more hypothesis").
  - **The `human-observed | non-deterministic | not agent-runnable` label is the
    taxonomy the five guardrails operate ON — it is NOT a sixth guardrail.** This
    packet and the decision memo both use **five** as the guardrail count.
  - **Statistical-sample red correctly stayed in-loop (NOT downgraded).**
    VH-Solara's serial ×50 reproduction of a flaky-race signal — the
    `downgrade-protocol.md:46-49` case ("Race that fails 1-in-50 but tightens
    under 50× serial repeat → NOT a downgrade yet") — was correctly driven with
    `bgshell-job` rather than downgraded. The downgrade fired only for the
    genuinely human-observed GPU signal. This is the key negative-space
    validation: the escape does not over-fire on legitimate deterministic-red
    attempts that happen to be slow.
  - **`diagnostics-export` + `bgshell-job` boundary holds.** The two skills occupy
    disjoint regions: bgshell inside the deterministic-red attempt (its output CAN
    become a red signal); diagnostics-export at the downgrade boundary (packaging
    a handoff for a human). No role confusion observed.
  - **Confidence on the design (post-pilot): HIGH.** The escape wording survived a
    real non-deterministic saga; the five guardrails fired; the statistical-sample
    red was not falsely downgraded.
- **One flagged non-blocking tension → `P2-SKILLS-003`.** VH-Solara's
  competent-team-validated serial ×50 red takes 15–27min wall-clock per run. The
  `red-signal-recipes.md` "fast" property (step-1 completion: deterministic +
  fast + agent-runnable) and its "slow" anti-pattern can be misread as flagging
  this legitimate predeclared-aggregate red. Non-blocking: the guardrails in
  `SKILL.md` and `downgrade-protocol.md:46-49` need NO change; the follow-up is a
  wording clarification in `red-signal-recipes.md` only — bless the
  predeclared-aggregate red shape (predeclared threshold + reproducible count +
  no cheaper seam + bgshell-hosted) into the "fast" property + "slow" anti-pattern
  so a validated slow aggregate red is not falsely flagged. Tracked as
  `P2-SKILLS-003` (todo, skills).
- **Evidence tier: retrospective.** The heat saga was already fixed by the time
  the pilot reported; the validation reconstructed the loop against the resolved
  defect rather than running it live. This is adequate for a wording/escape
  validation (the failure mode and the guardrail behavior are observable in
  retrospect), but it is not a live forward-run. Noted for honesty, not as a
  blocker.

## Finding 4 — incidental dogfood findings from the VH-Solara pilot

The D2C validation surfaced two incidental findings that are NOT blockers for
`P2-SKILLS-002` (the skill is promoted) but are captured as follow-ups or
near-miss notes for the craft record.

- **Inherited harness testing boilerplate → `P2-SKILLS-004`.** VH-Solara's D3-
  reconcile step (the tdd-loop authority-honesty rule: localize the consumer's
  testing rules in the same slice) surfaced that the harness `AGENTS.md` testing
  section ships a generic `pytest` / `tests/{unit,integration,e2e}` block that
  consumers inherit verbatim and never reconcile to their real topology.
  VH-Solara is Go + Vitest + Playwright (6 lanes, no pytest, no `tests/unit/`),
  so the inherited block is actively misleading until a human rewrites it. This
  is a harness-side dogfood defect, not a skill defect. Tracked as `P2-SKILLS-004`
  (todo, skills): investigate whether the testing section should be
  tokenized / localize-on-install-nudged or carry an explicit "reconcile to your
  real topology" marker. A draft fix sketch lived at
  `tmp/agent-runs/skill-craft-import/vh-solara-agents-testing-section-draft.md`
  (ephemeral, not committed).
  - **Confidence: HIGH** — the inherited block is verifiable in this repo's own
    `AGENTS.md` ("Testing rules" section).
- **web-unit `node`-default-vs-`jsdom` authority-honesty near-miss (no row).**
  During the same pilot arc, a localization seam-map nearly cited a `jsdom`
  environment for a web-unit test that actually ran under the default `node`
  environment — the kind of aspirational/wrong authority reference D3 forbids.
  Caught before commit; no artifact carried the wrong reference. Recorded here as
  a near-miss confirming the D3 rule is load-bearing, not as a follow-up row.
  - **Confidence: MED** — pilot-reported near-miss; no committed artifact to point
    at (by construction — it was caught pre-commit).

## Contradiction audit

Two contradictions were surfaced during this cycle:

1. **Session memory vs canon — resolved by this slice.** The skill-craft-import
   session memory (`open-questions.md`) listed **both** #3 and #4 as "deferred per
   S2, awaiting pilot." This contradicted canon: #4 (tdd-loop) was already
   committed (`f56b964`) with S2 satisfied. Resolved by reconciling the session
   note to point at the backlog rows as canonical and correcting the #4 line to
   "done @ f56b964." Session memory is disposable; the backlog is canon.
2. **D1A "outlier" vs canonical TDD — resolved SOFT.** The TrueAI pilot framed
   in-loop refactor as a "2-vs-1 outlier" (implying it was the minority view).
   Re-resolved as **alignment with the majority**: canonical Beck TDD literature
   is in-loop, so the pilot agreed with canon rather than overturning it. Resolved
   SOFT because it is a re-interpretation of the pilot's framing, not new
   evidence; the committed `f56b964` reflects the in-loop placement.

## Files referenced (verified paths)

- `templates/core/.opencode/skills/tdd-loop/` — committed skill (in `f56b964`).
  - `SKILL.md` (+84), `references/seam-localization.md` (+81).
- `templates/core/.opencode/skills/debugging-loop/` — committed skill (in
  `c52268a`; previously held untracked pending the VH-Solara pilot on the D2C
  escape — that pilot has now validated the escape, see Finding 3).
  - `SKILL.md` (+110), `references/red-signal-recipes.md` (+66),
    `references/downgrade-protocol.md` (+90).
  - The committed `SKILL.md` carries exactly **five** D2C guardrails (the
    `human-observed | non-deterministic | not agent-runnable` label is the
    taxonomy those guardrails operate on, not a sixth guardrail).
- `templates/core/.opencode/skills/skill-creator/references/skill-lifecycle.md` —
  where the S2 rule shipped (no tracking mechanism; the 2026-07-14 decision memo
  + this packet closed that gap).
- `docs/planning/backlog.md` — canonical status rows `P2-SKILLS-001` (done),
  `P2-SKILLS-002` (done — promoted `c52268a`), plus follow-ups `P2-SKILLS-003`
  and `P2-SKILLS-004` (todo, skills).
- `.opencode/state/sessions/skill-craft-import/memory/open-questions.md` —
  reconciled session note (disposable; corrected to stop contradicting canon).
