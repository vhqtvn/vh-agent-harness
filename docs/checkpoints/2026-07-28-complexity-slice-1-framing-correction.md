# Complexity Slice 1 — framing correction + self-exemption reconciliation

Date: 2026-07-28
Scope: corrects the overclaim framing of commit `9362e11` ("feat(complexity):
Slice 1 foundation") and reconciles the self-exemption asymmetry between the
live `.opencode/**` render and its `templates/core/.opencode/**` embedded-source
mirror. Two no-regret fixes; the WARN-only advisory invariant is unchanged.

## Why this checkpoint exists

`9362e11` shipped doctor check #20 (`complexity-advisory`) and titled itself
"Slice 1 foundation." A reader of the commit message + the shipped types
(`Signal`/`Metric`/`BoundaryIndicator`) + the dispositions schema enum
(`file_loc | function_loc`) could reasonably conclude more than one complexity
axis is live. It is not. This checkpoint is the durable record of what Slice 1
actually shipped, what was deferred, and why the check is WARN-only.

`9362e11` is local-only (not pushed) but is not HEAD (`413a5b2` sits on top), so
the commit message stands; this document plus the follow-up commits carry the
correction. No amend.

## Slice 1 shipped scope — 1 of 4 design-declared axes (file_loc only)

The design declares four complexity axes. Slice 1 shipped exactly ONE live
metric. The exact type/field names (re-derived from
`internal/complexity/signal.go`):

| Axis | Type / field | Status in Slice 1 |
|------|--------------|-------------------|
| **file_loc** | `MetricFileLoc MetricKind = "file_loc"` (signal.go:41) | **LIVE — the only metric the scanner computes.** `ComputeSignal` (policy.go:171) always sets `Metric.Kind = MetricFileLoc`. |
| **function_loc** | `MetricFunctionLoc MetricKind = "function_loc"` (signal.go:42) | **Deferred enum value.** The constant exists and the dispositions schema accepts it (`internal/schema/complexity_dispositions.go` enum), but the scanner never produces it — signal.go:36-37 reserves it "for where a real parser exists (do NOT regex-approximate)." No parser ships in v1. |
| **agent-context** (the design-declared DOMINANT axis) | — (no struct field) | **Entirely absent.** There is no field in `Signal`, `Metric`, or `BoundaryIndicator` carrying agent-context complexity. This is the most important gap: the axis the design names as dominant is not even stubbed. |
| **BoundaryIndicator** | `BoundaryIndicator` struct (signal.go:70) | **Hardcoded `not_collected`.** `ScanRepo` (scanner.go:44) stamps every signal with `BoundaryIndicatorNotCollected()` (policy.go:218-223), which returns `{Kind: "top_level_symbol_count", Value: 0, Evidence: "not_collected"}`. The snapshot scanner counts lines only; it runs no parser. |

**Bottom line:** only `file_loc` is live. `function_loc` is a reserved enum
value, `BoundaryIndicator` is a structural placeholder reporting
`not_collected`, and the design's dominant axis (agent-context) has no
representation in the code at all. The "Slice 1 foundation" framing is accurate
in the sense that the contract/types/scanner/advisory plumbing are laid, but a
reader should not infer that multiple complexity signals are computed.

## Limitations — Goodhart risk and why the check is WARN-only

The `complexity-advisory` check is **WARNING-ONLY by sacred invariant**
(`internal/cli/doctor_complexity.go`: structurally `tierSkip`/`tierPass`/
`tierWarn`, never `tierFail`). This is not a temporary state; it is the design.
The reason is the Goodhart risk the single live metric creates:

- **The LOC heuristic is gameable.** A file at 600 lines can be "fixed" by
  splitting it into two 300-line files. Split-to-pass-LOC *lowers* the measured
  signal while *worsening* the underlying property (coupling/fan-out across the
  new boundary). The advisory would report the breach resolved when the real
  complexity may have increased.
- **That gaming is invisible in Slice 1.** Detecting split-to-pass-LOC requires
  the agent-context axis (axis 3) — the design's dominant signal — which would
  surface cross-file coupling and boundary fan-out. With that axis entirely
  absent, the advisory cannot distinguish a genuine simplification from a
  cosmetic split. Surfacing such a gameable signal as a gate would reward the
  wrong behavior.
- **Therefore advisory, not gate.** Complexity signals INFORM; they never
  authorize a transition. The operator is expected to record a disposition
  (accept-as-cohesive / split-defer) in `complexity-dispositions.yml` (seeded
  blank in Slice 1; Slice 4 records real dispositions), not to mechanically
  chase the threshold.

This limitation persists until the agent-context axis ships. Until then, every
`file_loc` nomination should be read as "this file is long; a human must decide
whether it is genuinely cohesive," never as "this file must be split."

## Self-exemption asymmetry — reconciliation (exclude both)

### The asymmetry

`complexity-policy.yml` excludes `.opencode/**` from the snapshot projection
(rationale per signal.go:32: "rendered output"). The same logical files also
live at `templates/core/.opencode/**` in this dogfood repo — that is the
**embedded source** baked into the Go binary via `go:embed`
(`templates/core/` per AGENTS.md: "the embedded corpus; THIS is what ships into
projects"). The exclude pattern `.opencode/**` anchors at path segment 0 and
does **not** match `templates/core/.opencode/**`, so the embedded-source mirror
is scanned while the rendered twin is exempt.

This is material, not theoretical: 12 files under `templates/core/.opencode/`
exceed the 500-line snapshot threshold (the largest is `state-lib.js` at 6147
lines; also `verify-task-registry.js` 1709, `repo-mail-egress-gate.js` 1232,
`check-defer-triggers.js` 1059, and 8 more). Their byte-identical `.opencode/`
renders (`diff` exit=0) are excluded. So the advisory would warn about 12
harness-internal scripts while treating their rendered twins as fine.

### Fact question — managed/generated vs hand-authored refactor target

> Are `templates/core/.opencode/scripts/*.js` genuinely managed (regenerated by
> `make update` / hand-maintained as the embedded source) or hand-authored
> refactor targets the check should catch?

**Evidence:**
- They are **hand-maintained as the embedded corpus.** AGENTS.md dogfood loop:
  "Edit `templates/core/` (the source), rebuild, then `make update` to
  regenerate." `make update` reads FROM `templates/core/`, it does not write to
  it. The files are the go:embed source baked into every binary release.
- They are **byte-identical** to the `.opencode/` renders (no templating tokens
  in the `.js` bodies; `diff` exit=0), so the two locations carry the same
  complexity in every sense that matters to the line-count metric.
- They are **harness-release-managed**, not project-domain code. A change to
  these files ships through the harness release process (go:embed + versioned
  binary + `update`), not through project refactoring. An operator-facing
  complexity advisory that warns "refactor this" points at files the operator
  can only change by cutting a harness release — a workflow the advisory does
  not serve.
- **Consumers never see `templates/core/`.** Only this dogfood source-checkout
  repo carries both locations. A consumer repo has only the rendered `.opencode/`
  (already excluded).

### Decision: exclude both locations

The evidence supports treating the embedded corpus as a **managed harness
artifact** (akin to vendored library code), not a project refactor target. The
consistent policy is to exclude both the rendered output and the embedded-source
mirror:

- `snapshot_paths` exclude gains `templates/core/.opencode/**`, mirroring the
  existing `.opencode/**` exclusion.
- Applied to **both** `.vh-agent-harness/complexity-policy.yml` (repo-root live)
  and `templates/core/.vh-agent-harness/complexity-policy.yml` (embedded
  default) so they stay in sync across `make update`. The added pattern is a
  no-op for consumers (no such path exists in a consumer repo).
- The complexity advisory remains meaningful for all actual project code (Go
  source under `internal/`, `cmd/`, and any consumer project code). Only the
  harness's own embedded scripts (source + render) are exempted.

### Why not the alternative (flag both)

Flagging both would require removing the `.opencode/**` exclusion. That
exclusion is load-bearing for **consumers**, whose entire `.opencode/` tree is
generated output that must not be complexity-checked. Removing it would break
the consumer experience and is not viable. Excluding both is the only
reconciliation that is consistent, consumer-safe, and resolves the asymmetry.

### Dogfooding trade-off (recorded honestly)

Excluding `templates/core/.opencode/**` means the dogfood repo no longer
self-checks the complexity of its own embedded scripts. This is an accepted
trade-off: those scripts' complexity is tracked through the harness's own
backlog/review/release process, not through the project-facing complexity
advisory. The 12 large scripts are already known to the team (the team authored
them); silencing a non-actionable advisory warning about them removes noise
without losing a signal that is acted on elsewhere.

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Only `file_loc` is live; `ComputeSignal` always sets it | `grep -n 'MetricFileLoc\|MetricFunctionLoc' internal/complexity/policy.go internal/complexity/signal.go` — `MetricFileLoc` used at policy.go:182; `MetricFunctionLoc` never referenced in scanner/policy | yes |
| `BoundaryIndicator` hardcoded `not_collected` | scanner.go:44 stamps `BoundaryIndicatorNotCollected()`; policy.go:218-223 returns `{Evidence: "not_collected"}` | yes |
| WARN-only invariant (never tierFail) | `internal/cli/doctor_complexity.go` returns tierSkip/tierPass/tierWarn only | yes |
| 12 templates/core/.opencode/ files exceed 500 lines | `wc -l templates/core/.opencode/scripts/*.js templates/core/.opencode/repo-configs/*.js` — 12 files >500 | yes |
| Source and render byte-identical | `diff .opencode/scripts/complexity-signal-lib.js templates/core/.opencode/scripts/complexity-signal-lib.js` → exit 0 | yes |
| Exclude asymmetry pre-fix | `.opencode/**` matches `.opencode/scripts/X.js`; does NOT match `templates/core/.opencode/scripts/X.js` (glob anchored at segment 0) | yes |
| README.agent.md does not overclaim axes | description says "file LOC" only; no multi-axis claim | yes |
| `complexity-policy.yml` ownership = `platform_armed` | `core_manifest.go:252` embed manifest map: `".vh-agent-harness/complexity-policy.yml": ownership.ClassPlatformArmed`; corroborated by `internal/schema/complexity_policy.go:12` doc comment ("Ownership class: platform_armed") and `internal/cli/doctor_complexity.go:13` | yes |

## Findings
- **file_loc is the sole live metric**: source=signal.go:41+policy.go:182, confidence=high, type=fact
- **agent-context axis entirely absent**: source=signal.go (no field), confidence=high, type=fact
- **BoundaryIndicator hardcoded not_collected**: source=scanner.go:44+policy.go:218, confidence=high, type=fact
- **templates/core/.opencode is go:embed source, not generated**: source=AGENTS.md dogfood loop + diff exit=0, confidence=high, type=fact
- **12 embedded scripts exceed threshold**: source=wc -l output, confidence=high, type=fact

## Contradictions
- Task assumed a complexity Slice 1 **checkpoint file** existed (`glob docs/checkpoints/*complexity*` → none). The "foundation/4-axes" framing lived only in the `9362e11` commit message. Resolved by creating this checkpoint as the durable correction (no file to edit; no amend since 9362e11 is not HEAD).
- Task's decision tree offered "exclude both" vs "flag both (remove .opencode/** exclusion)." "Flag both" is not viable because `.opencode/**` exclusion is load-bearing for consumers. "Exclude both" is the only consumer-safe reconciliation; documented above.

## Follow-up
- The agent-context axis (design's dominant signal) remains entirely absent. When it ships, revisit whether `function_loc` and a real `BoundaryIndicator` should also activate, and whether the Goodhart caveat above can be softened.
- `state-lib.js` (6147 lines) and the other large embedded scripts are now excluded from the advisory; their complexity should be tracked in the harness backlog separately if the team chooses to address it.

## Re-enable safety & honesty caveats (recorded at workstream close)

### Armed-policy gotcha — the re-enable path

`complexity-policy.yml` carries ownership class **`platform_armed`**.
Consequence: `make update` does **NOT** propagate field-level state (notably
`enabled`) from the template
(`templates/core/.vh-agent-harness/complexity-policy.yml`) to the armed
instance. The **armed instance `.vh-agent-harness/complexity-policy.yml` is
what `doctor` actually reads.** Therefore whoever re-enables the complexity
advisory when the agent-context axis lands **MUST edit BOTH files** — editing
only the template and running `make update` will leave the armed instance
unchanged and `doctor` will still see `enabled: false`.

This qualifies the "so they stay in sync across `make update`" note above (the
`snapshot_paths` exclude edit): that sync was a **manual** two-file edit, not an
auto-sync. Armed files are not overwritten by render, so `enabled` (and any
state field) must be set in the armed instance directly, with the template
updated separately for consistency.

### S3-canon honesty caveat

The S3 probe + model-study baselines are now on canon (commit `08920d5`) as
**byte-faithful records**, but they are **not re-derivable off-host**
(Class-A evidence) — they depend on the on-host DB
(`~/.local/share/opencode/opencode.db`, ~24 GB). A future reader should treat
them as a recorded snapshot, not something independently reproducible from the
repo alone.

### Resting path

This workstream rests at: **agent-context axis (axis-2) build → re-measure
against the S3 baseline → re-enable** (editing both `complexity-policy.yml`
files per the armed-policy gotcha above).
