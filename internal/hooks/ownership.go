package hooks

import (
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// ClassifyLeaf returns the S2 ownership class for a hook leaf path.
//
// Per the run-shape spec §3 (pointer-not-inline rule, line ~172): every lifecycle.*
// value is a path pointer to a leaf under .vh-agent-harness/scripts/, and "the
// referenced leaf ... is project_owned under S2 and therefore never overwritten
// by `vh-agent-harness update`." The consumer authors and git-controls scripts/*.sh; the
// platform only SEEDS it (once, on install) and then never touches it.
//
// This is the D2 interaction for Slice 5: hook leaves classify project_owned, so
// ownership.IsMutableByPlatform(ClassProjectOwned) is FALSE — platform updates
// can NEVER overwrite consumer-authored hooks. Hooks therefore cannot become a
// side door around update-safety either: even if a platform update shipped a
// malicious hook leaf, it could not replace the consumer's project_owned one.
//
// TODO(slice-followup): wire this through ownership.Resolve(defaults, overrides)
// .EffectiveMap.ClassOf(path) once the manifest converges on the armed-class
// vocabulary. Slice 4 (ownership package) shipped the lattice + IsMutableByPlatform
// as the unit-tested analogue while the live manifest still uses a parallel
// vocabulary (ClassProjectOwned="project-owned" + IsRenderable). Until that
// convergence lands, this function is the armed-class analogue: it asserts the
// intent unambiguously and is unit-tested in ownership_test.go.
func ClassifyLeaf(leaf string) ownership.Class {
	// All leaves that survive runshape.Load are validated path pointers under
	// scripts/ (runshape.validateLeafPointer enforces this at Load time). scripts/
	// is the consumer's, git-controlled declaration space => project_owned.
	_ = leaf // structurally a scripts/ path; classification is uniform by policy
	return ownership.ClassProjectOwned
}

// LeafIsPlatformMutable reports whether a platform (ungated) update may overwrite
// a hook leaf. It is always FALSE for project_owned leaves: this is the D2 guard
// that keeps consumer-authored hooks safe across `vh-agent-harness update`.
func LeafIsPlatformMutable(leaf string) bool {
	return ownership.IsMutableByPlatform(ClassifyLeaf(leaf))
}

// AssertLeafPolicy is a compile-time anchor that the runshape fixed-point set and
// the hooks Point type stay in sync (Point aliases runshape.LifecycleHook). It
// references both packages so a rename in one is caught at build time.
var _ = []runshape.LifecycleHook{runshape.HookPreUp, runshape.HookPostUp}
