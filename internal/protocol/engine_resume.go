// engine_resume.go — P4 session lifecycle on the wire, engine side:
// ResumeSession (open the EXISTING log — never create; the only
// truncation is RecoverTail's committed-prefix trim of a TORN tail —
// replay, derive the summary, and make the session active with
// create's supersede semantics for a DIFFERENT id; a same-id resume of
// the ACTIVE session is the typed session-active refusal, D-F1) and
// ListSessions (enumerate top-level session logs under the engine dir).
//
// Resume is NOT fork: the resumed session IS the prior durable stream
// (same file, same session id, seq continues from the recovered tail).
// session/create with an existing id remains CREATE (truncate) — the
// two entrances are deliberately distinct; the client refuses to fake
// resume through create.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// Resume refusal kinds (SessionResumeError.Kind).
const (
	// ResumeKindNotFound: no log exists at <Dir>/<sessionId>.jsonl.
	ResumeKindNotFound = "not-found"
	// ResumeKindChildSession: the log's header names a parent session —
	// v1 resumes TOP-LEVEL sessions only (children resume through their
	// parent's manager, not the wire).
	ResumeKindChildSession = "child-session"
	// ResumeKindIDMismatch: the log's header sessionId disagrees with
	// the requested id — the header is the durable identity; a
	// mismatching request is refused rather than adopted.
	ResumeKindIDMismatch = "id-mismatch"
	// ResumeKindSessionActive (D-F1): the requested id IS the engine's
	// currently-active session. A same-id resume would open a SECOND
	// live session.Log on the open durable file (independent seq
	// counter); any interleaved append mints duplicate seqs and
	// validateStructure rejects the log FOREVER. Refused up front; the
	// active session stays servable through its existing seams.
	ResumeKindSessionActive = "session-active"
)

// SessionResumeError is the typed refusal for session/resume. The
// handler maps it to a clean -32602 carrying Kind + Detail; clients can
// distinguish not-found (nothing to resume) from the structural
// refusals (child session, id mismatch) without parsing prose.
type SessionResumeError struct {
	SessionID string
	Kind      string
	Parent    string // set for child-session refusals
	Detail    string
}

func (e *SessionResumeError) Error() string {
	switch e.Kind {
	case ResumeKindChildSession:
		return fmt.Sprintf("protocol: resume %s refused: child session (parent %s): %s", e.SessionID, e.Parent, e.Detail)
	default:
		return fmt.Sprintf("protocol: resume %s refused (%s): %s", e.SessionID, e.Kind, e.Detail)
	}
}

// ResumeSummary is the session/resume result body: the recovered
// surface plus the derived identity (title, usage) the client renders.
type ResumeSummary struct {
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	// Events is the recovered event count (post torn-tail recovery).
	Events int `json:"events"`
	// Messages is the derived surface snapshot at resume time (the
	// same projection session/surface serves; multi-turn continuation
	// operates on it unchanged).
	Messages []session.Message `json:"messages"`
	// Title is the derived session title (first user prompt, one line,
	// truncated — session.DeriveTitle).
	Title string `json:"title"`
	// Usage is the replay-derived cumulative token sum over every
	// llm/response envelope (session.SumUsage).
	Usage session.Usage `json:"usage"`
	// UnsettledJobs lists fold-visible jobs with no terminal
	// job/settled event. They are REPORTED, never silently
	// re-dispatched (the R9 recovery repair is a daemon-startup
	// concern, not a resume side effect — see host-protocol.md §4e).
	UnsettledJobs []string `json:"unsettledJobs,omitempty"`
}

// SessionEntry is one row of the session/list result.
type SessionEntry struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title"`
	Events    int    `json:"events"`
	// LastActivity is the log file's mtime. Session events carry no
	// timestamps (the slice-1 determinism design), so the filesystem
	// mtime is the only activity signal the engine can state honestly.
	LastActivity time.Time `json:"lastActivity"`
}

// ResumeSession opens the EXISTING session log at <Dir>/<id>.jsonl and
// returns a live EngineSession continuing that exact durable stream,
// plus the resume summary (derived surface, title, usage, unsettled
// jobs). Fail-closed on every branch BEFORE any state changes:
//
//   - hostile/invalid ids: session.ValidateIDComponent → typed
//     *SessionPathError (the same grammar create enforces);
//   - the engine's CURRENTLY-ACTIVE session id (D-F1): typed
//     *SessionResumeError{session-active} — a same-id resume would open
//     a SECOND live session.Log on the open durable file (independent
//     seq counter); one more append through either writer mints a
//     duplicate seq and validateStructure rejects the log FOREVER
//     (unresumable, --verify-log fails, session/list fails the dir
//     closed). The active session stays servable through its existing
//     seams (session/surface et al.); resume targets a SUPERSEDED id;
//   - absent log: typed *SessionResumeError{not-found} — resume never
//     creates or truncates anything;
//   - header topology: a child log (parentSessionId set) refuses
//     naming the parent; a filename/header id mismatch refuses;
//   - structural log damage: RecoverTail's fail-closed contract.
//
// Supersede semantics mirror NewSession for a DIFFERENT session id.
// Both run under the server's createMu on the wire path, and the
// engine additionally holds its lifecycle lock (lifeMu) across the
// ENTIRE transition — validate → file open → surface derive → subagent
// supersede → activeID publish — so engine-direct callers get the same
// serialization (R2: the P4 reserve/rollback guard released the record
// mid-recovery, letting a concurrent engine-direct create publish over
// the reservation and leaving a committed resume's id unrecorded — the
// D-F1 same-id refusal could then be bypassed, opening a second live
// writer on one durable file). The previous active session's subagent
// manager is stopped and unbound, the resumed one bound; an in-flight
// turn on the old session completes atomically on its own log. Same-id
// resume is NOT a supersede — it is the session-active refusal above.
func (e *FileEngine) ResumeSession(sessionID string, sink io.Writer) (*EngineSession, *ResumeSummary, error) {
	// The lifecycle lock covers the WHOLE transition (R2): the same-id
	// check, the file recovery, the surface derivation, the supersede,
	// and the activeID publish are one non-interleaved stage. A
	// concurrent engine-direct NewSession/ResumeSession parks until
	// this transition finishes, so a successful resume always returns
	// with its own id recorded (and a failed one never publishes).
	e.lifeMu.Lock()
	defer e.lifeMu.Unlock()
	if verr := session.ValidateIDComponent(sessionID); verr != nil {
		return nil, nil, &SessionPathError{
			Path:   sessionID,
			Detail: fmt.Sprintf("sessionId rejected (%v): session/resume composes <sessionDir>/%s.jsonl and accepts only a strict filename component", verr, "<sessionId>"),
		}
	}
	// D-F1: refuse a same-id resume of the engine's active session — it
	// would open a SECOND live session.Log on the open durable file
	// (independent seq counter); one more append through either writer
	// mints a duplicate seq and validateStructure rejects the log
	// FOREVER.
	if sessionID == e.activeID {
		return nil, nil, &SessionResumeError{
			SessionID: sessionID, Kind: ResumeKindSessionActive,
			Detail: "this session is ALREADY the engine's active session — a same-id resume would open a second live writer on the open log (duplicate seqs corrupt the stream permanently); use the active session's own seams (session/surface, session/prompt), or resume a superseded session id",
		}
	}

	path := filepath.Join(e.Dir, sessionID+".jsonl")
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, &SessionResumeError{
				SessionID: sessionID, Kind: ResumeKindNotFound,
				Detail: fmt.Sprintf("no session log at %s (resume never creates one)", path),
			}
		}
		return nil, nil, fmt.Errorf("protocol: resume %s: inspect %s: %w", sessionID, path, err)
	}
	// R5: an EMPTY log is not a session (no header — session/list skips
	// it too); refusing it as the typed not-found family keeps every
	// resume refusal client-actionable instead of the untyped -32000
	// "cannot resume an empty event list" that leaked from the library
	// seam before this check existed.
	if fi.Size() == 0 {
		return nil, nil, &SessionResumeError{
			SessionID: sessionID, Kind: ResumeKindNotFound,
			Detail: fmt.Sprintf("the session log at %s is EMPTY (no header) — an empty file is not a session; nothing to resume", path),
		}
	}

	lg, err := session.ResumeFileTee(path, sink)
	if err != nil {
		return nil, nil, fmt.Errorf("protocol: resume %s: %w", sessionID, err)
	}
	// RecoverTail guarantees a well-formed non-empty log here (the
	// header-first/version/seq invariants); the topology checks below
	// are the resume-specific refusals. Every refusal AFTER the tee
	// opened the log closes it first — a typed refusal must not leak
	// the append-mode fd (FD cluster).
	var hp session.HeaderPayload
	if uerr := json.Unmarshal(lg.Events()[0].Payload, &hp); uerr != nil {
		_ = lg.Close()
		return nil, nil, fmt.Errorf("protocol: resume %s: malformed header payload: %w", sessionID, uerr)
	}
	if hp.ParentSessionID != "" {
		_ = lg.Close()
		return nil, nil, &SessionResumeError{
			SessionID: sessionID, Kind: ResumeKindChildSession, Parent: hp.ParentSessionID,
			Detail: "v1 resumes top-level sessions only; a child resumes through its parent's subagent manager",
		}
	}
	if hp.SessionID != sessionID {
		_ = lg.Close()
		return nil, nil, &SessionResumeError{
			SessionID: sessionID, Kind: ResumeKindIDMismatch,
			Detail: fmt.Sprintf("log header sessionId %q disagrees with the requested id — the header is the durable identity", hp.SessionID),
		}
	}

	if e.SpillPolicyFor != nil {
		lg.SetSpillPolicy(e.SpillPolicyFor(sessionID))
	}
	m, err := jobs.NewManager(lg, e.Executor, e.JobsOpts)
	if err != nil {
		_ = lg.Close()
		return nil, nil, fmt.Errorf("protocol: resume %s: jobs manager: %w", sessionID, err)
	}
	es := &EngineSession{Log: lg, Jobs: m, Schedules: e.Schedules, Path: path}

	// Unsettled jobs are REPORTED from the seeded fold (queued or
	// running in Snapshot terms). No re-dispatch, no synthetic settle:
	// those are the R9 startup-recovery decisions, deliberately NOT
	// taken on the interactive resume path.
	var unsettled []string
	for _, st := range m.Snapshot() {
		if st.State != jobs.StateSettled {
			unsettled = append(unsettled, st.JobID)
		}
	}

	// Derive the summary surface BEFORE any engine state changes (R3):
	// lg.Surface is the one fallible derivation left, and a
	// surface-invalid (but structurally valid) log must refuse with the
	// subagent registry/manager state UNTOUCHED — pre-R3 the supersede
	// block below ran first, unbinding and stopping the active
	// session's manager before this refusal fired, contradicting the
	// fail-closed-before-any-state-changes contract above. Title/usage
	// are total folds (they cannot refuse) but are derived here too so
	// the whole summary is settled before the supersede.
	msgs, derr := lg.Surface()
	if derr != nil {
		_ = lg.Close()
		return nil, nil, fmt.Errorf("protocol: resume %s: derive surface: %w", sessionID, derr)
	}
	events := lg.Events()
	title := session.DeriveTitle(events)
	usage := session.SumUsage(events)

	if e.SubagentExecutor != nil {
		store := e.SubagentStore
		if store == nil {
			store = subagents.NewFileStore(filepath.Join(e.Dir, "subagents"))
		}
		sm, err := subagents.NewManager(lg, e.SubagentExecutor, store, e.SubagentOpts)
		if err != nil {
			_ = lg.Close()
			return nil, nil, fmt.Errorf("protocol: resume %s: subagent manager: %w", sessionID, err)
		}
		es.Subagents = sm
		// Supersede discipline, mirrored from NewSession: the previous
		// active session's manager stops accepting spawns and drains;
		// the registry binding follows.
		e.subMu.Lock()
		if e.curSub != nil {
			if e.SubagentRegistry != nil {
				e.SubagentRegistry.Remove(e.curSubID)
			}
			e.curSub.Stop()
		}
		e.curSub = sm
		e.curSubID = sessionID
		e.subMu.Unlock()
		if e.SubagentRegistry != nil {
			e.SubagentRegistry.Put(sessionID, sm)
		}
	}

	sum := &ResumeSummary{
		SessionID:     sessionID,
		Path:          path,
		Events:        len(events),
		Messages:      msgs,
		Title:         title,
		Usage:         usage,
		UnsettledJobs: unsettled,
	}
	// The active-id publish is the LAST state change of the transition,
	// under the lifecycle lock held since entry (R2): the resume
	// returns with its id recorded — atomic with the supersede above,
	// never publishable by a failed attempt.
	e.activeID = sessionID
	return es, sum, nil
}

// ListSessions enumerates the TOP-LEVEL session logs directly under
// the engine dir (*.jsonl, files only): the durable sessions this
// engine can resume. Child logs (header parentSessionId set — normally
// parked under <dir>/subagents/) are excluded BY HEADER TOPOLOGY even
// when found at the top level; non-.jsonl entries are ignored.
//
// Ordering: newest lastActivity first; ties broken by sessionId
// ascending (deterministic). lastActivity is the file's mtime —
// session events carry no timestamps, and the engine states its rule
// rather than inventing one.
//
// Fail-closed: a structurally corrupt *.jsonl fails the whole listing
// with a typed error naming the file (the operator cleans it; a
// half-trusted listing is worse than an honest refusal). A TORN final
// record (crashed writer, no trailing newline) is tolerated — the
// crashed session still lists.
func (e *FileEngine) ListSessions() ([]SessionEntry, error) {
	root := e.Dir
	if root == "" {
		root = "."
	}
	matches, err := filepath.Glob(filepath.Join(root, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("protocol: list sessions: %w", err)
	}
	entries := make([]SessionEntry, 0, len(matches))
	for _, path := range matches {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("protocol: list sessions: stat %s: %w", path, err)
		}
		if fi.IsDir() {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("protocol: list sessions: open %s: %w", path, err)
		}
		events, _, _, rerr := session.RecoverTail(f)
		if cerr := f.Close(); rerr == nil {
			rerr = cerr
		}
		if rerr != nil {
			return nil, fmt.Errorf("protocol: list sessions: %s: %w", path, rerr)
		}
		if len(events) == 0 {
			continue // an empty file is not a session (no header)
		}
		var hp session.HeaderPayload
		if uerr := json.Unmarshal(events[0].Payload, &hp); uerr != nil {
			return nil, fmt.Errorf("protocol: list sessions: %s: malformed header: %w", path, uerr)
		}
		if hp.ParentSessionID != "" {
			continue // child session: listed through its parent's fold, not here
		}
		entries = append(entries, SessionEntry{
			SessionID:    hp.SessionID,
			Title:        session.DeriveTitle(events),
			Events:       len(events),
			LastActivity: fi.ModTime(),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].LastActivity.Equal(entries[j].LastActivity) {
			return entries[i].LastActivity.After(entries[j].LastActivity)
		}
		return entries[i].SessionID < entries[j].SessionID
	})
	return entries, nil
}

// errResumeDisabled is the honest refusal when the engine behind the
// wire does not implement the resume seam (non-FileEngine compositions).
var errResumeDisabled = errors.New("protocol: this engine does not implement session/resume")
