package ownership

import (
	"errors"
	"fmt"
	"sort"
)

// PathRule is one platform-module default entry: the default armed class for a
// path plus its provenance (which module/overlay/provider declared it). The
// platform module rules are INPUT 1 of the S2 effective ownership map.
type PathRule struct {
	// Class is the default armed ownership class the platform module assigns.
	Class Class
	// Provenance names the module/overlay that declared this default (e.g.
	// "core.research_workflow", "<project>.<subsystem>"). "" is allowed but
	// discouraged for real modules; it is carried onto the effective entry for
	// auditability.
	Provenance string
}

// ModuleDefaults is the platform-authored default ownership map: path -> default
// PathRule. It is INPUT 1 of the effective-map computation.
type ModuleDefaults map[string]PathRule

// Override is one project-authored amendment (INPUT 2 of the effective map). The
// consumer-repo harness-ownership.yml overrides block deserializes into
// Overrides.
type Override struct {
	// Class is the target armed class the project wants in effect.
	Class Class
	// Reason is an optional human note the project may record. It is carried on
	// the effective entry for auditability; it does NOT affect the raise/lower
	// decision.
	Reason string
}

// Overrides is the project-authored harness-ownership.yml override map:
// path -> Override.
type Overrides map[string]Override

// Origin records how an EffectiveEntry's class was determined.
type Origin string

const (
	// OriginDefault means no override applied; the platform default is in effect.
	OriginDefault Origin = "default"
	// OriginOverrideRaise means an override RAISED protection and was accepted
	// (project-wins-on-raise).
	OriginOverrideRaise Origin = "override-raise"
	// OriginOverrideNoop means an override set the SAME class as the default
	// (accepted as a benign no-op; protection unchanged).
	OriginOverrideNoop Origin = "override-noop"
)

// EffectiveEntry is the resolved update-safety classification for one path: the
// effective armed class, its provenance, and how it was derived.
type EffectiveEntry struct {
	// Class is the effective armed ownership class (default, or the accepted
	// override class).
	Class Class
	// Provenance is the module/overlay that owns the path (from the platform
	// default). When an override raised the class, the provenance still records
	// the originating module so the update path knows what produced the file.
	Provenance string
	// Origin records how the class was derived (default / override-raise /
	// override-noop).
	Origin Origin
}

// EffectiveMap is the resolved S2 ownership map: path -> EffectiveEntry.
type EffectiveMap map[string]EffectiveEntry

// Resolve computes the effective ownership map from platform module defaults
// (INPUT 1) and project overrides (INPUT 2), applying the D2-A raise-only rule.
//
// Semantics:
//
//   - Every path in defaults starts at its platform-default class (OriginDefault).
//   - Each override is applied IN SORTED PATH ORDER (deterministic). An override
//     is ACCEPTED only when it RAISES protection (or is a same-class no-op); it
//     is REJECTED when it downgrades, targets an unknown path, uses an invalid
//     class, or touches an off-lattice class.
//   - project-wins precedence holds ONLY for accepted (raise/no-op) overrides.
//     A downgrade override is rejected even though project-wins in general.
//   - The first INVALID class literal encountered among the platform defaults
//     aborts immediately with that InvalidClassError (corrupted upstream input
//     is not partially honored).
//
// When one or more overrides are rejected, Resolve returns the partial effective
// map (defaults applied; accepted overrides applied) AND a non-nil error joining
// every violation, so an operator sees the full picture in one pass. Each
// underlying typed error is reachable via errors.As even when joined.
//
// Resolve never silently invents a class, never silently disarms a path, and
// never partially applies a downgrade. It is the mechanical D2-A guarantee.
func Resolve(defaults ModuleDefaults, overrides Overrides) (EffectiveMap, error) {
	effective := make(EffectiveMap, len(defaults))

	// 1. Seed from platform defaults. Validate each class literal up front: an
	//    invalid default is corrupted upstream input — abort rather than
	//    partially honoring it.
	for path, rule := range defaults {
		if !rule.Class.IsValid() {
			return effective, &InvalidClassError{Class: string(rule.Class)}
		}
		effective[path] = EffectiveEntry{
			Class:      rule.Class,
			Provenance: rule.Provenance,
			Origin:     OriginDefault,
		}
	}

	if len(overrides) == 0 {
		return effective, nil
	}

	// 2. Apply overrides in deterministic (sorted) path order.
	paths := make([]string, 0, len(overrides))
	for p := range overrides {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var violations []error
	for _, path := range paths {
		ov := overrides[path]
		entry, applyErr := decideOverride(path, defaults, ov)
		if applyErr != nil {
			violations = append(violations, applyErr)
			continue
		}
		effective[path] = entry
	}

	if len(violations) > 0 {
		// errors.Join produces a multi-error whose wrapped errors are reachable
		// via errors.As (Go traverses Unwrap() []error). So IsDowngradeError /
		// IsUnknownPathError / IsNotHandOverridableError all still match.
		return effective, errors.Join(violations...)
	}
	return effective, nil
}

// decideOverride computes the EffectiveEntry for a single override, or returns a
// typed error explaining why the override is rejected. It is the single
// chokepoint for the D2-A decision:
//
//   - UnknownPathError      : override path has no platform default.
//   - InvalidClassError     : override class literal is not one of the six.
//   - NotHandOverridableError: from/to class is off-lattice
//     (external_generated / local_only).
//   - DowngradeError        : override lowers protection on the lattice.
//
// Accepted outcomes: OriginOverrideRaise (to > from) or OriginOverrideNoop
// (to == from). The provenance is inherited from the platform default.
func decideOverride(path string, defaults ModuleDefaults, ov Override) (EffectiveEntry, error) {
	rule, known := defaults[path]
	if !known {
		return EffectiveEntry{}, &UnknownPathError{Path: path}
	}
	// Validate the override class literal before any lattice reasoning.
	if !ov.Class.IsValid() {
		return EffectiveEntry{}, &InvalidClassError{Class: string(ov.Class)}
	}

	decision, err := Compare(rule.Class, ov.Class)
	if err != nil {
		// Compare returns a path-agnostic NotHandOverridableError (or an
		// InvalidClassError, already handled above). Attach the path so the
		// message names the offending override.
		var nhe *NotHandOverridableError
		if errors.As(err, &nhe) {
			nhe.Path = path
			return EffectiveEntry{}, nhe
		}
		return EffectiveEntry{}, err
	}

	switch decision {
	case DecisionLower:
		return EffectiveEntry{}, &DowngradeError{
			Path:   path,
			From:   rule.Class,
			To:     ov.Class,
			Reason: "protection lowered on the raise/lower lattice",
		}
	case DecisionRaise:
		return EffectiveEntry{
			Class:      ov.Class,
			Provenance: rule.Provenance,
			Origin:     OriginOverrideRaise,
		}, nil
	case DecisionEqual:
		return EffectiveEntry{
			Class:      ov.Class,
			Provenance: rule.Provenance,
			Origin:     OriginOverrideNoop,
		}, nil
	default:
		// Unreachable: Compare returns one of the three decisions on-lattice.
		return EffectiveEntry{}, fmt.Errorf(
			"ownership: indeterminate decision %d for path %q (%s -> %s); failing closed",
			int(decision), path, rule.Class, ov.Class,
		)
	}
}

// ClassOf is a small lookup helper returning the effective class for a path, and
// whether the path is present in the effective map. It keeps callers (and tests)
// from indexing the map inline with a second ok check.
func (m EffectiveMap) ClassOf(path string) (Class, bool) {
	e, ok := m[path]
	if !ok {
		return "", false
	}
	return e.Class, true
}
