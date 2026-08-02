package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renderGuardRefusal is the exact stderr substring the assertRenderedNotSource
// IIFE in state-lib.js throws when it detects an unrendered source copy.
const renderGuardRefusal = "REFUSING to load state-lib.js from an UNRENDERED templates/core/ source copy"

// sourceCopyFixtureDir is the committed frozen source-copy snapshot of
// state-lib.js + its f3-design-readiness.js import target. See the fixture
// README for why this is a committed copy rather than a runtime copy of the
// live source.
const sourceCopyFixtureDir = "tests/integration/fixtures/state-lib-source-copy"

// TestStateLibRenderGuard is the persistent crux-#1 regression pin for the
// state-lib.js render-location guard (IIFE assertRenderedNotSource, shipped in
// commit 186ba269). It runs a SOURCE copy of state-lib.js — whose bytes still
// contain the literal coordinator-directory template token {{COORDINATOR_DIR}}
// (resolved only at render time) — and asserts node refuses to load it:
// non-zero exit + stderr containing renderGuardRefusal.
//
// Manual demonstrations in 186ba269 do not survive a future edit; this test
// pins crux #1 (source-copy refuses to load). Crux #2/#3 (rendered
// verify-f3-authoring-surfaces.js passes / verify-no-unrendered-paths.js FAILs
// on a recreated marker) are already persistently CI-covered by the verify-script
// runs themselves, so they are deliberately NOT duplicated here.
//
// See P1-TESTS-002 in docs/planning/backlog.md.
func TestStateLibRenderGuard(t *testing.T) {
	nodeBin := requireNode(t)

	srcCopy := filepath.Join(repoRoot, sourceCopyFixtureDir, "state-lib.js")
	if _, err := os.Stat(srcCopy); err != nil {
		t.Fatalf("source-copy fixture missing at %s: %v", srcCopy, err)
	}

	out, exit := runNodeScript(t, nodeBin, filepath.Dir(srcCopy), srcCopy)
	if exit == 0 {
		t.Fatalf("source-copy state-lib.js loaded (exit 0) — render-location guard "+
			"did NOT fire:\n%s", out)
	}
	if !strings.Contains(out, renderGuardRefusal) {
		t.Fatalf("render-location guard fired with an unexpected message (want stderr "+
			"containing %q); exit=%d:\n%s", renderGuardRefusal, exit, out)
	}
	t.Logf("render-location guard fired as expected (exit=%d):\n%s", exit, out)
}

// TestStateLibRenderGuard_RedControl proves TestStateLibRenderGuard pins crux #1
// and is not a tautology: against a RENDERED copy (the {{COORDINATOR_DIR}} token
// substituted away), state-lib.js loads PAST the guard — the guard does not
// fire. If the red control ever refuses a rendered copy, the guard has become
// over-broad (it would block every legitimate consumer render, since a render
// resolves the token).
//
// The token is substituted by simple string replacement, mirroring what the
// render pipeline does to the one literal occurrence (the path-construction
// line `path.join(repoRoot(), ".local", "{{COORDINATOR_DIR}}")`). The guard
// itself builds its token-search delimiter at runtime via char codes precisely
// so the renderer never resolves the guard's own condition; only the literal
// path-line token needs substituting to make this copy "rendered".
func TestStateLibRenderGuard_RedControl(t *testing.T) {
	nodeBin := requireNode(t)

	fixtureDir := filepath.Join(repoRoot, sourceCopyFixtureDir)
	srcBytes, err := os.ReadFile(filepath.Join(fixtureDir, "state-lib.js"))
	if err != nil {
		t.Fatalf("read source-copy fixture: %v", err)
	}

	// Produce a RENDERED copy: substitute the coordinator-directory token away.
	rendered := strings.ReplaceAll(string(srcBytes), "{{COORDINATOR_DIR}}", "coordinator")
	if strings.Contains(rendered, "{{COORDINATOR_DIR}}") {
		t.Fatalf("red-control setup: token substitution incomplete; the fixture has " +
			"multiple token shapes and the substitution needs widening")
	}

	// Write the rendered copy + the sibling f3-design-readiness.js import target
	// into a fresh temp dir.
	renderedDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(renderedDir, "state-lib.js"), []byte(rendered), 0o644); err != nil {
		t.Fatalf("write rendered copy: %v", err)
	}
	if err := copyFile(
		filepath.Join(fixtureDir, "f3-design-readiness.js"),
		filepath.Join(renderedDir, "f3-design-readiness.js"),
	); err != nil {
		t.Fatalf("copy f3-design-readiness.js sibling: %v", err)
	}

	out, exit := runNodeScript(t, nodeBin, renderedDir, filepath.Join(renderedDir, "state-lib.js"))
	if exit != 0 {
		t.Fatalf("rendered (token-substituted) state-lib.js should load PAST the guard "+
			"(exit 0) but exited %d — guard is over-broad (would block consumer renders):\n%s",
			exit, out)
	}
	if strings.Contains(out, renderGuardRefusal) {
		t.Fatalf("rendered copy triggered the refusal — guard misfired on a token-free "+
			"(rendered) copy:\n%s", out)
	}
	t.Logf("red control OK: rendered (token-substituted) copy loaded past the guard "+
		"(exit=%d):\n%s", exit, out)
}
