# Record Lifecycle (card / DEFER)

> **Term contract.** "Agent harness" is a **HANDLE ONLY**. This doc carries the
> **Coordination** layer (routing/tracking/handoff of work).

This is the canonical lifecycle for **transport cards** — work tracked under
`.local/coordinator/tasks/` (implementation, study, research, DEFER,
p2-followup). It codifies six rules so the holding area stays a **drain**, not
a reservoir:

1. **Drain flow.** Work cards directly from transport. No backlog row is
   created before or after by default.
2. **Promotion is exceptional.** A card gets a `docs/planning/backlog.md` row
   ONLY when it crosses a named boundary (below). Backlog is a status ledger,
   not a record store.
3. **Retire timing is landing-gated.** A `completed` card is deleted only AFTER
   the commit-gate confirms the work landed. Never on review approval alone.
4. **Done cards are deleted.** A `completed` card lingering in transport
   corrupts the count signal; the lifecycle retires it, not parks it.
5. **The landing commit is the durable record.** The reachable commit carrying
   the card id is the record. Checkpoints are reserved for campaign closeouts
   or reopenable decisions. No tombstones, no done-rows.
6. **Intake has a bar.** Drain rules do not stop re-growth. A card is filed only
   after admission (resolve-first / admitted-value), not on every surfaced
   finding.

The holding area is **transport, not truth**. Unpromoted and unretired cards
may be lost — that is intentional, because they are not trusted work yet. Do
not create a parallel committed ledger for them.

## The lifecycle

```
intake (admission bar)
   │   resolve-first / admitted-value BEFORE filing
   ▼
transport work   (.local/coordinator/tasks/, status: draft → ready → working)
   │
   ├── boundary? (blocked / owner-change / must-survive-session)
   │     YES → promote ONE backlog row (exceptional side path)
   │     NO  → keep draining in transport
   ▼
review           (commit-reviewer, separate predicate from landing)
   ▼
landing          (commit-gate commits; commit carries `Task-Card: <card-id>`)
   ▼
retirement       (card deleted AFTER landing confirmed reachable)
```

### Intake (admission bar)

A card is filed only when it passes admission. Before filing, answer:

- **Precise question** — the work can be stated as a sharp question now (fog is
  NOT a ticket; see the fog-vs-ticket test below).
- **Concrete area + file/subsystem scope** — names the repo boundary and the
  files it concerns.
- **Validation approach** — how "done" will be checked.
- **An ADMITTED BLOCKER or reason the work cannot be resolved now** — for
  deferred work, the reason it cannot be done in the current session.
- **A ground-truth-derivable trigger** — when deferral is trigger-dependent, a
  predicate the checker can re-derive from repo state
  (`path_touched(<path>)`, `after_tag(<tag>)`, …).
- **Provenance** — where the candidate came from (`source:review-defer`,
  `source:p2-followup`, …).
- **Dedup** — no materially-equivalent open card already exists.

Disposition at intake (apply the `resolve-first` classifier):

- **Resolvable now** → resolve it; do NOT file.
- **Decision-derivable now** → drive to a verdict; do NOT defer.
- **Real deferred work meeting the bar** → file as a `draft` transport card.
- **Duplicate / fog** → collapse into the existing card, or discard.

### Fog vs ticket (triage test)

A finding is **ticket-ready** when you can state the question precisely now —
even if blocked. A finding is **fog** when you cannot yet phrase it that
sharply: in-scope, but not yet specifiable. Fog belongs in transport only while
it sharpens; it is promoted or retired, never left to accumulate as noise.

## Promotion (exceptional side path)

The default is to drain resolved transport directly. A card is promoted into a
`docs/planning/backlog.md` row ONLY when it crosses one of these boundaries:

- **Blocked** — the card cannot progress without an external decision or owner
  input, so its blocked state must outlive the session that discovered it.
- **Owner-change** — responsibility moves to another session/operator, so the
  row is the handoff artifact.
- **Must-survive-session** — the work is longer than one session and the
  durable status ledger (not the transport card) is the right home for its
  state.

A promoted row carries the card's stable ID in its Notes so the join is
auditable. Promotion does NOT retire the transport card — the card is retired
on landing (below), not on promotion. Conflict discipline for promoted rows
(re-read + re-apply + retry on `could_not_land`, never blind-revert) lives in
the `backlog` skill and the `PROMOTER_RUNBOOK`.

Backlog is a **status ledger**, not a record store: a promoted row tracks live
status, not a durable history of completed work. The durable record is the
landing commit.

## Review approval and landing are DISTINCT predicates

- **Review approval** — `commit-reviewer` returned `approve` for the slice.
  This means the change is acceptable to commit. It does NOT mean the change
  landed.
- **Landing** — the commit-gate committed the slice and the resulting commit is
  reachable from a branch (any local branch tip — see the landing-proof
  contract below).

A card is retired as `done` only on **landing**, never on review approval
alone. The 2026-08-07 lesson: a card was deleted on review approval alone and
the commit later failed to land, so the work was lost from the record while the
gate reported failure. Review approval is necessary but not sufficient.

## The landing-proof contract

A `completed` card may be retired as done ONLY when a commit carrying the exact
`Task-Card: <card-id>` trailer line is reachable from a branch. The verifier is
a **reachability** check (per the closure-verifier reachability rule — never
object existence):

```
git log --branches --fixed-strings --grep=Task-Card: <card-id>
```

The `--grep` is a substring pre-filter; the verifier then post-filters each
candidate commit's body to a line EXACTLY equal to `Task-Card: <card-id>` (a
`Task-Card: alpha-next` line does NOT satisfy card `alpha`). ≥1 exact, reachable
match means the work landed and the card may be retired. 0 matches means the
work has NOT landed; the card MUST NOT be retired (it stays `completed`, and if
the commit failed it is re-driven).

- **Branch scope.** `--branches` walks ALL local branch tips (`refs/heads/*`)
  — branch-GENERIC with no hardcoded `main` (mirrors doctor #24's
  `git rev-list --branches`); a match proves BOTH landing AND reachability in
  one query. The choice is deliberately broader than a single integration
  branch: it avoids bricking retirement on a stale/wrong gate-reported branch
  name, and a card is local transport (gitignored), so a premature retire is a
  local self-footgun, not a system-integrity breach. The singular `<branch>`
  notation used in earlier drafts is realized branch-generically by `--branches`.
- **Exact trailer line.** A commit body carries one `Task-Card: <card-id>`
  line per card the commit satisfies. The verifier matches the line exactly
  (trimmed equality), not as a substring — prefix collisions (`alpha` vs
  `alpha-next`) do not authorize the shorter id.
- **Committer is pass-through.** The committer agent does not rewrite the
  commit message, so the trailer MUST originate in the `commit-message` draft
  — it cannot be gate-appended today.
- **Future hardening (NOT v1).** A gate-appended trailer and a gate-ledger
  card-id→commit join are documented future-hardening, not the v1 contract.

## Retention preservation (decision records)

Landing proof says the work LANDED; it says nothing about whether
**decision-class knowledge** — owner authorizations, materially-rejected
alternatives, non-approve review verdicts, measured outcomes — SURVIVES
retirement. Retiring a completed card deletes the card AND its report
directory, the last local copy of that knowledge. The retention-preservation
gate (owner-approved 2026-10-25; coverage rule + review classification
owner-approved 2026-10-26) makes ordinary retirement conditional on that
knowledge being verifiably preserved in committed git state.

### The decision_record

Every closeout saved under the current gate declares ONE fenced
```decision-record` JSON block (see CLOSEOUT_TEMPLATE.md) whose entries are
exactly three kinds:

| Kind | Means | Verified at retirement |
|------|-------|------------------------|
| `commit` with `revision` DECLARED | decision content lives in that specific git commit | the declared revision resolves to a commit that is reachable from a local branch, and carries the marker (declared, default = card id) in its MESSAGE **or** a blob at that committed revision; a covering entry's card-id ANCHOR and every excerpt must be present in that same commit |
| `commit` with `revision` OMITTED | the content is expected in a reachable commit MESSAGE — the landing-commit body pattern, which cannot be sha-pinned at closeout time | at least one branch-reachable commit MESSAGE contains the marker (fixed-string search); a covering entry needs one message containing the card-id anchor AND every excerpt |
| `artifact` | content promoted to a designated durable file (`docs/checkpoints/`, `researches/decisions/`, `researches/sources/`, `docs/planning/`) | the path is git-tracked, the revision is reachable, and the card-id back-reference is present in the COMMITTED blob; a covering entry's excerpts must be present in the same blob |
| `routine` | attestation that nothing decision-class needs preserving | no git check; valid only under the coverage rule when every decision-class field is empty |

There is deliberately **no `inline` kind**: content written only in the local
closeout report is not durable and never satisfies the gate. Working-tree
content never counts — only committed blobs at reachable revisions.

`revision` is optional on `commit`/`artifact` entries. Omitted means
*search mode*: the verifier searches reachable commit messages (`commit`) or
reachable revisions of the path (`artifact`, capped walk) for the marker.
Search mode exists because the landing/durable commits usually do not exist at
closeout time; a declared `revision` is verified precisely.

### The coverage rule (content, never presence)

A **marker** — including the card-id default that the landing trailer itself
satisfies — proves only that an arbitrary string is present somewhere. It
covers NOTHING. The presence of explicit decision-class metadata does not
imply its content survives retirement. Every non-empty decision-class field
must be **covered** by an entry declaring `carries`:

```json
{ "kind": "commit",
  "carries": {
    "owner_notes": "OWNER 2026-10-26: approved the bounded correction over a full rewrite"
  } }
```

- The **field set** is mechanical: `owner_notes`, `measured_outcome`,
  non-empty `open_questions`, and the decision-class review signals below.
- The **excerpt** must be a verbatim single line drawn from the covered
  field's own content (whitespace-normalized binding), at least 24 characters
  trimmed, and must not equal the card id or the entry marker (a
  marker-shaped "excerpt" is the trivial-satisfaction path this rule kills).
- At retirement the excerpt must be present — together with the **card-id
  anchor** — in the SAME committed carrier (commit message/blob or committed
  artifact blob). An excerpt floating in an unrelated, unanchored message
  verifies nothing.
- A `routine` attestation is the empty-coverage case: valid only when every
  decision-class field is empty (nothing to lose). A bare `{kind:"commit"}`
  or marker-only entry is likewise valid only in that nothing-to-lose case.

### Review classification (process vs decision)

A review that resolved the card to **`completed`** — the ordinary clean
approve — is a **process record**, not decision-class: routine tasks retire
after a normal review with zero promotion burden. What remains decision-class:

- `last_review` whose status is NOT `completed` (blocked / ready / reported /
  cancelled / unknown) — a review that changed the card's trajectory, with
  its reasoning;
- any `task_reviewed` history event with a non-completed status (scanned per
  event: a later clean approve does not launder an earlier blocked review);
- owner decisions, objections, and risk acceptances recorded at review time
  belong in `owner_notes` (always decision-class) — the verdict shape is the
  smallest honest mechanical distinction the actual field shapes support.

### Retrievability (what a fresh session can find after retirement)

Retirement destroys the card and its decision_record; the surviving anchor is
git itself. A fresh session knowing ONLY the card id recovers the decision
content with bounded queries:

1. `git log --branches --fixed-strings --format=%H --grep='Task-Card: <id>'`
   → the trailer-bearing commit(s);
2. read each hit's body (`git log -1 --format=%B <sha>`) — a covering
   `commit` entry guarantees the anchored excerpt (verbatim field content)
   is there; the documented authoring pattern is to carry the FULL decision
   text in the landing commit body;
3. for file-carried content: `git log --branches -S'<id>' --format=%H` (the
   pickaxe finds the commit that introduced the card-id back-reference) →
   `git show <sha>:<path>`.

The card-id **anchor rule** on every covering carrier is what makes this
chain work: no index, no archive, no tombstone — the anchored commits are the
record.

### Refusals at retirement

The retirement path (`delete_coordination_task`, completed branch, after
landing proof) resolves every declared pointer and refuses — naming each
missing/uncommitted/unreachable item — with:

- `decision_record_unresolved` — a declared commit/artifact entry does not
  resolve against committed, branch-reachable state, or a covering entry's
  excerpt/anchor is missing from its carrier (`anchor_missing`,
  `excerpt_not_in_carrier`, `carrier_content_scattered`);
- `decision_content_unpromoted` — decision-class card fields are not covered
  by any entry's `carries` (a `routine` attestation with non-empty fields, a
  bare/marker-only record, or a legacy card with no decision_record);
  `refusal.fields` names each uncovered field.

### Compatibility (legacy + in-flight cards)

- New closeouts (post-gate) require the block — including `blocked`/`reported`
  closeouts; a one-line `routine` entry is the whole burden for routine work.
- In-flight cards (working/reported/blocked) at upgrade are unaffected until
  they close out; the closeout then runs the current gate.
- In-flight cards carrying a v1 decision_record (marker-only or bare entries,
  closed out under the 2026-10-25 gate) retire only under the coverage rule:
  if decision-class fields are non-empty and uncovered, retirement is refused
  with `decision_content_unpromoted` — re-declare via a fresh closeout with
  covering entries.
- Legacy completed-but-unretired cards without a decision_record retire under
  the same coverage rule with an empty coverage set: refused if
  decision-class fields are non-empty, otherwise routine retirement proceeds.

### Force path and private content

The operator force path keeps its semantics but the destructive warning NAMES
the decision-class content about to be destroyed (card fields + declared
entries + their coverage claims). Force still bypasses both the landing check
and the preservation check — it is the explicit emergency route.

**Privacy rule:** never commit sensitive owner notes merely to satisfy the
gate. If decision-class content genuinely cannot be promoted (private,
sensitive, or embargoed), the sanctioned path is the documented operator
force at retirement — the destructive warning names exactly what is being
destroyed — never a hollow `carries` declaration.

### Honest limits (what this gate does NOT prove)

- **Field-list completeness is mechanical** — the classifier sees
  `owner_notes`, `measured_outcome`, `open_questions`, and review signals;
  decision content left in no listed field can still be lost.
- **Excerpt choice is author honesty** — the gate proves the excerpt (a
  verbatim, length-floored line drawn from the field's own text) survived in
  an anchored, committed carrier; whether that excerpt carries the SUBSTANCE
  of the field (vs an innocuous long line) is author + reviewer judgment.
  The documented authoring pattern — full decision text in the landing
  commit body — removes the gap in practice.
- **Local reachability only** — the gate proves LOCAL-repository
  reachability (`git log --branches` / committed blobs). Cross-machine
  availability comes from the normal push/release flow; the gate performs no
  network operations and claims nothing about remotes.

### Legacy

The landing-proof requirement applies to **new cards filed after codification**.
Pre-existing cards in transport at codification time were grandfathered (retired
under the prior convention); since the Slice-2 retirement code landed (commit
`00b5add`), all `completed` retirements are landing-gated regardless of filing
date. The retention-preservation gate follows the same shape: it binds at
closeout-time for new closeouts and at retirement-time (contradiction rule
only) for legacy completed cards.

## Records

The **landing commit** (carrying the card id via the `Task-Card:` trailer) is
the durable record of completed work. There are:

- **No tombstones** — retiring a transport card leaves no marker card behind;
  the reachable commit is the record.
- **No done-rows by default** — a drained card is not transcribed into a
  backlog `Done` row. Only cards that crossed the promotion boundary have a
  backlog history row (and the normalizer archives those).
- **Checkpoints only for campaign closeouts / reopenable decisions** — a single
  card's completion is recorded by its landing commit, not a checkpoint. A
  checkpoint is written when a decision, blocker, or campaign closeout is worth
  reopening later.

## What this does NOT change

- `docs/planning/backlog.md` remains the canonical task-status ledger; its
  format, normalizer, and split-commit discipline are unchanged.
- `commit-reviewer` DEFER findings remain non-blocking; the intake bar governs
  whether a DEFER finding becomes a transport card, not whether it blocks.
- The operator escape hatch and the gated-commit protocol are unchanged.
- `/task-delete <id>` remains the sanctioned single-card retire wrapper for
  `draft`/`ready`/`cancelled` cards. For `completed` cards, retirement is
  landing-gated: the sanctioned op enforces the reachability check, so a direct
  `rm` of a `completed` card is NOT equivalent — it bypasses the landing check.

## Reference

- [PROMOTER_RUNBOOK.md](PROMOTER_RUNBOOK.md) — drain-first promoter procedure
  + boundary promotion + backlog conflict discipline.
- [README.md](README.md) — Canonical State Map (transport vs truth).
- [RUNTIME_MODEL.md](RUNTIME_MODEL.md) — transport-versus-truth and promotion.
- `.opencode/skills/backlog/SKILL.md` — backlog edit discipline + intake /
  boundary / drain routing quick-reference.
- `.opencode/agents/commit-message.md` — the `Task-Card:` trailer convention
  for card-driven commits.
- `.opencode/agents/commit-reviewer.md` — DEFER as an intake predicate routed
  through the admission bar.
