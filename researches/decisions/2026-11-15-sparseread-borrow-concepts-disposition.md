# SparseRead (arXiv:2608.22237) — Borrow-Concepts Disposition

- **Date:** 2026-11-15
- **Status:** DECIDED (verdict: BORROW-CONCEPTS)
- **Source Packet:** `researches/sources/2026-11-15-sparseread-2608.22237.md` (pointer-only evidence record & code audit)

## 1. Verdict & Confidence Split

- **Verdict [Fact]:** **BORROW-CONCEPTS** — adopt protocol semantics into repo-native surfaces; do NOT install the package; do NOT pilot the integration.
- **Confidence [Inference]:** HIGH on borrow-concepts vs. pass; MEDIUM-HIGH on rejecting package adoption/pilot.
- **Trial caveat [Fact]:** No live trial was executed; confidence rests on the static code audit, architectural overlap analysis, and repo governance constraints.

## 2. Landed Borrowables (Commit 004a6e22)

The three borrowables already landed via commit `004a6e22` (`docs(coordination): add evidence coverage & read-accounting convention`, 2026-11-15):
1. **Per-slot coverage receipts & honesty semantics [Fact]:** Slot status tracks examination, not sufficiency ("coverage != sufficiency"; evidence declarations never carry completion authority) → `templates/core/docs/coordination/REPORT_ENVELOPES.md` § "Evidence Coverage and Read Accounting" (referenced in researcher/repo-explorer agent prompts).
2. **Episode-scoped read accounting as a recorded ledger [Fact]:** Cumulative exposure ledger with explicit units and measurement basis (`measured`, `estimated`, `unknown`; `unknown != 0`; partial ledger is never a bound) → `templates/core/docs/coordination/REPORT_ENVELOPES.md` § "Read ledger (a record, not a budget)".
3. **Freshness-aware cross-session reuse dedup [Fact]:** Reuse by default with five explicit exceptions (changed source version, new/broader question, insufficient depth, required verification, missing entry) → `.opencode/skills/deep-research/SKILL.md` and `references/run-packet.md`.

## 3. Decisive Reasons Against Installing or Piloting

Four decisive reasons reject package adoption and live piloting:
1. **Existing layer mitigation [Fact]:** Read costs are already bounded where it matters: agent charters mandate snippet-first reads (`ls`, `grep`, `head`), while the MCP layer (`vhmcp`) enforces query-scoped fetches and character limits.
2. **Untrustworthy benchmark transfer [Fact + Inference]:** Audit revealed QwenClawBench task literals baked directly into core reader heuristics (`text.py`, `collection.py`) [Fact]; repo workflows primarily read code and markdown rather than the evaluated web, PDF, and spreadsheet formats [Inference].
3. **Mechanical governance conflict [Fact]:** Integrating the external package conflicts with repo architecture: generated `.opencode/` tree, overlay-pack boundaries, stale-preserve drift on `opencode.jsonc`, an ungoverned Python bridge daemon, and bootstrap-inventory plus domain-free core rules bar shipping it.
4. **Near-zero native implementation cost [Inference]:** The high-value innovations are prompt/schema-level conventions that cost essentially nothing to maintain natively.

## 4. Key Audit Findings Driving Rejection

The static code audit of `packages/sparseread-core/` (see source packet addendum for full detail) established [Fact]:
- **Algorithm 1:** DEVIATES from paper specification into a simplified pipeline.
- **Budgeted Read Gate:** PARTIAL; tracks characters, but lacks the theoretical 2L worst-case invariant, allows first-block budget overshoots, and omits transcript-fallback retirement.
- **Calibrated Reader Selection:** ABSENT; uses a deterministic veto table with hardcoded floors instead of calibrated Wilson intervals or cost estimators.
- **Verification:** Lacks budget-bound and calibration tests; test suite validates hardcoded benchmark task answers.

## 5. Read-Isolation Positioning Note

Under the repo's read-isolation policy (`researches/decisions/read-isolation-policy.md`), tool-layer read interception can only ever provide efficiency, never read authority [Fact]. Content admission or pruning in a tool cannot gate, verify, or substitute for confidentiality boundaries or transition authority [Fact].

## 6. Open Follow-up

- **Follow-up task [Fact]:** Phase 2 episode evaluation of the landed convention is tracked in local task card `coverage-convention-phase2-episode` (in flight).
