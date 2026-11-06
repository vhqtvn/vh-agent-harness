# native-deep-research-pilot

A **project-local overlay pack** (pilot) adding a bounded deep-research
procedure on top of the EXISTING vh-agent-harness runtime: a
`/deep-research` command plus a `deep-research` skill. Build orchestrates;
the existing `researcher` subagent gathers sources; the existing vhmcp MCP
tools are the search surface.

## What it adds

| Capability | Provided here |
|---|---|
| Bounded procedure | frame → local-first reuse → breadth-first (3–5 material questions) → gap ranking by flip potential → one selective depth pass (≤2 gaps) → claim audit → honest stop |
| Claim-level evidence records | every consequential claim classified (`supported`/`partial`/`contradicted`/`unsupported`/`inference`) and bound to an actually-read excerpt |
| Source-level resume | the run packet records completed queries, attempted reads, duplicates, and source-family relationships; a resumed run deduplicates recorded completed work — logical resume, not an exactly-once guarantee |
| Honest stop reasons | `sufficient_evidence` \| `budget_exhausted` \| `capability_blocked` — surface failure is labeled as capability failure, never as "no evidence found" |

## vs. existing surfaces

- **`researcher`** — gathers facts/sources for a bounded prompt; no
  procedure spanning multiple questions, no claim-level audit, no resumable
  packet. This pack keeps researcher as its gatherer and adds the procedure
  around it.
- **`debate`** — adversarial option comparison over facts you already have;
  no retrieval budgeting, no evidence records. Complementary: deep-research
  can feed a debate.
- **`/solution-brief`** — one read-only compare-and-plan pass
  (researcher → debate → planner) producing a brief. Heavier and
  plan-oriented; not resumable at source level; no claim-classification
  vocabulary. Use deep-research when the deliverable is an evidence-grounded
  recommendation with auditable sources, not an execution plan.

## Explicit exclusions (by design)

- no new agent, runtime, database, or vault
- no service, browser fleet, or provider adapter
- no permission changes and no new approval system
- no default critic/adversarial stage (audit is a classification step, not a
  second model lane)
- no mandatory task card, workstream, or session-start prerequisite
- no automatic promotion — durable `researches/` artifacts are written only
  on an explicit `promotion=` (or explicit caller request)

## Surface facts (this installation)

- **Primary search surface: the `vhmcp` MCP tools** (`web_search`,
  `fetch_url`, `read_document`, `web_answer`, `code_docs`), callable from a
  `researcher` subagent.
- **Native opencode `websearch` is NOT registered in this installation**
  (provider-gated at registration; env flags default off). Any other
  installation must probe rather than assume.
- **`webfetch` is permitted for `researcher`** as a fallback for known URLs.

## Local-pack status

This pack lives at `.vh-agent-harness/overlays/native-deep-research-pilot/`
and is **project-local, not shipped**: it is not under `templates/overlays/`,
never travels with the binary, and therefore never appears in the bootstrap
inventory ledger (`internal/cli/bootstrap_inventory_test.go` governs shipped
packs only). Core promotion would be a separate, deliberately-gated decision.

## Rendered surface

Selecting the pack in `vh-harness-profile.yml` and running `make update`
renders (unit files render verbatim under `.opencode/`):

- `commands/deep-research.md` → `.opencode/commands/deep-research.md`
- `skills/deep-research/SKILL.md` → `.opencode/skills/deep-research/SKILL.md`
- `skills/deep-research/references/run-packet.md` →
  `.opencode/skills/deep-research/references/run-packet.md`

(The pack `README.md` is pack documentation and is deliberately NOT rendered.)

Rendered skills/commands are discovered by opencode at process start — a
session opened before `make update` will not see them until restarted.

## Usage (synthetic examples)

```text
/deep-research Should a small public CLI adopt SQLite or plain JSON files for local state; budget=20m; persistence=tmp; promotion=none
/deep-research Which open-source static site generator fits a docs-only site; budget=30m; promotion=decision
/deep-research resume tmp/agent-runs/demo-alias/deep-research/20260106-141522-cli-state/run-packet.md
```

## Known limitations (from pilot validation)

Procedure-text ambiguities documented during the 2026-11-06 validation runs
(durable snapshot: `docs/checkpoints/2026-11-06-deep-research-pilot.md`).
Recorded as **inputs for future skill iteration** — deliberately not fixed in
the pilot text:

1. Do researcher-child subagent queries count against the orchestrator's query budget? (pilot counted them; skill silent)
2. Do FAILED read attempts count toward the read limit? (pilot used a successful-distinct basis)
3. Deliberate phase-boundary stops don't fit the 3-value stop-reason vocabulary (pilot overrode `budget_exhausted` by note)
4. Packet path is schema-mandated yet caller-overridable without documentation
5. "Retry once after a surface change" vs retrying via a different extraction path with no surface change — letter and intent diverge
6. Pilot table "Depth passes: 1" vs step 5 "at most 2 gaps, one pass each" — 1 total or 1 per gap?
7. Does `web_answer` count against the query budget?
8. `pilot_usage` packet field hardcodes /6 /12 /1 /2 while budgets are caller-adjustable
9. Planted/duplicate reads aren't a native packet concept (handled as labeled rows + annex) — worth formalizing for validation runs
