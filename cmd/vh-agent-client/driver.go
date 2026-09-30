// driver.go — the client's protocol driver: initialize → session
// create → subscribe → render/approve loop, plus the one-shot and REPL
// input modes and the client-side resume posture.
//
// Resume posture (P4, real over the wire): --resume (bare or with an
// explicit id) drives the daemon's session/resume method — the EXISTING
// log is opened without truncating, so a second daemon lifetime on the
// same session dir continues the SAME durable stream. The client tracks
// its last session in a pointer file under the session dir and re-points
// it at every live session; resume is never faked through
// session/create (create with an existing id TRUNCATES — os.Create
// semantics).
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

	// bgMu guards the background-job tail state (P6): jobs observed
	// enqueued during THIS client process, each with its output cursor.
	bgMu   sync.Mutex
	bgJobs map[string]*bgTail
}

// bgTail is the driver-side tail state of one background job.
type bgTail struct {
	cursor int64 // next jobs/output offset to read
	// terminalDone guards the ONE-TIME terminal record: once a job is
	// settled AND fully consumed, the drain emits a single empty-chunk
	// settled-state record (the deterministic end-of-tail marker for
	// machine consumers — the final BYTES may have been read a poll
	// earlier, while the job was still running).
	terminalDone bool
}

// observeJobEvent tracks job/enqueued events so the drain loop knows
// which jobs to tail (any kind — output-producing kinds stream;
// echo/fail kinds simply read empty).
func (d *driver) observeJobEvent(ev *session.Event) {
	if ev.Type != session.TypeJobEnqueued {
		return
	}
	var p session.JobPayload
	if len(ev.Payload) > 0 {
		_ = json.Unmarshal(ev.Payload, &p)
	}
	if p.JobID == "" {
		return
	}
	d.bgMu.Lock()
	defer d.bgMu.Unlock()
	if d.bgJobs == nil {
		d.bgJobs = make(map[string]*bgTail)
	}
	if _, ok := d.bgJobs[p.JobID]; !ok {
		d.bgJobs[p.JobID] = &bgTail{}
	}
}

// onSessionEvent feeds the renderer and the background-job tracker.
func (d *driver) onSessionEvent(params json.RawMessage) {
	var ev session.Event
	if err := json.Unmarshal(params, &ev); err == nil {
		d.observeJobEvent(&ev)
	}
	d.renderer.RenderEvent(params)
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
	d.client.OnNotification("session/event", d.onSessionEvent)
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

// establishSession applies the session posture: --resume (bare or with
// an explicit id) resumes the EXISTING durable session over the wire
// (session/resume — the daemon opens the old log without truncating,
// recovers any torn tail, and makes it active); otherwise a FRESH
// session is created and the pointer updated.
func (d *driver) establishSession() error {
	pointer := filepath.Join(d.cfg.SessionDir, lastSessionFile)
	var prior string
	if raw, err := os.ReadFile(pointer); err == nil {
		prior = strings.TrimSpace(string(raw))
	}

	if d.cfg.Resume || d.cfg.ResumeID != "" {
		id := d.cfg.ResumeID
		if id == "" {
			if prior == "" {
				return usagef("--resume: no prior session recorded in %s (nothing to resume; drop --resume to start fresh, or pass an explicit id: --resume <sessionId>)", pointer)
			}
			id = prior
		}
		var sum struct {
			SessionID string `json:"sessionId"`
			Path      string `json:"path"`
			Events    int    `json:"events"`
			Title     string `json:"title"`
			Usage     struct {
				PromptTokens     int `json:"promptTokens"`
				CompletionTokens int `json:"completionTokens"`
				TotalTokens      int `json:"totalTokens"`
			} `json:"usage"`
			UnsettledJobs []string `json:"unsettledJobs"`
		}
		if err := d.client.Call("session/resume", map[string]any{"sessionId": id}, &sum); err != nil {
			return fmt.Errorf("session/resume %s: %w", id, err)
		}
		d.note("resumed session %s (log: %s; events %d; title: %s; tokens %d in / %d out / %d total)",
			sum.SessionID, sum.Path, sum.Events, sum.Title,
			sum.Usage.PromptTokens, sum.Usage.CompletionTokens, sum.Usage.TotalTokens)
		if len(sum.UnsettledJobs) > 0 {
			d.note("warning: %d unsettled job(s) at resume (reported, not re-dispatched): %s",
				len(sum.UnsettledJobs), strings.Join(sum.UnsettledJobs, ", "))
		}
		// The pointer follows the LIVE session (an explicit-id resume
		// re-points it).
		if err := os.MkdirAll(d.cfg.SessionDir, 0o755); err != nil {
			return fmt.Errorf("session dir: %w", err)
		}
		if err := os.WriteFile(pointer, []byte(sum.SessionID+"\n"), 0o644); err != nil {
			d.note("warning: could not write last-session pointer %s: %v", pointer, err)
		}
		return nil
	}

	if prior != "" {
		d.note("prior session %s recorded; resume with --resume (or --resume %s) — starting a FRESH session", prior, prior)
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
// output in human mode), drains observed background jobs to
// settlement (P6 tailing — without this the daemon would be torn down
// at client exit with the jobs still running), and returns.
func (d *driver) oneShot() error {
	content, err := d.converse(d.cfg.Prompt)
	if err != nil {
		return err
	}
	d.drainBackgroundJobs()
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

// bgWaitMax bounds the background-drain loop: a job is itself bounded
// by the run_shell timeout cap (600s default), so waiting the same
// ceiling for settlement is enough; exceeding it gives up honestly.
const bgWaitMax = 10 * time.Minute

// bgPollInterval is the jobs/status + jobs/output poll cadence.
const bgPollInterval = 150 * time.Millisecond

// drainBackgroundJobs tails every job observed enqueued during this
// client process until settlement: each poll pages jobs/output from
// the per-job cursor (rendering every non-empty chunk), and once every
// tracked job is settled it calls session/surface ONCE so pending
// job/report notices land in the durable log and the model-visible
// surface (the daemon emits reports only at prompt/surface — the next
// conversation turn would otherwise carry them, and a one-shot client
// has no next turn).
//
// HONEST SCOPE: REPL mode does NOT drain (tailing there is the deferred
// slash-command slice); a drain gives up after bgWaitMax with a note.
func (d *driver) drainBackgroundJobs() {
	d.bgMu.Lock()
	tracked := make([]string, 0, len(d.bgJobs))
	for id := range d.bgJobs {
		tracked = append(tracked, id)
	}
	d.bgMu.Unlock()
	if len(tracked) == 0 {
		return
	}
	d.note("draining %d background job(s) — tailing via jobs/output", len(tracked))

	deadline := time.Now().Add(bgWaitMax)
	for {
		if isDone(d.client) {
			return
		}
		allSettled := d.pollOnce(tracked)
		if allSettled {
			// Flush pending job/report notices into the log + surface.
			if err := d.client.Call("session/surface", nil, nil); err != nil {
				d.note("warning: session/surface after job drain: %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			d.note("warning: background jobs still unsettled after %s — giving up the tail (settlement will surface on the next prompt/surface)", bgWaitMax)
			return
		}
		time.Sleep(bgPollInterval)
	}
}

// pollOnce runs one jobs/status + jobs/output pass over the tracked
// jobs and reports whether every one of them is settled with its tail
// fully consumed.
func (d *driver) pollOnce(tracked []string) bool {
	var st struct {
		Jobs []struct {
			JobID string `json:"jobId"`
			State string `json:"state"`
		} `json:"jobs"`
	}
	if err := d.client.Call("jobs/status", nil, &st); err != nil {
		d.note("warning: jobs/status during drain: %v", err)
		return false
	}
	states := make(map[string]string, len(st.Jobs))
	for _, j := range st.Jobs {
		states[j.JobID] = j.State
	}
	allSettled := true
	for _, id := range tracked {
		state, known := states[id]
		if !known {
			continue // engine folded it away? nothing to tail
		}
		if !d.tailJob(id) {
			allSettled = false // output still flowing
		}
		if state != "settled" {
			allSettled = false
		}
	}
	return allSettled
}

// tailJob pages jobs/output from the job's cursor, rendering every
// non-empty chunk plus a ONE-TIME empty terminal record once the job
// is settled and fully consumed; it reports whether the tail is fully
// consumed (cursor at written, nothing pending). The typed re-sync
// errors (§4g — output-evicted / output-ahead) are consumed IN-LOOP
// via resyncTailCursor, so a job whose early output fell behind the
// retention window still drains to completion over its retained tail
// (a persistent evicted condition is never treated as unsettled).
func (d *driver) tailJob(jobID string) bool {
	d.bgMu.Lock()
	t := d.bgJobs[jobID]
	d.bgMu.Unlock()
	if t == nil {
		return true
	}
	for i := 0; i < 64; i++ { // bounded pages per poll; rest waits for the next
		var ch jobsOutputChunk
		err := d.client.Call("jobs/output", map[string]any{"jobId": jobID, "offset": t.cursor}, &ch)
		if err != nil {
			if d.resyncTailCursor(jobID, t, err) {
				continue // cursor re-synced (§4g): page from the new offset
			}
			d.note("warning: jobs/output %s: %v", jobID, err)
			return false
		}
		if ch.Chunk != "" {
			d.renderer.RenderJobOutput(JobOutputRecord{
				Kind: "job-output", JobID: jobID, State: ch.State,
				Offset: ch.Offset, NextOffset: ch.NextOffset, Chunk: ch.Chunk,
				Written: ch.Written, HasMore: ch.HasMore, EvictedBytes: ch.EvictedBytes,
			})
		}
		consumed := !ch.HasMore && ch.NextOffset >= ch.Written
		if consumed && ch.State == "settled" && !t.terminalDone {
			// §4g uniform contract (B-F1): the ONE-TIME empty terminal
			// record follows EVERY settled-and-fully-consumed read —
			// including when that final read carried bytes (rendered
			// above), so the end-of-tail marker is deterministic for
			// machine consumers regardless of whether the last drain
			// poll raced settlement. The marker sits AT the post-read
			// cursor (offset == nextOffset == written), keeping the
			// record chain's offset == previous-nextOffset invariant.
			t.terminalDone = true
			d.renderer.RenderJobOutput(JobOutputRecord{
				Kind: "job-output", JobID: jobID, State: ch.State,
				Offset: ch.NextOffset, NextOffset: ch.NextOffset, Chunk: "",
				Written: ch.Written, HasMore: false, EvictedBytes: ch.EvictedBytes,
			})
		}
		t.cursor = ch.NextOffset
		if !ch.HasMore {
			return consumed
		}
	}
	return false
}

// resyncTailCursor handles the two typed jobs/output -32602 errors
// whose error.data carries a re-sync hint (§4g). output-evicted: the
// retention ring dropped the bytes before data.evictedBase — jump
// FORWARD to the oldest retained byte and keep paging (the retained
// tail is served from there; the evicted prefix is honestly absent,
// surfaced once here and structurally via evictedBytes on every
// subsequent record). output-ahead: the cursor is beyond the produced
// output (a client arithmetic bug — nextOffset never exceeds written)
// — clamp BACK to the server's written. Both are one-shot per call: a
// hint that does not move the cursor falls through to the caller's
// warning path, so a misbehaving server cannot spin the paging loop.
// It reports whether the cursor moved (the caller continues paging).
func (d *driver) resyncTailCursor(jobID string, t *bgTail, err error) bool {
	var perr *protocol.Error
	if !errors.As(err, &perr) || len(perr.Data) == 0 {
		return false
	}
	var hint struct {
		Kind        string `json:"kind"`
		EvictedBase int64  `json:"evictedBase"`
		Evicted     int64  `json:"evicted"`
		Written     int64  `json:"written"`
	}
	if json.Unmarshal(perr.Data, &hint) != nil {
		return false
	}
	switch hint.Kind {
	case "output-evicted":
		if hint.EvictedBase > t.cursor {
			d.note("job %s: %d tail bytes were evicted behind the output retention window — resuming at the oldest retained byte %d (the earlier %d evicted bytes are gone)",
				jobID, hint.EvictedBase-t.cursor, hint.EvictedBase, hint.Evicted)
			t.cursor = hint.EvictedBase
			return true
		}
	case "output-ahead":
		if hint.Written < t.cursor {
			d.note("job %s: cursor %d was ahead of the produced output — re-syncing back to written %d",
				jobID, t.cursor, hint.Written)
			t.cursor = hint.Written
			return true
		}
	}
	return false
}

// jobsOutputChunk is the wire shape of one jobs/output response.
type jobsOutputChunk struct {
	JobID        string `json:"jobId"`
	State        string `json:"state"`
	Chunk        string `json:"chunk"`
	Offset       int64  `json:"offset"`
	NextOffset   int64  `json:"nextOffset"`
	HasMore      bool   `json:"hasMore"`
	Written      int64  `json:"written"`
	EvictedBytes int64  `json:"evictedBytes"`
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
