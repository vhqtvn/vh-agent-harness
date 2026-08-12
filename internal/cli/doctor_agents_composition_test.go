package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckAgentsComposition_SkipWhenNoMissionSource proves the opt-in contract:
// when .vh-agent-harness/AGENTS.mission.md is ABSENT, the check SKIPs — a legacy
// consumer that never adopted the core/mission split has a legitimately
// hand-authored root AGENTS.md (project_owned), and doctor must never flag it.
// This mirrors composeAgentsMd's no-op-on-no-mission rule.
func TestCheckAgentsComposition_SkipWhenNoMissionSource(t *testing.T) {
	dir := t.TempDir()
	// A hand-authored root AGENTS.md the legacy consumer owns.
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# my hand-authored AGENTS\nDOMAIN\n")
	// Core source present, but NO mission source → opt-out.
	mustWrite(t, filepath.Join(srcDir(t, dir), "AGENTS.core.md"), "# Core Rules\nGENERIC\n")

	r := checkAgentsComposition(dir)
	if r.tier != tierSkip {
		t.Fatalf("no mission source: want SKIP, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "mission") {
		t.Errorf("SKIP detail should explain the opt-in mission-absent reason; got %q", r.detail)
	}
}

// TestCheckAgentsComposition_PassWhenRootMatchesComposed proves the happy path:
// when a project HAS adopted the core/mission split AND the root AGENTS.md equals
// the composed (AGENTS.core.md + AGENTS.mission.md) body the seam would write,
// the check PASSES — the unified file is in sync with its sources.
func TestCheckAgentsComposition_PassWhenRootMatchesComposed(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), "# Core Rules\nGENERIC\n")
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")

	// Let the seam compose the canonical root AGENTS.md (the source of truth for
	// what "composed" means), then the check must agree it is in sync.
	if err := composeAgentsMd(dir); err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}

	r := checkAgentsComposition(dir)
	if r.tier != tierPass {
		t.Fatalf("root matches composed: want PASS, got %s: %s", r.tier, r.detail)
	}
	if !strings.Contains(r.detail, "matches") {
		t.Errorf("PASS detail should state the match; got %q", r.detail)
	}
}

// TestCheckAgentsComposition_WarnWhenRootDrifted is the load-bearing F2-narrow
// assertion: when the mission source IS present but the root AGENTS.md DIFFERS
// from the composed body (stale or hand-edited unified file), the check WARNs —
// NEVER FAILs. WARN-only by construction: this is read-only observability over a
// project_owned file, so it carries zero disruption risk and never makes the repo
// UNHEALTHY. The detail points at `vh-agent-harness update` to re-compose.
func TestCheckAgentsComposition_WarnWhenRootDrifted(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), "# Core Rules\nGENERIC\n")
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")

	// Compose the canonical body, then MUTATE the root so it is stale.
	if err := composeAgentsMd(dir); err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# stale hand-edit\nNOT THE COMPOSED BODY\n")

	r := checkAgentsComposition(dir)
	// THE GATE ASSERTION: WARN, NEVER FAIL.
	if r.tier != tierWarn {
		t.Fatalf("stale root: want WARN (NEVER FAIL), got %s: %s", r.tier, r.detail)
	}
	if r.tier == tierFail {
		t.Fatalf("GATE VIOLATION: agents-composition must NEVER FAIL a repo (read-only observability over a project_owned file); got FAIL: %s", r.detail)
	}
	if !strings.Contains(r.detail, "differs") {
		t.Errorf("WARN detail should state the difference; got %q", r.detail)
	}
	if !strings.Contains(r.detail, "vh-agent-harness update") {
		t.Errorf("WARN detail should point at `vh-agent-harness update` to re-compose; got %q", r.detail)
	}
}

// TestCheckAgentsComposition_WarnWhenRootAbsent proves the missing-root branch:
// mission source present but no root AGENTS.md at all → WARN (the unified file
// should exist; update composes it). Still NEVER FAIL.
func TestCheckAgentsComposition_WarnWhenRootAbsent(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), "# Core Rules\nGENERIC\n")
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	// Deliberately do NOT create root AGENTS.md.

	// Ensure it really is absent (no stray file from a prior helper).
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Fatalf("precondition: root AGENTS.md should be absent")
	}

	r := checkAgentsComposition(dir)
	if r.tier != tierWarn {
		t.Fatalf("mission present + root absent: want WARN, got %s: %s", r.tier, r.detail)
	}
	if r.tier == tierFail {
		t.Fatalf("GATE VIOLATION: missing root must NEVER FAIL; got FAIL: %s", r.detail)
	}
	if !strings.Contains(r.detail, "absent") {
		t.Errorf("WARN detail should state the root is absent; got %q", r.detail)
	}
}

// TestCheckAgentsComposition_NoCoreSourceSkips proves the second opt-out branch:
// a mission source present but NO core source → the composition is a no-op
// (composeAgentsMdBytes returns not-present), so the check SKIPs rather than
// false-flag a half-configured tree.
func TestCheckAgentsComposition_NoCoreSourceSkips(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	// Mission present but core absent.
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), "# whatever\n")

	r := checkAgentsComposition(dir)
	if r.tier != tierSkip {
		t.Fatalf("no core source: want SKIP, got %s: %s", r.tier, r.detail)
	}
}
