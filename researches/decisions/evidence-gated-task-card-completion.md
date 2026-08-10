---
type: decision
date: 2026-08-10
scope: coordination/closeout — evidence-gated task-card completion (HARD gate)
status: candidate-recorded (operator cross-check P2)
source-basis: ADV-0 taxonomy, docs/coordination/CLOSEOUT_TEMPLATE.md
---

# Evidence-Gated Task-Card Completion — Decision Memo

## Decision statement

**QUESTION:** Should the harness's task-card completion transition (`/task-closeout`, `saveCoordinationTaskCloseout`) require/consume structured evidence (crux receipts, verification) as a HARD gate, with advisory checks feeding it?

*IMPORTANT FRAMING: The operator's original cross-check called this "advisory disposition." This framing was REFUTED by the advisory solution-brief (`tmp/agent-runs/solution-brief-advisory-tier/brief.md`). A completion gate is a HARD gate. It may CONSUME advisory-produced evidence, but conflating the hard gate with the advisory evidence is a category error.*

This is a recommendation the operator will decide on; the memo body does not speak as live repo policy.

## Decision context

The current task-card closeout flow (governed by `.opencode/scripts/state-lib.js`, the task-card lifecycle, and `docs/coordination/CLOSEOUT_TEMPLATE.md`) relies on `behavioral-closure` tokens as a consistency declaration. It is already a hard gate, but it relies on string-matched crux declarations rather than fully structured, multi-tier evidence.

In parallel, the ADV-0 authority taxonomy establishes formal advisory tiers. Consumers (vh-solara, Consumer B, vh-video-maker) have independently hand-built domain versions of evidence gating: frame-evidence protocols, eval-gate ladders, and bespoke `verify-*` scripts.

## Consumer field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — operator-provided; all three consumers independently invented mechanisms to gate completion on evidence (eval-gate ladder, frame-evidence protocol). 

## This-harness posture (read by this researcher)

The harness currently enforces `behavioral-closure` as its hard-completion rule. Expanding this into a richer, structurally enforced evidence gate that consumes advisory-tier outputs is a candidate feature. However, the system's current complexity is already managed effectively by the `behavioral-closure` discipline.

## Options considered

- **OPT-A — Implement evidence-gated completion as a hard gate now:** Overhaul `saveCoordinationTaskCloseout` to parse, validate, and require explicit advisory-tier evidence receipts before allowing the completion transition.
- **OPT-B — Defer pending a concrete closeout-evidence gap (RECOMMENDED):** Maintain `behavioral-closure` as the core completion gate. Acknowledge the distinction between hard gates and advisory evidence, but defer richer evidence gating.

## Recommendation

**OPT-B.** The harness already has `behavioral-closure` as a hard-completion rule. A richer evidence-gated completion mechanism is a candidate feature (and must NEVER be labeled advisory), but we should defer it pending a concrete closeout-evidence-gap that `behavioral-closure` cannot cover.

## Findings

- **(finding)**: source=tmp/agent-runs/solution-brief-advisory-tier/brief.md, confidence=high, type=fact — The advisory solution brief refuted the "advisory disposition" misnomer; a completion gate is by definition a HARD gate, even if fed by advisory evidence.
- **(finding)**: source=docs/coordination/CLOSEOUT_TEMPLATE.md, confidence=high, type=fact — The harness currently relies on `behavioral-closure` tokens as a structural consistency declaration for closeout.

## Contradictions

- The operator's initial framing ("evidence-gated task-card completion as ADVISORY disposition") directly contradicted the ADV-0 taxonomy. The gate is hard; the evidence may be advisory.

## Risks / open-questions

- As consumers build increasingly complex eval-gate ladders, will `behavioral-closure` become too anemic to ensure execution safety at closeout?