package redlines

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Slice 0 tests settle the exact-object locator contract. They use a real
// temporary git repository to PROVE that an immutable tree hash binds the
// scanner input to the authorized state even after the working tree and shared
// index are mutated. No registry, no matching, no gate wiring is involved.

// gitInitRepo creates a throwaway git repo under t.TempDir(), returns its
// absolute path. Configures a synthetic author so commits do not depend on the
// host's git identity.
func gitInitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "config", "user.email", "synthetic@example.invalid")
	gitRun(t, dir, "config", "user.name", "synthetic-test")
	// Default branch name varies by git version; normalize to "main" for
	// deterministic HEAD references in assertions.
	gitRun(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	return dir
}

// gitRun runs a git command in dir, failing the test on error. Stdout is
// trimmed and returned for the caller to assert on.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git -C %s %v: %v\n%s", dir, args, err, out.String())
	}
	return strings.TrimSpace(out.String())
}

// writeTreeOf builds a single synthetic file, stages it on the SHARED index
// (the gate uses a private index; here we only need SOME tree object to obtain
// an immutable hash), and returns the `git write-tree` hash. The shared index
// is used purely to mint a hash; the test then proves the hash is decoupled
// from further shared-index/working-tree mutation.
func writeTreeOf(t *testing.T, repo, name, content string) string {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	gitRun(t, repo, "add", "--", name)
	return gitRun(t, repo, "write-tree")
}

// TestScanTarget_ImmutableTreeBindsAfterMutation is the load-bearing Slice 0
// proof: after a ScanTarget is minted from a tree hash, mutating BOTH the
// working tree AND the shared index does not change what that hash resolves to.
// `git ls-tree -r <hash>` still yields the ORIGINAL content, proving the
// scanner input follows the supplied object — not the live state.
func TestScanTarget_ImmutableTreeBindsAfterMutation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
	repo := gitInitRepo(t)

	// State S1: one synthetic file with content "synthetic-alpha".
	treeS1 := writeTreeOf(t, repo, "synthetic-marker.txt", "synthetic-alpha\n")

	target, err := NewScanTarget(repo, treeS1)
	if err != nil {
		t.Fatalf("NewScanTarget: %v", err)
	}

	// Mutate working tree AND shared index to a DIFFERENT state S2. This is
	// exactly the hazard the locator must reject: if the scanner read the
	// working tree or shared index it would see S2; it must see S1.
	if err := os.WriteFile(filepath.Join(repo, "synthetic-marker.txt"), []byte("synthetic-beta REWRITE\n"), 0o644); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	gitRun(t, repo, "add", "--", "synthetic-marker.txt")
	// Also add a brand-new file and stage it, so the shared index clearly
	// diverges from the tree the hash captured.
	if err := os.WriteFile(filepath.Join(repo, "synthetic-extra.txt"), []byte("synthetic-extra\n"), 0o644); err != nil {
		t.Fatalf("write extra: %v", err)
	}
	gitRun(t, repo, "add", "--", "synthetic-extra.txt")

	// The hash still resolves to S1: original content, original single file.
	ls := gitRun(t, repo, "ls-tree", "-r", "--format=%(path)%x09%(objectmode)%x09%(objectname)", target.TreeHash)
	lines := strings.Split(strings.TrimSpace(ls), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "synthetic-marker.txt\t") {
		t.Fatalf("tree hash resolved to unexpected entry set after WT/index mutation:\n%s", ls)
	}
	blob := gitRun(t, repo, "cat-file", "blob", target.TreeHash+":"+"synthetic-marker.txt")
	if blob != "synthetic-alpha" {
		t.Fatalf("tree hash content drifted after mutation: want synthetic-alpha, got %q", blob)
	}

	// The target itself is unchanged by construction.
	if target.TreeHash != treeS1 {
		t.Fatalf("ScanTarget.TreeHash mutated: want %s, got %s", treeS1, target.TreeHash)
	}
}

// TestScanTarget_ValidationFailsClosed covers the contract's fail-safe cases:
// missing repo root, non-absolute repo root, missing hash, malformed hash. Each
// must return a non-nil error and a zero-value ScanTarget, and must NEVER
// substitute HEAD/working-tree/shared-index state.
func TestScanTarget_ValidationFailsClosed(t *testing.T) {
	const validHash = "0000000000000000000000000000000000000000"
	cases := []struct {
		name     string
		repoRoot string
		treeHash string
	}{
		{"missing repo root", "", validHash},
		{"relative repo root", "relative/path", validHash},
		{"missing tree hash", "/tmp/whatever", ""},
		{"malformed hash short", "/tmp/whatever", "abc123"},
		{"malformed hash non-hex", "/tmp/whatever", "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"},
		{"malformed hash uppercase", "/tmp/whatever", "0123456789ABCDEF0123456789ABCDEF01234567"},
		{"malformed hash wrong length", "/tmp/whatever", "0123456789abcdef0123456789abcdef"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			target, err := NewScanTarget(c.repoRoot, c.treeHash)
			if err == nil {
				t.Fatalf("expected error, got nil target=%+v", target)
			}
			// Zero-value on failure: no partial locator leaks out.
			if target.TreeHash != "" || target.RepoRoot != "" {
				t.Fatalf("expected zero-value ScanTarget on error, got %+v", target)
			}
			// The error must be opaque and must not propose a fallback.
			msg := err.Error()
			for _, frag := range []string{"HEAD", "worktree", "working tree", "index"} {
				if strings.Contains(strings.ToLower(msg), strings.ToLower(frag)) {
					// "no implicit fallback" is allowed (it names what we refuse
					// to do); anything suggesting we DID fall back is a bug.
					if strings.Contains(msg, "no implicit fallback") {
						continue
					}
					t.Fatalf("error suggests fallback to %s: %v", frag, err)
				}
			}
		})
	}
}

// TestScanTarget_NoFallbackAPI proves structurally that there is no constructor
// deriving a ScanTarget from HEAD/the working tree/the shared index. This is a
// compile-time-resolved guarantee, but we assert the public surface by trying
// the only documented constructor and confirming it REQUIRES an explicit hash.
func TestScanTarget_NoFallbackAPI(t *testing.T) {
	// Every input that omits the explicit tree hash is rejected. There is no
	// alternative entry point that would supply one implicitly.
	if _, err := NewScanTarget("/tmp/whatever", ""); err == nil {
		t.Fatal("NewScanTarget accepted an empty tree hash — implicit fallback is not allowed")
	}
}

// TestScanTarget_ZeroFootprint proves the locator layer creates no files and
// emits no side effects. (The registry-level zero-footprint guarantee for the
// no-registry case is exercised in registry_test.go.)
func TestScanTarget_ZeroFootprint(t *testing.T) {
	dir := t.TempDir()
	before, _ := os.ReadDir(dir)
	target, err := NewScanTarget(dir, "0000000000000000000000000000000000000000")
	if err != nil {
		t.Fatalf("NewScanTarget: %v", err)
	}
	after, _ := os.ReadDir(dir)
	if len(before) != len(after) {
		t.Fatalf("NewScanTarget created files in %s: before=%d after=%d target=%+v", dir, len(before), len(after), target)
	}
	_ = target // target is a pure value; holding it writes nothing
}

// TestScanTarget_HexRegexAcceptsBothHashSizes guards the object-id regex
// against the SHA-1 (40) / SHA-256 (64) split that git supports.
func TestScanTarget_HexRegexAcceptsBothHashSizes(t *testing.T) {
	good := []string{
		strings.Repeat("0", 40),
		strings.Repeat("a", 64),
		"0123456789abcdef0123456789abcdef01234567",
	}
	for _, h := range good {
		if !hexObjectIDRe.MatchString(h) {
			t.Errorf("expected hash %q to be accepted", h)
		}
	}
	bad := []string{
		strings.Repeat("0", 39),
		strings.Repeat("0", 41),
		strings.Repeat("0", 63),
		strings.Repeat("0", 65),
		strings.Repeat("g", 40), // non-hex
	}
	for _, h := range bad {
		if hexObjectIDRe.MatchString(h) {
			t.Errorf("expected hash %q to be rejected", h)
		}
	}
}
