Durable home: researches/decisions/native-engine-02-solution-brief.md (landed 2026-08-20).

# Solution Brief: vh-agent-harness as a Native Full-Stack Agent
**Date**: 2026-08-19 (v3)
**Status**: PROPOSAL v3 — §D decisions of 2026-08-19 + 2026-08-19b recorded (provider scope, async dispatch, A2A seam); STILL OPEN now only: S2 no-interception rule (default yes) + cutover evidence (default: headless engine completes N real tasks end-to-end driven only via host protocol by an external client).

## Operator Decisions (2026-08-19 and 2026-08-19b)
Recorded verbatim-intent from the operator messages; replaces v1 §D decision points.

**DECIDED (2026-08-19)**
1. **Scope = personal use only** (v1 §D1). Plugins are built by **including code** — "like telegraf": every plugin is a Go package implementing a narrow interface, registered via `init()`/aggregator file, compiled INTO the single binary; config selects/activates. No in-process dynamic plugin platform; MCP/ACP demoted from primary seam to **optional later interop** (D23 stays interop-only).
2. **No first-party UI this horizon** (v1 §D4). Provide **communication interfaces only**, so the operator can implement frontends externally — e.g. **vh-solara as a WebUI plugin**. NOT TUI-first: headless host contract first.
3. **Modularize like deepseek harness** (v1 §D5): dsh-style package-group modularity, realized in Go as **compile-time module composition** (per decision 1), not runtime DI.

**DECIDED (2026-08-19b)**
4. **Provider scope answered** (v1 §D2): first native adapters = **OpenAI-compatible AND Anthropic-compatible** (replaces the "OpenAI-compatible / DeepSeek-first" default). DeepSeek stays reachable via its OpenAI-compatible endpoint — noted, not dropped.
5. **Async dispatch is a first-class feature** — "I want the async dispatch feature of claude" (Claude Code-style). For vh: `prompt/dispatch` returns an **enqueue receipt immediately** (never blocks on completion — dsh SDK pattern: "prompt = enqueue receipt only"); work executes as **durable background jobs** (dsh jobs semantics: `<kind>-N` ids, owner-fenced, first-wins settlement, `reported` flag suppressing duplicate notices); long-running work as **continuable subagent Activations** (running/waiting/settled, manager-owned settlement, provenance-clean reporting); **settlement notifications** arrive on the event stream (session.event class) so any external client (vh-solara) renders progress/completion without polling; optional scheduling dimension (dsh schedule: after/at/every, idle-only dispatch, at-least-once) noted as later. Design refs: dsh session-cognition packet (jobs/schedule/subagent sections) + llm-protocols-tools packet (SDK transport).
6. **A2A answered — adds a seam, splits into two levels** ("Do we have plan for A2A? (Session to session, maybe more generalize later)"; prior briefs had NO A2A plan):
   - **Internal A2A (architect now)**: generalize dsh's subagent inbox (Agent inbox = the only FIFO) + child→parent `reportFrom` into a **session-to-session durable messaging seam** — any session may hold a durable inbox; addressed messages are session-log events (durable, replayable, provenance-tagged); parent-child subagent communication becomes the first specialization of a general any-to-any session bus. No new ledger — messages ARE session events (model-visible ⟺ logged invariant preserved).
   - **External A2A (optional interop later, R5-class)**: an A2A protocol adapter (Google's Agent2Agent web standard is the external name to track; ACP/MCP stay in the same optional-interop bucket) so a vh session/agent interoperates with external agents. NOT committed this horizon — a designed-for seam (the internal bus + host protocol make it additive later); tracked as open follow-up trigger: "external A2A adapter when interop with external agents becomes a real need".

**STILL OPEN** (defaults in parens)
- S2 no-interception rule (default: yes — fail-closed independent slices; ACP side-integration later) — v1 §D3.
- Cutover evidence (default: headless engine completes N real tasks end-to-end driven ONLY via the host protocol by an external client) — v1 §D6.

## Executive Summary
This brief proposes migrating `vh-agent-harness` from an OpenCode-dependent wrapper to a standalone, native **headless** Go agent engine owning the LLM, session, and tool layers, exposed through a **stable communication interface (host protocol)**; frontends live outside the binary as external clients (vh-solara named as the motivating WebUI example). Driven by the debate verdict plus the 2026-08-19 and 2026-08-19b operator decisions, we adopt a capacity-gated composite strategy: harvest immediate runtime-independent wins (O5), climb a progressive strangler ladder (O2), and land at a static Go headless engine + host protocol (O1, re-scoped headless). We explicitly avoid a big-bang rewrite; the current OpenCode lane remains stable for dogfooding until native parity is evidenced. All existing safety invariants (ownership, execution sandboxing, candidate-only outputs) carry over unconditionally.

## Objective & Product-Class Framing
**Objective**: Decide the architectural path for `vh-agent-harness` to become its own full-stack agent product without breaking its self-hosting (dogfood) capability — now scoped **personal-use, headless-first**.
**Product-Class Framing**: We target the class of standalone single-binary agents like Codex (Scout M3 / E13-E16), with one deliberate divergence: **no first-party frontend** — the UX surface is the host protocol. Unlike Node/TS ecosystems (opencode, pi, dsh) that rely on dynamic in-process plugins (Cordis/JS), `vh-agent-harness` uses **telegraf-style compile-time plugin inclusion** (Go packages registered via `init()`/aggregator, selected by config) plus declarative manifests; MCP/ACP are optional interop, not the primary seam. Static binary distribution preserved.

## Recommended Direction
**Capacity-gated composite: O5 harvest → O2 strangler ladder → O1 headless end-state; NO big-bang rewrite.**
We progressively replace OpenCode via a vertical strangler pattern: first harvest runtime-independent design ideas from DeepSeek Harness (dsh); then build headless native Go kernels (adapter, event log, tool sandbox) one at a time, composed telegraf-style into the single binary via dsh-style package groups. The system operates a dual-stack model where the current `.opencode/` lane remains the primary dogfooding environment; native slices are developed and validated in parallel. Only when a native slice demonstrates parity for sacred invariants does it become the default cutover. The end-state is a standalone **headless** Go engine owning provider→tool layers, publishing a **stable host protocol**; frontends (e.g. vh-solara WebUI) live outside the binary as protocol clients. Web/IDE interop stays additive (ACP/MCP, optional).

## End-State Architecture Map

| Layer | Native-Go Ownership | Primary Source | Notes |
|---|---|---|---|
| **LLM / Provider** | Native Go adapter registry — OpenAI-compatible + Anthropic-compatible adapters first (DeepSeek via its OpenAI-compat endpoint) | D4, D5; operator decision 4 (2026-08-19b) | Durable retries; provider scope DECIDED. |
| **Session / Context** | Native Go event log | D1, D2, D3 | Append-only JSONL; derived projections; context relief. |
| **Tool Pipeline** | Native Go waterfall | D7, D8, D9 | Deny-only guards; parallel-safe classification. |
| **Sandbox / Exec** | Existing `exec` family | E3, E10, D11, D12 | Host-local Landlock/seccomp; typed failures. |
| **Frontend** | **None (headless)** — stable host protocol (JSON-RPC/NDJSON over stdio, dsh SDK pattern — see llm-protocols-tools packet; HTTP/WS optional later for browser clients) | Operator decision 2 | Frontends are external clients (vh-solara WebUI as motivating example). |
| **Extension Model** | telegraf-style compile-time inclusion + declarative profile/overlay manifests; MCP/ACP = optional interop only | Operator decision 1; E12 | Plugins are Go packages compiled in; config selects/activates. |
| **Modularity** | dsh-style package groups under `internal/` (`adapters/`, `session/`, `tools/`, `guards/`, `protocol/`…) with narrow Go interfaces; one aggregator assembles the binary | Operator decision 3 | Compile-time composition, not runtime DI. |
| **Async Dispatch / Jobs** | Durable job queue: enqueue-receipt prompts (`prompt/dispatch` never blocks), continuable subagent Activations, settlement events on the stream | dsh D10 + jobs/schedule/SDK patterns; operator decision 5 (2026-08-19b) | Scheduling dimension (after/at/every, idle-only) later. |
| **A2A / Session Bus** | Internal any-to-any durable inbox seam now (addressed messages = session events); external A2A adapter optional later | Operator decision 6 (2026-08-19b); dsh inbox/`reportFrom` | Parent-child is the first specialization; Google Agent2Agent tracked as R5-class interop. |
| **State / Coordination** | Existing file-based | E11 | Task cards, `commit-gate`, local-first. |

## Migration Ladder

| Rung | Ownership Gained | Dogfood Rule | Gate to Proceed |
|---|---|---|---|
| **R0 Harvest** | Runtime-independent dsh imports + host-protocol spec | Current OpenCode lane untouched | Provider scope DECIDED (2026-08-19b) — no open blocker remains. |
| **R1 Kernel Spike** | One provider adapter, event log, bounded tool round-trip; async dispatch minimal: dispatch→receipt→settlement event for ONE bounded job | Go runtime experimental/shadow | Deterministic replay + guarded/logged tool result. |
| **R2 Session Slice** | Event-backed session, compaction/retry, pipeline | Dogfoods a bounded task class | Completes chosen workflow without bypassing gates. |
| **R3 Host Protocol & External Client** | Publish the headless communication contract; drive the engine end-to-end via protocol only (reference client or vh-solara prototype) | Engine steerable without OpenCode | Operator runs a real workflow from an external client with full gate/invariant enforcement visible on the protocol, incl. async observability (external client sees job lifecycle over the protocol). |
| **R4 Cutover** | Native headless engine replaces OpenCode as the agent executor; OpenCode remains optional client/fallback | Default flips to native | Parity evidence for sacred invariants (cutover evidence STILL OPEN). |
| **R5 Expansion** | Web/interop (optional) | Additive to native | Separately approved. |

## Validation Spikes & Kill Criteria

| Spike | Focus | Kill Criterion | Consequence if Killed |
|---|---|---|---|
| **S1 Kernel Replay** | Native kernel event-log replay | Replay non-deterministic / guards not authoritative | Halt native core; stay O5 (harvest only). |
| **S2 OpenCode Compat** | Interception/split-brain probe | Co-ownership split-brain / no interception possible | Continue independent vertical slices; ACP/MCP side-integration only. |
| **S3′ Protocol-Drive Slice** | Can an external minimal client complete a real bounded workflow over the host protocol with events/gates observable? | Protocol can't express event stream + approvals + tool authorization cleanly | Redesign the protocol **before any frontend investment**. |

## Carry-overs, Imports & Non-Imports

### Carry-overs (vh)
| Asset | Source ID | Notes |
|---|---|---|
| Ownership / Render | E2 | Ownership-as-safety renderer; `platform_managed` rules. |
| Corpus & Overlays | E1, E12 | Profile/overlay manifests; domain-free `templates/core/`. |
| Exec Family / Guards | E3, E10 | `exec`, `exec-ro`, `exec-sandbox`, `shell`; shell-guard policy. |
| Coordination Protocol | E11 | Local `.coordinator/` task cards; commit gates. |
| Doctor / Discipline | E2 | Drift/dry-run checks; model-candidate invariant. |

### dsh Imports
| Idea | Source ID | Notes |
|---|---|---|
| Event Log / Sessions | D1, D2, D3 | Append-only; surface-replacement compaction; relief. |
| Adapters / Retries | D4, D5 | Registry; retry-after-relief; durable turns. |
| Tool Waterfall / Guard | D7, D8, D9 | Observers; deny-only guard lattice. |
| Sandbox & Failures | D11, D12 | Typed failure/escalation; pinned argv grammar. |
| Subagents & Catalogs | D10, D18-20 | Activations; dump/boot parity; gen/verify. |
| Interop (demoted) | D23 | Optional later interop only; its NDJSON/ACP wire patterns serve as design references for the host protocol. |

### Non-Imports
| Idea | Source ID | Notes |
|---|---|---|
| Cordis Plugins (runtime DI) | D17 | Not merely avoided — **REPLACED** by telegraf-style compile-time inclusion (operator decision 1). |
| Node/TS Dependency | D24 | Stay static Go binary (`go:embed`). |
| First-party TUI / Web UI | Operator decision 2 | No first-party UI this horizon; communication interface only. |
| Third-party extension ecosystem | Operator decision 1 | No third-party runtime extension requirement; personal use only. |
| Web-first Posture | D15, D16 | Deferred to R5; trust-fence/layering ideas reusable for the optional HTTP/WS face. |

## Risk Register
- **R1**: Capacity-evidence limit → **Mitigation**: Resolved by personal-use scoping (operator decision 1); the 12-18-month capacity gate no longer applies.
- **R2**: OpenCode interception split-brain → **Mitigation**: Probe first (S2); assume independent vertical slices.
- **R3**: Ladder ossification (permanent dual-stack) → **Mitigation**: Explicit cutover gate (R4) to retire fallback.
- **R4**: Extension expressiveness (no dynamic plugins) → **Mitigation**: Resolved by telegraf-style compile-time inclusion (decision 1); MCP/ACP optional interop.
- **R5′**: Protocol stability for external frontends — interface churn breaks vh-solara-class consumers → **Mitigation**: versioned protocol + compat tests.
- **R6**: Import dsh principles not implementations → **Mitigation**: Rewriting D1-D24 in idiomatic Go (interfaces).
- **R7/R8**: Resolved framing disputes → **Mitigation**: Native vertical slices safer than shared ownership; overlays ≠ runtime plugins.
- **R9**: Async durability — crash mid-job must have defined recovery semantics → **Mitigation**: dsh-style durable job state in the session log + recovery pass; semantics fixed in the R0 protocol spec.
- **R10**: Session-bus scope creep — any-to-any generalization invites premature generality → **Mitigation**: implement parent-child first; generalize only when a second topology is real.

## Non-goals
- Big-bang rewrite of `vh-agent-harness`.
- Replacing the coordination runtime model or commit gate scripts.
- Embedding a V8/Node runtime or dynamic WebAssembly plugin platform in the immediate horizon.
- Any first-party TUI or Web UI this horizon (frontends are external protocol clients).
- Building a third-party extension ecosystem (personal-use compile-time plugins only).
- Immediate feature parity with OpenCode's web UI and 25+ tool integrations.

## Immediate Next Slice (If Approved)
- **R0 concrete work**:
  1. Define the **host communication protocol spec** — methods, event stream, approval/authorization model; MUST enumerate the **async dispatch contract** (dispatch → receipt {jobId/activationId} → streamed events → settlement) and **session-bus event types** (design refs: dsh SDK NDJSON + ACP patterns, llm-protocols-tools packet; dsh session-cognition jobs/schedule/subagent sections).
  2. Define the event log (D1) and tool waterfall (D7) Go interfaces (contracts only, no wiring).
  3. Propose the modular package layout (dsh-style groups → `internal/` tree; one aggregator).
  4. A2A internal seam design note: event vocabulary for addressed session messages (durable inbox, provenance tags, parent-child `reportFrom` as first specialization).
  5. Optional parallel: land D18/D19/D20 (dump/boot parity, gen/verify catalogs) into the current `doctor`.

## Verification
| Claim | Verifying command/output | Verified |
|---|---|---|
| Single static Go binary invariant | `README.md` (`go:embed`); Makefile (E1) | yes |
| Current OpenCode delegates LLM & Session | opencode.jsonc model mapping (E6) | yes |
| Go plugins friction (Cordis non-import) | D17 / E18 scout inference | yes |
| Sandbox & exec-verb ownership | E3, E10 (exec, Landlock) | yes |
| Product-class peers own full stack | E13-E16 (pi, opencode, Claude, Codex) | yes |
| Decision 1 — personal use only; telegraf-style compile-time plugins; MCP/ACP demoted to optional interop | operator message 2026-08-19; type: operator directive | yes |
| Decision 2 — no first-party UI this horizon; communication interface only (vh-solara as external WebUI frontend example) | operator message 2026-08-19; type: operator directive | yes |
| Decision 3 — dsh-style package-group modularity realized as compile-time composition in Go | operator message 2026-08-19; type: operator directive | yes |
| Decision 4 (2026-08-19b) — provider scope: OpenAI-compatible + Anthropic-compatible adapters first; DeepSeek via its OpenAI-compat endpoint | operator message 2026-08-19b; type: operator directive | yes |
| Decision 5 (2026-08-19b) — async dispatch first-class: enqueue-receipt prompts, durable jobs, continuable activations, settlement events on the stream | operator message 2026-08-19b; type: operator directive | yes |
| Decision 6 (2026-08-19b) — A2A seam: internal session-to-session durable messaging now; external A2A adapter optional later (trigger-tracked) | operator message 2026-08-19b; type: operator directive | yes |
