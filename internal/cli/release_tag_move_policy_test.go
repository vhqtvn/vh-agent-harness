package cli

// Policy-regression tests for scripts/release-tag.sh pinning the
// "never move a published version" immutability contract (solution-brief
// ses_01a307789 — RETIRE-O1 / adopt-never-move).
//
// These tests pin TWO refusal paths the wrapper has enforced since its origin
// but that were NOT previously covered by an explicit regression test (the E10
// gap from the brief):
//
//   (A) CREATE-FLOW existing-tag refusal (release-tag.sh:232-238) — the
//       immutability enforcement: an already-cut vX.Y.Z is REFUSED by the
//       create flow, never moved/replaced/recreated. The annotated tag object
//       SHA and its dereferenced commit SHA are byte-identical before and after
//       the refused invocation.
//
//   (B) NO-MOVE-MODE — every plausible "move the published tag" verb (--move,
//       --force, --move-published-tag) reaches the default flag-parsing case
//       (release-tag.sh:287-290) and is refused as an unknown argument. This
//       proves the wrapper ships NO force/move branch.
//
// These tests do NOT change release-tag.sh behavior. They pin the existing
// refusal semantics so a future edit that adds a move branch (or weakens the
// existing-tag check) fails here before it lands.
//
// See README.agent.md → "Published-tag immutability (never move a published
// version)" for the full 7-point policy contract.

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCreateFlow invokes the wrapper in the CREATE flow (no --push-only) with
// RELEASE_TAG_MESSAGE_FILE set. cwd is <scratch> so the wrapper resolves its
// relative-path dependencies. Returns exit code, parsed JSON result, raw
// stdout, raw stderr. RELEASE_TAG_MESSAGE_FILE is set in every call because
// the create-flow validation (release-tag.sh:188-193) runs BEFORE the
// existing-tag refusal (line 234) and the flag-parsing loop (line 260).
func runCreateFlow(t *testing.T, wrapper, msgFile, version string, extraArgs []string) (int, releaseTagManifestResult, string, string) {
	t.Helper()
	args := []string{wrapper, version}
	args = append(args, extraArgs...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper)) // <scratch>
	cmd.Env = append(os.Environ(),
		"RELEASE_TAG_MESSAGE_FILE="+msgFile,
	)
	var outb, errb strings.Builder
	cmd.Stdout = &outb
	cmd.Stderr = &errb
	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bash spawn error: %v\nstderr: %s", runErr, errb.String())
		}
	}
	stdout := outb.String()
	stderr := errb.String()
	var result releaseTagManifestResult
	if stdout != "" {
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("wrapper output must be valid JSON (exit=%d): %v\nstdout:\n%s\nstderr:\n%s",
				exitCode, err, stdout, stderr)
		}
	}
	return exitCode, result, stdout, stderr
}

// =============================================================================
// (A) CREATE-FLOW existing-tag refusal — the immutability enforcement
// =============================================================================

// TestReleaseTag_PublishedTagImmutability_RefusesExistingTagCreate — a
// successfully-published tag (here modeled by a locally-cut annotated tag at
// HEAD) MUST be REFUSED by the create flow. The wrapper never moves, replaces,
// or recreates it. The annotated tag object SHA and its dereferenced commit
// SHA must be byte-identical before and after the refused invocation, proving
// no silent rewrite. This is the core guarantee of the "never move a published
// version" policy: correction ships as a SUCCESSOR version through the full
// N→R→M + G0-G7 ceremony, never as a move of the existing tag.
//
// This test was MISSING from the suite (E10 gap): existing tests covered
// push-only refusal paths and manifest-gate refusals, but no test pinned the
// create-flow existing-tag refusal (release-tag.sh:232-238) with an
// immutability proof. Added here.
func TestReleaseTag_PublishedTagImmutability_RefusesExistingTagCreate(t *testing.T) {
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)

	// Simulate an already-published v0.2.0: cut an annotated tag at HEAD.
	// (The default fixture leaves v0.2.0 uncut, so this models the
	// "already published" precondition the immutability contract protects.)
	cutAnnotatedTag(t, scratch, "v0.2.0", msgFile)
	targetBefore := tagTargetSHA(t, scratch, "v0.2.0")
	objBefore := tagObjectSHA(t, scratch, "v0.2.0")

	// Re-invoke the create flow for the SAME version. The wrapper must
	// REFUSE with "already exists" and leave the tag byte-identical.
	exitCode, result, _, _ := runCreateFlow(t, wrapper, msgFile, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("create flow must REFUSE an existing tag (never move a published version); got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if result.Pushed {
		t.Errorf("expected pushed=false; got true")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "already exists") {
		t.Errorf("error must mention 'already exists'; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "v0.2.0") {
		t.Errorf("error must name the version v0.2.0; got %v", result.Error)
	}

	// Immutability proof: the annotated tag object AND its dereferenced
	// commit must be byte-identical (no silent move/replacement). This is
	// the load-bearing assertion — a future edit that "helpfully" recreates
	// the tag fails here.
	if after := tagTargetSHA(t, scratch, "v0.2.0"); after != targetBefore {
		t.Errorf("tag target commit must be unchanged (never moved); before=%s after=%s", targetBefore, after)
	}
	if after := tagObjectSHA(t, scratch, "v0.2.0"); after != objBefore {
		t.Errorf("tag object SHA must be unchanged (never replaced); before=%s after=%s", objBefore, after)
	}
}

// =============================================================================
// (B) NO-MOVE-MODE — every move/force verb is an unknown-argument refusal
// =============================================================================

// TestReleaseTag_PublishedTagImmutability_RefusesMoveVerbs — the wrapper ships
// NO --force / --move / --move-published-tag branch. Any such verb reaches the
// default flag-parsing case (release-tag.sh:287-290) and is refused as an
// unknown argument. This pins that no move mode exists: a future edit adding a
// move branch fails this test before it lands.
//
// The version requested (v0.2.0) is UNPUBLISHED in the default fixture (uncut),
// so the existing-tag check (release-tag.sh:234) passes and the parser reaches
// the move-verb refusal rather than the existing-tag refusal exercised in test
// (A). Both refusals uphold immutability; this test isolates the no-move-mode
// path.
func TestReleaseTag_PublishedTagImmutability_RefusesMoveVerbs(t *testing.T) {
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)

	// Fixture sanity: v0.2.0 is uncut, so the existing-tag check passes and
	// the parser reaches the move-verb refusal (not the existing-tag refusal
	// covered by test A).
	if tagExists(t, scratch, "v0.2.0") {
		t.Fatalf("fixture sanity: v0.2.0 must be uncut for the no-move-mode test")
	}

	for _, verb := range []string{"--move", "--force", "--move-published-tag"} {
		verb := verb
		t.Run(verb, func(t *testing.T) {
			exitCode, result, _, _ := runCreateFlow(t, wrapper, msgFile, "v0.2.0", []string{verb})
			if exitCode == 0 {
				t.Fatalf("create flow must REFUSE %s (no move mode); got exit 0", verb)
			}
			if result.OK {
				t.Errorf("expected ok=false; got true")
			}
			if result.Error == nil || !strings.Contains(*result.Error, "unknown argument") {
				t.Errorf("error must mention 'unknown argument'; got %v", result.Error)
			}
			if result.Error == nil || !strings.Contains(*result.Error, verb) {
				t.Errorf("error must name the refused verb %s; got %v", verb, result.Error)
			}
			// No tag may be created by a refused move verb.
			if tagExists(t, scratch, "v0.2.0") {
				t.Errorf("tag v0.2.0 must NOT be created by refused verb %s", verb)
			}
		})
	}
}
