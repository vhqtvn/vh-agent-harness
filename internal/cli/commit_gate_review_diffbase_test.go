package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitGate_ReviewDiffBase is the S1 regression guard for approved-tree
// integrity under concurrency (decision 2026-07-27-commit-scope-integrity, S1:
// "review diff base repoint"). S1 forwards head_at_acquire at the
// committer→reviewer handoff and repoints the reviewer's scope diff off bare
// `HEAD` onto the acquire-time anchor. The reviewer-side repoint is prose (an
// LLM leaf), so it cannot be driven mechanically; this test pins the SUBSTRATE
// contract the repoint relies on, and directly demonstrates the phantom-file
// hazard the repoint closes:
//
//   - acquire emits head_at_acquire == the actual acquire-time HEAD (the anchor
//     the reviewer is now instructed to diff against);
//   - the staged tree_hash is built from a private index seeded from
//     head_at_acquire and stages ONLY the requested paths, so an unrequested
//     dirty/concurrent file never enters the reviewed tree;
//   - after a concurrent commit moves HEAD, `git diff <head_at_acquire>
//     <tree_hash>` (the S1 review base) names ONLY the requested file, while
//     `git diff <moved-HEAD> <tree_hash>` (the OLD bare-`HEAD` review base)
//     names the requested file AND a phantom reverse-change for the concurrent
//     file — the exact symptom S1 eliminates.
//
// It black-boxes the RENDERED .opencode/scripts/commit-gate.sh (NOT the
// template) inside a fully-isolated scratch git repo, mirroring
// TestCommitGate_CloseoutLedger's harness.
func TestCommitGate_ReviewDiffBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	repoRoot := filepath.Join("..", "..")
	srcScripts := filepath.Join(repoRoot, ".opencode", "scripts")
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `vh-agent-harness update` first)", f, err)
		}
	}

	setupScratchRepo := func(t *testing.T) (dir, dstScripts string) {
		t.Helper()
		dir = t.TempDir()
		dstScripts = filepath.Join(dir, ".opencode", "scripts")
		if err := os.MkdirAll(dstScripts, 0o755); err != nil {
			t.Fatalf("mkdir scripts: %v", err)
		}
		for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
			data, err := os.ReadFile(filepath.Join(srcScripts, f))
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if err := os.WriteFile(filepath.Join(dstScripts, f), data, 0o755); err != nil {
				t.Fatalf("write %s: %v", f, err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"),
			[]byte(`{ "$schema": "https://opencode.ai/config.json", "agent": { "build": { "description": "test" } } }`),
			0o644); err != nil {
			t.Fatalf("write opencode.jsonc: %v", err)
		}
		agentsDir := filepath.Join(dir, ".opencode", "agents")
		if err := os.MkdirAll(agentsDir, 0o755); err != nil {
			t.Fatalf("mkdir agents: %v", err)
		}
		if err := os.WriteFile(filepath.Join(agentsDir, "build.md"),
			[]byte("---\ndescription: test\nmode: primary\n---\n# build\n"), 0o644); err != nil {
			t.Fatalf("write build.md: %v", err)
		}
		gitIn := func(args ...string) {
			t.Helper()
			cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		gitIn("init", "-q")
		gitIn("symbolic-ref", "HEAD", "refs/heads/main")
		gitIn("config", "user.email", "t@t")
		gitIn("config", "user.name", "t")
		gitIn("config", "commit.gpgsign", "false")
		return dir, dstScripts
	}

	genUUID := func(t *testing.T, dstScripts string) string {
		t.Helper()
		out, err := exec.Command("bash", filepath.Join(dstScripts, "readonly-scripts.sh"), "gen-uuid").Output()
		if err != nil {
			t.Fatalf("gen-uuid: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	writeAgentMsg := func(t *testing.T, dir, uuid string) string {
		t.Helper()
		rel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-"+uuid))
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir scratch: %v", err)
		}
		if err := os.WriteFile(full, []byte("test commit message\n"), 0o644); err != nil {
			t.Fatalf("write msg: %v", err)
		}
		return rel
	}

	// runGate invokes commit-gate.sh with the given subcommand + args (cwd =
	// scratch repo) and returns the parsed status JSON.
	runGate := func(t *testing.T, dstScripts, dir string, args ...string) map[string]any {
		t.Helper()
		cmd := exec.Command("bash", append([]string{filepath.Join(dstScripts, "commit-gate.sh")}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		combined := string(out)
		var parsed map[string]any
		for _, line := range strings.Split(combined, "\n") {
			tl := strings.TrimSpace(line)
			if !strings.HasPrefix(tl, "{") || !strings.HasSuffix(tl, "}") {
				continue
			}
			var cand map[string]any
			if json.Unmarshal([]byte(tl), &cand) == nil {
				if _, ok := cand["status"]; ok {
					parsed = cand
				}
			}
		}
		if err != nil {
			t.Fatalf("commit-gate %v failed: %v\n%s", args, err, combined)
		}
		if parsed == nil {
			t.Fatalf("commit-gate %v produced no status JSON\n%s", args, combined)
		}
		return parsed
	}

	gitIn := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			ec, _ := cmd.CombinedOutput()
			t.Fatalf("git %v: %v\n%s", args, err, ec)
		}
		return strings.TrimSpace(string(out))
	}

	// diffNames returns the sorted, trimmed set of changed paths a given
	// `git diff <base> <tree>` reports (the review scope a leaf would see under
	// that base). This is the crux comparator: under head_at_acquire the set is
	// exactly the requested slice; under a moved bare HEAD the set acquires a
	// phantom reverse-change for the concurrent file.
	diffNames := func(t *testing.T, dir, base, tree string) []string {
		t.Helper()
		out := gitIn(t, dir, "diff", "--no-color", "--name-only", base, tree)
		if out == "" {
			return nil
		}
		names := strings.Split(out, "\n")
		for i := range names {
			names[i] = strings.TrimSpace(names[i])
		}
		return names
	}

	// -----------------------------------------------------------------
	// Subtest 1: acquire emits head_at_acquire == the actual acquire-time HEAD
	// (the anchor S1 instructs the reviewer to diff against).
	// -----------------------------------------------------------------
	t.Run("acquire_emits_head_at_acquire_anchor", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		// Seed an initial commit so HEAD is a real anchor (not unborn).
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v1\n"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		gitIn(t, dir, "add", "file.txt")
		gitIn(t, dir, "commit", "-q", "-m", "seed")
		headAtAcquire := gitIn(t, dir, "rev-parse", "HEAD")

		// Dirty the requested file before acquire.
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "diffbase-test")
		if status, _ := acq["status"].(string); status != "acquired" {
			t.Fatalf("expected status acquired, got %v", acq)
		}
		got, _ := acq["head_at_acquire"].(string)
		if got != headAtAcquire {
			t.Errorf("acquire head_at_acquire = %q, want actual HEAD %q (the anchor the reviewer must diff against)", got, headAtAcquire)
		}
		if _, ok := acq["tree_hash"]; !ok {
			t.Errorf("acquire output missing tree_hash: %v", acq)
		}
	})

	// -----------------------------------------------------------------
	// Subtest 2: an unrequested dirty file never enters the reviewed
	// tree_hash/scope. The staged tree is built from a private index seeded
	// from head_at_acquire containing ONLY the requested paths.
	// -----------------------------------------------------------------
	t.Run("reviewed_tree_excludes_unrequested_dirty_file", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v1\n"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		gitIn(t, dir, "add", "file.txt")
		gitIn(t, dir, "commit", "-q", "-m", "seed")
		headAtAcquire := gitIn(t, dir, "rev-parse", "HEAD")

		// Make BOTH file.txt (requested) AND other.txt (unrequested) dirty.
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify file.txt: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("dirty\n"), 0o644); err != nil {
			t.Fatalf("modify other.txt: %v", err)
		}

		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "diffbase-test")
		if status, _ := acq["status"].(string); status != "acquired" {
			t.Fatalf("expected status acquired, got %v", acq)
		}
		treeHash, _ := acq["tree_hash"].(string)
		if treeHash == "" {
			t.Fatalf("acquire returned empty tree_hash: %v", acq)
		}

		// The S1 review base (head_at_acquire): scope is EXACTLY [file.txt].
		// other.txt — dirty in the worktree but NOT requested — must NOT appear.
		got := diffNames(t, dir, headAtAcquire, treeHash)
		if len(got) != 1 || got[0] != "file.txt" {
			t.Errorf("S1 review scope (diff head_at_acquire tree_hash) = %v, want exactly [file.txt] (unrequested dirty other.txt must not enter the reviewed tree)", got)
		}
	})

	// -----------------------------------------------------------------
	// Subtest 3 (the S1 crux): under a concurrent commit that moves HEAD, the
	// acquire-anchor base excludes the concurrent file; bare HEAD does not.
	// This directly demonstrates the phantom-file hazard S1 eliminates and
	// pins that the correct review base is head_at_acquire, not bare HEAD.
	// -----------------------------------------------------------------
	t.Run("acquire_anchor_excludes_concurrent_phantom_files", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v1\n"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		gitIn(t, dir, "add", "file.txt")
		gitIn(t, dir, "commit", "-q", "-m", "seed")
		headAtAcquire := gitIn(t, dir, "rev-parse", "HEAD")

		// Requested slice: edit file.txt and acquire it. (other.txt does not
		// exist yet at acquire time.)
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify file.txt: %v", err)
		}
		uuidA := genUUID(t, dstScripts)
		msgRel := writeAgentMsg(t, dir, uuidA)
		acq := runGate(t, dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "diffbase-test")
		if status, _ := acq["status"].(string); status != "acquired" {
			t.Fatalf("expected status acquired, got %v", acq)
		}
		treeHash, _ := acq["tree_hash"].(string)
		if treeHash == "" {
			t.Fatalf("acquire returned empty tree_hash: %v", acq)
		}

		// Simulate a CONCURRENT committer landing an UNRELATED file (other.txt)
		// during the lock-free review window. HEAD moves H0 -> H1.
		if err := os.WriteFile(filepath.Join(dir, "other.txt"), []byte("concurrent\n"), 0o644); err != nil {
			t.Fatalf("write other.txt: %v", err)
		}
		gitIn(t, dir, "add", "other.txt")
		gitIn(t, dir, "commit", "-q", "-m", "concurrent")
		movedHead := gitIn(t, dir, "rev-parse", "HEAD")
		if movedHead == headAtAcquire {
			t.Fatalf("concurrent commit did not move HEAD; setup invariant violated")
		}

		// S1 review base (head_at_acquire): scope is EXACTLY [file.txt] — the
		// concurrent other.txt does NOT pollute the reviewed scope.
		s1Scope := diffNames(t, dir, headAtAcquire, treeHash)
		if len(s1Scope) != 1 || s1Scope[0] != "file.txt" {
			t.Errorf("S1 review scope (diff head_at_acquire tree_hash) = %v, want exactly [file.txt]; the acquire anchor must exclude the concurrent file", s1Scope)
		}

		// OLD bare-HEAD review base (movedHead): scope ACQUIRES a phantom
		// reverse-change for other.txt — the exact symptom S1 eliminates. This
		// assertion documents the hazard: if a future change reverts the
		// reviewer to bare HEAD, this is the phantom that re-appears.
		oldScope := diffNames(t, dir, movedHead, treeHash)
		hasPhantom := false
		for _, n := range oldScope {
			if n == "other.txt" {
				hasPhantom = true
			}
		}
		if !hasPhantom {
			t.Errorf("OLD bare-HEAD review scope (diff movedHEAD tree_hash) = %v; expected it to contain phantom 'other.txt' (proving bare HEAD is the wrong base). If other.txt is absent, the phantom demonstration is stale and the test must be revisited.", oldScope)
		}
	})
}
