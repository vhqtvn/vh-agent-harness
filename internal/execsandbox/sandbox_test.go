package execsandbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDecodeNetPolicy locks the D-5 fail-closed contract at the child boundary:
// the VH_EXEC_SANDBOX_NET value MUST decode to a known policy or fail-closed
// with an error. Empty/missing maps to NetDeny (the safe default); "deny" and
// "allow" are accepted; "ask" and any unknown nonempty value are INTERNAL
// CONTRACT VIOLATIONS (the parent must resolve "ask" to deny/allow before
// serializing NET to env) and MUST return an error rather than risk fail-open.
//
// Kernel-independent: this is pure string decoding.
func TestDecodeNetPolicy(t *testing.T) {
	cases := []struct {
		name    string
		val     string
		wantNet NetPolicy
		wantErr bool
	}{
		// Empty/missing is the only value that gets an implicit default.
		{"empty → deny (fail-closed default)", "", NetDeny, false},
		// Explicit deny.
		{"deny", "deny", NetDeny, false},
		// Explicit allow.
		{"allow", "allow", NetAllow, false},
		// "ask" at the child is a contract violation: the parent must have
		// resolved it before serializing. Returning NetDeny silently would
		// hide the bug; error makes the violation visible.
		{"ask → error (contract violation)", "ask", "", true},
		// Unknown nonempty values are also contract violations.
		{"unknown 'maybe' → error", "maybe", "", true},
		{"unknown 'Deny' → error (case-sensitive)", "Deny", "", true},
		{"unknown ' net ' → error (not trimmed)", " net ", "", true},
		{"empty-looking '0' → error", "0", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeNetPolicy(tc.val)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("decodeNetPolicy(%q) returned nil error, want non-nil (fail-closed)", tc.val)
				}
				if got != "" {
					t.Errorf("decodeNetPolicy(%q) error case returned non-empty policy %q; want empty to avoid misuse", tc.val, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("decodeNetPolicy(%q) returned unexpected error: %v", tc.val, err)
			}
			if got != tc.wantNet {
				t.Errorf("decodeNetPolicy(%q) = %q, want %q", tc.val, got, tc.wantNet)
			}
		})
	}
}

// TestProfileFromEnvFrom_StrictDecoding locks the D-5 contract end-to-end:
// profileFromEnvFrom MUST default NET to NetDeny when missing, MUST accept
// "deny"/"allow" from env, and MUST propagate decodeNetPolicy's error for
// "ask"/unknown values (rather than silently falling back). This is the
// fail-closed guarantee at the child trampoline boundary.
//
// Kernel-independent.
func TestProfileFromEnvFrom_StrictDecoding(t *testing.T) {
	// Minimal valid profile: a single RO dir so profileFromEnvFrom does not
	// reject the env for lacking any path entries.
	const roDir = "VH_EXEC_SANDBOX_RO_DIRS=/repo"

	cases := []struct {
		name    string
		env     []string
		wantNet NetPolicy
		wantErr bool
		errSub  string
	}{
		{
			name:    "NET missing → default NetDeny (fail-closed)",
			env:     []string{roDir},
			wantNet: NetDeny,
		},
		{
			name:    "NET empty → default NetDeny (fail-closed)",
			env:     []string{roDir, "VH_EXEC_SANDBOX_NET="},
			wantNet: NetDeny,
		},
		{
			name:    "NET=deny",
			env:     []string{roDir, "VH_EXEC_SANDBOX_NET=deny"},
			wantNet: NetDeny,
		},
		{
			name:    "NET=allow",
			env:     []string{roDir, "VH_EXEC_SANDBOX_NET=allow"},
			wantNet: NetAllow,
		},
		{
			name:    "NET=ask → error (contract violation)",
			env:     []string{roDir, "VH_EXEC_SANDBOX_NET=ask"},
			wantErr: true,
			errSub:  "contract violation",
		},
		{
			name:    "NET=unknown → error",
			env:     []string{roDir, "VH_EXEC_SANDBOX_NET=anything"},
			wantErr: true,
			errSub:  "not a recognized network policy",
		},
		{
			name:    "no profile at all → error (missing VH_EXEC_SANDBOX_* vars)",
			env:     []string{"PATH=/usr/bin", "HOME=/root"},
			wantErr: true,
			errSub:  "no sandbox profile",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := profileFromEnvFrom(tc.env)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("profileFromEnvFrom(%v) returned nil error, want non-nil", tc.env)
				}
				if tc.errSub != "" && !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("profileFromEnvFrom(%v) error = %q, want substring %q", tc.env, err.Error(), tc.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("profileFromEnvFrom(%v) returned unexpected error: %v", tc.env, err)
			}
			if p.Net != tc.wantNet {
				t.Errorf("profileFromEnvFrom(%v).Net = %q, want %q", tc.env, p.Net, tc.wantNet)
			}
		})
	}
}

// TestResolveAskNet_NonTTYHardDenies locks the D-6 contract: when stdin is not
// a TTY, resolveAskNet MUST return (NetDeny, non-nil error) WITHOUT prompting.
// Agents cannot auto-accept --net=ask. Uses os.Pipe to construct a deterministic
// non-TTY reader (a pipe is not a character device, so isTTY returns false).
//
// Kernel-independent: no Landlock/seccomp dependency.
func TestResolveAskNet_NonTTYHardDenies(t *testing.T) {
	// os.Pipe returns a non-character-device file → isTTY returns false →
	// resolveAskNet takes the non-TTY hard-deny branch WITHOUT reading.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// Sanity: confirm the pipe reader is NOT detected as a TTY. This is the
	// kernel-independent property the test relies on.
	if isTTY(r) {
		t.Fatalf("TEST PRECONDITION FAILED: os.Pipe reader was detected as a TTY; the test no longer exercises the non-TTY path")
	}

	var buf bytes.Buffer
	policy, askErr := resolveAskNet(r, &buf)

	if policy != NetDeny {
		t.Errorf("resolveAskNet on non-TTY returned policy %q, want %q (hard-deny)", policy, NetDeny)
	}
	if askErr == nil {
		t.Fatalf("resolveAskNet on non-TTY returned nil error, want non-nil (callers rely on the error to abort)")
	}
	if !strings.Contains(askErr.Error(), "non-interactive") {
		t.Errorf("resolveAskNet error = %q, want it to mention 'non-interactive'", askErr.Error())
	}
	// No prompt should be printed for the non-TTY path (the prompt is TTY-only).
	if buf.Len() != 0 {
		t.Errorf("resolveAskNet on non-TTY wrote unexpected output (a prompt must not be printed): %q", buf.String())
	}
}

// TestRun_NetAskNonTTY_HardDeniesAndDoesNotExecutePayload locks the D-6
// enclosing-path guarantee: when --net=ask is invoked under a non-TTY stdin,
// Run() MUST return (1, non-nil error) BEFORE the payload is executed. A marker
// file the payload would create must NOT exist after Run returns.
//
// Kernel-independent: the ask-resolution short-circuit fires BEFORE Detect().
//
// NOTE: Run() reads os.Stdin directly (not a parameter). This test temporarily
// swaps os.Stdin for a deterministic non-TTY reader (an os.Pipe) and restores
// it via defer. Go test runs process tests sequentially within a package by
// default; t.Parallel MUST NOT be used here.
func TestRun_NetAskNonTTY_HardDeniesAndDoesNotExecutePayload(t *testing.T) {
	// Construct a deterministic non-TTY stdin.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if isTTY(os.Stdin) {
		t.Fatalf("TEST PRECONDITION FAILED: swapped os.Stdin still detected as a TTY")
	}

	// Marker file the payload would create if executed. Using a path under
	// t.TempDir() keeps the test hermetic.
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "executed-marker")

	ctx := context.Background()
	profile := Profile{Net: NetAsk}
	exitCode, runErr := Run(ctx, ModeBestEffort, profile, tmp, "touch", []string{marker})

	if exitCode != 1 {
		t.Errorf("Run(non-TTY --net=ask) exitCode = %d, want 1", exitCode)
	}
	if runErr == nil {
		t.Errorf("Run(non-TTY --net=ask) returned nil error, want non-nil")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("payload marker %q was created — non-TTY --net=ask hard-deny MUST NOT execute the payload", marker)
	}
}

// TestRun_ModeOffPlusNetAsk_RejectedBeforeExecution locks the D-7 contract:
// --sandbox=off + --net=ask is an incompatible combination. The ask prompt
// cannot be enforced when the sandbox is off (ModeOff skips the trampoline, so
// a "deny" answer has no seccomp filter to back it). Run() MUST reject the
// combination BEFORE payload execution (no prompt, no payload run).
//
// Kernel-independent: the validation runs at the very top of Run() before
// Detect() or ask-resolution.
//
// NOTE: also swaps os.Stdin to defend against a future regression that moves
// ask-resolution before the compatibility check — if ask-resolution ran first,
// the non-TTY swap would still deny the payload, masking the regression. With
// the swap, a regression that prompts would either hang or hard-deny here.
// Go test runs tests sequentially within a package by default; t.Parallel MUST
// NOT be used here.
func TestRun_ModeOffPlusNetAsk_RejectedBeforeExecution(t *testing.T) {
	// Swap stdin to a deterministic non-TTY reader so a future regression that
	// moved ask-resolution before the compatibility check would not hang.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	tmp := t.TempDir()
	marker := filepath.Join(tmp, "executed-marker")

	ctx := context.Background()
	profile := Profile{Net: NetAsk}
	exitCode, runErr := Run(ctx, ModeOff, profile, tmp, "touch", []string{marker})

	if exitCode != 1 {
		t.Errorf("Run(ModeOff + NetAsk) exitCode = %d, want 1", exitCode)
	}
	if runErr == nil {
		t.Fatalf("Run(ModeOff + NetAsk) returned nil error, want the incompatible-combination error")
	}
	if !strings.Contains(runErr.Error(), "incompatible") {
		t.Errorf("Run(ModeOff + NetAsk) error = %q, want it to mention 'incompatible'", runErr.Error())
	}
	if !strings.Contains(runErr.Error(), "--sandbox=off") || !strings.Contains(runErr.Error(), "--net=ask") {
		t.Errorf("Run(ModeOff + NetAsk) error = %q, want it to name both flags so the operator can fix the invocation", runErr.Error())
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatalf("payload marker %q was created — the combination MUST be rejected BEFORE payload execution", marker)
	}
}

// TestRun_ModeOffWithExplicitDenyOrAllow_StillRuns confirms that the D-7 fix
// is scoped to ModeOff + NetAsk ONLY: ModeOff + NetDeny and ModeOff + NetAllow
// must still execute the payload directly (off means off). This guards against
// an over-broad validation regressing the legitimate ModeOff surface.
//
// Kernel-independent: ModeOff short-circuits before Detect().
func TestRun_ModeOffWithExplicitDenyOrAllow_StillRuns(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "out.txt")
	ctx := context.Background()

	for _, tc := range []struct {
		name string
		net  NetPolicy
	}{
		{"ModeOff + NetDeny runs", NetDeny},
		{"ModeOff + NetAllow runs", NetAllow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.Remove(marker)
			profile := Profile{Net: tc.net}
			exitCode, runErr := Run(ctx, ModeOff, profile, tmp, "touch", []string{marker})
			if runErr != nil {
				t.Fatalf("Run(%s) returned unexpected error: %v", tc.name, runErr)
			}
			if exitCode != 0 {
				t.Fatalf("Run(%s) exitCode = %d, want 0", tc.name, exitCode)
			}
			if _, statErr := os.Stat(marker); statErr != nil {
				t.Errorf("Run(%s) did not create the marker — ModeOff must still run the payload: %v", tc.name, statErr)
			}
		})
	}
}
