# Fidelity Binding — exec-sandbox mode-floor safety invariant (FV S1 pilot)

**Pilot:** first real run of the S1 overlay skill at
`.opencode/skills/formal-verification/`.

**Engine:** Lean 4.32.2 (`x86_64-unknown-linux-gnu`, commit
`f3b06c705e6c85f5314019d5d3baab0fec5b580c`, Release), from the
operator-provisioned image `formal-verify/lean4:v4.32.2`. Invoked offline:
`docker run --rm --network=none -v <dir>:/work:ro formal-verify/lean4:v4.32.2 lean /work/SandboxFloor.lean`
→ exit status 0 (green), no diagnostics.

**Proof artifact:** `researches/sources/2026-08-02-formal-verification-pilot-1-mode-floor/SandboxFloor.lean` (Lean core
only — no mathlib, no Batteries).

**Red-on-divergence test:** `internal/cli/exec_sandbox_floor_test.go`,
`TestApplyFloorToRequest_FloorSafetyInvariant_FVBinding`.

---

## The invariant (what is being modeled)

The exec-sandbox **mode-floor**: a caller can NEVER run below the configured
floor. For a floor `F` and any caller-requested mode `M`, the effective mode
`E = max(F, M)` satisfies `F ≤ E` (equivalently, `rank(F) ≤ rank(E)`). This is
the load-bearing containment property that makes a plain
`exec-sandbox *` permission grant safe — an agent cannot escape `strict` by
passing `--sandbox=off`, by duplicating the flag, or by interspersing it.

## Model ↔ code mapping (explicit)

| Lean model (`SandboxFloor.lean`) | Go code | Notes |
|---|---|---|
| `inductive Mode` (`off` / `bestEffort` / `strict`) | `execsandbox.SandboxMode` (`ModeOff` / `ModeBestEffort` / `ModeStrict`) at `internal/execsandbox/profile.go` | The three ordered states. |
| `rank : Mode → Nat` (`off=0`, `bestEffort=1`, `strict=2`) | `modeRank(m)` at `internal/execsandbox/floor.go` (`ModeStrict=2`, `ModeBestEffort=1`, `ModeOff=0`) | The strictness order. Higher = more strict. Identical mapping. |
| `ApplyFloor requested floor := if rank floor > rank requested then floor else requested` | `func ApplyFloor(requested, floor SandboxMode) SandboxMode` at `internal/execsandbox/floor.go` — `if modeRank(floor) > modeRank(requested) { return floor }; return requested` | **This is the MAX** (the stricter of the two wins). The proof models this function literally. |
| `floor_le_effective`: `∀ requested floor, rank floor ≤ rank (ApplyFloor requested floor)` | The containment guarantee enforced via `applyFloorToRequest` → `ApplyFloor`. | The headline safety invariant. |
| `applyFloorToRequest(reqMode, reqNet, realCWD, repoRoot)` (`internal/cli/exec_sandbox.go`) | — | The CLI-layer clamp. It takes the **MAX over the chain** `{realCWD-floor, repoRoot-floor}` (via `execsandbox.ApplyFloor(floor, f)` in the loop) and then `effMode := ApplyFloor(reqMode, floor)`. The proof models the inner `ApplyFloor`; the chain-MAX is the same `max` law applied twice and is covered by transitivity of `≤`. |

## What the model abstracts (and does NOT prove)

- **Proves (the model):** for the `ApplyFloor` function as defined in Lean, the
  floor's rank is always ≤ the effective rank, universally over all
  `(requested, floor)` pairs. Stated and discharged two ways: a tactic proof
  (`floor_le_effective`, `requested_le_effective`) and a closed decidable
  witness evaluated by `decide` over all 9 pairs
  (`floor_le_effective_all_pairs`).
- **Does NOT prove (the code):** the Go implementation is not extracted from
  the proof, and no model↔code equivalence is mechanically established. The
  binding is a reviewed, falsifiable mapping — the red-on-divergence test is
  the cheapest concrete recheck that the code's `ApplyFloor` actually computes
  the `max` the model assumes.
- **Abstracts away:** the walk-up `runshape.FindMinMode` filesystem resolution,
  the run-shape YAML decoding, the fail-closed error paths, the net-policy
  clamp, and the kernel `execsandbox.Run` enforcement. These are exercised by
  the existing `internal/cli/exec_sandbox_floor_test.go` and
  `internal/execsandbox/floor_test.go` suites; the FV pilot deliberately models
  only the pure `max`-over-modes arithmetic that is the crux of the invariant.
- **The chain-MAX in `applyFloorToRequest`** (`floor = ApplyFloor(floor, f)` in
  the `{realCWD, repoRoot}` loop) is the same `max` law. `floor ≤ max(floor, f)`
  by the same lower-bound fact, so the chain cannot weaken the floor either;
  this is noted here, not separately re-proven.

## Distinct-union evidence (skill condition 4)

The engine check (green) AND this fidelity binding AND the red-on-divergence
test must all hold. Fail-closed if any fails.

- **Engine check:** GREEN — `lean /work/SandboxFloor.lean` exit 0, Lean 4.32.2.
- **Fidelity binding:** this document (reviewed mapping above).
- **Red-on-divergence test:** `TestApplyFloorToRequest_FloorSafetyInvariant_FVBinding`,
  verified to go RED when `ApplyFloor`'s max is deliberately broken (flipped to
  min), and GREEN on the faithful implementation. See the test's comment and
  the closeout report for the captured red/green outputs.

## Anti-over-claim / anti-laundering (restated)

A proof of the **model** is not a proof of the **code**. This pilot's outputs
**INFORM only** — they never gate commits, releases, `doctor`, or `update`. The
green engine check is a token that the modeled invariant holds in the model; it
does not certify the Go code. The code's correctness rests on the repo's own
live verification (the existing floor tests + the kernel dogfood probes), of
which the new red-on-divergence test is one targeted piece.

## Classification decision (skill "Invocation / Classification Rule")

Branch **(a)** — pure-logic / algebraic invariant over a finite ordered enum →
Lean4. No concurrency to model (the floor clamp is single-threaded arithmetic),
so TLAPS is not indicated; no liveness/temporal component; TLC/model-checking
not used (the agent authors the proof; the engine only checks).
