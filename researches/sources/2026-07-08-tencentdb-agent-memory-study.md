# Sources: TencentDB-Agent-Memory (TencentCloud) — Reference Study for vh-agent-harness's typed agent-memory layer

**Date:** 2026-07-08
**Topic:** How TencentDB-Agent-Memory (TencentCloud's open-source memory system for
AI agents) implements tiered agent memory, durable storage, and prompt
injection, and what vh-agent-harness adopted from it.
**Kind:** Source/option packet. NOT active repo guidance — a reference study plus
adoption outcome. **This is the durable, post-hoc capture of a study whose raw
research was conducted in chat and whose recommended code (R1–R3, R5) has since
been DELIVERED as committed code (slices 1–5).** It records both the original
findings and the delivery outcome against the current repo HEAD.
**Studied against (our side):** `vh-agent-harness` repo. The cognitive layer
landed in commits `21ae11f` ("feat(memory): add typed record DTO and append-only
JSONL store"), `f37a374c` ("feat(memory): add write-safety to append-only JSONL
store"), `a5d3a99` ("docs(memory): document R3 budgeted-memory-injection
discipline"), and `f36a7f8` ("feat(cli): add diagnostics-export subcommand"). At
memo-write time HEAD is `f36a7f8`, which includes all four. No `exec-sandbox`
dependency; this memo concerns the memory + diagnostics surfaces only.

---

## Research question & scope

- **Question:** What memory-system patterns from TencentDB-Agent-Memory are
  worth adopting in vh-agent-harness, given we are a single static Go binary on
  host-shell, OpenCode-first, with no vector store, and a flat
  session/workstream memory model?
- **Scope:** TencentDB-Agent-Memory v0.3.6 (a TypeScript OpenClaw plugin plus a
  Python Hermes adapter), read from a shallow local clone at
  `refs/TencentDB-Agent-Memory/`. **Architectural patterns only** — no code is
  importable (TypeScript/Node + Python vs. our Go), so every recommendation is a
  *pattern transplant*, not a vendoring. Tooling boundary, runtime lifecycle,
  and benchmark claims are explicitly out of scope (see Rejections).
- **Time-sensitivity:** STABLE. v0.3.6 is a versioned plugin; the patterns of
  interest (tiered record DTO, append-only durability spine, prompt-cache-aware
  injection, field-aware redaction) are architectural, not version-fragile.
- **Source policy:** PRIMARY = the local shallow clone read directly via file
  tools. All `file:line` refs are under `refs/TencentDB-Agent-Memory/`
  **at study time (clone since deleted — see Task 2 / Closeout)**. No web sources
  were used; no benchmark was reproduced. The clone was gitignored (committed at
  `ad1e5fa`) and is no longer present, so these refs cannot be re-walked in tree
  — treat them as a captured snapshot of v0.3.6.

## Confidence legend

- **HIGH** — verified directly against the clone (file:line), mechanism
  unambiguous, and (for delivered items) re-confirmed against the landed Go code
  at HEAD `f36a7f8`.
- **MED** — single-source structural inference, or a behavioral claim derived
  from reading one file rather than a re-read of the whole call path.
- **LOW** — anecdotal / vendor self-report only; directionally useless for
  adoption and explicitly rejected (see Rejections: benchmark numbers).

## How citations are formatted

Two citation flavors, kept distinct:

1. **Source (theirs)** — `refs/TencentDB-Agent-Memory/<path>:<line-range>`,
   captured from the v0.3.6 clone. Because the clone has been deleted, these are
   a frozen snapshot, not re-walkable in tree. Each is phrased as "under
   `refs/TencentDB-Agent-Memory/...` at study time (clone since deleted)".
2. **Delivery (ours)** — the vh-agent-harness path/commit that implements (or,
   for R4, would implement) the recommendation, verified at HEAD `f36a7f8`.

A confidence tag follows each. Where a source line range was the basis for a Go
transplant, the Go path is named so the lineage is traceable end-to-end.

---

## Findings — the five recommendations (R1–R5)

> Each recommendation states the finding, the harness **layer(s)** it touches
> (1 Prescriptive / 2 Cognitive / 3 Coordination / 4 Safety / 5 Capability /
> 6 Environment), the disposition (Adopt / Inspired-by / Defer), the source
> refs, and — because this memo is retrospective — the **delivery outcome**
> (commit SHA + slice, or "deferred").

### R1 — Typed tiered memory-record DTO + append-only JSONL source-of-truth
**(Layers 2 Cognitive; Adopt → DELIVERED at `21ae11f`, slice 1)**

> **(finding)**: TencentDB-Agent-Memory models memory as typed, tiered records
> persisted append-only to JSONL, with an index derived (rebuildable) on top
> rather than mutated in place — source=clone, confidence=HIGH, type=architecture

**Source (theirs),** under `refs/TencentDB-Agent-Memory/` at study time (clone
since deleted):
- `src/core/record/l1-writer.ts:32-74` — the `MemoryRecord` DTO: minimal shape
  with `type ∈ persona|episodic|instruction`, `priority ∈
  low|normal|high|critical`, plus `scene?`, `workstream?`, `source_ref?`,
  `session_key?`, timestamps, and a `body`. The tiering (type × priority) is the
  load-bearing idea; the rest is provenance.
- `src/core/record/l1-writer.ts:194-264` — JSONL append path with a dual-write
  (raw record + derived index). Append-only is the source of truth; the index is
  rebuildable from the log.
- `src/core/store/types.ts:1-16` — fault-tolerant store contract: a malformed
  record must not abort the read; missing store ⇒ empty (not error).

**Delivery (ours),** at HEAD `f36a7f8`:
- `internal/memory/record/` (`record.go`, `record_test.go`) — pure DTO. The
  record type imports only `time`/`encoding/json`/`fmt`; no network, no DB, no
  framework. Shape: `{type, priority, scope ∈ session|workstream, scene?,
  workstream?, source_ref?, session_key?, created_at, updated_at, body}` — the
  TencentDB tiering with a `scope` axis added for our session/workstream split.
- `internal/memory/store/` (`store.go`, `store_test.go`) — append-only JSONL I/O
  at `.opencode/state/sessions/<alias>/memory/records.jsonl` and
  `.opencode/state/workstreams/<slug>/records.jsonl`. Bounded linear reader:
  filter → dedup-by-id (last-write-wins) → sort → `priority` desc, then
  `updated_at` desc → cap at `MaxRecords`. Fault-tolerant exactly as in their
  store contract: malformed lines skipped + counted, missing file ⇒ empty.

### R2 — Split-state durability: per-owner partition, flock + fsync, atomic order
**(Layers 2+4 Cognitive+Safety; Adopt as pattern → DELIVERED at `f37a374c`, slice 2)**

> **(finding)**: Durability is achieved by partitioning durable state by owner
> (no two-writer clobber), keeping cursors per-session (not global), and writing
> atomically with flock + fsync on both the data file and the parent dir on
> create — source=clone, confidence=HIGH, type=persistence

**Source (theirs),** under `refs/TencentDB-Agent-Memory/` at study time (clone
since deleted):
- `src/utils/checkpoint.ts:4-24,40-109` — checkpoint split-state model; owner
  partitioning prevents two concurrent writers from clobbering each other's
  durable state.
- `src/utils/checkpoint.ts:42-47,156-179` — per-session cursors, not a global
  cursor; concurrent sessions advance independently.
- `src/utils/checkpoint.ts:204-238,459-486` — atomic ordering: temp file + rename
  to target; `flock` for mutual exclusion; `fsync` of the data file **and** the
  parent directory on initial create (the dir-fsync is the crash-durability
  guarantee that a renamed/created entry actually survives).

**Delivery (ours),** at HEAD `f36a7f8`:
- Write-safety hardening on `internal/memory/store/store.go`'s append path:
  `golang.org/x/sys/unix.Flock` (`LOCK_EX`) for cross-process mutual exclusion;
  `unix.Fsync` on the data file after each append; `fsyncDir` on first create
  (gated on a pre-write `stat`, so it runs once per store lifetime, not per
  append).
- In-process serialization via a package-level per-path `sync.Mutex` map (the
  flock handles cross-process; the mutex handles concurrent goroutines within one
  process).
- `fsyncDir` is exposed as a swappable package-level `var` (`var fsyncDir = …`)
  so the durability call can be stubbed in regression tests without a real FS.
- **What we did NOT take:** TencentDB's temp-file + rename. Our store is
  in-place append only (no rewrite/compaction pass exists yet), so the
  rename-atomicity is unnecessary; we kept just flock + fsync(data) + fsyncDir.
  If a rewrite/compaction step is ever added, revisit temp+rename at that point.

### R3 — Prompt-cache-aware, timeout-bounded, budgeted memory injection
**(Layers 2+4+6 Cognitive+Safety+Environment; Adopt → DELIVERED at `a5d3a99`, slice 3)**

> **(finding)**: Memory injection that wants to stay cheap splits stable records
> into the cacheable SYSTEM prompt and dynamic records into the per-turn USER
> turn, hard-skips on timeout (never blocks the agent), truncates to a char
> budget at code-point granularity, never bisects a tool_use/tool_result pair,
> and tags each injected atom with provenance — source=clone, confidence=HIGH,
> type=architecture

**Source (theirs),** under `refs/TencentDB-Agent-Memory/` at study time (clone
since deleted):
- `src/core/hooks/auto-recall.ts:83-100,186-218` — stable vs. dynamic split:
  persona/instruction (stable) → SYSTEM (cacheable); episodic/recent (dynamic)
  → USER (per-turn, cache-busting by design).
- `src/core/hooks/auto-recall.ts:708-789` — hard timeout on the recall path:
  on timeout it **skips** injection for that turn rather than blocking the agent;
  char budget with code-point-granular truncation; never bisects a
  tool_use/tool_result pair; provenance via `source_ref` on each atom;
  fault-tolerant reads (a corrupt record cannot poison the prompt).
- `src/offload/mmd-injector.ts:200-282` — the offload/context-injection variant
  of the same discipline (budget + provenance + skip-on-failure).

**Delivery (ours),** at HEAD `f36a7f8`:
- Delivered as a **terse, domain-free discipline section** — `## Typed records
  and budgeted injection` (~39 lines) inside
  `templates/docs/opencode-memory-model.md` (the embedded generic doc served by
  the `vh-agent-harness docs opencode-memory-model` verb). It captures the
  stable→SYSTEM / dynamic→USER split, the hard-skip-on-timeout rule, the char
  budget + code-point truncation, the never-bisect-tool-use rule, and the
  provenance-via-source_ref contract.
- **No code injection helper was built.** This was a deliberate scope call: we
  are OpenCode-first and inject via explicit slash-command/session seams, NOT via
  runtime lifecycle hooks. R3 is therefore documented as **discipline for any
  future consumer** of the typed records, which reaffirms the harness's existing
  Anti-spam capstone (explicit-invocation seam) against TencentDB's runtime-hook
  model (their `agent_end` / `before_prompt_build` hooks — see Rejections).

### R4 — Recover-on-restart coordination-runtime discipline
**(Layers 3+4 Coordination+Safety; Inspired-by → DEFERRED, no slice yet)**

> **(finding)**: A coordination runtime that survives restart does so by serial
> per-lane queues, always-persist-on-shutdown, an explicit
> recoverPendingSessions() on start, distinct flushSession() (per-session) vs
> destroy() (whole-process) teardown, bounded retry with a restore-buffer, and
> cold-session GC — source=clone, confidence=MED (single-module structural read,
> not a full runtime re-walk), type=scheduling

**Source (theirs),** under `refs/TencentDB-Agent-Memory/` at study time (clone
since deleted):
- `src/utils/pipeline-manager.ts:440-499,502-553` — serial per-lane queue model
  + always-persist-on-shutdown.
- `src/utils/pipeline-manager.ts:703-732` — `recoverPendingSessions()` on start;
  a restarted process rehydrates in-flight lanes from durable state.
- `src/utils/pipeline-manager.ts:1059-1103,1117-1133` — **the bug lesson:** their
  code once conflated `flushSession()` (one session) with `destroy()` (whole
  process); calling the latter where the former was meant wiped *all* concurrent
  sessions. The lesson is to keep these two teardown primitives lexically and
  semantically distinct. Also bounded-retry with a restore-buffer and
  cold-session GC.

**Delivery (ours):** **DEFERRED.** No coordinator background runtime exists yet
under `.local/coordinator/`. This recommendation is to be encoded as **doctrine
when `.local/coordinator/` background work is actually built** — specifically the
serial-per-lane, persist-on-shutdown, recover-on-start, flush-vs-destroy split,
and bounded-retry+restore-buffer rules. Not actionable today.

### R5 — Field-aware redacting diagnostics-bundle export
**(Layers 4+5 Safety+Capability; Adopt → DELIVERED at `f36a7f8`, slice 5)**

> **(finding)**: A safe diagnostics export redacts in three field-aware layers —
> (1) field-name substring match against a secret-lexicon, with the value
  replaced by a length-honoring placeholder; (2) whole-section redaction of
  known-secret sections; (3) value-pattern redaction of bearer tokens / cloud
  keys / connection strings — and must never auto-upload — source=clone,
  confidence=HIGH, type=safety

**Source (theirs),** under `refs/TencentDB-Agent-Memory/` at study time (clone
since deleted):
- `SKILL-DIAGNOSTIC-EXPORT.md:103-113` — the field-aware redaction rules:
  substring match on field names → placeholder preserving the original length;
  sensitive key with a map/array value → entire subtree redacted.
- `scripts/export-diagnostic.sh` — the export driver; the bundle is written
  locally and never auto-uploaded.

**Delivery (ours),** at HEAD `f36a7f8`:
- `vh-agent-harness diagnostics-export [--dry-run] [--output <path>]` — a new
  verb: `internal/cli/diagnostics.go` (measured 988 lines at HEAD) +
  `internal/cli/diagnostics_test.go` (measured 952 lines at HEAD, **28 `Test*`
  functions** — matches the brief).
- 3-layer field-aware redaction:
  1. **Field-name substring** match against `[apikey token secret password
     passwd credential auth bearer privatekey accesskey clientsecret]` →
     `***REDACTED(Nchars)***` (rune count, not byte count). Sensitive key with a
     map/array value ⇒ the entire subtree is redacted, not just the scalar.
  2. **Whole-section** redaction of `secrets` / `env` / `environment` /
     `credentials` / `models-at-root`.
  3. **Value-pattern** redaction of `Bearer …` / `AKIA…` cloud keys / connection
     strings.
- Output-path hardening against symlink escape: write to a temp file then
  `os.Rename` (rename does not follow the destination symlink), so a symlinked
  `--output` cannot redirect the bundle out of the intended location.
- **Never auto-uploads** — operator must move the bundle themselves.
- Domain-free skill at `templates/core/.opencode/skills/diagnostics-export/SKILL.md`
  so it ships into adopting projects unchanged.

---

## Rejections — patterns NOT adopted (do not re-propose)

These were considered and explicitly rejected. Listing them so a future pass does
not re-litigate:

- **Hardcoded home-dir data paths** (their `~/.openclaw/` data root). Violates
  our **repo-relative-paths** rule (AGENTS.md → "Shell, container, and workspace
  hygiene"); a TencentDB-style `$HOME`-rooted store is a non-starter here.
- **Node sidecar + Python supervisor architecture.** Opposite of our **single
  static Go binary, host-shell backend** contract. The Hermes adapter's
  `supervisor.py` spawning a Node plugin is exactly the runtime shape we refuse.
- **Vector / SQLite store as the baseline.** We have no vector store and do not
  want one; keyword/grep over flat JSONL satisfies our scale (per-session, human-
  scoped). Their `src/core/store/sqlite*` + TencentDB-Vector-DB path is not for us.
- **Mermaid context-offload subsystem.** Heavyweight, LLM-driven, niche; the
  `src/offload/mmd-injector.ts` offload path is more machinery than our use case
  justifies. We took only its *budget + provenance + skip-on-failure* discipline
  (folded into R3) and dropped the rest.
- **Their benchmark numbers** (`-61.38%` tokens / `+51.52%` pass rate /
  `SWE-bench +9.93%`). Unreproducible vendor self-report; **do not cite as
  evidence** anywhere in this repo. Confidence LOW; rejected.
- **Letting memory-atom granularity turn our compaction into an LLM-extraction
  pipeline.** Their extractor (`src/core/record/l1-extractor.ts`) LLM-splits
  conversation into atoms; our compaction stays an explicit `compress` op, not an
  extraction model.
- **Pattern-matching their runtime lifecycle hook names** (`agent_end`,
  `before_prompt_build`) onto our slash-command/session seams. Our seams are
  explicit-invocation; mapping their hook vocabulary onto us would smuggle in the
  runtime-hook model we rejected in R3.

---

## Source packet — key paths under `refs/TencentDB-Agent-Memory/` (v0.3.6, clone since deleted)

Captured here so the deleted clone's layout is recoverable from this memo alone.
All paths are under `refs/TencentDB-Agent-Memory/` at study time (clone since
deleted, gitignored at `ad1e5fa`).

- **Orientation:** `README.md`, `package.json`, `openclaw.plugin.json`,
  `SKILL.md`, `SKILL-MIGRATION.md`, `SKILL-DIAGNOSTIC-EXPORT.md`, `index.ts`,
  `LICENSE` (MIT).
- **Core facade + types:** `src/core/tdai-core.ts` (facade), `src/core/types.ts`
  (`RuntimeContext`, `HostAdapter`, `LLMRunnerFactory`).
- **Record layer (highest relevance for R1):** `src/core/record/l1-writer.ts`
  (`MemoryRecord` DTO + dual-write), `src/core/record/{l1-extractor,l1-dedup,
  l1-reader}.ts`.
- **Memory surfaces:** `src/core/scene/`, `src/core/persona/`,
  `src/core/hooks/{auto-capture,auto-recall}.ts`, `src/core/tools/{memory-search,
  conversation-search}.ts`.
- **Store layer:** `src/core/store/{types,sqlite,factory,…}.ts`.
- **Durability spine (highest relevance for R2/R4):** `src/utils/checkpoint.ts`,
  `src/utils/pipeline-manager.ts`, `src/utils/{serial-queue,managed-timer,…}.ts`.
- **Adapters / gateway:** `src/adapters/{openclaw,standalone}/`,
  `src/gateway/server.ts` (`:8420`).
- **Hermes (Python):** `hermes-plugin/memory/memory_tencentdb/{plugin.yaml,
  supervisor.py,client.py,__init__.py}`.
- **CLIs / scripts:** `bin/{read-local-memory,export-tencent-vdb,
  migrate-sqlite-to-tcvdb}.mjs`, `scripts/` (incl. `export-diagnostic.sh` for R5).

---

## Closeout

- **Time-sensitivity:** STABLE. v0.3.6 is a versioned plugin; the adopted
  patterns are architectural.
- **Confidence:** HIGH on architecture, data model, persistence, scheduling, and
  redaction (all read directly from the clone; the four delivered items were
  re-confirmed against the landed Go at HEAD `f36a7f8`). LOW on benchmark
  numbers, and those were rejected outright (see Rejections).
- **Disposition tally:** all five recommendations disposed — **4 delivered**
  (R1 @ `21ae11f`, R2 @ `f37a374c`, R3 @ `a5d3a99`, R5 @ `f36a7f8`) and **1
  deferred** (R4, awaiting a `.local/coordinator/` background runtime).
- **Clone deletion:** the local clone at `refs/TencentDB-Agent-Memory` was
  deleted after the study was captured into this memo. It was gitignored
  (committed at `ad1e5fa`), so deletion is a filesystem op only — **no git
  change, no commit**. The `file:line` refs above are a frozen snapshot of
  v0.3.6 and cannot be re-walked in tree.
- **Lineage note:** vh-agent-harness originally forked from TrueAI; the operator
  later reversed the relationship (TrueAI migrated to vh-agent-harness), so
  vh-agent-harness is now upstream. The **flat session/workstream memory model**
  was ported from TrueAI; the **typed-memory layer** (this study's R1–R3 work) is
  NEW and TencentDB-inspired, not inherited.
- **Promotion status:** this packet is a SOURCE study + adoption record, NOT
  active guidance. No `docs/ai/` promotion is warranted — the discipline that
  needed to persist (R3) is already embedded in the generic
  `opencode-memory-model` doc that the `docs` verb serves.
