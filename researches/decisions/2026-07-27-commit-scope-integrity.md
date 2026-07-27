# Decision — commit-scope integrity under concurrency

**Date:** 2026-07-27
**Status:** decided (implementation in flight)
**Origin:** vh-solara incident report (harness/v0.18.0); `commit-scope-integrity-reframe` session
**Run artifacts:** `tmp/agent-runs/commit-scope-integrity-reframe/{research,debate,brief}.md`

## Context
A downstream incident (vh-solara) exposed that commit-reviewer's scope-integrity diff anchors on bare `HEAD`, not `head_at_acquire`, so a concurrent commit between acquire and review makes the review-time diff show phantom files. Investigation confirmed the incident was a **false alarm** (correct block, wrong reason) AND surfaced a second, deeper surface: the gate's commit-time CAS path can replace the reviewed tree with a 3-way merged tree, commit it, and emit `rebased:true` — with **no consumer forcing re-review or refusal**. The committed tree can therefore differ from the reviewed tree, unreviewed.

## Decision
Treat both surfaces as one defect class — **approved-tree integrity under concurrency** — and fix with two separable slices:

- **S1 (review-time, option b):** forward `head_at_acquire` through the committer→reviewer handoff and repoint the review diff off bare `HEAD`. Regression invariant: *an unrequested dirty/concurrent file never enters the reviewed scope; review base = acquire anchor.*
- **S2 (commit-time, fail-closed):** when the CAS path would commit a merged tree ≠ reviewed tree, refuse the commit and require re-acquire (reusing the existing `could_not_land` recovery pattern). Regression invariant: *committed_tree == reviewed_tree, OR refusal triggered.*
- **Instrumentation (part of S2):** log the rebase/CAS event rate so the throughput question becomes measured rather than guessed.

## Rationale
- **S1 = (b)** repairs the false referent directly, using the anchor the gate already records and uses for its scope-correct `files` calculation. (a) field+doc is adjunct only; (c) consume-files is a strong complement but insufficient alone.
- **S2 = fail-closed** because: (1) do not defer a known reachable integrity hole while waiting to quantify a throughput cost — close with the safe default, measure, revisit only if it materializes; (2) fail-closed reuses the existing re-acquire recovery, not a new burden; (3) auto RE-VIEW carries livelock risk and needs bounded-retry built against an unmeasured need.

## Alternatives considered (rejected)
- **RE-VIEW merged tree:** deferred (livelock risk + unmeasured rebase rate) → `.local/coordinator/tasks/defer-review-rebase-retry`.
- **Disjoint-auto-merge hybrid:** rejected (no safe semantic-conflict classifier).
- **Accept current behavior:** rejected (E2 is a reachable integrity hole).
- **"Document `head_at_acquire` anchor and stop" (F-NOOP):** eliminated — no such anchor exists in any reviewer-facing surface (`diff_base` is only a local var in `cmd_acquire`).

## Consequences
- Both slices tracked; neither trigger-deferred. Landing S1 alone does NOT resolve S2.
- RE-VIEW-with-bounded-retry parked as `defer-review-rebase-retry` (draft, trigger-gated on *measured rebase rate makes fail-closed friction material*).
- Edits land in `templates/core/` (canonical), rendered via `make update`; `README.agent.md` sync if the command surface or gate contract changes; `go test ./...` / `gofmt` / `go vet` must pass.

## References
- Verified facts & citations: `tmp/agent-runs/commit-scope-integrity-reframe/debate.md`
- Execution plan: `tmp/agent-runs/commit-scope-integrity-reframe/brief.md`
- Source sites: `commit-reviewer.md:120,149` / `commit-reviewer-a.md:36` / `commit-review.md:69` / `committer.md:99-105` (S1); `commit-gate.sh:1007-1075, 1119-1123` / `committer.md:107-117` (S2)
