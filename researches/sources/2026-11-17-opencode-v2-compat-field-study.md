# OpenCode v2 — Live Compatibility Field Study (harness surfaces vs v2.2.6)

> **Provenance**: this packet is the durable promotion of the completed
> study card `opencode-v2-compat-study` (executed 2026-11-07 by a build
> subagent; coordinator review accepted it as completed the same day). It
> distills the two surviving coordination reports verbatim-recorded under
> `.local/coordinator/reports/opencode-v2-compat-study/` (closeout
> 2026-11-07T19-48-06Z, review 2026-11-07T19-48-27Z; session
> `opencode-v2-compat-study`, ses_e488b697affeQ5E3DM6jJRQFAX).
> **Second-generation evidence, honestly stated**: the study's primary
> artifacts — REPORT.md, RIG.md (pinned image digest), MANIFEST.txt, and 17
> probe logs — lived under gitignored `tmp/agent-runs/opencode-v2-compat-study/`
> under the tmp-only mandate and have since been cleaned; at authoring time
> (2026-11-17) only the two reports above survive. Nothing below goes beyond
> what those reports record; where the reports are thin (e.g. the three
> live-beats-docs contradiction cases are COUNTED but not itemized — their
> details lived in REPORT.md), that thinness is preserved, not papered over.
> Companion context: `researches/sources/opencode-v2-study-2026-10.md`
> (2026-10-18 web-research landscape of v2 — the "before" picture this field
> study probed live). Durable home:
> `researches/sources/2026-11-17-opencode-v2-compat-field-study.md`.

---

## 1. Study shape (as recorded)

- **Method**: all 8 harness surfaces probed live against a pinned
  `ghcr.io/anomalyco/opencode:2.2.6` image (docker 29.9.3; image digest was
  recorded in the study's RIG.md — now cleaned with the rest of tmp/).
  17 probe logs plus a scratch manifest were captured under
  `tmp/agent-runs/opencode-v2-compat-study/`.
- **Repo impact of the study itself**: none — zero commits; git status
  showed only pre-existing untracked `internal/eslink/` (not the study's)
  and gitignored `tmp/`. Durable promotion of findings was explicitly
  deferred to task-review as an operator decision (this packet is that
  decision, executed).
- **Evidence quality at review time** (coordinator, verified on disk before
  the cleanup): every verdict cited a captured command + log excerpt; the
  honesty section listed the non-demonstrated surfaces; the contradictions
  section flagged 3 live-beats-docs cases against `refs/opencode/specs/v2`.

## 2. Verdict: compatible with changes

Config, agents, commands, skills, and permissions ride v2's v1-normalization
layer successfully; plugins and file-based custom tools are hard breaks; the
state-dir change is degraded/informational only.

| Surface | Verdict (as recorded in the reports) |
|---|---|
| **Config** | works — v1 shape normalized in-memory |
| **Agents** | works — 24/24 |
| **Commands** | works — run-mode expansion proven |
| **Skills** | works — progressive disclosure |
| **Permissions** | works — rule order + agent-overrides-global preserved |
| **Plugins** | **HARD BREAK (class L)** — 14/14 files fail to load: v1 `{id,server}` export vs v2 default `{id,effect\|setup}`; shell-guard + all safety/coordination gates dead under v2 until ported (port surface: `templates/core/.opencode/plugins/`) |
| **Custom tools** | **HARD BREAK (class M)** — `.opencode/tools/plan-state.js` not registered; file-based tool discovery gone |
| **State-dir** | degraded/informational — v2 keeps its own state in per-user SQLite; our `.opencode/state/` file IO unaffected |

The `L` / `M` letter classes are the study's own impact classification as
recorded in the reports; the reports do not itemize the remaining classes.

## 3. Honesty limits (recorded, preserved)

Not demonstrated by the study: TUI interactive surfaces, `serve` + OpenAPI
listing, permission ask-path UX, MCP, plugin hot-reload. Tool-list and
permission-transcript evidence was model-mediated, with structural anchors
in debug config/agents outputs. The review additionally recorded 3
live-beats-docs contradiction cases against `refs/opencode/specs/v2`
(count only — the itemized cases lived in the cleaned REPORT.md).

## 4. Recorded follow-ups (operator decisions at review, unchanged here)

1. Open the v2-migration implementation task — **plugin port (L) first**,
   shell-guard is the critical path (all safety/coordination gates depend
   on it).
2. Port the custom-tools surface (M) — `.opencode/tools/plan-state.js`
   needs a v2-native registration path or a replacement mechanism.
3. The card itself may retire as completed once these decisions land.

Task-Card: opencode-v2-compat-study
