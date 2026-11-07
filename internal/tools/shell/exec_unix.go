//go:build unix

package shell

import (
	"os/exec"
	"syscall"
)

// applyProcessGroup makes the child a process-group leader so timeout
// teardown can address the WHOLE group (the shell plus any background
// jobs/pipelines it spawned) with one negative-pid signal. Without
// Setpgid, killing only the direct child would leave grandchildren
// running as orphans.
func applyProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup is the exec.Cancel hook: on deadline it SIGKILLs the
// child's process group (negative pid). If the process never started
// (Process nil) there is nothing to kill.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// terminatingSignal returns the signal name when the reaped process
// died by signal, and whether it was signaled at all. It is consulted
// only on the non-timeout path (the timeout path classifies before
// looking at signals, so a timeout kill is never misread as an external
// signal).
func terminatingSignal(cmd *exec.Cmd) (string, bool) {
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return "", false
	}
	return ws.Signal().String(), true
}
