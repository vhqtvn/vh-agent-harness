# Native-Engine Landscape Gap — 2026-10-18
Status: SYNTHESIS for operator prioritization (not a commitment)
Sources: researches/sources/opencode-v2-study-2026-10.md, researches/sources/harness-landscape-2026-10.md

## Where the engine already differentiates (do not regress)
- Real kernel sandbox for shell (Landlock+seccomp incl. io_uring closure) — opencode v2 ships NO shell sandbox; Codex only added sandbox-integrity pre-checks. Type: fact (both packets).
- Byte-deterministic replayable sessions as a stated invariant — no surveyed harness pitches this. Type: inference from absence.
- Fail-closed approvals + policy hard-deny lattice + MCP ask-by-default — field moved toward classifier+audit permissioning (CC auto-mode w/ OTEL verdicts, Codex Guardian); our posture is stricter-by-construction.
- Single static Go binary — no new Go entrant appeared (new entrants chose Zig/Rust; fx = 6MB Zig is the closest kin). Type: fact.
- Skills: Agent Skills (SKILL.md) settled as THE cross-vendor extension layer (11 documented clients) — our guarded three-tier implementation is spec-aligned. Type: fact.
- MCP: 2026-07-28 stable spec went STATELESS (sessions/handshake removed) — our stdio JSON-RPC design is now spec-aligned. Type: fact.

## Deliberate divergences reaffirmed (document, don't chase)
- No runtime plugin platform w/ hot-reload (opencode v2's 80-plugin core; HN flags self-modification security tradeoff) — our compile-time telegraf model stands; revisit only with a concrete need.
- No embedded JS sandbox / Code Mode (opencode routes MCP through it) — run_shell + guarded tools cover our surface.
- LSP: opencode v2 REMOVED it entirely — validates never building it. Type: fact.

## Gap candidates (P9+ shortlist, operator prioritizes)
| # | Gap | Evidence | Why it matters here | Est. |
|---|---|---|---|---|
| G1 | Git worktree tools + parallel-agent isolation | CC/Codex/opencode/Amp/dsh/Cursor ALL converged on worktrees + /agents views | subagents today share the session tree; parallel repo work is the daily-driver killer feature we lack | M |
| G2 | MCP stateless-spec upgrades | 2026-07-28 spec: _meta versioning, server/discover, ttlMs/cacheScope hints, MRTR input_required | MCP interop is a parity pillar; our host predates the stable spec | S |
| G3 | Provider-reliability kit | opencode: output-limit auto-continuation, warming, zstd bodies, cache-marker compat | QoL for long real sessions on DeepSeek/Anthropic | S-M |
| G4 | Instruction-epoch sync | opencode v2: instruction sources diffed per attempt, frozen text projected chronologically | complements our compiled sys-prompt (stale-prompt detection between turns) | S |
| G5 | HTTP/WebSocket transport for the host protocol | opencode v2 multi-surface; every player has remote continuation | enables vh-solara + remote access without stdio-only coupling | M |
| G6 | Session snapshots (undo/redo/fork surface) | opencode SQLite snapshots | our log+resume+SeedTurns cover the mechanics; a surface is new | M |
| G7 | Claude Code hooks/Mods compat layer | dsh building CC Mods compat + shipping CC/Codex bundles — hook-API portability demand | unlocks ecosystem plugins/skills without adopting CC | S-M (study first) |

## Watch items (no action)
- opencode Console breach (distillation attack, provider keys of ~1.4% hosted users) — hosted-service risk, not binary; validates single-operator local-first posture.
- Voice steering (Codex/Amp/opencode) — novelty, not our surface.
- Antigravity CLI replaced Gemini CLI for consumer tiers — landscape churn; no action.

## Recommendation
After merge + daily-drive stabilizes: G1 (worktrees) and G2 (MCP spec) carry the most daily-driver value; G5 unlocks the intended external-frontend story. G4/G3 are cheap riders. G7 deserves a study card before build.
