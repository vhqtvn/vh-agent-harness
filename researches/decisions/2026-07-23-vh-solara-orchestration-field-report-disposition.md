# Decision: vh-solara Orchestration Field Report — Maintainer Disposition

**Date:** 2026-07-23
**Status:** Accepted (disposition + record-of-decision). No code lands in this slice.
Verdicts are recorded; deferred items carry named triggers; adopted items become
backlog intake for future slices; one report sub-claim is refuted and one
recommendation rejected.
**Supersedes:** none.
**See also:**
[`../sources/2026-07-23-vh-solara-harness-adoption-field-report.md`](../sources/2026-07-23-vh-solara-harness-adoption-field-report.md)
(the evidence, preserved verbatim with a provenance header).
[`../../docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`](../../docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md)
(failure taxonomy F1–F7, §4 mechanism menu).
[`./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`](./2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md)
(owned scope §4.3/§4.1/§4.2; the authority line this disposition engages).
[`./2026-07-23-release-defer-dual-mechanism-reconciliation.md`](./2026-07-23-release-defer-dual-mechanism-reconciliation.md)
(union fail-closed; dual mechanisms must not collapse).
[`./2026-07-05-commit-gate-shared-file-coupling.md`](./2026-07-05-commit-gate-shared-file-coupling.md)
(W1/W4/G1 — the decided position the report's Pattern 4 re-opens).

## Framing

An external consuming repo (vh-solara) sent a field report on the orchestration
layer. It makes claims of two classes:

- **Class A — consuming-repo claims** (vh-solara transcripts, commits, DB rows,
  code anchors). These cannot be re-derived in this repo. They are treated as
  *asserted-by-reporter, unverified-by-us*. The report itself flags this ("Every
  claim below cites a transcript, a commit, or a DB row. Metadata was never
  trusted on its own").
- **Class B — repo-class claims** (about THIS repo's contracts/tooling: AGENTS.md
  wording, commit-gate.sh behavior, agent capabilities, task-contract/closeout
  formats, commit-reviewer dispositions). **Every one was verified against the
  actual files.** Verdicts are in §1.

The decisive meta-finding: **every Class B structural gap stands independent of
the Class A evidence.** Six of the report's seven patterns (1, 2, 3, 4, 5, 7)
rest on a verified structural absence that holds whether or not the consuming-repo
instance occurred. Only Pattern 6 (band-aid loops) is load-bearing on Class A
evidence (see §3, §4). This means the disposition can adopt the structural
findings without depending on un-re-derivable consuming-repo proof.

## 1. Claim verification (Class B)

| ID | Report claim (condensed) | Verdict | Citation |
|----|--------------------------|---------|----------|
| RC1 | AGENTS.md testing contract has no load-bearing-path / crux / behavioral-coverage notion; treats "a test in the right dir passed" as sufficient | **CONFIRMED** | `AGENTS.md` Testing rules ("add or update appropriate verification"; "verified testing seam localization"); grep for `load.bearing.path\|crux\|behavioral.coverage` across `AGENTS.md`, `.opencode/`, `templates/core/`, `docs/ai/` → empty |
| RC2 | "no requirement that a lane emit a single machine-checkable verdict token at a fixed position" | **PARTIALLY CONFIRMED** (report overstates) | Closest existing: checkpoint Verification table (`.vh-agent-harness/docs/opencode-session-workflow.md:191-199`) + Structured Findings confidence (`:201-205`) + "Final Response Format" capture (`AGENTS.md`). These tie claims to verifying commands but are multi-row prose, NOT a fixed-position enum token. Gap is "no single token," not "no verification convention." |
| RC3 | commit-gate.sh is per-invocation/stateless; no cross-lane or HEAD-staleness signal | **CONFIRMED** | `.opencode/scripts/commit-gate.sh`: `head_at_acquire` per-session only; statuses `acquired/no_changes/path_error/error/contended/cas_conflict/committed/released/free/held/stale/heartbeat_refreshed/rebased`; no cross-session HEAD-movement aggregation |
| RC4 | private-index gate stages from shared working tree; concurrent-same-file hunks coexist (not separable) | **CONFIRMED** | Wording `AGENTS.md:166` ("mechanically excluded by the private-index gate"); gate `commit-gate.sh:702,731,740` (`GIT_INDEX_FILE` private index over single shared checkout) |
| RC5 | harness HAS `isolation: "worktree"` as an agent capability, not default for build lanes | **REFUTED** | grep `isolation:` across `.opencode/agents/`, `.opencode/commands/`, `templates/core/.opencode/` → empty. No agent/subagent schema has an `isolation` field. All `isolation` matches are runtime-backend (host-shell/bare/docker) in `run-shape.yml`, a repo-wide setting, NOT a per-lane capability. **The report's premise is false.** |
| RC6 | prompt-guide asks for "settled assumptions" but no motivation-satisfaction check in closeout | **CONFIRMED** | `templates/docs/opencode-prompt-guide.md` (settled assumptions + load-bearing-premises 4-tuple); no closeout/contract template asks "does this design achieve its stated motivation?" |
| RC7 | commit-reviewer findings can be `drop`; advisory "not exercised e2e" drops out of the durable ledger | **CONFIRMED** | `.opencode/agents/commit-reviewer.md`: disposition vocab = `block\|defer\|drop`; `drop` recorded in `dropped_findings[]` (visible, NON-gating); ONLY `defer` routes to `.local/coordinator/tasks/` (`source:review-defer`) |
| RC8 | memory opt-in/human-curated; findings not a required closeout artifact; handoffs carry diff+next-task not accumulated knowledge | **CONFIRMED (structure); PARTIALLY REFUTED (handoff detail)** | No closeout template requires a findings-delta (`.opencode/commands/task-closeout.md`). Handoffs DO carry a Structured Findings section (`.opencode/commands/handoff-save.md:34-38`) — more than diff+next-task — but these are session-scoped, NOT accumulated across sessions. Typed-memory substrate (`internal/memory/store/`) is DORMANT (no `records.jsonl` anywhere). |

## 2. Shipped mechanisms already in this problem space (verified)

| Commit | Mechanism | Failure class | State |
|--------|-----------|---------------|-------|
| `69e0104` | doctor check #12 — §4.3 generic defer-liveness release gate (`internal/cli/release_gate.go`); FAIL on open-card↔released-claim contradiction; fail-closed on malformed cards | F1 / §4.3 | shipped + active; **gate ACTS** (authority line respected) |
| `c197171` | §4.1 claim/verifier closure kernel over typed-memory substrate (`internal/memory/claims/`); in-memory re-derivation (not persisted) with `SourceRef` provenance | §4.1 | shipped + active; **read/inform only** — never writes canon, never gates directly |
| `0f5b104` | §4.2 premise-recheck 4-tuple `(value, source, re_derivation_command, observed_at)` injected into session-workflow, prompt-guide, handoff-save, resume-task | §4.2 / F2 | shipped + active as **protocol (discipline, not gate)** |
| `da09e9e` | wired doctor + staged-errata gates into release ceremony (G0c) — closes the release-defer-dual third failure mode | §4.3 staging hole | shipped + active |

The substrate this disposition reuses is the DORMANT typed-memory store
(`internal/memory/store/`); the claims kernel is its first (release-gate-scoped)
consumer.

## 3. Reconciliation: merge into the taxonomy, or stand separate?

**Verdict: MERGE + EXTEND. Do not let the report's patterns stand as a parallel
taxonomy.** The report is organized by *symptom surface* (green-tests,
metadata-reporting, commit-freeze, tangle, design-drop, band-aid, re-discovery);
the case study is organized by *lossy boundary* (compression, delegation,
release, time, authoring, design). They are two views of the one underlying event
the case study states once: *a claim crossed a lossy boundary without being
re-checked, and the receiving side treated it as truth.* Standing them up as a
separate taxonomy would be a dual derivation of "what went wrong" — exactly the
smell §4.4 forbids.

| Report pattern | Case-study class | Relation |
|----------------|------------------|----------|
| 1 green-tests/broken-product | F4 (node-tested/path-untested) + §4.5 | **EXTENDS — and flags a false closure** (see below) |
| 2 metadata not ground truth | F2 (fact staleness) + §4.2 | MERGE (new instance of F2 at the compression→coordinator seam) |
| 3 silent commit-gate failures | none (NEW class) | EXTENDS taxonomy — propose F8 |
| 4 concurrent-lane tangle | none in case study; 2026-07-05 W4/G1 | EXTENDS — but re-opens a DECIDED position |
| 5 core-principle dropped in design | F2 (staleness at the design boundary) | MERGE |
| 6 band-aid loops | F1 (recurrence) + §4.3 | MERGE — but Class-A-dependent (weak) |
| 7 re-discovery / no durable memory | F1 + §4.3 | MERGE |

**False-closure flag (load-bearing).** The 2026-07-22 decision memo marks F4 as
DONE via `ae5b30d` (CoreOutputs). That closure is REAL but NARROW:
`ae5b30d` is capability-owned *output filtering* (media-perception rendering), not
*test-path coverage*. The report's Pattern 1 is a different instance of F4 — the
*testing* contract cannot distinguish "the test exercises the load-bearing path"
from "a test in the right directory passed." Reading "F4 closed" as "the
node-vs-path problem is solved" is wrong. This does not reopen the
output-filtering closure; it narrows the claim. A defer card is filed so no one
treats F4 as fully closed for testing.

**Dual-derivation position (weighed).** §4.4 ("declare once, derive everywhere")
and the release-defer-dual memo ("union fail-closed, do not collapse two
distinct failure modes into one") both govern how the report's proposed signals
should land:

- Where the report proposes a PARALLEL "done" signal (verdict token vs
  green-tests/diff-review), the disposition MERGES them: the verdict token is the
  single declared source of done-ness; green-tests and diff-review *feed* it, not
  rival it. One declaration, derived everywhere.
- Where the report proposes a genuinely NEW property (HEAD-staleness), it stands
  as its own gate (union, per release-defer-dual), NOT derived from the
  defer-liveness gate. Different vocabulary, different input, both must pass.

No position is violated.

**Bearings on parked/deferred questions:**

- **`defer-002` (symptom-signature-stability):** the report's Pattern 6 recurrence
  detector is *blocked* on this (a detector on an unstable signature is noise).
  The report does not resolve the signature problem; it assumes it. Standing
  unchanged — still parked — and the report's single Class-A instance does not
  escalate it.
- **F4-closure-narrowness:** NEW flag raised by this report; changes the standing
  of "F4 closed" from settled to narrow. Filed as a defer card.
- **Async-recovery runtime (deferred, limitation #3 of the 2026-07-22 memo):** the
  report's Pattern 3 HEAD-watchdog *could* tempt a background observer. This
  disposition routes it to a **synchronous doctor check** instead, so it does NOT
  reopen the no-background-runtime decision. Consistent.

## 4. Disposition per recommendation

Adopted items are recorded here as next-slice backlog intake (NO code this slice).
Deferred items carry named triggers and are filed as cards in §6. The release in
flight is v0.15.0 (HEAD `f585702`); no adopted item is release-blocking — all are
post-v0.15.0 improvements.

| Report rec | Verdict | Owning seam | core/overlay | Cost | Ordering | Evidence to change verdict |
|------------|---------|-------------|--------------|------|----------|----------------------------|
| P0-A verdict token + crux gate | **ADOPT (split)** | closeout/contract templates + AGENTS core testing section | core (domain-free) | low-med | next slice | a counter-instance where a verdict token would not have helped |
| P0-A "not-exercised" finding → defer-not-drop | **ADOPT** | commit-reviewer agent prompt + gated-commit skill | core | low | next slice (cheap, release-safe) | none — verified gap (RC7) |
| P0-B HEAD-progress + commit-failure surfacing | **ADOPT (gate-shaped)** | commit-gate.sh + doctor + closeout format | core | low-med | next slice | none — verified gap (RC3) |
| P1-A default worktree isolation | **REJECT (W4); DEFER (G1 + file-scope lease)** | runtime / commit-gate / dispatcher | core | high (W4/G1), med (lease) | long-term | runtime change (W4); G1 validation packet; ≥1 more tangle instance |
| P1-B motivation-satisfaction check | **ADOPT (template checklist)** | prompt-guide + closeout templates + task-contract non-negotiables | core | low | next slice | none — verified gap (RC6) |
| P2-A recurrence detector | **DEFER (blocked on defer-002)** | new tool, downstream of symptom-signature | core | med-high | long-term | defer-002 resolved; ≥1 more band-aid instance |
| P2-B findings-delta closeout | **ADOPT (field); DEFER (auto-load)** | closeout template (field); typed-memory substrate (auto-load) | core | low (field), med (auto-load) | next slice (field) | none (field); file→memory mapping design (auto-load) |
| Cross-cut compaction ≠ completion | **ADOPT (merge into verdict token)** | closeout/verdict convention | core | low | with P0-A | none |

### 4.1 P0-A — split, because it conflates two seams

The report's "behavioral done gate" bundles (a) *did we test the load-bearing
path* (a testing/§4.5 concern) with (b) *did the lane report honestly* (a
closeout/verdict concern). These are related but separable, and the split is
load-bearing:

- **Crux / load-bearing-path clause (ADOPT).** Add to the AGENTS core testing
  contract: for any change whose value is a runtime behavior, the lane names the
  *specific* path the change fixes, the *specific* verification that exercises it,
  and asserts whether the sanctioned seam actually reaches it; a lane that cannot
  self-declares `crux_unproven`. Generic and domain-free → `templates/core`
  (AGENTS.core.md testing section). This is §4.5 territory, currently UNOWNED.
- **Verdict token (ADOPT, MERGED — not parallel).** Extend the existing
  `Final Response Format` / closeout convention (do not invent a parallel system)
  to carry a structured verdict enum `proven | inconclusive | failed | abandoned`
  plus a `crux: proven | skipped | not-demonstrable` sub-field. `crux_unproven`
  maps to `inconclusive`. This is the single declared source of done-ness;
  green-tests and diff-review feed it. Honest-limitation (same as the 2026-07-22
  memo's #1): a verdict token only helps if lanes emit it honestly — it is a
  §4.2-style *softened* mechanism at the compression boundary, not a hard gate.
- **"not-exercised" finding → defer-not-drop (ADOPT).** Change the commit-reviewer
  policy: a finding of the form "not exercised end-to-end / crux-not-covered" may
  not be `drop`; it must `defer` (routing to `.local/coordinator/tasks/`) so it is
  never lost from the durable record. This is the highest-leverage, lowest-cost
  item in the report. NOTE a refinement over the report: the report wants these
  findings *defer-blocking*; the disposition makes them *defer-not-drop* (persist)
  while the *blocking* is owned by the verdict token (a lane declaring
  `crux_unproven → inconclusive` blocks "complete"). Two mechanisms, not one — less
  aggressive, and it does not block legitimate commits merely because e2e is thin.
- **Coordinator-surfaces-weakest-claim → CONVERT.** The report gives the
  coordinator enforcement ("MUST surface"). The disposition handles this
  structurally: if the closeout MUST carry a verdict token, the coordinator reads
  it; no coordinator authority is granted (see §5).
- **Full §4.5 behavioral-parity-matrix (DEFER).** The OFF-row, route-not-node test
  infrastructure is the case study's highest test-authoring cost item; filed as a
  defer card.

### 4.2 P0-B — adopt, but gate-shaped (no coordinator authority)

RC3 confirms the commit-gate is per-invocation with no HEAD-staleness signal — a
genuinely new failure class (propose F8). The report's fix gives the coordinator
transition authority ("coordinator must reject a 'committed' closeout"). Converted:

- **Committer closeout records pre/post HEAD (ADOPT).** The gate already records
  `head_at_acquire`; adding post-commit HEAD to the closeout is low-cost.
- **Doctor HEAD-staleness check (ADOPT, synchronous).** A `doctor` check — the
  proven §4.1 host — that compares last-successful-committer-closeout timestamp
  vs current HEAD and WARNs when N successful closeouts precede an unchanged HEAD.
  This is a synchronous boundary read (doctor runs when invoked), consistent with
  the no-background-runtime decision. The doctor INFORMS (warn); it does not give
  the coordinator the power to reject.
- **Distinct commit-gate status for could-not-land (ADOPT).** Extend the gate's
  status vocabulary with `no_head_progress` / `could_not_land` (reason:
  tangle/build-fail/test-fail). The gate acts; the coordinator reads.

The 6h12m freeze the report documents is a SYMPTOM of Pattern 4 (same-file
tangle). P0-B is the cheap near-term mitigation; P4 root-cause is long-term.

### 4.3 P1-A — reject W4, defer G1 + file-scope lease, correct RC5

- **RC5 correction (load-bearing).** The report's premise — that the harness
  already ships `isolation: "worktree"` as an agent capability — is REFUTED. No
  such field exists. The "make it default" framing rests on a false premise and is
  rejected as stated.
- **W4 worktree-default (REJECT).** The 2026-07-05 memo explicitly evaluated and
  rejected worktrees as scope creep: they conflict with the host-shell
  single-checkout runtime and are a much larger change than the problem demands.
  The report provides new evidence the *problem* is real and costly, but supplies
  no answer to the *rejection rationale* (the runtime conflict). W4 stays rejected
  unless the runtime changes.
- **G1 line-level merge (DEFER, already deferred — cross-reference).** The
  report's evidence strengthens the case for prioritizing G1's validation packet.
  G1 is already recorded as deferred in the 2026-07-05 memo; this disposition
  cross-references it rather than duplicating the ledger.
- **File-scope-lease dispatcher (DEFER, genuinely new).** Serialize a second lane
  onto a file an in-flight lane owns. Lane *scheduling* (do not dispatch lane B
  onto lane A's leased file) is a coordination routing function, not a state
  transition — defensible without crossing the authority line — as long as it
  never blocks commits. Filed as a defer card.
- **Disagreement on priority.** The report ranks this P1 (high). The disposition
  ranks it long-term: it is the most expensive item, its core premise is false
  (RC5), its root-cause solution (W4/G1) is rejected/deferred, and its symptom is
  cheaply mitigated by P0-B. It is correctly acknowledged, incorrectly prioritized.

### 4.4 P1-B — adopt (template checklist), convert the gate version

RC6 confirms no motivation-satisfaction check exists. ADOPT a "Motivation check"
section in design/research closeouts (restate the brief's top goals; demonstrate,
quantitatively where possible, that the design achieves them). MERGE with existing
mechanisms: motivation belongs in the task-contract *non-negotiables* (which
already exist) so it survives compaction; the closeout re-derives it; the
§4.2 premise-recheck 4-tuple carries it across handoffs. The report's "coordinator
acceptance gate" version CROSSES the authority line and is converted to a
template checklist (like the Verification table) — prose discipline, not
enforcement (see §5).

### 4.5 P2-A — defer (blocked, weak evidence)

Pattern 6 is the one pattern whose structural contribution is thin (only "no
within-session recurrence detector exists") and whose phenomenon is almost
entirely Class A (HIGH collapse risk if the consuming-repo evidence were wrong).
It overlaps F1/§4.3 + `defer-002`, and a detector on an unstable signature is
noise. DEFER, blocked on `defer-002`. The report does not resolve the signature
problem; it assumes it.

### 4.6 P2-B — adopt the field, defer the auto-load, reuse the dormant substrate

RC8 confirms no findings-delta closeout artifact and a DORMANT typed-memory
substrate. ADOPT a `findings_delta` closeout field (low cost, generic, core).
The interesting long-term piece — subsystem-memory auto-loading by file path —
would make the dormant `internal/memory/store/` substrate its second
(development-cycle-scoped) consumer. DEFER the auto-load behind a file→memory
mapping design. MERGE "design-principles ride the whole handoff chain" into the
already-shipped §4.2 protocol (`0f5b104`) rather than a new channel.

### 4.7 Cross-cut (compaction ≠ completion) — adopt by merging

ADOPT as the `abandoned` verdict state: a lane dispatched with no closeout is
`abandoned`, not done. The coordinator's lane ledger already tracks
dispatched-vs-closed; the verdict token makes it machine-checkable. Compressor
guidance to preserve the verdict token + open crux is a §4.2-style softened
mechanism (the harness does not control OpenCode's compressor directly).

## 5. Authority-line engagement (engaged, not silently violated)

The 2026-07-22 decision memo fixes the line: *coordinator state informs (flags
WARN, emits "do not release"); safety-layer gates act (doctor / commit-gate /
release-validator / tests block).* A coordinator that acquired transition
authority would collapse the model-output-is-candidate invariant. **This
disposition does not challenge that line; it confirms it.** Three report fixes
cross it and are converted:

| Report fix (crosses the line) | Why it crosses | Gate-shaped conversion |
|-------------------------------|----------------|------------------------|
| P2: "coordinator MUST surface the verdict token" | enforces what the lane must emit | a **closeout/commit-gate check** that refuses a closeout artifact lacking a valid `VERDICT:` token — the gate acts, the coordinator reads |
| P3: "coordinator must reject a 'committed' closeout where pre/post HEAD equal" | blocks a committed state | the **commit-gate** aggregates its own history and emits `no_head_progress`; a **doctor** HEAD-staleness check WARNs — gate/doctor act, coordinator reads |
| P5: "coordinator acceptance gate" (design motivation) | gates a design from proceeding | a **design-template checklist** (like the Verification table) + motivation in task-contract non-negotiables — prose discipline, not enforcement |

The fixes that do NOT cross the line (testing-contract clause, runtime/config
worktrees, the recurrence tool, the findings-delta field) are adopted or deferred
on their merits without conversion.

## 6. Deferred work (named triggers)

Filed as cards in `.local/coordinator/tasks/` (transport, not committed canon):

- **`defer-behavioral-parity-matrix-full.json`** — full §4.5 route-not-node /
  OFF-row test infrastructure. *Trigger:* test-path-coverage gap recurs, OR a
  parity-matrix validation packet is prepared.
- **`defer-file-scope-lease-dispatcher.json`** — serialize a second lane onto an
  in-flight lane's declared file. *Trigger:* ≥1 more documented concurrent-same-
  CODE-file tangle, OR G1 line-level merge validated.
- **`defer-within-session-recurrence-detector.json`** — Pattern 6 detector.
  *Blocked on* `defer-002-symptom-signature-stability`. *Trigger:* defer-002
  resolved.
- **`defer-findings-delta-auto-load-by-path.json`** — subsystem-memory auto-loading
  for fix-lane prompts by file path (second consumer of the dormant typed-memory
  store). *Trigger:* a file→subsystem-memory mapping design is proposed.
- **`defer-f4-closure-narrowness-reconcile.json`** — reconcile the F4-closure claim
  in the case study + 2026-07-22 memo to "narrow (output-filtering), not general
  (test-path coverage)." *Trigger:* next docs-reconciliation slice, OR anyone
  treating F4 as fully closed for testing.

G1 line-level merge is **cross-referenced** to the 2026-07-05 memo (already
deferred); no duplicate card.

## 7. Rejected

- **W4 default worktree isolation.** Premise refuted (RC5); rejection rationale
  (host-shell single-checkout runtime) unaddressed by the report.

## 8. Disagreements with the report's framing (stated, with reasons)

1. **RC5 false premise** (Pattern 4) — `isolation: "worktree"` is not a harness
   capability; factual error, corrected.
2. **Pattern 4 priority too high** — it is the most expensive item, its core
   premise is false, its root-cause solution is rejected/deferred, and its symptom
   is cheaply mitigated by P0-B. Long-term, not P1.
3. **Pattern 6 evidence too thin** — the one pattern load-bearing on un-re-derivable
   Class A evidence; defer, blocked on `defer-002`.
4. **Authority-line crossings** (P2/P3/P5 fixes) — three fixes give the
   coordinator transition authority; converted to gate-shaped (§5).
5. **RC2 overstatement** — the report says "no requirement that a lane emit a
   machine-checkable verdict"; a Verification-table convention DOES exist. The gap
   is "no single fixed-position enum token," not "no verification convention."
6. **"not-exercised → defer-blocking" too aggressive** — would block legitimate
   commits; split into defer-not-drop (persist) + verdict-token-blocks-complete
   (enforce).

## Evidence / Provenance

| Claim | Verifying artifact / command | Verified |
|-------|------------------------------|----------|
| RC1–RC8 verdicts | researcher source packet (citation-heavy audit of AGENTS.md, commit-gate.sh, agent schemas, commit-reviewer.md, closeout/handoff templates) | yes |
| Shipped mechanisms 69e0104 / c197171 / 0f5b104 / da09e9e | `git show --stat` + commit subjects | yes |
| F4-closure is `ae5b30d` (CoreOutputs, narrow) | 2026-07-22 memo "out of scope" + `git show --stat ae5b30d` | yes |
| W4/G1 rejected/deferred | 2026-07-05 memo Options W4/G1 | yes |
| Authority line wording | 2026-07-22 memo "Authority and transition routing" | yes |
| Class A consuming-repo evidence | NOT re-derivable in this repo; treated as asserted-by-reporter | n/a (by design) |

House style: bolded-metadata frontmatter + Framing → Verification → Reconciliation
→ Disposition → Authority-line → Deferred → Rejected → Evidence, following the
`2026-07-05` / `2026-07-22` convention (not YAML frontmatter).
