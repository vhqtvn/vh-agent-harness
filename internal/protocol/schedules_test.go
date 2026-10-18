// schedules_test.go — the schedule/* wire tests (B3): validation
// fail-closed (-32602 table), the add→list→remove round-trip (with the
// durable state file checked on disk), the engine seam-stamping, the
// restart persistence path over the wire, and the slice crux — a
// wire-registered schedule DISPATCHES through the scheduler's own loop
// (injected manual clock, the jobs package's deterministic pattern),
// settles in the session log, streams to subscribers as ordinary job/*
// events, and replays from disk.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// --- deterministic clock (the jobs package's manual-clock pattern) ---------------

// schedClock is the injected jobs.Clock for wire tests: Now is a
// test-held instant; After registers a waiter that fires when Advance
// moves now past its deadline (buffered cap 1 — Advance never blocks).
type schedClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []schedWaiter
}

type schedWaiter struct {
	deadline time.Time
	ch       chan time.Time
}

func newSchedClock(start time.Time) *schedClock {
	return &schedClock{now: start.UTC()}
}

func (c *schedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *schedClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	c.waiters = append(c.waiters, schedWaiter{deadline: c.now.Add(d), ch: ch})
	return ch
}

// Advance moves now forward (monotonic) and fires due waiters.
func (c *schedClock) Advance(to time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if to.After(c.now) {
		c.now = to.UTC()
	}
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.deadline.After(c.now) {
			w.ch <- w.deadline
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

// waiterCount is the "loop has armed its ticker" probe (no lost wakeup).
func (c *schedClock) waiterCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

// --- late-binding seams (the test twin of vh-agentd's sessionTracker) -----------

// lateSeams binds a scheduler's Dispatch/Gate to the active session's
// manager once it exists — resolved at CALL time, exactly like the
// daemon's trackerDispatcher/trackerIdleGate routing. No bound target
// fails closed (dispatch errors, gate reads idle by vacuity).
type lateSeams struct {
	mu sync.Mutex
	m  JobDispatcher
}

func (l *lateSeams) bind(m JobDispatcher) {
	l.mu.Lock()
	l.m = m
	l.mu.Unlock()
}

func (l *lateSeams) Dispatch(kind string, payload json.RawMessage) (jobs.Receipt, error) {
	l.mu.Lock()
	m := l.m
	l.mu.Unlock()
	if m == nil {
		return jobs.Receipt{}, fmt.Errorf("schedule harness: no bound session manager (schedule waits for session/create)")
	}
	return m.Dispatch(kind, payload)
}

func (l *lateSeams) InFlight() int {
	l.mu.Lock()
	m := l.m
	l.mu.Unlock()
	if m == nil {
		return 0
	}
	n := 0
	for _, st := range m.Snapshot() {
		if st.State == jobs.StateQueued || st.State == jobs.StateRunning {
			n++
		}
	}
	return n
}

// --- harness ---------------------------------------------------------------------

var schedWireBase = time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

// schedWireHarness is one server+client over net.Pipe with a FileEngine
// whose Schedules seam is a REAL jobs.Scheduler over the late-binding
// seams (bound to the active session's manager right after
// session/create) and a manual clock.
type schedWireHarness struct {
	t       *testing.T
	eng     *FileEngine
	srv     *Server
	client  *Client
	rec     *eventRecorder
	served  chan error
	sched   *jobs.Scheduler
	clk     *schedClock
	seams   *lateSeams
	logPath string
	dir     string
}

func newSchedHarness(t *testing.T, statePath string, start time.Time) *schedWireHarness {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sess-sched.jsonl")
	clk := newSchedClock(start)
	seams := &lateSeams{}
	sched, err := jobs.NewScheduler(jobs.SchedulerOptions{
		Dispatch:  seams,
		Gate:      seams,
		Clock:     clk,
		StatePath: statePath,
	})
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	eng := &FileEngine{
		Dir:       dir,
		Executor:  noopExecutor{},
		Ad:        &scriptedAdapter{},
		TurnOpts:  tools.TurnOptions{Model: "test-model"},
		Schedules: sched,
	}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)

	rec := newEventRecorder()
	client.OnNotification("session/event", func(params json.RawMessage) {
		ev := &sessionEvent{}
		if err := json.Unmarshal(params, ev); err != nil {
			t.Errorf("event params: %v", err)
			return
		}
		rec.add(ev)
	})

	h := &schedWireHarness{
		t: t, eng: eng, srv: srv, client: client, rec: rec,
		served: served, sched: sched, clk: clk, seams: seams,
		logPath: logPath, dir: dir,
	}
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-sched"}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	if err := client.Call("session/subscribe", nil, nil); err != nil {
		t.Fatalf("session/subscribe: %v", err)
	}
	// Bind the scheduler seams to the ACTIVE session's real manager
	// (the daemon's tracker routing, test-side; package-internal peek
	// under srv.mu — set before handleSessionCreate responded).
	h.srv.mu.Lock()
	es := h.srv.active
	h.srv.mu.Unlock()
	if es == nil {
		t.Fatal("no active session after session/create")
	}
	seams.bind(es.Jobs)
	return h
}

func (h *schedWireHarness) close() {
	_ = h.client.Close()
	select {
	case <-h.served:
	case <-time.After(2 * time.Second):
		h.t.Errorf("server did not exit")
	}
	h.sched.Stop() // drained: a running loop exits before Stop returns
}

// listSchedules calls schedule/list over the wire.
func (h *schedWireHarness) listSchedules() []scheduleDTO {
	h.t.Helper()
	var res struct {
		Schedules []scheduleDTO `json:"schedules"`
	}
	if err := h.client.Call("schedule/list", nil, &res); err != nil {
		h.t.Fatalf("schedule/list: %v", err)
	}
	return res.Schedules
}

// addSchedule calls schedule/add over the wire, failing the test on error.
func (h *schedWireHarness) addSchedule(params map[string]any) scheduleDTO {
	h.t.Helper()
	var added scheduleDTO
	if err := h.client.Call("schedule/add", params, &added); err != nil {
		h.t.Fatalf("schedule/add %+v: %v", params, err)
	}
	return added
}

// --- round-trip ------------------------------------------------------------------

// TestScheduleAddListRemoveRoundTripOverWire: add (relative after +
// recurring every), canonicalization echoed (at UTC, nextRun), list in
// dispatch-priority order, remove persists to the state file, unknown
// remove and duplicate add are clean -32602s.
func TestScheduleAddListRemoveRoundTripOverWire(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "sched-state.json")
	h := newSchedHarness(t, statePath, schedWireBase)
	defer h.close()

	// One-shot via a relative delay: canonicalized to an absolute UTC at.
	once := h.addSchedule(map[string]any{
		"name": "once", "after": int64(5 * time.Minute), "payload": map[string]any{"n": 1},
	})
	if once.At == nil || !once.At.Equal(schedWireBase.Add(5*time.Minute)) {
		t.Fatalf("once canonical at = %+v, want %v", once.At, schedWireBase.Add(5*time.Minute))
	}
	if !once.NextRun.Equal(schedWireBase.Add(5 * time.Minute)) {
		t.Fatalf("once nextRun = %v, want %v", once.NextRun, schedWireBase.Add(5*time.Minute))
	}
	if once.Kind != "" {
		t.Fatalf("once stored kind = %q, want empty (derived at dispatch as sched-<name>)", once.Kind)
	}

	// Recurring with an explicit kind: first due at record time.
	tick := h.addSchedule(map[string]any{
		"name": "tick", "kind": "digest", "every": int64(time.Minute),
	})
	if !tick.NextRun.Equal(schedWireBase) {
		t.Fatalf("tick nextRun = %v, want record time %v", tick.NextRun, schedWireBase)
	}
	if tick.Every != int64(time.Minute) {
		t.Fatalf("tick every = %d, want %d", tick.Every, int64(time.Minute))
	}

	// List: dispatch-priority order (nextRun, then name).
	got := h.listSchedules()
	if len(got) != 2 || got[0].Name != "tick" || got[1].Name != "once" {
		t.Fatalf("list = %+v, want [tick once] in priority order", got)
	}
	if got[1].Kind != "" || got[0].Kind != "digest" {
		t.Fatalf("list kinds = %q/%q, want \"\"(derived)/digest", got[0].Kind, got[1].Kind)
	}

	// Remove: acknowledged, list drops it, the STATE FILE drops it.
	var rm struct {
		Removed bool `json:"removed"`
	}
	if err := h.client.Call("schedule/remove", map[string]any{"name": "tick"}, &rm); err != nil {
		t.Fatalf("schedule/remove: %v", err)
	}
	if !rm.Removed {
		t.Fatalf("remove result = %+v, want removed:true", rm)
	}
	if left := h.listSchedules(); len(left) != 1 || left[0].Name != "once" {
		t.Fatalf("list after remove = %+v, want only once", left)
	}
	assertStateFile(t, statePath, []string{"once"})

	// Removing an unknown name: clean -32602 carrying the typed text.
	err := h.client.Call("schedule/remove", map[string]any{"name": "tick"}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("remove unknown error = %v, want -32602", err)
	}
	if !strings.Contains(perr.Message, "schedule not found") {
		t.Fatalf("remove unknown text = %q, want the typed ErrScheduleNotFound text", perr.Message)
	}

	// Duplicate add: -32602 (the name is the schedule's identity).
	err = h.client.Call("schedule/add", map[string]any{"name": "once", "every": int64(time.Minute)}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("duplicate add error = %v, want -32602", err)
	}

	// Remove+re-add is the v1 pause path.
	h.addSchedule(map[string]any{"name": "tick", "every": int64(time.Minute)})
	if left := h.listSchedules(); len(left) != 2 {
		t.Fatalf("list after re-add = %+v, want 2", left)
	}
}

// assertStateFile decodes the persisted scheduler state (the v1 JSON
// shape, read from disk) and checks the surviving schedule names.
func assertStateFile(t *testing.T, path string, wantNames []string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scheduler state %s: %v", path, err)
	}
	var sf struct {
		Version int `json:"version"`
		Specs   []struct {
			Spec struct {
				Name string `json:"name"`
			} `json:"spec"`
		} `json:"specs"`
	}
	if err := json.Unmarshal(raw, &sf); err != nil {
		t.Fatalf("parse scheduler state: %v\n%s", err, raw)
	}
	if sf.Version != 1 {
		t.Fatalf("state version = %d, want 1", sf.Version)
	}
	if len(sf.Specs) != len(wantNames) {
		t.Fatalf("state file carries %d specs (%+v), want %d (%v)", len(sf.Specs), sf.Specs, len(wantNames), wantNames)
	}
	for i, w := range wantNames {
		if sf.Specs[i].Spec.Name != w {
			t.Fatalf("state spec[%d] = %q, want %q", i, sf.Specs[i].Spec.Name, w)
		}
	}
}

// --- validation -------------------------------------------------------------------

// TestScheduleAddValidationFailClosed: the -32602 table — slug grammar,
// missing cadence, both starts, negative every, non-RFC3339 at, the
// zero at, unknown params fields.
func TestScheduleAddValidationFailClosed(t *testing.T) {
	h := newSchedHarness(t, filepath.Join(t.TempDir(), "s.json"), schedWireBase)
	defer h.close()

	at := schedWireBase.Add(time.Hour).Format(time.RFC3339Nano)
	for _, params := range []map[string]any{
		{"name": "Bad_Name", "every": int64(time.Minute)},         // slug violation
		{"every": int64(time.Minute)},                             // no name
		{"name": "x"},                                             // no cadence
		{"name": "x", "after": int64(time.Second), "at": at},      // both starts
		{"name": "x", "every": -1},                                // negative every
		{"name": "x", "at": "not-a-time"},                         // non-RFC3339 at
		{"name": "x", "at": "0001-01-01T00:00:00Z"},               // zero at
		{"name": "x", "every": int64(time.Minute), "bogus": true}, // unknown field
		{"name": "x", "every": 1.5},                               // fractional ns
	} {
		err := h.client.Call("schedule/add", params, nil)
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
			t.Fatalf("schedule/add %+v error = %v, want -32602", params, err)
		}
	}
	// Nothing registered: every refusal was fail-closed.
	if got := h.listSchedules(); len(got) != 0 {
		t.Fatalf("list after validation refusals = %+v, want empty", got)
	}
}

// --- error-code mapping (F2: caller faults vs engine faults) ----------------------

// errSchedules is a ScheduleManager double failing Add/Remove with a
// fixed error — the wiring-level stand-in for a scheduler whose persist
// layer is failing (the real persistFn injection seam is jobs-internal;
// the WIRE MAPPING is what this double exercises).
type errSchedules struct{ err error }

func (e errSchedules) Add(jobs.ScheduleSpec) (jobs.ScheduleRecord, error) {
	return jobs.ScheduleRecord{}, e.err
}
func (e errSchedules) Remove(string) error             { return e.err }
func (e errSchedules) Snapshot() []jobs.ScheduleRecord { return nil }

// newSchedErrClient builds an initialized wire (initialize +
// session/create) whose engine carries the GIVEN schedule seam.
func newSchedErrClient(t *testing.T, seam ScheduleManager) *Client {
	t.Helper()
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}, Schedules: seam}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	go func() { _ = srv.Serve(nil) }()
	client := NewClient(cli)
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{
		"path": filepath.Join(dir, "sess-err.jsonl"), "sessionId": "sess-err",
	}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	return client
}

// TestScheduleWireErrorCodeMapping (reviewer F2): the -32602/-32000
// split on schedule/add and schedule/remove — validation refusals and
// unknown names are CALLER faults (-32602, the §4c table); persist /
// infrastructure failures are ENGINE faults (-32000) with the underlying
// message preserved. The defect it guards: the handlers used to map
// EVERY seam error to -32602, so a persist failure masqueraded as bad
// params.
func TestScheduleWireErrorCodeMapping(t *testing.T) {
	underlying := errors.New("scheduler: create temp state /sess/scheduler-state.json.tmp: permission denied")
	cases := []struct {
		name     string
		method   string
		params   map[string]any
		seamErr  error
		wantCode int
		wantText string
	}{
		{
			name:     "remove persist failure is engine error with message preserved",
			method:   "schedule/remove",
			params:   map[string]any{"name": "tick"},
			seamErr:  fmt.Errorf("%w after remove: %w", jobs.ErrSchedulePersist, underlying),
			wantCode: ErrEngine,
			wantText: "permission denied",
		},
		{
			name:     "add persist failure is engine error with message preserved",
			method:   "schedule/add",
			params:   map[string]any{"name": "tick", "every": int64(time.Minute)},
			seamErr:  fmt.Errorf("%w after add: %w", jobs.ErrSchedulePersist, underlying),
			wantCode: ErrEngine,
			wantText: "permission denied",
		},
		{
			name:     "unknown-name remove stays invalid params",
			method:   "schedule/remove",
			params:   map[string]any{"name": "ghost"},
			seamErr:  fmt.Errorf("%w: %q", jobs.ErrScheduleNotFound, "ghost"),
			wantCode: ErrInvalidParams,
			wantText: "schedule not found",
		},
		{
			name:     "jobs-side validation refusal stays invalid params",
			method:   "schedule/add",
			params:   map[string]any{"name": "x", "every": int64(-time.Minute)},
			seamErr:  errors.New(`scheduler: every for "x" must be positive, got -1m0s`),
			wantCode: ErrInvalidParams,
			wantText: "must be positive",
		},
		{
			name:     "empty name is invalid params before the seam",
			method:   "schedule/add",
			params:   map[string]any{"every": int64(time.Minute)},
			seamErr:  errors.New("seam must not be reached with an empty name"),
			wantCode: ErrInvalidParams,
			wantText: "name is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newSchedErrClient(t, errSchedules{err: tc.seamErr})
			err := client.Call(tc.method, tc.params, nil)
			var perr *Error
			if !errors.As(err, &perr) {
				t.Fatalf("%s error = %v, want a JSON-RPC error", tc.method, err)
			}
			if perr.Code != tc.wantCode {
				t.Fatalf("%s code = %d (%q), want %d — caller faults and engine faults must not share a code", tc.method, perr.Code, perr.Message, tc.wantCode)
			}
			if !strings.Contains(perr.Message, tc.wantText) {
				t.Fatalf("%s message = %q, want it to carry %q (the underlying text travels)", tc.method, perr.Message, tc.wantText)
			}
		})
	}
}

// --- session/seam gating -----------------------------------------------------------

// TestScheduleWithoutSessionAndWithoutSeam: add/remove without a session
// are -32003; without scheduler wiring they fail closed -32000 while
// list stays an honest empty (the jobs/status mirror).
func TestScheduleWithoutSessionAndWithoutSeam(t *testing.T) {
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	go func() { _ = srv.Serve(nil) }()
	client := NewClient(cli)
	defer func() { _ = client.Close() }()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	// No session: add/remove -32003, list honest-empty.
	err := client.Call("schedule/add", map[string]any{"name": "x", "every": int64(time.Minute)}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrNoSession {
		t.Fatalf("add without session error = %v, want -32003", err)
	}
	err = client.Call("schedule/remove", map[string]any{"name": "x"}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrNoSession {
		t.Fatalf("remove without session error = %v, want -32003", err)
	}
	var res struct {
		Schedules []scheduleDTO `json:"schedules"`
	}
	if err := client.Call("schedule/list", nil, &res); err != nil {
		t.Fatalf("list without session: %v", err)
	}
	if res.Schedules == nil || len(res.Schedules) != 0 {
		t.Fatalf("list without session = %+v, want []", res.Schedules)
	}

	// Session on an engine WITHOUT the seam: add/remove fail closed
	// -32000; list still honest-empty.
	if err := client.Call("session/create", map[string]any{
		"path": filepath.Join(dir, "sess-bare.jsonl"), "sessionId": "sess-bare",
	}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	err = client.Call("schedule/add", map[string]any{"name": "x", "every": int64(time.Minute)}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrEngine {
		t.Fatalf("add without seam error = %v, want -32000", err)
	}
	err = client.Call("schedule/remove", map[string]any{"name": "x"}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrEngine {
		t.Fatalf("remove without seam error = %v, want -32000", err)
	}
	if err := client.Call("schedule/list", nil, &res); err != nil {
		t.Fatalf("list without seam: %v", err)
	}
	if len(res.Schedules) != 0 {
		t.Fatalf("list without seam = %+v, want []", res.Schedules)
	}
}

// --- engine wiring -----------------------------------------------------------------

// staticSchedules is an identity-probe seam for the stamping test.
type staticSchedules struct{}

func (staticSchedules) Add(jobs.ScheduleSpec) (jobs.ScheduleRecord, error) {
	return jobs.ScheduleRecord{}, nil
}
func (staticSchedules) Remove(string) error { return nil }
func (staticSchedules) Snapshot() []jobs.ScheduleRecord {
	return nil
}

// TestFileEngineStampsScheduleSeam: FileEngine.Schedules lands on every
// EngineSession as its Schedules seam (owner-built pass-through — the
// engine neither constructs nor owns the scheduler); nil stays nil.
func TestFileEngineStampsScheduleSeam(t *testing.T) {
	dir := t.TempDir()
	var want ScheduleManager = staticSchedules{}
	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}, Schedules: want}
	es, err := eng.NewSession(filepath.Join(dir, "s1.jsonl"), "sess-stamp", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if es.Schedules != want {
		t.Fatalf("session seam = %#v, want the engine-level scheduler stamp", es.Schedules)
	}

	bare := &FileEngine{Dir: dir, Executor: noopExecutor{}}
	es2, err := bare.NewSession(filepath.Join(dir, "s2.jsonl"), "sess-bare2", io.Discard)
	if err != nil {
		t.Fatalf("NewSession bare: %v", err)
	}
	if es2.Schedules != nil {
		t.Fatalf("bare engine seam = %#v, want nil", es2.Schedules)
	}
}

// --- restart persistence -------------------------------------------------------------

// TestScheduleStateSurvivesRestartOverWire: a schedule registered over
// the wire survives a full harness restart (new engine, new scheduler,
// same state file) and list re-reports it with the PERSISTED cursor;
// the adopted state is live (a duplicate add still refuses).
func TestScheduleStateSurvivesRestartOverWire(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "sched-state.json")
	h1 := newSchedHarness(t, statePath, schedWireBase)
	due := schedWireBase.Add(time.Hour)
	h1.addSchedule(map[string]any{
		"name": "weekly", "at": due.Format(time.RFC3339Nano), "every": int64(7 * 24 * time.Hour),
	})
	h1.close() // client disconnect + scheduler drained

	// RESTART: fresh engine + scheduler over the same state file, clock
	// 30 minutes on (before the persisted cursor — nothing due).
	h2 := newSchedHarness(t, statePath, schedWireBase.Add(30*time.Minute))
	defer h2.close()
	got := h2.listSchedules()
	if len(got) != 1 {
		t.Fatalf("post-restart list = %+v, want the persisted weekly", got)
	}
	w := got[0]
	if w.Name != "weekly" || w.Kind != "" {
		t.Fatalf("post-restart record = %+v (kind stays empty: derived at dispatch)", w)
	}
	if !w.NextRun.Equal(due) {
		t.Fatalf("post-restart nextRun = %v, want the persisted cursor %v", w.NextRun, due)
	}
	if w.Every != int64(7*24*time.Hour) {
		t.Fatalf("post-restart every = %d, want %d", w.Every, int64(7*24*time.Hour))
	}

	// The adopted state is LIVE in the new scheduler: duplicate add refuses.
	err := h2.client.Call("schedule/add", map[string]any{
		"name": "weekly", "every": int64(24 * time.Hour),
	}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("duplicate add after restart = %v, want -32602", err)
	}

	// And remove works against the adopted state (persisted).
	var rm struct {
		Removed bool `json:"removed"`
	}
	if err := h2.client.Call("schedule/remove", map[string]any{"name": "weekly"}, &rm); err != nil || !rm.Removed {
		t.Fatalf("remove after restart = (%v, %+v)", err, rm)
	}
	assertStateFile(t, statePath, nil)
}

// --- the slice crux --------------------------------------------------------------------

// TestScheduleCruxAddDispatchSettleOverWire is the B3 crux: a schedule
// registered OVER THE WIRE dispatches through the scheduler's OWN loop
// (manual clock advanced past the cursor), lands job/enqueued +
// job/settled in the ACTIVE session's durable log, streams to the
// subscriber as ordinary job/* events, reflects in jobs/status and
// schedule/list (cursor advanced), and replays from disk.
func TestScheduleCruxAddDispatchSettleOverWire(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "sched-state.json")
	h := newSchedHarness(t, statePath, schedWireBase)
	defer h.close()

	// Register a recurring schedule over the wire: due at the clock's
	// now, every 1m, carrying a payload.
	added := h.addSchedule(map[string]any{
		"name": "tick", "at": schedWireBase.Format(time.RFC3339Nano),
		"every": int64(time.Minute), "payload": map[string]any{"job": "digest"},
	})
	if !added.NextRun.Equal(schedWireBase) {
		t.Fatalf("nextRun = %v, want %v", added.NextRun, schedWireBase)
	}

	// The scheduler's own loop drives dispatch (engine-side cadence).
	if err := h.sched.Start(); err != nil {
		t.Fatalf("scheduler Start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for h.clk.waiterCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if h.clk.waiterCount() < 1 {
		t.Fatal("scheduler loop never armed its ticker")
	}
	h.clk.Advance(schedWireBase.Add(time.Minute))

	// Ordinary job/* events stream over the wire.
	enq := h.rec.waitType(t, "job/enqueued")
	var ep session.JobPayload
	if err := json.Unmarshal(enq.Payload, &ep); err != nil {
		t.Fatalf("enqueued payload: %v", err)
	}
	if ep.Kind != "sched-tick" || ep.JobID != "sched-tick-1" {
		t.Fatalf("enqueued = %+v, want kind sched-tick job sched-tick-1", ep)
	}
	set := h.rec.waitType(t, "job/settled")
	var sp session.JobPayload
	if err := json.Unmarshal(set.Payload, &sp); err != nil {
		t.Fatalf("settled payload: %v", err)
	}
	if sp.JobID != "sched-tick-1" || sp.Result != session.JobResultCompleted {
		t.Fatalf("settled = %+v, want sched-tick-1 completed", sp)
	}

	// jobs/status reflects the fold; schedule/list shows the ADVANCED
	// cursor (collapse: one dispatch covers the gap, next = +2m).
	var status struct {
		Jobs []jobs.Status `json:"jobs"`
	}
	if err := h.client.Call("jobs/status", nil, &status); err != nil {
		t.Fatalf("jobs/status: %v", err)
	}
	if len(status.Jobs) != 1 || status.Jobs[0].JobID != "sched-tick-1" ||
		status.Jobs[0].State != "settled" || status.Jobs[0].Result != "completed" {
		t.Fatalf("jobs/status = %+v, want sched-tick-1 settled completed", status.Jobs)
	}
	got := h.listSchedules()
	if len(got) != 1 || !got[0].NextRun.Equal(schedWireBase.Add(2*time.Minute)) {
		t.Fatalf("post-dispatch list = %+v, want nextRun %v", got, schedWireBase.Add(2*time.Minute))
	}

	// The durable log replays from disk with the scheduled job settled.
	replayed, err := session.ReplayFile(h.logPath)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	var seenEnq, seenSet bool
	for _, ev := range replayed {
		if ev.Type != session.TypeJobEnqueued && ev.Type != session.TypeJobSettled {
			continue
		}
		var p session.JobPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if p.JobID != "sched-tick-1" {
			t.Fatalf("unexpected job %q in the replayed log", p.JobID)
		}
		if ev.Type == session.TypeJobEnqueued && p.Kind == "sched-tick" {
			seenEnq = true
		}
		if ev.Type == session.TypeJobSettled && p.Result == session.JobResultCompleted {
			seenSet = true
		}
	}
	if !seenEnq || !seenSet {
		t.Fatalf("replayed log missing the scheduled job (enq=%v set=%v)", seenEnq, seenSet)
	}
}
