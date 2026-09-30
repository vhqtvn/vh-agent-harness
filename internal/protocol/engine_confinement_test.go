// engine_confinement_test.go — TB-F1 regression tests: the client-
// controlled session/create path parameter must NEVER let a wire peer
// create-or-truncate (os.Create is O_TRUNC) a file outside the engine's
// session Dir. Every escape shape — lexical ../, absolute outside the
// root, a symlink at the target, a symlinked parent dir — must be
// rejected with the typed *SessionPathError (surfaced on the wire as a
// clean -32602 ErrInvalidParams) leaving the filesystem untouched and
// no partial server state. Valid paths (relative in-root, absolute
// in-root, in-root symlinks) must keep working: the wire shape is
// unchanged, validation is server-side only.
package protocol

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newConfinedPair boots a real FileEngine server rooted at a fresh temp
// dir (the production composition: Dir always declared).
func newConfinedPair(t *testing.T) (*Client, *FileEngine, func()) {
	t.Helper()
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)
	cleanup := func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}
	return client, eng, cleanup
}

// TestNewSessionConfinementTable drives the engine seam directly: every
// escape path is rejected with the typed error and creates NOTHING; every
// valid path resolves inside the root.
func TestNewSessionConfinementTable(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "sessions")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}
	sibling := filepath.Join(parent, "sibling")
	if err := os.Mkdir(sibling, 0o755); err != nil {
		t.Fatalf("mkdir sibling dir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	rejects := []struct {
		name string
		path string
		// mustNotExist is asserted absent after the rejected create.
		mustNotExist string
	}{
		{"relative parent escape", "../sibling/escape.jsonl", filepath.Join(sibling, "escape.jsonl")},
		{"deep relative escape", "sub/../../sibling/escape2.jsonl", filepath.Join(sibling, "escape2.jsonl")},
		{"dot-dot filename", "..", ""},
		{"root itself", ".", ""},
		{"absolute outside root", filepath.Join(sibling, "abs-escape.jsonl"), filepath.Join(sibling, "abs-escape.jsonl")},
	}

	for _, tc := range rejects {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
			es, err := eng.NewSession(tc.path, "sess-reject", io.Discard)
			var spe *SessionPathError
			if err == nil {
				if es != nil {
					_ = es.Log.Close()
				}
				t.Fatalf("NewSession(%q) succeeded (path=%s): want typed rejection", tc.path, es.Path)
			}
			if !errors.As(err, &spe) {
				t.Fatalf("NewSession(%q) error = %v (%T), want *SessionPathError", tc.path, err, err)
			}
			if tc.mustNotExist != "" {
				if _, statErr := os.Stat(tc.mustNotExist); statErr == nil {
					t.Fatalf("rejected create left a file at %s", tc.mustNotExist)
				}
			}
		})
	}

	valids := []struct {
		name string
		path string
		want string // expected es.Path (canonical, inside dir)
	}{
		{"relative in root", "sess.jsonl", filepath.Join(dir, "sess.jsonl")},
		{"relative nested", "sub/sess.jsonl", filepath.Join(dir, "sub", "sess.jsonl")},
		{"lexically cleaning", "sub/../sess-clean.jsonl", filepath.Join(dir, "sess-clean.jsonl")},
		{"absolute in root", filepath.Join(dir, "abs-sess.jsonl"), filepath.Join(dir, "abs-sess.jsonl")},
	}
	for _, tc := range valids {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}
			es, err := eng.NewSession(tc.path, "sess-valid", io.Discard)
			if err != nil {
				t.Fatalf("NewSession(%q): %v", tc.path, err)
			}
			defer es.Log.Close()
			if es.Path != tc.want {
				t.Fatalf("es.Path = %q, want %q", es.Path, tc.want)
			}
			fi, statErr := os.Stat(tc.want)
			if statErr != nil || fi.Size() == 0 {
				t.Fatalf("session file at %s not created non-empty: %v", tc.want, statErr)
			}
		})
	}
}

// TestNewSessionRejectsSymlinkEscape covers the symlink shapes: a
// symlink AT the target (os.Create follows it and truncates whatever it
// points at) and a symlinked PARENT directory leading outside the root.
// Both must be rejected WITHOUT touching the outside files; an in-root
// symlinked dir stays valid.
func TestNewSessionRejectsSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "sessions")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir sessions dir: %v", err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "deep"), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	const sentinel = "SENTINEL-do-not-truncate\n"
	victim := filepath.Join(outside, "victim.jsonl")
	if err := os.WriteFile(victim, []byte(sentinel), 0o644); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	eng := &FileEngine{Dir: dir, Executor: noopExecutor{}}

	// Shape 1: symlink at the final component, pointing outside.
	link := filepath.Join(dir, "sess-link.jsonl")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatalf("symlink target: %v", err)
	}
	if _, err := eng.NewSession("sess-link.jsonl", "sess-sym1", io.Discard); err == nil {
		t.Fatal("NewSession through target symlink succeeded: want typed rejection")
	} else {
		var spe *SessionPathError
		if !errors.As(err, &spe) {
			t.Fatalf("target-symlink error = %v (%T), want *SessionPathError", err, err)
		}
	}
	got, rerr := os.ReadFile(victim)
	if rerr != nil || string(got) != sentinel {
		t.Fatalf("victim through target symlink was truncated: content=%q err=%v", got, rerr)
	}
	if fi, lerr := os.Lstat(link); lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink at %s was replaced by a regular file: %v", link, lerr)
	}

	// Shape 2: symlinked PARENT directory leading outside the root.
	plink := filepath.Join(dir, "out-link")
	if err := os.Symlink(outside, plink); err != nil {
		t.Fatalf("symlink parent: %v", err)
	}
	hijack := filepath.Join(outside, "hijack.jsonl")
	if _, err := eng.NewSession("out-link/hijack.jsonl", "sess-sym2", io.Discard); err == nil {
		t.Fatal("NewSession through symlinked parent succeeded: want typed rejection")
	} else {
		var spe *SessionPathError
		if !errors.As(err, &spe) {
			t.Fatalf("parent-symlink error = %v (%T), want *SessionPathError", err, err)
		}
	}
	if _, serr := os.Stat(hijack); serr == nil {
		t.Fatalf("hijack file created outside the root at %s", hijack)
	}

	// Shape 3 (valid): a symlinked directory pointing INSIDE the root
	// resolves inside the root and stays admissible.
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatalf("mkdir real: %v", err)
	}
	inlink := filepath.Join(dir, "in-link")
	if err := os.Symlink(real, inlink); err != nil {
		t.Fatalf("symlink in-link: %v", err)
	}
	es, err := eng.NewSession("in-link/sess.jsonl", "sess-sym3", io.Discard)
	if err != nil {
		t.Fatalf("NewSession through in-root symlink: %v", err)
	}
	defer es.Log.Close()
	if es.Path != filepath.Join(inlink, "sess.jsonl") {
		t.Fatalf("es.Path = %q, want the in-root symlinked path", es.Path)
	}
	if _, serr := os.Stat(filepath.Join(real, "sess.jsonl")); serr != nil {
		t.Fatalf("in-root symlinked session file missing: %v", serr)
	}
}

// TestSessionCreateWireConfinement drives the WIRE: an escaping path
// yields a clean -32602 JSON-RPC error, leaves no file, and no partial
// server state (no active session); a valid relative create then
// succeeds against the declared root.
func TestSessionCreateWireConfinement(t *testing.T) {
	client, eng, stop := newConfinedPair(t)
	defer stop()
	outside := t.TempDir()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Absolute path outside the engine root: rejected as invalid params.
	escape := filepath.Join(outside, "wire-escape.jsonl")
	err := client.Call("session/create", map[string]any{"path": escape, "sessionId": "sess-escape"}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("session/create(escape) error = %v, want *Error{Code: ErrInvalidParams}", err)
	}
	if _, serr := os.Stat(escape); serr == nil {
		t.Fatalf("escape create left a file at %s", escape)
	}

	// Relative parent escape over the wire, same discipline.
	err = client.Call("session/create", map[string]any{"path": "../wire-rel-escape.jsonl", "sessionId": "sess-esc2"}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("session/create(relative escape) error = %v, want *Error{Code: ErrInvalidParams}", err)
	}

	// No partial state: nothing became the active session.
	err = client.Call("session/prompt", map[string]any{"text": "x"}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrNoSession {
		t.Fatalf("session/prompt after refused create error = %v, want ErrNoSession", err)
	}

	// Valid relative create against the declared root.
	var res createResult
	if err := client.Call("session/create", map[string]any{"path": "wire-sess.jsonl", "sessionId": "sess-wire"}, &res); err != nil {
		t.Fatalf("session/create(valid): %v", err)
	}
	want := filepath.Join(eng.Dir, "wire-sess.jsonl")
	if res.SessionID != "sess-wire" || res.Path != want {
		t.Fatalf("create result = %+v, want path %s", res, want)
	}
	if fi, serr := os.Stat(want); serr != nil || fi.Size() == 0 {
		t.Fatalf("valid session file at %s missing/empty: %v", want, serr)
	}
}
