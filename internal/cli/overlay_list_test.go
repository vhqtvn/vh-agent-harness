package cli

// overlay_list_test.go covers `vh-agent-harness overlay list` — the discovery
// command that enumerates every pack (embedded + project-local) with its source
// + selected status. Coverage targets the core incident fix: a
// shipped-but-unselected pack MUST surface as "available", never as absent.
//
// The hermetic tests use a clean temp dir (no profile) so the assertions rest
// only on the embedded corpus, not on this dogfood repo's profile state.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runOverlayListIn runs `overlay list` with the given target and args,
// returning the combined output buffer + the returned error. It resets the
// package-level overlayListFl so tests do not leak flag state.
func runOverlayListIn(t *testing.T, target string, args ...string) (string, error) {
	t.Helper()
	overlayListFl = &overlayListFlags{target: target}
	cmd, buf := newOutCmd()
	err := runOverlayList(cmd, args)
	return buf.String(), err
}

// packLine returns the first output line mentioning name, or "" if none. Used
// to assert a pack's status appears on the SAME line as its name (grep-friendly).
func packLine(t *testing.T, out, name string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, name) {
			return line
		}
	}
	t.Fatalf("output has no line mentioning %q\n--- output ---\n%s", name, out)
	return ""
}

// TestOverlayList_ShowsEveryShippedPack confirms `overlay list` projects the
// FULL shipped set (all six embedded packs), not just already-selected ones.
// This is the core enumeration fix: no shipped pack is invisible.
func TestOverlayList_ShowsEveryShippedPack(t *testing.T) {
	dir := t.TempDir() // clean repo: no profile, no project-local packs
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	for _, name := range []string{
		"auto-classifier-pilot",
		"contract-invariant-audit-pilot",
		"formal-verification-pilot",
		"release",
		"repo-mail",
		"resolve-first-pilot",
	} {
		if !strings.Contains(out, name) {
			t.Errorf("overlay list output missing shipped pack %q\n--- output ---\n%s", name, out)
		}
	}
}

// TestOverlayList_AutoClassifierAvailableInCleanRepo is the incident crux: in a
// repo that does NOT select auto-classifier-pilot, `overlay list` visibly shows
// it as AVAILABLE — never absent. (The 2026-08-06 incident had a coordinator
// conclude auto-classifier-pilot "does not exist" and authorize a rebuild while
// it ships embedded.) This test is hermetic: a clean temp dir selects nothing.
func TestOverlayList_AutoClassifierAvailableInCleanRepo(t *testing.T) {
	dir := t.TempDir()
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	line := packLine(t, out, "auto-classifier-pilot")
	if !strings.Contains(line, "available") {
		t.Errorf("auto-classifier-pilot should be 'available' in a clean repo; line=%q", line)
	}
	if strings.Contains(line, "selected") {
		t.Errorf("auto-classifier-pilot should NOT be 'selected' in a clean repo; line=%q", line)
	}
	// Source attribution: it ships embedded.
	if !strings.Contains(line, "embedded") {
		t.Errorf("auto-classifier-pilot line should show source 'embedded'; line=%q", line)
	}
}

// TestOverlayList_ReleaseAvailableInCleanRepo confirms the release pack (the
// dual-selectable capability pack) shows as available when neither selected via
// overlays nor via capabilities:[core/release].
func TestOverlayList_ReleaseAvailableInCleanRepo(t *testing.T) {
	dir := t.TempDir()
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	line := packLine(t, out, "release")
	if !strings.Contains(line, "available") {
		t.Errorf("release should be 'available' in a clean repo; line=%q", line)
	}
}

// TestOverlayList_DefaultOnPilotsSelected confirms the three default-on
// skills-only pilots surface as 'selected' (they render via platform-default
// feature keys) even when not listed under overlays:. This prevents the inverse
// false impression — a rendering pack reported as merely "available".
func TestOverlayList_DefaultOnPilotsSelected(t *testing.T) {
	dir := t.TempDir()
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	for _, pilot := range []string{
		"formal-verification-pilot",
		"resolve-first-pilot",
		"contract-invariant-audit-pilot",
	} {
		line := packLine(t, out, pilot)
		if !strings.Contains(line, "selected") {
			t.Errorf("default-on pilot %s should be 'selected' (feature-default-on); line=%q", pilot, line)
		}
	}
}

// TestOverlayList_ProjectLocalPackSource confirms a project-local pack at
// <target>/.vh-agent-harness/overlays/<name>/ is enumerated with source
// "project-local" (the project-wins attribution KnownPacksFor shares with
// OpenPackFor).
func TestOverlayList_ProjectLocalPackSource(t *testing.T) {
	dir := t.TempDir()
	packDir := filepath.Join(dir, ".vh-agent-harness", "overlays", "acme-custom")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", packDir, err)
	}
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	line := packLine(t, out, "acme-custom")
	if !strings.Contains(line, "project-local") {
		t.Errorf("acme-custom (project-local pack) should show source 'project-local'; line=%q", line)
	}
}

// TestOverlayList_GrepFriendlyHeader confirms the table carries a header line
// and the footer points at the discovery + enable commands, so an agent can
// parse the output and know the next step.
func TestOverlayList_GrepFriendlyHeader(t *testing.T) {
	dir := t.TempDir()
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	for _, want := range []string{
		"PACK", // header
		"SOURCE",
		"STATUS",
		"vh-agent-harness overlay docs <name>",
		"overlays:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay list output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestOverlay_ReleaseDualSelectionViaCapabilities confirms the release pack is
// marked 'selected' when core/release appears under `capabilities:` (the
// dual-selection path; both overlays:[release] and capabilities:[core/release]
// converge). This pins the capability-manifest dual-selection logic.
func TestOverlay_ReleaseDualSelectionViaCapabilities(t *testing.T) {
	dir := t.TempDir()
	vh := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(vh, 0o755); err != nil {
		t.Fatal(err)
	}
	// A schema-valid profile that selects core/release via capabilities and
	// opts OUT of all default-on pilots so only release's selection is under
	// test (no pilot feature noise).
	profile := "profile: minimal\n" +
		"features:\n" +
		"  backlog: true\n" +
		"  formal-verification-pilot: false\n" +
		"  resolve-first-pilot: false\n" +
		"  contract-invariant-audit-pilot: false\n" +
		"overlays: []\n" +
		"capabilities:\n" +
		"  - core/release\n"
	if err := os.WriteFile(filepath.Join(vh, "vh-harness-profile.yml"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runOverlayListIn(t, dir)
	if err != nil {
		t.Fatalf("overlay list: unexpected error %v (out=%q)", err, out)
	}
	line := packLine(t, out, "release")
	if !strings.Contains(line, "selected") {
		t.Errorf("release should be 'selected' via capabilities:[core/release]; line=%q", line)
	}
}

// --- overlay parent command: unknown-verb non-zero exit -------------------

// TestOverlay_NoArgsPrintsHelp confirms bare `vh-agent-harness overlay` (no
// subcommand) prints the parent help and exits 0 — the documented behavior the
// new parent RunE preserves.
func TestOverlay_NoArgsPrintsHelp(t *testing.T) {
	out, err := executeCapture(t, []string{"overlay"})
	if err != nil {
		t.Fatalf("bare `overlay`: want nil error (exit 0, print help), got %v", err)
	}
	for _, want := range []string{"list", "docs", "new"} {
		if !strings.Contains(out, want) {
			t.Errorf("bare `overlay` help should list the %q subcommand; got:\n%s", want, out)
		}
	}
}

// TestOverlay_UnknownVerbIsAnError confirms a genuinely-unknown overlay verb
// (e.g. `overlay frob`) exits non-zero instead of silently printing parent help
// and looking like success. This is the silent-false-negative path surface 3
// closes: previously `overlay <typo>` printed help and exited 0.
func TestOverlay_UnknownVerbIsAnError(t *testing.T) {
	_, err := executeCapture(t, []string{"overlay", "frob"})
	if err == nil {
		t.Fatal("unknown overlay verb `overlay frob`: want non-nil error (non-zero exit), got nil")
	}
}
