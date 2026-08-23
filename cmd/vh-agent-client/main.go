// vh-agent-client is the minimal reference CLI client that drives a
// vh-agentd daemon end-to-end over the REAL host protocol: live event
// rendering, interactive approvals, and a machine (--json) mode. It is
// the canonical protocol-consumer example (docs/native-engine/
// host-protocol.md) and the executable half of the client UX contract
// (README.agent.md → "vh-agent-client").
//
// Output discipline mirrors the daemon's stdout-is-protocol purity:
// rendered events and prompts go to STDERR; stdout carries
// machine-readable final content only (the one-shot final assistant
// text; NDJSON events in --json mode).
//
// Exit codes: 0 clean · 1 protocol/engine error · 2 usage/validation.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/vhqtvn/vh-agent-harness/cmd/vh-agent-client/policy"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// childPipes joins the daemon's stdout (read) and the daemon's stdin
// (write) into the Client's ReadWriteCloser seam; Close closes only the
// write side — the client-close ladder that triggers the daemon's EOF
// path (deny pending approvals, drain, exit 0).
type childPipes struct {
	io.ReadCloser
	io.WriteCloser
}

func (c childPipes) Close() error { return c.WriteCloser.Close() }

// isTTYFunc reports whether stdin is a terminal (char device) — the
// REPL default posture. Non-file readers (tests, pipes) are never a
// TTY.
func isTTYFunc(stdin io.Reader) func() bool {
	return func() bool {
		f, ok := stdin.(*os.File)
		if !ok {
			return false
		}
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		return fi.Mode()&os.ModeCharDevice != 0
	}
}

// run is the testable entry core: parse+validate (exit 2), spawn the
// daemon (default `vh-agentd` from PATH; --exec spec otherwise), drive
// the protocol, and map the outcome to the process exit code. The
// daemon's stderr is inherited (its diagnostics flow to the client's
// stderr); its stdout is the protocol pipe and is never mixed into the
// client's own output.
func run(args []string, stdin io.Reader, stdout, stderrw io.Writer) (exit int) {
	cfg, err := parseArgs(args, isTTYFunc(stdin), stderrw)
	if err != nil {
		if err == errHelp {
			return 0 // usage doc already printed
		}
		fmt.Fprintf(stderrw, "vh-agent-client: %v\n\n%s", err, usageDoc)
		return exitCodeFor(err)
	}

	// P3 policy engine: load and parse BEFORE the daemon spawns — a
	// broken policy file is a usage error (exit 2) naming the exact
	// offending line, never a silently-absent policy and never a run
	// that already started a daemon.
	var pol *policy.Policy
	if cfg.PolicyPath != "" {
		p, perr := policy.LoadFile(cfg.PolicyPath)
		if perr != nil {
			fmt.Fprintf(stderrw, "vh-agent-client: %v\n\n%s", perr, usageDoc)
			return 2
		}
		pol = p
		fmt.Fprintf(stderrw, "vh-agent-client: policy loaded (%d rules) from %s\n", len(pol.Rules()), cfg.PolicyPath)
	}

	// Spawn the daemon.
	argv := cfg.daemonArgv()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stderr = stderrw
	cmd.Env = os.Environ()
	daemonStdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agent-client: daemon stdin pipe: %v\n", err)
		return 1
	}
	daemonStdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(stderrw, "vh-agent-client: daemon stdout pipe: %v\n", err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderrw, "vh-agent-client: cannot start daemon %s: %v\n", argv[0], err)
		return 1
	}

	client := protocol.NewClient(childPipes{daemonStdout, daemonStdin})
	defer func() { _ = client.Close() }()

	// Single stdin owner (hotfix b-F1): ONE dispatcher goroutine reads
	// every stdin line and routes it — REPL lines and approval answers
	// (by approvalId in --json mode, prompt order otherwise). The
	// responders and the REPL never touch the shared bufio.Reader
	// directly, so concurrently-pending approvals cannot race a line
	// onto the wrong approvalId.
	shared := bufio.NewReader(stdin)
	hub := newStdinHub(shared, stderrw, cfg.JSON)
	hub.start()
	var renderer Renderer
	var approver ApproverFunc
	if cfg.JSON {
		renderer = newJSONRenderer(stdout)
		approver = jsonApprover(hub)
	} else {
		renderer = newHumanRenderer(stderrw)
		approver = interactiveApprover(hub, stderrw)
	}
	// P3: compose the policy engine IN FRONT of the responder —
	// delegation, not replacement (unmatched asks still reach the
	// interactive/--json approver; --json --policy composes).
	if pol != nil {
		approver = policyApprover(pol, approver, stderrw)
	}

	var selfStdin io.Closer
	if f, ok := stdin.(*os.File); ok {
		selfStdin = f
	}

	d := &driver{
		cfg:       cfg,
		client:    client,
		renderer:  renderer,
		approver:  approver,
		answers:   hub,
		out:       stdout,
		errw:      stderrw,
		daemonIn:  daemonStdin,
		selfStdin: selfStdin,
	}
	fmt.Fprintf(stderrw, "vh-agent-client: daemon %s (pid %d)\n", argv[0], cmd.Process.Pid)

	// Ctrl-C contract: send nothing, close the connection — the
	// daemon's EOF ladder denies every pending approval (fail-closed)
	// and it exits; the client reports that honestly and exits 0.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	stopSig := make(chan struct{})
	defer close(stopSig)
	go func() {
		select {
		case <-sig:
			d.interrupt()
		case <-stopSig:
		}
	}()

	runErr := d.run(context.Background())

	// Client close ladder for every path (idempotent): closing the
	// write side is the daemon's EOF signal.
	_ = daemonStdin.Close()
	_ = client.Close()

	if d.wasInterrupted() {
		fmt.Fprintln(stderrw, "vh-agent-client: interrupted — connection closed; the daemon denied any pending approvals (fail-closed) and exited")
		_ = cmd.Wait()
		return 0
	}
	if errors.Is(runErr, errDaemonGone) {
		fmt.Fprintln(stderrw, "vh-agent-client: daemon exited")
		return settleDaemon(cmd, stderrw, 1)
	}
	if runErr != nil {
		if isUsageError(runErr) {
			fmt.Fprintf(stderrw, "vh-agent-client: %v\n\n%s", runErr, usageDoc)
			return settleDaemon(cmd, stderrw, 2)
		}
		fmt.Fprintf(stderrw, "vh-agent-client: %v\n", runErr)
		return settleDaemon(cmd, stderrw, 1)
	}
	return settleDaemon(cmd, stderrw, 0)
}

// settleDaemon waits for the daemon after the close ladder and maps its
// exit honestly: a clean client run whose daemon exited non-zero (or
// failed to exit) is an engine failure (1), not a success; an already
// failing run keeps its own code.
func settleDaemon(cmd *exec.Cmd, stderrw io.Writer, clientExit int) int {
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case werr := <-waitCh:
		if werr == nil {
			return clientExit
		}
		if ee, ok := werr.(*exec.ExitError); ok {
			fmt.Fprintf(stderrw, "vh-agent-client: daemon exited with code %d\n", ee.ExitCode())
		} else {
			fmt.Fprintf(stderrw, "vh-agent-client: daemon wait: %v\n", werr)
		}
		if clientExit == 0 {
			return 1
		}
		return clientExit
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		fmt.Fprintln(stderrw, "vh-agent-client: daemon did not exit after EOF; killed")
		if clientExit == 0 {
			return 1
		}
		return clientExit
	}
}
