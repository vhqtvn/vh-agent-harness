---
description: Repair one incomplete research task card or one degraded task card through the sanctioned repair paths
agent: build
subtask: false
---

Repair one local task card.

Task id:
$ARGUMENTS

Workflow:
- consult `docs/ai/codebase-operational-primitives.md` (when one exists) for canonical paths, helper functions, container names, env conventions, and API response shapes before acting — do not chase or fail when it is absent, and do not rediscover these from scratch when it is present.
- git mutations must flow through the `committer` agent via the gated-commit protocol **where `core/gated-commit` is selected**; on profiles without it, automated committing is unavailable — preserve the work, report the missing route, and request separately-authorized activation or operator handling (`.opencode/docs/git-execution-routing.md` → "Capability condition"). Load the `gated-commit` skill for details.
- call `plan_state` with:
  - `operation: read_coordination_task`
  - `task_id: $ARGUMENTS`
- route on what the read shows:
  - a healthy research card with a missing-but-tolerated contract takes the research-contract path: stop if the task is not `task_type: research` or the contract is already complete (use `/task-update <id>` for broader metadata changes); repair only the missing contract fields (`research_question`, `source_policy`, optional `source_allowlist`, `desired_artifact_type`, `target_artifact_path`); the lifecycle status is preserved
  - a degraded card (`diagnostics.degraded: true`) takes the enum-core-field path: accepted payload fields are the offending core fields (`title`, `task_type`, `coordination_mode`, `primary_lane`, `report_envelope`) plus `status` when its value is draft/ready/staged — restore-only; `files_in_scope`/`success_criteria`/`validation_plan` are never repairable (refused as unsupported fields)
- degraded-card lifecycle-transition guards: a verdict/execution status (working/blocked/reported/completed/cancelled) is never restored to a pre-execution status; a ready-target crossing requires an `f3_design_readiness` envelope bound to the current design digest; a staged-target is errata-class only (`task_id` begins `errata-`) and requires a repo-relative `staged_path`; a staged→draft target is refused directionally — a staged card is release-HOLDING, so demotion to draft is never a repair, while a staged→staged field refresh and the F3-gated staged→ready restore remain sanctioned targets
- for a healthy errata-class card, `restage_coordination_task` (plan-state) is the sanctioned ready→staged route — staging is a release-mechanic handoff, not a review verdict
- call `plan_state` with:
  - `operation: repair_coordination_task`
  - `task_id: $ARGUMENTS`
  - `task_payload`: a JSON object containing only the accepted repair fields for the routed path
- stop if the resulting task still fails its research contract or still loads degraded; report the remaining offenders instead of widening the payload

Return:
- task id and resulting status
- repaired fields
- current next action
- next recommended command

For git operations, follow `.opencode/docs/git-execution-routing.md` — including its "Capability condition" section (on profiles without `core/gated-commit` selected, automated committing is unavailable: preserve the work, report the missing route, and request separately-authorized activation or operator handling).
