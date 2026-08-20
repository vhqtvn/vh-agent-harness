// Package shell implements the run_shell tool: native subprocess
// execution behind the slice-3 tool Pipeline, with orthogonal outcome
// facts, explicit child-environment hygiene, output caps, and an
// injectable sandbox seam.
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
// pre-execute waterfall, approval) is the intended safety boundary,
// and wave B wires real confinement behind the same seam. Every
// Outcome records which sandbox applied (Config.SandboxName, default
// "none") so a logged run never hides its confinement level.
//
// # Existing runtime seams (integration decision)
//
// internal/runtime was studied for reuse (runtime.Runner,
// runtime.Backend, the bare backend). Both are importable without
// import cycles, but they are CLI-verb-shaped: host-stdio wiring,
// error-only returns, no Env field, and no SysProcAttr/process-group
// path — none of the orthogonal outcome facts, env hygiene, or group
// kill this tool requires can be expressed through them. Adapting them
// would either silently drop those guarantees or fake them. Decision:
// slice A2 ships the SandboxFunc seam only; wave B should either
// extend runtime.RunOpts (Env + SysProcAttr + captured streams) or
// introduce a model-facing exec surface in internal/runtime and adapt
// it here behind the unchanged SandboxFunc signature.
//
// # Non-goals (recorded follow-ups)
//
//   - Oversize output spills to a session-adjacent file: NON-GOAL here;
//     output beyond MaxCapturedBytes is dropped with an in-band marker.
//   - Windows support: NON-GOAL (see platform assumption above).
package shell
