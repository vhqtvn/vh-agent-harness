package hooks

import (
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// TestClassifyLeaf_ProjectOwned is the D2 interaction proof (criterion 6): a hook
// leaf under scripts/ classifies project_owned, so IsMutableByGenericRender is FALSE —
// platform updates can NEVER overwrite consumer-authored hooks. Hooks therefore
// cannot become a side door around update-safety (S2) either.
//
// This is the unit-tested analogue (see TODO in ownership.go) until the manifest
// converges on the armed-class vocabulary; the ownership primitive itself
// (IsMutableByGenericRender returns true ONLY for ClassPlatformManaged) is the real
// guard, proven here against the project_owned class hook leaves carry.
func TestClassifyLeaf_ProjectOwned(t *testing.T) {
	for _, leaf := range []string{
		"scripts/migrate-db.sh",
		"scripts/clean-web-cache.sh",
		"scripts/web-smoke.sh",
	} {
		c := ClassifyLeaf(leaf)
		if c != ownership.ClassProjectOwned {
			t.Errorf("ClassifyLeaf(%q) = %s, want project_owned", leaf, c)
		}
		if !c.IsValid() {
			t.Errorf("project_owned must be a valid class; got invalid")
		}
		if ownership.IsMutableByGenericRender(c) {
			t.Errorf("D2 VIOLATION: project_owned hook leaf %q must NOT be platform-mutable (IsMutableByGenericRender returned true)", leaf)
		}
		if LeafIsMutableByGenericRender(leaf) {
			t.Errorf("LeafIsMutableByGenericRender(%q) = true, want false", leaf)
		}
	}
}

// TestIsMutableByGenericRender_OnlyPlatformManaged — the ownership lattice primitive:
// only platform_managed is plain-mutable. Every other on-lattice class —
// including project_owned — is protected. This is the bedrock the D2 guard rests on.
func TestIsMutableByGenericRender_OnlyPlatformManaged(t *testing.T) {
	mutable := []ownership.Class{ownership.ClassPlatformManaged}
	protected := []ownership.Class{
		ownership.ClassPlatformArmed,
		ownership.ClassOverlayExtension,
		ownership.ClassProjectOwned,
		ownership.ClassExternalGenerated,
		ownership.ClassLocalOnly,
	}
	for _, c := range mutable {
		if !ownership.IsMutableByGenericRender(c) {
			t.Errorf("%s should be platform-mutable", c)
		}
	}
	for _, c := range protected {
		if ownership.IsMutableByGenericRender(c) {
			t.Errorf("D2 VIOLATION: %s must NOT be platform-mutable", c)
		}
	}
}
