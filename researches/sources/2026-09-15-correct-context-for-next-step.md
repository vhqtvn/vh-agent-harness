# Source Packet: Correct context for the next step — survey of all technique families

**Date:** 2026-09-15
**Task card:** `task-2026-09-15t10-41-50-survey-all-context-correctness-approaches-incl.-literature-under-the-next-step-reframe`
**Research question:** What are ALL known approaches to ensuring an LLM agent holds the CORRECT context for its next step — where correctness = (D1) needed facts alive, (D2) instructions/obligations live, (D3) premises fresh, (D4) no pollution/drift/injection — and which are applicable to an OpenCode-first harness that owns only the prompt/context/policy layer (plugins own `output.prompt`/`output.context`/projection; no KV-cache control, no model training, no inference-runtime control)?
**Reframe honored:** DCP-style conversation compaction is surveyed as ONE candidate solution among seven families, not as the goal.
**Time-sensitivity:** academic cores are stable; vendor behavior (Claude Code / OpenCode / Letta / LangGraph) and 2025–2026 preprints are time-sensitive as of 2026-09-15.

## Evidence-tier legend

| Tier | Meaning |
|---|---|
| **A** | Peer-reviewed with benchmarks (EMNLP/NeurIPS/ICLR/ICML/TACL/UIST) |
| **B** | Preprint or vendor research report with methodology, unreviewed |
| **C** | Vendor doc / official guidance / verified vendored source |
| **D** | Field anecdote (practitioner blogs, forum synthesis) |
| **R** | Repo ground truth (offline fixture `bc78aaa`, vendored OpenCode 1.18.5) — strongest evidence for OUR runtime, local truth not general truth |

Repo ground truth used throughout: fixture results (`worktrees/dcp-fixture/tmp/agent-runs/compaction-architecture-evaluation/results.md`, offline, no model calls); boundary ledger (`tmp/agent-runs/dcp-boundary-research/notes.md`); evaluation brief (`tmp/agent-runs/compaction-architecture-evaluation/brief.md`); r/LocalLLaMA synthesis 2026-09-10 (as summarized in the task dispatch).

---

## Family 1 — In-context construction (layout, placement, register, tails)

### 1.1 Instruction placement (primacy/recency)
- **Mechanism**: put obligations/critical facts at the very start or very end of the context; never mid-context. Decoder-only attention yields a U-shaped recall curve.
- **Dimensions**: D2 (instructions), D1 partially.
- **Evidence (A)**: Liu et al., *Lost in the Middle: How Language Models Use Long Contexts*, TACL 2024 (arXiv:2307.03172) — GPT-3.5-Turbo multi-doc QA drops >20% when the answer is mid-context, sometimes below the 56.1% closed-book baseline; effect persists at 128K+ windows. One cited study: source incorporation 96% start / 94% end / 52% middle.
- **Failure modes**: doesn't help obligations injected mid-context by a summary; recency alone discards old-but-live constraints.
- **Cost**: free (ordering only).
- **Maturity**: high; replicated widely.

### 1.2 Register boundaries / structured formats
- **Mechanism**: separate system / user / tool / memory registers; keep record-sourced text out of user-labeled sections; typed blocks (contract, findings, todos).
- **Dimensions**: D2, D4.
- **Evidence (C)**: Anthropic, *Effective context engineering for AI agents* (2025-09-29, anthropic.com/engineering/effective-context-engineering-for-ai-agents) — system prompt as "immutable operating system"; structured agent state. **(R)**: fixture case 7 — B/H/C all avoid misattributing model-forged user-styled lines when records stay outside user sections; B's attribution losses (cases 2/7) were budget truncations of genuine user constraints.
- **Failure modes**: register mixing inside a narrative summary (our register-mismatch finding, §Cross-check 4).
- **Cost**: trivial.
- **Maturity**: field-proven.

### 1.3 Raw-tail policy (preserve recent turns verbatim)
- **Mechanism**: keep the last N turns raw alongside any summary; recency is where the next step usually lives.
- **Dimensions**: D1 for recent facts, D2 for recent obligations.
- **Evidence (C/R)**: OpenCode 1.18.5 `session/compaction.ts` — `tail_turns` default 2, preserve-recent budget 2k–8k tokens (vendored source, `refs/opencode`, repo evidence); Claude Code keeps recent tool results inline (Anthropic docs, tier C/D). **(A)**: recency half of the Liu et al. U-curve.
- **Failure modes**: obligations stated early and never restated are NOT in the tail — the tail is not a memory strategy, only a locality one.
- **Cost**: linear in tail size; bounded by config.
- **Maturity**: shipped in both Claude Code and OpenCode.

### 1.4 Cache-stable prefix layout
- **Mechanism**: keep the context head byte-stable across turns so the provider KV prefix cache hits; put volatile content at the tail.
- **Dimensions**: none directly; protects D1/D2 economically (cheaper re-injection).
- **Evidence (C/D)**: Manus, *Context Engineering for AI Agents* (2025-07, manus.im/blog) — KV-cache hit rate is the #1 production metric, ~10× cached-vs-uncached cost delta, 100:1 input/output ratio; Anthropic prompt caching (docs.anthropic.com — cache read 0.1×, write 1.25×, 5-min TTL); OpenAI automatic caching (50% off ≥1024 tokens).
- **Failure modes**: dynamic tool definitions at the head invalidate caches (Manus).
- **Cost**: negative (saves money).
- **Maturity**: production-standard.

### 1.5 Microcompaction / offload-with-reference
- **Mechanism**: replace bulky old tool outputs with a pointer ("content cleared, retrievable at path"), keep recent ones inline.
- **Dimensions**: D1 partially (reference survives), D3 no.
- **Evidence (C/R)**: Claude Code microcompaction (large tool outputs saved to disk, path kept in context — Anthropic docs/field reports); **(R)** OpenCode 1.18.5 `prune()` marks tool parts `state.time.compacted`, model view shows `[Old tool result content cleared]`, storage bytes retained (`message-v2.ts:293-296`).
- **Failure modes**: pointer without retrievability or freshness check = silent dead reference (our C strategy's no-failure-channel risk).
- **Cost**: tiny inline; retrieval on demand.
- **Maturity**: shipped in both CLIs.

---

## Family 2 — Compaction / summarization

### 2.1 Recursive / hierarchical summarization (the failure-mode baseline)
- **Mechanism**: summarize the head when nearing the limit; repeat on the summary-of-summary as limits recur.
- **Dimensions secured**: token budget only — none of D1–D4 by construction.
- **Evidence**: **(B)** Labash et al., *SummHay* (arXiv:2407.01370, Salesforce) — coverage+citation over insight haystacks is an open challenge; GPT-4o / Claude 3 Opus without retrievers score <20% joint vs humans ~56%; **(C/D)** map-reduce loses cross-document context, "Refine"-style iterative summaries propagate early mistakes (LangChain summarization docs; practitioner analyses); **(A, adjacent)** Laban et al., *LLMs Get Lost in Multi-Turn Conversation* (arXiv:2505.06120) — 39% avg drop across six generation tasks in multi-turn vs single-turn; degradation is unreliability (over-reliance on earlier own outputs), not lack of aptitude; **(D)** r/LocalLLaMA synthesis 2026-09-10 — "autophagy": recursive summarization feeds on its own output, drift compounds; **(R)** fixture case 3 — B's projection re-derived from its own post-compaction context sheds o4 permanently from generation 1 (4/7 anchors, 3 attribution errors).
- **Failure modes**: progressive obligation drop, hallucinated continuity, register flattening.
- **Cost**: one model call per compaction.
- **Maturity**: ubiquitous (Claude Code `/compact`, OpenCode auto-compaction) with widely acknowledged lossiness.

### 2.2 Anchored / structured summary templates
- **Mechanism**: fixed schema (objective / important details / work state / next move / files) instead of free-form narrative.
- **Dimensions**: D1/D2 partially (named slots survive better than prose).
- **Evidence (C/R)**: OpenCode V2 core compaction template (Objective/Important Details/Work State/Next Move/Relevant Files, buffer 20k, keep 8k, cap 4096 — vendored 1.18.5); Anthropic guidance "maximize recall before precision" when choosing what survives compaction (2025-09-29).
- **Failure modes**: slot budgets silently truncate (our c9: FRF lines 1–20 kept, 21+ dropped silently).
- **Cost**: one bounded model call.
- **Maturity**: shipped (OpenCode core; Claude Code summary sections).

### 2.3 Extractive verbatim carries (vs abstractive paraphrase)
- **Mechanism**: obligations/security constraints carried as verbatim quoted strings, not paraphrase.
- **Dimensions**: D2 strongly, D4 (attribution integrity).
- **Evidence (R)**: fixture — all strategies keep record-sourced text outside user sections (case 7), and B's attribution errors correlate with deterministic line-budget truncation (§8.2 budgets); **(B)** SummHay's citation axis shows models are weak at tying claims to sources; **(C)** Anthropic context-engineering post (compaction must preserve "essential information"; repo `compaction-discipline` skill codifies verbatim security clauses).
- **Failure modes**: verbatim carries cost tokens; truncation budgets applied to verbatim blocks = silent obligation loss.
- **Cost**: linear in carried bytes (H pays 1.1–1.6× B — R).
- **Maturity**: our slice-2 proposal; partially field-adopted (CLAUDE.md verbatim re-injection).

### 2.4 Re-derivation-from-log (grounded recovery)
- **Mechanism**: on resume/compaction, re-derive premises and obligations from retained exact evidence (digests, immutable sources), refusing when sources are missing/stale.
- **Dimensions**: D1, D3 strongly; D4 via provenance labels.
- **Evidence (R)**: H strategy — retains all recoverable anchors, visible failures on c5 (silent v1→v2 substitution refused), c6a (deleted contract), c6b (truncated checkpoint), c8b (unresolved contradiction flagged); r/LocalLLaMA synthesis: "re-derive-from-log beats re-summarize"; **(C/D)** Manus — "restorable context": keep URLs/paths, prefer re-fetch over destructive truncation; Claude Code microcompaction offloads-with-reference rather than deleting.
- **Failure modes**: no recovery when original bytes are gone (H 1/3 on c6a — honest ceiling); re-derivation costs extra calls (24 tokens, 1 call on c4 — R).
- **Cost**: 1.1–1.6× B inline + occasional re-derivation calls (R).
- **Maturity**: our proposal (slice 1); directionally supported by field practice.

### 2.5 Compaction triggers / budgets
- **Mechanism**: when to compact (overflow thresholds) and how much to keep (buffers, caps).
- **Dimensions**: cost control; indirect D2 (budget composition decides what survives).
- **Evidence (C/R)**: OpenCode auto settings (buffer 20k, keep 8k, summary cap 4096 — vendored); repo `opencode.jsonc` (`auto: true, prune: true, reserved: 12000`); Claude Code auto-compact near the window limit (~95%, version-dependent — C/D), `/compact focus on X` takes instructions.
- **Failure modes**: budget truncation is silent by default (our c9).
- **Maturity**: shipped; visibility of truncation is NOT shipped anywhere surveyed.

---

## Family 3 — External memory

### 3.1 MemGPT-style paging (LLM-as-OS)
- **Mechanism**: OS-inspired tiers — main context (system + working memory + FIFO queue) vs archival storage; the agent pages memories in/out via tool calls (search/archival insert), with interrupts for control flow.
- **Dimensions**: D1 via recall; D2/D3 only if paged back in time; D4 risk — recall is itself generative.
- **Evidence (A⁻/B)**: Packer et al., *MemGPT: Towards LLMs as Operating Systems* (arXiv:2310.08560, 2023) — doc analysis beyond window with stable performance; deep-memory retrieval 92.5% (GPT-4) vs 32.1% baseline full-context; ~2.5k tokens/query = 85% cost reduction vs 25k full-context. The paper itself flags hallucination risk when recalling from summaries (self-reported limitation).
- **Failure modes**: recall misses; summary-mediated recall can hallucinate; per-query retrieval token cost (matches our C finding).
- **Cost**: low inline, moderate retrieval per query.
- **Maturity**: high (Letta productionized).

### 3.2 Letta memory blocks + sleep-time compute (successor)
- **Mechanism**: labeled, size-limited memory blocks (persona/human/knowledge) persistently visible in-context; core (in-window) vs recall (searchable history) vs archival (cold store); sleep-time agents rewrite memory during idle periods, turning raw context into "learned context".
- **Dimensions**: D1 (archival), D2 (core blocks always in window — obligations), D3 via background rewriting (risk: drift), D4 unaddressed formally.
- **Evidence (C)**: Letta docs/blog (letta.ai; letta.com/blog memory-blocks, sleep-time-compute).
- **Failure modes**: block size limits reintroduce silent drop; sleep-time rewriting is model-generated ⇒ drift risk (same class as recursive summarization, offline).
- **Maturity**: commercial product.

### 3.3 Blackboard architectures (classic)
- **Mechanism**: shared structured workspace that cooperating specialists read/write; control driven by workspace state, not any single agent's context.
- **Dimensions**: D1/D2 durable across agent swaps; D4 via explicit source labeling on entries.
- **Evidence (A, classic)**: Erman et al., Hearsay-II (IEEE Trans. Computers, 1980); Hayes-Roth, BB1 (1985). **(R)**: repo's `.local/coordinator/` + `researches/` + plan-state is a blackboard; the fixture's obligation-bearing artifacts (contracts, checkpoints, decisions) are exactly blackboard entries.
- **Failure modes**: stale entries on the board (premise rot) unless invalidation is explicit.
- **Cost**: file I/O; zero model tokens.
- **Maturity**: 45 years; the repo already implements it.

### 3.4 Files-on-disk as persistent instructions/memory
- **Mechanism**: `CLAUDE.md`/`NOTES.md`/`todo.md` written outside the window and re-injected each request — instructions survive compaction by re-injection, not by being summarized well.
- **Dimensions**: D2 strongly (the documented purpose), D3 partially.
- **Evidence (C)**: Anthropic context-engineering guidance (structured note-taking / agentic memory); Claude Code CLAUDE.md "cascading context management" (global → project → local re-injection); **(C/D)** Manus todo.md scratchpad.
- **Failure modes**: files can go stale (premise rot — our 4-tuple protocol exists for this); re-injection spends tokens every turn; mutable file overwrite destroys history (R: `state-lib.js` E5 — contract md+json overwrite in place, gitignored).
- **Cost**: small inline per turn.
- **Maturity**: field-standard.

### 3.5 Knowledge-base projection + salience selection (Generative Agents)
- **Mechanism**: memory stream of natural-language observations; retrieval scores recency (exponential decay) × importance (LLM-scored at creation) × relevance (embedding similarity to the query).
- **Dimensions**: D1 (selection), D3 via recency weighting, D4 no.
- **Evidence (A)**: Park et al., *Generative Agents: Interactive Simulacra of Human Behavior*, UIST 2023 (arXiv:2304.03442) — ablation effect d=8.16 vs no-memory/planning/reflection baseline; 25-agent sandbox emergent behavior.
- **Failure modes**: salience ≠ obligation: an unimportant-sounding constraint (a return-format clause) scores low on importance but is D2-critical. Salience selection must be overridden for obligations.
- **Cost**: embedding + scoring per retrieval.
- **Maturity**: academic classic; the scoring pattern is production-adopted in variants.

### 3.6 Temporal knowledge graphs (Zep/Graphiti)
- **Mechanism**: temporally-aware KG (episode/entity/community subgraphs) with bi-temporal validity (fact `valid_from`/`valid_to`, event time vs ingestion time); new facts invalidate old ones WITHOUT deleting history.
- **Dimensions**: D3 strongest of all families (explicit supersession), D1, D4 partial (provenance edges).
- **Evidence (B)**: Rasmussen et al., *Zep: A Temporal Knowledge Graph Architecture for Agent Memory* (arXiv:2501.13956, 2025-01) — self-reported LoCoMo 94.7% accuracy, p50/p95 latency 87/155ms, median 5,760 context tokens/question; LongMemEval +18.5% accuracy, −90% latency vs baselines. **Contradiction (flagged)**: Mem0's counter-runs report Zep at ~80–83% on LoCoMo and themselves claim higher numbers — benchmark configurations are disputed between vendors.
- **Failure modes**: KG extraction errors; heavy infra; vendor benchmark inflation risk.
- **Cost**: service calls; nontrivial.
- **Maturity**: commercial; the *pattern* (explicit invalidation) is portable, the service is out-of-boundary.

### 3.7 A-MEM (agentic memory organizer)
- **Mechanism**: Zettelkasten-style notes with structured attributes (context, keywords, tags), dynamic linking of new notes to related old ones, and memory evolution (new memories update old memories' representations).
- **Dimensions**: D1, D3 via evolution; D4 risk — evolution is model-generated rewriting (drift class).
- **Evidence (B)**: Xu et al., *A-MEM: Agentic Memory for LLM Agents* (arXiv:2502.12110, 2025-02, Rutgers/AIOS) — claims SOTA on six foundation models; 85–93% token reduction vs MemGPT.
- **Failure modes**: self-rewritten memories inherit summarization-drift risks; benchmark disputes (same LoCoMo family).
- **Maturity**: preprint + open source.

---

## Family 4 — Retrieval (over own history)

### 4.1 RAG-over-own-history
- **Mechanism**: index conversation/history artifacts; retrieve per step into the window.
- **Dimensions**: D1 when retrieval hits; D2/D3 only if the right artifact is retrieved at the right time.
- **Evidence (A)**: Wu et al., *LongMemEval* (ICLR 2025, arXiv:2410.10813) — 500 questions, five abilities (extraction, multi-session reasoning, temporal reasoning, knowledge updates, abstention); commercial assistants and long-context LLMs drop ~30% on sustained-interaction memory; evaluates memory as indexing → retrieval → reading.
- **Failure modes**: recall misses are silent (no failure channel — matches our C finding); query formation quality bounds everything.
- **Cost**: index maintenance + per-step retrieval tokens (R: C pays 180–540 retrieval tokens/case on top of ~100 inline).
- **Maturity**: production-common; correctness-sensitive use needs a failure channel.

### 4.2 Pointer + search (slug/pointer-only context — our C)
- **Mechanism**: inline context is a list of typed pointers (slugs, paths, IDs); detail fetched on demand.
- **Dimensions**: D1 via fetch; D2 weak (obligations behind pointers die when unfetched); D3 no freshness signal; D4 no channel.
- **Evidence (R)**: fixture — C retains full anchors when retrieval succeeds (9/9, 5/5, 7/7) at lowest inline cost (~105 tokens) but FAILS first-action compliance on premise re-derivation (case 4: NO) and silently substitutes v1-era refs with live v2 content (case 5, 0 visible failures). **(B)** MemGPT/Zep retrieval-token costs corroborate that retrieval is not free.
- **Failure modes**: mutable locators (repo E4: `source_ref` informational, not dereferenced), dead pointers, silent substitution.
- **Maturity**: viable as a fallback channel; not as the sole channel.

### 4.3 Embedding salience for what-to-retrieve
- **Mechanism**: cosine similarity between current-step query and artifact embeddings (Generative-Agents relevance; LangGraph semantic stores).
- **Evidence (A/C)**: Park et al. 2023 (relevance term); LangGraph store docs (langchain.com/docs, semantic search namespaces).
- **Failure modes**: obligation-bearing text often lexically dissimilar to the current step (a return-format clause vs a debugging step) — embedding miss; NoLiMa (A) shows models themselves fail latent-association recall, and retrieval by embedding has the same failure shape.
- **Maturity**: standard tooling.

### 4.4 Agent-formed queries (self-directed retrieval)
- **Mechanism**: the agent decides what to search for (MemGPT tool calls; Letta archival search).
- **Evidence (B/C)**: MemGPT, Letta docs.
- **Failure modes**: the agent doesn't know what it has forgotten (unknown-unknowns) — queries can't target unremembered obligations. This is the structural argument for projection (push) over pure pull.
- **Maturity**: production.

---

## Family 5 — State externalization (durable spine)

### 5.1 Task contracts / plans / todo state machines
- **Mechanism**: mission, obligations, non-goals, closeout checklists live in files/state machines outside the conversation; the conversation is disposable, the spine is not.
- **Dimensions**: D2 (the core mechanism), D3 (premise 4-tuples with re-derivation commands), D4 (provenance notes).
- **Evidence (R)**: repo task contracts/checkpoints/handoffs (E7); premise-recheck protocol with `(value, source, re_derivation_command, observed_at)` 4-tuples; **(C/D)** Manus todo.md; Anthropic structured agent state; **(C)** LangGraph checkpointer (thread-scoped short-term memory persisted for resume) + procedural memory (instructions as memory, refined by reflection).
- **Failure modes**: spine overwrite destroys history (R: E5 — contracts overwrite in place, gitignored ⇒ unrecoverable prior versions; fixture case 5); spine/state divergence if the spine is updated from a degraded context (R: case-3 drift model rewrites contracts from post-compaction context — the autophagy failure relocated into the spine).
- **Cost**: small writes; near-zero model tokens.
- **Maturity**: field-proven; the repo's center of gravity.

### 5.2 Reflexion (verbal self-reflection into episodic memory)
- **Mechanism**: Actor/Evaluator/Self-reflection loop; task-failure feedback is verbalized into an episodic buffer that conditions the next trial.
- **Dimensions**: D3 (lessons learned carried forward), D1 partial.
- **Evidence (A)**: Shinn et al., *Reflexion*, NeurIPS 2023 (arXiv:2303.11366) — 91% HumanEval pass@1 (vs 80% GPT-4); 97% AlfWorld (vs 75% ReAct).
- **Failure modes**: reflections are model-generated ⇒ can encode rationalized wrong lessons (drift/pollution, D4 risk) unless grounded in verbatim evidence.
- **Cost**: one reflection call per failure.
- **Maturity**: high (academic), widely adopted in variants.

### 5.3 Reflection over memory streams (generative-agents style)
- **Mechanism**: periodically synthesize higher-level inferences from accumulated memories; store as new memory entries.
- **Evidence (A)**: Park et al. 2023 — reflection ablation contributes to the d=8.16 effect.
- **Failure modes**: same drift class as 5.2.
- **Maturity**: academic.

### 5.4 Scratchpads / working notes
- **Mechanism**: running notes file the agent maintains as the durable trace of reasoning state.
- **Evidence (C/D)**: Manus; Anthropic NOTES.md guidance; repo session memory files.
- **Failure modes**: notes go stale (D3) unless re-derived; duplication with contracts.
- **Maturity**: field-standard.

---

## Family 6 — KV-cache level (surveyed for completeness — **OUT OF BOUNDARY** for us)

### 6.1 StreamingLLM
- **Mechanism**: retain a few initial "attention sink" tokens + sliding recent window in the KV cache; stream indefinitely.
- **Evidence (A)**: Xiao et al., ICLR 2024 (arXiv:2309.17453) — 4M+ tokens stable, 22.2× speedup vs sliding-window recompute.
- **Correctness verdict**: preserves recency + anchors only; mid-history facts are EVICTED BY DESIGN — D1 fails structurally. Boundary: inference runtime — we own none of it.

### 6.2 H2O
- **Mechanism**: KV eviction retaining "heavy-hitter" tokens by cumulative attention + recent tokens; submodular guarantee.
- **Evidence (A)**: Zhang et al., NeurIPS 2023 (arXiv:2306.14048) — up to 29× throughput, 1.9× latency reduction at 20% retention.
- **Correctness verdict**: eviction is permanent within a run; FlashAttention incompatibility noted; instruction tokens with low attention mass can be evicted (D2 risk). Out of boundary.

### 6.3 SnapKV
- **Mechanism**: per-head KV compression by clustering important positions observed in a late "observation window".
- **Evidence (A)**: Li et al., NeurIPS 2024 (arXiv:2404.14469) — 3.6× generation speed, 8.2× memory efficiency at 16K; 380K on one A100-80GB.
- **Correctness verdict**: prompt-focused; compresses by what late-window attention saw — obligations early in the head risk under-selection. Out of boundary.

### 6.4 KV / internal-memory editing
- **Mechanism**: edit key-value memories inside the model (FFN as KV-memory view) to change latent judgments without retraining.
- **Evidence (A)**: Gemma et al., *Editing Common Sense in Transformers*, EMNLP 2023 (arXiv:2305.14956).
- **Correctness verdict**: research-stage model surgery. Out of boundary.

### 6.5 Prompt-caching economics (the one in-boundary spillover)
- **Evidence (C)**: Anthropic docs — cache read 0.1× base input, write 1.25×, 5-min TTL (1-h option); OpenAI — automatic 50% discount ≥1024 tokens.
- **Implication**: stable-prefix layout (Family 1.4) captures most reachable benefit; anything mutating the head per turn (e.g. per-turn obligation rewriting) forfeits ~10× cost deltas (Manus, C/D).

---

## Family 7 — Training / eval-time (surveyed for completeness — **OUT OF BOUNDARY**)

### 7.1 Context rot
- **Evidence (B)**: Chroma Research, *Context Rot* (2025-07, research.trychroma.com/context-rot) — 18 frontier models all degrade with input length well before window limits; degradation is non-uniform across task type, distractor similarity, ambiguity; distractor interference and ambiguity amplification named as mechanisms.
- **Implication (in-boundary)**: bigger windows do NOT solve next-step correctness — projection discipline stays necessary at any window size.

### 7.2 NoLiMa
- **Evidence (A)**: Modarressi et al., *NoLiMa*, ICML 2025 (arXiv:2502.05167) — 13 models; 11/13 score below 50% of their short-context baseline at 32K on tasks needing associative (not literal) recall; GPT-4o 99.3% → 69.7% at 32K; effective length often ≤2K (paper's analysis, medium confidence).
- **Implication**: "just extend the window and keep everything" fails; long-window presence ≠ usable knowledge.

### 7.3 Multi-turn degradation (no compaction involved)
- **Evidence (A)**: Laban et al. (arXiv:2505.06120) — see 2.1. Constraints are lost through turn accumulation alone; compaction accelerates an existing failure mode.

### 7.4 Distribution shift over history formats
- **Evidence (D)**: field reports — coding CLIs trained on structured transcript formats degrade when history is summarized/reformatted ("lossy compaction", "sanitized superficial history", "cold-start amnesia"; e.g. Santoni 2026: Claude Code auto-compaction 132k → 2.3k tokens, "loses most of the nuanced reasoning, architectural understanding, and convention knowledge"); industry's fix (CLAUDE.md re-injection) exists precisely because summarized-away instructions fail. No controlled academic isolation of the summary-register variable — **gap**; our fixture's register findings are novel evidence.

### 7.5 Compaction-aware fine-tuning
- **Evidence (weak)**: practice is emerging (vendors tune compaction prompts/quality, e.g. Anthropic engineering notes on compaction fidelity); explicit fine-tuning on compacted histories has no strong published evaluation found — **gap**. Out of boundary regardless.

### 7.6 Benchmarks
- **LongMemEval (A)** — ICLR 2025, the strongest memory benchmark. **LoCoMo (B, disputed)** — Zep self-report 94.7% vs Mem0-run Zep ~80–83%: vendor-dependent. **SummHay (B)** — coverage+citation over summaries. **EvolIF multi-turn instruction following (B, low confidence)** — stability scores (GPT-5 66.4%) reported in secondary sources. **No benchmark exists for coding-agent continuation fidelity after compaction** — **gap**; our offline fixture is a step toward one.

---

## Taxonomy table (all seven families)

| Family | Technique | D1 facts | D2 obligations | D3 freshness | D4 no-pollution | In our boundary? | Cost shape | Maturity | Tier |
|---|---|---|---|---|---|---|---|---|---|
| 1 | Placement (primacy/recency) | ◐ | ● | ○ | ○ | YES | free | high | A |
| 1 | Register boundaries | ○ | ● | ○ | ● | YES | trivial | field | C |
| 1 | Raw-tail policy | ◐(recent) | ◐(recent) | ○ | ○ | YES (config) | linear-tail | shipped | C/R |
| 1 | Cache-stable prefix | ○ | (protects) | ○ | ○ | YES | negative | prod | C |
| 1 | Microcompaction/offload | ◐(ref) | ○ | ○ | ○ | YES (native) | tiny+fetch | shipped | C/R |
| 2 | Recursive summarization | ✕ | ✕ | ✕ | ✕(drifts) | YES (default) | 1 call/compact | ubiquitous, lossy | A/B/D |
| 2 | Anchored summary template | ◐ | ◐ | ○ | ◐ | YES | 1 call/compact | shipped | C/R |
| 2 | Extractive verbatim carries | ◐ | ● | ○ | ● | YES (slice 2) | linear bytes | proposal+field | B/R |
| 2 | Re-derivation-from-log | ● | ● | ● | ◐ | YES (slice 1) | 1.1–1.6×+calls | proposal | R/C |
| 2 | Triggers/budgets | ○ | ◐ | ○ | ◐ | YES | free | shipped | C/R |
| 3 | MemGPT paging | ● | ◐ | ◐ | ✕(recall risk) | partial (tools) | low-inline+retrieval | high | A⁻ |
| 3 | Letta blocks/sleep-time | ● | ●(core) | ◐ | ✕(rewrite drift) | NO (service) | service | commercial | C |
| 3 | Blackboard | ● | ● | ◐ | ◐ | YES (already have) | file I/O | 45y classic | A/classic |
| 3 | Files-on-disk (CLAUDE.md/todo) | ◐ | ● | ◐ | ○ | YES | small/turn | field-standard | C |
| 3 | Salience projection (GenAg) | ◐ | ✕(low-importance obligations) | ◐ | ○ | pattern YES | embed+score | classic | A |
| 3 | Temporal KG (Zep) | ● | ◐ | ● | ◐ | pattern YES / service NO | service | commercial | B (disputed) |
| 3 | A-MEM | ● | ◐ | ◐ | ✕(evolution drift) | NO | service | preprint | B |
| 4 | RAG-over-history | ◐(misses) | ◐ | ◐ | ○ | YES (tools) | idx+retrieval | common | A |
| 4 | Pointer+search (our C) | ◐ | ✕ | ✕(no signal) | ✕(no channel) | YES | ~100 inline+180–540 fetch | comparator | R |
| 4 | Embedding salience | ◐ | ✕ | ◐ | ○ | YES | embed | standard | A/C |
| 4 | Agent-formed queries | ◐ | ✕(unknown-unknowns) | ◐ | ○ | YES | per-query | production | B/C |
| 5 | Contracts/plans/todos | ● | ● | ●(4-tuples) | ◐ | YES (core spine) | files | field-proven | R/C |
| 5 | Reflexion | ◐ | ○ | ● | ✕(rationalization) | YES | 1 call/failure | high | A |
| 5 | Reflection on streams | ◐ | ○ | ◐ | ✕ | pattern | 1 call | academic | A |
| 5 | Scratchpads | ◐ | ◐ | ◐ | ○ | YES | small | field | C/D |
| 6 | StreamingLLM | ✕(evicts mid) | ✕ | ○ | ○ | **NO** | runtime | ICLR | A |
| 6 | H2O / SnapKV eviction | ✕ | ✕ | ○ | ○ | **NO** | runtime | NeurIPS | A |
| 6 | KV editing | ✕ | ✕ | ◐ | ✕ | **NO** | research | EMNLP | A |
| 6 | Prompt-cache economics | ○ | ○ | ○ | ○ | spillover YES | negative | prod | C |
| 7 | Long-context (context rot/NoLiMa) | ✕ | ✕ | ○ | ✕ | **NO** (evidence) | — | measured | A/B |
| 7 | History-format training | ✕ | ✕ | ○ | ✕ | **NO** | — | gap | D |
| 7 | Compaction-aware FT | ◐ | ◐ | ○ | ◐ | **NO** | — | gap | weak |

● strong ◐ partial ○ none/NA ✕ fails-by-design

---

## Applicability ranking (filtered to OUR boundary: prompt/context/policy layer only)

1. **Source-bound projection with visible-failure channel (H)** — owns the failure channel no other in-boundary technique has; matches field direction (restorable context, offload-with-reference); 1.1–1.6× measured cost. (R; C/D support)
2. **Persistent obligation re-injection in a stable head** — CLAUDE.md-proven; obligations survive compaction by re-injection, not by being summarized well; doubles as cache-stable prefix. (C)
3. **Owned summary contract with verbatim security + attribution clauses (slice 2)** — extractive carries for D2/D4; our attribution-error data shows budget truncation is the leak; Anthropic's recall-over-precision ordering applies. (R + C)
4. **Durable state spine: contracts / checkpoints / todos / premise 4-tuples** — already our architecture; literature and field converge on it (blackboard, todo.md, checkpointer, procedural memory). Extend with immutability (see impact). (R + C + classic)
5. **Per-context raw-tail policy (slice 3)** — native config exists (`tail_turns`, preserve-recent budgets); recency evidence (A) + shipped defaults (C/R). Cheapest correctness win per token.
6. **Salience-ranked projection selection** (Generative-Agents scoring) — for choosing what enters the projection, with an obligation override (importance scoring must never gate D2 items). (A, adapted)
7. **Temporal supersession marking** (Zep pattern) — fold bi-temporal validity (`observed_at`, `superseded_by`, explicit invalidation) into H's recovery ledger; gives D3 a visible signal. (B, pattern port)
8. **Pointer+search retrieval (C) as fallback channel only** — cheapest inline, but no failure channel, silent substitution, mutable locators; never the sole carrier of obligations. (R)
9. **Sub-agent context isolation** — already implemented in this harness (specialist fan-out returns distilled reports); keeps pollution out of the coordinator context. Maintain; nothing new to build. (C)
10. **Cache-stable layout discipline** — policy-level rule: obligations live in the stable head; per-turn churn confined to the tail. (C/D)

Out of boundary (do not pursue): KV eviction/editing (6.x), long-context reliance (7.1/7.2), training-time (7.4/7.5). Zep/A-MEM/Letta as services (3.2/3.6/3.7) — patterns portable, infrastructure not.

---

## Literature-vs-our-findings cross-check

**1. B's silent obligation loss (8/11 fixture rows). CONFIRMED (multi-source, indirect).**
- Anthropic's own guidance treats compaction as lossy and prescribes recall-maximizing selection + external notes (C, 2025-09-29); Claude Code field reports document lost early instructions and 132k→2.3k collapse (D, Santoni 2026).
- Laban et al. (A): constraint loss accumulates in multi-turn even with NO compaction — B's failure is an accelerated instance of a measured baseline failure, not an artifact of our fixture.
- Chroma context rot (B): performance degrades with context length even when nothing is dropped — keeping MORE inline is not free either; silent budget truncation (our c9) is the worst of both.
- SummHay (B): coverage + source-citation under summarization is an open challenge — B's un-anchored prose summary sits in the documented weak spot.
- No direct external replication of the exact 8/11 number (fixture is synthetic, offline) — directional confirmation only.

**2. H's visible-failure / source-bound design. SUPPORTED (field + partial-academic).**
- Manus (C/D): restorable context — keep references, prefer re-fetch over destructive truncation = H's pointer+digest+refusal philosophy, at production scale.
- Claude Code microcompaction / OpenCode prune (C/R): offload-with-reference is shipped behavior; H extends the same reference discipline from tool outputs to obligations, and adds the missing piece — refusal when the reference no longer resolves.
- Zep/Graphiti (B): explicit temporal invalidation instead of silent overwrite — direct support for H's c5 refusal (v1→v2 silent substitution) and c8b contradiction flagging.
- Anthropic (C): compaction guidance emphasizes deciding what survives — but gives no failure channel; H's contribution (visible degradation, per our behavioral spec §5) remains OUR design; no published equivalent found (novelty claim, not a literature claim).

**3. C's retrieval cost and missing failure channel. CONFIRMED (quantitatively aligned).**
- MemGPT (A⁻): paging costs ~2.5k tokens/query — retrieval-heavy in practice despite 85% savings vs full-context; our C's 180–540 retrieval tokens/case is the same shape at fixture scale (R).
- Zep (B): median 5,760 context tokens per answered question — retrieval-based memory is NOT tiny-inline; matches our finding that C's total often exceeds B inline.
- LongMemEval (A): ~30% drops with sustained-interaction memory — recall misses are real and, critically, SILENT; no surveyed retrieval system emits a failure signal on miss. C's zero visible failures in our fixture is the class behavior, and case 5's silent v1→v2 substitution is the mutable-locator failure MemGPT/Zep mitigate only with extra machinery.
- LoCoMo vendor dispute (Zep 94.7% self-report vs ~80–83% in Mem0's runs): retrieval-memory numbers are configuration-dependent — treat all such benchmarks cautiously.

**4. Register mismatch (narrative summary vs literal turns). PARTIALLY SUPPORTED — our finding is ahead of the literature.**
- Laban et al. (A): models over-rely on their own earlier utterances and degrade through "distorted" intermediate turns — a first-turn/paraphrase distortion effect adjacent to register mismatch; strongest published analog.
- Field (D): "lossy compaction" / "sanitized superficial history" failure mode; industry's CLAUDE.md re-injection exists precisely because paraphrased-into-narrative obligations fail — implied confirmation.
- SummHay's citation axis (B): summaries detach claims from sources — the same flattening our attribution errors exhibit.
- NO controlled academic study isolates "summary register vs literal-turn register" as a variable — flagged as an open gap; our fixture result (B's attribution errors cluster on budget-truncated verbatim user constraints, cases 2/3/5/7) is novel evidence worth externalizing.

---

## Impact statement on re-scoped roadmap slices 1–3

**Slice 1 — H projection (source-bound recovery, visible failures): CONFIRM, with two additions.**
- ADD: temporal supersession fields (`observed_at`, supersession state) in the recovery ledger — Zep's bi-temporal pattern; makes D3 visible rather than implicit in freshness checks.
- ADD: obligation re-injection into the STABLE HEAD, not only the compaction context — CLAUDE.md-style re-injection survives compaction by construction and preserves cache-stable prefixes (Manus economics). H context injected only at compaction time leaves obligations mid-context between compactions (lost-in-the-middle exposure).
- Keep B as baseline; H cost (1.1–1.6×) is within the range field systems already pay for restorability.

**Slice 2 — owned summary contract (verbatim security + attribution clauses, typed findings, pointers, retention classes): CONFIRM, sharpen one clause, adopt one taxonomy.**
- SHARPEN: an explicit REGISTER clause — user-issued obligations must be carried VERBATIM in user-labeled register, never paraphrased into narrative summary text (our attribution-error data + Laban + field). This is the contract's answer to the register-mismatch failure.
- ADOPT: LangGraph's semantic/episodic/procedural taxonomy for retention classes (or the repo's equivalent: obligations/premises/findings) rather than inventing a novel one — C-tier convergence beats B-tier novelty.
- The compaction-discipline five-section structure already mirrors OpenCode V2's anchored template (R) — keep aligned.

**Slice 3 — per-context tail policy: CONFIRM, add one invariant.**
- Native knobs exist (`tail_turns`, preserve-recent budget, reserved) — slice is config + policy, cheap.
- ADD invariant: truncation must be VISIBLE (our c9: FRF lines 21+ silently dropped) — any deterministic budget that drops content must emit a degradation marker, else slice 3 recreates B's silent loss inside H.
- Couple tail size/composition to cache-stability: volatile tail only; never mutate the stable head per turn (Manus 10× cost delta).

**Nothing to DROP from the roadmap.** Families 6–7 are explicitly out of boundary; their evidence (context rot, NoLiMa) *strengthens* the case for slices 1–3 by eliminating "longer windows / better models" as alternatives.

---

## Contradictions

1. **LoCoMo vendor dispute** — Zep's self-reported 94.7% vs Mem0-run Zep ~80–83% (and Mem0's own >91% claims): retrieval-memory benchmarks are configuration-dependent. Resolution: none available; treat all LoCoMo numbers as vendor-conditional (B-tier with an asterisk).
2. **LLMLingua's "up to 20× compression with ~1.5% loss" (A, benchmark tasks) vs field/vendor caution that compression loses crucial details** — benchmark-vs-domain mismatch, not a factual contradiction: obligation-dense coding-agent contexts differ from GSM8K-style evaluation. Do not cite LLMLingua as license for token-level compression of obligations.
3. **MemGPT's 92.5% deep-retrieval accuracy vs its own acknowledged recall-hallucination risk** — the paper reports both; paging works when retrieval hits and fabricates when it misses. Consistent with our C verdict (high retention, no failure channel).
4. **Claude Code auto-compact trigger point and preserved-context specifics vary by source and version** (~95% trigger, 8k-preserve claims) — time-sensitive vendor detail, medium confidence; not load-bearing for our design.
5. **No contradiction found between published evidence and our fixture findings** — every external source either confirms or fails to address them; none contradicts.

## Open gaps (honest)

- **No controlled study of summary-register vs literal-turn register** (cross-check 4) — our fixture evidence is novel and unreplicated externally.
- **No benchmark for coding-agent continuation fidelity after compaction** — LongMemEval/LoCoMo/SummHay measure adjacent things; our offline fixture (no model calls) cannot measure model-side fidelity either — the planned live B/H cold-replay comparison remains necessary.
- **Compaction-aware fine-tuning**: practice exists, published evaluation weak — nothing to cite strongly (7.5).
- **H's 1.1–1.6× cost has no external benchmark analog** — offline chars/4 estimates only; live-model cost measurement pending.
- **OpenCode native compaction V2 (core) has no plugin hooks** (R, vendored 1.18.5) — slice 1's injection rides V1's `experimental.session.compacting`; upstream drift is a standing risk (time-sensitive).

## Source register (count by tier)

- **A (peer-reviewed benchmarked): 11** — Liu et al. TACL 2024 (2307.03172); Park et al. UIST 2023 (2304.03442); Shinn et al. NeurIPS 2023 (2303.11366); Xiao et al. ICLR 2024 (2309.17453); Zhang et al. NeurIPS 2023 (2306.14048); Li et al. NeurIPS 2024 (2404.14469); Jiang et al. EMNLP 2023 (2310.05736); Modarressi et al. ICML 2025 (2502.05167); Wu et al. ICLR 2025 (2410.10813); Laban et al. 2025 (2505.06120); Gemma et al. EMNLP 2023 (2305.14956).
- **B (preprint / research report): 6** — Packer et al. 2023 (2310.08560); Rasmussen et al. 2025 (2501.13956); Xu et al. 2025 (2502.12110); Sun et al. 2025 Context-Folding (2510.11967); Chroma Context Rot 2025 (research.trychroma.com/context-rot); Labash et al. SummHay 2024 (2407.01370). (+EvolIF, low confidence, supplementary.)
- **C (vendor doc / official guidance / vendored source): 8** — Anthropic context-engineering (2025-09-29); Anthropic prompt-caching docs; OpenAI prompt-caching docs; Claude Code docs/field behavior; Letta docs; LangGraph docs; Manus blog (2025-07); vendored OpenCode 1.18.5 (repo).
- **D (field anecdote): 3 named** — Santoni 2026 (auto-compaction field report); r/LocalLLaMA synthesis 2026-09-10; multi-agent "cascading context drift / lossy compaction" practitioner reports.
- **R (repo ground truth): 4 artifacts** — fixture results (bc78aaa worktree); boundary ledger; evaluation brief; behavioral spec `docs/ai/context-pruning-engine-behavioral-spec.md`.

Total ≈ 28 external + 4 repo artifacts.

## Provenance note

Web verification performed live 2026-09-15 via grounded web-answer searches (gemini/zai engines) with per-question source lists; arXiv IDs cross-checked where returned. Claims marked medium/low confidence are flagged inline (NoLiMa effective-length figure; EvolIF; Claude Code version-specific trigger details; Anthropic pricing doc figures widely reported but verify against docs.anthropic.com before external quoting). Engine-returned redirect URLs were canonicalized to destination domains; re-fetch before quoting externally.
