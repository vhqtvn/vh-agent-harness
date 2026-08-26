// background.go — run_shell's background:true path (P6 job tailing):
// instead of executing synchronously, the tool body VALIDATES the call
// exactly like the sync path, then dispatches one durable job (kind
// "shell") whose body runs the SAME exec core (RunStreamed: env scrub,
// sandbox WrapCommand, workdir policy, process-group teardown — reused,
// never duplicated) and returns the enqueue receipt {jobId} as the tool
// result immediately. The turn never blocks; output streams into the
// jobs capture seam (internal/jobs output.go); settlement carries the
// exit facts in job/settled Detail and job/report notifies the model.
//
// TIMEOUT POSTURE (deliberate, disclosed): the SAME per-call timeout
// vocabulary and hard cap apply — timeout_ms clamps to MaxTimeoutMs
// (default 600000) — but an OMITTED timeout defaults to the CAP, not
// the sync 30s default: background dispatch signals long-running
// intent, and surprising the model with a 30s kill would defeat the
// feature. The cap itself is unchanged: a background job is bounded by
// the same per-call ceiling as a sync one (never an unbounded process).
// On expiry the process group is killed by the identical teardown and
// the job settles failed with the timeout reason.
//
// SANDBOX POSTURE: a configured sandbox applies inside the job body
// exactly as in the sync path (fail-closed per call, never a silently
// unconfined run). Additionally the dispatch is REFUSED (typed, no job
// created) when the confinement backend is unavailable — a pre-flight
// probe wrap on a fully-built throwaway command catches the
// environmental unavailable class before a job exists; per-call setup
// failures inside the job still fail closed there (the job settles
// failed, never unconfined).
package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// BackgroundKind is the jobs kind of background shell jobs (job ids
// `shell-N`, per-kind monotonic on the session's log).
const BackgroundKind = "shell"

// BackgroundArgs is the FROZEN job payload of a background run_shell
// call: the validated command, the RESOLVED effective timeout (the job
// body enforces it without re-resolving), and the admitted workdir.
type BackgroundArgs struct {
	Command   string `json:"command"`
	TimeoutMs int64  `json:"timeoutMs"`
	Workdir   string `json:"workdir,omitempty"`
}

// BackgroundDispatch is the seam the tool body resolves when
// background:true: it enqueues one validated job (kind "shell", payload
// = the frozen BackgroundArgs) onto the EXECUTING session's jobs
// manager and returns the job id. The daemon wires this through its
// session→manager registry (cmd/vh-agentd); nil keeps run_shell
// synchronous-only and a background:true call fails closed with a
// typed error.
type BackgroundDispatch func(ctx context.Context, args BackgroundArgs) (jobID string, err error)

// backgroundReceipt is the tool-result content of a background
// dispatch: what the model sees instead of the sync Outcome. It is
// deterministic (no durations) so the durable tool/result replays
// byte-identically.
type backgroundReceipt struct {
	Background         bool   `json:"background"`
	JobID              string `json:"jobId"`
	Command            string `json:"command"`
	EffectiveTimeoutMs int64  `json:"effectiveTimeoutMs"`
	// Note is the model-facing retrieval hint: the tail is host-side
	// (jobs/output on the wire), settlement arrives as job/report.
	Note string `json:"note,omitempty"`
}

const backgroundNote = "output streams to the job's capture channel; settlement arrives as a job report; tail via jobs/output"

// resolveBackgroundTimeout maps timeout_ms onto the background
// deadline: omitted/nil/0 ⇒ the configured CAP (not the sync 30s
// default — see the timeout posture above); explicit values clamp to
// the cap; negative is invalid.
func resolveBackgroundTimeout(cfg *Config, timeoutMs *int64) (int64, error) {
	if timeoutMs == nil || *timeoutMs == 0 {
		return cfg.MaxTimeoutMs, nil
	}
	if *timeoutMs < 0 {
		return 0, fmt.Errorf("run_shell: timeout_ms must be >= 0, got %d", *timeoutMs)
	}
	if *timeoutMs > cfg.MaxTimeoutMs {
		return cfg.MaxTimeoutMs, nil
	}
	return *timeoutMs, nil
}

// sandboxPreflight probes the configured confinement on a fully-built
// THROWAWAY command (identical argv/env/workdir/process-group attrs; a
// wrap mutates only the cmd, which is discarded, never started). It
// surfaces the typed unavailable class at dispatch time so a
// background:true call fails closed BEFORE a job exists — never a job
// that silently runs unconfined.
func sandboxPreflight(cfg *Config, command, workdir string, timeoutMs int64) error {
	if cfg.Sandbox == nil {
		return nil // off is the loud documented default; nothing to probe
	}
	cmd := newCommand(context.Background(), cfg, command)
	cmd.Dir = workdir
	cmd.Env = buildEnv(cfg)
	applyProcessGroup(cmd)
	if err := cfg.Sandbox(cmd); err != nil {
		return fmt.Errorf("run_shell: background dispatch refused: sandbox %q unavailable at dispatch: %w", cfg.sandboxLabel(), err)
	}
	return nil
}

// executeBackground is the background:true branch of the tool body.
// It shares the sync path's validation (command/workdir/policy) and
// differs only after it: resolve the background deadline, preflight
// the sandbox, dispatch, return the receipt.
func executeBackground(ctx context.Context, cfg *Config, a Args) (string, error) {
	if cfg.Background == nil {
		return "", errors.New("run_shell: background:true is not available in this wiring (no background dispatcher configured; fail-closed)")
	}
	timeoutMs, err := resolveBackgroundTimeout(cfg, a.TimeoutMs)
	if err != nil {
		return "", err
	}
	if err := sandboxPreflight(cfg, a.Command, a.Workdir, timeoutMs); err != nil {
		var unavail *SandboxUnavailableError
		if errors.As(err, &unavail) {
			return "", unavail // typed fail-closed: refuse dispatch, never unconfined
		}
		return "", err
	}
	jobID, err := cfg.Background(ctx, BackgroundArgs{
		Command:   a.Command,
		TimeoutMs: timeoutMs,
		Workdir:   a.Workdir,
	})
	if err != nil {
		return "", fmt.Errorf("run_shell: background dispatch failed: %w", err)
	}
	content, merr := json.Marshal(backgroundReceipt{
		Background:         true,
		JobID:              jobID,
		Command:            a.Command,
		EffectiveTimeoutMs: timeoutMs,
		Note:               backgroundNote,
	})
	if merr != nil {
		return "", fmt.Errorf("run_shell: marshal background receipt: %w", merr)
	}
	return string(content), nil
}

// DecodeBackgroundArgs strictly decodes a job payload back into the
// frozen BackgroundArgs (the daemon executor side). Unknown fields are
// rejected — the payload is engine-authored, so anything else is a
// writer bug.
func DecodeBackgroundArgs(payload []byte) (BackgroundArgs, error) {
	var a BackgroundArgs
	if len(payload) == 0 {
		return a, errors.New("run_shell: background job payload is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("run_shell: invalid background job payload: %w", err)
	}
	if a.Command == "" {
		return a, errors.New("run_shell: background job payload missing command")
	}
	return a, nil
}
