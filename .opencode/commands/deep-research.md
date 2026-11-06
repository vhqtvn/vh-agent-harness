---
description: Run a bounded deep-research pass on a decision-support question (frame, gather via researcher, audit claims, stop honestly)
agent: build
subtask: false
---

Run a bounded deep-research pass for this question (or resume a prior run):

$ARGUMENTS

**Invocation contract:**

```text
/deep-research <question>; budget=20m; persistence=tmp; promotion=none
/deep-research resume <run-packet-path>
```

- `budget=<time>` — wall-clock guidance for the whole run (default `20m`).
- `persistence=tmp` — run state lives under
  `tmp/agent-runs/<alias>/deep-research/<run-id>/` (disposable; the only
  currently supported value — kept in the contract so callers are explicit).
- `promotion=none|sources|decision` — default `none`. Durable artifacts are
  written to `researches/sources/` or `researches/decisions/` ONLY when the
  caller explicitly sets `promotion=` or asks for them. NEVER auto-promote.
- `resume` — continue a prior run from its run packet instead of starting
  fresh.

**Execution:**

- Load the `deep-research` skill and execute its procedure exactly; the skill
  is the procedure of record (frame → local-first → breadth-first → rank gaps
  → selective depth → claim audit → decide & stop).
- Source gathering goes through the EXISTING `researcher` subagent (you
  orchestrate; you do not become a second researcher). The vhmcp MCP tools
  (`web_search`, `fetch_url`, `read_document`, `web_answer`, `code_docs`) are
  the primary search surface available to it.
- NO task card is required, NO workstream prerequisite, NO automatic backlog
  promotion; the command adds no session-start prerequisite beyond repo
  session rules (AGENTS.md still governs non-trivial work). This command is
  deliberately lightweight.
- Keep every scratch artifact (run packet, excerpts) under
  `tmp/agent-runs/<alias>/deep-research/<run-id>/`; when no session alias is
  bound (e.g. ad-hoc/headless invocation), the packet directory is
  `tmp/agent-runs/deep-research/<run-id>/`.
- Probe and record search-surface availability FIRST: a tool that cannot run
  is a capability failure, never "no evidence found".

**Return:**

- the run-packet path
- the recommendation, what would change it, and remaining uncertainty
- the stop reason (`sufficient_evidence` | `budget_exhausted` |
  `capability_blocked`)
- pilot-limit usage (queries / reads / gap passes / researcher delegations)
- promoted artifact path, if and only if the caller asked for promotion
- the final report must INLINE the locator URL of every evidence record it
  cites — citing an E-ID or the packet alone is not a citation; the report
  carries the URL so the reader is zero hops from the source

**Example invocations (synthetic):**

- `/deep-research Should a small public CLI adopt SQLite or plain JSON files for local state; budget=20m; persistence=tmp; promotion=none`
- `/deep-research Which open-source static site generator best fits a docs-only site with <10 pages; budget=30m; promotion=decision`
- `/deep-research resume tmp/agent-runs/demo-alias/deep-research/20260106-141522-cli-state/run-packet.md`
