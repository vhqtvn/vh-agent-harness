package overlay

// Shadowing guard (Slice 3 of the unified extension model): overlays ADD new
// paths. An overlay MAY NOT silently overwrite/shadow an existing core builtin
// path (or an earlier active pack's unit path) at render time. On collision the
// apply FAILS CLOSED with an *ErrShadowBuiltin that points the consumer to the
// explicit S2 managed→owned replacement path (harness-ownership.yml override
// raising the path to project_owned). The consumer edits the live file directly;
// the harness never auto-shadows from an overlay.
//
// Full D2-C replacement resolution (acting on a proposal to actually perform a
// managed→owned swap in-tree) is held beyond v0. v0 implements the load-bearing
// half: block unsafe shadowing at render time and emit guidance.
//
// See the extension model.

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// ErrShadowBuiltin is the fail-closed error returned when an overlay pack would
// render a unit file at a LIVE path an existing rendered file (a core builtin,
// or an earlier active pack's unit) already occupies. Overlays ADD new paths;
// they MUST NOT silently shadow an existing rendered path.
//
// The error message carries the S2 managed→owned replacement guidance so a
// consumer who genuinely needs to REPLACE a managed file knows the supported
// path: raise the path to project_owned in harness-ownership.yml and edit the
// live file directly (the explicit S2 replace verb of the unified extension
// model), instead of shadowing it from an overlay.
type ErrShadowBuiltin struct {
	// Pack is the overlay pack whose unit collided.
	Pack string
	// Collisions lists the LIVE .opencode-relative paths the pack would shadow,
	// sorted.
	Collisions []string
}

// Error implements the error interface. The message names the colliding paths
// and emits the S2 managed→owned replacement guidance.
func (e *ErrShadowBuiltin) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "overlay %q shadows existing rendered path(s); overlays must ADD new paths, not replace builtins:", e.Pack)
	for _, c := range e.Collisions {
		fmt.Fprintf(&b, "\n  - %s", c)
	}
	b.WriteString("\n\nTo REPLACE a managed file, raise the path to project_owned in ")
	b.WriteString(".vh-agent-harness/harness-ownership.yml (the S2 managed->owned ")
	b.WriteString("replacement path) and edit the live file directly; do not shadow ")
	b.WriteString("it from an overlay. (Full D2-C replacement resolution is held beyond v0.)")
	return b.String()
}

// UnitPaths returns the sorted list of LIVE .opencode-relative paths this pack
// would render as units (the same set RenderUnits writes), WITHOUT writing
// anything. It is the read-only projection of RenderUnits used by the shadow
// guard to detect collisions against the already-rendered staging tree before a
// pack renders. Merge-content/catalog files and extension snippets are excluded
// (they are not units).
func (p *Pack) UnitPaths() ([]string, error) {
	var live []string
	err := fs.WalkDir(p.FS, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !isUnitFile(rel) {
			return nil
		}
		live = append(live, opencodePrefix+rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("overlay %s: list unit paths: %w", p.Name, err)
	}
	sort.Strings(live)
	return live, nil
}

// DetectShadowing returns a non-nil *ErrShadowBuiltin if any of the pack's unit
// paths is already present in existing (the set of paths core / earlier packs
// rendered into staging). Returns (nil, nil) when the pack adds only new paths.
// existing is the LIVE .opencode-relative path set already on disk in staging.
func (p *Pack) DetectShadowing(existing map[string]bool) (*ErrShadowBuiltin, error) {
	paths, err := p.UnitPaths()
	if err != nil {
		return nil, err
	}
	var collisions []string
	for _, lp := range paths {
		if existing[lp] {
			collisions = append(collisions, lp)
		}
	}
	if len(collisions) == 0 {
		return nil, nil
	}
	sort.Strings(collisions)
	return &ErrShadowBuiltin{Pack: p.Name, Collisions: collisions}, nil
}
