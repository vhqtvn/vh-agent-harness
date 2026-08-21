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
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/execsandbox"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
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
		maxTokens         = new(int)
		approvalTimeoutMs = new(int)
		cacheBreakpoints  = new(int)
		sandbox           = new(string)
		compilePrompt     = new(bool)
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
	fs.IntVar(maxTokens, "max-tokens", 0, "max_tokens override (0 = adapter default; anthropic requires one)")
	fs.IntVar(approvalTimeoutMs, "approval-timeout-ms", defaultApprovalTimeoutMs, "bound on each pending approval (0 = wait while connected)")
	fs.IntVar(cacheBreakpoints, "cache-breakpoints", 0, "Anthropic prompt-cache breakpoint budget: 0=off (default), 1-4 explicit (anthropic only; openai caching is implicit and rejects this flag)")
	fs.StringVar(sandbox, "sandbox", "off", "run_shell confinement: off (default: NO confinement, engine privileges), read-only (whole FS readable, no writes, network denied), workspace-write (writes only under the session dir + OS temp; network denied). Kernel-enforced (Landlock+seccomp); if unavailable, sandboxed calls FAIL CLOSED with a typed error")
	fs.BoolVar(compilePrompt, "compile-prompt", false, "run the offline prompt compilation with the current config (reference Dedup optimizer), write the artifact under <session-dir>/compiled-prompts/, and exit — no network, no key, no protocol session")
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

	cfg, err := validate(*adapter, *model, *baseURL, *apiKeyEnv, *sessionDir, *maxTokens, *approvalTimeoutMs, *cacheBreakpoints, *sandbox)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
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

	// Offline compile mode: assemble + optimize + write the artifact
	// and exit. Deliberately BEFORE the credential check — a compile
	// run makes no network calls and needs no key (--api-key-env must
	// still NAME a variable; the variable itself may be unset here).
	if *compilePrompt {
		if err := compilePromptOffline(context.Background(), cfg, toolSpecsForPrompt(cfg), stderrw); err != nil {
			log.Printf("%v", err)
			return 1
		}
		return 0
	}

	// Credential rule: read the key from the named env var, fail closed
	// when unset. The value never reaches the logs (only its name does).
	apiKey := getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		fmt.Fprintf(stderrw, "vh-agentd: API key environment variable %s is not set (fail-closed: refusing to start without credentials; note --compile-prompt runs offline and needs no key)\n\n%s", cfg.APIKeyEnv, usageDoc)
		return 2
	}

	log.Printf("starting: adapter=%s model=%s base-url=%s session-dir=%s api-key-env=%s approval-timeout-ms=%d cache-breakpoints=%d sandbox=%s",
		cfg.Adapter, cfg.Model, cfg.BaseURL, cfg.SessionDir, cfg.APIKeyEnv, cfg.ApprovalTimeoutMs, cfg.CacheBreakpoints, cfg.SandboxMode)

	srv, _, tracker, served := buildServer(cfg, apiKey, rwc)
	if served.Reason != "" {
		log.Printf("system prompt: source=%s reason=%s (run with --compile-prompt to populate the artifact)", served.Source, served.Reason)
	} else {
		log.Printf("system prompt: source=%s", served.Source)
	}

	// Scheduler: real Manager seams through the tracker, state file
	// under the session dir, STARTED before Serve, DRAINED at shutdown.
	// No protocol surface registers schedules in B1 — the loop runs so
	// persisted state is adopted and the lifecycle is live.
	sched, err := buildScheduler(cfg, tracker)
	if err != nil {
		log.Printf("scheduler: construct: %v", err)
		return 1
	}
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
