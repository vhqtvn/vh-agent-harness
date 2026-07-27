package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckComplexityAdvisory_WarningOnly is the SACRED INVARIANT test for the
// complexity doctor check: it MUST be structurally incapable of returning
// tierFail. No threshold breach, malformed policy, or scanner error may
// increment the problem count or authorize a transition. This is the authority
// line — WARN never becomes FAIL.
func TestCheckComplexityAdvisory_WarningOnly(t *testing.T) {
	cases := []struct {
		name         string
		setup        func(t *testing.T, dir string)
		wantTier     string
		wantInDetail string
	}{
		{
			name: "absent-policy-skips",
			setup: func(t *testing.T, dir string) {
				// No policy file: greenfield, SKIP.
			},
			wantTier: tierSkip,
		},
		{
			name: "disabled-policy-skips",
			setup: func(t *testing.T, dir string) {
				writePolicy(t, dir, "version: 1\nenabled: false\ndefaults:\n  event_file_lines: 350\n  snapshot_file_lines: 500\n")
			},
			wantTier: tierSkip,
		},
		{
			name: "no-candidates-passes",
			setup: func(t *testing.T, dir string) {
				writePolicy(t, dir, defaultPolicyYAML)
				// A small file under the threshold: no candidates.
				os.MkdirAll(filepath.Join(dir, "src"), 0755)
				os.WriteFile(filepath.Join(dir, "src", "small.go"), []byte("package src\n"), 0644)
			},
			wantTier: tierPass,
		},
		{
			name: "candidates-warn-never-fail",
			setup: func(t *testing.T, dir string) {
				writePolicy(t, dir, defaultPolicyYAML)
				// A large file over the threshold: should WARN, not FAIL.
				os.MkdirAll(filepath.Join(dir, "internal", "big"), 0755)
				os.WriteFile(filepath.Join(dir, "internal", "big", "huge.go"), []byte(makeLines(600)), 0644)
			},
			wantTier:     tierWarn,
			wantInDetail: "huge.go",
		},
		{
			name: "malformed-policy-warns-not-fails",
			setup: func(t *testing.T, dir string) {
				// A garbage policy: WARN (armed-schema owns the FAIL), not FAIL.
				writePolicy(t, dir, "this: is: not: valid: yaml: {{{\n")
			},
			wantTier: tierWarn,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			c.setup(t, dir)
			// Init a git repo so the scanner uses git ls-files (tracked-only).
			initGitRepo(t, dir)
			res := checkComplexityAdvisory(dir)
			if res.tier != c.wantTier {
				t.Fatalf("tier: got %q want %q (detail: %s)", res.tier, c.wantTier, res.detail)
			}
			// SACRED INVARIANT: NEVER tierFail, regardless of input.
			if res.tier == tierFail {
				t.Fatalf("SACRED INVARIANT VIOLATION: complexity advisory returned tierFail (detail: %s)", res.detail)
			}
			if c.wantInDetail != "" && !strings.Contains(res.detail, c.wantInDetail) {
				t.Fatalf("detail must contain %q; got: %s", c.wantInDetail, res.detail)
			}
		})
	}
}

// TestCheckComplexityAdvisory_NeverIncrementsProblems is the integration-level
// proof that the complexity advisory cannot make doctor UNHEALTHY: run applyTier
// on the result and assert problems stays 0.
func TestCheckComplexityAdvisory_NeverIncrementsProblems(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, defaultPolicyYAML)
	os.MkdirAll(filepath.Join(dir, "internal", "big"), 0755)
	os.WriteFile(filepath.Join(dir, "internal", "big", "huge.go"), []byte(makeLines(1000)), 0644)
	initGitRepo(t, dir)

	res := checkComplexityAdvisory(dir)
	problems, warns := 0, 0
	applyTier(res.tier, &problems, &warns)
	if problems != 0 {
		t.Fatalf("SACRED INVARIANT VIOLATION: complexity advisory incremented problems to %d", problems)
	}
	if warns != 1 {
		t.Fatalf("expected 1 warning from a nominated candidate, got %d (tier=%s detail=%s)", warns, res.tier, res.detail)
	}
}

const defaultPolicyYAML = `version: 1
enabled: true
defaults:
  event_file_lines: 350
  snapshot_file_lines: 500
per_language:
  ".go": {}
exclude:
  event_paths:
    - "tmp/**"
  snapshot_paths:
    - ".opencode/**"
  snapshot_suffixes:
    - "_test.go"
doctor:
  max_candidates: 10
recurrence:
  enabled: false
`

func writePolicy(t *testing.T, dir, content string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, ".vh-agent-harness"), 0755)
	if err := os.WriteFile(filepath.Join(dir, ".vh-agent-harness", "complexity-policy.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	// Best-effort git init + add so the scanner uses git ls-files (tracked-only).
	// If git is unavailable or not configured, the scanner falls back to WalkDir
	// and the test still holds (the warning-only invariant is independent of the
	// enumeration method).
	os.Setenv("GIT_AUTHOR_NAME", "test")
	os.Setenv("GIT_AUTHOR_EMAIL", "test@test")
	os.Setenv("GIT_COMMITTER_NAME", "test")
	os.Setenv("GIT_COMMITTER_EMAIL", "test@test")
	_ = exec.Command("git", "-C", dir, "init").Run()
	_ = exec.Command("git", "-C", dir, "add", "-A").Run()
	_ = exec.Command("git", "-C", dir, "commit", "-m", "init").Run()
}

func makeLines(n int) string {
	var b []byte
	for i := 0; i < n; i++ {
		b = append(b, "// line\n"...)
	}
	return string(b)
}
