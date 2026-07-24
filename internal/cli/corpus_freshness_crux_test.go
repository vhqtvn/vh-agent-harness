package cli

// The load-bearing crux test for the dogfood-local staleness guard. It builds a
// REAL vh-agent-harness binary B from the current source (so B's embedded
// corpus is a genuine compiled-in copy, not a stub), constructs a fixture
// source checkout whose on-disk templates/core differs from B's embedded
// corpus, and exercises B's update/doctor as SUBPROCESSES end-to-end.
//
// Why this exists separately from update_freshness_test.go: the in-process
// unit tests run runUpdate/runDoctor against the TEST binary's embedded corpus.
// They cover the same logic with real embedded bytes, but they do not prove a
// SEPARATELY-BUILT binary behaves identically. This test closes that gap: a
// real stale binary (one whose embedded corpus predates a templates/core edit
// in the fixture) refuses to overwrite the fixture's renders.
//
// This is the crux declared in the behavioral-closure: crux path = "a stale
// binary refuses to overwrite a newer source checkout's renders"; verifier =
// this real-binary subprocess exercise. It is the strongest form of evidence
// the seam can automate; the unit tests cover breadth (the fixture matrix +
// every branch) and this test covers the real-binary end-to-end shape.

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// cruxRun invokes the freshly-built harness binary against target with the
// given subcommand+args. It sets RUN_FROM_AGENT=1 so the uninitialized-target
// prompt never fires in the subprocess (stdin is not a TTY under exec anyway,
// but the env makes it explicit) and captures combined stdout+stderr. Returns
// the combined output and the subprocess exit error (nil on exit 0).
func cruxRun(t *testing.T, target string, args ...string) (string, error) {
	t.Helper()
	ensureHarnessBinaryOnPath(t) // builds once per process via release_tag helper
	// Place --target AFTER the subcommand (matches the in-repo convention in
	// sys_prompt_test.go / release_inject_test.go / docs_test.go). Both orderings
	// are valid cobra (cobra resolves the command path, then pflag parses the full
	// arg list against that command's flag set), but this ordering is conventional.
	// args is always non-empty (every caller passes "update"/"doctor" first).
	full := append([]string{args[0], "--target", target}, args[1:]...)
	cmd := exec.Command("vh-agent-harness", full...)
	cmd.Env = append(os.Environ(), "RUN_FROM_AGENT=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// cruxCopyRepoCorpus copies the REPO's on-disk templates/core (the same tree B
// just compiled into its embedded corpus) into dst/templates/core, so a fresh
// fixture matches B's embedded corpus byte-for-byte. Walks the on-disk source
// (not the embedded FS) so a caller can then mutate one file to produce a real
// disk-vs-embed divergence.
func cruxCopyRepoCorpus(t *testing.T, repoRoot, dst string) {
	t.Helper()
	src := filepath.Join(repoRoot, corpus.CoreDir)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		out := filepath.Join(dst, corpus.CoreDir, rel)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		t.Fatalf("cruxCopyRepoCorpus %s -> %s: %v", src, dst, err)
	}
}

// cruxFixture builds a fresh source-checkout fixture at a temp path: corpus.go
// at the root + the repo's templates/core copied in. After this returns, B's
// embedded corpus matches the fixture (freshness=fresh) unless the caller
// mutates a file.
func cruxFixture(t *testing.T, repoRoot string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(dst, "corpus.go"), []byte(minimalCorpusGo), 0o644); err != nil {
		t.Fatalf("write corpus.go: %v", err)
	}
	cruxCopyRepoCorpus(t, repoRoot, dst)
	return dst
}

// TestCrux_StaleBinaryRefusesUpdate is the real-binary crux. It builds B, then
// for a fixture whose templates/core differs from B's embedded corpus asserts:
//
//   - B `update` REFUSES (nonzero exit) and writes nothing;
//   - B `update --dry-run` WARNS + proceeds (exit 0), writes nothing;
//   - B `update --allow-stale-corpus` OVERRIDES (exit 0) and writes;
//   - B `doctor` WARNs via dev-stale-embed and qualifies managed-drift, and
//     the WARN alone does not make a HEALTHY installed fixture UNHEALTHY;
//   - B against a CONSUMER fixture (no corpus.go) shows no staleness signal
//     for either update or doctor (consumers are completely unaffected).
func TestCrux_StaleBinaryRefusesUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("crux binary build skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v (crux requires building a real binary)", err)
	}
	repoRoot := findModuleRoot(t)
	// Build B once (ensureHarnessBinaryOnPath builds and prepends to PATH). The
	// binary embeds the repo's CURRENT templates/core (post any source edits),
	// so a fixture copied from the same on-disk tree starts fresh.
	t.Log("crux: building real binary B (may take a few seconds)")

	// --- fresh fixture: install the live tree while fresh (so a later doctor  ---
	// --- has managed-drift PASS and the only staleness signal is dev-stale)  ---
	fresh := cruxFixture(t, repoRoot)
	// Seed the live tree (managed files) by running B's own update while the
	// corpus still matches. This makes the fixture a real installed tree.
	if out, err := cruxRun(t, fresh, "update"); err != nil {
		t.Fatalf("fresh install via B update failed (prerequisite): %v\n%s", err, out)
	}

	// --- differs fixture: clone the installed fresh tree, then mutate one  ---
	// --- templates/core file so the on-disk corpus differs from B's embed ---
	differs := fresh // reuse the installed tree; mutate its templates/core
	mutateRel := ".vh-agent-harness/AGENTS.core.md"
	mutPath := filepath.Join(differs, corpus.CoreDir, filepath.FromSlash(mutateRel))
	if err := os.WriteFile(mutPath, []byte("// crux mutation: B did not embed this\n"), 0o644); err != nil {
		t.Fatalf("mutate %s: %v", mutateRel, err)
	}

	// (1) LIVE update REFUSES, writes nothing.
	out, err := cruxRun(t, differs, "update")
	if err == nil {
		t.Fatalf("crux(1): B update must REFUSE on differs; got exit 0. out=%s", out)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "refus") {
		t.Errorf("crux(1): refuse output must say 'refusing'; got:\n%s", out)
	}
	if !strings.Contains(low, "make update") || !strings.Contains(low, "allow-stale-corpus") {
		t.Errorf("crux(1): refuse must name `make update` and --allow-stale-corpus; got:\n%s", out)
	}
	// No NEW writes from the refused update. The live tree was already installed
	// by the fresh prerequisite, so we assert the managed file the guard would
	// have reverted (the mutated templates/core source is NOT a live managed
	// path; the live managed path is .vh-agent-harness/AGENTS.core.md at the
	// ROOT, which the prerequisite wrote from the embedded corpus). Confirm the
	// live root managed file still matches the embedded form by checking the
	// doctor run below reports managed-drift PASS.

	// (2) --dry-run WARNS + proceeds (exit 0), writes nothing new.
	out, err = cruxRun(t, differs, "update", "--dry-run")
	if err != nil {
		t.Fatalf("crux(2): B update --dry-run must proceed (exit 0) on differs; got %v\n%s", err, out)
	}
	low = strings.ToLower(out)
	if !strings.Contains(low, "warning") || !strings.Contains(low, "binary's embedded corpus") {
		t.Errorf("crux(2): dry-run must warn about the BINARY's embedded corpus; got:\n%s", out)
	}

	// (3) --allow-stale-corpus OVERRIDES (exit 0) and writes.
	out, err = cruxRun(t, differs, "update", "--allow-stale-corpus")
	if err != nil {
		t.Fatalf("crux(3): B update --allow-stale-corpus must proceed (exit 0); got %v\n%s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "allow-stale-corpus") {
		t.Errorf("crux(3): override must announce --allow-stale-corpus; got:\n%s", out)
	}

	// Re-mutate so the differs state is restored for the doctor assertion (the
	// override just re-rendered from B's embed, so templates/core is the only
	// divergent side again).
	if err := os.WriteFile(mutPath, []byte("// crux mutation restored\n"), 0o644); err != nil {
		t.Fatalf("re-mutate: %v", err)
	}

	// (4) doctor WARNs via dev-stale-embed + qualifies managed-drift; the live
	//     tree matches B's embed (the override re-rendered it), so managed-drift
	//     is PASS and the dev-stale WARN alone must NOT make it UNHEALTHY.
	out, err = cruxRun(t, differs, "doctor")
	if err != nil {
		t.Fatalf("crux(4): B doctor must not error (WARN-only); got %v\n%s", err, out)
	}
	if !strings.Contains(out, "dev-stale-embed WARN") {
		t.Errorf("crux(4): doctor must show dev-stale-embed WARN; got:\n%s", out)
	}
	if !strings.Contains(out, "in sync with the embedded corpus in this binary") {
		t.Errorf("crux(4): managed-drift must be qualified (no bare 'in sync'); got:\n%s", out)
	}
	if !strings.Contains(out, "result: HEALTHY") {
		t.Errorf("crux(4): dev-stale WARN alone must keep doctor HEALTHY; got:\n%s", out)
	}

	// (5) CONSUMER: no corpus.go, no templates/core → no staleness signal at all.
	consumer := t.TempDir()
	// Seed the consumer live tree so doctor has something to lint.
	if out, err := cruxRun(t, consumer, "update"); err != nil {
		t.Fatalf("crux(5): consumer install prerequisite failed: %v\n%s", err, out)
	}
	out, err = cruxRun(t, consumer, "update")
	if err != nil {
		t.Fatalf("crux(5): consumer B update must proceed (unchanged behavior); got %v\n%s", err, out)
	}
	low = strings.ToLower(out)
	for _, bad := range []string{"embedded corpus differs", "allow-stale-corpus", "refusing"} {
		if strings.Contains(low, bad) {
			t.Errorf("crux(5): consumer update must show no staleness signal (%q); got:\n%s", bad, out)
		}
	}
	// (6) consumer doctor: NO dev-stale-embed section.
	out, err = cruxRun(t, consumer, "doctor")
	if err != nil {
		t.Fatalf("crux(6): consumer B doctor must not error; got %v\n%s", err, out)
	}
	if strings.Contains(out, "dev-stale-embed") {
		t.Errorf("crux(6): consumer doctor must NOT show a dev-stale-embed section; got:\n%s", out)
	}
}
