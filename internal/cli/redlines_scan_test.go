package cli

// redlines_scan_test.go tests `vh-agent-harness redlines scan --tree <hash>` —
// the headless exact-tree scanner. All fixtures use OBVIOUSLY synthetic terms.

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- git test helpers (scan-specific; complement gitInit from gitignore_check_test.go) ---

// gitConfigTest sets the local git identity so commits work in a temp repo.
func gitConfigTest(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "config", "user.email", "test@example.com")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
}

// gitRun runs a git command in dir, failing the test on error.
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s in %s: %v%s", strings.Join(args, " "), dir, err, stderr.String())
	}
}

// gitOut runs a git command in dir and returns trimmed stdout, failing on error.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s: %v%s", strings.Join(args, " "), dir, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// writeRepoFile writes content to name under dir, creating parent dirs.
func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// commitAll stages everything and commits, returning the HEAD tree hash.
func commitAll(t *testing.T, dir string) string {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "test")
	return gitOut(t, dir, "rev-parse", "HEAD^{tree}")
}

// runScan executes `redlines scan --tree <hash> -C <dir>` and returns combined
// output + error.
func runScan(t *testing.T, dir, treeHash string) (string, error) {
	t.Helper()
	return executeCapture(t, []string{"redlines", "scan", "--tree", treeHash, "-C", dir})
}

// scanExitCode maps the RunE error to the exit code the binary would use.
func scanExitCode(err error) int {
	return exitCodeFromError(err)
}

// assertNoLeak fails if output contains any of the synthetic configured terms.
// This is the no-leakage grep across all violation tests.
func assertNoLeak(t *testing.T, output string, label, desc string, terms ...string) {
	t.Helper()
	for _, term := range terms {
		if strings.Contains(output, term) {
			t.Errorf("%s [%s]: output leaks configured term %q:\n--- output ---\n%s", label, desc, term, output)
		}
	}
}

// synthetic terms used across tests (all obviously fake).
const (
	termScrub    = "synthetic-scrub-vocab"
	termSideA    = "synthetic-org-alpha"
	termSideB    = "synthetic-domain-beta"
	termAmbientB = "synthetic-ambient-gamma"
	termAmbientA = "synthetic-ambient-org"
)

// --- inert cases ---

// TestRedlinesScan_AbsentRegistryIsInert: no registry → exit 0, short status
// line, no files written, no terms.
func TestRedlinesScan_AbsentRegistryIsInert(t *testing.T) {
	xdg, xdgCleanup := setRedlinesXDG(t)
	defer xdgCleanup()
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	tree := commitAll(t, repoDir)

	assertRedlinesNoNewFiles(t, xdg, "XDG dir")
	assertRedlinesNoNewFiles(t, repoDir, "repo dir")

	out, err := runScan(t, repoDir, tree)
	if err != nil {
		t.Fatalf("absent registry: want nil error (exit 0), got %v (code=%d)", err, scanExitCode(err))
	}
	if !strings.Contains(out, "no registry configured") {
		t.Errorf("absent registry: output should say 'no registry configured'; got:\n%s", out)
	}
	for _, banned := range []string{termScrub, termSideA, termSideB, "subj-"} {
		if strings.Contains(out, banned) {
			t.Errorf("absent registry: output must not contain %q:\n%s", banned, out)
		}
	}
}

// TestRedlinesScan_NonBindingRegistryIsInert: registry exists but no subject
// binds this repo → exit 0, short status line.
func TestRedlinesScan_NonBindingRegistryIsInert(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
    repos: ["/totally/elsewhere/**"]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "README.md", "# clean\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if err != nil {
		t.Fatalf("non-binding: want nil error (exit 0), got %v", err)
	}
	if !strings.Contains(out, "no redlines bind this repository") {
		t.Errorf("non-binding: output should say so; got:\n%s", out)
	}
	if strings.Contains(out, termScrub) {
		t.Errorf("non-binding: must not emit terms:\n%s", out)
	}
}

// TestRedlinesScan_CleanApplicableRegistry: binding registry, but the tree has
// no synthetic terms → exit 0, no findings.
func TestRedlinesScan_CleanApplicableRegistry(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "README.md", "# clean repo\nno issues here\n")
	writeRepoFile(t, repoDir, "src/main.go", "package main\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if err != nil {
		t.Fatalf("clean applicable: want nil error (exit 0), got %v", err)
	}
	if strings.Contains(out, "subj-") {
		t.Errorf("clean applicable: must not emit findings:\n%s", out)
	}
	if !strings.Contains(out, "lexical/best-effort") {
		t.Errorf("clean applicable: honesty pointer missing:\n%s", out)
	}
}

// --- violation cases ---

// TestRedlinesScan_ScrubViolation: a committed blob contains the synthetic scrub
// label → finding printed opaque, exit 1, NO configured term in output.
func TestRedlinesScan_ScrubViolation(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
    why: private-scrub-rationale-text
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "notes.md", "# notes\nthis mentions "+termScrub+" in passing\n")
	writeRepoFile(t, repoDir, "clean.go", "package main\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if code := scanExitCode(err); code != 1 {
		t.Fatalf("scrub violation: want exit 1, got %d (err=%v)", code, err)
	}
	// Finding must be opaque: path + subj-id + reason.
	if !strings.Contains(out, "subj-test-scrub") {
		t.Errorf("scrub violation: output missing subj-id:\n%s", out)
	}
	if !strings.Contains(out, "scrub-term") {
		t.Errorf("scrub violation: output missing reason code 'scrub-term':\n%s", out)
	}
	if !strings.Contains(out, "notes.md") {
		t.Errorf("scrub violation: output missing path 'notes.md':\n%s", out)
	}
	// No configured term / why may appear.
	assertNoLeak(t, out, "scrub-violation", "stdout", termScrub, "private-scrub-rationale-text")
}

// TestRedlinesScan_RelationCoOccurrence: both synthetic sides in the SAME file →
// finding, exit 1.
func TestRedlinesScan_RelationCoOccurrence(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-rel
    kind: forbidden-relation
    side_a: [`+termSideA+`]
    side_b: [`+termSideB+`]
    unit: file
    why: private-relation-rationale
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "doc.txt", "project uses "+termSideA+" with "+termSideB+"\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if code := scanExitCode(err); code != 1 {
		t.Fatalf("co-occurrence: want exit 1, got %d", code)
	}
	if !strings.Contains(out, "subj-test-rel") {
		t.Errorf("co-occurrence: output missing subj-id:\n%s", out)
	}
	if !strings.Contains(out, "relation-co-occurrence") {
		t.Errorf("co-occurrence: output missing reason code:\n%s", out)
	}
	if !strings.Contains(out, "doc.txt") {
		t.Errorf("co-occurrence: output missing path:\n%s", out)
	}
	assertNoLeak(t, out, "co-occurrence", "stdout", termSideA, termSideB, "private-relation-rationale")
}

// TestRedlinesScan_RelationAmbientViolation: repo bound as ambient, side_b term
// alone in a blob → finding, exit 1.
func TestRedlinesScan_RelationAmbientViolation(t *testing.T) {
	setRedlinesXDG(t)
	repoDir := t.TempDir()
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-amb
    kind: forbidden-relation
    side_a: [`+termAmbientA+`]
    side_b: [`+termAmbientB+`]
    ambient_repos: ["`+repoDir+`**"]
    why: private-ambient-rationale
`)
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	// SideB term alone (no SideA) — but ambient means SideA is implied by repo identity.
	writeRepoFile(t, repoDir, "config.yml", "endpoint: "+termAmbientB+"\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if code := scanExitCode(err); code != 1 {
		t.Fatalf("ambient: want exit 1, got %d", code)
	}
	if !strings.Contains(out, "subj-test-amb") {
		t.Errorf("ambient: output missing subj-id:\n%s", out)
	}
	if !strings.Contains(out, "relation-ambient-side-b") {
		t.Errorf("ambient: output missing reason code:\n%s", out)
	}
	assertNoLeak(t, out, "ambient", "stdout", termAmbientA, termAmbientB, "private-ambient-rationale")
}

// TestRedlinesScan_RelationSidesInSeparateFiles: side_a in one file, side_b in
// another → NO finding (cross-unit non-hit), exit 0.
func TestRedlinesScan_RelationSidesInSeparateFiles(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-rel
    kind: forbidden-relation
    side_a: [`+termSideA+`]
    side_b: [`+termSideB+`]
    unit: file
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "a.txt", "org: "+termSideA+"\n")
	writeRepoFile(t, repoDir, "b.txt", "domain: "+termSideB+"\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if err != nil {
		t.Fatalf("separate files: want nil error (exit 0), got %v (code=%d)", err, scanExitCode(err))
	}
	if strings.Contains(out, "subj-test-rel") {
		t.Errorf("separate files: must NOT emit a finding (cross-unit non-hit):\n%s", out)
	}
}

// --- fail-closed ---

// TestRedlinesScan_InvalidRegistryFailsClosed: a present-but-malformed registry
// → exit 2, opaque error, no term leakage.
func TestRedlinesScan_InvalidRegistryFailsClosed(t *testing.T) {
	setRedlinesXDG(t)
	// Invalid: subject id is not opaque (fails the subj-* id validation).
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: not-opaque-id
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "README.md", "# clean\n")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if code := scanExitCode(err); code != 2 {
		t.Fatalf("invalid registry: want exit 2 (fail-closed), got %d (err=%v)", code, err)
	}
	// The label must NOT appear in the opaque error output.
	assertNoLeak(t, out, "invalid-registry", "output", termScrub)
}

// --- exact-tree isolation ---

// TestRedlinesScan_ExactTreeIsolation: scan T1 (clean tree), then mutate the
// working tree with a violation WITHOUT staging/committing, and re-scan T1.
// The scan must reflect T1 only — proving NO fallback to the working tree.
func TestRedlinesScan_ExactTreeIsolation(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	// T1: clean tree (no synthetic terms).
	writeRepoFile(t, repoDir, "README.md", "# clean tree\n")
	t1 := commitAll(t, repoDir)

	// Scan T1 → clean (exit 0).
	out1, err1 := runScan(t, repoDir, t1)
	if err1 != nil {
		t.Fatalf("T1 clean: want exit 0, got %v (code=%d)", err1, scanExitCode(err1))
	}
	if strings.Contains(out1, "subj-") {
		t.Fatalf("T1 should be clean:\n%s", out1)
	}

	// Mutate working tree: add a violating file WITHOUT staging/committing.
	writeRepoFile(t, repoDir, "unstaged.md", "leaked "+termScrub+"\n")
	// Also modify an existing tracked file in the working tree.
	writeRepoFile(t, repoDir, "README.md", "# mutated\n"+termScrub+"\n")

	// Re-scan T1 → must STILL be clean (exit 0, no finding).
	out2, err2 := runScan(t, repoDir, t1)
	if err2 != nil {
		t.Fatalf("T1 after working-tree mutation: want exit 0 (no fallback), got %v (code=%d)", err2, scanExitCode(err2))
	}
	if strings.Contains(out2, "subj-") {
		t.Errorf("T1 scan must NOT reflect working-tree mutation (no fallback to worktree):\n%s", out2)
	}
	if strings.Contains(out2, "unstaged.md") {
		t.Errorf("T1 scan must NOT see the unstaged file:\n%s", out2)
	}
}

// TestRedlinesScan_ExactTreeReflectsCommittedViolation: T1 has a violation; T2
// (a later commit that removes it) is clean. Scanning each tree reflects ONLY
// that tree's content — a deleted path (in HEAD but absent in T2) is not
// enumerated, proving the tree scan sees current tree entries only.
func TestRedlinesScan_DeletedPathNotEnumerated(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)

	// T1: has a file with the synthetic term.
	writeRepoFile(t, repoDir, "secret.md", "contains "+termScrub+"\n")
	t1 := commitAll(t, repoDir)

	// Scan T1 → violation found (exit 1).
	out1, err1 := runScan(t, repoDir, t1)
	if code := scanExitCode(err1); code != 1 {
		t.Fatalf("T1 (with violation): want exit 1, got %d", code)
	}
	if !strings.Contains(out1, "secret.md") {
		t.Errorf("T1: output should reference 'secret.md':\n%s", out1)
	}

	// Delete the file and commit → T2 (without the file).
	os.Remove(filepath.Join(repoDir, "secret.md"))
	t2 := commitAll(t, repoDir)

	// Scan T2 → clean (exit 0). The deleted path is not in T2's tree.
	out2, err2 := runScan(t, repoDir, t2)
	if err2 != nil {
		t.Fatalf("T2 (deleted path): want exit 0, got %v (code=%d)", err2, scanExitCode(err2))
	}
	if strings.Contains(out2, "secret.md") {
		t.Errorf("T2: deleted path must NOT be enumerated:\n%s", out2)
	}
	if strings.Contains(out2, "subj-") {
		t.Errorf("T2: must not emit findings:\n%s", out2)
	}
}

// --- tree-entry edge cases: symlink + submodule skip ---

// TestRedlinesScan_SymlinkSkipped: a symlink (mode 120000) whose target text
// contains the synthetic label is NOT scanned — no finding, no crash.
func TestRedlinesScan_SymlinkSkipped(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)

	// Regular file (clean) so the commit is non-empty.
	writeRepoFile(t, repoDir, "README.md", "# repo\n")
	// Symlink whose target text IS the synthetic label. If the scanner did
	// not skip symlinks, the blob content (= target text) would match.
	linkPath := filepath.Join(repoDir, "link.txt")
	if err := os.Symlink(termScrub, linkPath); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	gitRun(t, repoDir, "add", "link.txt")
	tree := commitAll(t, repoDir)

	out, err := runScan(t, repoDir, tree)
	if err != nil {
		t.Fatalf("symlink skip: want exit 0, got %v (code=%d)", err, scanExitCode(err))
	}
	if strings.Contains(out, "subj-") {
		t.Errorf("symlink must be skipped (no finding); a finding means the symlink blob was scanned:\n%s", out)
	}
}

// TestRedlinesScan_SubmoduleEntrySkipped: a mode-160000 gitlink entry (submodule)
// is skipped — no crash, no false finding.
func TestRedlinesScan_SubmoduleEntrySkipped(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "README.md", "# repo\n")
	gitRun(t, repoDir, "add", "-A")
	gitRun(t, repoDir, "commit", "-q", "-m", "init")
	headCommit := gitOut(t, repoDir, "rev-parse", "HEAD")

	// Plant a gitlink entry (mode 160000) pointing at our own commit.
	gitRun(t, repoDir, "update-index", "--add", "--cacheinfo",
		fmt.Sprintf("160000,%s,vendor/dep", headCommit))
	tree := gitOut(t, repoDir, "write-tree")

	// Verify the tree actually has the 160000 entry (sanity).
	lsOut := gitOut(t, repoDir, "ls-tree", tree, "vendor/dep")
	if !strings.HasPrefix(lsOut, "160000 commit ") {
		t.Fatalf("test setup: expected 160000 commit entry, got: %s", lsOut)
	}

	out, err := runScan(t, repoDir, tree)
	if err != nil {
		t.Fatalf("submodule skip: want exit 0, got %v (code=%d)", err, scanExitCode(err))
	}
	if strings.Contains(out, "vendor/dep") {
		t.Errorf("submodule entry must be skipped (not scanned):\n%s", out)
	}
}

// --- flag validation ---

// TestRedlinesScan_MissingTreeFlag: no --tree → exit 2, no scan, no term leakage.
func TestRedlinesScan_MissingTreeFlag(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	writeRepoFile(t, repoDir, "README.md", "# clean\n")
	commitAll(t, repoDir)

	out, err := executeCapture(t, []string{"redlines", "scan", "-C", repoDir})
	if err == nil {
		t.Fatal("missing --tree: want non-nil error (exit non-zero), got nil")
	}
	if code := scanExitCode(err); code != 2 {
		t.Errorf("missing --tree: want exit 2, got %d", code)
	}
	assertNoLeak(t, out, "missing-tree", "output", termScrub)
}

// TestRedlinesScan_InvalidTreeHash: non-hex --tree → exit 2, no scan.
func TestRedlinesScan_InvalidTreeHash(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	commitAll(t, repoDir)

	out, err := runScan(t, repoDir, "not-a-valid-hash")
	if err == nil {
		t.Fatal("invalid tree: want non-nil error, got nil")
	}
	if code := scanExitCode(err); code != 2 {
		t.Errorf("invalid tree: want exit 2, got %d", code)
	}
	assertNoLeak(t, out, "invalid-tree", "output", termScrub)
}

// TestRedlinesScan_NonexistentTreeHash: valid hex format but not a real object
// → git ls-tree fails → exit 2 (fail-closed), no term leakage.
func TestRedlinesScan_NonexistentTreeHash(t *testing.T) {
	setRedlinesXDG(t)
	writeRedlinesUserReg(t, `version: 1
subjects:
  - id: subj-test-scrub
    kind: scrub-project
    labels: [`+termScrub+`]
`)
	repoDir := t.TempDir()
	gitInit(t, repoDir)
	gitConfigTest(t, repoDir)
	commitAll(t, repoDir)

	fakeHash := strings.Repeat("a", 40) // valid format, not a real object
	out, err := runScan(t, repoDir, fakeHash)
	if err == nil {
		t.Fatal("nonexistent tree: want non-nil error, got nil")
	}
	if code := scanExitCode(err); code != 2 {
		t.Errorf("nonexistent tree: want exit 2, got %d", code)
	}
	assertNoLeak(t, out, "nonexistent-tree", "output", termScrub)
}
