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
    reflected in the committed `f56b964` artifact (tdd-loop) and the held
    debugging-loop files.

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

## Finding 3 — debugging-loop D2C escape / VH-Solara forward pilot (PENDING)

- **Skill:** debugging-loop core skill, **D2C (non-deterministic red) hybrid
  escape** specifically.
- **Pilot repo:** VH-Solara (forward pilot — volunteered).
- **S2 verdict:** **PENDING.** This is the open gate for `P2-SKILLS-002`.
- **Why VH-Solara is the stress-test repo:** VH-Solara runs a 6-lane
  Go + Vitest + Playwright stack with a **GPU/WebRender heat-saga** — a class of
  non-deterministic red signals that **breaks the deterministic-red keystone** of
  the debugging-loop. It is the canonical case the D2C escape was designed for:
  a red signal that is `human-observed | non-deterministic | not agent-runnable`,
  where theorizing into the noise is the failure mode the escape prevents.
- **What the pilot must validate:** the D2C downgrade-and-handoff escape — that
  the agent (1) classifies the non-deterministic signal correctly, (2) stops
  theorizing after the downgrade, (3) packages the handoff via
  `diagnostics-export`, and (4) ends the loop. And the five guardrails
  (human-observation-never-promoted-to-deterministic-red;
  do-not-continue-theorizing-after-downgrade; diagnostics-export packages the
  handoff; bgshell-job only for non-GPU long probes; END the loop after handoff).
- **Slot to fill when the VH-Solara report arrives:**

  > **[PENDING — what held vs what broke; whether the escape needs a
  > statistical-sample variant.]** Specifically: did the downgrade classification
  > fire correctly on the GPU/WebRender heat-saga signals? Did the agent hold the
  > "do not continue theorizing" guardrail? Did the handoff bundle carry enough
  > for a human to pick up? And: does the flip condition (a compact
  > agent-runnable statistical protocol with a predeclared aggregate gate) need to
  > be authored now, or can it stay deferred?

- **Confidence on the *design* (pre-pilot): MED.** The escape is reasoned from the
  deterministic-red keystone and the failure mode (theorizing into noise), but the
  wording has not been stress-validated against a real non-deterministic saga.
  This MED confidence is **why the skill holds untracked** rather than
  blanket-committing (see the decision memo's held-vs-committed policy).
- **Next decision (on pilot report):**
  - if the escape **holds** → `commit-review` + commit `debugging-loop/` to core,
    move `P2-SKILLS-002` to `done`;
  - if it **breaks** → iterate the wording → re-validate against VH-Solara.

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
- `templates/core/.opencode/skills/debugging-loop/` — **held untracked** pending
  the VH-Solara pilot on the D2C escape.
  - `SKILL.md` (+110), `references/red-signal-recipes.md` (+66),
    `references/downgrade-protocol.md` (+90).
- `templates/core/.opencode/skills/skill-creator/references/skill-lifecycle.md` —
  where the S2 rule shipped (no tracking mechanism; this slice closes that gap).
- `docs/planning/backlog.md` — canonical status rows `P2-SKILLS-001` (done),
  `P2-SKILLS-002` (blocked).
- `.opencode/state/sessions/skill-craft-import/memory/open-questions.md` —
  reconciled session note (disposable; corrected to stop contradicting canon).
