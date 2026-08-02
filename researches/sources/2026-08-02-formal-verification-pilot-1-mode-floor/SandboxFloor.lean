/-
  Formal-verification S1 pilot — exec-sandbox mode-floor SAFETY invariant.
  Engine: Lean 4 v4.32.2 (Lean core ONLY; NO mathlib, NO Batteries).
  Provisioned image: formal-verify/lean4:v4.32.2.

  MODELS (see FIDELITY-BINDING.md for the full model↔code mapping):
    internal/cli/exec_sandbox.go          :: applyFloorToRequest
      -> internal/execsandbox/floor.go    :: ApplyFloor(requested, floor)
           + modeRank (the strictness order off < best-effort < strict)

  INVARIANT (the load-bearing containment property):
    A caller can NEVER downgrade below the configured floor.
    The effective mode  E = MAX(floor, caller) ; therefore  floor ≤ E .

  SCOPE / HONESTY:
    This file proves the MODEL, not the CODE. A proof of the model is not a
    proof of the code. The concrete proof↔code binding is the red-on-divergence
    Go test next to exec_sandbox.go
    (TestApplyFloorToRequest_FloorSafetyInvariant_FVBinding in
    internal/cli/exec_sandbox_floor_test.go), which goes RED the moment
    ApplyFloor drops the max (e.g. flipped to min / floor ignored).
    This is INFORM-only output; it never gates commits, releases, doctor, or
    updates.
-/

-- The three sandbox modes, ordered  off < best-effort < strict.
-- Mirrors execsandbox.SandboxMode (internal/execsandbox/profile.go) and its
-- modeRank (internal/execsandbox/floor.go): ModeOff=0, ModeBestEffort=1, ModeStrict=2.
inductive Mode where
  | off        : Mode
  | bestEffort : Mode
  | strict     : Mode
  deriving DecidableEq, Repr

/-- rank: the strictness order. Mirrors execsandbox.modeRank EXACTLY.
    Higher rank = more strict (more containment). -/
def rank : Mode → Nat
  | Mode.off        => 0
  | Mode.bestEffort => 1
  | Mode.strict     => 2

/-- `ApplyFloor requested floor` mirrors `execsandbox.ApplyFloor(requested, floor)`:
    the STRICTER (higher-rank) of the two wins, so a caller can never run below
    the floor. Go (internal/execsandbox/floor.go):
      `if modeRank(floor) > modeRank(requested) { return floor }; return requested` -/
def ApplyFloor (requested floor : Mode) : Mode :=
  if rank floor > rank requested then floor else requested

/-- SAFETY INVARIANT (headline): the floor's rank is ≤ the effective rank.
    The caller can NEVER downgrade below the floor.

    This is the `max`-lower-bound law (`Nat.le_max_right`-level) inlined: after
    unfolding `ApplyFloor`, the two branches reduce to
      (rank floor ≤ rank floor)              -- floor branch: trivial
      (rank floor ≤ rank requested)          -- requested branch: from ¬(floor>requested)
    Neither goal contains a `Nat.max` atom, so `omega` discharges both. -/
theorem floor_le_effective (requested floor : Mode) :
    rank floor ≤ rank (ApplyFloor requested floor) := by
  unfold ApplyFloor
  by_cases h : rank floor > rank requested
  · rw [if_pos h]
    omega
  · rw [if_neg h]
    omega

/-- Symmetric: the requested mode's rank is ≤ the effective rank — a caller
    asking STRICTER than the floor keeps that strictness, and the floor never
    downgrades an already-stricter caller. `Nat.le_max_left`-level; same shape. -/
theorem requested_le_effective (requested floor : Mode) :
    rank requested ≤ rank (ApplyFloor requested floor) := by
  unfold ApplyFloor
  by_cases h : rank floor > rank requested
  · rw [if_pos h]
    omega
  · rw [if_neg h]
    omega

/-- Exhaustive witness: across all 9 (requested, floor) pairs, the floor's rank
    is ≤ the effective rank. A CLOSED decidable proposition; `decide` evaluates
    it concretely (a different mechanism than the tactic proof above), so a
    rank/ApplyFloor regression is a compile failure here too. -/
theorem floor_le_effective_all_pairs :
    rank Mode.off        ≤ rank (ApplyFloor Mode.off        Mode.off)        ∧
    rank Mode.off        ≤ rank (ApplyFloor Mode.bestEffort Mode.off)        ∧
    rank Mode.off        ≤ rank (ApplyFloor Mode.strict     Mode.off)        ∧
    rank Mode.bestEffort ≤ rank (ApplyFloor Mode.off        Mode.bestEffort) ∧
    rank Mode.bestEffort ≤ rank (ApplyFloor Mode.bestEffort Mode.bestEffort) ∧
    rank Mode.bestEffort ≤ rank (ApplyFloor Mode.strict     Mode.bestEffort) ∧
    rank Mode.strict     ≤ rank (ApplyFloor Mode.off        Mode.strict)     ∧
    rank Mode.strict     ≤ rank (ApplyFloor Mode.bestEffort Mode.strict)     ∧
    rank Mode.strict     ≤ rank (ApplyFloor Mode.strict     Mode.strict) :=
  by decide

-- Sanity: the ordering used by the model matches the Go modeRank exactly.
-- (Each reduces by `rfl` on the constructor; a rank-mapping drift makes these
-- a compile failure.)
example : rank Mode.off        = 0 := rfl
example : rank Mode.bestEffort = 1 := rfl
example : rank Mode.strict     = 2 := rfl
