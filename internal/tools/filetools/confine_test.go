// confine_test.go — table tests for the symlink-safe workdir-root
// confinement helper: the file tool family's safety core. Every rule
// that can reject must reject BEFORE any filesystem effect (the
// rejected-write-leaves-no-trace contract is exercised in write_test).
package filetools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRoots builds Roots over fresh temp dirs.
func newTestRoots(t *testing.T, n int) (Roots, []string) {
	t.Helper()
	dirs := make([]string, 0, n)
	for i := 0; i < n; i++ {
		dirs = append(dirs, t.TempDir())
	}
	r, err := NewRoots(dirs)
	if err != nil {
		t.Fatalf("NewRoots: %v", err)
	}
	return r, dirs
}

// wantRule asserts err is a *ViolationError with the given rule.
func wantRule(t *testing.T, err error, rule string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want violation rule %q, got nil", rule)
	}
	var v *ViolationError
	if !errors.As(err, &v) {
		t.Fatalf("want *ViolationError, got %T: %v", err, err)
	}
	if v.Rule != rule {
		t.Fatalf("violation rule = %q, want %q (detail: %s)", v.Rule, rule, v.Detail)
	}
	if v.Path == "" {
		t.Fatalf("violation must name the offending path: %+v", v)
	}
}

func TestNewRootsValidation(t *testing.T) {
	if _, err := NewRoots(nil); err == nil {
		t.Fatal("NewRoots(nil) must fail: no roots means every path is rejected — wire at least the working directory")
	}
	if _, err := NewRoots([]string{""}); err == nil {
		t.Fatal("NewRoots with an empty entry must fail")
	}
	if _, err := NewRoots([]string{"relative/path"}); err == nil {
		t.Fatal("NewRoots with a relative entry must fail (roots are configured absolute)")
	}
	if _, err := NewRoots([]string{filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Fatal("NewRoots with a nonexistent directory must fail")
	}
	// The zero Roots value rejects everything fail-closed.
	var zero Roots
	_, _, err := zero.resolveRead("x.txt")
	wantRule(t, err, RuleNoRoots)
}

// TestNewRootsRejectsNonDirectory: a configured root must resolve to
// an existing DIRECTORY. EvalSymlinks alone admits a regular file
// (directly or through a symlink) — a root that is a file would make
// every relative resolution target a non-directory and narrow the
// workspace silently, so it rejects fail-closed at construction with
// the typed not-a-directory rule.
func TestNewRootsRejectsNonDirectory(t *testing.T) {
	dir := t.TempDir()
	fileRoot := filepath.Join(dir, "plain.txt")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkToFile := filepath.Join(dir, "link-file")
	if err := os.Symlink(fileRoot, linkToFile); err != nil {
		t.Fatal(err)
	}

	for _, root := range []string{fileRoot, linkToFile} {
		_, err := NewRoots([]string{root})
		wantRule(t, err, RuleNotADirectory)
		var v *ViolationError
		if !errors.As(err, &v) || !strings.Contains(v.Detail, "not a directory") {
			t.Fatalf("detail must say \"not a directory\": %v", err)
		}
	}

	// Positive control: a symlink TO a directory stays admitted
	// (canonicalized through to its target) — the new check is on
	// directory-ness, never on symlinks themselves.
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f.txt"), []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkToDir := filepath.Join(dir, "link-dir")
	if err := os.Symlink(sub, linkToDir); err != nil {
		t.Fatal(err)
	}
	r, err := NewRoots([]string{linkToDir})
	if err != nil {
		t.Fatalf("symlinked directory root must be admitted: %v", err)
	}
	if len(r) != 1 {
		t.Fatalf("roots = %v, want exactly the one resolved root", r)
	}
	if _, _, err := r.resolveRead("f.txt"); err != nil {
		t.Fatalf("resolution through a symlinked-directory root must work: %v", err)
	}
}

func TestResolveReadTable(t *testing.T) {
	roots, dirs := newTestRoots(t, 2)
	root0, root1 := dirs[0], dirs[1]

	// A file in root0 to read.
	if err := os.WriteFile(filepath.Join(root0, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A nested file reached through an in-root subdirectory.
	if err := os.MkdirAll(filepath.Join(root0, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root0, "sub", "b.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		path    string
		wantAbs string // "" means expect rejection (checked by wantRule)
		rule    string
	}{
		{name: "relative resolves against first root", path: "a.txt", wantAbs: filepath.Join(root0, "a.txt")},
		{name: "nested relative", path: "sub/b.txt", wantAbs: filepath.Join(root0, "sub", "b.txt")},
		{name: "cleaned dot path", path: "./sub/../a.txt", wantAbs: filepath.Join(root0, "a.txt")},
		{name: "absolute inside first root", path: filepath.Join(root0, "a.txt"), wantAbs: filepath.Join(root0, "a.txt")},
		{name: "absolute inside second root", path: filepath.Join(root1, "c.txt"), wantAbs: filepath.Join(root1, "c.txt")},
		{name: "parent climb escapes", path: "../escape.txt", rule: RuleEscape},
		{name: "deep climb escapes", path: "sub/../../escape.txt", rule: RuleEscape},
		{name: "absolute outside all roots", path: "/etc/hostname", rule: RuleOutsideRoots},
		{name: "root itself is not a file", path: ".", rule: RuleNotAFile},
		{name: "missing file is a plain read error, not a violation", path: "no-such.txt", wantAbs: filepath.Join(root0, "no-such.txt")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			abs, _, err := roots.resolveRead(tc.path)
			if tc.rule != "" {
				wantRule(t, err, tc.rule)
				return
			}
			if err != nil {
				t.Fatalf("resolveRead(%q): %v", tc.path, err)
			}
			if abs != tc.wantAbs {
				t.Fatalf("abs = %q, want %q", abs, tc.wantAbs)
			}
		})
	}
}

func TestResolveReadSymlinkEscape(t *testing.T) {
	roots, dirs := newTestRoots(t, 1)
	root := dirs[0]
	outside := t.TempDir() // a second temp dir, outside the root

	// A symlinked DIRECTORY inside the root pointing outside.
	if err := os.Symlink(outside, filepath.Join(root, "link-out")); err != nil {
		t.Fatal(err)
	}
	_, _, err := roots.resolveRead("link-out/file.txt")
	wantRule(t, err, RuleSymlinkEscape)

	// A symlinked directory CHAIN: link-in -> in-root dir is fine to
	// walk? No: EvalSymlinks resolves it back inside, admitted.
	if err := os.Mkdir(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real", "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link-in")); err != nil {
		t.Fatal(err)
	}
	abs, _, err := roots.resolveRead("link-in/f.txt")
	if err != nil {
		t.Fatalf("in-root symlinked directory should resolve (parent containment holds): %v", err)
	}
	if abs != filepath.Join(root, "link-in", "f.txt") {
		t.Fatalf("abs = %q", abs)
	}

	// A symlink AT the final component: rejected outright (the
	// confineSessionPath rule — os.Create would truncate its target,
	// and read would follow it, an in-root indistinguishable escape).
	if err := os.WriteFile(filepath.Join(outside, "target.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "target.txt"), filepath.Join(root, "final-link.txt")); err != nil {
		t.Fatal(err)
	}
	_, _, err = roots.resolveRead("final-link.txt")
	wantRule(t, err, RuleSymlinkFinal)

	// An in-root final symlink is rejected too (indistinguishable at
	// check time; fail closed).
	if err := os.Symlink(filepath.Join(root, "real", "f.txt"), filepath.Join(root, "inroot-link.txt")); err != nil {
		t.Fatal(err)
	}
	_, _, err = roots.resolveRead("inroot-link.txt")
	wantRule(t, err, RuleSymlinkFinal)
}

func TestResolveWriteMissingParents(t *testing.T) {
	roots, dirs := newTestRoots(t, 1)
	root := dirs[0]

	// A write target whose parent chain does not exist yet resolves
	// fine: creation is the write tool's job.
	abs, _, err := roots.resolveWrite("deep/nested/new.txt")
	if err != nil {
		t.Fatalf("resolveWrite with missing parents: %v", err)
	}
	if abs != filepath.Join(root, "deep", "nested", "new.txt") {
		t.Fatalf("abs = %q", abs)
	}

	// An existing INTERMEDIATE component that is a FILE (not a dir)
	// rejects before any effect.
	if err := os.WriteFile(filepath.Join(root, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = roots.resolveWrite("blocker/under.txt")
	wantRule(t, err, RuleNotADirectory)

	// An existing intermediate SYMLINK rejects (fail-closed: the walk
	// never follows it, so the write can never land outside).
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "sneak")); err != nil {
		t.Fatal(err)
	}
	_, _, err = roots.resolveWrite("sneak/under.txt")
	wantRule(t, err, RuleSymlinkEscape)

	// Final component as an existing directory rejects.
	if err := os.Mkdir(filepath.Join(root, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err = roots.resolveWrite("adir")
	wantRule(t, err, RuleIsDirectory)

	// Same escapes as read, before any effect.
	_, _, err = roots.resolveWrite("../out.txt")
	wantRule(t, err, RuleEscape)
	_, _, err = roots.resolveWrite("/etc/passwd-new")
	wantRule(t, err, RuleOutsideRoots)
}

// TestDefinitionsPanicsOnNonDirectoryRoot pins the panic posture: the
// Definitions panic is the WIRING-BUG backstop for library callers
// that bypass validation, NOT the reachable path for daemon input —
// a --workdir-roots file root refuses at validate (exit 2, see
// cmd/vh-agentd TestRunWorkdirRootsNotDirectoryExits2) long before
// Definitions is wired. The panic converting the NEW not-a-directory
// rule keeps that posture: fail loudly at startup, never mid-session.
func TestDefinitionsPanicsOnNonDirectoryRoot(t *testing.T) {
	fileRoot := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Definitions with a file root must panic (wiring-bug backstop)")
		}
	}()
	_ = Definitions(Config{Roots: []string{fileRoot}})
}
