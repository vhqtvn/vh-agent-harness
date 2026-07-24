package cli

// Tests for the dogfood-local staleness guard (internal/cli/corpus_freshness.go).
//
// Two units under test:
//
//  1. isSourceCheckout — the identity HEURISTIC (fire condition). Table-driven
//     over the fixture matrix F1-F11 plus the acknowledged limits (symlink
//     policy, absent corpus.go, absent templates/core, resolved --target vs
//     CWD).
//
//  2. compareCorpus — the embedded-manifest-driven comparator. Uses a synthetic
//     fstest.MapFS as the embedded manifest so the comparator logic is
//     exercisable independently of the real embedded corpus: matching corpora,
//     modified file, embedded-path-missing-on-disk, disk-only-file (the
//     IGNORE policy), renamed/removed file, and the fail-safe error path.
//
// The wired entry point (checkCorpusFreshness) and the end-to-end update/
// doctor behaviors are covered by update_freshness_test.go,
// doctor_freshness_test.go, and the real-binary corpus_freshness_crux_test.go.

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"testing/fstest"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// minimalCorpusGo is a corpus.go body that imports only "embed" and carries the
// `//go:embed all:templates/core` directive — module-path-independent, so a
// fork under any module path still matches the heuristic. Used by the positive
// fixtures (F1-F4, F9, F11).
const minimalCorpusGo = `package corpus

import "embed"

//go:embed all:templates/core
var CoreFS embed.FS
`

// writeCorpusGo writes minimalCorpusGo at abs/corpus.go.
func writeCorpusGo(t *testing.T, abs string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(abs, "corpus.go"), []byte(minimalCorpusGo), 0o644); err != nil {
		t.Fatalf("write corpus.go: %v", err)
	}
}

// writeDiskCorpusFile writes one file under abs/templates/core/<rel>.
func writeDiskCorpusFile(t *testing.T, abs, rel, body string) {
	t.Helper()
	diskPath := filepath.Join(abs, corpus.CoreDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(rel), err)
	}
	if err := os.WriteFile(diskPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// mapFS builds a fstest.MapFS from a map of forward-slash rel -> body, for the
// embedded-manifest side of compareCorpus.
func mapFS(files map[string]string) fs.FS {
	m := fstest.MapFS{}
	for path, body := range files {
		m[path] = &fstest.MapFile{Data: []byte(body)}
	}
	return m
}

// --- identity heuristic (F1-F11 + limits) ----------------------------------

// TestIsSourceCheckout_FixtureMatrix exercises the identity heuristic over the
// 11-fixture matrix. Positive cases return true (the guard fires); negative
// cases return false (consumer — guard stays inert).
func TestIsSourceCheckout_FixtureMatrix(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T) string // returns abs
		wantPos bool
	}{
		// F1 fresh clone — corpus.go + templates/core at the root.
		{
			name: "F1 fresh clone (corpus.go + templates/core)",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				writeCorpusGo(t, abs)
				writeDiskCorpusFile(t, abs, ".vh-agent-harness/AGENTS.core.md", "x")
				return abs
			},
			wantPos: true,
		},
		// F2 worktree — a git worktree of the source is structurally identical
		// (corpus.go + templates/core at its root).
		{
			name: "F2 worktree",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				writeCorpusGo(t, abs)
				writeDiskCorpusFile(t, abs, "opencode.jsonc.tmpl", "x")
				return abs
			},
			wantPos: true,
		},
		// F3 fork-same-module — corpus.go present; module path is irrelevant to
		// the heuristic (it does not parse imports).
		{
			name: "F3 fork same module",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				writeCorpusGo(t, abs)
				writeDiskCorpusFile(t, abs, "a/b.txt", "x")
				return abs
			},
			wantPos: true,
		},
		// F4 fork-renamed-module — corpus.go imports only "embed" and is
		// module-path-independent, so a fork under a different module still
		// matches. The heuristic does not read the module path.
		{
			name: "F4 fork renamed module (corpus.go imports only embed)",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				// A corpus.go whose package/module differs; still matches.
				body := "package corpus\n\nimport \"embed\"\n\n//go:embed all:templates/core\nvar CoreFS embed.FS\n"
				if err := os.WriteFile(filepath.Join(abs, "corpus.go"), []byte(body), 0o644); err != nil {
					t.Fatalf("write corpus.go: %v", err)
				}
				writeDiskCorpusFile(t, abs, "deeply/nested/file.md", "x")
				return abs
			},
			wantPos: true,
		},
		// F5 normal consumer — no corpus.go, no templates/core.
		{
			name: "F5 normal consumer (no corpus.go, no templates/core)",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				// Write some consumer files that are NOT the source markers.
				if err := os.WriteFile(filepath.Join(abs, "README.md"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return abs
			},
			wantPos: false,
		},
		// F6 vendored templates/core WITHOUT corpus.go — proves templates/core
		// alone is too weak to fire (the heuristic requires BOTH markers). This
		// is the decisive "templates/core alone does not make a consumer look
		// like a source checkout" guard against false positives.
		{
			name: "F6 vendored templates/core but no corpus.go (negative)",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				writeDiskCorpusFile(t, abs, "some/file.md", "x") // templates/core exists
				// Deliberately NO corpus.go.
				return abs
			},
			wantPos: false,
		},
		// F7 non-harness repo — unrelated files only.
		{
			name: "F7 non-harness repo",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				if err := os.WriteFile(filepath.Join(abs, "main.go"), []byte("package main\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return abs
			},
			wantPos: false,
		},
		// F8 temp/staging dir — empty.
		{
			name: "F8 temp/staging (empty)",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			wantPos: false,
		},
		// F9 partial — go.mod missing/corrupt, but corpus.go + templates/core
		// present → still a source checkout. The heuristic does not require
		// go.mod.
		{
			name: "F9 partial (no go.mod, corpus.go + templates/core present)",
			setup: func(t *testing.T) string {
				abs := t.TempDir()
				writeCorpusGo(t, abs)
				writeDiskCorpusFile(t, abs, "x.txt", "x")
				// No go.mod; a corrupt go.mod would be irrelevant too.
				return abs
			},
			wantPos: true,
		},
		// F11 source as a submodule — at its own root it still has both markers.
		{
			name: "F11 source as submodule",
			setup: func(t *testing.T) string {
				// The submodule lives under a parent repo dir but at its own
				// root carries corpus.go + templates/core like any source
				// checkout.
				parent := t.TempDir()
				submod := filepath.Join(parent, "vendor", "vh-agent-harness")
				if err := os.MkdirAll(submod, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(submod, "corpus.go"), []byte(minimalCorpusGo), 0o644); err != nil {
					t.Fatal(err)
				}
				writeDiskCorpusFile(t, submod, ".opencode/agents/build.md", "x")
				return submod
			},
			wantPos: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			abs := tc.setup(t)
			got := isSourceCheckout(abs)
			if got != tc.wantPos {
				t.Errorf("isSourceCheckout(%s) = %v, want %v", abs, got, tc.wantPos)
			}
		})
	}
}

// TestIsSourceCheckout_AbsentCorpusGo — the negative half of the fire
// condition: templates/core present but corpus.go absent → not a source
// checkout (mirrors F6, stated standalone for clarity).
func TestIsSourceCheckout_AbsentCorpusGo(t *testing.T) {
	abs := t.TempDir()
	writeDiskCorpusFile(t, abs, "a.txt", "x")
	if isSourceCheckout(abs) {
		t.Errorf("templates/core without corpus.go must NOT be a source checkout")
	}
}

// TestIsSourceCheckout_AbsentTemplatesCore — corpus.go present but no
// templates/core dir → not a source checkout.
func TestIsSourceCheckout_AbsentTemplatesCore(t *testing.T) {
	abs := t.TempDir()
	writeCorpusGo(t, abs)
	if isSourceCheckout(abs) {
		t.Errorf("corpus.go without templates/core must NOT be a source checkout")
	}
}

// TestIsSourceCheckout_CorpusGoIsDir — corpus.go present but as a DIRECTORY
// (not a regular file) → not a source checkout. Guards against a pathological
// tree where "corpus.go" is a directory name.
func TestIsSourceCheckout_CorpusGoIsDir(t *testing.T) {
	abs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(abs, "corpus.go"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDiskCorpusFile(t, abs, "a.txt", "x")
	if isSourceCheckout(abs) {
		t.Errorf("corpus.go as a directory must NOT be a source checkout")
	}
}

// TestIsSourceCheckout_NonRegularCorpusGoRejected — the feature contract says
// the heuristic fires iff corpus.go is a REGULAR file. A non-regular entry named
// corpus.go (here a FIFO, standing in for any pipe/socket/device) alongside a
// templates/core/ dir must NOT fire the guard — otherwise a consumer target
// carrying such an entry could see a spurious refuse/WARN, violating the
// consumer-safety invariant. The predicate uses fi.Mode().IsRegular(), which
// rejects directories, pipes, sockets, and devices alike.
//
// Created via syscall.Mkfifo (Unix only). The test t.Skip's on Windows and if
// Mkfifo errors (unsupported platform / sandbox), matching the reviewer's
// "where supported" guidance.
func TestIsSourceCheckout_NonRegularCorpusGoRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Mkfifo is Unix-only; skipping the non-regular-corpus.go fixture on Windows")
	}
	abs := t.TempDir()
	fifoPath := filepath.Join(abs, "corpus.go")
	if err := syscall.Mkfifo(fifoPath, 0o644); err != nil {
		t.Skipf("Mkfifo unsupported on this platform/sandbox (%v); cannot exercise the non-regular entry", err)
	}
	writeDiskCorpusFile(t, abs, "a.txt", "x") // templates/core present
	if isSourceCheckout(abs) {
		t.Errorf("a non-regular corpus.go (FIFO) must NOT satisfy the source-checkout predicate (feature contract: regular file)")
	}
	// Belt-and-suspenders: the wired entry point must also report not-applicable
	// for this target, proving no guard fires downstream.
	if r := checkCorpusFreshness(abs); r.status != freshnessNotApplicable {
		t.Errorf("checkCorpusFreshness on a non-regular-corpus.go target: status=%s want not-applicable; detail=%q", r.status, r.detail)
	}
}

// TestIsSourceCheckout_ResolvedTargetNotCWD — F10: invoked from a source
// checkout CWD but --target points at a consumer. The guard MUST evaluate the
// resolved target, never probe CWD. (CWD-independence is inherent: the
// function takes abs and never reads getwd.)
func TestIsSourceCheckout_ResolvedTargetNotCWD(t *testing.T) {
	sourceCWD := t.TempDir()
	writeCorpusGo(t, sourceCWD)
	writeDiskCorpusFile(t, sourceCWD, "a.txt", "x")
	consumerTarget := t.TempDir() // no markers

	// Run with CWD = the source checkout, but evaluate the consumer target.
	runWithCwd(t, sourceCWD, func() {
		got := isSourceCheckout(consumerTarget)
		if got {
			t.Errorf("guard must evaluate the RESOLVED target, not CWD: consumer target reported as source checkout while CWD was a source checkout")
		}
		// And the source CWD itself still matches when evaluated directly.
		if !isSourceCheckout(sourceCWD) {
			t.Errorf("source CWD evaluated directly should be a source checkout")
		}
	})
}

// TestIsSourceCheckout_SymlinkPolicy pins the documented symlink policy:
// os.Stat follows symlinks, so a symlink to a real corpus.go / a real
// templates/core directory COUNTS. This mirrors how a developer might alias
// the source tree, and is the more useful direction.
func TestIsSourceCheckout_SymlinkPolicy(t *testing.T) {
	if !symlinksSupported() {
		t.Skip("symlinks not supported (running as root without CAP_DAC_OVERRIDE? or unsupported OS)")
	}
	// A real source tree living elsewhere, aliased into abs via symlinks.
	real := t.TempDir()
	writeCorpusGo(t, real)
	writeDiskCorpusFile(t, real, "a.txt", "x")

	abs := t.TempDir()
	// abs/corpus.go -> real/corpus.go (symlink to a file). Parent (abs) exists.
	if err := os.Symlink(filepath.Join(real, "corpus.go"), filepath.Join(abs, "corpus.go")); err != nil {
		t.Fatalf("symlink corpus.go: %v", err)
	}
	// abs/templates/core -> real/templates/core (symlink to a dir). The symlink
	// entry lives under abs/templates/, which must exist before symlink(2).
	if err := os.MkdirAll(filepath.Join(abs, "templates"), 0o755); err != nil {
		t.Fatalf("mkdir abs/templates: %v", err)
	}
	if err := os.Symlink(filepath.Join(real, corpus.CoreDir), filepath.Join(abs, corpus.CoreDir)); err != nil {
		t.Fatalf("symlink templates/core: %v", err)
	}
	if !isSourceCheckout(abs) {
		t.Errorf("symlink policy: a symlinked source tree should COUNT as a source checkout")
	}
}

// symlinksSupported reports whether the test filesystem permits creating
// symlinks (some CI sandboxes run without it). Used to skip the symlink test
// honestly rather than fail on an environment limitation.
func symlinksSupported() bool {
	dir, err := os.MkdirTemp("", "vh-symlink-probe-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		return false
	}
	link := filepath.Join(dir, "link")
	return os.Symlink(target, link) == nil
}

// --- comparator (embedded-manifest-driven) ---------------------------------

// TestCompareCorpus_Fresh — embedded manifest and on-disk tree match
// path-for-path, byte-for-byte → freshnessFresh.
func TestCompareCorpus_Fresh(t *testing.T) {
	abs := t.TempDir()
	files := map[string]string{
		"a.txt":                 "alpha",
		"deeply/nested/file.md": "beta\n",
		".opencode/x.json":      "{}",
	}
	embedded := mapFS(files)
	for rel, body := range files {
		writeDiskCorpusFile(t, abs, rel, body)
	}
	r := compareCorpus(embedded, abs)
	if r.status != freshnessFresh {
		t.Errorf("matching corpora: status=%s want fresh; detail=%q diffs=%v", r.status, r.detail, r.diffs)
	}
}

// TestCompareCorpus_ModifiedFile — one disk file differs in bytes → differs.
// Never asserts direction (no stale/newer language in the result).
func TestCompareCorpus_ModifiedFile(t *testing.T) {
	abs := t.TempDir()
	embedded := mapFS(map[string]string{"a.txt": "alpha", "b.txt": "beta"})
	writeDiskCorpusFile(t, abs, "a.txt", "alpha")
	writeDiskCorpusFile(t, abs, "b.txt", "BETA-MUTATED") // differs
	r := compareCorpus(embedded, abs)
	if r.status != freshnessDiffers {
		t.Fatalf("modified file: status=%s want differs", r.status)
	}
	if len(r.diffs) != 1 || !strings.Contains(r.diffs[0], "b.txt") {
		t.Errorf("diffs should name b.txt; got %v", r.diffs)
	}
	// Direction-neutrality: no stale/newer/older language anywhere.
	for _, bad := range []string{"stale", "newer", "older"} {
		if strings.Contains(strings.ToLower(r.detail), bad) || sliceContainsAny(r.diffs, bad) {
			t.Errorf("result must not assert direction (%q found): detail=%q diffs=%v", bad, r.detail, r.diffs)
		}
	}
}

// TestCompareCorpus_EmbeddedPathMissingOnDisk — a path the embedded manifest
// lists but the disk lacks → differs (the embedded corpus and the checkout
// disagree about what files exist).
func TestCompareCorpus_EmbeddedPathMissingOnDisk(t *testing.T) {
	abs := t.TempDir()
	embedded := mapFS(map[string]string{"a.txt": "alpha", "gone.txt": "x"})
	writeDiskCorpusFile(t, abs, "a.txt", "alpha")
	// gone.txt deliberately not written on disk.
	r := compareCorpus(embedded, abs)
	if r.status != freshnessDiffers {
		t.Fatalf("missing on disk: status=%s want differs", r.status)
	}
	found := false
	for _, d := range r.diffs {
		if strings.Contains(d, "gone.txt") && strings.Contains(d, "missing on disk") {
			found = true
		}
	}
	if !found {
		t.Errorf("diffs should mark gone.txt (missing on disk); got %v", r.diffs)
	}
}

// TestCompareCorpus_DiskOnlyFileIgnored — the IGNORE policy: a path present on
// disk but NOT in the embedded manifest (editor swap, OS metadata, leftovers)
// is NOT counted as a mismatch. The render only writes embedded paths, so a
// disk-only file is never clobbered and is irrelevant to the staleness
// question.
func TestCompareCorpus_DiskOnlyFileIgnored(t *testing.T) {
	abs := t.TempDir()
	embedded := mapFS(map[string]string{"a.txt": "alpha"})
	writeDiskCorpusFile(t, abs, "a.txt", "alpha")
	// Disk-only files that look like real noise the OS / editors leave behind.
	writeDiskCorpusFile(t, abs, ".DS_Store", "junk")
	writeDiskCorpusFile(t, abs, ".a.txt.swp", "swap")
	writeDiskCorpusFile(t, abs, "leftover/old.md", "stale leftover")
	r := compareCorpus(embedded, abs)
	if r.status != freshnessFresh {
		t.Errorf("disk-only files must be IGNORED (policy): status=%s want fresh; detail=%q diffs=%v",
			r.status, r.detail, r.diffs)
	}
}

// TestCompareCorpus_RenamedFile — an embedded path X missing on disk PLUS a
// disk-only path Y (X renamed to Y) → differs (the manifest walk sees X
// missing; Y is ignored as disk-only).
func TestCompareCorpus_RenamedFile(t *testing.T) {
	abs := t.TempDir()
	embedded := mapFS(map[string]string{"old.txt": "alpha"})
	// Disk has new.txt (disk-only → ignored) but not old.txt (missing → differs).
	writeDiskCorpusFile(t, abs, "new.txt", "alpha")
	r := compareCorpus(embedded, abs)
	if r.status != freshnessDiffers {
		t.Errorf("renamed file: status=%s want differs (old.txt missing on disk)", r.status)
	}
}

// TestCompareCorpus_DiskFileIsDirError — the fail-safe: when reading a disk
// path fails for a reason other than NotExist (here the disk entry is a
// directory where the manifest expects a file), the comparator returns
// freshnessError rather than silently treating it as fresh or differs.
func TestCompareCorpus_DiskFileIsDirError(t *testing.T) {
	abs := t.TempDir()
	embedded := mapFS(map[string]string{"foo": "alpha"})
	// Make disk templates/core/foo a DIRECTORY — os.ReadFile returns a non-
	// IsNotExist error ("is a directory"), exercising the fail-safe branch.
	if err := os.MkdirAll(filepath.Join(abs, corpus.CoreDir, "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := compareCorpus(embedded, abs)
	if r.status != freshnessError {
		t.Errorf("unreadable disk file must fail-safe to error: status=%s want error; detail=%q", r.status, r.detail)
	}
}

// TestCheckCorpusFreshness_ConsumerShortCircuits — the wired entry point must
// return freshnessNotApplicable for a consumer WITHOUT touching the embedded
// FS. This is the decisive consumer-safety property: the guard is structurally
// inert for every consumer.
func TestCheckCorpusFreshness_ConsumerShortCircuits(t *testing.T) {
	abs := t.TempDir() // no corpus.go, no templates/core
	r := checkCorpusFreshness(abs)
	if r.status != freshnessNotApplicable {
		t.Errorf("consumer: status=%s want not-applicable; detail=%q", r.status, r.detail)
	}
}

// --- doctor-facing result mappers ------------------------------------------

// TestDevStaleCheckResult_Tiers pins the WARN-only contract: differs and error
// map to WARN (never FAIL); fresh maps to PASS; not-applicable (filtered by the
// caller) maps to SKIP.
func TestDevStaleCheckResult_Tiers(t *testing.T) {
	cases := []struct {
		status freshnessStatus
		want   string
	}{
		{freshnessFresh, tierPass},
		{freshnessDiffers, tierWarn},
		{freshnessError, tierWarn},
		{freshnessNotApplicable, tierSkip},
	}
	for _, tc := range cases {
		t.Run(tc.status.String(), func(t *testing.T) {
			r := devStaleCheckResult(freshnessResult{status: tc.status, detail: "d"})
			if r.tier != tc.want {
				t.Errorf("status %s: tier=%s want %s", tc.status, r.tier, tc.want)
			}
			if r.tier == tierFail {
				t.Errorf("dev-stale must NEVER be FAIL (WARN-only contract)")
			}
		})
	}
}

// TestDevStaleCheckResult_DiffersMentionsMakeUpdate — the WARN detail must
// point at `make update` (rebuild) as the recovery.
func TestDevStaleCheckResult_DiffersMentionsMakeUpdate(t *testing.T) {
	r := devStaleCheckResult(freshnessResult{status: freshnessDiffers, detail: "x"})
	if !strings.Contains(r.detail, "make update") {
		t.Errorf("differs WARN should point at `make update`; got %q", r.detail)
	}
}

// TestQualifyManagedDriftOnDevStale — the managed-drift detail qualifier: when
// dev-stale fires, a bare "in sync" becomes "in sync with the embedded corpus
// in this binary" + a dev-stale suffix; when dev-stale does not fire, the
// detail is unchanged.
func TestQualifyManagedDriftOnDevStale(t *testing.T) {
	// Fresh / not-applicable → unchanged.
	orig := checkResult{name: "managed-drift", tier: tierPass, detail: "5 managed file(s) in sync"}
	if got := qualifyManagedDriftOnDevStale(orig, freshnessResult{status: freshnessFresh}); got.detail != orig.detail {
		t.Errorf("fresh: detail must be unchanged; got %q", got.detail)
	}
	if got := qualifyManagedDriftOnDevStale(orig, freshnessResult{status: freshnessNotApplicable}); got.detail != orig.detail {
		t.Errorf("not-applicable: detail must be unchanged; got %q", got.detail)
	}
	// Differs → qualified: no bare "in sync", mentions embedded corpus + dev-stale-embed.
	q := qualifyManagedDriftOnDevStale(orig, freshnessResult{status: freshnessDiffers, detail: "x"})
	if strings.Contains(q.detail, "in sync") && !strings.Contains(q.detail, "in sync with the embedded corpus in this binary") {
		t.Errorf("differs: a bare 'in sync' must be qualified; got %q", q.detail)
	}
	if !strings.Contains(q.detail, "dev-stale-embed") {
		t.Errorf("differs: detail should point at dev-stale-embed; got %q", q.detail)
	}
	// Tier unchanged.
	if q.tier != tierPass {
		t.Errorf("differs: tier must stay PASS (qualifier never changes tier); got %s", q.tier)
	}
	// Error → qualified with the error-flavored suffix, tier unchanged.
	dr := checkResult{name: "managed-drift", tier: tierFail, detail: "3 drifted of 10 managed"}
	qe := qualifyManagedDriftOnDevStale(dr, freshnessResult{status: freshnessError, detail: "read: denied"})
	if qe.tier != tierFail {
		t.Errorf("error: tier must stay FAIL; got %s", qe.tier)
	}
	if !strings.Contains(qe.detail, "freshness check failed") {
		t.Errorf("error: detail should note the failed freshness check; got %q", qe.detail)
	}
}

// sliceContainsAny reports whether any string in list contains sub
// (case-insensitive), used for the direction-neutrality check.
func sliceContainsAny(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(strings.ToLower(s), sub) {
			return true
		}
	}
	return false
}
