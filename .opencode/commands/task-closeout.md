---
description: Save a durable local closeout report for one coordination task and checkpoint the execution session
agent: build
subtask: false
---

Save a durable local closeout report for one coordination task.

Task id:
$ARGUMENTS

Workflow:
- consult `docs/ai/codebase-operational-primitives.md` (when one exists) for canonical paths, helper functions, container names, env conventions, and API response shapes before acting — do not chase or fail when it is absent, and do not rediscover these from scratch when it is present.
- git mutations must flow through the `committer` agent via the gated-commit protocol **where `core/gated-commit` is selected**; on profiles without it, automated committing is unavailable — preserve the work, report the missing route, and request separately-authorized activation or operator handling (`.opencode/docs/git-execution-routing.md` → "Capability condition"). Load the `gated-commit` skill for details.
- call `plan_state` with `operation: current_session` and stop if no session alias is bound
- call `plan_state` with:
  - `operation: read_coordination_task`
  - `task_id: $ARGUMENTS`
- stop if the task is not currently `working`
- stop if the task is currently `working` but owned by a different `active_session_alias`
- use the task card's `report_envelope` to shape the closeout:
  - `minimal` for short work
  - `standard` for normal execution slices
  - `synthesis` only when the task actually gathered or reconciled multiple reports
- choose the closeout status carefully:
  - `completed` only when the task is truly finished
  - `blocked` when the next action depends on an unresolved blocker
  - otherwise `reported`
- extract the durable closeout from the current conversation:
  - what changed
  - files touched
  - validation run
  - blockers or remaining risks
  - next action
  - whether promotion into backlog/checkpoints is recommended later
  - optional `measured_outcome`: the actual result (vs any `predicted_impact` recorded at ready-time), captured here at closeout-time (skip for routine slices)
  - REQUIRED on every closeout: a fenced `decision-record` block (versioned JSON, exactly one) declaring how decision-class knowledge (owner authorizations, materially-rejected alternatives, non-approve review verdicts, measured outcomes) survives retirement — entries of kind `commit` (content in a reachable commit's message or blob at that committed revision), `artifact` (durable file — docs/checkpoints/, researches/decisions/, researches/sources/, docs/planning/ — with the card-id back-reference in the COMMITTED blob), or `routine` (one-line attestation that nothing decision-class needs preserving; valid only when decision-class fields are empty). The **coverage rule** (2026-10-26): every non-empty decision-class field (owner_notes, measured_outcome, non-empty open_questions, NON-APPROVE last_review/task_reviewed signals — a review that resolved the card to `completed` is a process record) must be covered by an entry declaring `carries: {"<field>": "<verbatim single-line excerpt from that field's content>"}` (excerpt >= 24 chars, not the card id, not the marker, no leading dash); at retirement the excerpt must be present — with the card-id anchor — in the SAME committed carrier. A bare `{kind:"commit"}` or marker-only entry covers nothing. There is deliberately NO `inline` kind — content only in the local closeout report never counts; working-tree content never counts. Retirement resolves every declared pointer against committed, branch-reachable git state (local reachability only). See `docs/coordination/CLOSEOUT_TEMPLATE.md` → "Decision record".
  - when the task touched a load-bearing path (a codepath whose end-to-end execution is the actual proof the behavior works): a `behavioral-closure` declaration — `verdict` (proven | inconclusive | failed | abandoned) and `result` (proven | skipped | not-demonstrable, the crux outcome), plus the crux path/verifier/command. `verdict: proven` is honest only when the crux `result: proven`; otherwise the verdict MUST be inconclusive/failed/abandoned. The declaration is honest and non-droppable; it does NOT prove the path executed (that needs the repo-specific live verification). When the verified seam cannot observe the load-bearing outcome (fixture too small, no prior surface, no real scale, no render), the result MUST be `not-demonstrable` (→ `verdict: inconclusive`); the honest authoring workflow routes such a crux to defer rather than `completed` (an honesty requirement of author + reviewer, NOT a mechanical refusal at the closeout transition, which parses `rewrite-parity` only) — never record the infeasibility as silent prose, and never claim `proven` for an outcome the seam could not observe. Omit the declaration for routine slices that touch no load-bearing path.
  - when the task was driven by a stated motivation (a user-given reason or success motivation): whether that motivation is now satisfied (advisory — distinct from the verdict/crux gate). Record the motivation and the satisfaction verdict in plain prose; this never blends into the behavioral-closure token.
  - when the task was an explicitly-declared deletion or rewrite slice (the task contract carried `mode: deletion_replacement` or `mode: modification_only_rewrite`): a fenced `rewrite-parity` contract in the body — versioned JSON with `prior_surface` (id, revision, paths, inventory_complete) and `behaviors[]` (id, description, prior_evidence, verifier{kind,locator}, result{status,receipt}). At closeout with `task_status: completed`, every behavior must be `proven` with a non-empty `receipt` (structural completeness; the gate verifies presence only — receipt tree-binding honesty is author + reviewer, NOT mechanically SHA-verified, consistent with behavioral-closure); `planned`/`failed`/`skipped`/`not-demonstrable`/missing-receipt refuse completion (`not-demonstrable` routes to defer). The contract is opt-in — omit it for ordinary deletes/refactors/renames. See `docs/coordination/CLOSEOUT_TEMPLATE.md` → "Rewrite-parity contract".
- call `plan_state` with:
  - `operation: save_coordination_task_closeout`
  - `task_id: $ARGUMENTS`
  - `title`
  - `body`
  - `task_status`
  - `report_envelope`
  - `promotion_recommended`
  - `next_action`
  - `measured_outcome` (optional)
- call `plan_state` with:
  - `operation: save_checkpoint`
  - `slug: task-closeout`
  - `title: Task Closeout`
  - `goal`: one-sentence task goal
  - `next_step`: the next action handed back to the coordinator
  - `body`: a compact checkpoint noting the saved closeout, status, validation, and follow-up
- do not edit `docs/planning/backlog.md` or `docs/checkpoints/` automatically from this command

Return:
- task id and updated status
- closeout report path
- whether promotion into durable repo canon is recommended
- checkpoint id and path
- next recommended command

For git operations, follow `.opencode/docs/git-execution-routing.md` — including its "Capability condition" section (on profiles without `core/gated-commit` selected, automated committing is unavailable: preserve the work, report the missing route, and request separately-authorized activation or operator handling).
