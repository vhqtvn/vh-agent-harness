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

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, stdioConn{}, os.Stdout, os.Stderr))
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

	cfg, err := validate(*adapter, *model, *baseURL, *apiKeyEnv, *sessionDir, *maxTokens, *approvalTimeoutMs, *cacheBreakpoints)
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agentd: %v\n\n%s", err, usageDoc)
		return 2
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
		if err := compilePromptOffline(context.Background(), cfg, toolSpecsForPrompt(), stderrw); err != nil {
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

	log.Printf("starting: adapter=%s model=%s base-url=%s session-dir=%s api-key-env=%s approval-timeout-ms=%d cache-breakpoints=%d",
		cfg.Adapter, cfg.Model, cfg.BaseURL, cfg.SessionDir, cfg.APIKeyEnv, cfg.ApprovalTimeoutMs, cfg.CacheBreakpoints)

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
