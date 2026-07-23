# OpenCode Prompt Guide

This doc describes how to structure effective prompts for this repo so that OpenCode sessions are consistent, path-aware, and aligned with the repo's memory, workstream, and checkpoint model.

## When to read this

Read this when writing prompts for non-trivial tasks that will run through the repo's session workflow. For small tasks (one-command answers, quick lookups), prompt structure does not matter.

## Prompt structure conventions

Think in two layers:

- **Contract layer**: mission, assumptions, scope fences, constraints, likely files, durable/tmp rules, and required outputs. This is the part that should survive compaction through the task contract.
- **Execution layer**: must-read docs, kickoff commands, phase order, checkpoint cadence, and cleanup steps. Use this for substantial tasks so the session starts in the right place and does not drift.

Every non-trivial prompt should include these fields when applicable:

### Required for all non-trivial prompts

| Field | Agent rule & processing instruction | Example |
| --- | --- | --- |
| **Task type** | Use this to select the appropriate workflow (`research`, `implementation`, `evaluation`, `cleanup/finalization`). | `implementation` |
| **Mission** | Treat this as the primary goal of the session. | "Audit prompt guidance and add templates." |
| **Settled assumptions** | Treat these as ground truth. Do not waste compute verifying them unless explicitly asked. If you discover a contradiction between these and the codebase, flag it explicitly before proceeding. | "The current backlog is authoritative." |
| **Closeout expectations** | Treat this as a strict checklist. You are not finished until every item is returned. | `Return:\n1. [INSERT FILES CHANGED]` |

### Required when the task changes files

| Field | Agent rule & processing instruction |
| --- | --- |
| **Exact files likely to change** | List these specific files before editing starts. Confirm or extend this list before making modifications. Do not modify other files without justification. |
| **Durable vs tmp output rules** | Commit only durable docs. Keep all transient/run-specific outputs under `tmp/agent-runs/[INSERT ALIAS]/`. |

### Required for evaluation prompts

| Field | Agent rule & processing instruction |
| --- | --- |
| **Must-read files for the dataset/fixture** | Name the inputs the evaluation depends on so the run is reproducible. |
| **Evidence tier** | Do not make strong claims from thin or smoke-only evidence. State the sample size / coverage the claim rests on. |
| **Per-slice breakdown** | Report results per relevant slice rather than a single aggregate when the aggregate can hide a regression. |
| **Decision scope** | Limit recommendations to the specified scope (promotion, rollback, config swap, or exploratory only). |

### Strongly recommended for substantial tasks

| Field | Agent rule & processing instruction |
| --- | --- |
| **In scope** | Treat this as the allowed change surface. Do not expand the task beyond it without justification. |
| **Out of scope / Non-goals** | Treat these as hard exclusions unless the user explicitly reopens them. |
| **Constraints** | Preserve these throughout execution. Flag conflicts before proceeding instead of silently relaxing them. |
| **Success criteria** | Use these as the pass/fail bar for closeout and checkpoint wording. |
| **Must-read files / Before doing anything else** | Read these first. If checkpoint/workstream/session commands are named, run or open them before broader exploration. |
| **Important framing / Decision rule** | Preserve the wording or acceptance posture here when the task has subtle boundaries. |
| **Execution phases** | For longer tasks, use titled phases with exit conditions so checkpoints and handoffs stay aligned. |

### Strongly recommended for evaluation prompts

| Field | Agent rule & processing instruction |
| --- | --- |
| **Acceptance standard** | Spell out whether the result must characterize, match, exceed, or only triage the baseline. Do not infer a lenient standard. |
| **Primary metrics / gates** | Treat these as the pass/fail metrics. Report secondary metrics separately. |

### Recommended for research prompts

| Field | Agent rule & processing instruction |
| --- | --- |
| **Sources to check** | Inspect these specific files, docs, or codepaths before synthesizing your answer. |
| **Contradiction audit** | Flag any stale guidance, outdated backlog entries, or superseded decisions discovered during research. |
| **Output format** | Match your final output strictly to the requested format (checkpoint, workstream update, or chat summary). |

### Recommended for cleanup/finalization prompts

| Field | Agent rule & processing instruction |
| --- | --- |
| **What to clean** | Safely remove the specified items (e.g., `tmp/` artifacts, stale backlog rows). |
| **What to preserve** | You MUST NOT delete or overwrite the items listed here. |
| **Verification** | Run the specified commands or checks to confirm cleanup is complete. |

### Recommended for prompts that carry load-bearing premises across a boundary

A **load-bearing premise** is a mutable fact the downstream step will act on as
true (e.g. "the profile does not select X", "the gate already passed"). When a
prompt, handoff, or task card carries one across a lossy boundary (handoff →
dispatch → resume → post-compression re-entry), encode it as a re-derivable
4-tuple instead of bare truth, and instruct the receiver to re-derive it before
acting:

| Field | Agent rule & processing instruction |
| --- | --- |
| **Load-bearing premises** | List each as `(value, source, re_derivation_command, observed_at)`. `re_derivation_command` must be a cheap, side-effect-free command that reproduces `value` from current ground truth. The receiving session MUST run it before acting on the premise; on disagreement the premise is stale and is re-adjudicated, not silently re-asserted. |

This is a **discipline, not a gate** — no boundary check can see inside the
receiving session's own (post-compression) context, which is the
structurally-hardest lossy surface. The field reduces stale-premise
re-assertion only insofar as the receiver actually runs the re-derivation.

### Verification fields for checkpoints and handoffs

All non-trivial checkpoints and handoffs MUST include these structured sections:

**Verification table** — every key claim must have a corresponding row:

```markdown
## Verification
| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| (each key claim) | (exact command or output that proves it) | yes/no |
```

**Structured Findings** — help downstream sessions assess evidence quality:

```markdown
## Findings
- **(finding)**: source=..., confidence=high|medium|low, type=fact|assumption|inference
```

**Contradiction Flags** — must be explicit, never silently omitted:

```markdown
## Contradictions
<!-- List any contradictions encountered, or "None detected." -->
```

When to include:
- Every `/checkpoint-save` with substantive claims about what changed or what was verified
- Every `/handoff-save` that passes claims or findings to a downstream specialist
- Omit only when the checkpoint or handoff is purely procedural (e.g., "paused, no findings")

## Recommended kickoff block for long tasks

For longer tasks, prefer an explicit bootstrap block instead of hoping the agent infers the right repo context.

```text
/session-start <alias>
/workstream-start <slug>   # only when the task belongs to a broader theme
/task-contract-save        # refresh if the kickoff contract needs cleaner structure

Before doing anything else:
- read and follow AGENTS.md
- read and follow opencode.jsonc
- use docs/planning/backlog.md
- /checkpoint-open
- /workstream-open         # only when a workstream is already relevant
```

Notes:
- `/session-start` already captures an initial contract; keep `/task-contract-save` when you want to restate or tighten it after kickoff.
- Only include workstream commands when the task is part of a broader long-lived theme.
- Keep the bootstrap list tight. Name only the docs, paths, and commands that materially affect the next decision.

## Iterative alignment interview (grilling)

Before building, alignment is sometimes more valuable than any prompt template. When the request is ambiguous, underspecified, or touches something irreversible, run an iterative alignment interview — a short, one-question-at-a-time exchange whose only purpose is to reach a shared understanding confident enough to write the task contract.

This is a conversation shape, not a batch primitive. `researcher`, `debate`, and `solution-brief` return artifacts; grilling returns a convergent understanding between you and the human.

### When to use

- Before building when alignment is uncertain — the request could be read several plausible ways.
- When the request is ambiguous or underspecified (missing scope, missing acceptance bar, unclear which surface is in play).
- Before enacting anything irreversible (deletions, promotions, migrations, config changes with no clean rollback).

It complements, and does not replace, the task contract (which captures the shared understanding once it exists) and `solution-brief` (which returns a brief from a compare-and-plan pass). Grilling is what you do to make either of those writable with confidence.

### The technique

The discipline has four load-bearing rules:

1. **One question at a time.** Ask a single sharp question; wait for the answer before proceeding. Do not batch a list of questions — a batch forces the human to hold context across answers and hides dependencies between them. Sequential questions let each answer sharpen the next.
2. **Look up facts yourself.** If a question has a factual answer discoverable in the repo, docs, or codebase, find it. Do not ask the human for information you can read. Grilling is for judgment gaps and ambiguity, not for facts you were too lazy to read.
3. **Put decisions to the human WITH a recommended answer.** When the question is a judgment call (not a fact), state your recommendation and a one-line why, then ask the human to confirm or override. Never present a bare open question when you have a defensible recommendation — a recommendation gives the human a fast path and surfaces your reasoning for correction.
4. **Do not enact until shared understanding.** No file edits, no commits, no side effects until the interview converges. The interview's purpose is to reach the point where the task contract can be written with confidence. If you cannot yet write the contract, you are not done interviewing.

### Example

A short, illustrative exchange — one question at a time, each carrying a recommendation:

```text
Agent: The request says "clean up the module." I read the code: the module
has 47 files, 12 of which are unreferenced. Should "clean up" mean deleting
the 12 unreferenced files only, or also refactoring the kept 35? I recommend
delete-only — refactoring is a separate scope and riskier. Confirm or override?

Human: Delete-only, but keep the two files under the api/ surface — they are
used by an external consumer I have not wired yet.

Agent: Noted — that leaves 10 to delete. One more decision: delete in a single
commit, or one commit per file for easier revert? I recommend a single commit
with all 10 — granular reverts are cheap via version control anyway, and it
keeps history clean. Confirm or override?
```

### When NOT to use

- **Routine or obvious work** — send it straight to `build`. Grilling adds latency for no gain when the task is already clear.
- **Pure research / fact-finding** — use `researcher`. Grilling is not the tool for a knowledge gap.
- **High-uncertainty compare-and-plan** — when the real question is "which approach among several," use `solution-brief` (it runs researcher → debate → planner and returns a brief).

Grilling is for the **alignment** gap (do you and the human mean the same thing?), not the **knowledge** gap (do you know a fact?). If you are unsure because you do not know something, go read it; if you are unsure because the request is ambiguous, grill.

### Relationship to existing primitives

Grilling is a conversation shape; `researcher`, `debate`, and `solution-brief` are artifact-returning shapes. They compose rather than compete:

- A grilling session often **concludes by writing a task contract** — the convergent understanding becomes the contract's mission, scope, and constraints.
- It may then **delegate to `solution-brief`** if the now-aligned task still carries genuine approach uncertainty, or **straight to `build`** if alignment resolved the ambiguity.
- Grilling never *replaces* the contract; it produces the clarity the contract needs to be writable.

## Task-type templates

### Research prompt

```text
Task type: research

Mission:
[INSERT ONE OR TWO SENTENCES]

Settled assumptions:
- [INSERT ASSUMPTION 1]
- [INSERT ASSUMPTION 2]

Must read first:
- [INSERT STABLE ENTRYPOINT OR DOC 1]
- [INSERT STABLE ENTRYPOINT OR DOC 2]

Sources to check:
- [INSERT FILE OR PATH 1]
- [INSERT FILE OR PATH 2]

Contradiction audit:
Flag any stale guidance, outdated backlog entries, or superseded decisions
discovered during research.

Output format:
[INSERT CHECKPOINT / WORKSTREAM-UPDATE / CHAT SUMMARY]

Closeout expectations:
Return:
1. [INSERT FINDING 1]
2. [INSERT FINDING 2]
3. recommended next prompt
```

### Implementation prompt

```text
Task type: implementation

Mission:
[INSERT ONE OR TWO SENTENCES]

Settled assumptions:
- [INSERT ASSUMPTION 1]

In scope:
- [INSERT ALLOWED CHANGE 1]
- [INSERT ALLOWED CHANGE 2]

Out of scope / Non-goals:
- [INSERT EXPLICIT EXCLUSION]

Constraints:
- [INSERT CONSTRAINT 1]

Success criteria:
- [INSERT CLOSEOUT BAR 1]

Exact files likely to change:
- [INSERT PATH 1]
- [INSERT PATH 2]

Durable vs tmp:
- [INSERT COMMIT: LIST DURABLE FILES]
- [INSERT TMP: LIST TRANSIENT OUTPUTS UNDER tmp/agent-runs/[ALIAS]/]

Must read first:
- [INSERT DOC OR PATH 1]
- [INSERT DOC OR PATH 2]

Before editing:
1. list settled assumptions you are preserving
2. list contradictions or stale guidance found
3. list exact files you expect to change
4. state what new durable artifacts you plan to add

Execution phases:
## Phase 1 — [INSERT AUDIT OR FIRST IMPLEMENTATION STEP]
After Phase 1:
- [INSERT CHECKPOINT OR DECISION NOTE IF NEEDED]

## Phase 2 — [INSERT FOLLOW-UP STEP]

Closeout expectations:
Return:
1. files changed
2. summary of changes
3. verification commands run
4. recommended next prompt
```

### Evaluation prompt

```text
Task type: evaluation

Mission:
[INSERT ONE OR TWO SENTENCES ABOUT WHAT IS BEING EVALUATED]

Settled assumptions:
- [INSERT BASELINE VERSION]
- [INSERT CONFIG / INPUT VERSION]

Acceptance standard:
[INSERT CHARACTERIZE / MATCH OR BEAT / TOLERANCE-BASED / EXPLORATORY ONLY]

Primary metrics / gates:
- [INSERT PRIMARY METRIC 1]
- [INSERT PRIMARY METRIC 2]

Evidence tier:
[INSERT SMOKE / DECISION-GRADE / HIGH-CONFIDENCE — STATE SAMPLE OR COVERAGE]

Per-slice breakdown:
[INSERT THE SLICES THAT MUST BE REPORTED SEPARATELY]

Decision scope:
[INSERT PROMOTION / ROLLBACK / CONFIG SWAP / EXPLORATORY ONLY]

Exact files likely to change:
- [INSERT CONFIG PATH IF ANY]
- [INSERT CHECKPOINT DOC PATH]

Durable vs tmp:
- commit: [INSERT CHECKPOINT DOC IF DECISION-WORTHY]
- tmp: [INSERT EVAL SCRIPTS AND RAW RESULTS UNDER tmp/agent-runs/[ALIAS]/]

Must read first:
- [INSERT CHECKPOINT / CONFIG / SCRIPT PATH 1]
- [INSERT CHECKPOINT / CONFIG / SCRIPT PATH 2]

Important framing:
- [INSERT ACCEPTANCE OR WORDING GUARDRAIL]

Execution phases:
## Phase 1 — [INSERT GATE / PROTOCOL REVIEW]
## Phase 2 — [INSERT EVAL OR COMPARISON PASS]

Closeout expectations:
Return:
1. eval results summary (per-slice)
2. evidence-tier assessment
3. decision recommendation with justification
4. files changed (if any)
5. recommended next prompt
```

### Cleanup / finalization prompt

```text
Task type: cleanup/finalization

Mission:
[INSERT ONE OR TWO SENTENCES ABOUT WHAT NEEDS CLEANING]

What to clean:
- tmp/ artifacts for session [INSERT ALIAS]
- stale backlog rows
- orphaned docs

What to preserve:
- [INSERT DURABLE CHECKPOINT AT PATH]
- [INSERT WORKSTREAM BRIEF AT PATH]

Constraints:
- [INSERT ANY DO-NOT-TOUCH RULE]

Verification:
- git status shows only intentional changes
- /backlog-cleanup (if backlog changed)
- no stale tmp/ artifacts remain

Closeout expectations:
Return:
1. artifacts cleaned
2. artifacts preserved
3. final git status
4. recommended next prompt
```

## Prompt anti-patterns to avoid

1. **Vague scope** - "Improve the module" without naming which component, which metric, or which decision the work supports.
2. **Missing evidence tier** - Asking for an evaluation without stating whether the result should be smoke-only or decision-grade. This leads to overclaiming from small samples.
3. **No closeout format** - If the prompt does not state what the agent should return, the agent may produce an unfocused summary instead of an actionable deliverable.
4. **Stale path assumptions** - Hardcoding dated checkpoint paths instead of routing through `docs/planning/backlog.md`.
5. **Mixed concerns** - Combining implementation, policy changes, and evaluation claims in one prompt. Split these into separate sessions and hand off between specialists.
6. **Silent contradiction absorption** - When the agent discovers a contradiction between the user's prompt and existing guidance, it should flag it explicitly rather than silently following the newer input or silently preserving the stale doc.
7. **Missing scope fences** - Long tasks without `In scope`, `Out of scope`, or `Constraints` tend to sprawl into adjacent repo work.
8. **No bootstrap block for long tasks** - Omitting the must-read list or checkpoint/workstream reopen steps forces the agent to infer too much repo state.
9. **No acceptance standard** - Evaluation prompts that omit match/beat/tolerance rules invite overclaiming even when the evidence tier is named.

## Integration with session workflow

When running through the full session workflow (`/session-start`), the prompt structure above maps to the task contract fields:

| Prompt field | Task contract section |
| --- | --- |
| Task type | Task type / workflow classification |
| Mission | Mission |
| Settled assumptions | Settled assumptions |
| In scope / Out of scope / Constraints | Exact user requirements / must-not-do constraints |
| Success criteria | Completion checklist |
| Must-read files / Before doing anything else | Must-read files or docs / required commands |
| Exact files likely to change | Files likely to change |
| Durable vs tmp | Required outputs / output placement rules |
| Closeout expectations | Final response format (exact `Return:` schema) plus required outputs for paths/files |
| Acceptance standard / primary metrics / evidence tier / per-slice breakdown / decision scope | Must-do steps / evaluation gates |
| Execution phases | Must-do steps or phases |

`/session-start` captures the initial contract automatically. Use `/task-contract-save` when the prompt is clarified or when you want to restate the contract in a cleaner durable shape after kickoff.
