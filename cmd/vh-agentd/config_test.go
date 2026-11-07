// config_test.go — the flag-parsing / validation matrix: every fail-closed
// startup refusal (missing required flags, unknown adapter, bad base URL,
// unset api-key env) exits 2 with an actionable message; --version reports
// the engine and protocol versions.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runArgs(t *testing.T, args []string, env map[string]string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	getenv := func(k string) string { return env[k] }
	code := run(args, getenv, nil, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestRunMissingSessionDirExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://x.test", "--api-key-env", "K",
	}, map[string]string{"K": "v"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "--session-dir is required") {
		t.Fatalf("stderr missing fail-closed message: %q", stderr)
	}
	if !strings.Contains(stderr, "no default") {
		t.Fatalf("stderr missing no-silent-default rationale: %q", stderr)
	}
}

func TestRunUnknownAdapterExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{
		"--adapter", "grok", "--model", "m", "--base-url", "http://x.test",
		"--api-key-env", "K", "--session-dir", t.TempDir(),
	}, map[string]string{"K": "v"})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, `invalid --adapter "grok"`) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestRunAPIKeyEnvUnsetExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{
		"--adapter", "anthropic", "--model", "m", "--base-url", "http://x.test",
		"--api-key-env", "VH_AGENTD_MISSING_KEY", "--session-dir", t.TempDir(),
	}, map[string]string{})
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "VH_AGENTD_MISSING_KEY is not set") {
		t.Fatalf("stderr missing fail-closed credential message: %q", stderr)
	}
	if !strings.Contains(stderr, "fail-closed") {
		t.Fatalf("stderr should name the posture: %q", stderr)
	}
}

func TestRunUnknownFlagExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{"--nope"}, nil)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if stderr == "" {
		t.Fatal("stderr empty for unknown flag")
	}
}

func TestRunMissingEachRequiredFlagExits2(t *testing.T) {
	base := map[string]string{"K": "v"}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"adapter", []string{"--model", "m", "--base-url", "http://x.test", "--api-key-env", "K", "--session-dir", "d"}, "invalid --adapter"},
		{"model", []string{"--adapter", "openai", "--base-url", "http://x.test", "--api-key-env", "K", "--session-dir", "d"}, "--model is required"},
		{"base-url", []string{"--adapter", "openai", "--model", "m", "--api-key-env", "K", "--session-dir", "d"}, "--base-url is required"},
		{"api-key-env", []string{"--adapter", "openai", "--model", "m", "--base-url", "http://x.test", "--session-dir", "d"}, "--api-key-env is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := runArgs(t, tc.args, base)
			if code != 2 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("exit=%d stderr=%q, want 2 + %q", code, stderr, tc.want)
			}
		})
	}
}

func TestRunInvalidBaseURLExits2(t *testing.T) {
	for _, bad := range []string{"not-a-url", "ftp://x.test", "http://"} {
		code, _, stderr := runArgs(t, []string{
			"--adapter", "openai", "--model", "m", "--base-url", bad,
			"--api-key-env", "K", "--session-dir", t.TempDir(),
		}, map[string]string{"K": "v"})
		if code != 2 || !strings.Contains(stderr, "invalid --base-url") {
			t.Fatalf("base-url %q: exit=%d stderr=%q", bad, code, stderr)
		}
	}
}

func TestRunInvalidAPIKeyEnvNameExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://x.test",
		"--api-key-env", "9BAD-NAME", "--session-dir", t.TempDir(),
	}, map[string]string{"9BAD-NAME": "v"})
	if code != 2 || !strings.Contains(stderr, "not a valid environment variable name") {
		t.Fatalf("exit=%d stderr=%q", code, stderr)
	}
}

// TestRunCompactThresholdNaNExits2: Go's flag parser accepts "NaN" for
// a float64 flag (strconv.ParseFloat), so a literal --compact-threshold
// NaN reaches validation as a real NaN — and NaN slips both sides of a
// `<0 || >1` range check. The binary-level refusal must be the explicit
// non-finite guard on the same exit-2 path as sibling flags. The env
// key is deliberately NOT set: validateCompaction runs BEFORE the
// credential check, so the finite-ratio message on stderr proves the
// guard fired (with the bug, this same run exits 2 at the credential
// check with the wrong message — no serving, no hang).
func TestRunCompactThresholdNaNExits2(t *testing.T) {
	code, _, stderr := runArgs(t, []string{
		"--adapter", "openai", "--model", "m", "--base-url", "http://x.test",
		"--api-key-env", "K", "--session-dir", t.TempDir(),
		"--compact-threshold", "NaN",
	}, map[string]string{})
	if code != 2 || !strings.Contains(stderr, "--compact-threshold") || !strings.Contains(stderr, "finite") {
		t.Fatalf("compact-threshold NaN: exit=%d stderr=%q, want 2 + finite-ratio rejection", code, stderr)
	}
}

// TestRunRelativeSessionDirExits2: a relative --session-dir is rejected
// at validation (fail-loud). It would otherwise be wired into the
// workspace-write Landlock RWDirs and resolve against the sandboxed
// child's working directory — not the daemon's startup cwd — denying
// session writes against an unintended root (confusing
// fails-toward-denial instead of a clear config error).
func TestRunRelativeSessionDirExits2(t *testing.T) {
	for _, rel := range []string{"d", "sessions", "./sessions", "../sessions"} {
		code, _, stderr := runArgs(t, []string{
			"--adapter", "openai", "--model", "m", "--base-url", "http://x.test",
			"--api-key-env", "K", "--session-dir", rel,
		}, map[string]string{"K": "v"})
		if code != 2 || !strings.Contains(stderr, "must be an absolute path") {
			t.Fatalf("relative session-dir %q: exit=%d stderr=%q, want 2 + absolute-path error", rel, code, stderr)
		}
	}
}

// TestRunWorkdirRootsNotDirectoryExits2: --workdir-roots is documented
// as existing DIRECTORIES. A regular file — directly or through a
// symlink — must refuse at startup (exit 2) naming the path, not pass
// validation and surface later as every relative resolution targeting
// a non-directory.
func TestRunWorkdirRootsNotDirectoryExits2(t *testing.T) {
	dir := t.TempDir()
	fileRoot := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(fileRoot, []byte("file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkToFile := filepath.Join(dir, "link-to-file")
	if err := os.Symlink(fileRoot, linkToFile); err != nil {
		t.Fatal(err)
	}

	for _, root := range []string{fileRoot, linkToFile} {
		code, _, stderr := runArgs(t, []string{
			"--adapter", "openai", "--model", "m", "--base-url", "http://x.test",
			"--api-key-env", "K", "--session-dir", t.TempDir(),
			"--workdir-roots", root,
		}, map[string]string{"K": "v"})
		if code != 2 || !strings.Contains(stderr, "not a directory") || !strings.Contains(stderr, root) {
			t.Fatalf("workdir root %q: exit=%d stderr=%q, want 2 + not-a-directory error naming the path", root, code, stderr)
		}
	}
}

// TestParseWorkdirRootsSymlinkedDirectoryAdmitted: the directory check
// is on directory-ness, never on symlinks — a symlink TO a directory
// stays admitted and canonicalizes to its target.
func TestParseWorkdirRootsSymlinkedDirectoryAdmitted(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link-to-dir")
	if err := os.Symlink(sub, link); err != nil {
		t.Fatal(err)
	}

	roots, err := parseWorkdirRoots(link)
	if err != nil {
		t.Fatalf("symlinked directory root must be admitted: %v", err)
	}
	if len(roots) != 1 || roots[0] != sub {
		t.Fatalf("roots = %v, want the resolved directory [%s]", roots, sub)
	}
}

func TestRunVersionPrintsEngineAndProtocol(t *testing.T) {
	code, out, _ := runArgs(t, []string{"--version"}, nil)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(out, engineVersion) {
		t.Fatalf("stdout missing engine version: %q", out)
	}
	if !strings.Contains(out, "protocol 1") {
		t.Fatalf("stdout missing protocol version: %q", out)
	}
}

func TestRunHelpExits0(t *testing.T) {
	code, _, stderr := runArgs(t, []string{"--help"}, nil)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !strings.Contains(stderr, "api-key-env") || !strings.Contains(stderr, "session-dir") {
		t.Fatalf("usage missing flags: %q", stderr)
	}
}

func TestUsageDocumentsCredentialHandling(t *testing.T) {
	if !strings.Contains(usageDoc, "environment variable NAMED by\n  --api-key-env") &&
		!strings.Contains(usageDoc, "--api-key-env") {
		t.Fatal("usageDoc does not mention api-key-env")
	}
	for _, want := range []string{"never written to session logs", "adapter memory only"} {
		if !strings.Contains(usageDoc, want) {
			t.Fatalf("usageDoc missing credential note %q", want)
		}
	}
}

func TestValidateAdapterAliases(t *testing.T) {
	for in, want := range map[string]string{"openai": "openaicompat", "OpenAICompat": "openaicompat", "anthropic": "anthropic"} {
		cfg, err := validate(in, "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
		if err != nil || cfg.Adapter != want {
			t.Fatalf("alias %q: cfg=%+v err=%v, want adapter %q", in, cfg, err, want)
		}
	}
}

func TestValidateMaxTokensAndTimeoutBounds(t *testing.T) {
	// Absolute session dir so the maxTokens<0 branch is actually
	// reached: the previous relative "d" tripped the absolute-path
	// check first and the subtest passed VACUOUSLY (deferred F1 fix).
	if _, err := validate("openai", "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", -1, defaultApprovalTimeoutMs, 0, "off", 65536, ""); err == nil || !strings.Contains(err.Error(), "--max-tokens") || !strings.Contains(err.Error(), "must be >= 0") {
		t.Fatalf("negative max-tokens: err = %v, want the --max-tokens >= 0 rejection (not the session-dir error)", err)
	}
	if _, err := validate("openai", "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", 0, -1, 0, "off", 65536, ""); err == nil {
		t.Fatal("negative approval-timeout-ms accepted")
	}
}

// TestValidateOptimizerFlag covers the --optimizer surface: empty (the
// default) and case-insensitive llm/dedup normalize onto the config;
// anything else is a clear validation failure.
func TestValidateOptimizerFlag(t *testing.T) {
	for in, want := range map[string]string{"": "llm", "llm": "llm", "LLM": "llm", " llm ": "llm", "dedup": "dedup", "Dedup": "dedup"} {
		cfg, err := validate("openai", "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", in, 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
		if err != nil || cfg.Optimizer != want {
			t.Fatalf("optimizer %q: cfg=%+v err=%v, want %q", in, cfg, err, want)
		}
	}
	for _, bad := range []string{"bogus", "openai", "dedupe"} {
		_, err := validate("openai", "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", bad, 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
		if err == nil || !strings.Contains(err.Error(), "--optimizer") {
			t.Fatalf("optimizer %q: err = %v, want a clear --optimizer rejection", bad, err)
		}
	}
}

// TestValidateCacheBreakpoints covers the --cache-breakpoints surface:
// off by default, 1..4 accepted for anthropic, out-of-range rejected,
// and any explicit value on openai REJECTED with a clear error
// (OpenAI-compatible caching is implicit — no knob maps).
func TestValidateCacheBreakpoints(t *testing.T) {
	// Default off, both adapters.
	for _, adapter := range []string{"openai", "anthropic"} {
		cfg, err := validate(adapter, "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", 0, defaultApprovalTimeoutMs, 0, "off", 65536, "")
		if err != nil || cfg.CacheBreakpoints != 0 {
			t.Fatalf("adapter %s default: cfg=%+v err=%v, want breakpoints 0", adapter, cfg, err)
		}
	}
	// 1..4 accepted for anthropic and carried on the config.
	for n := 1; n <= 4; n++ {
		cfg, err := validate("anthropic", "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", 0, defaultApprovalTimeoutMs, n, "off", 65536, "")
		if err != nil || cfg.CacheBreakpoints != n {
			t.Fatalf("anthropic n=%d: cfg=%+v err=%v", n, cfg, err)
		}
	}
	// Out of range rejected.
	for _, n := range []int{-1, 5} {
		if _, err := validate("anthropic", "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", 0, defaultApprovalTimeoutMs, n, "off", 65536, ""); err == nil {
			t.Fatalf("breakpoints %d accepted, want range rejection", n)
		}
	}
	// openai (and its alias) rejects any explicit budget.
	for _, adapter := range []string{"openai", "openaicompat"} {
		err := func() error {
			_, err := validate(adapter, "m", "http://x.test", "K", "/tmp/vh-agentd-test-sessions", "", 0, defaultApprovalTimeoutMs, 2, "off", 65536, "")
			return err
		}()
		if err == nil {
			t.Fatalf("adapter %s accepted --cache-breakpoints, want rejection", adapter)
		}
		if !strings.Contains(err.Error(), "not supported") || !strings.Contains(err.Error(), "implicitly") {
			t.Fatalf("adapter %s rejection not clear: %v", adapter, err)
		}
	}
}
