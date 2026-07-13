package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitGate_DirectoryDeletionStaging is the regression guard for the
// `-r` flag on `git rm --cached` in cmd_acquire's staging loop. When a
// directory pathspec (e.g. "somedir") is passed via --paths and that
// directory has been deleted from the working tree but is still tracked at
// HEAD, the gate must stage the removal of every file under that prefix.
//
// Without `-r`, `git rm --cached -- <dir>` fatals with
// "not removing '<dir>' recursively without -r" (git exit 128) and acquire
// returns path_error/stage_remove_failed. With `-r`, the directory removal
// is staged and acquire returns "acquired". The `-r` flag is a no-op for
// single-file pathspecs, so existing single-file deletion behavior is
// preserved.
//
// This black-boxes the RENDERED .opencode/scripts/commit-gate.sh (NOT the
// template) inside an isolated scratch git repo, mirroring
// TestCommitGate_MessageCleanup's harness: isolated .opencode/scripts/ copy,
// minimal valid opencode.jsonc + one agents/*.md, git init + initial commit,
// drive the real gate, assert on the status JSON.
//
// Passing the DIRECTORY pathspec "somedir" (not the individual files) is what
// exercises the `-r` code path — two single-file pathspecs would each be
// handled by a separate loop iteration and never hit the recursive-removal
// refusal.
func TestCommitGate_DirectoryDeletionStaging(t *testing.T) {
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

	// Build an isolated scratch git repo with the rendered gate scripts +
	// minimal valid opencode config (so _config_validate passes) +
	// git init/identity.
	dir := t.TempDir()
	dstScripts := filepath.Join(dir, ".opencode", "scripts")
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

	// Seed a tracked directory with two files and commit at HEAD.
	subDir := filepath.Join(dir, "somedir")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir somedir: %v", err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(subDir, name), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	gitIn("add", "somedir")
	gitIn("commit", "-q", "-m", "seed dir")

	// Delete the entire directory from the working tree — the deletion vector
	// the gate must stage via `git rm --cached -r`.
	if err := os.RemoveAll(subDir); err != nil {
		t.Fatalf("remove somedir: %v", err)
	}

	// Author an agent message scratch file (acquire requires --message-file),
	// mirroring the committer's Write-tool path.
	uuidOut, err := exec.Command("bash", filepath.Join(dstScripts, "readonly-scripts.sh"), "gen-uuid").Output()
	if err != nil {
		t.Fatalf("gen-uuid: %v", err)
	}
	uuidA := strings.TrimSpace(string(uuidOut))
	msgRel := filepath.ToSlash(filepath.Join("tmp", "commit-gate-message", "msg-"+uuidA))
	msgFull := filepath.Join(dir, filepath.FromSlash(msgRel))
	if err := os.MkdirAll(filepath.Dir(msgFull), 0o755); err != nil {
		t.Fatalf("mkdir scratch: %v", err)
	}
	if err := os.WriteFile(msgFull, []byte("remove somedir\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}

	// Drive the real acquire with the DIRECTORY pathspec "somedir".
	cmd := exec.Command("bash", filepath.Join(dstScripts, "commit-gate.sh"), "acquire",
		"--paths", `["somedir"]`,
		"--message-file", msgRel,
		"--session-alias", "dir-removal-test")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	combined := string(out)

	// Find the last valid JSON object carrying a "status" key (the json_out
	// line is printed last; config-validation output, if any, precedes it).
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
	// Without the `-r` fix, `git rm --cached -- somedir` fatals (exit 128) and
	// the gate returns path_error/stage_remove_failed with a non-zero exit, so
	// err != nil here is the regression signal.
	if err != nil {
		t.Fatalf("acquire with directory pathspec failed (expected to succeed with the -r fix): %v\n%s", err, combined)
	}
	if parsed == nil {
		t.Fatalf("acquire produced no status JSON\n%s", combined)
	}
	if status, _ := parsed["status"].(string); status != "acquired" {
		t.Fatalf("expected status acquired for directory deletion staging, got %v\n%s", parsed, combined)
	}
}
