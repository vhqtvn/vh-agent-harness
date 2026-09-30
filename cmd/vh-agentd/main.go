// vh-agentd is the minimal real entrypoint of the native engine: it
// composes the slice-5 FileEngine (real adapters, real session logs,
// real jobs manager, real tool pipeline with the wire approval bridge)
// over stdio using the host protocol, making the program runnable
// end-to-end from an external client (netcat/jq or the reference
// protocol.Client).
//
// Composition-order contract (the slice-5 trap, see internal/protocol
// engine.go): the FileEngine is built bare, handed to protocol.NewServer
// (which injects the wire approval bridge via SetApprover), and ONLY
// THEN are tools registered on the engine's lazily-built Pipeline — the
// pipeline freezes its decision lattice (approver included) at
// construction.
//
// stdout is protocol (dsh purity): everything else the daemon says goes
// to stderr.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/shell"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, stdioConn{}, os.Stdout, os.Stderr))
}

// trampolineArgs reports whether args is a sandbox trampoline child
// invocation ([__exec_sandbox_child, --?, target, args...]) and returns
// the payload argv (verb + -- separator stripped) for
// execsandbox.RunChild. The check happens BEFORE flag parsing: the
// trampoline child carries the confined command's argv, not daemon
// flags. This is the daemon-side twin of the CLI's hidden cobra verb —
// run_shell confinement re-execs THIS executable as the trampoline
// host, so a sandboxed daemon can confine its own child commands.
func trampolineArgs(args []string) (rest []string, handled bool) {
	if len(args) == 0 || args[0] != execsandbox.TrampolineVerb {
		return nil, false
	}
	rest = args[1:]
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	return rest, true
}

// stdioConn adapts the process stdio to the protocol Conn's
// ReadWriteCloser seam. Close closes only the write side (stdout); the
// read side belongs to the parent.
type stdioConn struct{}

func (stdioConn) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdioConn) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdioConn) Close() error                { return os.Stdout.Close() }

// run is the testable entry core: it parses args, validates fail-closed,
// assembles the engine, and serves the protocol on rwc until EOF or
// ctx cancellation. It returns the process exit code:
//
//	0 — clean shutdown (EOF, signal, --version, --help)
//	2 — usage / validation failure (message on stderr)
//	1 — runtime failure (message on stderr)
func run(args []string, getenv func(string) string, rwc io.ReadWriteCloser, stdout, stderrw io.Writer) (exit int) {
	// Sandbox trampoline dispatch (BEFORE flag parsing — see
	// trampolineArgs). RunChild installs NoNewPrivs + seccomp +
	// landlock and syscall.Execs into the confined target; it returns
	// only on failure.
	if rest, handled := trampolineArgs(args); handled {
		if err := execsandbox.RunChild(rest); err != nil {
			fmt.Fprintf(stderrw, "vh-agentd: sandbox child: %v\n", err)
			return 1
		}
		return 0 // unreachable on success: the image was replaced
	}

	log := newLogger(stderrw)

	var (
		adapter           = new(string)
		model             = new(string)
		baseURL           = new(string)
		apiKeyEnv         = new(string)
		sessionDir        = new(string)
		optimizer         = new(string)
		maxTokens         = new(int)
		approvalTimeoutMs = new(int)
		cacheBreakpoints  = new(int)
		sandbox           = new(string)
		spillMaxInline    = new(int64)
		workdirRoots      = new(string)
		askTools          = new(string)
		skillsDir         = new(string)
		contextTokens     = new(int)
		compactThreshold  = new(float64)
		mcpConfig         = new(string)
		mcpTimeoutMs      = new(int)
		mcpAutoAllow      = new(bool)
		compilePrompt     = new(bool)
		verifyLog         = new(string)
		showVersion       = new(bool)
	)
	fs := flag.NewFlagSet("vh-agentd", flag.ContinueOnError)
	fs.SetOutput(stderrw)
	fs.Usage = func() { fmt.Fprint(stderrw, usageDoc) }
	fs.StringVar(adapter, "adapter", "", "LLM adapter: openai|openaicompat|anthropic (required)")
	fs.StringVar(model, "model", "", "model name sent with each request (required)")
	fs.StringVar(baseURL, "base-url", "", "absolute http(s) base URL of the provider endpoint (required)")
	fs.StringVar(apiKeyEnv, "api-key-env", "", "NAME of the environment variable holding the API key (required; never a literal key)")
	fs.StringVar(sessionDir, "session-dir", "", "directory for durable session logs (required; no default)")
	fs.StringVar(optimizer, "optimizer", "llm", "--compile-prompt optimizer: llm (default) = real adapter-backed optimizer, ONE compile-time LLM call, output is a candidate checked by the fail-closed compile invariants, needs the key variable SET (exit 2 without it — no silent dedup), NO retries (a failed compile is a rerun of this command); dedup = offline reference fake, no key")
	fs.IntVar(maxTokens, "max-tokens", 0, "max_tokens override (0 = adapter default; anthropic requires one)")
	fs.IntVar(approvalTimeoutMs, "approval-timeout-ms", defaultApprovalTimeoutMs, "bound on each pending approval (0 = wait while connected)")
	fs.IntVar(cacheBreakpoints, "cache-breakpoints", 0, "Anthropic prompt-cache breakpoint budget: 0=off (default), 1-4 explicit (anthropic only; openai caching is implicit and rejects this flag)")
	fs.StringVar(sandbox, "sandbox", "off", "run_shell confinement: off (default: NO confinement, engine privileges), read-only (whole FS readable, no writes, network denied), workspace-write (writes only under the session dir + OS temp; network denied). Kernel-enforced (Landlock+seccomp); if unavailable, sandboxed calls FAIL CLOSED with a typed error")
	fs.Int64Var(spillMaxInline, "spill-max-inline", 65536, "inline byte budget for tool results: content above it spills FULL to <session-dir>/<session-id>.spill/ and the event carries a bounded preview + opaque locator (retrievable via spill_read, hash-validated); 0 disables the spill (always inline). Default 65536 (matches the run_shell capture cap)")
	fs.StringVar(workdirRoots, "workdir-roots", "", "comma-separated ABSOLUTE paths to existing DIRECTORIES confining the file tools (read/write/edit/glob/search) and run_shell absolute workdirs: relative file paths resolve against the FIRST root, absolute paths must sit under a root, escapes and symlink crossings reject fail-closed with zero filesystem effects, and entries resolving to a non-directory (a file, even via symlink) refuse at startup; default = the daemon's working directory resolved absolute")
	fs.BoolVar(compilePrompt, "compile-prompt", false, "run the prompt compilation with the current config, write the artifact under <session-dir>/compiled-prompts/, and exit — no protocol session; default --optimizer llm makes ONE compile-time LLM call and requires the variable named by --api-key-env to be set (fail-closed exit 2 without it); --optimizer dedup is the offline, keyless alternative")
	fs.StringVar(verifyLog, "verify-log", "", "read-only mode: replay the session log at PATH, print ONE JSON line {events, format_version, surface_sha256, messages} (sha256 over the canonical derived-surface JSON) to stdout, and exit — no protocol session, no engine flags required; exit 1 with the reason on stderr on any replay error (fail-closed). Two runs on the same log print identical bytes (replay-determinism prover)")
	fs.StringVar(askTools, "ask-tools", "", "comma-separated REGISTERED tool names whose calls ride the approval waterfall (an operator-named ask source: ask → approval/request on the wire → the client's interactive/--json/policy approver; unanswerable = deny fail-closed). Unknown name = exit 2. Default empty = no named-tool asks (the mcp_ namespace asks by DEFAULT since P8.2 — see --mcp-auto-allow)")
	fs.StringVar(skillsDir, "skills-dir", "", "Agent Skills catalog directory of <name>/SKILL.md folders (agentskills.io convention; three-tier delivery: prompt lines + guarded skill_load tool + confined reference reads). Default ./.opencode/skills against the daemon cwd — absent default = zero skills with an honest startup line; an EXPLICITLY-passed missing dir = exit 2 (fail-closed). Catalog read once at startup")
	fs.IntVar(contextTokens, "context-tokens", defaultContextTokens, "context budget in tokens anchoring the post-turn compaction trigger (surface pressure = estimated tokens over this budget; estimate is chars/4 anchored by the last provider usage report; threshold from --compact-threshold, default 0.8). At/above threshold a turn boundary shadows the surface head behind ONE adapter-generated summary — log never rewritten, replay deterministic, a failed compaction never fails the turn (deferred to the next boundary). 0 DISABLES compaction. Default 128000")
	fs.Float64Var(compactThreshold, "compact-threshold", session.DefaultPressureThreshold, "surface-pressure ratio at which a turn boundary triggers compaction (0.8 default; must be within [0,1]; 0 takes the default). Only meaningful with a positive --context-tokens")
	fs.StringVar(mcpConfig, "mcp-config", "", "MCP host config: a JSON file that is EITHER a full opencode.json (its .mcp block is extracted) OR a bare {\"<name>\": {...}} server map, with local stdio servers ({\"type\":\"local\",\"command\":[...]}) and/or remote Streamable-HTTP servers ({\"type\":\"remote\",\"url\":\"https://…\"}) — each server's tools join the guarded registry as mcp_<server>_<tool> under the FULL approval/guard waterfall (external candidate input; url/headers/env values are credentials, redacted on every surface). Default (unset): ~/.config/opencode/opencode.json when it exists (honest startup line), else zero MCP. Explicitly-passed missing/invalid file = exit 2 fail-closed")
	fs.IntVar(mcpTimeoutMs, "mcp-timeout-ms", defaultMCPTimeoutMs, "bound on EVERY MCP exchange (initialize + tools/list + each tools/call), 60000 default; 0 takes the default; capped at 600000 (the run_shell cap — no unbounded waits, a hung server fails closed within the bound)")
	fs.BoolVar(mcpAutoAllow, "mcp-auto-allow", false, "OPT BACK IN to allowing mcp tool calls without asking. DEFAULT FALSE: the mcp_ namespace is ask-by-default — MCP tools are un-sandboxed external network egress (unlike run_shell under --sandbox confinement), so every mcp_ call rides the approval waterfall (approval/request on the wire; the client's --policy/interactive approver answers; unanswerable approvals deny fail-closed). With this flag the mcp ask observer is not registered and mcp calls execute without asking (the pre-P8.2 posture, now an explicit operator choice)")
	fs.BoolVar(showVersion, "version", false, "print engine and protocol versions and exit")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2 // flag package already printed the specific error
	}
	if *showVersion {
		fmt.Fprintf(stdout, "vh-agentd engine %s, protocol %d\n", engineVersion, protocol.ProtocolVersion)
		return 0
	}

	// --verify-log is a READ-ONLY operator mode, handled BEFORE the
	// required-flag validation (it needs no adapter/model/session-dir
	// and starts no protocol session). It is the replay-determinism
	// prover for real produced logs: same log in, same line out.
	if *verifyLog != "" {
		return runVerifyLog(*verifyLog, stdout, stderrw)
	}

	cfg, err := validate(*adapter, *model, *baseURL, *apiKeyEnv, *sessionDir, *optimizer, *maxTokens, *approvalTimeoutMs, *cacheBreakpoints, *sandbox, *spillMaxInline, *workdirRoots)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	// P3.5 --ask-tools: fail-closed syntax parse (empty entries,
	// garbage) on the same exit-2 path as validate(). The
	// registered-set check runs after buildServer (the catalog lives
	// on the engine's pipeline); both refuse BEFORE any serving.
	cfg.AskTools, err = parseAskTools(*askTools)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	// P5 compaction flags: same fail-closed exit-2 posture as every
	// sibling flag. A zero budget is the documented disable path (cfg
	// .Compaction stays zero — buildServer then arms no decorator).
	cfg.Compaction, err = validateCompaction(*contextTokens, *compactThreshold)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	// P7 skills catalog: load ONCE here (before buildServer and the
	// --compile-prompt path, both of which consume cfg.Skills for the
	// prompt section and the tool catalog). Startup honesty lines
	// (count / honest-absent / per-exclusion warnings) go to the daemon
	// log; an explicitly-passed-but-missing dir is a usage error: exit 2.
	cfg.Skills, err = loadSkillsCatalog(*skillsDir, log)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	// P8 MCP host: validate the timeout flag and CONNECT every
	// configured server NOW (launch stdio subprocesses, initialize
	// remotes, discover tools) — BEFORE buildServer and the
	// --compile-prompt path, both of which consume cfg.MCP for the
	// advertised tool catalog (the prompt content hash covers MCP
	// tools). Server launch is startup-only: a degraded server logs
	// its typed reason and stays degraded; relaunch is a daemon
	// restart (documented). The registry (its subprocesses) closes at
	// daemon exit on every exit path.
	mcpTimeout, err := validateMCPTimeout(*mcpTimeoutMs)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	cfg.MCP, err = setupMCP(*mcpConfig, mcpTimeout, log)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	// P8.2: the ask-by-default opt-back-in (default false — see
	// mcpask.go). Recorded on Config so the composition (buildServer →
	// armAskObservers) and the startup posture line read ONE source.
	cfg.MCPAutoAllow = *mcpAutoAllow
	if cfg.MCP != nil {
		defer func() {
			cfg.MCP.Close()
			log.Printf("mcp: servers stopped")
		}()
	}

	// Loud posture note: a requested confinement mode whose OS backend
	// is unavailable makes every sandboxed run_shell call fail closed
	// (typed sandbox-unavailable). Say so AT STARTUP, not at first
	// failure — this is a warning, not a refusal (the fail-closed
	// refusal is per-call, in the tool).
	if cfg.SandboxMode != shell.SandboxOff {
		if features := execsandbox.Detect(); !features.Available() {
			log.Printf("sandbox: mode %s requested but OS primitives unavailable (landlock=%v seccomp=%v); run_shell calls will FAIL CLOSED with typed sandbox-unavailable errors (start with --sandbox off to explicitly accept no confinement)", cfg.SandboxMode, features.Landlock, features.Seccomp)
		}
	}

	if err := os.MkdirAll(cfg.SessionDir, 0o755); err != nil {
		log.Printf("mkdir session dir %s: %v", cfg.SessionDir, err)
		return 1
	}

	// Compile mode: assemble + optimize + write the artifact
	// and exit. Deliberately BEFORE the serving credential check — a
	// compile run starts no protocol session. With --optimizer dedup it
	// is fully offline (no key value read). With --optimizer llm (the
	// default) the compile makes ONE real call through the configured
	// adapter, so the key variable must be SET: exit 2 naming the
	// missing piece — fail-closed, never a silent dedup fallback.
	if *compilePrompt {
		compileKey := ""
		if cfg.Optimizer == optimizerLLM {
			compileKey = getenv(cfg.APIKeyEnv)
			if compileKey == "" {
				fmt.Fprintf(stderrw, "vh-agentd: --optimizer llm requires the API key environment variable %s to be set (fail-closed: no silent dedup fallback; use --optimizer dedup for an offline compile)\n\n%s", cfg.APIKeyEnv, usageDoc)
				return 2
			}
		}
		if err := compilePromptOffline(context.Background(), cfg, compileKey, toolSpecsForPrompt(cfg), stderrw); err != nil {
			log.Printf("%v", err)
			return 1
		}
		return 0
	}

	// Credential rule: read the key from the named env var, fail closed
	// when unset. The value never reaches the logs (only its name does).
	apiKey := getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		fmt.Fprintf(stderrw, "vh-agentd: API key environment variable %s is not set (fail-closed: refusing to start without credentials; note --compile-prompt with --optimizer dedup runs offline and needs no key; the default llm optimizer requires the key variable set)\n\n%s", cfg.APIKeyEnv, usageDoc)
		return 2
	}

	log.Printf("starting: adapter=%s model=%s base-url=%s session-dir=%s api-key-env=%s approval-timeout-ms=%d cache-breakpoints=%d sandbox=%s optimizer=%s spill-max-inline=%d workdir-roots=%s",
		cfg.Adapter, cfg.Model, cfg.BaseURL, cfg.SessionDir, cfg.APIKeyEnv, cfg.ApprovalTimeoutMs, cfg.CacheBreakpoints, cfg.SandboxMode, cfg.Optimizer, cfg.SpillMaxInline, strings.Join(cfg.WorkdirRoots, ","))
	if cfg.Compaction.ContextBudgetTokens > 0 {
		thr := cfg.Compaction.PressureThreshold
		if thr <= 0 {
			thr = session.DefaultPressureThreshold
		}
		log.Printf("compaction: armed (budget %d tokens, threshold %.2f — post-turn surface-pressure check; failures defer to the next boundary, never fail a turn)", cfg.Compaction.ContextBudgetTokens, thr)
	} else {
		log.Printf("compaction: disabled (--context-tokens 0)")
	}

	srv, engine, tracker, served := buildServer(cfg, apiKey, rwc)
	if served.Reason != "" {
		log.Printf("system prompt: source=%s reason=%s (run with --compile-prompt to populate the artifact)", served.Source, served.Reason)
	} else {
		log.Printf("system prompt: source=%s", served.Source)
	}

	// P3.5 --ask-tools: validate against the REGISTERED tool set (now
	// that the engine's pipeline carries it) BEFORE Serve — an unknown
	// name is a usage error: exit 2, never a silently-unrouted run.
	// (Arming happens INSIDE buildServer — armAskObservers, the one
	// registration site; this validation refusing to serve leaves the
	// armed composition unobservable.)
	if err := validateAskTools(cfg.AskTools, engine.Pipeline().Definitions()); err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
	}
	if len(cfg.AskTools) > 0 {
		log.Printf("ask-tools: %s (calls to these tools ask the client; unanswerable approvals deny fail-closed)", strings.Join(cfg.AskTools, ","))
	}

	// P8.2 posture line: MCP tools ride the approval waterfall by
	// default (un-sandboxed external egress must not silently
	// auto-execute); --mcp-auto-allow is the loud operator opt-back-in.
	if cfg.MCP != nil {
		if cfg.MCPAutoAllow {
			log.Printf("mcp tools: auto-allow (operator opt-in via --mcp-auto-allow; mcp calls do NOT ask)")
		} else {
			log.Printf("mcp tools: ask-by-default (every mcp_ call asks the client; unanswered approvals deny fail-closed)")
		}
	}

	// Scheduler: real Manager seams through the tracker, state file
	// under the session dir, STARTED before Serve, DRAINED at shutdown.
	// B3: the scheduler is handed to the engine seam (BEFORE Serve, so
	// every session/create stamps it) — schedule/add|list|remove reach
	// it over the wire; the lifecycle stays daemon-owned.
	sched, err := buildScheduler(cfg, tracker)
	if err != nil {
		log.Printf("scheduler: construct: %v", err)
		return 1
	}
	engine.Schedules = sched
	if err := sched.Start(); err != nil {
		log.Printf("scheduler: start: %v", err)
		return 1
	}
	log.Printf("scheduler: started (state file %s)", filepath.Join(cfg.SessionDir, schedulerStateFilename))
	defer func() {
		sched.Stop() // drained: an in-flight Tick completes, then the loop exits
		log.Printf("scheduler: drained")
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := srv.Serve(ctx); err != nil && err != io.EOF && ctx.Err() == nil {
		log.Printf("serve: %v", err)
		return 1
	}
	return 0
}

// newLogger builds the stderr-only diagnostic logger (stdout is
// protocol).
func newLogger(w io.Writer) *log.Logger {
	return log.New(w, "vh-agentd ", log.LstdFlags|log.LUTC|log.Lmsgprefix)
}

// realNow is the default clock source for the clock tool.
func realNow() time.Time { return time.Now() }

// runVerifyLog is the --verify-log mode: replay the log at path through
// session.ReplayFile + DeriveMessages (existing exported surfaces only)
// and print ONE JSON line — {"events":N,"format_version":V,
// "surface_sha256":"<sha256 of the canonical surface JSON>","messages":M}
// — to stdout. Exit 0 on success; exit 1 with the reason on stderr on
// ANY replay error (fail-closed). The canonical surface JSON is
// json.Marshal of the derived []session.Message (deterministic field
// order, nil normalized to []), so two runs on the same file print
// byte-identical lines: this is the replay-determinism prover for real
// produced logs.
func runVerifyLog(path string, stdout, stderrw io.Writer) int {
	events, err := session.ReplayFile(path)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: --verify-log %s: replay failed: %v\n", path, err)
		return 1
	}
	if len(events) == 0 {
		fmt.Fprintf(stderrw, "vh-agentd: --verify-log %s: log has no session/header record (empty log?); refusing to verify a headerless session\n", path)
		return 1
	}
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: --verify-log %s: surface derivation failed: %v\n", path, err)
		return 1
	}
	if msgs == nil {
		msgs = []session.Message{}
	}
	surfaceJSON, err := json.Marshal(msgs)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: --verify-log %s: canonical surface marshal failed: %v\n", path, err)
		return 1
	}
	sum := sha256.Sum256(surfaceJSON)
	var header session.HeaderPayload
	if err := json.Unmarshal(events[0].Payload, &header); err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: --verify-log %s: malformed header payload: %v\n", path, err)
		return 1
	}
	out := struct {
		Events        int    `json:"events"`
		FormatVersion int    `json:"format_version"`
		SurfaceSHA256 string `json:"surface_sha256"`
		Messages      int    `json:"messages"`
	}{
		Events:        len(events),
		FormatVersion: header.FormatVersion,
		SurfaceSHA256: hex.EncodeToString(sum[:]),
		Messages:      len(msgs),
	}
	line, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: --verify-log %s: output marshal failed: %v\n", path, err)
		return 1
	}
	fmt.Fprintf(stdout, "%s\n", line)
	return 0
}
