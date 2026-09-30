// input.go — the client's single stdin owner (hotfix b-F1).
//
// DEFECT being fixed: the REPL and every approval responder used to
// share one *bufio.Reader under a claimed "sequential discipline",
// but each approval/request is answered on its OWN goroutine (the
// answer is a blocking Call whose response can only be read by the
// client's read loop — answering inline would deadlock). The engine's
// parallel tool pool admits ≥2 concurrently-pending approvals, so two
// responder goroutines could sit in bufio.Reader.ReadString at once:
// an unsynchronized shared buffer (data race) whose consumed line
// could then be applied to the WRONG approvalId — a grant landing on
// a tool the operator denied.
//
// FIX: stdinHub. ONE dispatcher goroutine owns the bufio.Reader for
// the whole process lifetime and routes every line it reads:
//
//   - interactive mode: lines queue in order and answer pending
//     approvals OLDEST-LINE-FIRST (prompt order — an ask registers
//     and prints its notice under one lock, so answer order is
//     exactly prompt order); with no approval waiting, lines go to
//     the REPL. A line that arrived before its approval asked is
//     delivered the moment that ask registers (a pre-scripted `yes`
//     pipe works exactly as it did with one buffered reader);
//   - --json mode: the line is parsed as {"id":"<approvalId>",
//     "approve":bool} and routed STRICTLY by approvalId — to the
//     pending approval with that id, or parked for an id that has
//     not registered yet (pre-scripted answer files work; a parked
//     answer outlives stdin EOF and delivers exactly once when its
//     id asks). A duplicate or already-settled id is REJECTED with
//     an honest stderr line — an answer is never re-applied. A line
//     that is not answer-shaped goes to the REPL in REPL mode (the
//     documented split: machine messages are plain text lines,
//     approval answers are JSON lines) and is rejected honestly in
//     one-shot mode.
//
// The REPL consumes the same ordered backlog through a condition
// variable (no second buffered reader, no closed-channel races with
// early EOF); the backlog is bounded, and a full backlog BLOCKS the
// dispatcher — kernel-pipe backpressure, so a flooded stdin cannot
// grow the client without bound.
//
// Fail-closed in every unanswerable direction, mirroring the daemon's
// approval bridge: EOF, a closed stdin, or the Ctrl-C / daemon-death
// ladder (halt) DENIES every approval that is registered and still
// unanswered at that moment. The daemon denies on its side too (its
// fail-closed bridge) — the deny here only keeps the client's honest
// bookkeeping.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// replBacklogBound bounds the ordered line backlog; a full backlog
// blocks the dispatcher (backpressure) instead of growing unboundedly.
const replBacklogBound = 64

// stdinHub is the single owner of the client's stdin reader.
type stdinHub struct {
	in   *bufio.Reader
	errw io.Writer
	json bool

	mu       sync.Mutex
	waiters  map[string]chan ApprovalAnswer // pending approvals by id
	order    []string                       // interactive registration order (FIFO)
	settled  map[string]bool                // ids already answered — never re-applied
	parked   map[string]ApprovalAnswer      // json answers that arrived before their id asked
	backlog  []string                       // ordered lines awaiting an approval or the REPL
	replOn   bool                           // a REPL is (about to be) consuming
	closed   bool                           // stdin ended/halted; unsettled asks deny fail-closed
	closeErr error                          // why stdin ended (honest late-deny notes)
	ready    *sync.Cond                     // signalled whenever the backlog shrinks or the hub closes
}

func newStdinHub(in *bufio.Reader, errw io.Writer, jsonMode bool) *stdinHub {
	h := &stdinHub{
		in:      in,
		errw:    errw,
		json:    jsonMode,
		waiters: make(map[string]chan ApprovalAnswer),
		settled: make(map[string]bool),
		parked:  make(map[string]ApprovalAnswer),
	}
	h.ready = sync.NewCond(&h.mu)
	return h
}

// start launches the dispatcher (the only goroutine that ever reads
// the shared bufio.Reader).
func (h *stdinHub) start() { go h.dispatch() }

// startRepl arms REPL consumption of the backlog.
func (h *stdinHub) startRepl() {
	h.mu.Lock()
	h.replOn = true
	h.mu.Unlock()
	h.ready.Broadcast()
}

// dispatch is the single reader goroutine: read one line, route it,
// repeat; a read error (EOF, or the Ctrl-C ladder closing our stdin)
// denies every registered-but-unanswered approval and ends the hub.
func (h *stdinHub) dispatch() {
	for {
		line, err := h.in.ReadString('\n')
		if line != "" {
			h.routeLine(line)
		}
		if err != nil {
			h.closeAll(err)
			return
		}
	}
}

// routeLine applies the mode's routing to one consumed line. A full
// backlog blocks here (backpressure), never with the lock held.
func (h *stdinHub) routeLine(line string) {
	if !h.json {
		h.enqueueBacklog(line)
		h.advance()
		return
	}
	h.routeJSONLine(line)
}

// enqueueBacklog appends one interactive line, blocking while the
// backlog is full (kernel-pipe-equivalent backpressure).
func (h *stdinHub) enqueueBacklog(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for len(h.backlog) >= replBacklogBound && !h.closed {
		h.ready.Wait()
	}
	if h.closed {
		return // halting — the line is dropped, not routed
	}
	h.backlog = append(h.backlog, line)
}

// advance delivers backlogged interactive lines to the oldest pending
// approval (a line typed before any prompt answers the next prompt —
// the single-buffer semantics this hub replaces). Caller holds no
// lock.
func (h *stdinHub) advance() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for len(h.backlog) > 0 && len(h.order) > 0 {
		line := h.backlog[0]
		h.backlog = h.backlog[1:]
		id := h.order[0]
		h.order = h.order[1:]
		ch := h.waiters[id]
		delete(h.waiters, id)
		h.settled[id] = true
		h.deliverInteractive(ch, line)
	}
	h.ready.Broadcast()
}

// deliverInteractive interprets one y/N answer and settles the waiter.
// Caller holds h.mu.
func (h *stdinHub) deliverInteractive(ch chan ApprovalAnswer, line string) {
	var ans ApprovalAnswer
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		fmt.Fprintln(h.errw, "→ granted")
		ans = ApprovalAnswer{Allow: true, Reason: ""}
	default:
		fmt.Fprintln(h.errw, "→ denied (default n — fail-closed)")
		ans = ApprovalAnswer{Allow: false, Reason: "denied by operator (default n)"}
	}
	ch <- ans
}

// routeJSONLine routes one machine-mode line strictly by approvalId.
func (h *stdinHub) routeJSONLine(line string) {
	var resp struct {
		ID      string `json:"id"`
		Approve bool   `json:"approve"`
	}
	if jerr := json.Unmarshal([]byte(line), &resp); jerr != nil || resp.ID == "" {
		// Not an answer line. In REPL machine mode a plain line is a
		// user message (messages are text, answers are JSON);
		// otherwise it is unattributable — reject honestly, apply to
		// no one.
		h.mu.Lock()
		replOn := h.replOn
		h.mu.Unlock()
		if replOn {
			h.enqueueBacklog(line)
			h.ready.Broadcast()
			return
		}
		fmt.Fprintf(h.errw, "vh-agent-client: malformed approval response %q: not a {\"id\",\"approve\"} line — ignored (never applied to an approval; a still-pending one denies at EOF/timeout)\n", strings.TrimSpace(line))
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.waiters[resp.ID]; ok {
		delete(h.waiters, resp.ID)
		h.settled[resp.ID] = true
		ch <- ApprovalAnswer{Allow: resp.Approve, Reason: approvalReasonFor(resp.Approve)}
		return
	}
	if h.settled[resp.ID] {
		fmt.Fprintf(h.errw, "vh-agent-client: approval response for already-settled id %q — ignored (an answer is never re-applied)\n", resp.ID)
		return
	}
	if _, dup := h.parked[resp.ID]; dup {
		fmt.Fprintf(h.errw, "vh-agent-client: duplicate approval response for id %q (not asked yet) — ignored (keeping the first)\n", resp.ID)
		return
	}
	h.parked[resp.ID] = ApprovalAnswer{Allow: resp.Approve, Reason: approvalReasonFor(resp.Approve)}
}

// approvalReasonFor mirrors the shipped responder reasons.
func approvalReasonFor(allow bool) string {
	if allow {
		return ""
	}
	return "denied by machine response"
}

// closeAll is the EOF ladder: every registered-but-unanswered approval
// denies fail-closed, and the hub marks itself closed (the REPL drains
// its backlog then exits; later asks deny honestly). Parked json
// answers and the interactive backlog SURVIVE — they answer later asks
// exactly once (a pre-scripted answer file piped ahead of the prompts
// keeps working, matching the single-buffer semantics this hub
// replaces). Idempotent under the closed flag.
func (h *stdinHub) closeAll(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	h.closeErr = err
	for _, id := range h.order {
		ch := h.waiters[id]
		delete(h.waiters, id)
		fmt.Fprintf(h.errw, "→ denied (no answer: %v — fail-closed)\n", err)
		ch <- ApprovalAnswer{Allow: false, Reason: "no answer (input closed) — fail-closed deny"}
	}
	h.order = nil
	// json waiters never enter h.order — deny them too.
	for id, ch := range h.waiters {
		delete(h.waiters, id)
		fmt.Fprintf(h.errw, "→ denied (no answer: %v — fail-closed)\n", err)
		ch <- ApprovalAnswer{Allow: false, Reason: "no answer (input closed) — fail-closed deny"}
	}
	h.ready.Broadcast()
}

// halt is the Ctrl-C / daemon-death ladder entry: same fail-closed
// semantics as EOF, callable from any goroutine (idempotent).
func (h *stdinHub) halt() {
	h.closeAll(errors.New("stdin halted (interrupt or daemon death)"))
}

// askFIFO registers one interactive approval and blocks for the next
// FIFO-routed answer. notice (the [y/N] prompt) runs UNDER the
// registration lock, so the prompt order is exactly the answer order —
// a line typed in response to the first shown prompt answers that
// approval, never a concurrently-pending sibling. A backlogged line
// (typed before this ask) settles the ask immediately.
func (h *stdinHub) askFIFO(approvalID string, notice func()) ApprovalAnswer {
	h.mu.Lock()
	for len(h.backlog) > 0 && len(h.order) == 0 {
		// Backlog exists with no pending approval: its head belongs to
		// THIS ask (oldest line answers the next prompt).
		line := h.backlog[0]
		h.backlog = h.backlog[1:]
		notice()
		h.settled[approvalID] = true
		ch := make(chan ApprovalAnswer, 1)
		h.deliverInteractive(ch, line)
		h.ready.Broadcast()
		h.mu.Unlock()
		return <-ch
	}
	if h.closed {
		cerr := h.closeErr
		h.mu.Unlock()
		fmt.Fprintf(h.errw, "→ denied (no answer: %v — fail-closed)\n", cerr)
		return ApprovalAnswer{Allow: false, Reason: "no answer (input closed) — fail-closed deny"}
	}
	ch := make(chan ApprovalAnswer, 1)
	h.waiters[approvalID] = ch
	h.order = append(h.order, approvalID)
	notice()
	h.mu.Unlock()
	return <-ch
}

// askByID registers one machine-mode approval and blocks for the
// answer routed to exactly this approvalId. An answer that arrived
// BEFORE registration (a pre-scripted answer file) is delivered from
// the parked table — exactly once, even after stdin EOF.
func (h *stdinHub) askByID(approvalID string) ApprovalAnswer {
	h.mu.Lock()
	if ans, ok := h.parked[approvalID]; ok {
		delete(h.parked, approvalID)
		h.settled[approvalID] = true
		h.mu.Unlock()
		return ans
	}
	if h.closed {
		h.mu.Unlock()
		return ApprovalAnswer{Allow: false, Reason: "no answer (input closed) — fail-closed deny"}
	}
	ch := make(chan ApprovalAnswer, 1)
	h.waiters[approvalID] = ch
	h.mu.Unlock()
	return <-ch
}

// replLine delivers the next REPL-bound line, blocking until one
// arrives or the hub closes; ok=false once stdin has ended (EOF or the
// Ctrl-C ladder) and no line remains.
func (h *stdinHub) replLine() (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for {
		if len(h.backlog) > 0 && len(h.order) == 0 {
			line := h.backlog[0]
			h.backlog = h.backlog[1:]
			h.ready.Broadcast() // backlog shrank — a blocked dispatcher may proceed
			return line, true
		}
		if h.closed {
			return "", false
		}
		h.ready.Wait()
	}
}

// isWaiting reports whether id is registered and unanswered (test
// introspection for synchronization).
func (h *stdinHub) isWaiting(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.waiters[id]
	return ok
}

// isSettled reports whether id already received its one answer (test
// introspection for synchronization).
func (h *stdinHub) isSettled(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.settled[id]
}
