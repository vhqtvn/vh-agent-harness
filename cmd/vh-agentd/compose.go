// compose.go — the daemon's engine assembly: real adapter (selected by
// flag, with the optional Anthropic cache mapping), the slice-5
// FileEngine over the explicit session dir (wrapped in a sessionTracker
// so the daemon-level scheduler reaches the active jobs.Manager), the
// protocol server with the wire approval bridge, the dogfood tool set
// (echo + clock + run_shell — the real shell tool from slice 6), the
// compiled-sysprompt serving rule wired into TurnOptions.System, and the
// durable retry ladder armed on every prompt turn. Everything here wires
// REAL implementations — there is deliberately no second, fake
// composition (engine.go contract).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/anthropic"
	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/loop"
	"github.com/vhqtvn/vh-agent-harness/internal/prompt"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/filetools"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/shell"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/spillread"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/subagenttools"
)

// buildAdapter selects and constructs the real adapter from validated
// config. The API key arrives here by injection only (read from the
// named env var in run) — adapters never read the environment
// themselves. --cache-breakpoints N (1..4) maps onto the anthropic
// CacheConfig; openaicompat was already rejected at validation
// (implicit caching — no knob).
func buildAdapter(cfg *Config, apiKey string) adapters.Adapter {
	switch cfg.Adapter {
	case "anthropic":
		return anthropic.New(anthropic.Config{
			BaseURL:   cfg.BaseURL,
			Model:     cfg.Model,
			APIKey:    apiKey,
			MaxTokens: cfg.MaxTokens,
			Cache: anthropic.CacheConfig{
				Enabled:        cfg.CacheBreakpoints > 0,
				MaxBreakpoints: cfg.CacheBreakpoints,
			},
		})
	default: // validate() normalized everything else to openaicompat
		return openaicompat.New(openaicompat.Config{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  apiKey,
		})
	}
}

// buildServer assembles the daemon's protocol surface over rwc:
//
//  1. the FileEngine (bare — no pipeline built yet), wrapped in the
//     sessionTracker (the scheduler's window onto the active session),
//  2. protocol.NewServer, which injects the wire approval bridge into
//     the engine (ApprovalAwareEngine.SetApprover),
//  3. ONLY THEN tool registration on the engine's lazily-built Pipeline
//     — the pipeline freezes its decision lattice (approver included) at
//     construction, so an early-built pipeline would silently miss the
//     bridge (the slice-5 composition-order trap).
//
// Approvals therefore surface on the wire and fail closed on absence,
// timeout, or disconnect (host-protocol.md §6). The turn options carry
// the SERVED system prompt (compiled artifact when present, raw
// assembly otherwise — resolveSystemPrompt; the ServeResult returns to
// the caller so the startup log records the source) and the ARMED
// retry ladder (loop defaults: 2 retries, 500ms→10s backoff,
// deterministic mid-band jitter).
//
// The schedule/* seam (FileEngine.Schedules) is the ONE deliberate
// post-build assignment: run() sets it after buildScheduler (the
// scheduler needs the tracker, which needs the engine) and BEFORE
// Serve, so every session/create stamps it — see sched.go.
func buildServer(cfg *Config, apiKey string, rwc io.ReadWriteCloser) (*protocol.Server, *protocol.FileEngine, *sessionTracker, prompt.ServeResult) {
	// The session→manager registry backs the MODEL-FACING subagent tool
	// family: the engine binds root managers here (create/supersede),
	// the child-turn executor binds per-turn child managers, and the
	// tool bodies resolve the executing session's manager through it.
	reg := subagents.NewRegistry()
	defs := daemonTools(realNow, cfg, reg)
	specs := make([]adapters.ToolSpec, 0, len(defs))
	for _, d := range defs {
		specs = append(specs, d.Spec())
	}

	engine := &protocol.FileEngine{
		Dir:      cfg.SessionDir,
		Executor: daemonExecutor{},
		Ad:       buildAdapter(cfg, apiKey),
		TurnOpts: tools.TurnOptions{
			Model:     cfg.Model,
			Tools:     specs,
			MaxTokens: cfg.MaxTokens,
			Retry:     &tools.RetryLadder{Config: loop.RetryConfig{}},
		},
	}
	// Oversize tool-result spill (dsh spill pattern): arm a per-session
	// policy rooted at <session-dir>/<session-id>.spill/. 0 disables it
	// (always-inline, the pre-spill behavior); a store construction
	// cannot fail (the directory is created lazily at first write).
	if cfg.SpillMaxInline > 0 {
		engine.SpillPolicyFor = func(sessionID string) *session.SpillPolicy {
			return &session.SpillPolicy{
				MaxInlineBytes: cfg.SpillMaxInline,
				Store:          session.NewFileSpillStore(cfg.SessionDir, sessionID),
			}
		}
	}
	tracker := &sessionTracker{Engine: engine}

	// B2 + child-of-child: arm the subagent surface — the REAL executor
	// (child turns run through this same pipeline + adapter + retry
	// ladder once the approval bridge is attached below, and any child
	// below the depth fence runs with the same model-facing spawn
	// capability), child logs under <session-dir>/subagents, and the
	// registry the spawn tools resolve through.
	wireSubagents(engine, cfg.SessionDir, reg)

	// Serving rule: compiled bytes when a matching artifact exists,
	// raw assembly otherwise. A fallback is never silent — the
	// ServeResult returns to the caller (run() logs the source at
	// startup). Assembly/hash failures are config bugs (strict
	// interpolation, catalog validation): fail loudly at startup
	// rather than serve a silently wrong prompt.
	sys, served, err := resolveSystemPrompt(cfg, specs)
	if err != nil {
		panic(fmt.Sprintf("vh-agentd: resolve system prompt: %v", err))
	}
	engine.TurnOpts.System = sys

	srv := protocol.NewServer(tracker, protocol.NewConn(rwc), protocol.ServerOptions{
		ApprovalTimeoutMs: cfg.ApprovalTimeoutMs,
	})

	// AFTER NewServer (approval bridge attached): register the tools.
	for _, d := range defs {
		if err := engine.Pipeline().Register(d); err != nil {
			// Duplicate/invalid definitions are a programming error in
			// this fixed tool set; fail loudly at startup.
			panic(fmt.Sprintf("vh-agentd: register tool %s: %v", d.Name, err))
		}
	}
	return srv, engine, tracker, served
}

// toolSpecsForPrompt returns the advertised tool specs for the OFFLINE
// compile path (--compile-prompt): the catalog must describe the same
// tool set the serving daemon advertises, so the artifact hash matches.
// The registry is per-call (specs only — the tool BODIES are never
// executed on this path).
func toolSpecsForPrompt(cfg *Config) []adapters.ToolSpec {
	defs := daemonTools(realNow, cfg, subagents.NewRegistry())
	specs := make([]adapters.ToolSpec, 0, len(defs))
	for _, d := range defs {
		specs = append(specs, d.Spec())
	}
	return specs
}

// daemonTools returns the daemon's tool set: the read-only dogfood
// probes (echo, clock), the REAL run_shell tool from
// internal/tools/shell, spill_read — the retrieval path for spilled
// oversize results (rooted at the session dir so it reaches every
// session's store; see internal/tools/spillread) — the MODEL-FACING
// subagent family (subagent_spawn/subagent_send, resolved through the
// session registry reg; advertised per-session while below the
// delegation-depth fence — the ROOT session, at depth 0, always is;
// see internal/tools/subagenttools and cmd/vh-agentd/subagents.go for
// the depth-conditional advertising on child turns) — and the
// MODEL-FACING FILE FAMILY (read/write/edit/glob/search from
// internal/tools/filetools), confined to the same WorkdirRoots that
// run_shell's absolute-workdir policy consults (--workdir-roots;
// default = the daemon's working directory resolved absolute — one
// root set for every path-taking surface). Config posture
// (documented defaults):
//
//   - policy lists default-EMPTY: AllowedCommands/DeniedCommands are
//     unset — in-tool hygiene is opt-in; the Pipeline guards/approval
//     waterfall are the policy layer (typed denial provenance), and
//     run_shell is IsConcurrencySafe=false so the scheduler drains the
//     parallel pool around it (exclusive barrier);
//   - sandbox off (the default): Config.Sandbox nil = NO CONFINEMENT —
//     the loud, deliberate pre-slice posture (see internal/tools/shell
//     doc.go: "none" is recorded in every Outcome so logs never hide
//     the confinement level; the host's guard/approval policy is the
//     safety boundary);
//   - sandbox read-only / workspace-write: the kernel confinement
//     backend (Landlock+seccomp via the execsandbox trampoline) is
//     armed behind the SandboxFunc seam, labeled with the mode, and
//     fail-closed per call when the OS primitives are unavailable
//     (typed sandbox-unavailable error; never a silently unconfined
//     run). workspace-write writable roots are the validated
//     Config.SandboxWritableRoots (session dir + OS temp);
//   - EnvAllowlist empty: the child env is the explicit base set
//     (PATH/HOME/TERM/LANG) — default-deny, scrubbed;
//   - WorkdirRoots = Config.WorkdirRoots (--workdir-roots, default
//     the daemon cwd): run_shell's ABSOLUTE workdirs are admitted
//     exactly under these roots (symlink-safe; the pre-file-family
//     default of rejecting every absolute workdir was the
//     empty-roots posture — a daemon configuring roots EXTENDS what
//     is allowed). Relative run_shell workdirs keep their
//     inside-the-engine-cwd behavior unchanged.
//
// now is injected for the deterministic clock tool.
func daemonTools(now func() time.Time, cfg *Config, reg *subagents.Registry) []tools.ToolDefinition {
	shellCfg := shell.Config{}
	// File-family roots: cfg is nil only for spec-only callers (the
	// offline --compile-prompt catalog); the tool BODIES never run
	// there, so the process cwd is a harmless stand-in that keeps the
	// descriptions/schemas identical.
	fileRoots := []string{}
	if cfg != nil {
		shellCfg.WorkdirRoots = cfg.WorkdirRoots
		fileRoots = cfg.WorkdirRoots
	} else {
		if cwd, err := os.Getwd(); err == nil {
			fileRoots = []string{cwd}
		}
	}
	if cfg != nil && cfg.SandboxMode != shell.SandboxOff {
		fn, err := shell.NewSandboxFunc(shell.SandboxOptions{
			Mode:          cfg.SandboxMode,
			WritableRoots: cfg.SandboxWritableRoots,
		})
		if err != nil {
			// A validated mode cannot fail construction; this is a
			// programming error in the daemon wiring — fail loudly.
			panic(fmt.Sprintf("vh-agentd: sandbox %s: %v", cfg.SandboxMode, err))
		}
		shellCfg.Sandbox = fn
		shellCfg.SandboxName = string(cfg.SandboxMode)
	}
	// spill_read retrieves from the session dir root (it has no session
	// context; content addressing + hash validation make the walk
	// exact). The ACTIVE policy's inline cap threads through as the
	// window bound, so every retrieval result fits inline by
	// construction. A nil cfg (spec-only callers) gets "" = the process
	// cwd and cap 0 = the default window.
	spillRoot := ""
	spillInline := int64(0)
	if cfg != nil {
		spillRoot = cfg.SessionDir
		spillInline = cfg.SpillMaxInline
	}
	defs := []tools.ToolDefinition{
		{
			Name: "echo", Description: "Echoes the given text back as the tool result (read-only dogfood probe).",
			Parameters:        json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","description":"text to echo back"}},"required":["text"],"additionalProperties":false}`),
			IsConcurrencySafe: true,
			TimeoutMs:         5000,
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				if len(args) == 0 {
					return "", errors.New("echo: args.text is required and must be non-empty")
				}
				var a struct {
					Text string `json:"text"`
				}
				dec := json.NewDecoder(bytes.NewReader(args))
				dec.DisallowUnknownFields()
				if err := dec.Decode(&a); err != nil {
					return "", fmt.Errorf("echo: invalid args: %w", err)
				}
				if a.Text == "" {
					return "", errors.New("echo: args.text is required and must be non-empty")
				}
				return a.Text, nil
			},
		},
		{
			Name:              "clock",
			Description:       "Returns the daemon's current UTC time (RFC 3339; read-only dogfood probe).",
			Parameters:        json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			IsConcurrencySafe: true,
			TimeoutMs:         5000,
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				if len(args) > 0 {
					var probe struct{}
					dec := json.NewDecoder(bytes.NewReader(args))
					dec.DisallowUnknownFields()
					if err := dec.Decode(&probe); err != nil {
						return "", fmt.Errorf("clock: invalid args: %w", err)
					}
				}
				return now().UTC().Format(time.RFC3339Nano), nil
			},
		},
		spillread.Definition(spillRoot, spillInline),
		shell.Definition(shellCfg),
	}
	defs = append(defs, filetools.Definitions(filetools.Config{Roots: fileRoots})...)
	defs = append(defs, subagenttools.Definitions(reg)...)
	return defs
}

// daemonExecutor is the minimal dogfood jobs.Executor: deterministic,
// no-subprocess bodies for the kinds the dogfood protocol exercises;
// every unknown kind settles failed (fail-closed — the executor is the
// seam where a real runtime attaches in a later slice).
type daemonExecutor struct{}

// Run executes one job body. "echo" settles completed; "fail" settles
// failed (exercising the failed settlement path over the wire);
// anything else is refused.
func (daemonExecutor) Run(ctx context.Context, job jobs.Job) error {
	switch job.Kind {
	case "echo":
		return nil
	case "fail":
		return errors.New("fail: requested failure")
	default:
		return fmt.Errorf("vh-agentd: unknown job kind %q (no executor registered; fail-closed)", job.Kind)
	}
}
