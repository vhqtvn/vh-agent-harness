# Checkpoint: G0 Green-Gate Receipt — v0.26.2..HEAD Arc (v0.27.0 Release-Readiness)

**Date:** 2026-10-01
**Status:** Green — all four gate legs passed, bound to the tree below
**Purpose:** G0 green-gate receipt for the v0.26.2..HEAD arc (v0.27.0
release-readiness). Drains the G0 blocker from the v0.27.0 release-readiness
assessment (last recorded gate evidence was 2026-08-18, pre-arc; the arc had
no recorded gate run).

## Tree binding

- **HEAD at run time:** `5057fa69b1b7e78679e2c7552e46f1ed54af0512`
  (re-derived with `git rev-parse HEAD` immediately before the run; subject
  line `test(cli): pin researcher steps override on live surfaces`).
- **Working tree:** clean — `git status --short` produced no output both
  immediately before and immediately after the gate run.
- **Arc covered:** `git log --oneline v0.26.2..HEAD` → 26 commits
  (e5dd37e..5057fa6).
- **Premise drift note:** the slice was dispatched against HEAD `7c184e8`;
  two commits landed before execution (`64d4d53`, `5057fa6`).
  `git merge-base --is-ancestor 7c184e8 HEAD` confirms `7c184e8` is an
  ancestor of the run-time HEAD, so this receipt covers a superset of the
  dispatched arc. Evidence is bound to the run-time HEAD, per the
  premise-recheck protocol.

## Commands run and outcomes

Form note: the four commands were run explicitly via
`vh-agent-harness exec` (not `make check`/`make test`): the Makefile `check`
target's `fmt` leg mutates (`gofmt -w .`) which is wrong for an
evidence-only slice, and `make test` omits `-count=1` (would allow cached
results). The four-command set matches the AGENTS.md non-negotiable gate
("`go test ./...`, `gofmt`, and `go vet` must pass").

| Command (exact form run) | Outcome |
|--------------------------|---------|
| `go test ./... -count=1` | PASS — exit 0; **32 ok / 0 fail** (3 packages `[no test files]`: `cmd/vh-agent-harness`, `internal/copieranswers`, `tmp/review-s2-parity`); slowest legs `internal/cli` 57.99s, `internal/permission` 38.99s |
| `go vet ./...` | PASS — exit 0, no output |
| `go build ./...` | PASS — exit 0, no output |
| `gofmt -l .` | PASS — empty output (no unformatted files) |

Scope note: the JS suite (`make test-js`) is not part of this four-command
G0 gate and was not run in this slice; it was last recorded green 2026-08-18
(157 pass / 0 fail, see `docs/checkpoints/2026-08-18-assurance-convergence-adoption.md`).
Docker-dependent suites (`test-auto-gate-live`, `test-e2e-auto-gate*`) are
separate opt-in make targets, also outside this gate.

## Findings

- **All four gate legs green at run-time HEAD**: source=this run's captured
  outputs (above), confidence=high, type=fact. Uncached (`-count=1`) test
  execution; result bound to `5057fa6` with a clean tree (B1 full
  verification + B2 clean transition state both hold for this evidence run).
- **Dispatched HEAD premise was stale**: source=`git rev-parse HEAD` at run
  time vs dispatched `7c184e8`, confidence=high, type=fact. Re-derived and
  receipt re-bound per protocol; ancestor check confirms superset coverage.
- **`tmp/review-s2-parity` appears as a `[no test files]` package**: type=fact,
  confidence=high. Pre-existing scratch package path picked up by `./...`;
  harmless to the gate (no tests, no failure). Left untouched — evidence-only
  slice.

## Contradictions

None detected. (Dispatched HEAD differed from run-time HEAD; resolved as
documented above — the mission explicitly required re-derivation at run time,
so this is premise drift handled by protocol, not a conflict.)

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| Run-time HEAD is `5057fa6…` | `git rev-parse HEAD` → `5057fa69b1b7e78679e2c7552e46f1ed54af0512` (immediately before the run) | yes |
| Working tree clean pre-run | `git status --short` → no output | yes |
| Go tests green, uncached | `go test ./... -count=1` → 32 ok / 0 fail, exit 0 | yes |
| Vet green | `go vet ./...` → no output, exit 0 | yes |
| Build green | `go build ./...` → no output, exit 0 | yes |
| Formatting clean | `gofmt -l .` → empty output | yes |
| Working tree clean post-run | `git status --short` → no output | yes |
| Arc is v0.26.2..HEAD, 26 commits | `git log --oneline v0.26.2..HEAD` → 26 lines (e5dd37e..5057fa6) | yes |
| Dispatched HEAD is ancestor of run-time HEAD | `git merge-base --is-ancestor 7c184e8 HEAD` → exit 0 | yes |

## Behavioral-closure note

This slice is evidence-recording, not a behavior change: no production or
test code was modified, no load-bearing behavior path is claimed as a crux,
and no `behavioral-closure` verdict is asserted. The B1/B2 claims above are
scoped to this gate run only.
