package ownership

import (
	"errors"
	"fmt"
)

// DowngradeError is returned when a project override LOWERS a path's protection
// on the raise/lower lattice (e.g. project_owned -> platform_managed). Per D2-A
// (enforced in v0), ordinary harness-ownership.yml overrides are raise-only: a
// downgrade is NOT representable as ordinary config drift. The error's guidance
// points to the future reviewed / scary downgrade workflow, which is held beyond
// v0 (the design notes Slice 6) and intentionally NOT implemented
// here.
//
// Detectable via errors.As or the IsDowngradeError helper, including when joined
// (errors.Join) or wrapped (fmt.Errorf("...: %w", err)):
//
//	var de *ownership.DowngradeError
//	if errors.As(err, &de) { ... }
//	if ownership.IsDowngradeError(err) { ... }
type DowngradeError struct {
	// Path is the offending repo-relative path.
	Path string
	// From is the current effective (platform default) class — the higher-
	// protection class being weakened.
	From Class
	// To is the override's target class — the lower-protection class.
	To Class
	// Reason is a short machine label ("protection lowered on the raise/lower
	// lattice"). It is informational and stable.
	Reason string
}

// Error implements the error interface. The message names the offending path and
// classes and directs the operator to the only allowed downgrade path.
func (e *DowngradeError) Error() string {
	return fmt.Sprintf(
		"ownership downgrade rejected for path %q: %s -> %s (%s).\n"+
			"Ordinary harness-ownership.yml overrides are raise-only (D2-A). "+
			"Weakening protection requires the reviewed, logged downgrade workflow:\n"+
			"    vh-agent-harness ownership downgrade --path %q\n"+
			"  or\n"+
			"    vh-agent-harness update --propose\n"+
			"(Not implemented in v0; tracked as the only future escape hatch.)",
		e.Path, e.From, e.To, e.reasonOrDefault(), e.Path,
	)
}

func (e *DowngradeError) reasonOrDefault() string {
	if e.Reason == "" {
		return "protection lowered on the raise/lower lattice"
	}
	return e.Reason
}

// IsDowngradeError reports whether err (or any error joined/wrapped beneath it)
// is a *DowngradeError. It is the convenience predicate callers use without
// importing the concrete pointer type.
func IsDowngradeError(err error) bool {
	var de *DowngradeError
	return errors.As(err, &de)
}

// UnknownPathError is returned when a project override targets a path that has
// NO platform-module default class. The validator fails closed rather than
// silently inventing a class for an unknown path (the config-authority model: the
// platform module rules are the authority for which paths exist; an override may
// only amend a known path's class, not mint one).
//
// Detectable via errors.As.
type UnknownPathError struct {
	// Path is the override path with no platform default.
	Path string
}

// Error implements the error interface.
func (e *UnknownPathError) Error() string {
	return fmt.Sprintf(
		"ownership override rejected for path %q: no platform-module default class "+
			"(unknown path). Overrides may only amend a path the platform modules "+
			"declare; add the path to the relevant module's ownership rules first.",
		e.Path,
	)
}

// IsUnknownPathError reports whether err (or any joined/wrapped error beneath
// it) is an *UnknownPathError.
func IsUnknownPathError(err error) bool {
	var u *UnknownPathError
	return errors.As(err, &u)
}

// NotHandOverridableError is returned when an override's FROM or TO class is off
// the raise/lower lattice (external_generated or local_only), or when an
// override attempts to set a path to an off-lattice class. Those classes are
// platform/provider/local-determined and are not expressible as ordinary
// hand-edited overrides (see package doc). Fail-closed, not a downgrade.
//
// Detectable via errors.As.
type NotHandOverridableError struct {
	// Path is the offending override path (may be "" when Compare is called
	// path-agnostically).
	Path string
	// Class is the off-lattice class involved (from or to).
	Class Class
	// Reason explains why the class is not hand-overridable.
	Reason string
}

// Error implements the error interface.
func (e *NotHandOverridableError) Error() string {
	path := e.Path
	if path == "" {
		path = "<path>"
	}
	reason := e.Reason
	if reason == "" {
		reason = "off-lattice class; not hand-overridable"
	}
	return fmt.Sprintf(
		"ownership override rejected for path %q: class %q is off the raise/lower "+
			"lattice (%s). external_generated and local_only classes are "+
			"platform/provider/local-determined and cannot be set or amended by an "+
			"ordinary harness-ownership.yml override.",
		path, e.Class, reason,
	)
}

// IsNotHandOverridableError reports whether err (or any joined/wrapped error
// beneath it) is a *NotHandOverridableError.
func IsNotHandOverridableError(err error) bool {
	var n *NotHandOverridableError
	return errors.As(err, &n)
}
