# Decision: Compaction Ownership & Strategy Disposition (dcp-ownership / correct-context-for-next-step)

**Date:** 2026-10-18  
**Status:** Accepted (Operator Disposition). Slices 1–3 plugin-layer productionization SHELVED; owned-compaction feature engineering absorbed by the `native-engine` initiative (P5 compaction wire `c62436b` + spill/`spill_read` retrieval). Offline measurement branch `dcp-fixture` disposed (commit SHA `bc78aaa`). Studies adopted as durable documentation (`researches/sources/2026-09-15-correct-context-for-next-step.md` commit `647820f` + this self-contained decision memo).  
**Supersedes:** none.  
**See also:**
- [`./2026-07-23-dcp-ownership-layer.md`](./2026-07-23-dcp-ownership-layer.md) (prior baseline: P3 native compaction accepted, P1/P4 unactivated, P2 dormant)
- [`../sources/2026-09-15-correct-context-for-next-step.md`](../sources/2026-09-15-correct-context-for-next-step.md) (source packet: 7 technique families, literature survey, tier-ranked evidence)
- [`../../docs/ai/context-pruning-engine-behavioral-spec.md`](../../docs/ai/context-pruning-engine-behavioral-spec.md) (behavioral specification, 7 insufficiency signals, §8.2 truncation budgets)
- [`./native-engine-04-resolved-pain-points.md`](./native-engine-04-resolved-pain-points.md) (native-engine pain ledger P3/P5/P10)
- [`./native-engine-02-solution-brief.md`](./native-engine-02-solution-brief.md) (native-engine R2 session slice, surface-replace compaction)

---

## 1. Executive Summary & Operator Disposition

### 1.1 The Investigation Arc
Between 2026-09-10 and 2026-09-15, the harness project investigated the feasibility, architecture, and correctness of owning conversation compaction and context preservation across five distinct phases:

1. **External Failure-Mode Analysis (2026-09-10):** Field analysis (r/LocalLLaMA synthesis) identified "autophagy" in iterative conversation summarization—where recursive summarization feeds on its own past summaries, compounding drift, flattening semantic registers, and causing catastrophic obligation loss over multi-generation agent sessions.
2. **Solution-Brief & Strategy Selection:** A solution-brief evaluated four architectural stances: Strategy A (append-only conversation log with pre-prune gates), Strategy B (current baseline: prompt discipline + unanchored context injection), Strategy C (slugs and pointer/search retrieval), and Strategy H (evidence-bound recovery augmentation). Strategy H was recommended as the leading direction, contingent on rigorous offline empirical evidence.
3. **Boundary Research (Vendored OpenCode 1.18.5 vs. Environment DCP):** An audit of upstream OpenCode confirmed that host/environment-layer DCP (`compress` tool, `<dcp-prune-boundary>`) operates below the OpenCode runtime and is uninterceptable. However, OpenCode's native compaction surfaces—specifically the `experimental.session.compacting` context projection hook, summary prompt configuration, per-turn context injection, and recent-turn tail policy (`tail_turns`, preserve-recent budgets)—are fully ownable at the harness plugin layer.
4. **Offline Measurement Fixture (Commit `bc78aaa`, branch `dcp-fixture`):** A hermetic, deterministic measurement harness was constructed across 9 case families and 4 candidate strategies. The fixture proved that the current shipped baseline (Strategy B) silently drops or corrupts obligations in 8 out of 11 test rows, while Strategy H retains all recoverable anchors and introduces the only visible degradation channel at modest token overhead (1.1–1.65×).
5. **Literature Survey & Source Packet (Commit `647820f`):** A comprehensive survey across 7 technique families and 28 external citations (`researches/sources/2026-09-15-correct-context-for-next-step.md`) confirmed B's silent degradation, validated H's source-bound recovery model, and identified two novel contributions: the visible-failure compaction channel and controlled evidence of register mismatch.

### 1.2 Operator Disposition (2026-10-18)
On 2026-10-18, the operator reviewed the architectural findings, measurement fixture, and strategic tradeoffs, entering the following binding disposition:

1. **Dispose the `dcp-fixture` Branch:** The measurement branch `dcp-fixture` (containing test-only fixture code) is closed unmerged and deleted. The commit remains recoverable from the git reflog at SHA `bc78aaa`.
2. **Adopt Studies as Durable Documentation:** The research packet (`researches/sources/2026-09-15-correct-context-for-next-step.md`, committed on `main` at `647820f`) and this decision memo are adopted as durable engineering guidance. To ensure no institutional knowledge is lost when temporary run directories (`tmp/`) are purged, this memo is fully self-contained.
3. **`native-engine` Absorbs the Compaction Feature Work:** The planned plugin-layer productionization (the proposed 3-slice roadmap for OpenCode JavaScript plugins) is **SHELVED**. Implementing complex source-bound recovery and bi-temporal ledgers inside OpenCode JS plugins represents throwaway work given the active development of the Go-native engine. Instead, the compaction engineering is transferred directly to the `native-engine` initiative, specifically targeting its P5 compaction wire (`c62436b feat(native-engine): P5 compaction wire — turn-boundary trigger, adapter-backed KV-prefix summarizer`) paired with secondary spill/`spill_read` retrieval.
4. **Accepted Consumer Tradeoff:** Consuming repositories using the OpenCode harness will continue operating on Strategy B (inheriting its measured 8/11 silent-loss behavior under heavy multi-turn compaction) until the native engine ships. This tradeoff is accepted: plugin-layer mitigations cannot prevent uninterceptable environment-level pruning, and scarce engineering bandwidth is reserved for the native Go substrate.

---

## 2. Integration Boundary & Architectural Baseline

### 2.1 The Two Compaction Realities
A foundational finding of this investigation is the sharp boundary between upstream OpenCode and environment-level execution:

- **Environment-Layer DCP:** In certain hosted execution environments, an external pruning engine executes out-of-band via the `compress` tool and metadata tags (`<dcp-prune-boundary>`). This engine operates below the OpenCode runtime. It exposes no pre-prune hook, no candidate-summary review hook, and no transactional rollback. An exception thrown from a plugin hook crashes the prompt loop rather than providing a clean veto over compaction.
- **OpenCode Plugin Layer:** Upstream OpenCode (audited at vendored version 1.18.5, `refs/opencode`) exposes two compaction code paths:
  1. *V1 Session Compaction (`opencode/src/session/compaction.ts`):* Fully hookable via `experimental.session.compacting`. Plugins receive `{sessionID}` and can mutate `{context: string[], prompt?: string}`. It implements a head/tail split (`tail_turns` defaulting to 2, preserve-recent token budget 2k–8k). Setting `prompt` completely overrides the default template but causes `output.context` to be ignored. Tool pruning (`prune()`) marks tool parts as compacted and renders `"[Old tool result content cleared]"` while retaining raw storage bytes.
  2. *V2 Core Compaction (`core/src/session/compaction.ts`):* An anchored summary template (Objective, Important Details, Work State, Next Move, Relevant Files) with hardcoded auto-settings (20k buffer, 8k preserve, 4096 summary token cap). It publishes `Compaction.Started`/`Ended` events but provides zero plugin hooks.
- **Installed Harness Projection:** The harness's `.opencode/plugins/session-state.js` hooks `experimental.session.compacting` to invoke `buildCompactionContext(sessionID, todos)` from `state-lib.js`. It projects the session alias, active workstream, task contract (with `final_response_format` truncated to 20 lines), active plan excerpt (14 lines), session brief, latest checkpoint, recent decisions, open questions, recorded artifacts, and top-5 todos.

### 2.2 Definitions of Evaluated Strategies
Four architectural approaches were evaluated:

- **Control (Cold Original-Payload Baseline):** No compaction applied. All obligation-bearing artifacts (contracts, checkpoints, memory files, ground-truth observations) are retained verbatim in context. Represents the theoretical upper bound on fidelity and anchor retention.
- **Strategy B (Current Shipped Baseline):** Prompt discipline (five mandatory compaction sections in `AGENTS.md`) plus `buildCompactionContext` injection into `experimental.session.compacting`, accompanied by static doctor linting. Summary generation is unanchored prose generated with `tools: []` (no tool access).
- **Strategy H (Evidence-Bound Recovery Augmentation):** Retains essential obligations, security constraints, and next actions inline; formats the five-section summary as navigation; and binds all detail to retained exact evidence via explicit source digests and bi-temporal ledgers. Crucially, Strategy H introduces an explicit *visible-failure channel*: when a source document has mutated, drifted, or disappeared, H refuses silent substitution and emits a visible failure or warning.
- **Strategy C (Pointer/Search Dominant):** Strips inline context to minimal identity slugs (~100 tokens inline) and relies on on-demand retrieval (`spill_read` / pointer store) to fetch full documents when needed.

---

## 3. Measured Results: Offline Compaction Fixture

The evaluation was conducted using a dedicated, deterministic offline test harness running inside a hermetic node environment without live LLM calls. Token costs were estimated using normalized character counts (`Math.ceil(chars/4)`).

### 3.1 Comparative Results Table (9 Case Families × 4 Strategies)

| Case Family | Control | Strategy B (Current) | Strategy H (Proposed) | Strategy C (Pointer/Search) |
|---|---|---|---|---|
| **1. Single Compaction** | 9/9 anchors (575 tok) | 9/9 anchors (600 tok) | 9/9 anchors (869 tok @ 1.45×) | 9/9 anchors (105 inline + 463 retr) |
| **2. Accumulating Obligations** (2–3 compactions) | 5/5 anchors (1253 tok) | **4/5 anchors, 1 attribution err** (443 tok) | 5/5 anchors (674 tok @ 1.52×) | 5/5 anchors (102 inline + 401 retr) |
| **3. ≥3 Generations** (progressive drop) | 7/7 anchors (1234 tok) | **4/7 anchors, 3 attribution errs** (402 tok) | 7/7 anchors (634 tok @ 1.58×) | 7/7 anchors (98 inline + 381 retr) |
| **4. Premise 4-Tuple** (re-derivation demand) | 3/3 anchors, compliant (551 tok) | 2/3 anchors, **NON-compliant** (462 tok) | 2/3 anchors, compliant (688 tok + 24 rederiv, 1 call) | 3/3 anchors, **NON-compliant** (106 inline + 450 retr) |
| **5. Overwritten Contract v1** (pointer visibility) | 4/4 anchors (767 tok) | **2/4 anchors, silent substitution** (390 tok) | 2/4 anchors + **1 VISIBLE FAILURE** (557 tok) | 2/4 anchors, silent substitution (110 inline + 334 retr) |
| **6a. Missing Source: Contract Deleted** | 3/3 anchors (417 tok) | **1/3 anchors, healthy-looking** (204 tok) | 1/3 anchors + **1 VISIBLE FAILURE** (236 tok) | 1/3 anchors (105 inline + 258 retr) |
| **6b. Truncated Source: Checkpoint Cut Mid-File** | 2/2 anchors (433 tok) | **1/2 anchors, no signal** (375 tok) | 1/2 anchors + **1 VISIBLE FAILURE** (500 tok) | 1/2 anchors (106 inline + 252 retr) |
| **7. Attribution & Anti-Injection** | 11/11 anchors (615 tok) | **9/11 anchors, 2 attribution errs** (552 tok) | 11/11 anchors (838 tok @ 1.52×) | 11/11 anchors (106 inline + 481 retr) |
| **8a. Contradiction: Declared Supersession** | 4/4 anchors (473 tok) | 4/4 anchors (433 tok) | 4/4 anchors (594 tok) | 4/4 anchors (106 inline + 223 retr) |
| **8b. Contradiction: Silently Unresolved** | 2/2 anchors (440 tok) | 2/2 anchors, silent (391 tok) | 2/2 anchors + **1 VISIBLE FAILURE** (590 tok) | 2/2 anchors (105 inline + 179 retr) |
| **9. Oversized Final Response Format (FRF)** | 5/5 anchors (631 tok) | **3/5 anchors, silent drop** (587 tok) | 5/5 anchors (969 tok @ 1.65×) | 5/5 anchors (102 inline + 536 retr) |

### 3.2 Deep Analysis of Findings

1. **The 8/11 Silent Loss Baseline:** Strategy B silently loses critical anchors or violates compliance in **8 out of 11** test conditions (Cases 2, 3, 4, 5, 6a, 6b, 7, and 9). The failure modes are completely invisible to the resuming agent:
   - *Progressive Obligation Drop (Case 3):* In multi-generation summarization, B's projection re-summarizes its own previous context. Under a deterministic 3-item must-do budget, obligation `o4` is truncated at generation 1 and permanently shed from generation 2 onward, retaining only 4/7 anchors.
   - *Premise Re-derivation Failure (Case 4):* Strategy B fails to trigger required premise re-derivation, yielding a non-compliant first action. Strategy H incurs a single lightweight re-derivation call (24 tokens) to verify ground truth, maintaining compliance. Strategy C, despite full retrieval, fails to instruct the agent to re-derive before acting.
   - *Silent Mutation Substitution (Case 5):* When a contract is updated from v1 to v2, checkpoints pointing to v1-era state resolve silently to the v2 file. Strategies B and C present the v2 content under the v1 reference without warning. Strategy H checks the content digest, detects the mutation, refuses silent substitution, and emits an explicit visible failure.
   - *Silent Budget Truncation (Case 9):* Strategy B enforces a hard 20-line cap on `final_response_format`. Lines 1–20 are retained; lines 21+ are discarded without an ellipsis or warning marker, silently stripping output constraints.
2. **Attribution Errors & The Register Finding:** In Cases 2, 3, and 7, Strategy B logged attribution errors. Detailed inspection revealed that no strategy misattributed model-forged text into user turns (the harness's strict anti-injection rules prevent model text from entering user-labeled sections). Rather, **B's attribution errors were caused by budget truncation clipping genuine user-issued constraints**, causing the model to paraphrase remaining items into narrative recap. This provided empirical proof that *budget truncation directly induces register distortion*.
3. **The Limits of Strategy H (Honesty Ceiling):** Strategy H does not magically resurrect lost bytes. When an original contract file is completely deleted (Case 6a) or truncated on disk (Case 6b), H retains only 1/3 and 1/2 anchors—identical to B. However, while B presents the depleted state as complete and healthy, H emits **VISIBLE FAILURES** (`[SOURCE DIGEST MISMATCH]` and `[PREMISE SOURCE ABSENT]`), alerting the model to state corruption.
4. **Token Economics:** Strategy H costs between 1.10× and 1.65× Strategy B's inline token footprint (mean ~1.5×). Strategy C appears cheap inline (~100 tokens) but incurs 179–536 retrieval tokens per turn, routinely exceeding H's total footprint on complex multi-turn evaluations while lacking any degradation signal.

---

## 4. Methodology & Fixture Recoverability

### 4.1 Fixture Architecture
The offline measurement fixture was constructed to ensure 100% reproducible, deterministic results without external API spend:

- **Component Structure:**
  - `tests/integration/compaction_fixture_test.go`: Go integration test driving the suite via `vh-agent-harness exec go test ./tests/integration -run TestCompactionFixture -v`.
  - `tests/integration/testdata/compaction-fixture/evaluate.js`: Pure ES-module JavaScript engine that sets up synthetic session states and executes the 9 case families.
  - `tests/integration/testdata/compaction-fixture/package.json`: Configures `"type": "module"` for Node 18+ ESM compatibility.
- **Hermetic Isolation:** State was completely decoupled from real developer sessions by redirecting all storage roots to isolated temporary test directories using built-in environment hooks in `state-lib.js`:
  - `OPENCODE_STATE_ROOT`
  - `OPENCODE_RUN_ROOT`
  - `OPENCODE_LOCAL_COORDINATOR_ROOT`
  - `OPENCODE_CLEARED_ASSUMPTIONS_PATH`
  - `OPENCODE_CWD`
- **Execution Methodology:** State was fabricated using real `state-lib.js` persistence primitives (`saveTaskContract`, `saveCheckpoint`, `appendDecision`, `writeSessionMemoryFile`). Contexts were generated by calling the actual `buildCompactionContext` function. A red control test verified that invalid projections and broken anchors failed assertions deterministically. Dual independent runs confirmed byte-identical metrics across all 36 evaluation cells (9 cases × 4 strategies).

### 4.2 Code Recoverability
The `dcp-fixture` branch was disposed unmerged per operator instruction. However, the complete fixture implementation is preserved in the git object database at commit:

```
Commit SHA: bc78aaa
Parent SHA: bb37624
Author/Date: 2026-09-14
Tree: tests/integration/compaction_fixture_test.go
      tests/integration/testdata/compaction-fixture/evaluate.js
      tests/integration/testdata/compaction-fixture/package.json
```

In the short term, this commit can be inspected directly via `git show bc78aaa` or recovered from the local reflog (`git checkout -b dcp-fixture bc78aaa`). Over the long term, the complete methodology, metrics, and behavioral contracts are preserved in this memo and in `researches/sources/2026-09-15-correct-context-for-next-step.md`.

---

## 5. Strategy H Contract Specification for `native-engine`

This section is the primary durable engineering specification resulting from this research. The planned OpenCode plugin implementation is shelved; instead, the `native-engine` initiative (specifically the P5 compaction wire at `c62436b` and subsequent session slices) must natively satisfy the following eight requirements:

### 5.1 Source-Bound References
Compaction projections must not treat references as mere string labels. Every reference to a past artifact (task contract, plan, checkpoint, source document) must bind to a content digest (SHA-256) and sequence locator. On compaction and session resume, the engine must resolve references against retained exact log events. If an underlying artifact has mutated, the engine must **never silently substitute** the latest content; it must either resolve to the historically bound version from the event log or raise a visible resolution failure.

### 5.2 Visible-Failure Channel (WARN-First with Critical Refusal)
The engine must establish an explicit degradation reporting channel inside the model context window:
- **WARN-First Reporting:** When non-critical artifacts (ancillary notes, secondary decisions, closed todos) are missing, truncated, or unresolvable, the engine injects a visible warning block (e.g., `[COMPACTION WARNING: Source checkpoint-002 truncated on disk; proceeding with partial context]`).
- **Hard Refusal Gates:** The engine must refuse prompt execution and halt the turn only when an unresolvable failure occurs on a **provenance-critical** surface:
  1. *Security Constraints & Rules:* Verbatim instructions governing tool permissions, execution sandboxes, or user safety fences.
  2. *Owned-File Boundaries:* Active file-scope leases and path ownership fences.
  3. *Final Response Format (FRF):* User-specified response schemas or closeout checklists.

### 5.3 Bi-Temporal Supersession Fields
To prevent stale premises from surviving across compactions, all premise assertions and mutable facts must be tracked using bi-temporal metadata:
- `observed_at`: Monotonic turn counter or timestamp when the fact was verified.
- `valid_until` / `superseded_by`: Identifier of the subsequent event or observation that invalidated or updated the fact.
- `re_derivation_command`: A deterministic, read-only command to re-verify the premise against live disk state prior to executing the next action.

### 5.4 REGISTER Discipline
To prevent the progressive distortion identified in Case 7 and the literature, the engine must enforce register separation:
- User-issued obligations, constraints, and instructions must be carried **verbatim** in a dedicated, user-labeled register (`user_obligations`).
- The engine must strictly forbid summarizer models from paraphrasing, abstracting, or flattening user instructions into assistant narrative recaps.
- Model-generated summaries, reflections, and tool outputs must remain strictly confined to their respective system, memory, or assistant registers.

### 5.5 Standardized Retention Taxonomy
Rather than inventing an ad-hoc categorization of state, the engine adopts LangGraph's three-part cognitive retention taxonomy:
1. **Semantic Memory:** Durable facts, architecture invariants, entity definitions, and codebase operational primitives (infrequently changed, high salience).
2. **Episodic Memory:** Chronological narrative of actions taken, test outputs, past turns, and tool receipts (subject to progressive aging, summarization, and tail eviction).
3. **Procedural Memory:** Active task contracts, required next actions, open todos, and verification criteria (strictly retained verbatim until terminal disposition).

### 5.6 Visible Truncation Markers
Deterministic slot budgets (e.g., maximum lines allocated to response formats or plan excerpts) must never silently truncate content. If a budget forces eviction:
- The engine must append an explicit marker: `[TRUNCATED: lines 21-45 omitted due to context budget; run 'spill_read <ref>' to view full block]`.
- The omitted content must be spilled to disk storage and registered with the `spill_read` tool.

### 5.7 Byte-Stable Obligation Head & Volatile Tail
To maximize provider KV-cache hit rates (which account for up to a 10× cost differential in long sessions):
- The head of the context window (system prompt, core security rules, invariant harness definitions, and active procedural obligations) must remain strictly **byte-stable** across turns.
- All dynamic state churn, tool results, conversational turns, and episodic summaries must be confined to the volatile tail.

### 5.8 Per-Context Tail Policy
The compaction engine must support distinct tail policies tailored to the active run-shape and session type:
- **Interactive Coding Sessions:** Maintain a raw tail of 2–4 recent turns (including recent tool results) to preserve immediate tactile context.
- **Autonomous Batch / Subagent Sessions:** Use a minimal 1-turn tail, relying on structured procedural records to maximize token availability for generation.

### 5.9 Literature Grounding & Novelty Claims
The design of Strategy H is grounded in the literature survey (`researches/sources/2026-09-15-correct-context-for-next-step.md`), incorporating findings from Liu et al. (TACL 2024, primacy/recency U-curve), Anthropic (Context Engineering 2025, recall-over-precision selection), Manus (2025, restorable context and KV-cache stability), and Zep/Graphiti (bi-temporal invalidation).

In addition, this investigation advances two **novel contributions** not found in published literature:
1. *The Visible-Failure Compaction Channel:* Prior academic and commercial compaction engines (MemGPT, OpenCode, Claude Code, Letta) operate on a "best-effort survival" model, where unrecoverable context is silently dropped. Strategy H is the first to introduce a formalized, visible degradation and refusal channel at the compaction boundary.
2. *Controlled Register-Mismatch Evidence:* While prior work observed general multi-turn performance degradation (Laban et al., 2025), our fixture isolated deterministic evidence that budget truncations of verbatim user constraints directly trigger model-side register confusion and paraphrasing errors.

---

## 6. Open Items & Defect Ledger

During the construction and evaluation of the fixture, two latent defects in the harness persistence layer were discovered. Both are carded in `.local/coordinator/tasks/` and will be resolved when the native engine absorbs session management:

### 6.1 Defect 1: Deleted-Contract Self-Heal to Empty v0 (`ensureSessionMemoryNamespace`)
- **Location:** `templates/core/.opencode/scripts/state-lib.js:7896-7925` (and rendered equivalents).
- **Mechanism:** In Case 6a, when the task contract files (`task-contract.md` and `task-contract.json`) were deleted to simulate data loss, `buildCompactionContext` did not throw or report missing files. Instead, it invoked `ensureSessionMemoryNamespace`, whose seed logic automatically generated a brand-new, completely empty version 0 (`v0`) default contract on disk.
- **Impact:** The projection engine proceeded to build context from this empty stub, creating a structurally valid, "healthy-looking" context that was semantically gutted.
- **Disposition:** Carded draft task in `.local/coordinator/tasks/`. The native engine's session store will distinguish missing state from initial empty state, refusing to auto-seed default contracts during read/compaction operations.

### 6.2 Defect 2: Contract Fixed-Path Overwrite
- **Location:** `templates/core/.opencode/scripts/state-lib.js:3278-3320` (`saveTaskContract`).
- **Mechanism:** Updating a task contract writes to fixed paths (`sessionTaskContractPath` Markdown and JSON twins) via atomic file writes, incrementing `version = existing.version + 1`. Prior version bytes are immediately overwritten on disk. Because `.opencode/state/` is gitignored, previous versions are permanently destroyed.
- **Impact:** Checkpoints that record `task_contract_version: 1` point to a file that now contains version 2 bytes, inducing the silent substitution measured in Case 5.
- **Disposition:** Carded draft task in `.local/coordinator/tasks/`. The native engine will store all contract mutations as immutable, append-only events in its session event log, allowing historical checkpoints to dereference exact historical versions.

### 6.3 Open Evaluation Gap: Model-Side Fidelity
The measurement fixture operated strictly offline over projected context strings, evaluating anchor presence, token footprint, and deterministic compliance. It did not evaluate live model continuation (the semantic quality of model responses when resuming from B vs. H projections). A live cold-replay benchmark with model invocations remains an open research item, to be executed against the native engine once its P5 compaction wire is operational.

---

## 7. Verification & Self-Containment Confirmation

- **Self-Containment Audit:** All findings, case families, numerical results, and architectural specifications originating in temporary workspaces (`worktrees/dcp-fixture/tmp/...` and `tmp/agent-runs/...`) have been fully transcribed and synthesized into this document and `researches/sources/2026-09-15-correct-context-for-next-step.md`. No claim in this memo relies on dangling temporary files.
- **Repository Safety:** In strict adherence to the task instructions, no git commits, stage operations, or branch mutations were executed during this documentation pass. All changes are confined exclusively to this single file.
