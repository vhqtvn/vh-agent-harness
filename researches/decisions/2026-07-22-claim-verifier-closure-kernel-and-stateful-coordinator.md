# Decision: Claim/Verifier Closure Kernel + Scoped Stateful Coordinator

**Date:** 2026-07-22
**Status:** Accepted (design + scope). No code lands in this slice; this is a
record-of-decision that fixes the owned scope, the authority line, and the
deferred-work boundary for the unverified-claim failure class. Implementation
slices follow under separate task contracts.
**Supersedes:** none (narrowes, does not replace, the case-study's §4 menu).
**See also:**
[`../../docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`](../../docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md)
(the failure-mode case study this decision responds to — read §1–§4 first).
[`../../docs/checkpoints/2026-07-16-auto-gate-ignore-gitignore-doctor-gap.md`](../../docs/checkpoints/2026-07-16-auto-gate-ignore-gitignore-doctor-gap.md)
(proven precedent for the prose-rule → doctor-check → leak-test → invariant
loop that §4.1 generalizes).
[`./2026-04-29-coordination-control-plane-options.md`](./2026-04-29-coordination-control-plane-options.md)
(the control-plane-options memo this decision partially refills — currently a
placeholder stub; see "Open questions / follow-up").

## Framing

The case study (`docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`)
catalogs seven failure classes (F1–F7) all reducible to one event: *a claim
crossed a lossy boundary without being re-checked, and the receiving side
treated it as truth.* Its §4 proposes a menu of mechanisms. This decision does
two things:

1. **Scopes what this track owns** out of that menu — three mechanisms (§4.3
   defer-liveness, §4.1 invariant/closure kernel, §4.2 premise-recheck) — and
   records what is already closed elsewhere so no duplicate work is proposed.
2. **Fixes the runtime shape**: a **scoped stateful coordinator** that hosts a
   **claim/verifier closure kernel**, where the coordinator is **read-only and
   non-authoritative** (it owns disposition *state*, never transition
   *authority*), and **no background runtime** is introduced.

The decisive evidence is that **every documented case-study failure was a
synchronous-decision loss** — a wrong call made at a boundary (release, commit,
delegation, authoring) with the wrong information in hand. None were
asynchronous-recovery losses (a correct decision going stale with no synchronous
moment to revisit it). That shape determines the runtime: flat-file + lazy-read
+ gate-read-at-release-boundary is sufficient because the failure surfaces at
boundaries the harness already controls synchronously. A background
daemon/poller would solve a failure mode the case study does not exhibit.

## Decision

**Adopt a scoped stateful coordinator hosting a claim/verifier closure kernel.**

- **Scoped.** The coordinator owns three mechanisms (the OWNED scope below),
  not the whole §4 menu. Three other classes (F7, F4, F6) are already closed or
  in-flight under other tracks and are explicitly OUT OF SCOPE.
- **Stateful.** It carries disposition state: which claims are open, which are
  released, which defers contradict which claims, when each was last
  re-derived. State lives on the existing typed-memory substrate
  (`internal/memory/`) — see "Reuse wins" — not in a new store.
- **Read-only and non-authoritative.** The coordinator *informs*: it flags
  WARN, it emits "do not release", it surfaces contradictions. Safety-layer
  gates *act*: `doctor`, the commit-gate, a release-readiness validator, and
  tests block. **State belongs to the coordinator; transition authority never
  does.** This is the same authority split the harness already enforces
  everywhere (model output is a candidate, never transition authority).
- **No background runtime.** Synchronous boundary reads suffice. The
  DEFERRED TencentDB-R4-style async-recovery runtime stays deferred — it solves
  stale-disposition self-healing, which is a real problem but not the one the
  case study documents, and the read-only tradeoff (limitation #3 below) is
  accepted precisely to keep the runtime flat.

## Mechanism (owned scope)

Three mechanisms are in scope. Each maps to a case-study §4 entry and a
concrete enforcement seam.

### §4.3 (F1) — defer-liveness (the headline)

A **release-readiness gate** that reads (a) all open defers and (b) all claims
in released or about-to-release docs, and **FAILs on contradiction**. This is
the direct closure of F1 (known bug shipped three releases) and the single
highest-leverage mechanism: it would have blocked v0.13.1 and v0.14.0 from
shipping around the known-false v0.12.0 claim by construction.

**The gate is mandatory-primary and reads at the release boundary regardless of
its host.** This is forced by the load-bearing evidence below — trigger-based
promotion cannot be relied on, so the release boundary must read defers
directly.

### §4.1 — invariant registry / closure kernel (the multiplier)

A **claim/disposition registry** (transport — append-only typed records, not a
parallel committed ledger) plus a **closure rule**: every MUST/NEVER the
harness asserts (AGENTS docs, skills, review checklists, migration notes) is
registered with a declared enforcement point (a test, a `doctor` check, a
shell-guard rule, or a commit-gate validator), and **an invariant without an
enforcement point is itself a `doctor` warning.** This converts rule discovery
(violation → operator catch → rule stated) into rule compilation (rule stated →
gate must exist before the thread closes).

`doctor` is the proven host for §4.1 — the
`docs/checkpoints/2026-07-16-auto-gate-ignore-gitignore-doctor-gap.md`
checkpoint demonstrates the exact loop shipping: a prose rule ("never-commit
config must be gitignored") → a read-only `doctor` check
(`checkAutoGateGitignored`) → a leak-assertion test pinning the D6
secret-never-emits invariant → a `doctor` warning class. That loop is §4.1
specialized; §4.1 generalizes it.

### §4.2 (F2) — premise-recheck protocol (softened, not mechanically closed)

Mutable facts carried across a boundary are stored as
`(value, source, re-derivation_command, observed_at)` — not "the profile does
not select X" but "profile selection: check `grep capabilities <profile file>`,
observed_at T". **Before a subsession asserts a premise, it re-derives the
cheap ones** (a grep, a `doctor` run) instead of trusting the parent's summary.

See Honest limitations #2: this is protocol discipline, not a hard gate. The
coordinator's *own* context (the compression boundary) is the
structurally-hardest lossy surface, and re-derivation is a habit the protocol
demands, not a check the gate enforces.

## Authority and transition routing

The coordinator carries disposition state; the following transitions are routed
to the authority that already owns each. The coordinator never performs any of
them.

| Transition | Authority | Seam |
| --- | --- | --- |
| Edit/revert a *released* migration note | blocked by test | `TestMigrationNotes_ReleasedImmutable` (the rule that became a gate at commit `e929149`) |
| Block a release | release-readiness gate | §4.3 defer-liveness (FAIL on open-defer↔released-claim contradiction) |
| Block a commit | commit-gate + commit-reviewer | `.opencode/scripts/commit-gate.sh` + `/commit-review` (gated-commit protocol) |
| Edit code / docs / skills | delegated specialist | `build` / `docs-steward` via task delegation (coordinator is read-only) |
| Promote a DEFER → backlog row | promoter under DoR | `.local/coordinator/tasks/` → `docs/planning/backlog.md` (trigger fired + DoR met) |

**The line, stated once:** coordinator state *informs* (flags WARN, emits "do
not release"); safety-layer gates *act* (doctor / commit-gate /
release-validator / tests block). This is load-bearing — a coordinator that
acquired transition authority would collapse the model-output-is-candidate
invariant the whole harness is built on.

## What is explicitly out of scope (closed elsewhere — do not re-implement)

These classes are cited so this memo does not propose duplicate work.

- **F7 / §4.6 — policy authored against the wrong evaluator.** DONE.
  `ba68c76` `feat(permconfig): add read_only HarnessPolicy for RO specialists`
  introduces a first-class `read_only` HarnessPolicy (deny-first + a 28-entry
  canonical read-only allow inventory) that resolves correctly under opencode's
  `findLast` evaluation; `5fcd295` regenerates the rendered corpus for it. The
  dead-`exec-ro`-rule and RO-leaf-reaches-mutation-verb classes are closed by
  authoring against the real evaluator. **Not re-claimed here.**
- **F4 / F6 — node-tested-path-untested; dual derivation of "managed set".**
  DONE / in-flight. `ae5b30d`
  `feat(core): add capability-owned CoreOutputs filtering for media-perception`
  makes capability selection control which capability-owned files render
  (resolver/substrate/CLI seams, + tests including
  `internal/resolver/core_outputs_test.go` and
  `internal/substrate/exclude_renderer_test.go`). This is the §4.4
  declare-once-derive-everywhere closure for the managed set, and the substrate
  for §4.5's route-not-node testing. **Not re-claimed here.**

This track therefore owns F1/F2 plus the §4.1 multiplier, and explicitly leaves
F4/F6/F7 to the commits above.

## §4.3 load-bearing evidence (justification for mandatory-primary release-gate)

**New evidence from the operator (not in the case study):** the errata-v0120
card remained in `draft` state across v0.12.0 → v0.13.1 → v0.14.0 *even though
its documented trigger (release-note created) fired three times* — once per
release. If trigger-based promotion worked, the card would have promoted on the
first release and forced re-adjudication before the second. It did not.

**Conclusion: the card-trigger predicate machinery fails mechanically.** A
defer with a satisfied trigger is not reliably promoted by the existing
trigger-check path. Therefore:

- **defer-liveness MUST gate at release-readiness**, reading open defers and
  released claims directly. It cannot rely on trigger-promotion having already
  moved the card.
- This is the justification for making the release-readiness gate
  **mandatory-primary** rather than a backstop behind trigger promotion. The
  predicate checker (`node .opencode/scripts/check-defer-triggers.js`) remains
  a **promoter-use-only aid**, never wired into a commit hook, never blocking —
  consistent with the existing DEFER/follow-up curation contract.

## Reuse wins (exact paths, verified this slice)

The kernel is **not greenfield.** Its substrate already ships.

- **Typed-memory record layer — `internal/memory/record/record.go`**
  (VERIFIED). The `Record` struct is the claim/disposition atom. Actual schema
  is the **11-field** shape: `id` (required, stable; the task shorthand that
  motivated this memo omitted `id` — corrected here), `type` (persona /
  episodic / instruction), `priority` (low / normal / high / critical, with a
  `Rank()` for sort), `scope` (session / workstream), `scene?`, `workstream?`,
  `source_ref?`, `session_key?`, `created_at`, `updated_at`, `body`. Unknown
  enums are rejected on decode; unknown fields are ignored (forward-compatible).
  This is the cognitive-layer work inspired by TencentDB-Agent-Memory, R1.
- **Append-only JSONL store — `internal/memory/store/store.go`** (VERIFIED).
  `flock` (cross-process `LOCK_EX`) + per-path in-process `sync.Mutex` +
  `fsync` of data file (and parent dir once, on create) for crash-durability;
  append-one-line write model (updates expressed as a newer line with same ID,
  last-write-wins). Fault-tolerant bounded linear reader: a 1 MiB
  (`maxLineBytes`) `bufio.Reader` cap, malformed/over-long lines skipped and
  counted in `Stats`, never fatal. Capped at `MaxRecords = 200`. The registry
  gets atomic appends, durable reads, and bounded scans for free.
- **Delivered but dormant.** No `records.jsonl` exists anywhere under
  `.opencode/state/` (VERIFIED — `find` returns nothing across `sessions/`,
  `workstreams/`, `session-bindings/`). The typed-memory layer has zero live
  consumers. **The closure kernel is its first real consumer.**
- **`doctor` is the proven §4.1 host.** See the checkpoint cited in "See also"
  and in §4.1 above: prose-rule → read-only `doctor` check → leak-assertion
  test → invariant warning class, all shipped and test-pinned.

## Honest limitations (do not oversell)

1. **The registry only helps claims someone registers.** A claim nobody enters
   into the kernel is invisible to it. The **load-bearing-claim threshold** is
   an open question: too broad and the registry devolves into a noisy lint over
   every doc sentence; too narrow and it misses the claims that actually
   detonate. The threshold is not settled by this decision.
2. **§4.2 / F2 is softened, not mechanically closed.** Re-derivation is a
   protocol discipline, not a gate-enforced check. The coordinator's *own*
   context — the compression boundary where "at time T, X" becomes "X" — is the
   structurally-hardest lossy surface, and no release-boundary gate sees inside
   it. F2 is reduced in frequency, not eliminated.
3. **Read-only means the coordinator cannot self-heal stale dispositions.**
   If a disposition goes stale with no synchronous boundary to revisit it, the
   coordinator can flag but not fix. This is the accepted tradeoff for staying
   flat-file with no background runtime; it bounds the system to the failure
   shape the case study actually exhibits and declines to solve the
   async-recovery shape it does not.

## Tradeoffs

- **(+)** Reuses a delivered, tested, dormant substrate — the kernel costs
  domain logic, not new storage.
- **(+)** Read-only coordinator preserves the model-output-is-candidate
  invariant; no new transition-authority surface.
- **(+)** Mandatory-primary release-gate closes F1 by construction at the
  boundary where it recurred, independent of the broken trigger path.
- **(−)** The registry's value is bounded by registration discipline
  (limitation #1).
- **(−)** F2 stays a protocol habit, not a gate (limitation #2).
- **(−)** Stale-disposition self-healing is explicitly declined (limitation #3).

## Deferred work

- **TencentDB R4 async-recovery runtime.** A background process that
  self-heals stale dispositions and re-derives facts on a schedule. **Stays
  deferred.** It solves async-recovery (a disposition correct at capture going
  stale with no synchronous revisit), which is a real failure mode but not one
  the case study documents. Revisit if a documented loss is traced to
  stale-without-boundary rather than wrong-at-boundary.

## Open questions / follow-up

- **`symptom_signature` stability (S3).** When the same symptom is reported
  three times, how do the three reports collapse to *one recurrence event* on
  an existing defer rather than spawning three new cards? Unresolved. This is
  the deduplication problem underneath §4.3's "recurrence must escalate"; without
  a stable signature, recurrence detection is manual.
- **Release-gate host: `doctor` vs a dedicated release-readiness validator.**
  Partially collapsed: the gate MUST read defers + released claims at the
  release boundary regardless of host (that part is decided). Whether it lives
  as a `doctor` check (proven loop, reuses the §4.1 host) or as a standalone
  release-validator command is not finalized. `doctor` is the default candidate.
- **S2 declaration-index home.** Now **LOW** priority. With F4/F6 owned by
  `ae5b30d` (capability-owned CoreOutputs), the blast radius of a separate
  declaration-index shrank — the managed-set dual-derivation that motivated S2
  is already closed at the source. S2 is not abandoned, just deprioritized.
- **Dangling-ref stub.** `docs/coordination/README.md:14-15` cites
  `researches/decisions/2026-04-29-coordination-control-plane-options.md`,
  which has never existed in git history (`git log` on the path is empty). Per
  the slice brief's "prefer (a)", a minimal **placeholder stub** was created at
  that path this slice, noting it is unfilled and pointing here as the partial
  refill. The README citation is thereby preserved rather than de-referenced;
  no edit to `docs/coordination/README.md` was made (out of scope for this
  slice). If the stub is later filled, replace its body in place.

## Source corrections

The on-disk case study
(`docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md`) carries
two factual corrections effective this date. **The case-study file was NOT
edited this slice** (out of scope); the corrections are recorded here so a
reader of this decision is not misled, and so a future docs slice can reconcile
the prose.

1. **§1.1 — the shipped agent is a single generalist leaf, not an orchestrator.**
   The cluster/orchestrator design was rejected; what shipped under
   `core/media-perception` is one generalist leaf agent plus caller routing.
   The on-disk case study still reads "orchestrator" at **L18** ("skill +
   orchestrator agent + caller routing") and **L40** ("an orchestrator
   checklist asserted") as of this writing.
2. **§1.5 — "3/3 modalities" was actually 3 image sub-classes.** The live
   behavioral pass that preceded the production self-refusal was across three
   image sub-classes, not three modalities. The on-disk case study still reads
   "(3/3 modalities)" at **L36** as of this writing.

**This decision's analysis does not depend on either claim.** The
lossy-boundary failure classes (F1–F7) and the §4 mechanisms are structural and
hold regardless of whether the media-perception agent is one leaf or an
orchestrator cluster, and regardless of whether the pre-production pass was
three modalities or three image sub-classes. The corrections are recorded for
fidelity, not because they re-open the decision.

## Evidence / Provenance

Verified this slice by direct read of the cited artifacts and `git show --stat`
of the three commits:

| Claim | Verifying artifact / command | Verified |
| --- | --- | --- |
| `record.go` 11-field `Record` (incl. required `id`) | `internal/memory/record/record.go:130-159` (struct), `:11-15` (doc: "11-field shape") | yes |
| `store.go` flock+fsync + bounded fault-tolerant reader | `internal/memory/store/store.go:200-283` (append: mutex+flock+fsync), `:349-477` (read: bounded, malformed skipped+counted) | yes |
| Typed-memory layer is dormant (no `records.jsonl`) | `find .opencode/state -name records.jsonl` → no output | yes |
| F7/§4.6 closed by read_only policy | `git show --stat ba68c76` (6 files, +1266/-65: model/tables/emit/test) + `5fcd295` (render regen, 4 files) | yes |
| F4/F6 closed by CoreOutputs filtering | `git show --stat ae5b30d` (21 files, +1703/-97: resolver/substrate/CLI + tests) | yes |
| `doctor` proven as §4.1 host | `docs/checkpoints/2026-07-16-auto-gate-ignore-gitignore-doctor-gap.md` (A+B+D hybrid; `checkAutoGateGitignored` + D6 leak-assertion tests) | yes |
| Dangling ref at README:14-15 never existed | `git log --oneline -- researches/decisions/2026-04-29-coordination-control-plane-options.md` → empty | yes |
| Case-study "orchestrator" / "3/3 modalities" line numbers | `docs/case-study/2026-07-22-unverified-claims-at-lossy-boundaries.md` L18, L36, L40 | yes |

House-style note: this memo follows the
`2026-07-05-commit-gate-shared-file-coupling.md` convention (bolded-metadata
header as frontmatter; Problem/Framing → Decision → Mechanism → Tradeoffs →
Deferred → Evidence sections), not the YAML-frontmatter convention used by the
later `2026-07-16-*` memos, per the slice brief naming 2026-07-05 as the
template.
