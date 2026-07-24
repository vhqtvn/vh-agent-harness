# 2026-07-24 — Behavioral-closure pilot (verdict/crux gate, motivation-check fold)

- **Status:** DECIDED + implemented (pilot). Both gates cleared: plan
  `/approve`-d and the v0.15.1 release tag landed.
- **Supersedes / extends:** `2026-07-23-vh-solara-orchestration-field-report-disposition.md`
  (the disposition canon, commit `94afbc0`; HYBRID reframe addendum, commit
  `cddac9c`). This memo resolves the two design decisions the disposition left
  open under §4 P0-A and records the docker-gold honesty caveat as a durable
  carry-forward.
- **Scope:** the five-deliverable behavioral-closure pilot. Narrow and
  reversible.

## Context

The disposition canon split the behavioral-closure problem (§4.1 P0-A): a
behavior change must not be declared "done" when the load-bearing path that
proves it was never exercised end-to-end. The HYBRID addendum refined the
declaration model: merge same-property declarations, union different properties.
Two design decisions were left open and are resolved here.

## Resolved design decision 1 — validator host = `doctor`

The structural validator lives in **`internal/cli/doctor.go`** (a new
synchronous check, #14), NOT in `internal/cli/release_gate.go` and NOT merely
as task-closeout prompt wording.

- **Reason:** the validator must be mechanical, unbypassable, and cover
  closeouts that NEVER reach a release. doctor is the seam health surface that
  already scans `.local/coordinator/` and durable markdown; `release_gate.go`
  owns RELEASE properties (defer-liveness against shipped migration notes); the
  task-closeout command is advisory wording.
- **Load-bearing precondition (verified):** doctor can READ the closeout
  artifact in a stable machine-readable form. The artifact is markdown at
  `.local/coordinator/reports/<id>/<ts>-closeout.md` (referenced by the task
  card's `latest_report.path`), and a stable fenced `behavioral-closure` token
  makes it machine-parseable. **Precondition passed; decision 1 stands.**
- **Independence:** the new check reads closeout markdown directly
  (`os.ReadFile` + `filepath.WalkDir`). It does NOT go through the claims kernel
  and does NOT touch `release_gate.go`. Behavioral-completion truth is routed
  through doctor only.

## Resolved design decision 2 — motivation-check is FOLDED but a distinct UNION property

P1-B motivation-check is delivered in the same slice but as a SEPARATE
property, not merged into the verdict/crux. Guardrails (all satisfied):

1. both properties are named independently (behavioral closure vs motivation
   check);
2. separate success-criteria / verification rows;
3. verdict/crux is gate-shaped (doctor-enforced); motivation-check is advisory
   prose;
4. NO blended "closure passed" verdict.

This is the HYBRID addendum applied: same-property declarations merge, but the
motivation property is different and is unioned, not blended.

## The gate (docker-gold: structure + consistency ONLY)

The `behavioral-closure` token carries `verdict` (proven | inconclusive |
failed | abandoned) and `result` (proven | skipped | not-demonstrable — the
crux outcome), plus the crux path/verifier/command.

- Consistency rule: `verdict: proven` requires `result: proven`. Otherwise FAIL.
- Absent token = PASS (the pilot does NOT force adoption; forcing it would mark
  every pre-pilot closeout UNHEALTHY).
- Unknown enum / malformed block = FAIL (fail-closed on garbage, mirroring
  defer-liveness).

## docker-gold carry-forward (the honesty caveat)

**The token makes a declaration HONEST and non-droppable; it does NOT prove the
cited crux command actually executed.** A syntactically-consistent
`verdict: proven` + `result: proven` declares the path was exercised; proving it
needs the repo-specific live verification (the verified testing seam, the test
suite, the demo run). doctor rejects an internally-inconsistent declaration; it
does NOT, and cannot, prove the path ran. This caveat is stated in: the doctor
check's file-level comment, the AGENTS core testing clause, the closeout
template, and the prompt-guide anti-pattern (#10 Token-as-proof).

This is the authority line held: coordinator state INFORMS (WARN, "do not
release"); the safety layer ACTS (doctor / commit-gate / tests). No coordinator
transition authority was introduced; no behavioral truth was routed through
`release_gate.go`.

## Verification

- `go test ./...` passes, including `TestCheckBehavioralClosure` (proven+skipped
  FAILs, inconclusive+not-demonstrable PASSes, absent-token PASSes, garbage
  FAILs, **and the canonical-template regression** — a closeout copied verbatim
  from `CLOSEOUT_TEMPLATE.md` with its inline `#` enum comments PASSES),
  `TestAnalyzeBehavioralClosureBlocksPure`, and `TestStripInlineComment`.
- `gofmt` + `go vet` clean.
- `vh-agent-harness update --dry-run` then `vh-agent-harness update` regenerate
  `.opencode/` cleanly with no hand-edits to generated files.

## Behavioral closure (this pilot's own crux)

Per the pilot's defer-not-drop rule, the pilot that introduces the
`behavioral-closure` token declares its own crux rather than leaving it implied.
The load-bearing path for this change is the structural-consistency gate
actually rejecting an inconsistent declaration and accepting a consistent one.
(Documented here; `researches/decisions/` is not a doctor scan surface, so this
token is honest disclosure, not a gated artifact.)

```behavioral-closure
verdict: proven
path: internal/cli/doctor_behavioral_closure.go (checkBehavioralClosure)
verifier: go test ./internal/cli/ -run 'TestCheckBehavioralClosure|TestAnalyzeBehavioralClosureBlocksPure|TestStripInlineComment'
command: go test ./internal/cli/ -run 'TestCheckBehavioralClosure|TestAnalyzeBehavioralClosureBlocksPure|TestStripInlineComment'
result: proven
```

## Non-goals (held)

- `internal/memory/store/` stays DORMANT (not activated; concepts reused from
  `internal/memory/claims/` only — and this check does not even use claims).
- No project-specific literals in `templates/core/` (domain-free; tokens only).
- No backlog.md edits (none owed).
- §4.3 defer-liveness and §4.1 claims kernel are cited/extended, not rewritten.
