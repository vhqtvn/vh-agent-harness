# AI Coding-Agent Harness Landscape — Delta Study: 2026-08 → 2026-10-18

**Retrieved**: 2026-10-18 (all web sources read live on this date)
**Baseline**: repo's 2026-08 landscape snapshot (then-leaders: Claude Code, Codex CLI, deepseek-harness, opencode, pi, Gemini CLI); dsh baseline = frozen study at commit `47f9438` (2026-08-13, v0.1.0-rc.5), see `researches/sources/deepseek-harness/README.md`.
**Method**: primary sources first (official changelogs, release pages, spec repos, vendor blogs); secondary technical press only for corroboration/positioning, marked as such. Every claim carries a URL. `type: inference` flags mark synthesis. No fabricated versions/dates; where a date could not be pinned, the claim is version-anchored or flagged.
**Durable home**: researches/sources/harness-landscape-2026-10.md (landed 2026-10-18).

---

## 1. Claude Code (Anthropic)

Current version **2.1.302** (Oct 16, 2026) — https://code.claude.com/docs/en/changelog ; raw: https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md

### Async dispatch / task evolution
- **Cloud sessions** are now a first-class surface: scheduled **routines**, org-level concurrency limits, self-hosted runners (git-based), "Rewind to here" in cloud sessions, per-routine failure banners (2.1.299–302 changelog entries).
- **`agent.spawn` for teammates** added Oct 3, 2026; one agent id across plugin hook events; idle/waiting states in `$.agent.list()` — https://code.claude.com/docs/en/changelog#october-3-2026
- **Fork subagents** (`subagent_type: "fork"`) now default-on in `claude -p`, Agent SDK, desktop, VS Code; background subagents default in desktop/VS Code; a subagent's own Agent calls run foreground by default (2.1.302).
- **Workflow tool + workflow agents**: background sessions carry workflows through approval; `agent.spawn` mod hook can rewrite workflow-agent `prompt`/`model` (2.1.300). Harness note that workflow agents' "task text carries no user authority" (2.1.301 fix) — an explicit authority-boundary guard.
- **Idle compaction** (2.1.299): a 200K+-token conversation idle ~54 min is compacted *before the one-hour prompt cache expires* — cache-aware compaction scheduling; tunable via `CLAUDE_CODE_IDLE_COMPACT_MIN_TOKENS`.
- `claude daemon` background service; `--bg`/`--background` sessions; scheduled wakeups (`ScheduleWakeup`); DST-safe scheduled tasks/loops (2.1.299 fixes).

### Skills / hooks / plugins ("Mods")
- **Mods** — an in-client programmable extension layer (hooks like `prompt.submit`, `prompt.change`, `tool.check`, `agent.spawn`, `session.append`; APIs `$.env.set`, `$.ui.view`, `$.model.fork`, `$.prompt.media`, `$.session.identity`; UI components Text/Box/Input with `rows`, `virtualText`, RTL `dir`), pervasive across 2.1.299–302. Third-party summaries call this "Claude Mods for deeper plugin behavior modification" — https://agentgid.com/agents/claude-code/changelog (secondary). `type: inference`: Mods appear to have shipped during Sept–Oct 2026; exact launch version not pinned from primary changelog slice available (see Open questions).
- **Plugin ecosystem formalized**: plugin developer portal + Claude directory with review tracking and usage analytics announced **Sept 25, 2026** — https://support.claude.com/en/articles/12138966-release-notes (Sept 25 entry).
- **Skill & plugin security scanning (beta)** for Enterprise since **Aug 6, 2026** (same release-notes URL) — automated malicious-content checks on upload/edit.
- Hooks: concurrency cap (max 12 same-event hooks run concurrently, 2.1.302); `updatedInput` from PreToolUse/PermissionRequest hooks now validated like native input (2.1.300) — hooks can no longer inject arbitrary keys.
- Skills mechanics fixes (args placeholder limits, memory caps, listing after `/rewind`) show heavy production use (2.1.301).

### Permissioning / safety
- **Auto mode classifier** (client- and server-side) with `CLAUDE_CODE_AUTO_MODE_MODEL`; OTEL `tool_decision` events now carry `classifier_verdict`, `permission_mode`, `classifier_category` (2.1.301–302) — classifier-driven permissioning is now an observable, auditable surface.
- `claude -p`/SDK sessions default to `default` permission mode on third-party providers unless `--permission-mode auto` (2.1.302).
- Sandbox hardening fixes: sandboxed shell can no longer write into `--plugin-dir` folders (2.1.299); macOS symlink traversal in denied paths fixed; `denyRead/allowRead` glob consistency (2.1.299); plan-mode write allowed by a PreToolUse hook is now refused (2.1.299 fix).
- Two cybersecurity features added to Claude Code per Oct 7 release notes ("/security …") — https://support.claude.com/en/articles/12138966-release-notes (secondary confirmation of the /security command: first search hit's snippet; primary detail thin). Anthropic 2026 Usage Policy update Oct 8: https://www.anthropic.com/news/2026-usage-policy-update

### Surfaces (web/IDE)
- **Claude Tag** (Slack): launched June 23, 2026 (release notes, June entry); by Oct it has admin tabs (Bundles/Connectors/Skills-and-plugins/Channels), Slack Connect support, private allow/deny approval cards, spend limits (2.1.300–302).
- **Remote Control** (drive local sessions from claude.ai/desktop/mobile) + **Trusted Devices** (June 25); **Computer use (preview)** surface in docs nav; **Claude in Chrome**; **Claude Code in the cloud** with self-hosted runners — docs nav: https://code.claude.com/docs/en/changelog
- Platform convergence: Claude Cowork merged into every conversation (Sept 16); Claude Design/Slides/Docs artifacts usable from Claude Code (Sept 16, release notes).
- Reproducibility: local session transcripts now record each API request's system prompt, tools, and parameters (2.1.300) — "a transcript you choose to share can reproduce a request exactly".

### Model wave (context)
Fable 5.1 + Mythos 5.1 (Sept 1) → Opus 5.5 (Sept 22, −40% cost vs Opus 5) → Sonnet 5.5 (Sept 28) → Haiku 5.5 (Oct 7) — https://support.claude.com/en/articles/12138966-release-notes . Amp's chronicle corroborates Opus 5.5 and Fable 5.1 availability: https://ampcode.com/chronicle

---

## 2. Codex CLI (OpenAI)

Current stable **0.163.0** (Oct 15, 2026); 0.164.0-alpha.3 in flight; ~171 releases tracked in 2026, ~1 release per 1.4 days — https://github.com/openai/codex/releases ; https://www.havoptic.com/tools/openai-codex (tracker, secondary)

- **Instant steering**: "Send a follow-up while Codex is responding to steer it immediately"; instant interrupts default-on, including stopping Code Mode cells and nested tool calls (0.163.0) — https://github.com/openai/codex/releases . Explainer: https://www.developersdigest.tech/blog/codex-cli-0-163-instant-steering-config-search-2026 (secondary)
- **Voice start/steer + `/agents` view** for delegating/tracking multiple tasks; prompt editing, session resume, worktrees, cleaner TUI — announced at **DevDay 2026 (Sept 29)**: https://openai.com/index/devday-2026-recap/
- **Codex in the cloud**: reusable dev environments with team-approved settings/permissions; run from phone/cloud (DevDay recap).
- **Agents SDK convergence**: the **Agents API** now exposes "Codex's multi-agent capabilities, tool search, tool calling, and context compaction" plus computer use; Bedrock Managed Agents brings the same to AWS (DevDay recap) — the CLI's harness internals are being productized as a hosted API.
- **Sandbox integrity program**: internal changelog (0.163.0) shows integrity checks run *before* commands and filesystem operations, with bubblewrap (Linux) and Seatbelt (macOS) backends, MXC sandbox (Windows) migrated to 1.0.0 SDK with managed-policy enforcement — https://github.com/openai/codex/releases
- **Guardian**: an internal review/classification subsystem (snapshot prefixes, conversation classifiers, trust levels, persisted review failures across restarts, orchestrator connector trust) — visible throughout 0.163.0 PR list. `type: inference`: Guardian appears to be Codex's safety/review layer; OpenAI docs do not describe it end-to-end in the pages read.
- **Permissions durable across resume**: saved threads restore permissions and network approvals subject to managed policy (0.163.0).
- **Parallel read-only tools**: skills, memory, and conversation-history lookups run concurrently (0.163.0).
- Model wave: GPT-6 family (migration guidance in bundled docs, 0.163.0); **GPT-6.1 Sol** launched at DevDay targeting agentic coding at 1/5 Astra price; **Ultrafast** tier up to 8× faster tokens (300 tok/s) in Codex; GPT-5.5 retired from Codex Oct 14 ( https://learn.chatgpt.com/docs/changelog , community mirror) ; GPT-5.3-Codex-Spark deprecated Sept 14 ( https://www.codex-docs.com/en/docs/changelog , community mirror).
- Also from DevDay: **Codex Security Cloud** (repo scanning, fix prep with laptop closed), **Code Review** in ChatGPT desktop with automatic cloud first-passes, **Decisions API** (Luna-powered classification for routing/next-action), managed Git worktrees + pinned tasks ( https://releasebot.io/updates/openai/codex , secondary).

---

## 3. deepseek-harness (dsh) — since commit 47f9438 (2026-08-13, v0.1.0-rc.5)

**Status: no v1.0.** Release ladder since baseline: v0.1.5-rc.2 (Sep 10) → v0.1.5-rc.3 (Sep 22, `a4c74a9`) → v0.2.0-rc.x → v0.2.1-alpha.x (Oct) → **v0.2.1-rc.2 (Oct 17, `1600d8a`)** — https://github.com/deepseek-ai/deepseek-harness/releases ; compare: https://github.com/deepseek-ai/deepseek-harness/compare/dsh-v0.1.5-rc.1...dsh-v0.1.5-rc.2

Headline deltas (all from v0.2.1-rc.2 release notes, URL above):
- **Experimental Claude Code Mods compatibility layer** — stated goal: "verify that the Claude Code Mods API's capabilities are broadly a subset of what DeepSeek Harness plugins can do". A third-party harness is now treating CC's extension API as a de-facto compat target.
- **Official plugin catalog ships Claude Code and Codex bundles** installable into dsh for headless, SDK, and ACP profiles — dsh hosts its competitors' agent bundles.
- **Plugin platform maturation**: custom plugin index sources, dependency sync on enable/disable/update, hot reload with restart prompts, "let the Agent create a plugin" creator mode, experimental **Session state-record APIs** for plugins.
- **AGENTS.md global instructions** via shared agents directory + `DSH_AGENTS_HOME`.
- `working_directory` tool + TS/Python SDK APIs; **SDK resume** (same session ID + working directory).
- **Standalone executable SSH helper runtimes** (remote files, processes, terminals, sandboxing, Node PTC).
- Experimental **Git Worktrees plugin**; **Developer Tools bundle** (raw session logs, chat↔conversation bidirectional navigation, embedded NodeJS debugger); **reasoning-translation plugin** (Bing/Google/DeepSeek Flash).
- Subagent/Agent Team rework: tools return child ID immediately (SDK runtime must support `session/wait`); direct-to-Inbox message delivery, outbox removed; per-subagent **Agent Presets**.
- Automations became built-in web capability (scheduled/reminder tools gated by mode).
- **Removed the runtime invariant plugins / `./invariant` exports** — the "explained-empty audit companions" machinery from our Aug study is gone from the product surface.
- pi-ai → 1.0.2; dynamic in-conversation system-prompt updates and tool add/remove per model capability.
- **Desktop public preview** (macOS/Windows) + settings transfer, per official preview page: https://www.deepseek.com/en/harness/ ; secondary: https://explainx.ai/blog/deepseek-harness-desktop-macos-windows-2026
- Architecture context (plugin framework "Cordis", "Everything is a Plugin"): https://github.com/deepseek-ai/deepseek-harness ; https://www.eesel.ai/blog/deepseek-harness (secondary)

`type: inference`: direction = ecosystem gravity play (compat + bundling of rival harnesses + plugin marketplace mechanics) rather than kernel re-architecture; the kernel concepts we studied (append-only session log, deny-only guards) show no public rewrite signal in the release notes read.

---

## 4. opencode — headline only (sibling researcher owns the v2 deep-dive)

**v2 exists and is GA**: v2.0.x line, v2.0.32 on Oct 16, 2026 — https://opencode.ai/changelog . The 2.0 architecture puts terminal, desktop, and web apps on a **shared service** (rebuilt API, embeddable SDK, "Code Mode"); upgrade from v1 is an explicit `/update` / `opencode upgrade --major` — https://thenewstack.io/opencode-2-shared-service (Oct 12, secondary). Repo org is now `anomalyco/opencode` (Anomaly): https://github.com/anomalyco/opencode/releases . Positioning: the open-source, provider-agnostic everyman — "used and trusted by over 16M developers every month" (self-reported, https://opencode.ai ), ~212k stars ( https://pinggy.io/blog/top_cli_based_ai_coding_agents/ , secondary), WinGet packages under `Anomaly.OpenCode`. Recent surface motion: built-in worktree tools in Code Mode (v2.0.27, Oct 12), remote device pairing via **OpenTunnel** (v2.0.29, Oct 14), per-project auto-accept (v2.0.28), native Windows ARM64, voice-session SDK (`client.voice.session(...)`, v2.0.32), reads `.claude` skills (`OPENCODE_DISABLE_CLAude_CODE_SKILLS` honored, v2.0.30). Free GLM 5.3 Flash + 1M-context "Reflection Beam" offering announced Oct 17 — https://aicoder.com/news/news-20261017-opencode-free-glm-5-3-flash-reflection-beam-1m-context (secondary).

---

## 5. The rest — one tight delta each (NEW since Aug 2026 unless noted)

- **Gemini CLI / Antigravity CLI (Google)**: Gemini CLI **replaced by Antigravity CLI on June 18, 2026** for unpaid/Google One tiers; remains supported only under Code Assist Standard/Enterprise licenses and Google Cloud — https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/ ; https://antigravity.google/blog/introducing-google-antigravity-cli/ ; repo banner: https://github.com/google-gemini/gemini-cli . The OSS repo still shows releases (v0.64.0, Oct 13 per tracker https://www.havoptic.com/tools/gemini-cli — secondary; repo-maintenance status ambiguous, see Contradictions). Antigravity CLI preserves Gemini CLI DX constructs: https://antigravity.google/docs/cli/gcli-migration/
- **Crush (Charm)**: Go-based TUI agent (go.mod at repo root: https://github.com/charmbracelet/crush/blob/main/go.mod ; "TUI-first Go-based agent, multi-model with MCP + LSP" per https://cleverhack.com/ai-coding-landscape , secondary); **Hyper, from Charm, is the official Crush provider** (subscription + free tier) — repo README via https://github.com/charmbracelet/crush ; ~28.7k stars per https://charm.land/ . Dated deltas since Aug not pinned (shallow coverage — no dated release page captured).
- **Amp (Sourcegraph/Amp Frontier Corp)**: went **free with BYO compute/keys** (Sept 13); one runner serves many directories (Sept 17) and creates worktrees from the composer (Sept 22); macOS app as runner (Sept 24); Opus 5.5 (Sept 28); **Puck meta-agent** now multi-conversation (Oct 6) and exposed **via MCP** to other agents (Oct 13); Claude Pro/Max plans usable in Amp free (Oct 10); see your macOS runner's desktop (Oct 16) — all dated entries: https://ampcode.com/chronicle
- **OpenHands**: **Agent Canvas** shipped scoped Agent Profiles, persistent automations, isolated Docker runtimes, easier Apps (Sept 24 blog: https://www.openhands.dev/blog ); v1.24.0 Sep 25 ( https://munderdiffl.in/blog/open-source-ai-agents/ , secondary); **OpenHands Index** benchmark live ( https://benchlm.ai/benchmarks/openhandsindex , secondary).
- **pi (Mario Zechner / Earendil)**: repo at `earendil-works/pi` (moved when Zechner joined Earendil, Apr 2026); **extensions API — "change the harness, not just the prompt"** ( https://www.aibuilderclub.com/blog/pi-agent-extensions-guide , secondary; https://flaviocopes.com/pi/ Oct 2, secondary); SDK-first positioning ( https://thomas-wiegold.com/blog/pi-coding-agent/ Aug 8, secondary); ~95k stars ( https://www.developersdigest.tech/blog/pi-deep-dive-agent-toolkit-architecture , secondary); used unmodified in research ( https://arxiv.org/html/2609.21157v1 ). Note: `badlogic/pi` on GitHub is now a *different* tool (vLLM GPU-pod CLI): https://github.com/badlogic/pi
- **Zed agent surfaces**: Zed 1.0 (Apr 29, 2026) shipped parallel agents; weekly stable cadence, current 1.23.3 (Oct 12: https://zed.dev/download ); **ACP (Agent Client Protocol)** collaboration with JetBrains since Jan 2026 ( https://agmazon.com/blog/articles/technology/202603/zed-ide-complete-guide-en.html , secondary); recent: agent-panel **checkpointing** telemetry (opt-in), plan updates from ACP agents ( https://releasebot.io/updates/zed , secondary; https://zed.dev/releases/stable ).
- **GitHub Copilot CLI / workspace**: Copilot reorganized into **Home / Code / Autopilot** (Sept 25) — Autopilot is a persistent, proactive personal agent — https://blogs.microsoft.com/blog/2026/09/25/introducing-the-new-copilot-with-home-code-and-autopilot/ ; Copilot CLI plugin lifecycle actions + Entra broker auth on macOS, sandbox credential config (changelog "95" entry 2026-10-09: https://github.com/github/copilot-cli/blob/main/changelog.md ); "dynamic workflows, code-defined orchestration" public preview across CLI/app/SDK Oct 1 ( https://dev.to/pwd9000/project-hydrafusion-github-copilot-stops-picking-a-model-and-starts-building-a-workflow-4lh5 , secondary); Copilot CLI agent in JetBrains IDEs since May 13 ( https://github.blog/changelog/2026-05-13-introducing-copilot-cli-agent-and-unified-sessions-view-in-github-copilot-for-jetbrains-ides/ ). Security note: encrypted-prompt-injection PoC against Copilot CLI publicized Oct 7 ( https://delana.co/cybersecurity/github-copilot-cli-cryptographic-context-injection-encrypted-prompt-injection-2026/ , secondary).
- **Cursor**: v3.x line; **side chats** for parallel agents (v3.11, July 10 — https://www.developersdigest.tech/blog/cursor-3-11-side-chats-developer-guide-2026 , secondary); Oct: **Cloud Agents on Cursor web** (week of Oct 5), cloud-agent **canvases**, beta **Environments API**, cloud environments up to 300 repos loading 5× faster (Oct 13–16) — https://cursor.com/docs/release-notes ; https://cursor.com/changelog ; Cascade retired mid-2026 ( https://rockstardeveloperuniversity.com/best-code-editors-for-developers/ , secondary).

---

## 6. New entrants since Aug 2026

- **Muse Code (Meta)** — beta **Aug 5, 2026**, left beta **Sept 1, 2026** with SDK developer preview + subscriptions, adding more since — https://aq.dev/agents/muse-code/ (Oct 10, secondary). Meta entering the coding-agent field inside our window.
- **fx (Vercel Labs)** — coding agent as a **native ~6MB Zig binary**, Apache-2.0, WASM-capable — https://flaviocopes.com/fx/ (Sept 21, secondary); https://o16g.com/resources/ (secondary). Closest new thing to our "single static binary" design class.
- **Herdr / Herder** — agent-native terminal multiplexer, single ~10MB **Rust** binary, no GUI/account; v0.9.1 by Sept 28, ~41k stars — https://www.developersdigest.tech/blog/herdr-deep-dive-agent-terminal-multiplexer (Aug 23, secondary); https://inamdaraditya.medium.com/herder-the-rust-terminal-multiplexer-that-finally-makes-ai-agents-manageable-f5e7391ad7b9 (secondary). Not a harness, but a session/orchestration layer agents plug into.
- **Ante** — "self-contained offline coding agent in a single binary" (Show HN; undated lead) — https://o16g.com/resources/ (secondary). Unverified beyond listing; see Open questions.
- Context (pre-window but newly visible): **Warp** open-sourced Apr 28, 2026 ( https://ai-data-base.com/archives/en/119798 , secondary); **OpenClaw** (pi-based personal agent) and **Spotify's Honk** (internal agent merging 1,000+ PRs/day) circulate in ecosystem feeds — https://daily.dev/tags/opencode (secondary, undated).
- **Jev** — a model-routing/decision layer adopted by multiple harnesses (Amp's file ordering "powered by Jev": https://ampcode.com/chronicle Oct 12; ecosystem: https://jevfinder.com/ , https://jev.page/101/resources ). `type: inference`: an emerging shared model-router/verdict-service seam between harness and providers.

No notable NEW Go-based single-binary *harness* found beyond Crush's continuing line; the new single-binary entrants are Zig (fx) and Rust (Herdr).

---

## 7. Standards movement

### MCP — two revisions shipped after 2025-06-18
Release ladder (primary): 2025-06-18 → **2025-11-25** (stable Nov 25, 2025) → **2026-07-28** (RC May 29; **stable Jul 28, 2026**) — https://github.com/modelcontextprotocol/modelcontextprotocol/releases
**2026-07-28 highlights** ( https://modelcontextprotocol.io/specification/2026-07-28/changelog ):
- **MCP is now stateless**: `initialize` handshake removed; protocol version + client capabilities ride in `_meta` on every request; new `server/discover` RPC; `Mcp-Session-Id` removed; SSE resumability/`Last-Event-ID` removed (SEP-2575).
- **`subscriptions/listen`**: one long-lived POST-response stream replaces GET endpoint + `resources/subscribe` (SEP-2575).
- **Tasks** moved to official extension `io.modelcontextprotocol/tasks` (polling `tasks/get`, client input via `tasks/update`) (SEP-2663).
- **Multi Round-Trip Requests (MRTR)**: servers return `resultType: "input_required"` with `inputRequests`; replaces server-initiated `roots/list`, `sampling/createMessage`, `elicitation/create`; all results now carry required `resultType` (SEP-2322).
- **Result caching**: required `ttlMs` + `cacheScope` on list/read results; deterministic `tools/list` ordering for prompt-cache hits (SEP-2549).
- **Deprecations** (12-month window policy, SEP-2596): Roots, Sampling, Logging features; HTTP+SSE transport; OAuth DCR (RFC 7591) → **Client ID Metadata Documents** (PR #2858).
- Auth hardening: RFC 9207 `iss` validation (SEP-2468); credentials keyed to issuer (SEP-2352); OTel trace-context propagation in `_meta` (SEP-414); JSON Schema 2020-12 fully (SEP-2106).
- Adoption proof from the field: Claude Code 2.1.302 fixes negotiation with servers that implement 2026-07-28 but omit `resultType`/`ttlMs`/`cacheScope` (CC changelog, above); opencode v2.0.30 improves "MCP compatibility … advertising spec-compliant elicitation capabilities" (opencode changelog).
- **MCP Events** — proposed specification; OpenAI announced ChatGPT plugin automations built on it (DevDay recap, above). Proposed-spec pointer: linked from https://openai.com/index/devday-2026-recap/
- Registry/auth context: GitHub's nav itself now lists "MCP Registry" as a product surface ( https://github.com/modelcontextprotocol/modelcontextprotocol/releases page chrome); `type: inference`: registry consolidation continues but no dated primary registry news captured this pass.

### Agent Skills spec (agentskills.io)
- Spec + docs live: https://agentskills.io/home ; https://agentskills.io/specification (SKILL.md = frontmatter + unrestricted Markdown body; discovery loads only name/description → activation on demand).
- Adoption timeline: open-sourced by Anthropic ~Dec 2025; **OpenAI and Microsoft adopted SKILL.md late Dec 2025; Google Gemini CLI support Jan 2026** ( https://denser.ai/blog/agent-skills-guide/ , secondary); **compatibility matrix of 11 clients whose own docs state they read SKILL.md, checked Aug 20, 2026** ( https://www.skillsboard.sh/resources , secondary); state-of-skills survey Oct 3 ( https://host0.ai/blog/state-of-agent-skills-2026 , secondary).
- Field evidence in-harness: Codex 0.163 runs skill lookups as parallel read-only tools; Amp "Global Plugins and Skills" (Aug 11, https://ampcode.com/chronicle ); opencode v2.0.30 honors `OPENCODE_DISABLE_CLAUDE_CODE(_SKILLS)` when discovering `.claude` skills; dsh's plugin page updates the `/`-menu skill list; Anthropic ships a plugin *directory* + security scanning + smart reports that recommend packaging repeated patterns as shared skills (Sept 10/25, release notes above). distribution hub: skills.sh ( https://inference.sh/blog/skills/agent-skills-overview , secondary).

### A2A
- **v1.0 March 2026**; **150+ orgs, major cloud platforms, first enterprise production use** (Linux Foundation press, Apr 9, 2026): https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year
- **Aug 17–20, 2026: A2A joined the Agentic AI Foundation (AAIF), consolidating alongside MCP** under one LF-directed body: https://tech.yahoo.com/ai/gemini/articles/google-a2a-protocol-joins-aaif-020554895.html ; https://entranttechnologies.com/tech-news/a2a-protocol-joins-agentic-ai-foundation-alongside-mcp (both secondary; LF-directed announcement).

### Emerging
- **Observability**: Claude Code emits **OTel GenAI semantic-convention** span attributes + cache-write splits by lifetime (2.1.300–301); MCP 2026-07-28 standardizes trace-context propagation (SEP-414). OTel GenAI conventions are becoming the shared telemetry dialect.
- **Memory formats**: Claude's cross-surface Topics (editable, sensitive-topic gating, Aug 25 release notes) and smart-reports skill-mining (Sept 10) are product-level, not an open format; no new open agent-memory standard found this pass (see Open questions).
- **Checkpointing**: Zed agent-panel checkpointing (telemetry opt-in) is the only surfaced signal; CC cloud "Rewind to here" is the session-variant. No open checkpoint format emerged.
- **Tool-result streaming/caching**: MCP `ttlMs`/`cacheScope` + deterministic list ordering (above) is the standard-level movement; Amp ships "incremental tool updates" metrics analog on the Codex side (0.163 PR #51893).
- **Session datasets**: SWE-chat — first large-scale dataset of real coding-agent sessions (Oct 1): https://arxiv.org/html/2604.20779v2

---

## 8. Cross-cutting ideas worth mining (patterns, cited)

1. **Cache-aware lifecycle scheduling** — CC compacts idle conversations *because the 1-hour prompt cache is about to expire* (2.1.299); MCP now mandates cache-friendliness at the protocol layer (deterministic `tools/list`, SEP-2549). Our engine's KV-prefix-preserving compaction has industry confirmation; scheduling compaction against cache-expiry clocks is the transferable trick.
2. **Steering as a first-class interrupt** — Codex instant steering/interrupts default (0.163.0), Amp "Steer, Don't Queue" (Sept 8, chronicle), CC mid-turn messages during permission checks (2.1.301 fix). Queue-vs-steer is now a visible differentiator.
3. **Stateless, per-request protocol identity** — MCP 2026-07-28's removal of sessions/handshake (SEP-2575) matches our stdio JSON-RPC design; adopting `_meta`-carried version/caps + `server/discover` keeps us on the upgrade path. MRTR (`input_required`) is a cleaner model for approvals/elicitation than server-initiated requests.
4. **Pre-execution sandbox integrity verification** — Codex runs integrity checks *before* commands and filesystem ops with per-OS backends (0.163.0 PRs #51828–51843); CC sandboxes the plugin-dir loophole (2.1.299). "Verify the sandbox, then run" is hardening into an expected guarantee.
5. **Extension API as compat surface** — dsh implements a Claude Code Mods compat layer; dsh's catalog distributes CC and Codex bundles; opencode reads `.claude` skills. Skills (SKILL.md) won; extension/hook APIs are the next compat frontier.
6. **Classifier-driven permissioning with audit events** — CC auto-mode classifier + OTEL `tool_decision` w/ `classifier_verdict`/`permission_mode` (2.1.301–302); Codex Guardian trust/classification machinery (0.163.0 PRs). Permission UX is moving from rules-only to model-in-the-loop with observable verdicts — exactly our fail-closed-approval + auto-mode-classifier design space.
7. **Multi-agent views + spawnable teammates everywhere** — CC `agent.spawn`/agents view/teammates (Oct 3), Codex `/agents` + multi-agent v2 children w/ idle eviction (0.163.0), Cursor side chats (v3.11), Amp agent-to-agent messaging (Jul 17). The TUI/IDE now assumes N agents as the default mental model.
8. **Worktrees as the isolation unit of 2026** — CC `--worktree` sessions + `WorktreeCreate` hooks; Codex managed Git worktrees (releasebot); opencode built-in worktree tools (v2.0.27); Amp runner worktrees (Sept 22); dsh worktrees plugin; Cursor cloud environments. Convergent answer to parallel-agent safety.
9. **Voice as a steering channel** — Codex voice start/steer (DevDay), Amp Puck realtime voice (Aug 18→Oct 13 MCP exposure), opencode voice SDK (v2.0.32). Voice is now an input surface for long-running agent control, not a gimmick.
10. **Transcript = reproducible request record** — CC transcripts capture full request params (2.1.300); dsh DevTools bundle exposes raw session logs w/ bidirectional navigation; SWE-chat mines real sessions. Durable replayable sessions (our core bet) are becoming an ecosystem expectation.
11. **Remote/local continuum** — CC Remote Control + cloud sessions + self-hosted runners; Codex cloud; Amp runners/app; opencode OpenTunnel pairing; Cursor cloud agents. Every major harness now spans "same session, anywhere".

---

## Findings

- **Claude Code 2.1.302 (Oct 16) current; Oct 3 added agent.spawn/teammates; cloud sessions+routines+self-hosted runners mature; Mods extension layer pervasive; plugin directory + security scanning live**: source=https://code.claude.com/docs/en/changelog + https://support.claude.com/en/articles/12138966-release-notes, confidence=high, type=fact
- **Anthropic model wave Sept–Oct: Fable 5.1+Mythos 5.1 (Sep 1), Opus 5.5 (Sep 22), Sonnet 5.5 (Sep 28), Haiku 5.5 (Oct 7)**: source=https://support.claude.com/en/articles/12138966-release-notes, confidence=high, type=fact
- **Codex CLI 0.163.0 (Oct 15): instant steering default, parallel read-only tools, sandbox-integrity pre-checks (bubblewrap/Seatbelt/MXC), Guardian subsystem, durable permissions on resume**: source=https://github.com/openai/codex/releases, confidence=high, type=fact
- **DevDay 2026 (Sept 29): Codex cloud envs, voice CLI steering, /agents view, Agents API absorbing Codex multi-agent/tool-search/compaction, Codex Security Cloud, GPT-6.1 Sol, Ultrafast 300 tok/s**: source=https://openai.com/index/devday-2026-recap/, confidence=high, type=fact
- **dsh at v0.2.1-rc.2 (Oct 17), no v1.0; ships experimental Claude Code Mods compat layer; official catalog distributes CC and Codex bundles; SDK resume; SSH helper runtimes; removed ./invariant exports**: source=https://github.com/deepseek-ai/deepseek-harness/releases, confidence=high, type=fact
- **dsh direction is ecosystem-gravity (compat/bundling/marketplace), not kernel rewrite**: source=release-notes synthesis, confidence=medium, type=inference
- **opencode v2 GA (v2.0.32, Oct 16): shared-service architecture, Code Mode, embeddable SDK, Anomaly org, 16M MAU self-claimed**: source=https://opencode.ai/changelog + https://thenewstack.io/opencode-2-shared-service + https://opencode.ai, confidence=high (facts) / self-reported metric flagged, type=fact
- **Gemini CLI replaced by Antigravity CLI June 18, 2026 for consumer tiers; enterprise continuation only**: source=https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/, confidence=high, type=fact
- **MCP spec: 2025-11-25 then 2026-07-28 stable (Jul 28); stateless protocol, MRTR input_required, ttlMs/cacheScope caching, Roots/Sampling/Logging deprecated, DCR→Client ID Metadata Documents**: source=https://github.com/modelcontextprotocol/modelcontextprotocol/releases + https://modelcontextprotocol.io/specification/2026-07-28/changelog, confidence=high, type=fact
- **Agent Skills (SKILL.md) is the de-facto cross-vendor extension standard; 11 documented clients as of Aug 20 matrix**: source=https://agentskills.io/specification + https://www.skillsboard.sh/resources (secondary for count), confidence=high (spec) / medium (count), type=fact
- **A2A v1.0 (Mar 2026), 150+ orgs (Apr 9), joined AAIF alongside MCP (Aug 17–20)**: source=https://www.linuxfoundation.org/press/a2a-protocol-surpasses-150-organizations-lands-in-major-cloud-platforms-and-sees-enterprise-production-use-in-first-year + https://tech.yahoo.com/ai/gemini/articles/google-a2a-protocol-joins-aaif-020554895.html, confidence=high (primary press) / medium (AAIF transfer), type=fact
- **New entrants in-window: Muse Code (Meta; beta Aug 5 → GA Sep 1), fx (Vercel Labs, ~6MB Zig binary), Herdr (agent-native Rust terminal multiplexer, ~41k stars by Sep 28)**: sources as cited in §6 (all secondary), confidence=medium, type=fact
- **No new Go single-binary harness emerged; new single-binary entrants are Zig/Rust**: source=absence across searches, confidence=low, type=inference
- **Field-level: every major harness converged on cloud/remote continuation + worktree isolation + N-agent TUIs + voice steering**: source=per-harness sections above, confidence=medium, type=inference

## Contradictions

- **Gemini CLI maintenance status**: Google's transition post + repo banner say consumer replacement (June 18), yet tracker shows v0.64.0 released Oct 13 with 319 updates ( https://www.havoptic.com/tools/gemini-cli , secondary). Both can be true (enterprise-licensed continuation), but primary confirmation of the OSS repo's cadence was not obtained.
- **Crush language**: two secondaries say Go (cleverhack, aimagicx) and the repo has a root `go.mod` — resolved as Go, but note Crush *originated* as an opencode (TypeScript) fork; a rewrite evidently occurred; no dated primary note of the rewrite captured.
- **"Claude Mods" naming**: primary changelog calls the surface "mods" without a launch banner in the slice read; only secondary sources (AgentGid) brand it "Claude Mods" as an October feature. Launch timing unpinned.
- **dsh v0.2.0 release dates**: rc.2 referenced but its own date not captured (releases page truncated); v0.2.1 series well-dated (Oct 16–17).
- **A2A/AAIF transfer dates**: Aug 17 (announcement) vs Aug 20 (transfer) across secondaries — immaterial but noted.

## Coverage

- **Deep**: Claude Code (full Oct changelog slice 2.1.289–2.1.302 + platform release notes), Codex CLI (0.163.0 full notes + DevDay), dsh (v0.2.1-rc.2 full bilingual notes), MCP spec (2026-07-28 full changelog), Amp (dated chronicle Jun–Oct).
- **Medium**: opencode (headline + 4 changelog versions; v2 deep-dive owned by sibling researcher), Agent Skills, A2A, Gemini/Antigravity (primary transition docs; no Antigravity CLI feature deep-dive), GitHub Copilot (blogs + changelog snippets).
- **Shallow (positioning-only, dated deltas thin)**: Crush (no dated release page), Zed (release-notes snippets), Cursor (docs/changelog snippets), OpenHands (blog + secondaries), pi (secondaries only — repo not fetched), Muse Code / fx / Herdr / Ante (single secondary each), Jev (ecosystem pages). Why: breadth-first mandate + packet budget; none materially affects the top-level shifts.
- **Not covered**: Kimi Code / Qwen Code internals, Warp post-opensource trajectory, OpenClaw, Devin, Windsurf, Cline, Aider velocity (mentioned as slowed: https://www.bytereviewer.com/open-source-coding-agent/ , secondary).

## Open questions

1. Claude Code **Mods launch version/date** and public API docs URL (primary) — needed before citing Mods as a stable compat target (relevant to dsh's compat layer).
2. **Antigravity CLI** feature surface since June 18 (primary docs fetch) — is it harness-parity with Gemini CLI or a thinner wedge?
3. **Gemini CLI OSS repo** commit/release cadence post-June (primary repo/releases fetch) to resolve the contradiction.
4. **MCP 2025-11-25 changelog** contents (skipped this pass; would complete the ladder for our MCP-host upgrade path) + MCP Events proposed-spec text.
5. **dsh v0.2.0 series** release notes (what shipped between 0.1.5 and 0.2.1) and whether the append-only session-log kernel survived 0.2.x unchanged (repo diff vs our frozen study).
6. **Crush** dated deltas since Aug (releases/CHANGELOG fetch) + whether "Charm Hyper" (official provider) changed Crush's positioning.
7. **Ante** (offline single-binary agent) — verify existence/URL/date from the Show HN lead.
8. **MCP Registry** GA status + auth (Client ID Metadata Documents) adoption breadth — no dated primary captured this pass.
9. Whether any **open agent-memory or checkpoint format** effort exists beyond vendor surfaces (Claude Topics, Zed checkpoints) — nothing found; possibly genuinely absent.
10. **Jev** as a cross-harness model-router seam — primary docs (jev.page) unread.

## Promotion targets (if this becomes repo guidance)

- `docs/ai/` MCP-host notes and the native-engine context-management notes would be the live docs to update **in a follow-up slice** if any pattern here is adopted (esp. MCP 2026-07-28 statelessness/MRTR, idle-cache-aware compaction, sandbox pre-verification). This packet is reference material, not active policy.
