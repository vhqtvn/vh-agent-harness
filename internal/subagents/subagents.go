// Package subagents implements the native engine's durable subagent
// activations over the session event log: parent-child topology ONLY.
//
// The semantics are ported from the dsh subagent subsystem (see
// researches/sources/deepseek-harness/session-cognition.md §subagent/,
// which transcribes docs/subsystems/subagent.md):
//
//   - TWO MODES: a one-shot run spawns with a prompt, runs to completion,
//     reports once, and settles (auto-settled by the executor callback);
//     a continuable Activation persists as its own child session — the
//     parent may send follow-up inbox messages, and settlement is
//     MANAGER-owned (the parent decides), derived from quiescence +
//     owned-children tracking.
//   - A CHILD SESSION IS ITS OWN LOG: its header carries its own session
//     id, ParentSessionID, and DelegationDepth (persisted so recursion
//     budgets survive resume — dsh persists delegationDepth in the
//     SessionHeader; the child header is the AUTHORITATIVE depth record).
//   - CHILD→PARENT REPORTING is provenance-clean: the report relay
//     (subagent/report, provenance subagent-report) enters the parent
//     surface as a USER-role context event — never assistant — so the
//     parent transcript never credits the child with runtime words it
//     did not say. The settlement notice (subagent/settled, provenance
//     subagent-settled) is MANAGER-authored, log-only, and deliberately
//     distinct from the report.
//   - THE AGENT INBOX IS THE ONLY FIFO: parent→child follow-ups
//     (subagent/message) land in the child log as addressed user-role
//     inbox messages; delivery order is append order.
//   - DEPTH FENCE: MaxDelegationDepth (default 3) is enforced fail-closed
//     at spawn — a session at depth N cannot spawn at N+1 beyond the cap
//     (error, never silent truncation). Depth comes from the session's
//     own persisted header (authoritative), cross-checked against the
//     parent's spawned record at manager (re)construction; mismatch is
//     fail-closed.
//   - SETTLEMENT IS FIRST-WINS (jobs discipline) and settled children
//     report at most once more (pending report flush; the reported
//     contentSeq is the at-most-once key).
//
// Scope fence (risk R10): any-to-any session-bus generalization is
// explicitly deferred — this slice implements parent-child only.
//
// Storage discipline (same as jobs): subagent state is a fold over
// subagent/* events in the PARENT log plus the child logs themselves,
// never a second storage. In-memory maps are fold-seeded accelerators
// only; NewManager rebuilds everything from the logs, reopening each
// child log through the injected Store and cross-checking its header.
package subagents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// DefaultMaxDelegationDepth is the default delegation recursion cap (a
// session at this depth can no longer spawn). Depth counts delegation
// edges from the root: root=0, child=1, grandchild=2, ….
const DefaultMaxDelegationDepth = 3

// Executor runs one child turn (the initial prompt or one follow-up
// inbox batch), driving the CHILD log. It is the injected seam where the
// real runtime attaches — tests script a fake; wiring it to
// Pipeline.RunTurn is a later slice (non-goal here). Run returning nil
// means the turn completed; for a one-shot child nil settles completed
// and an error settles failed. For a continuable child the manager owns
// settlement regardless of the run outcome (dsh: only the manager
// controls settlement); a failed continuable run therefore does not
// auto-settle — error propagation to the parent is the real-wiring
// slice's concern.
type Executor interface {
	Run(ctx context.Context, child Child) error
}

// Child is one delegated child session as handed to an Executor.
type Child struct {
	ID     string
	Kind   string
	Prompt string
	Role   string
	Depth  int
	Log    *session.Log
}

// Store opens child session logs, keyed by the OWNING parent session id
// plus the tree-unique child id (`<parentSessionID>.<N>`), so distinct
// children never collide. CreateChild is called at spawn; ReopenChild at
// manager (re)construction.
type Store interface {
	CreateChild(parentSessionID, childID string, header session.ChildHeader) (*session.Log, error)
	ReopenChild(parentSessionID, childID string) (*session.Log, error)
}

// FileStore is the default Store: child logs are files under
// <root>/<parentSessionID>/<childID>.jsonl.
//
// Confinement: BOTH ids are validated as strict single filename
// components (session.ValidateIDComponent) before any path is composed.
// The parent id arrives from the parent log's header and the child ids
// from the parent's subagent/spawned events — normally engine-minted,
// but a forged or foreign log could carry traversal ids
// ("../../victim"), and ReopenChild's ResumeFile TRUNCATES torn tails,
// so an unvalidated id is a write primitive. Validation is
// defense-in-depth behind the engine boundary (which already confines
// client-supplied session ids): it covers direct library use and
// hostile logs alike.
type FileStore struct {
	root string
}

// NewFileStore returns a file-backed Store rooted at dir.
func NewFileStore(dir string) *FileStore { return &FileStore{root: dir} }

// validateIDs confines both store keys to strict filename components.
func (s *FileStore) validateIDs(parentSessionID, childID string) error {
	if err := session.ValidateIDComponent(parentSessionID); err != nil {
		return fmt.Errorf("subagents: parent session id rejected: %w", err)
	}
	if err := session.ValidateIDComponent(childID); err != nil {
		return fmt.Errorf("subagents: child id rejected: %w", err)
	}
	return nil
}

// CreateChild creates the child log file (with its topology header).
func (s *FileStore) CreateChild(parentSessionID, childID string, header session.ChildHeader) (*session.Log, error) {
	if err := s.validateIDs(parentSessionID, childID); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.root, parentSessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("subagents: create child dir %s: %w", dir, err)
	}
	return session.OpenChildFile(filepath.Join(dir, childID+".jsonl"), childID, header)
}

// ReopenChild resumes an existing child log for append (torn-tail
// tolerant, same durable stream).
func (s *FileStore) ReopenChild(parentSessionID, childID string) (*session.Log, error) {
	if err := s.validateIDs(parentSessionID, childID); err != nil {
		return nil, err
	}
	return session.ResumeFile(filepath.Join(s.root, parentSessionID, childID+".jsonl"))
}

// Receipt is the immediate spawn receipt.
type Receipt struct {
	ChildID string `json:"childId"`
}

// Options configures one Manager.
type Options struct {
	// MaxDelegationDepth caps the delegation recursion; 0 ⇒
	// DefaultMaxDelegationDepth.
	MaxDelegationDepth int
}

// Manager owns the subagent lifecycle over one PARENT session log:
// spawn, child-turn execution (via the injected Executor), report
// flushing, settlement, and follow-up messaging. A Manager is bound to
// exactly one parent session, so the parent-child topology is structural.
type Manager struct {
	mu        sync.Mutex
	queueCond *sync.Cond // signaled when the run queue grows
	drained   *sync.Cond // broadcast when pending reaches 0

	parent   *session.Log
	executor Executor
	store    Store
	owner    string // parent session id (from its header)
	depth    int    // this session's delegation depth (from its header)
	maxDepth int

	queue   []string // child ids FIFO under mu
	pending int      // queued + running
	stopped bool

	children map[string]*childHandle
	order    []string // spawn order
	counter  int64

	executingID string // child currently running under the dispatcher ("" = none)
}

// childHandle couples a child's log handle with its fold-level record.
type childHandle struct {
	lg  *session.Log
	rec childRecord
	// settleWaiters are the AwaitChild waiters (closed exactly once at
	// settlement; the one-shot spawn tool blocks on these).
	settleWaiters []chan struct{}
}

// childRecord is the fold-level state of one child. reportedOriginSeq is
// the child-side origin seq of the assistant output last relayed in a
// subagent/report (the at-most-once report key, persisted as
// contentSeq in the report payload so it survives manager
// reconstruction).
type childRecord struct {
	childID           string
	kind              string
	prompt            string
	role              string
	depth             int
	spawnedSeq        int64
	settledSeq        int64
	settleResult      string
	settleReason      string
	reportedOriginSeq int64
}

// NewManager binds a Manager to parent (the owning session's log),
// executor, and store, seeding ALL state from the logs: it folds the
// parent's subagent/* events, reopens every child log through the store,
// and cross-checks each child header (session id, parent session id,
// delegation depth) against the parent's spawned record — any mismatch
// is fail-closed. It then starts the executor drain goroutine.
func NewManager(parent *session.Log, executor Executor, store Store, opts Options) (*Manager, error) {
	if parent == nil {
		return nil, errors.New("subagents: nil parent log")
	}
	if executor == nil {
		return nil, errors.New("subagents: nil executor")
	}
	if store == nil {
		return nil, errors.New("subagents: nil store")
	}
	maxDepth := EffectiveMaxDepth(opts.MaxDelegationDepth)

	events := parent.Events()
	if len(events) == 0 || events[0].Type != session.TypeSessionHeader {
		return nil, errors.New("subagents: parent log has no session header")
	}
	var hp session.HeaderPayload
	if err := unmarshal(events[0].Payload, &hp); err != nil {
		return nil, fmt.Errorf("subagents: malformed parent header: %w", err)
	}

	folded, err := FoldSubagents(events)
	if err != nil {
		return nil, err
	}
	reported := foldReportedContentSeqs(events)

	m := &Manager{
		parent:   parent,
		executor: executor,
		store:    store,
		owner:    hp.SessionID,
		depth:    hp.DelegationDepth,
		maxDepth: maxDepth,
		children: make(map[string]*childHandle),
	}
	m.queueCond = sync.NewCond(&m.mu)
	m.drained = sync.NewCond(&m.mu)

	for i := range folded {
		d := &folded[i]
		lg, err := store.ReopenChild(m.owner, d.ChildID)
		if err != nil {
			return nil, fmt.Errorf("subagents: reopen child %s: %w", d.ChildID, err)
		}
		if err := crossCheckChildHeader(lg, d.ChildID, m.owner, d.Depth); err != nil {
			return nil, err
		}
		rec := childRecord{
			childID: d.ChildID, kind: d.Kind, prompt: d.Prompt, depth: d.Depth,
			spawnedSeq: d.spawnedSeq, settleResult: d.SettledResult, settleReason: d.SettledReason,
			reportedOriginSeq: reported[d.ChildID],
		}
		if d.SettledState() {
			rec.settledSeq = d.settledSeq
		}
		m.children[d.ChildID] = &childHandle{lg: lg, rec: rec}
		m.order = append(m.order, d.ChildID)
		if n := idNumberOf(d.ChildID); n > m.counter {
			m.counter = n
		}
	}

	go m.dispatchLoop()
	return m, nil
}

// Spawn validates and queues a minimal spawn (no seeding) — the
// slice-8 surface, byte-identical to the pre-B2 behavior. See
// SpawnWithOptions for the full option set.
func (m *Manager) Spawn(kind, prompt, role string) (Receipt, error) {
	return m.SpawnWithOptions(SpawnOptions{Kind: kind, Prompt: prompt, Role: role})
}

// SpawnOptions is one spawn request. Kind/Prompt/Role mirror Spawn;
// SeedFromParent (B2 fork seeding) copies the parent's last-n COMPLETED
// turns' surface messages into the child log before the child's first
// turn (0 = no seeding — byte-identical to the legacy Spawn; n > turns
// available seeds all of them).
type SpawnOptions struct {
	Kind           string
	Prompt         string
	Role           string
	SeedFromParent int
}

// SpawnWithOptions validates the request, enforces the depth fence
// fail-closed, creates the child session log (its own log, header
// carrying ParentSessionID + DelegationDepth), optionally SEEDS it with
// the parent's last-n completed turns' surface messages (fork
// turn-prefix seeding), records the durable subagent/spawned descriptor
// (carrying the number of turns actually seeded) in the parent log,
// delivers the initial prompt to the child inbox as its first
// subagent/message, and queues the initial child turn — returning the
// receipt IMMEDIATELY (job-style enqueue semantics: the child turn runs
// on the manager's dispatch goroutine, never the caller's). One-shot
// children auto-settle when their run completes; continuable children
// persist for follow-ups.
func (m *Manager) SpawnWithOptions(opts SpawnOptions) (Receipt, error) {
	kind, prompt, role := opts.Kind, opts.Prompt, opts.Role
	if kind != session.SubagentKindOneShot && kind != session.SubagentKindContinuable {
		return Receipt{}, fmt.Errorf("subagents: invalid kind %q (want %q or %q)",
			kind, session.SubagentKindOneShot, session.SubagentKindContinuable)
	}
	if prompt == "" {
		return Receipt{}, errors.New("subagents: prompt is required")
	}
	if opts.SeedFromParent < 0 {
		return Receipt{}, fmt.Errorf("subagents: seedFromParent must be >= 0 (got %d)", opts.SeedFromParent)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return Receipt{}, errors.New("subagents: manager stopped")
	}
	// Depth fence FIRST, fail-closed: a session at depth N cannot spawn
	// at N+1 beyond the cap. Checked before any durable effect (no id
	// minted, no child log created).
	childDepth := m.depth + 1
	if childDepth > m.maxDepth {
		return Receipt{}, fmt.Errorf("subagents: delegation depth fence: session %q at depth %d cannot spawn (child depth %d exceeds cap %d)",
			m.owner, m.depth, childDepth, m.maxDepth)
	}

	m.counter++
	id := fmt.Sprintf("%s.%d", m.owner, m.counter)
	clg, err := m.store.CreateChild(m.owner, id, session.ChildHeader{
		ParentSessionID: m.owner,
		DelegationDepth: childDepth,
		Role:            role,
	})
	if err != nil {
		m.counter--
		return Receipt{}, fmt.Errorf("subagents: create child log for %s: %w", id, err)
	}
	// Fork turn-prefix seeding (B2): before the child's first turn (and
	// before the initial prompt lands), copy the parent's last-n
	// completed turns' surface messages. The role/persona hint rides the
	// child header as before.
	seededTurns := 0
	if opts.SeedFromParent > 0 {
		seededTurns, err = appendSeedTurns(clg, m.parent.Events(), opts.SeedFromParent)
		if err != nil {
			return Receipt{}, fmt.Errorf("subagents: seed child log for %s: %w", id, err)
		}
	}
	ev, err := m.parent.Append(session.TypeSubagentSpawned, nil, session.SubagentPayload{
		ChildID: id, Kind: kind, Prompt: prompt, Depth: childDepth, SeedTurns: seededTurns,
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("subagents: log subagent/spawned: %w", err)
	}
	// The initial prompt is the first inbox message (the inbox is the
	// only FIFO).
	if _, err := clg.Append(session.TypeSubagentMessage, &session.SurfaceOp{Op: session.SurfaceOpAppend}, session.SubagentPayload{
		ChildID: id, From: m.owner, Text: prompt,
	}); err != nil {
		return Receipt{}, fmt.Errorf("subagents: deliver initial prompt to %s inbox: %w", id, err)
	}

	m.children[id] = &childHandle{lg: clg, rec: childRecord{
		childID: id, kind: kind, prompt: prompt, role: role, depth: childDepth, spawnedSeq: ev.Seq,
	}}
	m.order = append(m.order, id)
	m.pending++
	m.queue = append(m.queue, id)
	m.queueCond.Signal()
	return Receipt{ChildID: id}, nil
}

// SendMessage delivers a follow-up parent→child message: it appends a
// subagent/message to the child's inbox (user-role, FIFO append order)
// and queues a follow-up child turn. Only continuable, not-yet-settled
// children accept follow-ups — everything else fails closed.
func (m *Manager) SendMessage(childID, text string) error {
	if text == "" {
		return errors.New("subagents: message text is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return errors.New("subagents: manager stopped")
	}
	h, ok := m.children[childID]
	if !ok {
		return fmt.Errorf("subagents: send to unknown child %q", childID)
	}
	if h.rec.settledSeq != 0 {
		return fmt.Errorf("subagents: child %q is settled: no further messages", childID)
	}
	if h.rec.kind != session.SubagentKindContinuable {
		return fmt.Errorf("subagents: child %q is %s: follow-ups require a continuable child", childID, h.rec.kind)
	}
	if _, err := h.lg.Append(session.TypeSubagentMessage, &session.SurfaceOp{Op: session.SurfaceOpAppend}, session.SubagentPayload{
		ChildID: childID, From: m.owner, Text: text,
	}); err != nil {
		return fmt.Errorf("subagents: deliver message to %s inbox: %w", childID, err)
	}
	m.pending++
	m.queue = append(m.queue, childID)
	m.queueCond.Signal()
	return nil
}

// Settle records the terminal state of one child: nil runErr settles
// completed, an error settles failed with its text. Settlement is
// FIRST-WINS: a second settle attempt (including the explicit settle of
// an already auto-settled one-shot) is a no-op returning nil. A pending
// unreported child output flushes at most one final report before the
// settlement notice lands (reports precede settlement).
func (m *Manager) Settle(childID string, runErr error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settleLocked(childID, runErr)
}

// settleLocked is Settle assuming m.mu is held.
func (m *Manager) settleLocked(childID string, runErr error) error {
	h, ok := m.children[childID]
	if !ok {
		return fmt.Errorf("subagents: settle unknown child %q", childID)
	}
	if h.rec.settledSeq != 0 {
		return nil // first-wins: the terminal event already landed
	}
	// Pending report flush: a settled child reports at most once more.
	if err := m.flushReportLocked(h); err != nil {
		return err
	}
	result, reason := session.JobResultCompleted, ""
	if runErr != nil {
		result, reason = session.JobResultFailed, runErr.Error()
	}
	ev, err := m.parent.Append(session.TypeSubagentSettled, nil, session.SubagentPayload{
		ChildID: childID, Kind: h.rec.kind, Result: result, Reason: reason,
		Provenance: session.SubagentProvenanceSettled,
	})
	if err != nil {
		return fmt.Errorf("subagents: log subagent/settled: %w", err)
	}
	h.rec.settledSeq = ev.Seq
	h.rec.settleResult = result
	h.rec.settleReason = reason
	// Wake every AwaitChild waiter (one-shot spawn tools block here).
	for _, ch := range h.settleWaiters {
		close(ch)
	}
	h.settleWaiters = nil
	return nil
}

// flushReportLocked relays the child's latest assistant output to the
// parent as a subagent/report (user-role context event, provenance
// subagent-report) IF it is newer than the last relayed output. The
// at-most-once key is the child-side origin seq of that output
// (persisted as contentSeq in the report payload so reconstruction
// cannot double-report).
func (m *Manager) flushReportLocked(h *childHandle) error {
	content, originSeq, ok := finalAssistant(h.lg)
	if !ok || originSeq <= h.rec.reportedOriginSeq {
		return nil
	}
	_, err := m.parent.Append(session.TypeSubagentReport, &session.SurfaceOp{Op: session.SurfaceOpAppend}, session.SubagentPayload{
		ChildID: h.rec.childID, Kind: h.rec.kind, Content: content, ContentSeq: originSeq,
		Provenance: session.SubagentProvenanceReport,
	})
	if err != nil {
		return fmt.Errorf("subagents: log subagent/report: %w", err)
	}
	h.rec.reportedOriginSeq = originSeq
	return nil
}

// finalAssistant returns the content and child-side origin seq of the
// child's latest assistant message (the report source), if any.
func finalAssistant(lg *session.Log) (string, int64, bool) {
	s, err := session.FoldSurface(lg.Events())
	if err != nil {
		return "", 0, false
	}
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "assistant" {
			return s.Messages[i].Content, s.OriginSeq[i], true
		}
	}
	return "", 0, false
}

// Snapshot returns the live derived activation states in spawn order:
// settled (settlement notice landed), running (a turn is queued or
// executing), waiting (continuable + quiescent + not settled — dsh
// derived state). One-shot children are running until they settle.
func (m *Manager) Snapshot() []Descriptor {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Descriptor, 0, len(m.order))
	queued := make(map[string]bool, len(m.queue))
	for _, id := range m.queue {
		queued[id] = true
	}
	for _, id := range m.order {
		h := m.children[id]
		d := Descriptor{
			ChildID:       h.rec.childID,
			Kind:          h.rec.kind,
			Prompt:        h.rec.prompt,
			Depth:         h.rec.depth,
			SettledResult: h.rec.settleResult,
			SettledReason: h.rec.settleReason,
			spawnedSeq:    h.rec.spawnedSeq,
			settledSeq:    h.rec.settledSeq,
		}
		switch {
		case h.rec.settledSeq != 0:
			d.State = StateSettled
		case m.executingID == id || queued[id]:
			d.State = StateRunning
		case h.rec.kind == session.SubagentKindContinuable:
			d.State = StateWaiting // continuable + quiescent + not settled
		default:
			d.State = StateRunning // one-shot: running until settled
		}
		out = append(out, d)
	}
	return out
}

// dispatchLoop is the serial pump (jobs discipline): one goroutine pulls
// child turns FIFO, runs the executor WITHOUT holding the manager lock
// (the executor drives the child log, whose own lock is always
// innermost), then under the lock flushes reports and auto-settles
// one-shot children.
func (m *Manager) dispatchLoop() {
	m.mu.Lock()
	for {
		for len(m.queue) == 0 && !m.stopped {
			m.queueCond.Wait()
		}
		if len(m.queue) == 0 { // stopped and drained
			m.mu.Unlock()
			return
		}
		id := m.queue[0]
		m.queue = m.queue[1:]
		h := m.children[id]
		m.executingID = id
		m.mu.Unlock()

		child := Child{
			ID: id, Kind: h.rec.kind, Prompt: h.rec.prompt, Role: h.rec.role,
			Depth: h.rec.depth, Log: h.lg,
		}
		runErr := m.executor.Run(context.Background(), child)

		m.mu.Lock()
		m.executingID = ""
		if h.rec.kind == session.SubagentKindContinuable {
			// Continuable: report the turn's output; settlement stays
			// parent-owned (never auto-settles).
			_ = m.flushReportLocked(h)
		} else {
			// One-shot: report once, then auto-settle (first-wins).
			_ = m.settleLocked(id, runErr)
		}
		m.pending--
		if m.pending == 0 {
			m.drained.Broadcast()
		}
	}
}

// Drain blocks until every queued or executing child turn has reached
// its post-run disposition (report flushed; one-shot settled). It is the
// test/verification seam for "all delegated work reached a terminal
// state".
func (m *Manager) Drain() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for m.pending > 0 {
		m.drained.Wait()
	}
}

// Stop halts the dispatcher once the queue has drained; it does not
// cancel an executing turn (cancellation propagation is a non-goal of
// this slice). Stop is idempotent.
func (m *Manager) Stop() {
	m.mu.Lock()
	m.stopped = true
	m.queueCond.Broadcast()
	m.mu.Unlock()
}

// idNumberOf parses the trailing `.<N>` of a `<parentSessionID>.<N>` child
// id (0 when unparseable). The parent id itself may contain dots; only the
// LAST segment is the per-parent counter.
func idNumberOf(id string) int64 {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] != '.' {
			continue
		}
		var n int64
		if _, err := fmt.Sscanf(id[i+1:], "%d", &n); err != nil {
			return 0
		}
		return n
	}
	return 0
}

// unmarshal is the shared payload decode helper.
func unmarshal(raw []byte, v any) error {
	if len(raw) == 0 {
		return errors.New("empty payload")
	}
	return json.Unmarshal(raw, v)
}
