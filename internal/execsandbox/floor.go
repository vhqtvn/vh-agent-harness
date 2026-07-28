package execsandbox

import "fmt"

// modeRank imposes the strictness order used by the mode-floor:
//
//	ModeOff (0) < ModeBestEffort (1) < ModeStrict (2)
//
// Higher rank = more strict. ApplyFloor clamps the caller's requested mode UP to
// the floor's rank, so a caller can never run BELOW the configured floor. An
// unknown mode ranks as ModeOff (rank 0) — the lowest — which is safe ONLY
// because ParseMinMode rejects unknown floor values at the config boundary, so
// an unknown mode can never reach ApplyFloor as the FLOOR. (An unknown REQUESTED
// mode is rejected earlier by parseSandboxMode in the CLI before ApplyFloor runs.)
func modeRank(m SandboxMode) int {
	switch m {
	case ModeStrict:
		return 2
	case ModeBestEffort:
		return 1
	case ModeOff:
		return 0
	}
	return 0
}

// ApplyFloor returns the effective sandbox mode: the stricter of the caller's
// requested mode and the configured floor. The caller can NEVER run below the
// floor — this is the load-bearing containment property that makes a plain
// `exec-sandbox *` permission grant safe (an agent cannot escape strict by
// passing --sandbox=off, by duplicating the flag, or by interspersing it;
// cobra resolves all --sandbox occurrences to one value before ApplyFloor runs,
// and ApplyFloor then clamps that resolved value up to the floor).
//
// A floor of ModeOff means "no floor" — the caller's requested mode is honored
// exactly (preserves standalone behavior when no floor is configured).
func ApplyFloor(requested, floor SandboxMode) SandboxMode {
	if modeRank(floor) > modeRank(requested) {
		return floor
	}
	return requested
}

// ParseMinMode decodes a config-sourced floor value (from
// exec_sandbox.min_mode in run-shape.yml). It is the fail-closed boundary for
// the floor: an explicit-but-unknown value returns an error rather than
// silently collapsing the floor to off (a typo like "strcit" must not silently
// disable the containment the operator asked for).
//
// Empty ("") — the key-absent case — maps to ModeOff (no floor), preserving
// standalone behavior when the operator has not configured a floor.
func ParseMinMode(s string) (SandboxMode, error) {
	switch s {
	case "", "off":
		return ModeOff, nil
	case "best-effort":
		return ModeBestEffort, nil
	case "strict":
		return ModeStrict, nil
	default:
		return "", fmt.Errorf("invalid exec_sandbox.min_mode=%q (use off|best-effort|strict)", s)
	}
}
