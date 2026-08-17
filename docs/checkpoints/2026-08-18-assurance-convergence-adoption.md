# Checkpoint: Assurance-Convergence Adoption (Narrow Program)

**Date:** 2026-08-18
**Status:** Implemented — narrow adoption set landed; adopt-later list trigger-gated
**Provenance:** debate→planner verdict on the deepseek-harness study
(`researches/sources/deepseek-harness/`, indexed 2026-08-17). This record
preserves the verdict durably against actual code/validation state.

## Adopt-now set (landed 2026-08-18, backlog P1-CI-002)

| Slice | dsh idea | What landed |
| --- | --- | --- |
| P1 CI contract test | eng-platform (b) CI-workflow-under-test | `internal/ci_contract_test.go` — parses go-test/render-check/release YAMLs, asserts durable blocking properties (incl. the deliberate dryrun+Fail-on-drift pair) |
| P2 stable aggregate | eng-platform (e) single-aggregate required check | `ci-aggregate` job in `.github/workflows/go-test.yml`: `if: always()`, needs all blocking jobs, fails on failure/cancelled/unexpected-skip, emits per-dependency summary; B1-vs-B2 semantics kept distinct in its output |
| P3 commit-gate audit | eng-platform (l) lock-safety contract + exec-safety #24 | AUDIT-ONLY: all six audited properties already hold (atomic mkdir; identity-bound stale-break; never delete unidentifiable lock; path validation before privileged rm; flock-serialized closeout ledger; cleanup swallowed only post-outcome). No changes — no testable gap found |
| P4 postmortem convention | exec-safety #9 postmortem discipline | `docs/ai/postmortem-convention.md` — class→mechanism→one guardrail→verifier rows over EXISTING durable records only; no new archive/ledger |

## Adopt-later (trigger-gated; NOT adopted — no seam exists yet)

- (a) gen/verify self-verifying catalogs — **trigger:** a new committed generated catalog lacking doctor re-derivation.
- (d) registry-decided idempotent publish — **trigger:** a rerunnable registry publish mechanism exists.
- (n) model-request-plane mock assertions — **trigger:** a model/provider request seam exists.
- (o) detached-worktree packing — **trigger:** release packing needs checkout isolation.
- exec-safety #11 invariant companion registry — **trigger:** package-owned doctor checks exist.
- exec-safety #15–#23 (env tombstones, `env -i`, randomized HOME, publish-then-verify, refuse ids ≤1, already-gone tolerance, AggregateError, zombie-excluding liveness, PID start-identity) — **trigger:** the matching execution machinery lands.

## Rejected this wave (with reasons)

- Execution-world seam (#13): no remote-sandbox adapter roadmap; would be a parallel stack, not adapter mounting.
- Session-log-as-store / persistence changes (#4, E12): plan-state persistence is out of scope for a narrow assurance wave.
- Frozen-archive manifests (c): violates the no-new-ledger constraint; existing checkpoints + RECORD_LIFECYCLE suffice.
- New generated catalogs: no new catalogs; doctor drift checks already cover rendered output.
- Release-ceremony / M-commit changes: releaser-owned N→R→M ceremony untouched by design.
- Other packet ideas (#1–#3, #5–#8, #10, #12, #14, f–k, m): not in this wave's candidate set — unadjudicated, not rejected on merits.

## Findings

- **P3 all-properties-hold**: source=`.opencode/scripts/commit-gate.sh` full read (2026-08-18), confidence=high, type=fact. One accepted tradeoff recorded: a dead writer between lock mkdir and meta write leaves a never-stale zombie lock; recovery is the operator escape hatch (documented fail-safe choice, lines 607–620).
- **Live-run of ci-aggregate not yet observed**: type=fact. Structural properties verified offline; first hosted exercise is the next push/PR after landing.

## Contradictions

None detected. (Mission-settled seams confirmed: yaml.v3 test seam exists; the three workflow files are real repo files; render-check's continue-on-error pattern is deliberate and the contract test encodes it.)

## Verification

| Claim | Verifying command/output | Verified |
|-------|--------------------------|----------|
| P1 tests green + mutation-sensitive | `go test ./internal/ -run TestCIContract_ -count=1` → 4/4 PASS; weakened `if: success()` and unwired Fail-on-drift each → FAIL | yes |
| P2 YAML parses + properties asserted | same suite (parse failure would fail tests); ci-aggregate job asserted | yes |
| P2 live-run behavior | — (first push/PR after landing) | no — honestly not-demonstrable pre-merge |
| Full suite, revision-bound | `go test ./... -count=1` → 32 ok / 0 fail on this tree | yes |
| `gofmt -l .` clean; `go vet ./...` clean | both → no output, exit 0 | yes |
| JS seam baseline (P3 unchanged) | `make test-js` → 157 pass / 0 fail | yes |
