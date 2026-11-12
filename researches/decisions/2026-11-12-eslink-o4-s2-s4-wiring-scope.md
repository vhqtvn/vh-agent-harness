# Decision: ESLink O4 S2–S4 Wiring-Scope Ratification Packet (v2)

- **Date:** 2026-11-12 (drafted) · **Ratified:** 2026-11-13 — operator, all lines as recommended
- **Status:** RATIFIED
- **Boundary:** ESLink static import-link validation wiring into seam apply, accept-platform, and doctor check #28
- **File:** `researches/decisions/2026-11-12-eslink-o4-s2-s4-wiring-scope.md`

---

## 1. Provenance Chain & Context

1. **v1 Base Packet** (planner ses_e31d3f7f4ffe): S0 standalone lexer fix, S2 seamApply pre-write gating with 6-element refusal contract, S3 doctor check #28, S4 honesty tests and docs, 2-cascades/slice budget.
2. **Researcher Verification Pass** (ses_e2f3489dbffe): confirmed core assumptions; found — `accept-platform` bypasses `seamApply` (`acceptOnePath` at `accept_platform.go:466` via `prepareSeamStaging:189`), directly creating skew unless explicitly gated; install-over-existing reaches `seamApply` (`install.go:344`); G0c doctor-HEALTHY gate lives at `scripts/release-tag.sh:642-644` (not `release_gate.go`); 15 UNKNOWN modules in the dogfood repo create expected #28 warnings; TOCTOU same-class-as-today; `eslink.Scan(map[string][]byte)` + `BrokenEdge{Importer,Specifier,Provider,Symbol,Reason}` + `HasBroken` fit the refusal contract; gate-5 required test already proves the state-lib-skew refusal case.
3. **Debate Pass** (ses_e2f2d9804ffe): extended pre-write refusal to `acceptOnePath` with prospective single-path substitution; pinned actual retained live bytes for preserved files in prospective assembly; registered `--allow-broken-cohort` across all three entry points; settled Decision 1 → **1B (informational dry-run)** and Decision 2 → **O3 (boundary predicate)**; budget 1+2+2+2.
4. **O3 Feasibility Follow-Up** (ses_e2f26a1ddffe — closed): staged render IS the membership oracle at both sites (doctor already re-renders via the shared `renderSeamStaging`, `doctor.go:1083-1100`, `seam.go:822-826`); active set = staged walk (slash-clean keys) minus `inactiveLive`, keeping classes `{platform_managed, overlay_extension}`; `origin-hashes.json` REJECTED as oracle (core-only, `apply.go:308-310`; raise→exit lag), as are `rendered-outputs.json` (skills-only) and `lineage.yml` (not a manifest); predicate semantics = currently-would-render; diverged-managed endpoints sit on the FAIL side; consumer-deleted platform paths remain members but cannot endpoint a broken edge (provider-presence precondition, `graph.go:221,312`); re-export chains decompose per-hop so either-endpoint membership evaluates correctly; helpers needed: shared `ActivePlatformPaths(stagingDir, inactive, classifier)` export + optional additive `BrokenEdge.Line` (or `EdgeLedger` join) for file:line receipts; NO eslink semantic changes.
5. **Consolidation (v2)** (planner ses_e2f22cc81ffe) → operator ratification 2026-11-13, all lines as recommended. Editorial realignment at landing: the v2 draft's rejected-alternative labels under Decision 2 had swapped the original 2A/2B definitions; corrected in this record (the adopted option was stated correctly and is unchanged).

---

## 2. Scope Sections (A through E)

### Section A: Slice 0 Standalone U+2028 Lexer Fix
- **Mechanism**: In `internal/eslink/lexer.go:348-356`, `isIdentPart` coarsely accepts `c >= 0x80`, inadvertently absorbing Unicode line separators U+2028 (LS, `\xE2\x80\xA8`) and U+2029 (PS, `\xE2\x80\xA9`) into identifier tokens. The fix terminates identifier continuation when `isLineTerm(src, i)` evaluates true, leaving the 3-byte sequence to `next()`'s existing line-terminator handler (`lexer.go:108-110`). Grammar-neutral (tokenization boundary fix, no new admitted variant — freeze record `12d8195` intact; one-line append-only amendment note in the variant map per the TLA-amendment precedent).
- **Corpus Invariant**: Embedded corpus has zero LS/PS bytes (grep receipt over `templates/`). Test counts remain pinned: 61/46/15 · 49 · 14/3/32 · 5 · 0.
- **Slice riders**: upgrade the KNOWN-DEFECT arm at `fixtures_test.go:242-267` to strict assertions (both separators, both adjacency positions); update the stale honest-defect comment; retire transport card `defer-eslink-u2028-ident-absorption` at closeout.
- **Delivery**: standalone prerequisite slice, independently committable.

RATIFICATION: [x] APPROVE  [ ] REJECT

---

### Section B: Slice 2 Pre-Write Refusal & Prospective Assembly
- **`seamApply` Refactor**: Convert positional signature (`update.go:238`, `install.go:344` — the only callers) to an options struct:
  ```go
  type SeamApplyOptions struct {
      Target            string
      Answers           map[string]string
      DryRun            bool
      Bootstrap         []byte
      AllowBrokenCohort bool
  }
  ```
- **Pre-Write Gating**: inside `seamApply` after staging assembly and before any live disk write. Dry-run Apply is needed only for action classification; the per-path `ClassifyPreserved` precedent (`accept_platform.go:447-451`) is a viable alternative.
- **Enforcement Extension (`acceptOnePath`)**: gated at `accept_platform.go:466` before the direct live write. Prospective byte map = current disk bytes with only the proposed single file's bytes substituted. No batch mechanism; sequential repair with the escape flag is the intentional path. Recovery wording must NOT promise that ordinary accept-platform always resolves an update refusal.
- **Prospective Byte Assembly**:
  - Advancing / Noop files: staged bytes from `stagingDir` (staging holds ALL managed bytes incl. Noop — full render, `seam.go:889`).
  - Preserved files: **actual retained live bytes** read from target disk (staged bytes ≠ final bytes for preserved paths).
  - Adopter/unmanaged files: live-tree sweep under `.opencode/` for whole-tree graph continuity (slash-clean keys per the `walkStaged` precedent, `apply.go:362-381`). Consumer-deleted paths are omitted (absent from the effective tree).
- **Refusal Contract (6 Elements)**:
  1. Broken edges and symbols identified (`BrokenEdge{Importer, Specifier, Provider, Symbol, Reason}`).
  2. SHA-256 hashes of all assessed prospective bytes.
  3. Explicit preservation confirmation for all preserved files.
  4. Documentation pointer: `vh-agent-harness docs preserved-managed-reconciliation`.
  5. Doctor non-suppression caveat: `--allow-broken-cohort` will not suppress subsequent doctor failures.
  6. Mandatory literal rider: *"link integrity only; behavior of preserved files is not verified"*.
- **Escape Flag**: `--allow-broken-cohort` registered on `update`, `install`, and `accept-platform`. One-shot, non-persisted in lineage or state.
- **Decision 1 integration**: under `--dry-run`, broken cohort findings print `WOULD REFUSE` + the full refusal contract, write nothing, and exit 0, with the disclaimer below.

RATIFICATION: [x] APPROVE  [ ] REJECT

---

### Section C: Slice 3 Doctor Check #28 (`cohort-links`)
- **Slot Allocation**: registered at `internal/cli/doctor.go:548`, immediately following check #27 (`dead-grants` at `:530`).
- **Severity Evaluation (O3 Boundary)**:
  - Proven broken edge where importer ∈ ActivePlatformPaths OR provider ∈ ActivePlatformPaths ⇒ **FAIL** (includes preserved/diverged platform endpoints — the state-lib skew case this arc exists for).
  - Proven broken edge where both endpoints are non-platform (adopter-internal) ⇒ **WARN**.
  - Unparseable file / unknown finding (dynamic import, open surface) ⇒ **WARN** (U6 discipline: unknown never refuses and never fails).
  - Clean tree ⇒ PASS; uninitialized directory (no `.opencode/` tree) ⇒ SKIP.
- **Membership Oracle**: the staged render — `renderSeamStaging` at both sites (`doctor.go:1083-1100`; `seam.go:822-826`), active set = staged walk minus `inactiveLive` keeping `{platform_managed, overlay_extension}`. Currently-would-render semantics (not ever-rendered).
- **Release Gate Impact**: `scripts/release-tag.sh:642-644` runs `vh-agent-harness doctor` as gate G0c and halts on FAIL. Check #28 failures block release tagging — deliberate, restated for ratification. Adopter CI blast radius: a proven platform-boundary broken link in an installed tree fails doctor — intended enforcement.
- **Dogfood Noise Handling**: this repo's 15 known UNKNOWN modules (14 update-op + compaction-primitives) yield a documented recurring WARN. No suppression, no hard-coded exception lists; a short doctor/docs note explains the known-unknown limitation.

RATIFICATION: [x] APPROVE  [ ] REJECT

---

### Section D: Slice 4 Honesty Tests & Documentation
- **`cohort_honesty_test.go`**:
  - Enforces presence of the literal rider in every refusal/warning/doctor message.
  - Asserts absence of banned phrases ("semantic verification", "behavior verified", "semantics verified", "gate verified", "behaviorally sound", "semantically sound", "logic verified", "gate intact").
  - Verifies `--allow-broken-cohort` is non-persistent and never suppresses doctor #28.
  - Verifies Decision 1B: `--dry-run` exits 0 with `WOULD REFUSE` + disclaimer.
  - Verifies sequential repair with the bypass flag on `accept-platform`; preview behavior; preserved-bytes assembly; boundary severity.
  - Vocabulary consistency with the landed warning (89ea161): safeguard-first, overlay additions-only (`ErrShadowBuiltin`), same-path via `project_owned` in `harness-ownership.yml`, docs pointer.
- **`driftRecoverySuffix` Repair**: correct `doctor.go:1433` — currently suggests the overlay-same-path route refuted by `ErrShadowBuiltin` (`seam.go:952-961`).
- **Docs Update**: `README.agent.md` + `templates/core/.opencode/skills/harness-operator/SKILL.md` — flag on three verbs, check #28, the 6-element refusal contract, sequential repair, the link-integrity-only contract. (templates/core change → `make update` + render-ledger sync per convention.)

RATIFICATION: [x] APPROVE  [ ] REJECT

---

### Section E: Execution Bounds & Invariants
- **Testing Seam**: `go test ./internal/eslink/...`, `go test ./internal/cli/...`; full battery + gofmt + vet green at each slice HEAD.
- **Bootstrap Inventory**: NO entry (flag attaches to existing CLI verbs; ledger scope = packs ∪ capabilities, `bootstrap_inventory_test.go:16-34`). Policy sentence carried in the change.
- **Pins Contract**: eslink corpus pins `61/46/15 · 49 · 14/3/32 · 5 · 0` frozen through all wiring slices; no production corpus changes.
- **Non-Goals**: no JS evaluation / semantic claims; no O3-Go duplication; no alteration of substrate preservation mechanics; no `export*` exposures; no compaction-primitives repair; no option-3 normalization; no batch accept verb; no stronger TOCTOU guarantee than today.
- **Budget**: 1 + 2 + 2 + 2 review cascades (Slice 0 lighter). Cascades never override existing gates or reviewer-requested escalation.

RATIFICATION: [x] APPROVE  [ ] REJECT

---

## 3. Key Decisions & Rationale

### Decision 1: Dry-Run Behavior on Broken Cohort — **ADOPTED: 1B (Informational Preview)**
- Under `--dry-run`, broken cohort findings output `WOULD REFUSE` + the full 6-element refusal contract, perform zero writes, and **exit 0**, with: *"Notice: dry-run exit 0 indicates preview completion only and does not certify live update readiness."*
- **Precedent**: `update.go:185-189` stale-corpus informational preview.
- **Rejected alternative (1A — fail-closed exit 1)**: preview tooling and pipelines treat non-zero dry-run exits as crashes, blocking inspection of the staged diff; no verified automation contract requires fail-closed preview (flip condition below reopens it if one appears).

RATIFICATION: [x] APPROVE  [ ] REJECT

### Decision 2: Boundary Predicate & Severity — **ADOPTED: O3 (whole-tree inspection, boundary-scoped FAIL)**
- Whole `.opencode/` JS tree analyzed. **FAIL/refuse**: any proven broken edge where EITHER endpoint ∈ ActivePlatformPaths. **WARN**: adopter-internal proven edges; all unknowns.
- **Rejected alternative 2A (whole-tree, fail-anywhere — the planner's original recommendation)**: adopter-internal WIP or scratch scripts with proven-broken imports would permanently block harness updates and doctor health — enforcement overreach into adopter-owned code.
- **Rejected alternative 2B (platform-managed paths only — scan-scope narrowing)**: misses parseable adopter-authored files with proven-broken imports of closed platform modules (the adopter→platform direction) and shrinks inspection below the runtime reality (relative imports execute in one runtime regardless of ownership).
- **Oracle + helpers**: per Section C; `ActivePlatformPaths` helper + optional `BrokenEdge.Line`.

RATIFICATION: [x] APPROVE  [ ] REJECT

---

## 4. Implementation Ordering & Cascade Budget

| Phase | Slice | Key Deliverables | Max Cascades |
|---|---|---|:---:|
| 1 | Slice 0 | Lexer U+2028/29 boundary fix (`lexer.go:348-356`), fixture upgrade, freeze-amendment note | 1 |
| 2 | Slice 2 | `seamApply` options struct + pre-write refusal; `acceptOnePath` gating; prospective assembly; `ActivePlatformPaths`; flag on three verbs; 1B preview | 2 |
| 3 | Slice 3 | Doctor check #28, O3 predicate, G0c wiring, dogfood-noise note | 2 |
| 4 | Slice 4 | `cohort_honesty_test.go`, `driftRecoverySuffix` fix, README + SKILL docs (+ledger sync) | 2 |

Total: 7 cascades across 4 slices.

RATIFICATION: [x] APPROVE  [ ] REJECT

---

## 5. Flip Conditions

1. **Revisit 1B → 1A**: if a verified downstream automation contract requires `--dry-run` to fail closed (non-zero) when a live run would refuse.
2. **Revisit accept-platform gating**: if real-world multi-file migrations show single-file sequential repair with the bypass flag is insufficiently ergonomic, necessitating an atomic multi-path batch verb (separately designed + ratified).

RATIFICATION: [x] APPROVE  [ ] REJECT

---

## 6. Ratification Summary

| Section / Item | Status |
|---|:---:|
| A — Slice 0 lexer fix | RATIFIED |
| B — S2 pre-write refusal + accept-platform extension + flag ×3 | RATIFIED |
| C — S3 doctor #28, O3 predicate | RATIFIED |
| D — S4 honesty tests + driftRecoverySuffix + docs | RATIFIED |
| E — bounds, pins, inventory policy, budget | RATIFIED |
| Decision 1 — 1B informational dry-run | RATIFIED |
| Decision 2 — O3 boundary predicate | RATIFIED |
| Implementation order & budget (1+2+2+2) | RATIFIED |
| Flip conditions | RATIFIED |

**Operator ratification:** "all as recommended" — 2026-11-13.
