// engine.go — FileEngine: the REAL-seam Engine composition (file-backed
// session logs each with a jobs.Manager over the injected Executor, the
// shared LLM adapter, and a lazily-built tools.Pipeline that consults
// the protocol approval bridge). This is the wiring a future main()
// hands to NewServer, and the wiring the protocol tests drive — there
// is deliberately no second, fake composition.
package protocol

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// FileEngine is the real-seam Engine: session files on disk, durable
// async jobs per session, one adapter, one tool pipeline.
//
// Composition contract: build the engine, pass it to NewServer (which
// injects the wire approval bridge via SetApprover), and ONLY THEN
// register tools/observers on Pipeline() — the pipeline freezes its
// decision lattice (approver included) at construction, so an
// early-built pipeline would silently miss the approval bridge.
type FileEngine struct {
	// Dir is the default directory for session logs when session/create
	// carries no explicit path.
	Dir string
	// Executor runs job bodies (the real runtime seam).
	Executor jobs.Executor
	// JobsOpts configure each session's jobs.Manager.
	JobsOpts jobs.Options
	// Ad is the LLM adapter used by prompt turns.
	Ad adapters.Adapter
	// TurnOpts are the adapter parameters for prompt turns.
	TurnOpts tools.TurnOptions
	// SubagentExecutor, when non-nil, arms a per-session subagents.
	// Manager on every NewSession (the subagent/* wire methods). It runs
	// one CHILD turn per queued dispatch (the real executor attaches
	// here — cmd/vh-agentd wires Pipeline.RunTurn). Nil keeps the engine
	// subagent-free: subagent/spawn and subagent/send fail closed
	// (-32000), subagent/list still folds.
	SubagentExecutor subagents.Executor
	// SubagentStore opens child session logs; nil ⇒ a FileStore rooted
	// at <Dir>/subagents (independent of per-session create paths).
	SubagentStore subagents.Store
	// SubagentOpts configure each session's manager (0 depth cap ⇒ the
	// subagents default of 3).
	SubagentOpts subagents.Options
	// Schedules, when non-nil, is stamped onto EVERY session as its
	// schedule seam (the schedule/* wire methods). The engine does NOT
	// build, start, or stop the scheduler — the composition root owns
	// it (vh-agentd hands its tracker-routed scheduler here after
	// buildScheduler and BEFORE Serve; the single deliberate
	// post-construction assignment, race-free because no session can
	// exist yet). Nil keeps sessions schedule-free: schedule/add and
	// schedule/remove fail closed -32000, schedule/list is an honest
	// empty.
	Schedules ScheduleManager

	mu       sync.Mutex
	approver tools.Approver
	pipeline *tools.Pipeline

	subMu  sync.Mutex
	curSub SubagentSpawner // manager of the ACTIVE session (superseded ⇒ stopped)
}

var (
	_ Engine              = (*FileEngine)(nil)
	_ ApprovalAwareEngine = (*FileEngine)(nil)
)

// SessionPathError marks a client-supplied session path rejected by the
// engine's confinement check (TB-F1): the path did not resolve inside
// the engine's session Dir, so creating (truncating) a file there is
// refused before any filesystem or server state changes. The wire
// handler maps it to a clean ErrInvalidParams JSON-RPC error.
type SessionPathError struct {
	Path   string
	Detail string
}

func (e *SessionPathError) Error() string {
	return fmt.Sprintf("protocol: session path %q rejected: %s", e.Path, e.Detail)
}

// escapesRoot reports whether resolved target leaves root: the relative
// path from root to target is malformed or climbs with "..". rel == "."
// (target IS the root) is rejected separately by the caller where a
// strict descendant is required.
func escapesRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// confineSessionPath validates a client-supplied session/create path
// (TB-F1: os.Create is O_TRUNC — an unconfined path lets a wire peer
// truncate ANY file the process can write) and returns the canonical
// on-disk candidate, which MUST resolve inside root. Fail-closed at
// each step:
//
//  1. lexical containment — the cleaned candidate (absolute paths are
//     admitted ONLY when inside root; relative paths are joined onto
//     root, so "../x" escapes and is caught) must be a strict
//     descendant of root: no ".." climb, not root itself;
//  2. symlink containment — EvalSymlinks resolves root and the
//     candidate's PARENT directory (the file itself need not exist
//     yet); the resolved parent must still be inside the resolved
//     root, so a symlinked directory leading outside is rejected.
//     Resolution failures (missing/unreadable dir) reject too: an
//     unresolvable location is an unknown location;
//  3. final component — a symlink AT the target is rejected outright:
//     os.Create follows it and truncates whatever it points at, and an
//     in-root symlink target is indistinguishable from an escape at
//     create time, so both are refused.
//
// The residual TOCTOU window between these checks and os.Create belongs
// to a local-filesystem adversary racing the validation; the wire peer
// — the threat modeled here — cannot win it (it controls only the path
// string, not the filesystem).
func confineSessionPath(root, path string) (string, error) {
	reject := func(detail string) (string, error) {
		return "", &SessionPathError{Path: path, Detail: detail}
	}
	if root == "" {
		// An engine with no declared Dir roots sessions at the process
		// working directory (the default-path branch already does).
		root = "."
	}
	root = filepath.Clean(root)

	var candidate string
	if filepath.IsAbs(path) {
		candidate = filepath.Clean(path)
	} else {
		candidate = filepath.Join(root, path)
	}
	if rel, err := filepath.Rel(root, candidate); err != nil || rel == "." {
		return reject(fmt.Sprintf("must be a file inside the session root %s", root))
	}
	if escapesRoot(root, candidate) {
		return reject(fmt.Sprintf("must resolve inside the session root %s", root))
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return reject(fmt.Sprintf("session root %s unresolved: %v", root, err))
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(candidate))
	if err != nil {
		return reject(fmt.Sprintf("parent directory of %s unresolved (it must exist and not escape via symlink): %v", path, err))
	}
	if escapesRoot(realRoot, realParent) {
		return reject(fmt.Sprintf("symlinked parent resolves outside the session root %s", root))
	}

	if fi, err := os.Lstat(candidate); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return reject("final path component is a symlink (create would truncate its target)")
		}
	} else if !os.IsNotExist(err) {
		return reject(fmt.Sprintf("cannot inspect the target: %v", err))
	}
	return candidate, nil
}

// SetApprover stores the wire approval bridge (called by NewServer
// before any request is served).
func (e *FileEngine) SetApprover(a tools.Approver) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.approver = a
}

// Adapter returns the LLM adapter.
func (e *FileEngine) Adapter() adapters.Adapter { return e.Ad }

// TurnOptions returns the adapter parameters for turns.
func (e *FileEngine) TurnOptions() tools.TurnOptions { return e.TurnOpts }

// TurnRunner returns the tool pipeline (built lazily so the approval
// bridge injected by NewServer is already in place).
func (e *FileEngine) TurnRunner() TurnRunner { return e.Pipeline() }

// Pipeline builds (once) and returns the tool pipeline.
func (e *FileEngine) Pipeline() *tools.Pipeline {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pipeline == nil {
		e.pipeline = tools.NewPipelineWithOptions(tools.PipelineOptions{Approver: e.approver})
	}
	return e.pipeline
}

// NewSession opens the durable session at path (Dir + <id>.jsonl when
// path is empty): the log file plus the notification fan-out both
// receive every appended record (session.Log writes exactly one
// complete JSONL line per Write, so the fan-out sees whole events),
// and a jobs.Manager binds the async-jobs lifecycle to the same log.
//
// Confinement (TB-F1, round 2 — BOTH branches validated before any
// file is touched, wire shape unchanged):
//
//   - sessionID is ALWAYS validated by session.ValidateIDComponent as
//     a strict single filename component — the default-path branch
//     composes Dir + <id>.jsonl, and an unvalidated id ("../victim")
//     escapes the session root through filepath.Join's lexical
//     cleaning. The id also seeds the subagents FileStore parent
//     directory, so it is confined regardless of branch.
//   - an explicit path is validated+confined to Dir by
//     confineSessionPath.
//
// A rejected id or path returns a typed *SessionPathError with no
// partial state (no file created or truncated, no session superseded).
//
// Error-after-create hygiene (F2/F3): every failure path AFTER
// os.Create succeeded closes the file AND removes the partial file —
// os.Create already truncated, so pre-existing bytes are gone either
// way and removing keeps "no abandoned partial session" honest. The
// log's own Close is NOT sufficient on this seam: session.NewLog wraps
// an io.MultiWriter, so lg.closer is nil and lg.Close() would leak the
// file descriptor.
func (e *FileEngine) NewSession(path, sessionID string, sink io.Writer) (*EngineSession, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("protocol: sessionID is required")
	}
	if verr := session.ValidateIDComponent(sessionID); verr != nil {
		return nil, &SessionPathError{
			Path:   sessionID,
			Detail: fmt.Sprintf("sessionId rejected (%v): it must be a strict filename component ^[A-Za-z0-9][A-Za-z0-9._-]*$ — it names %s/<id>.jsonl in the session root", verr, e.Dir),
		}
	}
	if path == "" {
		path = filepath.Join(e.Dir, sessionID+".jsonl")
	} else {
		confined, err := confineSessionPath(e.Dir, path)
		if err != nil {
			return nil, err
		}
		path = confined
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("protocol: create session file %s: %w", path, err)
	}
	// abandon removes every trace of the partial session (F2/F3). It
	// is the error path for everything below: close the fd explicitly
	// (lg.closer is nil on this seam), then remove the partial file.
	abandon := func(err error) error {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	lg, err := session.NewLog(io.MultiWriter(f, sink), sessionID, time.Now().UTC())
	if err != nil {
		return nil, abandon(err)
	}
	m, err := jobs.NewManager(lg, e.Executor, e.JobsOpts)
	if err != nil {
		_ = lg.Close()
		return nil, abandon(err)
	}
	es := &EngineSession{Log: lg, Jobs: m, Schedules: e.Schedules, Path: path}
	if e.SubagentExecutor != nil {
		store := e.SubagentStore
		if store == nil {
			store = subagents.NewFileStore(filepath.Join(e.Dir, "subagents"))
		}
		sm, err := subagents.NewManager(lg, e.SubagentExecutor, store, e.SubagentOpts)
		if err != nil {
			_ = lg.Close()
			return nil, abandon(fmt.Errorf("protocol: subagent manager: %w", err))
		}
		es.Subagents = sm
		// Supersede discipline (v1 single-active-session): the previous
		// session's manager stops accepting spawns and its dispatcher
		// drains its queue. Stop does not cancel an executing child turn
		// (cancellation propagation is the subagents slice's documented
		// non-goal); its report/settle appends land on that session's
		// own log.
		e.subMu.Lock()
		if e.curSub != nil {
			e.curSub.Stop()
		}
		e.curSub = sm
		e.subMu.Unlock()
	}
	return es, nil
}
