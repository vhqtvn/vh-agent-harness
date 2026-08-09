package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
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

// TestPrintDryRunPlan_ListsOverwritePaths is the regression lock for the
// surface-at-friction fix (researches/decisions/2026-08-04-capability-discovery-
// audit.md §6 entry 1). The destructive managed-overwrite category MUST list
// each path it will revert — not collapse to a count. Incident 2026-08-06: an
// approved "11 generic managed file(s)" preview reverted a 199-line skill +
// a version bump because no path was visible. This test pins path-by-path
// listing + the non-destructive overlay-promotion remedy.
func TestPrintDryRunPlan_ListsOverwritePaths(t *testing.T) {
	overwritePaths := []string{
		".opencode/skills/vh-solara/SKILL.md",
		".opencode/.version",
	}
	report := &substrate.ApplyReport{
		Outcomes: []substrate.FileOutcome{
			{Path: overwritePaths[0], Class: ownership.ClassPlatformManaged, Action: substrate.ActionManagedOverwrite},
			{Path: overwritePaths[1], Class: ownership.ClassPlatformManaged, Action: substrate.ActionManagedOverwrite},
			// A noop must NOT appear as an overwrite (it routes to its own line).
			{Path: ".opencode/agents/build.md", Class: ownership.ClassPlatformManaged, Action: substrate.ActionManagedNoop},
			// A seed must render under its own section (parallel structure preserved).
			{Path: "Makefile", Class: ownership.ClassProjectOwned, Action: substrate.ActionProjectSeeded},
		},
	}
	var buf bytes.Buffer
	printDryRunPlan(&buf, "update", "<target>", report)
	out := buf.String()

	// (1) Every overwrite path must be visible, path-by-path (the load-bearing fix).
	for _, p := range overwritePaths {
		if !strings.Contains(out, "  "+p+"\n") {
			t.Errorf("OVERWRITE preview must list path %q path-by-path; got:\n%s", p, out)
		}
	}
	// (2) The destructive category header must flag DESTRUCTIVE.
	if !strings.Contains(out, "Would OVERWRITE") || !strings.Contains(out, "DESTRUCTIVE") {
		t.Errorf("OVERWRITE header must flag DESTRUCTIVE; got:\n%s", out)
	}
	// (3) The non-destructive remedy for the overwrite set must appear inline.
	// With origin-hash, authored consumer edits are PRESERVED (not in this set);
	// the overwrite set is unedited/bit-rot files plus regenerated files
	// (allowed-commands.js), so the remedy is forbidden-patterns.project.js +
	// diff. (The overlay-promotion remedy for preserved authored edits lives in
	// the DIVERGED section — see TestPrintDryRunPlan_ListsDivergedPaths.)
	if !strings.Contains(out, "forbidden-patterns.project.js") || !strings.Contains(out, "vh-agent-harness diff") {
		t.Errorf("OVERWRITE section must offer the non-destructive remedy; got:\n%s", out)
	}
	// (4) Regression: the OLD count-only form must be gone.
	if strings.Contains(out, "generic managed file(s), force-refreshed") {
		t.Errorf("OLD count-only OVERWRITE form must be gone; got:\n%s", out)
	}
	// (5) The noop + seed sections still render (parallel structure preserved).
	if !strings.Contains(out, "Already current") {
		t.Errorf("noop section must still render; got:\n%s", out)
	}
	if !strings.Contains(out, "Would SEED") {
		t.Errorf("SEED section must still render; got:\n%s", out)
	}
}

// TestPrintDryRunPlan_ListsDivergedPaths is the dry-run regression lock for the
// origin-hash update sync: consumer-edited AUTHORED managed files route to
// ActionManagedDiverged and MUST appear path-by-path under their own PRESERVE
// section (not vanish, and not be folded into the destructive OVERWRITE set),
// with the re-baseline + overlay-promotion remedy inline.
func TestPrintDryRunPlan_ListsDivergedPaths(t *testing.T) {
	divergedPaths := []string{
		".vh-agent-harness/AGENTS.core.md",
		".opencode/agents/build.md",
	}
	report := &substrate.ApplyReport{
		Outcomes: []substrate.FileOutcome{
			{Path: divergedPaths[0], Class: ownership.ClassPlatformManaged, Action: substrate.ActionManagedDiverged},
			{Path: divergedPaths[1], Class: ownership.ClassPlatformManaged, Action: substrate.ActionManagedDiverged},
		},
	}
	var buf bytes.Buffer
	printDryRunPlan(&buf, "update", "<target>", report)
	out := buf.String()

	// (1) Every diverged (consumer-edited managed) path must be visible.
	for _, p := range divergedPaths {
		if !strings.Contains(out, "  "+p+"\n") {
			t.Errorf("DIVERGED preview must list path %q path-by-path; got:\n%s", p, out)
		}
	}
	// (2) The PRESERVE (consumer-edited managed) header must flag that these are
	// preserved, not overwritten.
	if !strings.Contains(out, "PRESERVE (consumer-edited managed)") || !strings.Contains(out, "origin-hash") {
		t.Errorf("DIVERGED header must flag origin-hash preservation; got:\n%s", out)
	}
	// (3) The non-destructive remedy (re-baseline via accept-platform +
	// overlay promotion) appears. The re-baseline MUST point at the
	// `accept-platform` operation, NOT at hand-editing the sidecar and NOT at
	// deleting the file (deleting the file is self-defeating: a deleted managed
	// file is also preserved as consumer-deleted, not re-seeded).
	if !strings.Contains(out, "overlay pack source") || !strings.Contains(out, ".vh-agent-harness/overlays/<pack>/") {
		t.Errorf("DIVERGED section must offer the overlay-promotion remedy; got:\n%s", out)
	}
	if !strings.Contains(out, "accept-platform") {
		t.Errorf("DIVERGED re-baseline remedy must point at `accept-platform` (the sanctioned recovery op); got:\n%s", out)
	}
	// (4) A diverged file must NOT appear under the destructive OVERWRITE header.
	if strings.Contains(out, "Would OVERWRITE") {
		t.Errorf("DIVERGED paths must not appear under the destructive OVERWRITE set; got:\n%s", out)
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

// TestGuide_InstalledSurfacesAvailableUnselectedPacks confirms the installed-
// phase guide carries an UNGATED discovery hint listing shipped (embedded) packs
// that are available but NOT selected. This is the incident 2026-08-06 fix: a
// coordinator or operator running `guide` learns a shipped pack EXISTS before
// authorizing a rebuild-from-scratch on the false conclusion it does not. The
// hint is hermetic — a clean temp dir selects nothing, so release and
// auto-classifier-pilot surface as available.
func TestGuide_InstalledSurfacesAvailableUnselectedPacks(t *testing.T) {
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
	st := detectHarnessState(dir)
	if st.Phase != phaseInstalled {
		t.Fatalf("phase = %q, want installed", st.Phase)
	}
	joined := strings.Join(nextSteps(st), " ")
	if !strings.Contains(joined, "Shipped packs available but not selected") {
		t.Errorf("installed guide should carry the ungated available-unselected hint; steps:\n%s", joined)
	}
	// In a clean repo, release + auto-classifier-pilot + repo-mail are unselected
	// embedded packs (the three pilots are default-on). The hint must name at
	// least release (a capability-bearing pack) and auto-classifier-pilot (the
	// incident pack) so a coordinator sees they exist.
	for _, pack := range []string{"release", "auto-classifier-pilot"} {
		if !strings.Contains(joined, pack) {
			t.Errorf("available-unselected hint should name %q; steps:\n%s", pack, joined)
		}
	}
	// The hint must point at the discovery + enable commands.
	if !strings.Contains(joined, "overlay list") {
		t.Errorf("hint should point at `overlay list`; steps:\n%s", joined)
	}
}
