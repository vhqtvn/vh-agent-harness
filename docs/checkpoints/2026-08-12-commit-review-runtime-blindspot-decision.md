# Checkpoint: Commit-Review Runtime-Blindspot Decision

**Date:** 2026-08-12
**Status:** Implemented — see "Implementation Addendum (2026-08-13)" below (original decision accepted 2026-08-12, preserved unchanged)

## Overview
This checkpoint records the accepted decision from the phased think-mode solution-brief for the "commit-review runtime-blindspot" problem. The problem involves runtime defects that are invisible to the diff (e.g., cross-origin iframe focus issues where tests pass on direct API calls).

## Problem Reframe
This is NOT primarily a reviewer-panel problem; it is a **crux-gate** problem. The commit reviewer can only ever be an advisory detection/falsification surface for this class (Verifiability Clause ceiling). The only existing mechanism with transition authority over "was the load-bearing path actually exercised" is the `behavioral-closure` crux gate — which does not currently ask about *user-event reachability*.

## Accepted Recommendation: Frame F3 (Composite)
- **M-x**: Extend the `behavioral-closure` crux gate so that *interaction-touching* changes require a **user-event-reachability receipt**.
- **M-a**: An advisory overlay skill acts as the reachability tracer/detector AND a reviewer falsification surface (advisory only; no BLOCK authority of its own).
- **M-d**: Authoring-side reachability claims feed the receipt.

**Detection/enforcement split:** M-a detects (advisory); M-x enforces (block authority); M-d inputs.

**Verdict on the original "frontend review skill" framing:** **SUBSUMED** — survives as the M-a advisory tracer, not as the answer. The skill alone has a strict DEFER/advisory ceiling and cannot block.

### Five Hardening Conditions (M-x)
1. **Receipt fields required**: real user action · target behavior · actual environment · verifier command/outcome · tree binding · observed user-visible outcome.
2. **Mechanism-asserting receipts downgraded**: API calls / flags / code-path assertions are classified `skipped`, never `proven`, unless the user-visible outcome was actually observed.
3. **Label honesty**: an M-x pass is labeled "structural completeness only," never "reachability proven."
4. **Named M-a review as a falsification surface**: advisory reviewer flags shallow/inconsistent/non-falsifiable receipts — no BLOCK authority.
5. **Honest infeasibility**: `not-demonstrable` / `inconclusive` when no verified seam can observe the outcome (blocks `completed`, routes to defer — existing mechanism).

### Authority Finding
The receipt-honesty dependency is **CONSISTENT with the existing `behavioral-closure` crux receipt pattern**, NOT a novel authority flaw. AGENTS.md already codifies all crux receipts as presence-verified (honesty = author+reviewer, not the gate). M-x extends an accepted pattern to a new receipt type. The critical sharpening: interaction reachability makes the existing **outcome-vs-mechanism** distinction load-bearing. A receipt asserting the mechanism rather than the outcome is the `skipped`-as-`proven` trap.

### Residual Risk
A dishonest or mutually-mistaken author+reviewer pair can file a detailed, tree-bound, but false outcome receipt. M-x passes structure; M-a cannot mechanically stop the transition. This is accepted knowingly as it is the SAME residual risk the entire crux-receipt system already carries (not new exposure).

### Out of Scope / Next Slice
The actual implementation is explicitly NOT authorized by this accept. Editing the AGENTS.md behavioral-closure crux section, authoring the M-a overlay skill under `.vh-agent-harness/overlays/`, and any `docs/ai/` playbook are part of a separate slice to be scoped later. Options explicitly NOT resurrected: adding a reviewer seat, 5-leaf panel reshape, or raising blind-spot volume (the `r4_blind_spot` reservation remains "not approved").

## Findings
- **Finding (Problem Boundary)**: source=Case study ff74f36, confidence=high, type=fact. Runtime defects invisible to the diff (e.g. cross-origin iframe focus issues) pass tests but fail in the real environment.
- **Finding (Reviewer Limit)**: source=Verifiability Clause, confidence=high, type=fact. Commit reviewer has a strict DEFER/advisory ceiling and cannot block based on diff alone for runtime reachability.
- **Finding (Authority Precedent)**: source=AGENTS.md behavioral-closure, confidence=high, type=fact. The proposed M-x receipt honesty model perfectly matches existing crux receipt mechanisms (presence-verified by gate, honesty verified by author+reviewer).

## Contradictions
None detected.

## Verification
| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Decision documented correctly without source edits | `git status` (shows only the checkpoint file created, no tracked files modified) | yes |
| Adherence to M-x receipt shape logic under case study ff74f36 | Replay test: honest receipt filing results in `not-demonstrable` and blocks `completed`. A falsely `proven` mechanism-receipt is caught by condition 2 + M-a falsification. | yes |

## Implementation Addendum (2026-08-13)

The F3 (Composite) implementation recorded by the accepted decision above has
now landed. This addendum preserves the original decision text (including the
pre-implementation Verification table above, dated 2026-08-12) unchanged and
records the actual implementation, its files, and its verification. It does
NOT alter the accepted decision, the authority finding, or the residual-risk
note — those stand as the historical record of the decision.

### Files (as implemented)
- **M-x validator (doctor check #14):** `internal/cli/doctor_behavioral_closure.go` — `analyzeInteractionReachability` (called from the behavioral-closure check) enforces the author-declared `interaction_touching: true` predicate, the six required `interaction_*` receipt fields, the `interaction_evidence: outcome|mechanism` classification, and the condition-2 mechanism-plus-proven rejection. A FAIL marks the repo UNHEALTHY → non-zero exit → blocks release G0c.
- **Crux / receipt text (core):** `templates/core/.vh-agent-harness/AGENTS.core.md` — the behavioral-closure crux section and the five interaction-reachability receipt conditions (author-declared predicate + six fields + outcome-vs-mechanism + structural-completeness-only labeling + honest infeasibility).
- **Receipt block template:** `templates/core/docs/coordination/CLOSEOUT_TEMPLATE.md` — the canonical interaction-reachability receipt shape rendered into consumer repos.
- **M-a overlay pack:** `templates/overlays/frontend-ui-pilot/skills/interaction-reachability/SKILL.md` — the advisory tracer/falsifier skill (Procedure A tracer, Procedure B falsifier, no BLOCK authority of its own).
- **Tests:** `internal/cli/doctor_behavioral_closure_test.go` — covers the FAIL-path conditions (missing receipt fields, missing `interaction_evidence`, mechanism+`proven`, garbage enums) and the PASS-path conditions.

### Authority (as implemented, restated for accuracy)
The enforcement surface is `vh-agent-harness doctor` check #14 (release/health
axis): a doctor FAIL marks the repo UNHEALTHY → non-zero exit → blocks release
G0c. It does NOT mechanically refuse the closeout transition itself:
`saveCoordinationTaskCloseout` parses `rewrite-parity` only, never
`behavioral-closure`. Routing an inconclusive crux to defer is an honesty
requirement of the authoring workflow (author + reviewer); doctor is the later
consistency audit that blocks release. M-a stays advisory (S2-hold, no BLOCK
authority). This matches the original decision's detection/enforcement split
(M-a detects; M-x enforces via the health/release axis; M-d inputs).

### Verification (implementation)
| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Validator logic unchanged by THIS slice | this slice is a wording/docs pass; `analyzeInteractionReachability` body is untouched (comment-only edit to the same file) | yes |
| doctor check #14 enforces the interaction receipt | `go test ./internal/cli/ -run TestCheckBehavioralClosure -count=1` → PASS | yes |
| Full suite green, tree-bound | `go test ./... -count=1` → all PASS | yes |
| doctor HEALTHY on the dogfood repo with a FRESH binary | rebuild then `vh-agent-harness doctor` → HEALTHY, behavioral-closure check PASS | yes |

> Note: the original "Verification" table above (dated 2026-08-12) recorded
> pre-implementation expectations against a `git status` that showed only the
> checkpoint file (no source edits yet). It is preserved unchanged as the
> historical decision record; the verification table in this addendum
> supersedes it for the implemented state.
