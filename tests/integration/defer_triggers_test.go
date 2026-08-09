package integration

// Integration tests for `vh-agent-harness defer-triggers` — the no-arg
// contained wrapper that runs the DEFER-trigger predicate checker
// (.opencode/scripts/check-defer-triggers.mjs) under ModeStrict + NetDeny +
// DefaultProfile. Shares TestMain (binary build) + sandboxBin + repoRoot with
// exec_sandbox_test.go.
//
// CRUX (behavioral-closure for O5_TMP / DEFER card
// defer-checker-runnability-readonly-role): a read-only role runs the checker
// via this wrapper under the strict sandbox and REAL GIT OUTPUT is observed
// (the diff-since line carries real changed-paths, NOT the "git unavailable"
// degradation). The Phase-0 probe (tmp/o5-phase0) proved the file-backed-FD
// mechanism in isolation; this test observes the OUTCOME through the real
// wrapper binary on the real kernel.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runDeferTriggers invokes `vh-agent-harness defer-triggers` with cmd.Dir set
// to dir and the given extra args (normally none). Returns combined output and
// the process exit code.
func runDeferTriggers(t *testing.T, dir string, extra ...string) (string, int) {
	t.Helper()
	args := append([]string{"defer-triggers"}, extra...)
	cmd := exec.Command(sandboxBin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to invoke defer-triggers: %v\n%s", err, out)
		}
	}
	return string(out), exitCode
}

// setupDeferScratchRepo builds a hermetic scratch git repo with a prior tag and
// a post-tag change, copies the RENDERED checker into it, and creates an empty
// tasks dir so the checker prints its full report (including the diff-since
// line that carries the crux signal). Returns the repo dir.
func setupDeferScratchRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v in scratch repo: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	git("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "fileA.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	git("tag", "v0.1.0")
	if err := os.WriteFile(filepath.Join(dir, "fileA.go"), []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatalf("write fileA v2: %v", err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "post-tag change")

	// Copy the RENDERED checker (the runtime source of truth) into the scratch
	// repo so the wrapper's checkerPath resolution (<repoRoot>/.opencode/scripts/
	// check-defer-triggers.mjs) finds it. The rendered checker carries the real
	// COORDINATOR_DIR ("coordinator"), so defaultTasksDir resolves under
	// <dir>/.local/coordinator/tasks.
	rendered := filepath.Join(repoRoot, ".opencode", "scripts", "check-defer-triggers.mjs")
	src, err := os.ReadFile(rendered)
	if err != nil {
		t.Fatalf("read rendered checker: %v", err)
	}
	scriptsDir := filepath.Join(dir, ".opencode", "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "check-defer-triggers.mjs"), src, 0o644); err != nil {
		t.Fatalf("copy checker: %v", err)
	}
	// Empty tasks dir so the checker prints its full report (the early "no tasks
	// dir" return would skip the diff-since line that carries the crux signal).
	tasksDir := filepath.Join(dir, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	return dir
}

// TestDeferTriggers_HappyPath_RealGitOutput is the O5_TMP CRUX, observed
// end-to-end on the real binary + real kernel: a scratch git repo runs the
// checker via the wrapper under the strict sandbox, and REAL GIT OUTPUT is
// observed — the diff-since line carries real changed paths, NOT the
// "git unavailable" degradation that the pre-rewrite pipe-based capture
// produced under NetDeny (libuv's socketpair(AF_UNIX) was blocked, so every
// git op failed and the checker degraded to "predicates degrade to not-met").
//
// The file-backed stdout FD (the rewrite) avoids the socketpair entirely, so
// git runs under the strict sandbox and its stdout is captured from the file.
// This test proves that OUTCOME through the real wrapper.
func TestDeferTriggers_HappyPath_RealGitOutput(t *testing.T) {
	sandboxFeatureCheck(t)
	dir := setupDeferScratchRepo(t)

	out, exit := runDeferTriggers(t, dir)

	if exit != 0 {
		t.Fatalf("defer-triggers should exit 0 in promoter mode (never blocking); got exit=%d:\n%s", exit, out)
	}
	// CRUX assertion 1: the checker ran (its report header is present).
	if !strings.Contains(out, "check-defer-triggers report") {
		t.Fatalf("expected the checker report header; got:\n%s", out)
	}
	// CRUX assertion 2 (the load-bearing one): REAL git output — the diff-since
	// line carries real changed paths. The pre-rewrite degradation printed
	// "git unavailable" here because every git op tripped the blocked
	// socketpair under NetDeny.
	if strings.Contains(out, "git unavailable") {
		t.Fatalf("CRUX BROKEN: checker degraded to 'git unavailable' under the strict sandbox — the file-backed capture did not take:\n%s", out)
	}
	if !strings.Contains(out, "changed paths") {
		t.Fatalf("expected the diff-since line to carry real changed paths; got:\n%s", out)
	}
}

// TestDeferTriggers_CreatesMissingTmp: the wrapper pre-creates <repoRoot>/tmp
// (the sole RWDir, where the checker's git-capture helpers write scratch). In a
// fresh scratch repo with NO tmp dir, the wrapper must create it AND the run
// must succeed (the checker's allocCaptureDir mkdir of <repo>/tmp/defer-git-*
// would fail if the parent did not exist).
func TestDeferTriggers_CreatesMissingTmp(t *testing.T) {
	sandboxFeatureCheck(t)
	dir := setupDeferScratchRepo(t)

	// Confirm tmp does not exist before the run.
	tmpDir := filepath.Join(dir, "tmp")
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("precondition: scratch repo tmp should not exist; stat err=%v", err)
	}

	out, exit := runDeferTriggers(t, dir)
	if exit != 0 {
		t.Fatalf("defer-triggers should exit 0 after creating tmp; got exit=%d:\n%s", exit, out)
	}
	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatalf("wrapper should have created %s; stat err=%v", tmpDir, err)
	}
}

// TestDeferTriggers_RejectsPositionalArg: the command takes NO arguments
// (cobra.NoArgs). A positional arg must be rejected with a non-zero exit so a
// caller cannot influence what the wrapper runs.
func TestDeferTriggers_RejectsPositionalArg(t *testing.T) {
	sandboxFeatureCheck(t)
	out, exit := runDeferTriggers(t, repoRoot, "extra-arg")
	if exit == 0 {
		t.Fatalf("defer-triggers must reject a positional arg; got exit=0:\n%s", out)
	}
}

// TestDeferTriggers_RejectsFlags_ModeAndNetNonOverridable: the wrapper defines
// NO flags, so --sandbox / --net (the exec-sandbox downgrade axes) are UNKNOWN
// flags and must be rejected. This is the non-overridability proof: a granted
// read-only agent CANNOT weaken the hardcoded ModeStrict + NetDeny containment
// because there is no flag surface to do so.
func TestDeferTriggers_RejectsFlags_ModeAndNetNonOverridable(t *testing.T) {
	sandboxFeatureCheck(t)
	for _, flag := range []string{"--sandbox=off", "--net=allow", "--sandbox=best-effort", "--cwd=/tmp"} {
		out, exit := runDeferTriggers(t, repoRoot, flag)
		if exit == 0 {
			t.Fatalf("defer-triggers must reject unknown flag %q (mode/net/cwd non-overridable); got exit=0:\n%s", flag, out)
		}
	}
}

// TestDeferTriggers_MissingChecker_ReturnsError: when the checker is absent at
// <repoRoot>/.opencode/scripts/check-defer-triggers.mjs (e.g. before
// `vh-agent-harness update`), the wrapper must return a clear non-zero error
// rather than running an undefined target.
func TestDeferTriggers_MissingChecker_ReturnsError(t *testing.T) {
	sandboxFeatureCheck(t)
	// A tempdir with .git (so findRepoRoot resolves here) but NO checker.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	out, exit := runDeferTriggers(t, dir)
	if exit == 0 {
		t.Fatalf("defer-triggers must fail when the checker is missing; got exit=0:\n%s", out)
	}
	if !strings.Contains(out, "checker not found") && !strings.Contains(out, "check-defer-triggers.mjs") {
		t.Fatalf("expected a 'checker not found' error pointing at update; got:\n%s", out)
	}
}

// TestDeferTriggers_ExitPropagated: the wrapper propagates the child exit code
// via os.Exit (mirroring exec-sandbox — NOT root.exitCodeFromError). The
// canonical checker exits 0 in promoter mode (never blocking), so the nonzero
// branch is structurally unreachable through the happy path. To witness
// propagation honestly, this test plants a checker copy that exits a SPECIFIC
// non-zero code (process.exit(7)); the wrapper must surface exactly that code.
// (The os.Exit mechanism itself is also proven by the exec-sandbox suite, which
// shares the identical propagation line.)
func TestDeferTriggers_ExitPropagated(t *testing.T) {
	sandboxFeatureCheck(t)
	dir := t.TempDir()
	// A real-enough repo skeleton so the wrapper's pre-checks pass: .git for
	// findRepoRoot, tmp will be created, and a checker copy that exits 7.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	scriptsDir := filepath.Join(dir, ".opencode", "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	// A checker that exits a distinctive non-zero code. node runs it under the
	// strict sandbox; the wrapper must os.Exit(7) to propagate it.
	probe := []byte("process.exit(7);\n")
	if err := os.WriteFile(filepath.Join(scriptsDir, "check-defer-triggers.mjs"), probe, 0o644); err != nil {
		t.Fatalf("write probe checker: %v", err)
	}
	out, exit := runDeferTriggers(t, dir)
	if exit != 7 {
		t.Fatalf("defer-triggers must propagate the child's exit code 7; got exit=%d:\n%s", exit, out)
	}
}

// TestDeferTriggers_NoWriteOutsideTmp: the wrapper runs the canonical checker
// under ModeStrict + NetDeny (hardcoded, no flags). The checker writes ONLY to
// <repo>/tmp (its git-capture scratch). After a successful run, no file must
// appear under .git/ (the integrity boundary). This is the containment witness
// for defer-triggers; the full ModeStrict+NetDeny containment contract is
// proven by the exec-sandbox suite (same execsandbox.Run path).
func TestDeferTriggers_NoWriteOutsideTmp(t *testing.T) {
	sandboxFeatureCheck(t)
	dir := setupDeferScratchRepo(t)

	// Snapshot .git/ contents before the run.
	gitDir := filepath.Join(dir, ".git")
	before, err := os.ReadDir(gitDir)
	if err != nil {
		t.Fatalf("readdir .git before run: %v", err)
	}
	beforeNames := make(map[string]bool, len(before))
	for _, e := range before {
		beforeNames[e.Name()] = true
	}

	out, exit := runDeferTriggers(t, dir)
	if exit != 0 {
		t.Fatalf("defer-triggers should exit 0; got exit=%d:\n%s", exit, out)
	}
	// No new file under .git/ (the checker must not write outside tmp).
	after, err := os.ReadDir(gitDir)
	if err != nil {
		t.Fatalf("readdir .git after run: %v", err)
	}
	for _, e := range after {
		if !beforeNames[e.Name()] {
			t.Fatalf("CRUX BROKEN: defer-triggers created a file under .git/ (%q) — write outside tmp under the strict sandbox:\n%s", e.Name(), out)
		}
	}
}
