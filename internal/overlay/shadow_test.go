package overlay

// Unit tests for the shadowing guard (Slice 3 of the unified extension model).
// Covers:
//   - UnitPaths: read-only projection of the LIVE paths a pack would render.
//   - DetectShadowing: collision with an existing rendered path fails-closed
//     (returns *ErrShadowBuiltin) with S2 managed→owned replacement guidance;
//     all-new paths return nil.

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
)

// TestUnitPaths_ExcludesSnippetsAndMergeContent confirms UnitPaths lists only
// renderable unit paths (snippets + merge-content excluded), opencode-prefixed
// and sorted.
func TestUnitPaths_ExcludesSnippetsAndMergeContent(t *testing.T) {
	pack := &Pack{
		Name: "web-overlay",
		FS: fstest.MapFS{
			"agents/web-builder.md":               {Data: []byte("unit")},
			"agents/build.extend.custom-verbs.md": {Data: []byte("snippet")},
			"opencode-append.jsonc":               {Data: []byte("{}")},
			"callable-graph-snippet.md":           {Data: []byte("graph")},
			"permission-pack.jsonc":               {Data: []byte("{}")},
			"commands/frontend.md":                {Data: []byte("cmd")},
		},
	}
	got, err := pack.UnitPaths()
	if err != nil {
		t.Fatalf("UnitPaths: %v", err)
	}
	want := []string{
		".opencode/agents/web-builder.md",
		".opencode/commands/frontend.md",
	}
	if len(got) != len(want) {
		t.Fatalf("UnitPaths: want %v, got %v", want, got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("UnitPaths[%d]: want %q, got %q", i, w, got[i])
		}
	}
}

// TestDetectShadowing_CollisionFailsClosed confirms a pack whose unit would
// land at an existing rendered path returns *ErrShadowBuiltin naming the
// colliding path, with the S2 managed→owned replacement guidance.
func TestDetectShadowing_CollisionFailsClosed(t *testing.T) {
	pack := &Pack{
		Name: "hostile-overlay",
		FS: fstest.MapFS{
			// Tries to shadow the core builtin agents/build.md.
			"agents/build.md":       {Data: []byte("replacement body")},
			"agents/extra-agent.md": {Data: []byte("new unit")},
		},
	}
	existing := map[string]bool{
		".opencode/agents/build.md": true, // core builtin already rendered
	}
	shadow, err := pack.DetectShadowing(existing)
	if err != nil {
		t.Fatalf("DetectShadowing: %v", err)
	}
	if shadow == nil {
		t.Fatalf("DetectShadowing: want *ErrShadowBuiltin for collision, got nil")
	}
	if shadow.Pack != "hostile-overlay" {
		t.Errorf("shadow.Pack = %q, want hostile-overlay", shadow.Pack)
	}
	if len(shadow.Collisions) != 1 || shadow.Collisions[0] != ".opencode/agents/build.md" {
		t.Errorf("Collisions = %v, want [.opencode/agents/build.md]", shadow.Collisions)
	}
	// The error message MUST carry S2 replacement guidance so the consumer
	// knows the supported escape-hatch (raise to project_owned), not the
	// blocked shadow path.
	msg := shadow.Error()
	for _, want := range []string{"project_owned", "harness-ownership.yml", "shadow"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q:\n%s", want, msg)
		}
	}
	// The error must be the *ErrShadowBuiltin itself (typed), so the seam can
	// type-assert for fail-closed handling.
	var target *ErrShadowBuiltin
	if !errors.As(shadow, &target) {
		t.Errorf("shadow must satisfy errors.As(*ErrShadowBuiltin): %T", shadow)
	}
}

// TestDetectShadowing_AllNewPathsReturnsNil confirms a pack that only ADDS new
// paths (no collision) returns nil — overlays are free to add.
func TestDetectShadowing_AllNewPathsReturnsNil(t *testing.T) {
	pack := &Pack{
		Name: "safe-overlay",
		FS: fstest.MapFS{
			"agents/web-builder.md": {Data: []byte("new unit")},
			"commands/web-smoke.md": {Data: []byte("new cmd")},
		},
	}
	existing := map[string]bool{
		".opencode/agents/build.md": true, // core builtin, NOT touched by this pack
	}
	shadow, err := pack.DetectShadowing(existing)
	if err != nil {
		t.Fatalf("DetectShadowing: %v", err)
	}
	if shadow != nil {
		t.Fatalf("DetectShadowing: want nil for all-new pack, got %v", shadow)
	}
}

// TestDetectShadowing_EmptyPackReturnsNil confirms an empty pack (no units) does
// not produce a shadow error.
func TestDetectShadowing_EmptyPackReturnsNil(t *testing.T) {
	pack := &Pack{
		Name: "empty-pack",
		FS:   fstest.MapFS{},
	}
	shadow, err := pack.DetectShadowing(map[string]bool{".opencode/agents/build.md": true})
	if err != nil {
		t.Fatalf("DetectShadowing: %v", err)
	}
	if shadow != nil {
		t.Fatalf("DetectShadowing: want nil for empty pack, got %v", shadow)
	}
}
