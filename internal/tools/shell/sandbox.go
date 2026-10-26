package shell

import (
	"os/exec"
)

// SandboxFunc is the injectable confinement seam: it receives the fully
// constructed child command (bash argv, explicit scrubbed env, workdir,
// process-group attrs already applied) BEFORE it starts, and may
// transform it in place — rewrapping argv to prefix a confinement
// runner (firejail/bwrap/docker compose exec/…), tightening attrs, or
// refusing the run by returning an error (classified CauseError, the
// command never executes).
//
// The default is nil, and nil means NO CONFINEMENT: the command runs
// with the engine process's own privileges. This is deliberate and
// loudly documented — the Pipeline's guards/waterfall are the policy
// layer; wave B wires real confinement behind this seam. Whatever
// rewrapping is done MUST preserve the invariant that the executed
// leader stays a process-group leader, or timeout teardown degrades to
// killing only the wrapper.
type SandboxFunc func(cmd *exec.Cmd) error
