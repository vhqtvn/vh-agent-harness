// engine_resume_test.go — P4 session/resume + session/list at the
// ENGINE seam. The load-bearing pinned adversarial test: an existing
// log survives resume BYTE-INTACT (session/create with an existing id
// TRUNCATES via os.Create; resume must never create, never truncate,
// never rewrite — only append after the recovered tail).
package protocol

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// resumeExec is a jobs.Executor that records dispatches and never
// settles on its own (settlement needs an explicit Settle call) — the
// observe-but-stay-quiet seam for resume tests.
type resumeExec struct {
	dispatched []string
}

func (e *resumeExec) Run(ctx context.Context, job jobs.Job) error {
	e.dispatched = append(e.dispatched, job.ID)
	return nil
}

// newResumeEngine builds a FileEngine over a temp dir.
func newResumeEngine(t *testing.T) (*FileEngine, string, *resumeExec) {
	t.Helper()
	dir := t.TempDir()
	ex := &resumeExec{}
	return &FileEngine{Dir: dir, Executor: ex}, dir, ex
}

// seedSession creates a session through the REAL engine seam with one
// prompt + one response and returns the log path.
func seedSession(t *testing.T, e *FileEngine, id, prompt string) string {
	t.Helper()
	es, err := e.NewSession("", id, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := es.Log.AppendPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	if _, err := es.Log.AppendLLMResponse("m", "assistant reply", nil, session.Usage{
		PromptTokens: 11, CompletionTokens: 4, TotalTokens: 15,
	}); err != nil {
		t.Fatal(err)
	}
	return es.Path
}

// TestResumeSessionNeverTruncates is the PINNED ADVERSARIAL test the
// P4 charter demands: create-with-existing-id truncates (os.Create);
// resume-with-existing-id must leave every prior byte intact and
// continue the stream — the exact crash-recovery semantics of
// session.ResumeFile, now on the engine seam.
func TestResumeSessionNeverTruncates(t *testing.T) {
	e, dir, _ := newResumeEngine(t)
	path := seedSession(t, e, "sess-keep", "preserve my history")
	// D-F1: same-id resume of the ENGINE'S ACTIVE session is refused,
	// so the seeding create is displaced by one more session first —
	// sess-keep is resumable, just not while it is the active one.
	if _, err := e.NewSession("", "sess-next", io.Discard); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var fanout bytesSink
	es, sum, err := e.ResumeSession("sess-keep", &fanout)
	if err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(before)) {
		t.Fatalf("resume rewrote the prior stream:\nbefore %q\nafter  %q", before, after)
	}
	if sum.Events != 3 { // header + prompt + llm/response
		t.Fatalf("summary events = %d, want 3", sum.Events)
	}
	if sum.Title != "preserve my history" {
		t.Fatalf("summary title = %q", sum.Title)
	}
	if sum.Usage != (session.Usage{PromptTokens: 11, CompletionTokens: 4, TotalTokens: 15}) {
		t.Fatalf("summary usage = %+v", sum.Usage)
	}
	if len(sum.Messages) != 2 {
		t.Fatalf("summary messages = %d, want 2 (user + assistant)", len(sum.Messages))
	}

	// Appends continue the SAME stream and reach the tee.
	if _, err := es.Log.AppendPrompt("after resume"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fanout.String(), "after resume") {
		t.Fatalf("fanout tee did not observe the post-resume append")
	}
	// The engine dir holds exactly the two seeded logs — resume added
	// none (sess-next is the displacing create above).
	entries, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(entries) != 2 {
		t.Fatalf("resume created extra files: %v", entries)
	}
}

func TestResumeSessionAbsentFailsClosedNeverCreates(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	_, _, err := e.ResumeSession("sess-nope", io.Discard)
	if err == nil {
		t.Fatal("resuming an absent session must fail closed")
	}
	var sre *SessionResumeError
	if !errors.As(err, &sre) || sre.Kind != ResumeKindNotFound {
		t.Fatalf("err = %v, want typed SessionResumeError not-found", err)
	}
	if _, statErr := os.Stat(filepath.Join(e.Dir, "sess-nope.jsonl")); !os.IsNotExist(statErr) {
		t.Fatalf("resume must never create the log: stat = %v", statErr)
	}
}

func TestResumeSessionHostileIDsRejected(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	for _, id := range []string{"", "..", ".", "../victim", "sess/x", `sess\y`, "sess id", "-leading"} {
		_, _, err := e.ResumeSession(id, io.Discard)
		if err == nil {
			t.Fatalf("hostile id %q must be rejected", id)
		}
		var spe *SessionPathError
		if !errors.As(err, &spe) {
			t.Fatalf("id %q: err = %v, want *SessionPathError", id, err)
		}
	}
}

func TestResumeSessionChildRefusedNamingParent(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	// A subagent CHILD log parked directly in the engine dir (forged or
	// copied): resume must refuse on the HEADER topology, not the path.
	path := filepath.Join(e.Dir, "sess-kid.jsonl")
	lg, err := session.OpenChildFile(path, "sess-kid", session.ChildHeader{
		ParentSessionID: "sess-parent", DelegationDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = e.ResumeSession("sess-kid", io.Discard)
	if err == nil {
		t.Fatal("child session resume must be refused (v1: top-level only)")
	}
	var sre *SessionResumeError
	if !errors.As(err, &sre) || sre.Kind != ResumeKindChildSession {
		t.Fatalf("err = %v, want typed child-session refusal", err)
	}
	if sre.Parent != "sess-parent" {
		t.Fatalf("refusal must name the parent session, got %q", sre.Parent)
	}
}

func TestResumeSessionHeaderIDMismatchRefused(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	// A log whose filename and header identity disagree: the header is
	// the durable identity; a mismatching request is refused rather
	// than adopted (fail-closed on malformed input).
	path := filepath.Join(e.Dir, "sess-a.jsonl")
	lg, err := session.OpenFile(path, "sess-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = e.ResumeSession("sess-a", io.Discard)
	var sre *SessionResumeError
	if !errors.As(err, &sre) || sre.Kind != ResumeKindIDMismatch {
		t.Fatalf("err = %v, want typed id-mismatch refusal", err)
	}
}

func TestResumeReportsUnsettledJobsWithoutRedispatch(t *testing.T) {
	e, _, ex := newResumeEngine(t)
	path := filepath.Join(e.Dir, "sess-jobs.jsonl")
	lg, err := session.OpenFile(path, "sess-jobs")
	if err != nil {
		t.Fatal(err)
	}
	// A settled job (enqueue → settle) and a never-settled one.
	if _, err := lg.Append(session.TypeJobEnqueued, nil, session.JobPayload{
		JobID: "background-0", Kind: "background", Owner: "sess-jobs",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := lg.Append(session.TypeJobSettled, nil, session.JobPayload{
		JobID: "background-0", Kind: "background", Owner: "sess-jobs", Result: "completed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := lg.Append(session.TypeJobEnqueued, nil, session.JobPayload{
		JobID: "background-1", Kind: "background", Owner: "sess-jobs",
	}); err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	es, sum, err := e.ResumeSession("sess-jobs", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.UnsettledJobs) != 1 || sum.UnsettledJobs[0] != "background-1" {
		t.Fatalf("unsettled jobs = %v, want [background-1]", sum.UnsettledJobs)
	}
	// The seeded manager mirrors the fold WITHOUT re-dispatching: the
	// never-settled job stays queued (a re-dispatch through resumeExec
	// would run it — dispatched stays empty proves the no-silent-
	// re-dispatch contract).
	st := es.Jobs.Snapshot()
	if len(st) != 2 || st[0].State != "settled" || st[1].State != "queued" {
		t.Fatalf("snapshot = %+v", st)
	}
	if len(ex.dispatched) != 0 {
		t.Fatalf("resume re-dispatched jobs %v — reporting only, never silent re-dispatch", ex.dispatched)
	}
}

// TestResumeSupersedesSubagentLifecycle (re-pinned by D-F1): the
// supersede discipline is the DIFFERENT-id story — resuming a
// non-active session stops and unbinds the active one's manager and
// rebinds the resumed id with a NEW manager. Same-id resume of the
// ACTIVE session is REFUSED (see TestResumeActiveSessionRefused below)
// — the pre-P4-hotfix test pinned same-id resume as supported, which is
// exactly the behavior D-F1 indicts (a second live log on the same
// durable file corrupts the stream).
func TestResumeSupersedesSubagentLifecycle(t *testing.T) {
	reg := subagents.NewRegistry()
	e, _, _ := newResumeEngine(t)
	e.SubagentExecutor = stuckChild{}
	e.SubagentRegistry = reg

	esA, err := e.NewSession("", "sess-a", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("sess-a"); !ok {
		t.Fatal("create must bind sess-a in the registry")
	}

	// A second create supersedes sess-a: unbound, its manager stopped.
	if _, err := e.NewSession("", "sess-b", io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Get("sess-a"); ok {
		t.Fatal("the superseding create must unbind sess-a")
	}
	if _, ok := reg.Get("sess-b"); !ok {
		t.Fatal("the superseding create must bind sess-b")
	}

	// Same-id resume of the ACTIVE session (sess-b) is refused: a
	// second live log on sess-b's open file would mint duplicate seqs.
	_, _, err = e.ResumeSession("sess-b", io.Discard)
	var sre *SessionResumeError
	if !errors.As(err, &sre) || sre.Kind != ResumeKindSessionActive {
		t.Fatalf("resume of the active session: err = %v, want typed session-active refusal", err)
	}

	// Different-id resume still supersedes: resuming the (superseded)
	// sess-a stops/unbinds sess-b and rebinds sess-a with a NEW manager.
	if _, _, err := e.ResumeSession("sess-a", io.Discard); err != nil {
		t.Fatal(err)
	}
	m2, ok := reg.Get("sess-a")
	if !ok {
		t.Fatal("resume must rebind the registry entry")
	}
	if m2 == esA.Subagents {
		t.Fatal("resume must install a NEW manager for the resumed session")
	}
	if _, ok := reg.Get("sess-b"); ok {
		t.Fatal("resume supersede must unbind the previously active session")
	}
}

// TestResumeActiveSessionRefusedLogStaysVerifiable is the D-F1
// regression, corruption-shaped: resuming the engine's CURRENTLY-ACTIVE
// session id used to open a SECOND live session.Log on the same durable
// file (independent seq counter). With an in-flight appending writer on
// the original log, one more append through either writer mints a
// duplicate seq and validateStructure rejects the log FOREVER
// (unresumable, --verify-log fails, session/list closes the dir). The
// fix refuses the same-id resume up front (typed session-active);
// the live session keeps appending and the log stays replayable.
func TestResumeActiveSessionRefusedLogStaysVerifiable(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	es, err := e.NewSession("", "sess-live", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// The in-flight appending writer on the ACTIVE session.
	if _, err := es.Log.AppendPrompt("in flight"); err != nil {
		t.Fatal(err)
	}

	es2, _, rerr := e.ResumeSession("sess-live", io.Discard)
	if rerr == nil {
		// PRE-FIX SHAPE — the indicted behavior: a second live log now
		// exists on the same file. Drive the corruption to capture the
		// red honestly: the in-flight ORIGINAL writer appends its seq 3,
		// then the resumed writer (whose counter recovered the tail
		// BEFORE that append) mints seq 3 AGAIN — duplicate seqs, and
		// the durable log is rejected on every later replay. (The
		// reverse order is no better: the original log's create-time fd
		// writes at its stale offset and silently OVERWRITES the
		// resumed writer's record — data loss instead of duplication.)
		if _, err := es.Log.AppendPrompt("via original writer"); err != nil {
			t.Fatal(err)
		}
		if _, err := es2.Log.AppendPrompt("via second writer"); err != nil {
			t.Fatal(err)
		}
		if _, verr := session.ReplayFile(es.Path); verr != nil {
			t.Fatalf("same-id resume of the active session corrupted the durable log: %v", verr)
		}
		t.Fatal("same-id resume of the active session must be refused (second live writer on the open log)")
	}
	var sre *SessionResumeError
	if !errors.As(rerr, &sre) || sre.Kind != ResumeKindSessionActive {
		t.Fatalf("err = %v, want typed session-active refusal", rerr)
	}
	// The refusal left the live session untouched: one more append
	// through the ORIGINAL writer, then the log replays clean.
	if _, err := es.Log.AppendPrompt("still healthy"); err != nil {
		t.Fatal(err)
	}
	if _, verr := session.ReplayFile(es.Path); verr != nil {
		t.Fatalf("log must stay valid after the refusal: %v", verr)
	}
}

// TestResumeRefusalsDoNotLeakFileDescriptors (FD cluster): the typed
// refusals that fire AFTER ResumeFileTee opens the log (child-session,
// id-mismatch) must Close it before returning — pre-fix they returned
// without Close, leaking one append-mode fd per refusal. Leak-shaped:
// loop the refusals and count /proc/self/fd (Linux; elsewhere the fd
// assertion is skipped and the typed-refusal KINDS remain covered by
// the dedicated tests above — stated honestly, no fake portability).
func TestResumeRefusalsDoNotLeakFileDescriptors(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	// A child log parked at the top level → child-session refusal.
	kidPath := filepath.Join(e.Dir, "sess-kid.jsonl")
	lg, err := session.OpenChildFile(kidPath, "sess-kid", session.ChildHeader{ParentSessionID: "sess-parent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	// A filename/header mismatch → id-mismatch refusal.
	mmPath := filepath.Join(e.Dir, "sess-a.jsonl")
	lg2, err := session.OpenFile(mmPath, "sess-b")
	if err != nil {
		t.Fatal(err)
	}
	if err := lg2.Close(); err != nil {
		t.Fatal(err)
	}

	fds := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Skipf("/proc/self/fd not readable (%v); typed-refusal coverage lives in the kind tests", err)
		}
		return len(entries)
	}
	runtime.GC() // settle finalizer noise before the baseline
	before := fds()
	for i := 0; i < 40; i++ {
		if _, _, err := e.ResumeSession("sess-kid", io.Discard); err == nil {
			t.Fatal("child-session refusal must fire")
		}
		if _, _, err := e.ResumeSession("sess-a", io.Discard); err == nil {
			t.Fatal("id-mismatch refusal must fire")
		}
	}
	after := fds()
	if growth := after - before; growth > 2 {
		t.Fatalf("typed resume refusals leaked file descriptors: fd count %d → %d (+%d) over 80 refusals", before, after, growth)
	}
}

// --- session/list -----------------------------------------------------------

func TestListSessionsOrdersByLastActivityDesc(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	p1 := seedSession(t, e, "sess-old", "oldest prompt")
	p2 := seedSession(t, e, "sess-new", "newest prompt")
	base := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(p1, base, base.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p2, base, base); err != nil {
		t.Fatal(err)
	}

	entries, err := e.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].SessionID != "sess-new" || entries[1].SessionID != "sess-old" {
		t.Fatalf("order = [%s, %s], want newest first", entries[0].SessionID, entries[1].SessionID)
	}
	if entries[0].Title != "newest prompt" || entries[1].Events != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	if !entries[0].LastActivity.Equal(base) {
		t.Fatalf("lastActivity = %v, want file mtime %v", entries[0].LastActivity, base)
	}
}

func TestListSessionsExcludesChildrenAndNonLogs(t *testing.T) {
	e, dir, _ := newResumeEngine(t)
	seedSession(t, e, "sess-top", "top level only")
	// A child log under the engine dir (defensive: children normally
	// live under <dir>/subagents/): excluded by HEADER topology.
	childPath := filepath.Join(dir, "sess-child.jsonl")
	lg, err := session.OpenChildFile(childPath, "sess-child", session.ChildHeader{ParentSessionID: "sess-top"})
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	// The subagents store root and a stray non-log file: not listed.
	if err := os.MkdirAll(filepath.Join(dir, "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subagents", "sess-sub.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := e.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].SessionID != "sess-top" {
		t.Fatalf("entries = %+v, want only sess-top", entries)
	}
}

func TestListSessionsCorruptLogFailsClosed(t *testing.T) {
	e, dir, _ := newResumeEngine(t)
	if err := os.WriteFile(filepath.Join(dir, "sess-bad.jsonl"), []byte("not json at all\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ListSessions(); err == nil {
		t.Fatal("a corrupt .jsonl in the session root must fail the listing closed")
	}
}

func TestListSessionsTornTailTolerated(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	p := seedSession(t, e, "sess-crash", "crashed mid write")
	// Append a torn (non-newline-terminated) fragment: the crashed
	// session must still list.
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":99,"type":"llm/re`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := e.ListSessions()
	if err != nil {
		t.Fatalf("torn tail must not fail the listing: %v", err)
	}
	if len(entries) != 1 || entries[0].SessionID != "sess-crash" {
		t.Fatalf("entries = %+v", entries)
	}
}

// --- test helpers -----------------------------------------------------------

// bytesSink is a minimal io.Writer tee capture.
type bytesSink struct{ b strings.Builder }

func (s *bytesSink) Write(p []byte) (int, error) { return s.b.Write(p) }
func (s *bytesSink) String() string              { return s.b.String() }

// stuckChild is a subagents executor that never completes a child turn
// (the manager test seam: spawn/dispatch stay pending).
type stuckChild struct{}

func (stuckChild) Run(ctx context.Context, child subagents.Child) error {
	<-ctx.Done()
	return ctx.Err()
}
