package shell

import (
	"context"
	"strings"
	"testing"
	"time"
)

// childEnvLines runs `printenv` through the tool and returns the
// child's environment as name=value lines (sorted for stability).
func childEnvLines(t *testing.T, cfg Config) []string {
	t.Helper()
	out := runQuick(t, cfg, "printenv", 5000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("printenv failed: %+v", out)
	}
	lines := strings.Split(strings.TrimSpace(out.Stdout), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	return lines
}

func hasLine(t *testing.T, lines []string, want string) bool {
	t.Helper()
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

// TestEnvScrubDropsSensitiveNames: the dual scrub — names matching the
// dsh SENSITIVE_ENV_PATTERN (/KEY|PASSWORD|SECRET|TOKEN/i) and the
// engine-credential prefix are dropped even when EXPLICITLY
// allowlisted; the scrub wins over the allowlist.
func TestEnvScrubDropsSensitiveNames(t *testing.T) {
	t.Setenv("MY_API_KEY", "leak-me-not")
	t.Setenv("GITHUB_TOKEN", "leak-me-not")
	t.Setenv("SESSION_PASSWORD", "leak-me-not")
	t.Setenv("DEPLOY_SECRET", "leak-me-not")
	t.Setenv("VH_AGENT_HARNESS_JWT_SECRET", "leak-me-not")
	t.Setenv("CI", "true")

	cfg := Config{EnvAllowlist: []string{
		"MY_API_KEY", "GITHUB_TOKEN", "SESSION_PASSWORD", "DEPLOY_SECRET",
		"VH_AGENT_HARNESS_JWT_SECRET", // engine denylist must drop it too
		"CI",
	}}
	lines := childEnvLines(t, cfg)

	for _, name := range []string{"MY_API_KEY", "GITHUB_TOKEN", "SESSION_PASSWORD", "DEPLOY_SECRET", "VH_AGENT_HARNESS_JWT_SECRET"} {
		for _, l := range lines {
			if strings.HasPrefix(l, name+"=") {
				t.Fatalf("sensitive env %s leaked to the child: %q", name, l)
			}
		}
	}
	if !hasLine(t, lines, "CI=true") {
		t.Fatalf("allowlisted non-sensitive CI missing from child env: %v", lines)
	}
}

// TestEnvExplicitConstruction: the base env is built explicitly —
// parent vars outside the allowlist never reach the child (default
// deny: no BASH_ENV, no incidental state).
func TestEnvExplicitConstruction(t *testing.T) {
	t.Setenv("RUN_SHELL_TEST_UNLISTED", "must-not-leak")
	t.Setenv("BASH_ENV", "must-not-leak")
	t.Setenv("HISTFILE", "must-not-leak")
	t.Setenv("CI", "keep-me")

	lines := childEnvLines(t, Config{EnvAllowlist: []string{"CI"}})

	for _, l := range lines {
		for _, banned := range []string{"RUN_SHELL_TEST_UNLISTED", "BASH_ENV", "HISTFILE", "must-not-leak"} {
			if l == banned || strings.HasPrefix(l, banned+"=") {
				t.Fatalf("unlisted parent var reached the child: %q", l)
			}
		}
	}
	if !hasLine(t, lines, "CI=keep-me") {
		t.Fatalf("allowlisted CI missing: %v", lines)
	}
	if !hasLine(t, lines, "TERM=dumb") {
		t.Fatalf("TERM=dumb missing from child env: %v", lines)
	}
	if !hasLine(t, lines, "LANG=C.UTF-8") && !strings.HasPrefix(findPrefix(lines, "LANG="), "LANG=") {
		t.Fatalf("LANG missing from child env: %v", lines)
	}
}

func findPrefix(lines []string, prefix string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	return ""
}

// TestEnvBaseCarriesParentPathHome: PATH/HOME pass through from the
// parent (the child must still find utilities and home-relative
// tooling).
func TestEnvBaseCarriesParentPathHome(t *testing.T) {
	t.Setenv("HOME", "/definitely/a/home")
	cfg := Config{EnvAllowlist: []string{"SOME_UNSET_VAR"}}
	lines := childEnvLines(t, cfg)
	if !hasLine(t, lines, "HOME=/definitely/a/home") {
		t.Fatalf("HOME not passed through: %v", lines)
	}
	if findPrefix(lines, "PATH=") == "" {
		t.Fatalf("PATH missing: %v", lines)
	}
}

// TestIsSensitiveEnvName pins the scrub predicate directly, including
// the conservative direction (KEYSTONE-style false positives DROP).
func TestIsSensitiveEnvName(t *testing.T) {
	for _, name := range []string{"API_KEY", "api_key", "MyToken", "PASSWORD", "db_secret", "SSH_KEY", "KEYSTONE", "TOKENIZER"} {
		if !isSensitiveEnvName(name) {
			t.Errorf("isSensitiveEnvName(%q) = false, want true (conservative drop)", name)
		}
	}
	for _, name := range []string{"PATH", "HOME", "TERM", "LANG", "CI", "EDITOR"} {
		if isSensitiveEnvName(name) {
			t.Errorf("isSensitiveEnvName(%q) = true, want false", name)
		}
	}
	if !isSensitiveEnvName("VH_AGENT_HARNESS_ANYTHING") {
		t.Errorf("engine-credential prefix must always drop")
	}
}

// TestWorkdirApplies: the optional workdir is used (and validated up
// front).
func TestWorkdirApplies(t *testing.T) {
	cfg := Config{}
	cfg.normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dir := t.TempDir()
	out := run(ctx, &cfg, "pwd", 5000, dir)
	if out.Cause != CauseExit || !strings.HasPrefix(out.Stdout, dir) {
		t.Fatalf("pwd in workdir: %+v (stdout=%q)", out, out.Stdout)
	}
}
