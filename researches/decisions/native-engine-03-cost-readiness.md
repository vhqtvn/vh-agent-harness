<!-- Durable home: researches/decisions/native-engine-03-cost-readiness.md (landed 2026-08-20). -->

# 03 — Cost/Time Model & Design-Readiness Audit
**Date**: 2026-08-19
**Scope**: Headless standalone Go agent engine (O1-headless via O2 ladder).

## 1. Cost & Time Model

**Assumptions**:
- **Operator capacity**: 1 operator, assumed 10–15 focused hours/week.
- **Workflow**: Agent-assisted development.
- **Execution**: Serial rungs (R0 → R4).
- **Skills**: High Go proficiency; no prior event-log codebase to reuse in Go.
- **Currency**: Omitted (no hourly rate or opportunity cost provided). Estimates in engineer-weeks (1 eng-week ≈ 10-15 agent-assisted hours).

**Estimates by Rung**:
- **R0 (Harvest/Protocol Spec)**: **1–2 weeks**. Basis: pure schema/interface design for 5 core domains (host protocol, A2A, package layout).
- **R1 + S1 (Kernel Spike)**: **2–3 weeks**. Basis: dsh LLM (116 files) + session (146 files) TS proxies translating to a minimal Go adapter registry (OpenAI/Anthropic) and a naive JSONL event log.
- **R2 (Session Slice)**: **3–4 weeks**. Basis: porting surface-replacement compaction and three-tier relief to Go requires non-trivial data structure design; dsh TS reference is complex (14 subpackages).
- **R3 + S3' (Host Protocol & External Client)**: **2–3 weeks**. Basis: wiring the NDJSON/stdio host contract and building a minimal external client (vh-solara prototype) state machine.
- **R4 + S2 (Cutover)**: **1–2 weeks**. Basis: wiring existing `exec-sandbox` (already implementation-only) into the new pipeline and defining/running parity checks.
- **R5 (Expansion)**: Excluded (optional/deferred).

**Total Program Estimate (R0–R4)**: **9–14 calendar weeks** (at assumed cadence).
**R0-Only Checkpoint**: **1–2 weeks**. (Operator can buy structural clarity cheaply before committing to implementation).

## 2. Design-Readiness Ledger

### IMPLEMENTATION-ONLY (Design Settled)
- **Sandbox / Exec Integration**: Existing `exec`, `exec-ro`, `exec-sandbox`, `shell` verbs. (Cite: v3 Architecture Map, existing vh codebase).
- **State / Coordination**: Local `.coordinator/` task cards, commit-gate scripts. (Cite: E11, existing vh codebase).
- **Doctor / Parity checks**: The comparison and drift-check framework exists. (Cite: E2, existing vh codebase).

### PARTIALLY-DESIGNED (Principles exist, Go design missing)
- **Event-log schema & surface-replace compaction**: dsh patterns (D1-D3) exist; concrete Go structs and index/fold logic unwritten.
- **LLM Adapter registry**: dsh patterns (D4-D5) exist. Concrete Go interfaces unwritten. *Risk flag*: Transferability of dsh KV-prefix-preserving patterns to Anthropic prompt-cache semantics must be mapped.
- **Tool waterfall interfaces**: Deny-only guard lattice (D7-D9) principle decided; Go middleware chain unwritten.
- **Subagent activations**: Mode-discriminated descriptors (D10) mapped in TS, Go state models unwritten.
- **Extension model / Package layout**: telegraf-style compile-time inclusion decided; aggregator wiring and `internal/` layout unwritten.

### UNSOLVED (Genuinely open design questions)
- **Host protocol spec**: Methods, event stream, approval model, and async dispatch contract over stdio/NDJSON are completely undefined.
- **Async job recovery semantics (R9)**: Behavior for a crash mid-job is explicitly undefined.
- **A2A / Session-bus generalization (R10)**: Event vocabulary for addressed messages (internal seam) is unwritten.
- **Cutover evidence definition**: STILL OPEN in v3 solution brief.
- **S2 OpenCode interception feasibility**: Unverified spike.

## 3. Readiness Verdict

**Are we having all solutions and only implementing remaining?**
**NO.**

**Verdict Breakdown**:
- ~25% Implementation-Only (mostly carry-over safety assets).
- ~40% Partially-Designed (requires translating TS patterns to Go).
- ~35% Unsolved (core structural protocols).

**Shortest list of designs that must exist before implementation-only status is true (R0 deliverables)**:
1. Host protocol spec (async contract, approval model, event stream vocabulary).
2. Async job recovery semantics (crash mid-job settlement).
3. A2A / Session-bus event vocabulary for internal routing.
4. Go schemas for event-log compaction and adapter KV-caching (Anthropic vs OpenAI).
5. Cutover evidence definition & S2 interception probe.