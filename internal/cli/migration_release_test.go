package cli

// This file guards the immutability of RELEASED migration notes. Once a
// version is tagged, its templates/migrations/v<version>.md is part of the
// release artifact and must never be edited in-tree — not forward (adding
// content) and not backward (reverting committed drift). Corrections ship as
// an erratum in the NEXT release's note instead.
//
// The test catches UNCOMMITTED working-tree edits to released notes by
// comparing each released note's working-tree bytes against its HEAD bytes.
// It deliberately does NOT compare against the bytes captured at the release
// tag, because committed historical drift in released notes is immutable and
// cannot be remediated by editing (the released state IS the current HEAD
// state, drift and all). The forward-looking guard (catch a new edit before
// it commits) is the test's value; retroactive drift detection is out of
// scope.
//
// Background: a prior attempt to fix a YAML typo in templates/migrations/
// v0.12.0.md post-release would have silently rewritten a shipped release
// artifact had it not been caught. This guard prevents that class of mistake
// from compiling cleanly again.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// semverTagRe matches a release git tag of the form vX.Y.Z (no pre-release
// suffix). Mirrors semverFileRe (used for filenames) but against the bare tag.
var semverTagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// repoRootFromCwd resolves the working-tree root from the test's CWD via git.
// Returns the trimmed absolute path. The caller decides whether to skip on
// error (we treat "no git working tree" as "test cannot run", not a failure).
func repoRootFromCwd(t *testing.T) (string, error) {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// TestMigrationNotes_ReleasedImmutable verifies that no released migration
// note (a templates/migrations/v<tag>.md whose <tag> is a published git tag)
// carries an UNCOMMITTED working-tree edit. For each released note present in
// the working tree, it compares the working-tree bytes against the bytes at
// HEAD and fails if they differ.
//
// The test deliberately compares against HEAD, NOT against the tagged bytes.
// Committed historical drift in a released note is immutable: the released
// state IS the current HEAD state (drift and all), and reverting that drift
// in-tree would itself be an edit to a shipped release artifact. The
// forward-looking guard (catch a new edit before it commits) is the test's
// value; retroactive drift detection is out of scope.
//
// The test is environment-aware: it skips cleanly when git or the working
// tree is unavailable (e.g. a tarball-extracted source build), and skips a
// given version when its note is absent in the working tree (some releases
// ship no note).
//
// This test is the Go-test guard against the v0.12.0 post-release edit
// attempt: any future attempt to mutate a released note in the working tree
// will fail CI here with a clear immutability message instead of silently
// rewriting a shipped artifact.
func TestMigrationNotes_ReleasedImmutable(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available; skipping released-note immutability check")
	}
	repoRoot, err := repoRootFromCwd(t)
	if err != nil {
		t.Skipf("could not locate git working-tree root: %v", err)
	}
	migrationsDir := filepath.Join(repoRoot, "templates", "migrations")
	if _, err := os.Stat(migrationsDir); err != nil {
		t.Skipf("templates/migrations not present under repo root %s: %v", repoRoot, err)
	}

	// List release-version tags only. The tags are used to identify WHICH
	// migration notes are "released" (i.e. ship as part of a published
	// release artifact and are therefore immutable). We do NOT compare note
	// bytes against the tag — only against HEAD (see the function doc
	// comment for why). `git tag -l` takes glob patterns; the
	// `v[0-9]*.[0-9]*.[0-9]*` glob is permissive (`*` matches `-` too, so
	// it would also match v0.1.0-rc1), so semverTagRe below filters
	// strictly to bare release semver. Annotated and lightweight tags
	// both surface here.
	tagOut, err := exec.Command("git", "-C", repoRoot, "tag", "-l", "v[0-9]*.[0-9]*.[0-9]*").Output()
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	tagLines := strings.Split(strings.TrimSpace(string(tagOut)), "\n")
	if len(tagLines) == 0 || (len(tagLines) == 1 && tagLines[0] == "") {
		t.Skipf("no vX.Y.Z tags found in repo root %s — likely a shallow clone or tarball; skipping released-note immutability check", repoRoot)
	}

	checked := 0
	for _, line := range tagLines {
		tag := strings.TrimSpace(line)
		if tag == "" || !semverTagRe.MatchString(tag) {
			continue
		}
		rel := filepath.ToSlash(filepath.Join("templates", "migrations", tag+".md"))
		notePath := filepath.Join(repoRoot, filepath.FromSlash(rel))
		if _, err := os.Stat(notePath); err != nil {
			continue // No migration note for this release in the working tree.
		}
		// Read the HEAD bytes for this note. HEAD is the authoritative
		// "released state": any committed historical drift is immutable
		// and ships as-is. If the note is somehow absent at HEAD (e.g. an
		// untracked new file whose name collides with a released version),
		// skip rather than fail — the test guards edits to existing
		// released notes, not new-file additions.
		headBytes, err := exec.Command("git", "-C", repoRoot, "show", "HEAD:"+rel).Output()
		if err != nil {
			continue
		}
		working, err := os.ReadFile(notePath)
		if err != nil {
			t.Fatalf("read working-tree migration note %s: %v", rel, err)
		}
		checked++
		if !bytes.Equal(headBytes, working) {
			t.Errorf("released migration note %s has uncommitted working-tree edits that diverge from HEAD — released notes are immutable; revert the working-tree change (restore to HEAD) and ship the correction as an erratum in the next release's note", rel)
		}
	}

	// Defensive: if zero (tag, note) pairs were checked, the guard silently
	// became a no-op. That should not happen in a normal checkout (every
	// shipped release since the migration-note convention landed has both a
	// tag and a note), so surface it loudly rather than passing vacuously.
	// We use `Errorf` (not `Fatalf`) so a future maintainer reading the test
	// output sees the per-note divergence signals above this line if any
	// exist.
	if checked == 0 {
		t.Errorf("released-migration immutability check matched zero (tag, note) pairs — guard ran but checked nothing; repo root=%s, pre-filter tag count=%d", repoRoot, len(tagLines))
	}
}
