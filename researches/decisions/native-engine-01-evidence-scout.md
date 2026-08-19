<!-- Durable home: researches/decisions/native-engine-01-evidence-scout.md (landed 2026-08-20). -->

# 01 — Evidence Scout: Should vh-agent-harness become its own full-stack agent product?

- **Question (debate handoff target)**: "Should vh-agent-harness become its own full-stack agent product — a single Go binary that owns everything from the LLM/provider layer up to the frontend (TUI/Web) — replacing its current OpenCode-based runtime — synthesizing design ideas from vh-agent-harness itself and from DeepSeek Harness (dsh), in the product class of dsh, pi, opencode, Claude Code, and Codex?"
- **Date**: 2026-08-19
- **Method**: three evidence mandates — M1 vh self-map (verified on disk this session), M2 dsh study-corpus mining (committed packets under `researches/sources/deepseek-harness/`), M3 bounded product-class web survey (≤6 lookups, architecture-level facts only).
- **Anti-guess declaration**: every fact row carries a source (repo path or URL). Inferences are labeled `type: inference`; portability judgments are labeled as scout inference. Facts not cheaply verifiable within budget are marked **unverified**, never guessed.

## problem_frame

- **objective**: decide whether vh-agent-harness should own the full agent stack (LLM providers → session/context engine → tool pipeline → frontend) as a standalone Go product in the dsh/pi/opencode/Claude Code/Codex class, vs. remain the harness layer above an external runtime — and if it moves, along which architecture path.
- **constraints (vh sacred invariants that any option must respect or explicitly amend)**:
  - single static Go binary (`go:embed` corpus, no runtime deps) — README.md:34, Makefile
  - `templates/core/` stays domain-free (tokens only) — AGENTS.md mission rules
  - ownership-as-safety-contract (render may only overwrite `platform_managed`; everything else preserved/seeded) — README.md:36-41
  - model-output-is-candidate-never-transition-authority — AGENTS.md core
  - dogfood continuity (this repo develops the harness with the harness)
  - no duplicate parallel ledgers (backlog/task-card transport discipline)
- **success_criteria**: debate receives (a) a verified ownership map of what vh owns vs delegates today, (b) a cited shortlist of dsh design ideas with Go-portability judgments, (c) a product-class comparison table, (d) 3-5 materially distinct options with evidence bindings, assumptions, risks, and a cheapest validation step each.

## criteria (for the debate; importance-tagged)

| # | Criterion | Importance |
|---|---|---|
| C1 | Migration feasibility for a dogfooding repo — can this repo keep developing itself during/after the transition; incremental vs big-bang | critical |
| C2 | Go ecosystem fit — LLM streaming, TUI (tview/bubbletea), plugin/extension mechanisms in Go vs TypeScript/Rust ecosystems | critical |
| C3 | Preservation of existing safety/gate assets — ownership classification, exec verb family + shell-guard, doctor, gated-commit scripts, coordination docs | critical |
| C4 | Extension-model expressiveness — can agents/commands/skills/overlays/deny-rules keep working; dynamic plugins vs declarative config | critical |
| C5 | Operator-team scale reality — maintenance load of owning LLM provider matrix + session engine + frontend vs a 1-few maintainer team | important |
| C6 | Distribution simplicity — single static binary preserved; cross-platform story (landlock vs seatbelt) | important |
| C7 | Interop headroom — MCP/ACP ecosystem compatibility, swappable frontends/engines | nice_to_have |

## evidence_register

(extended per mandate below; E-IDs cited in sections 5-8)

| E-ID | Statement | Source | source_type | quality | recency |
|---|---|---|---|---|---|
| E1 | vh binary is a single static Go binary: installer + manager + executor; corpus embedded via `go:embed` | README.md:34-36, corpus.go:36-37 | repo | high | current (read 2026-08-19) |
| E2 | Render pipeline: staging → ownership classify → per-class plan (fail-closed) → apply → lineage; 5 ownership classes | README.md:36-41 | repo | high | current |
| E3 | Runtime backend abstraction: host-shell / docker_compose / bare / proxy via run-shape.yml; exec family = exec / exec-ro / exec-sandbox / shell with shell-guard gate | README.md:42-44,117-123; internal/runtime/{host_shell,docker_compose,proxy,bare}.go | repo | high | current |
| E4 | This repo's run-shape is `backend: host-shell`, `exec_sandbox.min_mode: strict` | .vh-agent-harness/run-shape.yml:5,15 | repo | high | current |
| E5 | Profile selects `supervised` + overlays auto-classifier-pilot, harness-dogfood + capabilities media-perception, worker-read-only | .vh-agent-harness/vh-harness-profile.yml | repo | high | current |
| E6 | OpenCode provides the agent runtime: agent definitions with modes (all/subagent), per-agent permission maps (bash/edit/task/webfetch/skill), model-per-agent via `{file:...}` pointers, prompt files under .opencode/agents/ | opencode.jsonc:3-88 | repo | high | current |
| E7 | No `mcp` section in this repo's opencode.jsonc (searched; 0 hits) — MCP is an OpenCode capability but not configured in this repo's rendered config | opencode.jsonc (rg search) | repo | high | current |
| E8 | templates/core corpus structure: `.opencode/` (agents, commands, config, docs, package.json, plugins, repo-configs, scripts, skills, sys-scripts, tools), docs/, CLAUDE/README/Makefile templates, tmp/ | templates/core/ dir listing; corpus.go | repo | high | current |
| E9 | internal/ inventory (28 pkgs): cli, substrate, ownership, schema, lineage, runshape, runtime, hooks, overlay, proposals, drift, permission, permconfig, execro, execsandbox, memory, taskcard, redlines, originhash, manifest, managedfile, renderstate, resolver, scopecoverage, complexity, copieranswers, jsonc | internal/ dir listing; README.md:202-203 | repo | high | current |
| E10 | exec-sandbox is host-local Landlock + seccomp, never reaches backend; exec-ro is host-side read-only classifier, prompt-free | README.md:117; README.agent.md exec-family section | repo | high | current |
| E11 | Coordination runtime model: three planes (repo control / session-workstream / runtime messaging), A2A-lite task envelopes, `.local/coordinator/` layout — deliberately local-first, protocol-shaped for later external transport | docs/coordination/RUNTIME_MODEL.md:1-68 | repo | high | current |
| E12 | Overlay packs: project `.vh-agent-harness/overlays/<pack>/` + embedded shipped packs (release, auto-classifier-pilot, repo-mail); opencode-append.jsonc deep-merged; packs render conditionally | corpus.go:39-60; README.md:151-157 | repo | high | current |

## 5. Ownership map (M1) — what the vh binary owns TODAY vs delegates

### Layer table

| Layer | Owner today | Evidence / notes |
|---|---|---|
| LLM / provider layer | **Delegated to OpenCode** | Per-agent `model` fields in opencode.jsonc point at `{file:./.local/config/agent-model/<agent>}` files; OpenCode's provider stack makes the actual API calls. vh has zero LLM code (no internal/ package touches providers). [E6, E9] |
| Session / context engine | **Delegated to OpenCode** (hybrid conventions) | OpenCode owns sessions, compaction, context window management. vh owns the *conventions*: session-state docs (templates/core/.opencode/README-session-state.md), plan-state tooling, checkpoint/command surfaces — policy, not engine. [E6, E8] |
| Tool pipeline + permissions | **Hybrid** | OpenCode provides the tools (edit/bash/task/webfetch/skill) and the permission-matching engine; vh owns the *policy content* (per-agent bash allowlists rendered into opencode.jsonc), the shell-guard plugin (JS under .opencode/plugins/), and deny-rule builders. [E6] |
| Sandbox / exec | **Owned by vh (host-local)** | exec/exec-ro/exec-sandbox/shell verb family; exec-sandbox = host-local Landlock + seccomp trampoline that never reaches the backend; exec-ro = host-side read-only classifier; runtime backends host-shell/docker_compose/bare/proxy. [E3, E4, E10] |
| Coordination / state | **Owned by vh (file-based)** | Task cards (.local/coordinator/tasks), plan-state session memory, commit-gate.sh + cascade review scripts, A2A-lite runtime envelope model — all local-first files + scripts, deliberately not a service. [E11] |
| Frontend (TUI/Web) | **Fully delegated to OpenCode TUI** | No frontend code anywhere in cmd/ or internal/; vh's UX is CLI output + rendered docs. [E9] |
| Extension mechanism | **Hybrid** | vh owns overlays/profile/capability selection + corpus render (what exists); OpenCode provides the loading machinery for agents/commands/skills/plugins/modes (how it runs). [E5, E12] |
| Docs / corpus | **Owned by vh entirely** | Embedded templates/core + migrations + docs/examples/sys-prompts served by the binary; go:embed; render seam with ownership classes. [E1, E2, E8] |

### Carry-over assets (runtime-independent — survive any standalone future)

Scout inference (type: inference), grounded in the table above: these assets are file/render/policy-layer, not LLM/runtime-layer, so they port to ANY runtime choice:

1. Ownership classification + substrate render seam (5 classes, fail-closed plans) [E2]
2. Corpus/overlay/profile/capability system incl. conditional overlay packs [E12]
3. `doctor` health-check frame + drift checks (`diff`) [E2, README.md:116]
4. Gated-commit scripts + tiered cascade reviewers (`.opencode/scripts/`, JS but runtime-agnostic) [E11]
5. Exec verb family contracts (exec/exec-ro/exec-sandbox/shell) + shell-guard policy [E3, E10]
6. Coordination docs corpus (RUNTIME_MODEL, TASK_MODES, RECORD_LIFECYCLE...) [E11]
7. AGENTS.md compose discipline (core+mission), lineage/originhash three-way update [E2]
8. Migration-notes/help-surface embedding (binary-served docs) [E1]

## 6. dsh idea shortlist (M2) — most material for a standalone full-stack Go agent

All rows cited to `researches/sources/deepseek-harness/<packet>.md` (frozen 2026-08-17 study of dsh @ 47f9438, v0.1.0-rc.5). "Go-portability" column is scout inference (type: inference).

| # | Idea (1 line) | Cited to | Go-portability note (inference) |
|---|---|---|---|
| D1 | Append-only session event log + derived projection; `ignorable:true` forward-compat; model-visible ⟺ logged | kernel-architecture.md (transferable #3; C1-i1/C1-i2); README.md cross-cutting themes | **High** — JSONL + seq-indexed fold is idiomatic Go; no dynamic machinery needed; replaces "session as opaque runtime state" with replayable truth |
| D2 | Compaction as surface replacement: `surfaceOp`/`replaceGeneration`, replace-marker + cited sources, retry-after-relief only when relief is committed | session-cognition.md (ideas 1-2) | **High** — pure data-structure design; gives audit + fork + overflow-retry proof in one mechanism |
| D3 | Three-tier context relief (deterministic prune before LLM summarization) + KV-prefix-preserving summarization + summary-must-be-smaller assertion | session-cognition.md (ideas 3-5) | **High** — deterministic pruner trivial in Go; KV-prefix discipline is provider-agnostic |
| D4 | Retry as fresh durable turns; transport-level retry disabled; retry-after honored within cap | llm-protocols-tools.md (idea 4) | **High** — durable event log + bounded retry loop; replaces hidden SDK retries |
| D5 | LLM adapter registry: `LlmAdapter.stream()` translation, prepareCall binding, atomic route replace, reject-stale-preparation | llm-protocols-tools.md (idea 3; gap-fill OQ2) | **High** — Go interfaces are a natural fit; static registry (compile-time) loses dsh's runtime discovery but keeps the contract |
| D6 | Credentials as references-not-secrets; perm-bit rejection before read; error text never carries values | llm-protocols-tools.md (idea 5; gap-fill OQ5) | **High** — os file modes + YAML; matches vh env-var doctrine |
| D7 | Tool pipeline waterfall: observers accept/block/replace/addContext around a frozen canonical result; pure presenters for replay | llm-protocols-tools.md (idea 2) | **High** — middleware chain is idiomatic Go; gives replayable tool history |
| D8 | Deny-only guard signatures (force-allow unrepresentable) + most-restrictive-wins lattice (deny>ask>allow) shared by tools + hooks | kernel-architecture.md (transferable #6; C2-i1); gap-fill OQ2 | **High** — type-level trick portable via Go func signature returning only denial |
| D9 | Parallel-safe vs exclusive tool classification; bounded rolling pool (default 10); model-order result commits | kernel-architecture.md (gap-fill OQ1 scheduler) | **High** — errgroup/semaphore; classification-before-dispatch maps cleanly |
| D10 | Subagent activations: durable mode-discriminated descriptors, delegationDepth in session header, provenance-clean child reporting | session-cognition.md (ideas 9-11) | **High** — headers + event kinds; directly replaces OpenCode's opaque subagent task tool |
| D11 | Landlock launcher contract: pinned argv grammar, single sentinel exit, functional probe, fail-closed ENOSYS, static binaries | execution-safety.md (idea 3) | **Very high** — vh ALREADY ships exec-sandbox (Landlock+seccomp); dsh confirms/refines the contract [E10] |
| D12 | Typed runner-failure classification + guard/escalation ladder (denial paired with recovery hint; validate before spending a human prompt) | execution-safety.md (ideas 1-2) | **High** — strengthens exec-ro classifier + shell-guard ask flow |
| D13 | Execution-world seam: fs + subprocess interfaces so local→remote→sandbox is adapter mounting, not a parallel stack | execution-safety.md (idea 13) | **High** — Go interfaces; matches existing runtime backend abstraction [E3] |
| D14 | CAS observation policy for edits: `FS_NOT_OBSERVED` unless session previously read the file | execution-safety.md (idea 12) | **High** — mechanical read-before-write fence; vh edit-fence analog |
| D15 | Host/client faces: same package graph builds host (server) + client (browser) faces; thick stateful client rebuilt from wire; rpcId echo verification | client-runtime.md (§2.1, §2.4; ideas 1, 10) | **Medium-high** — Go serves HTTP/WS + embeds static assets well; the thick client itself is JS/TS (React), so a web face means maintaining a JS frontend inside a Go repo |
| D16 | Web-first UX doctrine: browser trust fence (reject 0.0.0.0), write-only credential UX, settings layering with redaction-safe path-ops + expectedRevision | web-ux-surfaces.md (ideas 4, 5, 11) | **High** (host side) — net/http + loopback binding; settings layering for opencode.jsonc analog |
| D17 | Cordis plugin/DI model: fiber lifecycle, services-on-ctx, slots with shadowing/abdication | kernel-architecture.md (transferable #1-2); client-runtime.md (§2.2) | **LOW-MEDIUM — the core Go friction point**: Go has no good in-process dynamic plugin story (`-buildmode=plugin` is fragile and cross-compile-hostile; WASM adds a runtime boundary). Realistic Go analog = declarative config + generated wiring + process-level (stdio/ACP) extensions |
| D18 | Generated + gated catalogs: gen-*/verify-* one-script-two-modes byte-diff; catalogs harvested by real boot | llm-protocols-tools.md (idea 1); engineering-platform.md (idea a) | **High** — go:embed + codegen + `doctor` check; kills doc-rot class |
| D19 | gen/verify duality: generated output is a candidate until paired verifier passes | engineering-platform.md (headline) | **High** — rhymes with vh's model-output-is-candidate invariant; extends it to generated artifacts |
| D20 | Dump/boot parity: `--dump-config` and boot share one parser/patch algorithm; per-row provenance | kernel-architecture.md (transferable #7); web-ux-surfaces.md (idea 1) | **High** — direct reinforcement of vh --dry-run/update parity rule |
| D21 | Disable-not-delete overlays + named plane-ownership criteria with failure modes | kernel-architecture.md (C3-i1, C3-i2) | **High** — config discipline for overlay/profile growth |
| D22 | Fail-closed defaults as product philosophy: unknown events, unanswerable approvals, unavailable runners all refuse | README.md cross-cutting themes | **High** — already vh's philosophy; dsh proves it scales to a full product |
| D23 | MCP client with budgeted reconnect supervisor; ACP bridge (allow-once/reject-once permissions, committed-text-only wire) | llm-protocols-tools.md (§2.6 F-MCP-3; gap-fill OQ3) | **High** — MCP has official Go SDK support; ACP (JSON-RPC over stdio) is language-neutral |
| D24 | Single-binary distribution via exe packaging (node single-executable; detached-worktree packing) | engineering-platform.md (release lane) | vh is ALREADY a single static Go binary — **ahead of dsh here**; nothing to import beyond release-lane hygiene |

**Packet-corpus contradiction feed** (for §10): C-1 in llm-protocols-tools.md (stale example README vs mcp-client src over auto-reconnect) and execution-safety C1-C4 (sandbox system-tmpfs semantics, partial-enforcement labeling, dev-null asymmetry) are recorded IN the dsh corpus; none block this scout.

## 7. Product-class table (M3) — surveyed 2026-08-19, ≤6 web lookups

| Product | Owns LLM→frontend? | Binary / language | TUI / Web / both | Extension mechanism | Single binary? |
|---|---|---|---|---|---|
| **opencode** (sst/opencode) | Yes — full stack: LLM providers, unified tool registry (25+ service deps), sessions in SQLite (Drizzle/Effect), HTTP+WS server, ACP | TypeScript (Effect framework), npm-distributed; Electron desktop shell | **Both** + VS Code ext + ACP clients; TUI has own feature-plugin architecture | JS plugins + lifecycle hooks, MCP servers (incl. OAuth), LSP; per-agent config | No — Node/Bun process; desktop = Electron wrap |
| **pi** (badlogic/pi-mono) | Yes — layered: `pi-ai` (unified multi-provider LLM API), `pi-agent-core` (provider-agnostic loop), `pi-tui`, `pi-client`/`pi-protocol` (schema+codec only, no LLM stack) | TypeScript monorepo; npm `pi` bin; Node ≥22; v0.84.x | **TUI** (4 run modes: interactive TUI, headless, RPC/SDK, ...) | Self-extensible: JS extensions via `ExtensionAPI` + bundled virtual modules; skills | No — npm package; also consumed as SDK library |
| **Claude Code** (Anthropic) | Yes — proprietary full stack | Compiled binary wrapping a Node-based JS bundle (source-map leak analyses report ~1,906 TS files — secondary sources, medium confidence) | **Terminal + IDE + desktop + web** (official docs) | Plugins: skills, agents, hooks, MCP servers, LSP servers, monitors (official reference) | Effectively yes for the CLI (installed binary) |
| **Codex CLI** (openai/codex) | Yes — `codex-rs` core crate, protocols, prompts, sandbox, TUI | **Rust** (`codex-rs/`) | **TUI default** + `codex exec` headless + app-server WebSocket (IDEs); `/agents` TUI dashboard w/ subagent status | MCP (+OAuth RFC 9728), hooks, config.toml profiles; not a general plugin runtime | **Yes** — one `codex` binary |
| **dsh** (from M2 corpus — primary, pinned commit) | Yes — kernel/agent-loop/tools/LLM adapters/web client; host+client build faces | TypeScript (Cordis DI); node exe packaging for distribution | **Web-first** (browser client + host server); headless mode | Cordis plugins (DI fiber lifecycle, slots); MCP; ACP bridge | Partially — packaged node single-executable |

**dsh↔pi relationship — VERIFIED from both sides**: dsh contains `packages/llm/llm-pi-ai` (dsh corpus: llm-protocols-tools.md coverage table) and pi-mono ships `pi-ai` as its "Unified Multi-Provider LLM API" — dsh consumes pi's LLM library as one of its adapters. Two independent products sharing one TS LLM abstraction.

Sources: opencode — zread.ai/sst/opencode (§6 architecture-overview, §1 overview, §19 CLI/TUI); pi — zread.ai/badlogic/pi-mono (§7 architecture-overview, §1 overview, §11 coding-agent, §16 extension-system, §19 packages); Claude Code — code.claude.com/docs/en/overview ("runs on several surfaces: terminal, IDE extensions, desktop app, web"), /en/plugins-reference, /en/desktop, /en/claude-code-on-the-web; binary internals via secondary leak analyses (agent-safehouse.dev, note.com engineers_hub — medium confidence); Codex — zread.ai/openai/codex (§1 "Rust-native system", §7 architecture, §11 sandbox implementations: Seatbelt macOS / bubblewrap+Landlock Linux w/ --cap-drop ALL, §21 config layering).

**Evidence register (M3 additions)**:

| E-ID | Statement | Source | source_type | quality | recency |
|---|---|---|---|---|---|
| E13 | opencode: single-process TS app exposing CLI/TUI/HTTP+WS/ACP; SQLite sessions; plugins+MCP+LSP; npm-distributed | zread.ai/sst/opencode/6-architecture-overview | web (repo docs mirror) | high | 2026-08-19 |
| E14 | pi: TS monorepo; pi-ai/pi-agent-core/pi-tui/pi-protocol layering; npm bin; JS extension system w/ virtual modules | zread.ai/badlogic/pi-mono/7-architecture-overview + 16-extension-system | web (repo docs mirror) | high | 2026-08-19 |
| E15 | Claude Code surfaces: terminal, IDE, desktop, web (official); plugins = skills/agents/hooks/MCP/LSP/monitors (official); Node-based compiled binary (secondary) | code.claude.com/docs/en/overview + /en/plugins-reference; agent-safehouse.dev (inferred label) | web (official + secondary) | high (surfaces) / medium (binary internals) | 2026-08-19 |
| E16 | Codex: Rust-native single binary; TUI+exec+app-server; Seatbelt/bwrap+Landlock sandboxes w/ capability drop; MCP+OAuth; layered config.toml | zread.ai/openai/codex/1-overview + 7 + 11 + 21 | web (repo docs mirror) | high | 2026-08-19 |
| E17 | dsh uses pi's pi-ai as an LLM adapter (`llm-pi-ai` package) | researches/sources/deepseek-harness/llm-protocols-tools.md coverage table + zread.ai/badlogic/pi-mono | repo + web | high | 2026-08-19 |
| E18 | Go in-process dynamic plugins are the weakest portability point vs TS plugin ecosystems (D17 corroboration) | scout inference from D17 + no Go dynamic-loading counter-evidence found in budget | inference | low-medium (no dedicated lookup spent) | 2026-08-19 |

## 8. Options (5 materially distinct architecture candidates)

### O1 — full-standalone-go-rewrite
- **title**: Full standalone Go rewrite — Go-native LLM/session/tool/TUI stack with a dsh-inspired event core
- **mechanism**: vh binary grows the whole stack: Go LLM adapter registry (D5), append-only event-sourced session log + derived projections (D1/D2), three-tier context relief (D3), tool waterfall with deny-only guards (D7/D8), subagent activations (D10), Go TUI (bubbletea/tview class) and/or embedded web face; corpus re-targets from opencode.jsonc to a vh-native agent spec; OpenCode dependency dropped at cutover.
- **adaptation_for_repo**: keep every §5 carry-over asset (ownership seam, overlays, doctor, exec family, gated-commit, coordination docs) as the harness layer; new internal/ packages (llm/, session/, tools/, tui/); run-shape gains a real `runtime: vh` path; dogfood switchover gated behind parity checks (D20 dump/boot parity discipline).
- **evidence_ids**: E1-E12, E13-E17, D1-D16, D22
- **assumptions**: [assumption] maintainer capacity exists for a provider matrix + TUI maintenance; [assumption] Go TUI ecosystem (charmbracelet et al.) is sufficient for the operator UX; [assumption] loss of OpenCode's LSP/IDE integrations during transition is tolerable.
- **risks**: largest lift, longest time-to-parity; provider-matrix churn is a permanent tax small teams underestimate (every peer in E13-E16 has a team); dogfood continuity break; extension model must be re-invented under Go's weak dynamic plugins (D17/E18).
- **cheapest_validation_step**: spike branch (no UI): event-sourced JSONL session log + ONE OpenAI-compat provider adapter streaming + ONE tool round-trip + byte-identical replay from the log. Proves D1+D5 in Go before any stack commitment.

### O2 — hybrid-ownership-ladder
- **title**: Progressive seam ownership — keep the harness layer, own seams one rung at a time behind vh-owned contracts
- **mechanism**: OpenCode stays the frontend/session engine initially; vh progressively owns seams behind vh contracts, provider layer first (a vh LLM proxy that routes, logs, retries-at-durable-turns, and audits model-visible traffic), then session-log ownership, then tool pipeline, frontend last (possibly never). Each rung independently shippable and dogfoodable; `run-shape.yml`/profile gain a runtime-selection knob mirroring the existing backend abstraction.
- **adaptation_for_repo**: extends patterns the repo already has — backend abstraction (E3), conditional capability selection (E12); each rung adds a doctor check; corpus dual-renders (opencode.jsonc now, vh-native spec when a vh runtime rung activates).
- **evidence_ids**: E3, E12, D4, D5, D20
- **assumptions**: [assumption] OpenCode remains maintained and its config surface stays stable enough to target; [assumption] each partial-ownership rung delivers standalone operator value; [prediction] rungs can be paused indefinitely without rotting.
- **risks**: permanent dual-stack maintenance; seams ossify into a hybrid nobody finishes; OpenCode API drift breaks rungs; "own the provider layer" may deliver less visible value than expected.
- **cheapest_validation_step**: build rung 1 only — vh provider proxy between OpenCode and providers, logging durable retry turns (D4) — a bounded spike, immediately dogfoodable in this repo.

### O3 — protocol-first-acp
- **title**: Own the protocol, not the monolith — vh implements ACP (+MCP) as its host contract; engines and frontends stay swappable
- **mechanism**: vh binary becomes an ACP host/agent: harness + coordination + safety surfaces exposed over ACP to any compliant client (editors, other agents), MCP for tool federation; a thin vh frontend (or a third-party one) comes later if ever. OpenCode already speaks ACP (E13); dsh has an ACP bridge with allow-once/reject-once permissions (D23); vh's A2A-lite envelope is already protocol-shaped (E11).
- **adaptation_for_repo**: internal/acp package; corpus render target adds ACP agent definitions + skills; exec verb family remains the host-side execution authority; doctor gains an ACP interop check.
- **evidence_ids**: E11, E13, E16, D23
- **assumptions**: [assumption] ACP adoption continues to mature; [assumption] vh's differentiated value (harness discipline) is client-portable; [prediction] ecosystem interop outweighs UX control for this product's actual users.
- **risks**: ACP spec churn; UX ceiling set by whatever client vh doesn't own; does NOT remove the OpenCode/other-engine dependence for the heavy runtime unless vh eventually ships its own engine anyway (converges to O1/O2 later).
- **cheapest_validation_step**: write an ACP adapter exposing vh's task-card/coordination surface to one existing ACP client; interop-test without building any engine or frontend.

### O4 — web-first-go-host
- **title**: dsh-style host/client faces — embedded Go HTTP+WS host with a browser frontend; TUI later or never
- **mechanism**: vh grows a loopback-bound, trust-fenced (D16) HTTP+WebSocket host face serving a browser frontend; session truth in the event log (D1); Go embeds the static frontend assets (go:embed, preserving the single binary); thick-client state machines live in the browser (D15); exec verbs are the tool backend.
- **adaptation_for_repo**: internal/server package; frontend authored in TS inside the Go repo (build-time embed) or experimentally WASM-Go; corpus/doctor/coordination unchanged.
- **evidence_ids**: D1, D15, D16, E13 (opencode web app), E15 (Claude Code web/desktop demand signal)
- **assumptions**: [assumption] the operator workflow tolerates browser-first (this repo's operators are terminal-native today — unverified); [assumption] maintaining a JS frontend inside a Go repo is acceptable to maintainers.
- **risks**: JS frontend maintenance tax (C5); abandons terminal-native UX the dogfood loop relies on; browser attack surface requires the trust fence to be right; D17-equivalent problem reappears as frontend extensibility.
- **cheapest_validation_step**: spike `vh-agent-harness serve` exposing a read-only event-log/session viewer over loopback WITH the trust fence (reject non-loopback bind, URL printed at readiness) — no agent loop needed.

### O5 — status-quo-harvest
- **title**: Status-quo control — stay OpenCode-based; harvest only the dsh ideas that need no runtime ownership
- **mechanism**: keep the current split (§5 table); import the no-runtime dsh ideas: credentials doctrine (D6), gen-verify catalogs into doctor (D18/D19), typed failure classification + escalation ladder into exec-ro/shell-guard (D12), most-restrictive-wins lattice into rendered policies (D8), dump/boot parity hardening (D20); consume OpenCode features (ACP mode, MCP) opportunistically as they ship.
- **adaptation_for_repo**: pure deepening of existing surfaces; no new layers; every import lands in doctor/commit-gate/policy scripts.
- **evidence_ids**: E1-E12, D6, D8, D12, D18-D20, D24
- **assumptions**: [assumption] OpenCode stays healthy and aligned with vh's needs; [assumption] the harness layer IS the product (the term contract's definition), not the runtime.
- **risks**: strategic dependence on upstream runtime decisions; hard ceiling on engine-layer guarantees (D14 CAS edit fence, D7 waterfall enforcement are only enforceable when you own the pipeline); every product-class peer differentiates on runtime UX vh cannot touch.
- **cheapest_validation_step**: land 2-3 no-runtime imports (e.g., a gen-verify catalog check in doctor; typed failure classification in exec-ro) — proves the harvest path delivers value this quarter.

## 9. Cross-option notes

- **Dominant tradeoff**: control over the full agent experience + engine-layer safety enforcement (O1/O4) vs continuity and maintenance reality (O5/O2) vs ecosystem leverage without ownership (O3). §5 shows the carry-over asset base is identical across options — the debate is purely about which NEW layers to own, not about protecting existing work.
- **Decisive under-evidenced criterion — C5 (operator-team scale)**: every full-stack peer surveyed has corporate/institutional backing (DeepSeek, Anthropic, OpenAI, sst); this repo shows single-or-few-operator signals but that is [assumption] — not verified. This gap should weigh heavily in the debate and is the #1 open question.
- **Extension model is the deepest technical asymmetry**: TS ecosystems (opencode plugins, pi extensions, dsh Cordis) get dynamic in-process plugins for free; Go does not (D17/E18). O1/O4 must substitute declarative config + generated wiring + process-level protocols — viable (Codex does exactly this in Rust: MCP+hooks, no general plugin runtime) but a real expressiveness downgrade vs OpenCode's plugin surface (which vh currently rides for shell-guard etc.).
- **Composability**: O2 and O3 compose (a ladder whose rungs are protocol contracts); O1 and O4 differ mainly in the frontend bet (TUI vs browser). O5 is the null-move that all others can fall back to at any rung.
- **High-upside/high-risk**: O1 (full product-class entry, full safety-stack ownership, maximum churn). **Safest-with-real-value**: O2 rung 1 (provider proxy: immediate durable-retry/audit value, zero UX change). **Cheapest experiment**: O3 (ACP adapter). **Demand-validated but workflow-mismatched**: O4 (web demand is real per E15, but the dogfood loop is terminal-native).
- **Codex is the existence proof for O1's shape**: Rust single binary, sandbox chain, MCP-not-plugins, layered config — structurally the closest peer to what vh would become (E16), including its harder-line extension model.

## 10. Contradictions

- **None detected** in M1 repo reads (README/AGENTS/profile/run-shape/opencode.jsonc/internal inventory mutually consistent) or between M1 and the dsh corpus.
- Honesty note (not a contradiction): this researcher session has an MCP server (vhmcp) available at the environment level while E7 records no `mcp` section in the repo's opencode.jsonc — session-level vs repo-level config; both facts stand.
- dsh-internal contradictions live in the frozen corpus (llm-protocols C-1 stale README; execution-safety C1-C4 sandbox asymmetries) — they concern dsh internals, not this decision.

## 11. Open questions

1. **Maintainer capacity / team size (C5)** — decisive for O1 vs O2/O5; not verifiable from repo reads alone; needs an operator answer.
2. **OpenCode ACP-mode maturity** (for O3) — existence confirmed in docs (E13); depth/compat untested here.
3. **Go TUI framework choice** for O1 — no evidence gathered in budget (bubbletea vs tview vs custom); needs a spike.
4. **Go dynamic-plugin successor** for D17 — unresolved by corpus; WASMI/extism-style process extensions are the leading candidates but unverified.
5. **Provider scope** — which LLM providers actually matter to this operator (DeepSeek-first? OpenAI-compat umbrella? Anthropic?) shapes O1/O2 rung-1 sizing.
6. **Operator tolerance for browser-first UX** (O4 assumption) — unverified.

## 12. debate_handoff_question

> "Given (a) vh-agent-harness's verified carry-over base — ownership/render, corpus+overlays, doctor, exec verb family + sandbox, gated-commit, coordination docs — all runtime-independent (§5); (b) dsh evidence that the highest-value runtime ideas (event-sourced session log D1/D2, LLM adapter registry D5, tool waterfall D7/D8, sandbox chain D11) are Go-portable EXCEPT dynamic plugins (D17); and (c) a product class where every peer owns its full stack but with materially larger teams (E13-E16): which ownership path should vh-agent-harness take — full standalone Go rewrite (O1), progressive seam ownership behind vh contracts (O2), protocol-first ACP host (O3), web-first Go host (O4), or status-quo harvest (O5) — under the real constraint of maintainer capacity (C5, unverified — treat as the debate's first sub-question), and what is the first cheap validation step for the winning path that preserves dogfood continuity?"

---
*Scout packet complete. Digest follows in session return. Promotion target: `researches/decisions/` after debate consumes it.*
