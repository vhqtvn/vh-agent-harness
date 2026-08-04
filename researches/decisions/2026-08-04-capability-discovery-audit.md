# Capability Discovery and Rediscovery Audit Brief

> **Status:** DESIGN MEMO. Top fix (surface-at-friction generalization) SIGNED OFF with observed instance (commit 2b903a1). Remaining 8 fix entries HELD pending Gate A–D evidence (debate returned need_evidence). Promoted to committed canon from tmp/ on operator sign-off.
>
> **Provenance:** Produced by solution-brief chain (researcher → debate → planner). Two fold-in additions integrated after the chain ran (marked `[FOLD-IN ...]`), supplied by the operator with verified evidence. Disposition section appended.
>
> **Authority/projection framing (E24) is the load-bearing finding** — see §3 E24 and the §11 callout. A project-wide review will build on it.
>
> **Closure verdict:** `behavioral-closure: verdict: inconclusive, result: not-demonstrable` — design brief only; no behavior executed; promotion deferred to committer under sign-off.

---

## 1. Status and decision frame

**Objective:** Determine why agents fail to discover or rediscover capabilities that the agent harness already provides, then rank the applicable discovery fixes.

**Exact open question:** Which discovery surfaces should be changed first to prevent repeated capability misses without increasing baseline context unnecessarily or creating competing sources of truth?

### Governing decision

Use this audit sequence:

1. **Friction-surface census**
2. **Grant-to-prompt parity**
3. **Rediscovery survivability**
4. Apply an **authority/projection consistency rule** across every phase
5. Use bounded historical incidents to calibrate frequency

Configuration and permission declarations remain authoritative. Prompts, descriptions, documentation, and friction messages are non-authoritative projections that must accurately expose that authority.

## 2. Evidence classification

- **FACT:** Directly established by current repository state or cited history.
- **ASSUMPTION:** Accepted premise not independently established by this audit.
- **PREDICTION:** Expected consequence of a proposed change.
- **PREFERENCE:** Selected design principle or prioritization rule.

## 3. Compact evidence register

| ID | Classification | Finding | Evidence |
|---|---|---|---|
| E01 | FACT | `exec-ro` denial is the positive control: denial stays final while surfacing the floor-aware sanctioned ladder. | `internal/execro/classifier.go:25-83`; commit `2b903a1` |
| E02 | FACT | The read-only policy defines Levels A/B/C and distinguishes classifier denial from actual permission denial. | `.opencode/docs/agents/read-only-execution-policy.md:17-54,125-159` |
| E03 | FACT | The policy prohibits fallback rerunning of a classifier-denied command through another verb. | `.opencode/docs/agents/read-only-execution-policy.md:125-149`; `README.agent.md:695-706` |
| E04 | FACT | `media-perception`, `repo-explorer`, and `researcher` are the current `exec-sandbox` grant holders. | `opencode.jsonc:1513,1963,2065` |
| E05 | FACT | Those prompts point to the policy but do not plainly state that the agent holds the grant. | `.opencode/agents/media-perception.md:194`; `.opencode/agents/repo-explorer.md:26-33`; `.opencode/agents/researcher.md:59-64` |
| E06 | PREDICTION | A concise own-grant projection may reduce unnecessary escalation. Its causal effect has not yet been measured. | Derived from E04–E05 |
| E08 | FACT | Skill discovery is cached per process; only process restart refreshes it. Update emits a restart hint when skill files change. | `AGENTS.md:130`; `README.agent.md:1078-1088`; `internal/cli/update.go:268-293` |
| E09 | FACT | There are 14 currently rendered skills. | `.opencode/skills/`; `.vh-agent-harness/overlays/*-pilot/skills/*/SKILL.md` |
| E10 | FACT | The current all-skill description total is 4,283 characters. | Current `.opencode/skills/*/SKILL.md` measurement |
| E11 | FACT | The three pilot descriptions are 225, 214, and 218 characters after `edd5ea5`. | Commit `edd5ea5`; current pilot `SKILL.md` frontmatter |
| E12 | FACT | All 14 descriptions contain trigger language; the largest are `think-mode`, `repo-recon`, and `media-perception`. | Current skill frontmatter |
| E13 | FACT | The current command roster contains 39 command descriptions, and baseline guidance names preferred commands including `/task-delete`. | `.opencode/commands/*`; `AGENTS.md:321` |
| E14 | FACT | `/task-delete` has strong refusal-time guidance, including refusal codes and deliberate-force semantics. | `.opencode/commands/task-delete.md:7-53` |
| E15 | FACT | DEFER transport and promotion rules have substantial baseline and workflow coverage. | `AGENTS.md:382-425`; `docs/coordination/README.md:48-85` |
| E18 | FACT | Compaction primitives retain selected operating rules but not complete skill, command, grant, or DEFER catalogs. | `templates/core/.opencode/plugins/compaction-primitives.js:1-35` |
| E19 | PREDICTION | A capability can cease to be salient after compaction unless it is re-derived from durable state. | E18; `AGENTS.md:292-297` |
| E20 | FACT | Current `vh-solara` has the three inspected `exec-sandbox` grants and policy pointers. | `../vh-solara/opencode.jsonc:1440,1896,1998`; corresponding rendered agent prompts |
| E23 | FACT | Shell-guard denials are expected to provide a reason and canonical alternative. | `AGENTS.md:125-126,138-184`; `templates/core/.opencode/repo-configs/forbidden-patterns.project.js:24` |
| E24 | PREFERENCE | Permission/configuration should be the authority; prompts and messages should be checked as projections. | Synthesis of E01–E23 |
| E25 | FACT | The `exec-sandbox` grant SHIPS into a consumer's rendered `opencode.jsonc`, but the `exec_sandbox.min_mode` floor CANNOT ship: `run-shape.yml` is project-owned and seeded only when absent. | Operator-verified; `run-shape.yml` ownership/seed semantics |
| E26 | FACT | An absent `min_mode` floor parses to the most-permissive mode, so every adopter receives the `exec-sandbox` capability UNFLOORED by default. | Operator-verified; floor-default semantics |
| E27 | FACT | A safety WATCH card (the exec-sandbox paired-invariant defer) was retired, and the invariant hole it was watching then went live. | Operator-verified; watch-card retirement history |

**Table note (supersedes the chain's original E21 framing):** The chain originally recorded E21 as "current repository state contradicts treating non-adoption in vh-solara as a live condition." E25–E27 SUPERSEDE that narrow framing. The grants are present (E20 holds), but the floor is absent (E25–E26), so the consumer-adoption gap is LIVE in a worse form — a binding/safety failure, not an availability gap. A harness fix is in flight. E27 adds watch-retirement as a distinct rediscovery failure mode.

## 4. Capability × discovery-mechanism matrix

Legend:

- **Present:** Direct evidence of a useful discovery mechanism.
- **Partial:** Mechanism exists but does not cover the complete moment-of-need path.
- **Absent/gap:** No suitable mechanism was found.
- **Gap — insufficient evidence:** The bounded audit did not establish the cell.
- **N/A:** The mechanism does not naturally apply.

| Capability | Baseline context | Workflow pointer | Surface at friction | Per-agent prompt naming | Restart/rediscovery |
|---|---|---|---|---|---|
| Exec family: `exec`, `exec-ro`, `exec-sandbox` | Present | Present | Strong for `exec-ro`; incomplete census elsewhere | Gap for three special grant holders | Gap after compaction |
| Read-only Levels A/B/C | Present | Present | Partial; distinction is documented but not confirmed across all denial text | Partial through policy pointer | Gap |
| `backlog` skill | Present | Present | Partial: content-tangle paths teach recovery | N/A | Global restart hint; compaction gap |
| `bgshell-job` | Present | Present | Gap — insufficient evidence | N/A | Global restart hint; compaction gap |
| `contract-invariant-audit` | Present; tight trigger | Present | Gap | N/A | Global hint; **consumer floor-binding gap (E25–E26)** |
| `debugging-loop` | Present | Present | Partial after invocation; weak before discovery | N/A | Global restart hint; compaction gap |
| `diagnostics-export` | Present | Present | Gap — insufficient evidence | N/A | Global restart hint; compaction gap |
| `formal-verification` | Present; tight trigger | Present | Gap | N/A | Global hint; **consumer floor-binding gap (E25–E26)** |
| `gated-commit` | Present | Present | Strong through git-mutation routing | N/A | Global restart hint; critical rule retained through compaction |
| `harness-operator` | Present | Present | Partial/strong for update and stale-corpus failures | N/A | Strong update-time signals |
| `media-perception` | Present; relatively costly | Present | Partial after invocation | Present for media routing; weak for its `exec-sandbox` grant | Global restart hint; compaction gap |
| `repo-recon` | Present; relatively costly | Present | Gap — insufficient evidence | N/A | Global restart hint; compaction gap |
| `resolve-first` | Present; tight trigger | Present | Partial in card-creation policy; refusal coverage incomplete | N/A | Global hint; **consumer floor-binding gap (E25–E26)** |
| `skill-creator` | Present | Present | Partial through harness extension guidance | N/A | Global restart hint; compaction gap |
| `tdd-loop` | Present | Present | Gap | N/A | Global restart hint; compaction gap |
| `think-mode` | Present; highest description cost | Present | Gap; ambiguity is not a deterministic error surface | N/A | Global restart hint; compaction gap |
| Slash-command catalog | Present | Present | Variable; incomplete census | N/A | Gap — command cache behavior not established |
| `/task-delete` | Present | Present | Strong refusal-time guidance | N/A | Gap — insufficient evidence |
| DEFER holding area and promotion DoR | Present | Strong | Partial; deletion and release paths teach some recovery | Gap — complete prompt parity not established | State persists, but cards may not re-enter salience |
| Trigger predicates and promotion checks | Present | Present | Partial | Gap — insufficient evidence | Durable state present; rediscovery path incomplete |
| Config layering and overlays | Present | Present | Partial through update/doctor failures | Gap across overlay-derived grants | Strong for changed skills; incomplete for general permission/config caching |
| Session/task-contract continuity | Present | Present | Partial | N/A | Durable when recorded; capabilities omitted before compaction remain a gap |
| Capability catalog requiring prior knowledge | Partial | Gap | Gap for silent non-attempts | Gap | Gap after compaction |
| **Safety WATCH cards (e.g. exec-sandbox paired-invariant defer)** | Present | Partial | Gap — retirement does not surface the re-opened hole | N/A | **Retirement is a rediscovery failure (E27): the watched invariant goes live silently when the watch is retired** |

### Matrix interpretation

The matrix supports three distinct failure classes:

1. **Initial discovery failure:** The capability is not named where the agent forms its action plan.
2. **At-friction routing failure:** A denial or refusal does not name the sanctioned alternative.
3. **Rediscovery failure:** The capability was once available or salient but disappeared after update, restart boundaries, compaction, consumer lag, or **watch retirement (E27, new)**.

A fourth disposition, **silent non-attempt**, must be retained. The absence of a denial does not prove successful discovery.

## 5. Audit design

### O1 — Friction-surface-first census

Enumerate every user- or agent-visible:

- `DENY`
- permission refusal
- protected-state refusal
- cannot-proceed result
- fail-closed result
- rejected transition
- missing-capability diagnostic

For each surface record:

- emitting component
- exact visible message
- triggering condition
- affected agents
- relevant capability
- sanctioned next action
- whether the next action is named inline
- canonical authority source
- evidence ID and observed date
- disposition: adequate, ambiguous, absent, or insufficient evidence

`exec-ro` is the positive control. It should be scored as conforming, not placed on the fix list.

### O2 — Grant-to-prompt parity

Derive special grants from configuration authority. For every grant-holder pair, determine whether the agent prompt:

1. plainly says the agent holds the grant;
2. describes the grant's safety boundary;
3. points to the canonical policy;
4. avoids copying the whole decision ladder.

Any proposed prompt statement remains a derived projection. It must not become independent permission authority.

### O3 — Rediscovery survivability

Evaluate each selected capability under separate conditions:

1. fresh process after configuration render;
2. same process after configuration or skill change;
3. new Task dispatch without process death;
4. post-compaction continuation;
5. resumed session from task contract or checkpoint;
6. consumer repository before and after update;
7. **after a safety watch is retired (E27, new)** — does any surface flag that the watched invariant is now unguarded?

Do not aggregate these into one "rediscovery works" result.

### O4 — Authority/projection consistency wrapper

Apply to every O1–O3 finding:

- **Authority:** permission map, profile, generated manifest, or lifecycle state.
- **Projection:** prompt, skill description, command description, workflow document, denial message, restart hint, or compaction primitive.

A projection may expose authority but may not contradict or replace it.

### O5 — Bounded incident-frequency calibration

Sample a small dated set of actual capability misses. Classify each as:

- capability never attempted;
- incorrect tool or command attempted;
- denial lacked an alternative;
- grant existed but was not known;
- process restart was required but not understood;
- capability disappeared after compaction;
- consumer had not adopted the capability;
- **consumer adopted the capability but the safety floor did not ship (E25–E26, new);**
- **safety watch was retired and the invariant hole went live (E27, new);**
- capability did not actually exist.

Historical state must not be silently restated as current state.

## 6. Provisional ranked fix list

**Sign-off status (operator disposition, Finding 2 → c):** ONLY entry 1 is SIGNED OFF — it has an observed working instance (commit 2b903a1, the exec-ro DENY ladder). Entries 2–9 are EXPLICITLY UNSIGNED and held pending the Gate A–D evidence-completion pass. Reason: this project has been repeatedly burned by unmeasured claims stated as facts (a count inflated ~10×; a ranking that inverted once scoped; a defer-invalid rate estimated ~30% that measured 11%; a "3/9" traced number that observed 6/9). debate returning need_evidence was the correct signal — honor it. The top fix does not need the gate because it has an observed instance; the rest do.

### 1. [SIGNED OFF — observed instance: 2b903a1] Generalize surface-at-friction across denial and refusal families

- **Surface:** Shell-guard denials, permission refusals, coordination lifecycle refusals, fail-closed execution results, and other cannot-proceed outputs.
- **Message shape:** `Denied because <reason>. The sanctioned alternative for this intent is <capability/action>. <Authority or safety condition>. See <canonical pointer>.`
- **Principle:** Surface at friction; single source of truth.
- **Failure unblocked:** Agent treats a denied command as proof that the task is impossible, retries through an unsafe route, or escalates prematurely.
- **Evidence:** E01, E14, E23.
- **Qualification:** First complete the surface census. Do not change already-conforming `exec-ro` output merely for uniformity.

### 2. [HELD — pending Gate A–D evidence] Name special grants in each grant-holding agent prompt

- **Surface:** Prompts for agents with non-default grants.
- **Message shape:** `This agent is granted <capability> under <canonical policy pointer>; the configuration remains authoritative.`
- **Principle:** Per-agent prompts name their own capabilities without duplicating the decision ladder.
- **Failure unblocked:** An agent delegates or reports blockage despite already holding the applicable capability.
- **Evidence:** E04–E06.
- **Qualification:** Causal leverage remains a prediction until incident evidence connects grant omission to real misses.

### 3. [HELD — pending Gate A–D evidence] Remove ambiguity between classifier denial and permission denial

- **Surface:** Read-only execution policy and any generic denial/handoff wording derived from it.
- **Message shape:** Explicitly distinguish: command is outside the classifier; command requires an already-granted higher read-only level; command requires a different authorized agent; command is genuinely mutating.
- **Principle:** One decision ladder; no premature escalation cue.
- **Failure unblocked:** "Command denied" is incorrectly interpreted as "handoff to build."
- **Evidence:** E02–E03.
- **Qualification:** Current policy does not literally require build handoff for every classifier denial. The audit question is whether nearby generic language still teaches that interpretation.

### 4. [HELD — pending Gate A–D evidence] Add a restart-required diagnostic shape

- **Surface:** `doctor` or equivalent synchronous diagnostic output.
- **Message shape:** `WARN: rendered capability configuration changed after this OpenCode process started. Restart OpenCode to refresh cached skills or permissions. Starting another Task does not refresh the process cache.`
- **Principle:** Rediscovery resilience; surface the actual recovery condition.
- **Failure unblocked:** Repeated Task dispatches fail because the operator assumes a new subagent refreshes process-local caches.
- **Evidence:** E08.
- **Qualification:** This is a design candidate. The repository evidence does not yet prove that `doctor` can reliably observe process start time and active in-memory configuration. If reliable observation is unavailable, the warning must be tied to an observable proxy and worded accordingly.

### 5. [HELD — pending Gate A–D evidence] Establish post-compaction capability re-derivation

- **Surface:** Compaction/resume workflow rather than baseline capability catalog.
- **Message shape:** Point to a canonical, cheap capability re-derivation action after a lossy boundary.
- **Principle:** Durable re-derivation over baseline token tax.
- **Failure unblocked:** Previously visible skills, commands, grants, or DEFER paths disappear from session salience.
- **Evidence:** E18–E19.
- **Qualification:** Do not inject the full catalog into every compaction primitive. The exact re-derivation command must be established from supported command surfaces rather than invented.

### 6. [HELD — pending Gate A–D evidence] Extend contextual routing to command and DEFER refusals

- **Surface:** Task-card creation, promotion, update, delete, and lifecycle-protection refusals.
- **Message shape:** State why the operation is blocked, the lifecycle-preserving alternative, and the applicable trigger or promotion requirement.
- **Principle:** Surface the workflow at the point where the invalid transition is attempted.
- **Failure unblocked:** Agents create direct backlog rows, use force as ordinary recovery, or confuse transport cards with durable truth.
- **Evidence:** E13–E15.
- **Qualification:** `/task-delete` is another positive example; other lifecycle surfaces require census.

### 7. [HELD — pending Gate A–D evidence] Conduct targeted skill-description review

- **Surface:** Frontmatter descriptions for the highest-cost or most overlapping skills.
- **Message shape:** Retain only task trigger, critical disambiguation, and required invocation condition; move procedure into the skill body or canonical workflow pointer.
- **Principle:** Tight when-to-invoke triggers with low per-turn baseline cost.
- **Failure unblocked:** Skill triggers compete or become too diffuse to select reliably.
- **Evidence:** E09–E12; commit `edd5ea5`.
- **Qualification:** Do not impose a uniform character limit. The current evidence shows that all 14 descriptions already contain trigger language. Review should start with `think-mode`, `repo-recon`, and `media-perception`, which account for 1,313 characters.

#### 8. [HELD — pending Gate A–D evidence] (New) Ship the min_mode floor binding alongside the exec-sandbox grant

- **Surface:** Consumer adoption / harness render path — the binding between a shipped capability grant and its safety floor.
- **Failure unblocked:** Every adopter receives `exec-sandbox` UNFLOORED; an absent floor parses to the most-permissive mode. This is a discovery/binding failure with a safety consequence, not just an availability gap.
- **Evidence:** E25–E26. Harness fix already in flight — this row records the audit's discovery dimension, not a new implementation.
- **Qualification:** The fix itself belongs to the in-flight harness lane; the audit contribution is naming "floor-binding as a discovery surface" so future capability grants carry their floor by default.

**Operator disposition (Finding 1 → b):** floor-binding is affirmed as a DISTINCT fix-list entry AND a separate dated audit incident. It is the only discovery failure in the register with a SAFETY consequence, and the sharpest instance of the E24 authority/projection split — the capability (authority) shipped while its containment precondition (also authority, but project-owned) did not. Folding it into a generic "availability lag" row would blur exactly what makes it instructive. The audit framing does NOT depend on the in-flight harness fix landing (option (c) explicitly rejected). Note: distinctness here is a register-structure decision; the FIX itself remains HELD pending Gate A–D evidence per Finding 2 → (c).

#### 9. [HELD — pending Gate A–D evidence] (New) Surface watch-card retirement as a rediscovery event

- **Surface:** The watch/defer retirement path and any durability ledger of retired watches.
- **Message shape:** When a safety watch is retired, record the invariant it guarded and flag that the invariant is now unguarded; surface the re-opened hole at the retirement point and in any re-derivation surface.
- **Principle:** Rediscovery resilience includes the negative space — retiring a watch is itself a rediscovery-relevant state change.
- **Failure unblocked:** The exec-sandbox paired-invariant defer was retired and the hole it watched then went live silently (E27).
- **Evidence:** E27.
- **Qualification:** Distinct from cache/compaction rediscovery. The capability catalog did not change; the safety overlay did.

## 7. Rediscovery design

### 7.1 Per-process cache and restart-required state

Established facts:

- Skill discovery is cached per process.
- A new Task dispatch does not kill the process.
- Update already emits a restart hint when skill files change.

Proposed diagnostic contract:

| Condition | Diagnostic |
|---|---|
| Rendered skill/config files changed and stale process state is reliably observable | Non-failing `doctor` warning naming full restart |
| Only filesystem change time is observable | Warning must say the process **may** be stale |
| No reliable relationship can be observed | Do not claim stale in-memory state; retain update-time restart message and document the limitation |

The diagnostic must not assert internal process state from filesystem changes alone.

**Correction (operator note):** E08 already partially closes the rediscovery gap this subsection lists as open — `internal/cli/update.go:268-293` emits a restart hint when skill files change. The OPEN remaining piece is narrower: a process-staleness signal at `doctor` time (not just update time), and coverage of permission/config changes (not just skill files). This correction matters for the ranking — the rediscovery gap is smaller than the brief initially implied.

### 7.2 Compaction resilience

The audit should test whether an agent can re-derive:

- available skills;
- relevant slash commands;
- special per-agent grants;
- DEFER routing;
- current execution-policy level.

The preferred direction is a compact canonical lookup after lossy boundaries, not perpetual injection of all capability descriptions.

### 7.3 Consumer adoption — floor-binding failure (SUPERSEDES the chain's original "consumer lag is historical" framing)

The chain originally concluded that vh-solara's current state (grants present, E20) made the consumer-lag framing historical. **That conclusion is superseded by E25–E26.** The grants ship (E20 holds), but the `exec_sandbox.min_mode` floor does NOT ship, because `run-shape.yml` is project-owned and seeded only when absent. An absent floor parses to the most-permissive mode. Therefore:

- the consumer-adoption gap is **LIVE**, not historical;
- it is **worse** than a lag/availability gap — it is a **binding failure with a safety consequence**: the capability is present but its configured safety boundary is silently absent;
- a harness fix is in flight;
- this is recorded as a discovery/binding surface, not designed here (the shipped-opt-out distribution lane remains out of scope).

### 7.4 Pilot-skill consumer gap

The three pilot skills remain associated with local overlay availability rather than general consumer availability. Record:

- capability motivation originated in cross-repository problems;
- discoverability in the producing repository does not imply consumer availability;
- distribution design belongs to the separate workstream.

### 7.5 Watch-retirement rediscovery failure (NEW — E27)

Rediscovery is not only about caches, restarts, and compaction. Retiring a safety WATCH is also a rediscovery failure: the invariant the watch guarded goes live silently, and no capability-catalog change signals it. The exec-sandbox paired-invariant defer was retired and the hole it watched then went live. The audit's rediscovery surface must include a durability ledger of retired watches and a re-derivation path that surfaces currently-unguarded invariants, not merely currently-available capabilities.

## 8. Scope discipline

### In scope

- Current `vh-agent-harness` repository evidence.
- Current or historical `vh-solara` evidence where reachable.
- Capability discovery and rediscovery surfaces.
- Denial, refusal, error, and cannot-proceed messages.
- Prompt/grant parity.
- Skill trigger quality and baseline-description cost.
- Command and DEFER discovery paths.
- Restart, compaction, and watch-retirement failure modes.
- Record-only consumer-adoption (floor-binding) gaps.

### Out of scope

- Implementation or repository edits.
- Permission-map changes.
- AGENTS changes.
- Designing the shipped-opt-out overlay distribution lane.
- Re-litigating the execution ladder.
- Plugin, toast, or background-nudge mechanisms.
- Treating model output as transition authority.
- Broadening research to repositories other than the two named repositories.

## 9. Rejected anti-patterns

### Plugin, toast, and background reminder nudges

**Rejected.**

A side-channel reminder is detached from the exact failed action. It can arrive too early, too late, or without the command, safety floor, lifecycle state, or refusal reason needed to choose the sanctioned alternative.

The `backlog-reminder.js` precedent demonstrates why generic nudging is not the desired discovery architecture.

The selected alternative is:

1. preserve the denial or refusal;
2. explain why it occurred;
3. name the sanctioned alternative in the same output;
4. point to the canonical authority;
5. never automatically retry or transition.

This reproduces the successful principle of `2b903a1` without copying the exec ladder into unrelated surfaces.

## 10. Evidence-completion gate

Before converting the provisional list into a signed-off frequency × leverage ranking, complete:

### Gate A — Friction-surface census

Acceptance criteria:

- Every in-scope emitted denial/refusal family has a terminal disposition.
- Each examined surface records exact text, source, trigger, agent scope, sanctioned alternative, and evidence date.
- Exclusions are explicit.
- Existing positive controls are distinguished from fix candidates.

### Gate B — Bounded dated incident sample

Acceptance criteria:

- At least 3–5 attributable incidents are sampled.
- Each incident has a date or revision boundary.
- Initial discovery, friction routing, rediscovery, consumer lag/floor-binding, watch-retirement, and silent non-attempt are distinguished.
- Historical incidents are not represented as current state.
- Frequency claims state their sample size and coverage limits.

### Gate C — Rediscovery observation protocol

Acceptance criteria:

- Fresh-process, same-stale-process, new-Task, post-compaction, resumed-session, and post-watch-retirement conditions are reported separately.
- The capability lookup expected in each condition is named.
- "Restart required" is tied to an observable condition.
- Probabilistic compaction findings are not reported as deterministic facts.

### Gate D — Matrix completion

Acceptance criteria:

- Every requested matrix row has a disposition for all five discovery mechanisms.
- Unsupported cells remain `gap — insufficient evidence`.
- Final ranking distinguishes measured frequency from predicted leverage.

## 11. Confidence and remaining uncertainty

### Load-bearing finding — E24 generalizes beyond discovery

E24 (permission/config = authority; prompts/descriptions/docs/friction-messages = projections that must expose it) is the strongest thing in this brief. It generalizes: the recurring failure shape across this project is an artifact not BOUND to the surface where it takes effect — capability↛friction-point, decision↛durable card, claim↛observation, source↛render, config↛process state. Every fix that has actually worked added a binding. A project-wide review will build on this framing; keep it prominent.

**Confidence:**

- **High** that surface-at-friction is the leading design principle.
- **High** that authority/projection separation is required to prevent drift.
- **High** that per-process caching creates a restart-sensitive rediscovery boundary.
- **High** that compaction does not preserve the complete capability catalog.
- **High** (new) that floor-binding is a live consumer-adoption failure with a safety consequence (E25–E26).
- **High** (new) that watch-retirement is a distinct rediscovery failure mode (E27).
- **Medium** that explicit own-grant prompt wording will materially reduce misses.
- **Low to medium** on relative failure frequency because no bounded incident sample was completed.

**Remaining uncertainty:**

1. Complete coverage of denial and refusal surfaces.
2. Frequency of silent non-attempts.
3. Causal benefit of own-grant prompt naming.
4. Reliable observability for a stale-process `doctor` warning.
5. Command discovery/cache behavior.
6. Repeatability of post-compaction testing.
7. Scope and timing of the in-flight floor-binding harness fix.

## 12. Next recommended command

`/research Complete the capability-discovery evidence gaps: exhaustive friction-surface census, bounded dated incident sample, fresh/stale/post-compaction/post-watch-retirement rediscovery protocol, and floor-binding adoption audit for vh-agent-harness and vh-solara only`

---

## 13. Recorded operator dispositions

The three findings below record the operator dispositions applied to this brief before promotion.

### Finding 1 — Consumer-adoption mechanism refined (was: "premise refuted")

**Original chain finding:** The "consumer lag is live" kickoff premise was refuted — vh-solara already has the exec-sandbox grants (E20), so the framing was historical.

**Refined by fold-in (E25–E26):** The grants ship, but the `min_mode` floor does not ship (`run-shape.yml` is project-owned, seeded only when absent; absent floor = most-permissive mode). So the consumer-adoption gap is LIVE in a worse form — a binding/safety failure, not an availability gap. A harness fix is in flight.

**DISPOSITION → (b):** floor-binding is a distinct fix-list entry (entry 8) AND a separate dated audit incident. (c) explicitly rejected — audit framing is independent of the in-flight harness fix. [Entry 8 FIX remains HELD per Finding 2 → (c).]

### Finding 2 — Ranking is leverage-only (debate returned need_evidence)

**Finding:** No bounded dated incident sample was completed, so the fix list (now 9 entries) is ordered by predicted leverage only, not frequency × leverage. debate returned `need_evidence`.

**DISPOSITION → (c):** ONLY the top fix (entry 1, surface-at-friction, observed instance 2b903a1) is SIGNED OFF. Entries 2–9 HELD pending Gate A–D evidence. Reason: project repeatedly burned by unmeasured claims stated as facts. The evidence-completion /research is a separate later lane.

### Finding 3 — tmp artifact persistence (RESOLVED + PROMOTED)

**Original finding:** The solution-brief agent refused to write the tmp/ artifact (read-only contract over-interpreted), so the brief existed only in the chain return — one compaction from loss (the same prose-only loss this audit is partly about).

**DISPOSITION → RESOLVED + PROMOTED:** brief promoted to committed canon at researches/decisions/2026-08-04-capability-discovery-audit.md via committer. tmp/ is gitignored disposable; same lesson as the formal-verification pilot whose evidence had to be preserved out of tmp/.
