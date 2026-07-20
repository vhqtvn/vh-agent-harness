package cli

// Push-only mode tests for scripts/release-tag.sh --push-only.
//
// --push-only is the sanctioned path for pushing an already-cut local
// annotated tag to origin, removing the need for agents to fall back to
// raw `git push` (which is forbidden by the git-mutation-bypass rule).
// It bypasses the tag-existence refusal (which protects re-running the
// create flow), the DEFER gate (already passed at tag-creation time),
// and the `git tag -a` mutation (the tag already exists). When set, the
// tag MUST exist locally — push-only is not a creation path.
//
// These tests reuse the shared setupReleaseTagRepo helper (no manifest
// required — push-only bypasses the DEFER gate entirely) and add a local
// bare repo as the "origin" remote to avoid touching any real remote.
// The push-only path does not require RELEASE_TAG_MESSAGE_FILE; the
// runner below intentionally omits it to prove the create-flow-only
// validation is skipped.

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// addBareRemote creates a bare git repo at a fresh temp path and registers
// it as "origin" in the scratch repo. Returns the remote path. Pushing to
// a bare local repo is the standard fixture for testing `git push` without
// touching any real remote.
func addBareRemote(t *testing.T, scratch string) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	rcmd := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	rcmd("init", "--bare", "-q", remote)
	rcmd("-C", scratch, "remote", "add", "origin", remote)
	return remote
}

// remoteTagList returns the list of tags recorded in the bare remote.
func remoteTagList(t *testing.T, remote string) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", remote, "tag", "-l").Output()
	if err != nil {
		t.Fatalf("git -C remote tag -l: %v", err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

// runPushOnly invokes the wrapper with --push-only from the scratch repo
// root (the wrapper references .opencode/scripts/... and
// .vh-agent-harness/... by relative path). RELEASE_TAG_MESSAGE_FILE is
// intentionally NOT set — push-only does not create a tag.
func runPushOnly(t *testing.T, wrapper, version string, extraArgs []string) (int, releaseTagManifestResult, string, string) {
	t.Helper()
	args := []string{wrapper, version, "--push-only"}
	args = append(args, extraArgs...)
	cmd := exec.Command("bash", args...)
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper)) // <scratch>
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

// cutAnnotatedTag creates an annotated tag at HEAD using git directly. This
// is the test fixture for "an already-cut local tag" — it bypasses the
// wrapper to simulate a prior create-only invocation.
func cutAnnotatedTag(t *testing.T, scratch, tag, msgFile string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", scratch, "tag", "-a", "-F", msgFile, tag).CombinedOutput(); err != nil {
		t.Fatalf("git tag -a %s: %v\n%s", tag, err, out)
	}
}

// tagTargetSHA returns the commit SHA the given tag points at (dereferenced
// through the tag object for annotated tags).
func tagTargetSHA(t *testing.T, scratch, tag string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", scratch, "rev-parse", "refs/tags/"+tag+"^{commit}").Output()
	if err != nil {
		t.Fatalf("git rev-parse refs/tags/%s^{commit}: %v", tag, err)
	}
	return strings.TrimSpace(string(out))
}

// tagObjectSHA returns the SHA of the tag object itself (annotated tags) or
// the commit SHA (lightweight tags). Used to prove push-only does NOT
// modify or recreate the existing tag object.
func tagObjectSHA(t *testing.T, scratch, tag string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", scratch, "rev-parse", "refs/tags/"+tag).Output()
	if err != nil {
		t.Fatalf("git rev-parse refs/tags/%s: %v", tag, err)
	}
	return strings.TrimSpace(string(out))
}

// =============================================================================
// HAPPY PATH — push-only succeeds when the tag exists
// =============================================================================

// TestReleaseTag_PushOnly_SucceedsWhenTagExists — the canonical push-only
// flow: an annotated tag was already cut locally (by a prior create-only
// invocation); push-only pushes it to origin, emits the success JSON with
// pushed=true, the JSON `commit` equals the tag's target (NOT current HEAD),
// and the local tag object is left byte-identical (no recreation).
//
// Also proves: (a) push-only succeeds, (c) push-only does NOT create a new
// tag (tag object SHA unchanged), and that RELEASE_TAG_MESSAGE_FILE is NOT
// required (the runner omits it).
func TestReleaseTag_PushOnly_SucceedsWhenTagExists(t *testing.T) {
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)
	remote := addBareRemote(t, scratch)

	// Cut v0.2.0 at HEAD (the test setup leaves v0.2.0 uncut).
	cutAnnotatedTag(t, scratch, "v0.2.0", msgFile)
	targetSHA := tagTargetSHA(t, scratch, "v0.2.0")
	tagObjSHA := tagObjectSHA(t, scratch, "v0.2.0")

	exitCode, result, _, _ := runPushOnly(t, wrapper, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("push-only must succeed (exit 0); got %d (ok=%v err=%v)",
			exitCode, result.OK, result.Error)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !result.Pushed {
		t.Errorf("expected pushed=true; got false")
	}
	if result.Tag == nil || *result.Tag != "v0.2.0" {
		t.Errorf("tag must be v0.2.0; got %v", result.Tag)
	}
	// JSON `commit` must equal the tag's target commit, NOT the current HEAD.
	// (For this fixture the tag is cut at HEAD, so target == HEAD; the
	// important guarantee is that the wrapper emits the tag's dereferenced
	// commit, not a fresh `git rev-parse HEAD`.)
	if result.Commit == nil || *result.Commit != targetSHA {
		t.Errorf("commit must equal tag's target %s; got %v", targetSHA, result.Commit)
	}
	// disclosures + accepted_overrides must be null (DEFER gate skipped).
	if result.Disclosures != nil {
		t.Errorf("disclosures must be null in push-only mode; got %v", result.Disclosures)
	}
	if result.AcceptedOverrides != nil {
		t.Errorf("accepted_overrides must be null in push-only mode; got %v", result.AcceptedOverrides)
	}

	// The tag must have been pushed to the remote.
	tags := remoteTagList(t, remote)
	if len(tags) != 1 || tags[0] != "v0.2.0" {
		t.Errorf("remote must have exactly [v0.2.0]; got %v", tags)
	}

	// Push-only must NOT modify or recreate the local tag object.
	if after := tagObjectSHA(t, scratch, "v0.2.0"); after != tagObjSHA {
		t.Errorf("push-only must not modify the tag object; before=%s after=%s", tagObjSHA, after)
	}
}

// =============================================================================
// REFUSAL — push-only errors cleanly when the tag is missing
// =============================================================================

// TestReleaseTag_PushOnly_ErrsWhenTagMissing — push-only is not a creation
// path: if the requested tag does not exist locally, the wrapper refuses
// cleanly with an error mentioning "does not exist". No remote push is
// attempted and no local tag is created.
func TestReleaseTag_PushOnly_ErrsWhenTagMissing(t *testing.T) {
	scratch, wrapper, _, _ := setupReleaseTagRepo(t)
	remote := addBareRemote(t, scratch)

	// v0.2.0 does NOT exist in the setup (only v0.1.0 does).
	exitCode, result, _, _ := runPushOnly(t, wrapper, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("push-only must REFUSE when tag is missing; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if result.Pushed {
		t.Errorf("expected pushed=false; got true")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "does not exist") {
		t.Errorf("error must mention 'does not exist'; got %v", result.Error)
	}
	if result.Error == nil || !strings.Contains(*result.Error, "v0.2.0") {
		t.Errorf("error must name the missing tag v0.2.0; got %v", result.Error)
	}

	// No tag should have been created locally.
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag v0.2.0 must NOT be created by push-only")
	}
	// Remote must have ZERO tags — push path not reached.
	if tags := remoteTagList(t, remote); tags != nil {
		t.Errorf("remote must have NO tags (push path not reached); got %v", tags)
	}
}

// =============================================================================
// DEFER gate bypass — push-only does NOT re-run the gate
// =============================================================================

// TestReleaseTag_PushOnly_DoesNotRunDeferGate — push-only must skip the
// DEFER gate entirely, because the tag already passed the gate at creation
// time. We prove this by setting up a manifest that WOULD block if the
// gate ran (yes+block+valid), manually cutting the tag (simulating a prior
// create-only invocation that ran before the blocker was committed to the
// manifest), then invoking push-only — which must succeed despite the
// blocking manifest. If the gate ran, it would refuse with classification
// `blocker` and the push would never happen.
func TestReleaseTag_PushOnly_DoesNotRunDeferGate(t *testing.T) {
	scratch, wrapper, _, _, _ := setupReleaseTagManifestRepo(t, manifestSpec{
		ReleaseBaseKind:     "tag",
		ReleaseBaseValue:    "v0.1.0",
		ReconciliationScope: "release arc from v0.1.0 through evaluated_commit",
		Records: []manifestRecordSpec{{
			DeferID:          "defer-block-push-only",
			ReleaseRelevance: "yes",
			Disposition:      "block",
			MetadataState:    "valid",
			Summary:          "release-block (would refuse if DEFER gate ran)",
			Reason:           "hard block",
			SourceRef:        ".local/x",
			StudiedAt:        "2026-07-15T00:00:00Z",
			ReviewedAt:       "2026-07-20T00:00:00Z",
		}},
	})
	remote := addBareRemote(t, scratch)

	// Manually cut v0.2.0 at HEAD (simulating a prior create-only run that
	// passed the DEFER gate at an earlier manifest state).
	msgFile := filepath.Join(scratch, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("release v0.2.0\n\n-test\n"), 0o644); err != nil {
		t.Fatalf("write msg: %v", err)
	}
	cutAnnotatedTag(t, scratch, "v0.2.0", msgFile)

	exitCode, result, _, _ := runPushOnly(t, wrapper, "v0.2.0", nil)
	if exitCode != 0 {
		t.Fatalf("push-only must succeed despite blocker manifest (DEFER gate skipped); got exit %d (ok=%v err=%v)",
			exitCode, result.OK, result.Error)
	}
	if !result.OK {
		t.Errorf("expected ok=true; got false (error=%v)", result.Error)
	}
	if !result.Pushed {
		t.Errorf("expected pushed=true")
	}

	// The tag must have been pushed to the remote (proves the DEFER gate
	// did not refuse and the push path was reached).
	tags := remoteTagList(t, remote)
	if len(tags) != 1 || tags[0] != "v0.2.0" {
		t.Errorf("remote must have exactly [v0.2.0] (DEFER gate skipped); got %v", tags)
	}
}

// =============================================================================
// GUARD RAIL — push-only refuses to combine with the override ceremony
// =============================================================================

// TestReleaseTag_PushOnly_RefusesCombinedWithOverride — the override
// ceremony is meaningless in push-only mode (the DEFER gate is skipped
// entirely, so there is nothing to override). Combining them refuses
// BEFORE any push attempt.
func TestReleaseTag_PushOnly_RefusesCombinedWithOverride(t *testing.T) {
	scratch, wrapper, _, msgFile := setupReleaseTagRepo(t)
	remote := addBareRemote(t, scratch)

	cutAnnotatedTag(t, scratch, "v0.2.0", msgFile)

	exitCode, result, _, _ := runPushOnly(t, wrapper, "v0.2.0",
		[]string{"--override-release-version", "v0.2.0", "--override-manifest-sha", strings.Repeat("a", 40)})
	if exitCode == 0 {
		t.Fatalf("push-only + override must REFUSE; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if result.Pushed {
		t.Errorf("expected pushed=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "cannot be combined") {
		t.Errorf("error must mention 'cannot be combined'; got %v", result.Error)
	}

	// Remote must have ZERO tags — push path not reached.
	if tags := remoteTagList(t, remote); tags != nil {
		t.Errorf("remote must have NO tags (push path not reached); got %v", tags)
	}
}

// =============================================================================
// DEFAULT FLOW UNCHANGED — --push-only is opt-in only
// =============================================================================

// TestReleaseTag_PushOnly_DefaultFlowStillRequiresMessageFile — a sanity
// guard that the default flow (no --push-only) still refuses when
// RELEASE_TAG_MESSAGE_FILE is missing. This pins backward compatibility:
// adding --push-only did not relax the create-flow validation.
func TestReleaseTag_PushOnly_DefaultFlowStillRequiresMessageFile(t *testing.T) {
	scratch, wrapper, _, _ := setupReleaseTagRepo(t)
	remote := addBareRemote(t, scratch)

	// Default flow, NO RELEASE_TAG_MESSAGE_FILE, NO --push-only.
	cmd := exec.Command("bash", wrapper, "v0.2.0")
	cmd.Dir = filepath.Dir(filepath.Dir(wrapper))
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("bash spawn error: %v\n%s", err, out)
		}
	}
	if exitCode == 0 {
		t.Fatalf("default flow without RELEASE_TAG_MESSAGE_FILE must REFUSE; got exit 0\n%s", out)
	}
	var result releaseTagManifestResult
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("default-flow output must be valid JSON: %v\n%s", err, out)
	}
	if result.OK {
		t.Errorf("expected ok=false")
	}
	if result.Error == nil || !strings.Contains(*result.Error, "RELEASE_TAG_MESSAGE_FILE") {
		t.Errorf("error must mention RELEASE_TAG_MESSAGE_FILE; got %v", result.Error)
	}
	if tagExists(t, scratch, "v0.2.0") {
		t.Errorf("tag must NOT be created")
	}
	if tags := remoteTagList(t, remote); tags != nil {
		t.Errorf("remote must have NO tags; got %v", tags)
	}
}

// =============================================================================
// REFUSAL — push-only refuses a lightweight (non-annotated) tag
// =============================================================================

// TestReleaseTag_PushOnly_RefusesLightweightTag — push-only trusts that the
// existing tag is an ANNOTATED tag object created by the full ceremony (which
// already passed the DEFER gate). A lightweight tag (`git tag <v>` with no
// `-a`) has no tag object and never passed the ceremony — pushing it would
// trip release.yml and defeat the annotated-tag invariant the wrapper exists
// to enforce. The catch: `git rev-parse --verify refs/tags/<v>^{commit}`
// resolves for BOTH tag object types, so the existence check alone is
// insufficient; the wrapper performs a separate `git cat-file -t` check to
// distinguish "tag" (annotated) from "commit" (lightweight).
//
// This test creates a lightweight tag and confirms push-only refuses BEFORE
// any push, leaves the bare remote untouched, and leaves the local lightweight
// tag unchanged (still present, still lightweight — no silent promotion).
func TestReleaseTag_PushOnly_RefusesLightweightTag(t *testing.T) {
	scratch, wrapper, _, _ := setupReleaseTagRepo(t)
	remote := addBareRemote(t, scratch)

	// Create a LIGHTWEIGHT tag (no -a, no -m). This is the bug condition:
	// `git rev-parse --verify refs/tags/v0.2.0^{commit}` succeeds for it, so
	// the existence check alone would let it reach `git push`.
	if out, err := exec.Command("git", "-C", scratch, "tag", "v0.2.0").CombinedOutput(); err != nil {
		t.Fatalf("git tag v0.2.0 (lightweight): %v\n%s", err, out)
	}
	// Fixture sanity: confirm v0.2.0 is in fact lightweight (cat-file -t
	// reports "commit", not "tag"). This pins the regression: if the setup
	// ever started producing annotated tags, the refusal under test would be
	// exercising the wrong path.
	if out, err := exec.Command("git", "-C", scratch, "cat-file", "-t", "refs/tags/v0.2.0").Output(); err != nil {
		t.Fatalf("git cat-file -t (pre): %v", err)
	} else if got := strings.TrimSpace(string(out)); got != "commit" {
		t.Fatalf("fixture sanity: v0.2.0 must be lightweight (cat-file -t == \"commit\"); got %q", got)
	}

	exitCode, result, _, _ := runPushOnly(t, wrapper, "v0.2.0", nil)
	if exitCode == 0 {
		t.Fatalf("push-only must REFUSE a lightweight tag; got exit 0")
	}
	if result.OK {
		t.Errorf("expected ok=false; got true")
	}
	if result.Pushed {
		t.Errorf("expected pushed=false; got true")
	}
	// The wrapper's message names both ("annotated tag object" and "got
	// lightweight tag"), so accept either keyword (prefix-match is fine per
	// the test contract).
	if result.Error == nil {
		t.Errorf("error must be non-nil")
	} else if !strings.Contains(*result.Error, "annotated") && !strings.Contains(*result.Error, "lightweight") {
		t.Errorf("error must mention 'annotated' or 'lightweight'; got %q", *result.Error)
	}
	// Refusal path emits an empty commit string (no SHA captured for output).
	if result.Commit != nil && *result.Commit != "" {
		t.Errorf("commit must be empty on refusal; got %q", *result.Commit)
	}
	// disclosures + accepted_overrides must be null (push-only contract).
	if result.Disclosures != nil {
		t.Errorf("disclosures must be null in push-only mode; got %v", result.Disclosures)
	}
	if result.AcceptedOverrides != nil {
		t.Errorf("accepted_overrides must be null in push-only mode; got %v", result.AcceptedOverrides)
	}

	// The bare remote must NOT have the tag — push path must not be reached.
	if tags := remoteTagList(t, remote); tags != nil {
		t.Errorf("remote must have NO tags (push path not reached); got %v", tags)
	}

	// The local lightweight tag must be unchanged: still present locally and
	// still lightweight (push-only must not silently promote it to annotated).
	if !tagExists(t, scratch, "v0.2.0") {
		t.Errorf("local tag v0.2.0 must still exist (push-only must not delete it)")
	}
	if out, err := exec.Command("git", "-C", scratch, "cat-file", "-t", "refs/tags/v0.2.0").Output(); err != nil {
		t.Fatalf("git cat-file -t (post): %v", err)
	} else if got := strings.TrimSpace(string(out)); got != "commit" {
		t.Errorf("local tag must still be lightweight (cat-file -t == \"commit\"); got %q", got)
	}
}
