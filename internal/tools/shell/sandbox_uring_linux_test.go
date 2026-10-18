//go:build linux

// sandbox_uring_linux_test.go — real-kernel proof (block D-F1) that the
// seccomp blocklist closes the io_uring net-deny bypass: io_uring_setup
// must FAIL inside the sandbox. Since kernel 5.19 io_uring ops
// (IORING_OP_SOCKET/CONNECT et al.) execute in kernel context and never
// re-enter syscall-entry filtering, so a creatable ring means a confined
// process could open a socket the networkSyscalls list would never see.
// The static membership of all three io_uring syscalls in
// highRiskSyscalls is pinned separately by
// TestHighRiskSyscallsBlockIoUring (internal/execsandbox).
package shell

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// uringProbe arms TestSandboxIoUringSetupHelper when the test binary is
// re-executed as a confined io_uring probe (flag-based for the same
// reason as net-probe-addr: the child env is a default-deny scrub).
var uringProbe = flag.Bool("uring-probe", false, "helper mode: attempt io_uring_setup and report the outcome")

// TestSandboxIoUringSetupHelper is not a standalone test: it is the body
// of the confined probe. When -uring-probe is set it calls
// io_uring_setup(4, &params) directly and prints exactly one marker:
//
//	URING-PROBE:CREATED          a ring fd was created (io_uring LIVE)
//	URING-PROBE:BLOCKED:<errno>  the syscall was denied
//
// It never fails in either branch — the parent decides which outcome is
// correct for its posture.
func TestSandboxIoUringSetupHelper(t *testing.T) {
	if !*uringProbe {
		t.Skip("probe helper: invoked only via -uring-probe (see TestSandboxIoUringSetupDenied)")
	}
	// struct io_uring_params is 120 bytes on 64-bit hosts; a zeroed
	// 128-byte buffer is a valid params struct on every kernel revision
	// (defaults, no flags, NULL user_addr; trailing bytes are never
	// read by the kernel).
	var params [128]byte
	fd, _, errno := unix.Syscall(unix.SYS_IO_URING_SETUP, 4, uintptr(unsafe.Pointer(&params[0])), 0)
	if errno != 0 {
		fmt.Printf("URING-PROBE:BLOCKED:%v\n", errno)
		return
	}
	unix.Close(int(fd))
	fmt.Println("URING-PROBE:CREATED")
}

// TestSandboxIoUringSetupDenied (real backend): under confinement
// io_uring_setup must be DENIED by the always-on highRiskSyscalls
// blocklist — the confined process cannot create an io_uring ring, so
// the kernel-context socket/connect ops that would bypass the
// syscall-entry network filter are unreachable.
//
// Control first: the identical probe runs UNCONFINED. CREATED → the
// host kernel has io_uring and the confined assertion below is
// meaningful. BLOCKED (typically ENOSYS) → the kernel predates io_uring
// and a denial inside the sandbox would be indistinguishable from the
// kernel's own ENOSYS, so the runtime assertion skips honestly (the
// list membership itself is still pinned by the static test).
func TestSandboxIoUringSetupDenied(t *testing.T) {
	skipWithoutBackend(t)

	probe := fmt.Sprintf("'%s' -test.run=^TestSandboxIoUringSetupHelper$ -uring-probe", os.Args[0])

	ctl := runQuick(t, Config{}, probe, 30000)
	if ctl.Cause != CauseExit || ctl.ExitCode != 0 {
		t.Fatalf("unconfined io_uring control probe failed to run: %+v (stderr=%q)", ctl, ctl.Stderr)
	}
	if !strings.Contains(ctl.Stdout, "URING-PROBE:CREATED") {
		if strings.Contains(ctl.Stdout, "URING-PROBE:BLOCKED") {
			t.Skipf("host kernel has no io_uring (unconfined setup also fails: %q); a confined denial would be indistinguishable from natural ENOSYS — list membership is pinned by TestHighRiskSyscallsBlockIoUring", ctl.Stdout)
		}
		t.Fatalf("unconfined io_uring control probe produced no marker — the probe mechanism is broken: stdout=%q stderr=%q", ctl.Stdout, ctl.Stderr)
	}

	fn, err := NewSandboxFunc(SandboxOptions{Mode: SandboxReadOnly})
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}
	out := runQuick(t, Config{Sandbox: fn, SandboxName: "read-only"}, probe, 30000)
	if out.Cause != CauseExit || out.ExitCode != 0 {
		t.Fatalf("confined io_uring probe failed to run as a normal exit: %+v (stderr=%q)", out, out.Stderr)
	}
	if !strings.Contains(out.Stdout, "URING-PROBE:BLOCKED") {
		t.Fatalf("confined probe stdout lacks the BLOCKED marker — io_uring_setup was NOT denied (net-deny bypass live): %q", out.Stdout)
	}
	if strings.Contains(out.Stdout, "URING-PROBE:CREATED") {
		t.Fatalf("confined probe CREATED an io_uring ring — kernel-context ops could bypass syscall-entry network filtering: %q", out.Stdout)
	}
}
