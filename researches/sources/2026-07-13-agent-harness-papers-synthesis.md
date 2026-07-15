# Sources: Two 9-Jul-2026 Agent-Harness Papers — Comparative Synthesis + vh-agent-harness Adoption Candidates

**Date:** 2026-07-13
**Topic:** A comparative read of two very recent arXiv papers — (1) "Workflow as
Knowledge: Semantic Persistence for LLM-Mediated Workflows" and (2) "Remember When
It Matters: Proactive Memory Agent for Long-Horizon Agents" — and an analysis-only
extraction of concrete adoption candidates for the `vh-agent-harness` repo.
**Kind:** Source/synthesis packet. NOT active repo guidance — a reference study plus
an adoption-candidates appendix. **Analysis only: no task cards were written, no
`docs/planning/backlog.md` row was touched, no code was edited.** Candidate capture
into `.local/coordinator/tasks/` is an explicit later coordinator step (each
candidate below carries a `trigger:` predicate for that curation, none executed here).
**Studied against (our side):** `vh-agent-harness` repo at working tree HEAD. Grounding
docs read: `AGENTS.md` (the "agent harness" six-layer term contract; the ownership
safety contract; the "model output is a candidate, never transition authority"
invariant; shell/container/workspace hygiene); `.vh-agent-harness/docs/opencode-session-workflow.md`
(compaction-is-aggressive; task-contract-as-truth; checkpoint Verification tables);
`templates/docs/opencode-memory-model.md` (four memory layers; Anti-spam rule;
typed-records store; budgeted-injection discipline); `internal/` package layout
(`ownership/`, `lineage/`, `runshape/`, `memory/{record,store}/`, `hooks/`,
`permission/`, `permconfig/`, `schema/`, `substrate/`, `proposals/`, `drift/`,
`overlay/`, `runtime/`, `cli/`); and the precedent memo
`researches/sources/2026-07-08-tencentdb-agent-memory-study.md` (matched for house
style and cross-referenced where relevant).

---

## Research question & scope

- **Question:** What conceptual and mechanical patterns from these two papers are
  worth (a) understanding comparatively and (b) considering for adoption in
  `vh-agent-harness`, given we are a single static Go binary on host-shell,
  OpenCode-first, with no vector store, a flat session/workstream memory model,
  a typed append-only JSONL records store, and a hard "model output is a
  candidate, never transition authority" safety invariant?
- **Scope (theirs):**
  - Paper 1 — arXiv:2607.08740v1 (Quinto, Rozzi, Zanitti), submitted 9 Jul 2026,
    39 pp, 18 figs, cs.AI/cs.PL/cs.SE, CC BY-SA 4.0.
  - Paper 2 — arXiv:2607.08716v1 (Wu, Zhang, Zhou, Wang, Peng, Li, Fan, Zhao;
    all Meta AI), submitted 9 Jul 2026, CC BY 4.0, code at
    `github.com/yifannnwu/proactive-memory-agent`.
- **Scope (ours):** conceptual + mechanical patterns only. No code is vendored
  (both papers' reference implementations are non-Go; paper 1 has no
  implementation at all). Every recommendation is a *pattern/discipline
  transplant*, not an import. Benchmark numbers from paper 2 are reported as the
  paper's own evidence and are NOT reproducible here.
- **Time-sensitivity:** FRESH but conceptually STABLE. Both papers are 4 days old
  at memo-write time. Paper 1 is a conceptual/position paper whose claims are
  architectural and not version-fragile. Paper 2's empirical claims are tied to
  specific benchmarks/models (Terminal-Bench 2.0, τ²-Bench, Claude Sonnet 4.5 /
  Opus 4.6) and may shift as those move; the *architectural* claims (two-phase
  memory, behavioral-state-decay failure mode) are stable.
- **Source policy:** PRIMARY = the arXiv full-text HTML render of each paper
  (`arxiv.org/html/2607.08740v1`, `arxiv.org/html/2607.08716v1`), fetched and
  read in full. No secondary commentary was used. Repo grounding is read from
  the working tree directly.

## Confidence legend

- **HIGH** — verified directly against the full paper text (section/table cited)
  and/or directly against a repo file; mechanism unambiguous.
- **MED** — single-source structural reading, or a behavioral claim derived from
  one section rather than a re-read of the whole argument; or a paper-2
  empirical number reported without the paper providing confidence intervals.
- **LOW** — vendor/author self-report only (paper-2 benchmark deltas), or an
  over-claim that the paper asserts but does not demonstrate.

## How citations are formatted

Three citation flavors, kept distinct:

1. **Paper 1** — `Paper 1 §<sec>` or `Paper 1 Table <n>` / `Paper 1 App. <X>`,
   from the arXiv HTML full text. Paper 1 is conceptual/position; it has NO
   experiments, so no "baseline" or "sample size" applies.
2. **Paper 2** — `Paper 2 §<sec>` or `Paper 2 Table <n>`, from the arXiv HTML
   full text. Paper 2 IS empirical; numbers are the paper's own pass@1 reports.
3. **Repo (ours)** — a repo-relative path or an `AGENTS.md` section name, read
   from the working tree at memo-write time.

A confidence tag follows each substantive claim.

---

## Part A — Comparative synthesis

### Paper 1 — "Workflow as Knowledge: Semantic Persistence for LLM-Mediated Workflows"

#### Problem / motivation
Paper 1 opens from a **fragmentation problem** (Paper 1 §3.1): a workflow run from
six months ago — its definition, the inferences it made, the context those
inferences depended on, the approvals/decisions rendered — is typically not
queryable alongside the *knowledge* it produced. The run's artifacts are scattered
across config files, execution logs, chat transcripts, and UI events that were
never persisted as first-class objects. The paper's diagnosis is *ontological*,
not merely a storage gap: the dominant framing — "workflow as configuration,
execution as process, results as output" — demotes durable knowledge to ephemeral
byproducts (Paper 1 §3.6). It proposes instead "workflow as object, execution as
instantiation, results as knowledge citizens" (Paper 1 §3.6).

- **(finding)**: Paper 1's motivating gap is that workflow knowledge is fragmented because the dominant ontology treats workflow definitions as config, execution as process, and results as disposable output rather than persistent knowledge — source=Paper 1 §3.1/§3.6, confidence=HIGH, type=fact

#### Core model
The central conceptual move is **semantic persistence** (Paper 1 §2.6, Fig 2,
Table 3): a distinction between *execution persistence* (runnable state,
checkpoints, logs, traces, outputs — what systems like LangGraph already do) and
*semantic persistence* (workflow definitions, workflow instances, inference
records, context snapshots, dependency relations modeled as **first-class
knowledge objects** in a shared substrate). The load-bearing formal distinction
is **derive vs infer** (Paper 1 §3.4, App. A):

- **derive** — deterministic, testable, replayable computation over available
  state (scripts, parsers, schema checks, linters, tests, routing rules). Given
  state `S` and deterministic expression `e`, evaluate `e` over `S` → value `v`,
  which may bind into `S'` and persist a `derived-object(v)` + dependency links.
- **infer** — mediated LLM judgment under declared context, requiring an
  explicit prompt, return type/schema, executor validation, persistence as an
  `inference-record`, and an explicit capability policy. Given `S`, context scope
  `C`, prompt `q`, policy `P`: the executor assembles a context-snapshot `CS`,
  the model returns a raw candidate, the executor validates it to a value `v` or
  rejection `r`, persists an `inference-record`, and binds `v` into `S'` **only
  through a declared transition**.

Crucially, a non-LLM nondeterministic operation (seeded RNG, simulation) still
counts as **derive** provided the seed, inputs, and algorithm version make it
replayable (Paper 1 §3.4).

- **(finding)**: Paper 1's core formal distinction is derive (deterministic/replayable) vs infer (LLM-mediated, executor-validated, capability-gated), with the rule that an infer result binds into state ONLY through a declared transition — source=Paper 1 §3.4/App. A, confidence=HIGH, type=architecture
- **(finding)**: Paper 1 frames the surrounding control-and-runtime machinery as "an agent harness" (citing O'Reilly "Agent Harness Engineering", Böckeler/martinfowler, HumanLayer), treating harness engineering as practitioner framing rather than a settled academic field — source=Paper 1 §2.7/§1, confidence=HIGH, type=fact

#### Key mechanisms
Paper 1 proposes a **three-layer conceptual model** (Paper 1 §1, Fig 1):

- **Lower — runtime service layer:** model adapters, tools, external processes,
  persistence/indexing.
- **Middle — control layer ("the DSL machine"):** grammar, object constructors,
  validator, policy layer, executor, knowledge-substrate interface. The executor
  interprets declared objects, assembles context, mediates model/tool calls,
  validates results, and applies permitted transitions.
- **Higher — semantic layer:** workflow definitions, instances, and linked
  inference/approval/panel records.

The **semantic object schema** (Paper 1 §3.3, Table 1) defines kinds:
`workflow-definition`, `workflow-instance`, `input-binding`, `resource-binding`,
`context-snapshot`, `retrieval-record`, `derived-object`, `inference-record`,
`tool-result-record`, `decision-record`, `approval-record`, `panel-record`,
`panel-template`, `dependency-link`, `supersession-link`. The **operating
vocabulary** (Table 2) names: `resource`, `guard`, `context`, `state`, `record`,
`approval`, `panel`, `derive`, `infer`, plus candidate refinements
`capability/action` and `handoff/promotion`.

Two distinctions deserve emphasis for this repo:

- **state vs record** (Paper 1 §3.3): `state` is *active workflow memory during
  execution* (graph state, checkpoint state, session memory); `record` is the
  *durable output/evidence after execution*. State becomes durable evidence only
  when a `record` (or snapshot) promotes it into the substrate.
- **approval vs panel** (Paper 1 §3.3): `approval` is an authorization gate
  (permit/reject/defer) over a transition; `panel` is *structured deliberation*
  (motion, options, arguments, decision, context) that captures a reasoning path
  without treating it as proof of correctness. The §4 scan surfaced `approval`
  as a primitive distinct from `panel` because many artifacts contained
  permission/confirmation gates that were not full deliberations.

**Execution semantics** (Paper 1 §3.5): the executor is the controller; it
creates/resumes a workflow-instance; an interrupted instance that remains in the
substrate *is* a semantic checkpoint. A scoped-reuse rule permits reusing a prior
`panel-record` only when a compatibility policy is satisfied (not general
interchangeability). The DSL deliberately avoids general `goto`, exposing only
`continue/repeat/stop/accept/reject/defer`.

**Safety alignment** (Paper 1 §3.4, App. C) — directly relevant to this repo:
"the LLM output is recorded as an inference result or candidate object;
deterministic validation, policy, and review decide whether it becomes executable
workflow structure," and "capability gating remains an executor policy
requirement: model output is mediated before external actions are executed." An
`infer` site may return a proposed DSL expression, but that **does not** give the
LLM authority over the workflow. A `branch` remains executor-applied even when
its condition depends on an `infer`.

- **(finding)**: Paper 1 separates active state from durable record (state is not durable evidence until a record promotes it), and separates authorization gates (approval) from structured deliberation (panel) — source=Paper 1 §3.3/§4, confidence=HIGH, type=architecture
- **(finding)**: Paper 1's safety model — model output is a candidate; validation/policy/review mediate whether it becomes executable structure; capability gating is an executor requirement — mirrors this repo's "model output is a candidate, never transition authority" invariant — source=Paper 1 §3.4/App. C, confidence=HIGH, type=fact

#### Evidence / experiments
**None.** Paper 1 is explicitly a *conceptual/position* paper (Paper 1 §1: "a
preliminary conceptual account of semantic persistence"; §6: "formal transition
semantics remain future work"). The single quantitative-looking element is a
**vocabulary scan of 77 skill-like artifacts** (Paper 1 §4, App. D: 17
local/internal, 30 Codex/Claude-style, 30 Workflow/OpenClaw), which the paper
states is "design feedback for the vocabulary, NOT empirical validation of the
thesis." It yields per-primitive counts (e.g. `derive` 16/25/30, `infer`
16/20/26, `panel` 8/3/9, `resume` 0/3/16 across the three groups) and qualitative
refinement suggestions (separate `approval` from `panel`; split `context` into
source-authority / inference-visible / operating / progressive-disclosure forms;
separate active state from durable record). There is no baseline, no benchmark,
no reproduction. The authors' planned future artifact is "a small explicit Common
Lisp prototype" (Paper 1 §6), not yet built.

- **(finding)**: Paper 1 has no implementation and no empirical evaluation; its 77-artifact scan is explicitly vocabulary-design feedback, not thesis validation — source=Paper 1 §1/§4/§6/App. D, confidence=HIGH, type=fact

#### Limitations
Stated (Paper 1 §5, §6): formal transition semantics are future work; the
derive/infer boundary is "not mathematically obvious" and is left to the DSL
author's explicit choice; **record-lifecycle policy is not yet chosen**
(retain/compress/supersede/delete, and what provenance survives, are open); the
evaluation of audit/trust/attribution/reproducibility quality is explicitly NOT a
claim of the paper; the PROV-DM mapping (App. C) is "provenance-compatible but
not provenance-complete"; governance/threat models/capability boundaries are
deferred.

Inferred: because there is no implementation, every mechanism is a *design
commitment pending validation*; the "shared knowledge substrate" implies a
query/index layer whose cost and complexity are undiscussed; the model adds many
object kinds whose lifecycle/bloat risk is acknowledged but unresolved.

- **(finding)**: Paper 1's central derive/infer distinction has no formal criterion (left to author choice), and its record-lifecycle (retain/compress/supersede/delete) is unresolved — source=Paper 1 §5/§6, confidence=HIGH, type=fact

#### Provenance summary (Paper 1)
All substantive claims above cite Paper 1 §1, §2.6, §2.7, §3.1, §3.3 (Tables 1–2),
§3.4, §3.5, §3.6, §4 (App. D counts), §5, §6, App. A, App. B, App. C. Full text
retrieved from `arxiv.org/html/2607.08740v1`; no gaps.

---

### Paper 2 — "Remember When It Matters: Proactive Memory Agent for Long-Horizon Agents"

#### Problem / motivation
Paper 2 names a failure mode **"behavioral state decay"** (Paper 2 §1): during
long-horizon execution, decision-relevant state — task requirements, environment
facts, prior attempts, failure diagnoses, intermediate discoveries, open
subgoals — stops influencing the agent's next decision, *even when it is still
present in the transcript/context window* (citing liu2024lost "Lost in the
Middle"). The paper's thesis is that this is distinct from a storage/retrieval
problem: the issue is not that the information is unavailable, but that the agent
fails to *act on* it. Memory, therefore, must decide **when** to intervene, not
just what to store or retrieve.

- **(finding)**: Paper 2's named failure mode "behavioral state decay" is decision-relevant state failing to influence the next decision even when still in context — distinct from storage/retrieval failure — source=Paper 2 §1, confidence=HIGH, type=fact

#### Core model
The core move is to recast memory as an **active intervention policy**, not a
passive retrieval mechanism (Paper 2 §1, §2.5). A **separate memory agent** runs
alongside an **unmodified action agent**. At fixed intervals, the memory agent
(1) updates a structured memory bank from a recent trajectory window, then (2)
decides whether to inject a memory-grounded reminder into the action agent's
next call **or remain silent** (silence is an explicit, first-class action). The
formulation is "memory as a policy over interventions" and "intervention
calibration, not fixed summarization" (Paper 2 §2.5, §3.1).

- **(finding)**: Paper 2's core model is memory-as-active-intervention: a separate agent decides whether/when to inject a reminder or stay silent, reframing memory as a policy over interventions rather than a retrieval function — source=Paper 2 §1/§2.5/§3.1, confidence=HIGH, type=architecture

#### Key mechanisms

**Memory bank schema** (Paper 2 §3.2): `B_t = (s_t, K_t, P_t)` —
- `s_t` — a private **status** field (progress, open issues, unresolved risks)
  that is **never shown to the action agent**;
- `K_t` — **knowledge** memories (stable facts: requirements, environment
  properties, paths, config, verified facts);
- `P_t` — **procedural** memories (attempts + outcomes: failed commands,
  successful fixes, ruled-out hypotheses, diagnostic signals).
Each entry is a short identifier + natural-language content + metadata (creation
time, access stats); identifiers enable explicit update/delete.

**Two-phase memory agent** (Paper 2 §3.3):
- **Phase 1 (memory management)** returns a list of predefined tool calls —
  `memory_update_status`, `memory_save_knowledge`, `memory_save_procedural`,
  `memory_delete` (zero or more, executed in order; no call ⇒ bank unchanged).
  This is a constrained sequence of bank edits, not a free-form summary.
- **Phase 2 (intervention selection + transient injection)** does NOT modify the
  bank; it reads the updated bank + recent trajectory and selects a reminder
  `r_t` or null `∅`. The agent is encouraged to intervene only when a remembered
  item is likely to affect the next decision (a requirement about to be
  violated, an environment fact explaining an observation, an attempt not to
  repeat, a relevant diagnosis, a neglected subgoal) and discouraged from broad
  strategic advice, restating visible information, or taking over planning.

**Triggering** (Paper 2 §3.4): `g(t)` = first step + fixed interval `N`. The
paper notes more selective triggers are possible (after tool errors, failed
tests, repeated commands, large context shifts) but uses a fixed interval to
isolate the effect of the intervention policy itself.

**Learning** (Paper 2 §3.5): the architecture does **not** require training — the
main instantiation is a *prompted* model. Training is explored as a step toward
open-weight viability: train ONLY the memory agent (action frozen). SFT distills
prompted trajectories (teaches the interface + discipline: compact writing,
updating stale state, avoiding unnecessary reminders); GRPO RL then calibrates
intervention (learning when silence is preferable).

- **(finding)**: Paper 2's mechanism is a two-phase memory agent — Phase 1 edits a structured bank via constrained tool calls, Phase 2 independently decides to inject a reminder or stay silent (null is explicit) — with the bank partitioned into private status, knowledge, and procedural memories — source=Paper 2 §3.2/§3.3, confidence=HIGH, type=architecture

#### Evidence / experiments
Paper 2 IS empirical (Paper 2 §4). Benchmarks:

- **Terminal-Bench 2.0** (Paper 2 §4.1): autonomous CLI tasks (inspect files, run
  commands, edit code, debug); 89 tasks, 85 paired valid reported (4 docker
  failures excluded); metric = pass@1 (verifier pass fraction).
- **τ²-Bench** (Paper 2 §4.1): conversational tool-use, 3 domains — airline (50),
  retail (114), telecom (114) = 278 tasks; single-sampled conversation; metric =
  task-evaluator satisfied.

Action agents: Claude Sonnet 4.5 (weaker) + Claude Opus 4.6 (stronger); memory
agent = Claude Opus 4.6 unless stated; k=8 message window; memory invoked every
step.

**Main results** (Paper 2 Table 1, pass@1 %): Terminal-Bench Sonnet 4.5
37.6 → 45.9 (+8.3pp); Opus 4.6 43.5 → 45.9 (+2.4pp). τ²-Bench Sonnet 4.5
task-weighted 55.0 → 61.8 (+6.8pp); Opus 4.6 66.2 → 68.7 (+2.5pp). By domain
(Sonnet 4.5): airline 68 → 78 (+10.0), retail 49.1 → 58.8 (+9.6), telecom
55.3 → 57.9 (+2.6). Gains are larger for the weaker agent but do not vanish for
the stronger one.

**Ablations** (Paper 2 Table 2, τ²-Bench, Sonnet 4.5 action / Opus 4.6 memory;
full = macro 64.3 / micro 61.2): exposing the whole bank every step ("full-bank
context", Phase 1 only) trails full by 2.8 macro; "always inject" (remove
silence) is competitive on micro (+0.3, attributed to run variance) but worse on
macro; "injection-only / no bank" (advisor-style, Phase 2 only) is less stable
and hurts airline vs baseline; a **Mem0** baseline (ADD interface + vector/BM25
top-10 retrieval) improves the average but does not improve airline over
baseline and falls short on macro. Conclusion: neither passive exposure,
always-on, nor generic advisory is sufficient — the best result is maintained
execution-state memory + selective intervention.

**Training study** (Paper 2 Table 4): train Qwen3.5-27B memory agent, freeze
Qwen3.5-122B-A10B action, train on SETA (Terminal-Bench 2.0 held out). On SETA
validation: action-only 0.709/56 → +untrained-27B-memory 0.693/54 (hurts) → +SFT
0.720/58 → +GRPO 0.734/58. Transfer to Terminal-Bench 85-task pass@1: action-only
37.6% → +SETA-trained-27B-memory 41.1% (+3.5pp). Conclusion: the policy is
learnable but needs calibration.

- **(finding)**: Paper 2 reports positive pass@1 gains from a proactive memory agent (+2.4 to +10.0pp across benchmarks/domains), larger for weaker action agents, and shows ablations that selective intervention beats always-on/full-bank/advisor-only/Mem0 — source=Paper 2 Table 1/Table 2, confidence=MED (paper reports no confidence intervals; single-sampled; "within expected run variance" stated qualitatively), type=fact
- **(finding)**: Paper 2's trained open-weight result is partial transfer (+3.5pp on Terminal-Bench vs +8.3pp prompted) and explicitly "preliminary" — source=Paper 2 Table 4/§4.5, confidence=HIGH, type=fact

#### Limitations
Stated and inferred (Paper 2 §4, §5): no confidence intervals or significance
testing (variance addressed only qualitatively); single-sampled conversations
(pass@1, no seed variance); all main-results evaluation uses proprietary frontier
models (Claude) as the prompted memory agent; the "plug-and-play with existing
agent harnesses" claim is **asserted in §1 but not demonstrated on any
third-party harness** — §4 runs the paper's OWN wrapper that injects a transient
memory context into action-agent calls; the trained open-weight result is a
"partial transfer"; remaining failures are calibration errors, not storage
failures. Open directions (§5): jointly train memory + action; learn WHEN memory
is invoked (not fixed schedule); distinguish verbatim reminders from
task-specific abstractions.

- **(finding)**: Paper 2's "plug-and-play with existing agent harnesses" claim is asserted (§1) but the experiments (§4) run the paper's own interposition wrapper, not a third-party harness — an over-claim relative to the evidence presented — source=Paper 2 §1 vs §3.1/§3.3/§4, confidence=HIGH, type=inference

#### Provenance summary (Paper 2)
All substantive claims above cite Paper 2 §1, §2.5, §3.1, §3.2, §3.3, §3.4, §3.5,
§4.1, Table 1, Table 2, Table 3, Table 4, §4.5, §5. Full text retrieved from
`arxiv.org/html/2607.08716v1`; only the final reference-list bibliography tail
(~935 bytes) was truncated — all cited works appear inline in the body, so no
substantive gap.

---

### Cross-cut — how the two papers relate

**Memory vs workflow.** Paper 1 is fundamentally about *workflow* persistence:
making definitions, instances, inferences, and decisions durable, queryable,
first-class knowledge objects in a shared substrate. Paper 2 is fundamentally
about *memory* as an *intervention*: a transient, just-in-time reminder injected
into the action agent's next call, with the structured bank rebuilt from recent
trajectory windows. These are complementary layers, not rivals — paper 1 asks
"what deserves to be a durable knowledge citizen," paper 2 asks "when should
remembered state re-enter the context to change the next decision." But they sit
at a genuine tension point: paper 1 wants *more* durable persistence (records as
knowledge citizens); paper 2's bank is deliberately *less* durable — its private
status field `s_t` is never shown to the action agent, and the bank is
re-derived from the live trajectory rather than serving as a long-lived
substrate.

**derive-vs-infer (P1) vs proactive-inject (P2).** These address different
mediation questions. Paper 1's derive/infer boundary is about *what kind of
operation* produced a piece of state (deterministic vs LLM-mediated) and ensuring
the LLM never gains transition authority. Paper 2's inject-vs-silent decision is
about *whether remembered state should influence the next action* and ensuring
memory intervenes only when it will change a decision. Notably, **both papers
converge on a non-authority principle for model output**: paper 1 ("model output
is not transition authority"; branch stays executor-applied even when condition
depends on infer — §3.4/App. C) and paper 2 (the memory agent's reminder is
advisory; the action agent is unmodified and free to ignore it; Phase 2 is
discouraged from taking over planning — §3.3). This is strong cross-paper
corroboration of this repo's "model output is a candidate, never transition
authority" invariant.

**Persistence models.** Paper 1 = richly-typed durable objects in a shared,
queryable substrate with dependency/supersession links; paper 2 = a compact
three-bucket bank (status/knowledge/procedural) that is updated in place by
explicit tool calls and serves transient injection. Paper 1's model is the
heavier, more ambitious one (and unbuilt); paper 2's is the lighter, empirically
tested one (but tied to its own wrapper).

**Authority/governance.** Both defer the hard governance questions: paper 1
leaves capability boundaries, threat models, and record-lifecycle policy open
(§5/§6); paper 2 leaves intervention-calibration and joint training open (§5).
Neither paper provides a threat model.

- **(finding)**: The two papers are complementary (P1 = what to persist as workflow knowledge; P2 = when remembered state should intervene), and both independently converge on a non-authority principle for model output that matches this repo's safety invariant — source=Paper 1 §3.4/App. C + Paper 2 §3.3, confidence=HIGH, type=inference

---

## Findings (consolidated)

- **(finding)**: Paper 1's core distinction is derive (deterministic/replayable) vs infer (LLM-mediated, executor-validated, capability-gated), with infer binding into state ONLY through a declared transition — source=Paper 1 §3.4/App. A, confidence=HIGH, type=architecture
- **(finding)**: Paper 1 separates active state from durable record, and authorization gates (approval) from structured deliberation (panel) — source=Paper 1 §3.3/§4, confidence=HIGH, type=architecture
- **(finding)**: Paper 1's safety model ("model output is a candidate; validation/policy/review mediate whether it becomes executable structure; capability gating is an executor requirement") mirrors this repo's invariant — source=Paper 1 §3.4/App. C, confidence=HIGH, type=fact
- **(finding)**: Paper 1 has no implementation and no empirical evaluation; its 77-artifact scan is explicitly vocabulary-design feedback only; its derive/infer boundary has no formal criterion and record-lifecycle is unresolved — source=Paper 1 §1/§4/§5/§6, confidence=HIGH, type=fact
- **(finding)**: Paper 1 frames the surrounding control-and-runtime machinery as "an agent harness" (citing O'Reilly, Böckeler/martinfowler, HumanLayer), as practitioner framing not settled academia — source=Paper 1 §2.7/§1, confidence=HIGH, type=fact
- **(finding)**: Paper 2's named failure mode "behavioral state decay" is decision-relevant state failing to influence the next decision even when still in context — distinct from storage/retrieval — source=Paper 2 §1, confidence=HIGH, type=fact
- **(finding)**: Paper 2's mechanism is a two-phase memory agent (Phase 1 edits a status/knowledge/procedural bank; Phase 2 independently injects a reminder or stays silent) running alongside an unmodified action agent — source=Paper 2 §3.2/§3.3, confidence=HIGH, type=architecture
- **(finding)**: Paper 2 reports positive pass@1 gains (+2.4 to +10.0pp) with selective intervention beating always-on/full-bank/advisor-only/Mem0 ablations — source=Paper 2 Table 1/Table 2, confidence=MED (no CIs; single-sampled; variance addressed qualitatively), type=fact
- **(finding)**: Paper 2's trained open-weight result is partial transfer (+3.5pp vs +8.3pp prompted) and explicitly preliminary — source=Paper 2 Table 4/§4.5, confidence=HIGH, type=fact
- **(finding)**: Paper 2's "plug-and-play with existing agent harnesses" claim is asserted but not demonstrated on any third-party harness (experiments run the paper's own interposition wrapper) — source=Paper 2 §1 vs §3.1/§3.3/§4, confidence=HIGH, type=inference
- **(finding)**: Both papers independently converge on a non-authority principle for model output (P1: model output is not transition authority; P2: the reminder is advisory, action agent unmodified), corroborating this repo's safety invariant — source=Paper 1 §3.4/App. C + Paper 2 §3.3, confidence=HIGH, type=inference

## Contradictions

<!-- Explicit contradiction audit: between the papers, within each paper, and between each paper's claims and THIS REPO's model. -->

**Between the two papers:**

- *Persistence philosophy tension (not a hard contradiction):* Paper 1 wants
  *more* durable persistence — records as first-class knowledge citizens in a
  queryable substrate (§3.1/§3.6). Paper 2's bank is deliberately *less* durable
  — its status field is never shown to the action agent, and the bank is rebuilt
  from recent trajectory windows for transient injection (§3.2). They target
  different layers, so this is a complementarity, not a logical conflict — but an
  adopter cannot take both at face value without resolving "durable knowledge
  citizen" vs "transient intervention buffer" for a given surface.
- *No contradiction on model authority:* both papers agree the LLM's output is
  non-authoritative (P1 §3.4/App. C; P2 §3.3). **Alignment, not conflict.**

**Within a paper:**

- *Paper 2 — claim vs evidence (the over-claim):* §1/abstract asserts "plug-and-play
  with existing agent harnesses," but §3.1/§3.3 require interposing a transient
  memory context into the action agent's next call, and §4 runs the paper's OWN
  wrapper — no third-party harness is demonstrated. The claim outruns the
  evidence. **Flagged as over-claim.**
- *Paper 2 — ablation undercut:* Table 2's "always inject" variant is competitive
  with the full selective agent on micro-avg (+0.3), which slightly undercuts the
  "selective silence is essential" thesis. The paper attributes this to run
  variance and notes macro is worse — an acknowledged, minor internal tension.
- *Paper 1 — central distinction has no formal criterion:* §5 concedes the
  derive/infer boundary is "not mathematically obvious" and defers to the DSL
  author's explicit choice. This is honest, not a contradiction, but it means the
  load-bearing distinction is currently a *convention*, not a *theorem*.
- *Paper 1 — no experiments:* by design (position paper), so no claim-vs-evidence
  contradiction is possible; the limitation is the absence of validation, which
  the paper states explicitly.

**Between a paper's claims and THIS REPO's actual model (over-claim / conflict audit):**

- *Paper 2 "works with existing agent harnesses" vs this repo:* OpenCode's
  coordination model has **no mid-turn injection hook** into a running action
  agent's context. The `coordination` primary agent is strictly read-only and
  *delegates* all work; memory surfacing is **explicit-invocation**
  (session-start / handoff / checkpoint), NOT a shadow agent injecting per
  interval (`templates/docs/opencode-memory-model.md` → "Injection rules … by
  explicit invocation … not always-on"). Paper 2's architecture does **not**
  plug-and-play onto this repo without building a new interposition seam that
  this repo's safety model does not currently expose. **Over-claim relative to
  this repo.**
- *Paper 2 fixed-interval injection vs this repo's Anti-spam rule:* Paper 2 §3.4
  triggers memory every `N` steps (always-on-by-clock). This repo's Anti-spam
  rule states a memory file should enter context "only if it changes the next
  action or prevents a repeated mistake," and injection is "by explicit
  invocation … not always-on." **Direct conflict** — fixed-interval injection is
  rejected here (see candidate C9).
- *Paper 2 "memory agent" as a second LLM vs this repo's read-only coordinator:*
  A shadow LLM agent making injection decisions would be a new influence path.
  The repo's coordinator-must-stay-read-only rule and the
  model-output-is-candidate invariant mean a second LLM's reminder must remain
  advisory and explicitly-invoked, not a live per-turn injection. **Tension**
  (resolved by keeping paper 2's *discipline* but rejecting its *live shadow
  agent* — see candidates C5/C6 adopt vs C8 reject).
- *Paper 1 "shared knowledge substrate" + queryable workflow-instances vs this
  repo's ownership/flat-file model:* This repo deliberately partitions state by
  **ownership class** (`internal/ownership/`: `platform_managed`,
  `overlay_extension`, preserved, seeded-once, schema-reconciled) and keeps flat
  files canonical with typed records additive only. A single shared substrate
  holding workflow-instances as persistent queryable objects would blur the
  ownership classification (who owns a workflow-instance?) and implies a
  query/index layer this repo explicitly avoids (no vector store; keyword/grep
  over flat JSONL — see the TencentDB precedent's explicit rejection of a vector
  store). **Tension** with the flat-file + no-vector-store + ownership
  contracts — resolved by rejecting the wholesale substrate (C4) while keeping
  the narrower vocabulary pieces.
- *Paper 1 derive/infer vs this repo:* **No contradiction.** The repo already
  implements exactly this boundary via the commit-gate, `commit-reviewer`, the
  ownership safety contract, and the "model output is a candidate" invariant.
  **Strong corroboration.**

---

## Part B — Harness-adoption candidates appendix (analysis only)

> **These are analysis candidates, NOT captured tasks.** No `.local/coordinator/tasks/`
> card was written and no `docs/planning/backlog.md` row was touched. Each
> candidate carries a `trigger:` predicate for the coordinator's *later*
> curation under the DEFER/follow-up DoR; none is executed here. Verdicts are
> this researcher's recommendation for that later review, not active repo policy.

### C1 — Adopt the derive/infer vocabulary as explicit naming for the existing gate/review split
- **idea:** Name the repo's existing "deterministic check vs LLM-mediated proposal"
  boundary with paper 1's `derive`/`infer` terms in durable docs.
- **source:** Paper 1 §3.4, Table 2, App. A (derive/infer definitions); App. C
  ("model output is not transition authority").
- **mechanism:** Paper 1 defines `derive` = deterministic/testable/replayable
  computation; `infer` = LLM-mediated judgment validated by the executor and
  bound into state only through a declared transition, with capability gating an
  executor requirement.
- **boundary touched:** the derive-vs-infer distinction vs this repo's current
  gate/review/promotion split (commit-gate, `commit-reviewer`, ownership safety
  contract, "model output is a candidate" invariant).
- **trigger predicate:** `trigger:area_touched(safety-invariant)` or
  `trigger:path_touched(AGENTS.md)`.
- **rough file scope:** `AGENTS.md` (safety-invariant section — read-only here,
  flagged as promotion target); possibly a short `docs/ai/` note.
- **validation-plan sketch:** docs-only. Confirm the named distinction maps 1:1
  to existing enforcement (commit-gate = declared transition; commit-reviewer =
  executor validation; ownership = capability gating). A reviewer should be
  unable to find a place where an `infer` output becomes state without a gate.
- **risks / rejection rationale:** Low. Purely a naming/teaching improvement that
  sharpens an invariant already enforced. No conflict with any non-negotiable
  rule; it *strengthens* the model-output-is-candidate invariant by giving it a
  citation and a crisper vocabulary.
- **verdict:** **adopt** (as doc vocabulary / promotion target).

### C2 — Enrich checkpoints with an inference-record + context-snapshot + dependency-link structure
- **idea:** Treat each LLM-mediated decision captured in a checkpoint/decision-log
  as a typed inference-record carrying its bounded context-snapshot and explicit
  dependency links to the records it informed.
- **source:** Paper 1 §3.3 (Table 1: `inference-record`, `context-snapshot`,
  `dependency-link`), §3.4, App. C.
- **mechanism:** Paper 1 persists each inference with the context that was
  visible to it and typed links to downstream branches, so the decision is
  reviewable in its original bounded context later.
- **boundary touched:** checkpoint + session memory
  (`.opencode/state/sessions/<alias>/memory/`); optionally the typed-records
  store (`internal/memory/record/`).
- **trigger predicate:** `trigger:area_touched(memory-model)` or
  `trigger:path_touched(.vh-agent-harness/docs/opencode-session-workflow.md)`.
- **rough file scope:** checkpoint template guidance in
  `.vh-agent-harness/docs/opencode-session-workflow.md` (Verification table
  already partially does this); `decision-log.md` convention; possibly a new
  record `type` in `internal/memory/record/record.go`.
- **validation-plan sketch:** Produce one sample checkpoint that links an infer
  decision to (a) the context-snapshot that was visible and (b) the downstream
  record it enabled; confirm a reviewer can reconstruct the decision's basis
  without re-reading the whole session.
- **risks / rejection rationale:** Lifecycle/bloat — paper 1 itself flags
  record-lifecycle as unresolved (§5). Must not violate the Anti-spam rule
  (these records stay retrieval-only unless they change the next action). Risk
  of duplicating what the checkpoint Verification table + decision-log already
  capture. Needs a study pass before committing to a schema.
- **verdict:** **study-more**.

### C3 — Make the approval-vs-panel distinction explicit in the review vocabulary
- **idea:** Distinguish simple authorization gates (`approval`: permit/reject/defer)
  from structured multi-option deliberation (`panel`: motion/options/arguments)
  when naming the repo's review surfaces.
- **source:** Paper 1 §3.3 (approval vs panel definitions), §4 (scan surfaced
  approval as distinct from panel), App. A.
- **mechanism:** Paper 1 separates a binary-ish authorization gate from a
  deliberative record that captures a reasoning path (motion, options,
  arguments) without treating it as proof of correctness.
- **boundary touched:** the gate/review/promotion split — specifically
  `commit-reviewer` (tiered cascade ≈ panel-like) vs `commit-gate` /
  `task-review` (≈ approval-like).
- **trigger predicate:** `trigger:area_touched(review-vocabulary)` or
  `trigger:path_touched(docs/coordination/)`.
- **rough file scope:** `docs/coordination/` review guidance;
  `.opencode/agents/` review agent docs; `.opencode/skills/gated-commit/`.
- **validation-plan sketch:** Map each existing review surface to approval vs
  panel; check whether any surface is currently overloaded (doing both jobs under
  one name) and whether renaming/clarity would help routing.
- **risks / rejection rationale:** Over-formalization — the repo's review is
  already structured (tiered cascade, fail-fast escalation). The gain may be
  marginal. No conflict with non-negotiable rules.
- **verdict:** **study-more**.

### C4 — Full semantic-persistence substrate / workflow-instances as first-class knowledge objects
- **idea:** Model all workflows, instances, inferences, and context snapshots as
  persistent queryable objects in a shared knowledge substrate (paper 1's whole
  proposal).
- **source:** Paper 1 §1, §3.1, §3.3 (Table 1), §3.5/§3.6.
- **mechanism:** A shared substrate holding workflow-definitions/instances/
  records as first-class objects with dependency and supersession links, queryable
  alongside the knowledge they produced.
- **boundary touched:** corpus/embedding + ownership + lineage + the entire
  runtime model.
- **trigger predicate:** `trigger:area_touched(substrate-architecture)` (intentionally
  high-bar; this is a product-level change).
- **rough file scope:** cross-cutting — would touch `internal/ownership/`,
  `internal/lineage/`, `internal/substrate/`, `internal/schema/`, and the flat-file
  memory model.
- **validation-plan sketch:** N/A — this is a different product, not an increment.
- **risks / rejection rationale:** Conflicts with multiple non-negotiable rules:
  the flat-file + no-vector-store contract (a queryable substrate implies an
  index/query layer the repo explicitly avoids — see the TencentDB precedent's
  vector-store rejection); the ownership safety contract (a substrate holding
  workflow-instances blurs the `platform_managed`/preserved line); and the
  simplicity/determinism engineering defaults. Paper 1 itself has no
  implementation and leaves formal semantics + lifecycle as future work. Taking
  it wholesale would be building an unbuilt product.
- **verdict:** **reject** (as wholesale architecture; the useful pieces are
  captured in C1/C2/C3 instead).

### C5 — Name "behavioral state decay" as a documented compaction failure mode
- **idea:** Adopt paper 2's term for the failure mode where decision-relevant
  state stops influencing the next decision even when still in context, and tie
  it to the existing compaction discipline.
- **source:** Paper 2 §1 (behavioral state decay definition), §2.5.
- **mechanism:** Paper 2 names the failure and ties it to context-window
  attention decay (liu2024lost), arguing it is an *intervention* problem, not a
  storage problem.
- **boundary touched:** "behavioral state decay" vs this repo's compaction +
  memory rules — specifically the "compaction is aggressive" guidance and the
  Anti-spam rule.
- **trigger predicate:** `trigger:area_touched(memory-model)` or
  `trigger:path_touched(templates/docs/opencode-memory-model.md)`.
- **rough file scope:** `templates/docs/opencode-memory-model.md` (Anti-spam /
  compaction section); `.vh-agent-harness/docs/opencode-session-workflow.md`
  (Memory rules / compaction).
- **validation-plan sketch:** Docs-only. Add the named failure mode next to the
  existing compaction/Anti-spam guidance and verify the existing discipline
  (explicit-invocation injection, brief.md/next-slice eligibility) already
  mitigates it.
- **risks / rejection rationale:** None. Vocabulary adoption that sharpens
  existing discipline; no conflict with any rule.
- **verdict:** **adopt** (as named failure mode in compaction discipline).

### C6 — Two-phase memory discipline: separate "maintenance" from "injection"; silence is first-class
- **idea:** Adopt paper 2's separation of *writing* memory (Phase 1) from
  *deciding whether to surface any of it this turn* (Phase 2, where null/silence
  is explicit), as a refinement of the repo's injection discipline.
- **source:** Paper 2 §3.3 (Phase 1 bank edits vs Phase 2 intervention-or-null),
  §3.2, §4.3 (ablation: always-on/full-bank underperform selective).
- **mechanism:** Phase 1 updates the durable bank; Phase 2 independently asks
  "does any of this change the next action?" and stays silent otherwise. The
  ablations show selective intervention beats always-on exposure.
- **boundary touched:** checkpoint + session memory; the typed-records injection
  rules (`templates/docs/opencode-memory-model.md` → "Injection rules"). The
  repo's "inject by explicit invocation … not always-on" + Anti-spam rule is a
  coarser version of this; paper 2 sharpens the *selection* step.
- **trigger predicate:** `trigger:area_touched(memory-model)` or
  `trigger:path_touched(templates/docs/opencode-memory-model.md)`.
- **rough file scope:** `templates/docs/opencode-memory-model.md` (Anti-spam +
  Injection rules sections); checkpoint-injection guidance in
  `.vh-agent-harness/docs/opencode-session-workflow.md`.
- **validation-plan sketch:** Docs-discipline. Confirm the Anti-spam rule already
  expresses Phase 2's intent ("enter context only if it changes the next
  action"); sharpen the checkpoint-injection step to make the "stay silent"
  outcome explicit rather than implied.
- **risks / rejection rationale:** Must NOT become always-on injection — paper 2
  injects every `N` steps, which this repo rejects (see C9). The transplant is
  the *selection discipline*, not the *fixed-interval trigger*. No conflict with
  non-negotiable rules when scoped this way.
- **verdict:** **adopt** (as injection-discipline sharpening; explicitly NOT the
  fixed-interval trigger).

### C7 — Map paper 2's status/knowledge/procedural bank split against the repo's record enum
- **idea:** Check whether the repo's typed-record `type` enum
  (`persona`|`episodic`|`instruction`) and session files
  (`task-contract.md`/`open-questions.md`/`decision-log.md`) already cover paper
  2's `s_t`/`K_t`/`P_t` buckets, before considering any new type axis.
- **source:** Paper 2 §3.2 (B_t = status s_t + knowledge K_t + procedural P_t).
- **mechanism:** Paper 2 partitions memory into private status, stable knowledge,
  and procedural attempts+outcomes, each updatable/deletable by identifier.
- **boundary touched:** typed records store — `internal/memory/record/record.go`
  (the `type` enum) and the session/workstream file model.
- **trigger predicate:** `trigger:area_touched(memory-model)` or
  `trigger:path_touched(internal/memory/record/record.go)`.
- **rough file scope:** `internal/memory/record/record.go`; the memory-model doc.
- **validation-plan sketch:** Produce the bucket→file/type mapping
  (`s_t` status ≈ task-contract.md + open-questions.md; `K_t` knowledge ≈
  instruction/persona records; `P_t` procedural ≈ episodic records +
  decision-log). Confirm there is no gap that a new type would fill better than
  an existing file.
- **risks / rejection rationale:** Type-axis explosion / shadowing. The repo
  deliberately keeps flat files canonical with records additive only; adding
  `procedural`/`status` record types risks duplicating task-contract/decision-log
  content under a second representation. Likely "covered by existing files."
- **verdict:** **study-more** (lean toward: no new type needed; existing split
  covers the buckets).

### C8 — Live shadow memory-agent running alongside the action agent
- **idea:** Run a separate memory agent that observes the action agent's
  trajectory, maintains a bank, and proactively injects reminders mid-trajectory.
- **source:** Paper 2 §1, §3.1, §3.3 (memory agent π_M alongside unmodified π_A).
- **mechanism:** A second agent watches the live trajectory and injects transient
  memory context into the action agent's next call.
- **boundary touched:** the coordinator/read-only-delegation model; the
  model-output-is-candidate invariant; the explicit-invocation injection model.
- **trigger predicate:** `trigger:area_touched(coordinator-runtime)` (high-bar).
- **rough file scope:** would require a new interposition seam not present in the
  repo; cross-cutting.
- **validation-plan sketch:** N/A at repo level (see rejection).
- **risks / rejection rationale:** (a) Over-claim — paper 2 runs its OWN wrapper
  that interposes on action-agent calls; "plug-and-play with existing agent
  harnesses" is asserted, not demonstrated on OpenCode or any third-party
  harness. OpenCode's seams do not expose a per-turn injection hook into a
  running agent's context. (b) Conflicts with the explicit-invocation Anti-spam
  discipline. (c) Adds inference cost (paper 2 admits this). (d) A shadow LLM
  injecting into the action agent's turns is a new influence path the safety
  model does not currently mediate. The *discipline* is captured via C5/C6 at
  checkpoint boundaries; the *live agent* is rejected.
- **verdict:** **reject** (as live shadow agent; discipline salvaged via C5/C6).

### C9 — Fixed-interval memory invocation trigger
- **idea:** Invoke memory surfacing every N steps.
- **source:** Paper 2 §3.4 (g(t) = first step + fixed interval N).
- **mechanism:** Periodic, clock-driven memory injection.
- **boundary touched:** the injection trigger model.
- **trigger predicate:** N/A (rejected).
- **rough file scope:** none.
- **validation-plan sketch:** N/A.
- **risks / rejection rationale:** Directly conflicts with the Anti-spam rule
  ("enter context only if it changes the next action") and the explicit-invocation
  discipline ("inject by explicit invocation … not always-on"). OpenCode has no
  mid-turn injection hook. Always-on-by-clock is the opposite of this repo's
  model.
- **verdict:** **reject**.

### C10 — Selective signal-triggered surfacing as non-blocking coordination toasts
- **idea:** Surface memory/coordination hints on signal events (after tool errors,
  failed tests, repeated commands, large context shifts) as non-blocking toasts,
  rather than on a fixed clock.
- **source:** Paper 2 §3.4 (notes more selective triggers possible: after tool
  errors, failed tests, repeated commands, large context shifts).
- **mechanism:** Event-driven, not clock-driven, intervention triggers.
- **boundary touched:** hooks (`internal/hooks/`) + the coordination-toast model.
  The repo ALREADY emits "non-blocking coordination toasts after edits when a
  turn crosses coordination boundaries or grows a code file too far" (per the
  session-workflow doc).
- **trigger predicate:** `trigger:area_touched(coordination-toasts)` or
  `trigger:path_touched(internal/hooks/)`.
- **rough file scope:** `internal/hooks/`; the `.opencode` coordination plugin.
- **validation-plan sketch:** Evaluate whether enriching the toast trigger
  predicates (tool-error / test-failure / repeated-command signals) reduces
  repeated mistakes without becoming spam, and confirm toasts stay *hints* (not
  forced injection, which would violate Anti-spam).
- **risks / rejection rationale:** Must stay non-blocking hints — paper 2's
  injections are authoritative context; this repo's toasts are explicitly "hints,
  not policy overrides." Promoting them to forced injection would violate the
  Anti-spam rule. Needs a study pass to confirm the signal-to-noise tradeoff.
- **verdict:** **study-more**.

### C11 — Cross-validate and reinforce the "model output is a candidate" invariant with both papers
- **idea:** Cite the cross-paper convergence on model-output-non-authority as
  corroboration for the repo's safety invariant, and flag the invariant section
  as a promotion target if the docs ever need strengthening.
- **source:** Paper 1 §3.4/App. C ("model output is not transition authority";
  "capability gating remains an executor policy requirement") + Paper 2 §3.3
  (Phase 2 reminder is advisory; action agent unmodified and free to ignore).
- **mechanism:** Both papers independently require that LLM output be mediated
  before it affects state — paper 1 via the derive/infer boundary + executor
  validation, paper 2 via advisory-only reminders.
- **boundary touched:** the safety invariant itself.
- **trigger predicate:** `trigger:area_touched(safety-invariant)`.
- **rough file scope:** `AGENTS.md` (safety-invariant section — read-only here,
  flagged as promotion target); possibly a `docs/checkpoints/` decision note.
- **validation-plan sketch:** Confirm the three (paper 1 / paper 2 / repo) agree
  on the principle; if a future change weakens the invariant, this corroboration
  is the counter-evidence.
- **risks / rejection rationale:** None. Strengthens an existing invariant with
  external corroboration; no conflict.
- **verdict:** **adopt** (as corroborating citation / promotion-target flag).

### C12 — Record-lifecycle policy: consider a supersession-link instead of silent overwrite
- **idea:** When a typed record is updated, consider preserving the prior version
  with an explicit supersession link (old survives + links to new) rather than
  silent last-write-wins, for reviewable inferences.
- **source:** Paper 1 §3.3 (`supersession-link`), §5 (record-lifecycle
  retain/compress/supersede/delete is open); Paper 2 §3.2 (`memory_delete` +
  update-by-id, last-write-wins).
- **mechanism:** Paper 1's supersession-link preserves an old inference and links
  the new one; paper 2's simpler model is update-by-id / delete-by-id.
- **boundary touched:** typed records store — `internal/memory/record/record.go`,
  `internal/memory/store/store.go`.
- **trigger predicate:** `trigger:path_touched(internal/memory/store/store.go)`.
- **rough file scope:** `internal/memory/store/store.go` (reader dedup + a
  possible supersession relation); `internal/memory/record/record.go`.
- **validation-plan sketch:** Determine whether a review-loss problem exists
  today when a record is overwritten (probably low-frequency at this repo's
  human-scale, per-session size); if so, prototype a supersession-link read path.
- **risks / rejection rationale:** Bloat; the repo is human-scale per-session.
  Paper 1 itself flags lifecycle as open; paper 2's simpler delete/overwrite may
  suffice. Low priority.
- **verdict:** **study-more** (low priority).

---

## Closeout

- **Memo path:** `researches/sources/2026-07-13-agent-harness-papers-synthesis.md`
- **Time-sensitivity:** FRESH (papers 4 days old) but conceptually STABLE.
  Paper 2's empirical numbers are model/benchmark-specific; both papers'
  architectural claims are stable.
- **Confidence:** HIGH on both papers' conceptual models and on the repo
  grounding (full paper text retrieved; repo files read directly). MED on
  paper-2's benchmark deltas (no CIs, single-sampled, variance addressed
  qualitatively). LOW on paper-2's "works with existing agent harnesses" claim
  as applied to THIS repo (over-claim; not demonstrated on OpenCode).
- **Coverage:**
  - Paper 1 — FULL TEXT retrieved (all of §1–§7 + Appendices A/B/C/D +
    references; Tables 1–3; vocabulary-scan counts). No gaps.
  - Paper 2 — FULL TEXT retrieved (all of §1–§5 + Tables 1–4). Only the final
    reference-list bibliography tail (~935 bytes) was truncated; all cited works
    appear inline in the body, so no substantive gap.
- **Disposition tally (adoption candidates, analysis only):** 12 candidates —
  **4 adopt** (C1 derive/infer vocabulary; C5 behavioral-state-decay naming;
  C6 two-phase maintenance-vs-injection discipline; C11 model-output-non-authority
  reinforcement), **5 study-more** (C2 inference-record/context-snapshot
  enrichment; C3 approval-vs-panel vocabulary; C7 status/knowledge/procedural
  bucket mapping; C10 signal-triggered toasts; C12 supersession-link lifecycle),
  **3 reject** (C4 wholesale semantic-persistence substrate; C8 live shadow
  memory-agent; C9 fixed-interval trigger).
- **Read-only compliance:** Only the one staged memo file (under `researches/sources/`) was
  written. No code, no `AGENTS.md`, no `docs/planning/backlog.md` row, no
  `.local/coordinator/tasks/` card was touched. No git mutations. Candidate
  capture is deferred to a later coordinator step (each candidate carries a
  `trigger:` predicate for that curation under the DEFER/follow-up DoR).
- **Promotion targets (flagged, not executed):** C1 and C11 point at the
  `AGENTS.md` safety-invariant section and possibly a `docs/ai/` note; C5 and C6
  point at `templates/docs/opencode-memory-model.md` and
  `.vh-agent-harness/docs/opencode-session-workflow.md`. These are the live docs
  a follow-up slice would update IF a candidate is promoted — not updated here.
- **Cross-reference:** This memo complements
  `researches/sources/2026-07-08-tencentdb-agent-memory-study.md`, which
  delivered the typed-records layer (R1–R3) that C2, C7, and C12 would extend.
  The TencentDB precedent's explicit rejection of a vector store and a runtime-
  hook injection model is directly relevant to rejecting C4 (substrate) and C8
  (live shadow agent).
