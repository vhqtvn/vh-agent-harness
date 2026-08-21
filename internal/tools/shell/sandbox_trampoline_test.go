package shell

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
)

// TestMain is the trampoline host for real-backend sandbox tests: the
// sandbox func rewraps the child argv to [<test binary>,
// __exec_sandbox_child, --, <target>, <args...>], so when the TEST
// BINARY is spawned with that verb it must dispatch into
// execsandbox.RunChild (install NoNewPrivs + seccomp + landlock, then
// syscall.Exec the target) instead of running the test suite. This is
// the same hidden-verb contract the daemon implements in run().
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == execsandbox.TrampolineVerb {
		if err := execsandbox.RunChild(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox trampoline child: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0) // unreachable on success: syscall.Exec replaced us
	}
	os.Exit(m.Run())
}

// skipWithoutBackend skips real-backend tests when the OS sandbox
// primitives are unavailable (non-Linux / kernels without
// landlock+seccomp). The fail-closed behavior on such hosts is covered
// kernel-independently by TestSandboxUnavailableFailClosed.
func skipWithoutBackend(t *testing.T) {
	t.Helper()
	if !execsandbox.Detect().Available() {
		t.Skipf("landlock+seccomp unavailable on this host; real-backend confinement not testable in-process")
	}
}

// TestSandboxReadOnlyRunsClean: under read-only confinement ordinary
// read-only commands still work (the sandbox must not break normal
// execution — only writes are denied).
func TestSandboxReadOnlyRunsClean(t *testing.T) {
	skipWithoutBackend(t)
	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxReadOnly})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	out := runQuick(t, Config{Sandbox: fn, SandboxName: "read-only"}, "printf ro-ok", 15000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("clean command failed under read-only: %+v (stderr=%q)", out, out.Stderr)
	}
	if out.Stdout != "ro-ok" {
		t.Fatalf("stdout = %q, want ro-ok", out.Stdout)
	}
	if out.Sandbox != "read-only" {
		t.Fatalf("sandbox = %q, want read-only", out.Sandbox)
	}
}

// TestSandboxReadOnlyDeniesWrite (real backend): a write attempt under
// read-only confinement is DENIED by the kernel (EACCES surfaces in
// the shell as Permission denied), the file is never created, and the
// outcome classifies honestly as a NORMAL non-zero exit — the
// orthogonal-fact model stays intact (runtime denial, not tool error).
func TestSandboxReadOnlyDeniesWrite(t *testing.T) {
	skipWithoutBackend(t)
	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxReadOnly})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	target := filepath.Join(t.TempDir(), "denied-write")
	out := runQuick(t, Config{Sandbox: fn, SandboxName: "read-only"}, "echo x > "+target, 15000)
	if out.Cause != CauseExit {
		t.Fatalf("cause = %q, want exit (runtime denial is an ordinary non-zero exit)", out.Cause)
	}
	if out.ExitCode == 0 {
		t.Fatalf("write under read-only must fail, got exit 0 (stderr=%q)", out.Stderr)
	}
	if !strings.Contains(out.Stderr, "Permission denied") {
		t.Fatalf("stderr lacks the EACCES diagnostic: %q", out.Stderr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("denied write left a file behind: %s", target)
	}
}

// TestSandboxWorkspaceWriteInsideAndOutside (real backend):
// workspace-write allows writes under the configured WritableRoots and
// denies writes outside them.
func TestSandboxWorkspaceWriteInsideAndOutside(t *testing.T) {
	skipWithoutBackend(t)
	inside := t.TempDir()
	outside := t.TempDir() // sibling temp dir: exists, but NOT a writable root
	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxWorkspaceWrite, WritableRoots: []string{inside}})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	cfg := Config{Sandbox: fn, SandboxName: "workspace-write"}

	insideFile := filepath.Join(inside, "allowed.txt")
	out := runQuick(t, cfg, "echo ws-ok > "+insideFile, 15000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("write inside the writable root denied: %+v (stderr=%q)", out, out.Stderr)
	}
	got, err := os.ReadFile(insideFile)
	if err != nil || strings.TrimSpace(string(got)) != "ws-ok" {
		t.Fatalf("inside write content = %q, %v", got, err)
	}
	if out.Sandbox != "workspace-write" {
		t.Fatalf("sandbox = %q, want workspace-write", out.Sandbox)
	}

	outsideFile := filepath.Join(outside, "denied.txt")
	out = runQuick(t, cfg, "echo x > "+outsideFile, 15000)
	if out.Cause != CauseExit || out.ExitCode == 0 {
		t.Fatalf("write outside the writable root must be denied: %+v (stderr=%q)", out, out.Stderr)
	}
	if !strings.Contains(out.Stderr, "Permission denied") {
		t.Fatalf("stderr lacks the EACCES diagnostic: %q", out.Stderr)
	}
	if _, err := os.Stat(outsideFile); !os.IsNotExist(err) {
		t.Fatalf("denied write left a file behind: %s", outsideFile)
	}
}

// TestSandboxTimeoutStillKillsProcessGroup ports the group-kill proof
// under the sandbox rewrap: the SandboxFunc contract requires the
// EXECUTED LEADER to stay a process-group leader, or timeout teardown
// degrades to killing only the trampoline. A background child spawned
// by the timed-out sandboxed command must be dead after run returns.
func TestSandboxTimeoutStillKillsProcessGroup(t *testing.T) {
	skipWithoutBackend(t)
	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxReadOnly})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	out := runQuick(t, Config{Sandbox: fn, SandboxName: "read-only"}, "sleep 30 & echo bgpid=$!; sleep 30", 4000)
	if out.Cause != CauseTimeout || !out.TimedOut {
		t.Fatalf("cause=%q timedOut=%v, want timeout", out.Cause, out.TimedOut)
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(out.Stdout), "bgpid=%d", &pid); err != nil {
		t.Fatalf("cannot parse background pid from stdout %q: %v", out.Stdout, err)
	}
	if pid <= 1 {
		t.Fatalf("refusing pid %d as a background-child proof", pid)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return // reaped: the group kill reached the background child
		}
		if err != nil {
			t.Fatalf("liveness probe on pid %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("background sleep (pid %d) survived the timeout teardown; the rewrap broke group leadership", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestSandboxOffIdenticalToPreSlice: mode off is expressed by leaving
// Config.Sandbox nil — byte-identical to the pre-slice posture. The
// child environment carries NO sandbox plumbing and the outcome says
// "none".
func TestSandboxOffIdenticalToPreSlice(t *testing.T) {
	cfg := Config{} // zero value = the loud pre-slice default
	if cfg.Sandbox != nil {
		t.Fatalf("zero Config must have a nil SandboxFunc")
	}
	out := runQuick(t, cfg, "env | grep -c VH_EXEC_SANDBOX_ || true", 5000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("env probe failed: %+v (stderr=%q)", out, out.Stderr)
	}
	if strings.TrimSpace(out.Stdout) != "0" {
		t.Fatalf("sandbox env vars leaked into an unconfined child env: %q", out.Stdout)
	}
	if out.Sandbox != "none" {
		t.Fatalf("sandbox = %q, want none", out.Sandbox)
	}
}

// --- real-kernel network denial (block D-F1 proof) ---
//
// The probe plumbing is FLAG-based, not env-based: buildEnv() is a
// default-deny scrub (TERM/PATH/HOME/LANG + allowlist only), so an env
// var set by the test would never reach the confined child. A custom
// test flag survives the re-exec because the test binary parses its
// own flags inside the confinement (seccomp+landlock are inherited
// across exec).

// netProbeAddr arms TestSandboxNetProbeHelper when the test binary is
// re-executed as a confined network probe.
var netProbeAddr = flag.String("net-probe-addr", "", "helper mode: dial this TCP address and report the outcome")

// TestSandboxNetProbeHelper is not a standalone test: it is the body of
// the confined probe. When -net-probe-addr is set (only the re-exec'd
// probe sets it), it attempts a real TCP connect and prints exactly one
// outcome marker on stdout:
//
//	NET-PROBE:CONNECTED      the dial succeeded (network NOT denied)
//	NET-PROBE:DENIED:<err>   the dial failed (network denied)
//
// It never fails the test in either branch — the PARENT decides which
// outcome is correct for its posture (control run wants CONNECTED,
// confined run wants DENIED).
func TestSandboxNetProbeHelper(t *testing.T) {
	if *netProbeAddr == "" {
		t.Skip("probe helper: invoked only via -net-probe-addr (see TestSandboxDeniesNetwork)")
	}
	conn, err := net.DialTimeout("tcp", *netProbeAddr, 5*time.Second)
	if err != nil {
		fmt.Printf("NET-PROBE:DENIED:%v\n", err)
		return
	}
	conn.Close()
	fmt.Println("NET-PROBE:CONNECTED")
}

// TestSandboxDeniesNetwork (real backend, block D-F1 behavioral proof):
// a confined process attempting a TCP connect to a LIVE local listener
// must FAIL — the dial is denied by the seccomp network blocklist, the
// connection is never established. This is the kernel-level net-deny
// guarantee the sandbox promises, proven against a real listener.
//
// Design: a TCP listener runs in the test process (outside the
// sandbox); the probe is THIS TEST BINARY re-executed under confinement
// (the Go-idiomatic helper pattern — os.Args[0] -test.run=...), so the
// dial is made by a real Go program from inside the sandbox, not by a
// shell /dev/tcp trick that depends on bash build flags.
//
// The CONTROL phase first runs the identical probe UNCONFINED and
// requires NET-PROBE:CONNECTED: if the probe mechanism were broken
// (listener dead, re-exec failed), the control fails the test instead
// of letting a broken probe pass vacuously as "denied".
func TestSandboxDeniesNetwork(t *testing.T) {
	skipWithoutBackend(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot bind a local TCP listener on this host: %v", err)
	}
	defer ln.Close()
	go func() { // accept loop: drains dials so a successful connect completes instantly
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	probe := fmt.Sprintf("'%s' -test.run=^TestSandboxNetProbeHelper$ -net-probe-addr=%s", os.Args[0], ln.Addr().String())

	// Control: identical probe, no confinement → must connect.
	ctl := runQuick(t, Config{}, probe, 30000)
	if ctl.Cause != CauseExit || ctl.ExitCode != 0 {
		t.Fatalf("unconfined control probe failed to run: %+v (stderr=%q)", ctl, ctl.Stderr)
	}
	if !strings.Contains(ctl.Stdout, "NET-PROBE:CONNECTED") {
		t.Fatalf("unconfined control probe did not connect — the probe mechanism is broken, the denial assertion below would be vacuous (stdout=%q stderr=%q)", ctl.Stdout, ctl.Stderr)
	}

	// Confined: same probe under read-only (NetDeny) → dial must fail.
	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxReadOnly})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	out := runQuick(t, Config{Sandbox: fn, SandboxName: "read-only"}, probe, 30000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("confined probe failed to run as a normal exit: %+v (stderr=%q)", out, out.Stderr)
	}
	if !strings.Contains(out.Stdout, "NET-PROBE:DENIED") {
		t.Fatalf("confined probe stdout lacks the DENIED marker — the dial was NOT denied: %q", out.Stdout)
	}
	if strings.Contains(out.Stdout, "NET-PROBE:CONNECTED") {
		t.Fatalf("confined probe CONNECTED through the net-deny policy: %q", out.Stdout)
	}
	if out.Sandbox != "read-only" {
		t.Fatalf("sandbox = %q, want read-only", out.Sandbox)
	}
}
