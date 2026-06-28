# 2026-06-28 — Make vh-agent-harness agent-operable in consumer repos (overlay extension UX)

> **Decision checkpoint.** The supporting research for this decision exists only
> in ephemeral session results, so this file is intentionally self-contained:
> every claim below is anchored to a concrete, verifiable source path so the
> decision can be reconstructed without the original session. This is a decision
> record, not a closeout — implementation is tracked in
> `docs/planning/backlog.md` as `P1-OVERLAY-003` (Slice 1) and `P1-OVERLAY-004`
> (Slice 2). Maps to roadmap milestone "Agent-operable in consumer repos" and
> "Overlay scaffolding" (`docs/planning/roadmap.md`, lines 27–31).

## Title

Make vh-agent-harness agent-operable in consumer repos (overlay extension UX).

## Problem — three observed failure modes

A consumer-repo coding agent asked to "add a subagent that releases versions"
failed three distinct ways. These are the failure modes this decision must
permanently close:

- **FM1 (edits the generated tree).** The agent used OpenCode's built-in
  `customize-opencode` skill to edit the generated `.opencode/` and
  `opencode.jsonc`. Those files are regenerated on every `vh-agent-harness
  update`, so the edits silently vanish. Nothing in the agent's always-on
  context told it `.opencode/` is generated.
- **FM2 (cannot find what an overlay is).** Told to "use an overlay," the agent
  could not discover what an overlay *is*, where overlay packs live, or how they
  are selected. The only file in its always-on context was silent on overlays.
- **FM3 (escapes only via an external URL).** The agent only recovered by
  following an external URL to `README.agent.md` — i.e. it needed out-of-repo
  documentation that is never shipped into the consumer and is not part of
  OpenCode's `instructions[]`.

## Root cause (verified against source)

The **only** file in a consumer agent's always-on context is the composed
`AGENTS.md`. It is built by concatenating `templates/core/.vh-agent-harness/
AGENTS.core.md` + the project's `AGENTS.mission.md`
(`internal/cli/seam.go`, `composeAgentsMd`, ~lines 308–332).

That core file is **silent on the agent-operable surface**. Verified by grep
on `templates/core/.vh-agent-harness/AGENTS.core.md`: the token "overlay"
appears only inside the **term-contract preamble** (defining the
`AGENTS.mission.md` *overlay doc* concept, lines ~7/34/41) — never as the
extension mechanism. There are **zero operational matches** for
`customize-opencode`, `vh-agent-harness guide`, `vh-agent-harness example`,
`.vh-agent-harness/overlays/`, or a `/harness` command. (Confirmed: 0 matches
across `templates/core/` for the customize-opencode warning and the overlay
extension recipe.)

The correct knowledge exists today, but **only in `README.agent.md`**, which:

- is **not shipped into consumer repos** (it lives in the harness source repo),
  and
- is **not in OpenCode's `instructions[]`** —
  `templates/core/opencode.jsonc.tmpl` keeps `instructions[]` minimal and
  config-driven ("MANAGED: instructions[] is config-driven … keep minimal;
  project appends its own docs", lines ~6–13), so `README.agent.md` never
  reaches the consumer agent's context.

Two secondary gaps amplify the silence:

1. The binary's `guide` command **already prints the right overlay recipe for
   installed repos** (`internal/cli/guide.go`, ~lines 133–137: "create an
   overlay pack at `.vh-agent-harness/overlays/<name>/` (agents/, commands/,
   skills/, opencode-append.jsonc) and list `<name>` under `overlays:` … then
   `vh-agent-harness update`"), but **nothing in always-on context invites an
   agent to run it**. The recipe fires only after the agent already knows to
   ask.
2. The repo's own `skill-creator` skill **actively misdirects** agents into the
   generated `.opencode/skills/` tree
   (`templates/core/.opencode/skills/skill-creator/SKILL.md`, lines 13, 30, 60:
   "repo-local skills under `.opencode/skills/<name>/SKILL.md`", "Initialize or
   update the skill under `.opencode/skills/`"). In a harness-managed repo that
   path is generated and will be overwritten.

**Conclusion:** the always-on context (`AGENTS.core.md`) is the only channel
that fires *before* an agent thinks to run a command, so the keystone fix must
live there. Fixing `guide`/`example`/`README.agent.md` alone cannot close the
gap, because the agent never gets the prompt to consult them.

## Resolved decisions (operator-confirmed, 2026-06-28)

1. **`customize-opencode` → reason-gated soft warning, not a hard block.**
   Wording intent: do **not** use OpenCode's built-in `customize-opencode`
   skill to change the harness — use an overlay. Only invoke
   `customize-opencode` when you have a specific reason unrelated to the
   generated tree. (A shell-guard-style hard deny on writes to `opencode.jsonc`
   outside the merge path is explicitly **not** in v1 — see Out-of-scope.)
2. **`vh-agent-harness overlay new` scaffolder (fix C) is scoped for v1 but
   deferred to Slice 2 (P1-OVERLAY-004) for risk isolation**, not dropped to P2.
   This collapses the gap between "know overlays exist" and "produce one".
3. **`example` pack skeleton is a flat listing under `vh-agent-harness
   example`**, named `_pack-skeleton` (leading underscore sorts it to the top
   and signals "reference pack") with a one-line doc. It is **not** a new
   `example overlay` subcommand.

## Ranked v1 fix set (all five ship in v1)

### A — Keystone: edit `templates/core/.vh-agent-harness/AGENTS.core.md`

Add an **"Extending the harness"** section (≤ ~25 lines; domain-free; tokens
only). It must cover:

- `.opencode/` and `opencode.jsonc` are **generated** — edits vanish on
  `vh-agent-harness update`.
- The **reason-gated `customize-opencode` warning** (decision 1).
- **Overlays are the extension unit**: a pack at
  `.vh-agent-harness/overlays/<pack>/` carrying `agents/` / `commands/` /
  `skills/` + `opencode-append.jsonc` (optional `permission-pack.jsonc`).
- Select the pack under `overlays:` in `vh-harness-profile.yml`, then
  `vh-agent-harness update`.
- When unsure, run `vh-agent-harness guide`; run `/harness` for the full
  recipe.

Also add `harness` to the command enumeration in `AGENTS.core.md` (~line 218).

**Closes FM1 + FM2 + FM3** — it is the only channel that fires before an agent
thinks to run a command. This is why A is the keystone and ships first.

### B — Add `templates/core/.opencode/commands/harness.md`

A new `/harness` command: the golden path + overlay anatomy + verify steps +
anti-patterns (do not edit `.opencode/`; do not `customize-opencode` for
harness changes). **New command surface → `README.agent.md` updates in the same
change.**

### C — Add `internal/cli/overlay_new.go` (+ register in `internal/cli/root.go` + embedded domain-free skeleton)

`vh-agent-harness overlay new <name> [--agent <a> | --command <c> | --skill
<s>] [--dry-run]`.

**Risk (highest-risk slice, built last and isolated):** this command **appends
`<name>` to `overlays:` in `vh-harness-profile.yml`**, whose ownership class is
`platform_armed`. It **MUST go through the schema/reconcile path, NOT a naive
text edit.** Requirements: strict `--dry-run` (prints file manifest + profile
diff, writes nothing); validate name collisions and partial packs; **unit tests
required. New command surface → `README.agent.md` updates in the same change.**

### D — Edit `templates/core/.opencode/skills/skill-creator/SKILL.md` + `scripts/init_skill.py`

Add a **"Where skills live"** decision: in a harness-managed repo, new skills
go in `.vh-agent-harness/overlays/<pack>/skills/<name>/`, **not** the generated
`.opencode/skills/`. Keep the `.opencode/skills/` path only for editing
`templates/core/`. `init_skill.py` accepts an overlay target path and warns if
writing under the generated `.opencode/skills/`.

### E — Edit `internal/cli/guide.go` + add `templates/examples/.vh-agent-harness/overlays/_pack-skeleton/`

- Tighten the installed-phase overlay step in `guide.go` to name
  `agents/<name>.md` + `opencode-append.jsonc` + optional
  `permission-pack.jsonc`; add a footer that **always** advertises `/harness`
  and `example`.
- Add `templates/examples/.vh-agent-harness/overlays/_pack-skeleton/` with
  `agents/.keep`, `opencode-append.jsonc`, `permission-pack.jsonc`,
  `callable-graph-snippet.md` so `vh-agent-harness example` can print a
  copy-paste skeleton (decision 3).

## Golden path — "add a subagent via overlay" (the task that originally failed)

1. `vh-agent-harness guide`
2. Create `.vh-agent-harness/overlays/<pack>/`
3. Add `agents/<name>.md` — frontmatter `description` + `mode: subagent` + a
   prompt body. Tokens resolve at render time.
4. Add `opencode-append.jsonc` with the agent block **and** `task`
   `allow-injections` into the core orchestrators (`build`, `coordination`,
   `project-coordinator`).
5. (Optional) `permission-pack.jsonc` — auto-resolves the roster and
   `delegateFrom`.
6. (Optional) `callable-graph-snippet.md`.
7. (Optional) `commands/<name>.md`.
8. List the pack under `overlays:` in `vh-harness-profile.yml`.
9. `vh-agent-harness update --dry-run`, then `vh-agent-harness update`.
10. Verify with `vh-agent-harness doctor`; confirm the rendered
    `.opencode/agents/<name>.md` and the `opencode.jsonc` agent block; restart
    OpenCode.

Worked reference overlay: `docs/adoption-examples/web/`.

## Build plan — two slices

- **Slice 1 = A + B + D + E** (templates & guidance; low-risk; one build pass).
  Tracked as `P1-OVERLAY-003`, status `in_progress`.
- **Slice 2 = C** (Go CLI; highest-risk; isolated). Tracked as
  `P1-OVERLAY-004`, status `todo`/Next. **Queued behind Slice 1.**

## Out of scope / deferred to v2

- Changes to overlay merge logic (`internal/overlay/*.go`).
- An `example overlay` subcommand (the flat `_pack-skeleton` listing under
  `example` is the v1 answer — decision 3).
- Any domain literal in `templates/core/` or `templates/`.
- Stronger `customize-opencode` enforcement — e.g. a shell-guard-style rule
  denying writes to `opencode.jsonc` outside the merge path. **Noted as a
  possible future hardening; explicitly not in v1** (decision 1 keeps it a
  reason-gated soft warning).

## Cross-cutting rules (every slice must satisfy)

- `templates/core/` and `templates/` edits stay **domain-free**: only
  `{{PROJECT_NAME}}` / `{{PROJECT_SLUG}}` / `{{COORDINATOR_DIR}}` tokens.
- `go test ./...` + `gofmt` + `go vet` pass before commit.
- Fixes **B** and **C** change the command surface, so `README.agent.md` is
  updated **in the same change** (repo rule: a stale `README.agent.md` is a
  bug).
- `templates/core/` edits require a rebuild + `vh-agent-harness update` to
  regenerate `.opencode/` (the dogfood loop).
- Commit via `commit-reviewer` then `committer` (gated-commit protocol).

## Verification gates

- Slice 1: `go build && ./vh-agent-harness update`; grep the composed
  `AGENTS.md` for the "Extending the harness" section and the reason-gated
  `customize-opencode` warning; confirm `example` lists `_pack-skeleton`;
  confirm `README.agent.md` documents `/harness`.
- Slice 2: `go test ./...` green for `overlay_new`; `--dry-run` writes nothing
  and prints manifest + profile diff; `README.agent.md` documents
  `overlay new`.

## Source anchors (verified 2026-06-28)

- `internal/cli/seam.go` — `composeAgentsMd` (~308–332): composes
  `AGENTS.core.md` + `AGENTS.mission.md` → `AGENTS.md`.
- `templates/core/.vh-agent-harness/AGENTS.core.md` — silent on the extension
  surface; "overlay" appears only in the term-contract preamble; command
  enumeration (~218) has no `harness`.
- `internal/cli/guide.go` (~133–137) — already prints the overlay recipe for
  installed repos, but only when `len(st.Overlays) == 0` and with no standing
  invitation.
- `templates/core/opencode.jsonc.tmpl` (~6–13) — `instructions[]` is minimal
  and config-driven; `README.agent.md` is not injected.
- `templates/core/.opencode/skills/skill-creator/SKILL.md` (13, 30, 60) —
  misdirects new skills into the generated `.opencode/skills/`.
- `internal/cli/overlay_new.go` — **does not yet exist** (fix C creates it).
- `templates/examples/` — currently holds only `docs/` (fix E adds
  `.vh-agent-harness/overlays/_pack-skeleton/`).

## Backlog tracking

- `P1-OVERLAY-001` (research) → `done` 2026-06-28; research resolved into this
  checkpoint.
- `P1-OVERLAY-002` (coarse "implement A/B/C") → `cancelled` (superseded);
  decomposed into the two slices below.
- `P1-OVERLAY-003` — Slice 1 (A+B+D+E), `done` (commit `4c0b6f0`), owner `build`.
- `P1-OVERLAY-004` — Slice 2 (fix C: `overlay new`), `done` (commit `450c123`), owner `build`.

## Slice 2 completion (2026-06-28)

Fix C (`overlay new`) landed in `450c123` (local, not pushed), completing the
two-slice agent-operability effort. Slice 1 (A+B+D+E) landed earlier in
`4c0b6f0`. With both slices in, all three failure modes (FM1/FM2/FM3) are
closed and the full golden path is reachable from a cold consumer agent.

### What `overlay new` does

`vh-agent-harness overlay new <name> [--agent <a> | --command <c> | --skill <s>]
[--dry-run] [--target <dir>]` (source: `internal/cli/overlay_new.go`, registered
in `internal/cli/root.go`):

- Scaffolds an overlay pack at `.vh-agent-harness/overlays/<name>/` with the
  requested unit skeletons (`agents/<a>.md`, `commands/<c>.md`,
  `skills/<s>/SKILL.md` from the embedded `templates/overlay-skeleton/*.md`)
  plus the always-on `opencode-append.jsonc` (active agent wiring when
  `--agent` is given, no-op shell otherwise), `permission-pack.jsonc`, and
  `callable-graph-snippet.md`.
- Appends `<name>` to `overlays:` in `vh-harness-profile.yml`. This file is
  `platform_armed`, so the append goes through
  `schema.HarnessProfile{}.AppendOverlay`
  (`internal/schema/harness_profile.go`, ~139–167) — the same
  load/validate/marshal path `update` reconciles with — **NOT** a naive text
  insert. `marshalHarnessProfile` re-serializes data only (sorts/dedups arrays,
  drops comments), so the append is structurally clean.
- `--dry-run` is strict: writes nothing, prints a file-creation manifest and a
  `vh-harness-profile.yml` before→after diff.
- Fail-closed: rejects an existing pack dir, existing files (name collision),
  invalid names (`nameRe`), and a missing `.vh-agent-harness/`.

### B-F1 — the block and the fix (the highest-risk part of fix C)

The scaffolder's `buildPermissionPack` (`internal/cli/overlay_new.go`,
~428–473) originally emitted **both** `gate: "deny"` **and**
`gateExempt: true` for the same `--agent`. That combination violates the
permission-pack contract enforced by `validateRules()` in
`templates/core/.opencode/sys-scripts/update-opencode-config.js` (~lines
924–948): a gate-exempt agent (one in `GATE_EXEMPT_AGENTS`, which
`gateExempt: true` adds the overlay agent to at ~lines 550–552) **must not
carry a `gate` key**. The first end-to-end render reached
`node .opencode/sys-scripts/update-opencode-config.js` and failed
`validateRules()` (non-zero exit) — the binary's own `update`/`doctor` did not
catch it.

**Fix:** `buildPermissionPack` omits `gate` for a gate-exempt agent. The
generated `permission-pack.jsonc` location now carries only
`wildcard`/`readonly`/`git_readonly`/`devSh`, plus `gateExempt: true`,
`task { *: deny, committer: allow }`, and `delegateFrom` the three core
orchestrators (`build`/`coordination`/`project-coordinator`). A regression
test was added (`internal/cli/overlay_new_test.go` /
`agf6_output_assertions_test.go`).

### Phase A validation outcome (fresh throwaway consumer repo)

Validated end-to-end in a throwaway consumer repo (`tmp/validate-slice2/`,
removed after validation; no commit). All checks PASS:

| # | Check | Result |
| --- | --- | --- |
| 1 | Fresh binary build + `overlay new --help` shows Slice-2 flags | PASS |
| 2 | Throwaway `install` + minimal mission + `update` composes `AGENTS.md` | PASS |
| 3 | `--dry-run` purity (writes nothing; prints 6-file manifest + profile `[]→[demo]` diff) | PASS |
| 4 | Apply creates the 6 expected files + appends `demo` to profile | PASS |
| 5 | Profile append structurally clean (valid YAML; `overlays:` list; profile enum untouched) | PASS |
| 6 | B-F1 fix: `permission-pack.jsonc` greeter location has no `gate` key | PASS |
| 7 | `update --dry-run` → 0 conflicts | PASS |
| 8 | `update` renders greeter agent/command/skill + wires `opencode.jsonc` (`greeter` block + `task.greeter:"allow"` in build/coordination/project-coordinator) | PASS |
| 9 | `doctor` (after `update`, before node script) healthy, 0 problems | PASS |
| 10 | `node .opencode/sys-scripts/update-opencode-config.js` EXIT 0 — the B-F1 critical step | PASS |
| 11 | Edge: minimal pack (no unit flags) + stderr warning + append | PASS |
| 12 | Edge: name-collision rejection (refuse, write nothing, exit 1) | PASS |
| 13 | Edge: `--dry-run` purity reaffirm (file count unchanged) | PASS |

Two pre-existing gaps were re-confirmed (NOT introduced by Slice 2; tracked
separately): (1) `install` alone does not compose `AGENTS.md` (needs mission +
`update`) — `P2-INSTALL-001`; (2) `validateRules()` runs only in the
operator-run node script, not in the Go binary (see next subsection).

### Pre-existing gap — `validateRules()` verification asymmetry

`validateRules()` (the permission-pack contract check — e.g. a gate-exempt
agent must not carry a `gate` key) lives only in the operator-run
`node .opencode/sys-scripts/update-opencode-config.js` script
(`templates/core/.opencode/sys-scripts/update-opencode-config.js`, ~lines
924–948, invoked in `main()` at ~1088). The Go binary's `update` and `doctor`
commands never invoke it. Consequence: an active overlay pack can be "valid"
per the binary (0 conflicts, doctor healthy) yet fail the permission contract
on the first real node-script run. This is exactly what surfaced B-F1.
Tracked as `P2-VERIFY-001`. A related pre-existing drift — the node script
rewrites managed `opencode.jsonc` permission blocks in ways that diverge from
the corpus render (drops the `features.backlog` line; canonicalizes an overlay
agent's permission block so `doctor` reports drift after the node script) — is
tracked as `P2-CONFIG-001`.

### Source anchors (Slice 2, verified 2026-06-28)

- `internal/cli/overlay_new.go` — the scaffolder (~558 lines);
  `buildPermissionPack` ~428–473 (omits `gate` for gate-exempt agents — the
  B-F1 fix).
- `internal/schema/harness_profile.go` — `AppendOverlay` ~139–167
  (load/validate/append/marshal; data-only serialization).
- `internal/cli/overlay_new_test.go` + `agf6_output_assertions_test.go` — unit
  + regression tests.
- `templates/overlay-skeleton/{agent,command,skill}.md` — embedded unit
  skeletons (`corpus.go` embed root).
- `templates/core/.opencode/sys-scripts/update-opencode-config.js` —
  `validateRules()` ~924–948; `GATE_EXEMPT_AGENTS` seeding ~550–552; called in
  `main()` ~1088.
- `README.agent.md` — golden-path order documents `overlay new` (~lines
  183–189).
