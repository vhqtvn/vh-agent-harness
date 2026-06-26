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
