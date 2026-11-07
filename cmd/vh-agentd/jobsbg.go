// jobsbg.go — the daemon's P6 background-shell wiring: the session→
// jobs.Manager registry the run_shell background dispatcher resolves
// through, the shared shell.Config construction (ONE config, used by
// BOTH the model-facing tool body and the job executor — the exec path
// is reused, never duplicated), and the "shell" job kind executor that
// streams combined child output into the job's capture channel and
// settles with compact exit facts as the terminal Detail.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/shell"
)

// jobsRegistry maps session id → that session's jobs dispatcher (the
// concrete *jobs.Manager), the seam the run_shell background
// dispatcher resolves the EXECUTING session's manager through
// (tools.WithExecutingSession in the turn context — the subagenttools
// pattern, so a CHILD session's background dispatch lands on the
// child's own log, never the root's). Root managers are registered by
// the sessionTracker at create/resume; child managers by the subagent
// turn executor for the duration of each child turn. Thread-safe;
// entries live as long as the daemon (superseded sessions stay
// resumable).
type jobsRegistry struct {
	mu map[string]protocol.JobDispatcher // guarded by sync.Mutex below
	sync.Mutex
}

func newJobsRegistry() *jobsRegistry {
	return &jobsRegistry{mu: make(map[string]protocol.JobDispatcher)}
}

func (r *jobsRegistry) Put(sessionID string, m protocol.JobDispatcher) {
	r.Lock()
	defer r.Unlock()
	r.mu[sessionID] = m
}

func (r *jobsRegistry) Get(sessionID string) (protocol.JobDispatcher, bool) {
	r.Lock()
	defer r.Unlock()
	m, ok := r.mu[sessionID]
	return m, ok
}

func (r *jobsRegistry) Remove(sessionID string) {
	r.Lock()
	defer r.Unlock()
	delete(r.mu, sessionID)
}

// dispatcher returns the shell.BackgroundDispatch closure resolving the
// executing session's manager. Fail-closed postures: no executing
// session in the context (tool invoked outside a turn) and no manager
// bound (unknown session) are typed errors — never a fallback onto
// another session's log.
func (r *jobsRegistry) dispatcher() shell.BackgroundDispatch {
	return func(ctx context.Context, a shell.BackgroundArgs) (string, error) {
		sessionID := tools.ExecutingSessionFrom(ctx)
		if sessionID == "" {
			return "", fmt.Errorf("no executing session in tool context (background run_shell runs inside turns only)")
		}
		m, ok := r.Get(sessionID)
		if !ok {
			return "", fmt.Errorf("no jobs manager bound to session %q (background dispatch not armed for this session)", sessionID)
		}
		payload, err := json.Marshal(a)
		if err != nil {
			return "", fmt.Errorf("marshal background payload: %w", err)
		}
		receipt, err := m.Dispatch(shell.BackgroundKind, payload)
		if err != nil {
			return "", err
		}
		return receipt.JobID, nil
	}
}

// bindSession registers (or replaces) the session's dispatcher — called
// by the sessionTracker on NewSession/ResumeSession.
func (r *jobsRegistry) bindSession(es *protocol.EngineSession, sessionID string) {
	if es != nil && es.Jobs != nil {
		r.Put(sessionID, es.Jobs)
	}
}

// countingWriter counts the bytes streamed into the capture channel
// (the terminal Detail reports the produced volume).
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// daemonExecutor is the daemon's jobs.Executor (replacing the former
// echo/fail-only zero struct): the deterministic dogfood kinds ("echo"
// settles completed, "fail" settles failed) plus the "shell" kind —
// the background body of run_shell background:true. It implements
// jobs.OutputExecutor so every kind runs with the job's output writer
// (the dogfood kinds simply never write).
//
// Shell settlement mapping (disclosed posture): exit and signal causes
// — including NON-ZERO exits — are NORMAL command outcomes (the sync
// tool's classification), so the job settles completed with the facts
// in the terminal Detail; timeout settles failed with the timeout
// reason; spawn-class failures (sandbox setup, missing shell) settle
// failed. The job NEVER settles failed on the command's own stderr.
type daemonExecutor struct {
	shell shell.Config
}

// detailLine renders the compact terminal facts. Deterministic field
// order; durationMs is wall-clock (log CONTENT varies across runs, but
// replay of a written log is byte-stable — the battery never compares
// logs across sessions).
func detailLine(o shell.Outcome, outputBytes int64) string {
	d := fmt.Sprintf("cause=%s", o.Cause)
	switch o.Cause {
	case shell.CauseExit:
		d += fmt.Sprintf(" exitCode=%d", o.ExitCode)
	case shell.CauseSignal:
		d += fmt.Sprintf(" signal=%s", o.Signal)
	}
	d += fmt.Sprintf(" durationMs=%d outputBytes=%d sandbox=%s", o.DurationMs, outputBytes, o.Sandbox)
	return d
}

// Run satisfies the base Executor (the manager prefers RunWithOutput;
// the base method keeps the type usable as a plain Executor).
func (e daemonExecutor) Run(ctx context.Context, job jobs.Job) error {
	_, err := e.RunWithOutput(ctx, job, io.Discard)
	return err
}

// RunWithOutput executes one job body, streaming progressive output
// into out for output-producing kinds. Every unknown kind settles
// failed (fail-closed — the executor is the seam where further real
// runtimes attach).
func (e daemonExecutor) RunWithOutput(ctx context.Context, job jobs.Job, out io.Writer) (string, error) {
	switch job.Kind {
	case "echo":
		return "", nil
	case "fail":
		return "", errors.New("fail: requested failure")
	case shell.BackgroundKind:
		return e.runShell(ctx, job, out)
	default:
		return "", fmt.Errorf("vh-agentd: unknown job kind %q (no executor registered; fail-closed)", job.Kind)
	}
}

// runShell is the "shell" job body: decode the frozen BackgroundArgs,
// run the SAME exec core streamed into the job's capture channel, and
// return the compact exit-facts Detail.
func (e daemonExecutor) runShell(ctx context.Context, job jobs.Job, out io.Writer) (string, error) {
	a, err := shell.DecodeBackgroundArgs(job.Payload)
	if err != nil {
		return "", err
	}
	counted := &countingWriter{w: out}
	o := shell.RunStreamed(ctx, &e.shell, a.Command, a.TimeoutMs, a.Workdir, counted)
	switch o.Cause {
	case shell.CauseExit, shell.CauseSignal:
		return detailLine(o, counted.n), nil
	case shell.CauseTimeout:
		return detailLine(o, counted.n), &shell.TimeoutError{
			EffectiveTimeoutMs: o.EffectiveTimeoutMs,
		}
	default: // CauseError: spawn class (sandbox setup failure, missing shell, ...)
		return detailLine(o, counted.n), fmt.Errorf("run_shell background: spawn failed: %s", o.SpawnError)
	}
}
