// Package shell implements the run_shell tool: native subprocess
// execution behind the slice-3 tool Pipeline, with orthogonal outcome
// facts, explicit child-environment hygiene, output caps, and an
// injectable sandbox seam. The optional background:true arg (P6,
// background.go) dispatches the SAME execution path as a durable jobs
// job instead of running synchronously — the tool returns an enqueue
// receipt and the output streams to the job's capture channel.
//
// # Platform assumption (unix)
//
// This package is unix-only: command construction places the child in
// its own process group (Setpgid) and timeout teardown kills the whole
// group with a negative-pid signal. Windows is a NON-GOAL; the build
// constraint on exec_unix.go makes the package fail loudly (no Go
// files) on windows rather than silently degrading. The shell is
// invoked as bash -c (non-interactive, --noprofile --norc): no history
// is read or written, no job control is set up, and no startup files
// run (the explicit child env never carries BASH_ENV).
//
// # Sandbox: nil means NO CONFINEMENT — LOUD DEFAULT
//
// Config.Sandbox is the confinement seam. The default is nil, and nil
// means the command runs with the ENGINE PROCESS'S OWN privileges on
// the host: same uid, same filesystem reach, same network. That is a
// deliberate, documented default — the policy layer (Pipeline guards,
// pre-execute waterfall, approval) is the intended safety boundary —
// and it is exactly what mode "off" preserves. Every Outcome records
// which sandbox applied (Config.SandboxName, default "none") so a
// logged run never hides its confinement level.
//
// # Real confinement (sandbox_modes.go)
//
// NewSandboxFunc adapts SandboxOptions{Mode: read-only |
// workspace-write, WritableRoots} onto the repo's REAL kernel
// confinement backend (internal/execsandbox: Landlock filesystem
// integrity + seccomp-BPF network/syscall hardening) by REWRAPPING the
// constructed command as the backend's re-exec trampoline child:
// [self, __exec_sandbox_child, --, bash, ...] plus the serialized
// profile in the child env. The caller-owned invariants (captured
// streams, scrubbed env, process-group leadership for timeout
// teardown) are preserved verbatim by the rewrap. The backend
// fail-closes: when the OS primitives are unavailable a sandboxed call
// returns a typed *SandboxUnavailableError (an isError tool result;
// NEVER a silently unconfined run). Runtime denials (a confined write
// hitting EACCES) classify honestly as ordinary non-zero exits with
// the kernel diagnostic on stderr — the orthogonal outcome facts stay
// intact. Confinement is an INTEGRITY + NETWORK boundary (network is
// denied under either confining mode), not a confidentiality boundary.
// The trampoline host must dispatch the hidden __exec_sandbox_child
// verb BEFORE its own argument parsing (the vh-agentd daemon and the
// test binaries do; execsandbox.TrampolineVerb is the verb constant).
//
// # Existing runtime seams (integration decision — LANDED)
//
// internal/runtime was originally studied for reuse (runtime.Runner,
// runtime.Backend). Both proved CLI-verb-shaped — host-stdio wiring,
// error-only returns, no Env field, no SysProcAttr/process-group path
// — and carry NO confinement primitives at all; the real backends live
// in internal/execsandbox. The A2 decision ("seam only; wave B adapts")
// is settled as sandbox_modes.go: the SandboxFunc signature is
// unchanged, and the adaptation happens through execsandbox's narrow
// programmatic surface (WrapCommand/ProfileEnv/TrampolineVerb) rather
// than runtime.RunOpts.
//
// # Non-goals (recorded follow-ups)
//
//   - Oversize output spills to a session-adjacent file: NON-GOAL here;
//     output beyond MaxCapturedBytes is dropped with an in-band marker.
//   - Windows support: NON-GOAL (see platform assumption above).
package shell
