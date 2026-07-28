# Sources: Commit-Review Panel Model-Study Series — Panel Slice Plan (S3/S6/O3 settled)

**Date:** 2026-07-27 (plan; carries 2026-07-28 reconciliation log) (study date; promoted to durable sources 2026-07-28)
**Topic:** the panel slice plan. Carries the 2026-07-28 reconciliation log: S3 = DONE → Branch B (HALT O2/R4); S6 = RESOLVED (standardize on finding-survival); O3 = build-as-experiment, NOT declare-effective (the R2 lens uplift is unmeasured — the A/B harness preflight-blocks and has never run).
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** plan (carries reconciliation log).
- **Evidence class:** Class-A — read-only research + planning slice; reasons over the DB-cited corpus.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-panel-slice-plan.md`
  (volatile; lives under the gitignored `tmp/` tree, not committed canon; promoted to
  durable sources on 2026-07-28 so the series is discoverable and citable).
- **Promotion basis:** part of the commit-review panel model-study series. The series
  lived only under `tmp/` and was undiscoverable (a researcher searching for "the
  panel probe" could not find it); promoted wholesale so the S3 probe's over-raise
  corroboration and its upstream/downstream memos are citable canon, not tmp-only.
- **Consuming decision:** complexity-control over-raise policy. The S3 probe
  (`2026-07-28-s3-probe-dropped-blindspot.md`) is the corroborating evidence on whose
  basis the complexity-over-raise check was silenced. The consuming policy is owned by
  a concurrent session and is not in `researches/decisions/` at promotion time.
- **Promotion commit:** the commit that introduces this file. A committed file cannot
  embed its own introducing commit's hash without an amend (the hash depends on the
  file's bytes); resolve via
  `git log -1 --format=%H -- researches/sources/2026-07-27-panel-slice-plan.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# 2026-07-27 Panel Slice Plan

> ## Reconciliation log — 2026-07-28 (S3 / S6 / O3 now SETTLED)
>
> This plan was drafted 2026-07-27 when S3 (probe), S6 (precision), and O3
> (R2 backstop) were open. They are now **settled** by two later memos in this
> directory. The section-level changes are listed here; the body below is
> updated in place.
>
> - **S3 = DONE → Branch B.** (`2026-07-28-s3-probe-dropped-blindspot.md`)
>   Stratified hand-classification of dropped blind-spot findings returned
>   **0/39 real-wrongly-dropped** (Wilson 95% CI [0%, 9%], high confidence).
>   The "blind-spot gap" is an illusion — over-raising of advisory noise, not
>   wrong dropping of real findings. → **HALT O2/R4**; no lens/prompt detection
>   mechanism is warranted. The disposition/gating layer is healthy; nothing
>   routes to the complexity-control brief.
>   *Noise breakdown:* 26% positive-verification notes mislabeled as findings,
>   31% future-scale advisory, 18% minor optimization (no correctness impact),
>   18% real-but-immaterial races (self-healing / documented-as-accepted), 10%
>   stylistic.
> - **S6 = RESOLVED.** (`2026-07-27-gpt-precision-rederivation.md`) The 94% vs
>   42.9% "divergence" was a **naming collision, both correct**. 94% =
>   *block-calibration* = P(coord-block | leaf-block) = 208/220 ("when GPT
>   blocks, is the coordinator right?"). 42.9% = *finding-survival* =
>   P(coord-upheld | matched) = 291/668 ("of everything raised, what's
>   actionable?"). Controlled co-present finding-survival 43.2% confirms the
>   42.9% aggregate is **uninflated**. Non-GPT weakness is **volume/recall**
>   (finding-survival 9.5% vs GPT 38.3%, 4× gap), NOT calibration
>   (block-calibration 88.9% vs 92.2%, negligible). **Standardize on
>   finding-survival for panel decisions.**
> - **O3 = build-as-experiment, NOT declare-effective.** The R2 lens uplift is
>   **unmeasured**: the A/B harness
>   (`2026-07-28-r2-upheld-volume-harness.py`) preflight-blocks (no labeled
>   sample + model API key in this environment) and has never run. O3 requires
>   an A/B on a labeled contract/data sample showing a **finding-survival
>   delta** (behavioral closure) before it counts as proven. Do not ship O3 as
>   "the GPT backstop" on an assertion.
>
> **Section changes made:**
> - §1 Direction Summary — rewritten (O2/R4 halted on Branch B; O3 demoted to
>   build-as-experiment; S1 schema prep = implemented Track-1).
> - §2 S0 — added status note (disposition healthy; S0 stays as-scoped).
> - §2 S1 — added status note (schema_v2 implemented; held on commit-gate-lane
>   conflict).
> - §2 S2 (O3) — reframed as build-as-experiment with an unmeasured uplift.
> - §2 S3 — marked DONE → Branch B with the result stats.
> - §2 S4, S5 — marked HALTED (Branch B; no detection gap to close).
> - §2 S6 — marked RESOLVED (naming collision; standardize on finding-survival).
> - §3 dependency graph — annotated with resolved states.
> - §4 decision tree — recorded Branch B as the branch actually taken.
> - §5 carried open-research — S6 marked resolved.
>
> No source/config/panel edits were made. This is a gitignored `tmp/` doc patch
> only.

---

## 1. Direction Summary

**Updated 2026-07-28 — see reconciliation log at top.** The pre-S3 probe (S3)
ran and returned **Branch B (noise-correctly-filtered dominates)**: 0/39
dropped blind-spot findings were real (Wilson 95% CI [0%, 9%]). The "blind-spot
gap" is an illusion — over-raising of advisory noise, not wrong dropping of
real findings. Consequences:

- **HALT O2 (prompt amendment) and R4 (5-leaf reshape).** No lens/prompt
  detection mechanism for blind-spot classes is warranted: adding detection
  volume to classes whose existing dropped volume is 100% noise would only add
  noise.
- **Disposition/gating layer is healthy.** No gating-policy change, no
  escalation redesign, and no routing to the complexity-control brief (S0 stays
  as-scoped).
- **O3 (non-GPT R2 contract/data backstop) is demoted to a build-as-experiment.**
  It is NOT declared effective: the R2 lens uplift is unmeasured (the A/B
  harness preflight-blocks, never run). O3 may proceed as a controlled
  contract/data experiment ONLY after it demonstrates a **finding-survival
  delta** on a labeled sample (behavioral closure). See S2 below.
- **S1 (schema_v2 prep) is implemented** as Track-1 and lands as its own
  reviewed commit (decision-record memo:
  `2026-07-28-schema-v2-decision-record.md`); it is additive /
  backward-compatible / domain-free and does not by itself change behavior
  (defaults reduce to current).

S6 (GPT precision) is also resolved (naming collision; standardize on
finding-survival). The plan's open questions on blind-spot detection and
precision are now closed; the only live experiment is O3's survival-coupled A/B.

## 2. Slice List & Dependencies

**S0: Complexity-Control Coordination (Additive Prep)**
- **Scope:** Define the boundary between panel blind-spot flagging and the separate complexity-control brief.
- **Settles:** Panel *detects/flags* complexity; complexity-brief *thresholds/rejects*.
- **Gates:** pre-S3 Probe.
- **Signal:** Documented boundary consensus.
- **Risk:** Bleed between panel scope and complexity-control logic.
- **Status (2026-07-28):** Unchanged — the S3 probe (Branch B) confirmed the
  disposition layer is healthy for blind-spot classes, so S0 stays as-scoped.
  No urgent rework; nothing from the blind-spot track routes here.

**S1: Schema + Config Shape (Additive Prep) — Track-1 schema_v2**
- **Scope:** Introduce `role`, `lens`, `class_owner`, `quorum`, and `escalation`
  disposition states to the panel JSON schemas (schema_version 1→2): an
  additive `panel` block carrying `roles`, `lenses`, `leaf_roles`,
  `class_ownership`, a `quorum` rule `owner_or_redundant`, and
  `escalation_disclosures[]`.
- **Settles:** The structural capacity to route specialized prompts and weigh
  findings by class ownership without hardcoding domain rules.
- **Gates:** O3, O2, pre-S3 Step 2.
- **Signal:** Schema validation passes; unit tests confirm class-owner quorum
  routing works; existing 4-leaf default parses are unaffected (clean-omit).
- **Risk:** Breaking existing 4-leaf default parses (mitigated: additive +
  defaults reduce to current behavior).
- **Status (2026-07-28):** **Implemented + verified** (`go test` / `gofmt` /
  `go vet` pass). **Held** on the commit-gate-lane file conflict; lands as its
  own reviewed commit when that lane clears. Decision-record:
  `2026-07-28-schema-v2-decision-record.md`.

**S2: O3 Non-GPT R2 Backstop (BUILD-AS-EXPERIMENT — NOT declare-effective)**
- **Scope:** Repoint a redundant uniform leaf into a dedicated non-GPT R2
  (contract/data-integrity) lens.
- **Status:** STAGED proposal only
  (`2026-07-28-r2-contract-data-lens-prompt.md`). The R2 lens prompt is
  designed; the measurement harness exists
  (`2026-07-28-r2-upheld-volume-harness.py`) but **preflight-blocks** (no
  labeled sample + model API key in this environment) and has **never run**.
  The finding-survival uplift is therefore **unmeasured**.
- **Settles:** Whether a sharper contract/data prompt lifts non-GPT
  finding-survival (controlled baseline 9.5%) toward GPT's 38.3%. **NOT yet
  settled.**
- **Gates:** O3 counts as proven ONLY after an A/B on a labeled contract/data
  sample shows a finding-survival delta (behavioral closure: `result: proven`).
  Do NOT ship O3 as "the GPT backstop" on an assertion.
- **Signal:** Treatment upheld-volume (finding-survival) meaningfully exceeds
  baseline uniform-prompt on owned classes, WITHOUT collapsing upheld-precision
  below a pre-set floor.
- **Risk:** Non-GPT models may lack the precision to execute the R2 prompt
  effectively; the uplift may be capability-bound, not prompt-recoverable.
- **Note:** The S3 probe does NOT gate O3 — it targets blind-spot classes,
  whereas O3 targets contract/data. The probe refutes only the fear that O3's
  volume would flood a broken filter (the filter is healthy: non-GPT
  block-calibration 88.9%).

**S3: pre-S3 Probe Step 1 — Real-vs-Noise Classification (DONE → Branch B)**
- **Scope:** Hand-classify dropped coordinator findings for blind-spot classes.
- **Result (2026-07-28):** **0/39 real-wrongly-dropped, 39 noise-correctly-
  filtered, 0 indeterminate.** Wilson 95% CI for real-share [0.0%, 9.0%].
  Branch A (real-wrongly-dropped dominates) excluded by a wide margin.
  **Verdict: Branch B — noise-correctly-filtered dominates; high confidence.**
- **Noise breakdown:** 26% positive-verification notes mislabeled as findings;
  31% future-scale advisory; 18% minor optimization (no correctness impact); 18%
  real-but-immaterial races (self-healing / documented-as-accepted); 10%
  stylistic.
- **Settles:** The blind-spot "gap" is an illusion — over-raising of advisory
  noise, not wrong dropping of real findings. The disposition/gating layer is
  healthy.
- **Consequence:** HALT S4 and S5 (no detection gap to close). No lens/prompt
  mechanism for blind-spot classes is warranted.
- **Source:** `2026-07-28-s3-probe-dropped-blindspot.md`.

**S4: pre-S3 Probe Step 2 — The 4-Way Comparison (HALTED — Branch B)**
- **Scope:** *Was conditional on S3 proving a detection gap.* S3 returned
  Branch B (no gap). The 4-way comparison fixture is therefore **not
  justified** — HALTED.

**S5: O2 Prompt-Amendment Experiment (HALTED — Branch B)**
- **Scope:** *Was conditional on S3 proving a detection gap.* S3 returned
  Branch B (no gap). Adding blind-spot classes to uniform prompts would add
  detection volume to classes whose existing dropped volume is 100% noise.
  **HALTED.**

**S6: GPT Controlled-Precision Re-derivation (RESOLVED)**
- **Scope:** Re-run the co-present controlled query with matched-finding
  denominators persisted.
- **Result (2026-07-27):** **RESOLVED — naming collision, both figures
  correct.**
  - **94% = block-calibration** = P(coord-block | leaf-block) = 208/220 = 94.5%
    ("when GPT blocks, is the coordinator right?").
  - **42.9% = finding-survival** = P(coord-upheld | matched) = 291/668 = 43.6%
    ("of everything raised, what's actionable?").
  - Controlled co-present finding-survival = 43.2% (gpt-latest), confirming the
    42.9% aggregate is **uninflated** (Δ ≤ 1.1pp).
- **Settles:** Non-GPT weakness is **volume/recall** (finding-survival 9.5% vs
  GPT 38.3%, 4× gap), NOT calibration (block-calibration 88.9% vs 92.2%,
  negligible). **Standardize on finding-survival for panel decisions**; use
  block-calibration only for seat-trust.
- **Source:** `2026-07-27-gpt-precision-rederivation.md`.

## 3. Dependency Graph

```text
[S0: Complexity Coordination]  (status: unchanged — disposition healthy)
    │
    ├──> [S1: Schema_v2 Prep] (status: implemented + verified; HELD on lane conflict)
    │        └──> [S2: O3 R2 Backstop] (status: build-as-experiment; uplift UNMEASURED)
    │
    └──> [S3: pre-S3 Probe Step 1]  (status: DONE → Branch B; 0/39 real)
            │
            ├── (Branch A: Real-Wrongly-Dropped)   — NOT TAKEN (excluded, Wilson ≤9%)
            ├── (Branch B: Noise-Correctly-Filtered) ✅ TAKEN → HALT O2/R4. No gap.
            └── (Branch C: Detection Gap)          — NOT TAKEN
                    ├──> [S4: 4-way compare]        — HALTED (no gap)
                    └──> [S5: O2 Prompt-Amendment]  — HALTED (no gap)

[S6: GPT Precision Re-derivation] (status: RESOLVED — naming collision)
    └──> Standardize on finding-survival (informs O3's A/B metric)
```

## 4. Decision Tree for the Probe
- **Trigger:** Evaluate the dropped findings in Concurrency, Performance, and Blast Radius/Complexity.
- **Branch A: Real-wrongly-dropped dominates.** → Stop lens experiments; redesign disposition policy.
- **Branch B: Noise-correctly-filtered dominates.** → Stop lens experiments; no blind-spot mechanism is needed.
- **Branch C: Genuine detection gap exists.** → Proceed to S4 and S5.

**RESULT (2026-07-28): Branch B taken.** 0/39 dropped blind-spot findings were
real-wrongly-dropped (Wilson 95% CI [0%, 9%]). The gap is an illusion: models
over-raise advisory noise and the coordinator correctly filters it. O2/R4 are
halted; the disposition layer needs no change.

## 5. Carried Open-Research List
1. **GPT Precision — RESOLVED (was #3).** The 94% vs 42.9% divergence was a
   naming collision, not a data conflict. Standardize on finding-survival;
   use block-calibration for seat-trust only.
   (`2026-07-27-gpt-precision-rederivation.md`)
2. **python3-sqlite3 Read-Isolation:** Captured as a DEFER
   (`.local/coordinator/tasks/defer-harness-read-isolation.json`, status draft).
3. **O3 finding-survival A/B (NEW, the one live experiment).** Does the R2
   contract/data prompt lift non-GPT finding-survival (9.5% baseline) on a
   labeled sample? Gated on running the A/B harness (currently preflight-blocks).

## 6. What is Explicitly NOT in this Plan
- **NO 5-leaf R4 commitment:** R4 was a conditional candidate surviving only if
  the probe validated a detection gap. S3 returned Branch B → **R4 is confirmed
  NOT committed.**
- **NO commit-gate changes:** S1 schema_v2 changes do not alter `commit-gate.sh`
  or active blocking policy; the quorum rule `owner_or_redundant` reduces to
  current behavior under defaults.
- **NO complexity-threshold design:** This plan only detects/flags complexity;
  threshold and rejection logic are deferred to the separate complexity-control
  brief.
- **NO read-isolation work:** Tracked entirely via the existing DEFER task.
- **NO empty-rate-as-quality:** Empty rates are classified strictly as transport
  failures, not model quality signals.
- **NO O3-as-proven-backstop:** O3 is build-as-experiment until its A/B shows a
  finding-survival delta.
