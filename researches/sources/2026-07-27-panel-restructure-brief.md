# Sources: Commit-Review Panel Model-Study Series — Panel Restructure Brief (self-derived, NON-EMPTY basis)

**Date:** 2026-07-27 (brief; carries 2026-07-28 reconciliation log) (study date; promoted to durable sources 2026-07-28)
**Topic:** the self-derived panel restructure brief on a NON-EMPTY basis. Carries the 2026-07-28 reconciliation log: §8/Table-4 blind-spot counts corrected (issue-text keyword matching, not category labels); blind-spot track = Branch B (no gap); S6 figures reconciled.
**Series:** [`./2026-07-27-commit-review-panel-model-study-series.md`](./2026-07-27-commit-review-panel-model-study-series.md) (commit-review panel model-study
series, 2026-07-27 → 2026-07-28).

## Provenance

- **Source type:** brief (self-derived; carries reconciliation log).
- **Evidence class:** Class-A — independent re-derivation from the live opencode DB (read-only); not re-derivable outside this host.
- **Original location:** `tmp/agent-runs/review-model-comparison/2026-07-27-panel-restructure-brief.md`
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
  `git log -1 --format=%H -- researches/sources/2026-07-27-panel-restructure-brief.md`.
- **Status:** durable source packet. Internal `tmp/` cross-references to peer memos
  resolve to the promoted canon paths listed in the series index above.
- **Faithfulness:** the body below the `---` delimiter is a byte-for-byte verbatim copy of the original `tmp/` study; only this provenance header is new — the study's own title, internal provenance, and cross-references to peer memos are unchanged.

---

# Commit-Review Panel — Restructure Brief (self-derived, NON-EMPTY basis)

**Author:** build agent (read-only research + planning slice, 2026-07-27).
**Source of truth:** independent re-derivation from the live opencode DB.
**Status:** BRIEF ONLY. No source/config/panel edits were made. The only file
written is this one (gitignored `tmp/`).

> ## Reconciliation log — 2026-07-28 (§8 counts corrected + blind-spot track resolved)
>
> Two later memos in this directory settle items this brief framed as open.
>
> - **§8 / Table 4 blind-spot counts corrected.** The upstream full-sample
>   study §8 (which this brief's blind-spot framing inherited) reported
>   **performance=244, concurrency=65** RAISED — but that came from
>   **issue-TEXT keyword matching, not category labels** (≈10× inflation for
>   perf/conc). The S3 probe's raw-category diagnostic
>   (`2026-07-28-s3-probe-dropped-blindspot.md` §1.1) gives the actual v2
>   non-empty category-label counts: **performance≈24, concurrency≈22,
>   complexity≈216, blast_radius=0**. This brief's own Table 4
>   (coordinator-merged basis) was already far below the §8 counts; a per-row
>   correction note is added at Table 4.
> - **Blind-spot track = Branch B (no gap).** The S3 probe hand-classified 39
>   dropped blind-spot findings: **0/39 real-wrongly-dropped** (Wilson 95% CI
>   [0%, 9%]). The "blind-spot gap" is an illusion — over-raising of advisory
>   noise. **HALT O2/R4**; no lens/prompt mechanism is warranted (see PART B §2
>   and open-research §5.1 updates).
> - **No conclusion changes.** The gap was thinner than assumed AND entirely
>   noise. This brief's blind-spot *direction* ("prompt gaps no model-swap
>   fixes") accidentally accorded with reality — but the *prescription* (a
>   dedicated R4 lens) is now retracted: Branch B says no detection mechanism
>   is warranted at all.
> - **S6 precision figures reconciled** (see the slice-plan reconciliation
>   log): 94% = block-calibration; 42.9% = finding-survival; both correct;
>   standardize on finding-survival.
>
> No source/config/panel edits were made. Gitignored `tmp/` doc patch only.

> **Headline correction up front.** The prior study
> (`tmp/agent-runs/review-model-comparison/2026-07-27-model-audit-quality-study.md`)
> **systematically understated GPT-class value on three independent axes**
> (precision, coverage, unique-catches), all because of a contaminated matched
> denominator and participation-confounded coverage. The single most important
> number for the restructure: **34.5% of all block-catching rounds are caught
> *only* by a GPT-class model.** GPT is simultaneously the strongest
> block-finder and the most quota-fragile — so the panel must be designed to
> *backstop* GPT, not treat it as a redundant 4th slot the prior recommended.

---

## 0. Provenance, keying, and data access (verified, not fabricated)

- **DB used:** `~/.local/share/opencode/opencode.db` (the operator's original,
  24 GB, opened `mode=ro` via Python's `sqlite3` stdlib v3.45.1 inside the dev
  container — `sqlite3` the binary is not installed in the container and is not
  in the host read-only inspection set, so queries ran as
  `vh-agent-harness exec python3 tmp/commit-study/qNN.py`). I did **not** use
  the copy at the other repo's `tmp/commit-study/opencode.db` (also 24 GB) to
  avoid naming a consumer repo path anywhere in written output.
- **`sqlite3` confirmation:** ran successfully for me (see starter table below).
  No denial.
- **Starter query result (keying sanity, `agent LIKE 'commit-reviewer-%'`,
  `HAVING n>=20`):**

  | model.id | n | empty |
  |---|---:|---:|
  | kiro-claude-opus-4-6:free | 2305 | 371 |
  | glm-5.2-high | 1753 | 0 |
  | gpt-latest:free | 1119 | 23 |
  | grok | 1118 | 13 |
  | grok-4.3 | 975 | 9 |
  | gpt-5.5:free | 955 | 9 |
  | glm-5.1 | 318 | 0 |
  | gpt-5.4 | 257 | 4 |
  | kiro-claude-opus-4-5:free | 188 | 8 |
  | claude-opus-4-8 | 141 | 0 |
  | gpt-5.4:free | 140 | 0 |
  | claude-opus-4-7 | 124 | 3 |
  | glm-5.2 | 120 | 0 |
  | grok-4.5 | 30 | 4 |
  | gemini-3.1-pro-preview:free | 30 | 0 |
  | kiro-claude-opus-4-5 (**paid**) | 21 | **21** |

- **Kiro transient quarantine (CONFIRMED distinct rows):**
  - `kiro-claude-opus-4-6:free` — 2305 runs, 06-08→07-25 (48 days), 16.1% empty.
  - `kiro-claude-opus-4-5:free` — 188 runs, 07-25→07-27 (3 days), 4.5% empty (thin/provisional).
  - `kiro-claude-opus-4-5` **(paid, no `:free`)** — 21 runs, **single day 07-25, 21/21 = 100% empty** — provider glitch on the paid id. **QUARANTINED; never defines a verdict.**

- **Schema learned:** `session.parent_id` links leaves (`commit-reviewer-a/b/c/d`)
  to their coordinator (`agent='commit-reviewer'`). Findings live in the final
  assistant message's ```` ```json ```` block. Two result schemas exist:
  - **v2** `commit-review-result.v2`: `findings[]` with per-finding
    `disposition ∈ {block, defer, drop}` → the upheld/overturned signal lives here.
  - **v1** `commit-review-result.v1` (era **2026-05-25 → 2026-06-08**): uses
    `blocking_issues[]` + `followups[]`, **no `disposition` field at all** —
    unscoreable on upheld-precision. Transition ~June 6–8.

- **Parse coverage:** coordinators 2908/3026 = **96.1%**; leaf result JSON
  parsed on 5767/9485 parented leaves (empties and v1-era leaves carry no
  scorable `findings[]`).

- **Match methodology (the load-bearing choice, stated honestly):** explicit
  leaf attribution (`source_leaf` 4.3%, `leaf` 2.2% of coord findings) is too
  rare to use, so each leaf finding is matched to a coordinator finding by
  **(canonical category agreement) + (file-path match, line numbers stripped)
  + issue-token overlap**, score ≥ 0.45. Admit rate (fraction of leaf findings
  that find a coord match) is 94–99%, consistent with the coordinator merging
  nearly all leaf findings and tagging disposition. **This is a
  per-leaf-finding ledger**, not converged-attribution (the prior credited
  every leaf that converged on a shared coord finding — see cross-check §X.3).

- **Operator directive honored:** every quality metric below is on
  **NON-EMPTY v2 leaves** (`tokens_output>0`). Empty-rate is reported only as a
  transport-reliability column, never as a ranking criterion.

---

## 1. Re-derived scorecard (PART A)

### Table 1 — Model quality on NON-EMPTY v2 leaves (the load-bearing table)

Ranked by **upheld-precision** = (matched findings the coordinator upheld) /
(matched findings). `matched` = leaf findings that reached the coordinator's
merged disposition. All on `tokens_output>0`, schema v2.

| model | v2NE | block+splt% | fnd/leaf | matched | upheld | drop | **uph-prec** | overturn% |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| **gpt-latest:free** | 811 | **22.4%** | 0.81 | 646 | 277 | 369 | **42.9%** | 57.1% |
| **gpt-5.5:free** | 616 | **16.6%** | 0.74 | 435 | 125 | 310 | **28.7%** | 71.3% |
| grok | 706 | 3.0% | 2.43 | 1642 | 176 | 1466 | 10.7% | 89.3% |
| glm-5.1 *(thin)* | 44 | 4.5% | 3.34 | 142 | 14 | 128 | 9.9% | 90.1% |
| glm-5.2-high | 890 | 3.5% | 2.38 | 2076 | 194 | 1882 | 9.3% | 90.7% |
| grok-4.3 | 342 | 1.8% | 0.54 | 172 | 15 | 157 | 8.7% | 91.3% |
| kiro-claude-opus-4-5:free *(thin, 3-day)* | 109 | 1.8% | 1.28 | 130 | 11 | 119 | 8.5% | 91.5% |
| glm-5.2 *(thin)* | 54 | 7.4% | 3.22 | 173 | 14 | 159 | 8.1% | 91.9% |
| **kiro-claude-opus-4-6:free** | 891 | 1.2% | 1.96 | 1684 | 92 | 1592 | **5.5%** | 94.5% |

**Read:**
- **GPT-class are tight, high-precision graders.** `gpt-latest:free` raises
  only 0.81 findings/leaf but 42.9% of those survive as blocks/defers — by far
  the strongest signal-per-finding. `gpt-5.5:free` is the #2 precision and the
  **highest critical-severity share** of upheld (19%).
- **grok / glm-5.2-high are loud and broad** (2.4 findings/leaf, ~90%
  overturned) but still contribute real volume: grok 176 upheld, glm 194 — the
  largest non-GPT upheld counts.
- **kiro-4-6:free is the weakest on every quality axis** (5.5% precision,
  94.5% overturned, 1.2% block-rate) **and** its upheld findings are **57%
  info-severity** (Table 9) — the lowest-signal upheld pool.

**Below threshold / unscoreable (reported, not headlined):**
- **v1-only, unscoreable** (no `disposition`): `gpt-5.4`, `gpt-5.4:free`,
  `claude-opus-4-8`, `claude-opus-4-7`, `claude-opus-4-6-thinking:free`,
  early `glm-5.1`, early `gpt-5.5:free`, early `grok-4.3` (all pre-June-8 era).
- **Thin:** `gemini-3.1-pro-preview:free` (v2NE=13; mostly the `debate-critic`
  fixed-role specialist anyway), `grok-4.5` (v2NE=17, 10.3% empty).

### Table 1b — Empty-rate (TRANSPORT reliability, NOT a quality criterion)

Per operator directive, this is **not** used for ranking. It is one named
follow-up (transport hardening, §5).

| model | empty/total | note |
|---|---|---|
| kiro-claude-opus-4-5 (paid) | 21/21 = 100% | 1-day transient, quarantined |
| grok-4.5 | 3/29 = 10.3% | thin |
| **kiro-claude-opus-4-6:free** | **366/2277 = 16.1%** | **persistent baseline** (flat 11–20% weekly, not a glitch) |
| kiro-claude-opus-4-5:free | 6/186 = 3.2% | thin, 3-day |
| gpt-latest:free | 20/1107 = 1.8% | solid |
| grok | 13/1106 = 1.2% | solid |
| gpt-5.5:free, grok-4.3, glm-* | ≤1.0% | solid |

### Table 2 — Unique upheld catches (ROUND-level: model was the *sole* upholder)

| model | part-rounds | upheld-rounds | **solo-rounds** | solo/part |
|---|---:|---:|---:|---:|
| **gpt-latest:free** | 811 | 210 | **87** | **10.7%** |
| **gpt-5.5:free** | 616 | 111 | **63** | **10.2%** |
| glm-5.2 *(thin)* | 54 | 12 | 7 | 13.0% |
| glm-5.1 *(thin)* | 44 | 11 | 8 | 18.2% |
| grok | 706 | 138 | 27 | 3.8% |
| glm-5.2-high | 890 | 151 | 32 | 3.6% |
| kiro-claude-opus-4-6:free | 891 | 70 | 13 | **1.5%** |
| grok-4.3 | 342 | 13 | 1 | 0.3% |
| kiro-claude-opus-4-5:free *(thin)* | 109 | 9 | 1 | 0.9% |

**Read:** among well-sampled models, **GPT-class has the highest unique-catch
rate** (gpt-latest 10.7%, gpt-5.5 10.2%) — they catch blocks no other leaf
catches, more often per-round than grok/glm/kiro. `kiro-4-6` is the weakest
unique contributor (1.5%) despite the highest participation — it mostly echoes
what others already caught. *(This directly contradicts the prior §4 — see
cross-check.)*

### Table 3 — Capability fingerprint (canonical-bucket share of each model's UPHELD findings)

| bucket | gpt-latest | gpt-5.5 | grok | glm-5.2-high | kiro-4-6 |
|---|---:|---:|---:|---:|---:|
| **contract_drift** | **52.3%** | **36.0%** | 8.5% | 7.2% | 7.6% |
| correctness | 7.9% | 15.2% | 22.7% | 28.9% | 30.4% |
| test_coverage | 17.0% | 4.8% | 22.7% | 25.3% | 23.9% |
| **security** | 7.6% | **15.2%** | 4.5% | 6.7% | 9.8% |
| **data_integrity** | 10.1% | 10.4% | 2.8% | 2.1% | 3.3% |
| complexity | 0.0% | 0.0% | 3.4% | 4.6% | 1.1% |
| dependency | 0.7% | 4.8% | 7.4% | 2.6% | 5.4% |
| doc_drift *(noise)* | 0.4% | 2.4% | 11.9% | 9.3% | 12.0% |
| **TOTAL upheld** | **277** | **125** | **176** | **194** | **92** |

**Read (lens assignment evidence):**
- **Contract + data-integrity = GPT territory.** gpt-latest is 62%
  contract+data_integrity; gpt-5.5 is 46%. No non-GPT model approaches this —
  the contract lens *is* the GPT-dependent capability.
- **Security is genuinely spread** (gpt-5.5 15%, kiro-4-6 10%, glm-5.2 36%
  but thin n=14) — a security lens can be made GPT-independent.
- **grok / glm-5.2-high are the broad correctness/test workhorses** — least
  specialized, most volume.
- **kiro-4-6's only distinctive edge is a security-leaning fingerprint (9.8%)**
  — the single argument against fully retiring it. Everything else about it is
  weak (precision, unique, severity, empty-rate).

### Table 4 — Blind spots & noise classes (coordinator findings, ALL models)

| class | raised | upheld | uph% | verdict |
|---|---:|---:|---:|---|
| **blast_radius** | **~0** | 0 | — | **#1 blind spot — prompt never asks** |
| performance | 39 | 3 | 7.7% | near-absent |
| concurrency/race | ~18–24 | 2 | ~10% | near-absent |
| complexity/maintainability | 333 | 22 | 6.6% | raised, low-yield |
| **doc_drift** | **2528** | 46 | **1.8%** | **dominant noise — stop soliciting** |
| style | 623 | 1 | 0.2% | pure noise |
| testability | 61 | 0 | 0.0% | pure noise |
| scope_violation | 102 | 3 | 2.9% | noise |
| overclaim | 76 | 1 | 1.3% | noise |
| contract_drift | 293 | 208 | **71.0%** | highest-yield class |
| data_integrity | 82 | 64 | **78.0%** | highest-yield class |
| security | 178 | 74 | 41.6% | high-yield |
| dependency_risk | 189 | 42 | 22.2% | decent |

**Read (direction, retained):** blast-radius / concurrency / performance are
**prompt gaps no model swap fixes** — the *direction* is right. **However, see
the §8 correction + Branch B note directly below: the prescription (a dedicated
lens) is retracted — the dropped volume is 100% noise, so no detection mechanism
is warranted.** Symmetrically, `doc_drift` + `style` + `overclaim` +
`testability` = **3387 raised findings for 48 upheld (1.4%)** — the prompt
should stop soliciting them.

> **§8 correction + Branch B resolution (added 2026-07-28).** The upstream
> full-sample-study §8 reported **performance=244, concurrency=65** RAISED,
> which framed this as a large gap. That was a **methodology error**: §8 matched
> **issue-TEXT keywords, not category labels** (≈10× inflation for perf/conc).
> The S3 probe's raw-category diagnostic gives the actual v2 non-empty
> category-label counts (leaf-side): **performance≈24, concurrency≈22,
> complexity≈216, blast_radius=0**. This brief's Table 4 above is on a
> coordinator-merged basis and was already far below the §8 counts, so its rows
> stand as-is; treat the §8 244/65 figures as superseded wherever they appear
> (including the cross-check §X row citing the prior's "~0/0").
>
> **Branch B (decisive):** the S3 probe hand-classified 39 dropped blind-spot
> findings — **0/39 real-wrongly-dropped** (Wilson 95% CI [0%, 9%]). The gap is
> entirely noise (26% positive-verification notes, 31% future-scale advisory,
> 18% minor optimization, 18% real-but-immaterial races, 10% stylistic). **No
> lens/prompt detection mechanism for these classes is warranted.** The PART B
> R4 lens design below is therefore **SUSPENDED** (see §2 and §5.1).
> Source: `2026-07-28-s3-probe-dropped-blindspot.md`.

### Table 5 — Infra-noise (transport/orchestrator blocks at the coordinator)

| metric | value |
|---|---|
| coordinator block findings | 577 |
| infra-pattern blocks (leaf produced no/ non-parseable output) | **56 (9.7%)** |
| coordinator defer findings | 492 |

These 56 infra-blocks are the coordinator blocking because a leaf **failed to
produce output** (mostly the 16% kiro empty-rate and the kiro-4-5-paid
transient), not a code catch. Real capability-block yield is ~90% of the
headline. *(Separate transport follow-up, §5.)*

### Table 6 — Coverage curve (greedy over the 435 rounds with ≥1 upheld finding)

| step | +model | cum upheld-round coverage |
|---|---|---:|
| 1 | gpt-latest:free | **48.3%** |
| 2 | + gpt-5.5:free | 73.8% |
| 3 | + glm-5.2-high | 86.0% |
| 4 | + grok | 92.4% |
| 5 | + kiro-claude-opus-4-6:free | 95.6% |

**Caveat (stated honestly):** greedy pick-order is **participation-confounded**
— gpt-latest/gpt-5.5 ran only in the recent 18-day window (07-10→07-27) while
glm/grok/kiro ran longer, so the *order* over-credits GPT's recency. **The
shape is robust and the confound works *against* GPT in older rounds** — yet
GPT still tops the curve. The participation-*independent* signal is Table 10.

### Table 9 — Severity calibration (LEAF severity of each model's UPHELD findings)

| model | critical | major | minor | info | (crit % of upheld) |
|---|---:|---:|---:|---:|---:|
| gpt-5.5:free | 24 | 45 | 12 | 8 | **19%** ← highest crit share |
| grok-4.3 *(thin)* | 4 | 2 | 1 | 5 | 25% |
| gpt-latest:free | 20 | 153 | 44 | 4 | 7% ← concentrated in **major** |
| grok | 15 | 22 | 52 | 43 | 8% ← noisy floor (info 23%) |
| glm-5.2-high | 6 | 31 | 52 | 38 | 3% ← noisy floor (info 20%) |
| **kiro-4-6:free** | 7 | 7 | 25 | **53** | 8% ← **57% info-severity** |

**Read:** gpt-class concentrates upheld findings in major/critical; grok/glm/kiro
are info-heavy. **kiro-4-6's upheld pool is 57% info-severity** — its "upheld"
findings are the weakest-signal of any model.

### Table 10 — GPT-INDEPENDENCE (the hard constraint for Part B)

GPT-class = {`gpt-latest:free`, `gpt-5.5:free`}.

| metric | full panel | non-GPT bench | lost if GPT vanishes |
|---|---:|---:|---:|
| upheld-**rounds** covered | 435/435 (100%) | 285/435 (**65.5%**) | **150 (34.5%)** |
| upheld **findings** | 952 | 546 (**57.4%**) | **406 (42.6%)** |

**This is the number the restructure is designed around: removing GPT-class
drops block-catching coverage by ~⅓ of rounds and ~43% of upheld findings.**
The non-GPT bench's best precision is grok at 10.7% — less than ¼ of
gpt-latest's 42.9%. **The panel does NOT gracefully survive GPT loss today.**

### Cross-repo (harness vs consumer-A)

All scoreable models run in both repos; the **ranking is stable across both**
(gpt-latest leads upheld/round in both). Every model upholds more per-round on
the harness repo (e.g. gpt-latest 135 uph / 284 harness-rounds = 0.48 vs 50 /
323 consumer-A-rounds = 0.15) — a **repo/prompt effect, not a model effect**
(the review prompt is tuned to the harness domain). Stable ordering ⇒ the
restructure generalizes, but lens thresholds must be **per-repo config**, not
hardcoded.

---

## X. Cross-check vs the prior study (CONFIRM / CONTRADICT / PARTIAL)

| prior claim | prior # | my # | verdict | why |
|---|---|---|---|---|
| §0 kiro paid transient quarantine | 21/1-day/100% empty | 21/1-day/100% empty | **CONFIRM** | identical; three ids distinct |
| §2 "gpt blocks ~36× kiro" | gpt-latest 17.9%, kiro 0.5% | gpt-latest **22.4%**, kiro **1.2%** | **CONFIRM (ordering); magnitudes higher** | I key verdict=`blocked` (prior used `block`); ordering identical, GPT ≫ rest |
| **§3 gpt-latest upheld-precision** | **18.4%** (matched=1824) | **42.9%** (matched=646) | **CONTRADICT** | prior's matched (1824) **exceeds the ~657 findings gpt-latest actually raised** — prior counted coord-findings-in-attending-rounds (converged attribution), not the model's own findings. My denominator is the real per-leaf-finding base. |
| §3 gpt-5.5 upheld-precision | 11.9% | **28.7%** | **CONTRADICT** | same contaminated-denominator cause |
| §3 grok / glm / kiro precision | 11.2 / 8.9 / 6.8% | 10.7 / 9.3 / 5.5% | **CONFIRM (±2pp)** | non-GPT models unaffected by the denominator bug |
| §3 infra-noise | 15.4% (41/267) | **9.7%** (56/577) | **PARTIAL** | both confirm ~10–15% infra slice; different block-count bases (I parse more coords) |
| **§4 unique-upheld leaders** | grok 6.5%, glm 6.2%, **gpt 2.9% (weakest)** | **gpt-latest 10.7%, gpt-5.5 10.2% (strongest)** | **CONTRADICT** | prior counted solo *pairs* (cat,loc), inflating loud models; my round-level metric shows GPT is the strongest unique contributor |
| **§6 coverage curve** | top-1 glm **83.1%**, gpt-latest 4th (+1.8pp, "redundant") | **top-1 gpt-latest 48.3%** | **CONTRADICT** | prior's "glm 83.1%" is **arithmetically impossible** — glm upheld in only 151/435 rounds (max 34.7%). Prior curve used the contaminated base. |
| §10 blind spots (blast_radius ~0) | 2 raised / 1 upheld | ~0 / 0 | **CONFIRM** | blast_radius is the #1 prompt gap; performance/concurrency/complexity all near-absent |
| §1 kiro-4-6 16% persistent empty | 16.2% flat | 16.1% flat | **CONFIRM** | persistent baseline, not a glitch |
| §13 "retire to 3 models, gpt-latest is redundant 4th" | — | — | **CONTRADICT (the restructure-critical one)** | gpt-latest is #1 on precision, #1 on coverage, #1 on unique-catches. It is the *opposite* of redundant. |

**Net:** the prior's *direction* (GPT blocks more and survives; blast-radius is
a blind spot; kiro is weak) is right. The prior's *magnitudes for GPT-class*
are wrong by 2–2.4× (precision) and its coverage curve and unique-catch
ranking are inverted. The prior's policy conclusion ("gpt-latest is the
redundant 4th slot, retire toward 3 models") is **directly contradicted** by
the re-derivation and is unsafe to act on — it would remove the panel's
single strongest block-finder.

---

## PART B — Panel restructure brief

### Design principle (one line)

> **A lens finds, the gate acts.** Lenses *propose* findings; the coordinator +
> commit-gate *dispose* (uphold/defer/drop). The panel's job is to maximize the
> chance that a real block is *proposed* by *someone*; the gate already decides
> survival. Therefore: redundancy on the **high-stakes block-finding role**,
> lens-diversity to cover the **prompt-blind classes**, and **GPT pinned as
> primary-but-backstopped**, never as a single point of failure.

### 2. Role/lens × model boundary table

Stakes tier: **H** = redundant (≥2 leaves), **M** = single leaf with fallback
chain, **L** = single leaf advisory. Fallback chains are **ranked**, resolved
at runtime to the first available model — the *chain* is the dependency, not
any pinned model (this is what makes a slot GPT-independent by construction).

| role | lens (prompt scope) | stakes | primary | fallback chain (ranked) | evidence |
|---|---|---|---|---|---|
| **R1 block-finder (redundant)** | full-spectrum block hunt (the current uniform prompt) | **H** — 2 leaves | `gpt-latest:free` | `gpt-5.5:free` → `grok` → `glm-5.2-high` | #1 precision 42.9%, #1 coverage 48.3%, #1 unique 10.7%. Two leaves on this role so a transport failure on one still leaves a block attempt. |
| **R1′ block-finder backup** | (same prompt, 2nd leaf) | **H** | `grok` | `glm-5.2-high` → `kiro-claude-opus-4-6:free` | the strongest **non-GPT** block-finder (10.7% prec, 176 upheld) — guarantees a block attempt survives total GPT loss. Weaker than R1 by design; degradation quantified in §3. |
| **R2 contract / data-integrity lens** | contract_drift + data_integrity + spec-conformance (drop doc_drift) | **M** | `gpt-latest:free` or `gpt-5.5:free` | `glm-5.2-high` → `grok` | GPT owns 46–62% of this class; **acknowledged GPT-dependent** — the weakest GPT-loss point. Fallback is broad-but-imprecise. |
| **R3 security lens** | security + data-policy + leakage | **M** | `gpt-5.5:free` | `kiro-claude-opus-4-6:free` → `glm-5.2-high` | security is spread (gpt-5.5 15%, kiro 10%); **can** be made GPT-independent. This is the repoint for the weak kiro slot — its one distinctive edge. |
| **R4 blast-radius / concurrency / complexity lens** | blast_radius + concurrency + performance + complexity (the blind spots) | **M** | any available (`glm-5.2-high` preferred for breadth) | `grok` → `kiro-claude-opus-4-6:free` | **NO model currently upholds these** — the value is the *prompt*, not the model. Any model can run it; this is the reinvested redundant slot. **Boundary with the separate complexity-control brief must be coordinated (§5) — do not redesign the complexity brief's internals here.** |

**Net leaf count: 5** (R1, R1′, R2, R3, R4) vs the current 4 — one more leaf,
but the 4th current slot (uniform model-diversity) is replaced by a
**blind-spot lens** that no amount of model-diversity reproduces. The cost is
~¼ more review latency, traded for covering the panel's biggest gap.

> **R4 SUSPENDED (2026-07-28, Branch B).** The S3 probe showed the blind-spot
> gap is an illusion (0/39 real-wrongly-dropped, Wilson 95% CI [0%, 9%]). No
> lens/prompt mechanism for blast-radius/concurrency/performance/complexity is
> warranted. The 5-leaf design above was drafted *before* the probe; **do not
> implement R4.** The net leaf count reverts to 4 (R1, R1′, R2, R3) until and
> unless a forward-looking defect-seeding study proves a real detection gap.
> (R2/R3 remain valid — they are high-yield-class lenses, not blind-spot fills.)

**Push-back on the considerations (evidence-bound):**
- *"Reinvest redundant slots into a security lens"* — **partially.** Security is
  NOT a blind spot (178 raised / 74 upheld = 41.6%, high-yield). The security
  lens is **GPT-independence insurance for a high-yield class**, not a
  blind-spot fill. Blast-radius/concurrency is the real blind-spot lens.
- *"Single-pass on advisory lenses"* — **agreed.** grok/glm/kiro are 90%+
  overturned; R2/R3/R4 are single-pass with fallback chains, not redundant.
- *"Retire the kiro slot"* — **evidence is mixed → defer to debate.** kiro-4-6
  is weakest on every axis EXCEPT a security-leaning fingerprint (9.8%) and
  zero cost. Repoint to the R3 security-lens fallback (not a general 4th
  reviewer), AND require the transport-hardening follow-up (its 16% empty-rate
  directly manufactures 9.7% of all infra-blocks). Do **not** retire it on the
  kiro-4-5-paid transient (CONFIRMED quarantined).

### 3. GPT-less degradation analysis

**Designing on the non-GPT fallback bench** (R1′ primary `grok`, R2 fallback
`glm-5.2-high`, R3 fallback `kiro-4-6`, R4 any):

| capability | with GPT | without GPT | degradation |
|---|---|---|---|
| block-finding precision (best leaf) | 42.9% (gpt-latest) | **10.7% (grok)** | **−33pp** — blocks become 4× less precise |
| upheld-round coverage | 100% (435/435) | **65.5%** (285/435) | **−34.5%** of block-catching rounds lost |
| upheld findings | 952 | **546** | **−42.6%** |
| contract/data-integrity lens | strong (GPT owns 46–62%) | **weak** (glm 7–9%, grok 8%) | the worst-hit lens |
| security lens | strong (gpt-5.5 15%) | **survivable** (kiro 10%, glm 7%) | mild — designed for this |
| blast-radius/concurrency lens | unaffected (prompt-driven) | unaffected | **none** — the lens itself is the value |

**How the fallback bench compensates:**
1. **R1′ guarantees a block attempt always runs** (non-GPT), so the gate never
   fails-open just because GPT quota vanished. Precision drops 4×, but the
   *fail-closed* property is preserved (a weak block-finder that still blocks
   >> no block-finder).
2. **R4 (blind-spot lens) is GPT-independent by construction** — its value is
   the prompt, so GPT loss costs nothing there.
3. **R3 (security) is explicitly designed on a non-GPT primary-capable chain.**
4. **R2 (contract) is the honest loss** — contract_drift + data_integrity are
   the two highest-yield classes (71% / 78% upheld) and they are
   GPT-territory. Under GPT loss, contract-drift catch-rate falls sharply.
   This is **unavoidable** with current non-GPT models; mitigation is
   *prompt-side* (a sharper contract-lens prompt that lifts glm/grok's
   contract precision), tracked as open-research §5.1.

**Bottom line:** the panel *survives* GPT loss (fail-closed holds, security and
blind-spot lenses are intact) but **degrades meaningfully** on the two
highest-yield classes (contract, data-integrity). That degradation is the
price of GPT-fragility; the restructure makes it **contained and labeled**,
not a silent single-point-of-failure.

### 4. Disagreement-resolution under lens-diversity (the hard integration problem)

The current coordinator assumes **uniform-lens leaves** — every leaf runs the
same prompt, so a simple majority/unanimity composes cleanly (3-of-4 agree →
resolved). **Lens-diversity breaks this:** a security-lens block and a
contract-lens block are about *different things*; they cannot majority-vote
each other.

**Proposed aggregation (per-class quorum + escalation):**

1. **Class ownership.** Each finding class is *owned* by exactly one lens
   (R2 owns contract_drift/data_integrity; R3 owns security; R4 owns
   blast_radius/concurrency/complexity; R1/R1′ own the rest + act as
   cross-cutting block-finders). A finding's class routes it to its owner.
2. **Per-class quorum.** A class-C finding is *proposed* by its owning lens
   (and independently by R1/R1′ if they also catch it). It is **upheld** if
   ≥1 owner-lens OR ≥1 of the redundant R1 block-finders raises it; it is
   *strongly* upheld if ≥2 independent leaves raise it. There is **no
   cross-lens outvote** — a contract-lens leaf cannot drop a security-lens
   finding it never examined.
3. **Escalation on cross-lens conflict.** When two lenses disagree on the
   *same* finding (same location+class, one blocks, one drops), the
   coordinator **escalates** (marks `disagreement`, demands a tie-break leaf
   or operator) rather than majority-resolving — because the lenses have
   different precision (gpt 42.9% vs kiro 5.5%) and a raw vote would let a
   loud-imprecise lens outvote a quiet-precise one.
4. **Authority line preserved.** Lenses propose; the coordinator merges with
   per-class quorum; the commit-gate disposes (block/defer/drop). No lens
   ever acts alone.
5. **Weighting (open — §5.2).** Whether a lens's vote is weighted by its
   measured precision (gpt R1 vote > kiro R3 vote) is left to debate; the
   schema must support a `weight` per lens even if v1 sets them all =1.

**Reconciliation with existing machinery:** the 4-leaf fan-out becomes a
**5-leaf role-tagged fan-out**; the coordinator's merge step gains a
`class_owner` lookup (per-repo config) and an `escalation` disposition state.
The commit-gate is unchanged (it already consumes the coordinator's final
disposition). This is a reshape of *what each leaf does*, not a rip-out.

### 5. Named open-research list (what a debate pass must settle)

1. **Does lens-diversity actually raise unique upheld catches vs uniform-lens?**
   My data shows uniform-lens uniqueness is already GPT-dominated (10.7%).
   The claim that R4 (blast-radius lens) adds net-new upheld blocks is
   *plausible but unmeasured* — needs a controlled A/B (same diffs, lens vs
   uniform prompt). **This was the #1 open question.**

   > **RESOLVED 2026-07-28 (Branch B).** The S3 probe showed the blind-spot gap
   > is an illusion (0/39 real-wrongly-dropped, Wilson 95% CI [0%, 9%]). The R4
   > lens would add detection volume to classes whose existing dropped volume is
   > 100% noise. The controlled A/B is not justified. **R4 retracted; this open
   > question is closed.** Source: `2026-07-28-s3-probe-dropped-blindspot.md`.
2. **How to weight lens votes** given 8× precision spread (gpt 42.9% vs kiro
   5.5%). Equal-weight lets noise outvote signal; precision-weight needs a
   stable per-lens calibration that drifts with model updates.
3. **Complexity-lens boundary with the complexity-control brief.** R4 includes
   "complexity" but a separate complexity-control brief owns complexity
   internals. **Coordinate the boundary** — R4 should *detect and flag*
   complexity for the complexity brief; it must not redefine the complexity
   thresholds. (Out of scope to design here.)
4. **Is the 34.5% GPT-only coverage real capability or a recency artifact?**
   GPT ran only 07-10→07-27. A same-round controlled comparison (rounds where
   GPT + non-GPT both present) is needed to strip the participation confound.
   Preliminary: even in co-present rounds GPT dominates, but the exact
   "lost-if-GPT-gone" number carries ±uncertainty.
5. **Stop soliciting doc_drift / style / overclaim / testability?** They are
   3387 raised / 48 upheld (1.4%) — pure noise that inflates overturn rates
   and coordinator load. A prompt change, not a model change.
6. **Transport hardening (named follow-up, NOT model selection).** The 16%
   kiro empty-rate manufactures 9.7% of infra-blocks and jams the fail-closed
   gate. Fix = leaf-output validation + retry + treating empty as transport
   failure (operator directive), not "kiro is a worse model." Track
   separately from this restructure.

### 6. Slice plan (ordered, bounded — NOT implemented)

Each slice is independently shippable. S3 is the risky one (reshapes fan-out).

- **S1 — Schema: per-finding class-ownership + escalation state.** Add
  `owned_class` / `source_lens` / `disagreement` to the coordinator
  result schema (config-driven class→lens map). Pure additive; existing
  consumers ignore unknown fields. *Validate:* round-trip a sample.
- **S2 — Config shape for role/lens × model.** Per-repo YAML:
  `roles: {R1: {lens: <prompt-id>, models: [ranked...], stakes: H, leaves: 2}, …}`.
  **Domain-free core** ships the token + the schema; **per-repo config** ships
  the actual model rankings/thresholds (never hardcoded in `templates/core/`).
  *Validate:* `--dry-run` render + config lint.
- **S3 — Reshape the fan-out to 5 role-tagged leaves** (R1, R1′, R2, R3, R4)
  behind the coordinator. Keep the coordinator's merge; add the per-class
  quorum from §4. *Validate:* the existing coordinator-round fixtures still
  merge; add lens-diversity fixtures. **Highest-risk slice — gate behind S1+S2.**
- **S4 — Per-class quorum + cross-lens escalation** in the coordinator merge
  (the §4 logic). *Validate:* disagreement-fixture produces `escalation`, not
  majority-drop.
- **S5 — Transport hardening** (leaf-output validation + retry; empty-as-
  transport-failure). **Separate workstream** — kills the 9.7% infra-noise and
  the gate-jam. *Validate:* an injected empty-leaf no longer manufactures an
  infra-block.
- **S6 — Prompt: drop doc_drift/style/overclaim/testability solicitation; add
  blast-radius/concurrency/performance/complexity to R4.** *Validate:*
  re-baseline noise-rate on a fixed diff set.
- **S7 — Re-baseline the scorecard** (re-run this study's pipeline) after S3+S6
  to measure whether lens-diversity raised unique upheld catches (open-research
  §5.1). *This is the experiment that closes the #1 open question.*
  > **Premise changed (2026-07-28, Branch B).** §5.1 is closed: the blind-spot
  > gap is all noise, so a lens-diversity re-baseline is not justified for
  > blind-spot classes. S7 retains value only as a routine scorecard refresh
  > after the schema_v2 (S1) and any future O3 (S2) work lands — not as a
  > blind-spot experiment.

**Non-goals (out of scope):** redesigning the complexity-control brief;
touching the commit-gate; retiring any model on the kiro-4-5-paid transient;
ranking models by empty-rate.

---

## Persist-guard compliance

This brief contains **aggregate model statistics only**. No consumer-repo
name/path, no consumer file paths, no verbatim consumer finding-text, no
`/home/` absolute paths (the DB is referenced in tilde form as the operator's
directive authorizes). Cross-repo references use the neutral label
`consumer-A`. The only illustrative session-level examples would be
harness-repo (none needed in the final tables). Session-id citations are
omitted from the persisted tables (available in the `tmp/` work scripts if a
follow-up session needs them); the brief is safe to surface to the
coordinator verbatim.
