# 2026-08-02 — Resolve-first DEFER-processing (overlay S1 pilot)

- **Status:** DECIDED + authored (pilot). Skill shipped at `0f692b4`; NOT run.
  S2/core promotion is NOT authorized — the eight evidence classes are unmet
  until a real pilot runs (see "S2 record").
- **Supersedes / extends:** None. This CREATES the resolve-first capability;
  it does not reopen the defer-liveness gate or the promotion Definition of
  Ready. It composes with (does not replace) the back end established by
  `researches/decisions/2026-07-30-defer-liveness-release-gate.md`.
- **Scope:** the resolve-first overlay pilot skill + runbook. Narrow and
  reversible (S1 overlay; nothing enters `templates/core/`).

## Context

The defer-liveness release gate (doctor #12 / release-prep liveness) is a
BACK-END control: it governs how a parked DEFER card is *promoted and released*,
reading state directly so a card cannot escape a forced verdict at tag time.
But the back end cannot make an agent *create fewer cards in the first place* —
it only acts on cards that already exist. The empirical decay evidence is the
motivation here:

- `researches/decisions/2026-07-30-defer-liveness-release-gate.md` §F1-D1
  records that **16 `draft` DEFER cards had fired `path_touched` targets in
  `v0.18.0..HEAD` and would have shipped without a forced verdict**, and **one
  erratum card's trigger fired on three consecutive releases and stayed `draft`
  through all of them.** Those cards were parked *while their triggers were
  firing* — the trigger machinery itself fails mechanically, not only severity
  verdicts.

**Parking decays.** The back end catches parked cards at release; resolve-first
exists to shrink the pile upstream by refusing to park work that could be landed
or decided in the session that surfaces it.

## Resolved design decision 1 — the philosophy: a DEFER is deferred-AND-amplified cost

A DEFER is **not free parking**. It is deferred-AND-amplified cost: the same
work must still be done later, but "later" re-pays for context that is hot NOW
(the file is open, the failure is reproduced, the reviewer is present, the
decision frame is loaded). Parking compounds cost with a re-derivation tax. So
the **default is RESOLVE NOW**: the pile shrinks by resolution, not grows by
parking. This is grounded in §F1-D1, not aesthetic.

## Resolved design decision 2 — the mechanism: a FRONT gate that composes with (does NOT replace) the back end

The existing back end STAYS unchanged (`check-defer-triggers.js` trigger
grammar, the promotion Definition of Ready, doctor #12 / release-prep
liveness). The skill adds a front gate at card-creation that emits **exactly
three legal outputs**; **"resolve later" is NOT a legal output** (there is no
fourth branch that parks under a vague "later" label):

1. **landed-this-session** — a diff/commit exists; nothing enters `.local/`.
2. **decided-to-verdict** — a `/debate` or `/solution-brief` artifact exists;
   nothing parked.
3. **defer-with-trigger** — one `status:draft` card with a whitelist tag + a
   real trigger.

Authority is **INFORMS-only**: the skill is a candidate classifier, never a
resolution certifier; it NEVER lowers edit-review/ownership gates, and the back
end reads state directly and NEVER trusts the skill's self-report (same
discipline the back end already enforces).

## Resolved design decision 3 — the decision procedure

- **STEP 0 VERIFY PREMISE FIRST** (~1/3 evaporate): re-derive against current
  repo state; already done / obsolete / mis-diagnosed → DROP + rm the card.
- **STEP 1 resolve-first three-way, biased hard toward (1)/(2):**
  - executable-now (small fix, or clear scope+approach) → JUST DO IT this
    session (the edit still passes edit-review/ownership).
  - needs-a-decision / unclear-approach / reframe → drive to verdict NOW via
    `/debate` or `/solution-brief` (the decision is RESOLVED, not deferred).
  - genuinely-blocked (a whitelist reason) → defer as `status:draft` with the
    blocking condition as the trigger.

## Resolved design decision 4 — the valid-defer whitelist (exactly four tags), reconciled to the back-end grammar

The ONLY reasons a defer may be created:

- **blocked-on-absent-evidence** — external evidence does not yet exist.
- **blocked-on-sibling-slice** — depends on an in-flight sibling slice;
  resolving now equals guaranteed rework.
- **pure-future-watch** — a true future-conditional watch with ZERO
  actionable-now component (the no-now-work property MUST be asserted).
- **operator-reserved-signoff** — an operator-only authority call; the enabling
  brief MUST be DONE and attached, only the sign-off is deferred.

**NON-reasons** (must NOT trigger a defer): "marginal/low value"; "some review
cost / safety-contract edit cost" (DO IT, the gate stays); "it's a bit big";
bare capacity/batching (a rider only, never standalone); "unclear approach"
(brief it now).

### Grammar reconciliation (the F1 fix during authoring)

During `/commit-review`, a contract drift was caught and verified: the
whitelist's prose trigger labels (`event`, "sibling lands", bare `after_tag`,
"operator review") are NOT parseable by the back end.
`check-defer-triggers.js` `parsePredicate` accepts **exactly two** predicates,
both requiring an argument: `path_touched(<path>)` and `after_tag(<tag>)`;
anything else is `unknown-predicate` (never fires) in promoter mode and
evaluator-error (fail-closed) in release mode. The reconciliation maps each tag
to a real legal predicate OR honestly declares no mechanical trigger:

- blocked-on-absent-evidence → `after_tag(<specific-tag>)` when tag-correlated;
  else no mechanical trigger (operator-driven).
- blocked-on-sibling-slice → `path_touched(<path-the-sibling-will-produce>)`.
- pure-future-watch → `path_touched(<exact-repo-relative-path>)`.
- operator-reserved-signoff → explicitly NO mechanical trigger (the sign-off is
  an operator action, not path/tag-observable).

This **strengthens** (does not water down) the composition: a skill that taught
non-grammar triggers would violate the very "back end reads state directly"
discipline it claims. Prose-only defers MUST still be logged (draft + trigger) —
the #1 loss failure mode.

## Resolved design decision 5 — the falsifiers (named and forbidden)

- **Rationalization engine** — re-labeling a defer as "resolve later" with no
  this-session landing. "Resolve" means a commit ref THIS session.
- **Busy-work** — rush-resolving a `pure-future-watch` whose trigger hasn't
  fired, or forcing a `blocked-on-sibling-slice` before the sibling.
- **Loophole via operator-reserved-signoff with no attached brief** — invalid.
- **Gate-weakening** — lowering edit-review/ownership "to resolve it." Banned.
- **Re-defer churn** — re-parking the same card across releases (§F1-D1 decay
  in motion); flagged, not accepted as routine.

## The two-sided scoreboard (predeclared; off deterministic observers, never self-report)

Health is read off observers that already exist (`check-defer-triggers.js` for
the card pile and its trigger states; git history for whether resolve-now edits
stuck) — **never off the skill's own emitted outputs.**

- **Card-pile side:** healthy = pile SHRINKS (gate biting); decay = flat/growing
  (rationalization + re-defer churn).
- **Resolve-now side:** healthy = edits STICK (low revert); decay = revert spike
  (busy-work).

Health is BOTH sides. These metrics are advisory — a flat pile does not block a
release (the back end governs release) — but a pilot whose scoreboard never
moves toward health should be reshaped or retired.

## Skill-shape parity (precedent)

This skill mirrors two precedents and does not claim resolve-first as one (it
does not exist prior to this memo):

- **`researches/decisions/2026-07-24-behavioral-closure-pilot.md`** — the slim
  decision-memo shape (Context → resolved design decisions → mechanism →
  verification → behavioral closure → non-goals). This memo follows that shape.
- **`.opencode/skills/contract-invariant-audit/SKILL.md`** — the
  instruction-only / INFORMS-only skill shape: a `references/` runbook
  companion, `compatibility: opencode` frontmatter, an explicit INFORMS-only
  authority section, an S2 record that declines core promotion, and no wiring
  into any commit/release/doctor/update path. The resolve-first skill follows
  this posture and file shape (instruction-only; no deterministic helper ships).

## Verification

- `go test ./...` green; `gofmt -l .` clean; `go vet ./...` clean.
- `vh-agent-harness skill validate .opencode/skills/resolve-first` OK.
- Source (`.vh-agent-harness/overlays/resolve-first-pilot/skills/resolve-first/`)
  ↔ rendered (`.opencode/skills/resolve-first/`) byte-identical after `make update`.
- `./bin/vh-agent-harness doctor` HEALTHY: managed-drift PASS (193 in sync);
  skills PASS (14 valid, including resolve-first); dev-stale-embed PASS.
  (The PATH `vh-agent-harness doctor` reports a pre-existing
  dev-stale-embed/managed-drift artifact because the PATH binary's embedded
  corpus is older than the checkout's `templates/core/` — the documented
  dogfood condition; the rebuilt `bin/` binary is HEALTHY.)
- `/commit-review` round 2 on slice A: verdict APPROVE (unanimous, high
  confidence, low risk); F1 (trigger-grammar drift) cleanly resolved; F2
  (this memo's dangling reference) dropped as advisory — resolved by this memo
  landing.
- No `templates/core/`, `check-defer-triggers.js`, doctor, or Go-source edits.

## Behavioral closure (this pilot's own crux)

The load-bearing path is: *the front gate emits one of the three legal outputs
and the card-pile shrinks (gate biting) while resolve-now edits stick.* Per the
operator's STOP-after-authoring instruction, the pilot was NOT run — no live
defer-processing pass was observed. The verified seam (the pilot itself + the
deterministic observers) cannot observe the load-bearing outcome on an
authoring-only slice. This is the verifier-infeasible case: the crux is
declared `not-demonstrable`, not `proven`. (`researches/decisions/` is not a
doctor scan surface, so this token is honest disclosure, not a gated artifact.)

```behavioral-closure
verdict: inconclusive
path: .opencode/skills/resolve-first/SKILL.md (front gate three legal outputs + card-pile shrink + resolve-now stick)
verifier: a live pilot / the deterministic observers (check-defer-triggers.js card pile; git history revert rate)
command: none — authoring-only slice; no live defer-processing pass run
result: not-demonstrable
```

## S2 record

S2/core promotion is NOT authorized. The eight evidence classes remain unmet
until a real pilot runs and records positive evidence: real repeated trigger,
classification precision, whitelist-tag fidelity, falsifier detection,
resolve-stick rate, pile-shrink rate, false-positive and recovery behavior, and
authority containment. This memo only AUTHORS the pilot; it does NOT run one.

## Non-goals (held)

- The back end (`check-defer-triggers.js`, doctor #12 / release-prep liveness,
  the promotion Definition of Ready) stays UNCHANGED. This pilot composes with
  it; it does not replace, lower, or trust away any back-end control.
- No `templates/core/` edits (domain-free; the embedded corpus is untouched).
- No agent/command/permission/gate surface change (`opencode-append.jsonc` is a
  no-op `{}`).
- No README.agent.md edit (skills-only; the documented surface is unchanged).
- No pilot run executed (authoring only).
