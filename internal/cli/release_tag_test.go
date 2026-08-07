package cli

// Shared release-tag wrapper-test helpers for scripts/release-tag.sh.
//
// scripts/release-tag.sh is the sanctioned release-tag wrapper (PROJECT-LOCAL,
// not templates/core). It invokes the deterministic release-DEFER evaluator
// (.opencode/scripts/check-defer-triggers.mjs --mode=release) BEFORE the
// `git tag -a` mutation. Release mode is manifest-authority ONLY (the legacy
// .local/-scan release path has been RETIRED); the manifest-mode wrapper tests
// live in release_tag_manifest_test.go.
//
// The helpers below (setupReleaseTagRepo, tagExists) are shared with
// release_tag_manifest_test.go. They black-box the wrapper inside a fully-
// isolated temp git repo, mirroring the hermetic pattern of
// TestCommitGate_BacklogSplitPreflight. The evaluator template is copied from
// templates/core/ and the {{COORDINATOR_DIR}} token is rendered to "coordinator"
// (what `vh-agent-harness update` produces), so the tests do NOT depend on
// `make update` having run.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permission"
)

// harnessBinaryOnce builds the vh-agent-harness binary at most once per test
// process and prepends its directory to PATH. This lets scripts/release-tag.sh's
// G0c gate resolve the binary when run from scratch test fixtures. Because
// os.Setenv modifies the process environment, ALL subsequent os.Environ() calls
// (used to build cmd.Env) inherit the binary.
//
// After the Layer 1 ceremony-binary pin, G0c resolves ./bin/vh-agent-harness
// (NEVER PATH); copyHarnessBinaryToCeremony places the built binary at that
// ceremony-local path inside a scratch fixture.
var (
	harnessBinaryOnce sync.Once
	harnessBinaryPath string
	harnessBinaryErr  error
)

func ensureHarnessBinaryOnPath(t *testing.T) {
	t.Helper()
	harnessBinaryOnce.Do(func() {
		root := findModuleRoot(t)
		dir, err := os.MkdirTemp("", "vh-test-bin-*")
		if err != nil {
			harnessBinaryErr = fmt.Errorf("mkdtemp: %w", err)
			return
		}
		binPath := filepath.Join(dir, "vh-agent-harness")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/vh-agent-harness")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			harnessBinaryErr = fmt.Errorf("go build harness binary: %w\n%s", err, out)
			return
		}
		harnessBinaryPath = binPath
		os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		// G0c gates doctor on seam-installation (lineage.yml presence); scratch
		// fixtures lack lineage.yml, so G0c skips there naturally — no env-var
		// bypass is needed or shipped. The binary-on-PATH is still required for
		// the dedicated G0c test whose fixture DOES write a lineage.yml AND
		// copies the binary to ./bin/vh-agent-harness (the ceremony-local path
		// G0c resolves after the Layer 1 pin).
	})
	if harnessBinaryErr != nil {
		t.Fatalf("ensure harness binary on PATH: %v", harnessBinaryErr)
	}
}

// copyHarnessBinaryToCeremony copies the once-built harness binary to
// <destDir>/bin/vh-agent-harness — the ceremony-local path scripts/release-tag.sh
// G0c resolves after the Layer 1 stale-PATH-binary fix (the wrapper NEVER
// resolves PATH for G0c). Tests that force G0c to run (by writing a lineage.yml)
// MUST place the ceremony binary via this helper, otherwise G0c refuses with the
// "ceremony binary missing" recipe-list red before reaching doctor.
func copyHarnessBinaryToCeremony(t *testing.T, destDir string) {
	t.Helper()
	ensureHarnessBinaryOnPath(t)
	dst := filepath.Join(destDir, "bin", "vh-agent-harness")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir ceremony bin: %v", err)
	}
	data, err := os.ReadFile(harnessBinaryPath)
	if err != nil {
		t.Fatalf("read built harness binary: %v", err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatalf("write ceremony binary: %v", err)
	}
}

// requireNodeMinMajor resolves node from PATH and skips the test (naming node,
// the observed version, and the required bar) when node is missing, below the
// declared permission.NodeMinMajor, or misbehaving (e.g. `node --version`
// output cannot be parsed).
//
// This replaces a presence-only exec.LookPath("node") in the release-tag
// wrapper-test setup. scripts/release-tag.sh spawns `node` (the .mjs
// release-DEFER evaluator) inside the scratch fixture; a below-bar or
// misbehaving node used to fall through to an opaque `JSON.parse(”)` failure
// deep in the wrapper across ~17 ReleaseTag* tests. Gating on the shared
// permission.NodeMinMajor source-of-truth (the same bar `harness preflight` /
// `harness doctor` / the shell-guard bridge enforce) converts that into a
// clean, named skip at the test boundary, while a meets-bar node runs the
// suite unchanged.
func requireNodeMinMajor(t *testing.T) {
	t.Helper()
	bin, major, err := permission.ProbeNode()
	if err != nil {
		t.Skipf("node >= %d unavailable for release-tag wrapper test: %v "+
			"(install Node >= %d, or point PATH/nvm at a satisfying node)",
			permission.NodeMinMajor, err, permission.NodeMinMajor)
	}
	if major < permission.NodeMinMajor {
		t.Skipf("node %s reports major %d; release-tag wrapper tests require node >= %d "+
			"(switch nvm alias / PATH to a satisfying node)",
			bin, major, permission.NodeMinMajor)
	}
}

// setupReleaseTagRepo creates an isolated scratch git repo with:
//   - scripts/release-tag.sh copied from the repo source (project-local)
//   - .opencode/scripts/check-defer-triggers.mjs copied from the TEMPLATE with
//     {{COORDINATOR_DIR}} rendered to "coordinator" (simulating the rendered
//     artifact that `vh-agent-harness update` produces)
//   - .local/coordinator/tasks/ ready for test-written cards
//   - a prior tag v0.1.0 + post-tag changes to fileA.go and dir/fileC.go
//     (so the release arc v0.1.0..HEAD contains exactly those two paths;
//     fileB.go is UNCHANGED and therefore NOT in the arc)
//   - a valid annotated-tag message file at a temp path OUTSIDE <scratch>
//     (so the wrapper's G0b clean-worktree gate does not flag it as an
//     untracked file).
//
// Returns (scratch, wrapperPath, tasksDir, msgFile).
func setupReleaseTagRepo(t *testing.T) (scratch, wrapper, tasksDir, msgFile string) {
	t.Helper()
	for _, bin := range []string{"bash", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH: %v", bin, err)
		}
	}
	requireNodeMinMajor(t)
	ensureHarnessBinaryOnPath(t)
	root := findModuleRoot(t)

	// Copy the project-local wrapper (source = shipped).
	wrapperSrc := filepath.Join(root, "scripts", "release-tag.sh")
	wrapperBody, err := os.ReadFile(wrapperSrc)
	if err != nil {
		t.Fatalf("read wrapper source %s: %v", wrapperSrc, err)
	}
	// Copy the TEMPLATE evaluator and render the coordinator token (what
	// `vh-agent-harness update` produces for .opencode/scripts/).
	evalSrc := filepath.Join(root, "templates", "core", ".opencode", "scripts", "check-defer-triggers.mjs")
	evalBody, err := os.ReadFile(evalSrc)
	if err != nil {
		t.Fatalf("read evaluator template %s: %v", evalSrc, err)
	}
	renderedEval := strings.ReplaceAll(string(evalBody), "{{COORDINATOR_DIR}}", "coordinator")

	scratch = t.TempDir()
	// Write wrapper at <scratch>/scripts/release-tag.sh.
	wrapper = filepath.Join(scratch, "scripts", "release-tag.sh")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(wrapper, wrapperBody, 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	// Write rendered evaluator at <scratch>/.opencode/scripts/.
	evalDst := filepath.Join(scratch, ".opencode", "scripts", "check-defer-triggers.mjs")
	if err := os.MkdirAll(filepath.Dir(evalDst), 0o755); err != nil {
		t.Fatalf("mkdir .opencode/scripts: %v", err)
	}
	if err := os.WriteFile(evalDst, []byte(renderedEval), 0o644); err != nil {
		t.Fatalf("write evaluator: %v", err)
	}
	// Create .local/coordinator/tasks/ (the rendered default tasks dir).
	tasksDir = filepath.Join(scratch, ".local", "coordinator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks dir: %v", err)
	}

	// git init + identity + initial commit + tag v0.1.0 + post-tag changes.
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", scratch}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	git("config", "commit.gpgsign", "false")

	writeFile := func(rel, content string) {
		t.Helper()
		full := filepath.Join(scratch, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// Seed a buildable, gofmt-stable Go module so the wrapper's G0
	// green-tree gate (go test/vet/build/gofmt) passes by default. The
	// package name is non-main so no `func main()` is required.
	//
	// Seed .gitignore with `bin/` (mirrors the real repo) so a ceremony-local
	// binary placed at ./bin/vh-agent-harness (the path G0c resolves after the
	// Layer 1 pin) is invisible to the G0b clean-worktree gate. Without this,
	// the untracked ceremony binary would dirty `git status --short` and refuse
	// the tag at G0b before G0c ever runs. The .gitignore is in the INITIAL
	// (pre-tag) commit, so it is NOT in the release arc (fileA.go + dir/fileC.go).
	writeFile(".gitignore", "bin/\n")
	writeFile("go.mod", "module scratch\n\ngo 1.21\n")
	writeFile("fileA.go", "package scratch\n")
	writeFile("fileB.go", "package scratch\n")
	writeFile("dir/fileC.go", "package scratch\n")
	git("add", "-A")
	git("commit", "-q", "-m", "initial")
	git("tag", "v0.1.0")
	// Post-tag changes (in the arc v0.1.0..HEAD): fileA.go + dir/fileC.go.
	// Use declarations (not bare comments) so gofmt -l stays clean.
	writeFile("fileA.go", "package scratch\n\n// FileAChanged marks the arc commit.\nconst FileAChanged = true\n")
	writeFile("dir/fileC.go", "package scratch\n\n// FileCChanged marks the arc commit.\nconst FileCChanged = true\n")
	git("add", "-A")
	git("commit", "-q", "-m", "changes for release")

	// Annotated-tag message file.
	msgFile = filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	return scratch, wrapper, tasksDir, msgFile
}

// tagExists checks whether the given tag exists in the scratch repo.
func tagExists(t *testing.T, scratch, tag string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", scratch, "tag", "-l", tag)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	return strings.TrimSpace(string(out)) == tag
}
