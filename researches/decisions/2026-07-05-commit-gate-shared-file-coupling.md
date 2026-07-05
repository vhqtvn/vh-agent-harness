# Decision: Commit-Gate Shared-File Coupling (W1 Single-Writer-Promotion)

**Date:** 2026-07-05
**Status:** Accepted (W1 model + C enforcement). Enforcement landed in
`2f6af9d5`; this slice ships the docs/coordination changes and record-of-decision.
**Supersedes:** none.
**See also:**
[`../sources/2026-07-05-concurrent-shared-file-commit-contention.md`](../sources/2026-07-05-concurrent-shared-file-commit-contention.md)
(evidence trail).
[`../../docs/coordination/PROMOTER_RUNBOOK.md`](../../docs/coordination/PROMOTER_RUNBOOK.md)
(promoter procedure).

## Problem

Parallel lanes each commit their own code **plus** the shared canonical task
file `docs/planning/backlog.md`. The commit gate
(`.opencode/scripts/commit-gate.sh`) is **atomic-per-acquire** with **blob-level
3-way merge** (`git read-tree -m -i`): a blob that does not conflict merges
cleanly; a conflicting blob produces `cas_conflict` and blocks the **entire**
acquire (commit), including clean code blobs in the same acquire.

The coupling is **synthetic**:

- **Code isolation works.** Two lanes editing disjoint files never conflict;
  their acquires land independently.
- **Backlog coupling is the failure.** Every lane that follows the
  "update your backlog row before/after work" rule edits the **same** file.
  A backlog blob-conflict → `cas_conflict` → the entire commit (clean code
  included) is blocked. Committers fall back to `revert backlog.md` every cycle
  to unblock code, which discards legitimate status updates and forces a
  re-promotion. This is pure overhead: the backlog edits are independent rows,
  not semantically conflicting, but git's blob-level merge cannot tell.

## Options Considered

### Workflow axis (W1–W4)

- **W1 — Single-writer promotion (CHOSEN).** Workers stop editing the backlog
  directly; status intents flow through `.local/coordinator/tasks/` (transport).
  One promoter batch-promotes consolidated results to canon per cycle. Eliminates
  backlog blob-conflict at the source: workers never commit a backlog blob.
- **W2 — Split acquires (REJECTED).** Let each acquire land code and backlog
  separately so a backlog conflict does not block code. Rejected because it
  **formalizes partial landings** — a slice's code lands while its status update
  is stuck, leaving canon incoherent. Trades one failure mode for a subtler one.
- **W3 — Per-session branches + git merge (REJECTED).** Move concurrency out of
  the gate into git's own merge. Rejected as a **workflow change**: it replaces
  the single-checkout, gate-mediated model the harness is built around, and
  pushes merge resolution onto every operator.
- **W4 — Worktrees (REJECTED as scope creep; long-term contingency).** One
  worktree per lane gives each its own checkout and eliminates working-tree
  contention entirely. Rejected now because it conflicts with the host-shell
  single-checkout runtime and is a much larger change than the problem demands.
  Kept as a contingency if W1's promoter model proves insufficient at scale.

### Gate-mechanical axis (G1–G4)

- **G1 — Line-level merge for shared paths + normalizer check (DEFERRED).**
  Teach the gate to do a line-level 3-way merge for designated shared files
  (initially `docs/planning/backlog.md`), falling back to blob-level for
  everything else. Would let independent row edits merge cleanly without W1's
  transport detour. **Deferred** because it has core-wide blast radius (the gate
  is on every acquire for every repo) and needs a dedicated validation packet
  before it can be trusted. Tracked as follow-up.
- G2–G4 — other gate-mechanical options; out of scope for this decision. See the
  source packet for the full matrix.

### Format axis (F1–F4)

- **F2 — Per-row file split (REJECTED).** Split the single backlog into one file
  per row so each row is its own blob. Rejected because it **rewrites the
  747-line normalizer** and the section/rotation model, and inflates the file
  count without addressing the root coupling for any future shared file.
- F1, F3, F4 — other format options; see the source packet.

### External axis (E1–E4)

- Jujutsu-style deferred conflicts, Claude Code `PreToolUse`, Cursor
  `clampd-guard`, Uber `SubmitQueue`, git worktrees as the dominant multi-agent
  pattern. See the source packet for precedents and confidence levels.

## Enforcement Layers

Three layers were on the table to make W1 real:

- **A — Edit-guard plugin (DEFERRED).** An opencode plugin (shell-guard-style)
  that refuses worker edits to the backlog and emits a denial message pointing
  at the canonical alternative (`.local` transport / `/write-task`). A subagent
  gap was hypothesized (can plugins see the agent identity?); the gap was
  **refuted empirically** — the per-agent permission map is the harness's own
  idiom for per-agent rules, and the shell-guard "O3 hint-only design"
  precedent (`shell-guard-core.js:830`) shows the hint-only UX pattern is already
  in-repo. Deferred because layer C is cheaper and sufficient; A remains the
  upgrade path if the C denial UX proves unhelpful (see Verdict below).
- **B — Commit-gate refuse-mixed (BACKSTOP, deferred).** Teach the gate to
  refuse an acquire that mixes code blobs with backlog blobs from a worker.
  Useful as a defense-in-depth backstop behind C, but redundant while C holds.
  Deferred.
- **C — Per-agent edit permission map (CHOSEN).** Add an `edit` override to
  `build` and `docs-steward` in the permconfig emitter:
  `{"*":"allow","docs/planning/backlog.md":"deny"}`. **Cheapest, reliable for
  subagents, matches the harness idiom.** Key order is load-bearing: `"*"` first,
  deny last — the opencode engine is `findLast` (last-match-wins), so the narrow
  deny wins for that one path while every other path resolves to the broad
  allow. This is the **inverse** of the committer's pattern (broad deny + narrow
  allow for `tmp/commit-gate-message/**`).

## Decision

**W1 model + C enforcement.** Workers (`build`, `docs-steward`) are denied
`docs/planning/backlog.md` edits via the opencode per-agent permission map
(layer C on both agents: `{"*":"allow","docs/planning/backlog.md":"deny"}`,
key order load-bearing). The promoter (operator/coordination initially) is the
sole writer. G1 deferred behind a validation packet. A/B deferred hardening.

This is consistent with `docs/coordination/TASK_MODES.md` Non-Negotiable #2
("one writer promotes fan-in results back to canon"), which already prescribed
the model at fan-in closeout; this decision extends it to **during execution**
for the canonical status file.

## Tradeoffs

- **(+)** Eliminates backlog blob-conflict at the source. Code commits can never
  be blocked by a backlog edit again.
- **(+)** Smallest enforcement surface (one permconfig seam, no gate change, no
  plugin). Matches the harness's own idiom.
- **(+)** Workers keep full edit access to every file except the one canonical
  status file — no productivity loss on code/docs.
- **(−)** Introduces a **stale-status window**: `backlog.md` lags live state
  between promoter runs. Mitigated by documenting it explicitly (see
  `PROMOTER_RUNBOOK.md`) and bounding it via cadence.
- **(−)** Adds a human-in-the-loop promoter step until a dedicated promoter
  agent exists. Bounded: the promoter is a batch batch-promote, not per-edit.
- **(−)** C enforcement alone gives a bare denial UX (see Verdict). If operators
  find it unhelpful, layer A graduates to "needed."

## Deferred Work

- **G1** — line-level merge for shared paths + normalizer check. Needs a
  validation packet covering merge correctness, conflict semantics, and the
  normalizer interaction before it can be trusted on the gate's hot path.
- **A** — edit-guard plugin. Graduates to "needed" iff the C denial UX is
  judged unhelpful (see Verdict in the source packet's self-probe).
- **B** — commit-gate refuse-mixed backstop. Revisit if C is bypassed in
  practice.
- **Dedicated promoter agent.** Not needed at current scale; the operator /
  coordination session carries it.

## Evidence

- `docs/coordination/TASK_MODES.md` Non-Negotiable #2 already prescribes "one
  writer promotes fan-in results back to canon."
- `researches/` did not exist prior to this memo — three dangling references in
  the gate header (`.opencode/scripts/commit-gate.sh`) and
  `docs/coordination/README.md` pointed at `researches/decisions/` paths. This
  memo and its source packet establish the directory and the record-of-decision
  convention. (Note: pre-existing dated refs to other memo filenames remain
  dangling; those citations live under `templates/core/` and render into every
  consumer's `.opencode/` and `docs/` on `update` — a consumer agent reading
  e.g. `maxoutputtokens.js` hits the dangling ref in its own repo, so this is
  consumer-shipped debt, not dogfood-only. They remain out of scope for this
  slice.)
- **Benchmark:** DEV.to (Feb 2025) — git `diff3` produces false conflicts on
  ~52% of independent same-file edits. This is the empirical motivation for
  treating backlog coupling as a real, not theoretical, failure mode. See the
  source packet for the citation and confidence level.
- **opencode `edit` permission semantics:** governs `edit` + `write` +
  `apply_patch` uniformly; a `deny` verdict is a hard `DeniedError`, not a
  prompt. This is what makes layer C reliable for subagents (no prompt to
  bypass).
