---
description: Start the frontend app stack for browser-assisted work
agent: build
subtask: false
---

Bring up the frontend app stack for this repo:

- consult `docs/ai/codebase-operational-primitives.md` for canonical paths, helper functions, container names, env conventions, and API response shapes before acting — do not rediscover these from scratch.
- git mutations must flow through the `committer` agent via the gated-commit protocol. Load the `gated-commit` skill for details.

```bash
./dev.sh web-up
```

Then report:
- whether `api` and `web` started successfully
- the user-facing URL
- the smallest next browser command to run

For git operations, follow `.opencode/docs/git-execution-routing.md`.
