package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// requireNode skips the test if node is not on PATH (the consumer-render
// verifiers are JS scripts invoked via node).
func requireNode(t *testing.T) string {
	t.Helper()
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not on PATH; skipping JS verifier integration test: %v", err)
	}
	return nodeBin
}

// copyFile copies src to dst (best-effort 0o644). Used to assemble consumer
// trees from the repo's rendered .opencode/ without dragging in template tokens.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// runNodeScript runs `node <script>` with cmd.Dir=dir, returning combined output
// and the process exit code (0 on success, non-zero on exit error). A non-ExitError
// is fatal (node could not be spawned).
func runNodeScript(t *testing.T, nodeBin, dir, script string) (string, int) {
	t.Helper()
	cmd := exec.Command(nodeBin, script)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	exit := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("failed to invoke node %s: %v\n%s", script, err, out)
		}
	}
	return string(out), exit
}

// renderedOpencode joins repo-relative parts under the repo's RENDERED .opencode/
// tree (tokens resolved). A faithful consumer render ships the resolved copies;
// the templates/core/.opencode/ SOURCE copy still carries unrendered tokens and
// is NOT a consumer artifact.
func renderedOpencode(parts ...string) string {
	p := []string{repoRoot, ".opencode"}
	p = append(p, parts...)
	return filepath.Join(p...)
}

// assembleNonGoConsumer builds a synthetic non-Go consumer tree in a fresh temp
// dir OUTSIDE the repo (so the repo's go.mod is not an ancestor) and returns its
// root. The tree mirrors a faithful consumer render:
//
//   - pyproject.toml          — Python consumer identity (deliberately NO go.mod)
//   - .git/                   — directory marker so the polyglot .git-anchored
//     findRepoRoot resolves to the consumer root
//   - .opencode/scripts/      — verify-no-unrendered-paths.js,
//     verify-f3-authoring-surfaces.js, f3-design-readiness.js
//   - .opencode/commands/     — task-ready.md, approve-plan.md, write-task.md,
//     draft-plan.md
//   - .opencode/agents/       — build.md (RESOLVED consumer build-agent surface)
//
// The committed fixture at tests/integration/fixtures/non-go-consumer/ supplies
// the consumer-identity pyproject.toml; everything else is assembled at runtime
// from the rendered .opencode/ so the committed fixture stays minimal and never
// drifts with template content.
func assembleNonGoConsumer(t *testing.T) string {
	t.Helper()
	consumerRoot := t.TempDir() // outside the repo tree => no go.mod ancestor

	// Consumer identity: pyproject.toml (NOT go.mod) + .git marker.
	fixturePyproj := filepath.Join(
		repoRoot, "tests", "integration", "fixtures", "non-go-consumer", "pyproject.toml")
	if err := copyFile(fixturePyproj, filepath.Join(consumerRoot, "pyproject.toml")); err != nil {
		t.Fatalf("copy fixture pyproject.toml: %v", err)
	}
	// .git may be a directory (clone) or a file (submodule/worktree gitdir
	// pointer); the polyglot findRepoRoot counts either via fs.existsSync. A
	// plain empty directory is sufficient and avoids a git dependency.
	if err := os.Mkdir(filepath.Join(consumerRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir consumer .git marker: %v", err)
	}

	// Assemble the consumer .opencode/ render from the repo's rendered copies.
	dstScripts := filepath.Join(consumerRoot, ".opencode", "scripts")
	if err := os.MkdirAll(dstScripts, 0o755); err != nil {
		t.Fatalf("mkdir consumer scripts dir: %v", err)
	}
	for _, s := range []string{
		"verify-no-unrendered-paths.js",
		"verify-f3-authoring-surfaces.js",
		"f3-design-readiness.js",
	} {
		if err := copyFile(renderedOpencode("scripts", s), filepath.Join(dstScripts, s)); err != nil {
			t.Fatalf("copy rendered script %s: %v", s, err)
		}
	}

	dstCmds := filepath.Join(consumerRoot, ".opencode", "commands")
	if err := os.MkdirAll(dstCmds, 0o755); err != nil {
		t.Fatalf("mkdir consumer commands dir: %v", err)
	}
	for _, c := range []string{
		"task-ready.md",
		"approve-plan.md",
		"write-task.md",
		"draft-plan.md",
	} {
		if err := copyFile(renderedOpencode("commands", c), filepath.Join(dstCmds, c)); err != nil {
			t.Fatalf("copy rendered command %s: %v", c, err)
		}
	}

	dstAgents := filepath.Join(consumerRoot, ".opencode", "agents")
	if err := os.MkdirAll(dstAgents, 0o755); err != nil {
		t.Fatalf("mkdir consumer agents dir: %v", err)
	}
	if err := copyFile(renderedOpencode("agents", "build.md"), filepath.Join(dstAgents, "build.md")); err != nil {
		t.Fatalf("copy rendered agents/build.md: %v", err)
	}

	return consumerRoot
}

// TestConsumerRenderVerifierFixture is the persistent regression-pin for the
// b69fb1f polyglot findRepoRoot repair. It runs BOTH consumer-render verifiers
// (verify-no-unrendered-paths.js + verify-f3-authoring-surfaces.js) against a
// synthetic non-Go consumer tree and asserts each exits 0.
//
// Pre-b69fb1f the findRepoRoot walk anchored on go.mod (a Go-SPECIFIC manifest);
// a non-Go consumer (Python/Node) with no go.mod in the ancestor chain returned
// null and verify-no-unrendered-paths.js exited 1 — the unrendered-paths guard
// leaked the very source-checkout-only class it was built to catch. b69fb1f
// re-anchored on .git (every consumer render is a git repo, regardless of
// language). This test catches reversion to the go.mod-only heuristic.
//
// See P1-TESTS-001 in docs/planning/backlog.md.
func TestConsumerRenderVerifierFixture(t *testing.T) {
	nodeBin := requireNode(t)
	consumerRoot := assembleNonGoConsumer(t)

	for _, script := range []string{
		"verify-no-unrendered-paths.js",
		"verify-f3-authoring-surfaces.js",
	} {
		script := script
		t.Run(script, func(t *testing.T) {
			scriptPath := filepath.Join(consumerRoot, ".opencode", "scripts", script)
			out, exit := runNodeScript(t, nodeBin, consumerRoot, scriptPath)
			if exit != 0 {
				t.Fatalf("%s exited %d in the non-Go consumer tree (expected 0 — "+
					"polyglot findRepoRoot regression?):\n%s", script, exit, out)
			}
			t.Logf("%s -> exit 0\n%s", script, out)
		})
	}
}

// preB69fb1fFindRepoRoot is the go.mod-only findRepoRoot body that b69fb1f
// replaced. Used by the negative control to prove TestConsumerRenderVerifierFixture
// actually catches a reversion (and is not a tautology).
const preB69fb1fFindRepoRoot = `function findRepoRoot(start) {
    // PRE-b69fb1f go.mod-only heuristic (restored by the negative control).
    let dir = start;
    for (;;) {
        if (fs.existsSync(path.join(dir, "go.mod"))) {
            return dir;
        }
        const parent = path.dirname(dir);
        if (parent === dir) {
            return null; // filesystem root reached, no go.mod found.
        }
        dir = parent;
    }
}`

// findRepoRootRe matches the polyglot findRepoRoot function in
// verify-no-unrendered-paths.js (from its `function findRepoRoot(start) {`
// header to the first column-1 closing brace) so the negative control can swap
// in the pre-b69fb1f go.mod-only body. The match is anchored on column-1 braces
// so the nested (indented) blocks inside the function do not terminate it early.
var findRepoRootRe = regexp.MustCompile(`(?ms)^function findRepoRoot\(start\) \{.*?^\}`)

// TestConsumerRenderVerifierFixture_NegativeControl proves the regression-pin
// value: it restores the PRE-b69fb1f go.mod-only findRepoRoot on the consumer
// tree's copy of verify-no-unrendered-paths.js, runs it, and asserts it FAILS
// (non-zero exit) — exactly the acute defect b69fb1f repaired (findRepoRoot
// returns null in a non-Go consumer → the guard exits 1). If this negative
// control ever passes, TestConsumerRenderVerifierFixture has stopped pinning the
// polyglot repair and become a tautology.
func TestConsumerRenderVerifierFixture_NegativeControl(t *testing.T) {
	nodeBin := requireNode(t)
	consumerRoot := assembleNonGoConsumer(t)

	// The negative control requires the consumer tree to have NO go.mod in any
	// ancestor; otherwise the go.mod-only walk would resolve a root instead of
	// returning null and the test would not demonstrate the regression. Skip
	// (not fail) if the environment cannot demonstrate it.
	for d := consumerRoot; d != string(filepath.Separator); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			t.Skipf("consumer root %s has a go.mod ancestor (%s); cannot demonstrate "+
				"the go.mod-only regression in this environment", consumerRoot, d)
		}
	}

	verifierPath := filepath.Join(consumerRoot, ".opencode", "scripts", "verify-no-unrendered-paths.js")
	src, err := os.ReadFile(verifierPath)
	if err != nil {
		t.Fatalf("read rendered verifier for negative-control patch: %v", err)
	}
	if !findRepoRootRe.MatchString(string(src)) {
		t.Fatalf("negative control: could not locate findRepoRoot in the rendered " +
			"verifier; the script shape has changed and findRepoRootRe needs updating")
	}
	patched := findRepoRootRe.ReplaceAllString(string(src), preB69fb1fFindRepoRoot)
	if err := os.WriteFile(verifierPath, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched (go.mod-only) verifier back into consumer tree: %v", err)
	}

	out, exit := runNodeScript(t, nodeBin, consumerRoot, verifierPath)
	if exit == 0 {
		t.Fatalf("negative control: pre-b69fb1f go.mod-only findRepoRoot should have "+
			"EXITED non-zero in the non-Go consumer tree (regression-pin value), but got exit 0:\n%s", out)
	}
	t.Logf("negative control OK: pre-b69fb1f go.mod-only verifier exited %d as expected "+
		"(demonstrates TestConsumerRenderVerifierFixture catches the reversion):\n%s", exit, out)
}
