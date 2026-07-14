package cli

// overlay_docs_test.go covers `vh-agent-harness overlay docs <name>` — the
// pack-README discovery command. Coverage targets the three resolution paths:
// an embedded pack with a README (auto-classifier-pilot), an embedded pack
// resolved purely by name (release), and the unknown-name error path.
//
// These tests resolve EMBEDDED packs: this dogfood repo has no project-local
// auto-classifier-pilot or release pack under .vh-agent-harness/overlays/, so
// overlay.OpenPackFor falls through to the embedded FS (project-local absent).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runOverlayDocsIn runs `overlay docs <name>` with the given target and args,
// returning the combined output buffer + the returned error. It resets the
// package-level overlayDocsFl so tests do not leak flag state.
func runOverlayDocsIn(t *testing.T, target string, args ...string) (string, error) {
	t.Helper()
	overlayDocsFl = &overlayDocsFlags{target: target}
	cmd, buf := newOutCmd()
	err := runOverlayDocs(cmd, args)
	return buf.String(), err
}

// TestOverlayDocs_EmbeddedPack confirms `overlay docs auto-classifier-pilot`
// prints the embedded pack README (non-empty, containing a known string from
// the README header).
func TestOverlayDocs_EmbeddedPack(t *testing.T) {
	out, err := runOverlayDocsIn(t, ".", "auto-classifier-pilot")
	if err != nil {
		t.Fatalf("overlay docs auto-classifier-pilot: unexpected error %v (out=%q)", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("overlay docs auto-classifier-pilot: empty output")
	}
	if !strings.Contains(out, "auto-classifier") {
		t.Fatalf("overlay docs auto-classifier-pilot: output missing known README string 'auto-classifier'\nout:\n%s", out)
	}
}

// TestOverlayDocs_ReleasePack confirms `overlay docs release` prints the
// embedded release pack README (non-empty).
func TestOverlayDocs_ReleasePack(t *testing.T) {
	out, err := runOverlayDocsIn(t, ".", "release")
	if err != nil {
		t.Fatalf("overlay docs release: unexpected error %v (out=%q)", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("overlay docs release: empty output")
	}
}

// TestOverlayDocs_UnknownPack confirms an unknown pack name errors cleanly
// (non-zero exit) with the not-found message.
func TestOverlayDocs_UnknownPack(t *testing.T) {
	out, err := runOverlayDocsIn(t, ".", "does-not-exist-xyz")
	if err == nil {
		t.Fatalf("overlay docs nonexistent: expected error, got nil (out=%q)", out)
	}
	if !strings.Contains(out, "not found") {
		t.Fatalf("overlay docs nonexistent: output missing 'not found'\nout:\n%s", out)
	}
}

// TestOverlayDocs_TraversalRejected confirms a path-traversal pack name is
// rejected by name validation BEFORE resolution, so it cannot escape the
// overlays directory and print an arbitrary readable dir's README. The error
// must surface "invalid pack name", and NO file content may reach stdout.
func TestOverlayDocs_TraversalRejected(t *testing.T) {
	out, err := runOverlayDocsIn(t, ".", "../../../etc")
	if err == nil {
		t.Fatalf("overlay docs ../../../etc: expected error, got nil (out=%q)", out)
	}
	if !strings.Contains(err.Error(), "invalid pack name") {
		t.Fatalf("overlay docs ../../../etc: error missing 'invalid pack name'\nerr=%v", err)
	}
	if out != "" {
		t.Fatalf("overlay docs ../../../etc: expected no stdout, got %q", out)
	}
}

// TestOverlayDocs_AbsolutePathRejected confirms an absolute path pack name is
// rejected by name validation (absolute paths fail nameRe and never reach
// resolution).
func TestOverlayDocs_AbsolutePathRejected(t *testing.T) {
	out, err := runOverlayDocsIn(t, ".", "/etc")
	if err == nil {
		t.Fatalf("overlay docs /etc: expected error, got nil (out=%q)", out)
	}
	if !strings.Contains(err.Error(), "invalid pack name") {
		t.Fatalf("overlay docs /etc: error missing 'invalid pack name'\nerr=%v", err)
	}
	if out != "" {
		t.Fatalf("overlay docs /etc: expected no stdout, got %q", out)
	}
}

// TestOverlayDocs_ProjectLocalShadowsEmbedded confirms that a project-local
// pack at <target>/.vh-agent-harness/overlays/<name>/ SHADOWS the embedded pack
// of the same name: `overlay docs` prints the project-local README, not the
// embedded one. This exercises the full runOverlayDocs → OpenPackFor project-
// local resolution path (os.Stat project-local dir → os.DirFS wins over the
// embedded FS) that this test file did not previously cover — all prior tests
// resolve EMBEDDED packs.
func TestOverlayDocs_ProjectLocalShadowsEmbedded(t *testing.T) {
	dir := t.TempDir()
	// OpenPackFor's project-local resolution does
	// os.Stat(target/.vh-agent-harness/overlays/auto-classifier-pilot) → dir
	// exists → os.DirFS(dir) wins over the embedded FS. Seed that dir with a
	// README carrying a unique sentinel.
	packDir := filepath.Join(dir, ".vh-agent-harness", "overlays", "auto-classifier-pilot")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", packDir, err)
	}
	const sentinel = "PROJECT-LOCAL-SENTINEL"
	if err := os.WriteFile(filepath.Join(packDir, "README.md"), []byte(sentinel), 0o644); err != nil {
		t.Fatalf("write project-local README: %v", err)
	}

	out, err := runOverlayDocsIn(t, dir, "auto-classifier-pilot")
	if err != nil {
		t.Fatalf("overlay docs project-local: unexpected error %v (out=%q)", err, out)
	}
	if !strings.Contains(out, sentinel) {
		t.Fatalf("project-local README should contain sentinel %q\nout:\n%s", sentinel, out)
	}
	// The embedded README must NOT have won. "three-hook plugin" is a
	// distinctive phrase from the embedded README (line 3) that the minimal
	// project-local README does not carry — its presence would mean the
	// embedded FS was read instead of the project-local dir.
	const embeddedOnly = "three-hook plugin"
	if strings.Contains(out, embeddedOnly) {
		t.Fatalf("project-local README should shadow embedded; output still contains embedded-only string %q\nout:\n%s", embeddedOnly, out)
	}
}
