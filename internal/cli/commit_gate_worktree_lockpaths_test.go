package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitGate_WorktreeLockPathHonesty is the automated seam coverage for
// the AGG-F2 deferred finding (trigger path_touched(templates/core/.opencode/
// scripts/commit-gate.sh) — fired via f0ce85e) plus the two same-file
// hardenings that landed with it. It black-boxes the RENDERED
// .opencode/scripts/commit-gate.sh inside fully-isolated scratch git repos,
// mirroring the harness of TestCommitGate_MessageCleanup /
// TestCommitGate_CloseoutLedger / TestCommitGate_HalfBornLockStaleBreak.
//
// NOTE on `git worktree add` below: it is a TEST-side operation on a scratch
// repo under t.TempDir() (Go test binaries are not shell-guarded); it never
// touches the real repo checkout or its worktrees/.
//
// Coverage (each subtest is a real behavioral outcome, not mechanism-assertion):
//   - linked_worktree_round_trip: the per-invocation git-dir resolution
//     end-to-end. `git worktree add` a scratch checkout inside the scratch
//     repo, acquire from the worktree cwd, and assert the private index lands
//     under <main-scratch>/.git/worktrees/<name>/commit-gate/index-<uuid>
//     (NOT under the main checkout's .git — the pre-worktree-safe code's
//     literal `.git/...` failed ENOTDIR there because a worktree's .git is a
//     FILE). Release then reclaims the per-worktree session scratch cleanly.
//   - lock_path_as_file_honest_error: a stray regular FILE at the resolved
//     lock path (<gitdir>/commit-gate.lock) must classify as
//     status=error reason=lock_dir_unavailable — NOT contended/race_lost.
//     Before the Part-2 masquerade guard, EEXIST was treated as contention
//     unconditionally, so this unretryable environment failure masqueraded
//     as a losable race (the committer-noted finding). The stray file must
//     SURVIVE (the gate never destroys a path it cannot identify as its
//     own — same fail-safe as _stale_break's empty-uuid refusal).
//   - index_dir_as_file_rollback: a regular FILE at the gate index dir
//     (<gitdir>/commit-gate) must surface status=error
//     reason=gate_index_dir_unavailable with the half-born LOCK_DIR rolled
//     back. Before the AGG-F3 guard, readonly-scripts.sh prep-tempdir died
//     non-zero (its own set -e) and, under the gate's set -e, killed the
//     gate mid-acquire with NO status JSON at all — the caller could not
//     classify the failure — while leaving the acquired lock held
//     (unbreakable by _stale_break's fail-safe, which refuses meta-less
//     locks).
func TestCommitGate_WorktreeLockPathHonesty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}

	// Rendered corpus scripts (repo-relative; go test cwd = internal/cli/).
	repoRoot := filepath.Join("..", "..")
	srcScripts := filepath.Join(repoRoot, ".opencode", "scripts")
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `vh-agent-harness update` first)", f, err)
		}
	}

	// setupScratchRepo mirrors the harness in the sibling gate tests: isolated
	// temp git repo + rendered gate scripts + minimal valid opencode config +
	// git init/identity.
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

	// copyGateFixtureInto copies the scripts + minimal opencode config from
	// the main scratch repo into another checkout root (the linked worktree),
	// mirroring a real per-worktree harness install so _config_validate
	// (which anchors REPO_ROOT at the SCRIPT's parents[2]) validates the
	// worktree's own config, not the main checkout's.
	copyGateFixtureInto := func(t *testing.T, srcRoot, dstRoot string) string {
		t.Helper()
		dstScripts := filepath.Join(dstRoot, ".opencode", "scripts")
		if err := os.MkdirAll(dstScripts, 0o755); err != nil {
			t.Fatalf("mkdir wt scripts: %v", err)
		}
		for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
			data, err := os.ReadFile(filepath.Join(srcRoot, ".opencode", "scripts", f))
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if err := os.WriteFile(filepath.Join(dstScripts, f), data, 0o755); err != nil {
				t.Fatalf("write %s: %v", f, err)
			}
		}
		if err := os.WriteFile(filepath.Join(dstRoot, "opencode.jsonc"),
			[]byte(`{ "$schema": "https://opencode.ai/config.json", "agent": { "build": { "description": "test" } } }`),
			0o644); err != nil {
			t.Fatalf("write wt opencode.jsonc: %v", err)
		}
		agentsDir := filepath.Join(dstRoot, ".opencode", "agents")
		if err := os.MkdirAll(agentsDir, 0o755); err != nil {
			t.Fatalf("mkdir wt agents: %v", err)
		}
		if err := os.WriteFile(filepath.Join(agentsDir, "build.md"),
			[]byte("---\ndescription: test\nmode: primary\n---\n# build\n"), 0o644); err != nil {
			t.Fatalf("write wt build.md: %v", err)
		}
		return dstScripts
	}

	// runGateJSON invokes commit-gate.sh with the given subcommand + args
	// (cwd = the given dir). Returns the parsed JSON (last {status:...} line),
	// combined output, and any exec error. Non-fatal so callers can assert on
	// refused/error statuses that exit non-zero.
	runGateJSON := func(dstScripts, dir string, args ...string) (map[string]any, string, error) {
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
		return parsed, combined, err
	}

	// gitOut runs git -C dir and returns trimmed stdout.
	gitOut := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.Output()
		if err != nil {
			ec, _ := cmd.CombinedOutput()
			t.Fatalf("git %v: %v\n%s", args, err, ec)
		}
		return strings.TrimSpace(string(out))
	}

	// gitRun runs git -C dir, failing on error (no stdout needed).
	gitRun := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	seedAndCommit := func(t *testing.T, dir, body string) {
		t.Helper()
		full := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
		gitRun(t, dir, "add", "file.txt")
		gitRun(t, dir, "commit", "-q", "-m", "seed")
	}

	// genUUID shells out to the in-repo readonly-scripts.sh gen-uuid.
	genUUID := func(t *testing.T, dstScripts string) string {
		t.Helper()
		out, err := exec.Command("bash", filepath.Join(dstScripts, "readonly-scripts.sh"), "gen-uuid").Output()
		if err != nil {
			t.Fatalf("gen-uuid: %v", err)
		}
		return strings.TrimSpace(string(out))
	}

	// writeAgentMsg writes the agent-authored message scratch file at
	// tmp/commit-gate-message/msg-${uuid} inside dir and returns its
	// dir-relative slash path.
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

	// absFrom resolves p (possibly git-dir-relative) against base (the cwd
	// the gate ran in) to a clean absolute path.
	absFrom := func(base, p string) string {
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		return filepath.Clean(p)
	}

	// isUnder reports whether path is strictly inside dir.
	isUnder := func(path, dir string) bool {
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return false
		}
		return rel != "." && !strings.HasPrefix(rel, "..")
	}

	// pathExists / isRegularFile are small stat helpers.
	pathExists := func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	}
	isRegularFile := func(p string) bool {
		info, err := os.Stat(p)
		return err == nil && info.Mode().IsRegular()
	}

	// ---------------------------------------------------------------------
	// Subtest 1: linked-worktree round-trip (per-invocation git-dir
	// resolution, end-to-end). A worktree's `.git` is a FILE (a gitdir
	// pointer), so any literal cwd-relative `.git/...` path fails ENOTDIR.
	// The gate resolves the git dir per invocation, so the lock, private
	// index, and session metadata must all anchor to the worktree's REAL git
	// dir <main-scratch>/.git/worktrees/<name>/ — and acquire→release must
	// round-trip cleanly there.
	// ---------------------------------------------------------------------
	t.Run("linked_worktree_round_trip", func(t *testing.T) {
		dir, _ := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")

		// TEST-side scratch worktree INSIDE the scratch repo (never the real
		// repo's worktrees/). Branch wt-aggt from HEAD; checkout at dir/wt-aggt.
		const wtName = "wt-aggt"
		wtDir := filepath.Join(dir, wtName)
		gitRun(t, dir, "worktree", "add", "-b", wtName, wtDir)

		// Premise: a linked worktree's .git is a FILE (the gitdir pointer).
		if !isRegularFile(filepath.Join(wtDir, ".git")) {
			t.Fatalf("test premise broken: %s/.git is not a regular file (not a linked worktree?)", wtDir)
		}

		// Per-worktree harness install (scripts + config), mirroring a real
		// per-worktree install so _config_validate sees the worktree's config.
		wtScripts := copyGateFixtureInto(t, dir, wtDir)

		// The worktree's resolved git dir must be the per-worktree directory
		// under the MAIN scratch repo's .git/worktrees/<name>/.
		wtGitDir := absFrom(wtDir, gitOut(t, wtDir, "rev-parse", "--git-dir"))
		mainWorktreesDir := filepath.Join(dir, ".git", "worktrees")
		if !isUnder(wtGitDir, mainWorktreesDir) || filepath.Base(wtGitDir) != wtName {
			t.Fatalf("test premise broken: worktree git dir %s is not under %s/<%s>", wtGitDir, mainWorktreesDir, wtName)
		}

		// A real working-tree change in the worktree + agent msg scratch.
		if err := os.WriteFile(filepath.Join(wtDir, "file.txt"), []byte("v2-from-worktree\n"), 0o644); err != nil {
			t.Fatalf("modify wt file: %v", err)
		}
		uuidA := genUUID(t, wtScripts)
		msgRel := writeAgentMsg(t, wtDir, uuidA)

		// Acquire FROM the worktree cwd (this is where the historical literal
		// `.git/...` paths failed ENOTDIR).
		acq, combined, errA := runGateJSON(wtScripts, wtDir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "wt-roundtrip")
		if errA != nil {
			t.Fatalf("worktree acquire failed: %v\n%s", errA, combined)
		}
		statusA, _ := acq["status"].(string)
		if statusA != "acquired" {
			t.Fatalf("expected status acquired, got %v\n%s", acq, combined)
		}
		uuidG, _ := acq["uuid"].(string)
		if uuidG == "" {
			t.Fatalf("acquire did not return a uuid: %v", acq)
		}

		// THE core assertion: private_index landed under the WORKTREE git
		// dir's commit-gate/, not the main checkout's .git.
		piRaw, _ := acq["private_index"].(string)
		if piRaw == "" {
			t.Fatalf("acquire did not return private_index: %v", acq)
		}
		privateIndex := absFrom(wtDir, piRaw)
		if want := filepath.Join(wtGitDir, "commit-gate", "index-"+uuidG); privateIndex != want {
			t.Fatalf("private_index %s does not equal the per-worktree path %s (git-dir resolution regressed)", privateIndex, want)
		}
		if !pathExists(privateIndex) {
			t.Fatalf("private_index %s does not exist on disk", privateIndex)
		}
		// And the MAIN checkout's .git must NOT have grown a commit-gate/ dir
		// (proves no fallback to the literal main .git path).
		if pathExists(filepath.Join(dir, ".git", "commit-gate")) {
			t.Fatalf("main checkout .git/commit-gate exists — gate state leaked out of the worktree git dir")
		}
		// The acquire-phase lock is released by acquire itself; nothing held.
		if pathExists(filepath.Join(wtGitDir, "commit-gate.lock")) {
			t.Fatalf("worktree lock dir still present after acquire (acquire must release it)")
		}
		// Session metadata is per-worktree too.
		if !pathExists(filepath.Join(wtGitDir, "commit-gate", "meta-"+uuidG)) {
			t.Fatalf("session meta-%s must exist under the worktree git dir's commit-gate/", uuidG)
		}

		// Release from the worktree cwd; the per-worktree session scratch
		// must be reclaimed cleanly.
		rel, combinedR, errR := runGateJSON(wtScripts, wtDir, "release",
			"--uuid", uuidG,
			"--message-file", msgRel)
		if errR != nil {
			t.Fatalf("worktree release failed: %v\n%s", errR, combinedR)
		}
		if statusR, _ := rel["status"].(string); statusR != "released" {
			t.Fatalf("expected status released, got %v\n%s", rel, combinedR)
		}
		for _, gone := range []string{
			filepath.Join(wtGitDir, "commit-gate", "index-"+uuidG),
			filepath.Join(wtGitDir, "commit-gate", "meta-"+uuidG),
			filepath.Join(wtGitDir, "commit-gate.lock"),
			filepath.Join(wtDir, filepath.FromSlash(msgRel)),
		} {
			if pathExists(gone) {
				t.Fatalf("post-release cleanliness: %s still exists (worktree session scratch not reclaimed)", gone)
			}
		}
		t.Logf("linked-worktree round-trip clean: private_index=%s (uuid=%s)", privateIndex, uuidG)
	})

	// ---------------------------------------------------------------------
	// Subtest 2: lock-path-as-file → honest lock_dir_unavailable error.
	// A stray regular FILE at <gitdir>/commit-gate.lock makes mkdir fail
	// EEXIST; before the masquerade guard, EEXIST was unconditionally
	// contended/race_lost — an unretryable environment failure dressed as a
	// losable race. (Covers mission items 2 AND 4: the Part-2 guard is
	// applied, so the honest error is asserted, not just documented.)
	// ---------------------------------------------------------------------
	t.Run("lock_path_as_file_honest_error", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		msgRel := writeAgentMsg(t, dir, genUUID(t, dstScripts))

		// The stray regular FILE at the resolved lock path (parent .git/ is a
		// fine dir — only the lock path itself is occupied by a file).
		strayLock := filepath.Join(dir, ".git", "commit-gate.lock")
		if err := os.WriteFile(strayLock, []byte("stray\n"), 0o644); err != nil {
			t.Fatalf("write stray lock file: %v", err)
		}

		acq, combined, _ := runGateJSON(dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "lockfile-stray")

		if acq == nil {
			t.Fatalf("acquire produced no status JSON at all (AGG-F3-class abort):\n%s", combined)
		}
		status, _ := acq["status"].(string)
		reason, _ := acq["reason"].(string)
		if status == "contended" {
			t.Fatalf("MASQUERADE REGRESSION: stray regular file at the lock path classified as contended/race_lost (%v) — must be a distinct error\n%s", acq, combined)
		}
		if status != "error" || reason != "lock_dir_unavailable" {
			t.Fatalf("expected status=error reason=lock_dir_unavailable, got status=%q reason=%q\n%s", status, reason, combined)
		}

		// The stray file is NOT ours to destroy: it must survive untouched.
		if !isRegularFile(strayLock) {
			t.Fatalf("REGRESSION: the gate removed or replaced the stray regular file at the lock path %s (never destroy an unidentified path)", strayLock)
		}
		t.Logf("lock-path-as-file honest: status=error reason=lock_dir_unavailable (stray file preserved)")
	})

	// ---------------------------------------------------------------------
	// Subtest 3: index-dir-as-file → gate_index_dir_unavailable + LOCK_DIR
	// rollback. A regular FILE at <gitdir>/commit-gate makes
	// readonly-scripts.sh prep-tempdir die non-zero (its own set -e) —
	// before the AGG-F3 guard that killed the gate under set -e with NO
	// status JSON, leaving the freshly-mkdir'd lock held (meta-less =
	// unbreakable by _stale_break's fail-safe). The guard must emit the
	// honest error AND roll the half-born lock back.
	// ---------------------------------------------------------------------
	t.Run("index_dir_as_file_rollback", func(t *testing.T) {
		dir, dstScripts := setupScratchRepo(t)
		seedAndCommit(t, dir, "v1\n")
		if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0o644); err != nil {
			t.Fatalf("modify: %v", err)
		}
		msgRel := writeAgentMsg(t, dir, genUUID(t, dstScripts))

		// The stray regular FILE at the gate index dir path.
		strayIndexDir := filepath.Join(dir, ".git", "commit-gate")
		if err := os.WriteFile(strayIndexDir, []byte("stray\n"), 0o644); err != nil {
			t.Fatalf("write stray index-dir file: %v", err)
		}

		acq, combined, _ := runGateJSON(dstScripts, dir, "acquire",
			"--paths", `["file.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "indexdir-stray")

		if acq == nil {
			t.Fatalf("AGG-F3 REGRESSION: acquire aborted with no status JSON (unguarded prep-tempdir failure killed the gate under set -e):\n%s", combined)
		}
		status, _ := acq["status"].(string)
		reason, _ := acq["reason"].(string)
		if status != "error" || reason != "gate_index_dir_unavailable" {
			t.Fatalf("expected status=error reason=gate_index_dir_unavailable, got status=%q reason=%q\n%s", status, reason, combined)
		}

		// THE rollback assertion: the half-born LOCK_DIR (mkdir succeeded,
		// meta never written) must have been removed — the gate is not left
		// holding an unbreakable meta-less lock.
		lockDir := filepath.Join(dir, ".git", "commit-gate.lock")
		if pathExists(lockDir) {
			t.Fatalf("AGG-F3 REGRESSION: half-born lock dir %s left behind after gate_index_dir_unavailable (gate left held)", lockDir)
		}
		// And only the LOCK was rolled back — the stray file is untouched.
		if !isRegularFile(strayIndexDir) {
			t.Fatalf("REGRESSION: the gate removed or replaced the stray regular file at %s (rollback must touch only its own lock dir)", strayIndexDir)
		}
		t.Logf("index-dir-as-file honest: status=error reason=gate_index_dir_unavailable, half-born lock rolled back")
	})
}
