// confine.go — the symlink-safe workdir-root confinement core shared
// by the file tool family. Every user-supplied path is resolved
// against the configured Roots set BEFORE any filesystem effect:
// rejected paths produce a typed ViolationError naming the rule and
// leave zero traces (no reads through escapes, no created parents, no
// truncated targets).
//
// The semantics REUSE the shapes already proven in this tree —
// internal/protocol confineSessionPath (lexical Rel-containment,
// EvalSymlinks on the resolved root and the candidate's parent, final-
// component symlink refusal via Lstat) and internal/tools/shell
// confinedToRoot (multi-root admission of absolute paths). They are
// re-implemented here rather than shared because protocol and shell
// are sealed boundaries of their slices; the natural dedup home is a
// small internal/pathconfine leaf imported by all three (DEDUP
// CANDIDATE — noted, deliberately not taken in this slice).
package filetools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Violation rules (closed vocabulary; the typed error names one).
const (
	// RuleNoRoots: no confinement root is configured — every path is
	// rejected rather than guessed at.
	RuleNoRoots = "no-roots"
	// RuleEscape: a relative path climbs out of its resolution root.
	RuleEscape = "escape"
	// RuleOutsideRoots: an absolute path is under no configured root.
	RuleOutsideRoots = "outside-roots"
	// RuleNotAFile: the path denotes a root itself, not a file within.
	RuleNotAFile = "not-a-file"
	// RuleSymlinkEscape: a resolved symlink component leads outside
	// the root (the walk never follows it).
	RuleSymlinkEscape = "symlink-escape"
	// RuleSymlinkFinal: the final path component IS a symlink —
	// rejected outright (create would truncate its target; read would
	// follow it; an in-root symlink target is indistinguishable from
	// an escape at check time).
	RuleSymlinkFinal = "symlink-final"
	// RuleNotADirectory: an existing path that is not a directory —
	// either an intermediate path component of a candidate (a regular
	// file, so the target cannot exist under it) or a CONFIGURED ROOT
	// that resolves to a non-directory (EvalSymlinks alone cannot
	// tell; a file root would break every relative resolution).
	RuleNotADirectory = "not-a-directory"
	// RuleIsDirectory: the write target already exists as a directory.
	RuleIsDirectory = "is-a-directory"
	// RuleRootUnresolved: a configured root cannot be resolved.
	RuleRootUnresolved = "root-unresolved"
)

// ViolationError is the typed fail-closed confinement violation: it
// names the user-supplied path as given, the rule that fired, and a
// human-readable detail. Callers can errors.As the rule for policy.
type ViolationError struct {
	Path   string
	Rule   string
	Detail string
}

func (e *ViolationError) Error() string {
	return fmt.Sprintf("path %q rejected by confinement policy [%s]: %s", e.Path, e.Rule, e.Detail)
}

// violation builds the typed error.
func violation(path, rule, detail string) error {
	return &ViolationError{Path: path, Rule: rule, Detail: detail}
}

// Roots is the configured confinement set: absolute, existing
// directories. The zero value rejects every path (fail-closed);
// construct via NewRoots.
type Roots []string

// NewRoots validates and canonicalizes the configured roots: each
// entry must be a non-empty ABSOLUTE path to an existing directory
// (verified with os.Stat AFTER canonicalization — EvalSymlinks
// succeeds on regular files, so without the stat a file root —
// directly or through a symlink — would slip through), resolvable
// through symlinks. Duplicate resolved roots collapse. An empty set
// is an error — an unrooted file tool would be a same-class hole as
// an unconfined session path.
func NewRoots(dirs []string) (Roots, error) {
	if len(dirs) == 0 {
		return nil, violation("", RuleNoRoots, "at least one workdir root is required (the daemon defaults to its working directory)")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if strings.TrimSpace(d) == "" {
			return nil, violation(d, RuleNoRoots, "empty workdir root entry")
		}
		if !filepath.IsAbs(d) {
			return nil, violation(d, RuleNoRoots, "workdir roots must be absolute paths")
		}
		real, err := filepath.EvalSymlinks(filepath.Clean(d))
		if err != nil {
			return nil, violation(d, RuleRootUnresolved, fmt.Sprintf("unresolved: %v", err))
		}
		if fi, err := os.Stat(real); err != nil {
			return nil, violation(d, RuleRootUnresolved, fmt.Sprintf("cannot inspect resolved root %s: %v", real, err))
		} else if !fi.IsDir() {
			return nil, violation(d, RuleNotADirectory, fmt.Sprintf("resolved root %s is not a directory (workdir roots must be existing directories)", real))
		}
		if seen[real] {
			continue
		}
		seen[real] = true
		out = append(out, filepath.Clean(d))
	}
	if len(out) == 0 {
		return nil, violation("", RuleNoRoots, "no usable workdir roots after dedup")
	}
	return out, nil
}

// escapes reports whether target leaves root: the relative path from
// root to target is malformed or climbs with ".." (the engine's
// escapesRoot shape).
func escapes(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// candidate lexically maps a user-supplied path onto an absolute
// candidate plus the matched root: relative paths join the FIRST
// root; absolute paths must sit under SOME root (lexical, cleaned).
func (r Roots) candidate(path string) (cand, root string, err error) {
	if len(r) == 0 {
		return "", "", violation(path, RuleNoRoots, "no workdir roots configured")
	}
	if filepath.IsAbs(path) {
		clean := filepath.Clean(path)
		for _, root := range r {
			rc := filepath.Clean(root)
			if rel, err := filepath.Rel(rc, clean); err == nil && rel != "." && !escapes(rc, clean) {
				return clean, rc, nil
			}
		}
		return "", "", violation(path, RuleOutsideRoots, "absolute paths must be under a configured workdir root")
	}
	root = filepath.Clean(r[0])
	cand = filepath.Join(root, path)
	if rel, err := filepath.Rel(root, cand); err != nil || rel == "." {
		return "", "", violation(path, RuleNotAFile, "must be a file inside a workdir root, not a root itself")
	}
	if escapes(root, cand) {
		return "", "", violation(path, RuleEscape, "relative paths must stay inside the first workdir root")
	}
	return cand, root, nil
}

// resolveRead confines a path for an EXISTING-target operation (read,
// edit): the candidate's parent directory must exist and resolve —
// through symlinks — inside the resolved root, and the final component
// must not itself be a symlink. A missing target is NOT a violation
// (the tool layer reports the ordinary read error); an unresolvable
// parent is (an unknown location).
func (r Roots) resolveRead(path string) (string, string, error) {
	cand, root, err := r.candidate(path)
	if err != nil {
		return "", "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", violation(path, RuleRootUnresolved, fmt.Sprintf("root %s unresolved: %v", root, err))
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(cand))
	if err != nil {
		return "", "", violation(path, RuleSymlinkEscape, fmt.Sprintf("parent directory unresolved (it must exist and not escape via symlink): %v", err))
	}
	if escapes(realRoot, realParent) {
		return "", "", violation(path, RuleSymlinkEscape, fmt.Sprintf("symlinked parent resolves outside the workdir root %s", root))
	}
	if fi, err := os.Lstat(cand); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", "", violation(path, RuleSymlinkFinal, "final path component is a symlink")
		}
	} else if !os.IsNotExist(err) {
		return "", "", violation(path, RuleNotADirectory, fmt.Sprintf("cannot inspect the target: %v", err))
	}
	return cand, root, nil
}

// resolveWrite confines a path for a CREATE-capable operation (write):
// the parent chain may be partially or wholly missing (the write tool
// creates missing parents under the root), so containment walks the
// EXISTING prefix components with Lstat — never following a symlink —
// and rejects when any existing intermediate is a symlink out of the
// walk, a non-directory, or when the final component already exists
// as a symlink or directory. All checks are read-only: a rejection
// here precedes every filesystem effect (the TOCTOU window between
// this walk and the atomic rename belongs to a local-filesystem
// adversary racing the validation, not to the model, which controls
// only the path string).
func (r Roots) resolveWrite(path string) (string, string, error) {
	cand, root, err := r.candidate(path)
	if err != nil {
		return "", "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", violation(path, RuleRootUnresolved, fmt.Sprintf("root %s unresolved: %v", root, err))
	}
	rel, err := filepath.Rel(root, cand)
	if err != nil {
		return "", "", violation(path, RuleEscape, err.Error())
	}
	parts := strings.Split(rel, string(filepath.Separator))
	cur := realRoot
	for i, part := range parts[:len(parts)-1] {
		next := filepath.Join(cur, part)
		fi, err := os.Lstat(next)
		if err != nil {
			if os.IsNotExist(err) {
				// First missing component: everything below is
				// creatable by the write layer; stop the walk.
				_ = i
				break
			}
			return "", "", violation(path, RuleNotADirectory, fmt.Sprintf("cannot inspect %s: %v", next, err))
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", "", violation(path, RuleSymlinkEscape, fmt.Sprintf("intermediate component %s is a symlink (the walk never follows it)", next))
		}
		if !fi.IsDir() {
			return "", "", violation(path, RuleNotADirectory, fmt.Sprintf("intermediate component %s is not a directory", next))
		}
		cur = next
	}
	if fi, err := os.Lstat(cand); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", "", violation(path, RuleSymlinkFinal, "final path component is a symlink (the write would replace or truncate its target)")
		}
		if fi.IsDir() {
			return "", "", violation(path, RuleIsDirectory, "target exists and is a directory")
		}
	} else if !os.IsNotExist(err) {
		return "", "", violation(path, RuleNotADirectory, fmt.Sprintf("cannot inspect the target: %v", err))
	}
	return cand, root, nil
}

// nearestRoot returns the configured root whose resolved form contains
// target (for display-relative paths in glob/search). Falls back to
// the first root.
func (r Roots) nearestRoot(target string) string {
	best := ""
	for _, root := range r {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if !escapes(real, target) {
			if best == "" || len(real) > len(best) {
				best = real
			}
		}
	}
	if best != "" {
		return best
	}
	return filepath.Clean(r[0])
}
