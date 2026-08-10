---
type: decision
date: 2026-08-08
scope: render/apply pipeline — honor features.* flags in AGENTS.md + command composition (bug fix)
status: research-complete, decision-recorded (operator-pre-classified P1 FIX; hermes as grammar reference)
source-basis: refs/hermes-agent @ 005421d888a40865cc61d143ff77efd87a037a1e (gitignored transport), cross-checked
---

# Flag-Gated Composition Fix — Decision Memo

## Decision statement

**FIX the harness's OWN flag-gated composition bug:** `features.backlog: false`
(and other `features.*` flags) must actually suppress the corresponding rules
and command registrations in the rendered `AGENTS.md` / commands, not only in
the permission block. Today the flag is honored in exactly two narrow places
(the `opencode.jsonc` agent block and the permission `bash` block) and ignored
everywhere else, so consumers who turn a feature off still ship its full
rule + command surface. This is a **bug fix**, not a borrow — but hermes
supplies the composition grammar (bundle-safe-disable: subtract only the
non-core delta so shared/core content survives a bundle disable) that informs
the fix shape and prevents the obvious failure mode (disabling a feature
wipes shared content).

This is a recommendation the operator will decide on; the memo body does not
speak as live repo policy.

## Why this is P1 (decision context)

`features.*` is a documented, consumer-facing on/off switch in
`vh-harness-profile.yml`. A consumer who sets `features.backlog: false` is
stating "I do not use the backlog workflow." The harness silently ignores
that statement for the most visible surfaces (`AGENTS.md` rules, the
`backlog-cleanup` command registration, the coordination docs that reference
backlog) while honoring it for the permission block. The result is an agent
that is told (in `AGENTS.md`) to follow backlog rules it has no permission
to execute and no command to run — an incoherent posture. Worse, the
consumer cannot tell from the rendered output whether their flag took
effect. P1 because a feature flag that silently no-ops on its primary
surface is a correctness defect in the composition pipeline, and it has
already shipped broken to two consumers.

## Hermes finding (grammar reference, verified)

Hermes's `toolsets.py` implements the composition algebra that makes
flag-gating correct. The load-bearing idea is **bundle-safe-disable**: when a
bundle is disabled, subtract only its non-core delta so shared core tools
survive.

- `resolve_toolset` `source=refs/hermes-agent/toolsets.py:745-824` —
  recursive resolution with cycle detection (`:778-784`, visited-set shared
  across sibling includes so diamond deps resolve once); `"all"/"*"` = union
  over every toolset (`:768-776`); includes resolved recursively
  (`:820-822`).
- `bundle_non_core_tools` `source=refs/hermes-agent/toolsets.py:717-742` —
  "When a bundle name appears in `disabled_toolsets`, subtracting the whole
  bundle would strip core tools (terminal, read_file, …) shared by every
  other enabled toolset, emptying the model's tool list (#33924). This
  returns only the bundle's non-core delta … so disabling a bundle removes
  its platform tools while leaving core intact." (`:717-726`)
- Fallback for unknown/garbage names: `resolve_toolset(name) - core`
  (`:736`) — never re-introduces the core wipe.

This is the grammar lesson for the fix: disabling `features.backlog` must
remove the backlog-specific rules/commands/docs, but must NOT remove content
that backlog shares with non-backlog workflows (e.g. the general
docs/checkpoints guidance, or a command referenced by a non-backlog rule).
[from pass-1 slice study, confidence high — read directly by this researcher]

## Consumer field evidence (operator cross-check, studied 2026-08-08)

`source=operator-cross-check, studied:2026-08-08` — operator-provided; this
researcher cannot re-verify consumer repos from here.

- `features.backlog: false` yet the rendered `AGENTS.md` ships the full
  backlog rules + the `backlog-cleanup` command in BOTH **vh-solara** and
  **another consumer repo**. This is the harness's own bug manifesting in real
  consumers; hermes supplies the grammar that informs the fix.

## This-harness posture (read by this researcher)

The renderer **supports** `features.*` conditionals; the composition pipeline
**does not use them** for the affected surfaces. That is the bug, precisely
located.

- **The renderer evaluates `{{ if .features.backlog }}`.**
  `source=internal/substrate/renderer.go:254-273` (`buildTemplateData`): nests
  dotted keys (`features.backlog`) into nested maps so `{{ .features.backlog }}`
  resolves through the chain; coerces `"true"/"false"` to Go bools so the
  conditional evaluates correctly; always seeds a `features` map (even empty)
  so an unset flag is falsy, not an error. Confirmed by
  `source=internal/substrate/renderer_test.go` (backlog_true includes block /
  backlog_false excludes block / backlog_absent excludes block).
- **But the flag is consulted in only TWO places.**
  - `source=templates/core/opencode.jsonc.tmpl:94` — `{{- if .features.backlog }}`
    gates a comment/maintainer block in the rendered `opencode.jsonc`.
  - `source=internal/permconfig/emit.go:85` — "This is the only place
    features.backlog adds an entry" — the permission `bash` block
    (`computeBashBlock` `:270-312`, gated at `:312`). So the permission layer
    honors the flag.
- **The AGENTS.md compose step is feature-blind.**
  `source=internal/cli/seam.go:473-497` (`composeAgentsMd`): `AGENTS.md =
  AGENTS.core.md + "\n\n" + AGENTS.mission.md`. It reads
  `.vh-agent-harness/AGENTS.core.md` and `.vh-agent-harness/AGENTS.mission.md`
  and concatenates. **No feature-gating.** The composed `AGENTS.md` is
  `platform_managed` and regenerated on every update.
- **The AGENTS.core.md source is preserve-as-is, not templated.**
  `source=templates/core/.vh-agent-harness/AGENTS.core.md` — the full
  "Backlog tracking rules" section (the rules at roughly lines 443-613 of
  that file, including the `backlog-cleanup` command in the standard
  command list and the entire backlog-ledger discipline) is present
  **unconditionally**. The file has no `.tmpl` suffix, so per
  `source=internal/substrate/renderer.go:33,68` it is treated as
  "preserve-as-is" — copied verbatim, never parsed as a Go template. So even
  if the section were wrapped in `{{ if .features.backlog }}`, the renderer
  would not evaluate it.
- **Command registrations are likewise ungated.** The standard command
  templates list in `AGENTS.core.md` includes `backlog-cleanup`,
  `docs-sync`, etc. unconditionally; nothing in the composition pipeline
  suppresses a command registration based on `features.*`.

So the bug has two layers: (1) the `AGENTS.core.md` content is not wrapped in
conditionals at all, and (2) even if it were, the file is preserve-as-is so
the conditionals would not fire. The permission block works because it is
generated by Go code (`permconfig/emit.go`) that reads the feature map
directly, not by templating a static markdown file.

## Options considered

- **OPT-A — Templated AGENTS.core.md with conditionals + bundle-safe command gating (RECOMMENDED).**
  (a) Convert the feature-scoped sections of `AGENTS.core.md` into a
  `.tmpl` (or a templated compose) so `{{ if .features.backlog }}…{{ end }}`
  wraps the backlog rules, and evaluate it at compose time
  (`composeAgentsMd`). (b) Gate command registrations the same way
  (`features.<x>: false` suppresses the `<x>` command registrations).
  (c) Apply hermes's bundle-safe-disable grammar: suppress only the
  feature-specific delta, never shared/core content. The exact render-pipeline
  stage where the flag check fires is the compose step
  (`internal/cli/seam.go:composeAgentsMd`) for AGENTS.md and the command
  render/walk for commands.
- **OPT-B — Generate AGENTS.md from structured blocks instead of concatenating markdown.**
  Model `AGENTS.core.md` as a sequence of named blocks, each tagged with the
  feature it depends on, and emit only the blocks whose feature is on. This
  is more invasive but more maintainable than wrapping prose in
  conditionals. Rejected for now as the larger refactor; OPT-A is the
  minimal fix, OPT-B is a possible later consolidation.
- **OPT-C — Document the flag as permission-only.** Tell consumers
  `features.backlog` only affects the permission block, not the rules.
  Rejected: the flag name promises feature suppression, and two consumers
  already reasonably expect it to suppress the rules. Renaming/documenting
  around the bug entrenches the incoherence.

## Recommendation

**OPT-A.** Honor `features.*` at render/compose time (framing for a
separately-authorized slice):

1. **AGENTS.md.** Make the feature-scoped sections of the AGENTS source
   evaluable and wrap them in `{{ if .features.<x> }}`. The compose step
   (`internal/cli/seam.go:composeAgentsMd:473-497`) is the exact pipeline
   stage where the flag check must fire: it already has the rendered
   `AGENTS.core.md` content in hand and already writes the composed
   `AGENTS.md`; it must run the template substitution (with the
   `buildTemplateData` feature map from `internal/substrate/renderer.go`) over
   the core half before concatenation. (Today it does a raw
   `bytes.TrimRight` + concat with no substitution.)
2. **Commands.** The command corpus walk must suppress a command whose
   feature flag is off. Apply hermes's `bundle_non_core_tools` lesson
   (`toolsets.py:717-742`): suppress only the feature-specific delta, never a
   command referenced by a non-backlog rule. Audit cross-references first
   (open question below).
3. **Docs.** Feature-scoped docs (e.g. coordination docs that are purely
   backlog machinery) follow the same gate.
4. **Token-stability / domain-free.** The fix lives in `templates/core/`
   composition, so it must stay token-based and domain-free — no project
   literals introduced by the gating.

The hermes grammar's value is the **bundle-safe-disable** discipline: the
failure mode to avoid is "disabling `features.backlog` also drops the general
docs/checkpoints guidance because it lived in the same section." The fix
must scope suppression to the backlog-specific delta.

## Findings

- **(finding)**: source=internal/cli/seam.go:473-497, confidence=high, type=fact — `composeAgentsMd` concatenates `AGENTS.core.md` + `AGENTS.mission.md` with no feature-gating and no template substitution; this is the pipeline stage where the bug lives for AGENTS.md.
- **(finding)**: source=templates/core/.vh-agent-harness/AGENTS.core.md, confidence=high, type=fact — the backlog rules (incl. the `backlog-cleanup` command in the standard list and the full backlog-ledger section) are present unconditionally; the file is preserve-as-is (no `.tmpl`), so even adding conditionals would not fire without a compose-time substitution step.
- **(finding)**: source=internal/permconfig/emit.go:85,270-312, confidence=high, type=fact — the permission `bash` block IS feature-gated (`features.Backlog`), proving the renderer supports the flag; this is the only place it takes effect, which is why consumers see the flag "work" for permissions but not for rules.
- **(finding)**: source=internal/substrate/renderer.go:254-273, confidence=high, type=fact — `buildTemplateData` already nests dotted feature keys, coerces bools, and seeds the `features` map — the templating primitive the fix needs already exists.
- **(finding)**: source=refs/hermes-agent/toolsets.py:717-742, confidence=high, type=fact — hermes's `bundle_non_core_tools` is the grammar that prevents disabling a bundle from wiping shared core content; the fix must apply the same delta-only suppression. [confidence high — read directly]
- **(inference)**: source=synthesis, confidence=medium, type=inference — the minimal fix is to run the existing template substitution over the AGENTS core half inside `composeAgentsMd` and wrap feature-scoped sections in conditionals; no new templating machinery is required.

## Contradictions

- One prior adoption-study memo (`2026-07-16-adoption-progressive-disclosure.md`)
  noted "The renderer renders the full tree unconditionally; only agent
  BLOCKS are capability-gated." That is consistent with this finding (the
  full tree renders unconditionally) and does NOT contradict it — it was
  describing agent-block capability gating, not `features.*` gating. No
  contradiction; this memo extends the picture to the `features.*` gap.
- No contradiction with the hermes grammar: hermes uses the bundle-safe-
  disable algebra for toolsets; the harness needs the same algebra for
  rules/commands. The lesson transfers; the surface differs.

## Risks / open-questions

- **Composition dependency (cross-references).** Are any backlog commands
  referenced by non-backlog rules? If `docs-sync` or a checkpoint rule
  references `/backlog-cleanup`, suppressing the command breaks the rule.
  Open: a cross-reference audit of the rendered AGENTS/commands must precede
  the gating. Hermes's `bundle_non_core_tools` (`:717-742`) is the model for
  resolving this: subtract only the delta that is exclusively the feature's.
- **Backward-compat for already-shipped consumers.** vh-solara and another consumer repo
  shipped with the bug — their rendered `AGENTS.md` carries backlog rules
  they did not ask for. A re-render after the fix will REMOVE those rules,
  which is the desired behavior but is a visible diff. Open: do those
  consumers need a migration note, or is "re-render and the unwanted rules
  disappear" the correct one-shot fix? (Likely the latter, since they set
  the flag to false intentionally.)
- **Substrate vs template gating.** Does the fix belong in the substrate
  renderer (general) or in the `composeAgentsMd` step (specific)?
  Recommendation: the conditional wrapping belongs in the source
  (`AGENTS.core.md` → `.tmpl` or a templated compose), and the substitution
  belongs in `composeAgentsMd` where the composed artifact is produced.
  Keep the substrate renderer's existing primitives (`buildTemplateData`) as
  the single source of the feature map.
- **Other `features.*` flags.** Is `backlog` the only flag with this gap, or
  do `safe_defaults` / future flags have the same split (honored in
  permissions, ignored in rules)? Open: audit the full `features.*` surface
  so the fix is general, not a backlog-specific patch.
- **Preserve-as-is invariant.** Converting `AGENTS.core.md` to a templated
  form must not break the "files without `.tmpl` are copied verbatim"
  invariant (`renderer.go:33,68`) — the compose step is a separate
  post-render pass, so the cleanest fix is to template the compose, not to
  reclassify the source file.

## Recommended durable artifact path

`researches/decisions/flag-gated-composition-fix.md` (intended target;
staged under `tmp/decisions-staging/` — read-only execution policy denied
the direct write; see session handoff).

## Promotion targets (if the operator accepts)

When this becomes active guidance, the live targets a follow-up slice would
touch — **not** this memo's job to edit: `internal/cli/seam.go`
(`composeAgentsMd` — add feature-aware substitution), `templates/core/.vh-agent-harness/AGENTS.core.md`
(wrap feature-scoped sections in `{{ if .features.<x> }}`), the command
corpus render/walk (gate command registration on `features.*`), and tests
asserting `features.backlog:false` suppresses both the rules block AND the
command registration (the test that would have caught the consumer defect).