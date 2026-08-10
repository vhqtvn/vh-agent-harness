# hermes-agent — Tools & Capability Surface (source packet)
**Source:** `refs/hermes-agent/` @ HEAD `005421d888a40865cc61d143ff77efd87a037a1e` on `main` (Nous Research, MIT, v0.20.0). Gitignored reference transport in this repo; NOT this repo's own code.
**Method:** files read in BODY off local disk; giant files sampled at structurally important sections, not dumped whole.
**Status:** NEUTRAL study. Borrow/reject verdicts characterize fit vs vh-agent-harness discipline (static-Go, small-boundary, token-stable, overlay-based, narrow-waist); they are NOT commands to change this repo.

> See `hermes-agent-internals.md` for the consolidated study and the cross-check corrections.

## §5 Tools & capability surface (slice 1).
- Registry singleton `tools/registry.py:911`; tools self-register at import; `discover_builtin_tools()` (`registry.py:67-118`) AST-scans `tools/*.py` for top-level `registry.register()`, mtime+size-cached. `handle_function_call` (`model_tools.py:1123-1540`) single dispatcher: coerce→bridge-route→middleware→pre-hook→edit-approval→`registry.dispatch()`→post-hook→transform→sanitize-error. `check_fn` TTL cache (30s) + failure-grace (60s). (source=refs/hermes-agent/tools/registry.py:911, confidence=high, type=fact)
- **82 statically-registered tools across 30 toolsets** + dynamic MCP tools (3 call-sites in `mcp_tool.py`, unbounded) + plugin tools. Categories: browser(10+2cdp), file(4), terminal(2), web(2), x_search, vision(2), video, image_gen, video_gen(4 BFL+3 xai), tts, todo, memory, session_search, skills(3), clarify, execute_code, delegate_task, cronjob, homeassistant(4), kanban(12), computer_use, desktop_ui(6), discord(2), feishu(5), yuanbao(5). (source=refs/hermes-agent/tools/registry.py, confidence=high, type=fact)
- `toolsets.py` (~45 entries): capability + scenario bundles (debugging/safe/coding) + ~24 platform bundles. `resolve_toolset()` (`:745-824`) recursive w/ cycle detection; `"all"/"*"`=union. `bundle_non_core_tools()` (`:717-742`) bundle-safe disable. (source=refs/hermes-agent/tools/toolsets.py:745, confidence=high, type=fact)
- `toolset_distributions.py` = RL/datagen sampling profiles (NOT runtime routing); 16 distributions. (source=refs/hermes-agent/tools/toolset_distributions.py, confidence=high, type=fact)
- Environments `tools/environments/base.py:553` `BaseEnvironment` ABC (subclasses implement `_run_bash()`+`cleanup()`; base provides `execute()` w/ snapshot/CWD/interrupt/bounded capture). **7 env_type backends, 8 classes** (Modal direct+managed): local, docker, singularity, modal, modal(managed), daytona, vercel_sandbox, ssh. Lazy-import daytona/vercel. (source=refs/hermes-agent/tools/environments/base.py:553, confidence=high, type=fact)
- Delegation `tools/delegate_tool.py:4325`: fresh AIAgent children; parent-toolsets∩request minus `DELEGATE_BLOCKED_TOOLS` (`:49-57`: delegate_task,clarify,memory,send_message,cronjob); own task_id+credential lease; roles leaf/orchestrator (orchestrator gated by `max_spawn_depth` default 2); ThreadPoolExecutor w/ heartbeat; ContextVar isolation (`agent/delegation_context.py`); env-scrub of 7 `HERMES_KANBAN_*` keys across fork. (source=refs/hermes-agent/tools/delegate_tool.py:4325, confidence=high, type=fact)
- execute_code sandbox: local UDS / remote file-RPC; only 7 tools callable (`web_search,web_extract,read_file,write_file,search_files,patch,terminal`); dynamic per-session schema; resource caps (300s/50calls/50KB). (source=refs/hermes-agent/tools/delegate_tool.py, confidence=high, type=fact)

## §10 Safety gates.
- **Three-tier dangerous-command detection** `tools/approval.py`: `detect_hardline_command` (`:520`, unbypassable even in YOLO) > `_match_user_deny_rule` (`:542`, config-driven, unbypassable) > `detect_dangerous_command` (`:2179`, bypassable under YOLO). Heavy anti-obfuscation: `_normalize_command_for_detection` (`:1003`), `_deobfuscate_shell_word_for_detection` (`:1884`), `_command_detection_variants` (`:2098`). (source=refs/hermes-agent/tools/approval.py:520, confidence=high, type=fact)
- ESTOP `agent/estop.py` (167 lines): sentinel file `$HERMES_HOME/ESTOP`; `is_engaged()` = single `os.stat` (`:59`); fail-safe (corrupt/empty still engaged); `check_paused` per-component log-once (`:135`). **Pauses NEW work only, never kills in-flight** (`:12`). Call sites: cron scheduler tick, kanban dispatcher, gateway new-turn. (source=refs/hermes-agent/agent/estop.py:12, confidence=high, type=fact)
- ACP edit approval `acp_adapter/edit_approval.py` (338): ContextVar `_EDIT_APPROVAL_REQUESTER` (`:38`); `should_auto_approve_edit` (`:200`): sensitive paths (`.env*`, `id_rsa`, `.git`, `.ssh`) always ask. ACP permissions `acp_adapter/permissions.py` (timeout ≠ deny). (source=refs/hermes-agent/acp_adapter/edit_approval.py:200, confidence=high, type=fact)
- `agent/file_safety.py` (693): HARD write-denied + HARD read-blocked + SOFT cross-profile/sandbox-escape warnings. (source=refs/hermes-agent/agent/file_safety.py, confidence=high, type=fact)

## Verification
| Claim | Verifying command/read | Verified |
|-------|------------------------|----------|
| Tool registry has 82 static tools | AST scan code read pass | yes |
| Environments have 7 backends/8 classes | Structural code read pass | yes |
| ESTOP pauses new work only | Structural code read pass | yes |
| Three-tier command detection | Structural code read pass | yes |

## Contradictions
None detected.

## Could-not-verify flags
- Giant files were structurally sampled; exhaustive line-by-line verification of all edge-case conditionals was not performed. (confidence=medium, type=inference)