package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCommitGate_RewriteParityStage1 black-boxes the OPT-D Stage 1 rewrite-
// parity mechanical precheck against the RENDERED .opencode/scripts/commit-
// gate.sh + rewrite-parity-validate.py in an isolated scratch git repo.
//
// The gate is OPT-IN: --rewrite-parity-contract is absent for ordinary
// deletes (zero burden, Case 1) and present only for an explicitly-declared
// deletion/rewrite slice. When present, the contract must be structurally
// valid, its prior_surface.revision must equal head_at_acquire, and its
// prior_surface.paths must cross-check against the tree-bound acquire diff.
//
// This mirrors TestCommitGate_DirectoryDeletionStaging's harness: isolated
// .opencode/scripts/ copy (including the validator), minimal valid
// opencode.jsonc + one agents/*.md, git init/identity, drive the real gate,
// assert on the status JSON.
func TestCommitGate_RewriteParityStage1(t *testing.T) {
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
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py", "rewrite-parity-validate.py"} {
		if _, err := os.Stat(filepath.Join(srcScripts, f)); err != nil {
			t.Skipf("rendered script %s unavailable: %v (run `vh-agent-harness update` first)", f, err)
		}
	}

	dir := t.TempDir()
	dstScripts := filepath.Join(dir, ".opencode", "scripts")
	if err := os.MkdirAll(dstScripts, 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	for _, f := range []string{"commit-gate.sh", "readonly-scripts.sh", "validate-opencode-config.py", "rewrite-parity-validate.py"} {
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

	// Seed two tracked files; only fileA.txt will be deleted.
	if err := os.WriteFile(filepath.Join(dir, "fileA.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write fileA: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fileB.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("write fileB: %v", err)
	}
	gitIn("add", "fileA.txt", "fileB.txt")
	gitIn("commit", "-q", "-m", "seed")

	head, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	headSHA := strings.TrimSpace(string(head))

	// Delete fileA.txt from the working tree (the deletion the gate will stage).
	if err := os.Remove(filepath.Join(dir, "fileA.txt")); err != nil {
		t.Fatalf("remove fileA: %v", err)
	}

	// Author a message scratch file (acquire requires --message-file).
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
	if err := os.WriteFile(msgFull, []byte("remove fileA\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}

	// runAcquire drives acquire with the given (optional) contract file and
	// returns the last parsed status JSON object.
	runAcquire := func(contractFile string) map[string]any {
		t.Helper()
		args := []string{filepath.Join(dstScripts, "commit-gate.sh"), "acquire",
			"--paths", `["fileA.txt"]`,
			"--message-file", msgRel,
			"--session-alias", "rewrite-parity-test"}
		if contractFile != "" {
			args = append(args, "--rewrite-parity-contract", contractFile)
		}
		cmd := exec.Command("bash", args...)
		cmd.Dir = dir
		out, _ := cmd.CombinedOutput()
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
		if parsed == nil {
			t.Fatalf("acquire produced no status JSON\n%s", combined)
		}
		return parsed
	}

	// writeContract writes a contract JSON file in the temp dir and returns its path.
	writeContract := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write contract %s: %v", name, err)
		}
		return p
	}

	// Case 1: NO contract flag => zero burden, ordinary delete acquires.
	if got := statusOf(runAcquire("")); got != "acquired" {
		t.Fatalf("Case 1 (no flag): expected acquired, got %q", got)
	}

	// Case 2: valid contract (paths match the deletion, revision matches HEAD).
	valid := writeContract("valid.json", fmt.Sprintf(`{
  "version": 1,
  "applies": "fileA deletion-replacement (test)",
  "mode": "deletion_replacement",
  "prior_surface": {
    "id": "fileA", "revision": "%s",
    "paths": ["fileA.txt"], "inventory_complete": true
  },
  "behaviors": [
    {"id": "a", "description": "fileA behavior preserved",
     "prior_evidence": ["fileA.txt"],
     "verifier": {"kind": "smoke", "locator": "true"},
     "result": {"status": "planned"}}
  ]
}`, headSHA))
	if got := statusOf(runAcquire(valid)); got != "acquired" {
		t.Fatalf("Case 2 (valid contract): expected acquired, got %q", got)
	}

	// Case 3: revision mismatch => rewrite_parity_error.
	badRev := writeContract("badrev.json", fmt.Sprintf(`{
  "version": 1, "applies": "rev mismatch", "mode": "deletion_replacement",
  "prior_surface": {"id": "f", "revision": "ffffffffffffffffffffffffffffffffffffffff",
    "paths": ["fileA.txt"], "inventory_complete": true},
  "behaviors": [{"id": "a", "description": "d", "prior_evidence": ["e"],
    "verifier": {"kind": "smoke", "locator": "true"}, "result": {"status": "planned"}}]
}`))
	if got := statusOf(runAcquire(badRev)); got != "rewrite_parity_error" {
		t.Fatalf("Case 3 (revision mismatch): expected rewrite_parity_error, got %q", got)
	}

	// Case 4: declared path not in the deletion set => rewrite_parity_error.
	badPath := writeContract("badpath.json", fmt.Sprintf(`{
  "version": 1, "applies": "path mismatch", "mode": "deletion_replacement",
  "prior_surface": {"id": "f", "revision": "%s",
    "paths": ["nonexistent.go"], "inventory_complete": true},
  "behaviors": [{"id": "a", "description": "d", "prior_evidence": ["e"],
    "verifier": {"kind": "smoke", "locator": "true"}, "result": {"status": "planned"}}]
}`, headSHA))
	if got := statusOf(runAcquire(badPath)); got != "rewrite_parity_error" {
		t.Fatalf("Case 4 (path not deleted): expected rewrite_parity_error, got %q", got)
	}

	// Case 5: malformed JSON => rewrite_parity_error.
	malformed := writeContract("malformed.json", `{not valid json`)
	if got := statusOf(runAcquire(malformed)); got != "rewrite_parity_error" {
		t.Fatalf("Case 5 (malformed): expected rewrite_parity_error, got %q", got)
	}
}

// status is a small helper extracting the "status" string from a parsed gate
// JSON object.
func statusOf(m map[string]any) string { s, _ := m["status"].(string); return s }
