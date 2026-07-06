package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitGate_BacklogSplitPreflight is the O1 regression guard for the
// commit-gate.sh packaging-policy preflight (the W2/split-commit enforcement).
// The shared docs/planning/backlog.md ledger must NEVER travel in the same
// acquire as code/docs changes. This test black-boxes the RENDERED script
// (templates/core is the source; .opencode/scripts/commit-gate.sh is what
// ships) inside a fully-isolated temp git repo, so the assertion exercises the
// real guard end-to-end: config validation → path parse → preflight.
//
// Cases:
//   - mixed (backlog + code)              → nonzero exit, status backlog_must_commit_separately
//   - backlog-only                        → proceeds (status acquired)
//   - code-only (no backlog)              → proceeds (status acquired)
//   - mixed with ./ prefix bypass         → rejected (normalization catches leading ./)
//   - mixed with /../ collapse bypass     → rejected (normalization collapses .. segments)
//
// The temp repo gets its OWN .opencode/scripts/ copy + a minimal valid
// opencode.jsonc + one agents/*.md so validate-opencode-config.py (which
// resolves REPO_ROOT from the script's own location) passes against the temp
// tree. No real-repo state is mutated.
func TestCommitGate_BacklogSplitPreflight(t *testing.T) {
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

	runCase := func(t *testing.T, name string, paths []string, wantReject bool) {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()

			// Isolated .opencode/scripts/ in the temp repo.
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
			// Minimal valid opencode.jsonc (agent entry with non-empty description).
			if err := os.WriteFile(filepath.Join(dir, "opencode.jsonc"),
				[]byte(`{ "$schema": "https://opencode.ai/config.json", "agent": { "build": { "description": "test" } } }`),
				0o644); err != nil {
				t.Fatalf("write opencode.jsonc: %v", err)
			}
			// One valid agents/*.md (frontmatter with description + mode).
			agentsDir := filepath.Join(dir, ".opencode", "agents")
			if err := os.MkdirAll(agentsDir, 0o755); err != nil {
				t.Fatalf("mkdir agents: %v", err)
			}
			if err := os.WriteFile(filepath.Join(agentsDir, "build.md"),
				[]byte("---\ndescription: test\nmode: primary\n---\n# build\n"), 0o644); err != nil {
				t.Fatalf("write build.md: %v", err)
			}

			// git init + identity + initial commit so HEAD exists.
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
			}
			git("init", "-q")
			git("config", "user.email", "t@t")
			git("config", "user.name", "t")
			git("config", "commit.gpgsign", "false")

			// Seed the files the cases reference, commit them at HEAD, then modify
			// so the "proceeds" cases have a real change to stage.
			writeAndCommit := func(rel, body string) {
				t.Helper()
				full := filepath.Join(dir, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", rel, err)
				}
				if err := os.WriteFile(full, []byte(body+"-v1\n"), 0o644); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
				git("add", rel)
				git("commit", "-q", "-m", "seed "+rel)
				// Modify after commit so there is a working-tree change to acquire.
				if err := os.WriteFile(full, []byte(body+"-v2\n"), 0o644); err != nil {
					t.Fatalf("modify %s: %v", rel, err)
				}
			}
			for _, p := range paths {
				writeAndCommit(p, "// content for "+p)
			}

			// Message file (canonical form). The preflight runs after arg parsing
			// reads --message-file, so it must exist even for the reject case.
			msgFile := filepath.Join(dir, "msg.txt")
			if err := os.WriteFile(msgFile, []byte("test: preflight\n"), 0o644); err != nil {
				t.Fatalf("write msg: %v", err)
			}

			pathsJSON, err := json.Marshal(paths)
			if err != nil {
				t.Fatalf("marshal paths: %v", err)
			}

			cmd := exec.Command("bash", filepath.Join(dstScripts, "commit-gate.sh"),
				"acquire",
				"--paths", string(pathsJSON),
				"--message-file", msgFile,
				"--session-alias", "preflight-test")
			cmd.Dir = dir
			out, _ := cmd.CombinedOutput() // exit code decided below; capture both streams
			combined := string(out)
			exitCode := 0
			if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
				exitCode = cmd.ProcessState.ExitCode()
			}

			if wantReject {
				if exitCode == 0 {
					t.Fatalf("mixed path list must be REJECTED (nonzero exit); got exit 0\n%s", combined)
				}
				if !strings.Contains(combined, "backlog_must_commit_separately") {
					t.Errorf("reject JSON must carry backlog_must_commit_separately; got:\n%s", combined)
				}
				if !strings.Contains(combined, "must be committed separately from code/docs changes") {
					t.Errorf("teaching message must appear on stderr; got:\n%s", combined)
				}
				if !strings.Contains(combined, "Load the `backlog` skill") {
					t.Errorf("teaching message must point at the backlog skill; got:\n%s", combined)
				}
				return
			}

			// ALLOW path: expect status "acquired" (we modified the file post-commit).
			if exitCode != 0 {
				t.Fatalf("path list must PROCEED (exit 0); got exit %d\n%s", exitCode, combined)
			}
			if !strings.Contains(combined, `"status":"acquired"`) {
				t.Errorf("expected status acquired; got:\n%s", combined)
			}
		})
	}

	runCase(t, "mixed_backlog_and_code_rejected",
		[]string{"docs/planning/backlog.md", "internal/foo.go"}, true)
	runCase(t, "backlog_only_proceeds",
		[]string{"docs/planning/backlog.md"}, false)
	runCase(t, "code_only_proceeds",
		[]string{"internal/foo.go"}, false)
	// Non-canonical spellings must NOT bypass the guard (the path is
	// lexically normalized before the backlog comparison).
	runCase(t, "mixed_dot_slash_prefix_rejected",
		[]string{"./docs/planning/backlog.md", "internal/foo.go"}, true)
	runCase(t, "mixed_dotdot_collapse_rejected",
		[]string{"docs/planning/../planning/backlog.md", "README.md"}, true)
}
