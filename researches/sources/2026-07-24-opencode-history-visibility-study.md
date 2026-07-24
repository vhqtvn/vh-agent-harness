# Sources: Operator-visibility evidence study — vh-solara O1 → server-owned-tree arc

**Date:** 2026-07-24 (study date; promoted to durable sources 2026-07-25)
**Topic:** what the operator saw vs what was already knowable at each decision point
on the vh-solara O1 → server-owned-tree arc — the demonstrated ~26–28h pivot-delay
bottleneck and the synthesis + option-generation + persistence failure it exposed.
This is the Class-A evidence study the seven-controls property map and the S2-a
topology decision memo were decided on.
**Decision memo:**
[`../decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md`](../decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md).
**Companion property map:**
[`./2026-07-25-seven-controls-property-map.md`](./2026-07-25-seven-controls-property-map.md).

## Provenance

- **Source type:** operator-visibility evidence study (Class-A).
- **Evidence class:** Class-A — this-machine session/timestamp/SHA-cited vh-solara
  evidence. Every claim in the verbatim body below is cited to a session ID, a
  timestamp, and/or a commit SHA under `/home/vhnvn/repo/vh-solara`. It is readable
  only on this machine and NOT re-derivable inside vh-agent-harness; its numeric
  claims (e.g. the 26–28h pivot delay) are asserted-by-operator,
  unverified-by-us — treated as Class-A, not re-derived here.
- **Original location:** `tmp/operator-visibility/2026-07-24-opencode-history-visibility-study.md`
  (volatile; lives under `tmp/`, not committed canon; promoted to durable sources on
  2026-07-25).
- **Promotion basis:** same evaporation principle as the seven-controls property-map
  source packet — the volatile study is the citation root for both
  [`./2026-07-25-seven-controls-property-map.md`](./2026-07-25-seven-controls-property-map.md)
  and
  [`../decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md`](../decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md),
  so it is promoted to durable sources to keep its citations resolvable after the
  `tmp/` original evaporates.
- **Promotion commit:** the commit that introduces this file —
  `docs(researches): promote operator-visibility evidence study to durable sources`
  (2026-07-25). A committed file cannot embed its own introducing commit's hash
  without an amend (the hash depends on the file's bytes), so it is referenced
  resolvably rather than inlined; resolve via
  `git log -1 --format=%H -- researches/sources/2026-07-24-opencode-history-visibility-study.md`.
- **Status:** durable source packet. Routes verdicts to `researches/decisions/`.

The body below the `---` delimiter is a faithful verbatim copy of the original
study; only this provenance header is new — the study's content (including its own
title, frontmatter, and internal line-number references to peer memos) is unchanged.

---

# Operator-visibility study: what the operator saw vs what was knowable — the vh-solara O1 → server-owned-tree arc

**Date:** 2026-07-24.
**Method:** direct read-only SQLite queries against `~/.local/share/opencode/opencode.db`
(`session`/`message`/`part`, roles via `message.data.$.role`, text via `part.data.$.text`),
direct transcript reads of the anchor sessions and their child lanes, and `git log` of
`/home/vhnvn/repo/vh-solara`. All timestamps are local (+07). Session IDs are shortened
to their first 9 hex chars after `ses_`; full IDs in the appendix.
**Caveat:** all vh-solara transcripts and commits cited here are **Class-A evidence** —
readable only from this machine, not re-derivable inside vh-agent-harness. The prior
field report (`researches/sources/2026-07-23-vh-solara-harness-adoption-field-report.md`)
already established *what failed* (7 patterns); this study asks a different question:
**at each decision point, what did the operator see vs what was already knowable somewhere
in the system?** It does not re-derive the field report's patterns.

---

## 1. Thesis under test

Operator's framing: *"the 3-day vh-solara issue — if I had understood from the start, I
would have chosen the server-owned direction and not spent 2 days on 'optimization'
band-aids."*

Decomposed into a testable claim: decisive evidence for the real root cause
(client/server dual-ownership of tree structure) existed in the system materially earlier
than the pivot to the server-owned redesign (2026-07-23 09:17, `ses_0733c95c4`), and
better visibility would have moved the pivot to day 0–1.

---

## 2. Timeline reconstruction

Arc skeleton (all verified against DB rows and commits):

| T | When | Event |
|---|---|---|
| T0 | 07-21 17:03–17:34 | O1 design: "Idle-collapse solution brief" (`ses_07bde882d`), "Idle-collapse build-readiness gate" researcher (`ses_07bcf07cc`) returns **BUILD-READY, all 6 gates, HIGH confidence** (17:34:15) |
| T1 | 07-21 19:36–21:59 | O1 lands: 6 commits `f272c7d…ff7c459` ("Phase 1..6 of O1 projection"). Coordinator DCP topic 22:00: *"O1 build complete"* |
| T2 | 07-22 01:19–07:42 | First band-aids: `4bc41db`, `de782d8` (duplicate row, 01:19/01:45); operator reports stale-busy 05:00:20; **ship-review verdict NEEDS CHANGES 05:31:48**; operator pastes duplicate-row root-cause 05:35; coordinator routes "the full correctness fix slice" 05:37–05:38; anti-resurrection guard `94cb2c2` 07:27 |
| T3 | 07-22 13:24–19:14 | Amplifier hunt: operator's live perf study correction 14:03; `d2d3529`, `6cef3c1`, `f54ffff`, `e88f19e`, `48fbd2f`, `b3aa2e4`, `bfdfde6` |
| T4 | 07-23 00:19–03:46 | Demotion-gap study (`ses_07527ea79`, closeout 00:25); operator's "unifying root cause" drift brief 01:30; overnight fixes `dfbdaf1`, `388652a`, `1695b4c`, `39d501b`, `555532f`, `52a3fd0` |
| T5 | 07-23 07:47–~09:00 | Round-2 stabilization A/B/C/D (`ses_0738dbfc2`), first in-system co-listing of six symptom flows D1–D6 |
| T6 | 07-23 09:11–09:17 | **Pivot**: operator launches "Server-owned session tree design" (two aborted starts 09:11/09:14, real one `ses_0733c95c4` 09:17) with the dual-ownership synthesis |

Band-aid volume between T1 and T6: ~20 `fix(web|state|sync)` commits (`4bc41db` 07-22
01:19 → `52a3fd0` 07-23 03:46), matching the operator's "2 days".

### 2.1 Earliest-evidence exhibits (verbatim, strongest first)

**E1 — 07-21 17:20:29, pre-build (T0).** The build-readiness gate prompt
(`ses_07bcf07cc`, coordinator-authored) names the ownership seam as THE blocker before a
line of O1 was written:

> "Gate A — Projected-authority state machine (the core blocker):
> `applySnapshot` currently treats an OMITTED session as DELETED (wholesale-replace).
> For O1, omission must mean 'hidden, not deleted.'"

The researcher answered at 17:34:15: *"O1: BUILD-READY — proceed to /draft-plan. …
A — Projected-authority state machine | BUILD-READY | HIGH"*. The hazard was named and
declared contract-solved. Location: subagent prompt + closeout; the operator surface at
this moment was the DCP topic line "Gate C prototype passed" and, later, "O1 build complete".

**E2 — 07-22 05:31:48 (T2).** Ship-review of the O1 surface (`ses_07937089e`), lane's own
verdict, ~4.5h after the first band-aid commit:

> "Verdict: NEEDS CHANGES … I found multiple correctness gaps in the
> projection/reconnect/lazy-expand paths, including two that are **direct siblings of the
> bugs already observed**: stale state after a missed event or ring-gap snapshot; **stale
> branch data resurrected or merged after a concurrent structural change**."

This *predicted the resurrection family* (first guard `94cb2c2` landed 07:27 the same
morning; tombstone `555532f` 07-23 03:15; third guard `ef5ecac` 07-23 16:49 — after the
pivot). Per-gate: **A FAIL, B FAIL, D FAIL** — the same Gate A that was BUILD-READY/HIGH
12 hours earlier.

**E3 — 07-22 05:35:21 (T2).** The operator's own message to the coordinator
(`ses_09593d48`), pasting a root-cause summary of the already-fixed duplicate row:

> "ROOT CAUSE (client-side, NOT server-side) … **Two different merge rules are applied to
> the same snapshot** … `state.sessions[S]` AND `state.branchStubs[S]` now coexist for the
> same id."

This is the dual-authority-in-the-client signature, named by the **human**, on day 1
at 05:35. (Provenance note: the fix commits `4bc41db`/`de782d8` landed 01:19/01:45 from a
lane outside this coordinator; the operator carried the diagnosis in by paste.)

**E4 — 07-22 05:33:57 (T2).** The coordinator DID surface E2 to the operator, and even
generalized it:

> "The reviewer did its job: it found real correctness gaps, several direct siblings of
> the two bugs that already bit. Per-gate: A FAIL, B FAIL, D FAIL … the projection surface
> is currently **'fast but leaky.'**"

…and then, at 05:37–05:38, routed the response as repair: *"On it — routing the full
correctness fix slice. **Compressing the now-settled O1 design/build/bug provenance first
(we're in the fix phase; the design gates and build are history)**, then routing."*
DCP compression #36 ("O1 provenance through bug fixes") ran at 05:38. No
step-back-to-design option was generated — a DB-wide search for "redesign" across the
arc finds **zero** occurrences before the operator's own pivot prompt.

**E5 — 07-23 00:25:03 / 01:30:44 (T4).** The demotion study closeout (`ses_07527ea79`)
proves the drift class is an *absence-of-code* structural fact ("no time-driven
frontier-change site exists", F1, file:line table). The operator's drift-recovery brief
at 01:30 authors the closest pre-pivot synthesis:

> "Unifying root cause: 'works right after restart, drifts into a broken state over
> time' … the optimization removed the frequent full re-projections that used to
> CONTINUOUSLY self-heal client/daemon state."

Still repair-framed ("restore correctness, keep the bandwidth win") — 7h47m before the
pivot reframed the same facts as ownership: the self-heal was only load-bearing *because*
the client owned a reconstructable copy of the structure.

**Negative finding:** one cascade symptom — "tree flatten on load" (caused by persisting
tree structure to localStorage) — has **no diagnosis anywhere in the perf-opt arc's
transcripts**. Its first in-DB root-cause statement is the operator's own pivot prompt
(07-23 09:11, "Do NOT persist tree structure to localStorage (that caused the flatten)").
Part of the synthesis existed only in the operator's head or an unrecorded surface.

### 2.2 Evidence-vs-visible contrast table

| Moment | Operator-visible surface | Knowable in full transcripts at that moment |
|---|---|---|
| T0 07-21 ~17:35 | DCP topic lines; titles "Idle-collapse solution brief / build-readiness gate"; verdict rolled up as BUILD-READY | E1: ownership inversion named "the core blocker", mitigated only by declared client-merge contracts (`ses_07bcf07cc` prompt + closeout) |
| T1 07-21 22:00 | "O1 build complete" (DCP); 6 clean `feat/perf` one-liners | Same as T0; no new warnings |
| T2 07-22 05:31–07:30 | Coordinator quote: "A FAIL, B FAIL, D FAIL … fast but leaky" (05:33); git: independent `fix(...)` one-liners | E2 (structural failure class + resurrection prediction), E3 (operator's own dual-merge-rule diagnosis), and their conjunction with E1 — three ownership-class hits in 2 hours, never joined |
| T3 07-22 ~14:00 | Operator's own perf study; more `fix(...)` one-liners | Growing same-subsystem fix streak (9 commits to projection/stream paths in ~13h); no ledger counted it |
| T4 07-23 01:30 | Operator authors "unifying root cause" (drift) | E5 structural absence-of-code fact + E1/E2/E3 in history; resurrection family now fixed twice, orphan family twice |
| T5 07-23 07:48 | Round-2 prompt (operator-approved) lists D1–D6 flows | First co-listing of the symptom families — inside a subagent prompt, 1.5h before pivot |
| T6 07-23 09:17 | Operator writes the dual-ownership synthesis from scratch | Everything above; the synthesis reuses E3's content and E5's framing but had to be re-derived by the human |

---

## 3. Counterfactual verdict

**Split verdict — the claim holds for day 1, not for day 0.**

**Day 0 (07-21, before/at O1 build): the decisive information was NOT present.** What
existed was a *named hazard* (E1, Gate A "the core blocker") with a declared mitigation,
examined by a dedicated researcher lane that concluded BUILD-READY/HIGH — wrongly, but
after actually reading the code. There was no accumulated-failure evidence; choosing
server-owned at T0 would have required overriding a high-confidence lane verdict on
design judgment alone. No visibility mechanism fixes that; that is a
design-review/adversarial-gate problem (the field report's Pattern 5 territory).

**Day 1 morning (07-22, by ~07:30): the decisive information WAS present, and partially
even displayed.** Within 2 hours the system held: a ship-review declaring the surface
structurally leaky with A/B/D gate FAILs and naming two *predicted* failure families as
"direct siblings of the bugs already observed" (E2); the operator's own diagnosis that
the client applies two conflicting authority rules to one snapshot (E3); and the
pre-build warning that this exact seam was "the core blocker" (E1). The pivot came at
07-23 09:17 — a **latency of ~26–28 hours** during which ~16 more band-aid commits landed
(`94cb2c2` 07-22 07:27 → `52a3fd0` 07-23 03:46), plus the resurrection family being
re-fixed after already being predicted.

**Which bottleneck?** Not raw display: the coordinator quoted the ship-review verdict to
the operator verbatim at 05:33 (E4). The failure is **synthesis + option-generation +
persistence**:

1. Nothing joined E1+E2+E3 into "three independent hits on the same structural seam in
   12 hours." Each lived in a different container (subagent prompt, lane closeout,
   operator paste) with no shared signature.
2. "Step back to redesign" was never generated as an option. The only decision fork ever
   presented was fix-now (05:37). Zero pre-pivot "redesign" mentions system-wide.
3. The evidence *decayed*: at 05:37–05:38 the coordinator explicitly compressed the
   design-gate provenance ("the design gates and build are history") at the exact moment
   the Gate-A-class bugs were biting. By T4, the operator was re-deriving from symptoms
   what E1/E2 already said.

So: the visibility lane's target is not a better live dashboard of lane text. It is
(a) a persistent, queryable record of gate verdicts / hazards / failure families that
survives compaction and accretes across lanes, and (b) a mechanism that converts
recurrence into a surfaced "question the design" decision point. Quantified prize, from
this arc: ~1 day of the ~2-day band-aid window and ~16 commits — not the full 3 days the
operator's framing implies.

---

## 4. Decision-record audit

| Decision | Where the rationale actually lives | Findable without reading raw transcripts? |
|---|---|---|
| Adopt O1 collapsed-frontier (07-21 ~17:35) | Solution-brief + build-readiness closeouts (transcripts of `ses_07bde882d`, `ses_07bcf07cc`); commit one-liners say only "Phase N of O1 projection" | **No.** vh-solara's `researches/decisions/` holds 4 memos — none about projection/tree ownership. The BUILD-READY verdict and its Gate-A reasoning exist nowhere durable |
| Keep fixing vs step back (07-22 05:37) | Coordinator transcript only ("routing the full correctness fix slice"); it was never framed as a fork, so no rationale was recorded anywhere | **No.** This — the arc's most expensive decision — has no record at all; it is only reconstructable by reading `ses_09593d48` messages |
| Pivot to server-owned + Phase-1 acceptance (07-23 09:17 / 09:55) | Operator's prompt (transcript); then `docs/design/server-owned-tree.md` (`4e06e16`, revised `5995161` after review caught the dropped motivation) | **Partly.** The design doc is the arc's ONLY durable decision artifact — and its v1 shipped with the motivation dropped, caught post-commit (field report Pattern 5) |
| "Phase 2 complete" claim (07-23 18:25, `e5d09d4`) | Committer transcript `ses_071521f22` (reviewer finding F9 "not exercised end-to-end" → disposition `drop`); live-smoke INCONCLUSIVE verdict buried in §7 of `ses_0712fa42d`'s closeout | **No.** The operator surface (commit one-liner + lane title "Live smoke test…") reads as done; the weakest claim is 5+ sections deep in one lane's closeout |

Pattern: **decisions that produced commits left a trace (the commit); decisions that
chose between directions left none.** The one durable design record was created only at
the pivot — i.e., the record-keeping began exactly when the operator's understanding did.

---

## 5. Derived visibility requirements (evidence-anchored; not solution designs)

Each requirement cites the moment that proves the need, and the existing harness seam
that partially covers it (so the lane extends rather than reinvents).

**R1 — Gate/lane verdicts must persist in an operator-visible arc ledger, not just in
the chat scroll.** Needed at: T2→T4 — "A FAIL, B FAIL, D FAIL" existed at 05:33 07-22 as
one chat message and was functionally gone by T4 (DCP #36 compressed the provenance at
05:38; by 07-23 the operator re-derived the same facts). Existing seams: verdict-token
ADOPT (disposition memo §4.1, `researches/decisions/2026-07-23-…-disposition.md:139`),
checkpoint Verification table (RC2 row). Gap: those make the *lane* emit a token; nothing
accumulates tokens across lanes where the operator can see the streak.

**R2 — A failure-family recurrence count must be visible before the Nth same-family
fix is dispatched.** Needed at: resurrection ×3 (`94cb2c2` 07-22 07:27, `555532f` 07-23
03:15, `ef5ecac` 07-23 16:49 — the third *after* the pivot) and orphan ×3 (`b245173`,
`e88f19e`, `52a3fd0`); no surface ever showed "×3". Existing seam: P2-A recurrence
detector — **DEFERRED**, blocked on `defer-002` symptom-signature-stability, whose
unblock condition is "≥1 more band-aid instance" (disposition memo:144, 301). This study
documents the canonical instance; note it is the *same* instance the defer was written
from, so it satisfies evidence-of-need, not the ≥1-*more* condition.

**R3 — At any repair-routing moment following a structural-class review verdict, the
"step back to design" fork must be explicitly surfaced and its rejection recorded.**
Needed at: 07-22 05:37 — the fix-slice routing after a NEEDS-CHANGES with 3 gate FAILs
was the arc's most expensive unrecorded decision (§4 row 2). Existing seam:
`researches/decisions/` memo convention exists (both repos use it) but nothing triggers
memo creation at a routing fork; the coordinator card store (`.local/coordinator/tasks/`,
RC7 row) records deferred findings, not direction choices.

**R4 — Design-phase hazard statements must stay pinned to the arc while their subsystem
is under repair.** Needed at: 07-22 05:37 — the coordinator compressed "the design gates
and build are history" at the exact moment Gate-A-class bugs were biting; E1's "core
blocker" framing never met E2/E3 in any surface. Existing seams: compaction-preserves-
verdict-and-crux ADOPT (disposition memo:146, "compaction ≠ completion" cross-cut);
DCP ownership layer memo (`researches/decisions/2026-07-23-dcp-ownership-layer.md`). Gap:
both protect the *lane's own* verdict, not cross-lane hazard→symptom linkage.

**R5 — Operator-authored syntheses must become durable, addressable artifacts at the
moment they are written.** Needed at: E3 (05:35 07-22) and the 01:30 07-23 drift
synthesis — both lived only as chat messages; the flatten diagnosis lived nowhere at all
(§2.1 negative finding); 28h later the pivot prompt had to reconstruct the full cascade
list from memory. Existing seam: P2-B findings-delta field (ADOPTED for lane closeouts,
disposition memo:145) — but it binds *agent* closeouts, not operator messages.

**R6 — A completion claim must carry the arc's weakest live claim at the same surface
where "complete" is displayed.** Needed at: 07-23 18:25 — `e5d09d4` "complete Phase 2"
was operator-visible while "INCONCLUSIVE / crux SKIPPED / NOT DEMONSTRABLE"
(`ses_0712fa42d` §7) and the dropped F9 were transcript-buried. Existing seams: verdict
token + crux clause + defer-not-drop (all ADOPTED, disposition memo:139–140); this
requirement is their *operator-surface* half — the token must be readable where the
operator reads status, not only checkable by the gate.

**R7 — Symptom co-listing must reach the operator surface when it first exists anywhere.**
Needed at: 07-23 07:48 — the D1–D6 co-listing (Empty-on-select, Drift/ghosts,
Multi-client, Archive/resurrection, Orphan cleanup, Subagent-as-root) was the system's
first joint view of the families and appeared only inside a subagent prompt, 89 minutes
before the operator pivoted anyway. Had that co-listing surfaced on 07-22, it would have
been the recurrence evidence R2 needs. Existing seam: none directly; closest is the
coordinator's task-card store, which tracks tasks, not symptom families.

---

## 6. Appendix — evidence IDs

**Sessions** (all directory `/home/vhnvn/repo/vh-solara`):
- Performance Optimization coordinator — `ses_09593d48dffe22OgQznItWbrWN` (2026-07-16 17:15)
- Idle-collapse solution brief — `ses_07bde882dffe8Fw9m7GeeajRto` (07-21 17:03)
- Idle-collapse build-readiness gate (researcher) — `ses_07bcf07ccffe5woS8ozf0Bc8cn` (07-21 17:20; BUILD-READY 17:34:15)
- O1 collapsed-frontier build — `ses_07b800d36ffeAb0e6C8VmgkZJ5` (07-21 18:46)
- Ship-review O1 change surface — `ses_07937089effeQ4HvrJ9PHpDHGL` (07-22 05:25; verdict 05:31:48)
- Fix O1 projection correctness gaps — `ses_0792aa752ffenh8nts18V4VXNJ` (07-22 05:39)
- Study demotion gap, pick design — `ses_07527ea79ffe5Y5olgX3UhrR5e` (07-23 00:20; closeout 00:25:03)
- Drift-recovery 5-issue fix — `ses_074e5f525ffemfmipFTALUu9zL` (07-23 01:32)
- Round-2 stabilization A/B/C/D — `ses_0738dbfc2ffeB64VWCKPyR19X4` (07-23 07:48)
- Server-owned session tree design — `ses_0733c95c4ffeiYE91DLVRAJgeT` (07-23 09:17; aborted starts `ses_07341ef20…` 09:11, `ses_0733efbc6…` 09:14)
- Phase-1 design researcher — `ses_0733abffcffe7H5qbqRCGcNikS` (07-23 09:19)
- Live smoke test tree=2 daemon — `ses_0712fa42dffeE5GsHPb2Fxs785` (07-23 18:50)
- Untangle working tree into 3 commits — `ses_071a82d87fferj7Krisj6xbQZK` (07-23 16:39)
- Phase-2 committer (F9 drop) — `ses_071521f22ffeIr1xRInwd7ZHx2`

**Commits** (vh-solara): O1 phases `f272c7d…ff7c459` (07-21 19:36–21:59); band-aid
cluster `4bc41db` (07-22 01:19) → `52a3fd0` (07-23 03:46), ~20 commits; resurrection
family `94cb2c2`/`555532f`/`ef5ecac`; orphan family `b245173`/`e88f19e`/`52a3fd0`;
design docs `4e06e16`/`5995161`; "complete Phase 2" `e5d09d4` (07-23 18:25).

**Operator messages quoted** (all in `ses_09593d48…`, role=user): 07-22 05:00:20
(stale-busy report), 05:35:21 (duplicate-row root-cause paste, E3), 14:03:40 (perf-study
correction), 07-23 00:19:39 (demotion follow-up), 01:30:44 (drift unifying root cause).
Coordinator messages quoted (role=assistant): 07-22 05:33:57 (E4), 05:36:22 (merge-
asymmetry synthesis), 05:37:43 + 05:38:31 (fix-slice routing + provenance compression).

**Harness seams referenced** (vh-agent-harness):
`researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md`
(RC1–RC8 verdicts; adoption rows :139–146; `defer-002` :118, :301),
`researches/sources/2026-07-23-vh-solara-harness-adoption-field-report.md`,
`researches/decisions/2026-07-23-dcp-ownership-layer.md`,
`.opencode/scripts/commit-gate.sh`, `.local/coordinator/tasks/` card store.
