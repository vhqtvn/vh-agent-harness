---
description: Manage the local browser viewer service and print the noVNC URL
agent: build
subtask: false
---

Bring up the local-only browser viewer:

- consult `docs/ai/codebase-operational-primitives.md` for canonical paths, helper functions, container names, env conventions, and API response shapes before acting — do not rediscover these from scratch.
- git mutations must flow through the `committer` agent via the gated-commit protocol. Load the `gated-commit` skill for details.

```bash
./dev.sh web-view up
```

Print the current local viewer URL:

```bash
./dev.sh web-view url
```

Shut the viewer down when done:

```bash
./dev.sh web-view down
```

Then report:
- the local noVNC URL
- whether the expose hook is configured
- the smallest next step: open the viewer, stop it, or pair it with a live browser smoke run

For git operations, follow `.opencode/docs/git-execution-routing.md`.
