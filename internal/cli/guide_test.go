package cli

import (
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
