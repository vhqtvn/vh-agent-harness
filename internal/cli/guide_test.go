package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectHarnessState_Greenfield(t *testing.T) {
	st := detectHarnessState(t.TempDir())
	if st.Phase != phaseGreenfield {
		t.Fatalf("phase = %q, want greenfield", st.Phase)
	}
	if !strings.Contains(strings.Join(nextSteps(st), " "), "install") {
		t.Error("greenfield steps should mention install")
	}
}

func TestDetectHarnessState_Adoptable(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".opencode"), 0o755); err != nil {
		t.Fatal(err)
	}
	st := detectHarnessState(dir)
	if st.Phase != phaseAdoptable {
		t.Fatalf("phase = %q, want adoptable (existing .opencode, no lineage)", st.Phase)
	}
	joined := strings.Join(nextSteps(st), " ")
	if !strings.Contains(joined, "Adopt") || !strings.Contains(joined, "preserved") {
		t.Error("adoptable steps should explain adopt + preservation")
	}
}

func TestDetectHarnessState_InstalledNeedsMission(t *testing.T) {
	dir := t.TempDir()
	vh := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(vh, 0o755); err != nil {
		t.Fatal(err)
	}
	// A run-shape makes FindForRoot resolve the install root.
	if err := os.WriteFile(filepath.Join(vh, "run-shape.yml"),
		[]byte("run_shape_version: \"0.1\"\nruntime:\n  backend: host-shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := detectHarnessState(dir)
	if st.Phase != phaseInstalled {
		t.Fatalf("phase = %q, want installed", st.Phase)
	}
	if st.RuntimeBackend != "host-shell" {
		t.Errorf("backend = %q, want host-shell", st.RuntimeBackend)
	}
	if st.HasMission {
		t.Error("HasMission should be false without AGENTS.mission.md")
	}
	joined := strings.Join(nextSteps(st), " ")
	if !strings.Contains(joined, "AGENTS.mission.md") {
		t.Error("installed-without-mission steps should prompt to write AGENTS.mission.md")
	}
	if !strings.Contains(joined, "doctor") {
		t.Error("installed steps should mention doctor for verification")
	}

	// Once a mission source exists, the mission step drops out.
	if err := os.WriteFile(filepath.Join(vh, "AGENTS.mission.md"), []byte("# Mission\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st2 := detectHarnessState(dir)
	if !st2.HasMission {
		t.Error("HasMission should be true after writing AGENTS.mission.md")
	}
}

// TestWriteGuide_InstalledSummary exercises the Slice-3 progressive-disclosure
// additions to the installed-phase guide: the "You have: agents N · skills M ·
// commands K" summary (rendered from live os.ReadDir counts, defaulting to 0
// when the .opencode/{agents,skills,commands} dirs are absent via os.ErrNotExist),
// the "First: write ..." single recommended task when the mission is absent, and
// the footer disambiguations (binary-vs-slash note + overlay-new pointer). It
// also guards Slice 1's restart hint against regression.
func TestWriteGuide_InstalledSummary(t *testing.T) {
	dir := t.TempDir()
	vh := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(vh, 0o755); err != nil {
		t.Fatal(err)
	}
	// A run-shape makes FindForRoot resolve the install root (installed phase).
	if err := os.WriteFile(filepath.Join(vh, "run-shape.yml"),
		[]byte("run_shape_version: \"0.1\"\nruntime:\n  backend: host-shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Deliberately create NO .opencode/{agents,skills,commands} dirs and NO
	// AGENTS.mission.md: this exercises the os.ErrNotExist → 0 count path AND
	// the mission-absent "First: write ..." next step.
	st := detectHarnessState(dir)
	if st.Phase != phaseInstalled {
		t.Fatalf("phase = %q, want installed", st.Phase)
	}
	if st.HasMission {
		t.Fatal("HasMission should be false without AGENTS.mission.md")
	}
	steps := nextSteps(st)
	var buf bytes.Buffer
	writeGuide(&buf, st, steps)
	out := buf.String()

	// The "You have:" summary must render in the installed phase.
	if !strings.Contains(out, "You have: agents") {
		t.Errorf("installed guide should print 'You have: agents' summary; got:\n%s", out)
	}
	// Missing .opencode/{agents,skills,commands} dirs must default to 0 (the
	// os.ErrNotExist → 0 path), not error or panic.
	if !strings.Contains(out, "You have: agents 0 · skills 0 · commands 0") {
		t.Errorf("summary should show all-zero counts when dirs absent; got:\n%s", out)
	}
	// The single recommended first task must appear when mission is absent.
	if !strings.Contains(out, "First: write .vh-agent-harness/AGENTS.mission.md") {
		t.Errorf("installed guide without mission should print 'First: write ...' next step; got:\n%s", out)
	}
	// Slice 3 footer additions.
	if !strings.Contains(out, "vh-agent-harness overlay new") {
		t.Errorf("guide footer should mention 'overlay new' pointer; got:\n%s", out)
	}
	if !strings.Contains(out, "binary subcommand (runs anywhere)") {
		t.Errorf("guide footer should include binary-vs-slash disambiguation; got:\n%s", out)
	}
	// Regression guard: Slice 1's restart hint must still render.
	if !strings.Contains(out, "Restart opencode") {
		t.Errorf("Slice 1 restart hint should still render in footer; got:\n%s", out)
	}
}

// TestWriteGuide_InstalledWithMission verifies the "First: write ..." next step
// drops out once the mission exists, while the "You have:" summary still renders.
func TestWriteGuide_InstalledWithMission(t *testing.T) {
	dir := t.TempDir()
	vh := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(vh, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vh, "run-shape.yml"),
		[]byte("run_shape_version: \"0.1\"\nruntime:\n  backend: host-shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vh, "AGENTS.mission.md"), []byte("# Mission\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := detectHarnessState(dir)
	if !st.HasMission {
		t.Fatal("HasMission should be true after writing AGENTS.mission.md")
	}
	var buf bytes.Buffer
	writeGuide(&buf, st, nextSteps(st))
	out := buf.String()
	if !strings.Contains(out, "You have: agents") {
		t.Errorf("summary should still render when mission present; got:\n%s", out)
	}
	if strings.Contains(out, "First: write") {
		t.Errorf("'First: write ...' should NOT render when mission present; got:\n%s", out)
	}
}

// TestCountDirEntries_ErrNotExist confirms a missing dir counts as 0 (the
// documented os.ErrNotExist path) and a populated dir counts its entries.
func TestCountDirEntries_ErrNotExist(t *testing.T) {
	if got := countDirEntries(filepath.Join(t.TempDir(), "does-not-exist")); got != 0 {
		t.Errorf("missing dir count = %d, want 0", got)
	}
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := countDirEntries(dir); got != 3 {
		t.Errorf("populated dir count = %d, want 3", got)
	}
}
