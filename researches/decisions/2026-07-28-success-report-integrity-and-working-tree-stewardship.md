**Date:** 2026-07-28
**Status:** Accepted (record-of-decision) — recommended status for the build session to confirm and land
**Supersedes:** None. Extends, but does not reopen, Decision 1 of `2026-07-25-f1-synthesis-family-and-s2a-topology.md`.
**See also:**
- `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md`
- `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md`
- `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md`
- `researches/decisions/2026-07-27-commit-scope-integrity.md`
- `researches/sources/2026-07-25-seven-controls-property-map.md`
- `docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`

# Success-report integrity and working-tree stewardship

## Framing

This decision places and designs two findings that were not among the seven controls adjudicated by the fixed F1/F2/F3 topology:

- **Finding A — coverage-enforcement:** an agent must account for everything in its declared scope before reporting success. A salient fragment, truncated handoff, or partial leaf result is insufficient.
- **Finding B — "green = full-build":** a green success report must mean both:
  1. the canonical full build/test verification actually ran successfully; and
  2. the relevant transition does not leave or consume unexplained dirty or scratch state.

Finding B contains two independently falsifiable sub-claims and therefore must be split before property scoring:

- **B1 — full-build execution integrity**
- **B2 — working-tree/artifact-identity integrity**

The governing HYBRID rule is:

> same property identity → MERGE; differ on at least one of owner, cadence, authority, failure-mechanism, or data-role → UNION.

The five axes have the following meanings in the existing property map:

1. **Owner** — the role responsible for producing, evaluating, or applying the control.
2. **Cadence** — the lifecycle point at which the control runs or is consumed.
3. **Authority** — whether the mechanism only INFORMS or is converted into a safety-layer gate that BLOCKS a transition.
4. **Failure-mechanism** — the causal failure mode the control detects or prevents.
5. **Data-role** — the control's relation to the controlled data: producer, renderer/consumer, design gate, or another independently justified role.

The fixed topology remains authoritative for the original seven controls:

- **F1 — synthesis-producing:** R1, R3, P-a
- **F2 — rendering/persistence:** R5, P-b, P-c
- **F3 — design-gate:** DAY-0

Its family-defining axis is data-role. Authority is orthogonal and does not turn every blocking mechanism into F3.

## Decision

### 1. A, B1, and B2 UNION; they do not MERGE

Finding A, B1, and B2 are three separate properties. They fail the HYBRID full-identity test on multiple axes and can fail independently.

The phrase "success-report integrity" is an umbrella description, not evidence of shared property identity.

| Property | Owner | Cadence | Authority | Failure-mechanism | Data-role |
|---|---|---|---|---|---|
| **A — declared-scope coverage** | Review orchestrator and review leaves produce coverage records; a deterministic validator compares them | Commit-review, handoff, or decision-success boundary | Reviewer/model declarations INFORM. A gate-shaped completeness validator may BLOCK only after the compared representations are canonical | Declared items omitted, fail-fast truncation, empty or partial leaf coverage, or partial examination represented as complete approval | **Validator/steward:** consumes declared scope and coverage evidence; emits a coverage disposition |
| **B1 — canonical full verification ran** | Authoritative repository verifier or transition wrapper | Before a success claim; currently recomputed at the pre-tag release boundary | Prose and receipts INFORM unless independently validated. The release G0 wrapper BLOCKS | Partial or targeted suite reported as full green; skipped command; stale result; result not bound to the assessed revision | **Verifier/steward:** executes the canonical verifier set and binds its result to an identified revision/tree |
| **B2 — clean transition state** | Transition wrapper or repository-state gate | Immediately before a transition for which artifact identity requires cleanliness; currently release G0b | Doctor/agent observations INFORM. The release wrapper BLOCKS tag creation on a dirty worktree | Uncommitted or untracked bytes make the verified/reviewed state differ from the state being released | **State steward:** observes repository state and protects transition-artifact identity |
| **Finding B as one aggregate** | Mixed | Mixed | Mixed | Conflates partial execution with dirty-state divergence | Mixed; not a valid single property |

The falsifying cases are decisive:

- A can pass while B1 fails: every declared file is reviewed, but the full build never runs.
- B1 can pass while A fails: the full build passes, but a declared file is omitted from examination.
- B1 can pass while B2 fails: all canonical commands pass, then an uncommitted edit or untracked file remains before the transition.
- B2 can pass while B1 fails: the tree is clean, but compilation or tests fail.
- A can pass while B2 fails: review coverage is complete, but the tree later becomes dirty.
- B2 can pass while A fails: a clean tree says nothing about whether every declared item was examined.

Therefore:

> **A ≠ B1 ≠ B2. They UNION and fail independently.**

This applies the same bar that Decision 1 used to reject S1/MERGE-all. Shared theme, shared script host, shared protected outcome, or shared blocking consequence is not full property identity.

### 2. The original F1/F2/F3 meanings remain unchanged

A, B1, and B2 do not fit the canonical meanings of F1, F2, or F3:

- They are not synthesis producers, so they are not F1.
- They may be rendered or persisted through F2 surfaces, but rendering their evidence is derived plumbing rather than their canonical data-role.
- They may be enforced by gates, but F3 is specifically the pre-code/BUILD-READY adversarial design gate. Authority alone is not a family-defining axis.

Placing B1 or B2 into F3 merely because they can block would silently broaden the fixed topology and contradict Decision 1's explicit treatment of authority as orthogonal.

### 3. Add F4 — assurance/integrity-stewardship

The topology is extended with a narrow fourth family:

> **F4 — assurance/integrity-stewardship:** controls that consume declarations, verifier evidence, or repository state and emit an integrity disposition about whether a success claim or protected transition is justified.

F4 is distinct on the topology's primary data-role axis:

- F1 produces synthesis.
- F2 renders or persists synthesis.
- F3 challenges design readiness before code.
- **F4 validates or stewards the relationship among declared scope, observed evidence, assessed revision, and transition artifact.**

F4 contains three UNION controls:

- **F4-A — declared-scope coverage**
- **F4-B1 — canonical verifier execution**
- **F4-B2 — transition-state/artifact identity**

This family label does not merge the three properties. It supplies a topology placement for independently failing controls with a common validator/steward data-role.

F4 also does not create new transition authority. Each mechanism still receives authority only through its actual enforcement host:

- reviewer and coordinator output remains INFORM;
- doctor findings remain INFORM unless an independently justified hard invariant is converted into a gate;
- commit-gate, release wrappers, tests, and other safety-layer mechanisms apply refusal.

This extension does not reopen Decision 1 for the original seven controls. Decision 1 remains unchanged: those seven controls still form exactly F1/F2/F3 according to their own property map. This decision adjudicates new controls that were outside that set.

## Mechanism

### 1. Prevention: prompt, contract, and closeout discipline

Prevention is an INFORM surface. It reduces ambiguity but is not proof.

#### F4-A — declared-scope coverage

Prompts and review contracts should require:

1. an explicit, normalized declared-scope list;
2. a coverage disposition for every declared item;
3. explicit reasons for exclusions or items that could not be examined;
4. a distinction between:
   - examined,
   - not examined,
   - excluded by contract,
   - blocked by missing evidence;
5. a prohibition on aggregate approval when any declared item lacks a disposition;
6. a stable revision or acquire-time anchor when concurrent changes could alter the apparent scope.

A success report must not use "reviewed," "complete," or equivalent language merely because the salient or load-bearing-looking fragment was examined.

This operationalizes the case study's:

- §4.1 invariant registry and closure rule;
- §4.2 facts-as-cache-entries discipline;
- §4.5 requirement to test the path rather than only the node;
- §4.7 correction closure.

#### F4-B1 — canonical verifier execution

A green report must state:

- the canonical required command set;
- which commands actually ran;
- each command's outcome;
- the revision or tree against which they ran;
- any required command that was skipped;
- whether the evidence is authoritative recomputation or only a supplied receipt.

Targeted or smoke commands must be labeled targeted or smoke. They must not be summarized as full green.

If full execution cannot be observed or bound to the assessed state, the result is `inconclusive` or `not-demonstrable`, not green.

#### F4-B2 — transition-state stewardship

Closeout and transition contracts should distinguish:

- an exact committed/reviewed slice;
- unrelated concurrent working-tree changes;
- scratch files created by the current task;
- global cleanliness required by a release transition.

Agents must remove their own temporary files and identify unresolved owned dirt. They must not erase or revert unrelated concurrent work to obtain a cosmetically clean status.

"Clean tree" is therefore transition-relative:

- release/tag transitions may require global cleanliness;
- ordinary exact-slice commits must preserve reviewed-tree identity without requiring unrelated concurrent work to disappear.

### 2. Detection and gate-shaped conversion

#### F4-A — scope-coverage detection

The existing commit-reviewer surface aggregates leaf `reviewed_files`. That provides a substrate for structural coverage validation but does not prove semantic examination.

A deterministic proxy can compare:

```text
normalized_declared_scope
against
normalized_union_of_coverage_dispositions
```

The validator should report:

- missing declared items;
- duplicate or ambiguous identities;
- reported items outside the declared scope;
- declared items with no terminal disposition;
- fail-fast termination that prevented complete coverage.

The proxy proves only that every declared item received a coverage disposition. It cannot prove review quality, attention, or semantic understanding.

**Gate decision:** this may become a hard gate only after:

1. declared scope has one authoritative representation;
2. coverage evidence has one authoritative representation;
3. item identity and exclusion semantics are deterministic;
4. the validator is attached to the actual approval transition;
5. fixtures prove that partial/truncated coverage refuses approval.

Until then, doctor or review diagnostics should WARN/INFORM rather than overclaim closure.

#### F4-B1 — full-build detection

A prose claim or behavioral-closure token cannot prove that a full command ran. Doctor check #14 is intentionally structural: it validates internal token consistency but does not prove path execution, and an absent token currently passes.

A full-build receipt can improve diagnostics only if it includes at least:

- canonical command identity;
- exit status;
- assessed HEAD/tree;
- observation time;
- producing wrapper;
- invalidation rule when HEAD/tree changes.

Even such a receipt is evidence, not transition authority.

**Gate decision:**

- At release, authoritative recomputation by `scripts/release-tag.sh` G0 is a hard invariant and may BLOCK.
- A doctor check may WARN when a closeout claims full green without a complete, revision-bound receipt.
- The ordinary commit-gate must not claim full-build enforcement until it either recomputes the canonical suite or validates a trustworthy receipt bound to the exact approved tree.
- A missing or unverifiable receipt produces `inconclusive`, not a fabricated gate pass.

The field-report example in which a targeted `go test ... -run` selected only 2 of 21 tests demonstrates why command exit status alone is an inadequate "full green" proxy.

#### F4-B2 — dirty-tree detection

Dirty working-tree state is mechanically observable, but whether it is a hard invariant depends on the protected transition.

**Release/tag boundary:** global cleanliness is a hard invariant. A dirty tree means the tagged commit does not include the bytes present during the transition. `scripts/release-tag.sh` G0b correctly refuses.

**Ordinary exact-slice commit boundary:** global cleanliness is not a valid universal invariant. This repository intentionally supports concurrent dirty work through an exact-file/private-index commit gate. Blocking all commits on unrelated dirt would contradict that operating model and encourage destructive cleanup.

The commit-gate should instead preserve the harder and narrower invariant already established by commit-scope integrity:

> the committed tree for the authorized slice must equal the reviewed/approved tree for that slice.

Doctor or closeout diagnostics may WARN about:

- task-owned scratch files left behind;
- unexplained owned modifications;
- a claim of global cleanliness contradicted by current repository state.

They must not infer ownership solely from global `git status`, and they must not direct an agent to discard unrelated changes.

### 3. Dogfood attachment points

#### Commit-gate

Relevant seams:

- `.opencode/scripts/commit-gate.sh`
- the `gated-commit` skill
- closeout-ledger and approved-tree integrity tests under `internal/cli/`

Attachment:

- preserve exact-slice and approved-tree identity;
- record revision/tree anchors and transition outcomes;
- do not add a universal globally-clean-tree precondition;
- do not treat an unverified "full build ran" declaration as authority;
- a future build receipt may be validated only if it is bound to the exact approved tree.

#### Doctor

Relevant seam:

- doctor check #14, implemented by `internal/cli/doctor_behavioral_closure.go`
- behavioral-closure pilot commits `7f95f29` and `4d8d725`

Attachment:

- retain check #14's structural honesty;
- add non-blocking integrity diagnostics only where their input contract is deterministic;
- warn on internally inconsistent or stale success evidence;
- do not claim that token presence proves execution;
- do not silently turn missing evidence into a pass.

A new diagnostic should remain INFORM unless and until a separate safety-layer transition adopts the underlying predicate as a hard invariant.

#### Closeout ledger and behavioral-closure token

The behavioral-closure token should remain focused on the load-bearing path and honest crux result.

Success-report integrity should parallel or extend its evidence vocabulary without laundering structural declarations into execution proof:

- F4-A records scope and coverage disposition.
- F4-B1 records command evidence and assessed revision.
- F4-B2 records transition-relative repository-state evidence.

The token or ledger may carry these declarations, but the executor or gate remains responsible for applying transitions.

### 4. Disposition

Each property fails independently and receives its own disposition:

| Property | Missing or failed evidence | Disposition |
|---|---|---|
| F4-A | Declared scope is incomplete, coverage is truncated, or representations are not comparable | No complete approval. Return incomplete/inconclusive. If the structural contract exists and comparison fails, the approval gate may refuse. |
| F4-B1 | Canonical full verification did not run, failed, or cannot be bound to the assessed revision | Never report full green. Return failed, skipped, inconclusive, or not-demonstrable as applicable. Release G0 refuses on actual command failure. |
| F4-B2 | Transition requires artifact identity and the tree is dirty | Release/tag transition refuses. At ordinary commit, preserve exact-slice identity and report task-owned leftovers without blocking on unrelated dirt. |

Implementation gaps with a concrete area, file scope, validation plan, and clear slice may become backlog work.

Speculative enhancements remain DEFER candidates under `.local/coordinator/tasks/`. They do not become direct backlog rows unless their trigger has fired and the promotion Definition of Ready is met.

## Authority-line audit

| Mechanism | Property | Classification | May apply a transition? | Rationale |
|---|---|---|---|---|
| Agent prompt and closeout language | A, B1, B2 | **INFORM** | No | Prevention vocabulary cannot prove compliance. |
| Reviewer/model coverage declaration | A | **INFORM** | No | Model output is candidate evidence, never transition authority. |
| Coordinator synthesis | A, B1, B2 | **INFORM** | No | Coordinator state informs; safety-layer gates act. |
| `reviewed_files` aggregation | A | **INFORM / detection input** | No | Aggregation does not itself prove equality with declared scope or semantic examination. |
| Doctor scope-coverage diagnostic | A | **INFORM** initially | No | It remains a warning until canonical comparable representations and a protected transition are defined. |
| Deterministic declared-scope equality validator attached to approval | A | **GATE-SHAPED CONVERSION** | Yes, after prerequisites are met | Structural completeness can be a hard invariant; semantic review quality cannot. |
| Behavioral-closure token | B1 and load-bearing outcome | **INFORM / declaration** | No | The token does not prove the path executed. |
| Doctor check #14 | Behavioral-closure structure | **INFORM health diagnostic** | No transition authority established here | It validates token consistency, not execution. |
| Full-build receipt | B1 | **INFORM / cache entry** | No by itself | A receipt may be stale, incomplete, or unbound. |
| `scripts/release-tag.sh` G0 recomputation | B1 | **GATE-SHAPED CONVERSION** | Yes | The wrapper runs the canonical command set and refuses the release transition on failure. |
| Global `git status` observation in closeout prose | B2 | **INFORM** | No | It may not identify ownership and may become stale. |
| Doctor scratch/dirty-state warning | B2 | **INFORM** | No | Useful for stewardship, but concurrency makes global cleanliness unsuitable as a general closeout gate. |
| Commit-gate approved-tree/exact-slice check | B2-related artifact integrity | **GATE-SHAPED CONVERSION** | Yes | It protects the committed slice without discarding or blocking unrelated concurrent work. |
| `scripts/release-tag.sh` G0b clean-tree check | B2 | **GATE-SHAPED CONVERSION** | Yes | Global cleanliness is a hard invariant at tag creation because the tag must identify the verified artifact. |
| DEFER candidate capture | Follow-up disposition | **INFORM / transport** | No | `.local/coordinator/tasks/` is transport, not canon or lifecycle authority. |
| Backlog promotion after trigger and DoR | Concrete follow-up | **GATE-CONTROLLED DISPOSITION** | Yes, through the repository's promotion workflow | A finding does not become canonical work merely because a model proposed it. |

## Contradictions resolved

### 1. "Both are about success reports" does not justify MERGE

This is thematic similarity, not full property identity. A, B1, and B2 differ in owner, cadence, protected transition, failure-mechanism, and data-role. They can fail independently.

This resolution is consistent with Decision 1's rejection of S1/MERGE-all.

### 2. Blocking authority does not place all three controls in F3

F3 is the pre-code/BUILD-READY design gate. Authority is orthogonal to the topology's data-role axis. Using F3 as a generic bucket for release or closeout checks would silently contradict the fixed topology.

### 3. F4 does not reopen the original three-family decision

The fixed F1/F2/F3 result covered the original seven synthesis, rendering, and design-gate controls. A, B1, and B2 were not among them.

F4 is an extension for new validator/steward controls. It neither moves nor reclassifies the original seven.

### 4. The release-readiness warning and release-wrapper refusal are not competing authorities

A read-only readiness agent may warn about dirty state. The authoritative release wrapper may refuse the actual tag transition. The difference is an authority-layer distinction, not a factual contradiction.

### 5. "Full checks before commit" is stronger in prose than in current commit-gate enforcement

Repository policy requires `go test ./...`, `go vet ./...`, and formatting verification before commit, but the inspected commit-gate ledger does not attest full-suite execution.

The full suite is deterministically recomputed at release G0. A future pre-commit gate would require either authoritative recomputation or a trustworthy exact-tree-bound receipt. This decision does not claim that such enforcement already exists.

### 6. "Clean tree" is not the same invariant at commit and release

At release, global cleanliness protects tag-to-artifact identity and may block.

At ordinary commit, concurrent unrelated dirt is normal. The hard invariant is exact authorized-slice identity, not global cleanliness.

### 7. Behavioral closure cannot prove command execution

Doctor check #14 is a structural consistency check. Its token is a declaration, not proof, and absence currently passes. It must not be cited as existing enforcement of A or B1.

## Open questions

1. What is the canonical declared-scope representation for F4-A: exact paths, path plus concern, review units, or another stable identity?
2. How are intentional exclusions represented so that they cannot be confused with omissions?
3. Should fail-fast review ever be permitted to produce an approval, or must it always produce an incomplete/split disposition?
4. What exact evidence binds a B1 full-build result to the approved tree or release commit?
5. Is a build receipt worth implementing before a concrete consumer exists, or should authoritative recomputation remain the only hard proof?
6. Should doctor warn when a closeout claims full green but supplies no complete revision-bound command evidence?
7. Can task-owned scratch state be identified reliably without misclassifying unrelated concurrent dirt?
8. Should the F4 family label be promoted into a broader topology index after the first control is implemented, or remain local to this decision until another consumer needs it?

## Evidence / Provenance

| Evidence ID | Claim supported | Source | Commit(s) | Verifying command |
|---|---|---|---|---|
| E-HYBRID-RULE | Same property merges; independently failing properties union | `researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md:355-380` | Current accepted decision chain | `git show HEAD:researches/decisions/2026-07-23-vh-solara-orchestration-field-report-disposition.md` |
| E-TOPOLOGY | F1/F2/F3 are defined by data-role; authority is orthogonal | `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md:46-105` | `9dbab50`, `68a8fc4` | `git show --stat 9dbab50 && git show --stat 68a8fc4` |
| E-FALSIFYING-TEST | MERGE requires full identity across the five property axes | `researches/decisions/2026-07-25-f1-synthesis-family-and-s2a-topology.md`; `researches/sources/2026-07-25-seven-controls-property-map.md:39-52` | `9dbab50`, `68a8fc4` | `git show HEAD:researches/sources/2026-07-25-seven-controls-property-map.md` |
| E-CASE-STUDY | Invariant registry, facts-as-cache-entries, path parity, and correction closure supply the design vocabulary | `docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`, especially §4.1, §4.2, §4.5, §4.7, §5 | Current case-study history | `git show HEAD:docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md` |
| E-A-PARTIAL-SEAM | Commit reviewer aggregates `reviewed_files`; complete tier execution is configurable, but no declared-scope equality gate is established | `templates/core/.opencode/agents/commit-reviewer.md:100-104,202-228` | Current source | `git grep -n "reviewed_files\|complete coverage\|fail_fast" -- templates/core/.opencode/agents/commit-reviewer.md` |
| E-A-SCOPE-ANCHOR | Approved-tree and acquire-time anchoring protects review scope under concurrency but does not prove exhaustive examination | `researches/decisions/2026-07-27-commit-scope-integrity.md:8-20`; `templates/core/.opencode/agents/committer.md:99-115`; `internal/cli/commit_gate_review_diffbase_test.go` | `4bc821e` | `git show --stat 4bc821e` |
| E-BEHAVIORAL-CLOSURE | Doctor check #14 is structural and does not prove path execution | `internal/cli/doctor_behavioral_closure.go:3-51,95-155`; `docs/coordination/CLOSEOUT_TEMPLATE.md:16-46` | `7f95f29`, `4d8d725` | `git show --stat 7f95f29 && git show --stat 4d8d725` |
| E-BEHAVIORAL-CLOSURE-TEST | Structural token consistency has a concrete verifier seam | `internal/cli` behavioral-closure tests | `7f95f29` | `go test ./internal/cli -run 'TestCheckBehavioralClosure|TestAnalyzeBehavioralClosureBlocksPure'` |
| E-FALSE-GREEN | A targeted green command can exercise only a fraction of the intended test set | `researches/sources/2026-07-23-vh-solara-harness-adoption-field-report.md:81-131` | Source-report history | `git show HEAD:researches/sources/2026-07-23-vh-solara-harness-adoption-field-report.md` |
| E-B1-G0 | Release G0 runs the canonical full verifier set and refuses on failure | `scripts/release-tag.sh:483-539` | `5d749fd` | `git grep -n "G0 — green tree\|go test \./\.\.\.\|go vet \./\.\.\.\|go build \./\.\.\." -- scripts/release-tag.sh` |
| E-B2-G0B | Release G0b separately refuses a dirty worktree | `scripts/release-tag.sh:541-553` | `5d749fd` | `git grep -n "G0b — clean worktree\|dirty worktree" -- scripts/release-tag.sh` |
| E-AUTHORITY | Coordinator/model output informs; safety-layer gates act | `researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md:117-136`; AGENTS.md safety invariant | Current accepted authority decision | `git show HEAD:researches/decisions/2026-07-22-claim-verifier-closure-kernel-and-stateful-coordinator.md` |
| E-COMMIT-LEDGER | Commit-gate persists transition/tree/HEAD outcomes but does not attest full build or global clean-tree state | `.opencode/scripts/commit-gate.sh:76-78,1018-1168`; `internal/cli/commit_gate_closeout_ledger_test.go` | `a4afe6e8`, `5495a9e`, `4bc821e` | `git show --stat a4afe6e8 && git show --stat 5495a9e && git show --stat 4bc821e` |
| E-COMMIT-LEDGER-TEST | Closeout-ledger behavior has an executable repository seam | `internal/cli/commit_gate_closeout_ledger_test.go` | `a4afe6e8` | `go test ./internal/cli -run TestCommitGate_CloseoutLedger` |

## Conclusion

The success-report integrity problem is not one control.

It is a fail-closed UNION of three independently falsifiable controls:

1. declared-scope coverage;
2. canonical full-verifier execution;
3. transition-state/artifact identity.

They belong in a narrow F4 assurance/integrity-stewardship family because their canonical data-role is validation and stewardship, not synthesis, rendering, or pre-code design challenge.

Their evidence may be produced and rendered by other families, but their transition authority remains federated:

- prose and model output INFORM;
- doctor detects and warns;
- deterministic safety-layer gates alone refuse transitions.
