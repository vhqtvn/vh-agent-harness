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
//  1. Reject incompatible flag combinations (ModeOff + NetAsk) up front.
//  2. ModeOff → run directly (no sandbox, no GIT_OPTIONAL_LOCKS rewrite).
//  3. Resolve --net=ask (TTY prompt or non-TTY hard-deny).
//  4. Detect OS primitives. If both available → full trampoline sandbox.
//  5. Primitives unavailable + strict → fail-closed (return error).
//  6. Primitives unavailable + best-effort → loud warn + exec-ro classify +
//     run if allowed, deny if not.
//
// Returns the child exit code and any setup/teardown error.
func Run(ctx context.Context, mode SandboxMode, profile Profile, repoRoot, target string, args []string) (int, error) {
	// D-7: --sandbox=off + --net=ask is an incompatible combination. The ask
	// prompt cannot be honored when the sandbox is off — a "deny" answer has
	// no seccomp filter to enforce (ModeOff skips the trampoline entirely),
	// and silently dropping the ask to allow ordinary host networking is
	// fail-open. Reject BEFORE payload execution. Do NOT prompt; the operator
	// must pick --net=allow or --net=deny when running with --sandbox=off.
	if mode == ModeOff && profile.Net == NetAsk {
		return 1, fmt.Errorf("--sandbox=off and --net=ask are incompatible: " +
			"--net=ask resolves via a TTY prompt whose deny/allow answer cannot " +
			"be enforced when the sandbox is off (use --net=allow or --net=deny " +
			"with --sandbox=off)")
	}

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
	return profileFromEnvFrom(os.Environ())
}

// profileFromEnvFrom is the testable form of profileFromEnv: it reconstructs
// the profile from a supplied env slice (each entry is "KEY=VALUE"). Tests use
// this to avoid mutating the process env.
//
// D-5 fail-closed: the child boundary independently decodes NET to a known
// safe value. Missing or empty NET defaults to NetDeny (the documented safe
// default); "deny"/"allow" are accepted; "ask" or any unknown nonempty value
// returns an error rather than silently falling back to fail-open behavior.
// Parent-side serialization (profileToEnv) is useful mitigation but is NOT
// sufficient — the child boundary must independently fail-closed.
func profileFromEnvFrom(env []string) (Profile, error) {
	var p Profile
	p.Net = NetDeny // safe default; overwritten by VH_EXEC_SANDBOX_NET if present
	for _, kv := range env {
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
			net, err := decodeNetPolicy(val)
			if err != nil {
				return p, err
			}
			p.Net = net
		}
	}
	if len(p.RODirs) == 0 && len(p.RWDirs) == 0 {
		return p, fmt.Errorf("no sandbox profile found in environment (missing %s* vars)", envPrefix)
	}
	return p, nil
}

// decodeNetPolicy strictly decodes the VH_EXEC_SANDBOX_NET value at the child
// trampoline boundary. The child MUST only ever see "deny" or "allow" — the
// parent resolves "ask" (TTY prompt or non-TTY hard-deny) before serializing
// NET to env. So "ask" or any unrecognized nonempty value at the child is an
// internal contract violation: return an error rather than silently picking a
// fallback (which would risk fail-open if the default were ever wrong).
//
// Empty/missing is the only value that gets an implicit default (NetDeny);
// every other value MUST be on the explicit allowlist.
func decodeNetPolicy(val string) (NetPolicy, error) {
	switch val {
	case "", "deny":
		return NetDeny, nil
	case "allow":
		return NetAllow, nil
	case "ask":
		return "", fmt.Errorf("%sNET=ask is an internal contract violation: "+
			"the parent must resolve --net=ask before serializing the profile "+
			"(the child boundary only accepts deny|allow)", envPrefix)
	default:
		return "", fmt.Errorf("%sNET=%q is not a recognized network policy "+
			"(expected deny|allow at the child boundary)", envPrefix, val)
	}
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
