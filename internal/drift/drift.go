// Package drift computes the difference between a harness manifest and the
// files actually on disk. It is the engine behind `vh-agent-harness diff`.
//
// A Report classifies every relevant path into one of four categories:
//
//	ok         - on disk and the disk SHA256 matches the manifest hash
//	drifted    - on disk but the hash differs (someone edited a managed file)
//	missing    - the manifest lists it but it is not on disk
//	unexpected - on disk under a managed root but not tracked by the manifest
//
// `unexpected` detection is scoped to the `.opencode/` tree (the managed
// component roots) minus the drift-exempt subtrees `.opencode/state/`,
// `.opencode/plans/`, and `.opencode/node_modules/`, and excluding the manifest
// file itself. `.local/` is not scanned because it is dominated by per-project
// runtime data; this keeps the drift signal clean without false-flagging
// coordinator tasks/reports. See managedSubtreesToSkip for the principled
// (ownership-class) basis of each exemption.
package drift

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
)

// Category is the drift classification for one path.
type Category int

const (
	// OK means on disk with matching hash.
	OK Category = iota
	// Drifted means on disk with a non-matching hash.
	Drifted
	// Missing means the manifest lists the path but it is absent on disk.
	Missing
	// Unexpected means on disk under a managed root but not in the manifest.
	Unexpected
)

// String returns the lower-case category label used in `vh-agent-harness diff` output.
func (c Category) String() string {
	switch c {
	case OK:
		return "ok"
	case Drifted:
		return "drifted"
	case Missing:
		return "missing"
	case Unexpected:
		return "unexpected"
	default:
		return fmt.Sprintf("category(%d)", int(c))
	}
}

// Entry is one classified path.
type Entry struct {
	// Path is install-relative (e.g. ".opencode/agents/researcher.md").
	Path string
	// Category is the drift classification.
	Category Category
	// ManifestHash is the hash recorded in the manifest ("sha256:..." or "").
	ManifestHash string
	// DiskHash is the hash computed from disk ("sha256:..." or "" when absent).
	DiskHash string
}

// Report is the full diff result.
type Report struct {
	// Entries holds one Entry per classified path, sorted by Path. OK entries
	// are included so callers can count them; `vh-agent-harness diff` filters them when
	// printing.
	Entries []Entry
	// Counts tallies entries per category for the summary line.
	Counts map[Category]int
}

// HasProblems reports whether any non-OK category is present (drifted, missing,
// or unexpected). `vh-agent-harness diff` uses this to decide its exit code.
func (r *Report) HasProblems() bool {
	return r.Counts[Drifted]+r.Counts[Missing]+r.Counts[Unexpected] > 0
}

// Drift exemptions for off-lattice ownership classes (see
// internal/ownership/class.go). Paths in these sets are NOT hand-authored, NOT
// platform-tracked, and NOT a legitimate drift signal, so the `unexpected`
// discovery walk skips them. Manifest-tracked files are unaffected: step 1 of
// Compute still hashes/classifies every manifest path regardless; these sets
// govern only the `unexpected` walk.
//
// Two principled bases, both OFF the raise/lower lattice:
//
//   - local_only runtime state: per-session scratch, coordinator tasks/reports,
//     and draft plans the harness never tracks. Subtrees `.opencode/state`,
//     `.opencode/plans`.
//   - external_generated guard-deps: the shell-guard's npm-install OUTPUT. The
//     gate (eval.js) loads web-tree-sitter, which `npm install` provisions from
//     the authored, tracked `.opencode/package.json`. That install produces two
//     outputs under `.opencode/`: the `node_modules/` tree
//     (managedSubtreesToSkip) and the `package-lock.json` lockfile
//     (guardDepsGeneratedFiles) — neither authored, neither tracked, both
//     reproducible from package.json. (The template ships only package.json;
//     .gitignore ignores node_modules; the lockfile is generated, not committed
//     scaffold.) Exempting both lets a host-shell repo be doctor-HEALTHY and
//     gate-functional at the same time: otherwise `npm install` produces ~120
//     `unexpected` drift entries that make doctor UNHEALTHY even though the
//     install is exactly what the gate requires.
//
// The exemptions are NARROW, not blanket globs:
//   - node_modules matches only the top-level `.opencode/node_modules` subtree
//     via its FIRST path segment, so a nested `.opencode/<other>/node_modules`
//     is still surfaced as unexpected drift.
//   - the lockfile exemption names exactly `.opencode/package-lock.json` — not
//     a `*.lock` or `package-lock*` glob.
//
// DEFERRED real-repo question (this prototype fix sidesteps but does NOT
// resolve): how does the real domain-neutral harness distribute its own
// shell-guard? The node-based eval.js gate is what creates this drift surface
// at all. Options the real build must decide between:
//
//	(a) Go-native / static shell-guard — no node deps, no node_modules/lockfile
//	    drift surface (eliminates these exemptions' reason for existing).
//	(b) install-time provisioning — `vh-agent-harness install` runs the dep install for
//	    the guard and records the outputs as external_generated (these
//	    exemptions stay).
//	(c) keep a node-based guard + these drift exemptions as-is.
//
// The real-build phase picks one; this principled minimal patch keeps the
// prototype's install-to-runnable proof honest regardless of that choice.
var managedSubtreesToSkip = []string{
	filepath.Join(manifest.DirName, "state"),
	filepath.Join(manifest.DirName, "plans"),
	// external_generated guard-deps install tree (shell-guard's node_modules).
	filepath.Join(manifest.DirName, "node_modules"),
}

// guardDepsGeneratedFiles are the individual npm-install output FILES at the
// `.opencode/` root (external_generated guard-deps, same basis as node_modules
// above). Currently the npm lockfile; `npm install` regenerates it alongside
// node_modules from the tracked package.json. See the managedSubtreesToSkip
// comment for the principled basis + the deferred real-repo distribution
// question.
var guardDepsGeneratedFiles = map[string]bool{
	filepath.ToSlash(filepath.Join(manifest.DirName, "package-lock.json")): true,
}

// Compute compares manifest state against disk under projectDir.
//
// Manifest-tracked paths are classified ok / drifted / missing by hashing the
// on-disk file. The `.opencode/` tree is then walked to find unexpected files
// (present on disk, absent from the manifest), skipping the runtime-state
// subtrees and the manifest file itself.
func Compute(projectDir string, m *manifest.Manifest) (*Report, error) {
	r := &Report{Counts: map[Category]int{}}

	// 1. Classify manifest-tracked paths.
	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		mf := m.Files[p]
		entry := Entry{Path: p, ManifestHash: mf.Hash}
		diskHash, err := hashFile(filepath.Join(projectDir, filepath.FromSlash(p)))
		if err != nil {
			if os.IsNotExist(err) {
				entry.Category = Missing
			} else {
				return nil, fmt.Errorf("hash %s: %w", p, err)
			}
		} else {
			entry.DiskHash = diskHash
			if diskHash == mf.Hash {
				entry.Category = OK
			} else {
				entry.Category = Drifted
			}
		}
		r.Entries = append(r.Entries, entry)
		r.Counts[entry.Category]++
	}

	// 2. Discover unexpected files under .opencode/ (minus runtime state).
	tracked := m.Files
	opencodeDir := filepath.Join(projectDir, manifest.DirName)
	manifestRel := filepath.ToSlash(filepath.Join(manifest.DirName, manifest.FileName))
	skip := map[string]bool{}
	for _, s := range managedSubtreesToSkip {
		skip[filepath.ToSlash(s)] = true
	}

	walkErr := fs.WalkDir(os.DirFS(projectDir), manifest.DirName, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d == nil {
				return err // .opencode itself missing: nothing to scan
			}
			return nil
		}
		rel := filepath.ToSlash(p)
		if d.IsDir() {
			if rel == manifest.DirName {
				return nil // descend into .opencode
			}
			// Skip runtime-state subtrees entirely.
			first := firstSegmentAfter(rel, manifest.DirName+"/")
			if skip[manifest.DirName+"/"+first] {
				return fs.SkipDir
			}
			return nil
		}
		// File: skip the manifest itself, the external_generated guard-deps
		// output files (e.g. npm package-lock.json), and anything inside
		// state/plans/node_modules (the dir check already SkipDir'd those, but
		// be defensive).
		if rel == manifestRel {
			return nil
		}
		if guardDepsGeneratedFiles[rel] {
			return nil
		}
		if _, ok := tracked[rel]; ok {
			return nil // tracked elsewhere -> not unexpected
		}
		r.Entries = append(r.Entries, Entry{Path: rel, Category: Unexpected})
		r.Counts[Unexpected]++
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return nil, fmt.Errorf("scan %s: %w", opencodeDir, walkErr)
	}

	// Re-sort so output is stable (manifest entries + unexpected interleaved).
	sort.Slice(r.Entries, func(i, j int) bool { return r.Entries[i].Path < r.Entries[j].Path })
	return r, nil
}

// firstSegmentAfter returns the first path segment of rel that follows prefix,
// or "" if rel does not start with prefix. Used to identify which .opencode/
// subtree a path is in.
func firstSegmentAfter(rel, prefix string) string {
	if !strings.HasPrefix(rel, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(rel, prefix)
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// hashFile reads a file and returns its "sha256:<hex>" digest. A missing file
// returns an error wrapping os.ErrNotExist so callers can distinguish absent
// from unreadable.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
