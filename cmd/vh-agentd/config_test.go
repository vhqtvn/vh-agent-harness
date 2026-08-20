// config_test.go — the flag-parsing / validation matrix: every fail-closed
// startup refusal (missing required flags, unknown adapter, bad base URL,
// unset api-key env) exits 2 with an actionable message; --version reports
// the engine and protocol versions.
package main

import (
	"bytes"
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
		cfg, err := validate(in, "m", "http://x.test", "K", "d", 0, defaultApprovalTimeoutMs, 0)
		if err != nil || cfg.Adapter != want {
			t.Fatalf("alias %q: cfg=%+v err=%v, want adapter %q", in, cfg, err, want)
		}
	}
}

func TestValidateMaxTokensAndTimeoutBounds(t *testing.T) {
	if _, err := validate("openai", "m", "http://x.test", "K", "d", -1, defaultApprovalTimeoutMs, 0); err == nil {
		t.Fatal("negative max-tokens accepted")
	}
	if _, err := validate("openai", "m", "http://x.test", "K", "d", 0, -1, 0); err == nil {
		t.Fatal("negative approval-timeout-ms accepted")
	}
}

// TestValidateCacheBreakpoints covers the --cache-breakpoints surface:
// off by default, 1..4 accepted for anthropic, out-of-range rejected,
// and any explicit value on openai REJECTED with a clear error
// (OpenAI-compatible caching is implicit — no knob maps).
func TestValidateCacheBreakpoints(t *testing.T) {
	// Default off, both adapters.
	for _, adapter := range []string{"openai", "anthropic"} {
		cfg, err := validate(adapter, "m", "http://x.test", "K", "d", 0, defaultApprovalTimeoutMs, 0)
		if err != nil || cfg.CacheBreakpoints != 0 {
			t.Fatalf("adapter %s default: cfg=%+v err=%v, want breakpoints 0", adapter, cfg, err)
		}
	}
	// 1..4 accepted for anthropic and carried on the config.
	for n := 1; n <= 4; n++ {
		cfg, err := validate("anthropic", "m", "http://x.test", "K", "d", 0, defaultApprovalTimeoutMs, n)
		if err != nil || cfg.CacheBreakpoints != n {
			t.Fatalf("anthropic n=%d: cfg=%+v err=%v", n, cfg, err)
		}
	}
	// Out of range rejected.
	for _, n := range []int{-1, 5} {
		if _, err := validate("anthropic", "m", "http://x.test", "K", "d", 0, defaultApprovalTimeoutMs, n); err == nil {
			t.Fatalf("breakpoints %d accepted, want range rejection", n)
		}
	}
	// openai (and its alias) rejects any explicit budget.
	for _, adapter := range []string{"openai", "openaicompat"} {
		err := func() error {
			_, err := validate(adapter, "m", "http://x.test", "K", "d", 0, defaultApprovalTimeoutMs, 2)
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
