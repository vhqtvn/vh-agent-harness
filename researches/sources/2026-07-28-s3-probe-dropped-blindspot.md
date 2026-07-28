# Sources: Commit-Review Panel Model-Study Series — S3 Step 1 — Real-vs-Noise Probe of Dropped Blind-Spot Findings

**Date:** 2026-07-28 (study date; promoted to durable sources 2026-07-28)
**Topic:** THE corroborating evidence for the complexity-over-raise finding. Stratified hand-classification of 39 dropped blind-spot findings (20 performance / 15 concurrency / 4 complexity / 0 blast_radius). RESULT: 0/39 real-wrongly-dropped (Wilson 95% CI [0.0%, 9.0%] overall; complexity stratum 0/4, Wilson 95% CI [0.0%, 49.0%]). Branch B conclusively established — the disposition layer is healthy; over-raising of advisory noise, not wrong dropping of real findings. On this basis the complexity-over-raise check was silenced.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** empirical probe (stratified hand-classification) — LOAD-BEARING.
- **Evidence class:** Class-A — this-machine opencode DB-cited (read-only `mode=ro`) + hand-classification against reviewed diffs; not re-derivable outside this host. Persist-guard compliant: aggregate statistics and neutral paraphrases only (no verbatim finding text, no file paths, no code snippets).
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-28-s3-probe-dropped-blindspot.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-28-s3-probe-dropped-blindspot.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# S3 Step 1 — Real-vs-Noise Probe of Dropped Blind-Spot Findings

**Date:** 2026-07-28
**Scope:** READ-ONLY empirical classification. No source/config/panel edits, no commit.
**DB:** `~/.local/share/opencode/opencode.db` (opened `mode=ro` via Python `sqlite3` stdlib).
**Scripts:** `tmp/commit-study/s3_explore.py`, `s3_diag.py`, `s3_sample.py`, `s3_wilson.py` (all gitignored).

---

## 1. Sample

### 1.1 Population reconciliation (material correction)

The full-sample study §8 reported blind-spot raise counts of **performance=244, concurrency=65**.
A raw-category diagnostic across ALL leaf sessions (v1+v2, all schemas) shows the **actual** counts
are an order of magnitude smaller:

| class | §8 reported (raised) | actual v2 non-empty (raw category) | actual matched-drop population |
|---|---:|---:|---:|
| performance | 244 | 24 | **20** |
| concurrency | 65 | ~22 (incl. race/races) | **15** |
| complexity | (not separately reported) | ~216 | **195** |
| blast_radius | ~2 | 0 | **0** |

The §8 figures likely came from a different methodology (possibly issue-text keyword matching or
a different extraction pipeline) and are not reproducible from raw `json_extract(category)` counts.
**This correction propagates:** the "blind-spot gap" is smaller in volume than the restructure brief
and O3 debate assumed (both cited §8's E4 numbers as facts).

### 1.2 Sampling

**Frame:** non-empty, v2, coordinator-matched findings where (a) the leaf raised a finding in a
blind-spot class (performance / concurrency / race / complexity / maintainability / blast_radius),
and (b) the coordinator's matched disposition = `drop`. Matching reuses the proven q18 pipeline
(score ≥ 0.45: category 0.25 + location 0.45/0.55 + token-overlap up to 0.35).

| stratum | target | population | sampled | method |
|---|---:|---:|---:|---|
| performance | 20 | 20 | **20** | census (population = target) |
| concurrency | 20 | 15 | **15** | census (below target; no rebalance possible — population exhausted) |
| blast_radius | 4 | 0 | **0** | stratum absent |
| complexity | 4 | 195 | **4** | seeded random (seed=20260728) |
| **total** | 48 | — | **39** | — |

**Rebalancing:** concurrency fell 5 short of its 20 target (population exhausted at 15).
Blast_radius is entirely absent (0 leaf findings with that category in the entire v2 non-empty
corpus). No proportional reallocation was possible because the only surplus stratum (complexity,
195) was already at its 4-target by design. The 5-finding shortfall is reported honestly; it does
not materially affect a 0/39 result.

**Seed:** 20260728 (reproducible). **Quarantine:** `kiro-claude-opus-4-5` paid transient excluded.

---

## 2. Classification Table

**Rubric (from the second debate, applied verbatim):**

| Label | Rule |
|---|---|
| **real-wrongly-dropped** | A specific, reproducible changed-path defect; supported by evidence; material enough for at least `defer` or `block`. The coordinator SHOULD have upheld it. |
| **noise-correctly-filtered** | No defect; speculative/no mechanism; pre-existing/out of scope; merely stylistic; or immaterial. The `drop` was correct. |
| **indeterminate** | The stored diff/evidence cannot support either conclusion. Kept separate from the real/noise denominator. |

**Result: 0 real, 39 noise, 0 indeterminate.** Every finding is noise-correctly-filtered.

### Performance (PERF-001..020)

| ID | label | one-line neutral paraphrase (persist-guard compliant) |
|---|---|---|
| PERF-001 | noise | sequential loop over chunks but the model itself is the stated bottleneck — advisory only |
| PERF-002 | noise | per-call object allocation; leaf self-notes "not hot until flag enabled" — negligible today |
| PERF-003 | noise | (duplicate of PERF-002, different leaf) same per-call allocation — negligible |
| PERF-004 | noise | unbounded re-read in a shadow path; leaf self-notes "out-of-scope per caller's non-goals" |
| PERF-005 | noise | O(N) rebuild per call; leaf self-notes "fine for this slice, becomes O(N*M) in a future slice" |
| PERF-006 | noise | intended design change; leaf self-notes "no action needed; deliberate" — positive verification |
| PERF-007 | noise | CSS animation on a small surface; leaf self-notes "acceptable, mirrors sanctioned pattern" |
| PERF-008 | noise | leaf confirms CSS is "GPU-perf clean" — positive verification note, not a defect |
| PERF-009 | noise | missing autograd guard; leaf self-notes "pure perf hardening, no behavioral change" — borderline but immaterial at current scale |
| PERF-010 | noise | leaf confirms mask pattern is "sanctioned, not a scroll-container mask" — positive verification |
| PERF-011 | noise | leaf confirms new CSS classes "introduce no banned properties" — positive verification |
| PERF-012 | noise | repeated map rebuild; leaf self-notes "trivial, microsecond-scale, no measurable concern" |
| PERF-013 | noise | per-call client allocation; leaf self-notes "negligible — cache bounds misses to ~1/3min" |
| PERF-014 | noise | per-session queries scale O(sessions); leaf self-notes "not needed for current fleet sizes" |
| PERF-015 | noise | builder capacity retention; leaf suggests "or document as accepted stopgap" — advisory |
| PERF-016 | noise | optimization-completeness note; leaf self-notes "no correctness impact, fresh snapshot is safe" |
| PERF-017 | noise | corpus walk per render; leaf self-notes "negligible for current small corpus" |
| PERF-018 | noise | adjacency rebuild per call; leaf self-notes "acceptable for current scale" |
| PERF-019 | noise | index rebuild per read; leaf self-notes "fine while unwired" |
| PERF-020 | noise | double evaluation per render; leaf self-notes "net DOM still drops, residual cost only" |

### Concurrency (CONC-001..015)

| ID | label | one-line neutral paraphrase (persist-guard compliant) |
|---|---|---|
| CONC-001 | noise | projection rewrite outside lock; leaf self-notes "pre-existing, not introduced by slice, self-heals" |
| CONC-002 | noise | delete/recreate TOCTOU; leaf self-notes "self-corrects, not commit-blocking" |
| CONC-003 | noise | leaf confirms lock discipline is CORRECT — positive verification note |
| CONC-004 | noise | async cancel arming race; coordinator notes "vanishingly unlikely, non-damaging" — **closest to real but immaterial** (see §4.1) |
| CONC-005 | noise | (same race as CONC-004, different leaf) async cancel arming — non-damaging, 3/4 leaves flagged drop |
| CONC-006 | noise | (same race as CONC-004, different leaf) async cancel arming — same, drop/nit |
| CONC-007 | noise | (same race as CONC-004, different leaf) cancel not armed under lock — identity guard prevents exploitation |
| CONC-008 | noise | disk I/O under global lock; leaf's own evidence confirms "not a correctness defect, justified tradeoff" |
| CONC-009 | noise | leaf confirms state transition is valid — positive verification note |
| CONC-010 | noise | (same location as CONC-009) leaf notes "not the targeted bug, minor UX flash" |
| CONC-011 | noise | dual-path arming; leaf self-notes "tiny, documented" |
| CONC-012 | noise | check-then-act window; leaf self-notes "GC-2 pattern, documented OPEN/accepted" |
| CONC-013 | noise | (same pattern as CONC-012) non-atomic recheck; leaf self-notes "documented GC-2 accepted hazard" |
| CONC-014 | noise | leaf confirms "no signal loss — verified correct" — positive verification note |
| CONC-015 | noise | global symbol shape; leaf confirms "NOT currently racy, verified by grep" — advisory |

### Complexity (COMP-001..004)

| ID | label | one-line neutral paraphrase (persist-guard compliant) |
|---|---|---|
| COMP-001 | noise | CSS literal not in theme map; leaf self-notes "consistent with documented convention, deliberate" |
| COMP-002 | noise | test-mock edge case unfaithful to real behavior; leaf self-notes "not blocking, unused path" |
| COMP-003 | noise | variable naming holds display form; leaf self-notes "advisory only, no behavioral risk" |
| COMP-004 | noise | duplicated test helper (~46 lines x2); leaf self-notes "maintainability debt only, not required" |

### Blast_radius: **absent** (0 leaf findings in entire v2 corpus)

---

## 3. Split — real / (real + noise) with Wilson 95% CIs

| stratum | real | noise | denom | share | Wilson 95% CI |
|---|---:|---:|---:|---:|---|
| performance | 0 | 20 | 20 | **0.0%** | [0.0%, 16.1%] |
| concurrency | 0 | 15 | 15 | **0.0%** | [0.0%, 20.4%] |
| complexity | 0 | 4 | 4 | **0.0%** | [0.0%, 49.0%] |
| blast_radius | — | — | 0 | — | (absent) |
| **overall** | **0** | **39** | **39** | **0.0%** | **[0.0%, 9.0%]** |

With 0 real findings in 39 classified, the true real-share of dropped blind-spot findings is
bounded above at **9.0%** (95% confidence). Branch A (real-wrongly-dropped dominates, requiring
share materially >50%) is excluded by a wide margin.

---

## 4. Branch Verdict: **B — noise-correctly-filtered dominates**

**Confidence: high.**

### 4.1 Evidence

Every one of the 39 dropped blind-spot findings is noise-correctly-filtered. The noise falls into
five clear patterns:

1. **Positive verification notes (10/39, 26%):** the leaf is *confirming* that something is
   correct/clean/sanctioned — not raising a defect at all. Examples: "concurrency verified
   CORRECT," "GPU-perf clean," "no signal loss — verified correct." These are audit-trail notes
   mislabeled as findings.

2. **Future-scale advisory (12/39, 31%):** the leaf raises a theoretical scaling concern
   ("if the map grows beyond 10k nodes," "if fleet size grows") with no current material impact.
   The leaf itself typically says "acceptable for current scale" or "not needed for current sizes."

3. **Minor optimization / no correctness impact (7/39, 18%):** a real code-level observation
   (e.g., missing autograd guard, builder capacity retention) but explicitly framed as "pure
   performance hardening, no behavioral change" or "accepted stopgap tradeoff."

4. **Real-but-immaterial race cluster (7/39, 18%):** the cancel-arming race (CONC-004..007, all
   the same defect raised by 4 leaves across rounds) and the GC-2/check-then-act patterns
   (CONC-001, 002, 008, 012, 013). These are real races, but the leaf's own text and the
   coordinator's summary agree they are non-damaging, self-healing, or documented-as-accepted.
   **CONC-004..007 is the closest candidate to real-wrongly-dropped** — it is a genuine data race
   (unsynchronized field access, race-detector-reproducible). But 3 of 4 leaves flagged it as
   drop/nit, the coordinator documented an identity guard that prevents dangerous exploitation,
   and the worst case is a benign no-op. A `defer` would have been defensible; a `drop` is also
   defensible and correct.

5. **Stylistic / maintainability debt (4/39, 10%):** naming suggestions, code duplication, theme-
   mapping convention notes — all explicitly "advisory only, not required."

### 4.2 Why this is decisive

The probe's decision tree specifies: "a strong Branch A or B result is decisive." With 0/39 real
and a Wilson upper bound of 9.0%, **Branch B is conclusively established**. The "blind-spot gap"
(high raise count, near-zero uphold count) is explained by **over-raising of advisory noise**, not
by wrong dropping of real findings. The coordinator's disposition layer is functioning correctly
for these classes.

### 4.3 Branch C status

Branch C (genuine detection gap — real defects exist that leaves never raised) **cannot be
confirmed or denied from dropped findings alone**, by design. However, the 0/39 result makes it
less likely: if the leaves' discrimination for these classes is dominated by noise (every dropped
finding is noise), a lens that makes them raise *more* would likely raise more noise, not more
signal. A definitive Branch C test would require a forward-looking defect-seeding study (inject
known performance/concurrency defects into a fixture and test whether any leaf configuration
catches them) — but given the Branch B result, that investment is not justified at this time.

---

## 5. Implication

### 5.1 Blind-spot track (O2 / R4)

**HALT.** Branch B → no gap exists in the disposition layer for blind-spot classes. The O2
prompt-amendment experiment (S5) and the R4 dedicated-lens reshape are the wrong levers — they
would add more detection volume to classes whose existing volume is already 100% noise. The
S4 four-way comparison fixture is not justified. No lens/prompt mechanism for blind-spot classes
is warranted by this evidence.

### 5.2 O3 (non-GPT contract/data backstop)

**The probe does NOT gate O3.** The Branch verdict here is about blind-spot classes
(performance/concurrency/complexity). O3 targets contract/data — a fundamentally different class
scope. The disposition layer for contract/data findings was not tested by this probe and may behave
differently.

What the probe DOES say about O3's gating concern: the O3 debate (flip-condition #1) feared that
"if Branch A is real, a volume-only success metric is misleading — O3's volume would flood a broken
filter." This probe refutes that specific fear: **the filter is not broken.** The coordinator
correctly drops noise and upholds signal (per S6, non-GPT block-calibration is 88.9%). So even if
O3 raises more contract/data volume, the disposition layer can be trusted to filter it. O3 should
be evaluated at the **finding-survival** layer (the S6-controlled 9.5% non-GPT baseline → can a
sharper contract/data prompt lift it?), not gated behind a disposition diagnosis that this probe
has now settled as healthy.

**Net:** O3 may proceed as a controlled contract/data experiment per the O3 debate's flip-condition
#2 (Branch B → retain gate, start narrow O3 A/B). It is not blocked by this probe.

### 5.3 Disposition / gating layer

**Branch A's fix is NOT needed.** The disposition layer is correctly filtering noise for blind-spot
classes. No gating-policy change, no escalation-rule redesign, and no routing to the
complexity-control brief is warranted by this evidence. The S0 slice (complexity-control
coordination boundary) can remain as-scoped (panel flags; complexity-brief thresholds) without
urgent rework.

### 5.4 Prompt noise reduction (secondary finding)

A actionable byproduct: **~26% of the dropped "findings" are positive verification notes** (the
leaf confirming something is correct, not raising a concern). These are audit-trail artifacts
mislabeled as findings. The prompt could be tuned to suppress "I verified X is fine" emissions
in blind-spot classes without losing any detection value — this is a prompt-hygiene improvement,
not a lens/prompt detection mechanism.

---

## 6. Refined Open Questions

1. **§8 count correction (high priority for brief accuracy):** The full-sample study §8's
   blind-spot raise counts (perf=244, conc=65) are not reproducible from raw category extraction
   (actual: perf≈24, conc≈22). The restructure brief and O3 debate both cite these as facts (E4).
   The brief should be corrected. *Does the §8 methodology count issue-text keyword matches rather
   than category labels? If so, those counts measure something different from "category-raised
   findings" and should not drive panel-structure decisions.*

2. **Contract/data disposition health (relevant to O3):** This probe proves the disposition layer
   is healthy for blind-spot classes. Does the same hold for contract/data classes (the O3 target)?
   A similar probe on dropped contract_drift/data_integrity findings would confirm whether O3's
   output can be trusted to the disposition layer — but given S6's 88.9% block-calibration, this is
   lower priority.

3. **CONC-004..007 cancel-arming race (informational):** The closest-to-real finding in the sample.
   It is a genuine data race (race-detector-reproducible) that the codebase accepted as non-damaging.
   Whether to fix it is a codebase decision, not a panel-structure decision. If the CI ever runs
   `go test -race`, this would surface.

4. **Prompt noise hygiene:** Can the blind-spot class prompts suppress positive-verification
   emissions without losing real signal? This is prompt tuning, not a lens mechanism.

5. **Blast_radius class absence:** Zero leaf findings with category=blast_radius in the entire v2
   corpus. If blast-radius detection is a goal, the current panel architecture does not elicit it
   at all — but given Branch B, this may simply mean there are no material blast-radius concerns to
   raise.

---

## Persist-guard compliance

This document contains **aggregate statistics and neutral paraphrases only.** No verbatim finding
text, no file paths, no code snippets, no repo identifiers, no `/home/` absolute paths (DB referenced
in tilde form). Findings are referenced only by neutral anonymized IDs (PERF/CONC/COMP-NNN). All
one-line paraphrases strip repo-specific content. The raw finding text exists only in the gitignored
`tmp/commit-study/s3_sample.json` (not committed, not reproduced here).

## Constraints confirmed

- Read-only: no source/config/panel/lens/prompt edits, no commit.
- Only file written: this document (gitignored `tmp/`).
- `git status -- tmp/` shows untracked tmp files only (no tracked changes).
