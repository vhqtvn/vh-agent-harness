package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// This file implements the dogfood-local staleness guard: a heuristic that
// detects when the harness is running inside its OWN source checkout whose
// on-disk templates/core differs from the corpus embedded in the running
// binary, plus an embedded-manifest-driven comparator. update uses it to
// REFUSE a live re-render that would silently clobber newer in-tree renders
// with older bytes; doctor uses it to WARN (without failing) and to qualify
// the managed-drift output so it never prints an unqualified "in sync".
//
// Background (the footgun this closes): the corpus is baked into the binary at
// BUILD time via `//go:embed all:templates/core` (corpus.go). `update` renders
// ONLY from that embedded copy, never from the working tree. In this dev/
// dogfood repo the on-disk templates/core is frequently newer than the PATH
// binary's baked-in copy; a bare `vh-agent-harness update` then silently
// rewrites .opencode/ mirrors to older content, and `doctor` cannot detect the
// revert because checkManagedDrift re-renders from the SAME embedded corpus
// and byte-compares (it self-reports "in sync"). Consumer repos whose binary
// embeds the same corpus they run with are unaffected — and the guard is
// structurally inert for them: the comparator never runs unless the target
// positively looks like a source checkout.

// freshnessStatus is the shared structured result of the staleness guard. It
// is the single value update and doctor consume.
//
// The statuses:
//   - freshnessNotApplicable: the resolved target is not a source checkout
//     (no corpus.go + templates/core at the target root). Every consumer hits
//     this; behavior is byte-identical to before this guard existed.
//   - freshnessFresh: the embedded corpus and the on-disk templates/core
//     match path-for-path, byte-for-byte. No staleness signal.
//   - freshnessDiffers: at least one embedded path is absent on disk or
//     differs in bytes. The result NEVER asserts a direction (stale/newer/
//     older): a byte comparison proves difference, not chronology. The
//     recovery is `make update` (rebuild from source) regardless of which
//     side is "ahead".
//   - freshnessError: the embedded walk or a disk read failed. Fail-safe:
//     callers refuse (update) or WARN (doctor) rather than proceed blindly.
type freshnessStatus int

const (
	freshnessNotApplicable freshnessStatus = iota
	freshnessFresh
	freshnessDiffers
	freshnessError
)

func (s freshnessStatus) String() string {
	switch s {
	case freshnessNotApplicable:
		return "not-applicable"
	case freshnessFresh:
		return "fresh"
	case freshnessDiffers:
		return "differs"
	case freshnessError:
		return "error"
	}
	return "?"
}

// freshnessResult carries the status plus a human detail and the deterministic
// list of canonical relative paths that differ (for diagnostics). diffs is
// non-empty only when status == freshnessDiffers.
type freshnessResult struct {
	status freshnessStatus
	detail string
	diffs  []string
}

// sourceCheckoutCorpusFile is the module-root Go file whose presence (together
// with a templates/core directory) identifies a source checkout. corpus.go
// carries the `//go:embed all:templates/core` directive; a repo with both this
// file and templates/core/ is the harness's own source. The file imports only
// "embed" and is module-path-independent, so a fork under a different module
// path still matches.
const sourceCheckoutCorpusFile = "corpus.go"

// maxFreshnessDiffsInDetail caps the number of differing paths rendered in the
// human detail so the message stays readable on a large divergence; the full
// set stays available on freshnessResult.diffs.
const maxFreshnessDiffsInDetail = 8

// isSourceCheckout reports whether abs looks like the harness's own source
// checkout (the harness dogfoods itself). HEURISTIC, NOT A PROOF.
//
// Fire condition: abs/corpus.go is a regular (non-directory) file AND
// abs/templates/core is a directory, evaluated against the RESOLVED target
// root abs (the path update/doctor will write into, derived from --target or
// cwd). It NEVER probes a fresh CWD; it sees exactly the tree the caller
// resolved.
//
// Acknowledged limits (deliberately NOT claimed as zero-FP/zero-FN anywhere):
//   - os.Stat follows symlinks. SYMLINK POLICY: a symlink to a real corpus.go
//     or a real templates/core directory COUNTS, mirroring how a developer
//     might alias the source tree. This is the more useful direction (a
//     symlinked source root still needs the guard) and is explicitly tested.
//   - A consumer or monorepo could manually carry both files (false positive).
//     Acceptable: such a tree is indistinguishable from a source checkout and
//     the guard's worst case is a refused update, recoverable via
//     --allow-stale-corpus or `make update`.
//   - A future refactor moving the embed directive out of corpus.go produces a
//     false negative (the guard goes silent). That is the SAFE failure
//     direction — the guard degrades to not-applicable, never to a wrong
//     refuse.
//   - Partial clones missing corpus.go or templates/core are not-applicable.
//
// This heuristic is the SOLE fire condition for the comparator; the comparator
// never runs for a consumer (no corpus.go + templates/core), which is the
// decisive consumer-safety property.
func isSourceCheckout(abs string) bool {
	// os.Stat follows symlinks (documented policy): a symlink to a real regular
	// corpus.go file counts as presence. The predicate requires a REGULAR file:
	// `!IsDir()` alone would accept a FIFO/socket/device named corpus.go, which
	// is not the "embed-bearing source file" the heuristic means and could fire
	// the guard spuriously on a consumer target. IsRegular() rejects all
	// non-regular forms (directories, symlinks-to-dirs, pipes, sockets,
	// devices) while still accepting a symlink whose target is a regular file
	// (os.Stat follows it and reports the target's regular mode).
	fi, err := os.Stat(filepath.Join(abs, sourceCheckoutCorpusFile))
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	// Likewise a symlink to a real templates/core directory counts.
	di, err := os.Stat(filepath.Join(abs, corpus.CoreDir))
	if err != nil || !di.IsDir() {
		return false
	}
	return true
}

// compareCorpus is the embedded-manifest-driven comparator. It walks the
// embedded corpus fs.FS (rooted at the corpus root — i.e. templates/core
// CONTENTS, the same fs.Sub coreSubFSImpl returns) as the AUTHORITATIVE
// manifest. For each embedded relative file path it reads the matching on-disk
// file under abs/templates/core and compares path+bytes deterministically.
//
// It does NOT hash an arbitrary OS walk. The `all:` go:embed directive
// includes dot-prefixed paths (`.opencode/`, `.local/`) that an OS walk may
// diverge on, so the embedded tree is the single canonical manifest. The walk
// is lexical (fs.WalkDir visits lexically), mirroring the renderer's walk in
// internal/substrate/embed_renderer.go so "the corpus" means the same set of
// files here and at render time.
//
// POLICIES (explicit, tested):
//
//   - DISK-ONLY files (present on disk but NOT in the embedded manifest —
//     editor swaps like `.foo.swp`, OS metadata like `.DS_Store`, leftovers):
//     IGNORED. They never count as a mismatch. Rationale: the render only
//     writes embedded paths, so a disk-only file under templates/core is never
//     clobbered by an update and is irrelevant to the staleness question. The
//     embedded manifest is authoritative for what "the corpus" is.
//
//   - EMBEDDED paths MISSING on disk (a partially-deleted checkout, or a
//     genuinely different source state): counts as differs. The embedded
//     corpus and the checkout disagree about what files exist.
//
// The result NEVER asserts direction (stale/newer/older). A difference is a
// difference; the recovery is `make update` (rebuild from source) either way.
//
// embedded is parameterized (rather than reading corpus.CoreFS directly) so
// the comparator logic is unit-testable with a synthetic manifest. The wired
// entry point checkCorpusFreshness passes the real embedded corpus.
func compareCorpus(embedded fs.FS, abs string) freshnessResult {
	diskRoot := filepath.Join(abs, corpus.CoreDir)
	var diffs []string
	walkErr := fs.WalkDir(embedded, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// rel is canonical: forward-slash, lexical order via fs.WalkDir.
		embeddedBytes, rerr := fs.ReadFile(embedded, rel)
		if rerr != nil {
			// WalkDir listed the path but ReadFile failed — an internal
			// inconsistency in the embedded FS. Fail-safe: surface as error.
			return fmt.Errorf("read embedded %q: %w", rel, rerr)
		}
		diskPath := filepath.Join(diskRoot, filepath.FromSlash(rel))
		diskBytes, derr := os.ReadFile(diskPath)
		if derr != nil {
			if os.IsNotExist(derr) {
				diffs = append(diffs, rel+" (missing on disk)")
				return nil
			}
			// Unreadable disk file (permission, transient I/O) — fail-safe.
			return fmt.Errorf("read disk %q: %w", rel, derr)
		}
		if !bytes.Equal(embeddedBytes, diskBytes) {
			diffs = append(diffs, rel+" (modified)")
		}
		return nil
	})
	if walkErr != nil {
		return freshnessResult{status: freshnessError, detail: walkErr.Error()}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		return freshnessResult{
			status: freshnessDiffers,
			detail: fmt.Sprintf(
				"embedded corpus differs from %s: %d path(s) differ: %s",
				diskRoot, len(diffs), truncList(diffs, maxFreshnessDiffsInDetail),
			),
			diffs: diffs,
		}
	}
	return freshnessResult{
		status: freshnessFresh,
		detail: fmt.Sprintf("embedded corpus matches on-disk %s", diskRoot),
	}
}

// truncList joins up to max paths lexically and appends an "... and N more"
// suffix when the list is longer, keeping diagnostic lines readable.
func truncList(paths []string, max int) string {
	if len(paths) <= max {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:max], ", ") + fmt.Sprintf(", ... and %d more", len(paths)-max)
}

// checkCorpusFreshness is the wired entry point used by update and doctor. It
// resolves the real embedded corpus (corpus.CoreFS via the memoized
// coreSubFSImpl — the same fs.FS the renderer reads) and runs compareCorpus
// against abs/templates/core. A target that is not a source checkout
// short-circuits to freshnessNotApplicable WITHOUT touching the embedded FS —
// the decisive consumer-safety path (consumers never pay the walk cost and
// never see a staleness signal).
func checkCorpusFreshness(abs string) freshnessResult {
	if !isSourceCheckout(abs) {
		return freshnessResult{
			status: freshnessNotApplicable,
			detail: "not a source checkout (no corpus.go + templates/core at the target root)",
		}
	}
	sub, err := coreSubFSImpl()
	if err != nil {
		return freshnessResult{
			status: freshnessError,
			detail: fmt.Sprintf("resolve embedded corpus: %v", err),
		}
	}
	return compareCorpus(sub, abs)
}

// devStaleCheckResult maps a freshnessResult to a doctor checkResult. WARN-only
// by construction for the non-fresh cases: the DEV-STALE signal NEVER fails the
// command by itself (real drift is owned by managed-drift; dev-stale is the
// distinct, softer signal that the binary's corpus and the checkout disagree).
// freshnessNotApplicable is filtered by the caller (consumers see no section).
func devStaleCheckResult(fr freshnessResult) checkResult {
	const name = "dev-stale-embed"
	switch fr.status {
	case freshnessFresh:
		return checkResult{name: name, tier: tierPass,
			detail: "embedded corpus matches checkout's templates/core"}
	case freshnessDiffers:
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf(
				"embedded corpus DIFFERS from checkout's templates/core — run `make update` (rebuilds first) or rebuild the binary; a live `vh-agent-harness update` would be refused. %s",
				fr.detail,
			)}
	case freshnessError:
		return checkResult{name: name, tier: tierWarn,
			detail: fmt.Sprintf("freshness check failed (fail-safe WARN): %s", fr.detail)}
	}
	return checkResult{name: name, tier: tierSkip, detail: "not a source checkout"}
}

// qualifyManagedDriftOnDevStale rewrites a managed-drift checkResult's DETAIL so
// it never prints an unqualified "in sync" when the embedded corpus differs
// from (or could not be compared against) the checkout. The managed-drift TIER
// is unchanged — real drift still FAILs and a byte-match still passes — but the
// detail is qualified so an operator is not misled into thinking the tree is
// fully current. When dev-stale fires and managed-drift reports a match, the
// detail explicitly states the match is against the BINARY's embedded corpus
// (not the checkout's source) and points at `make update`/rebuild plus the
// dev-stale-embed section.
//
// This is the correction for the circular-drift-check finding: a stale binary
// that reverted mirrors re-renders those same older bytes and self-reports "in
// sync"; qualifying the detail breaks that illusion without changing the tier
// (the WARN lives in dev-stale-embed, not here).
func qualifyManagedDriftOnDevStale(dr checkResult, fr freshnessResult) checkResult {
	if fr.status != freshnessDiffers && fr.status != freshnessError {
		return dr
	}
	var suffix string
	switch fr.status {
	case freshnessDiffers:
		suffix = " (dev-stale: embedded corpus differs from checkout — managed-drift compares live files against the BINARY's embedded corpus, NOT the checkout's templates/core; run `make update`/rebuild, see dev-stale-embed)"
	case freshnessError:
		suffix = " (dev-stale: freshness check failed — managed-drift compares live files against the BINARY's embedded corpus; see dev-stale-embed)"
	}
	// Replace the first bare "in sync" with the qualified form so the output
	// never claims an unqualified sync against a divergent checkout. If the
	// detail has no "in sync" (drifted/missing cases), Replace is a no-op and
	// only the suffix is appended — still valuable, since the "run update"
	// recovery in those messages would otherwise hit the update guard.
	dr.detail = strings.Replace(dr.detail, "in sync", "in sync with the embedded corpus in this binary", 1) + suffix
	return dr
}
