package shell

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestOutputTruncationAtCapWithMarker: output past MaxCapturedBytes is
// dropped, counted, and marked IN-BAND; the in-memory capture never
// exceeds the cap.
func TestOutputTruncationAtCapWithMarker(t *testing.T) {
	cfg := Config{MaxCapturedBytes: 1024}
	out := runQuick(t, cfg, "head -c 131072 /dev/zero | tr '\\0' 'a'", 10000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("command failed: %+v", out)
	}
	if !out.Truncated {
		t.Fatalf("truncated = false, want true")
	}
	if out.DroppedBytes != 131072-1024 {
		t.Fatalf("droppedBytes = %d, want %d", out.DroppedBytes, 131072-1024)
	}
	if !strings.HasSuffix(out.Stdout, fmt.Sprintf("\n[run_shell: output truncated, %d bytes dropped (cap 1024B)]", 131072-1024)) {
		t.Fatalf("stdout missing truncation marker; tail = %q", tail(out.Stdout, 120))
	}
	stored := len(out.Stdout) - len(fmt.Sprintf("\n[run_shell: output truncated, %d bytes dropped (cap 1024B)]", 131072-1024))
	if stored != 1024 {
		t.Fatalf("stored stdout payload = %d bytes, want exactly the 1024-byte cap", stored)
	}
	if strings.Count(out.Stdout[:stored], "a") != 1024 {
		t.Fatalf("stored payload is not the first 1024 'a's")
	}
}

// TestNoTruncationUnderCap: under-cap output passes through unmarked.
func TestNoTruncationUnderCap(t *testing.T) {
	out := runQuick(t, Config{}, "printf 0123456789", 5000)
	if out.Truncated || out.DroppedBytes != 0 || strings.Contains(out.Stdout, "[run_shell: output truncated") {
		t.Fatalf("under-cap output altered: %+v stdout=%q", out, out.Stdout)
	}
	if out.Stdout != "0123456789" {
		t.Fatalf("stdout = %q", out.Stdout)
	}
}

// TestStderrTruncatedIndependently: each stream gets its own budget —
// a chatty stderr cannot eat stdout's cap.
func TestStderrTruncatedIndependently(t *testing.T) {
	cfg := Config{MaxCapturedBytes: 256}
	out := runQuick(t, cfg, "printf 'out-ok'; head -c 4096 /dev/zero | tr '\\0' 'b' 1>&2", 10000)
	if out.Cause != CauseExit {
		t.Fatalf("failed: %+v", out)
	}
	if out.Stdout != "out-ok" {
		t.Fatalf("stdout eaten by stderr truncation: %q", out.Stdout)
	}
	if !out.Truncated || out.DroppedBytes != 4096-256 {
		t.Fatalf("stderr truncation wrong: truncated=%v dropped=%d", out.Truncated, out.DroppedBytes)
	}
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// TestSandboxFuncRewrapsCommand: the seam can transform the constructed
// command in place (here: replace the bash invocation with a direct
// printf), and the outcome records the configured sandbox name.
func TestSandboxFuncRewrapsCommand(t *testing.T) {
	cfg := Config{
		SandboxName: "test-rewrap",
		Sandbox: func(cmd *exec.Cmd) error {
			printfPath, err := exec.LookPath("printf")
			if err != nil {
				return err
			}
			cmd.Path = printfPath
			cmd.Args = []string{"printf", "SANDBOXED:%s", cmd.Args[len(cmd.Args)-1]}
			return nil
		},
	}
	out := runQuick(t, cfg, "original-command", 5000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("sandboxed run failed: %+v", out)
	}
	if out.Stdout != "SANDBOXED:original-command" {
		t.Fatalf("stdout = %q, want the sandbox rewrap to take effect", out.Stdout)
	}
	if out.Sandbox != "test-rewrap" {
		t.Fatalf("sandbox = %q, want test-rewrap", out.Sandbox)
	}
}

// TestSandboxFailureNeverRuns: a sandbox that refuses the run
// classifies CauseError and the command never executes.
func TestSandboxFailureNeverRuns(t *testing.T) {
	cfg := Config{
		SandboxName: "refusing",
		Sandbox: func(cmd *exec.Cmd) error {
			return fmt.Errorf("confinement unavailable")
		},
	}
	out := runQuick(t, cfg, "true", 5000)
	if out.Cause != CauseError {
		t.Fatalf("cause = %q, want error", out.Cause)
	}
	if !strings.Contains(out.SpawnError, "sandbox") || !strings.Contains(out.SpawnError, "confinement unavailable") {
		t.Fatalf("spawnError = %q, want sandbox refusal provenance", out.SpawnError)
	}
	if out.TimedOut || out.ExitCode != 0 {
		t.Fatalf("foreign facts on sandbox refusal: %+v", out)
	}
}

// TestDefaultSandboxNoneRecorded: nil Sandbox means NO confinement and
// every outcome says so ("none") — the loud default.
func TestDefaultSandboxNoneRecorded(t *testing.T) {
	out := runQuick(t, Config{}, "true", 5000)
	if out.Sandbox != "none" {
		t.Fatalf("sandbox = %q, want none", out.Sandbox)
	}
}

// TestCommandPolicyLists: coarse allow/deny lists (empty = allow; deny
// wins; whole-word prefix matching).
func TestCommandPolicyLists(t *testing.T) {
	cfg := Config{DeniedCommands: []string{"rm -rf"}}
	cfg.normalize()

	if ok, _ := policyAllows(&cfg, "rm -rf /tmp/x"); ok {
		t.Fatalf("denylist entry must deny the whole-word-prefix match")
	}
	if ok, _ := policyAllows(&cfg, "rm -rfx /tmp"); !ok {
		t.Fatalf("a command that merely shares a prefix must NOT be denied (whole-word matching)")
	}
	if ok, _ := policyAllows(&cfg, "ls -la"); !ok {
		t.Fatalf("empty allowlist must allow")
	}

	allow := Config{AllowedCommands: []string{"go test", "ls"}}
	allow.normalize()
	if ok, _ := policyAllows(&allow, "go test ./..."); !ok {
		t.Fatalf("allowlist prefix match failed")
	}
	if ok, _ := policyAllows(&allow, "curl evil.example"); ok {
		t.Fatalf("non-allowlisted command must be refused when an allowlist exists")
	}
	allow.DeniedCommands = []string{"go test ./internal/private"}
	if ok, _ := policyAllows(&allow, "go test ./internal/private"); ok {
		t.Fatalf("denylist must win over the allowlist")
	}
}

// TestResolveTimeoutMapping: omitted/0 → default; negative → invalid;
// over-cap → clamped.
func TestResolveTimeoutMapping(t *testing.T) {
	cfg := Config{} // defaults: 30s default, 600s cap
	cfg.normalize()

	got, err := resolveTimeout(&cfg, nil)
	if err != nil || got != 30000 {
		t.Fatalf("nil timeout = %d, %v; want default 30000", got, err)
	}
	zero := int64(0)
	if got, _ := resolveTimeout(&cfg, &zero); got != 30000 {
		t.Fatalf("explicit 0 = %d; want default", got)
	}
	neg := int64(-5)
	if _, err := resolveTimeout(&cfg, &neg); err == nil {
		t.Fatalf("negative timeout must be invalid")
	}
	over := int64(10_000_000)
	if got, _ := resolveTimeout(&cfg, &over); got != 600000 {
		t.Fatalf("over-cap timeout = %d; want clamp to 600000", got)
	}
	five := int64(5000)
	if got, _ := resolveTimeout(&cfg, &five); got != 5000 {
		t.Fatalf("in-range timeout = %d; want passthrough", got)
	}
}

// TestExecuteInvalidArgs: argument validation failures are typed errors
// (the Pipeline will normalize them into isError results).
func TestExecuteInvalidArgs(t *testing.T) {
	cfg := Config{}
	cfg.normalize()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := execute(ctx, &cfg, nil); err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("empty args: err = %v", err)
	}
	if _, err := execute(ctx, &cfg, []byte(`{"command":""}`)); err == nil {
		t.Fatalf("empty command must fail")
	}
	if _, err := execute(ctx, &cfg, []byte(`{"command":"true","surprise":1}`)); err == nil {
		t.Fatalf("unknown arg fields must fail (DisallowUnknownFields)")
	}
	if _, err := execute(ctx, &cfg, []byte(`{"command":"true","timeout_ms":-1}`)); err == nil {
		t.Fatalf("negative timeout_ms must fail")
	}
	if _, err := execute(ctx, &cfg, []byte(`{"command":"true","workdir":"/nonexistent/dir/xyz"}`)); err == nil {
		t.Fatalf("missing workdir must fail")
	}
}
