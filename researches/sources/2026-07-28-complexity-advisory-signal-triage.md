# Sources: Complexity-Advisory Signal Triage (doctor check #20) — Over-Raise Validation

**Date:** 2026-07-28 (triage date; promoted to durable sources 2026-07-28)
**Topic:** independent validation that the complexity-advisory signal (doctor check
#20, Slice 1 foundation `9362e11`) is **mostly OVER-RAISE**. Faithful read-only
replication of the live scanner (`git ls-files`-tracked, snapshot projection,
`observed > 500`) over 40 day-1 nominees, plus structural sampling of 11/40 and
role analysis of all 40. RESULT: **1/40 genuine** (`doctor.go`), **2/40 ambiguous**
(`state-lib.js`, `seam.go`), **37/40 cohesive** → **≤7.5% actionable yield** → the
signal's present-value precision is too low to justify surfacing 40 WARNs on day 1.
This is the **independent, different-mechanism corroboration** of the S3 probe's
over-raise verdict (S3 = model-judgment disposition probe; this = deterministic LOC
heuristic). Recommendation recorded in the body — Fix 2b (skip/silent) + defer Slice
4 — has since been **acted on** (2b landed in `78e3610`).
**Series:** corroborates
[`./2026-07-28-s3-probe-dropped-blindspot.md`](./2026-07-28-s3-probe-dropped-blindspot.md)
(S3 dropped-blind-spot probe, the load-bearing corroboration for the
complexity-over-raise decision), indexed by
[`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md)
(commit-review panel model-study series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** empirical triage / evaluation (deterministic scanner replication
  + structural sampling) — the independent corroboration of the S3 probe's
  over-raise finding via a **different mechanism** (LOC heuristic, not model
  judgment).
- **Evidence class:** **re-derivable** code-structural evaluation over git-tracked
  files (unlike the S3 probe, which is Class-A this-machine DB-cited and not
  re-derivable outside this host). The scan can be reproduced by re-running the
  faithful replication against the live policy at
  `.vh-agent-harness/complexity-policy.yml`; the per-candidate dispositions are
  read-only inference over the tracked source. The `doctor.go` genuine-target call
  is grounded in `grep '^func '` + the sibling-file split pattern
  (`doctor_f1.go`, `doctor_f2.go`, `doctor_complexity.go`, …).
- **Original location:** `tmp/complexity-scan/2026-07-28-complexity-advisory-signal-triage.md`
  (volatile; lives under the gitignored `tmp/` tree, not committed canon; promoted to
  durable sources on 2026-07-28 so the over-raise finding is citable when the
  agent-context axis lands and complexity-signal yield is re-measured).
- **Promotion basis:** the packet corroborates the S3 probe's over-raise verdict via
  an **independent mechanism**. The S3 probe (model-judgment disposition probe, 0/39
  real-wrongly-dropped) and this triage (deterministic LOC scanner, 1–3/40
  actionable) are **different signal classes** but the **same outcome class —
  over-raise**. Promoted so both corroborations are citable canon, not tmp-only.
- **Consuming decision:** complexity-control over-raise policy (Fix 2b). The packet's
  recommendation — default the advisory to skip/silent until a second axis (the
  dominant agent-context axis) raises precision; defer Slice 4 disposition machinery
  — has since been **acted on**: `78e3610` flips `complexity-policy.yml` to
  `enabled: false` (both `.vh-agent-harness/` and `templates/core/`). The consuming
  policy is owned by the design lane (complexity-control session) and is **not** in
  `researches/decisions/` at promotion time.
- **Cross-reference (corroborating prior art):**
  [`./2026-07-28-s3-probe-dropped-blindspot.md`](./2026-07-28-s3-probe-dropped-blindspot.md)
  — the S3 dropped-blind-spot probe, promoted to citable canon in commit `08920d5`
  (part of the commit-review panel model-study series). That probe is the load-bearing
  corroboration for the complexity-over-raise decision; **this packet is its
  independent, different-mechanism corroboration**.
- **Promotion commit:** the commit that introduces this file. A committed file cannot
  embed its own introducing commit's hash without an amend (the hash depends on the
  file's bytes); resolve via
  `git log -1 --format=%H -- researches/sources/2026-07-28-complexity-advisory-signal-triage.md`.
- **Status:** durable source packet. The body is preserved verbatim as the historical
  record of the triage that motivated Fix 2b.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim
  copy of the original `tmp/` packet; only this provenance header is new.

## Time-of-triage vs now (reconciliation carried by this header)

The body was written at triage time and is preserved verbatim. Two of its statements
are **historically accurate but now resolved** by later events; this header records
the resolution so a future reader is not misled:

1. **"The S3 probe premise could not be sourced"** (body §3, `## Contradictions`).
   This was **accurate at triage time**: the S3 probe lived only under `tmp/` and was
   undiscoverable to a researcher searching `researches/`/`docs/`/`.local/`/
   `.opencode/state/`. The S3 probe has since been **promoted to citable canon** in
   commit `08920d5` (see cross-reference above), so the sourcing gap is now closed.
   Note that the complexity signal's independently-derived over-raise ratio (1–3/40
   actionable) **does not depend** on the S3 "0/39" number and stands on its own; the
   comparison in body §3 is "consistent-direction" regardless of the S3 magnitude.
2. **`## Promotion targets`** (body, end). The recommended live-doc landings have
   partially landed: **Fix 2b (`enabled: false`) landed in `78e3610`**. The remaining
   targets (Slice 4 as DEFER-with-trigger, session-memory decision-log entries,
   `doctor_complexity.go` doc-comment describing check #20 as silent-by-default)
   remain open as recorded.

---
# Complexity-Advisory Signal Triage (doctor check #20) — Source Packet

**Date:** 2026-07-28
**Scope:** Validate whether the complexity-advisory signal (doctor check #20, commit
`9362e11`, Slice 1 foundation) flags **genuine refactor targets** or **over-raise
noise**. Gates fix 2's fork (2a wire-suppression vs 2b skip/silent) and whether
Slice 4 disposition machinery is worth scheduling.
**Evidence tier:** MEDIUM-HIGH — faithful read-only replication of the live scanner
(`git ls-files`-tracked, snapshot projection, `observed > 500`), independent
confirmation of the day-1 count, plus structural sampling (func/section grep) of
11/40 files and category-level role analysis of all 40.
**Type:** read-only source packet / evaluation. Not a decision memo (no option
scoring matrix); the operator owns the fork choice. Recommendation is included
because the task explicitly requested it.
**NOTE:** `researches/sources/` is edit-denied to the researcher role; this packet
is staged in `tmp/`. Promotion to `researches/sources/2026-07-28-complexity-advisory-signal-triage.md`
is an operator/docs-steward action.

---

## How the scan was reproduced (method, read-only)

The installed PATH binary is **stale** relative to the working tree — `doctor`
reports `dev-stale-embed WARN` (embedded corpus differs from checkout) and does
NOT carry check #20 (the doctor run ends at `head-progress`; no
`complexity-advisory` line). This is the documented dogfood footgun, not a
defect. So the live check could not be invoked through the binary.

Instead the scanner was replicated faithfully from source
(`internal/complexity/scanner.go` `ScanRepo` + `policy.go` `Eligible`/`ComputeSignal`)
against the LIVE policy at `.vh-agent-harness/complexity-policy.yml`:

- **enumeration:** `git ls-files --cached` (tracked-only) — scanner.go L54–83.
- **supported extensions:** `.go .cjs .cts .js .jsx .mjs .mts .py .ts .tsx`.
- **snapshot excludes:** `.git/**`, `.opencode/**`, `tmp/**`, `**/__pycache__/**`,
  `**/node_modules/**` (snapshot_paths) AND basename suffix `_test.go`
  (snapshot_suffixes).
- **threshold:** `snapshot_file_lines: 500`; **nomination:** `observed > 500`
  (strict, policy.go L176).
- **line semantics:** `CountLines` (trailing newline does NOT create a phantom
  line); reproduced with `wc -l` + trailing-newline compensation. Boundary is
  clean (509 nominated vs 487 not), so the count is exact.

**Critical exclusion verified:** `refs/` is gitignored (`.gitignore:71`). The
scanner's `git ls-files` therefore does NOT enumerate the vendored reference
bundles (`refs/cc-re/bundle_formatted.js` = 729,281 lines, dozens of
`refs/opencode/**` files). A naive `find` walk over-counts dramatically; the
faithful git-tracked scan does not. The scanner.go docstring's assumption that
"refs/" is gitignored holds for this repo.

**Count reconciliation:** tracked supported-ext files = 379; eligible (after
snapshot exclusions) = 327; **nominated (`> 500`) = 40.** The 40 matches the
operator's stated "~40 cohesive files" exactly and independently.

---

## 1. Actual day-1 candidate list (40 nominated, `observed > 500`)

Snapshot threshold = 500. Sorted by descending LOC. Category tag:
`CORE` = `templates/core/` (embedded corpus source, ships to consumers),
`OVLY` = `templates/overlays/` (overlay source), `SRC` = `internal/` (Go binary),
`TEST` = `tests/e2e/`.

| # | LOC | Cat | Path |
|---|-----|-----|------|
| 1 | 6147 | CORE | templates/core/.opencode/scripts/state-lib.js |
| 2 | 4415 | OVLY | templates/overlays/auto-classifier-pilot/plugins/auto-tool-gate.js |
| 3 | 2420 | OVLY | templates/overlays/auto-classifier-pilot/plugins/auto-gate-live.js |
| 4 | 2102 | SRC  | internal/cli/doctor.go |
| 5 | 1709 | CORE | templates/core/.opencode/scripts/verify-task-registry.js |
| 6 | 1413 | TEST | tests/e2e/auto-gate-opencode/run-e2e.mjs |
| 7 | 1232 | CORE | templates/core/.opencode/repo-configs/repo-mail-egress.js |
| 8 | 1218 | CORE | templates/core/.opencode/plugins/shell-guard-core.js |
| 9 | 1087 | SRC  | internal/cli/f2_projection.go |
| 10 | 1059 | CORE | templates/core/.opencode/scripts/check-defer-triggers.js |
| 11 | 988 | SRC  | internal/cli/diagnostics.go |
| 12 | 976 | SRC  | internal/cli/seam.go |
| 13 | 960 | SRC  | internal/execro/classifier.go |
| 14 | 923 | CORE | templates/core/.opencode/scripts/verify-f3-design-readiness.js |
| 15 | 844 | SRC  | internal/permconfig/emit.go |
| 16 | 784 | SRC  | internal/cli/f1_validator.go |
| 17 | 759 | CORE | templates/core/.opencode/scripts/verify-f3-dispatch-backstop.js |
| 18 | 756 | CORE | templates/core/.opencode/tools/plan-state.js |
| 19 | 755 | OVLY | templates/overlays/repo-mail/plugins/repo-mail-egress-wiring.js |
| 20 | 755 | CORE | templates/core/.opencode/scripts/verify-session-state.js |
| 21 | 747 | CORE | templates/core/.opencode/scripts/normalize-backlog.js |
| 22 | 740 | CORE | templates/core/.opencode/sys-scripts/tier_cascade.py |
| 23 | 718 | CORE | templates/core/.opencode/scripts/f3-design-readiness.js |
| 24 | 668 | OVLY | templates/overlays/auto-classifier-pilot/plugins/auto-gate-tiered.js |
| 25 | 651 | CORE | templates/core/.opencode/scripts/verify-f3-task-ready.js |
| 26 | 647 | SRC  | internal/memory/claims/claim.go |
| 27 | 617 | SRC  | internal/cli/profile.go |
| 28 | 607 | SRC  | internal/permconfig/tables.go |
| 29 | 599 | SRC  | internal/cli/release_gate.go |
| 30 | 593 | CORE | templates/core/.opencode/skills/bgshell-job/scripts/bgshell_job.py |
| 31 | 567 | TEST | tests/e2e/auto-gate-classifier/e2e-driver.mjs |
| 32 | 566 | CORE | templates/core/.opencode/scripts/verify-no-source-tree-only-paths.js |
| 33 | 561 | SRC  | internal/cli/f1_r1.go |
| 34 | 558 | SRC  | internal/cli/overlay_new.go |
| 35 | 551 | SRC  | internal/cli/update.go |
| 36 | 542 | SRC  | internal/memory/store/store.go |
| 37 | 536 | OVLY | templates/overlays/auto-classifier-pilot/plugins/auto-gate-scrub.js |
| 38 | 531 | CORE | templates/core/.opencode/scripts/verify-coordination-hints.js |
| 39 | 522 | SRC  | internal/substrate/renderer.go |
| 40 | 509 | SRC  | internal/cli/f2_streak.go |

**Category counts:** CORE 16 (40%) · OVLY 5 (12.5%) · SRC 17 (42.5%) · TEST 2 (5%).
Templates (CORE+OVLY) = 21 (52.5%); real binary source (SRC) = 17 (42.5%); test
drivers = 2 (5%).

---

## 2. Per-candidate assessment (top ~15, present-value standard)

Standard (from the complexity-control session's corrected-framing checkpoint):
**"split-defer needs evidence, not just separability."** A file is a *genuine*
refactor target only if a real decomposition exists AND an operator would act on
it now. *Cohesive* = large but one concern; splitting relocates complexity rather
than reduces it. *Ambiguous* = a plausible seam exists but no acute evidence /
high split cost. "Generated/managed" distinction: all 40 are **hand-authored**
(neither refs/ nor rendered-output); CORE/OVLY are template *source* that is
embedded/rendered but still authored by hand — they are the Fix 2b asymmetry case
(exempt `.opencode/` rendered mirror, flag `templates/core/` source mirror).

1. **state-lib.js (6147, CORE) — AMBIGUOUS.** 326 functions; a flat shared state
   helper library (sessions/workstreams/drafts/coordinator/memory paths + locks +
   IO). Separable sub-domains visibly exist, so splitting is *possible*. But it is
   the consumer-shipping embedded lib with a load-sensitivity guard
   (`render-location guard`, closeout seam) — splitting changes the consumer
   surface and carries real risk. Cohesive-by-design as "the state library"; no
   acute evidence.
2. **auto-tool-gate.js (4415, OVLY) — COHESIVE.** Overlay plugin
   (auto-classifier-pilot, explicitly a *pilot*). Self-contained config-read +
   normalize + cache layers; overlay plugins are deliberately monolithic. Only
   present if overlay selected (this repo selects it).
3. **auto-gate-live.js (2420, OVLY) — COHESIVE.** Overlay plugin, same rationale.
4. **doctor.go (2102, SRC) — GENUINE.** 39 funcs; the catch-all doctor aggregator
   containing multiple separable check clusters (auto-gate validation
   `validateAutoGatePluginConfig`/`validateAutoGateLlmConfig` + ~15 `autoGate*`
   helpers L1240–1777; gitignore helpers `gitTracked`/`gitCheckIgnoreVerbose`/
   `parseIgnoreSource` L1883–2038). An established split pattern already exists —
   sibling files `doctor_f1.go`, `doctor_f2.go`, `doctor_complexity.go`,
   `doctor_head_progress.go`, `doctor_behavioral_closure.go` prove the team
   incrementally decomposes checks. The complexity feature's OWN home file is the
   clearest real seam.
5. **verify-task-registry.js (1709, CORE) — COHESIVE.** One verifier CLI; single
   concern (validate the local task registry).
6. **run-e2e.mjs (1413, TEST) — COHESIVE.** E2E test orchestration driver; test
   drivers are conventionally large monolithic flow scripts.
7. **repo-mail-egress-gate.js (1232, CORE) — COHESIVE.** The repo-mail egress gate
   (secret/pattern redaction for outbound mail); one gate's validation rules.
8. **shell-guard-core.js (1218, CORE) — COHESIVE.** Shell-guard command
   classifier/parser; one plugin's classification logic.
9. **f2_projection.go (1087, SRC) — COHESIVE.** 21 funcs, all F2 markdown
   rendering (`renderF2*` helpers). One concern (projecting F1→F2 markdown);
   splitting relocates render helpers.
10. **check-defer-triggers.js (1059, CORE) — COHESIVE.** Defer-trigger predicate
    checker CLI; one tool.
11. **diagnostics.go (988, SRC) — COHESIVE.** 32 funcs, all secret redaction
    (`redact*`, `is*Key`); one concern.
12. **seam.go (976, SRC) — AMBIGUOUS.** The load-bearing seam apply pipeline
    (classify→apply→render→validate). Three "seed" funcs
    (`seedAgentModelDefaults`/`composeAgentsMd`/`seedRunShapeDefault`) are
    arguably extractable, but this is central orchestration; splitting risks the
    pipeline's cohesion. No acute evidence.
13. **classifier.go (960, SRC) — COHESIVE.** execro classifier; one concern.
14. **verify-f3-design-readiness.js (923, CORE) — COHESIVE.** One F3 verifier.
15. **emit.go (844, SRC) — COHESIVE.** 16 funcs; permconfig permission emitter.
    One concern (emit permission JSON + JS).

**Remainder (16–40, summarized by role):** all COHESIVE — verify-*/checker CLIs
(one concern each), overlay plugins (self-contained), recently-built F1/F2
family files (`f1_validator`/`f1_r1`/`f2_streak`/`renderer`/`store`/`claim` — each
a single feature module from the operator-visibility initiative), and CLI
command files (`profile`/`update`/`overlay_new`/`release_gate`). One additional
AMBIGUOUS-leaning candidate is `f2_streak.go`/`renderer.go` but both are
single-purpose modules just over the line.

---

## 3. Tally + ratio

| Disposition | Count | % | Files |
|-------------|------:|--:|-------|
| **Genuine refactor target** (real seam + evidence + act now) | **1** | 2.5% | doctor.go |
| Ambiguous (plausible seam, no acute evidence / high split cost) | 2 | 5% | state-lib.js, seam.go |
| Accept-as-cohesive | 37 | 92.5% | all others |

**Generous upper bound** (counting both ambiguous as genuine): **3/40 = 7.5%.**
Conservative lower bound: **1/40 = 2.5%.**

### Comparison to the panel-S3 probe (the operator's baseline)

**The S3 probe premise could not be sourced.** It appears exactly once in the
repo, as an aside in the `complexity-control` session memory
(`.../checkpoints/2026-07-28T07-52-46-slice1-corrected-framing.md` L31): *"the
panel-probe concurrency/perf blind-spots (0/39 real, models over-raise, correctly
filtered)."* No primary probe document, methodology, or raw panel output was found
under `researches/`, `docs/`, `.local/`, or `.opencode/state/`. **Treat "0/39" as
an unverified secondary premise, not established fact.** This is flagged as a
contradiction/gap below.

**Structural comparison (mechanism):** the two signals are DIFFERENT classes.
- **S3 (claimed):** *model judgment* over-raise — reviewers hallucinated
  concurrency/perf blind-spots. The failure mode is model over-confidence.
- **Complexity (measured):** *deterministic LOC* over-raise — a heuristic flags
  every cohesive large file. The failure mode is low correlation between raw LOC
  and "should refactor now."

**Outcome comparison (result):** **SAME outcome class — over-raise.** The
complexity signal's independently-derived ratio (1–3 of 40 actionable, ~2.5–7.5%)
is consistent with the claimed S3 ratio (0/39 ≈ 0%) **regardless of whether the
exact S3 number is right**: both describe a signal whose actionable yield is near
zero at high triage cost. The dominant driver here is that 52.5% of nominees are
embedded-corpus/overlay templates (Fix 2b asymmetry — source mirrors of excluded
rendered files) and most of the rest are cohesive feature modules from a mature
codebase.

---

## 4. Verdict

**The complexity signal is mostly OVER-RAISE — the same outcome class as the
claimed S3 probe, via a different mechanism.** ~1/40 genuine (2.5%), ~2/40
ambiguous (5%), ~37/40 cohesive (92.5%). The lone clear genuine target
(`doctor.go`) is already being incrementally addressed by the team's existing
split-checks-to-sibling-files pattern — it does not need an advisory gate to be
found. The signal's present-value precision is too low (≤7.5% even generously) to
justify surfacing 40 WARNs on day 1.

The corrected-framing checkpoint's own framing reinforces this: Slice 1 shipped
**only axis-1 (file_loc) of 4 designed axes**; axis-4 (agent-context — the
*dominant* axis per design) is ENTIRELY ABSENT, and the shipped LOC heuristic is
"the exact Goodhart-gameable measure the design warned against." A 1-of-4-axes,
LOC-only signal over-raising at ~92.5% is the *expected* behavior of that
truncated design, not an anomaly.

---

## 5. Recommendation for Fix 2's fork + Slice 4

**Fix 2: choose (b) skip/silent by default; do NOT choose (a) wire-suppression now.**

- **(a) wire accept-as-cohesive suppression now** would let the operator
  acknowledge ~37 cohesive files. But 37 acknowledgements of noise *is* the
  fatigue (boy-who-cried-wolf) — suppression treats the symptom (no acknowledge
  path) while leaving the disease (over-raise) live. It also builds durable
  disposition records around a 1-of-4-axes signal that the design itself intends
  to replace.
- **(b) default the check to skip/silent until the signal improves** is the lower-
  cost, lower-regret path. Concretely: keep the scanner + policy + parity fixture
  (the investment is not wasted — it is the foundation the other 3 axes plug
  into), but gate the advisory's *surface* (WARN) behind either `enabled: false`
  or a "silent/aggregate-only" mode until a second axis (preferably the dominant
  agent-context axis) raises precision.

**Slice 4 (disposition machinery): NOT worth scheduling now.** Defer. The whole
purpose of disposition machinery (accept-as-cohesive / split-defer records) is to
triage *actionable* nominees. At ≤7.5% actionable yield the machinery solves a
non-problem and would primarily record "cohesive" 37 times. Revisit Slice 4 only
after the signal gains the agent-context axis (or another precision-raising
feature) and the actionable yield is re-measured.

**Adjacent clean fix (ride-along, not gated):** Fix 2b's self-exemption asymmetry
— exempt `.opencode/` rendered but flag `templates/core/` source mirror — should
be resolved *as part of* the silent-mode decision, not as a reason to keep the
WARN live. Resolving it by also excluding `templates/core/**` (managed corpus)
would cut nominees from 40 → ~19 (internal + tests only), but even then the
yield is ~1/19 — so the asymmetry fix alone does NOT rescue the signal; it
merely reduces the noise floor.

---

## Findings

- **(finding)**: The installed PATH `vh-agent-harness` binary is stale relative to
  the working tree and does NOT carry check #20; `doctor` reports
  `dev-stale-embed WARN`. The feature is in-tree at `9362e11`. — source=`vh-agent-harness doctor` run + `git show 9362e11`, confidence=high, type=fact
- **(finding)**: The complexity scanner enumerates `git ls-files` (tracked-only);
  `refs/` is gitignored (`.gitignore:71`), so vendored reference bundles
  (729k-line `bundle_formatted.js`, dozens of `refs/opencode/**`) are NOT
  candidates. — source=`git check-ignore` + `internal/complexity/scanner.go` L54–83, confidence=high, type=fact
- **(finding)**: Day-1 nominated count = **40 files** (`observed > 500`,
  snapshot threshold), independently confirmed and matching the operator's stated
  "~40". Boundary is clean (509 nominated / 487 not). — source=replicated scan, confidence=high, type=fact
- **(finding)**: Category split — templates (embedded corpus + overlays) = 21
  (52.5%), internal Go source = 17 (42.5%), e2e test drivers = 2 (5%). — source=replicated scan, confidence=high, type=fact
- **(finding)**: Under the present-value standard, **1/40 genuine**
  (`doctor.go`), 2/40 ambiguous (`state-lib.js`, `seam.go`), 37/40
  accept-as-cohesive. Upper-bound actionable yield ≤7.5%. — source=structural sampling of 11/40 files (func/section grep) + role analysis of all 40, confidence=medium-high, type=inference
- **(finding)**: `doctor.go` (2102, 39 funcs) has real separable clusters
  (auto-gate validation, gitignore helpers) AND the team already incrementally
  splits checks to sibling files — it is a genuine but self-resolving target. — source=`grep '^func ' internal/cli/doctor.go` + sibling-file glob, confidence=high, type=fact
- **(finding)**: The S3 "0/39" baseline is referenced once in session memory but
  has NO primary source anywhere in the repo. — source=exhaustive grep of researches/docs/.local/.opencode/state, confidence=high, type=fact
- **(finding)**: The complexity signal and the claimed S3 probe are DIFFERENT
  mechanisms (deterministic LOC vs model judgment) but the SAME outcome class
  (over-raise; near-zero actionable yield). — source=comparison of measured ratio vs claimed S3 ratio, confidence=medium, type=inference
- **(inference)**: Fix 2 should choose (b) skip/silent; Slice 4 should NOT be
  scheduled now. — source=this triage, confidence=medium, type=inference

## Contradictions

- **The "0/39 panel-S3 probe" could not be sourced.** It is stated as prior
  settled knowledge in one session-memory checkpoint but no probe document,
  methodology, or raw output exists in `researches/`, `docs/`, `.local/`, or
  `.opencode/state/`. The verdict above does NOT depend on the exact S3 number
  (the independently-derived 1–3/40 ratio over-raises on its own), but the
  operator should not treat "0/39" as established canon. If a primary probe
  exists outside the repo (e.g., a session that was never persisted), it should
  be cited explicitly or the comparison reframed as "consistent-direction,
  unverified-magnitude."
- **Operator premise vs scanner reality (reconciled, not a true contradiction):**
  the operator cited "~40 files led by `doctor.go` (2102)"; the scan confirms 40
  and `doctor.go` is the largest *internal* file, but the overall largest
  nominee is `state-lib.js` (6147, a template). No contradiction once the
  template/source distinction is applied.

## Method reproducibility / artifacts

- Replication script: `tmp/complexity-scan/scan.sh` (bash; could not be executed
  via `exec-ro` because bash is not provably read-only — ran the equivalent
  `find`+`wc`+`grep`+`sort` pipeline directly, which is in the read-only set).
- Raw nominated list preserved at `tmp/complexity-scan/` (TSV). These are
  scratch artifacts; clean after operator review.

## Promotion targets (if the operator adopts the recommendation)

The recommendation, once adopted, should land in live docs rather than this
packet:
- `internal/cli/doctor_complexity.go` doc-comment / `README.agent.md` — describe
  check #20 as silent-by-default (or `enabled:false`) pending axis-2/4.
- `.vh-agent-harness/complexity-policy.yml` — flip `enabled: false` (or add a
  silent/aggregate-only mode) as the chosen Fork-2b mechanism.
- `docs/planning/backlog.md` — record Slice 4 (disposition machinery) as
  DEFER-with-trigger (trigger: a second complexity axis lifts actionable yield
  above a re-measured threshold).
- The `complexity-control` session memory `open-questions.md` / `decision-log.md`
  are currently blank — this triage's verdict should be recorded there as the
  Fix-3 answer (Fix 2 then defers to it per the checkpoint).

---

## Correction addendum (2026-08-04) — body framing superseded by the canonical "staged advisory hybrid" policy

**This is an appended correction. The original body below the `---` delimiter is
preserved verbatim as the historical record; the existing provenance header
(above the delimiter) already records that Fix 2b landed. Only this addendum is
new.**

The body's recommendation (§5: "Fix 2 choose (b) skip/silent; Slice 4 defer")
was ACTED ON — `complexity-policy.yml` was flipped to `enabled: false` in
`78e3610` (both `.vh-agent-harness/` and `templates/core/`; confirmed still
`enabled: false` at the time of this addendum). That part of the body matches
what shipped.

The body's FRAMING, however, has been superseded by the canonical complexity
policy articulated after this triage. The body frames the silence as a
PRECISION stopgap ("default to skip/silent UNTIL a second axis raises
precision"; "Revisit Slice 4 only after the signal gains the agent-context
axis"), implicitly modeling complexity as a signal that could graduate toward
gate-worthiness as its precision improves. Current policy is sharper and more
restrictive:

- **Staged advisory hybrid — complexity signals INFORM; they NEVER gate.** This
  is a DESIGN CONTRACT, not a temporary measure (`internal/schema/complexity_policy.go`
  design-contract doc comment; `internal/cli/doctor_complexity.go` carries the
  SACRED INVARIANT that check #20 is structurally incapable of returning
  `tierFail` — it returns `tierSkip`/`tierPass`/`tierWarn` only, and NEVER
  authorizes a transition).
- **WARN-only is permanent design, driven by Goodhart risk**, not a precision
  stopgap: the live `file_loc` metric is gameable (split-to-pass-LOC lowers the
  measured signal while worsening coupling), and that gaming is invisible until
  the agent-context axis ships. Even after that axis ships, the advisory nature
  is the design; precision gains re-enable the ADVISORY, they do not promote it
  to a gate. See `docs/checkpoints/2026-07-28-complexity-slice-1-framing-correction.md`
  ("This is not a temporary state; it is the design").
- **Resting path:** agent-context axis (axis-2) build → re-measure against the
  S3 baseline → re-enable the ADVISORY by editing BOTH `complexity-policy.yml`
  files (the armed-policy gotcha: `make update` does NOT propagate `enabled`
  from the template to the armed instance, so both must be edited by hand).

**Net:** read the body's §5 recommendation as the triage-time input that
motivated Fix 2b (acted on), but read the CANONICAL policy from
`internal/schema/complexity_policy.go` + `internal/cli/doctor_complexity.go` +
`docs/checkpoints/2026-07-28-complexity-slice-1-framing-correction.md`, not from
this body. The body's "Slice 4 disposition machinery — defer until precision
improves" stance is consistent with the resting path (disposition machinery
remains deferred), but its precision-→-gate-worthiness model is NOT current
policy.
