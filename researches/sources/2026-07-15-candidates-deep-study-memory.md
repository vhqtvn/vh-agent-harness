# Deep Study Pass — 5 Memory-Cluster Adoption Candidates (C2, C5, C6, C7, C12)

**Date:** 2026-07-15
**Kind:** Source/decision study packet. NOT active repo guidance — read-only
evidence-grounded resolution of 5 harness-adoption candidates against the ACTUAL
`vh-agent-harness` memory model. **Analysis only: no task card, no backlog row,
no code, no git mutation.**
**Studied against:** `vh-agent-harness` repo at working-tree HEAD.

---

## Research question, scope, policy

- **Research question (per candidate):** Does the repo's EXISTING memory model
  already cover this candidate; and if not, what is the single smallest change
  that captures the value — with file:line evidence, no hand-waving?
- **Scope:** 5 candidates drawn from the synthesis memo's Part B
  (`researches/sources/2026-07-13-agent-harness-papers-synthesis.md`): C2, C5,
  C6, C7, C12. All concern the MEMORY MODEL (typed records + flat session/
  workstream files + injection discipline).
- **Time-sensitivity:** STABLE. This is a repo-grounding pass, not a
  recency-sensitive fact lookup. The two underlying papers are conceptual/
  architectural (paper 1) or empirically-tied-to-specific-models (paper 2); their
  architectural claims are stable, and they were already fully read in the
  synthesis memo. No web source was needed.
- **Recency requirement:** None — no new external source consulted.
- **Source policy:** PRIMARY = the live repo working tree (file:line below) +
  the two prior research memos already in `researches/sources/`. No external
  source was fetched; the papers' mechanisms are taken from the synthesis memo's
  Part A (which read both arXiv full texts). Prior adoption precedent
  (`2026-07-08-tencentdb-agent-memory-study.md`) is honored: **no vector store,
  no runtime-hook injection, flat files canonical, records additive only,
  explicit-invocation injection, human-scale per-session.**

## Repo sources checked

Read in full:
- `internal/memory/record/record.go` (223 lines) — the typed-record DTO + enum.
- `internal/memory/store/store.go` (542 lines) — append-only JSONL reader/writer.
- `templates/docs/opencode-memory-model.md` (188 lines) — 4 layers, Anti-spam,
  typed-records + budgeted-injection discipline.
- `templates/docs/opencode-session-workflow.md` (218 lines) — Memory/Checkpoint/
  Workstream rules incl. the Verification-table mandate.
- `templates/core/.opencode/commands/checkpoint-save.md` (56 lines) — checkpoint
  + decision-log append flow.
- `researches/sources/2026-07-13-agent-harness-papers-synthesis.md` (840 lines)
  — Part A paper mechanics + Part B candidates C1–C12.
- `researches/sources/2026-07-08-tencentdb-agent-memory-study.md` (340 lines)
  — prior adoption precedent + Rejections.

Read partially (targeted slices):
- `templates/core/.opencode/scripts/state-lib.js` (5439 lines) — only
  `sessionDecisionLogPath`/path fns (470–499) and `appendDecision` (2954–2984).
- `templates/core/.opencode/README-session-state.md` (378 lines) — only the
  tool/plugin responsibility block (40–79).

Grepped (not full read):
- `templates/` for `supersed|inference-record|context-snapshot|dependency-link`
  → ZERO matches for the paper-1 object kinds; `superseded` appears only in
  migration-note prose and the prompt-guide's "superseded decisions" phrase.
- `templates/` for `decision-log` → confirmed it is a code-backed session file.
- `internal/memory/` for `last-write-wins|dedup|UpdatedAt.Before` → confirmed
  the dedup contract and its test coverage.

## External sources checked

None. The papers were already fully read in the synthesis memo; this pass
grounds the candidates against the repo, it does not re-read the papers.

---

## Headline finding (read this first)

**The repo's existing memory model already covers 4 of the 5 candidates in
function; only 1 (C5) is a clean docs-only adoption, 2 (C6, C2) reduce to
small docs sharpenings, and 2 (C7, C12) should be closed as already-covered /
reject-new-mechanism.** No new record `type`, no new object kind, no schema
change, and no supersession-link read path is warranted. The single most
important structural fact — decisive for C12 and reinforcing for C2/C7 — is
that the records store is **append-only**, so prior record versions and prior
decision-log entries are **never destroyed**; only the *read API* collapses to
last-write-wins. The "review-loss" problem C12 posits does not manifest at this
repo's human-scale per-session volume, and the data to reconstruct any history
already exists on disk.

---

## Per-candidate resolution

### C2 — Inference-record + context-snapshot + dependency-link checkpoints

- **candidate:** C2 (study-more) — treat each LLM-mediated decision in a
  checkpoint/decision-log as a typed inference-record carrying a bounded
  context-snapshot and dependency links to records it informed.
- **repo evidence:**
  - The checkpoint flow ALREADY mandates an evidence-basis structure:
    `checkpoint-save.md:25-31` requires a **Verification** table mapping each
    claim → the exact verifying command/output (this *is* a context-snapshot of
    the evidence that grounded the decision); `checkpoint-save.md:32-36`
    requires **Structured Findings** with `source=…, confidence=…, type=…`
    (this *is* per-finding provenance); `checkpoint-save.md:37-41` requires
    **Contradiction Flags**.
  - Material decisions are logged separately as a first-class append-only
    session file: `state-lib.js:2954-2984` `appendDecision` writes a
    `## <createdAt> - <title>` entry + free-form body into `decision-log.md`
    (path at `state-lib.js:479-481`); invoked from `checkpoint-save.md:48`
    ("if the conversation contains a material decision, also call …
    `operation: append_decision`"). The file is listed as canonical session
    memory at `opencode-memory-model.md:29`.
  - The record enum already has a home for "remembered events, decisions,
    checkpoints": `TypeEpisodic` (`record.go:42`, semantics `record.go:33-34`;
    `opencode-memory-model.md:174` "remembered events, decisions, checkpoints").
  - Grep for the paper-1 object-kind names (`inference-record`,
    `context-snapshot`, `dependency-link`) across `templates/` → **zero
    matches**: the *vocabulary* is absent even though the *function* is
    substantially present.
- **coverage finding:** **PARTIAL-GAP.** Function (decision + evidence basis +
  provenance + contradiction) is largely covered by the checkpoint Verification
  table + Structured Findings + decision-log. The real gaps are narrow: (a) no
  *explicit forward dependency-link* (decision → downstream record/artifact it
  enabled), and (b) no bounded *context-snapshot object* distinct from the
  checkpoint body — both are folded into free-form prose today.
- **minimal warranted change:** **No schema/type change.** Do NOT add an
  `inference` record type — `episodic` already covers it (would shadow it; see
  flat-file-canonical rule). The single smallest warranted change is **docs-only
  prose convention**: one sentence in
  `.vh-agent-harness/docs/opencode-session-workflow.md` (Checklist/Checkpoint
  rules) and/or the `decision-log` guidance noting that a decision-log entry for
  an LLM-mediated (`infer`) decision SHOULD (a) cross-reference the
  Verification-table row that was its evidence basis and (b) name the downstream
  artifact/checkpoint it enabled. This carries the dependency-link as prose,
  reusing the two structures that already exist — no new object kind.
- **sharpened verdict:** **refine** — downgrade from "study-more (possibly new
  record type)" to "docs-only decision-log entry convention." The candidate's
  own Open Q ("can a reviewer reconstruct the basis without re-reading the whole
  session?") is already answerable YES today via the Verification table; the
  convention edit just makes the forward link explicit.
- **updated trigger predicate:** keep `trigger:path_touched(.vh-agent-harness/
  docs/opencode-session-workflow.md)`; narrow the candidate's scope note to
  "decision-log entry convention, NOT a new record type."

### C5 — Name "behavioral state decay" as a compaction failure mode

- **candidate:** C5 (adopt) — adopt paper 2's term for decision-relevant state
  failing to influence the next decision even when still in context; tie to
  compaction discipline.
- **repo evidence:**
  - The mitigation is already in place: `opencode-memory-model.md:144-148`
    **Anti-spam rule** ("enter context only if it changes the next action or
    prevents a repeated mistake"); `opencode-session-workflow.md:157`
    "Compaction is aggressive. Do not rely on chat history alone."; the load
    policy keeps `brief.md`/`next-slice.md` eligible
    (`opencode-memory-model.md:96-105`) precisely so decision-relevant state
    survives compaction.
  - The named failure mode is **absent**: grep for "behavioral state decay" →
    zero matches. The discipline exists; the *name* does not.
- **coverage finding:** **PARTIAL-GAP.** Discipline = covered. Named failure
  mode = absent.
- **minimal warranted change:** **Docs-only, one line.** In
  `templates/docs/opencode-memory-model.md`, add the term "behavioral state
  decay" next to the Anti-spam rule (or the compaction guidance), stated as the
  failure mode the existing explicit-invocation + brief/next-slice eligibility
  discipline mitigates. Cite paper 2 §1. No code, no schema.
- **sharpened verdict:** **confirm-adopt** (docs-only naming, exactly as the
  synthesis proposed; the Open Q "verify existing discipline already mitigates
  it" resolves to YES per the file:line above).
- **updated trigger predicate:** keep `trigger:path_touched(templates/docs/
  opencode-memory-model.md)`.

### C6 — Two-phase memory discipline: maintenance vs injection; silence first-class

- **candidate:** C6 (adopt) — separate writing memory (Phase 1) from deciding
  whether to surface any of it (Phase 2, null/silence explicit). HARD CONSTRAINT:
  must NOT become always-on injection (C9 is rejected).
- **repo evidence:**
  - The two phases are ALREADY structurally separated as distinct code paths and
    distinct doc sections. Phase 1 (maintenance/write) = `store.go:200-283`
    `appendTo` (O_APPEND, flock+fsync); Phase 2 (retrieval that an injection
    decision would gate) = `store.go:349-477` `readFile`. They share no state;
    writing never implies injecting.
  - Phase 2's *intent* is already expressed: `opencode-memory-model.md:146`
    "enter context only if it changes the next action" (= "inject only if it
    will change the next decision"). The always-on rejection is explicit:
    `opencode-memory-model.md:187-188` "Inject by explicit invocation at
    session-start, handoff, or checkpoint — not always-on." So the HARD
    CONSTRAINT is already honored in prose.
  - The **gap**: "silence/null is an explicit first-class outcome" is *implied*
    by the Anti-spam rule but never *named*. Nothing says "staying silent is the
    default, not a failure."
- **coverage finding:** **ALREADY-COVERED** for the phase separation and the
  not-always-on constraint. **PARTIAL-GAP** only for making "silence" an explicit
  named outcome.
- **minimal warranted change:** **Docs-only, one bullet.** In
  `templates/docs/opencode-memory-model.md` Injection rules
  (`opencode-memory-model.md:177-188`), add an explicit bullet: "Selection may
    return null — silence is the default outcome, not a failure; inject only
    when a recalled item would change the next action (Anti-spam rule above)."
  This sharpens the existing discipline; it does not add a trigger. The HARD
  CONSTRAINT is preserved (the section already says "not always-on").
- **sharpened verdict:** **refine** — the phase split is already covered
  (separate code paths + separate doc sections); refine only the explicit
  "silence is first-class" naming. Do NOT transplant paper 2's fixed-interval
  trigger (that is C9, rejected).
- **updated trigger predicate:** keep `trigger:path_touched(templates/docs/
  opencode-memory-model.md)`.

### C7 — Map paper 2's status/knowledge/procedural split vs the repo's record enum

- **candidate:** C7 (study-more) — check whether the record `type` enum
  (persona|episodic|instruction) + session files already cover paper 2's
  s_t (status) / K_t (knowledge) / P_t (procedural).
- **repo evidence & bucket→file/type mapping:**
  - **s_t (status — progress, open issues, unresolved risks; "private, never
    shown to the action agent"):** maps to **TWO** repo surfaces with
    *different* load policies — `task-contract.md` (mission/status; **always
    eligible**, `opencode-memory-model.md:96-99`, path `state-lib.js:487-489`)
    and `open-questions.md` (unresolved risks; **retrieval-only unless clearly
    needed**, `opencode-memory-model.md:108`). Covered, but see Contradiction #1
    below — paper 2's s_t is *private*; the repo's task-contract half is the
    *most* eligible thing by design.
  - **K_t (knowledge — stable facts: requirements, env properties, paths,
    config):** maps to `TypePersona` + `TypeInstruction` records
    (`record.go:40,42`; semantics `record.go:31-36`) — the *stable* records that
    R3 routes to the cacheable SYSTEM region (`opencode-memory-model.md:179-
    181`). Covered.
  - **P_t (procedural — attempts + outcomes: failed commands, fixes, ruled-out
    hypotheses):** maps to `TypeEpisodic` records (`record.go:42`,
    "event/decision/checkpoint") + `decision-log.md` (material decisions,
    `state-lib.js:2954-2984`). Covered.
  - The enum is locked at three values by design:
    `record.go:12-15` ("the three Type enums … are the locked contract");
    `record.go:46-52` `Valid()` rejects any fourth value; `record.go:199-215`
    `UnmarshalJSON` treats an unknown enum as data corruption.
- **coverage finding:** **ALREADY-COVERED.** All three paper-2 buckets map to
  existing repo surfaces. No gap that a new `status`/`procedural` record type
  would fill better than the existing files — and adding such types would
  **shadow** `task-contract.md`/`decision-log.md`, violating the flat-file-
  canonical + records-additive-only rule (`store.go:10-13`).
- **minimal warranted change:** **None — already covered.** (Optional,
  non-warranted hygiene: a one-line mapping note in `opencode-memory-model.md`
  recording the s_t/K_t/P_t → file/type mapping so a future reader doesn't
  re-propose a type split. Not required.)
- **sharpened verdict:** **downgrade-study-more → reject-new-type** (confirm the
  existing split covers all three buckets; explicitly close: no new record type).
- **updated trigger predicate:** refine to `trigger:path_touched(internal/
  memory/record/record.go)` with note "re-open ONLY if a concrete
  bucket-with-no-home is demonstrated; current mapping covers s_t/K_t/P_t."

### C12 — Supersession-link vs silent overwrite

- **candidate:** C12 (study-more, low priority) — when a typed record is
  updated, preserve the prior version with an explicit supersession link instead
  of last-write-wins.
- **repo evidence (decisive):**
  - The store is **strictly append-only at the write layer**:
    `store.go:200-283` `appendTo` opens with `O_APPEND|O_CREATE|O_WRONLY`
    (`store.go:230`) and issues a single append write (`store.go:262`). There is
    **no rewrite, no in-place update, no delete**. A prior version line is
    therefore **never removed from disk** — it remains in the JSONL tail forever
    (until an out-of-band compaction that does not exist today:
    `store.go:22-24` "There is intentionally NO tmp-file-then-rename rewrite …
    rewrite/compaction is not added").
  - Only the **read API** collapses to last-write-wins: `store.go:445-459`
    keeps the newest `UpdatedAt` per ID (`store.go:457`
    `if !rec.UpdatedAt.Before(prev.UpdatedAt)`); documented at
    `opencode-memory-model.md:163-164` ("Update = append a new line with the
    same id and a newer updated_at; readers take the newest per id
    (last-write-wins)").
  - `record.go:130-159` has **no supersession field**; grep confirms no
    `supersession` anywhere in `internal/memory`.
  - The decision-log is likewise history-preserving: `appendDecision`
    (`state-lib.js:2954-2984`) reads current content and rewrites the file with
    the new entry concatenated — prior entries survive inside the file.
- **coverage finding:** **PARTIAL-GAP (but immaterial at human-scale).** Prior
  versions ARE preserved at the storage layer (append-only JSONL); they are
  merely not *surfaced* by the read API, and there is no supersession *link
  field*. A reviewer can already reconstruct any record's history by grepping
  the JSONL file for its ID (the lines are physically present). The
  "review-loss" problem the candidate posits does not manifest at this repo's
  human-scale per-session volume, and the data needed to build a history read
  path already exists on disk.
- **minimal warranted change:** **None — already covered at the storage layer.**
  A supersession-link read path is not warranted: it would add read-path +
  schema complexity for no demonstrated loss. IF one were ever needed, it is an
  *alternative reader* (return all lines for an ID ordered by `UpdatedAt`), not
  a schema change — and the on-disk data already supports it. No edit to
  `record.go` or `store.go` is warranted now.
- **sharpened verdict:** **downgrade-study-more → reject (for now).** The
  append-only store already preserves history; the last-write-wins collapse is
  the intended read contract; review-loss is unobserved at human-scale. Re-open
  only if (a) per-session record count grows enough that tail-grepping becomes
  painful AND (b) an actual review-loss incident is observed.
- **updated trigger predicate:** refine to `trigger:area_touched(memory-model)
  AND observed_review_loss` — gate on a demonstrated incident, not on theory.

---

## Consolidated resolution table

| C-id | sharpened-verdict | one-line evidence | minimal change (or none) |
|------|-------------------|-------------------|--------------------------|
| **C2** | **refine** (docs-only) | Checkpoint Verification table + Structured Findings + decision-log already capture decision+basis+provenance (`checkpoint-save.md:25-41`, `state-lib.js:2954-2984`); `episodic` type already covers inference-records (`record.go:42`). | One-sentence decision-log convention: cross-ref evidence row + name downstream artifact. NO new record type. |
| **C5** | **confirm-adopt** (docs-only) | Anti-spam + "compaction is aggressive" already mitigate it (`opencode-memory-model.md:144-148`, `session-workflow.md:157`); the *name* is absent. | One line in `opencode-memory-model.md` naming "behavioral state decay" + citing the existing mitigation. |
| **C6** | **refine** (docs-only) | Phase split already separate code paths (`store.go:200-283` write vs `349-477` read) + "not always-on" explicit (`opencode-memory-model.md:187-188`); silence not named. | One bullet in Injection rules: "silence/null is the default, not a failure." HARD CONSTRAINT preserved. |
| **C7** | **downgrade-study-more → reject-new-type** | s_t→task-contract+open-questions, K_t→persona+instruction, P_t→episodic+decision-log all map to existing surfaces; enum locked at 3 (`record.go:12-15,46-52`). | None — already covered. (Optional: one-line mapping note.) |
| **C12** | **downgrade-study-more → reject (for now)** | Store is append-only — prior versions never deleted from disk (`store.go:200-283,230`); only read API collapses (`store.go:445-459`); review-loss unobserved at human-scale. | None — history already preserved on disk. Re-open only on demonstrated review-loss. |

---

## Findings

- **(finding)**: The records store is strictly append-only at the write layer; prior record versions are never removed from disk, only collapsed by the read API's last-write-wins dedup — source=`store.go:200-283,230,445-459` + `opencode-memory-model.md:163-164`, confidence=HIGH, type=fact
- **(finding)**: The checkpoint flow already mandates an evidence-basis structure (Verification table: claim→verifying command) and per-finding provenance (Structured Findings: source/confidence/type), so the *function* of paper 1's inference-record/context-snapshot is substantially present without the vocabulary — source=`checkpoint-save.md:25-41`, confidence=HIGH, type=fact
- **(finding)**: `decision-log.md` is a real code-backed append session file (`appendDecision`, free-form timestamped entries), not just a doc convention — source=`state-lib.js:2954-2984,479-481` + `checkpoint-save.md:48`, confidence=HIGH, type=fact
- **(finding)**: Paper 2's s_t/K_t/P_t buckets all map to existing repo surfaces (task-contract+open-questions / persona+instruction / episodic+decision-log); no new record type is needed and adding one would shadow the canonical flat files — source=`record.go:12-15,40-42` + `opencode-memory-model.md:29,96-108`, confidence=HIGH, type=fact
- **(finding)**: The two-phase maintenance-vs-injection separation is already structural (distinct write vs read code paths + distinct doc sections) and the not-always-on constraint is explicit; only the "silence is first-class" naming is missing — source=`store.go:200-283,349-477` + `opencode-memory-model.md:146,187-188`, confidence=HIGH, type=fact
- **(finding)**: None of paper 1's object-kind vocabulary (inference-record, context-snapshot, dependency-link, supersession-link) appears anywhere in `templates/` — the words are absent even where the function is covered — source=grep over `templates/`, confidence=HIGH, type=fact

## Contradictions

<!-- Explicit contradiction audit beyond those already flagged in the synthesis memo. -->

**NEW (beyond the synthesis memo):**

- **Paper 2's "s_t is private, never shown to the action agent" (Paper 2 §3.2)
  vs this repo's task-contract being ALWAYS eligible.** Paper 2 deliberately
  hides status from the action agent; this repo's closest status analog,
  `task-contract.md`, is the MOST eligible thing in the model
  (`opencode-memory-model.md:96-99` "Always eligible: … current session task
  contract"). This is a **direct design-intent conflict**, not a remapping: the
  repo's whole compaction-survival strategy depends on the task contract being
  always-present, so paper 2's s_t privacy is **rejected**, not transplanted.
  Paper 2's s_t bundles "progress + open issues + unresolved risks," which the
  repo splits across TWO files with **opposite** load policies —
  `task-contract.md` (always-on) and `open-questions.md` (retrieval-only,
  `opencode-memory-model.md:108`). Only the open-questions half approximates
  paper 2's privacy. **This strengthens the C7 "no new type" verdict**: the repo
  deliberately does not want a single private status field, and forcing paper 2's
  private-s_t shape onto the task contract would break compaction survival.

**Already flagged in the synthesis memo (confirmed, not re-litigated):**
paper 2 fixed-interval injection vs Anti-spam (C9 reject); paper 2 "plug-and-play
with existing harnesses" over-claim vs OpenCode's lack of a mid-turn injection
hook; paper 1 shared-substrate vs flat-file/ownership/no-vector-store (C4
reject); paper 1 derive/infer = strong corroboration of the repo's
model-output-is-candidate invariant (C1/C11 adopt). No additional contradiction
found within or between the papers themselves beyond what the synthesis recorded.

---

## Confidence

- **HIGH** on all repo-grounding claims (file:line cited above, read directly).
- **HIGH** on the architectural reading of both papers (inherited from the
  synthesis memo, which read both arXiv full texts).
- The verdicts are evidence-bound; the only *inference* is C12's "review-loss
  unobserved at human-scale" — that is a judgment call grounded in the
  append-only storage fact, not a measured incident.

## Recommended artifact type & path

- **Type:** `decision` (option resolution / verdict memo), since this pass
  sharpens verdicts for downstream coordinator curation rather than gathering
  new sources.
- **Intended durable path:** `researches/decisions/2026-07-15-memory-cluster-candidates-resolution.md`
  (coordinator to relocate from this `tmp/` path). The two underlying source
  packets already live in `researches/sources/`.

## Promotion targets (flagged, NOT executed)

If the coordinator promotes any verdict into live docs, the target files are:
- **C5, C6** → `templates/docs/opencode-memory-model.md` (Anti-spam rule +
  Injection rules sections). These are the only two candidates with a concrete
  one-line/one-bullet docs edit.
- **C2** → `.vh-agent-harness/docs/opencode-session-workflow.md` (decision-log
  entry convention) — docs-only prose, no schema.
- **C7, C12** → no promotion; close as already-covered / reject.

No `AGENTS.md`, no `docs/planning/backlog.md`, no code, no record enum change is
warranted by this cluster.

## Recommended next specialist / command

- Hand back to the **coordinator** (`/coordination` or `/task-review`) for
  curation: the 5 verdicts are commit-ready as a decision memo. The two docs-
  only edits (C5 one-line, C6 one-bullet) are small enough to be a single
  follow-up slice via `/write-task` → `build` if the operator wants them landed;
  C2's convention edit can ride in the same slice. C7/C12 need no slice — they
  should be recorded as closed so a future pass does not re-litigate.

---

## Closeout (verification)

This is a read-only study pass; the only side effect is this memo file under
`tmp/`. No claim below requires a runtime command to verify — each is grounded in
a static file:line citation above.

## Verification
| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Store is append-only (prior versions never deleted) | `store.go:230` O_APPEND; `store.go:22-24` "no rewrite/compaction" | yes |
| Read API collapses to last-write-wins | `store.go:457` `if !rec.UpdatedAt.Before(prev.UpdatedAt)` | yes |
| Checkpoint mandates Verification table + Findings | `checkpoint-save.md:25-41` | yes |
| decision-log.md is code-backed, append session file | `state-lib.js:479-481,2954-2984` | yes |
| Record enum locked at 3 values; 4th rejected | `record.go:12-15,46-52,199-215` | yes |
| No paper-1 object-kind vocabulary in templates | grep `supersed\|inference-record\|context-snapshot\|dependency-link` → 0 matches | yes |
| Anti-spam + "not always-on" already in docs | `opencode-memory-model.md:144-148,187-188` | yes |
