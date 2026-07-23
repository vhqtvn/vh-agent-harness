<!--
  PROVENANCE: external consuming-repo (vh-solara) field report, addressed to
  vh-agent-harness maintainers. Preserved verbatim as evidence on 2026-07-23.
  CLAIM CLASSES:
    - Class A (consuming-repo): vh-solara transcripts, commits, DB rows, code
      anchors (e.g. pkg/fixtures/opencode.go:1214). NOT re-derivable in this repo.
      Treated as asserted-by-reporter, unverified-by-us.
    - Class B (this repo's contracts/tooling): verified against actual files; see
      researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md
      for the RC1-RC8 verdict table with citations.
  KNOWN FACTUAL ERROR IN THE REPORT: the claim that the harness ships an
  `isolation: "worktree"` agent capability (Pattern 4) is REFUTED — no such field
  exists in any agent/command schema (verified). See disposition memo.
  This file is the raw evidence; the disposition memo is the analysis.
-->

---

# vh-solara harness-adoption study: why the orchestration layer keeps losing the "why"

**Audience:** vh-agent-harness maintainers.
**Scope:** the multi-agent *orchestration* layer (coordination, subagent handoff,
commit gate, memory) — not vh-solara product code.
**Method:** read the harness contract (`AGENTS.md`, `.vh-agent-harness/`,
`.opencode/scripts/commit-gate.sh`), the six test lanes, and the **actual
transcripts** (OpenCode SQLite `message`/`part`) of two recent coordinators and
their subagent lanes:

- **Performance Optimization** — `ses_09593d48dffe22OgQznItWbrWN` (2026-07-16),
  ~60 child lanes (solution-brief → planner → build → ship-review → committer).
- **Server-owned session tree design** — `ses_0733c95c4ffeiYE91DLVRAJgeT`
  (2026-07-23), 11 child lanes, plus the sibling **Untangle working tree into 3
  commits** recovery session `ses_071a82d87fferj7Krisj6xbQZK`.

Every claim below cites a transcript, a commit, or a DB row. Metadata was never
trusted on its own — inferring status from titles/git is one of the failures
under study (and one the operator has *already* had to write into memory as
`always-read-session-texts.md`, origin `faf43636…` — this very study line).

---

## Executive summary

The harness is excellent at **producing motion** (fan-out, gated commits, tiered
review, green tests) and poor at **preserving intent and proving reality**. Seven
observed failures all reduce to four structural gaps in the orchestration layer:

1. **No live-verify forcing function.** "Done" is satisfiable by green
   unit/e2e + a passing diff review. The one lane that could have proven the
   load-bearing path live never finished, and nothing blocked the "Phase 2
   complete" commit anyway. (Patterns 1, 2)

2. **Reporting reads the coordinator's summary/metadata, not the lane's buried
   conclusion.** A lane whose own verdict is "INCONCLUSIVE / crux SKIPPED /
   NOT DEMONSTRABLE" is trivially summarized upward as "ran, clean, no bug."
   (Pattern 2)

3. **No isolation and no failure-surfacing at the commit boundary.** Concurrent
   lanes edit the same working tree / same file; the private-index gate handles
   concurrent-*dirty* (different files) but not concurrent-*same-file*, so the
   tree becomes uncommittable and HEAD silently freezes on a docs-only commit
   for ~6 h until a human notices and launches a dedicated untangle. (Patterns
   3, 4)

4. **Handoffs and compaction carry the diff, not the design's motivation or the
   prior findings.** The Phase-1 design "missed the entire motivation of the
   rewrite" and had to be revised post-commit; the same root cause
   (client/server dual-ownership) was band-aided ~20 times and the same bugs
   (archive resurrection, orphan false-positive) were re-diagnosed repeatedly
   because neither the "why" nor the findings are durable across boundaries.
   (Patterns 5, 6, 7)

The harness optimizes each lane locally and never re-asserts the global "why" or
the global "is it actually working." Fixes below are concrete and mostly
contract/gate/handoff-shaped.

---

## Pattern 1 — GREEN-TESTS / BROKEN-PRODUCT

### Evidence
- The in-process e2e (`tests/e2e/`) is green but **structurally cannot** exercise
  the load-bearing path. The fake OpenCode always emits the delete event:
  `pkg/fixtures/opencode.go:1214-1217` — *"Emit a session.deleted event so the
  aggregator's event subscriber drops it"* → `f.emit("session.deleted", …)`.
- That is exactly the path the fix exists to cover. `pkg/aggregator/tree_reconcile.go:27-28`:
  *"Missed session.deleted → the store holds a ghost node; this tick emits
  node.remove to evict it deterministically."* The reconcile tick's whole reason
  to exist is the **missed** delete — which the fake never produces. So the
  e2e proves the *event-driven* delete and leaves the *missed-delete* crux
  unproven.
- The lane that was supposed to prove it live/adversarially — **"Prove Phase 2
  in docker-gold lane"** `ses_070f57e44ffeaAwXnITi4Q09fN` (the docker-gold lane
  is the only place that can produce a genuine missed-event delete via raw
  SQLite row-delete) — **never completed**. Its last transcript activity is a
  mid-recon thought: *"Now I need the exact JSON shapes for the tree
  assertions. Let me read the tree emitter/children/node source"* (2026-07-23
  13:07), after a `-84.3K` DCP compaction. It never wrote an assertion, never
  ran `run.sh`, never proved the crux.
- Despite that, Phase 2 was committed as done: `e5d09d4` *"complete Phase 2
  server — reconcile loop, op-replay, orphan hooks"* (2026-07-23 18:25).
- Even the diff reviewer saw the hole and let it pass: the Phase-2 committer
  (`ses_071521f22`) records reviewer finding **F9 (advisory): "reconcile-tick
  ghost-eviction self-heal is unit-tested but not exercised end-to-end (e2e uses
  the event-driven delete path)"** — disposition **`drop`**, not even deferred to
  a ledger.
- A separate, concrete "green means nothing" trap the untangle coordinator
  caught: the specified gate `go test … -run 'Tree'` matched **2 of 21** Phase-2
  tests and *"prints ok instantly (0.003s) — a false-green trap"* (its words).

### Harness-layer root cause
There is **no live-verify forcing function**. The AGENTS.md testing contract
says "add or update appropriate verification" and defers placement to the repo's
"verified testing seam localization" (the six lanes) — but it treats a green lane
as sufficient proof of behavior. Nothing in the contract distinguishes *"the test
exercises the load-bearing path"* from *"a test in the right directory passed."*
The fake-fixture blind spot is invisible to a green run, and the one lane meant to
close it can be abandoned by compaction with zero consequence, because "Phase 2
complete" is gated on diff-review + unit/e2e, not on observed behavior.

### Fix
- **Add a "load-bearing path" clause to the task contract and the commit gate.**
  For any change whose value is a runtime behavior, the lane must name the
  *specific* path the change fixes and the *specific* verification that
  exercises it — and must assert whether the sanctioned lane actually reaches
  it. If the only green lane uses a fake that short-circuits that path, the lane
  must self-declare `crux_unproven: true`.
- **A `crux_unproven` / `not-live-verified` flag must block a "complete"
  claim**, not sit as advisory. The reviewer's F9 should have been a gate stop,
  not a dropped advisory — diff-review disposition of a "not exercised e2e"
  finding should default to *defer-blocking*, never *drop*.
- **Compaction of a verification lane must not read as completion.** A lane that
  was compacted mid-work and never emitted a closeout must surface as
  `abandoned`, not silently disappear from the coordinator's ledger (see
  Pattern 2 fix).

---

## Pattern 2 — REPORTING FROM METADATA, NOT GROUND TRUTH

### Evidence
- The **"Live smoke test tree=2 daemon"** lane `ses_0712fa42dffeE5GsHPb2Fxs785`.
  Its *own* transcript verdict is unambiguous:
  - §7 Verdict: **"Live smoke INCONCLUSIVE on the data-dependent behaviors — NOT
    a clean Phase-3 clear, and NOT a FAIL (no defect found)."**
  - Behavior 1 (cold lazy frontier): **"NOT DEMONSTRABLE LIVE (no data)"** —
    `nodes:null`, **0 live sessions** (53 all-archived).
  - Behavior 2 (expand round-trip): **"NOT DEMONSTRABLE LIVE (no data)"**.
  - Behavior 3, THE CRUX (missed-delete reconcile): **"SKIPPED per operator
    decision"**, and the lane explicitly flags that **e2e doesn't cover it
    either** — *"the live missed-delete → ~5s tick → node.remove, no-resurrection
    integration against real opencode remains unproven by both live and e2e."*
  - Behavior 4 ring-gap: **"NOT INDUCIBLE"**.
- Yet the operator's memory note records that this lane's **metadata read as
  "ran, clean tree, no bug"** and that *"inference would have wrongly reported
  Phase 2 as live-verified"* (`always-read-session-texts.md`). The honest
  conclusion is buried in §5–§7 of a long closeout; the rosy read is what
  surfaces from the summary line + clean `git status`.

### Harness-layer root cause
The reporting path privileges the **coordinator's summary and session metadata**
over the lane's **buried caveats**. A closeout that ends "git tree clean, no
source edited, no commits" plus a neutral title ("Live smoke test…") flattens to
green upward. The harness has no requirement that a lane emit a **single
machine-checkable verdict token** at a fixed position, and no rule that the
coordinator must quote the lane's *weakest* claim, not its cleanest.

### Fix
- **Mandate a structured verdict header as the FIRST line of every verification
  closeout**, e.g. `VERDICT: inconclusive` with an enum
  (`proven | inconclusive | failed | abandoned`) and a `crux: proven|skipped|
  not-demonstrable` field. Put it in the task-contract "Final Response Format" so
  it survives compaction.
- **Coordinator handoff-ingest rule:** when summarizing a lane upward, the
  coordinator MUST surface the verdict token and the *lowest-confidence* claim
  verbatim; a "clean git status" may never be reported as behavioral proof.
- **Ban "no defect found" as a synonym for "verified."** "Did not run the check"
  and "ran the check and it passed" must be distinct, non-collapsible states.

---

## Pattern 3 — SILENT COMMIT-GATE FAILURES

### Evidence
- The operator's own untangle prompt (`ses_071a82d87`, 2026-07-23 16:41):
  **"Three separate uncommitted workstreams are interleaved and NO code has
  committed for hours (HEAD is stuck at 5995161, a docs-only commit)."**
- Git confirms the freeze: `5995161` *docs(design): revise Phase 1* at **10:32**
  → next code commit `dc14846` at **16:44**. **~6 h 12 m** of HEAD frozen on a
  docs-only commit while three build lanes produced code.
- Committers ran in that window and did **not** move HEAD. The Phase-2 build lane
  `ses_07185669a` finished its work at 11:10; its committer
  `ses_071521f22` was active 11:13–11:25 — yet the code did not land until after
  the *human-initiated* untangle (its successful commit's parent is `936a738`,
  produced by the untangle at 16:56). Nothing between 11:25 and 16:41 surfaced
  "your commits are failing / HEAD is stale."
- `commit-gate.sh` returns per-invocation status (`no_changes`, `cas_conflict`,
  stale-lock break) but has **no cross-lane / HEAD-staleness signal** — there is
  no code path that says "N committer runs, HEAD unchanged for M hours."

### Harness-layer root cause
The gate is **per-invocation and stateless about progress**. A committer that
can't land (because the tree is a tangle, Pattern 4) reports locally and the
coordinator moves on; there is no aggregate observer asserting the invariant
*"work claimed done should advance HEAD."* The failure is real but never
escalated, so it takes a human eyeballing `git log` to notice hours later.

### Fix
- **A HEAD-progress watchdog in the coordination layer.** When a lane reports a
  successful commit but `git rev-parse HEAD` is unchanged, or when N committer
  lanes close without HEAD advancing, raise a blocking coordinator alert. Cheap:
  the coordinator already tracks committer lanes; have it record pre/post HEAD
  and diff them.
- **Committer closeout must include pre-HEAD and post-HEAD**, and the coordinator
  must reject a "committed" closeout where they are equal.
- **Surface "commit could not land" as a first-class blocker state**, distinct
  from `cas_conflict`, with the reason (tangle / build-fail / test-fail).

---

## Pattern 4 — CONCURRENT-LANE TANGLE

### Evidence
- The tangle that caused Pattern 3: three lanes (Phase-2 `tree=2` build, the
  model-selection-override fix, and the Round-2 archive/-race work) all edited
  **the same working tree**, and **`pkg/web/server.go` was shared** between the
  Phase-2 and Round-2 lanes. The untangle had to fold both concerns into one
  commit: `936a738` *"…(server.go also carries folded archive re-assert)"*, and
  the untangle prompt calls this out: *"server.go carries BOTH the tree=2 wiring
  AND the Round-2 reassert seams … deliberately EXCLUDE server.go [from Commit 2]
  — it is shared with Phase-2 and rides Commit 3."*
- The private-index gate is explicitly designed for concurrent-*dirty*
  (different files): AGENTS.md §"Git operations" — *"A concurrently-dirty working
  tree is normal during concurrent sessions … they are mechanically excluded by
  the private-index gate."* But it stages from the **shared working tree**
  (`GIT_INDEX_FILE` private index over the same checkout), so when two lanes edit
  **the same file**, both lanes' hunks coexist in `server.go` and cannot be
  separated into clean slices — the exact situation that jammed commits for 6 h.

### Harness-layer root cause
**No default worktree isolation and no same-file serialization.** The isolation
story stops at "commit only the files you pass"; it assumes lanes touch disjoint
files. There is no allocator that (a) gives each concurrent build lane its own
`git worktree`, or (b) detects that two live lanes have declared overlapping file
scopes and serializes them. The harness even *has* worktree isolation as an
agent capability (`isolation: "worktree"`) but it is not the default for
concurrent build lanes.

### Fix
- **Default concurrent build lanes to worktree isolation.** Each build lane gets
  its own `git worktree`; commits rebase/merge through the committer. Overlap
  becomes a normal merge, not an uncommittable tangle.
- **If shared-checkout is kept, add a file-scope lease.** The coordinator records
  each active lane's declared file list; a second lane declaring an
  already-leased file (e.g. `server.go`) is **serialized or told to branch**, not
  allowed to co-edit. The declared file lists already exist (they're passed to
  the committer) — enforce them at *edit* time, not just *commit* time.
- **Detect the tangle early:** before dispatching a second lane onto a file an
  in-flight lane owns, warn/serialize instead of discovering it hours later at
  commit.

---

## Pattern 5 — CORE-PRINCIPLE DROPPED IN DESIGN

### Evidence
- The Phase-1 design brief (`ses_0733abffc`) gave the researcher the motivation
  as an explicit ground-truth principle: **"Principle 3 — Lazy by default: the
  initial payload is roots + the active path(s) + every other node as a collapsed
  placeholder (childCount only)."** The whole point (per the same prompt) was to
  kill a *"2-day bug cascade"* from *"client/server DUAL-ownership."*
- The researcher nonetheless produced a v1 design whose cold load was **still
  O(total sessions)** — its own decision D4 shipped a self-contained placeholder
  for *every* node ("no stubs at all"). It optimized for structural correctness
  and dropped the bounded-cold-load goal.
- This was **committed** (`4e06e16` *docs(design): add Phase 1 … (v1)*) and only
  caught **post-commit at review**. The revision lane's prompt
  (`ses_07316bd9d`) is blunt:
  **"REVISION 1 — TRUE LAZY FRONTIER … The v1 §5 shipped a self-contained
  placeholder for EVERY known node, so cold load was still O(total sessions)
  (~1047 nodes for deep-fake-detection; ~1 MB) — it missed the entire motivation
  of the rewrite."** → revision commit `5995161` *"revise Phase 1 — true lazy
  frontier."*
- The coordinator did not catch the drop; a downstream reviewer did, one commit
  later.

### Harness-layer root cause
**Handoffs carry the deliverable spec, not a checkable motivation.** The brief
listed "why this exists" and "core principles" as prose at the top, but there was
no step forcing the researcher (or the coordinator accepting the deliverable) to
**re-derive the design against the stated goal** and answer "does the cold-load
payload stay bounded as idle sessions accumulate?" The prompt guide asks for
"settled assumptions" but not for a **motivation-satisfaction check** in the
closeout. So "correct" beat "correct *and* achieves the point."

### Fix
- **Require a "motivation check" section in every design/research closeout:**
  restate the top-1/2 goals from the brief and show, quantitatively where
  possible, that the design achieves them (here: "cold-load = O(frontier), not
  O(total); at 1047 sessions ships ~N nodes, not 1047"). A design that can't fill
  this section is not done.
- **Coordinator acceptance gate for design deliverables:** before accepting, the
  coordinator must confirm each stated core principle maps to a concrete design
  decision — and reject if a principle has no corresponding, verified mechanism.
- **Put the motivation in the task contracts's non-negotiables**, not just the
  prompt preamble, so it survives compaction and is re-checkable at closeout.

---

## Pattern 6 — BAND-AID LOOPS

### Evidence
- The git log is a wall of symptom-patches around **one** root cause
  (client/server dual-ownership of tree structure) over ~2 days, before anyone
  stepped back to a redesign:
  - `4bc41db` suppress session/stub duplicate row
  - `de782d8` reconcile demoted sessions out of state.sessions
  - `86d929d` self-heal stale busy flag via periodic reconcile
  - `b3b0751` reconnect/replay authority for projected snapshots
  - `9d24514` clear stale facets on materialized sessions
  - `94cb2c2` anti-resurrection guard for lazy-expand
  - `a22d08c` stale-cursor signal + collapse-on-abandon
  - `d2d3529` coalesce Stream1 promotion re-snapshots
  - `1695b4c` periodic + on-focus tree resync to self-heal O1 drift
  - `dfbdaf1` time-based demotion sweep …
  - `388652a` break cold-stub reveal-gate deadlock …
  - `555532f` guard archived sessions from clobber-resurrection …
  - `52a3fd0` tolerate 404/410 in orphan archive …

  (~20 `fix(web|state|sync)` commits in this cluster.)
- The synthesis — *"the current O1 collapsed-frontier projection has client/server
  DUAL-ownership … which caused a 2-day bug cascade: false orphans … tree flatten
  … archived-session resurrection … subagent shown as root … demotion drift"* —
  appears **only** in the redesign brief (`ses_0733abffc`), i.e. it was framed by
  a **human** launching the server-owned-tree rewrite, *after* the 2 days of
  band-aids, not surfaced by the harness mid-loop.

### Harness-layer root cause
**No cross-session root-cause synthesis.** Each fix lane is scoped to one symptom
and closes green. Nothing aggregates "these N fixes touch the same subsystem and
keep reopening — step back and question the design." The harness has memory
primitives and a backlog, but no mechanism that notices a *recurrence signature*
(same files, same failure family, N times) and escalates from "fix" to
"redesign candidate."

### Fix
- **Recurrence detector in the coordination layer.** Track a rolling window of
  fix commits by subsystem/file and failure family (title/label). When a
  threshold trips (e.g. ≥4 fixes to the same subsystem in K days), auto-raise a
  "root-cause review" task before another symptom fix is dispatched.
- **A "is this a symptom of a known cause?" prompt clause** in the build/fix
  lane: require the lane to check the last N fixes in the same area and state
  whether it's patching a shared cause. Cheap, and it front-loads the synthesis.
- **Make "step back to design" a first-class coordinator move**, with a template,
  triggered by the recurrence detector rather than by a human's patience running
  out after 2 days.

---

## Pattern 7 — RE-DISCOVERY / NO DURABLE MEMORY

### Evidence
- The same failure families were fixed **repeatedly**, each time re-diagnosed:
  - **Archive resurrection** — three separate guards: `94cb2c2` *anti-resurrection
    guard*, `555532f` *guard archived sessions from clobber-resurrection via
    tombstone*, `ef5ecac` *resurrection guard*.
  - **Orphan false-positive** — three separate fixes: `b245173` *revalidate
    orphan queue (GC-3 race)*, `e88f19e` *gate orphan bulk-archive on archived
    root*, `52a3fd0` *tolerate 404/410 in orphan archive*.
- None of these root-cause findings made it into durable memory. The memory dir
  (`~/.claude/projects/.../memory/`) holds `firefox-webrender-gpu-heat`,
  `session-load-slow-firehose`, `multi-project-single-tab`, etc. — but **nothing
  about dual-ownership, resurrection, or orphan classification**. The one new
  memory added during this window (`always-read-session-texts.md`) is a *process*
  lesson (read the transcript), not the *design* finding.
- Even where a good durable note exists (`session-load-slow-firehose.md`
  explicitly says *"Do not re-diagnose these"* and lists the fixing commits), it
  is the exception, hand-curated by the operator — not a systematic output of the
  fix lanes.

### Harness-layer root cause
**Findings and design principles are not a required closeout artifact, so they
die with the lane's context.** A build lane fixes resurrection, closes, and its
diagnosis evaporates; the next lane hits an adjacent symptom and re-derives the
same cause from scratch. Memory is opt-in and human-curated; handoffs carry the
diff and the next task, not the accumulated "what we now know is true about this
subsystem."

### Fix
- **Require a durable "findings delta" on closeout** for any non-trivial
  diagnosis: one or two lines of "what we now know is true" + the anchoring
  commit, written to a subsystem memory note (e.g.
  `.opencode/state/.../memory/tree-ownership.md`). Make it part of the commit
  gate for `fix(...)` commits touching a flagged subsystem.
- **Load subsystem memory into the fix-lane prompt automatically** by file path:
  a lane editing `pkg/state/tree_*.go` should receive the tree-ownership memory
  note in its context, so "already known / do not re-diagnose" travels with the
  work.
- **Carry the design's principles across the handoff chain**, not just the
  immediate task — coordinator→researcher→planner→build should each re-emit the
  non-negotiables in their handoff so compaction can't strand them (ties to
  Pattern 5).

---

## Cross-cutting harness recommendations (prioritized)

**P0 — Behavioral "done" gate (kills Patterns 1, 2).**
Add a machine-checkable verdict to every verification/lane closeout
(`VERDICT: proven|inconclusive|failed|abandoned`, plus `crux: proven|skipped|
not-demonstrable`). A "complete"/"done"/"gate-cleared" claim is rejected unless
the load-bearing path is `proven`. Diff-review findings of the form "not
exercised end-to-end" default to *defer-blocking*, never *drop*. The coordinator
must surface the lane's weakest claim, never a clean `git status`, as behavioral
proof.

**P0 — HEAD-progress + commit-failure surfacing (kills Pattern 3).**
Committer closeout records pre/post HEAD; the coordinator rejects "committed" with
unchanged HEAD and raises a blocking alert when committer lanes close without HEAD
advancing. Add distinct blocker states for "could not land: tangle / build-fail /
test-fail."

**P1 — Default worktree isolation for concurrent build lanes (kills Pattern 4).**
Make `isolation: "worktree"` the default when >1 build lane runs concurrently, or
enforce file-scope leases from the already-declared file lists at *edit* time.
Same-file overlap becomes a merge or a serialization, never a 6-hour tangle.

**P1 — Motivation-satisfaction check in design/research closeouts (kills Pattern 5).**
Every design deliverable must restate the brief's top goals and demonstrate the
design achieves them (quantitatively where possible). Coordinator acceptance is
gated on every stated core principle mapping to a concrete, verified mechanism.
Put motivation in the task contract's non-negotiables, not just the prompt.

**P2 — Recurrence detector → root-cause review (kills Pattern 6).**
Track fix commits by subsystem/failure-family; auto-raise a "step back to design"
task at a threshold before dispatching another symptom fix.

**P2 — Findings-delta memory as a closeout requirement (kills Pattern 7).**
Non-trivial diagnoses must write a durable subsystem memory note; fix-lane
prompts auto-load the memory for the files they touch. Design principles ride the
whole handoff chain, not just the first hop.

**Cross-cut — compaction must not read as completion.**
A lane compacted mid-work with no closeout must surface as `abandoned`, never
disappear from the coordinator's ledger (the docker-gold crux lane vanished this
way). Compaction summaries should preserve the verdict token and the open crux,
not just "removed 84K."

---

## Appendix — evidence IDs

### Coordinators
- Performance Optimization — `ses_09593d48dffe22OgQznItWbrWN` (2026-07-16)
- Server-owned session tree design — `ses_0733c95c4ffeiYE91DLVRAJgeT` (28 msgs, the real one)
  - near-duplicate aborted starts (2 msgs each, same title, within 6 min):
    `ses_07341ef20ffejEjyB3h0gUqNif`, `ses_0733efbc6ffex0HZa3M5ynyCYA`
    (symptom of coordinator-start fragility / context loss)
- Untangle working tree into 3 commits — `ses_071a82d87fferj7Krisj6xbQZK` (2026-07-23 16:41)

### Subagent lanes cited
- Phase-1 design (@researcher) — `ses_0733abffcffe7H5qbqRCGcNikS` (Patterns 5, 6 synthesis text)
- Apply Phase-1 design revisions (@build) — `ses_07316bd9dffevYrjO6q3zOnhIb` ("missed the entire motivation")
- Finish Phase 2 server (@build) — `ses_07185669affeNtb7JiHgj5LEo4` (active 10:17–11:10)
- Commit Phase 2-rest slice (@committer) — `ses_071521f22ffeIr1xRInwd7ZHx2` (active 11:13–11:25; F9 dropped)
- Live smoke test tree=2 daemon (@build) — `ses_0712fa42dffeE5GsHPb2Fxs785` (INCONCLUSIVE / crux SKIPPED)
- Prove Phase 2 in docker-gold lane (@build) — `ses_070f57e44ffeaAwXnITi4Q09fN` (abandoned mid-recon, crux never proven)
- Untangle committers — `ses_071a46812…` (Commit 1), `ses_0719fcdab…` (Commit 2), `ses_0719bec3a…` (Commit 3)
- PerfOpt Phase-1 solution-brief (@solution-brief) — `ses_0959240d1ffeJM4vpZrz4fEj17` (contrast: a lane that DID keep the goal)

### Commits
- `e5d09d4` complete Phase 2 server (committed with crux unproven) — 2026-07-23 18:25
- `936a738` Phase 2 emitter + deltas (server.go carries folded archive re-assert) — 16:56
- `ef5ecac` bound archive re-assert + resurrection guard — 16:49
- `dc14846` explicit model pick wins — 16:44
- `5995161` docs: revise Phase 1 — true lazy frontier — 10:32 (HEAD frozen here ~6 h)
- `4e06e16` docs: add Phase 1 v1 (motivation-dropped design) — 09:55
- `52a3fd0` tolerate 404/410 in orphan archive — 03:46
- `555532f` guard archived sessions from clobber-resurrection (tombstone) — 03:15
- resurrection guards: `94cb2c2`, `555532f`, `ef5ecac`
- orphan fixes: `b245173`, `e88f19e`, `52a3fd0`
- ~20-commit dual-ownership band-aid cluster: `4bc41db … 52a3fd0` (2026-07-22 → 07-23)

### Code anchors
- `pkg/fixtures/opencode.go:1214-1217` — fake always emits `session.deleted` (why e2e can't hit the crux)
- `pkg/aggregator/tree_reconcile.go:27-28` and `pkg/state/tree_reconcile.go:22-51` — the missed-delete/ghost path the crux exists to prove
- `.opencode/scripts/commit-gate.sh` — per-invocation status only; no HEAD-staleness / cross-lane progress signal
- AGENTS.md §"Git operations" — private-index gate handles concurrent-*dirty*, not concurrent-*same-file*

### DB / method
- `~/.local/share/opencode/opencode.db` (SQLite; `message`/`part`; role via `message.data.role`; lanes via `session.parent_id`). Transcripts read directly — not inferred from titles/metadata.
