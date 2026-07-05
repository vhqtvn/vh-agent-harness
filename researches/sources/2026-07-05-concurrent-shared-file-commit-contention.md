# Sources: Concurrent Shared-File Commit Contention

**Date:** 2026-07-05
**Topic:** Evidence trail for the W1 single-writer-promotion decision.
**Decision memo:**
[`../decisions/2026-07-05-commit-gate-shared-file-coupling.md`](../decisions/2026-07-05-commit-gate-shared-file-coupling.md).

This packet records the evidence, option matrix, external precedents, and the
contradiction audit behind the decision. It is intentionally a record, not a
recommendation — the decision lives in the memo.

## Confidence legend

- **HIGH** — verified against repo source or reproducible behavior.
- **MED** — single-source claim or behavioral inference; not independently
  re-verified in this slice.
- **LOW** — anecdotal / blog-level; directionally useful only.

## Problem framing (verified)

- The commit gate (`.opencode/scripts/commit-gate.sh`) is **atomic-per-acquire**
  with **blob-level 3-way merge** via `git read-tree -m -i`.
  - **Confidence: HIGH** — verified by reading the gate script and its design
    refs (`researches/decisions/2026-06-09-concurrent-commit-gate-design.md`,
    referenced from the gate header).
- A blob that does not conflict merges cleanly; a conflicting blob produces
  `cas_conflict` and blocks the **entire** acquire, including clean code blobs.
  - **Confidence: HIGH** — gate behavior.
- Every lane editing `docs/planning/backlog.md` produces a backlog blob; two
  concurrent lanes therefore contend on a single blob even when their row edits
  are semantically independent.
  - **Confidence: HIGH** — structural.
- Committers `revert backlog.md` to unblock code commits, discarding legitimate
  status updates.
  - **Confidence: MED** — observed pattern; frequency not instrumented.

## Option matrix

Four axes were considered. Only the chosen or rejected-with-reason entries are
expanded; the rest are listed for completeness.

### Workflow (W1–W4)

| Option | Summary | Verdict | Confidence |
| --- | --- | --- | --- |
| W1 | Single-writer promotion; workers → transport, promoter → canon | **CHOSEN** | HIGH |
| W2 | Split acquires (code and backlog land independently) | Rejected — formalizes partial landings | MED |
| W3 | Per-session branches + git merge | Rejected — workflow change | MED |
| W4 | Worktrees (one per lane) | Rejected as scope creep; long-term contingency | MED |

### Gate-mechanical (G1–G4)

| Option | Summary | Verdict | Confidence |
| --- | --- | --- | --- |
| G1 | Line-level merge for shared paths + normalizer check | **DEFERRED** (validation packet needed) | MED |
| G2 | (other gate-mechanical) | out of scope | — |
| G3 | (other gate-mechanical) | out of scope | — |
| G4 | (other gate-mechanical) | out of scope | — |

### Format (F1–F4)

| Option | Summary | Verdict | Confidence |
| --- | --- | --- | --- |
| F1 | (format option) | out of scope | — |
| F2 | Per-row file split (one file per backlog row) | Rejected — rewrites the 747-line normalizer | MED |
| F3 | (format option) | out of scope | — |
| F4 | (format option) | out of scope | — |

### External (E1–E4)

| Option | Summary | Verdict | Confidence |
| --- | --- | --- | --- |
| E1 | Jujutsu deferred conflicts | Precedent — informative | LOW |
| E2 | Claude Code `PreToolUse` hook | Precedent — plugin pattern reference | MED |
| E3 | Cursor `clampd-guard` | Precedent — informative | LOW |
| E4 | Uber `SubmitQueue` / git worktrees (dominant multi-agent pattern) | Precedent — worktrees validated as the field's default | MED |

## External precedents

- **Jujutsu deferred conflicts.** Jujutsu treats conflicts as first-class state
  and defers resolution, avoiding the "merge now or block" binary git imposes.
  Informs G1 (conflicts do not have to be hard-fail), but does not remove the
  need for a canonical writer.
  - **Confidence: LOW** — design-philosophy level; not re-verified.
- **Claude Code `PreToolUse` hook.** A pre-tool hook can refuse an edit and emit
  a structured message. This is the plugin pattern layer A would follow, and the
  reference point for shell-guard's own hook model.
  - **Confidence: MED** — matches the harness's existing plugin surface.
- **Cursor `clampd-guard`.** A guard that clamps agent edits to an allowlist.
  Informative for the deny-UX question; not directly applicable (different
  host).
  - **Confidence: LOW.**
- **Uber `SubmitQueue` / git worktrees.** Large-scale multi-agent workflows
  converge on **worktrees** (one checkout per agent) to eliminate working-tree
  contention. This validates W4 as the field's default escape hatch, but does
  not justify its cost at the harness's current scale.
  - **Confidence: MED.**

## Benchmark: false-conflict rate

- **Claim:** git `diff3` produces false conflicts on **~52%** of independent
  same-file edits.
- **Source:** DEV.to article (Feb 2025).
- **Confidence: LOW.** Single blog-level source; the exact percentage should not
  be treated as a precise measurement. The directional claim — that independent
  same-file edits false-conflict often enough to be a real operational burden —
  is consistent with the gate's observed `cas_conflict` → `revert backlog.md`
  loop and is sufficient to motivate the decision. A future re-evaluation should
  replace this with an instrumented measurement from this repo's own gate logs.

## Contradiction audit

Three contradictions were surfaced during this decision:

1. **Lock-free invariant — resolved SOFT.** The gate advertises a lock-free,
   atomic-per-acquire model, but the backlog-coupling failure makes a clean
   code acquire depend on a backlog blob it does not care about. Resolved
   **SOFT** by W1: the dependency is removed by eliminating the backlog blob
   from worker acquires entirely, rather than by strengthening the merge. A HARD
   resolution (G1) remains deferred.
2. **Design memos missing — resolved by this packet.** `researches/` did not
   exist; the gate header and `docs/coordination/README.md` referenced
   `researches/decisions/` paths that dangled. This packet and its companion
   decision memo establish the directory and the record-of-decision convention.
   - **Note:** pre-existing dated refs (e.g. `2026-04-29-...`,
     `2026-06-09-...`, `2026-06-03-...`) still dangle; they are dogfood-specific
     and were not in scope for this slice. Flagged for a separate cleanup.
3. **Canonical-source vs off-tree status — resolved by W1 docs.** The
   coordination docs described `.local/` as transport and `backlog.md` as canon,
   but the agent rules told workers to edit canon directly. Resolved by the
   docs slice that accompanies this decision: workers now route status to
   transport, and the promoter is documented as the sole canon writer.

## Enforcement-layer verification

- **Layer C (chosen) is config-correct.** `opencode.jsonc` carries
  `{"*":"allow","docs/planning/backlog.md":"deny"}` on both `build` and
  `docs-steward` (verified: build deny at the `edit` block; docs-steward deny at
  its `edit` block). The emitter source (`internal/permconfig/tables.go`) pins
  key order: broad allow first, narrow deny last, exploiting the engine's
  `findLast` (last-match-wins) semantics. This is the documented inverse of the
  committer's broad-deny + narrow-allow pattern.
  - **Confidence: HIGH** — read from source.
- **opencode `edit` permission governs edit + write + apply_patch uniformly.**
  A `deny` is a hard `DeniedError`, not a prompt. This is what makes C reliable
  for subagents: there is no prompt to talk past.
  - **Confidence: MED** — based on opencode's permission semantics; the exact
    error class name should be confirmed against the opencode source if it
    becomes load-bearing.

## Self-probe verdict (deny UX)

A single deliberate edit to `docs/planning/backlog.md` was attempted from a
`build` session **after** the C enforcement landed.

- **Result:** the edit **succeeded** — the deny did **not** fire.
- **Root cause:** the running opencode session was not loading the repo's
  rendered `opencode.jsonc`. It was running on a **stale user-level
  `~/.opencode` snapshot** that predates commit `2f6af9d5`, so the `edit` deny
  for `docs/planning/backlog.md` was not in effect for that session.
- **Implication for layer A:** the self-probe could **not** evaluate the
  in-session denial UX (helpful vs unhelpful) because the deny was inert in the
  probing session. Therefore the verdict on whether plugin A (edit-guard with a
  shell-guard-style canonical-alternative pointer) should graduate to "needed"
  is **DEFERRED, not discharged**. The C-only enforcement is config-correct but
  was runtime-inert in the probed session due to the stale snapshot — which is
  itself an operator-actionable finding (the user-level `~/.opencode` needs a
  refresh so the rendered config takes effect), explicitly out of scope for this
  slice per its non-goals.
- **Recommendation:** re-run the self-probe after the user-level snapshot is
  refreshed. If the refreshed session's deny message is a bare unhelpful
  "denied," layer A graduates to "needed." If it points at the `.local`
  transport / `/write-task` alternative (or the operator finds the bare deny
  acceptable given the docs), C-only stands.

## Files referenced (verified paths)

- `.opencode/scripts/commit-gate.sh` — gate implementation and design refs.
- `internal/permconfig/tables.go` — `BacklogPromoterDenyPath`, `EditOverrides`,
  key-order comments.
- `internal/permconfig/emit_test.go` — pins key order and verdict.
- `opencode.jsonc` — rendered `edit` deny for `build` and `docs-steward`.
- `.opencode/plugins/shell-guard-core.js` — "O3 hint-only design" at line 830
  (the precedent for layer A's deny-with-pointer UX).
- `docs/coordination/TASK_MODES.md` — Non-Negotiable #2.
- `docs/coordination/README.md` — Canonical State Map.
