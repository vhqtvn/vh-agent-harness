package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// skipGateTestEnv returns a deterministic environment for SKIP_COMMIT_GATE
// runs of the gate: the ambient environment with any OPENCODE_SESSION_ID
// scrubbed (so the operator-context check passes regardless of where go test
// was launched) and SKIP_COMMIT_GATE=1 set.
func skipGateTestEnv() []string {
	env := []string{"SKIP_COMMIT_GATE=1"}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "OPENCODE_SESSION_ID=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// skipGateSetup builds the shared scratch-repo harness for SKIP-path tests:
// rendered gate scripts copied into an isolated .opencode/scripts/, minimal
// valid opencode.jsonc + one agents/*.md, git init + identity + one seed
// commit containing tracked.txt. Mirrors TestCommitGate_DirectoryDeletionStaging.
func skipGateSetup(t *testing.T) (dir, dstScripts string) {
	t.Helper()
	for _, tool := range []string{"git", "bash", "python3"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}

	// Rendered corpus scripts (repo-relative; go test cwd = internal/cli/).
	repoRoot := filepath.Join("..", "..")
	srcScripts := filepath.Join(repoRoot, ".opencode", "scripts")
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `vh-agent-harness update` first)", f, err)
		}
	}

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
	gitIn("config", "user.email", "t@t")
	gitIn("config", "user.name", "t")
	gitIn("config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write tracked.txt: %v", err)
	}
	gitIn("add", "tracked.txt")
	gitIn("commit", "-q", "-m", "seed")
	return dir, dstScripts
}

// skipGateRun runs the rendered gate with the given env and args, returning
// combined output and the process error.
func skipGateRun(t *testing.T, dir, dstScripts string, env []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join(dstScripts, "commit-gate.sh")}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// skipGateStatus extracts the last JSON object carrying a "status" key from
// combined gate output (json_out prints last).
func skipGateStatus(t *testing.T, combined string) map[string]any {
	t.Helper()
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
	if parsed == nil {
		t.Fatalf("gate produced no status JSON\n%s", combined)
	}
	return parsed
}

// TestCommitGate_SkipPathCommitsIndexAsLeft is the regression guard for the
// implicit-wholesale-staging hazard in the SKIP_COMMIT_GATE escape hatch: the
// SKIP acquire path historically ran `git add -A`, sweeping every dirty and
// untracked file in the repo into the escape-hatch commit (the pattern behind
// a documented 83.9 MB history-pollution incident).
//
// New semantics under test:
//   - SKIP acquire stages NOTHING implicitly — after acquire, the index holds
//     exactly what the operator staged (file A), while unrelated untracked B
//     and modified-tracked C stay out of the index.
//   - SKIP commit commits the index exactly as left: the commit contains A
//     only, and B/C remain dirty/untracked in the worktree afterwards.
//   - Message handling is preserved (--message-file reaches the commit).
func TestCommitGate_SkipPathCommitsIndexAsLeft(t *testing.T) {
	dir, dstScripts := skipGateSetup(t)
	env := skipGateTestEnv()

	// Operator stages the deliberate slice: A.
	if err := os.WriteFile(filepath.Join(dir, "a-slice.txt"), []byte("slice\n"), 0o644); err != nil {
		t.Fatalf("write a-slice.txt: %v", err)
	}
	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitIn("add", "a-slice.txt")

	// Unrelated dirt the escape hatch must NOT sweep in: untracked B and
	// modified-tracked C.
	if err := os.WriteFile(filepath.Join(dir, "b-untracked.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write b-untracked.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write tracked.txt v2: %v", err)
	}

	// SKIP acquire (message-file passed to preserve the committer invocation
	// shape; the SKIP branch parses but does not persist it).
	msgRel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-skip-fixture"))
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, msgRel)), 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, msgRel), []byte("skip fixture message\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	combined, err := skipGateRun(t, dir, dstScripts, env, "acquire", "--message-file", msgRel)
	if err != nil {
		t.Fatalf("SKIP acquire failed: %v\n%s", err, combined)
	}
	if status, _ := skipGateStatus(t, combined)["status"].(string); status != "acquired" {
		t.Fatalf("expected SKIP acquire status acquired, got %v\n%s", skipGateStatus(t, combined), combined)
	}

	// HAZARD ASSERTION: acquire staged nothing implicitly. The index must hold
	// exactly A; B stays untracked and C stays modified-unstaged. Under the
	// old `git add -A` behavior all three collapse into staged entries here.
	st, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	got := string(st)
	want := "A  a-slice.txt\n M tracked.txt\n?? .opencode/\n?? b-untracked.txt\n?? opencode.jsonc\n?? tmp/\n"
	if got != want {
		t.Fatalf("SKIP acquire must not stage implicitly:\n got %q\nwant %q", got, want)
	}

	// SKIP commit with --message-file (message handling preserved).
	combined, err = skipGateRun(t, dir, dstScripts, env, "commit", "--message-file", msgRel)
	if err != nil {
		t.Fatalf("SKIP commit failed: %v\n%s", err, combined)
	}
	parsed := skipGateStatus(t, combined)
	if status, _ := parsed["status"].(string); status != "committed" {
		t.Fatalf("expected SKIP commit status committed, got %v\n%s", parsed, combined)
	}

	// The commit contains ONLY the staged slice.
	show, err := exec.Command("git", "-C", dir, "show", "--pretty=format:", "--name-only", "HEAD").Output()
	if err != nil {
		t.Fatalf("git show: %v", err)
	}
	if strings.TrimSpace(string(show)) != "a-slice.txt" {
		t.Fatalf("escape-hatch commit must contain only the explicitly staged slice, got %q", string(show))
	}

	// The commit message came from --message-file.
	msg, err := exec.Command("git", "-C", dir, "log", "-1", "--pretty=%B").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if strings.TrimSpace(string(msg)) != "skip fixture message" {
		t.Fatalf("SKIP commit must use the --message-file message, got %q", string(msg))
	}

	// B and C remain in the worktree, unchanged by the commit.
	st, err = exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	got = string(st)
	want = " M tracked.txt\n?? .opencode/\n?? b-untracked.txt\n?? opencode.jsonc\n?? tmp/\n"
	if got != want {
		t.Fatalf("unrelated dirt must survive the escape-hatch commit untouched:\n got %q\nwant %q", got, want)
	}
}

// TestCommitGate_SkipPathEmptyIndexRefused proves the fail-closed side of the
// new SKIP semantics: with nothing staged, SKIP commit refuses with guidance
// to stage explicit paths first — it never falls back to staging -A or `.` —
// and no commit is created.
func TestCommitGate_SkipPathEmptyIndexRefused(t *testing.T) {
	dir, dstScripts := skipGateSetup(t)
	env := skipGateTestEnv()

	// Dirty worktree but EMPTY index (nothing staged).
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}
	headBefore, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}

	combined, err := skipGateRun(t, dir, dstScripts, env, "commit", "--message", "should be refused")
	if err == nil {
		t.Fatalf("SKIP commit with empty index must fail closed, got success:\n%s", combined)
	}
	parsed := skipGateStatus(t, combined)
	if status, _ := parsed["status"].(string); status != "error" {
		t.Fatalf("expected status error for empty-index SKIP commit, got %v\n%s", parsed, combined)
	}
	if reason, _ := parsed["reason"].(string); reason != "skip_gate_empty_index" {
		t.Fatalf("expected reason skip_gate_empty_index, got %v\n%s", parsed, combined)
	}
	if msg, _ := parsed["message"].(string); !strings.Contains(msg, "git add") {
		t.Fatalf("empty-index refusal must guide the operator to stage explicit paths (git add <paths>), got %q", msg)
	}

	// Fail closed: no commit was created.
	headAfter, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	if string(headBefore) != string(headAfter) {
		t.Fatalf("empty-index SKIP commit must not create a commit: HEAD moved %s -> %s",
			strings.TrimSpace(string(headBefore)), strings.TrimSpace(string(headAfter)))
	}
}

// environReflectsEnvp reports whether /proc/self/environ in the current
// environment mirrors the exec-time envp. Some container runtimes present an
// empty /proc/self/environ even though the live environment is populated
// (observed in the dev container: 0 environ entries vs a fully populated
// `env`), which makes the gate's OpenCode-context refusal — a check that
// reads /proc/self/environ — unobservable there.
func environReflectsEnvp(t *testing.T) bool {
	t.Helper()
	cmd := exec.Command("bash", "-c",
		`tr '\0' '\n' < /proc/self/environ | grep -q '^COMMIT_GATE_ENVIRON_PROBE='`)
	cmd.Env = append(os.Environ(), "COMMIT_GATE_ENVIRON_PROBE=1")
	return cmd.Run() == nil
}

// TestCommitGate_SkipPathRefusedInsideOpenCode pins the operator-only guard:
// SKIP_COMMIT_GATE is refused when OPENCODE_SESSION_ID is present in the
// script's initial environment, even with the bypass flag set.
func TestCommitGate_SkipPathRefusedInsideOpenCode(t *testing.T) {
	if !environReflectsEnvp(t) {
		t.Skip("/proc/self/environ does not reflect exec-time envp in this environment " +
			"(container runtime masks it); the OpenCode-context refusal is not observable here")
	}
	dir, dstScripts := skipGateSetup(t)
	env := append(skipGateTestEnv(), "OPENCODE_SESSION_ID=skip-test-session")

	combined, err := skipGateRun(t, dir, dstScripts, env, "acquire")
	if err == nil {
		t.Fatalf("SKIP acquire inside OpenCode must be refused, got success:\n%s", combined)
	}
	parsed := skipGateStatus(t, combined)
	if status, _ := parsed["status"].(string); status != "error" {
		t.Fatalf("expected status error for in-OpenCode SKIP acquire, got %v\n%s", parsed, combined)
	}
	if reason, _ := parsed["reason"].(string); reason != "skip_gate_refused" {
		t.Fatalf("expected reason skip_gate_refused, got %v\n%s", parsed, combined)
	}
}
