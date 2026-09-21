// component.go — the canonical session-identity filename grammar shared
// by every storage boundary that derives a filesystem path from a
// session id or a subagent child id (the protocol engine's default-path
// branch and the subagents FileStore). One grammar, one place: the
// engine boundary (wire-facing) and the FileStore (log-derived ids from
// possibly forged logs) both delegate here so the two surfaces cannot
// drift apart.
package session

import (
	"fmt"
	"regexp"
	"strings"
)

// idComponentPattern is the strict single-filename-component grammar:
// an alphanumeric first character, then alphanumerics, dots,
// underscores, and hyphens. Because the first character must be
// alphanumeric and the rest exclude both path separators, a matching id
// can never be ".", "..", an absolute path, or a multi-component path —
// filepath.Join(root, id+".jsonl") therefore always names a file
// DIRECTLY inside root, with no lexical cleaning escape.
var idComponentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ErrInvalidIDComponent is the typed sentinel for a rejected id; the
// detailed errors wrap it, so callers can errors.Is against it while
// still carrying their own path context.
var ErrInvalidIDComponent = fmt.Errorf("id is not a valid filename component")

// ValidateIDComponent reports whether id is a strict single filename
// component safe to interpolate into <dir>/<id>.jsonl storage paths.
// It rejects, fail-closed: the empty string; "." and ".."; any path
// separator ('/' or '\' — belt and braces: the grammar already excludes
// both); and any character outside the grammar. It is PURE (no
// filesystem access) so callers can run it before any durable effect.
func ValidateIDComponent(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty id", ErrInvalidIDComponent)
	}
	// Redundant with the grammar but explicit: these shapes are the
	// documented escape primitives, and rejecting them by name keeps
	// the contract readable even if the grammar ever changes.
	if id == "." || id == ".." {
		return fmt.Errorf("%w: %q is a relative-path primitive", ErrInvalidIDComponent, id)
	}
	if strings.ContainsAny(id, `/\`) {
		return fmt.Errorf("%w: %q contains a path separator", ErrInvalidIDComponent, id)
	}
	if !idComponentPattern.MatchString(id) {
		return fmt.Errorf("%w: %q does not match ^[A-Za-z0-9][A-Za-z0-9._-]*$", ErrInvalidIDComponent, id)
	}
	return nil
}
