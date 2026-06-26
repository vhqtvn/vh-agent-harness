---
description: Run frontend browser smoke tests, fixture-backed by default
agent: build
subtask: false
---

If the user explicitly asks for the live app stack, run:

- consult `docs/ai/codebase-operational-primitives.md` for canonical paths, helper functions, container names, env conventions, and API response shapes before acting — do not rediscover these from scratch.
- git mutations must flow through the `committer` agent via the gated-commit protocol. Load the `gated-commit` skill for details.

```bash
./dev.sh web-smoke-live
```

Otherwise run the deterministic fixture-backed smoke suite:

```bash
./dev.sh web-smoke
```

Then report:
- which mode you used: `fixture` or `live`
- pass/fail
- artifact path if Playwright produced traces, videos, or an HTML report
- the smallest next fix if the smoke failed

For git operations, follow `.opencode/docs/git-execution-routing.md`.
