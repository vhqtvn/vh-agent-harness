package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// compactionFixtureDir is the committed fixture source (evaluate.js + its ESM
// package.json sibling). The state-lib.js runtime copy is NOT committed here:
// it is assembled at test time from templates/core (token-substituted), so the
// fixture always measures the CURRENT projection source rather than a frozen
// snapshot.
const compactionFixtureDir = "tests/integration/testdata/compaction-fixture"

// coordinatorToken is the single template token a rendered state-lib.js must
// no longer carry (mirrors the render-guard red control substitution).
const coordinatorToken = "{{COORDINATOR_DIR}}"

// templateTokenRe matches any unsubstituted {{UPPER_SNAKE}} template token in
// a rendered corpus file (state-lib.js's render guard protects only the
// coordinator token; this catches the general shape during fixture assembly).
var templateTokenRe = regexp.MustCompile(`\{\{[A-Z][A-Z_]*\}\}`)

// assembleCompactionFixture builds the hermetic execution tree in a fresh temp
// dir T:
//
//	T/.opencode/package.json                     — ESM execution context
//	T/.opencode/scripts/state-lib.js             — RENDERED (token-substituted)
//	T/.opencode/scripts/{f3-design-readiness,
//	                     rewrite-parity-validate,
//	                     pause-new-work}.js      — sibling imports, verbatim
//	T/.opencode/scripts/evaluate.js              — the fixture script
//
// Because state-lib.js computes repoRoot() as resolve(__dirname, "../.."),
// placing it at T/.opencode/scripts makes repoRoot() == T: every
// repo-relative path the library records (and prints into the projection) is
// stable and production-shaped, and the hermetic env roots below redirect all
// persistence into T.
func assembleCompactionFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	scripts := filepath.Join(root, ".opencode", "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		t.Fatalf("mkdir fixture scripts dir: %v", err)
	}

	// state-lib.js: copy from templates/core and substitute the coordinator
	// token — byte-equivalent to what the render pipeline produces for this
	// file (the token's only occurrences are the guarded path-construction
	// sites; the render-guard IIFE builds its own token at runtime).
	srcPath := filepath.Join(repoRoot, "templates", "core", ".opencode", "scripts", "state-lib.js")
	src, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read templates/core state-lib.js: %v", err)
	}
	rendered := strings.ReplaceAll(string(src), coordinatorToken, "coordinator")
	// Guard against ANY unsubstituted template token (not just the one we
	// substituted), so a future token shape cannot slip through silently.
	for _, m := range templateTokenRe.FindAllString(rendered, -1) {
		t.Fatalf("fixture assembly: unsubstituted template token %q in state-lib.js render", m)
	}
	if err := os.WriteFile(filepath.Join(scripts, "state-lib.js"), []byte(rendered), 0o644); err != nil {
		t.Fatalf("write rendered state-lib.js: %v", err)
	}

	// Sibling imports (token-free today; copied verbatim).
	for _, sibling := range []string{
		"f3-design-readiness.js",
		"rewrite-parity-validate.js",
		"pause-new-work.js",
	} {
		if err := copyFile(
			filepath.Join(repoRoot, "templates", "core", ".opencode", "scripts", sibling),
			filepath.Join(scripts, sibling),
		); err != nil {
			t.Fatalf("copy sibling %s: %v", sibling, err)
		}
	}

	// The fixture script + its ESM package.json (mirrors the corpus layout
	// where .opencode/package.json sits above scripts/).
	fixtureDir := filepath.Join(repoRoot, compactionFixtureDir)
	if err := copyFile(filepath.Join(fixtureDir, "evaluate.js"), filepath.Join(scripts, "evaluate.js")); err != nil {
		t.Fatalf("copy evaluate.js: %v", err)
	}
	if err := copyFile(
		filepath.Join(fixtureDir, "package.json"),
		filepath.Join(root, ".opencode", "package.json"),
	); err != nil {
		t.Fatalf("copy fixture package.json (ESM execution context): %v", err)
	}
	return root
}

// compactionFixtureEnv returns a hermetic environment: the ambient env minus
// any pre-existing opencode root overrides, plus fixture-scoped overrides that
// redirect ALL persistence (state, run artifacts, coordinator transport,
// cleared assumptions) into the temp tree.
func compactionFixtureEnv(root string) []string {
	const stateRootEnv = "OPENCODE_STATE_ROOT"
	const runRootEnv = "OPENCODE_RUN_ROOT"
	const coordRootEnv = "OPENCODE_LOCAL_COORDINATOR_ROOT"
	const clearedEnv = "OPENCODE_CLEARED_ASSUMPTIONS_PATH"
	const cwdEnv = "OPENCODE_CWD"
	filtered := make([]string, 0, len(os.Environ())+5)
	for _, kv := range os.Environ() {
		switch strings.SplitN(kv, "=", 2)[0] {
		case stateRootEnv, runRootEnv, coordRootEnv, clearedEnv, cwdEnv:
			continue
		}
		filtered = append(filtered, kv)
	}
	return append(filtered,
		stateRootEnv+"="+filepath.Join(root, ".opencode", "state"),
		runRootEnv+"="+filepath.Join(root, "tmp", "agent-runs"),
		coordRootEnv+"="+filepath.Join(root, ".local", "coordinator"),
		clearedEnv+"="+filepath.Join(root, ".local", "cleared-assumptions.yaml"),
		cwdEnv+"=/compaction-fixture",
	)
}

// runCompactionFixture runs evaluate.js inside the assembled tree with the
// hermetic env, returning combined output and the exit code.
func runCompactionFixture(t *testing.T, nodeBin, root string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(nodeBin, append([]string{filepath.Join(root, ".opencode", "scripts", "evaluate.js")}, args...)...)
	cmd.Dir = root
	cmd.Env = compactionFixtureEnv(root)
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("failed to invoke node evaluate.js: %v\n%s", err, out)
		}
	}
	return string(out), exit
}

// metricsJsonFrom extracts and parses the single-line metrics-json payload
// from the fixture output so the Go layer can assert on the metrics
// structurally (not just on markers).
func metricsJsonFrom(t *testing.T, out string) []map[string]any {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "metrics-json: ") {
			continue
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "metrics-json: ")), &rows); err != nil {
			t.Fatalf("metrics-json line is not valid JSON: %v", err)
		}
		return rows
	}
	t.Fatalf("no metrics-json line in fixture output:\n%s", out)
	return nil
}

// TestCompactionFixture is the offline compaction-measurement fixture: it
// fabricates 9 case families through the real state-lib.js persistence
// primitives inside a hermetic temp tree and compares Control (cold
// original-payload), B (live buildCompactionContext projection), H (B +
// inline essentials + source-bound recovery with explicit visible failures),
// and C (slug/pointer-only with counted retrieval). The script pins per-case
// behavioral expectations (B's deterministic truncation budgets, silent
// substitution on the overwritten contract, silent degradation on
// missing/truncated state, H's visible-failure accounting) and exits non-zero
// if any fails, so the projection baseline this fixture measures cannot
// silently regress.
//
// Determinism: inputs are frozen constants, timestamps are normalized before
// token counting, and no assertion anchors on a timestamp, so repeated runs
// produce identical metrics and exit codes. No model calls, no network, no
// writes outside the temp tree (templates/core and .opencode/ are untouched —
// B's live behavior is only ever CALLED).
func TestCompactionFixture(t *testing.T) {
	nodeBin := requireNode(t)
	root := assembleCompactionFixture(t)

	out, exit := runCompactionFixture(t, nodeBin, root)
	if exit != 0 {
		t.Fatalf("compaction fixture exited %d (expected 0):\n%s", exit, out)
	}
	for _, marker := range []string{
		"compaction-fixture: ok",
		"cases: 9",
		"strategy-checks: ok",
		"results-md-begin",
		"results-md-end",
	} {
		if !strings.Contains(out, marker) {
			t.Fatalf("fixture output missing marker %q:\n%s", marker, out)
		}
	}

	rows := metricsJsonFrom(t, out)
	if len(rows) != 44 { // 11 reported rows (6a/6b, 8a/8b are family sub-rows) x 4 strategies
		t.Fatalf("expected 44 strategy rows (11 reported rows across 9 case families), got %d:\n%s", len(rows), out)
	}
	// Every reported family must expose all four strategies exactly once.
	byFamily := map[string]map[string]bool{}
	for _, row := range rows {
		family, _ := row["case"].(string)
		strategy, _ := row["strategy"].(string)
		if byFamily[family] == nil {
			byFamily[family] = map[string]bool{}
		}
		if byFamily[family][strategy] {
			t.Fatalf("duplicate strategy row for family %q", family)
		}
		byFamily[family][strategy] = true
	}
	for _, family := range []string{"1", "2", "3", "4", "5", "6a", "6b", "7", "8a", "8b", "9"} {
		for _, strategy := range []string{"Control", "B", "H", "C"} {
			if !byFamily[family][strategy] {
				t.Fatalf("family %q is missing strategy %q:\n%s", family, strategy, out)
			}
		}
	}

	t.Logf("compaction fixture ok (%d strategy rows):\n%s", len(rows), out)
}

// TestCompactionFixture_RedControl proves TestCompactionFixture is not a
// tautology by exercising the fixture's own detection machinery: --red-control
// (1) removes a known anchor from a real B context and requires the anchor
// checker to notice, and (2) doctors metric rows so a pinned expectation is
// false and requires the expectation checker to reject them. The mode exits 0
// only when BOTH injected defects are detected; if the checkers ever pass
// sabotaged inputs, this control (and the fixture) has stopped pinning
// anything.
func TestCompactionFixture_RedControl(t *testing.T) {
	nodeBin := requireNode(t)
	root := assembleCompactionFixture(t)

	out, exit := runCompactionFixture(t, nodeBin, root, "--red-control")
	if exit != 0 {
		t.Fatalf("red control exited %d (expected 0 — defect detection should succeed):\n%s", exit, out)
	}
	if !strings.Contains(out, "red-control: defect detection verified") {
		t.Fatalf("red control output missing verification marker:\n%s", out)
	}
	t.Logf("red control OK: %s", out)
}
