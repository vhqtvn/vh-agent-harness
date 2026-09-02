// engine_lifecycle_test.go — hotfix round 2 regressions at the ENGINE
// seam: the engine-direct activeID race (R2), the surface-before-
// supersede ordering (R3), and the empty-log typed refusal (R5).
//
// R2 receipt honesty: the reserve/commit split the race lives in is
// INTERNAL to ResumeSession, so no wrapper decorator can widen it (the
// concurrency-gate stage-widening pattern widens splits BETWEEN call
// boundaries). The deterministic red instead parks the resume
// mid-transition through a REAL engine seam — SpillPolicyFor, consulted
// after file recovery — the same widening IDEA (a test-controlled hook
// inside the critical window) expressed through existing configuration
// rather than a new decorator. A goroutine-shuffle storm alone would be
// a probabilistic red only; it is included as the -race exercise, with
// the deterministic assertions carried by the parked-resume test.
package protocol

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// TestResumeCreateInterleaveRecordsActiveID is the R2 regression
// (deterministic red via the SpillPolicyFor mid-transition seam). The
// indicted interleave: ResumeSession reserves the active id, releases
// the guard during file recovery, a concurrent ENGINE-DIRECT NewSession
// completes and publishes its own id over the reservation, and the
// resume then returns committed with its own id unrecorded — so a LATER
// same-id resume sails past the D-F1 session-active refusal and opens a
// SECOND live log on the same durable file (duplicate seqs, permanently
// unreplayable). Post-fix the whole transition holds the engine
// lifecycle lock, so the create cannot even START until the resume
// finishes: the record always names the last completed transition.
func TestResumeCreateInterleaveRecordsActiveID(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	seedSession(t, e, "sess-a", "durable history")
	// Displace sess-a (same-id resume of the ACTIVE session is the
	// D-F1 refusal) so it is resumable.
	if _, err := e.NewSession("", "sess-dash", io.Discard); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var hook sync.Once
	e.SpillPolicyFor = func(id string) *session.SpillPolicy {
		if id == "sess-a" {
			hook.Do(func() {
				entered <- struct{}{}
				<-release
			})
		}
		return nil
	}

	resumeDone := make(chan error, 1)
	go func() {
		_, _, err := e.ResumeSession("sess-a", io.Discard)
		resumeDone <- err
	}()
	<-entered // the resume is mid-transition: file recovered, id held

	createDone := make(chan error, 1)
	go func() {
		_, err := e.NewSession("", "sess-c", io.Discard)
		createDone <- err
	}()

	// Classification of the interleave. Post-fix this branch is
	// DETERMINISTIC, not timing-based: the create parks on the engine
	// lifecycle lock and cannot complete while the resume is
	// mid-transition, so the timeout always fires (200ms is scheduler
	// slack, never a green-path dependency — the lock guarantees the
	// park). Pre-fix the create completes during the park.
	var createErr error
	createFinishedDuringPark := false
	select {
	case createErr = <-createDone:
		createFinishedDuringPark = true
	case <-time.After(200 * time.Millisecond):
		createFinishedDuringPark = false
	}
	close(release)
	if err := <-resumeDone; err != nil {
		t.Fatalf("resume must succeed: %v", err)
	}
	if !createFinishedDuringPark {
		createErr = <-createDone // first (and only) consumption
	}
	if createErr != nil {
		t.Fatalf("create must succeed: %v", createErr)
	}

	if createFinishedDuringPark {
		// PRE-FIX interleave (the indictment): the create published
		// over the reservation while the resume was parked, and the
		// RESUME completed last — it must own the record.
		if e.activeID != "sess-a" {
			t.Fatalf("engine-direct race: the resume of sess-a completed last but activeID = %q — a committed resume returned with its own id unrecorded", e.activeID)
		}
		_, _, err := e.ResumeSession("sess-a", io.Discard)
		var sre *SessionResumeError
		if !errors.As(err, &sre) || sre.Kind != ResumeKindSessionActive {
			t.Fatalf("a later same-id resume must hit the session-active refusal (the unrecorded id otherwise lets a SECOND live writer open on sess-a's durable file), got %v", err)
		}
	} else {
		// SERIALIZED (the fix): the resume ran to completion first;
		// the create is the last transition and owns the record.
		if e.activeID != "sess-c" {
			t.Fatalf("activeID = %q, want sess-c (the id of the last completed transition)", e.activeID)
		}
		_, _, err := e.ResumeSession("sess-c", io.Discard)
		var sre *SessionResumeError
		if !errors.As(err, &sre) || sre.Kind != ResumeKindSessionActive {
			t.Fatalf("same-id resume of the recorded active session must be the typed session-active refusal, got %v", err)
		}
	}
}

// TestInterleavedEngineTransitionsRaceClean is the R2 -race exercise:
// genuine goroutine contention over interleaved engine-direct
// NewSession/ResumeSession transitions (each success appends one event
// through its returned log), then a deterministic serialization tail.
// The storm itself is a probabilistic red pre-fix (the deterministic
// red lives in the parked-resume test above); post-fix every assertion
// holds by construction: transitions serialize under the lifecycle
// lock, each pool id is resumed at most once (the superseded-but-open
// double-writer hazard is v1 engine semantics, deliberately not
// exercised here), and every durable log replays clean.
func TestInterleavedEngineTransitionsRaceClean(t *testing.T) {
	e, dir, _ := newResumeEngine(t)
	const pool = 8
	for i := 0; i < pool; i++ {
		seedSession(t, e, fmt.Sprintf("sess-p%d", i), "storm pool history")
	}

	const goroutines = 6
	const perGoroutine = 8
	var resumes atomic.Int32
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if n := resumes.Add(1) - 1; j%2 == 0 && n < pool {
					es, _, err := e.ResumeSession(fmt.Sprintf("sess-p%d", n), io.Discard)
					if err == nil {
						if _, err := es.Log.AppendPrompt("post-resume append"); err != nil {
							t.Errorf("append after resume: %v", err)
						}
					}
				} else {
					es, err := e.NewSession("", fmt.Sprintf("sess-c%d-%d", g, j), io.Discard)
					if err == nil {
						if _, err := es.Log.AppendPrompt("post-create append"); err != nil {
							t.Errorf("append after create: %v", err)
						}
					}
				}
				runtime.Gosched() // widen genuine contention
			}
		}(g)
	}
	wg.Wait()

	// Deterministic tail: one final transition owns the record, and a
	// same-id resume of it must refuse.
	if _, err := e.NewSession("", "sess-final", io.Discard); err != nil {
		t.Fatal(err)
	}
	if e.activeID != "sess-final" {
		t.Fatalf("activeID = %q after the storm + final create, want sess-final", e.activeID)
	}
	_, _, err := e.ResumeSession("sess-final", io.Discard)
	var sre *SessionResumeError
	if !errors.As(err, &sre) || sre.Kind != ResumeKindSessionActive {
		t.Fatalf("same-id resume of the active session must refuse, got %v", err)
	}

	// Every durable log replays clean — no interleaving minted a
	// duplicate seq or a partial record.
	logs, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("the storm produced no logs")
	}
	for _, p := range logs {
		if _, rerr := session.ReplayFile(p); rerr != nil {
			t.Fatalf("log %s does not replay after the storm: %v", p, rerr)
		}
	}
}

// TestResumeSurfaceInvalidRefusalKeepsSubagentState is the R3 ordering
// regression: a structurally valid but SURFACE-invalid log (a
// message-bearing event without a surfaceOp — RecoverTail accepts it,
// FoldSurface refuses it) must refuse resume BEFORE the subagent
// supersede block mutates any engine state. Pre-fix the supersede ran
// first (the active session's manager unbound and stopped, the resumed
// id bound), then the surface derivation refused — a refusal that had
// already changed state, contradicting "fail-closed before any state
// changes".
func TestResumeSurfaceInvalidRefusalKeepsSubagentState(t *testing.T) {
	reg := subagents.NewRegistry()
	e, _, _ := newResumeEngine(t)
	e.SubagentExecutor = idleSubagentExecutor{}
	e.SubagentRegistry = reg

	esA, err := e.NewSession("", "sess-a", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	mA, ok := reg.Get("sess-a")
	if !ok || mA != esA.Subagents {
		t.Fatal("create must bind sess-a's manager in the registry")
	}

	// sess-bad: valid header, then a hand-written session/prompt with
	// NO surfaceOp — structurally valid (RecoverTail passes), surface
	// invalid (FoldSurface refuses).
	badPath := filepath.Join(e.Dir, "sess-bad.jsonl")
	lg, err := session.OpenFile(badPath, "sess-bad")
	if err != nil {
		t.Fatal(err)
	}
	if err := lg.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(badPath, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":2,"type":"session/prompt","payload":{"text":"surfaceless"}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	_, _, rerr := e.ResumeSession("sess-bad", io.Discard)
	if rerr == nil {
		t.Fatal("a surface-invalid log must refuse resume")
	}

	// Registry/manager invariance on the refusal: sess-a's manager is
	// STILL bound (not superseded away) and still accepts spawns (not
	// stopped); the refused id never got a binding.
	got, ok := reg.Get("sess-a")
	if !ok || got != mA {
		t.Fatalf("the surface-invalid refusal must leave sess-a's manager bound (got bound=%v, same-manager=%v)", ok, got == mA)
	}
	if _, ok := reg.Get("sess-bad"); ok {
		t.Fatal("the refused resume must not bind sess-bad in the registry")
	}
	if _, serr := mA.SpawnWithOptions(subagents.SpawnOptions{
		Kind: session.SubagentKindOneShot, Prompt: "still accepting spawns",
	}); serr != nil {
		t.Fatalf("the active session's manager must NOT be stopped by the refused resume: %v", serr)
	}

	// The engine's active record is unchanged too: sess-a is still the
	// active session (its same-id resume refuses), and the file was
	// left byte-untouched by the refusal.
	_, _, aerr := e.ResumeSession("sess-a", io.Discard)
	var sre *SessionResumeError
	if !errors.As(aerr, &sre) || sre.Kind != ResumeKindSessionActive {
		t.Fatalf("sess-a must still be the engine's active session after the refusal, got %v", aerr)
	}
}

// TestResumeEmptyLogTypedRefusal is the R5 polish regression: an EMPTY
// session log (file exists, zero bytes — no header) is not a session;
// resuming it must be the typed not-found refusal family, not an
// untyped -32000 engine error.
func TestResumeEmptyLogTypedRefusal(t *testing.T) {
	e, _, _ := newResumeEngine(t)
	emptyPath := filepath.Join(e.Dir, "sess-empty.jsonl")
	if err := os.WriteFile(emptyPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := e.ResumeSession("sess-empty", io.Discard)
	var sre *SessionResumeError
	if !errors.As(err, &sre) || sre.Kind != ResumeKindNotFound {
		t.Fatalf("empty-log resume must be the typed not-found refusal, got %v", err)
	}
	// The file is untouched: still present, still empty (resume never
	// creates, never truncates, never removes).
	fi, serr := os.Stat(emptyPath)
	if serr != nil {
		t.Fatal(serr)
	}
	if fi.Size() != 0 {
		t.Fatalf("empty log must stay untouched by the refusal (size = %d)", fi.Size())
	}
}
