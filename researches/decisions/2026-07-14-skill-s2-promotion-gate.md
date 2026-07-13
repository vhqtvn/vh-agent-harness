# Decision: Skill S2 Promotion Gate (Overlay-Pilot-Then-Promote Tracking)

**Date:** 2026-07-14
**Status:** Accepted (tdd-loop promoted; debugging-loop gated on forward pilot).
**Supersedes:** none.
**See also:**
[`../sources/2026-07-14-skill-craft-pilot-evidence.md`](../sources/2026-07-14-skill-craft-pilot-evidence.md)
(evidence trail: the TrueAI retrospective pilots + the VH-Solara forward-pilot slot).
[`../../docs/planning/backlog.md`](../../docs/planning/backlog.md)
(canonical status home: rows `P2-SKILLS-001` done, `P2-SKILLS-002` blocked).

## Problem

The **S2 rule** — *a new core skill MUST pilot in ≥1 overlay against a real
consuming repo before promotion to `templates/core/`*, because core ships into
every consumer's baseline context-load — shipped in Slice 1
(`skill-creator/references/skill-lifecycle.md`). It defined **no tracking
mechanism**: no backlog row, no decision memo, no evidence packet. A skill that
had been designed + piloted but not yet committed lived only in churn-bound
session memory, invisible across context resets.

This is the **same failure shape as a registry-drift gap**: the rule is on the
books but nothing enforces or records that it was satisfied. Two concrete
instances surfaced it this cycle:

- **tdd-loop (#4)** — designed, piloted (TrueAI), HIGH-confidence decisions, and
  already committed (`f56b964`), but with **no canonical record** of *why* it was
  promotable or what the pilot validated.
- **debugging-loop (#3)** — designed, piloted (TrueAI) for the core discipline,
  but with **one MEDIUM-confidence design decision** (the D2C escape) that needs a
  forward pilot before it can blanket-commit into core. Built + held untracked,
  tracked only in session memory that contradicts canon.

Both were held only in
`.opencode/state/sessions/skill-craft-import/memory/open-questions.md`, which
marked *both* as "deferred per S2, awaiting pilot" — a stale record that already
contradicts reality (#4 is committed).

## Options Considered

### Tracking axis

- **Backlog rows + decisions/sources pairing (CHOSEN).** Reuse the harness's
  existing durable surfaces: `docs/planning/backlog.md` as the canonical status
  home (one row per skill promotion), `researches/decisions/` for the
  record-of-decision (the design decisions that gate promotion), and
  `researches/sources/` for the pilot evidence. No new mechanism.
- **A skill registry / lifecycle table (REJECTED).** A new file enumerating
  skills + S2 status. Rejected as **a parallel ledger**: the backlog already is
  the status source of truth, and a second one invites the same drift that
  motivated this slice. The S2 rule is enforced by *using* the existing
  surfaces, not by inventing a new one.
- **Skill-internal provenance only (REJECTED).** Carry the pilot evidence inside
  each skill's own `references/`. Rejected because a skill author can self-attest
  its own promotion — the record must live outside the artifact under review, the
  same way a commit-gate decision lives outside the gate script.

### Commit-policy axis (held vs committed)

- **Held-vs-committed, gated on confidence + S2 (CHOSEN).** A skill whose design
  decisions are all HIGH-confidence AND whose S2 pilot has returned positive
  signal commits to core. A skill with a MEDIUM-confidence-on-wording design
  decision holds untracked pending the specific validating pilot, rather than
  blanket-committing.
- **Always-commit-then-iterate (REJECTED).** Commit the whole skill now and fix
  wording on a later slice. Rejected because **core ships into every consumer's
  baseline context-load**: a half-validated design lands in every consumer's
  `templates/core/` render on their next `update`, with no recall path. The cost
  of one round-trip with the willing pilot is much smaller than the cost of a
  wrong-wording design propagating to every consumer.
- **Never-commit-until-all-HIGH (REJECTED).** Hold tdd-loop too. Rejected because
  the S2 gate *was* satisfied for tdd-loop (TrueAI pilot returned real evidence,
  not a speculative yes) and all its design decisions are HIGH-confidence;
  holding it would be process for process's sake.

## Decision

**(a) Canonical status home.** Each skill promotion is tracked as a backlog row:
`P2-SKILLS-001` (tdd-loop, `done`) and `P2-SKILLS-002` (debugging-loop, `blocked`
on the VH-Solara forward pilot). The backlog is the status source of truth; this
memo records the *decision*; the companion source packet records the *evidence*.

**(b) Held-vs-committed policy.** Commit-to-core iff **all** design decisions are
HIGH-confidence **and** the S2 gate is satisfied (a real overlay pilot returned
real positive signal, not a speculative yes). Otherwise hold untracked pending
the specific validating pilot. This is the operationalization of S2: the gate is
not "did a pilot happen" but "did a pilot return evidence on the specific
uncertain design."

**(c) Promotion conditions — the three design decisions.** A skill is promotable
when each of its open design decisions has reached the confidence bar below. For
this cycle:

- **D1A — refactor placement: in-loop "refactor under green" (HIGH confidence →
  committed in tdd-loop).** Canonical Beck TDD: red → green → **refactor** → next
  slice; the refactor step is *inside* the loop, not deferred to an in-review
  pass. The TrueAI pilot's "2-vs-1 outlier" framing miscounted: canonical TDD
  literature is in-loop, so the pilot aligned with the majority rather than
  overturning it. **This overturns the packet's previously-adopted
  mattpocock in-review stance** — no committed artifact had locked that stance, so
  the overlay is free to override the single Refactor step. The skill's Refactor
  step now runs under green, in-loop.

- **D2C — non-deterministic red signal: hybrid escape (MEDIUM confidence → held,
  gated on VH-Solara forward pilot).** Deterministic red stays the **only**
  agent-iterated flagship (the reproduce→minimise→hypothesise→instrument→fix→regress
  loop runs to a fix). For a **non-deterministic** red signal, the agent takes an
  explicit **downgrade-and-handoff** path rather than theorizing into noise: the
  signal is classified `human-observed | non-deterministic | not agent-runnable`,
  packaged, and handed off. Five guardrails: (1) a human observation is **never**
  silently promoted to a deterministic-red claim; (2) the agent does **not**
  continue theorizing after the downgrade; (3) `diagnostics-export` packages the
  handoff bundle; (4) `bgshell-job` is allowed **only** for non-GPU long probes;
  (5) the agent loop **ends** after the handoff (no "one more hypothesis"). **Flip
  condition:** a compact agent-runnable statistical protocol yielding a
  predeclared aggregate gate would graduate non-deterministic red back into the
  agent-iterated flagship; until such a protocol exists, the escape stands.

- **D3 — localization authority-honesty: YES (HIGH confidence → committed in
  tdd-loop).** A localization file's authority references **must be real and
  verifiable in the consumer's current repo state**, not aspirational. The TrueAI
  pilot caught a seam-map citing a `billing` package that did not exist — an
  aspirational authority reference that would mislead the agent. Additionally, the
  consumer's `AGENTS.md` testing-rules section is localized **in the same slice**
  (two-sources-of-truth drift is the failure VH-Solara flagged): the skill's
  localization seam and the consumer's own testing rules must agree.

## Tradeoffs

- **(+)** Eliminates the registry-drift failure shape: S2 satisfaction is now
  recorded on durable surfaces that survive context resets, not in session memory.
- **(+)** Held-vs-committed stops a half-validated design from landing in every
  consumer's baseline. The cost is borne once (one round-trip with the willing
  pilot), not paid by every consumer on their next `update`.
- **(+)** Reuses existing surfaces (backlog + decisions/sources) — no new
  mechanism to learn or drift.
- **(−)** The D2C escape ships only after the willing pilot (VH-Solara) rather
  than blanket-committing now. This costs a round-trip and leaves
  debugging-loop untracked-but-held in the interim. Accepted: the alternative
  (blanket-commit a MEDIUM-confidence-on-wording design into core) is worse.
- **(−)** There is no automated pilot-completion trigger: the debugging-loop row
  stays `blocked` until an operator-relayed pilot report arrives. Manual, by
  design (a pilot is a real validation, not a CI signal).

## Deferred Work

- **debugging-loop core commit.** Gated on the VH-Solara forward pilot validating
  the D2C escape. On pilot report: if the escape holds → `commit-review` + commit;
  if it breaks → iterate wording → re-validate. Tracked as `P2-SKILLS-002`.
- **S2-tracking automation.** No automated pilot-completion trigger exists today.
  A future slice could add a trigger predicate (e.g. `overlay-pilot-returns-positive-signal`)
  wired into the DEFER/follow-up curation path. Low priority; the manual relay
  works at current scale.
- **`researches/AGENTS.md` dangling-reference cleanup.** Pre-existing dated refs
  to memo filenames that do not (yet) exist remain; out of scope for this slice.
- **Stray scratch in `researches/sources/`.** A scratch artifact from the research
  arc sits alongside the dated packets; clean up in a separate housekeeping slice.

## Evidence

- Companion evidence packet:
  [`../sources/2026-07-14-skill-craft-pilot-evidence.md`](../sources/2026-07-14-skill-craft-pilot-evidence.md)
  — the TrueAI retrospective pilots (tdd-loop S2 SATISFIED; debugging-loop core
  discipline S2 SATISFIED) and the VH-Solara forward-pilot slot (D2C escape,
  PENDING).
- **tdd-loop commit:** `f56b964` (SKILL.md +84, `references/seam-localization.md`
  +81). S2 gate satisfied via the TrueAI overlay pilot (in-loop refactor + seam
  discipline validated).
- **debugging-loop held files:** `templates/core/.opencode/skills/debugging-loop/`
  (SKILL.md +110, `references/red-signal-recipes.md` +66,
  `references/downgrade-protocol.md` +90) — built but held untracked pending the
  VH-Solara pilot on the D2C escape.
- **Pilot provenance:** the pilot contributions came from **two consuming repos**
  (TrueAI, VH-Solara) responding to an adopt-question; both returned **real pilot
  evidence** rather than a speculative yes. This is what discharges S2's
  "real pilot" requirement — a speculative yes does not.
- **Precedent:** the decisions/sources pairing follows
  [`2026-07-05-commit-gate-shared-file-coupling.md`](2026-07-05-commit-gate-shared-file-coupling.md)
  + its source packet, which established the `researches/decisions/` directory and
  the record-of-decision convention.
