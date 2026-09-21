// engine_id_confinement_test.go — TB-F1 round-2 regression tests: the
// client-supplied session/create sessionId composes the DEFAULT session
// path (Dir + <id>.jsonl) and must be validated as a strict single
// filename component BEFORE any filesystem effect. filepath.Join lexically
// cleans, so "../../victim" escapes the session root — the exact hole the
// round-1 fix left in the default-path branch. Also covers: the
// error-after-create cleanup contract (F2/F3 — no abandoned partial file,
// no leaked fd) and the fail-closed session-id mint (F6).
package protocol

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewSessionHostileSessionID drives the ENGINE seam (default-path
// branch — path omitted): every hostile id shape is rejected with the
// typed *SessionPathError and creates NOTHING anywhere near or outside
// the root; valid ids keep working (wire shape unchanged).
func TestNewSessionHostileSessionID(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "sessions")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}

	rejects := []struct {
		name string
		id   string
		// mustNotExist is asserted absent after the rejected create
		// (the canonical escape landing spot for that shape).
		mustNotExist string
	}{
		{"dot-dot climb", "../../victim", filepath.Join(parent, "victim.jsonl")},
		{"single dot-dot", "..", ""},
		{"bare dot", ".", ""},
		{"deep climb", "../sessions/../../evil", filepath.Join(parent, "evil.jsonl")},
		{"separator-bearing", "sub/victim", filepath.Join(dir, "sub")}, // no dir created either
		{"absolute id", filepath.Join(parent, "abs-victim.jsonl"), filepath.Join(parent, "abs-victim.jsonl")},
		{"leading dot", ".hidden", filepath.Join(dir, ".hidden.jsonl")},
		{"space-bearing", "ses sion", ""},
		{"newline-bearing", "ses\nsion", ""},
	}
	for _, tc := range rejects {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
			es, err := eng.NewSession("", tc.id, io.Discard)
			var spe *SessionPathError
			if err == nil {
				if es != nil {
					_ = es.Log.Close()
				}
				t.Fatalf("NewSession(id=%q) succeeded (path=%s): want typed rejection", tc.id, es.Path)
			}
			if !errors.As(err, &spe) {
				t.Fatalf("NewSession(id=%q) error = %v (%T), want *SessionPathError", tc.id, err, err)
			}
			if tc.mustNotExist != "" {
				if _, statErr := os.Stat(tc.mustNotExist); statErr == nil {
					t.Fatalf("rejected id left a filesystem trace at %s", tc.mustNotExist)
				}
			}
		})
	}

	// Nothing was created inside the root by any rejected create.
	ents, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("readdir %s: %v", dir, readErr)
	}
	if len(ents) != 0 {
		t.Fatalf("rejected creates left entries in the session root: %v", ents)
	}

	valids := []string{"sess-ok", "s1", "A-b_2.x", "sess-0123456789abcdef", "root-1.1"}
	for _, id := range valids {
		t.Run("valid/"+id, func(t *testing.T) {
			eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
			es, err := eng.NewSession("", id, io.Discard)
			if err != nil {
				t.Fatalf("NewSession(id=%q): %v", id, err)
			}
			defer es.Log.Close()
			want := filepath.Join(dir, id+".jsonl")
			if es.Path != want {
				t.Fatalf("es.Path = %q, want %q", es.Path, want)
			}
			if fi, serr := os.Stat(want); serr != nil || fi.Size() == 0 {
				t.Fatalf("session file at %s not created non-empty: %v", want, serr)
			}
			if rmErr := os.Remove(want); rmErr != nil {
				t.Fatalf("cleanup remove %s: %v", want, rmErr)
			}
		})
	}
}

// TestNewSessionHostileSessionIDWithExplicitPath: the id is validated on
// EVERY branch (it seeds the log header and, later, the subagents
// FileStore parent directory), so a hostile id is rejected even when the
// explicit path itself is confined and valid.
func TestNewSessionHostileSessionIDWithExplicitPath(t *testing.T) {
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
	es, err := eng.NewSession(filepath.Join(dir, "fine.jsonl"), "../../victim", io.Discard)
	if err == nil {
		if es != nil {
			_ = es.Log.Close()
		}
		t.Fatal("NewSession with hostile id + valid explicit path succeeded: want typed rejection")
	}
	var spe *SessionPathError
	if !errors.As(err, &spe) {
		t.Fatalf("error = %v (%T), want *SessionPathError", err, err)
	}
	if _, serr := os.Stat(filepath.Join(dir, "fine.jsonl")); serr == nil {
		t.Fatal("rejected create touched the explicit path")
	}
}

// TestSessionCreateWireHostileSessionID drives the WIRE: session/create
// with a traversal sessionId and NO path yields a clean -32602, leaves
// the filesystem untouched OUTSIDE the engine root, and no partial
// server state; a subsequent valid create still succeeds.
func TestSessionCreateWireHostileSessionID(t *testing.T) {
	client, eng, stop := newConfinedPair(t)
	defer stop()
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	hostile := []string{
		"../../wire-victim",       // climb out of the session root
		"..",                      // relative primitive
		".",                       // relative primitive
		"sub/victim",              // separator-bearing (nested path)
		"/etc/cron.d/evil",        // absolute
		strings.Repeat("../", 40), // deep climb
	}
	var perr *Error
	for _, id := range hostile {
		err := client.Call("session/create", map[string]any{"sessionId": id}, nil)
		if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
			t.Fatalf("session/create(sessionId=%q) error = %v, want *Error{Code: ErrInvalidParams}", id, err)
		}
	}

	// Filesystem untouched outside the root: the engine root's PARENT
	// must contain nothing beyond the root itself.
	eng.mu.Lock()
	root := eng.Dir
	eng.mu.Unlock()
	parentEnts, rerr := os.ReadDir(filepath.Dir(root))
	if rerr != nil {
		t.Fatalf("readdir parent of root: %v", rerr)
	}
	for _, e := range parentEnts {
		if e.Name() != filepath.Base(root) {
			t.Fatalf("hostile creates left an entry outside the session root: %s", e.Name())
		}
	}

	// No partial state: nothing became the active session.
	if err := client.Call("session/prompt", map[string]any{"text": "x"}, nil); !errors.As(err, &perr) || perr.Code != ErrNoSession {
		t.Fatalf("session/prompt after refused creates error = %v, want ErrNoSession", err)
	}

	// A valid id-only create still works (default path branch).
	var res createResult
	if err := client.Call("session/create", map[string]any{"sessionId": "sess-wire-ok"}, &res); err != nil {
		t.Fatalf("session/create(valid id): %v", err)
	}
	want := filepath.Join(root, "sess-wire-ok.jsonl")
	if res.Path != want {
		t.Fatalf("create result path = %q, want %q", res.Path, want)
	}
	if fi, serr := os.Stat(want); serr != nil || fi.Size() == 0 {
		t.Fatalf("valid session file at %s missing/empty: %v", want, serr)
	}
}

// TestNewSessionLeavesNoPartialFileOnLateFailure (F2/F3): a failure
// AFTER os.Create succeeded (jobs.NewManager rejects the nil executor)
// must close the fd and REMOVE the partial file — no abandoned
// truncated session, no fd leak. The same discipline covers the
// subagent-manager failure path (single abandon helper).
func TestNewSessionLeavesNoPartialFileOnLateFailure(t *testing.T) {
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: nil} // jobs.NewManager fails AFTER the file exists
	es, err := eng.NewSession("", "sess-partial", io.Discard)
	if err == nil {
		if es != nil {
			_ = es.Log.Close()
		}
		t.Fatal("NewSession with nil executor succeeded: want jobs manager failure")
	}
	if !strings.Contains(err.Error(), "nil executor") {
		t.Fatalf("error = %v, want the jobs.NewManager nil-executor failure", err)
	}
	path := filepath.Join(dir, "sess-partial.jsonl")
	if _, serr := os.Stat(path); !os.IsNotExist(serr) {
		t.Fatalf("partial session file at %s was abandoned (stat err = %v): want removed", path, serr)
	}
}

// TestMintSessionIDFailsClosed (F6): when crypto/rand fails, id minting
// returns an error (no time-based fallback) and session/create without
// a sessionId surfaces a clean engine error leaving no state.
func TestMintSessionIDFailsClosed(t *testing.T) {
	orig := randRead
	randRead = func(b []byte) (int, error) { return 0, errors.New("entropy exhausted (test)") }
	defer func() { randRead = orig }()

	if id, err := mintSessionID(); err == nil {
		t.Fatalf("mintSessionID under rand failure returned %q: want error", id)
	}

	client, _, stop := newConfinedPair(t)
	defer stop()
	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	err := client.Call("session/create", map[string]any{}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrEngine {
		t.Fatalf("session/create without sessionId under rand failure error = %v, want *Error{Code: ErrEngine}", err)
	}
	var eerr *Error
	if errors.As(err, &eerr) && !strings.Contains(eerr.Message, "crypto/rand") {
		t.Fatalf("engine error message = %q, want it to name crypto/rand", eerr.Message)
	}
}
