package complexity

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ScanRepo enumerates the repo at root and produces a Signal for every file
// that is supported (by extension) and not excluded (by the snapshot projection
// rules), computing file_loc against the policy's snapshot threshold. It returns
// ALL eligible signals in canonical presentation order (descending observed LOC,
// then ascending path); presentation truncation is the caller's responsibility.
//
// ENUMERATION: ScanRepo prefers `git ls-files` so only TRACKED source is scanned
// (naturally excluding gitignored vendored/reference directories like refs/,
// build artifacts, and tmp/). When git is unavailable or the root is not a git
// work tree, it falls back to a filesystem walk. Both paths apply the SAME
// policy exclusion rules on top.
//
// The scanner never performs I/O beyond reading file bytes; it runs no parser
// and collects no boundary indicator beyond the not_collected placeholder.
func ScanRepo(root string, policy Policy) ([]Signal, error) {
	files, err := enumerateSourceFiles(root)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var signals []Signal
	for _, relPath := range files {
		if !policy.Eligible(relPath, ProjectionRepoSnapshot) {
			continue
		}
		absPath := filepath.Join(root, filepath.FromSlash(relPath))
		content, readErr := os.ReadFile(absPath)
		if readErr != nil {
			// An unreadable file is not a scanner error; skip it.
			continue
		}
		sig := ComputeSignal(relPath, content, policy, ProjectionRepoSnapshot)
		sig.BoundaryIndicators = []BoundaryIndicator{BoundaryIndicatorNotCollected()}
		signals = append(signals, sig)
	}
	SortSignals(signals)
	return signals, nil
}

// enumerateSourceFiles returns the repo-relative paths of files to scan. It
// prefers `git ls-files` (tracked source only, respects .gitignore) and falls
// back to a filesystem walk when git is unavailable.
func enumerateSourceFiles(root string) ([]string, error) {
	if files, ok, err := gitTrackedFiles(root); err != nil {
		return nil, err
	} else if ok {
		return files, nil
	}
	return walkFiles(root)
}

// gitTrackedFiles runs `git ls-files` in root and returns the tracked file list.
// ok is false (no error) when root is not a git work tree or git is unavailable.
func gitTrackedFiles(root string) (files []string, ok bool, err error) {
	cmd := exec.Command("git", "-C", root, "ls-files", "--cached", "--no-empty-directory")
	out, runErr := cmd.Output()
	if runErr != nil {
		// Not a git repo or git unavailable: fall back to walk (not an error).
		return nil, false, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, filepath.ToSlash(line))
		}
	}
	if len(files) == 0 {
		return nil, false, nil
	}
	return files, true, nil
}

// walkFiles is the fallback filesystem walk (used when git is unavailable).
func walkFiles(root string) ([]string, error) {
	var files []string
	walkErr := filepath.WalkDir(root, func(absPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		relPath, relErr := filepath.Rel(root, absPath)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(relPath))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

// TruncatePresentation returns at most max candidates, preserving the full
// count so the caller can display "showing N of M". max <= 0 means "no limit".
func TruncatePresentation(signals []Signal, max int) (shown []Signal, total int) {
	total = len(signals)
	if max <= 0 || total <= max {
		return signals, total
	}
	// Return a copy of the first max to avoid aliasing the underlying slice.
	out := make([]Signal, max)
	copy(out, signals[:max])
	return out, total
}

// Nominated returns only the signals whose Metric.Nominated is true, in the same
// order. The scanner returns all eligible files (nominated or not); callers that
// want only threshold-breaching candidates filter through this.
func Nominated(signals []Signal) []Signal {
	out := make([]Signal, 0, len(signals))
	for _, s := range signals {
		if s.Metric.Nominated {
			out = append(out, s)
		}
	}
	return out
}

// SortSignalsPathAsc is a stable path-ascending sort, useful for deterministic
// full-list rendering independent of observed counts.
func SortSignalsPathAsc(signals []Signal) {
	sort.SliceStable(signals, func(i, j int) bool {
		return signals[i].Path < signals[j].Path
	})
}
