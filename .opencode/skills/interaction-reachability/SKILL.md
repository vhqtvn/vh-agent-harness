---
name: interaction-reachability
description: "Advisory tracer + reviewer falsifier for the interaction-reachability receipt the behavioral-closure crux gate requires on interaction-touching changes (real user events through the real runtime event model). INFORMS only — never blocks."
compatibility: opencode
---

# Interaction-reachability (overlay-only S2 pilot; advisory)

This skill is the **M-a** advisory surface for the `ff74f36` runtime-blindspot
class — defects that are diff-locally-correct, invisible to static analysis and
to every diff-scoped reviewer, and emerge only when a **real user action** is
dispatched through a **real runtime event model**. Canonical case: a cross-origin
iframe swallows host focus/click events; a direct API test (`handler.setActive()`)
succeeds; the real user event never reaches the handler; the change lands broken.

The blocking authority for this class does NOT live here. It lives in the
**`behavioral-closure` crux gate** (core `AGENTS.md` → "Behavioral closure" →
"Interaction-reachability receipt"), which requires an
**interaction-reachability receipt** on interaction-touching changes. This skill
does two ADVISORY jobs around that receipt:

1. **Tracer (builder-side):** help the builder construct an *honest* receipt that
   observes the OUTCOME in the real environment, not the MECHANISM.
2. **Falsifier (reviewer-side):** inspect a filed receipt for shallow /
   inconsistent / non-falsifiable claims and surface a DEFER. Advisory only.

**S2 overlay pilot.** This is an experimental overlay skill. Core promotion is
NOT authorized. It carries browser/runtime tracing procedure that is
adopter-domain; the generic interaction-reachability *label* and the *receipt
requirement* live in core `AGENTS.md` (domain-free), while the *tracing
procedure* lives here. See `references/pilot-runbook.md` for pilot evidence and
the S2-hold promotion path.

## Authority — INFORMS ONLY

> Every output of this skill **INFORMS**. It never certifies reachability, never
> gates a commit/release/doctor/update, never BLOCKs a review, never approves or
> unblocks a closeout, and never transitions state on its own.

Concretely:

- A tracer recommendation NEVER triggers a stop, gate, or transition. Landing
  the change still passes through the `behavioral-closure` crux gate and
  edit-review/ownership.
- A falsifier finding is **ADVISORY ONLY**. It may inform a reviewer who then
  issues a DEFER. A runtime-only concern cannot ground a diff-verifiable BLOCK
  (the Verifiability Clause), so this skill grants no BLOCK authority.
- The crux gate reads the receipt's **presence and internal consistency**. It
  NEVER trusts this skill's self-report of "reachable" or "not reachable": a
  structurally complete receipt is "structural completeness only," NEVER
  "reachability proven" (condition 3).
- Nothing this skill emits weakens the outcome-vs-mechanism rule, the
  `not-demonstrable` fallback, or any existing crux receipt requirement.

**Critic's blocker (load-bearing):** the skill MUST NOT be wired into any
commit, release, doctor, or update path. Promoting any tracer/falsifier output
to a gate is a SEPARATE decision that must identify the protected transition,
the deterministic predicate, ownership of the validator, false-positive and
recovery behavior, and explicit authority to gate. **This skill grants no such
authority.** The M-x crux extension in core `AGENTS.md` is the gate; this skill
is the advisory detection/falsification surface around it.

## When to use

- A change touches a **user-interaction path**: focus, click, keypress, pointer,
  drag, or any gesture dispatched through the real event model — especially when
  a host page embeds frames/iframes, web components, or shadow DOM where event
  retargeting or cross-origin boundaries can swallow events.
- The `review_mode` for the slice is `frontend-ui`, or the task contract names
  this skill for an interaction-touching change.
- Building or auditing the `behavioral-closure` interaction-reachability receipt
  required by core `AGENTS.md`.

## When NOT to use

- As a **reachability certificate** — a receipt this skill helps shape is a
  declaration, not proof the event reached the handler (condition 3).
- For changes with **no user-interaction path** (pure data logic, build config,
  docs, non-interactive CLI). The interaction-reachability receipt is ADDITIVE
  and applies only to interaction-touching changes; ordinary crux receipts are
  unchanged.
- As a substitute for a **real-runtime fixture**. If no verified seam can observe
  the outcome in the real environment, the honest result is
  `not-demonstrable` → `verdict: inconclusive` → the authoring workflow routes
  to defer rather than `completed` (condition 5; an honesty requirement of
  author + reviewer, NOT a mechanical refusal at the closeout transition).
  This skill does not manufacture a `proven` out of a missing fixture.
- To re-label an API-call-returned / flag-set / code-path-ran assertion as
  reachability. That is MECHANISM evidence → `result: skipped`, never `proven`
  (condition 2 — the `ff74f36` trap).

## The five conditions this skill traces and falsifies

The crux gate (core `AGENTS.md`) defines these authoritatively; this skill does
NOT restate or override them. It exists to make each one auditable:

| # | Condition (authoritative in core AGENTS.md) | This skill's advisory role |
|---|---|---|
| 1 | Receipt fields: real user action · target behavior · actual environment · verifier+outcome · tree binding · observed user-visible outcome | Tracer: walk the builder through filling each field honestly (Procedure A) |
| 2 | Mechanism-asserting receipts → `skipped`, never `proven` | Falsifier: flag any receipt whose "outcome" is indistinguishable from the API-call mechanism |
| 3 | Pass = "structural completeness only", never "reachability proven" | Falsifier: flag any prose/label that overstates a consistent receipt as proof |
| 4 | Advisory falsification surface (this skill), no BLOCK authority | Self-description: this IS condition 4's named surface |
| 5 | No verified seam → `not-demonstrable` → `inconclusive` → authoring workflow routes to defer (not `completed`) | Tracer: when no real-runtime fixture exists, guide to honest `not-demonstrable` instead of a fabricated `proven` |

## Procedure A — Tracer (builder-side, construct an honest receipt)

Run when building the interaction-reachability receipt. Each step ends on a
completion criterion.

### A0. Confirm the change is interaction-touching

- Completion criterion: you can name the **real user gesture** (e.g. "user clicks
  the pane-0 region") that the new behavior depends on, AND the **handler** it
  must reach. If you cannot name a real gesture, the change is not
  interaction-touching — the interaction-reachability receipt does not apply;
  stop (an ordinary crux receipt suffices).

### A1. Identify the real-runtime seam

- Completion criterion: you have named an **actual environment** that dispatches
  the gesture through the real event model (a real browser, a real device — NOT
  jsdom, NOT a headless stand-in that elides cross-origin focus or event
  retargeting). If no such seam is available, go to A5 (honest infeasibility).

### A2. Dispatch the REAL gesture and observe the USER-VISIBLE outcome

- Completion criterion: the verifier command dispatches the **literal gesture
  through the real event model** and you recorded **what a human would see**
  (e.g. "focus moved to pane-0 and the active class applied"), not "the
  function returned".
- **What counts as a REAL gesture (valid):**
  - A physical input on a real device (a real touch/click/keypress).
  - Automation that drives the **real interaction path** — e.g. Playwright
    `page.click(selector)` or `page.keyboard.press(key)`, which dispatch a
    trusted event through the browser's real event model against the live DOM.
    The key property is that the gesture travels the SAME event pipeline a real
    user's input would (focus chain, retargeting, cross-origin dispatch), not
    that a human's hand performed it.
- **What is an INVALID stand-in (mechanism, NOT outcome):**
  - Direct programmatic `page.focus(selector)` — this sets focus via the DOM API
    without dispatching a focus/pointer event through the event model, eliding
    exactly the cross-origin/retargeting behavior this class targets.
  - Any direct handler-API call (e.g. `handler.setActive()`) that bypasses the
    event model entirely.
  - A synthesized UNtrusted event that the runtime treats as synthetic and
    short-circuits.
- This is the load-bearing step. A verifier that calls the handler API directly
  or programmatically sets focus/state is mechanism, not outcome (condition 2)
  → the result is `skipped`.

### A3. Bind to the assessed tree

- Completion criterion: the receipt carries a tree/revision binding (git sha) for
  the exact state the verifier ran against.

### A4. File the receipt

- Completion criterion: the `behavioral-closure` crux carries the
  interaction-reachability receipt — declare `interaction_touching: true`, fill
  all six receipt fields (`interaction_action`, `interaction_target`,
  `interaction_environment`, `interaction_verifier`, `interaction_outcome`,
  `interaction_tree` — condition 1), and classify the evidence via
  `interaction_evidence: outcome | mechanism`. Pair an honest `result`:
  `proven` only if A2 observed a real user-visible outcome
  (`interaction_evidence: outcome`); `skipped` for mechanism-only
  (`interaction_evidence: mechanism`); `not-demonstrable` per A5. State the
  result as "structural completeness only," never "reachability proven"
  (condition 3).

### A5. Honest infeasibility (no real-runtime seam)

- Completion criterion: when A1/A2 cannot be satisfied because no verified seam
  can dispatch a real gesture and observe the outcome, the receipt records
  `result: not-demonstrable` → the crux `verdict: inconclusive`. The honest
  authoring workflow routes this to defer rather than `completed` (condition 5;
  an honesty requirement of author + reviewer, NOT a mechanical refusal at the
  closeout transition). Record the infeasible seam explicitly; never silently
  call it `proven`.

## Procedure B — Falsifier (reviewer-side, challenge a filed receipt)

Run when reviewing a change that filed an interaction-reachability receipt.
Emits one ADVISORY finding per suspicious shape; the reviewer may convert a
finding to a DEFER. NEVER BLOCK.

### B1. Mechanism-as-outcome (the `ff74f36` shape)

- Finding criterion: the `interaction_verifier` calls the handler API directly,
  sets a flag/state, or asserts a code path ran; the `interaction_outcome` is
  indistinguishable from "the call returned." This is mechanism, not outcome →
  the `result` should be `skipped`, not `proven`. Surface as DEFER-candidate.
  (`interaction_evidence: mechanism` paired with `result: proven` is the
  structurally-rejected shape the gate FAILs on.)

### B2. Mocked/elided environment

- Finding criterion: `interaction_environment` cites jsdom, a headless
  stand-in, or any fixture where the cross-origin / event-retargeting / focus behavior is absent
  or stubbed. A real-runtime claim on a mocked runtime is non-falsifiable → the
  `result` should be `not-demonstrable` or `skipped`, not `proven`.

### B3. Overstated label

- Finding criterion: prose anywhere (receipt, closeout, PR) labels a consistent
  receipt as "reachability proven" / "verified the user path" / "E2E confirmed"
  when the evidence is structural only. Surface the condition-3 wording fix.

### B4. Non-falsifiable outcome

- Finding criterion: `interaction_outcome` is a mechanism assertion ("state
  updated", "handler invoked", "no error thrown") rather than something a human
  would see. Same disposition as B1.

### B5. Missing tree binding

- Finding criterion: `interaction_tree` is absent or stale relative to the
  assessed state. A receipt not bound to the assessed tree cannot support
  `proven`.

Every finding is ADVISORY. The reviewer decides whether to DEFER; this skill
does not BLOCK, approve, or unblock.

## Falsifiers (anti-patterns this skill refuses to endorse)

- **"The unit test called the handler and it returned."** Mechanism, not outcome
  → `skipped`. (B1)
- **"jsdom dispatched the event."** A mocked event model elides exactly the
  runtime behavior (cross-origin focus, retargeting) that defines this class →
  `not-demonstrable` or `skipped`, never `proven`. (B2)
- **"The code path ran, so it's reachable."** A code-path-ran assertion is
  mechanism. Reachability requires the REAL gesture to reach the handler and the
  OUTCOME to be observed. (B1/B4)
- **"Structural completeness = proven."** No. The gate verifies presence/
  consistency, not that the event reached the handler. (B3)
- **Waiving the receipt "because the change is small."** The receipt is required
  on interaction-touching changes; size does not waive condition 1.

## Output

A tracer run yields the six-field receipt + an honest `result`. A falsifier run
yields zero or more ADVISORY findings (shape, criterion, suggested DEFER). Both
are candidate-only material; the crux gate and the reviewer decide what takes
effect.
