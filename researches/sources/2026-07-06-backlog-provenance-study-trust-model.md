# Sources: Backlog-Item Provenance / "Studied" Trust Model

**Date:** 2026-07-06
**Topic:** Per-item trust signal for `docs/planning/backlog.md` rows — what
"studied" affirms, how staleness is computed, the format that needs no
normalizer rewrite, and whether provenance enables a lighter review fast-path
for backlog-only commits.
**Kind:** Source packet (design-research). NOT active repo guidance — proposes
a model for downstream `debate`, not a landed decision.
**Deepens:** the provenance thread sketched (but never committed) in prior
chat-level curation research; this is its first durable write-up.

## Research question & scope

- **Question:** Can a per-item "studied" trust signal on a markdown backlog
  ledger make picking cheaper/safer, and could it justify a lighter review
  fast-path for backlog-only commits?
- **Scope:** the trust/provenance model only. Commit mechanics, the gate merge
  algorithm, the opencode plugin API, and commit timing/atomicity are SETTLED
  (commits `b56b2bd`+`77dc6f4`, 2026-07-06) and out of scope.
- **Time-sensitivity:** STABLE. Repo mechanics are read from source; external
  precedents (code-review labels, branch protection, W3C PROV, HTTP cache
  validation) are established standards, not fast-moving.
- **Source policy:** repo canon first (HIGH confidence); official external docs
  for precedents (MED confidence on applicability — borrowable mechanics, not
  direct imports).

## Confidence legend

- **HIGH** — verified against repo source (file:line) or a stable primary standard.
- **MED** — single-source claim, behavioral inference, or precedent-applicability
  judgment; not independently re-verified here.
- **LOW** — anecdotal / blog-level; directionally useful only.

---

## Key repo findings (HIGH confidence unless noted)

### F1 — The "studied" concept ALREADY exists operationally (just not mechanized)

A real worked example lives at
`docs/checkpoints/2026-07-01-deferred-backlog-study.md`:
- L3-8: "A read-only study pass assessed each deferred backlog candidate
  against the shipped machinery before any promotion... records the outcome so
  the deferrals are durable knowledge rather than implicit neglect."
- L17-19: "each deferred backlog candidate was studied before any promotion...
  A read-only researcher pass assessed each against the shipped machinery."
- L61-62: "When a revisit trigger fires, re-study the item against the
  then-current machinery before promotion."

That checkpoint records, per item: the **study event**, **studied-at**
(2026-07-01), **studied-against** (commits `15c0887`→`c79622cf`), the
**outcome** (confirmed deferred / obviated / premature), and **studied-by**
(read-only researcher). This is **exactly the operator's intended "studied"
semantic** — re-examined against current code/state and confirmed still
accurate. **But it is a separate durable doc, not a per-row, machine-checkable
Notes field.** The operator's ask = surface that record into a row-level
signal so the picking consumer can check it without reading a checkpoint.

### F2 — CRITICAL: the `studied:` token ALREADY MEANS SOMETHING ELSE (collision)

The existing `studied:` Notes-prefix convention means **"when the DEFER finding
was PRODUCED"**, not "when the row was re-verified against code":
- `.opencode/skills/backlog/SKILL.md:124` — `studied:2026-04-30  # when the
  finding was produced`
- `SKILL.md:130` — "`studied:` is the date the finding was produced, for
  staleness awareness."
- Documented as part of the DEFER holding-card provenance
  (`source:review-defer` / `trigger:...` / `studied:YYYY-MM-DD`), carried into a
  backlog row's Notes on promotion via DoR #6 (`SKILL.md:147`;
  `PROMOTER_RUNBOOK.md:68-69`).

The token `studied:` is referenced in **26 places** across the corpus
(`AGENTS.md`, `templates/core/...`, the four `commit-reviewer-{a,b,c,d}.md`
agent prompts, `BLOCKER_POLICY.md`, `PROMOTER_RUNBOOK.md`, `SKILL.md`, and the
`check-defer-triggers.js` header comment). Reusing `studied:` for the operator's
"re-verified" meaning is a **wide blast-radius semantic collision** that ships
to every consumer via `templates/core/`. **Recommendation: introduce a DISTINCT
token** (see Format). Do not overload `studied:`.

### F3 — The holding area has NEVER been exercised; the 8 rows carry prose, not the prefix

- `.local/coordinator/tasks/` does NOT exist in this dogfood repo. The curation
  model is fully scripted (`check-defer-triggers.js`, 287 lines) and documented
  (`SKILL.md`, `PROMOTER_RUNBOOK.md`) but has zero real cards.
- The 8 DEFER/follow-up rows (`P2-PERMCFG-001/002/003/004`, `P1-DRIFT-003`,
  `P2-LINEAGE-001`, `P2-DOCTOR-001`, `P1-INSTALL-001` in `Now`/`Next`/`Later`)
  carry **prose provenance** ("v0.2.1 follow-up #5", "2026-06-28 surfaced by
  Slice-1 validation", "Surfaced during the F3 fix"), NOT the
  `source:/trigger:/studied:` prefix. They were promoted via the OLD direct-row
  path, pre-curation.
- **Implication:** the `studied:` convention is 100% documented, 0% populated
  in the canonical ledger today. A new trust token has no migration burden and
  no existing data to reinterpret.

### F4 — The normalizer's date-collision hazard (format constraint)

`extractCompletionDate` (`.opencode/scripts/normalize-backlog.js:208-211`) does
`/\b(20\d{2}-\d{2}-\d{2})\b/.exec(notes)` and returns the **FIRST** date match.
That value drives:
- `sortHistoricalRows` (`:278-290`) — done/cancelled newest-first ordering.
- `periodKeyForDate` (`:213-220`) — archive-quarter bucketing (`:313,398,539`).

**Hazard:** any date-shaped provenance token placed in Notes BEFORE the real
completion date is misread as the completion date, corrupting history ordering
+ archive placement. E.g. a `done` row `"... 2026-07-05 DONE"` that gets
`rechecked:2026-07-06` prepended would be re-bucketed to Q3 and re-sorted.
Mitigation is either (a) a placement convention (fragile, unenforced) or
(b) a prefix-aware regex (a normalizer edit). The cleanest escape is
**a non-date token shape** — a git SHA does not match the date regex at all.

### F5 — Normalizer vocab is CLOSED; a schema change is a core ship

- Statuses (`:13-14`): `todo|in_progress|blocked|done|cancelled` — closed.
- Sections (`:10-11`): `Now|Next|Later|Done|Cancelled` — closed.
- Columns (`:16-17`): `ID|Status|Area|Task|Owner|Notes|Links` — fixed 7;
  `parseTaskRow` (`:145-185`) folds any extra cells into Notes
  (`rawCells.slice(5,-1).join(" | ")` at `:162`).
- `validateNoDuplicateIDs` (`:251-276`) rejects dup IDs across main + archives.
- The file is 747 lines and ships to consumers via `templates/core/`. A new
  status, section, or column costs a rewrite + a core ship. A Notes-prefix token
  costs neither.

### F6 — Both "last-modified" signals are cheaply computable via git

- **Row-self last-modified** (was the row edited after study?):
  `git log -S "<row-id>" -- docs/planning/backlog.md` (pickaxe on the row ID)
  → last commit touching that row's byte content. Verified live:
  `git log -S "P2-PERMCFG-004"` → `2026-07-05 797ed77`. Cost: one git call/row.
- **Cited-file drift** (did the code the row points at move past the study?):
  `git log --since=<studied-sha> -- <cited-path>` → non-empty = drifted.
  Cost: one git call per cited path. **Requires the cited path to be
  machine-readable.** Today only DEFER cards encode it (`trigger:path_touched(
  <path>)`); general rows bury it in prose.
- Pickaxe caveat: a study that ADDS a `rechecked:` token itself registers as a
  row touch, so row-last-touched == studied-sha is the FRESH state, and
  row-last-touched > studied-sha = someone edited the row AFTER studying →
  stale. This is the desired semantics, for free.

### F7 — The 2026-07-05 decision memo is STALE on enforcement (flag)

`researches/decisions/2026-07-05-commit-gate-shared-file-coupling.md` chose
"W1 model + C enforcement" (edit-blocking via per-agent `edit` deny on
`docs/planning/backlog.md`). Commits `b56b2bd`+`77dc6f4` (2026-07-06, the very
next day) **unwound** that:
- `b56b2bd` "unwind W1 edit-blocking, ship hybrid split-commit + DEFER curation
  model" — removed `edit-guard.js` (-141), rewrote `permconfig/tables.go`
  (dropped the edit-deny), added `check-defer-triggers.js` (+287) + backlog
  `SKILL.md` (+191).
- `77dc6f4` "eventual-consistency model — gate split-preflight (O1)..." — added
  the commit-gate O1 preflight (+65 to `commit-gate.sh`) that refuses a mixed
  acquire, and de-authorized the `backlog-reminder.js` plugin it had just added.
- Current truth (`docs/coordination/README.md:23,31-59`): "Agents edit it
  **freely**... split-commit is ENFORCED at the commit boundary by the O1
  preflight."

The memo's W1/C-enforcement verdict is **superseded**; its option matrix and
contradiction audit remain useful history. Any downstream doc citing the memo
for "edit-blocking" must be corrected.

---

## The studied / staleness / trust model (synthesis)

### What "studied" affirms (minimal affirmation)

To mark a row studied, an agent affirms ALL of these against the state at a
specific commit `<sha>`:

1. **Still real.** Re-read the cited files/state at `<sha>`; the task's premise
   still holds (the gap/feature is not already closed/obviated).
2. **Scope still valid.** The cited file scope (the `trigger:path_touched(<p>)`
   path, or the paths named in Task) still names the right files.
3. **Trigger still fires** (if the row carries one). For DEFER-origin rows,
   re-running `check-defer-triggers.js`-style logic confirms the predicate is
   still the right condition; for general rows, the "when this becomes real"
   condition is still accurate.
4. **Still not done elsewhere.** No other row / landed commit closed it.

The minimal affirmation is #1 (still real). #2-#4 strengthen it. This matches
the `2026-07-01-deferred-backlog-study.md` checkpoint's actual practice
(re-read against shipped machinery → confirm deferred/obviated/premature with a
revisit trigger). A study event is **idempotent and repeatable**; it produces a
(record, outcome, studied-at-commit, studied-by) tuple.

### Trust states (three, computed from two signals)

- **`fresh`** — `rechecked:<sha>` present AND row-not-edited-since-`<sha>` AND
  cited-paths-not-drifted-since-`<sha>`. Safe to act on without re-study.
- **`stale`** — `rechecked:<sha>` present BUT (row-edited-since OR
  cited-paths-drifted-since). Two sub-flavors with different remediation:
  - **row-stale** (the row itself was edited after study) → the study no longer
    describes THIS row; re-study needed.
  - **code-stale** (cited files drifted after study) → the row is unchanged but
    the world moved; re-study against new HEAD needed.
- **`unstudied`** — no `rechecked:` token. The DEFAULT for every current row.
  R1 re-study stands in full before acting.

The two staleness sources are precisely the two the operator intuited
("studied but then updated from other tasks — is it still studied?"): (a)
row-edited and (b) cited-code-drifted. Both detectable cheaply (F6).

### What this is NOT

- Not a substitute for the R1 picking contract (AGENTS.core.md:325-330;
  SKILL.md:175-182). R1 says "a row is a pointer, not a specification; re-study
  before acting." `rechecked` makes the re-study **skipable when fresh**, not
  abolishes it. The pointer-not-spec philosophy is preserved.
- Not a second ledger. It is a Notes-prefix on the existing canonical row.

---

## Format recommendation (no normalizer rewrite)

### Recommended: Notes-prefix `rechecked:<short-sha>` (+ optional `rechecked-by:<role>`)

```
| P2-PERMCFG-004 | todo | permconfig | ... | build | rechecked:797ed77 rechecked-by:promoter 2026-07-05 Surfaced during... | D-F1 |
```

Why this shape wins on the constraints:

1. **No normalizer change.** Notes already accepts arbitrary content
   (`parseTaskRow` folds extra cells, `:162`); the token rides in Notes. No new
   status/section/column → no 747-line rewrite → no core ship.
2. **No date-collision (F4).** A short SHA `/[0-9a-f]{7,}/` does not match
   `extractCompletionDate`'s `20\d{2}-\d{2}-\d{2}` regex. Zero corruption of
   completion-date sort/archive bucketing. (A date-shaped `rechecked:YYYY-MM-DD`
   would collide unless placed after the completion date — fragile.)
3. **Carries more than a date.** The SHA pins the EXACT commit studied against,
   which is what makes the code-drift staleness check cheap
   (`git log --since=<sha> -- <path>`). A bare date cannot do that.
4. **Distinct verb, no `studied:` collision (F2).** `rechecked` / `restudied` /
   `verified` / `confirmed` are all unclaimed (grep confirmed only benign prose
   fragments). `rechecked` is preferred: it names the action (re-examination)
   and cannot be confused with `studied:` (finding-produced).
5. **Optional `rechecked-by`** carries the trust gradient (junior agent vs
   operator vs promoter) without a schema change. Also non-date → no collision.

### Priced alternatives (only if a schema change is later warranted)

- **New `Studied` column** (8th column): cleanest to read, but rewrites
  `TABLE_HEADER` + `parseTaskRow` + the cell-fold logic + every consumer's
  mental model, and ships through `templates/core/`. Cost: HIGH. Reject unless
  the Notes-prefix proves unworkable in practice.
- **`studied:` reuse (overload):** REJECT. Collides with the finding-produced
  meaning across 26 corpus sites (F2).
- **Separate per-row study file** (mirror the 2026-07-01 checkpoint pattern
  per row): REJECT as the primary mechanism. It already exists conceptually but
  is not machine-checkable from the row; making it per-row multiplies files.
  Keep checkpoints for NARRATIVE study records; use the row token for the
  machine signal.

---

## Review-fast-path safety verdict

### The fast-path is narrower than it first appears

A backlog-only commit changes ONLY markdown rows — it cannot break code, tests,
or builds. So "lighter review" for backlog-only commits is **already
implicitly low-impact**, independent of provenance, because the gate O1
preflight already forces backlog-only (refuses mixed acquire; `README.md:23`,
`PROMOTER_RUNBOOK.md:140-143`). The real question is whether provenance lets us
cut REVIEW DEPTH, not review EXISTENCE.

**Provenance enables skipping the R1 RE-VERIFICATION step (re-studying the row's
accuracy against code) for `fresh` rows. It does NOT enable skipping review of
the EDIT itself** (did the editor corrupt the row, change scope, smuggle a
status flip?). Those are orthogonal:

| Review concern | `fresh` row | `stale`/`unstudied` row |
| --- | --- | --- |
| Is the EDIT to the row sound? (corruption, scope creep, status integrity) | Still review | Still review |
| Is the row's PREMISE still true vs current code? (R1 re-study) | **Skip** — `rechecked:<sha>` + non-stale vouches for it | Full R1 re-study |

So the fast-path = "skip the costly re-study-against-code step for `fresh`
rows." That is a real saving (the 2026-07-01 study pass was a whole researcher
session). It is NOT "skip review entirely."

### Gaming-risk assessment (agent self-marking)

- **Risk:** an agent self-marks `rechecked:<sha>` without studying. A SHA must
  exist in git to be plausible, but its existence proves nothing about whether
  study happened.
- **This is the load-bearing risk.** The format cannot prove the study; only
  the authorization model can.
- Borrowed analogy: GitHub's "dismiss stale approvals" trusts the APPROVER's
  identity/role, not just the approval's existence (see External).

### Authorization model (who may mark `rechecked`) — the mitigation

Borrowed from Gerrit `Code-Review+2` (maintainer-restricted; the Go project
forbids self-approval: "The CL owner cannot approve their own CL. Requiring
multiple people ensures that code cannot be submitted unilaterally from a
single compromised account." — `go.dev/wiki/GerritAccess`, MED):

- **`rechecked` may be set by a reviewer-grade role only:** the **promoter**,
  **operator**, or a read-only **researcher/coordination** session — NOT by the
  `build` agent that will pick the row up and execute it. Separation of duties.
- This matches the EXISTING promoter role (`PROMOTER_RUNBOOK.md`): the curator
  is already the trust boundary for DEFER→backlog promotion. Extending
  `rechecked`-marking to the same role adds no new privileged surface.
- `rechecked-by:` records the role so a consumer can apply a trust gradient
  (operator-marked > promoter-marked > unmarked). The 2026-07-01 checkpoint
  already followed this (study by "a read-only researcher pass").

### Verdict

Provenance enables a **narrow, real fast-path** (skip R1 re-study for `fresh`
rows marked by a trusted role) but **does not** enable skipping edit review, and
is **only safe under the authorization model** (reviewer-grade marker,
separation from the executor). Without the authorization constraint, the
fast-path is gameable and must not be trusted.

---

## Findings (structured)

- **(finding)** The "studied" concept already operates in-repo via
  `docs/checkpoints/2026-07-01-deferred-backlog-study.md` (re-read against
  machinery → confirm outcome + studied-against commit + studied-by), but as a
  separate doc, not a per-row machine signal.
  source=`docs/checkpoints/2026-07-01-deferred-backlog-study.md:3-8,17-19,61-62`,
  confidence=HIGH, type=fact.
- **(finding)** The `studied:` Notes token already means "finding PRODUCED
  date" (`SKILL.md:124,130`), appears in 26 corpus sites, and must NOT be
  overloaded with the "re-verified" meaning.
  source=`.opencode/skills/backlog/SKILL.md:124,130` + corpus grep,
  confidence=HIGH, type=fact.
- **(finding)** `rechecked:<short-sha>` is the cheapest safe format: no
  normalizer change (Notes carries it), no date-collision (SHA ≠ date regex at
  `normalize-backlog.js:209`), pins the studied-against commit for cheap
  code-drift detection.
  source=`normalize-backlog.js:145-185,208-211` + git pickaxe demo,
  confidence=HIGH, type=inference (design).
- **(finding)** Both staleness signals are one git call each: row-self via
  `git log -S <id> -- docs/planning/backlog.md`; code-drift via
  `git log --since=<sha> -- <cited-path>` (the latter needs the path
  machine-readable, which only DEFER `trigger:` lines provide today).
  source=`git log` live demo + `check-defer-triggers.js:154-179`,
  confidence=HIGH, type=fact.
- **(finding)** Provenance enables skipping the R1 re-study for `fresh` rows
  but NOT skipping edit review; the fast-path is only safe if `rechecked` is
  set by a reviewer-grade role (promoter/operator/researcher), never the
  executor `build` agent — Gerrit +2 / Go self-approval-forbiddance precedent.
  source=`go.dev/wiki/GerritAccess` + `gerrit-review.googlesource.com/Documentation/config-labels.html`,
  confidence=MED, type=inference (applicability).
- **(finding)** A backlog-only commit is already low-impact (gate O1 forces
  backlog-only), so the fast-path question is review DEPTH, not review
  existence.
  source=`docs/coordination/README.md:23,38-45` + `PROMOTER_RUNBOOK.md:140-143`,
  confidence=HIGH, type=fact.
- **(finding)** The 2026-07-05 decision memo's "W1 + C edit-blocking" verdict
  is SUPERSEDED by the 2026-07-06 free-edits + O1-preflight + promoter model.
  source=commits `b56b2bd`+`77dc6f4` + `README.md:23,31-59`,
  confidence=HIGH, type=fact (staleness flag).

## Contradictions

- **`studied:` token has two incompatible meanings.** The SKILL/doctrine
  meaning is "finding produced" (`SKILL.md:124,130`); the operator's intended
  meaning is "re-verified against code." Resolution proposed here: do not
  overload — use a distinct `rechecked:` token. Flagged for debate.
- **2026-07-05 decision memo vs landed code.** The memo's enforcement verdict
  (edit-blocking) was unwound 1 day later. The memo is stale on that point;
  its option matrix remains useful history. Flagged for a docs-correction slice
  (out of scope for this packet).
- **R1 "re-study always" vs `fresh` fast-path.** R1 (AGENTS.core.md:325-330) is
  unconditional ("re-study before acting"). A `fresh` fast-path that skips
  re-study is a genuine softening. Resolution: the fast-path is opt-in and
  scoped (skip the re-study-against-code step only; edit review still stands),
  preserving the pointer-not-spec philosophy. Flagged for debate.

---

## External precedents (borrowable mechanics)

### GitHub stale-approval dismissal — THE staleness analogy (MED, primary)

`docs.github.com` (official): when "Dismiss stale pull request approvals" is on,
"approving reviews will be dismissed as stale if the diff changes from [the
approved] state." Two distinct settings map to two staleness policies:
- "Dismiss stale approvals when new commits are pushed" → dismiss on ANY push
  (aggressive; ~ row-self-stale on any edit).
- "Require approval of the most recent reviewable push" → only the latest push
  needs fresh approval, prior approvals persist (cheaper; ~ only-HEAD-matters).
- Merge-base-change nuance (2023-06-06 changelog): approval dismissed when the
  base moves after review → maps DIRECTLY to code-drift staleness ("studied
  against commit X but cited files moved past X").

**Borrowable:** the (approved-diff-snapshot, current-diff) comparison is
exactly (studied-against-commit, current-cited-files). The two-settings choice
(aggressive vs cheap) is the same tradeoff the operator faces for staleness
policy.

### Gerrit Code-Review labels — the authorization/trust-grade analogy (MED, primary)

`gerrit-review.googlesource.com/Documentation/config-labels.html` (official):
`+2` = "I read the code and I'm confident it is correct and appropriate to
submit." Borrowable for who-may-mark-studied:
- `+2` is restricted to maintainers/owners; the **CL owner cannot +2 their own
  CL** (Go project; `go.dev/wiki/GerritAccess`) — anti-self-marking, separation
  of duties.
- Go's separate **`Trust` label** (additive, distinct from Code-Review;
  golang/go#40699) shows a precedent for SEPARATING "reviewed for correctness"
  from "trusted-to-merge" — relevant to the "2 faces of trust" framing
  (provenance-record vs trust-semantics are distinct).
- **Sticky Votes**: a vote persists across patch sets unless changed — relevant
  to "studied then row-edited, does study persist?" (Gerrit: yes until
  explicitly changed, but submit rules can demand re-review on new patch sets).

### W3C PROV-O — the provenance-record shape (MED, primary standard)

`w3.org/TR/prov-o` (official Recommendation). Borrowable record structure:
- `prov:wasGeneratedBy` (Entity ← Activity) + `prov:generatedAtTime` → the
  studied-at bound to a study event.
- `prov:wasAttributedTo` (Entity ← Agent) → studied-by.
- `prov:wasDerivedFrom` / `wasRevisionOf` → row revised → new entity; old
  provenance may not carry (motivates invalidation on edit).
- `prov:actedOnBehalfOf` (Agent delegation) → the junior-agent-vs-operator
  trust gradient.
- **PROV's own limitation (PAV ontology paper, pmc.ncbi.nlm.nih.gov/PMC4177195):
  "PROV-O does not itself provide any distinctions between authors, curators,
  contributors."** PROV gives the RECORD shape, not the TRUST semantics — you
  layer trust on top. This validates that the harness needs its own trust layer
  over any provenance record (matches the "2 faces of trust" framing).

### HTTP cache validation — the staleness-computation primitive (HIGH, established standard)

RFC 9110 §13 (formerly RFC 7232): `ETag` (version-stamp) + `If-Modified-Since`
/ `If-None-Match`. The client caches a representation with its validator; a
later request asks "changed since?" and gets 304 (not modified) or 200 (fresh).
Maps directly to: row carries `rechecked:<sha>` (the ETag); consumer asks
"row or cited-code changed since `<sha>`?" via git. Borrowable: a version-stamp
is the right primitive, and a cheap "modified since?" check is the standard way
to invalidate a cache. (Cited from established RFC knowledge; not a fresh
fetch — stable standard, HIGH confidence.)

### Signed commits / trusted authors (LOW–MED, established)

GPG/SSSH signing proves authorship, not review. Relevant only as: the
`rechecked-by:` role is the trust anchor, analogous to a trusted-author identity.
Not directly borrowable for the study-affirmation semantics.

### Data lineage (OpenLineage / OpenMetadata) — (LOW, not fetched)

Facet-based provenance records with run/producer/version. Heavier than needed
for a markdown ledger; the PROV-O primitives suffice. Not pursued.

---

## Open questions for `debate`

1. **Token verb + shape:** `rechecked:<sha>` (recommended) vs `restudied:` vs
   `verified:` vs `confirmed:`; and SHA vs `<sha>@<date>` vs date-only. Confirm
   cheapest-acceptable. (Recommendation: `rechecked:<short-sha>` +
   optional `rechecked-by:<role>`.)
2. **Authorization model:** promoter-only (matches Gerrit +2 maintainer +
   current promoter role), operator-only, or any reviewer-grade agent? Should
   `build` be explicitly FORBIDDEN from setting `rechecked` (Go self-approval
   rule)?
3. **Enforcement posture:** hard-block picking an `unstudied`/`stale` row,
   warn-and-allow, or informational-only? Given R1 is explicitly advisory
   ("pointer, not specification"), the recommendation is **warn, do not block**
   — but this is the core judgment call for debate.
4. **Auto-invalidation vs human-clear:** should staleness auto-clear
   `rechecked` (GitHub-style auto-dismiss on drift) or require a human to
   re-study and re-mark? Recommendation: auto-detect + surface as `stale`
   state, but leave the token in place (auditable); human re-marks on re-study.
5. **Scope of rows:** does `rechecked` apply to `done`/`cancelled` history
   (probably no — history is immutable) or only active rows? And does it apply
   to non-DEFER rows (the operator's framing implies ALL rows)?
6. **Cited-path machine-readability:** to make code-drift cheap, should general
   rows adopt a `scope:<path>` Notes token (mirroring DEFER's
   `trigger:path_touched(<path>)`)? Without it, code-drift is only detectable
   for DEFER-origin rows.
7. **Promoter cadence:** does the eventual-consistency pass re-study ALL stale
   rows each cycle, or only on-demand when a consumer is about to pick? (Cost
   vs freshness tradeoff.)

## Debate handoff question

> Given that `rechecked:<sha>` needs no normalizer change and both staleness
> signals are one git call each, is the right enforcement posture (a) warn-only
> (preserve R1 advisory spirit, trust the consumer to heed), (b) hard-block
> unstudied/stale rows from picking (force a study event first), or (c) a
> tiered model where promoter/operator-marked `fresh` rows get the R1-skip
> fast-path but agent/unmarked rows require full re-study — and does the
> Gerrit-style "executor cannot self-mark" rule need to be encoded in the
> permconfig `edit` map (hard) or left to convention (soft)?

---

## Recommended durable artifact & next steps

- **Artifact:** this source packet at
  `researches/sources/2026-07-06-backlog-provenance-study-trust-model.md`
  (sources, not decisions — no recommendation landing yet).
- **Next specialist:** `debate` on the model (esp. open questions #2, #3, #4 —
  authorization, enforcement posture, auto-invalidation). Then `planner` for an
  execution brief if a direction is agreed. If the operator wants one read-only
  compare-and-plan pass, `/solution-brief` wraps researcher→debate→planner.
- **Promotion targets (IF a direction is agreed, separate slice):**
  - `templates/core/.opencode/skills/backlog/SKILL.md` — add the
    `rechecked:<sha>` / `rechecked-by:` convention + the fresh/stale/unstudied
    state model + the authorization rule.
  - `templates/core/.vh-agent-harness/AGENTS.core.md` (Backlog → Picking
    contract R1) — make the re-study obligation field-aware ("skip for `fresh`
    rows marked by a reviewer-grade role").
  - `templates/core/docs/coordination/PROMOTER_RUNBOOK.md` — add the re-study +
    re-mark step to the eventual-consistency pass.
  - Possibly `.opencode/scripts/` — a read-only `check-row-freshness.js`
    (promoter-use-only, never blocking; mirrors `check-defer-triggers.js`).
  - Possibly `internal/permconfig/tables.go` — if debate picks the hard
    "executor cannot self-mark" rule.
  - NO change to `normalize-backlog.js` (the whole point of the SHA-format
    recommendation).
- **Stale-guidance correction (separate slice, out of scope here):** the
  2026-07-05 decision memo's enforcement verdict should be marked superseded,
  or a successor memo written.

## Progress summary

- Repo canon: read (`backlog.md`, `AGENTS.core.md`, `normalize-backlog.js`,
  `PROMOTER_RUNBOOK.md`, `README.md`, `SKILL.md`, `check-defer-triggers.js`,
  the 2026-07-01 checkpoint, both prior researches). Complete.
- Landed-model verification via git: commits `b56b2bd`+`77dc6f4` confirmed;
  stale memo flagged. Complete.
- External precedents: GitHub stale-dismissal + Gerrit labels + W3C PROV-O
  grounded from official docs; HTTP cache validation from established RFC.
  Sufficient for MED-confidence applicability.
- Open: the 7 debate questions above. This packet is COMPLETE as a source
  packet; no further research pass is pending.
