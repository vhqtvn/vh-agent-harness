package execsandbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/execro"
)

// envPrefix is used for all profile-serialization env vars passed parent→child.
const envPrefix = "VH_EXEC_SANDBOX_"

// LoudGracefulSkipWarning mirrors internal/runtime/bare.go's
// bareNoIsolationWarning style. It is printed to stderr when best-effort mode
// cannot obtain OS primitives and falls back to exec-ro classification.
const LoudGracefulSkipWarning = "WARNING: exec-sandbox OS primitives unavailable (landlock/seccomp); " +
	"falling back to exec-ro classification level. This is NOT kernel-enforced isolation."

// AskNonTTYNotice is printed when --net=ask is used in a non-interactive session.
const AskNonTTYNotice = "exec-sandbox: network requested (--net=ask) but session is non-interactive. " +
	"Rerun with explicit --net=allow or --net=deny."

// Run executes the command according to the sandbox mode and profile.
//
// Decision tree:
//  1. ModeOff → run directly (no sandbox, no GIT_OPTIONAL_LOCKS rewrite).
//  2. Resolve --net=ask (TTY prompt or non-TTY hard-deny).
//  3. Detect OS primitives. If both available → full trampoline sandbox.
//  4. Primitives unavailable + strict → fail-closed (return error).
//  5. Primitives unavailable + best-effort → loud warn + exec-ro classify +
//     run if allowed, deny if not.
//
// Returns the child exit code and any setup/teardown error.
func Run(ctx context.Context, mode SandboxMode, profile Profile, repoRoot, target string, args []string) (int, error) {
	// ModeOff: no sandbox at all.
	if mode == ModeOff {
		return runDirect(ctx, target, args, nil)
	}

	// Resolve --net=ask before anything else (may exit the process).
	if profile.Net == NetAsk {
		resolved, err := resolveAskNet(os.Stdin, os.Stderr)
		if err != nil {
			// Non-TTY: hard-deny.
			fmt.Fprintln(os.Stderr, AskNonTTYNotice)
			return 1, err
		}
		profile.Net = resolved
	}

	features := Detect()

	// Both primitives available → full sandbox.
	if features.Available() {
		return runTrampoline(ctx, profile, repoRoot, target, args)
	}

	// Primitives unavailable.
	if mode == ModeStrict {
		return 1, fmt.Errorf(
			"strict sandbox mode requires OS primitives that are unavailable "+
				"(landlock=%v seccomp=%v); refusing to run without kernel-enforced isolation",
			features.Landlock, features.Seccomp,
		)
	}

	// Best-effort fallback: loud warning + exec-ro classifier (read-only import).
	// B1: classify the FULL argv (target + args), not just the bare executable.
	// Classify(target) would see only e.g. `find`, match the `find *` readonly
	// group with an empty arg tail, skip the per-binary write/exec flag scan, and
	// ALLOW — then the mutating args would execute unclassified via runDirect
	// with NO kernel isolation and NO exec-ro arg-level protection (reachable
	// bypass on non-Linux / kernels lacking landlock+seccomp). ClassifyArgs
	// preserves the real argv so `find . -delete` / `sort -o out in` /
	// `git diff --output=x` / `sed -n 'w /path' f` are correctly DENIED. Unit
	// regression: internal/execro/classifier_test.go TestClassifyArgs.
	fmt.Fprintln(os.Stderr, LoudGracefulSkipWarning)
	verdict := execro.ClassifyArgs(target, args, repoRoot)
	if !verdict.Allow {
		return 1, fmt.Errorf("command %q rejected by exec-ro classifier (sandbox unavailable): %s",
			target, verdict.Notice)
	}
	// exec-ro allows → run directly with GIT_OPTIONAL_LOCKS=0 (defensive).
	return runDirect(ctx, target, args, []string{"GIT_OPTIONAL_LOCKS=0"})
}

// resolveAskNet handles the --net=ask flow. When the session has a TTY it
// prints a [Y/n] prompt and blocks on stdin. Non-TTY returns an error (the
// caller prints AskNonTTYNotice and exits non-zero). Agents cannot auto-accept.
func resolveAskNet(in *os.File, out io.Writer) (NetPolicy, error) {
	if !isTTY(in) {
		return NetDeny, fmt.Errorf("non-interactive session cannot resolve --net=ask")
	}
	fmt.Fprint(out, "exec-sandbox: network requested (--net=ask). Allow? [Y/n] ")
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return NetDeny, fmt.Errorf("reading ask response: %w", err)
	}
	resp := strings.TrimSpace(strings.ToLower(line))
	switch resp {
	case "", "y", "yes":
		return NetAllow, nil
	default:
		return NetDeny, nil
	}
}

// isTTY reports whether the given file is a character device (terminal).
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// runDirect executes the command via os/exec with optional extra env vars.
// No sandbox, no landlock, no seccomp. Used for ModeOff and the exec-ro fallback.
func runDirect(ctx context.Context, target string, args []string, extraEnv []string) (int, error) {
	cmd := exec.CommandContext(ctx, target, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

// --- Profile serialization (parent → child env vars) ---

// profileToEnv serializes the profile into VH_EXEC_SANDBOX_* env entries.
func profileToEnv(p Profile) []string {
	return []string{
		envPrefix + "RO_DIRS=" + strings.Join(p.RODirs, string(os.PathListSeparator)),
		envPrefix + "RW_DIRS=" + strings.Join(p.RWDirs, string(os.PathListSeparator)),
		envPrefix + "RW_FILES=" + strings.Join(p.RWFiles, string(os.PathListSeparator)),
		envPrefix + "NET=" + string(p.Net),
	}
}

// profileFromEnv reconstructs the profile from VH_EXEC_SANDBOX_* env vars.
func profileFromEnv() (Profile, error) {
	var p Profile
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, envPrefix) {
			continue
		}
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		switch key {
		case envPrefix + "RO_DIRS":
			if val != "" {
				p.RODirs = strings.Split(val, string(os.PathListSeparator))
			}
		case envPrefix + "RW_DIRS":
			if val != "" {
				p.RWDirs = strings.Split(val, string(os.PathListSeparator))
			}
		case envPrefix + "RW_FILES":
			if val != "" {
				p.RWFiles = strings.Split(val, string(os.PathListSeparator))
			}
		case envPrefix + "NET":
			p.Net = NetPolicy(val)
		}
	}
	if len(p.RODirs) == 0 && len(p.RWDirs) == 0 {
		return p, fmt.Errorf("no sandbox profile found in environment (missing %s* vars)", envPrefix)
	}
	return p, nil
}

// envForTarget strips internal VH_EXEC_SANDBOX_* vars from the environment,
// producing a clean env for the target command. GIT_OPTIONAL_LOCKS=0 is
// preserved (it was set by the parent before fork).
func envForTarget(env []string) []string {
	var clean []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, envPrefix) {
			clean = append(clean, kv)
		}
	}
	return clean
}

// buildChildEnv constructs the child trampoline's environment: parent env +
// GIT_OPTIONAL_LOCKS=0 + serialized profile vars.
func buildChildEnv(profile Profile) []string {
	env := os.Environ()
	env = setEnvVar(env, "GIT_OPTIONAL_LOCKS", "0")
	// Remove any pre-existing VH_EXEC_SANDBOX_* to avoid leakage from parent env.
	env = envForTarget(env)
	env = append(env, profileToEnv(profile)...)
	return env
}

// setEnvVar sets or replaces a key in an env slice.
func setEnvVar(env []string, key, val string) []string {
	prefix := key + "="
	var result []string
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			result = append(result, prefix+val)
			found = true
		} else {
			result = append(result, kv)
		}
	}
	if !found {
		result = append(result, prefix+val)
	}
	return result
}

// exitCodeFromWait extracts the exit code from a Wait error (or returns 1
// for non-ExitError failures).
func exitCodeFromWait(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return 1
}
