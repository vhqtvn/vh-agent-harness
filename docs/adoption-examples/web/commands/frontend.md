---
description: Load the repo-root context needed for frontend work before touching apps/web
agent: build
subtask: false
---

Prepare for frontend work from the repo root.

- consult `docs/ai/codebase-operational-primitives.md` for canonical paths, helper functions, container names, env conventions, and API response shapes before acting — do not rediscover these from scratch.
- git mutations must flow through the `committer` agent via the gated-commit protocol. Load the `gated-commit` skill for details.

Read these files in order:

- `AGENTS.md`
- `apps/AGENTS.md`
- `apps/web/AGENTS.md`
- `docs/ai/frontend-prompt-guide.md`
- `apps/web/README.md`
- `apps/web/package.json`
- `apps/web/playwright.config.ts`

When relevant, also read:

- `.opencode/skills/web-dev-loop/SKILL.md`
- `.opencode/skills/web-fixtures/SKILL.md`

If local-only frontend references exist under `tmp/local-refs/frontend/`, you may read them for implementation context, but treat them as private working material only:

- do not cite them in backlog rows, checkpoints, or durable docs
- do not treat them as the canonical source of truth
- summarize only the resulting implementation decisions, not the private reference files themselves

If the task is non-trivial, recommend the local workstream:

```text
/workstream-open frontend-foundation
```

Return:

- files loaded
- current frontend stack and test entrypoints
- fixture workflow vs live workflow
- first likely files to touch
- whether local-only frontend references were present

For git operations, follow `.opencode/docs/git-execution-routing.md`.
