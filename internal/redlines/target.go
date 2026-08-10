package redlines

import (
	"fmt"
	"path/filepath"
	"regexp"
)

// ScanTarget names the EXACT git object a redlines scan must read. It is the
// sole locator contract between the commit gate and the scanner, settled before
// any matching or gate wiring was built.
//
// # Decision: immutable tree hash (not a private-index path)
//
// The commit gate builds a PRIVATE index during `acquire` and then captures an
// immutable tree from it:
//
//	tree_hash=$(GIT_INDEX_FILE="$private_index" git write-tree)   # commit-gate.sh ~L982
//
// and persists that hash in the acquire metadata alongside the private-index
// PATH (commit-gate.sh ~L1075-1076). Both candidates were evaluated against the
// real gate mechanics. The IMMUTABLE TREE HASH wins:
//
//  1. Sufficiency — `git ls-tree -r <tree>` deterministically recovers every
//     entry's path, mode, and blob hash; `git cat-file blob <blob>` recovers
//     content. That is ALL a redlines scanner needs: file paths for include/
//     skip rules, blob content for lexical term matching, and modes to skip
//     non-blob entries (symlinks/submodules). Rename semantics (the RXXX status
//     from `git diff-tree --name-status`) are a DIFF concept, not a TREE
//     concept, and are not needed for content/path-based scanning.
//
//  2. Immutability — a tree hash is content-addressed. Once written it cannot
//     be mutated by working-tree changes, shared-index changes, or a concurrent
//     acquire. A scanner that reads `git ls-tree/cat-file <hash>` reads EXACTLY
//     the bytes the gate authorized, even if the working tree or shared index
//     is later rewritten. A private-index PATH has none of this guarantee: the
//     gate itself `rm -f "$private_index"` in several cleanup/error paths
//     (commit-gate.sh ~L945,955-959,968-969,985-986,998-999) and the path is
//     not content-addressed, so "read the index file at $path" can race the
//     gate's own lifecycle or, under path reuse, a later acquire.
//
//  3. No-fallback is structural — there is NO constructor that derives a
//     ScanTarget from the working tree, the shared .git/index, or HEAD. The
//     only way to obtain a ScanTarget is NewScanTarget(repo, hash). Fallback to
//     the wrong state is therefore impossible, not merely discouraged.
//
// This matches the brief's recommendation: prefer the simpler immutable
// tree-hash boundary unless staged metadata requires more. It does not.
type ScanTarget struct {
	// RepoRoot is the absolute filesystem path to the repository root. A future
	// scanner runs `git -C <RepoRoot> ls-tree/cat-file <TreeHash>` against it.
	// It is not sensitive (it is a path the operator already controls).
	RepoRoot string

	// TreeHash is the immutable, content-addressed git tree object the gate
	// captured via `git write-tree`. It is the exact object to scan. It is not
	// sensitive (a content hash reveals nothing about terms).
	TreeHash string
}

// hexObjectIDRe matches a canonical lowercase git object id: 40 hex chars
// (SHA-1 repos) or 64 hex chars (SHA-256 repos). git emits lowercase; an
// uppercase or wrong-length string is not a real object id as git would print
// it and is rejected.
var hexObjectIDRe = regexp.MustCompile(`^[0-9a-f]{40}$|^[0-9a-f]{64}$`)

// NewScanTarget is the sole constructor for a ScanTarget. It validates that the
// repo root is absolute and non-empty, and that the tree hash is a well-formed
// canonical git object id. It performs NO filesystem or git access (the object
// is verified to exist only when the scanner resolves it, at scan time). On any
// problem it returns a non-nil error and a zero-value target — it NEVER silently
// substitutes the working tree, the shared index, or HEAD.
//
// There is intentionally NO NewScanTargetFromWorktree / FromIndex / FromHEAD:
// fallback to the wrong state is structurally impossible, not merely avoided.
func NewScanTarget(repoRoot, treeHash string) (ScanTarget, error) {
	if repoRoot == "" {
		return ScanTarget{}, fmt.Errorf("redlines: scan target: repo root is required (no implicit fallback)")
	}
	if !filepath.IsAbs(repoRoot) {
		return ScanTarget{}, fmt.Errorf("redlines: scan target: repo root %q must be absolute (no implicit fallback)", repoRoot)
	}
	if treeHash == "" {
		return ScanTarget{}, fmt.Errorf("redlines: scan target: tree hash is required (no implicit fallback)")
	}
	if !hexObjectIDRe.MatchString(treeHash) {
		return ScanTarget{}, fmt.Errorf("redlines: scan target: tree hash %q is not a canonical git object id (40 or 64 lowercase hex chars); no implicit fallback", treeHash)
	}
	return ScanTarget{RepoRoot: repoRoot, TreeHash: treeHash}, nil
}
