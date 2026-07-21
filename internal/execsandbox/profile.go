// Package execsandbox provides a pure-Go, unprivileged, kernel-enforcing Linux
// sandbox composing Landlock (filesystem integrity) with pure-Go seccomp-BPF
// (network + syscall hardening).
//
// HONESTY FRAMING: exec-sandbox is an INTEGRITY + NETWORK boundary, NOT a
// confidentiality/path-hiding boundary. Landlock is additive: denied paths
// remain stat-able (metadata visible via stat/lstat) but unwritable
// (EACCES on open-for-write). opendir is also gated, so listing a denied
// directory (`ls <denied-dir>`) fails with EACCES while the directory's
// own metadata stays visible. The promise is "the agent cannot WRITE or
// NETWORK outside the contract," NOT "the agent cannot SEE anything."
//
// The sandbox is layered WITH exec-ro: exec-sandbox is the authoritative OS
// layer (kernel-enforced); exec-ro is the script-level heuristic pre-filter.
// They compose — exec-sandbox does NOT replace exec-ro.
package execsandbox

import "path/filepath"

// NetPolicy controls network access at the seccomp syscall layer.
//
// At the binary syscall layer this is a two-state filter: when denied, seccomp
// blocks network syscalls (socket/connect/bind/listen/accept/sendto/recvfrom);
// when allowed, they are permitted. The "ask" state is resolved by the parent
// (TTY → interactive [Y/n]; non-TTY → hard-deny + exit) before the child
// trampoline runs, so the child only ever sees deny or allow.
type NetPolicy string

const (
	// NetDeny blocks all network syscalls via seccomp. This is the default.
	NetDeny NetPolicy = "deny"
	// NetAllow permits network syscalls (no seccomp network blocklist).
	NetAllow NetPolicy = "allow"
	// NetAsk defers the deny/allow decision to an interactive TTY prompt.
	// Non-TTY sessions hard-deny and exit non-zero (agents cannot auto-accept).
	NetAsk NetPolicy = "ask"
)

// SandboxMode controls the strictness of sandbox enforcement.
type SandboxMode string

const (
	// ModeOff disables the sandbox entirely — the command runs with no OS-level
	// restrictions. Use only when the operator explicitly accepts the risk.
	ModeOff SandboxMode = "off"
	// ModeBestEffort enables the sandbox when OS primitives are available and
	// falls back to exec-ro classification + a LOUD warning when they are not.
	ModeBestEffort SandboxMode = "best-effort"
	// ModeStrict requires OS primitives — if unavailable, fail-closed (exit
	// non-zero, do not run the command).
	ModeStrict SandboxMode = "strict"
)

// Profile describes the filesystem and network constraints for a sandboxed
// command. It is serialized to environment variables for the child trampoline.
type Profile struct {
	// RODirs are directories accessible read-only (visible, readable, but
	// writes return EACCES).
	RODirs []string
	// RWDirs are directories accessible read-write (readable AND writable).
	RWDirs []string
	// RWFiles are individual files accessible read-write. Used for device files
	// like /dev/null that CLI tools open O_RDWR (writes to /dev/null are
	// discarded by the kernel — no security concern).
	RWFiles []string
	// Net controls network access at the syscall layer.
	Net NetPolicy
}

// DefaultProfile returns Profile B: read the repo root and system/tool paths;
// write ONLY ./tmp/; .git inherits read-only from the repo root (Landlock is
// additive — a subpath cannot be less restrictive than its parent in one
// layer); sibling repos and home-sensitive paths are denied (not in the
// ruleset → completely inaccessible, EACCES on open/stat).
//
// /dev is included as read-only because /dev/null is required by virtually
// every CLI tool (bash redirection, git, etc.). Reading device nodes still
// requires CAP_SYS_RAWIO which the sandboxed process lacks (NoNewPrivs +
// no effective capabilities), so /dev as RO is safe in practice.
func DefaultProfile(repoRoot string) Profile {
	return Profile{
		RODirs: []string{
			repoRoot,
			"/usr",
			"/bin",
			"/sbin",
			"/lib",
			"/lib64",
			"/lib32",
			"/etc",
			"/dev",
		},
		RWDirs: []string{
			filepath.Join(repoRoot, "tmp"),
		},
		RWFiles: []string{
			"/dev/null",
		},
		Net: NetDeny,
	}
}
