# Auto-Gate-Ignore Doctor Check & Seed Gitignore — Adopter Never-Commit Gap Closed

**Date:** 2026-07-16
**Status:** Landed on `main` (commits `d4cd818`, `73be4f5`, the top two commits at
HEAD); Go gates green; full suite green. **Not yet released** — the installed
harness binary is `0.11.0 (7f29378)`, which predates this work, so a freshly
built/released binary is required for the live `doctor` to surface the check.
**Scope of record:** durable decision + implementation checkpoint for the
adopter gitignore/doctor gap on the `auto-classifier-pilot` overlay. This is a
record of committed, verified state, not a plan; nothing here transitions state.

## TL;DR

Adopters of the `auto-classifier-pilot` overlay create two classes of
**never-commit** config under `.opencode/repo-configs/` — `auto-gate-llm.json`
(which may hold a literal `apiKey`) and `*.local.json` personal overrides
(highest-precedence, can change effective `mode`). Neither was gitignored by
default and no doctor check detected an un-ignored/un-tracked instance. A
solution-brief enumerated **12 problems** across **4 families** (prevention,
disclosure, policy/drift, detection/remediation). The agreed **A+B+D hybrid**
landed on `main`: a read-only `auto-gate-ignore` doctor check (**A**, detection),
seeded `templates/core/.gitignore` rules (**B**, prevention), and documented
owner remediation + CI gate with two honest residuals (**D**, process). Mechanism
**C** (overlay auto-injection into `.gitignore`) was **rejected**; an explicit
owner-authorized reconciliation command (**D8**) is **deferred**. The **D6
secret-never-emits** contract is a hard invariant, pinned by leak-assertion tests.

## Problem (12 problems, 4 families)

Grounding facts: `.gitignore` is **`project_owned`** — seeded once, preserved on
update (`core_manifest.go:170` `".gitignore": ownership.ClassProjectOwned`); the
overlay README documents both file classes as NEVER committed and permits a
literal `apiKey`; the pre-existing doctor gitignore probe covered only harness
runtime dirs.

- **Prevention gap** — P01: fresh/template repos begin unsafe (no rule by default).
- **Disclosure risk** — P02: a literal API credential on disk; P03:
  secrets-adjacent infra; P10: external CI artifact collectors bypassing
  `.gitignore`; P12: a prior disclosure that cannot be un-leaked.
- **Policy/drift** — P04: a personal override becoming team/CI policy; P05:
  personal drift and merge churn.
- **Detection/remediation gap** — P06: doctor false-clean; P07: existing-repo
  adoption unprotected; P08: a future seed fix never reaching the installed base
  (`.gitignore` is `project_owned`/preserved); P09: later overlay opt-in
  reopening the gap; P11: global/nested/negation rules giving false confidence.

## Resolution shape (A+B+D hybrid)

- **A — detection:** a new read-only `auto-gate-ignore` doctor check that
  evaluates **effective Git ignore resolution AND tracked state**, inert (SKIP)
  unless the overlay is selected or a relevant config file exists (v0.7.0
  discipline).
- **B — prevention:** seed `templates/core/.gitignore` with both rules so future
  greenfield installs are safe by default.
- **D — remediation/process:** document the required CI gate (`vh-agent-harness
  doctor` as a pre-stage/pre-package checkpoint) and exact owner remediation;
  state P10 (external collectors) and P12 (disclosed credentials) as honest
  residuals the harness cannot autonomously close.
- **C — REJECTED:** overlay auto-injection into `.gitignore` — violates the
  `project_owned`/transition-authority contract.
- **D8 — DEFERRED:** an explicit owner-authorized `.gitignore` reconciliation
  command — deferred pending demand evidence + a reviewed project-owned-file
  mutation contract.

## The 8 decisions (from a debate pass) and the 3 state classes

- **D1 (applicability):** non-SKIP when the overlay is selected OR a relevant
  config file exists; otherwise inert/SKIP.
- **D2 (selected, no file, no rule):** WARN — a readiness nudge, not an incident.
- **D3 (present + effectively unignored):** FAIL, **uniform across both file
  classes** (llm + local) — a split tier would invite treating `local` as
  safely-committable.
- **D4 (tracked):** FAIL unconditional, even if an ignore rule now matches —
  ignores do not untrack.
- **D5 (global-exclude-only / `.git/info/exclude`):** WARN — non-portable, but
  not an active local breach.
- **D6 (tracked `auto-gate-llm.json` with non-empty literal `apiKey`):** FAIL +
  require rotation/incident guidance; **NEVER emit the key value.**
- **D7 (CI contract):** REQUIRE `vh-agent-harness doctor` as a pre-stage/
  pre-package CI gate for overlay users; P10/P12 remain named process residuals.
- **D8 (reconciliation command):** DEFER.

Three state classes result: **readiness/portability → WARN** (D2, D5);
**active never-commit breach → FAIL** (D3, D4); **credential incident →
FAIL + rotate** (D6).

## D6 secret-never-emits contract (hard invariant)

`autoGateLlmHasLiteralKey` (`internal/cli/doctor.go:1656`) and the extracted
`hasNonEmptyLiteralKey` (`:1688`) return a **bare `bool`**; the `apiKey` value is
bound to a function-local used only in `s != ""` and discarded. It never escapes,
is never interpolated/logged/serialized; the D6 FAIL detail
(`doctor.go:1520-1521`) interpolates **only the file path**; on read/parse/type
errors both return `false` (the D4 tracked-state FAIL still applies via the
caller; only the rotate guidance is dropped). Regression-pinned by leak-assertion
tests.

## Implementation (landed on `main`)

- **`d4cd818`** — `feat(doctor): add auto-gate-ignore check and seed .gitignore
  for never-commit config paths`. 5 files, +624/-11:
  `internal/cli/doctor.go` (new `checkAutoGateGitignored` check + helpers + 2
  vars + type; the check table documents it at `:60` and it is invoked at
  `:178-181`), `internal/cli/autogate_ignore_check_test.go` (the check's test
  set), `templates/core/.gitignore` (both seed rules, `:67-68`), `README.agent.md`
  (check + CI-gate subsection), `templates/overlays/auto-classifier-pilot/README.md`
  (never-commit + CI gate section, `:455`/`:492-493`).
- **`73be4f5`** — `test(doctor): extend D6 literal-key detection to
  leaves[].apiKey and cover .git/info/exclude D5 path`. 2 files, +145/-6:
  `autoGateLlmHasLiteralKey` now also inspects `leaves[].apiKey` (extracted
  `hasNonEmptyLiteralKey`) and a `.git/info/exclude` D5 test variant was added.
  Rotate-guidance completeness; no live safety hole existed (D4 still fired).

The check is currently covered by **10 tests** in `autogate_ignore_check_test.go`
(D1–D6), all green.

## Honest residuals (cannot be autonomously closed)

- **P10** — external CI artifact collectors may ignore `.gitignore`; mitigated
  (not closed) by the required doctor CI gate (D7).
- **P12** — a disclosed credential cannot be un-leaked by any ignore rule;
  rotation/revoke (+ possible history rewrite) is an owner-driven incident
  action.

## Verification

Commands re-run this session against the working tree (HEAD = `73be4f5`; no
working-tree changes to `.go` files, so the Go gates reflect the committed
tree).

| Claim | Verifying command / output | Verified |
|-------|----------------------------|----------|
| `d4cd818` exists; 5 files; +624/-11 | `git show --stat d4cd818` → `5 files changed, 624 insertions(+), 11 deletions(-)` | yes |
| `73be4f5` exists; 2 files; +145/-6 | `git show --stat 73be4f5` → `2 files changed, 145 insertions(+), 6 deletions(-)` | yes |
| Both commits are on `main` (at HEAD) | `git log --oneline -6` → top two commits are `73be4f5`, `d4cd818` | yes |
| `gofmt` clean | `gofmt -l .` → no output; exit 0 | yes |
| `go vet` clean | `go vet ./...` → no output; exit 0 | yes |
| `go build` clean | `go build ./...` → no output; exit 0 | yes |
| Full test suite green | `go test ./...` → every package `ok`; no `FAIL`; pipe-exit 0 | yes |
| `auto-gate-ignore` check registered in source | `internal/cli/doctor.go:60` (check-table doc) and `:178-181` (invocation) | yes |
| 10 auto-gate-ignore tests green | `go test -run TestAutoGate ./internal/cli/` → 10 `--- PASS` (D1–D6) + `ok` | yes |
| PASS path test-verified (overlay selected + protected) | `TestAutoGateIgnore_PassWhenSelectedAndProtected` → `--- PASS` | yes |
| D6 never-emits pinned by leak-assertion tests | `go test -run 'FailTrackedLiteralKeyRotate\|TestAutoGateLlmHasLiteralKey\|FailTrackedLiteralKeyInLeaves' ./internal/cli/` → all `--- PASS` (top-level + leaf; 11 subtests) | yes |
| D6 detail interpolates only the path, never the key | `doctor.go:1520-1521` FAIL string uses only `pf.rel`; `autoGateLlmHasLiteralKey`/`hasNonEmptyLiteralKey` return bare `bool`, errors→`false` (`:1656-1694`) | yes |
| `.gitignore` is `project_owned` (seeded once, preserved) | `core_manifest.go:170` `".gitignore": ownership.ClassProjectOwned`; doc `:38`, case `:92` | yes |
| Both seed rules present | `templates/core/.gitignore:67-68` → `.opencode/repo-configs/*.local.json` and `.opencode/repo-configs/auto-gate-llm.json` | yes |
| Overlay README carries never-commit + required CI gate | `templates/overlays/auto-classifier-pilot/README.md:455` (Never-commit paths & CI gate), `:492-493` (CI gate = `vh-agent-harness doctor` pre-stage/pre-package) | yes |
| commit-review APPROVED for both slices | Both slices present at HEAD via the gated-commit protocol (which gates on commit-review). No separate durable verdict artifact is committed — review verdicts are session-ephemeral per the repo model; gate clearance is the durable evidence. | yes* |
| (Caveat) installed binary does not yet surface the check | `vh-agent-harness version` → `0.11.0 (7f29378)`; `vh-agent-harness doctor` output omits the `auto-gate-ignore` line. The binary predates `d4cd818`; source + tests are the verification surface until the next build/install. | yes (caveat) |

\* Gate clearance verified from commit presence; the historical verdict text
itself is not independently re-derivable from a stored artifact.

## Findings

- **Two never-commit file classes, neither protected by default.** source =
  overlay README + the 12-problem solution-brief; confidence = high; type = fact.
  `auto-gate-llm.json` (literal `apiKey` possible) and `*.local.json` (highest
  precedence, can change `mode`) were neither gitignored nor doctor-checked.
- **`.gitignore` is `project_owned`.** source = `core_manifest.go:170`; confidence
  = high; type = fact. Seeded once, preserved on update — the root cause of P08
  (a future seed fix never auto-reaches the installed base) and why mechanism C
  (overlay auto-injection) would violate the contract.
- **Resolution = A+B+D hybrid; C rejected; D8 deferred.** source = solution-brief
  + debate pass + commits `d4cd818`/`73be4f5`; confidence = high; type = decision.
  Detection (A) + prevention (B) + process (D); no overlay writes to a
  `project_owned` file.
- **D6 secret-never-emits is a hard, source-verified invariant.** source =
  `doctor.go:1656-1694` + `:1520-1521` + leak-assertion tests; confidence = high;
  type = fact. Bare-bool return, function-local value, path-only detail,
  errors→`false`.
- **Three state classes (WARN/FAIL/FAIL+rotate) implemented as decided.** source =
  `doctor.go:1502-1568` aggregation; confidence = high; type = fact. D2/D5 WARN;
  D3/D4 FAIL; D6 FAIL+rotate; D1 SKIP gate; uniform FAIL across llm + local (D3).
- **Honest residuals P10/P12 cannot be autonomously closed.** source = overlay
  README + D7; confidence = high; type = inference. Mitigated (P10) or
  owner-driven (P12), not solved by any ignore rule.
- **Installed harness binary predates the change.** source = `vh-agent-harness
  version` = `0.11.0 (7f29378)` vs `d4cd818` at HEAD; confidence = high;
  type = fact. Live `doctor` in this workspace omits the line by design until a
  rebuild/release; behavior is verified at source + test level.

## Contradictions

- **Live `doctor` omits the `auto-gate-ignore` line (resolved-by-explanation,
  not a defect).** The check exists in source and is invoked unconditionally
  (`doctor.go:178-181`), yet running `vh-agent-harness doctor` in this workspace
  does not print it. Cause: the installed binary is `0.11.0 (7f29378)`, which
  predates `d4cd818`. Resolution: the check's behavior (incl. the PASS path) is
  verified at source + unit-test level; a freshly built/released binary will
  surface the line. Recorded so a reader running `doctor` does not misread the
  absence as a regression.
- **No material contradiction in the landed work itself.** The brief's "7 tests"
  attribution to `d4cd818` is the pre-`73be4f5` snapshot; the file currently
  holds 10 auto-gate-ignore tests (all green). Recorded at the verified current
  count above.

## Obligations

- **Satisfied:** detection check (A) implemented, registered, and test-verified;
  seed rules (B) in `templates/core/.gitignore`; owner remediation + CI gate (D)
  documented in `README.agent.md` and the overlay README; D6 never-emits pinned;
  Go gates + full suite green at HEAD.
- **Remaining:** not yet released — the next harness release/build + install
  surfaces the check in the live `doctor`. D8 (owner-authorized reconciliation
  command) remains deferred pending demand + a reviewed project-owned-file
  mutation contract. Backlog closeout for this task is out of scope for this
  record.
