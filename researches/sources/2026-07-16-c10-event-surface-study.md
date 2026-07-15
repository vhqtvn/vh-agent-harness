# C10 — Signal-Triggered Coordination Toasts: Event-Surface & Value Deep Study

**Date:** 2026-07-16
**Researcher:** read-only deep-study pass (researcher subagent)
**Scope:** Resolve held candidate C10 by going DEEPER on the two open sub-questions (Q1 feasibility / Q2 value) against the ACTUAL OpenCode plugin + harness hook architecture, with file:line evidence.
**Status flag:** time-sensitivity = LOW. Both surfaces are stable source code checked out under `refs/opencode/`; the harness hook layer is committed Go. Findings are durable.
**Prior ground truth (treated as starting point, NOT re-verified):** toast infra exists and is Anti-spam-compliant (transient `tui.showToast` 5s + per-session per-key dedup; toasts ≠ context injection). Source: `researches/sources/2026-07-15-candidates-deep-study-safety-vocab.md:119-156`.

---

## 1. Verdict: **GO — narrowly scoped to command-repetition; HOLD the tool-error/test-failure slices**

The prior memo held C10 on two conjunctive preconditions: (a) an unconfirmed event surface, and (b) unproven value. This study **resolves precondition (a) in the AFFIRMATIVE** — the prior memo's central blocker ("Paper 2's signal events have no event source today… C10 would require new event-surface plumbing not confirmed to exist") is **CONTRADICTED** by the actual OpenCode plugin source checked out at `refs/opencode/`. The raw signals for command-repetition (`command.executed`) and tool-output errors (`tool.execute.after`) are already emitted, already on the plugin bus, and already delivered to the repo's `coordination-hints` plugin — which currently *chooses* to early-return on them. No OpenCode-side change, no Go change, no permission-surface change, and no `opencode.jsonc` registration change is required; the work is localized to two JS files under `.opencode/`.

Precondition (b) is **partially met**: there is documented evidence the command-repetition failure mode is real and is *structurally invisible* to the current path/content triggers (it produces no file diff), but it is *partially pre-empted* by existing rules (`AGENTS.md` command hygiene, shell-guard denials, committer consecutive-failure caps). Therefore the warranted scope is the **single highest-value, lowest-noise slice** — command-repetition toasts derived from `command.executed` — and the noisier tool-output-error/test-failure slices remain **held** pending an observed incident, because heuristic output parsing on the very-high-frequency `tool.execute.after` hook carries a real signal-to-noise / toast-spam risk that the existing per-session dedup mitigates but does not eliminate.

---

## 2. Q1 — FEASIBILITY: the event-surface gap is closeable, and the change is SMALL/LOCALIZED

### 2.1 Two distinct hook/event surfaces exist; do not conflate them

There are **two unrelated "hook" surfaces** in this tree. C10 lives entirely in the second:

1. **Harness lifecycle hooks** (`internal/hooks/dispatcher.go`) — the Go binary's OWN fixed lifecycle points (`pre_up`/`post_up`/`pre_exec`/`post_exec`/`on_first_install`/`on_update`/`pre_down`/`post_down`/`on_uninstall`) for the `vh-agent-harness up/down/exec/install/update/uninstall` verbs. Fire project-owned shell leaves under `.vh-agent-harness/run-shape.yml`. Gate-checked through the same `permission.Hook` as `exec`. **(`dispatcher.go:33-35`, `:183-232`)** — this is NOT the OpenCode plugin event bus and has no tool/test/command signals.
2. **OpenCode plugin hooks + events** (the surface C10 needs) — JS plugins in `.opencode/plugins/` return a hooks object; OpenCode invokes named hooks inline and delivers bus events to an `event` handler.

### 2.2 The full plugin event vocabulary emitted today (authoritative)

From the OpenCode plugin docs (`refs/opencode/packages/web/src/content/docs/plugins.mdx:142-208`) and confirmed against the generated SDK types (`refs/opencode/packages/sdk/js/src/gen/types.gen.ts`), plugins can subscribe to:

- **Command:** `command.executed`
- **File:** `file.edited`, `file.watcher.updated`
- **Installation:** `installation.updated`
- **LSP:** `lsp.client.diagnostics`, `lsp.updated`
- **Message:** `message.part.removed`, `message.part.updated`, `message.removed`, `message.updated`
- **Permission:** `permission.asked`, `permission.replied`
- **Server:** `server.connected`
- **Session:** `session.created`, `session.compacted`, `session.deleted`, `session.diff`, `session.error`, `session.idle`, `session.status`, `session.updated`
- **Todo:** `todo.updated`
- **Shell:** `shell.env`
- **Tool (HOOKS, not events):** `tool.execute.before`, `tool.execute.after`
- **TUI:** `tui.prompt.append`, `tui.command.execute`, `tui.toast.show`

> **Contradiction with the prior memo (flagged explicitly):** the prior memo stated "only THREE event types handled anywhere" (`session.diff`/`session.deleted`/`session.created`). That was true about what repo plugins *handle*, but it was over-narrow as a statement about what is *available*. The OpenCode plugin bus exposes a far richer vocabulary including `command.executed`, `session.error`, and the `tool.execute.after` hook. The memo's blocker claim ("C10 would require new event-surface plumbing… not confirmed to exist") is therefore **incorrect** — see §2.4.

### 2.3 Mapping C10's named signals to what actually exists

| Paper-2 / candidate signal | Native named event? | Closest available surface | Payload | Verdict |
|---|---|---|---|---|
| `command.repeated` | **No** native event | `command.executed` **event** | `{name, sessionID, arguments, messageID}` (`types.gen.ts:523-531`; schema `refs/opencode/packages/schema/src/v1/legacy-event.ts:8-16`) | **DERIVABLE plugin-side** by dedup-counting `arguments` per session. Clean source. |
| `tool.error` | **No** native event | `tool.execute.after` **hook** | `output: {title, output: string, metadata: any}` (`refs/opencode/packages/plugin/src/index.ts:274-281`) | **PARTIALLY derivable**: hook fires after successful tool execution carrying the output string; soft errors (non-zero exit, failing tests in stdout) are in `output.output`. **HARD tool crashes (thrown exceptions) are NOT caught** — see §2.5. No boolean `error` flag; detection is heuristic. |
| `test.failure` | **No** native event/hook | `tool.execute.after` (a test run is a `bash`/`exec` tool call) | same as above | **DERIVABLE only by parsing** `output.output` for test-runner failure patterns. Fuzziest of the three; highest spam risk. |
| (context) `session.error` | **Yes** native | `session.error` event | `{sessionID?, error?}` where error ∈ {ProviderAuth, Unknown, MessageOutputLength, MessageAborted, Api} (`types.gen.ts:591-597`) | Provider/session-level, NOT tool-level. Out of C10's intended scope. |

### 2.4 The signals reach the repo's plugin today — the plugin just ignores them

- **Emission confirmed:**
  - `command.executed` is published at `refs/opencode/packages/opencode/src/session/prompt.ts:1474` (`yield* events.publish(Command.Event.Executed, {...})`), aliased to the legacy schema at `refs/opencode/packages/opencode/src/command/index.ts:19`.
- **Dispatch to plugins confirmed:** OpenCode delivers **every** GlobalBus event to each plugin's `event` handler at `refs/opencode/packages/opencode/src/plugin/index.ts:255`:
  `void hook["event"]?.({ event: { id: event.id, type: event.type, properties: event.data } as any })`
- **The repo's plugin already receives these and discards them:** `.opencode/plugins/coordination-hints.js:28` does `if (event.type !== "session.diff") return;` — so `command.executed` and `session.error` arrive and are silently dropped. Adding a branch is a one-line gate change, not new plumbing.

### 2.5 The one real semantic limit (for the tool-error slice only)

`tool.execute.after` is triggered **inside** the tool's `execute` Effect generator (`refs/opencode/packages/opencode/src/session/tools.ts:121-125`), *after* `yield* item.execute(args, ctx)` (`:111`). If `item.execute` **throws** (yields a failure), the generator short-circuits and `tool.execute.after` is **never reached**. Consequence: a genuinely crashed tool is invisible to this hook. In agent practice, however, most "tool errors" are **soft** — the tool executes successfully and returns error *output* (non-zero exit code in `bash` output, failing-test summary, parse error). Those soft errors DO flow through `tool.execute.after` and live in `output.output`. So tool-error detection via this hook is feasible for the common case but **not deterministic** and requires heuristic parsing.

### 2.6 Change classification: SMALL and LOCALIZED (not architectural)

A C10 implementation touches **only** `.opencode/` JS — no OpenCode fork, no Go, no new event category, no new dispatch path, no new permission surface:

| File | Change |
|---|---|
| `.opencode/plugins/coordination-hints.js` | Add an `event` branch for `command.executed` (track per-session command history, emit repetition toast at threshold). Optionally add a `tool.execute.after` hook (held slice). Reuse existing `shownHintsBySession` dedup. |
| `.opencode/scripts/coordination-hints-lib.js` | Add predicate functions for the new signals (e.g. `buildRepetitionHint(commands, seen)`). |
| tests (e.g. `.opencode/scripts/verify-coordination-hints.js` pattern) | Cover the new predicate + dedup behavior. |

**Registration:** `opencode.jsonc` has **no** plugin entry (verified: grep for `coordination-hints|plugins|plugin` in `opencode.jsonc` returns nothing). Plugins auto-load from `.opencode/plugins/` at startup (`plugins.mdx:20-25,56-61`). Adding hooks *inside* the existing plugin module requires **zero** config change.

---

## 3. Q2 — VALUE: one real, structurally-invisible pattern; partially pre-empted by rules

### 3.1 The failure mode signal-triggers catch that path-triggers structurally CANNOT

The current 4 triggers all fire on `session.diff` and key off **touched file paths / file content** (`coordination-hints-lib.js:124-199`: `backlog-cleanup-reminder`, `coordination-surface-reminder`, `cross-boundary-slice-warning`, `large-file-warning`). They are blind to any failure mode that **produces no file diff** — i.e. a failure in the *command-execution trajectory*. The concrete catchable pattern:

> An agent repeats a permission-denied or failing command SHAPE across turns (e.g. the same unparseable heredoc, the same `&&`-chain with a non-allowlisted verb, the same failing test invocation) without ever editing a file. Path/content triggers on `session.diff` cannot see this because no path is touched; the signal lives entirely in the `command.executed` / `tool.execute.after` stream.

### 3.2 EVIDENCE the pattern actually occurs (documented, not manufactured)

- **`AGENTS.md:145`** — the command-hygiene rules exist *because* of this pattern: *"heredoc-in-braces + redirection tripped the matcher and caused **repeated failed-attempt stalls**."* The entire `AGENTS.md:137-166` "Command hygiene to avoid permission prompts" section is a rule-based mitigation for a repeated-command-failure pattern. (fact, high)
- **`.opencode/agents/committer.md:133`** — *"If 3 consecutive commit-reviewer rejections occur for the same change scope, escalate to the operator. **Do not retry indefinitely**."* A codified guard against repeated failure. (fact, high)
- **`.opencode/agents/commit-reviewer.md:314-338`** — retry-exhaustion handling for repeated empty leaf outputs (retry ONCE, then synthetic BLOCK). (fact, high)
- **`.opencode/skills/skill-creator/references/workflows.md:35`** — the skill-creator workflow explicitly treats *"Find archived sessions, prompts, or **repeated commands** for the target workflow"* as a distillation signal. The concept of "repeated commands" as a meaningful signal is already a named idea in the repo. (fact, high)

### 3.3 The honest caveat: value is narrow and partially pre-empted

The pattern is real and structurally invisible to path-triggers, **but it is already mitigated by rules**: `AGENTS.md` tells agents the sanctioned command forms; shell-guard denies the bad forms at the gate; the committer caps consecutive failures. A signal-triggered toast would be a **second nudge layer** for agents that ignore the rules — useful for the failure cases that still slip through, but not uncovering an unaddressed gap. There is **no logged incident** in this study of a specific case where a path-trigger *miss* caused harm; the evidence is *documented pattern + structural argument*, not an observed miss. (inference, medium)

### 3.4 Where value is purely speculative (do not implement)

- **Broad tool-output error parsing** (`tool.error` heuristic over all `tool.execute.after` output): `tool.execute.after` fires on every tool call — very high frequency. Heuristic error-string matching on arbitrary tool output is fuzzy and the dominant risk is toast-spam collapsing toward the Anti-spam line. No observed incident justifies it. **Speculative — hold.**
- **`test.failure` detection**: same high-frequency + fuzzy-parsing problem, with the added cost of per-test-runner output patterns. **Speculative — hold.**

---

## 4. Minimal implementation shape (for the GO slice: command-repetition only)

**Scope:** ONE new predicate keyed on `command.executed`.

1. **`.opencode/plugins/coordination-hints.js`** — extend the `event` handler:
   - keep the existing `session.deleted` (`:24`) and `session.diff` (`:28`) branches;
   - add `else if (event.type === "command.executed")` → feed `event.properties.arguments` into a new per-session command-history Map (`sessionID → Map<normalizedArgs, count>`), reuse the existing `shownHintsBySession` Set so the toast fires **once per session per repeated-command key** (preserves Anti-spam).
2. **`.opencode/scripts/coordination-hints-lib.js`** — add `buildRepetitionHint({ arguments, history })` returning a `{key, title, variant:"warning", message}` when a normalized command-shape count crosses a threshold (e.g. ≥3). Normalize by stripping volatile tokens (paths, quoted payloads) so `pytest tests/unit/foo.py` and `pytest tests/unit/bar.py` collapse to one shape — this is the hard implementation detail and the main tuning risk.
3. **Tests** — extend the `verify-coordination-hints.js` harness with: (a) under-threshold → no toast; (b) threshold → one toast; (c) same key twice → deduped; (d) Anti-spam preserved (≤1 toast/key/session).

**Validation plan:**
- Unit: the 4 cases above against the pure predicate in `coordination-hints-lib.js` (no OpenCode runtime needed — same pattern as the existing verifier).
- Live (manual, optional): run a session that repeats a sanctioned-form command 3× and confirm exactly one transient toast; confirm no toast for 3 distinct commands.
- Anti-spam audit: assert the per-session dedup Set size stays bounded and a toast never forces context into the action agent (toasts remain `tui.showToast`, never prompt injection — invariant preserved).

**Risks:**
- **Normalization tuning** (medium): over-normalization collapses distinct commands into false "repeats"; under-normalization never fires. Mitigation: start strict (high threshold ≥3, aggressive normalization) and relax only on observed misses.
- **Plugin-state growth** (low): per-session command history Map must be cleared on `session.deleted` (mirror `:24-27`) to bound memory.
- **Scope creep** (process): resist bundling the tool-error/test-failure slices — they are held for cause (§3.4).

**For the HELD slices (tool-error / test-failure):** unblock condition = a logged incident where an agent repeated a failing command/test across turns AND no path-trigger fired AND the existing rules did not catch it. Until then, do not implement.

---

## 5. Contradictions with the prior deep-study memo

- **Event-surface blocker (OVERTURNED):** memo `2026-07-15-candidates-deep-study-safety-vocab.md:138-143,150` claims "No existing plugin handles tool-error/test-failure/command-repeated events… only THREE event types handled anywhere" and "C10 would require new event-surface plumbing (opencode emitting tool.error/test.failure/command.repeated events to plugins) that is not confirmed to exist." This study, against the OpenCode source at `refs/opencode/`, finds the plumbing **does** exist under adjacent names: `command.executed` (event, emitted `prompt.ts:1474`, delivered `plugin/index.ts:255`) and `tool.execute.after` (hook, `plugin/src/index.ts:274-281`, triggered `tools.ts:121-125`). The memo's "not confirmed to exist" was correct *as uncertainty* but is now resolved in the affirmative. The memo's framing was right that no event is *named* `tool.error`/`test.failure`/`command.repeated` — those are derived, not native.
- **Value precondition (NUANCED, not contradicted):** the memo's "no concrete repeated-mistake pattern has been shown" (`:151`) is *partially* overturned — §3.2 documents the pattern is real and codified. But the memo's stricter bar ("missed by path-triggers") has no *observed incident*, only a structural argument. So the memo's caution stands for the noisy slices; only the command-repetition slice clears the bar.

---

## Findings

- **(finding)**: OpenCode's plugin event vocabulary is far richer than the prior memo recorded; `command.executed`, `session.error`, and the `tool.execute.after` hook are all available to plugins. source=`refs/opencode/packages/web/src/content/docs/plugins.mdx:142-208`, confidence=high, type=fact
- **(finding)**: `command.executed` is emitted at `prompt.ts:1474` and delivered to every plugin's `event` handler at `plugin/index.ts:255`; the repo's coordination-hints plugin receives it today and early-returns on it (`coordination-hints.js:28`). source=`refs/opencode/packages/opencode/src/session/prompt.ts:1474`/`refs/opencode/packages/opencode/src/plugin/index.ts:255`/`.opencode/plugins/coordination-hints.js:28`, confidence=high, type=fact
- **(finding)**: A C10 implementation requires NO OpenCode-side change, NO Go change, NO new permission surface, and NO `opencode.jsonc` registration (plugins auto-load from `.opencode/plugins/`); it is localized to `coordination-hints.js` + `coordination-hints-lib.js` + tests. source=`opencode.jsonc`(grep empty)/`plugins.mdx:20-25`, confidence=high, type=fact
- **(finding)**: `tool.execute.after` fires only after *successful* tool execution (`tools.ts:111-125`); hard tool crashes (thrown exceptions) short-circuit past it, so tool-error detection via this hook is feasible only for soft errors carried in `output.output` and is inherently heuristic. source=`refs/opencode/packages/opencode/src/session/tools.ts:106-125`/`refs/opencode/packages/plugin/src/index.ts:274-281`, confidence=high, type=fact
- **(finding)**: There is documented evidence the repeated-command-failure pattern is real and codified in repo rules (`AGENTS.md:145`, `committer.md:133`, `commit-reviewer.md:314-338`, `skill-creator/references/workflows.md:35`). source=`AGENTS.md:145`/`.opencode/agents/committer.md:133`, confidence=high, type=fact
- **(finding)**: The catchable failure mode (command-trajectory failures with no file diff) is structurally invisible to the current `session.diff` path/content triggers, which is a genuine capability gap — but it is already partially mitigated by existing rules, so the toast is a second nudge layer, not a missing control. source=`coordination-hints-lib.js:124-199`(path-only)/`AGENTS.md:137-166`(rule mitigation), confidence=medium, type=inference
- **(finding)**: `internal/hooks/dispatcher.go` is the harness's OWN lifecycle-hook surface for `vh-agent-harness` verbs, unrelated to the OpenCode plugin bus; conflating the two would mis-scope C10. source=`internal/hooks/dispatcher.go:33-35,183-232`, confidence=high, type=fact

## Contradictions

- **OVERTURNED**: prior memo's event-surface blocker ("no event source… not confirmed to exist") — see §5. The plumbing exists under adjacent names; only the *named* events `tool.error`/`test.failure`/`command.repeated` are absent (they are derived, not native).
- **NUANCED**: prior memo's "no concrete repeated-mistake pattern shown" — the pattern IS documented (§3.2), but no observed path-trigger *miss* incident exists; the noisy slices remain held on the memo's original caution.

## Confidence

- Q1 (feasibility, event surface closeable, change localized): **high** — grounded in committed OpenCode + harness source with emission/dispatch sites cited.
- Q2 value for the command-repetition slice: **medium** — documented pattern + structural argument; no observed incident of a path-trigger miss.
- Q2 value for tool-error / test-failure slices: **low** — speculative, high signal-to-noise risk; held.

## Memo path

`researches/sources/2026-07-16-c10-event-surface-study.md` (this file; may fold into the C10 card).

## Recommended next specialist / command

Hand to the **coordinator**: (1) update the C10 card from `held/blocked` → `go (scoped: command-repetition only)` with the unblock conditions for the held slices recorded; (2) if adopted, route the implementation slice to `build` via `/write-task` + `/resume-task` targeting the two `.opencode/` JS files + tests, NOT a debate/planner pass (the shape is now concrete and low-uncertainty).
