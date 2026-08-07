package cli

// Integration tests for the update-command freshness guard
// (internal/cli/update.go). These exercise the WIRED runUpdate path against
// fixtures built from the test binary's REAL embedded corpus (corpus.CoreFS),
// so the comparator compares real embedded bytes vs a real on-disk
// templates/core. They cover: live refuse on differs; --allow-stale-corpus
// override; --dry-run warn+proceed; fresh source checkout proceeds; consumer
// (no corpus.go + templates/core) completely unaffected.
//
// The real-subprocess-binary crux lives in corpus_freshness_crux_test.go; the
// pure-comparator logic lives in corpus_freshness_test.go.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// withAllowStaleCorpus sets the --allow-stale-corpus flag seam for the test
// and restores it on cleanup.
func withAllowStaleCorpus(t *testing.T, v bool) {
	t.Helper()
	saved := updateAllowStaleCorpus
	t.Cleanup(func() { updateAllowStaleCorpus = saved })
	updateAllowStaleCorpus = v
}

// copyEmbeddedCorpusToDisk walks the test binary's REAL embedded corpus
// (coreSubFSImpl — the same fs.FS the renderer and checkCorpusFreshness read)
// and writes every file under abs/templates/core, so the fixture's on-disk
// tree byte-matches the embedded corpus (freshness=fresh). Callers then mutate
// one file to produce a controlled freshness=differs.
func copyEmbeddedCorpusToDisk(t *testing.T, abs string) {
	t.Helper()
	sub, err := coreSubFSImpl()
	if err != nil {
		t.Fatalf("coreSubFSImpl: %v", err)
	}
	err = fs.WalkDir(sub, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, rerr := fs.ReadFile(sub, rel)
		if rerr != nil {
			return rerr
		}
		diskPath := filepath.Join(abs, corpus.CoreDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(diskPath, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy embedded corpus to %s: %v", abs, err)
	}
}

// sourceCheckoutFixture builds a temp source checkout: corpus.go at the root +
// the test binary's real embedded corpus copied to templates/core. Returned
// freshness is `fresh` unless mutate is called afterward.
func sourceCheckoutFixture(t *testing.T) string {
	t.Helper()
	abs := t.TempDir()
	writeCorpusGo(t, abs)
	copyEmbeddedCorpusToDisk(t, abs)
	return abs
}

// mutateEmbeddedFile mutates one known-embedded file under abs/templates/core so
// the on-disk tree differs from the embedded corpus (freshness=differs). Returns
// the mutated disk path.
func mutateEmbeddedFile(t *testing.T, abs, rel string) string {
	t.Helper()
	diskPath := filepath.Join(abs, corpus.CoreDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(diskPath, []byte("// mutated by freshness test\n"), 0o644); err != nil {
		t.Fatalf("mutate %s: %v", rel, err)
	}
	return diskPath
}

// runUpdateTargetNonInteractive runs runUpdate against target with the guard
// seams set to non-TTY / must-not-prompt, so the uninitialized-target guard
// never interferes with the freshness assertions. Returns stdout/stderr buffer
// and the error.
func runUpdateTargetNonInteractive(t *testing.T, target string) (string, error) {
	t.Helper()
	withGuardSeams(t, func() bool { return false }, confirmMustNotFire(t))
	return runUpdateTarget(t, target)
}

// --- live refuse on differs ------------------------------------------------

// TestUpdateFreshness_RefusesLiveWhenCorpusDiffers: a live update against a
// source checkout whose templates/core differs from the embedded corpus MUST
// refuse (non-zero exit) BEFORE any write. The error names `make update`
// (recovery) FIRST and --allow-stale-corpus (override) SECOND, and never
// asserts a direction (stale/newer/older).
func TestUpdateFreshness_RefusesLiveWhenCorpusDiffers(t *testing.T) {
	abs := sourceCheckoutFixture(t)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")

	out, err := runUpdateTargetNonInteractive(t, abs)
	if err == nil {
		t.Fatalf("update must REFUSE when corpus differs; got nil err. out=%q", out)
	}
	emsg := strings.ToLower(err.Error())
	if !strings.Contains(emsg, "refus") {
		t.Errorf("err must say refusing; got %v", err)
	}
	// Recovery (`make update`) appears BEFORE the override in the message.
	mkIdx := strings.Index(emsg, "make update")
	ovIdx := strings.Index(emsg, "allow-stale-corpus")
	if mkIdx < 0 {
		t.Errorf("err must mention `make update` recovery; got %v", err)
	}
	if ovIdx < 0 {
		t.Errorf("err must mention --allow-stale-corpus override; got %v", err)
	}
	if mkIdx >= 0 && ovIdx >= 0 && mkIdx > ovIdx {
		t.Errorf("`make update` must appear BEFORE --allow-stale-corpus in the error; got %v", err)
	}
	// Direction-neutrality.
	for _, bad := range []string{"stale", "newer", "older"} {
		// Allow "stale" only inside the flag name "allow-stale-corpus" and the
		// file path; the corpus-status word must not assert chronology.
		if containsDirectionWord(emsg, bad) {
			t.Errorf("err must not assert direction (%q) outside the flag name; got %v", bad, err)
		}
	}
	// No writes happened (refused before seamApply): .opencode/ must not exist.
	if pathExists(t, filepath.Join(abs, ".opencode")) {
		t.Errorf("refuse must not write .opencode/; out=%q", out)
	}
}

// containsDirectionWord reports whether the staleness result asserted a
// chronology word (stale/newer/older) OUTSIDE the --allow-stale-corpus flag
// name. The flag name legitimately contains "stale"; a bare "embedded corpus
// is stale" would be a direction claim the guard must never make.
func containsDirectionWord(lowered, bad string) bool {
	// Strip the flag name so its "stale" does not count, then look for the word.
	stripped := strings.ReplaceAll(lowered, "allow-stale-corpus", "")
	return strings.Contains(stripped, bad)
}

// --- --allow-stale-corpus override -----------------------------------------

// TestUpdateFreshness_AllowStaleCorpusOverrides: --allow-stale-corpus lets a
// differs-corpus update PROCEED (it warns about the override, then seamApply
// runs and writes). This is the explicit opt-in for operators who understand
// the rendered files will reflect the binary's corpus.
func TestUpdateFreshness_AllowStaleCorpusOverrides(t *testing.T) {
	abs := sourceCheckoutFixture(t)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")

	withAllowStaleCorpus(t, true)
	out, err := runUpdateTargetNonInteractive(t, abs)
	if err != nil {
		t.Fatalf("--allow-stale-corpus must proceed on differs; got %v (out=%q)", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "allow-stale-corpus") {
		t.Errorf("proceed must announce the override; got %q", out)
	}
	// Proceeded: seamApply rendered the live tree.
	if !pathExists(t, filepath.Join(abs, ".opencode")) {
		t.Errorf("--allow-stale-corpus must proceed to write .opencode/; out=%q", out)
	}
}

// --- --allow-stale-corpus + --dry-run combo (no misleading "proceeding") ----

// TestUpdateFreshness_AllowStaleCorpusWithDryRun: when BOTH --allow-stale-corpus
// AND --dry-run are set, the override is honored (no refuse) BUT, because dry-run
// writes nothing, the message MUST NOT claim "proceeding"/"rendered files will
// reflect" the way the live override does. It must clearly state it is a no-write
// dry-run preview. The combo also must write nothing to the tree.
func TestUpdateFreshness_AllowStaleCorpusWithDryRun(t *testing.T) {
	abs := sourceCheckoutFixture(t)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")

	withAllowStaleCorpus(t, true)
	withDryRun(t, true)
	out, err := runUpdateTargetNonInteractive(t, abs)
	if err != nil {
		t.Fatalf("--allow-stale-corpus + --dry-run must not error on differs; got %v (out=%q)", err, out)
	}
	low := strings.ToLower(out)
	// The combo must announce the override is active.
	if !strings.Contains(low, "allow-stale-corpus") {
		t.Errorf("combo must announce --allow-stale-corpus; got %q", out)
	}
	// The combo MUST NOT print the live-override "proceeding ... rendered files
	// will reflect" wording — that claims a render/write that dry-run does not do.
	if strings.Contains(low, "proceeding although") {
		t.Errorf("combo must not claim 'proceeding' (dry-run writes nothing); got %q", out)
	}
	if strings.Contains(low, "rendered files will reflect") {
		t.Errorf("combo must not claim rendered files (dry-run writes nothing); got %q", out)
	}
	// It MUST clearly state no writes occur.
	if !strings.Contains(low, "no files will be written") {
		t.Errorf("combo must state no files will be written (dry-run); got %q", out)
	}
	if !strings.Contains(low, "dry-run") {
		t.Errorf("combo must identify itself as a dry-run preview; got %q", out)
	}
	// The binary-corpus caveat must still be present (the override does not
	// change which corpus the preview represents).
	if !strings.Contains(low, "binary's embedded corpus") {
		t.Errorf("combo must still warn the preview is the BINARY's corpus; got %q", out)
	}
	// Dry-run writes nothing.
	if pathExists(t, filepath.Join(abs, ".opencode")) {
		t.Errorf("combo must not write .opencode/ (dry-run); out=%q", out)
	}
}

// --- --dry-run warn + proceed ----------------------------------------------

// TestUpdateFreshness_DryRunWarnsAndProceeds: --dry-run never refuses (it
// writes nothing); it WARNS prominently and proceeds. The warning must state
// the preview represents the BINARY's embedded corpus, not a rebuilt binary,
// and must name `make update`.
func TestUpdateFreshness_DryRunWarnsAndProceeds(t *testing.T) {
	abs := sourceCheckoutFixture(t)
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")

	withDryRun(t, true)
	out, err := runUpdateTargetNonInteractive(t, abs)
	if err != nil {
		t.Fatalf("dry-run must not error on differs; got %v (out=%q)", err, out)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "warning") {
		t.Errorf("dry-run must warn on differs; got %q", out)
	}
	if !strings.Contains(low, "binary's embedded corpus") {
		t.Errorf("dry-run warning must say the preview is the BINARY's corpus; got %q", out)
	}
	if !strings.Contains(low, "make update") {
		t.Errorf("dry-run warning must point at `make update`; got %q", out)
	}
	// Dry-run writes nothing.
	if pathExists(t, filepath.Join(abs, ".opencode")) {
		t.Errorf("dry-run must not write .opencode/; out=%q", out)
	}
}

// --- fresh source checkout: silent proceed ---------------------------------

// TestUpdateFreshness_FreshSourceCheckoutSilent: when the embedded corpus
// matches the on-disk templates/core, update proceeds with NO staleness signal
// (no warning, no refusal) — the guard is invisible when the binary is current.
func TestUpdateFreshness_FreshSourceCheckoutSilent(t *testing.T) {
	abs := sourceCheckoutFixture(t) // fresh by construction

	out, err := runUpdateTargetNonInteractive(t, abs)
	if err != nil {
		t.Fatalf("fresh source checkout must proceed; got %v (out=%q)", err, out)
	}
	low := strings.ToLower(out)
	for _, bad := range []string{"embedded corpus differs", "allow-stale-corpus", "refusing"} {
		if strings.Contains(low, bad) {
			t.Errorf("fresh source checkout must produce no staleness signal (%q found); got %q", bad, out)
		}
	}
}

// --- consumer: completely unaffected ---------------------------------------

// TestUpdateFreshness_ConsumerUnaffected: a target WITHOUT corpus.go +
// templates/core (every consumer) MUST behave exactly as before — no refuse,
// no warning, the update proceeds and scaffolds managed files. The freshness
// guard is structurally inert for consumers.
func TestUpdateFreshness_ConsumerUnaffected(t *testing.T) {
	abs := t.TempDir() // no corpus.go, no templates/core

	out, err := runUpdateTargetNonInteractive(t, abs)
	if err != nil {
		t.Fatalf("consumer update must proceed (unchanged behavior); got %v (out=%q)", err, out)
	}
	low := strings.ToLower(out)
	for _, bad := range []string{"embedded corpus differs", "allow-stale-corpus", "refusing", "dev-stale"} {
		if strings.Contains(low, bad) {
			t.Errorf("consumer must see no staleness signal (%q found); got %q", bad, out)
		}
	}
	// Current adopt behavior preserved: managed files scaffolded.
	if !pathExists(t, filepath.Join(abs, ".opencode")) {
		t.Errorf("consumer update must scaffold .opencode/ (unchanged behavior); out=%q", out)
	}
}

// TestUpdateFreshness_VendoredTemplatesCoreWithoutCorpusGoIsConsumer: a target
// that vendors templates/core but has NO corpus.go is a CONSUMER for the guard
// (the heuristic requires BOTH markers). Update proceeds without a staleness
// signal — proving templates/core alone does not make a consumer look like a
// source checkout. (Mirrors fixture F6 at the update level.)
func TestUpdateFreshness_VendoredTemplatesCoreWithoutCorpusGoIsConsumer(t *testing.T) {
	abs := t.TempDir()
	// Vendor a templates/core tree but deliberately do NOT add corpus.go.
	copyEmbeddedCorpusToDisk(t, abs)
	// Mutate one vendored file — irrelevant, because the guard never fires
	// without corpus.go.
	mutateEmbeddedFile(t, abs, ".vh-agent-harness/AGENTS.core.md")

	out, err := runUpdateTargetNonInteractive(t, abs)
	if err != nil {
		t.Fatalf("vendored templates/core without corpus.go is a consumer; update must proceed; got %v (out=%q)", err, out)
	}
	low := strings.ToLower(out)
	if strings.Contains(low, "embedded corpus differs") || strings.Contains(low, "refusing") {
		t.Errorf("vendored templates/core without corpus.go must NOT trigger the guard; got %q", out)
	}
}

// --- error result: fail-safe refuse ----------------------------------------

// TestUpdateFreshness_ErrorRefusesSafe: when the comparator cannot read a disk
// file for a reason other than NotExist (here a directory shadows an expected
// file), the result is freshnessError and a LIVE update fail-safe REFUSES
// (never silently proceeds). --dry-run would warn; --allow-stale-corpus would
// override (covered by the differs tests above via the same branch).
func TestUpdateFreshness_ErrorRefusesSafe(t *testing.T) {
	abs := sourceCheckoutFixture(t)
	// Shadow an embedded file path with a DIRECTORY so os.ReadFile fails with
	// a non-IsNotExist error (EISDIR), exercising the freshnessError branch.
	shadowRel := ".vh-agent-harness/AGENTS.core.md"
	shadowPath := filepath.Join(abs, corpus.CoreDir, filepath.FromSlash(shadowRel))
	if err := os.RemoveAll(shadowPath); err != nil {
		t.Fatalf("rm shadow: %v", err)
	}
	if err := os.MkdirAll(shadowPath, 0o755); err != nil {
		t.Fatalf("mkdir shadow dir: %v", err)
	}

	out, err := runUpdateTargetNonInteractive(t, abs)
	if err == nil {
		t.Fatalf("freshness error must fail-safe REFUSE a live update; got nil err. out=%q", out)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "refus") {
		t.Errorf("freshness error refuse must say refusing; got %v", err)
	}
	// No writes.
	if pathExists(t, filepath.Join(abs, ".opencode")) {
		t.Errorf("freshness error refuse must not write .opencode/; out=%q", out)
	}
}
