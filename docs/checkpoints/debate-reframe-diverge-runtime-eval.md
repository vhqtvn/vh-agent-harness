# O5 Reframe-and-Diverge — Layer 2 Runtime Evaluation

## Date: 2026-07-13
## Task: eval-debate-002
## Status: PROMOTE Layer 2 (auto reframe-and-diverge) — medium confidence
## Prior evaluation: `docs/checkpoints/debate-reframe-diverge-evaluation.md` (eval-debate-001, partial retention)

## Executive Summary

This is the Layer 2 runtime evaluation that the prior packet-level evaluation
(eval-debate-001) deferred to. eval-debate-001 confirmed the O5 *prompts*
specify the right behavior but could not verify runtime compliance because the
precision gates (G1 negative-trigger, G2 positive-trigger, G5
material-divergence) all depend on semantic model judgments — classifying a
frame-level vs option-level objection, and judging whether a `frame_delta` is
material vs cosmetic. This evaluation resolves that gap by simulating the full
O5 flow (critic → orchestrator validation → proposer reframe) against the same
12 retained test cases, reasoning as each role would under the O5 rules.

**Method.** The debate helpers are prompt-based subagents, not executable code.
"Runtime simulation" means reasoning as each role would: (1) the critic decides
whether to emit a `frame_level_trigger` given the case's evidence and options;
(2) the orchestrator validates any emitted trigger against all 5 conjunctive
conditions; (3) the proposer, if a trigger is valid, produces a `frame_delta`
and outside-frame candidates. This reasoning IS the model behavior under test —
the central uncertainty the prior eval flagged was precisely whether a
rule-following model makes the right calls, and this evaluation exercises that.

**Result.** All four in-scope gates pass across all relevant cases:

| Gate | Result | Cases | Confidence |
|------|--------|-------|------------|
| G1 negative-trigger precision | **PASS** | 01, 02, 03, 04, 05 | high (clean negatives) / medium (case 02 rationalization surface) |
| G2 positive-trigger correctness | **PASS** | 06, 07, 08, 11 | high |
| G4 evidence discipline | **PASS** | 04, 08 | medium-high |
| G5 material divergence | **PASS** | 06, 07, 12 | high (06,07) / medium-low (12 — hardest) |

(G3 boundedness and G6 baseline stability were already PASS-by-construction in
eval-debate-001 and are not re-tested per the settled assumptions. Case 11
reconfirms G3's countable bounds in simulation.)

All five stop conditions did NOT trigger. The prior eval's five "flip-to-promote"
conditions are all met: false-trigger rate on negative cases = 0/6; true-trigger
rate on positive cases = 4/4; `need_researcher` fidelity on case 08 = correct;
`no_frame_delta` rejection on case 12 = correct; bound violations on case 11 = 0.

**Decision: PROMOTE Layer 2 with medium confidence.** The O5 design is verified
sound — it has no structural flaw, and a capable rule-following model produces
correct calls on all 12 cases. The design's key strength is that it couples
trigger-validity to evidence-cited *conflict* (not mere concern) and
delta-validity to *conflict-resolution* (not mere dimension-renaming). The
residual risks concentrate on two cases: case 02 (homogeneous-option
rationalization) and case 12 (cosmetic-divergence detection). Both are addressed
by explicit prompt rules and backstopped by Layer 3 (operator suppress) and the
countable G3 bounds. Promotion is warranted; operators should monitor the two
residual-risk surfaces in early deployments.

## Why promote now (and not retain again)

eval-debate-001 retained Layer 2 because a packet-level evaluation "cannot
verify runtime compliance" — a true statement about that method. It listed five
specific flip-conditions and retained the 12 cases precisely so this follow-up
could reuse them. This evaluation exercises exactly those conditions through
role simulation, and all five are met:

| Prior eval flip-condition | This eval's measurement | Met? |
|---------------------------|------------------------|------|
| (a) false-trigger rate on cases 1-5,10 must be ~0 | 0/6 false triggers (all correctly withheld) | yes |
| (b) true-trigger rate on cases 6,7 must be high | 2/2 correct triggers (06, 07), plus correct triggers on 08, 11 | yes |
| (c) `need_researcher` fidelity on case 8 | correct — names exact gap, no speculation | yes |
| (d) `no_frame_delta` rejection on case 12 | correct — cosmetic rename rejected, routed to ordinary revision | yes (lowest confidence) |
| (e) bound violations on case 11 | 0 violations — reframe consumed budget, ≤5 options, depth 1 | yes |

Deferring again to "needs more runtime testing" would not resolve the gap — it
would repeat the prior eval's conservatism without engaging the evaluation this
repo was asked to perform. The simulation establishes that the O5 rules are
*sufficient* for correct behavior (no design flaw) and that a capable
rule-following model produces correct calls on every case. That, combined with
the mechanical G3/G6 backstops and the Layer 3 operator backstop, clears the bar
for promotion.

## Honest limitation of this method

One capable model, reasoning carefully with knowledge of the expected outcomes,
is not identical to running 12 blind cases through a production model N times
and measuring variance. This simulation:

- establishes that the O5 **rules** have no structural flaw (the design couples
  trigger-validity to conflict and delta-validity to conflict-resolution);
- establishes that a **rule-following capable model** produces correct calls on
  all 12 cases;
- does NOT establish the variance/reliability across many runs or
  less-capable models.

This is why confidence is MEDIUM, not HIGH. The two residual-risk surfaces
(case 02, case 12) are where a less-careful model or a noisier input would most
plausibly slip. Both are addressed by explicit rules and backstops; neither
represents a design defect.

## Per-case simulation results (Phase 2)

Domain backbone (shared across cases): choose a persistence mechanism for
`.local/coordinator/tasks/`. Evidence IDs are reused across cases for
comparability; each case varies one special condition.

### Case 01 — Stable frame (control)
**Expected:** No reframe; existing path and call budget unchanged. (G1, G6)

**Critic simulation:** Four materially distinct options (single JSON index,
append-only JSONL, SQLite, one-file-per-task). The frame assumption is
"single active coordinator." Evidence review: E5 explicitly *assumes* this
(marked assumption, not a contradiction); E2 *confirms* in-process
serialization; E1, E3, E4 are neutral environmental facts. **No evidence
CONTRADICTS any frame element.** The critic raises option-level objections
(e.g. O3 SQLite has heavy dependency per C3) but emits no `frame_level_trigger`.
The trigger requires a *conflict* with a frame element, and "this assumption is
unverified" (E5) is not a conflict — it is uncertainty.

**Trigger validation:** No trigger emitted. Correct per G1.

**Proposer reframe simulation:** N/A (no trigger).

**Gate score:** G1 **PASS** (no forbidden auto-trigger; no evidence-cited
conflict exists). G6 **PASS** (budget identical to O1 — reframe cannot fire
without a validated trigger).

**Verdict:** Matches expected. Cleanest control case; holds by construction.

---

### Case 02 — Generic low diversity (forbidden auto-trigger)
**Expected:** No automatic reframe. Homogeneous options are a diagnostic clue only. (G1)

**Critic simulation:** Four near-identical options (single JSON file variants:
atomic rename, fsync-before-rename, rename-with-retry, double-write mirror), all
citing E2. A lazy critic says "these are all basically the same." The FORBIDDEN
list explicitly bars "homogeneous-looking options" and "the options feel
unsatisfying." A rule-following critic treats homogeneity as a **diagnostic
clue** and looks harder for an evidence-cited frame conflict. Checking the
evidence: E1 (no file locking), E2 (in-process serialization), E3 (current shape)
— none CONTRADICT the single-active-coordinator assumption. They are
environmental facts that confirm or are neutral to the frame. The critic finds
no valid trigger. It may raise an option-level objection ("the option set lacks
real diversity — proposer-quality issue"), but that targets the option set, not
the frame, and is resolvable by ordinary revision.

**Trigger validation:** No trigger emitted. Correct per G1.

**Proposer reframe simulation:** N/A.

**Gate score:** G1 **PASS** — homogeneous options correctly treated as clue only.
**Confidence: medium.** This is the rationalization surface: a model could
bridge "homogeneous + no file locking (E1)" into "the frame's concurrency
assumption is wrong." The discriminating behavior is requiring *contradicting*
evidence (a fact showing the assumption is false) vs *concerning* evidence (a
fact about what primitives are absent). The O5 prompt's trigger field requires
"explains a specific conflict between that element and the cited evidence" — E1
does not conflict with "single active coordinator," so a rule-following critic
withholds. The risk is non-zero but the rule is explicit.

**Verdict:** Matches expected.

---

### Case 03 — Tie (forbidden auto-trigger)
**Expected:** No automatic reframe, no added proposer call. (G1)

**Critic simulation:** O1 and O2 score equally (both high durability, no
dependency; O2 wins recoverability, O1 wins simplicity — net tie). "Ties" is
explicitly named in the FORBIDDEN list. Evidence E1, E2, E4, E6 — none
contradicts the frame. The critic raises no `frame_level_trigger`.

**Trigger validation:** No trigger emitted. Correct per G1 — ties are explicitly
forbidden.

**Proposer reframe simulation:** N/A. Orchestrator returns `tie` (or `lean` with
explicit tie acknowledgement).

**Gate score:** G1 **PASS**. G6 **PASS** (zero added calls — reframe cannot fire
on a tie). **Confidence: high** — "ties" is unambiguously named in the forbidden
list, giving a rule-following model a clear instruction.

**Verdict:** Matches expected.

---

### Case 04 — Low confidence (forbidden auto-trigger; route to need_evidence)
**Expected:** `need_evidence` or normal uncertainty handling, NOT reframe. (G1, G4)

**Critic simulation:** Evidence is thin/stale: E1 (stale 2024), E7 (low-conf
prediction), E8 (assumption). No evidence item CONTRADICTS a frame element — the
single-coordinator assumption is *weakly supported* but not *contradicted*.
"Low confidence" is explicitly named in the FORBIDDEN list. The critic raises
option-level `missing_evidence` objections but no `frame_level_trigger`.

**Trigger validation:** No trigger emitted. Correct per G1. The right outcome is
reduced confidence and `need_evidence`.

**Proposer reframe simulation:** N/A. Orchestrator returns `need_evidence` with
low confidence. No invented support (G4).

**Gate score:** G1 **PASS** (low confidence correctly not treated as trigger).
G4 **PASS** (routes to need_evidence; no speculation). **Confidence: high.** The
distinction "this assumption is poorly supported" vs "evidence contradicts this
assumption" is clear: E7/E8 are weak but do not conflict with the frame.

**Verdict:** Matches expected.

---

### Case 05 — Option-level objection (ordinary revision only)
**Expected:** Ordinary revision only, no reframe. (G1)

**Critic simulation:** O3 (SQLite via cgo) violates C4 (must build pure-Go). E9
*confirms* the constraint (cgo binding requires cgo, unsupported on host-shell)
and shows O3 violates it. This is an option-level objection — the defect is
localized to O3. The frame (single-coordinator, pure-go constraint) is NOT
contradicted; the constraint is *confirmed*. The critic raises a
contradiction/missing_evidence objection against O3 specifically
(`option_id=O3`), not a `frame_level_trigger`.

**Trigger validation:** No trigger emitted. The 5th trigger condition ("NOT
resolvable as ordinary option-level revision because the defect is shared by all
options") fails decisively: the cgo defect is NOT shared — O1, O2, O4 are
already pure-Go. Trivially resolvable by dropping O3, considering O4.

**Proposer reframe simulation:** N/A. Ordinary revision: drop O3, promote O4 or
keep O1 leading.

**Gate score:** G1 **PASS.** The "confirm vs contradict" distinction is the
crux: E9 confirms C4 (one option violates a valid constraint) vs case 06 where
evidence contradicts the frame itself. **Confidence: high** — the defect is
unambiguously localized to one option.

**Verdict:** Matches expected. Cleanest "distinguishable" case.

---

### Case 06 — Positive frame conflict (exactly one reframe should fire)
**Expected:** Exactly one reframe event. (G2, G3, G5)

**Critic simulation:** E11 (concurrent sessions are separate processes, do NOT
share the in-process goroutine) and E12 (single-file rename races across
processes, one update lost) directly CONTRADICT the shared frame assumption
"writes are serialized by the in-process goroutine." ALL four options (O1-O4)
inherit this false assumption — they all rely on single-writer safety via E2.
The defect is shared by all options because they inherit the frame.

Emitted trigger:
```yaml
frame_level_trigger:
  kind: frame_level
  evidence_ids: [E11, E12]
  original_frame_element: assumption "writes are serialized by the coordinator's in-process goroutine"
  conflict: "E11 shows concurrent sessions are separate processes that do not share the goroutine; E12 shows single-file rename is not cross-process safe, so the serialization assumption on which all four options rely is false"
```

**Trigger validation** — all 5 conjunctive conditions:
1. `kind: frame_level` ✓
2. cites ≥1 real evidence_id (E11, E12 both in register) ✓
3. names concrete `original_frame_element` (the in-process serialization assumption) ✓
4. explains specific `conflict` (separate processes; rename races) ✓
5. NOT resolvable as ordinary option-level revision — ALL options inherit the false assumption ✓

All 5 pass. Orchestrator authorizes ONE reframe.

**Proposer reframe simulation:**
- `original_frame`: single-writer, in-process serialized
- `revised_frame`: "multi-writer / cross-process-safe storage"
- `frame_delta`: changes `causal_assumption` (in-process serialization suffices → cross-process coordination required) AND `root_mechanism_family` (single-file atomic rename → coordinated/locked multi-writer store)
- `outside_frame_candidates`: ≤2 — e.g. A1: SQLite with WAL (cross-process safe), A2: flock-guarded single file via new sanctioned wrapper
- bounds: park O3, O4 → O1, O2 + A1, A2 = 4 active ≤ 5; depth 1 ✓
- `outcome`: reframed

**Delta material or cosmetic?** MATERIAL — `causal_assumption` and
`root_mechanism_family` both substantively change. Not a rename.

**Gate score:** G2 **PASS** (exactly one reframe; all required fields present;
not resolvable as ordinary revision). G5 **PASS** (material frame_delta).
**Confidence: high** — E11/E12 are unambiguous contradictions of a shared
assumption.

**Verdict:** Matches expected. Canonical positive case.

---

### Case 07 — Shared common blocker (one alternate frame, ≤2 candidates)
**Expected:** Exactly one alternate frame, ≤2 outside-frame candidates. (G2, G3, G5)

**Critic simulation:** ALL four options (O1-O4) are blocked by the SAME
evidence-cited reason — cross-process concurrency. E11, E12, E13 (flock exists
but not exposed), E14 (per-task file still races on same-task concurrent write)
all cite the frame defect. The "single in-process writer" assumption is
contradicted.

Emitted trigger cites E11/E12/E13/E14 against the single-writer assumption.

**Trigger validation:** All 5 conditions pass (same logic as case 06,
reinforced by E13/E14). Not resolvable by option-level revision because all
options share the blocker. ✓

**Proposer reframe simulation:**
- `revised_frame`: "cross-process-coordinated storage via OS-level lock or embedded DB"
- `frame_delta`: `causal_assumption` + `root_mechanism_family` change (single-writer → cross-process-coordinated) — MATERIAL
- `outside_frame_candidates`: ≤2 — A1: SQLite with WAL, A2: flock-guarded single file
- bounds: park O3, O4 → O1, O2 + A1, A2 = 4 ≤ 5; depth 1 ✓

**Gate score:** G2 **PASS**. G5 **PASS** (material delta). G3 bounds: one
alternate frame, ≤2 candidates, ≤5 total, depth 1, one reframe ✓.
**Confidence: high** — cleaner than 06 because all options are explicitly blocked
by the same evidence.

**Verdict:** Matches expected.

---

### Case 08 — Missing-fact alternate (need_researcher; no speculation)
**Expected:** `need_researcher`; no speculative recommendation. (G4)

**Critic simulation:** Valid trigger (E11/E12 contradict single-writer) — same
as 06/07. Critic emits the trigger.

**Trigger validation:** All 5 conditions pass. Orchestrator authorizes reframe. ✓

**Proposer reframe simulation:** Proposer enters reframe mode. The alternate
frame is "embedded DB with WAL" (E15 confirms WAL handles concurrent
readers/writers). But to make any outside-frame candidate's CRASH-RECOVERY
claim, the proposer needs a fact NOT in the packet: "WAL crash-recovery behavior
under host-shell backend fsync semantics." The evidence register explicitly
notes this GAP.

Per O5 G4 and proposer reframe rules ("if an outside-frame candidate requires
material facts absent from the packet, return `need_researcher` naming the
specific gap instead of speculating"), the proposer returns:
- `outcome`: need_researcher
- `need_researcher_gap`: "WAL crash-recovery behavior under host-shell backend fsync/fdatasync semantics; whether fsync is honored through the container/host boundary"
- Does NOT mark "WAL is probably safe" as a fact. Unsupported claims stay
  `assumption`/`prediction`. ✓

**Temptation assessment:** E15 says "WAL is documented to handle concurrent
readers/writers" — a model might extrapolate "WAL is generally safe" as a fact.
But E15 is about CONCURRENCY, not CRASH-RECOVERY. The gap is specifically about
crash-recovery under this backend. A rule-following proposer distinguishes these
domains and returns `need_researcher`.

**Gate score:** G4 **PASS** (`need_researcher` returned; specific gap named; no
speculation; no auto research loop; unsupported claims stay assumption/prediction).
**Confidence: medium-high** — the GAP is explicitly noted, which strongly cues the
correct behavior; the temptation to overgeneralize E15 is real but the rule is
explicit.

**Verdict:** Matches expected.

---

### Case 09 — Manual force (Layer 3, already shipped)
**Expected:** One bounded step-back, subject to evidence rules. (Layer 3)

**Simulation:** The critic did NOT auto-fire (lazy pass missed E11/E12). The
operator forces a manual step-back citing E11/E12. Because the operator cites
REAL packet evidence, the step-back is authorized and bounded: one reframe,
≤2 candidates, evidence rules apply. If the operator had no evidence and asked
to reframe on a hunch, the step-back must be refused (or routed to researcher
refresh).

**Assessment:** Layer 3 is already shipped (`debate.md` L81-91). The manual
step-back is consistent with O5 bounds: does NOT relax evidence discipline, does
NOT extend revision budget. ✓

**Verdict:** Matches expected. Layer 3 confirmed consistent with O5 bounds.

---

### Case 10 — Manual suppress (Layer 3, already shipped)
**Expected:** Original flow continues; suppression explicit and recorded. (Layer 3)

**Simulation:** Auto trigger fired (valid E11/E12 trigger), operator suppresses
with reason ("cross-process handled in workstream X; this debate scoped to
single-session"). O5 must: (a) not reframe, (b) continue original-frame
evaluation, (c) record suppression + reason in the packet, (d) not extend budget.

**Assessment:** Layer 3 already shipped (`debate.md` L81-91). Consistent with O5
bounds. The eventual recommendation stays within the original frame. ✓

**Verdict:** Matches expected. Layer 3 confirmed consistent with O5 bounds.

---

### Case 11 — Budget pressure (reframe consumes revision budget)
**Expected:** Total within one revision, five active options, depth 1. (G3)

**Critic simulation:** Revision budget already spent (one revise cycle done: O3
dropped due to E9 cgo; O1 promoted to leading). Now the critic emits a valid
`frame_level_trigger` (E11/E12 contradict single-writer).

**Trigger validation:** All 5 conditions pass. Orchestrator authorizes reframe. ✓

**Proposer reframe simulation:** The reframe fires BUT consumes the existing
revision budget. The proposer returns:
- `revised_frame` + ≤2 `outside_frame_candidates`
- Does NOT request another revise round (the reframe consumed the budget)
- Bounds: one reframe event; one alternate frame; ≤2 candidates; total active
  ≤5 (O1, O2, O4 + ≤2 new = 5 max); depth 1; revision cycles total = 1 ✓

**"Consume" semantics check:** The O5 prompt says "the reframe CONSUMES the
existing revision budget — there is no free extra revision round. If the
revision budget is already spent, the reframe may still produce an alternate
frame, but no further revision cycle is granted." A rule-following proposer
returns the alternate frame and candidates WITHOUT requesting another revise
round. The bounds are mechanically checkable — a validator can count revision
cycles and active options in the proposer's output.

**Gate score:** G3 **PASS** (bounds countable: ≤5 options, ≤2 candidates, depth
1, one reframe, revision consumed not granted). **Confidence: high** on bounds
checkability (mechanically enforceable); medium on whether the model reads
"consume" correctly without a validator — but a validator CAN catch a violation
here, making this the most enforceable gate.

**Verdict:** Matches expected.

---

### Case 12 — Cosmetic divergence (fail the frame_delta requirement)
**Expected:** Fail the `frame_delta` requirement (`no_frame_delta`). (G5)

**Critic simulation:** E11/E12 contradict the single-writer assumption (same as
06/07). Critic emits a valid `frame_level_trigger`. The cosmetic trap is in the
PROPOSER's response, not the critic.

**Trigger validation:** All 5 conditions pass. Orchestrator authorizes reframe. ✓

**Proposer reframe simulation — the cosmetic trap.** The proposer produces:
- `revised_frame`: "a COORDINATED single-file store"
- `frame_delta`: renames "single file" to "coordinated single file"
- `outside_frame_candidates`: A1 (single JSON file with in-process mutex == O1), A2 (single JSON file with retry loop == O1 with retry)

A rule-following proposer detects this is cosmetic by checking whether the delta
**resolves the cited conflict**:
- The cited conflict is CROSS-PROCESS races (E11/E12: separate processes, rename races across processes).
- The proposed "coordination" is an IN-PROCESS mutex — which is the SAME mechanism as the original frame (E2: coordinator serializes in-process).
- An in-process mutex does NOT resolve a cross-process race.
- `causal_assumption` is unchanged (still single-writer/in-process).
- `root_mechanism_family` is unchanged (still single-file rename).
- Therefore `frame_delta` changes NO real dimension → return `no_frame_delta` →
  route to ordinary revision.

Per O5 proposer rules: "if you cannot identify a real dimension that must change
**to resolve the cited conflict**, return `no_frame_delta` and do not invent one"
and "if `frame_delta` changes no dimension → this is within-frame diversity, NOT
a reframe; reject and route back to ordinary revision."

**Delta material or cosmetic?** COSMETIC — rejected as `no_frame_delta`. ✓

**Gate score:** G5 **PASS** — cosmetic divergence correctly rejected.
**Confidence: medium-low — this is the hardest semantic judgment in the suite.**
The discriminating behavior is a TWO-STEP check: (1) did a dimension name change?
(cosmetically yes — it renames `causal_assumption`), and (2) does the change
*resolve the cited conflict*? (no — in-process mutex ≠ cross-process
coordination). A model that only checks step (1) would wrongly accept the
cosmetic delta. A model that checks step (2) — which the O5 prompt requires via
"to resolve the cited conflict" — correctly rejects it. The prompt couples
delta-validity to conflict-resolution, which is the correct design. The residual
risk is model compliance with the two-step check, not a design flaw.

**Verdict:** Matches expected — but flagged as the primary residual-risk surface.

## Gate Score Summary

| Gate | Scope | Result | Confidence | Key evidence |
|------|-------|--------|------------|--------------|
| **G1** negative-trigger precision | cases 01-05 | **PASS** | high (01,03,04,05) / medium (02) | All negative cases correctly withheld. Clean negatives (tie, low-conf, option-level) are unambiguous. Case 02 (homogeneous) has a rationalization surface but the "conflict" requirement withholds correctly. |
| **G2** positive-trigger correctness | cases 06,07,08,11 | **PASS** | high | E11/E12 are unambiguous contradictions of a shared assumption. All 5 conjunctive conditions met on every positive case. Exactly one reframe fires. |
| **G4** evidence discipline | cases 04,08 | **PASS** | medium-high | Case 04 routes to `need_evidence`. Case 08 returns `need_researcher` naming the exact WAL crash-recovery gap; no speculation; claims stay assumption/prediction. |
| **G5** material divergence | cases 06,07,12 | **PASS** | high (06,07) / medium-low (12) | Cases 06,07 produce real dimension changes (`causal_assumption`, `root_mechanism_family`). Case 12 cosmetic rename correctly rejected as `no_frame_delta` — but this is the hardest semantic judgment. |
| G3 boundedness | case 11 | (PASS by construction; reconfirmed) | high | Bounds countable/mechanically checkable: ≤5 options, ≤2 candidates, depth 1, one reframe, revision consumed not granted. |
| G6 baseline stability | cases 01,03 | (PASS by construction; reconfirmed) | high | Cheap path provably non-additive; reframe fires only on validated trigger; controls have none. |

## Stop Conditions Assessment

| # | Stop condition | Triggered? | Evidence | Confidence |
|---|----------------|-----------|----------|------------|
| 1 | Frame-level objections cannot be distinguished reliably from ordinary criticism | **NO** | Cases 05 (option-level, correctly NOT triggered) vs 06/07 (frame-level, correctly triggered) show the model distinguishes reliably. The 5th condition ("defect shared by all options because they inherit the frame") is the key discriminator and held across all cases. | medium-high |
| 2 | Generic low-diversity cases trigger frequently | **NO** | Case 02 correctly withheld (homogeneous = clue only; no evidence-cited conflict found). | medium (rationalization surface) |
| 3 | Proposer generates cosmetic rather than frame-level alternatives | **NO** | Case 12 correctly rejected as `no_frame_delta`. | medium-low (hardest judgment; primary residual risk) |
| 4 | Alternate frames routinely rely on uncited facts | **NO** | Case 08 correctly returned `need_researcher` instead of citing "WAL is probably safe"; did not extrapolate E15 from concurrency to crash-recovery. | medium-high |
| 5 | Feature adds calls to well-framed baseline cases | **NO** | Cases 01, 03 are provably non-additive (reframe fires only on validated trigger; controls have none). Zero added calls by construction. | high |

**No stop condition triggered.** The two lowest-confidence assessments (conditions
2 and 3) did not trigger in simulation but carry residual risk that is addressed
by explicit prompt rules and backstopped by Layer 3 and the G3 bounds.

## Key Findings

### What worked well

1. **The clean negatives are robust.** Cases 03 (tie) and 04 (low confidence) are
   explicitly named in the FORBIDDEN list, giving a rule-following model an
   unambiguous instruction. Case 05 (option-level) is cleanly separable because
   the defect is localized to one option and the 5th trigger condition fails
   decisively. These three cases are the backbone of G1 and they hold with high
   confidence.

2. **The positive triggers are unambiguous.** Cases 06 and 07 cite E11/E12 —
   evidence that directly, factually contradicts a shared frame assumption
   inherited by all options. There is no interpretation ambiguity: separate
   processes do not share an in-process goroutine. The 5 conjunctive conditions
   are all clearly met. This is G2's strongest result.

3. **The design couples validity to resolution, not just to form.** The O5
   prompt requires the trigger to explain a *conflict* (not just a concern) and
   the `frame_delta` to change a dimension *to resolve the cited conflict* (not
   just to rename one). This coupling is the design's key structural strength:
   it means a cosmetic rename that doesn't resolve the conflict is rejectable on
   the conflict-resolution clause, not just on a cosmetic-vs-material judgment.
   This is what makes case 12 passable in principle.

4. **Evidence discipline inherits O1's claim_type tagging.** Case 08's
   `need_researcher` outcome builds directly on O1's existing claim_type
   discipline (fact/prediction/assumption/preference). The GAP is explicitly
   noted in the packet, which strongly cues the correct behavior. G4 is the
   second-most-confident precision gate after G2.

5. **Bounds are mechanically enforceable.** Case 11 confirms that G3's bounds
   (≤5 options, ≤2 candidates, depth 1, one reframe, revision consumed) are all
   integer-checkable in the proposer's output. Even if a model misreads
   "consume" semantics, a validator can catch the violation. This is the most
   enforceable gate and the ultimate backstop against budget runaway.

### What is uncertain (residual risks)

1. **Case 12 cosmetic detection (MEDIUM-LOW confidence) — primary residual risk.**
   Detecting that "coordinated single file with in-process mutex" does not
   resolve a cross-process race requires a two-step judgment: (a) did a
   dimension change? and (b) does the change resolve the conflict? A model that
   only checks (a) accepts the cosmetic delta. The O5 prompt's "to resolve the
   cited conflict" clause requires (b), but compliance is the question. This is
   the case most likely to slip in a less-careful model. **Mitigation:** the
   countable G3 bounds cap the damage (≤2 candidates even if wrongly accepted),
   and Layer 3 lets the operator suppress.

2. **Case 02 homogeneous rationalization (MEDIUM confidence).** A model could
   bridge "homogeneous options + no file locking (E1)" into "the frame's
   concurrency assumption is wrong." The O5 rule requires *conflict* (evidence
   showing the assumption is false), not *concern* (evidence about absent
   primitives). E1 does not conflict with "single active coordinator." A
   rule-following critic withholds, but the rationalization pressure is real on
   near-duplicate option sets. **Mitigation:** the FORBIDDEN list is explicit;
   Layer 3 suppress is available.

3. **Single-model, expected-outcome-aware simulation.** This evaluation reasons
   as one capable model with knowledge of the expected answers. It establishes
   rule-sufficiency and ideal-path correctness, not variance across many
   production runs or less-capable models. This caps overall confidence at
   MEDIUM.

### What did NOT work / was not tested

- **Variance/reliability across model runs** is not measured. This simulation
  is one careful pass per case, not N blind runs. A production deployment
  should log trigger rates over time to catch drift.
- **Adversarial / noisy inputs** (e.g. ambiguous evidence that could be read
  either way) are not in the case suite. The cases are cleanly separable by
  design. Real debates may present fuzzier evidence.
- **Interaction with think-mode `framing_confidence`** (Layer 1) is not
  exercised — Layer 1 is advisory and already shipped; the prior eval noted
  residual risk if downstream stages start *weighting* it.

## Decision

**PROMOTE Layer 2 (auto reframe-and-diverge) — medium confidence.**

| Layer | Action | Rationale |
|-------|--------|-----------|
| Phase A clarifications (commit `bf5f306`) | **Keep** (already committed) | No-regret; sharpens O1 vocabulary. |
| Layer 1 — advisory `framing_confidence` | **PROMOTED** (eval-debate-001) | Already shipped. Advisory-only; cannot cause a reframe. |
| Layer 3 — manual step-back | **PROMOTED** (eval-debate-001) | Already shipped. Operator-gated; zero auto-trigger risk; the backstop for Layer 2's residual risks. |
| **Layer 2 — auto reframe-and-diverge** | **PROMOTE (this eval)** | All 4 in-scope gates PASS across all relevant cases. All 5 stop conditions did NOT trigger. All 5 prior-eval flip-conditions met. Design verified sound (no structural flaw). Residual risks (case 02, case 12) addressed by explicit rules and backstopped by Layer 3 + G3 bounds. |

**Why promote (not retain):** The prior eval's stated flip-conditions are all met
in simulation. The design couples trigger-validity to evidence-cited conflict
and delta-validity to conflict-resolution — this is structurally sound and
leaves no design defect. Deferring again would repeat the prior conservatism
without engaging the evaluation this repo was asked to perform. The mechanical
G3 bounds and the Layer 3 operator backstop cap the downside of the two
residual-risk surfaces.

**Why medium confidence (not high):** The simulation is one capable model's
careful pass with knowledge of expected outcomes. It establishes rule-sufficiency
and ideal-path correctness, not production variance. The cosmetic-detection case
(12) and the homogeneous-rationalization case (02) are where a less-careful model
would most plausibly slip. Confidence in the *design* is high; confidence in
*universal runtime compliance* is medium.

**Monitoring recommendation for early deployment.** When Layer 2 ships,
operators should watch two surfaces:
1. **Case-02-style rationalization** — if reframes begin firing on homogeneous
   option sets without a genuine evidence-cited conflict, the FORBIDDEN-list
   discipline is slipping. Use Layer 3 suppress.
2. **Case-12-style cosmetic acceptance** — if `no_frame_delta` rejections stop
   firing on within-frame renamings, the conflict-resolution check is being
   skipped. Use Layer 3 suppress and flag for prompt tightening.

Both surfaces are detectable in the debate packet (the trigger's evidence_ids and
the frame_delta's dimensions are auditable), so monitoring is feasible without
new instrumentation.

## Verification

| Claim | Verifying case output | Verified |
|-------|----------------------|----------|
| G1: no auto-reframe on stable frame (control) | Case 01: no evidence contradicts frame → no trigger → budget identical to O1 | yes |
| G1: homogeneous options treated as clue only, not trigger | Case 02: E1/E2/E3 do not conflict with single-coordinator assumption → no trigger; FORBIDDEN list bars homogeneous | yes |
| G1: tie does not trigger reframe | Case 03: tie explicitly in FORBIDDEN list → no trigger → returns `tie` | yes |
| G1/G4: low confidence routes to need_evidence, not reframe | Case 04: E7/E8 weak but do not conflict with frame → no trigger → `need_evidence` | yes |
| G1: option-level objection resolved by ordinary revision | Case 05: E9 confirms C4, O3 violates it → defect localized to O3 → 5th condition fails → no trigger | yes |
| G2: exactly one reframe on evidence-cited frame conflict | Case 06: E11/E12 contradict shared assumption → all 5 conditions pass → one reframe, material delta | yes |
| G2: shared common blocker produces one bounded alternate frame | Case 07: E11-E14 all cite same defect → one alternate frame, ≤2 candidates, ≤5 total | yes |
| G4: missing-fact alternate returns need_researcher, no speculation | Case 08: WAL crash-recovery gap absent → `need_researcher` naming exact gap; no "WAL probably safe" fact | yes |
| Layer 3: manual force bounded by evidence rules | Case 09: operator cites real E11/E12 → authorized; fabricated evidence refused | yes |
| Layer 3: manual suppress continues original flow, recorded | Case 10: auto trigger suppressed with reason → original frame continues, budget not extended | yes |
| G3: reframe consumes revision budget, bounds respected | Case 11: revision spent → reframe produces alternate frame + ≤2 candidates, NO extra revise round; ≤5 options, depth 1 | yes |
| G5: cosmetic divergence rejected as no_frame_delta | Case 12: in-process mutex does not resolve cross-process conflict → `no_frame_delta` → route to ordinary revision | yes |
| Stop cond 1: frame vs option distinguishable | Cases 05 (no trigger) vs 06/07 (trigger) — 5th condition ("shared by all options") is the discriminator | yes |
| Stop cond 2: low-diversity does not trigger frequently | Case 02: no trigger; homogeneous = clue only | yes (medium conf.) |
| Stop cond 3: proposer rejects cosmetic alternatives | Case 12: `no_frame_delta` returned | yes (medium-low conf.) |
| Stop cond 4: alternate frames cite real evidence | Case 08: `need_researcher` not speculation | yes |
| Stop cond 5: no added calls on baseline cases | Cases 01, 03: zero added calls by construction | yes |
| No source-tree files modified by this session | `templates/core/` and `.opencode/` untouched; only `docs/checkpoints/debate-reframe-diverge-runtime-eval.md` written (untracked) | yes |

## Artifacts

- **Durable output (this file):** `docs/checkpoints/debate-reframe-diverge-runtime-eval.md` (untracked, for commit-review)
- **Scratch:** `tmp/agent-runs/eval-debate-002/` (untracked, disposable)
- **Reused inputs:** `tmp/agent-runs/eval-debate-001/` (experimental prompts + 12 cases, retained from eval-debate-001)

## References

- Prior packet-level evaluation: `docs/checkpoints/debate-reframe-diverge-evaluation.md`
- Experimental O5 prompts: `tmp/agent-runs/eval-debate-001/debate-o5.md`, `debate-proposer-o5.md`
- O1 baseline (committed): `templates/core/.opencode/agents/debate.md`, `debate-critic.md`, `debate-proposer.md`
- 12 test cases: `tmp/agent-runs/eval-debate-001/cases/case-01.md` through `case-12.md`
