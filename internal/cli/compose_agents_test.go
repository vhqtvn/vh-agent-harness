package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// srcDir returns the .vh-agent-harness/ source dir under a temp project, created.
func srcDir(t *testing.T, dir string) string {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

// TestComposeAgentsMd_OptInPreservesHandAuthored confirms the compose is opt-in:
// with no .vh-agent-harness/AGENTS.mission.md source, a project's hand-authored
// root AGENTS.md is left alone.
func TestComposeAgentsMd_OptInPreservesHandAuthored(t *testing.T) {
	dir := t.TempDir()
	hand := "# my hand-authored AGENTS\nDOMAIN\n"
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), hand)
	mustWrite(t, filepath.Join(srcDir(t, dir), "AGENTS.core.md"), "# Core Rules\n")

	if err := composeAgentsMd(dir); err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(got) != hand {
		t.Errorf("no-mission: AGENTS.md was modified:\n%s", got)
	}
}

// TestComposeAgentsMd_Composes confirms that with a mission source present,
// AGENTS.md = AGENTS.core.md + blank line + AGENTS.mission.md (sources read from
// .vh-agent-harness/, output written to the root).
func TestComposeAgentsMd_Composes(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), "# Core Rules\nGENERIC\n")
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")

	if err := composeAgentsMd(dir); err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	s := string(got)
	if !strings.HasPrefix(s, "# Core Rules\nGENERIC") {
		t.Errorf("composed AGENTS.md should start with core:\n%s", s)
	}
	if !strings.Contains(s, "DOMAIN-MARKER") {
		t.Errorf("composed AGENTS.md should contain the mission:\n%s", s)
	}
	if i, j := strings.Index(s, "GENERIC"), strings.Index(s, "DOMAIN-MARKER"); i < 0 || j < 0 || i > j {
		t.Errorf("core must precede mission (core@%d, mission@%d)", i, j)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- feature-gated composition (flag-gated-composition-fix.md) ---------------
//
// The AGENTS.core.md source carries Go text/template conditionals
// ({{ if .features.backlog }}...{{ end }}) that the renderer's preserve-as-is
// copy leaves literal in the rendered file. composeAgentsMd must evaluate them
// at compose time so features.backlog:false suppresses the backlog section in
// the composed AGENTS.md. These tests are the load-bearing crux for that fix:
// they prove a render with the flag off drops the gated section, while the
// flag on (or the corpus default) keeps it.

const gatedCoreBody = "# Core Rules\nGENERIC\n{{ if .features.backlog }}\n## Backlog tracking rules\nBACKLOG-MARKER\n{{ end }}\n"

// TestComposeAgentsMd_BacklogGateTrue proves the gated section is PRESENT when
// features.backlog resolves true (explicit project override).
func TestComposeAgentsMd_BacklogGateTrue(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), gatedCoreBody)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	writeProfile(t, dir, "profile: minimal\nfeatures:\n  backlog: true\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if !strings.Contains(got, "BACKLOG-MARKER") {
		t.Errorf("backlog=true must include the gated section:\n%s", got)
	}
	if !strings.Contains(got, "## Backlog tracking rules") {
		t.Errorf("backlog=true must include the section heading:\n%s", got)
	}
	// The template action markers must NOT leak into the composed output.
	if strings.Contains(got, "{{ ") || strings.Contains(got, "{{if") {
		t.Errorf("composed AGENTS.md leaked a template action marker:\n%s", got)
	}
}

// TestComposeAgentsMd_BacklogGateFalse proves the gated section is ABSENT when
// features.backlog resolves false. This is the bug the fix targets: before the
// fix the section shipped regardless of the flag.
func TestComposeAgentsMd_BacklogGateFalse(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), gatedCoreBody)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	writeProfile(t, dir, "profile: minimal\nfeatures:\n  backlog: false\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if strings.Contains(got, "BACKLOG-MARKER") {
		t.Errorf("backlog=false must EXCLUDE the gated section:\n%s", got)
	}
	if strings.Contains(got, "## Backlog tracking rules") {
		t.Errorf("backlog=false must EXCLUDE the section heading:\n%s", got)
	}
	// The non-gated generic core content must survive (bundle-safe-disable:
	// suppress only the feature delta, never shared content).
	if !strings.Contains(got, "GENERIC") {
		t.Errorf("backlog=false must still keep the generic core content:\n%s", got)
	}
	if !strings.Contains(got, "DOMAIN-MARKER") {
		t.Errorf("backlog=false must still keep the mission content:\n%s", got)
	}
}

// TestComposeAgentsMd_BacklogGateDefault proves the corpus default
// (features.backlog=true, the shipped platform default) keeps the section when
// the project has not written a profile — backward-compatible with every
// existing consumer that never touched the flag.
func TestComposeAgentsMd_BacklogGateDefault(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), gatedCoreBody)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	// No profile written -> reconciledFeatures returns the corpus default.

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if !strings.Contains(got, "BACKLOG-MARKER") {
		t.Errorf("corpus default (backlog=true) must include the gated section:\n%s", got)
	}
}

// composeAndRead runs composeAgentsMd and returns the composed AGENTS.md body.
func composeAndRead(t *testing.T, dir string) (string, error) {
	t.Helper()
	if err := composeAgentsMd(dir); err != nil {
		return "", err
	}
	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read composed AGENTS.md: %v", err)
	}
	return string(got), nil
}
