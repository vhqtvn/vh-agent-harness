//go:build linux

package execsandbox

import "testing"

// TestHighRiskSyscallsBlockIoUring is the static companion to the
// real-kernel probes (internal/tools/shell: TestSandboxDeniesNetwork,
// TestSandboxIoUringSetupDenied): it pins the io_uring syscalls in the
// ALWAYS-ON blocklist so a future list edit cannot silently reintroduce
// the net-deny bypass. Kernel context: io_uring ops (IORING_OP_SOCKET/
// CONNECT since 5.19) run on kernel worker threads and never re-enter
// syscall filtering, so an unblocked io_uring_setup defeats the
// NetDeny policy regardless of the networkSyscalls list.
//
// The names must also be spelled EXACTLY as the elastic/go-seccomp-bpf
// arch tables spell them (lowercase, underscore — same convention as
// every other entry); an unknown name would make LoadFilter fail on
// every sandboxed run, which the trampoline tests would catch loudly.
func TestHighRiskSyscallsBlockIoUring(t *testing.T) {
	for _, name := range []string{"io_uring_setup", "io_uring_enter", "io_uring_register"} {
		found := false
		for _, blocked := range highRiskSyscalls {
			if blocked == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("highRiskSyscalls is missing %q: an allowed io_uring ring executes ops in kernel context and bypasses syscall-entry network filtering (net-deny bypass)", name)
		}
	}
}
