// writer_cleanup_test.go — the OpenChildFile orphan-hygiene contract:
// a header-write failure must not leave a 0-byte orphan child-log file
// on disk. The orphan is consequence-bounded (never referenced by a
// parent, fails reopen fail-closed) but it is hygiene debt — these
// tests pin the cleanup behavior.
//
// Failure injection: the log path is a SYMLINK to /dev/full, so
// os.Create succeeds (it opens the device for writing — no directory
// entry is created) but the first header Write fails with ENOSPC, a
// real kernel-level write failure on a path the fix can still unlink
// (removing a symlink never touches its target). Skipped on hosts
// without /dev/full (non-Linux).
package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// requireDevFull skips the test when /dev/full is unavailable.
func requireDevFull(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skipf("no /dev/full on this host; cannot inject a kernel-level header-write failure: %v", err)
	}
}

// TestOpenChildFileHeaderWriteFailureRemovesOrphan: when the header
// write fails after a successful create, OpenChildFile must close AND
// REMOVE the file — no 0-byte orphan may remain on disk. The original
// error is returned unwrapped when the removal succeeds.
func TestOpenChildFileHeaderWriteFailureRemovesOrphan(t *testing.T) {
	requireDevFull(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "child.jsonl")
	if err := os.Symlink("/dev/full", path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	lg, err := OpenChildFile(path, "sess-orphan", ChildHeader{ParentSessionID: "parent", DelegationDepth: 1})
	if err == nil {
		t.Fatal("OpenChildFile succeeded against /dev/full; the header-write failure was not injected")
	}
	if lg != nil {
		t.Fatalf("log = %v, want nil on failure", lg)
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Fatalf("error = %v, want the original header-write failure (ENOSPC)", err)
	}
	if _, serr := os.Stat(path); !os.IsNotExist(serr) {
		t.Fatalf("orphan child-log file left on disk after failed header write: %s", path)
	}
}

// TestOpenChildFileCleanupFailureKeepsOriginalError: when the
// best-effort removal itself fails (here: the parent directory is made
// non-writable, so unlink returns EACCES), the returned error must
// still carry the ORIGINAL header-write failure — never masked — with a
// note naming the failed cleanup.
func TestOpenChildFileCleanupFailureKeepsOriginalError(t *testing.T) {
	requireDevFull(t)
	dir := t.TempDir()
	defer os.Chmod(dir, 0o700) // restore for t.TempDir cleanup
	path := filepath.Join(dir, "child.jsonl")
	if err := os.Symlink("/dev/full", path); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod dir read-only: %v", err)
	}

	_, err := OpenChildFile(path, "sess-orphan2", ChildHeader{})
	if err == nil {
		t.Fatal("OpenChildFile succeeded against /dev/full; the header-write failure was not injected")
	}
	if !strings.Contains(err.Error(), "no space left on device") {
		t.Fatalf("error = %v, want the ORIGINAL header-write failure to survive (never masked)", err)
	}
	if !strings.Contains(err.Error(), "removal") {
		t.Fatalf("error = %v, want the cleanup-failure note alongside the original error", err)
	}
}
