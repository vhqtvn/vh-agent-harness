// driver.go — the client's protocol driver: initialize → session
// create → subscribe → render/approve loop, plus the one-shot and REPL
// input modes and the client-side resume posture.
//
// Resume posture (honest-cheap, documented): the wire has NO
// session/resume method yet (planned P4) — and session/create with an
// existing sessionId would TRUNCATE the old log (os.Create semantics),
// so a client-side fake resume is worse than none. The client
// therefore (a) tracks its last session in a pointer file under the
// session dir, (b) notes the prior session honestly on every fresh
// run, and (c) REFUSES --resume with a message pointing at P4.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// lastSessionFile is the client-side last-session pointer (one session
// id) under the session dir. True daemon-side resume arrives with the
// session/resume wire method (P4).
const lastSessionFile = ".vh-agent-client-last-session"

// driver drives one protocol Client through the client's UX contract.
type driver struct {
	cfg      *Config
	client   *protocol.Client
	renderer Renderer
	// approver answers approval/request notifications — the P3 policy
	// seam (default wiring: interactiveApprover / jsonApprover).
	approver ApproverFunc
	// answers is the single stdin owner (input.go): its dispatcher
	// routes REPL lines and approval answers, so the REPL and the
	// (concurrent, one-goroutine-per-approval) responders never touch
	// the shared bufio.Reader directly (hotfix b-F1).
	answers *stdinHub
	out     io.Writer // stdout: machine-readable final content only
	errw    io.Writer // stderr: rendered output + prompts
	// daemonIn is the daemon's stdin write side (closing it = the EOF
	// ladder: daemon denies pending approvals and exits).
	daemonIn io.WriteCloser
	// selfStdin, when non-nil, is closed to unblock an idle REPL read
	// (daemon-death watchdog / interrupt).
	selfStdin io.Closer

	mu          sync.Mutex
	interrupted bool
}

// interrupt implements the Ctrl-C contract: send nothing, close the
// daemon's stdin (its EOF ladder denies every pending approval and
// exits 0), halt the stdin hub (deny pending client-side, unblock an
// idle REPL wait), close our stdin (unblock a raw read if any), and
// mark the run so the exit mapping reports honestly instead of
// surfacing the connection-closed error as a failure.
func (d *driver) interrupt() {
	d.mu.Lock()
	d.interrupted = true
	d.mu.Unlock()
	if d.daemonIn != nil {
		_ = d.daemonIn.Close()
	}
	if d.answers != nil {
		d.answers.halt()
	}
	if d.selfStdin != nil {
		_ = d.selfStdin.Close()
	}
}

func (d *driver) wasInterrupted() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.interrupted
}

// note prints a client diagnostic line to stderr.
func (d *driver) note(format string, args ...any) {
	fmt.Fprintf(d.errw, "vh-agent-client: "+format+"\n", args...)
}

// run drives the configured mode to completion. The returned error is
// mapped by the caller (exitCodeFor); wasInterrupted() takes precedence
// for the honest Ctrl-C exit.
func (d *driver) run(ctx context.Context) error {
	// Notification wiring BEFORE subscribe (live-only stream).
	d.client.OnNotification("session/event", d.renderer.RenderEvent)
	d.client.OnNotification("approval/request", d.onApprovalRequest)
	d.client.OnNotification("protocol/error", d.renderer.RenderProtocolError)

	if err := d.client.Call("initialize", map[string]any{"protocolVersion": protocol.ProtocolVersion}, nil); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	if err := d.establishSession(); err != nil {
		return err
	}

	if err := d.client.Call("session/subscribe", nil, nil); err != nil {
		return fmt.Errorf("session/subscribe: %w", err)
	}

	switch d.cfg.Mode {
	case ModeOneShot:
		return d.oneShot()
	default:
		return d.repl(ctx)
	}
}

// establishSession applies the resume posture (pointer file + honest
// notes + --resume refusal) and creates a FRESH session, echoing the
// session id + log path at start (stderr).
func (d *driver) establishSession() error {
	pointer := filepath.Join(d.cfg.SessionDir, lastSessionFile)
	var prior string
	if raw, err := os.ReadFile(pointer); err == nil {
		prior = strings.TrimSpace(string(raw))
	}

	if d.cfg.Resume {
		if prior == "" {
			return usagef("--resume: no prior session recorded in %s (and session/resume does not exist yet — P4); start a fresh session by dropping --resume", pointer)
		}
		return usagef("--resume: resuming session %s is not possible yet — the session/resume wire method does not exist (planned P4); a client-side fake would create a NEW session and truncate the old log. Start fresh (drop --resume); true daemon-side resume arrives in P4", prior)
	}
	if prior != "" {
		d.note("prior session %s recorded (log untouched); resume is not supported yet (P4) — starting a FRESH session", prior)
	}

	var created struct {
		SessionID string `json:"sessionId"`
		Path      string `json:"path"`
	}
	if err := d.client.Call("session/create", nil, &created); err != nil {
		return fmt.Errorf("session/create: %w", err)
	}
	d.note("session %s (log: %s)", created.SessionID, created.Path)
	if err := os.MkdirAll(d.cfg.SessionDir, 0o755); err != nil {
		return fmt.Errorf("session dir: %w", err)
	}
	if err := os.WriteFile(pointer, []byte(created.SessionID+"\n"), 0o644); err != nil {
		d.note("warning: could not write last-session pointer %s: %v", pointer, err)
	}
	return nil
}

// onApprovalRequest answers one approval/request through the ApproverFunc
// seam and resolves it with approval/respond (best-effort on a dying
// connection: the daemon's fail-closed bridge already denies then).
//
// It answers on its OWN goroutine: the client's read loop dispatches
// notification handlers inline, and the answer is a blocking Call whose
// response can only be read by that same read loop — answering
// synchronously would deadlock. Concurrency is safe NOW (hotfix b-F1):
// the responder's stdin reads are arbitrated by the stdinHub's single
// dispatcher (input.go) — answers route strictly by approvalId (json
// mode) or prompt order (interactive), never by read-race; approvals
// are independent one-shots keyed by id, and session/event rendering
// stays inline and ordered.
func (d *driver) onApprovalRequest(params json.RawMessage) {
	go d.answerApproval(params)
}

func (d *driver) answerApproval(params json.RawMessage) {
	var p struct {
		ApprovalID string           `json:"approvalId"`
		Call       session.ToolCall `json:"call"`
		Reason     string           `json:"reason"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.ApprovalID == "" {
		d.note("warning: undecodable approval/request — ignoring (daemon will deny on timeout/disconnect)")
		return
	}
	d.renderer.RenderApproval(params)
	ans := d.approver(p.ApprovalID, p.Call, p.Reason)
	if err := d.client.Call("approval/respond", map[string]any{
		"approvalId": p.ApprovalID,
		"allow":      ans.Allow,
		"reason":     ans.Reason,
	}, nil); err != nil {
		// The daemon denies on absent/unanswerable answers; a failed
		// respond here means the connection is going away — the deny is
		// implicit (fail-closed bridge).
		d.note("warning: approval/respond for %s failed: %v (the daemon denies unanswerable approvals)", p.ApprovalID, err)
	}
}

// turnError marks a turn that ended kind=error (exit 1).
type turnError struct{ reason string }

func (e *turnError) Error() string {
	return fmt.Sprintf("turn ended with error: %s", e.reason)
}

// waitTurnEnd waits until the renderer has observed THIS turn's
// turn/end event (quiescence drain — the response can beat the
// notification handlers through the local pipeline) capped at 2s; a
// timeout proceeds (the Call result is authoritative). Meaningful
// only because converse resets the tracker per prompt (hotfix
// c-F2/d-F2): without the reset, `seen` stays true from the first
// turn on and the drain is a no-op, letting a PRIOR turn's kind=error
// misclassify the current turn.
func (d *driver) waitTurnEnd() {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, seen := d.renderer.LastTurnEnd(); seen {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		if isDone(d.client) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// isDone reports whether the client transport has terminated.
func isDone(c *protocol.Client) bool {
	select {
	case <-c.Done():
		return true
	default:
		return false
	}
}

// maxConversationTurns bounds the tool-continuation loop (a model that
// never stops calling tools must not spin the client forever).
const maxConversationTurns = 32

// converse drives the host-side multi-turn loop: session/prompt is ONE
// synchronous tool turn (the engine runs a single LLM call + tool batch
// per call; the loop is CLIENT-driven by design — the daemon stays
// thin). While the model requests tools, a minimal continuation prompt
// (" ") is sent so the surface (now carrying the tool results) is
// re-submitted; the loop ends at the first tool-free response.
func (d *driver) converse(text string) (string, error) {
	current := text
	for i := 0; i < maxConversationTurns; i++ {
		var result struct {
			Content   string `json:"content"`
			ToolCalls []any  `json:"toolCalls"`
		}
		// Per-prompt epoch (hotfix c-F2/d-F2): forget the previous
		// turn's end BEFORE issuing this prompt, so the drain below
		// waits for THIS turn's turn/end and a prior kind=error
		// cannot bleed into this turn's classification.
		d.renderer.ResetTurnEnd()
		err := d.client.Call("session/prompt", map[string]any{"text": current}, &result)
		d.waitTurnEnd()
		if err != nil {
			return "", fmt.Errorf("session/prompt: %w", err)
		}
		if kind, seen := d.renderer.LastTurnEnd(); seen && kind == "error" {
			return "", &turnError{reason: "engine reported turn/end kind=error"}
		}
		if len(result.ToolCalls) == 0 {
			return result.Content, nil
		}
		current = " " // continuation: the surface already carries the tool results
	}
	return "", fmt.Errorf("conversation did not converge within %d turns (the model kept requesting tools)", maxConversationTurns)
}

// oneShot sends the prompt, streams the turn(s), prints the final
// assistant text on stdout (machine-readable content — the ONLY stdout
// output in human mode), and returns.
func (d *driver) oneShot() error {
	content, err := d.converse(d.cfg.Prompt)
	if err != nil {
		return err
	}
	if d.cfg.JSON {
		// Machine mode: the final result object is the last NDJSON
		// line on stdout.
		line, merr := json.Marshal(map[string]any{
			"kind":    "prompt-result",
			"content": content,
		})
		if merr != nil {
			return merr
		}
		fmt.Fprintln(d.out, string(line))
		return nil
	}
	if content == "" {
		d.note("turn completed with no assistant text")
		return nil
	}
	fmt.Fprintln(d.out, content)
	return nil
}

// errDaemonGone marks a REPL send that failed because the daemon
// exited (distinct from an engine-level turn error).
var errDaemonGone = errors.New("daemon exited")

// repl reads lines routed by the stdin hub (a line = one user
// message) until exit/quit/EOF, daemon death, or ctx end. Clean EOF and
// exit/quit return nil; turn errors are rendered but the loop
// continues; a dead daemon ends the loop.
func (d *driver) repl(ctx context.Context) error {
	if !d.cfg.JSON {
		fmt.Fprintln(d.errw, "vh-agent-client REPL — a line is one message; exit/quit or Ctrl-D ends; Ctrl-C closes the connection (pending approvals are denied, fail-closed)")
	}
	d.answers.startRepl()
	// Daemon-death watchdog: when the transport dies while the loop is
	// blocked waiting for a line, say so and halt the hub (denies any
	// pending approvals, ends the REPL line wait).
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-d.client.Done():
			d.note("daemon connection closed — exiting")
			d.answers.halt()
			if d.selfStdin != nil {
				_ = d.selfStdin.Close()
			}
		case <-watchDone:
		}
	}()

	for {
		if ctx.Err() != nil {
			return nil
		}
		if !d.cfg.JSON {
			fmt.Fprint(d.errw, "vh-agent-client> ")
		}
		line, ok := d.answers.replLine()
		if !ok {
			// EOF (or closed-by-watchdog/interrupt stdin): send
			// nothing further; the caller closes the connection (the
			// daemon denies any pending approvals and exits).
			if !d.wasInterrupted() && !isDone(d.client) {
				d.note("input closed — exiting cleanly")
			}
			return nil
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		switch text {
		case "exit", "quit":
			return nil
		}
		if _, err := d.converse(text); err != nil {
			if isDone(d.client) {
				return errDaemonGone
			}
			// Turn-level failure: render honestly, keep the REPL
			// alive (the session may recover next turn).
			d.note("%v", err)
		}
	}
}
