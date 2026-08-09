# 2026-08-09 — harness read-isolation: trusted project-scoped projection as the PRIMARY confidentiality boundary

- **Status:** DECIDED (policy). The *confidentiality policy* is settled here.
  The *enforcement implementation* is **NOT** decided and stays **gated** on
  seven open implementation contracts recorded in §7 — those are GATES, not
  settled. This memo closes the **policy half** of DEFER card
  `defer-harness-read-isolation`; the implementation half is filed as a
  separate follow-up DEFER card (§8).
- **Scope:** the confidentiality boundary for cross-project reads of the
  shared OpenCode session DB from arbitrary-payload read-code (python3-sqlite3
  via exec). Narrow policy decision; reversible if an implementation contract
  refutes a premise.
- **Supersedes / extends:** extends the Level-B read-code capability settled
  in `researches/decisions/2026-07-29-exec-sandbox-level-b-mode-floor.md`
  (that memo settled the *write/network* containment axis; this memo settles
  the *read/confidentiality* axis the Level-B grant opened). It does NOT
  supersede the Level-B grant itself — preserving that capability is a stated
  policy outcome below.

- **Provenance:** authored from the settled solution-brief
  `ses_01cd7ee43ffeTdEj4uDC0so89b` (alias `defer-triage`). The brief returned
  a complete decided policy; this memo records it durably for canon. Operator
  pre-authorized autonomous execution. Where the brief's exact wording was not
  recoverable verbatim, the policy was re-derived from the card's four
  `open_questions` plus the settled-policy bullets supplied with the
  authoring mission; this re-derivation is recorded honestly and does not
  change any decision.

## Context — the confidentiality gap the Level-B grant opened

The harness stores OpenCode session/memory state in a shared per-install DB
that is **co-mingled across projects**: a session running in repo A and a
session running in repo B write rows into the same DB. The Level-B
exec-sandbox grant (settled in the 2026-07-29 memo, implemented in
`internal/permconfig/tables.go`) lets read-only agents run **arbitrary
read-code** — including `python3`/`node`/`bash` — under kernel containment
(Landlock + seccomp). The kernel floor makes *writes-outside-tmp* and
*network* physically impossible, which is why arbitrary read-code is safe for
a read-only agent **on the write/network axis**.

The gap is the **read/confidentiality axis**. A `python3 -c "import sqlite3;
..."` invocation under exec reads arbitrary files — including the shared
session DB — with no boundary separating repo A's session rows from repo B's.
This access **bypasses the binary allowlist**: it is not a harness verb, it
is an interpreter the harness permitted, so the binary cannot see which rows
the interpreter reads. Today the only thing standing between a session and
another project's rows is the **persist-guard**, which is **agent-discipline
documented in `docs/ai/shell-execution.md`** — it is not a mechanical check
and has no code artifact (grep for `persist` across the shell-guard plugin
and `docs/ai/shell-execution.md` returns zero enforcement sites; the guard
is a stated rule an agent is trusted to follow).

The DEFER card `defer-harness-read-isolation` framed this as four
`open_questions`. The solution-brief resolved all four into the policy below.

## The settled policy (six binding points)

1. **Trusted project-scoped projection is the PRIMARY confidentiality boundary.**
   The durable read surface a session may see is a *projection* of the shared
   DB scoped to the session's own project, not the raw co-mingled DB. The
   boundary is enforced at the data-projection layer the read path consumes,
   not at the command/text layer.

2. **The deliberate Level-B exec-sandbox read-code capability is PRESERVED.**
   This policy does **not** revoke or narrow the Level-B grant from the
   2026-07-29 memo. Arbitrary read-code under kernel containment stays a
   first-class read-only capability. The confidentiality fix is additive — it
   changes *what data the read path exposes*, not *whether an interpreter may
   run*.

3. **Projection REPLACES the raw co-mingled DB; it does not accompany it.**
   No raw DB handle and no raw co-mingled row set may enter the
   arbitrary-payload sandbox view. A session reads its projected surface; the
   raw DB is not reachable from the read-code path. (Allowing both would make
   the projection a cosmetic filter an interpreter can simply ignore by
   re-opening the raw DB.)

4. **permconfig is capability routing, NOT row authorization.**
   `internal/permconfig/tables.go` (the `CoreLocationRules` agent→command
   allowlist table) routes *which commands an agent may run*; it carries no
   per-row read-authz and is the wrong layer to acquire one. Row-level
   scoping belongs to the projection layer, not the capability table.

5. **shell-guard is defense-in-depth ONLY.**
   `shell-guard.js` / `shell-guard-core.js` classify **command text** (the
   `tool.execute.before` handler inspects `output.args.command`). Command text
   cannot guarantee interpreter-level row isolation: an arbitrary interpreter
   can read any reachable file by construction, and the plugin's own header
   notes a safe whole-command rewrite is unprovable. So shell-guard may add a
   coarse command-layer signal, but it **cannot** be the load-bearing
   confidentiality boundary.

6. **Durable output is closed-schema; unknown/raw fields are rejected.**
   Any persisted artifact derived from the projection (dumps, reports, neutral
   outputs) is written through a closed schema that rejects unknown or raw
   fields. This prevents the projection boundary from being laundered back
   into a co-mingled durable record.

## The four scoped questions — RESOLVED (policy)

Each of the card's `open_questions` receives a policy verdict here. (The
*implementation* of each verdict may be gated on a §7 contract; the policy
itself is settled.)

| # | Card `open_question` (condensed) | Policy verdict |
|---|---|---|
| Q1 | Is cross-project DB read a deliberate capability or an unintended side effect of python3 stdlib access via exec? | **Unintended side effect of the Level-B grant, now closed by policy.** Arbitrary read-code is deliberate (preserved, point 2); *cross-project row reachability* was never a granted capability. The projection boundary (point 1) makes the deliberate capability and the confidentiality boundary coexist. |
| Q2 | What read-isolation boundary would be enforceable without breaking legitimate same-project reads? | **A project-scoped projection that REPLACES the raw DB (points 1 + 3).** Per-directory row filtering at the interpreter layer was rejected as unenforceable (interpreter can re-open raw DB); per-session scoping alone is too narrow (legitimate same-project reads span sessions). The projection is scoped to *project*, served as the read path's only surface, and is enforceable because no raw handle is exposed. |
| Q3 | Does the fix live in shell-guard rules, the permission/policy layer, or an opencode-level isolation feature? | **At the data-projection layer consumed by the read path.** shell-guard is defense-in-depth only (point 5); permconfig is capability routing only (point 4). Neither is the fix site. The exact projection mechanism (an opencode-level feature vs. a harness-provisioned projection) is an **implementation contract** (§7 G2/G4), not a policy decision. |
| Q4 | Should the persist-guard be backed by a mechanical check rather than relying on agent discipline? | **Yes — as a consequence of point 3.** If no raw DB handle may enter the sandbox view, then "do not persist co-mingled rows" is no longer a trust-the-agent rule; the absence of the raw handle is the mechanical guarantee. The persist-guard's agent-discipline form is retired by the projection boundary, not strengthened alongside it. (The *mechanism* that removes the raw handle is gated on §7 G1/G4.) |

## Alternatives REJECTED

- **permconfig-only (row authz in the capability table).** Rejected by point
  4: `tables.go` routes agent→command allowlists; adding per-row read-authz
  there would couple capability routing to data authorization and still could
  not see what an interpreter reads at runtime. Wrong layer.
- **shell-guard-only (a command-text deny/block rule for python3 sqlite3
  cross-project reads).** Rejected by point 5: command-text inspection cannot
  prove interpreter-level row isolation. An interpreter can read the DB via
  paths a text classifier cannot enumerate (env-supplied scripts, `-c`
  payloads, imported modules). This was the framing the card's Q3 floated; it
  is explicitly not the boundary.
- **doc-only (strengthen the persist-guard as agent-discipline and stop).**
  Rejected by Q4 / point 3: relying on agent discipline for a confidentiality
  boundary that an arbitrary interpreter can trivially defeat is not a
  boundary. The persist-guard's discipline form is *retired* by the projection
  boundary, not fortified.

## Evidence register

Each load-bearing premise and the ground that confirms it. This is
**evidence-bearing**, not model-synthesized: every entry cites a real code/doc
surface verified during authoring.

| Premise | Ground (verified) |
|---|---|
| Level-B grant permits arbitrary read-code incl. python3/sqlite3 | `internal/permconfig/tables.go` (researcher + repo-explorer entries): comment "exec-sandbox lets researcher run arbitrary read-code (incl. python3/node/bash)"; `internal/permconfig/emit_execsandbox_test.go` exercises `vh-agent-harness exec-sandbox python3 …`. |
| Kernel floor contains writes/network, not reads | `researches/decisions/2026-07-29-exec-sandbox-level-b-mode-floor.md` decisions 1–2: Landlock makes writes-outside-tmp + network impossible; reads of repo/system paths are the granted surface. The read/confidentiality axis is exactly the residual gap. |
| permconfig is capability routing, not row authz | `internal/permconfig/tables.go` `CoreLocationRules`: agent→{Wildcard,Readonly,GitReadonly,Gate,Edit,ReadOnlyExtraAllows}. No row/read-authz field exists. |
| shell-guard is command-text classification | `templates/core/.opencode/plugins/shell-guard.js` header: the `tool.execute.before` handler inspects `output.args.command`; "a safe whole-command rewrite unprovable"; engine classifies command verbs only. |
| persist-guard is agent-discipline, not a code artifact | `grep -rn "persist" templates/core/.opencode/plugins/shell-guard.js docs/ai/shell-execution.md` → zero enforcement sites. The guard is a stated rule, not a mechanical check. |
| The session DB is co-mingled across projects | DEFER card `defer-harness-read-isolation` `research_question` (the threat surface the card was filed against) + the shared-DB persistence model the Level-B grant reads from. |

**Honesty caveat:** the evidence register confirms the *mechanism shape* of
each layer (capability routing, command-text classification, agent-discipline
guard). It does **not** prove a projection layer exists today — it does not
(§7 G4 is the contract to build/identify it). The policy is settled on the
shape; the existence is gated.

## The seven implementation-gate contracts — UNRESOLVED (GATES, not settled)

The enforcement implementation is gated on these seven contracts. Each is a
**gate**: until it is resolved, no enforcement slice ships. They are recorded
here so a future implementation slice does not silently re-decide policy by
picking one. None is settled by this memo.

- **G1 — Raw-DB provisioning ownership.** Who provisions the raw DB, and who
  owns the *seam* that turns it into a projection? The provisioning owner
  controls whether "no raw handle enters the sandbox view" (point 3) is even
  enforceable. Unresolved: the provisioning owner and the seam trace.
- **G2 — Project-identity canonicalization.** The projection is scoped to
  "project" (Q2). What canonicalizes a project identity (repo root? a
  configured ID? a hash?) must be settled before the projection can filter.
  Unresolved: the canonicalization rule.
- **G3 — Parent/child session ownership.** A session may have a parent
  (`session_bindings` carry `parent_session_id`). Whether a child inherits the
  parent's project projection, and how multi-session ownership of rows is
  modeled, is unresolved. Affects which rows a projection exposes.
- **G4 — Projection tables/schema.** The concrete shape of the projected
  surface (new tables? views? a derived store?) and its schema. This is the
  load-bearing implementation artifact for points 1+3; it does not exist
  today and is not designed here.
- **G5 — Multi-project operator-authz allowlist.** For operators who legitimately
  work across projects, what allowlist/override admits a cross-project read?
  Without it, the projection boundary blocks a real workflow; with it, the
  boundary needs an authz rule. Unresolved.
- **G6 — M1 compatibility.** The projection must not break the M1
  dogfood/use-case axis (the M1 natural-miss study path grounded in the
  2026-07-29 memo relies on a read-only agent reading a DB copy). The
  compatibility contract between the projection and the M1 read path is
  unresolved.
- **G7 — Versioned persistence schema.** Point 6's closed-schema durable
  output needs a versioned persistence schema so future changes are migratable
  and unknown/raw fields are rejected at a defined boundary. The schema
  version + rejection site are unresolved.

**These seven are GATES.** An enforcement slice that ships without resolving
one has not implemented this policy — it has re-scoped it. The follow-up DEFER
card (§8) carries them as its ready-criteria inputs.

## Verification (this slice — policy decision only)

This is a docs-only slice. No code changed.

- `go test ./...` — unaffected (docs-only; no Go source touched).
- `make update` — **not required**: `researches/decisions/` is not a rendered
  surface; the decision-memo corpus is not embedded and not regenerated by
  the seam.
- `gofmt` / `go vet` — unaffected.

```yaml
behavioral-closure:
  crux: >
    The confidentiality policy is DECIDED and recorded durably at
    researches/decisions/read-isolation-policy.md, AND the seven
    implementation-gate contracts are explicitly recorded as UNRESOLVED
    (gates, not settled).
  verdict: proven
  result: proven
  verifier: >
    The memo file exists at the target path and (a) records a verdict for each
    of the card's four open_questions (§"The four scoped questions"), (b)
    names the three rejected alternatives (§"Alternatives REJECTED"), (c)
    carries an evidence register bound to real code surfaces (§"Evidence
    register"), and (d) lists the seven implementation-gate contracts as
    explicitly unresolved gates (§7). This satisfies the card's
    validation_plan[0] (verdict recorded for all 4 open_questions).
  receipt: >
    Bound to tree HEAD 689db76 at authoring; the memo is the deliverable for
    the policy question. The enforcement half is NOT claimed proven — it is
    explicitly gated (§7) and filed as a follow-up DEFER card (§8).
  scope_note: >
    result:proven is honest only for the POLICY crux (decided + recorded). No
    enforcement path was exercised — none exists. This closes the policy HALF
    of the card, not the implementation half.
```

## Follow-up

The enforcement-implementation half is filed as a separate DEFER card
(recorded in `.local/coordinator/tasks/` as transport, status `draft`). Its
ready-criteria inputs are the seven §7 contracts plus the raw-DB provisioning
seam trace (G1). It is **not** promoted here — promotion awaits the §7
contracts being resolved enough to meet the implementation Definition of
Ready. The policy memo is the deliverable for the policy question; the card's
policy half is closed by this file existing.
