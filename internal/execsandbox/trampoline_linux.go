//go:build linux

package execsandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/elastic/go-seccomp-bpf"
	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"
)

// Features describes which OS sandbox primitives are available on this system.
type Features struct {
	Landlock bool
	Seccomp  bool
}

// Available reports whether both primitives are present (the full-sandbox gate).
// The sandbox requires both: landlock for filesystem integrity, seccomp for
// network/syscall hardening. If either is missing, strict mode fails and
// best-effort mode falls back to exec-ro classification.
func (f Features) Available() bool {
	return f.Landlock && f.Seccomp
}

// Detect probes the running kernel for Landlock and seccomp support.
func Detect() Features {
	return Features{
		Landlock: landlockSupported(),
		Seccomp:  seccomp.Supported(),
	}
}

// landlockSupported probes the kernel for landlock_create_ruleset support.
// The probe is stateless: landlock_create_ruleset(NULL, 0, 0) creates nothing.
// ENOSYS or EOPNOTSUPP → unsupported; any other errno (EINVAL, EOVERFLOW,
// EFAULT, or success) → the syscall exists → supported.
func landlockSupported() bool {
	_, _, errno := syscall.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, 0)
	switch errno {
	case syscall.ENOSYS, syscall.EOPNOTSUPP:
		return false
	default:
		return true
	}
}

// --- Parent: trampoline orchestration ---

// runTrampoline performs the two-stage re-exec:
//  1. Parent fork/execs itself as `vh-agent-harness __exec_sandbox_child -- <cmd> <args>`
//     in a new session+process group (Setsid), with the profile serialized to env.
//  2. The child installs PR_SET_PDEATHSIG → SetNoNewPrivs → seccomp → landlock,
//     then syscall.Exec's into the target.
//
// The parent forwards SIGINT/SIGTERM to the child's process group and waits.
func runTrampoline(ctx context.Context, profile Profile, repoRoot, target string, args []string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 1, fmt.Errorf("locating self executable for trampoline: %w", err)
	}

	// Build child argv: self __exec_sandbox_child -- <target> <args...>
	childArgv := []string{"__exec_sandbox_child", "--", target}
	childArgv = append(childArgv, args...)

	cmd := exec.CommandContext(ctx, self, childArgv...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = repoRoot
	cmd.Env = buildChildEnv(profile)
	// New session + new process group (Codex --new-session/--die-with-parent equiv).
	// Setsid alone suffices: it creates a new session AND makes the child the
	// process group leader. Do NOT also set Setpgid — Go calls setpgid(0,0)
	// BEFORE setsid(), making the child a pgrp leader, which then causes
	// setsid() to fail with EPERM (setsid requires the caller not be a leader).
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("starting sandbox child: %w", err)
	}

	childPid := cmd.Process.Pid

	// Forward interactive signals to the child's process group.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	go func() {
		for sig := range sigCh {
			// Negative pid = signal the process group.
			_ = syscall.Kill(-childPid, sig.(syscall.Signal))
		}
	}()

	waitErr := cmd.Wait()
	return exitCodeFromWait(waitErr), nil
}

// --- Child: lockdown sequence ---

// RunChild is the entry point for the hidden `__exec_sandbox_child` trampoline.
// It reads the profile from env, installs OS protections in the mandated order,
// then replaces the process image via syscall.Exec.
//
// Lockdown order (justified deviation from brief's listed order — the kernel
// requires NoNewPrivs BEFORE seccomp install for unprivileged processes;
// LoadFilter re-asserts it idempotently):
//  1. PR_SET_PDEATHSIG(SIGKILL) — die if parent dies
//  2. exec.LookPath — resolve target before restrictions
//  3. SetNoNewPrivs — irreversible privilege drop
//  4. seccomp.LoadFilter (FlagTsync) — network + syscall hardening
//  5. landlock.RestrictPaths — filesystem integrity
//  6. syscall.Exec — replace process image with target
//
// args is the argv after the `--` separator: [target, arg1, arg2, ...].
func RunChild(args []string) error {
	// Strip leading "--" if cobra passed it through.
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("no command specified for sandbox child")
	}
	target := args[0]
	targetArgs := args[1:]

	profile, err := profileFromEnv()
	if err != nil {
		return fmt.Errorf("reading sandbox profile: %w", err)
	}

	// 1. PR_SET_PDEATHSIG(SIGKILL) — child dies if parent harness exits.
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(unix.SIGKILL), 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_PDEATHSIG: %w", err)
	}
	// Race mitigation: if parent already died, getppid() == 1 (init/reaper).
	if os.Getppid() == 1 {
		fmt.Fprintln(os.Stderr, "exec-sandbox: parent already exited; aborting child")
		os.Exit(1)
	}

	// 2. Resolve target binary BEFORE installing restrictions (LookPath needs
	//    full FS access; the binary lives in RODirs but resolve early for safety).
	targetPath, err := exec.LookPath(target)
	if err != nil {
		return fmt.Errorf("resolving target %q: %w", target, err)
	}

	// 3. SetNoNewPrivs — required before seccomp for unprivileged processes.
	//    Irreversible: the process can never gain new privileges.
	if err := seccomp.SetNoNewPrivs(); err != nil {
		return fmt.Errorf("SetNoNewPrivs: %w", err)
	}

	// 4. Install seccomp filter (network + high-risk syscall blocklist).
	if err := installSeccomp(profile.Net); err != nil {
		return fmt.Errorf("installing seccomp filter: %w", err)
	}

	// 5. Install landlock filesystem restrictions.
	if err := installLandlock(profile); err != nil {
		return fmt.Errorf("installing landlock restrictions: %w", err)
	}

	// 6. Build target environment (strip internal vars, keep GIT_OPTIONAL_LOCKS=0).
	targetEnv := envForTarget(os.Environ())

	// 7. Replace process image — landlock + seccomp are inherited.
	return syscall.Exec(targetPath, append([]string{target}, targetArgs...), targetEnv)
}

// --- seccomp installation ---

// highRiskSyscalls are ALWAYS blocked regardless of network policy. These are
// syscalls that could be used to escape isolation, tamper with the kernel, or
// load code: ptrace, BPF, mount/namespace manipulation, module loading, etc.
//
// clone/clone3 are intentionally LEFT ALLOWED — blocking them breaks normal
// fork/thread creation. Namespace defense relies on blocking unshare/setns/
// mount/pivot_root/move_mount/fs* instead.
var highRiskSyscalls = []string{
	// process inspection / memory manipulation
	"ptrace",
	"process_vm_readv",
	"process_vm_writev",
	// kernel tracing / BPF
	"bpf",
	"perf_event_open",
	// filesystem handle escape
	"open_by_handle_at",
	// mount namespace manipulation
	"mount",
	"umount2",
	"pivot_root",
	"move_mount",
	"fsopen",
	"fsmount",
	"fsconfig",
	"open_tree",
	// namespace creation/entry
	"unshare",
	"setns",
	// swap management
	"swapon",
	"swapoff",
	// system power/control
	"reboot",
	"settimeofday",
	"clock_settime",
	// kernel image loading
	"kexec_load",
	"kexec_file_load",
	// kernel module loading
	"init_module",
	"finit_module",
	"delete_module",
	// splice from user memory (can trick write checks)
	"vmsplice",
}

// networkSyscalls are blocked when NetDeny is active. They cover all standard
// POSIX network operations at the syscall layer.
var networkSyscalls = []string{
	"socket",
	"socketpair",
	"connect",
	"bind",
	"listen",
	"accept",
	"accept4",
	"sendto",
	"recvfrom",
	"sendmsg",
	"recvmsg",
}

// installSeccomp loads a focused BLOCKLIST (not a broad allowlist). The default
// action is ALLOW; blocked syscalls return ENOSYS (the library's default errno
// for ActionErrno). When net is NetDeny, network syscalls are added to the
// blocklist.
func installSeccomp(net NetPolicy) error {
	blocked := make([]string, len(highRiskSyscalls))
	copy(blocked, highRiskSyscalls)

	if net == NetDeny {
		blocked = append(blocked, networkSyscalls...)
	}

	filter := seccomp.Filter{
		NoNewPrivs: true, // idempotent re-assert (SetNoNewPrivs already called)
		Flag:       seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{
			DefaultAction: seccomp.ActionAllow,
			Syscalls: []seccomp.SyscallGroup{
				{
					Names:  blocked,
					Action: seccomp.ActionErrno,
				},
			},
		},
	}
	return seccomp.LoadFilter(filter)
}

// --- landlock installation ---

// installLandlock applies filesystem restrictions via Landlock V9 BestEffort.
// Non-existent paths are filtered before passing to landlock (RestrictPaths
// errors on missing paths even in BestEffort mode; BestEffort only degrades
// the ABI version, not path validation).
func installLandlock(profile Profile) error {
	var rules []landlock.Rule

	roDirs := filterExistingDirs(profile.RODirs)
	if len(roDirs) > 0 {
		rules = append(rules, landlock.RODirs(roDirs...))
	}

	rwDirs := filterExistingDirs(profile.RWDirs)
	if len(rwDirs) > 0 {
		rules = append(rules, landlock.RWDirs(rwDirs...))
	}

	rwFiles := filterExistingFiles(profile.RWFiles)
	if len(rwFiles) > 0 {
		rules = append(rules, landlock.RWFiles(rwFiles...))
	}

	if len(rules) == 0 {
		return fmt.Errorf("no valid (existing) paths in sandbox profile")
	}

	// V9.BestEffort degrades to the highest supported kernel ABI and re-asserts
	// NoNewPrivs (idempotent). Returns nil no-op if landlock is entirely absent.
	return landlock.V9.BestEffort().RestrictPaths(rules...)
}

// filterExistingDirs returns only entries that exist on disk. Non-existent
// paths are silently skipped to avoid landlock errors on optional system paths
// (e.g., /lib64 may not exist on all architectures).
func filterExistingDirs(dirs []string) []string {
	var result []string
	for _, d := range dirs {
		// Use Lstat to catch symlinks too (merged-/usr systems symlink /bin → /usr/bin).
		if _, err := os.Lstat(d); err == nil {
			result = append(result, d)
		}
	}
	return result
}

// filterExistingFiles returns only entries that exist on disk (for RWFiles).
func filterExistingFiles(files []string) []string {
	var result []string
	for _, f := range files {
		if _, err := os.Lstat(f); err == nil {
			result = append(result, f)
		}
	}
	return result
}

// resolveRepoRoot is a helper for callers that need to absolutize the repo root.
func resolveRepoRoot(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
