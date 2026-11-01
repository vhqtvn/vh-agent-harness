# 2026-11-02 — Post-review decision-quality disposition (resolve-first four-output correction)

- **Status:** DECIDED + authored. Source-coherence correction to the
  resolve-first S1 overlay pilot's disposition contract. The pilot posture is
  unchanged: instruction-only, INFORMS-only, and S2/core promotion is NOT
  authorized (the 2026-08-02 memo's S2 record stands).
- **Supersedes / extends:** successor erratum to BOTH 2026-08-02 memos, each
  superseded on EXACTLY the disposition-contract point:
  - `researches/decisions/2026-08-02-resolve-first-defer-processing.md` — its
    resolved design decision 2 ("exactly three legal outputs") and the
    `executable-now` branch keying of its decision 3. It remains the accurate
    historical record of the original three-output design. Everything else it
    decided (philosophy, back-end composition, whitelist, trigger grammar,
    scoreboard, S2 hold) is extended, not reopened.
  - `researches/decisions/2026-08-02-tiered-defer-promotion-model.md` — its
    §"Source policy" citation of the three-output contract and its §5 STEP R1
    routing (keyed on `executable-now`, ELSE "DO IT or DROP"). Being a DECIDED
    memo carrying an agent-applicable deterministic routing procedure, it
    cannot be left prescribing a disposition that contradicts the corrected
    skill for the same scenario: it is reconciled by a dated, scoped, add-only
    supersession note at its header and affected sections (see "Drift
    reconciliation"). Its tier definitions, Test-B boundary, trigger grammar,
    promotion mechanics, and §6 open decisions are unaffected.
  Composition with
  `researches/decisions/2026-07-30-defer-liveness-release-gate.md` is
  unchanged.
- **Scope:** wording-level correction to the resolve-first overlay skill
  (`templates/overlays/resolve-first-pilot/skills/resolve-first/SKILL.md`) +
  its runbook + rendered twins. Narrow and reversible; nothing enters
  `templates/core/`.

## Context

The 2026-08-02 contract, read literally, had two inversion defects:

1. Its STEP 1 `executable-now` branch keyed ONLY on executable/small — forcing
   implementation of any in-scope executable work regardless of justification.
   The skill's purpose is to prevent unnecessary technical debt from small
   deferred fixes — NOT to require implementation of every technically valid,
   executable suggestion.
2. Its machine-checkable trace rule ("a disposition with no trace is not a
   disposition") made a legitimate no-work decision non-compliant unless an
   artifact (commit, brief, or card) was manufactured to justify it — an
   anti-pattern demanded as "compliance".

A second commit-review round on the corrected skill caught the residual
contract drift this memo closes: the skill's References still cited the
2026-08-02 memo as *the signed-off design spec* while the corrected skill said
four outputs — two then-current design authorities contradicting.

## Honest basis (load-bearing)

This is a **source-coherence correction authorized by the repo owner** after a
bounded comparison study. The study concluded **no measured behavioral
advantage** of the corrected contract over no-change; the correction is
justified by source coherence alone. **No measured behavioral improvement is
claimed anywhere**, and real-world effectiveness is unmeasured. The S2 evidence
classes remain unmet; this memo does not run a pilot and does not score S2.

## Resolved design decision 1 — the disposition contract is FOUR legal outputs

`declined-and-closed` is added as a terminal disposition for
**valid-but-unjustified** optional improvements:

- **Declining is not deferring.** No DEFER card, no future obligation, no debt
  item created merely to remember the candidate. The record is the inline
  disposition (with its reason) in the response/thread where the finding arose.
- It is the symmetric twin of STEP 0's DROP: DROP is for a **refuted premise**;
  decline-and-close is for a **valid premise with unjustified work**. Both are
  terminal.
- The parking ban is INTACT: "resolve later" remains illegal. Decline-and-close
  is categorically not the banned branch — it leaves nothing parked, so it is
  not parking under another name. What stays banned is parking, not closure.
- A valid finding is not automatically an authorized or worthwhile engineering
  action. **Mandatory correctness, security, safety, or acceptance requirements
  are NEVER declinable**, however costly implementation appears.

## Resolved design decision 2 — justification is the branch key

- The STEP 1 resolve branch rekeys from `executable-now` to
  **`justified-and-executable-now`**: the work must be necessary or worthwhile
  AND small/clear. The bias toward resolving justified work while context is
  hot is unchanged (in doubt between (1) and (3), pick (1); between (2) and
  (3), pick (2)).
- **Value/justification sizing is not a PARKING criterion — but it IS a decline
  criterion.** Justified and in scope → do it now; valid but not justified by
  the actual requirement, risk, or expected value → decline-and-close; out of
  scope → DROP. Mandatory requirements are exempt from decline (must-fix,
  never value-priced).
- **No ceremony for routine fixes:** analysis depth is proportional to
  consequence; a small, obviously justified fix needs no value-analysis
  ceremony. No new mandatory step exists for routine fixes.
- **Cost and exploration:** do not require reliable implementation-cost
  estimates before solution exploration — cost often becomes understandable
  only after investigating alternative approaches. For uncertain but
  potentially consequential work, a **bounded investigation** MAY be the
  smallest justified next action, before any implementation commitment.
- **No reflex briefs:** drive-to-verdict remains right when a real decision is
  genuinely unresolved — not as a reflex for unclear implementation cost on a
  possible improvement.
- Substantial engineering remains pursuable when approved requirements
  genuinely justify it; decline is not a softer substitute for justified
  substantial work.

## Resolved design decision 3 — the trace carve-out for no-work decisions

The machine-checkable trace requirement is UNCHANGED for work that lands,
decides, or defers (commit ref / brief artifact / draft card). A legitimate
decline-and-close is the exception by construction: its record IS the inline
disposition where the finding arose. Creating a commit, brief, or task card
solely to justify a no-work decision is an anti-pattern, not compliance. The
trace requirement exists to stop silent parking; a decline parks nothing.

## Resolved design decision 4 — two new falsifiers (both misuse directions)

- **Decline-as-dismissal** — using decline to wave off mandatory
  correctness/security/safety/acceptance work as "low-value" or "too costly",
  or re-labeling a must-fix defect as an "optional improvement" to make it
  declinable. BANNED.
- **Ceremony-for-closure** — manufacturing a card/brief/commit solely to
  justify a legitimate decline. BANNED as compliance theater.

The existing five falsifiers stand; "Rationalization engine" explicitly bans
parking, not closure.

## What is preserved (explicit)

- The parking ban ("resolve later" is not a legal output).
- The resolve-hot bias for justified work (no future debt cards for work that
  can reasonably be completed now; no new decision ceremonies for routine
  fixes within approved scope).
- The four-tag valid-defer whitelist, the `path_touched` / `after_tag` trigger
  grammar, DEFER trigger/release behavior, the promotion Definition of Ready,
  and the back end (`check-defer-triggers.mjs` + doctor #12 / release-prep
  liveness) — all byte-unchanged.
- INFORMS-only authority (the classifier never gates, certifies, or
  transitions).

## Drift reconciliation and known accepted drift

How each stale site discovered around this correction was handled:

- **Reconciled by scoped supersession note:**
  `researches/decisions/2026-08-02-tiered-defer-promotion-model.md`. It is a
  DECIDED memo carrying an agent-applicable deterministic routing procedure —
  two active decision artifacts must not prescribe different dispositions for
  the same concrete scenario (caught by the commit gate's anchored review). It
  is reconciled by a dated, add-only erratum at its header and affected
  sections, superseding its disposition-contract content (three-output
  citation + STEP R1 keying) on exactly this point; its original text is
  preserved unchanged.
- **Deliberately unsynced (accepted drift):** the generic coordination-canon
  intake lists still enumerate resolve/decide/defer WITHOUT decline (verified
  at authoring time):

- `AGENTS.md` DEFER-triage prose ("resolve-now vs. drive-to-verdict vs.
  defer-with-trigger");
- `docs/coordination/RECORD_LIFECYCLE.md` ("Resolvable now → resolve it… /
  Decision-derivable now → drive to a verdict… / Real deferred work…");
- `docs/coordination/PROMOTER_RUNBOOK.md` (the same three-way intake list).

The decline disposition is an **overlay-pilot refinement**; the generic canon
predates it and does not contradict it (its lists say what must not be parked,
not that every valid candidate must be implemented). A future authorized
generic-canon sync may enumerate decline there. Recorded here, deliberately
not fixed in this slice — no core expansion.

## Verification

- `vh-agent-harness skill validate` (rendered set + overlay source dir): OK,
  `resolve-first` frontmatter valid.
- `make update` clean; source ↔ rendered byte-identical (`diff` empty) for
  SKILL.md and runbook.
- `go test ./...` green; `gofmt -l .` clean; `go vet ./...` clean.
- `./bin/vh-agent-harness doctor` (freshly rebuilt) HEALTHY: managed-drift
  PASS, skills PASS.
- No Go/test file needed touching (no test asserts the skill's prose).
- No `templates/core/`, `check-defer-triggers.mjs`, doctor, schema, or
  lifecycle-code edits.

## Behavioral closure (this correction's own crux)

```behavioral-closure
verdict: inconclusive
path: templates/overlays/resolve-first-pilot/skills/resolve-first/SKILL.md (four-output disposition contract + decline carve-out)
verifier: skill validate + make update twin parity + doctor skills/managed-drift
command: vh-agent-harness exec ./bin/vh-agent-harness skill validate; vh-agent-harness exec diff <source> <rendered-twin>; vh-agent-harness exec ./bin/vh-agent-harness doctor
result: not-demonstrable
```

Wording/source-coherence change only: the rendered twin was regenerated and
byte-verified, frontmatter validated, and the repo gates are green — but no
behavioral effectiveness of the corrected contract was measured (per the honest
basis above, none is claimed). (`researches/decisions/` is not a doctor scan
surface, so this token is honest disclosure, not a gated artifact.)

## Non-goals (held)

- No back-end, lifecycle, card-schema, state-machine, or gate change; no new
  persistent state, ledger, or reporting obligation represents decline.
- No `templates/core/` edits; no generic-canon sync (see "Known accepted
  drift").
- No pilot run executed; no S2 scoring; core promotion remains unauthorized.
- No bundling of unrelated improvements.
