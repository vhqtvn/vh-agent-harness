# OpenCode Memory Model

Use this model to keep OpenCode memory layered, retrievable, and small enough to survive compaction without turning baseline context into noise.

## Layers

### 1. Instruction memory

Shared, committed, stable rules that tell the agent **how it should work**.

- Root repo contract: `AGENTS.md`
- Path-scoped repo rules: nested `AGENTS.md`
- Durable workflow docs: `docs/ai/`
- OpenCode config and agent permissions: `opencode.jsonc`

Use this layer for:

- rules that apply across many sessions
- boundaries, invariants, permissions, and document-placement rules
- behavior that should remain stable and predictable

Do not put speculative ideas, temporary discoveries, or one-session progress here.

### 2. Session memory

Alias-scoped, local state for the **current task execution thread**.

- Path: `.opencode/state/sessions/<alias>/memory/`
- Examples: `task-contract.md`, `brief.md`, `resolved-context.md`, `open-questions.md`, `decision-log.md`, checkpoints, handoffs

Use this layer for:

- the active task contract
- current progress and immediate blockers
- resolved path mappings
- checkpoints and handoffs for the current task
- stable closeout requirements such as a user-specified `Return:` checklist or exact final-response schema

Session memory should answer: what is this session doing right now?

### 3. Workstream memory

Local, cross-session state for a **long-lived theme** that should persist across many sessions but is not yet durable repo guidance.

- Path: `.opencode/state/workstreams/<slug>/`
- Typical themes: project restructure, skill creation, migration shape, evaluation strategy, architectural follow-up

Default files:

- `brief.md`
- `next-slice.md`
- `open-questions.md`
- `rejected-options.md`
- `links.md`

Use this layer for:

- stable cross-session context for a theme
- next slices that should survive session turnover
- dead ends worth not rediscovering
- links to checkpoints, plans, or codepaths that matter repeatedly
- incremental notes that should be appended or updated without rewriting the whole workstream

Workstream memory should answer: what are we still figuring out across sessions?

### 4. Local-private memory

Personal or machine-specific notes that should never become shared repo guidance.

- Suggested path: `.local/opencode/<repo>/`
- Coordination overlay example: `.local/coordinator/`

Use this layer for:

- personal reminders
- machine-specific setup notes
- private preferences that should not affect team behavior
- optional coordinator-runtime state such as task envelopes, report queues, or
  dashboards under `.local/coordinator/`

This layer should never auto-load by default.

A tracked `.local/AGENTS.md` may exist as a path-scoped entrypoint for the local
overlay, but it should still be treated as path-relevant guidance rather than
baseline session memory.

If a local coordinator runtime exists, treat it as a transport and synthesis
surface only. Raw local reports and task envelopes do not replace backlog rows,
checkpoints, or durable shared docs.

## Load policy

Load the smallest layer that changes the next decision.

Always eligible:

- root instructions
- path-relevant nested `AGENTS.md`
- current session task contract and session brief

Eligible when a session is bound to a workstream:

- workstream `brief.md`
- workstream `next-slice.md`

Manual or retrieval-only unless clearly needed:

- workstream `open-questions.md`
- workstream `rejected-options.md`
- workstream `links.md`
- old checkpoints and handoffs
- local-private memory
- full skill catalogs or unrelated workflow docs

## Promotion rules

Promote memory only when its scope becomes stable and shared.

- stable shared rule -> `AGENTS.md`, nested `AGENTS.md`, `docs/ai/`, or `opencode.jsonc`
- approved planned work -> `docs/planning/backlog.md`
- milestone, decision, or blocker worth versioning -> `docs/checkpoints/`
- still exploratory or local -> keep it in `.opencode/state/` or `.local/`
- runtime transport artifact -> keep it in `.local/coordinator/` unless a
  synthesizer has promoted the conclusion into canonical docs

Do not promote a note just because it was useful once.

## Parallel-task rules

- Separate independent tasks with separate session aliases.
- Reuse one workstream across many sessions when they contribute to the same long-lived theme.
- Use the same session alias only when collaborators are intentionally working on the same task state.
- Do not use workstreams as a second plan system. Plans stay session-scoped; workstreams stay theme-scoped.
- Treat workstream initialization as non-destructive by default. Use targeted updates or an explicit reset only when replacement is intentional.

## Size rules

- Keep `brief.md` short, preferably about 40-60 lines or less.
- Keep `next-slice.md` to the next 3-5 concrete steps.
- Keep rejected options short and decision-oriented.
- Prefer append/update flows for `open-questions.md`, `rejected-options.md`, and `links.md` instead of reinitializing the whole workstream.
- If a workstream file grows into durable team guidance, promote it out of `.opencode/state/`.

## Anti-spam rule

A memory file should enter context only if it changes the next action or prevents a repeated mistake.

If it is merely useful background, keep it retrievable instead of auto-loaded.
