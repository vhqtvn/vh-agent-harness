# OpenCode v2 — Feature / Architecture Landscape Study

> **Provenance**: retrieved 2026-10-18 by web research (researcher session, read-only).
> Primary sources: opencode.ai docs/changelog (v2 docs tree at `/v2/docs/`), GitHub
> releases (anomalyco/opencode, formerly sst/opencode), Hacker News, press.
> The local `refs/opencode` checkout was used ONLY to observe the "before" state and is
> **not** cited as evidence of shipped v2 behavior (see Contradictions).
> Durable home: researches/sources/opencode-v2-study-2026-10.md (landed 2026-10-18).
> Companion context: our native Go agent engine (designed ~2026-08 to replace opencode).

---

## 1. Version timeline (v1 late-2026-08 era → v2.x)

| Date | Version / event | Evidence |
|---|---|---|
| 2025-12-29 | v1.0.207 — last v1.0.x-line release visible on the releases page (LSP sync, theme preview, local MCP transport cwd) | https://github.com/sst/opencode/releases (repo now redirects to anomalyco/opencode) |
| through 2026 | v1.x line continued; version scheme moved from `v1.0.NNN` to `v1.NN.0` (exact transition release unretrieved — open question) | releases page + v1.19.0 tag |
| 2026-09-11 | **v2.0.0 tagged** (`63f7cee`, by thdxr; tag body only "fix(release): use V2 Docker artifact paths (#48571)" — no release notes) | https://github.com/anomalyco/opencode/releases/tag/v2.0.0 |
| mid-Sept 2026 | v2.0.14 cited in the wild (permission prompt preselects "Allow once"); v2.0.16 server shipped inside OpenChamber (Sept 19) | https://medium.com/@itaispector1/jev-opencode-how-to-connect-them-in-5-minutes-free-for-now-41237c38ce40 ; https://github.com/code-yeongyu/oh-my-openagent/issues/8485 |
| late Sept 2026 | v2.0.18–2.0.19 cited (native `Plugin.define` API in 2.0.19) | https://github.com/osovv/vv-opencode ; https://classic.yarnpkg.com/en/package/opencode-plugin-litellm |
| 2026-10-08 → 10-16 | v2.0.26 (Oct 8), v2.0.27 (Oct 12), v2.0.28 (Oct 13), v2.0.29 (Oct 14), v2.0.30 (Oct 15), v2.0.32 (Oct 16) — **near-daily patch cadence** | https://opencode.ai/changelog |
| 2026-10-11 | **v1.19.0 bridge release**: `opencode upgrade --major` moves 1.x → 2; TUI `/update`; "updates are no longer installed automatically" | https://github.com/anomalyco/opencode/releases/tag/v1.19.0 ; https://aicoder.com/news/news-20261017-opencode-free-glm-5-3-flash-reflection-beam-1m-context |
| 2026-10-12 | Press launch: The New Stack "major overhaul" (terminal+desktop+web on one shared service; 80+ internal plugins) | https://thenewstack.io/opencode-2-shared-service/ |
| 2026-10-11..14 | HN launch threads (praise + controversy) | https://news.ycombinator.com/item?id=50094994 |
| 2026-10-16/17 | **Console security breach** disclosed (see §6) | https://canberk.me/news/opencode-console-breach-provider-api-keys-what-is-known/ |

Company: **SST rebranded to Anomaly**; repo transferred `sst/opencode` → `anomalyco/opencode`
(winget IDs `Anomaly.OpenCode[-Desktop]`, brew tap `anomalyco/tap`, images
`ghcr.io/anomalyco/opencode`). Sources: changelog v2.0.28 (winget), https://opencode.ai/docs
(brew/docker), https://opencode.ai/v2/docs/ (docker tags).

Cadence summary: v2 ships roughly **daily** (v2.0.26→32 in 9 days), with categorized
Core/TUI/Desktop/SDK/Extensions sections per release. v1 maintenance continued in parallel
(v1.19.0 shipped *after* v2.0.26).

## 2. Architecture — what changed structurally

Headline (press + docs): **one shared background service, many thin clients**.

- **Client/server by default.** "A background server owns sessions, plugins, permissions,
  and other application state." Every local client (TUI, desktop, web, IDE extension,
  scripts) discovers/starts one per-user service. `opencode service status/start/stop/
  restart/set cors`, foreground `opencode serve`, `--standalone` private server,
  `--server <url>` attach. Service registry at `~/.local/state/opencode/service.json`;
  Windows background-service default port 45278.
  https://opencode.ai/v2/docs/troubleshooting/ ; https://opencode.ai/v2/docs/cli/ ; changelog v2.0.28.
  (HN nuance: client/server existed in v1 — e.g. neovim/Openchamber clients — v2 makes the
  shared *service* the default. https://news.ycombinator.com/item?id=50094994)
- **Sessions span surfaces**: start in terminal, continue in desktop/web; web UI via
  `opencode pair` (localhost URL + basic-auth credentials); remote pairing via OpenTunnel
  (persistent random subdomain; QR pairing; Settings→Pairing). Desktop app (mac/win/linux
  deb/rpm/AppImage) ships an attached **browser** surface. iOS app exists (delegations
  display as Voice messages). https://opencode.ai/v2/docs/ ; changelog v2.0.29/v2.0.30.
- **Storage moved to SQLite**: `~/.local/share/opencode/opencode.db` (`OPENCODE_DB`
  override; `opencode debug paths db` prints it for sqlite3). Session snapshots are packed
  object stores w/ background lossless packing. https://opencode.ai/v2/docs/troubleshooting/ ;
  https://opencode.ai/v2/docs/cli/ ; changelog v2.0.26.
- **HTTP API with OpenAPI operation IDs**; `opencode api get/post ...` CLI uses local
  service discovery + auth. Clients: `@opencode-ai/client` (network), Effect client,
  **`@opencode-ai/sdk` embedded host that assembles the server and routes API calls
  through its HTTP router in memory (no listener, no network hop)**, plus a
  Cloudflare/workerd host. https://opencode.ai/v2/docs/build/sdk/ ; changelog v2.0.27/v2.0.32.
- **Core loop re-architected on Effect-TS** (typed Effects/Streams; plugin system exposes
  both Promise and Effect flavors). HN: "they re-architected the loop around Effect."
  https://opencode.ai/v2/docs/build/plugins ; https://news.ycombinator.com/item?id=50094994.
- **80+ internal plugins implement the majority of functionality** (The New Stack).
  Third-party plugin API: `Plugin.define({id, setup(ctx)})`, auto-load from
  `.opencode/plugins/`, npm packages or local paths w/ options, **hot reload**,
  RPC plugins (out-of-process), CLI plugins (TUI), registry **transforms** replayed in
  registration order for deterministic rebuilds. https://opencode.ai/v2/docs/build/plugins.
- **Code Mode**: an `execute` tool running JavaScript with **no filesystem, imports,
  timers, or fetch** — it can only call tools in its catalog; used for parallel tool
  orchestration and keeping intermediate values out of model context. MCP tools are
  routed through Code Mode **by default** (`codemode: true`). Browser namespace, session
  utilities, and worktree tools live here too. https://opencode.ai/v2/docs/tools/ ;
  https://opencode.ai/v2/docs/mcp-servers/.
- **Config**: JSON/JSONC `opencode.json(c)` with published schema (dual V1/V2 shapes
  accepted during migration); system/MDM > project > user precedence ladder; `.opencode/`
  discovery from cwd to root; per-directory merges; **policies** (admin-enforced,
  outrank ordinary settings). https://opencode.ai/v2/docs/config ;
  https://opencode.ai/v2/docs/migrate-v1/.
- **Three intentional breaking changes** (official): new plugin API; new server API/client
  contracts; TUI config `tui.json(c)` layers → single global `cli.json`. Everything else
  is meant to be compatible ("treat as a compatibility bug"). Updates default to
  `"notify"` (manual) instead of automatic. **LSP removed**: v2 "does not run language
  servers, expose LSP tools, or produce LSP diagnostics" — use lint/typecheck/compiler
  commands instead. https://opencode.ai/v2/docs/migrate-v1/.

## 3. Feature inventory — what exists NOW in v2 (per area)

| Area | v2 state | Source |
|---|---|---|
| **Agents** | Markdown (`.opencode/agents/<name>.md`, nested-path IDs) + JSONC; modes `primary`/`subagent`/`all`; fields: description, model `provider/model#variant`, system, permissions, steps (max model steps; final step strips tools → text summary), hidden, color, disabled, per-agent `request.headers/body`; builtins: `build`, `plan` (primary), `general`, `explore` (subagents) + hidden `compaction`/`title`/`summary` maintenance agents; "no built-in scout agent"; overrides by same-ID | https://opencode.ai/v2/docs/agents/ |
| **Subagents** | `subagent` tool: foreground or `background: true` child sessions, returns `sessionID` for continuation; only subagent-mode agents; **default nesting depth = 1** (`experimental.subagent_depth` retained); child runs with its OWN permissions (not a parent subset); parent's `subagent` action gates which children; delegated commands auto-run in background and report to parent | https://opencode.ai/v2/docs/tools/ ; https://opencode.ai/v2/docs/agents/ ; https://opencode.ai/v2/docs/migrate-v1/ |
| **Permissions/safety** | Ordered rule array `{action, resource, effect}` — allow/deny/ask, **last matching rule wins**, default ask; wildcards `*`/`?`; `" *"` suffix matches bare command; multi-resource ops: any-deny→deny, else any-ask→ask; actions: read, edit, glob, grep, shell, subagent, skill, question, webfetch, websearch, `external_directory`, `<server>_<tool>` (MCP), `execute` (Code Mode gate); `external_directory` is a SEPARATE boundary decision before read/edit; per-agent defaults (build asks .env/external, explore read-only); approvals: once / always (tool-proposed saved patterns, durable, project-scoped, cannot override deny, docs warn patterns may be broader than displayed) / reject (rejects sibling pending asks); per-project auto-accept override; `ctx.permission.assert` for plugins; permission hooks see MCP `metadata.input` | https://opencode.ai/v2/docs/permissions/ ; changelog v2.0.28/v2.0.29/v2.0.30 |
| **Policies** | Separate `policies` config (top-level; was `experimental.policies`): `provider.use` allow/deny (deny-by-default provider allowlist), `tool.use` hard-deny outranking permissions, block MCP servers/skills/plugins; Console-managed policy; managed/MDM config outranks everything | https://opencode.ai/v2/docs/config ; https://opencode.ai/v2/docs/migrate-v1/ |
| **Shell tool** | Host-user shell (explicitly: full fs/process/network authority — **no sandbox**); `workdir`, `timeout` ms (default 2 min foreground), `background: true` (no timeout, notifies session, full output retained in managed file); **tree-sitter shell scanner** produces per-sub-command permission resources + infers directories (experimental portable scanner fails loud, no fallback) | https://opencode.ai/v2/docs/tools/ ; https://opencode.ai/v2/docs/permissions/ |
| **File tools** | read (2,000-line/50-KiB pages; PNG/JPEG/GIF/WebP/PDF ≤20 MiB to model), glob (limit 100, 30 s), grep (regex/literal + previews), edit (unique-match or replaceAll; formatter after write), write (creates parents), patch (multi-file add/update/move/delete; exposed only for supported GPT models); managed tool-output truncation (`tool_output.max_lines/max_bytes`); path-traversal protection on read/list | https://opencode.ai/v2/docs/tools/ ; https://opencode.ai/v2/docs/config ; changelog v1.0.204 |
| **Web tools** | webfetch (markdown/text/html, 120 s max, no binary), websearch (provider integrations incl. authenticated TinyFish; `websearch.provider: "random"`) | https://opencode.ai/v2/docs/tools/ ; changelog v2.0.30 |
| **Interaction** | `question` tool (choices + free-form; client must support); steering messages queue while busy; per-turn changed-files timeline w/ diffs; `/undo`, `/redo`, `/fork`, `/revert`, `/diff` docked viewer, `/copy` latest reply, session tabs, terminal program-status notifications | https://opencode.ai/v2/docs/tools/ ; changelog v2.0.30/v2.0.32 |
| **Skills** | Agent-Skills-style `SKILL.md` + flat `.md`; **progressive disclosure**: per step only id/name/description advertised; `skill` tool loads body on demand (up to 10 supporting-file path samples, contents NOT auto-loaded); discovery: `~/.config/opencode/skills`, project `.opencode/skills`, **compat `~/.claude/skills` + `~/.agents/skills` (global and project)**; `skills` config = ordered dirs + **HTTP catalogs** (`index.json`, versioned, same-origin, safe relative paths); frontmatter: name/description/slash/`opencode.autoinvoke`/portability fields accepted-not-interpreted; permission action `skill` (deny hides from model; ask before load); precedence built-ins → .claude → .agents → config skills → project → explicit entries | https://opencode.ai/v2/docs/skills/ |
| **LSP** | **Removed in v2** (config accepted, servers never run; use lint/typecheck/compiler instead) | https://opencode.ai/v2/docs/migrate-v1/ |
| **MCP host** | Local (stdio command; cwd/environment, `{env:NAME}` substitution) + Remote (Streamable HTTP); **OAuth by default**: discovery, PKCE, refresh, dynamic client registration, snake_case fields, `auth_server_metadata_url`, `callback_port`; `opencode mcp add/list/auth/logout` CLI; `/mcps` UI; timeouts startup 30 s / catalog 30 s / **execution 12 h**; `protocol: legacy (default) / auto / 2026-07-28` (new MCP revision); capabilities: tools, prompts, resources, **instructions**; MCP tools surface via Code Mode by default (`codemode: false` for direct); elicitation capabilities advertised; `*.localhost` OAuth token endpoints accepted | https://opencode.ai/v2/docs/mcp-servers/ ; changelog v2.0.30 |
| **Sessions/persistence** | Durable rows in SQLite; snapshots (undo/redo/revert, resilient capture, packed objects); session families w/ cost rollups incl. subagents; `/fork`; `session_rename`/`session_move` tools; project auto-archive; **steer vs queue delivery vocabulary** (steer by default; queued inputs promote at safe boundary); configuration loaded from the folder the session is created in | changelog v2.0.26/v2.0.29/v2.0.30 ; https://opencode.ai/v2/docs/tools/ ; HN 50094994 |
| **Compaction** | Checkpoint = structured summary (objective/details/completed/active/blockers/next/files; session model, tools off, ≤4096 out tokens) + serialized tail up to `keep.tokens` (tool output clipped 2000 chars; attachments → textual descriptors); later compactions **update previous summary + carry tail forward**; presented as historical context "explicitly not as new instructions"; auto preflight (JSON-size estimate, 4 chars/token) + **overflow-recovery compact-and-retry once** (works even with `auto:false`); manual compaction durably admitted, coalesces, idempotent by message id; **instruction epoch**: instruction sources diffed before each attempt, deltas durable, text frozen at admission and projected chronologically; compaction advances the epoch; **native provider compaction** opt-in per provider/model (`type: native|summary`) | https://opencode.ai/v2/docs/compaction/ ; https://opencode.ai/v2/docs/config |
| **Hooks/automation** | Plugin runtime hooks (incl. permission hooks inspecting proposed MCP calls); delegated commands run as background subagents; GitHub Action handles issues/workflow_dispatch events (v1 line); OpenCode GitHub App auto-reproduces filed bugs on cited version + dev branch and comments w/ repro script/video (community-observed) | changelog v2.0.30, v1.0.204 ; https://news.ycombinator.com/item?id=50094994 |
| **UI surfaces** | Full TUI (truecolor themes, OSC 8 hyperlinks, docked `/diff`, release highlights); `opencode mini` minimal interface; Desktop app (w/ attached browser panel, MCP UI, provider account manager, pairing, timeline w/ changed-files); **installable web app** (PWA behind auth proxies); VS Code extension (updated for v2); iOS app + remote device pairing | https://opencode.ai/v2/docs/cli/ ; https://opencode.ai/v2/docs/ ; changelog v2.0.27–30 |
| **Providers** | `/connect` TUI flow; provider accounts manager (switch/rename/remove/add); OpenCode Zen (curated models), **Console** (pay-as-you-go tokens, spend caps), **OpenCode Go** $10/mo open-source-model subscription (+Go Plus $40); BYOK any provider; AI-SDK-compatible custom providers via `aisdk:` packages; Azure/Vertex AI (gcloud ADC), GitHub Copilot routing, ChatGPT OAuth (gpt-6.1-sol 500k input), Interactions/Mistral Vertex routes; request-body encoding zstd/gzip; prompt-cache marker compatibility formats; streamed-chunk timeout 15 min; **warming** (periodic transient requests keep sessions warm); auto-continuation past output-token limits; Gemini continuation tokens | https://opencode.ai/v2/docs/ ; https://opencode.ai/v2/docs/config ; changelog v2.0.26–29 ; https://opencode.ai/go |
| **Headless/SDK** | `opencode run` (scripts/CI); `@opencode-ai/client` (JS network client, auto-discovers local service); Effect client; `@opencode-ai/sdk` **embedded host** (`OpenCode.create({plugins})`, in-memory router, no listener); Cloudflare workerd host; experimental server-brokered **voice sessions** (OpenAI GPT-Live/Realtime WebRTC, xAI Grok Voice WS) | https://opencode.ai/v2/docs/cli/ ; https://opencode.ai/v2/docs/build/sdk/ ; changelog v2.0.32 |
| **Extensions/plugins API** | `Plugin.define`; ctx = server client + transforms/hooks/registrations/options/storage/location; domains: agent, provider, model, tool, permission, credential (`credential.activate`); Promise + Effect + RPC + CLI plugin flavors; hot reload; official Figma plugin (MCP + 6 bundled skills); ecosystem plugins (notifier, GitLab auth PKCE, LiteLLM…) | https://opencode.ai/v2/docs/build/plugins ; changelog v2.0.28/29/30 |
| **Worktrees / isolation** | Built-in `worktree_create/list/remove` tools (remove needs edit approval; `force` discards); worktree parent dir configurable; **pluggable worktree strategies** (plugin-registered strategy becomes default in its runtime); sessions movable into worktrees (`session_move`) | changelog v2.0.27 ; https://opencode.ai/v2/docs/config ; https://opencode.ai/v2/docs/tools/ |
| **References** | Named outside context: local dirs or **Git repositories** (pinned branch) exposed as supporting context without external-directory approvals | https://opencode.ai/v2/docs/config |
| **Team/cloud** | Console (managed policies, pay-as-you-go), Zen free model lineup, Go subscription, pairing/OpenTunnel remote access; enterprise/MDM managed config | https://opencode.ai/v2/docs/config ; https://opencode.ai/v2/docs/ |

## 4. Mechanisms worth stealing (patterns for a Go engine)

1. **Durable prompt admission + steer/queue semantics** — prompts are durably admitted
   rows; "steer" inputs interleave mid-run at safe provider-turn boundaries, "queue"
   inputs promote one-at-a-time when the session would go idle; exact-retry idempotency
   by message ID. (Public behavior surface: compaction doc "durably admitted", manual
   compaction idempotency, HN "sub-sessions can be communicated with better".)
   https://opencode.ai/v2/docs/compaction/
2. **Instruction epochs** — instruction sources are re-diffed before every model attempt;
   changes become durable deltas whose model-facing text is frozen at admission and
   projected as *chronological* system messages; compaction advances the epoch; revert
   clears state forcing a full re-read. Prevents both stale and mid-turn-mutating
   instructions. https://opencode.ai/v2/docs/compaction/
3. **Checkpoint compaction with carry-forward** — summary + clipped tail; each later
   compaction *updates the previous summary* and carries the retained tail; checkpoint
   delivered as historical context explicitly-not-instructions (injection defense);
   single compact-and-retry on provider-classified overflow even when auto-compaction is
   off. https://opencode.ai/v2/docs/compaction/
4. **Ordered last-match-wins permission arrays + `external_directory` as a distinct
   boundary decision** — declarative rules with three effects; any-deny aggregation for
   multi-resource ops; saved approvals are project-scoped tool-proposed patterns that can
   never override configured deny, with docs that honestly warn patterns can be broader
   than displayed. https://opencode.ai/v2/docs/permissions/
5. **Tree-sitter shell scanner producing per-sub-command permission resources** — rules
   match `git push *` style sub-commands, and the experimental portable scanner *fails
   loud* instead of falling back (single-parser honesty). https://opencode.ai/v2/docs/permissions/
6. **Transform-based replayable registries** — plugins mutate domain registries (agents,
   models, providers) via synchronous transforms replayed in registration order onto a
   fresh value; deterministic, auditable, hot-reloadable. https://opencode.ai/v2/docs/build/plugins
7. **Code Mode `execute` sandbox** — JS orchestrator with *no* fs/fetch/timers that can
   only call cataloged tools; parallel fan-out without polluting model context; MCP tools
   routed through it by default. https://opencode.ai/v2/docs/tools/
8. **HTTP skill catalogs** — versioned `index.json`, same-origin safe relative paths,
   cached with version-bump refresh; plus `.claude`/`.agents` compat discovery.
   https://opencode.ai/v2/docs/skills/
9. **Embedded SDK host with in-memory HTTP router** — same API surface, zero network
   hop, explicit resource lifetime (`await using`), startup plugin seeding.
   https://opencode.ai/v2/docs/build/sdk/
10. **Reliability details**: auto-continuation past output-token limits w/ bounded retries;
    15-minute streamed-chunk timeout for long-reasoning models; zstd/gzip request bodies;
    provider prompt-cache marker compatibility; warming pings; snapshot packing.
    changelog v2.0.26–29; https://opencode.ai/v2/docs/config
11. **Worktree-as-tool + pluggable strategies** — isolated lanes created/listed/removed by
    the agent itself under edit-approval; sessions move into worktrees.
    changelog v2.0.27
12. **Diagnostics by signal** — SIGPROF → 10 s CPU profile, SIGUSR1 → heap snapshot into
    the log dir (DevTools-openable). https://opencode.ai/v2/docs/troubleshooting/

## 5. Parity-relevant gaps (our Go engine vs v2) — analysis, `type: inference`

Our engine already has: durable replayable sessions, compaction, guarded fail-closed
tool waterfall, async jobs, OpenAI+Anthropic adapters, Landlock/seccomp sandboxed shell,
file tools, three-tier guarded skills, MCP host (stdio + Streamable HTTP, ask-by-default),
subagents w/ depth fence, sys-prompt optimizer, spill + windowed retrieval, policy
auto-approver CLI. **Notable: v2's shell has NO sandbox (host-user authority, docs say
so explicitly) — our Landlock/seccomp shell is a genuine differentiator.** v2 also
*removed* LSP, which we never carried — convergent.

The 10 most parity-relevant things v2 has that we lack:
1. Multi-surface shared service — one server, many clients (TUI/desktop/web/IDE/iOS), pairing/tunnel remote access.
2. Plugin/extension system (transforms, hooks, RPC + CLI flavors, hot reload) — we have none.
3. Code Mode `execute` orchestration sandbox (incl. MCP-through-codemode).
4. Declarative ordered permission rules + saved approval patterns + external_directory as separate boundary (our waterfall is ask-based; rules layer is the delta).
5. Worktree tools + pluggable worktree strategies + session_move.
6. Instruction-epoch sync + checkpoint carry-forward compaction + overflow compact-retry-once.
7. HTTP skill catalogs + `.claude`/`.agents` compat discovery (we do Agent Skills spec; remote catalogs + compat dirs are the delta).
8. Embedded SDK / host API (in-memory router, programmatic embedding, workerd).
9. Session snapshots w/ undo/redo/revert/fork UI semantics on SQLite.
10. Provider-reliability kit: auto-continuation, warming, zstd bodies, cache-marker compatibility, 15-min chunk timeout, voice sessions (experimental).

## 6. Community signal

- **Adoption**: 43.6k GitHub stars, 3.7k forks, 3.2–3.3k open issues, ~513–836 open PRs
  (directly observed 2026-10-18 on github.com/sst/opencode → anomalyco redirect and the
  v1.19.0/v2.0.0 release pages). Daily release cadence with named external contributors.
- **HN launch thread** (https://news.ycombinator.com/item?id=50094994): praise for the
  client/server model, self-editing config, plugin hot-reload, background subagents,
  config-from-session-folder; standout ops anecdote — the OpenCode GitHub App reproduces
  filed bugs in <2 min with repro script + video; OpenTunnel praised (persistent random
  subdomain, wildcard cert generated on-machine so the platform can't see traffic).
  Criticism: hot-reload of plugins seen by some as a security misalignment (agent can
  mutate its own runtime — extended debate, comparisons to Pi harness); "v2.0 has no
  GitHub release" complaint (npm postinstall downloads the binary — called "galaxy brain");
  "too much change dropped at once", breaking-change fatigue; IDE-bloat concern; some
  switched to the Pi harness, others returned after 2.0 bug fixes.
- **v1→v2 friction** (secondary, via answer-engine synthesis of HN threads 50081348 /
  50057747 — not read directly, medium confidence): config-precedence criticism
  ("exact opposite of what it needs to be"), `/update` failures requiring manual installs.
  Reddit r/opencode thread "Opencode 2 vs v1.19.0" (~Oct 11): breaking changes mostly
  config schema/contracts; RAM improvements noted (snippet only; thread body blocked).
- **Breach (2026-10-16/17)**: during decommissioning of a legacy system (the hosted
  **Console**), attackers accessed stored OpenAI/Anthropic/Google API keys tied to a
  "large-scale distillation attack"; ~1.4% of Console users affected; one report of
  ~$1,700 in losses; Anomaly emailed users Oct 17 urging key rotation; Anthropic
  confirmed revoking affected Claude keys. This is the **hosted service**, not the local
  binary. Corroborating secondaries:
  https://canberk.me/news/opencode-console-breach-provider-api-keys-what-is-known/ ,
  https://www.byteherald.com/2026/10/opencodes-distillation-attack-what.html ,
  https://startupfortune.com/opencode-breach-exposes-stored-openai-and-anthropic-keys-prompts-urgent-key-rotation/ ,
  https://www.reddit.com/r/opencode/new/ . Primary disclosure (opencode X account / blog)
  not retrieved directly — open question.
- **Direction of travel**: everything-through-plugins + shared service; surfaces
  proliferation (desktop/web/iOS/voice); remote access (OpenTunnel/pairing); monetized
  model access (Console/Go/Zen); enterprise manageability (MDM/policies); faster
  provider plumbing. Simultaneously: no sandbox in core shell, LSP dropped, updates made
  manual — an opinionated "harness as platform" bet.

## Findings

- **v2.0.0 was tagged 2026-09-11; v2GA/press ~Oct 8–12, 2026**: source=https://github.com/anomalyco/opencode/releases/tag/v2.0.0 + https://opencode.ai/changelog, confidence=high, type=fact
- **v2 architecture = default shared background service owning sessions/plugins/permissions; SQLite storage; OpenAPI HTTP API; Effect-TS core; 80+ internal plugins**: source=https://opencode.ai/v2/docs/troubleshooting/ + /cli/ + /build/plugins + thenewstack.io/opencode-2-shared-service/, confidence=high, type=fact
- **Three intentional breaking changes: plugin API, server API contracts, tui.json→cli.json; updates now manual-by-default**: source=https://opencode.ai/v2/docs/migrate-v1/ + v1.19.0 release, confidence=high, type=fact
- **LSP removed in v2**: source=https://opencode.ai/v2/docs/migrate-v1/, confidence=high, type=fact
- **Permissions = ordered last-match-wins allow/deny/ask rules with external_directory boundary, saved project-scoped approval patterns, per-agent defaults; shell scanner = tree-sitter**: source=https://opencode.ai/v2/docs/permissions/, confidence=high, type=fact
- **Skills = progressive disclosure via `skill` tool; .claude/.agents compat dirs; HTTP catalogs**: source=https://opencode.ai/v2/docs/skills/, confidence=high, type=fact
- **Compaction = checkpoint summary+tail with carry-forward, overflow retry-once, instruction epochs**: source=https://opencode.ai/v2/docs/compaction/, confidence=high, type=fact
- **MCP = stdio + Streamable HTTP, OAuth PKCE+DCR default, protocol 2026-07-28 option, 12h execution timeout, tools routed through Code Mode by default**: source=https://opencode.ai/v2/docs/mcp-servers/, confidence=high, type=fact
- **Subagents: default depth 1, own-permissions child sessions, foreground/background, sessionID continuation**: source=https://opencode.ai/v2/docs/tools/ + /agents/, confidence=high, type=fact
- **v2 shell has no sandbox (host-user authority)**: source=https://opencode.ai/v2/docs/tools/ ("Shell has the host user's filesystem, process, and network authority"), confidence=high, type=fact
- **Company rebrand SST→Anomaly; repo now anomalyco/opencode**: source=winget IDs in changelog v2.0.28 + brew/docker in https://opencode.ai/docs, confidence=high, type=fact
- **Console breach 2026-10-16/17, ~1.4% of Console users, distillation attack, provider keys exposed**: source=canberk.me + byteherald.com + startupfortune.com (corroborating secondaries), confidence=medium, type=fact (primary disclosure not directly retrieved)
- **HN community: hot-reload security debate; no-GitHub-release criticism; breaking-change fatigue; ops automation (GitHub App bug repro) praised**: source=https://news.ycombinator.com/item?id=50094994, confidence=high, type=fact (community sentiment)
- **Config-precedence criticism and /update failure reports**: source=HN 50081348/50057747 via answer-engine synthesis (not read directly), confidence=medium, type=inference
- **Answer-engine figures of 213k stars / 7.5M MAU rejected as unsourced**: direct GitHub observation shows 43.6k stars on 2026-10-18, confidence=high, type=fact (contradiction flagged below)
- **Parity-gap list (§5)**: source=synthesis of the above against our engine's stated feature set, confidence=medium, type=inference

## Contradictions

- **Star count**: answer-engine synthesis cited "213.5k stars" and "160k+ stars / 7.5M MAU" (low-quality secondary sites); direct fetch of github.com/sst/opencode on 2026-10-18 shows **43.6k stars**. The larger figures are rejected; noted because they circulate.
- **"Client/server is new in v2"** (press framing) vs HN commenter: v1 already supported multiple clients against one server; v2's change is making the shared service the default (`opencode service` vs `opencode serve` start "two different things"). Both true at different granularity.
- **"v2 has no release"** (HN, launch week) vs v2.0.0 tag dated 2026-09-11 existing on GitHub: reconciled — v2 initially shipped without classic GitHub release *notes* (tag body is a docker-path fix), distribution via installer/npm-postinstall binary download; winget/installer routes came later (v2.0.28).
- **Local snapshot label vs content**: the operator context calls `refs/opencode` a "stale pre-v2 snapshot", but its AGENTS.md contains extensive **"V2 Session Core"** engineering rules (durable session_input admission, SessionExecution, steer/queue promotion, System Context algebra/epochs) — i.e. the snapshot appears to be a v2-development-era checkout, not pre-v2. Flagged per instructions; local file was NOT used as v2 evidence (public docs independently corroborate the same behaviors).
- **Docs default tree**: https://opencode.ai/docs still describes "OpenCode 1" as the primary install; v2 lives under /v2/. Not an error, but a trap for anyone reading "the docs" without the /v2/ prefix.

## Open questions

1. Exact v2.0.1→v2.0.25 changelog entries (the changelog index only served the latest entries; deeper pagination/anchors unretrieved) — fills the Sept-2026 feature trail precisely.
2. When did the v1 version scheme flip from v1.0.NNN to v1.NN.0, and what shipped at the flip (v1.1.0?)? Unretrieved.
3. Primary breach disclosure text (opencode X post / blog / status page) and whether the "legacy system" was the retired Console or another component; post-mortem status.
4. Server API protocol details beyond `opencode api` + OpenAPI operation IDs (auth scheme for local clients, event stream shape) — the /v2/docs/build API reference pages were not individually fetched.
5. Whether `experimental.subagent_depth` raises depth above 1 and how background subagents interact with the depth fence in practice.
6. Pricing/limit accuracy for Go/Go Plus/Console tiers (answer-engine figures medium-confidence; opencode.ai/go not fetched directly).
7. hooks/lifecycle event surface for plugins beyond permission hooks (a dedicated hooks page exists in the build docs tree, not fetched).

---
*End of packet. Retrieved 2026-10-18; v2 moves daily — re-verify changelog before relying on version-specific details.*
