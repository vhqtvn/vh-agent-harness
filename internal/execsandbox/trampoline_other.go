//go:build !linux

package execsandbox

import (
	"context"
	"fmt"
)

// Features describes which OS sandbox primitives are available. On non-Linux
// platforms neither Landlock nor seccomp-BPF exists, so this is always zero-valued.
type Features struct {
	Landlock bool
	Seccomp  bool
}

// Available reports whether both primitives are present. Always false on non-Linux.
func (f Features) Available() bool {
	return f.Landlock && f.Seccomp
}

// Detect always returns zero Features on non-Linux. The sandbox is Linux-first;
// macOS/other platforms get strict=fail-closed or best-effort=loud-warn+fallback.
func Detect() Features {
	return Features{}
}

// runTrampoline is unreachable on non-Linux (Detect().Available() is always
// false, so Run() never calls this). The stub exists for compilation.
func runTrampoline(_ context.Context, _ Profile, _, _ string, _ []string) (int, error) {
	return 1, fmt.Errorf("sandbox trampoline not supported on this platform")
}

// RunChild is unreachable on non-Linux. The stub exists for compilation.
func RunChild(_ []string) error {
	return fmt.Errorf("sandbox child not supported on this platform")
}
