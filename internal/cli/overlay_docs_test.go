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
