package cli

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/manifest"
	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// seamDriftReport is the seam-model equivalent of drift.Report: it categorizes
// every platform-controlled corpus path against the live tree by re-rendering
// the corpus into staging (seamInventory) and byte-comparing. It is the data
// behind `diff` and preflight's drift check, and mirrors doctor's managed-drift
// definition (only platform-controlled bytes are drift; project_owned /
// platform_armed are intentionally preserved/merged and never flagged).
type seamDriftReport struct {
	ok         []string // platform-controlled path present and byte-identical
	drifted    []string // present but bytes differ from the re-rendered corpus
	missing    []string // classified managed path absent from the live tree
	unexpected []string // live file under .opencode/ not produced by the corpus
}

func (r *seamDriftReport) hasProblems() bool {
	return len(r.drifted)+len(r.missing)+len(r.unexpected) > 0
}

// isPlatformDriftClass reports whether a class is byte-controlled by the
// platform and therefore subject to drift detection. platform_managed and
// overlay_extension files are deterministically rendered, so a live mismatch is
// real drift. platform_armed (schema-merged with project edits), project_owned
// (operator content), and external_generated (recon output) legitimately differ
// from a naive re-render and are NOT drift — they are linted/preserved by other
// surfaces (doctor's armed-schema; uninstall's preserve rules).
func isPlatformDriftClass(c ownership.Class) bool {
	return c == ownership.ClassPlatformManaged || c == ownership.ClassOverlayExtension
}

// seamUnexpectedSkip is the set of first-segment .opencode/ subtrees that hold
// per-project runtime state or generated install output the corpus never tracks,
// so they must not be reported as "unexpected". Mirrors the legacy drift
// package's managedSubtreesToSkip plus the session/run scratch the agents write.
var seamUnexpectedSkip = map[string]bool{
	"state":        true,
	"plans":        true,
	"runs":         true,
	"sessions":     true,
	"node_modules": true,
}

// computeSeamDrift re-renders the corpus + overlays and categorizes every
// platform-controlled path against the live tree, plus discovers unexpected
// files under .opencode/. The returned lists are sorted. It is read-only.
func computeSeamDrift(target string) (*seamDriftReport, error) {
	staging, eff, _, inactive, err := seamInventory(target)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	rep := &seamDriftReport{}
	expected := make(map[string]bool, len(eff))
	for rel, entry := range eff {
		expected[rel] = true
		if !isPlatformDriftClass(entry.Class) {
			continue
		}
		staged, sok := liveBytes(staging, rel)
		if !sok {
			// A classified managed path the renderer did not write is a
			// platform/template bug; surface it as drift rather than hide it.
			rep.drifted = append(rep.drifted, rel)
			continue
		}
		live, lok := liveBytes(target, rel)
		if !lok {
			rep.missing = append(rep.missing, rel)
			continue
		}
		if bytes.Equal(live, staged) {
			rep.ok = append(rep.ok, rel)
		} else {
			rep.drifted = append(rep.drifted, rel)
		}
	}
	rep.unexpected = findSeamUnexpected(target, expected, inactive)

	sort.Strings(rep.ok)
	sort.Strings(rep.drifted)
	sort.Strings(rep.missing)
	sort.Strings(rep.unexpected)
	return rep, nil
}

// findSeamUnexpected walks the live .opencode/ tree and returns every file not
// in expected, skipping the runtime-state / generated subtrees (by first path
// segment) the corpus never tracks. Scoped to .opencode/ so project files
// elsewhere in the repo are never flagged.
//
// inactive is the set of capability-owned core LIVE paths NOT in the resolved
// selection. A prior-version file left on disk from a selected→deselected
// transition is inactive residue: it is exempt from unexpected-drift detection
// BY EXACT PATH so the operator is not told to delete a file the harness leaves
// untouched on purpose. The exemption is exact-path only — a different file in
// the same directory (or a renamed copy) is still reported as unexpected.
func findSeamUnexpected(target string, expected map[string]bool, inactive map[string]bool) []string {
	root := filepath.Join(target, manifest.DirName)
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rerr := filepath.Rel(target, p)
		if rerr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if seg := firstOpencodeSegment(relSlash); seg != "" && seamUnexpectedSkip[seg] {
				return fs.SkipDir
			}
			return nil
		}
		if expected[relSlash] {
			return nil
		}
		// Exact-path residue exemption: an inactive capability-owned file is
		// not "unexpected" even though it is absent from the active expected
		// map. The harness leaves prior-version files untouched on a
		// deselection; flagging them would prompt a deletion that violates the
		// no-retirement contract.
		if inactive != nil && inactive[relSlash] {
			return nil
		}
		if seg := firstOpencodeSegment(relSlash); seg != "" && seamUnexpectedSkip[seg] {
			return nil
		}
		out = append(out, relSlash)
		return nil
	})
	return out
}

// firstOpencodeSegment returns the first path segment after ".opencode/" in a
// repo-relative slash path, or "" if rel is not under .opencode/.
func firstOpencodeSegment(rel string) string {
	prefix := manifest.DirName + "/"
	if !strings.HasPrefix(rel, prefix) {
		return ""
	}
	rest := rel[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}
