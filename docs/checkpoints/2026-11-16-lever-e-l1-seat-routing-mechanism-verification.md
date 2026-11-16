# Lever-E L1 (mechanism) verification — seat-routing metadata reaches all four seats in a live cascade

**Date:** 2026-11-16
**Session:** `eval-lever-e-l1` · **Backlog row:** P2-EVAL-001 (status: `in_progress`, L1 only)
**Scope:** L1 acceptance layer ONLY. L2 (outcome/recall comparison) is OUT of scope and remains gated behind its own prerequisites (fixed reference corpus + operator-authorized temporary baseline swap — neither performed, neither prepared).

## Result

**L1 verdict: mechanism-observed (proven per the row's L1 assertion set).** In a live
orchestrator cascade run (all four leaves invoked in parallel, tier `free`), the intended
per-seat routing metadata was observed — via the row's named evidence channel (leaf
invocation payloads / opencode task records) — to reach every seat correctly:

| Seat | Captured `seat_routing` block (DB task payload) | Spec assertion |
|---|---|---|
| commit-reviewer-a | `r2_contract_data` (specialist), lens `contract_data`, focus classes `contract_drift, data_integrity`, role description from `panel.roles`, PRIORITIZE declared-contract consistency / state-transition correctness / data invariants / subtle concurrency correctness, still-every-class clause | PASS (all 9 required fields present; generalist phrasing absent) |
| commit-reviewer-b | `r1_redundant` (generalist), `block_finder`, `* (all classes)`, independently-attributable, no special emphasis | PASS (specialist fields ABSENT from the block) |
| commit-reviewer-c | identical generalist block (seat name only differs) | PASS |
| commit-reviewer-d | identical generalist block (seat name only differs) | PASS |

Mechanism-alone-is-not-sufficient stands: L1 proven ≠ lever-E success; L2 (final-surviving
recall on the targeted classes vs the pre-E uniform baseline) is still REQUIRED and not run.

## What ran (live exercise)

- **Trigger:** direct `task` dispatch of the real `commit-reviewer` orchestrator agent
  (rendered `.opencode/agents/commit-reviewer.md`, Lever-E section live), review slice =
  historical commit `5c9f370` (4 files, mixed prompt+config → Step-0 lightweight check
  correctly fell through to the full tier cascade; `premium` tier disabled/skipped).
  No tree_hash (no gate acquire — historical-commit review via the orchestrator's own
  documented no-tree_hash fallback). Read-only: no git mutations, no config changes.
- **Orchestrator session:** `ses_e1e19360cffenhruzl46Mc8ngy` (opencode DB, read-only query).
- **Leaf sessions (children):** a `ses_e1e183a01ffe3c5xGRJdZtUTrw`, b `ses_e1e18225bffeqNtdm0ORk61dHM`, c `ses_e1e180a76ffePHvEQOJ7kHstjE`, d `ses_e1e17f33fffegzdySNKpKS1eLx`.
- **Evidence channels:** (1) PRIMARY — task tool-call payloads captured from the opencode
  session records (4 calls, one per leaf; prompt sizes 3771 for a vs 3225 identical for
  b/c/d) plus each leaf session's first-received payload (delivery fidelity: block core
  present in all four); (2) SECONDARY — orchestrator self-echo of the forwarded blocks
  (observation rider in the dispatch), consistent with the captured blocks on all four
  seats after normalizing only the seat-name token.
- Cascade verdict was `approve` (0 BLOCK, 3 DEFER, 7 DROP); one leaf (d) independently
  reported in-run that its dispatch carried the generalist block matching the config.
- Full evidence + assertion scripts: `tmp/agent-runs/eval-lever-e-l1/`
  (`l1-payload-capture.json`, `l1-verdict.json` — block-scoped assertions, all seats pass).

## Readiness prerequisites (4-tuples)

1. Panel config with Lever E activated — value: leaf_roles a=`r2_contract_data`/`contract_data`, b/c/d=`r1_redundant`/`block_finder`; source: this inspection; re_derivation: `jq '.panel.leaf_roles' .opencode/config/review-tiers.json`; observed_at: 2026-11-16. AVAILABLE.
2. Orchestrator forwarding rule live — value: "Seat routing metadata (Lever E)" section present in rendered orchestrator prompt; re_derivation: `grep -n "Seat routing metadata" .opencode/agents/commit-reviewer.md`; observed_at: 2026-11-16. AVAILABLE.
3. Lever-E trigger precondition — value: 5c9f370 landed (ancestor of HEAD); source: mission settled assumption; re_derivation: `git merge-base --is-ancestor 5c9f370 HEAD`; observed_at: 2026-11-16. AVAILABLE (re-derived TRUE).
4. Leaf agents registered with models — value: a–d registered in opencode.jsonc with model files in `.local/config/agent-model/`; re_derivation: `grep commit-reviewer-a opencode.jsonc` + `ls .local/config/agent-model/`; observed_at: 2026-11-16. AVAILABLE.
5. Live dispatch surface — value: `task` tool can invoke `commit-reviewer`; observed by execution. AVAILABLE.
6. Observation surface — value: opencode task records queryable read-only; re_derivation: `sqlite3 file:~/.local/share/opencode/opencode.db?mode=ro` session/part query (script: `tmp/agent-runs/eval-lever-e-l1/capture_payloads.py`); observed_at: 2026-11-16. AVAILABLE.

## Verification

| Claim | Verifying command/output | Verified |
|---|---|---|
| 5c9f370 is live in HEAD | `git merge-base --is-ancestor 5c9f370 HEAD` → exit 0 | yes |
| All 4 leaves invoked in one live cascade | DB query: 4 child sessions of `ses_e1e19360cffenhruzl46Mc8ngy`, 4 `task` tool calls | yes |
| Seat A block carries specialist fields | `assert_l1_v2.py` → a: pass=True (9/9 fields; generalist phrasing absent) | yes |
| Seats B/C/D blocks carry generalist fields | `assert_l1_v2.py` → b/c/d: pass=True (5/5 fields; `r2_contract_data`/`contract_drift`/`data_integrity`/`prioritize` ABSENT from blocks) | yes |
| Blocks actually delivered to leaves | delivery_fidelity: block core present in all 4 leaf sessions' first received payloads | yes |
| Orchestrator echo matches captured blocks | `echo_vs_capture.py` → consistent=True ×4 (seat-name token normalized) | yes |
| `r2_contract_data` string in B/C/D payloads is dispatch context, not contamination | `show_b_context.py` → single occurrence at char 1205, inside the eval session's own feature summary; block starts char 2680 | yes |

## Findings

- **L1 passes with all four seats observed on the primary channel.** type=fact (DB-captured payloads, block-scoped assertions).
- **Leaf prompts unchanged** (no `seat_routing` in leaf agent bodies; additive per-invocation metadata confirmed structurally). type=fact.
- **The Step-0 lightweight branch did not fire by design** (non-doc slice → full cascade). The Step-0 CHECK ran live and correctly fell through (orchestrator validation_notes). type=fact.
- **The cascade consumed a concurrent session's bound contract** (`convention-phase2-coordination`, empty skeleton, forwarded verbatim) — task-contract injection axis; read-only, no effect on seat routing, no concurrent dirt touched. type=fact.
- The review emitted 3 DEFER findings about the same coverage obligation this row exists to discharge (B-F2/C-F3/D-F3 in the cascade output) — consistent with this slice's purpose; they are the L1/L2 obligations themselves, not new defects. type=inference.

## Contradictions

- The row cites `.local/coordinator/tasks/recall-observational-validation-protocol.json` as "kept" (L2 methodology tracker); the file is ABSENT from `.local/coordinator/tasks/` (`find .local -name "*recall-observational*"` → empty). Does not affect L1; matters for L2 methodology continuity. Recorded, not acted on (out of scope).
- The first assertion run flagged `r2_contract_data` inside B/C/D payloads; traced to the eval session's own feature-summary text, not routing contamination (probe scope bug, fixed to block-scoped). No repo-level contradiction.
- None detected in the mechanism itself (captured blocks match config + spec exactly).

## Not verified / out of reach

- **L2 entirely** — blocked on (a) fixed reference-finding corpus (operator/test-corpus-dependent, not fixed in code) and (b) operator-gated temporary baseline config swap (pre-E uniform panel). Neither performed nor prepared, per scope fences.
- **Lightweight Step-0 single-leaf forwarding branch** — not exercised (slice was non-doc by design); outside the row's L1 assertion set (which names the four panel seats), noted for completeness.
- **The `/commit-review` command wrapper path** (gate acquire → tree_hash → head_at_acquire plumbing) — not part of this run; the seat-routing steps themselves (config read → block construction → per-leaf forwarding) were exercised live.
- Leaf model identity files — existence verified, contents not read (gitignored operator state).

## Disposition

Row P2-EVAL-001 stays `in_progress` (NOT `done`): L1 complete with verdict
**mechanism-observed (proven)**; L2 remains. Evidence retained under
`tmp/agent-runs/eval-lever-e-l1/` per the row's directive; this checkpoint is the durable
result record. No git mutations performed in this slice; commit routing is left to the
coordinator's gated chain.
