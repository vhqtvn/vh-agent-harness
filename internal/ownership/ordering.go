package ownership

import "fmt"

// protectionRank is the total raise-only ordering over the four platform-vs-
// project armed classes (low rank = less protection from platform writes). It
// is the lattice D2-A reasons over. external_generated and local_only are
// intentionally ABSENT: they are off-lattice (see package doc).
var protectionRank = map[Class]int{
	ClassPlatformManaged:  0,
	ClassPlatformArmed:    1,
	ClassOverlayExtension: 2,
	ClassProjectOwned:     3,
}

// IsHandOverridable reports whether a class sits on the raise/lower lattice and
// may therefore be the FROM or TO of a project override. external_generated and
// local_only are OFF the lattice; their class is determined by the platform /
// provider / local convention, never by a hand override.
func IsHandOverridable(c Class) bool {
	_, ok := protectionRank[c]
	return ok
}

// rank returns the protection rank of an on-lattice class. The bool result is
// false when c is off-lattice (external_generated / local_only) or invalid.
func rank(c Class) (int, bool) {
	r, ok := protectionRank[c]
	return r, ok
}

// Decision is the outcome of comparing two on-lattice classes on the lattice.
type Decision int

const (
	// DecisionEqual means from == to (no protection change). Accepted as a
	// no-op override (project-wins trivially; the protection level is unchanged).
	DecisionEqual Decision = iota
	// DecisionRaise means to is strictly higher protection than from. Accepted
	// (project-wins-on-raise).
	DecisionRaise
	// DecisionLower means to is strictly lower protection than from. Rejected
	// as a downgrade (D2-A).
	DecisionLower
)

// String returns the lower-case decision label.
func (d Decision) String() string {
	switch d {
	case DecisionEqual:
		return "equal"
	case DecisionRaise:
		return "raise"
	case DecisionLower:
		return "lower"
	default:
		return fmt.Sprintf("decision(%d)", int(d))
	}
}

// Compare classifies the from -> to transition on the lattice. It returns
// DecisionRaise / DecisionEqual / DecisionLower when both classes are on-lattice.
// It returns an error (without a Decision) when either class is off-lattice
// (external_generated / local_only) or invalid; callers should surface those as
// NotHandOverridableError / InvalidClassError respectively rather than inventing
// a rank.
//
// Compare is the pure lattice predicate; it does NOT know about paths or the
// override map. The path-aware decision lives in resolve.go.
func Compare(from, to Class) (Decision, error) {
	if !from.IsValid() {
		return 0, &InvalidClassError{Class: string(from)}
	}
	if !to.IsValid() {
		return 0, &InvalidClassError{Class: string(to)}
	}
	rFrom, okFrom := rank(from)
	rTo, okTo := rank(to)
	if !okFrom || !okTo {
		return 0, &NotHandOverridableError{
			Path:  "", // Compare is path-agnostic; the resolver fills the path
			Class: offLatticeClass(from, to),
			Reason: "off-lattice classes (external_generated, local_only) are " +
				"not hand-overridable; their class is platform/provider-determined",
		}
	}
	switch {
	case rTo > rFrom:
		return DecisionRaise, nil
	case rTo == rFrom:
		return DecisionEqual, nil
	default:
		return DecisionLower, nil
	}
}

// offLatticeClass returns whichever of from/to is off-lattice (preferring from).
// Used to populate NotHandOverridableError.Class when Compare rejects an
// off-lattice pair. Both are guaranteed valid (Callers validated) but at least
// one is off-lattice.
func offLatticeClass(from, to Class) Class {
	if !IsHandOverridable(from) {
		return from
	}
	return to
}

// IsMutableByGenericRender reports whether the GENERIC (overlay-unaware) platform
// render/overwrite may touch a path of the given class. Only platform_managed is
// plain-mutable: it is the single class a plain re-render force-overwrites. Every
// other class is either protected, gated, merged-only, provider-owned, or off
// the managed path:
//
//   - platform_managed   : true  (generic force-overwrite on update)
//   - platform_armed     : false (mutable only via the armed/gated reconcile path)
//   - overlay_extension  : false (overwritten by the OVERLAY SYSTEM, not the
//     generic render — see IsOverwritableBySeamApply; Slice 4 made active overlay
//     units overwrite-wholesale, but their authority is the overlay pack, not a
//     plain render)
//   - project_owned      : false (NEVER touched by platform update)
//   - external_generated : false (provider/project-owned; seeded once, then drift-checked only)
//   - local_only         : false (not on the platform update path)
//
// This predicate is the guard behind hooks.LeafIsMutableByGenericRender
// (internal/hooks/ownership.go): it proves consumer-authored hook leaves classify
// project_owned and so are never overwritten by an ungated platform update. It is
// NOT called from the seam apply path (internal/substrate/apply.go): that path
// routes platform_managed to its managed-overwrite outcome via a hand-rolled
// per-class switch in planOutcome (see IsOverwritableBySeamApply for the
// wholesale-overwrite class set that switch honors). The legacy manifest model
// (internal/manifest) converges its vocabulary onto the armed lattice via
// IsRenderable, which honors the same six classes. This function is unit-tested
// to prove project_owned / external_generated / local_only are never
// plain-mutable.
func IsMutableByGenericRender(c Class) bool {
	return c == ClassPlatformManaged
}

// IsOverwritableBySeamApply reports whether a class is in the wholesale-overwrite
// set for a seam apply (install/update): the set of classes the seam apply path
// routes to its managed-* outcomes. Two classes qualify:
//
//   - platform_managed : the generic force-overwrite class (the single class
//     IsMutableByGenericRender is true for). A plain re-render overwrites it on every
//     update.
//   - overlay_extension: overwritten by the OVERLAY SYSTEM when its pack is
//     active (Slice 4). When a pack is deselected the unit is simply not staged,
//     so the live copy is left untouched; the authority for the overwrite is the
//     overlay pack selection, not a generic render.
//
// Every other class is preserved (project_owned / external_generated when
// present), seeded once (project_owned / external_generated when absent),
// schema-reconciled or turned into a proposal (platform_armed), or off the
// update path entirely (local_only). The seam apply switch
// (internal/substrate/apply.go planOutcome) routes only these two classes to the
// managed-* outcomes — ActionManagedOverwrite, or ActionManagedNoop
// (managed-unchanged) when the live instance is already byte-identical to the
// freshly rendered corpus. Reaching the managed-overwrite route for any other
// class is a platform bug and fails closed.
//
// Authority: the LIVE overwrite decision is that per-class switch in planOutcome
// (internal/substrate/apply.go), which is hand-rolled and does NOT call this
// predicate — the switch is the authority; this predicate documents the same
// class-set it routes. TestIsOverwritableBySeamApply pins the predicate's side of
// that agreement (a change to the predicate without reconciling the switch fails
// the test); switch-side drift is covered by the substrate apply tests plus
// review. (Wiring the predicate as the switch's single source of truth is a
// tracked follow-up, not the current state.)
func IsOverwritableBySeamApply(c Class) bool {
	return c == ClassPlatformManaged || c == ClassOverlayExtension
}
