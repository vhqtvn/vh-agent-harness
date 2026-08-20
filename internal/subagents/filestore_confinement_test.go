// filestore_confinement_test.go — TB-F1 round-2: the FileStore composes
// <root>/<parentSessionID>/<childID>.jsonl from LOG-DERIVED ids (the
// parent header's session id, and child ids folded from the parent's
// subagent/spawned events). Normally engine-minted, but a forged or
// foreign log can carry traversal ids — and ReopenChild's ResumeFile
// TRUNCATES torn tails, so an unvalidated id is a write primitive.
// Both ids must be strict filename components before any path is
// composed; hostile shapes create NOTHING.
package subagents

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TestFileStoreHostileIDs covers both store seams with hostile parent
// and child ids: CreateChild (MkdirAll + truncating create) and
// ReopenChild (read + possible torn-tail truncate).
func TestFileStoreHostileIDs(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "store")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("mkdir store root: %v", err)
	}
	outside := filepath.Join(parent, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	hostileIDs := []string{
		"../../evil",
		"..",
		".",
		"sub/victim",
		"/abs/victim",
		".hidden",
	}

	for _, id := range hostileIDs {
		t.Run("create/parent/"+id, func(t *testing.T) {
			s := NewFileStore(root)
			lg, err := s.CreateChild(id, "child-1", session.ChildHeader{ParentSessionID: id, DelegationDepth: 1})
			if err == nil {
				_ = lg.Close()
				t.Fatalf("CreateChild(parent=%q) succeeded: want confinement rejection", id)
			}
			if !strings.Contains(err.Error(), "parent session id") {
				t.Fatalf("CreateChild(parent=%q) error = %v, want it to name the parent session id", id, err)
			}
		})
		t.Run("create/child/"+id, func(t *testing.T) {
			s := NewFileStore(root)
			lg, err := s.CreateChild("sess-ok", id, session.ChildHeader{ParentSessionID: "sess-ok", DelegationDepth: 1})
			if err == nil {
				_ = lg.Close()
				t.Fatalf("CreateChild(child=%q) succeeded: want confinement rejection", id)
			}
			if !strings.Contains(err.Error(), "child id") {
				t.Fatalf("CreateChild(child=%q) error = %v, want it to name the child id", id, err)
			}
		})
		t.Run("reopen/parent/"+id, func(t *testing.T) {
			s := NewFileStore(root)
			if lg, err := s.ReopenChild(id, "child-1"); err == nil {
				_ = lg.Close()
				t.Fatalf("ReopenChild(parent=%q) succeeded: want confinement rejection", id)
			}
		})
		t.Run("reopen/child/"+id, func(t *testing.T) {
			s := NewFileStore(root)
			if lg, err := s.ReopenChild("sess-ok", id); err == nil {
				_ = lg.Close()
				t.Fatalf("ReopenChild(child=%q) succeeded: want confinement rejection", id)
			}
		})
	}

	// Nothing escaped: the store root still contains no per-parent
	// directories, and the outside dir is empty.
	ents, rerr := os.ReadDir(root)
	if rerr != nil {
		t.Fatalf("readdir root: %v", rerr)
	}
	if len(ents) != 0 {
		t.Fatalf("hostile ids left entries in the store root: %v", ents)
	}
	outEnts, oerr := os.ReadDir(outside)
	if oerr != nil {
		t.Fatalf("readdir outside: %v", oerr)
	}
	if len(outEnts) != 0 {
		t.Fatalf("hostile ids escaped to %s: %v", outside, outEnts)
	}
}

// TestFileStoreValidIDsRoundTrip: the strict grammar admits the
// engine-minted shapes (sess-<hex>, <parent>.<N> descendants) and the
// create/reopen round trip keeps working.
func TestFileStoreValidIDsRoundTrip(t *testing.T) {
	root := t.TempDir()
	s := NewFileStore(root)
	lg, err := s.CreateChild("sess-abc123", "sess-abc123.1", session.ChildHeader{ParentSessionID: "sess-abc123", DelegationDepth: 1})
	if err != nil {
		t.Fatalf("CreateChild: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("close child: %v", err)
	}
	want := filepath.Join(root, "sess-abc123", "sess-abc123.1.jsonl")
	if _, serr := os.Stat(want); serr != nil {
		t.Fatalf("child log at %s missing: %v", want, serr)
	}
	rlg, err := s.ReopenChild("sess-abc123", "sess-abc123.1")
	if err != nil {
		t.Fatalf("ReopenChild: %v", err)
	}
	if err := rlg.Close(); err != nil {
		t.Fatalf("close reopened child: %v", err)
	}
}

// TestFileStoreErrorsWrapSentinel: rejections wrap the session package's
// ErrInvalidIDComponent sentinel so callers can errors.Is on it.
func TestFileStoreErrorsWrapSentinel(t *testing.T) {
	s := NewFileStore(t.TempDir())
	_, err := s.CreateChild("../evil", "child-1", session.ChildHeader{})
	if !errors.Is(err, session.ErrInvalidIDComponent) {
		t.Fatalf("CreateChild error = %v, want it to wrap session.ErrInvalidIDComponent", err)
	}
	_, err = s.ReopenChild("sess-ok", "../../evil")
	if !errors.Is(err, session.ErrInvalidIDComponent) {
		t.Fatalf("ReopenChild error = %v, want it to wrap session.ErrInvalidIDComponent", err)
	}
}
