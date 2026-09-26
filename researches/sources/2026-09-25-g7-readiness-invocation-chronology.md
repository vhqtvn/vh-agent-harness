# Sources: G7 readiness invocation chronology (release-DEFER evaluator phase map)

**Date:** 2026-09-25 (research) / 2026-09-26 (landing)
**Topic:** Why the harness-release-readiness G7 step cannot execute the
release-mode DEFER evaluator at its own phase, and the O3 "minimal-honest"
corrective decision that followed.
**Task card:** `release-readiness-g7-invocation-surface` (`.local/coordinator/tasks/`).

**Provenance:**

- Researcher session `ses_f274b0d9fffeLE85G3GfQCpE3K` — invocation-surface
  chronology, grant-surface facts, and the 7-contradiction inventory.
- Debate session `ses_f27437389ffeEvzMZVmLW28HiU` — three-option fit analysis
  (pack-allow / new CLI verb / handed-off receipt) and the O3 recommendation.
- Operator decision — **O3 "minimal-honest variant" approved 2026-09-26**:
  remove G7 as an executable readiness step; phase-honest matrix rows;
  advisory parent-orchestrator release-prep optional; A2 recorded as a future
  option behind a 3-part evidence bar.
- Build session `g7-o3-corrective` re-pinned every cited site against current
  file state on 2026-09-26 before editing (line numbers below are as of that
  re-pin; locate by content when drifting).

## Confidence legend

- **HIGH** — verified against repo source in the 2026-09-26 build-session
  re-pin (read of the cited file at the cited range).
- **MED** — carried from the researcher/debate sessions' verification; spot-
  checked but not line-by-line re-verified at landing.
- **LOW** — inference; directionally useful only.

## 1. Ceremony timeline (N → R → M → tag)

**Confidence: HIGH** — `templates/overlays/release/agents/releaser.md`
(:191-229 handoff/authority model, :533-547 Step 3.2 readiness ceremony at
HEAD=N, :585-593 + :595-657 Step 3.3 manifest ceremony at HEAD=R → M) and
`.vh-agent-harness/overlays/harness-dogfood/agents/harness-release-readiness.md`
(:565-607, the "manifest ceremony (sacred)" block).

1. **N** — the note / release-preparation commit. The readiness agent runs at
   HEAD=N (pre-R, pre-M) and authors the readiness artifact with
   `commit_sha = N`.
2. **R** — the readiness-artifact-only child of N. At tag time the
   `HEAD^^..HEAD^` diff is exactly the readiness-artifact path and the
   artifact's `commit_sha` must equal `HEAD^^` (= N) — enforced by the wrapper
   (`scripts/release-tag.sh:817-825`).
3. **M** — the manifest-only child of R. The disposition manifest is written
   with R as `evaluated_commit` AND `manifest_parent_commit` and `tree(R)` as
   `evaluated_tree`; committed as the single-path final commit before tagging.
4. **Post-M pre-tag** — the release wrapper re-runs the evaluator at HEAD=M
   (the authoritative refusal point) and the Go defer-liveness gate
   (doctor #12 / G0c) re-checks card liveness independently
   (`VH_HARNESS_DEFER_DIFF_SINCE` env, `scripts/release-tag.sh:640`).
5. **Tag** — the wrapper calls `git tag -a` only after every gate passes.
6. **Post-tag** — CI re-runs the committed-manifest evaluator against the
   tagged commit (`.github/workflows/release.yml:58-81`) and refuses
   publication on any refusal.

The releaser's Step 3.3 item 4 ("Re-verify the handshake read-only",
`releaser.md:642-657`) re-runs the evaluator wrapped as
`vh-agent-harness exec bash -c 'node .opencode/scripts/check-defer-triggers.mjs --mode=release --release-version <vX.Y.Z>'`
at the new HEAD (= M) — a FINAL verification surface, not a readiness-phase
one.

## 2. Invocation phase map (8 surfaces)

**Confidence: HIGH** for every row (each surface read at the cited range on
2026-09-26).

| # | Surface | Phase | Status |
|---|---|---|---|
| 1 | Readiness G7 bare `node .opencode/scripts/check-defer-triggers.mjs --mode=release` (was prescribed in readiness.md) | post-N, pre-R | **structurally broken** — REMOVED by the O3 slice |
| 2 | Releaser Step 3.3 item 4 — wrapped `vh-agent-harness exec node …--mode=release` (`releaser.md:642-657`) | post-M | **FINAL** |
| 3 | Release-tag wrapper DEFER gate (`scripts/release-tag.sh:365-469`, invocation ~:393) | post-M, pre-tag | **FINAL, authoritative** |
| 4 | CI re-release manifest recompute (`.github/workflows/release.yml:58-81`) | post-tag | **FINAL, bypass-detection** (refuses publication; cannot un-tag) |
| 5 | CLI `vh-agent-harness defer-triggers` verb (`internal/cli/defer_triggers.go:31-42`, `:99-116`) | commit-time | promoter-mode-ONLY (Landlock ModeStrict + NetDeny, no mode flags — release modes deliberately unexposed) |
| 6 | resolve-first runbook check | curation-time | promoter-mode; not a release surface |
| 7 | Go defer-liveness gate — doctor check #12 / release G0c (`internal/cli/release_gate.go:104-263`, `:407-459`) | post-M, pre-tag (via wrapper) | independent SECOND surface over the same card pool |
| 8 | Release-prep enumerator `--mode=release-prep` (`check-defer-triggers.mjs:1576+`, worktree manifest read `:1558-1574`) | pre-N / at-N | **PRELIMINARY** (advisory enumeration; never a gate) |

## 3. Preliminary vs final — why release mode is HEAD=M-shaped

**Confidence: HIGH** — `templates/core/.opencode/scripts/check-defer-triggers.mjs`
`mainRelease` (starts :1165): manifest bytes read ONLY as a `HEAD:<path>` blob
(~:1195-1214) and the freshness handshake (~:1251-1286) requires ALL of:

- `evaluated_commit == HEAD^`
- `manifest_parent_commit == HEAD^`
- `evaluated_tree == tree(HEAD^)`
- `HEAD^..HEAD` diff is exactly the manifest path (manifest-only child commit)

That commit shape exists only when HEAD = M. At the readiness phase (HEAD = N)
the committed manifest at `HEAD:` is the PREVIOUS ceremony's standing file, so
a release-mode run there deterministically classifies `evaluator-error`
(handshake mismatch / non-manifest-only diff) — structurally meaningless
output, which the old readiness doc then escalated to BLOCKER → `ready: no`
on every ceremony.

Input availability by phase:

| Evaluator input | pre-R (HEAD=N) | post-M (HEAD=M) |
|---|---|---|
| committed manifest bytes (`HEAD:` blob) | previous ceremony's standing manifest | current ceremony's manifest (R→M child) |
| `HEAD^` = R with artifact-only diff | does not exist yet | yes |
| `HEAD^..HEAD` = manifest-only diff | no (HEAD is N or earlier) | yes |
| current-ceremony manifest content | worktree-dirty only — INVISIBLE to release mode | committed |

The post-M final verification CANNOT substitute for a pre-R verdict:

- **Artifact binding** — the wrapper enforces readiness-artifact
  `commit_sha == HEAD^^ == N` (`release-tag.sh:817-825`); a pre-R "final"
  verdict would have to be bound to a manifest that does not exist yet.
- **Dependency direction** — the readiness report gates the START of the
  releaser ceremony; the manifest M is authored later in that same ceremony.
- **Different questions** — pre-R work is ENUMERATION (which firing cards
  need dispositions for the N-commit reconciliation); post-M work is the
  COMMITTED-DISPOSITION handshake. The former is served by
  `--mode=release-prep` (worktree manifest, no handshake); the latter only by
  release mode.

## 4. Writes — the honest write posture

**Confidence: HIGH** — `check-defer-triggers.mjs` `allocCaptureDir`
(~:238-251: exclusive `mkdir` under `<repoRoot>/tmp`, mode 0700, UUID name,
EEXIST retry), `gitCapture` (~:270-299: capture file inside the exclusive
dir, best-effort `rmCaptureDir` in `finally`); CLI verb pre-creates repo
`tmp/` as the sole RWDir (`internal/cli/defer_triggers.go:83-90`).

The evaluator performs **no protected-state writes** — never `.git/`, never
tracked files — but it DOES write transient capture scratch under repo `tmp/`
per git call (best-effort cleanup). The prior "the evaluator is read-only —
it writes nothing" claims (readiness.md :43-49 / :190-195 / :616-617 as they
stood; evaluator self-comment :1298-1299) were overbroad and are corrected in
both docs by this slice.

## 5. Override authority — wrapper-only

**Confidence: HIGH** — evaluator header (`check-defer-triggers.mjs:103-115`)
and `applyDisposition` Layer A/B (~:1099-1141): `--override-confirmed-version`
is supplied by the authorized release wrapper ONLY, after the operator-side
`--override-release-version` + `--override-manifest-sha` ceremony agrees;
"Model/reviewer surfaces (the advisory readiness surface) cannot supply this
flag." The old readiness doc's override-forwarding prose (":614-616: pass
`--override-confirmed-version` … ONLY when the wrapper ceremony has
confirmed") contradicted this and is removed. The Go gate's escape is a
SEPARATE surface: `VH_HARNESS_DEFER_OVERRIDE_IDS`
(`internal/cli/release_gate.go`, surfaced in FAIL detail).

## 6. Grant-surface facts (why the readiness agent cannot run it anyway)

**Confidence: HIGH** — `internal/permconfig/emit.go:786+` (`parseLocation`
reads only `wildcard` / `readonly` / `git_readonly` / `gate` /
`harnessPolicy` / `harness` / `devSh` / `edit` / `editOverrides` location
keys — there is no pack-level per-command allow channel);
`internal/permconfig/model.go:141-160` (`ExtraBash` entries emit in region 3,
BEFORE the region 4a catch-all deny — last-match-wins makes 4a win over them;
`ReadOnlyExtraAllows` is region 4b, core-tables-only, "Populated by core
tables only; overlay-pack parsing does not read this field"). `node` is not
in the read-only command group; the harness-release-readiness agent is an
overlay-pack agent whose permission pack carries `harness: deny`. Therefore
no permission-shaped path existed for a readiness-agent bare-node or
wrapped-node G7 invocation short of new core grant machinery — which the
non-goals forbid.

## 7. Option fit and the O3 decision

A release-mode-shaped G7 at the readiness phase is chronology-infeasible
under all three candidate options:

1. **Pack-allow** (grant the readiness agent a node/exec exception) — grants
   an invocation whose output is a deterministic evaluator-error at that
   phase; also requires core grant machinery (no per-pack per-command allow
   channel exists — §6).
2. **New CLI verb** (a wrapped release-mode subverb) — same chronology
   defect one wrapper-layer up; the handshake still requires HEAD=M.
3. **Handed-off receipt** (wrapper/releaser publishes the post-M evaluator
   envelope; readiness "consumes" it) — the receipt does not exist pre-M
   (dependency inversion: readiness gates the releaser start), and an
   internal-consistency-only receipt is unverifiable before M.

**DECISION — O3 minimal-honest variant (operator-approved 2026-09-26):**

- Remove G7 as an EXECUTABLE readiness step entirely (no evaluator
  invocation, no override-forwarding, no G7-emitted blockers/warnings).
- Phase-honest matrix rows: pre-R carries "no final G7 verdict exists at this
  phase — final defer-gate verification occurs post-M at the release wrapper
  (authoritative), releaser Step 3.3 item 4, and CI."
- The wrapper (`scripts/release-tag.sh`), the releaser's Step 3.3 item 4, and
  the post-tag CI recheck remain the authoritative post-M controls,
  unchanged.
- Advisory (prose-only): a parent orchestrator (build-class, already
  exec-authorized) MAY run `--mode=release-prep` pre-N/at-N and hand the
  enumeration to the readiness agent as advisory input — never a
  readiness-agent prerequisite, never re-escalated to a gate.
- **A2 (contained release-prep subverb)** recorded as a FUTURE option behind
  a 3-part evidence bar: (a) a distinct pre-R catch class the current
  advisory flow demonstrably misses, (b) a demonstrated need for
  machine-checked preflight in the report, and (c) a containable design
  proven (Landlock ModeStrict + NetDeny profile like the promoter verb).
  Not built in this slice.

## 8. Contradiction inventory (7) with terminal dispositions

| # | Contradiction | Disposition (this slice) |
|---|---|---|
| a | G7 chronology defect — readiness prescribed release-mode evaluation at HEAD=N where only HEAD=M satisfies the handshake → deterministic evaluator-error → its own matrix forced BLOCKER → `ready: no` every ceremony | REMOVED — executable prescription, evaluation rule, matrix, and boundary reminder replaced with phase-honest rows (commit 1) |
| b | Override-forwarding prose (readiness agent told to pass `--override-confirmed-version`) vs wrapper-only authority | REMOVED with the prescription; replaced by an explicit "do not forward evaluator flags" note (commit 1) |
| c | Overbroad "never writes / read-only — writes nothing" claims in readiness.md (:43-49, :190-195, :616-617) and the evaluator self-comment (:1298-1299) | CORRECTED in both docs to "no protected-state writes; transient scratch under repo `tmp/`" (commit 1, source + mirror) |
| d | Stale "(after activation)" ref in the G7 matrix (concept retired v0.13.0) | REMOVED with the matrix replacement (commit 1) |
| e | Loose "SAME last_tag the report carries" prose (release mode derives its base itself; no `--since` operand) | REMOVED — release-arc bullet now states the report's `last_tag` is never an operand (commit 1) |
| f | Wrapper forwarded a dead `--since "$PRIOR_TAG"` to release mode (mainRelease never reads `options.since`; only mainPromoter/mainReleasePrep do) | REMOVED from `DEFER_ARGS` in `scripts/release-tag.sh` by this slice's second commit (same `Task-Card` trailer); `PRIOR_TAG` assignment retained (still used by the G0c doctor env at :640) |
| g | Stale path refs claiming the readiness agent lives in `templates/overlays/release/` | **REFUTED-BY-ABSENCE at the 2026-09-26 premise re-pin** — no such ref exists in the readiness source; the only `templates/overlays/release/` mention (readiness.md :278) correctly describes the release pack shipping the releaser, and `templates/overlays/release/agents/` contains only `releaser.md`. Nothing to correct; recorded here as the terminal disposition. |
