# Decision: Private Redlines — Resolution (adopted directions vs. original proposal)

**Date:** 2026-08-10
**Status:** Resolved (implemented in Slices 0–6; shipped)
**Basis:** the original proposal [`./2026-08-10-private-redlines-registry.md`](./2026-08-10-private-redlines-registry.md) (status: *Proposed — challengeable, not binding*); the seven coordinator-authoritative resolutions below; codebase seams cited inline.
**Supersedes:** the surface-specific positions of the original proposal (Decisions 1, 3, 4, 5, 6; Contract B) as noted per-row. The original memo's framing (K1 scrub-projects / K2 forbidden-relations), storage model (user-level XDG + optional repo-local additive), and schema (Contract A) stand unchanged.
**See also:** [`./2026-08-10-private-redlines-registry.md`](./2026-08-10-private-redlines-registry.md) (original proposal), [`./read-isolation-policy.md`](./read-isolation-policy.md)

## Purpose

The original proposal carried several surface-mechanism choices that, on
implementation, proved infeasible or inferior to alternatives. By repo convention
("record the reasoning in the decision-memo trail rather than deviating
silently"), this memo records each adopted direction, the original position it
supersedes, and the reason. The implemented surface is documented in
`README.agent.md` → "Private redlines".

All examples below use **synthetic terms only** (`subj-test-*`,
`synthetic-*`). No real operator knowledge appears anywhere in this file.

## Adopted directions

| # | Original memo position | Adopted direction | Reason |
|---|---|---|---|
| 1 | Feature-flag (`platform_armed`) enforcement via `features:` in `vh-harness-profile.yml` | **Always-compiled, registry-activated, otherwise inert** | Profile state is clone-local; a feature flag cannot protect a fresh clone that has not yet run setup (the operator's first commit on a new machine would be unprotected). "Zero footprint" means no generated state, no output, and no effect when no registry exists — not absence of compiled code. The code ships in every binary; the registry's absence is the inert gate. |
| 2 | Generated guidance doc (Surface 1) via `materializeContextDocs` appended to `opencode.jsonc` `instructions[]` | **Imperative `redlines guidance` command** (agent-invoked, local stdout) | `materializeContextDocs` is a fixed-set emitter writing 0644 files; the permission emitter does not own the `instructions[]` array; and off-corpus generated files trip the drift scanner. An imperative command (`redlines guidance`) dodges all three: no file is written, no `instructions[]` mutation, no drift surface. This restores the original memo's Decision 4 intent (full terms disclosed at the generation boundary) through a different mechanism. The boundary argument is unchanged: local agent context is not egress, and the commit gate is the mechanical backstop that catches any accidental echo into a commit. |
| 3 | Compiled deny-rules (Surface 2) via `loadPrivateDenyRules()` emitting `.opencode/repo-configs/forbidden-patterns.redlines.local.js` | **Deferred** — the commit gate scans direct from the registry | `loadPrivateDenyRules()` is implemented (not missing), but compiled outputs hit the same drift/ownership problems as the generated guidance doc (Surface 1). The scanner reads the registry directly; a compiled intermediate adds a materialization step without adding enforcement. Deferred to a future slice if the runtime deny-rule surface becomes load-bearing for a reason other than the commit gate. |
| 4 | `redlines scan --staged` (also `--range`, `--paths`, `--tracked`) | **Exact-tree locator (`--tree <hash>`); no fallback** | The commit gate operates on a private index/tree hash, not the shared `--staged` view. `--staged` is ambiguous between the shared index and a gate-private staging area; the tree hash is content-addressed and immutable, so the scan result cannot drift between the check and the write. `--range`/`--paths`/`--tracked` were dropped: the commit gate is the only consumer, and it needs exactly one locator (the tree the commit will land). |
| 5 | Gate + commit-reviewer both authoritative (Proposed Resolution 2) | **Gate authoritative; reviewer = optional defense-in-depth** | Only the gate binds to the exact acquired tree. The reviewer sees a different (working-tree or message-level) view. Making the reviewer authoritative would create a two-master problem where the reviewer could block a commit the gate cleared (or vice versa). The gate is the single authority; the reviewer may invoke `scan` advisorially but its verdict does not override the gate. |
| 6 | Registry 0600 "same as origin-hashes.json" (Decision 1) | **New secure-file contract (WARN-only `CheckFileSecurity`)** | There is no 0600 precedent in `internal/originhash`: that package's store is committed platform state written atomically with no explicit permission check. Origin hashes are not sensitive; redlines content is. A new platform-aware contract was authored (`internal/redlines/security.go`): POSIX-only, WARN on group/world-readable (`mode & 0o077 != 0`), never fails a load. Doctor surfaces the WARN; the registry is still trusted once it parses. |
| 7 | v1 lexical co-occurrence + ambient (Decision 5) | **Unchanged, with explicit honesty contract** | The original memo was correct. v1 detects lexical co-occurrence per unit plus ambient-repo degeneration. The honesty contract is printed verbatim by both `redlines guidance` (full) and `redlines scan` (one-line pointer): detection is best-effort, not proof. |
| 8 | `source_repos` schema field (Contract A: matching source of scrub-project) | **Rejected at load for v1 (fail-closed), mirroring the `unit: diff` precedent** | `source_repos` was schema-accepted, loaded, surfaced in guidance, and had an `IsSource()` predicate, but the engine's `scrubUnitMatches` only consults `Labels` — it NEVER matched `source_repos`. An operator relying on it got zero protection with no warning (the same honesty violation as `unit: diff`). v1 rejects any non-empty `source_repos` at registry load; operators MUST encode source-identifying fragments as `labels` (path-fragment labels are matched against both content and paths). `IsSource()` and the `SourceRepos` field are retained as reserved dead code for a future implementation that will derive path fragments from source repo identities. |
| 9 | (none — load-time gap found by commit-reviewer) | **Empty/whitespace-only terms in `labels` / `side_a` / `side_b` rejected at load (fail-closed), mirroring the `unit: diff` and `source_repos` precedent** | `validateSubject` checked term slices by length only, while the scanner's `scrubUnitMatches` / `anyTermInContent` both `continue` on empty terms (they skip rather than match), so `labels: [""]` (or `side_a` / `side_b`) loaded as a seemingly-valid subject that NEVER FIRED — a silent false-negative letting otherwise-blocked material into the acquired tree; v1 now rejects any element that is empty after `strings.TrimSpace` at registry load. |

## Slice-4 deviations (commit-gate integration)

Two implementation deviations from the gate's original integration sketch,
recorded for traceability:

**FLAG A — captures-and-discards on exit 0.** The scanner prints status lines on
exit 0 (a clean tree still reports what it checked). The gate captures the scan's
stdout and discards it on exit 0, so the gate's own output stays clean — no
redlines noise on a healthy commit. The opaque finding lines are shown only on
exit 1 (BLOCK `redlines_violation`) and exit 2 (BLOCK `redlines_error`). This
guarantees zero footprint for non-adopters and clean-tree commits.

**FLAG B — `redlines scan --help` probe for binary-version-skew robustness.** The gate probes
`vh-agent-harness redlines scan --help` before invoking `redlines scan`. If the binary is
absent or predates the `redlines scan` subcommand (an older clone on a teammate's
machine), the scan step is skipped silently — no block, no warning. This keeps
the gate forward-compatible: a version skew does not dead-lock commits. The probe
is the robustness layer; the scan itself is the enforcement layer. A future-
hardening idea (below) addresses the residual: a registry that exists on a
machine whose binary is missing/stale.

## Path-fragment nuance

Labels configured as path fragments naturally appear in `Finding.Path` — the
committed file's path — when the violation is a path match. This is safe: the
path is the *violation location being blocked*, not the *configured term echoed as
diagnostic*. The scanner's job is to BLOCK that path; the finding reports where
the leak is, which is structurally the same information the commit would have
landed. The `guidance` command is the only surface that echoes a configured term
*as a configured term*.

Example (synthetic): a scrub-project with label `synthetic-scrub-vocab` that
appears in the committed path `docs/synthetic-scrub-vocab-guide.md` produces a
finding `docs/synthetic-scrub-vocab-guide.md: subj-test-scrub (scrub-term)`. The
label appears because it is the violation's location, not because the scanner
echoed the registry. The opaque `subj-test-scrub` is the only registry-derived
identifier in the diagnostic.

## Deferred items (explicitly out of v1)

- **Compiled deny-rules + repo-mail wiring** (Surface 2): the runtime deny-rule
  surface and `loadPrivateDenyRules()` integration. Deferred per row 3.
- **LLM `--deep` scan**: semantic/paraphrase inference following the auto-gate LLM
  pattern. Out of scope for v1 (honesty contract row 7).
- **Profile-flag enforcement**: the `features:`/`platform_armed` gate. Replaced by
  always-compiled-inert per row 1; not re-adding.
- **Cross-machine sync**: operator's private dotfiles problem; the registry must
  never ride in any project repo.
- **History-scrub remediation**: `scan` reports; rewriting already-committed
  history is an operator incident action, not a harness feature.
- **Future hardening — fail-closed when registry-exists-AND-binary-missing**: the
  current skip-silently (FLAG B) means a machine with a registry but a stale
  binary scans nothing. A bash-layer registry-discovery probe (the gate shell
  checks for the registry file directly) could fail-closed in that narrow case.
  Not v1: it would duplicate the Go discovery path in bash and risks false
  negatives on path resolution. Tracked as a future hardening idea.

## Verification

- `go test ./internal/redlines/...` — registry discovery, loadability (inert +
  fail-closed), binding, scanner (exit codes + paste-safety), security helper.
- `go test ./internal/cli/ -run "TestRedlines|TestDoctorRedlines|TestCheckPrivateRedlines|TestCommitGateRedlines"` —
  CLI guidance, scan, doctor hygiene check, commit-gate integration.
- Every diagnostic test carries an `assertNoLeak` guard over synthetic terms.
- `gofmt`, `go vet ./...`, `go build ./...` green.
