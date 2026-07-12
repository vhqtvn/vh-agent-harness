# O5 Reframe-and-Diverge Evaluation

## Date: 2026-07-13
## Task: eval-debate-001
## Status: partial retention (Layer 1 + Layer 3 promote; Layer 2 gated behind runtime evaluation)

## Executive Summary

O5 is a three-layer "layered bounded hybrid" layered on top of the O1 adaptive sequential
debate. Layer 1 adds an advisory `framing_confidence` signal to think-mode. Layer 2 adds one
evidence-triggered reframe-and-diverge inside debate. Layer 3 adds a reactive, operator-gated
manual step-back backstop. The evaluation method was a packet-level prompt evaluation: four
experimental O5 prompt variants and twelve synthetic researcher packets were constructed
under `tmp/agent-runs/eval-debate-001/`, and O1-vs-O5 behavior was reasoned through for each
case against six promotion gates (G1–G6) and six stop conditions.

The design is **sound**: the Layer 2 trigger is strict (five conjunctive conditions plus a
structured payload), the forbidden-trigger list is explicit, and — crucially — the
boundedness gates (G3) and the baseline-stability gate (G6) are enforceable **by
construction** (countable bounds: ≤5 options, depth 1, ≤2 candidates, one reframe; and a
provably non-additive cheap path because the reframe can only fire on a validated trigger).
The cheap path never grows: on every control case (1, 3) O5 is byte-for-byte identical to O1
in call budget. This is O5's strongest property and it holds by design, not by hope.

The design's **weakness** is that the precision gates (G1 negative-trigger, G2
positive-trigger, G5 material-divergence) all depend on a **model judgment** — reliably
classifying a frame-level vs option-level objection, and reliably judging whether a
`frame_delta` is material vs cosmetic. The O5 prompt specifies these classifications clearly,
but a packet-level prompt evaluation **cannot verify runtime compliance**. That is the single
most important empirical uncertainty, and it is the reason Layer 2 is not promoted outright.

**Decision: partial retention.** Promote Layer 1 (advisory `framing_confidence` — harmless,
additive, cannot cause a reframe) and Layer 3 (manual step-back — reactive, operator-gated,
adds the one capability O1 provably lacks with zero auto-trigger risk). Do **not** promote
Layer 2 (auto reframe-and-diverge) until a runtime evaluation exercises the classifier on
real model calls. The no-regret Phase A clarifications (commit `bf5f306`) stay committed.
This is the conservative, evidence-bound path: ship the provably-safe layers now, gate the
auto-reframe behind the test that can actually resolve its central uncertainty.

## O5 Design Summary

Three layers, summarized from the experimental prompts in `tmp/agent-runs/eval-debate-001/`:

**Layer 1 — Advisory preflight framing signal (think-mode).** think-mode emits
`framing_confidence: high|fluid|unknown` with reason codes (`objective_ambiguous`,
`constraint_conflict`, `stakeholder_frame_unsettled`, `solution_presupposed`). It is
explicitly **advisory and non-authoritative**: `high` continues the existing path;
`fluid`/`unknown` recommends phased work or operator review. It does NOT trigger a reframe.
It is a structured rendering of O1's existing binary 90-minute self-check.

**Layer 2 — One evidence-triggered reframe-and-diverge in debate.** The orchestrator may
authorize ONE alternate-frame divergence only after a critic `frame_level_trigger` satisfies
ALL of: (1) `kind: frame_level`; (2) cites ≥1 real `evidence_id`; (3) names a concrete
`original_frame_element`; (4) explains a specific `conflict`; (5) is NOT resolvable as
ordinary option-level revision. Forbidden as auto-triggers: ties, generic low diversity, low
confidence, ordinary disagreement, unpopular leader, "feels unsatisfying," homogeneous
options (clue only). Bounds: one event, one alternate frame, ≤2 outside-frame candidates,
five active options total, depth 1, and the reframe **consumes** the existing revision
budget (no free extra round). Required proposer payload includes `frame_delta` that must
change ≥1 real dimension. Missing-fact alternates return `need_researcher`.

**Layer 3 — Manual step-back backstop (reactive).** The operator may force a step-back
(subject to the same evidence rules — no fabricated evidence), suppress an auto trigger,
discard alternate-frame candidates, or retain the original frame. Orchestrator-level only;
no `/stepback` command. Manual override does not extend the revision budget.

## Experimental Prompts

All four are delta descriptions over the O1 baseline (`templates/core/.opencode/`), kept
under `tmp/agent-runs/eval-debate-001/` so the source tree is never dirtied:

| File | Delta over | Key additions |
|------|-----------|---------------|
| `debate-o5.md` | `agents/debate.md` | (D1) critic must tag frame-level objections with a structured `frame_level_trigger`; (D2) new "reframe-and-diverge (one bounded event)" section with the 5-condition gate, forbidden list, hard bounds, required payload; (D3) "manual step-back" reactive section; (D4) synth handoff carries `frame_delta` + `trigger_reason` |
| `debate-proposer-o5.md` | `agents/debate-proposer.md` | (D1) new `reframe` mode alongside `proposal`/`revision`; (D2) reframe rules (one alternate frame, ≤2 candidates, `frame_delta` ≥1 dimension, `no_frame_delta` rejection, `need_researcher` on missing facts, consumes revision budget); (D3) reframe return shape |
| `think-mode-o5.md` | `skills/think-mode/SKILL.md` | (D1) replace binary 90-min check with `framing_confidence: high\|fluid\|unknown` + reason codes; (D2) explicit advisory-only contract; (D3) output-shape addition. Decision-tree routing unchanged |
| `solution-brief-o5.md` | `agents/solution-brief.md` | (D1) preserve `framing_confidence` through the chain as non-authoritative context; (D2) manual step-back loop-back (force/suppress/discard), single extra debate pass max, operator-initiated only |

The most consequential design choice is in `debate-o5.md` D1: O1 line 49 says the critic
"may not emit a typed `kind` field," so O5 *requires* an additive structured
`frame_level_trigger` object. This makes the trigger auditable (the orchestrator can reject
any trigger missing one of four fields) but is a behavior change whose runtime reliability is
unverified.

## 12-Case Validation Matrix Results

Domain (shared backbone): choose a persistence mechanism for `.local/coordinator/tasks/`.
Evidence IDs are reused across cases for comparability; each case varies one special
condition. Full packets live in `tmp/agent-runs/eval-debate-001/cases/case-NN.md`.

| # | Case | Input condition | O1 behavior | O5 behavior | Expected O5 | Verdict |
|---|------|-----------------|-------------|-------------|-------------|---------|
| 1 | Stable frame | 4 distinct options, no frame conflict | Clean: minor option-level revise, recommends O1/O2 | Identical path; `framing_confidence=high`; no trigger; no added calls | No reframe; budget unchanged | **PASS** (G1,G6) |
| 2 | Generic low diversity | 4 near-identical options, no evidence conflicts frame | Critic may grumble about homogeneity; option-level only | `framing_confidence=high`; homogeneous=clue only; no valid trigger (E1/E2/E3 don't conflict) → no reframe | No auto reframe | **COND** (G1) — rule forbids; model could rationalize a loose trigger |
| 3 | Tie | O1/O2 score equally | Returns `tie` | Explicitly forbidden trigger; no reframe; returns `tie` | No auto reframe, no added call | **PASS** (G1) |
| 4 | Low confidence | Thin/stale evidence, no specific conflict | Returns `need_evidence`, low confidence | Explicitly forbidden trigger; routes to `need_evidence` | need_evidence, NOT reframe | **PASS** (G1,G4) |
| 5 | Option-level objection | O3 (SQLite/cgo) violates C4 pure-go constraint | Drops O3 via ordinary revision | Evidence *confirms* constraint (E9), doesn't contradict frame → option-level, no trigger | Ordinary revision only | **COND** (G1) — "option violates constraint" vs "constraint is wrong" is distinguishable but judgeable |
| 6 | Positive frame conflict | E11/E12 contradict shared "single-writer" assumption | Surfaces concern but **cannot act** (debate.md L75-77 forbids alternate-frame) → flags or `need_researcher` | Valid trigger; ONE reframe; alternate frame "cross-process-safe storage"; ≤2 candidates; consumes budget | Exactly one reframe event | **COND** (G2,G3,G5) — design correct; critic must emit all 4 trigger fields, orchestrator must validate all 5 conditions |
| 7 | Shared common blocker | All options blocked by same cross-process assumption | Surfaces, cannot act | Valid trigger; ONE alternate frame; ≤2 candidates; ≤5 total; depth 1 | One alternate frame, ≤2 candidates | **COND** (G2,G3,G5) — cleanest positive case; bounds countable |
| 8 | Missing-fact alternate | Alternate frame needs WAL crash-safety facts absent from packet | `need_researcher` (cannot act, cannot speculate) | Trigger fires; proposer returns `need_researcher` naming exact gap; no speculation | need_researcher, no speculative rec | **COND** (G4) — rule explicit; temptation to mark "WAL generally safe" as fact is real |
| 9 | Manual force | Critic missed frame problem; operator cites E11/E12 | No structured step-back; operator must rewrite prompt | Operator forces step-back citing real evidence → bounded reframe | One bounded step-back, evidence rules apply | **PASS** (Layer 3) — clear capability gain, low risk |
| 10 | Manual suppress | Auto trigger fired; operator scopes it out | N/A (never reframes) | Suppresses with reason; original flow continues; suppression recorded | Original flow continues, explicit | **PASS** (Layer 3) — auditable, low risk |
| 11 | Budget pressure | Reframe fires after revision already spent | Unchanged (never reframes) | Reframe consumes budget; alternate frame + ≤2 candidates; NO extra revise round; ≤5 options; depth 1 | Within one revision, ≤5, depth 1 | **COND** (G3) — bounds countable/checkable; "consume" semantics could be misapplied but is verifiable |
| 12 | Cosmetic divergence | Proposer renames same mechanism as "alternate frame" | N/A | Must detect no real `frame_delta` → `no_frame_delta` → route to ordinary revision | Fail the frame_delta requirement | **COND** (G5) — semantic-equivalence detection is the hardest judgment; potential stop-condition trigger |

**Legend**: PASS = design guarantees correct behavior with no residual classification risk.
COND = design is correct but behavior depends on a model judgment that packet-level
evaluation cannot verify.

### Per-case reasoning notes

- **Case 1/3 (controls):** O5 is provably invisible. The reframe can only fire on a
  validated `frame_level_trigger`, and these packets contain no evidence that conflicts with
  any frame element. Call budget is byte-identical to O1. This is O5's strongest result and
  it holds by construction.
- **Case 2 (generic low diversity):** The FORBIDDEN list explicitly bars
  "homogeneous-looking options." A rule-following critic finds no evidence-cited conflict
  (E1/E2/E3 confirm the frame, don't contradict it). The residual risk is a model
  rationalizing a loose trigger from E1 ("no file locking → frame problem"). That
  rationalization would be a misclassification the prompt tries to prevent but cannot
  guarantee.
- **Case 5 (option-level):** The decisive distinction is "evidence *confirms* a constraint
  that one option violates" (option-level) vs "evidence *contradicts* a constraint itself"
  (frame-level). E9 confirms C4 (pure-go); O3 violates it. The O5 trigger requires the
  evidence to *contradict* the frame element. A rule-following model classifies correctly.
- **Case 6/7 (positive):** The trigger is clean by construction (E11/E12 directly
  contradict the shared assumption). The design produces exactly one bounded reframe. The
  uncertainty is whether the critic reliably emits all four trigger fields and the
  orchestrator reliably validates all five conditions — a runtime question.
- **Case 8 (missing fact):** Both O1 and O5 land on `need_researcher`, but O5 is more
  *specific* (names the exact gap: "WAL crash-recovery under host-shell fsync semantics").
  O5's decision advantage over O1 here is modest — precision, not a different outcome.
- **Case 11 (budget):** The bounds are **countable** (≤5 options, ≤2 candidates, depth 1,
  one reframe, revision consumed). Unlike fuzzy classifications, a validator can check these
  mechanically in the proposer's output. This makes G3 the most *enforceable* gate.
- **Case 12 (cosmetic):** The hardest case. Detecting that "coordinated single file" is the
  same mechanism as "single file" requires semantic-equivalence judgment. The prompt lists
  the seven valid `frame_delta` dimensions, which helps, but a model can still produce a
  cosmetic change dressed as a dimension change. This is the most likely stop-condition
  trigger in practice.

## Gate Scores

| Gate | Verdict | Evidence | Uncertainty |
|------|---------|----------|-------------|
| **G1** negative-trigger precision | **conditional pass** | Cases 1,3,4 pass cleanly (forbidden triggers explicitly named: ties, low confidence). Case 5 distinguishable (confirm vs contradict). Case 2 is the risk surface — rule forbids homogeneous-as-trigger, but model could rationalize. | Whether a model reliably treats "homogeneous options" and "option-violates-constraint" as non-triggers. Needs runtime. |
| **G2** positive-trigger correctness | **conditional pass** | Cases 6,7: trigger design (5 conjunctive conditions + structured payload) is correct and would fire on genuine conflicts. Payload is auditable (orchestrator rejects incomplete triggers). | (a) Critic reliably emitting all 4 trigger fields — note O1 L49 says critic "may not emit a typed kind field," so O5 requires a behavior change. (b) Orchestrator reliably validating all 5 conditions. Both runtime questions. |
| **G3** boundedness | **pass (with note)** | Bounds are countable and mechanically checkable in proposer output: ≤5 options, depth 1, ≤2 candidates, one reframe. Case 11 (budget pressure) tests consume-semantics; rule is explicit ("reframe CONSUMES existing revision budget — no free extra round"). | Residual risk: model misreading "consume" as "grants a new revise round." But this is *checkable and correctable* by a validator — the strongest enforceability profile of any gate. |
| **G4** evidence discipline | **conditional pass** | Case 8: rule is explicit (missing facts → `need_researcher` naming gap; unsupported claims stay `assumption`/`prediction`). Inherits Phase A claim_type discipline already in O1. | Temptation to mark "WAL is generally safe" as `fact`. Claim_type tagging already exists; compliance is the question, not design. |
| **G5** material divergence | **conditional pass** | Cases 6,7 produce real dimension changes (causal_assumption, root_mechanism_family). The 7 valid `frame_delta` dimensions are explicit and checkable. | Case 12: detecting *material* vs *cosmetic* change is a semantic-equivalence judgment ("coordinated single file" ≡ "single file"). Structural check ("did a dimension change?") is easier than semantic check ("is the change real?"). Hardest gate to satisfy at runtime. |
| **G6** baseline stability | **pass** | Cases 1,3: O5 is byte-identical to O1 in call budget. Holds **by construction** — the reframe can only fire on a validated trigger, and control packets have no valid trigger. No answer flipping without frame-level evidence. | None at design level. This is O5's provably-safe property. |

## Stop Conditions Check

| Stop condition | Occurred? | Evidence |
|----------------|-----------|----------|
| Frame-level objections can't be distinguished reliably from ordinary criticism | **UNVERIFIED (flag)** | The design provides clear rules (frame = shared assumption contradicted by evidence; option = specific option weakness). Packet-level evaluation confirms the *rules* are correct (cases 5 vs 6). Whether the *model* applies them reliably is a runtime question this evaluation cannot answer. Central risk. |
| Generic low-diversity cases trigger frequently | **UNVERIFIED (flag)** | Case 2 tests this; design forbids homogeneous-as-trigger. Runtime frequency unknown. |
| Proposer generates cosmetic rather than frame-level alternatives | **UNVERIFIED (flag)** | Case 12 tests this; design requires `no_frame_delta` rejection. Semantic-equivalence detection is the hard judgment. |
| Alternate frames routinely rely on uncited facts | **UNVERIFIED (flag)** | Case 8 tests this; design requires `need_researcher`. Compliance unknown at runtime. |
| Feature adds calls to well-framed baseline cases | **NO (design-confirmed)** | Cases 1,3: cheap path is provably non-additive. The reframe fires only on a validated trigger; controls have none. Zero added calls by construction. |
| Answer instability increases without corresponding evidence-cited frame corrections | **NO (design-confirmed)** | Reframe is gated to evidence-cited triggers, so any instability is bounded to evidence-cited frame corrections (cases 6,7). No unbounded instability path. |

Four of six stop conditions are **unverified** — not because the design fails to address
them, but because they are precisely the model-judgment questions a packet-level evaluation
cannot resolve. This is the honest state of the evidence.

## Decision

**PARTIAL RETENTION.**

| Layer | Action | Rationale |
|-------|--------|-----------|
| Phase A clarifications (commit `bf5f306`) | **Keep** (already committed) | No-regret; they only sharpen O1's existing vocabulary. |
| Layer 1 — advisory `framing_confidence` | **PROMOTE** | Purely advisory metadata. Cannot cause a reframe. Cannot add calls. Just labels the existing 90-min check's confidence. Genuinely harmless and slightly improves routing context. |
| Layer 3 — manual step-back | **PROMOTE** | Reactive, operator-gated. Cannot auto-fire (zero spurious-trigger risk). Adds the one capability O1 provably lacks (cases 9,10) — a structured, evidence-bound escape valve. Orchestrator-level only, no new command surface, exactly the conservative form the mission specified. |
| Layer 2 — auto reframe-and-diverge | **DO NOT PROMOTE YET** | Design is sound (G3, G6 pass by construction) but G1/G2/G5 depend on semantic classifications (frame vs option; material vs cosmetic) whose runtime reliability is unverified and unverifiable by packet-level evaluation. Four stop conditions live here. Gate behind a runtime evaluation that exercises the classifier on real model calls. |

**Why not full promotion?** The mission's decision rule: "If O5 doesn't clearly beat O1,
recommend retaining the status quo," and "If the evaluation reveals that frame-level trigger
classification is unreliable, say so." The evaluation reveals that classification
reliability is the unverified core. Recommending full promotion would overclaim what a
packet-level evaluation can establish. The honest position: design correct, runtime unproven.

**Why not full retention (status quo)?** Layers 1 and 3 are provably safe and add real value
(Layer 3 gives operators a capability O1 lacks with zero auto-trigger risk). Withholding them
would be over-cautious. Layer 2 is where all the risk concentrates; gating just that layer is
the proportionate response.

**What would flip Layer 2 to promote:** a runtime evaluation that runs the 12 cases (plus
noise variants) through actual model calls and measures (a) false-trigger rate on cases
1–5,10 (must be ~0), (b) true-trigger rate on cases 6,7 (must be high), (c) `need_researcher`
fidelity on case 8, (d) `no_frame_delta` rejection on case 12, and (e) bound violations on
case 11. The cases in `tmp/agent-runs/eval-debate-001/cases/` are retained precisely so this
follow-up can reuse them.

## Remaining Uncertainty

1. **Classifier reliability (highest priority).** Whether a real model, given the O5 critic
   prompt, reliably (a) emits frame-level triggers with all four fields only when warranted,
   and (b) withholds them on cases 2,3,4,5. This is the gate on which Layer 2 promotion
   turns, and it requires runtime measurement.
2. **Cosmetic-divergence detection.** Whether the proposer reliably rejects case-12-style
   cosmetic `frame_delta`s. Semantic-equivalence judgment is the weakest link.
3. **"Consume" semantics under budget pressure.** Whether the model treats the reframe as
   budget-consuming (no extra revise round) rather than budget-granting. Checkable, but the
   default interpretation is not guaranteed.
4. **Critic behavior change.** O1 explicitly says the critic "may not emit a typed kind
   field"; O5 requires it (additively). Whether critics comply reliably with the new
   structured `frame_level_trigger` shape is itself a runtime question.
5. **Interaction with think-mode signal.** Layer 1 is advisory, but if downstream stages
   start *weighting* `framing_confidence` it could subtly bias routing. The prompt says
   non-authoritative; runtime discipline is unverified.

None of (1)–(5) is resolvable by further prompt rewriting alone; all need real model calls.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| O1 baseline read and understood (debate.md incl. Phase A clarifications) | `read templates/core/.opencode/agents/debate.md` L41-88 — frame/option distinction, within-frame-only expansion, `need_researcher` all present | yes |
| O1 explicitly forbids alternate-frame divergence (the gap O5 fills) | `debate.md` L75-77: "alternate-frame divergence ... would require an explicit `frame_delta` and is NOT authorized by the current debate flow" | yes |
| O1 critic does not emit a typed `kind` field (O5 requires it) | `debate.md` L49: "the critic may not emit a typed `kind` field"; `debate-critic.md` return shape has no `kind` | yes |
| Four experimental O5 prompts written under tmp/ (not source tree) | `ls tmp/agent-runs/eval-debate-001/*.md` → debate-o5, debate-proposer-o5, think-mode-o5, solution-brief-o5 | yes |
| 12 synthetic cases written | `ls tmp/agent-runs/eval-debate-001/cases/` → case-01..case-12 | yes |
| No source-tree files modified by this session | `git status --short` shows only pre-existing dirty files from concurrent sessions (auto-classifier-pilot, diagnostics-export, config-transform); none owned by this session; tmp/ is untracked | yes |
| G6 (baseline stability) holds by construction | O5 reframe fires only on validated `frame_level_trigger`; control cases 1,3 contain no conflicting evidence → no trigger possible → zero added calls | yes |
| Bounds (G3) are countable/enforceable | O5 debate-o5.md D2: "one event, one alternate frame, ≤2 candidates, five active options, depth 1, one revision cycle" — all integer-checkable in proposer output | yes |
| Stop conditions 5,6 design-confirmed not occurred | Cheap-path non-additivity (cond. 5) and evidence-gated instability (cond. 6) both hold by construction in the O5 prompt | yes |
| Stop conditions 1-4 unverified (not confirmable by packet eval) | These are model-judgment questions; packet-level evaluation can confirm rule correctness but not runtime compliance | yes |

## Artifacts retained

The experimental prompts and 12 cases under `tmp/agent-runs/eval-debate-001/` are
**retained** (not deleted) because they are the evidence base for this evaluation and the
reusable input set for the Layer 2 runtime follow-up. They live under `tmp/` (untracked,
never committed) so they do not dirty the source tree. Delete them only after the runtime
evaluation either promotes or retires Layer 2.
