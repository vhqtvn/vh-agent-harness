package substrate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/ownership"
)

// GlobRule is one S2 ownership rule expressed as a glob (spike1 style). The
// Classifier evaluates rules in deterministic (sorted) order; the first match
// wins. Exact-path EffectiveMap entries take precedence over globs.
type GlobRule struct {
	Pattern string
	Class   ownership.Class
	// Provenance names the module/overlay that declared the rule (for audit).
	Provenance string
}

// UnclassifiedPolicy controls how the Classifier treats a path that matches no
// exact entry and no glob rule. The seam must be fail-closed by default so a
// mis-authored ownership map never silently overwrites a file whose class is
// unknown.
type UnclassifiedPolicy int

const (
	// UnclassifiedFail rejects an unclassified path with an error. This is the
	// default and matches spike1's default_when_unclassified: reject.
	UnclassifiedFail UnclassifiedPolicy = iota
	// UnclassifiedAssume returns a caller-chosen class for unclassified paths
	// (e.g. assume platform_managed). Use only when the operator has opted in.
	UnclassifiedAssume
)

// Classifier resolves a repo-relative path to its effective armed ownership
// class. It is the seam's read view over S2: it consults the exact-path
// EffectiveMap (from ownership.Resolve, raise-only) first, then the ordered glob
// rules, then the unclassified policy.
//
// The Classifier is a read-only query surface; it never mutates the ownership
// map. Mutating overrides is a separate, gated path (ownership.Resolve +
// D2-A).
type Classifier struct {
	exact  ownership.EffectiveMap
	globs  []GlobRule
	policy UnclassifiedPolicy
	assume ownership.Class // used only when policy == UnclassifiedAssume
}

// Classified is the Classifier's answer for one path.
type Classified struct {
	Class      ownership.Class
	Provenance string
	// Origin is "exact" (EffectiveMap hit), "glob" (glob rule hit), or
	// "assume" (unclassified-assume fallback).
	Origin string
}

// NewClassifier builds a Classifier over an exact-path EffectiveMap and an
// optional set of glob rules. The glob rules are sorted deterministically
// (pattern, then class) so evaluation order is stable across runs; the first
// match wins. The default unclassified policy is fail-closed.
func NewClassifier(exact ownership.EffectiveMap, globs []GlobRule, opts ...ClassifierOption) *Classifier {
	c := &Classifier{exact: exact, globs: append([]GlobRule(nil), globs...), policy: UnclassifiedFail}
	for _, o := range opts {
		o(c)
	}
	// Deterministic glob order: longest pattern first (most specific), then
	// lexical. This makes "first match wins" the most-specific match.
	sort.SliceStable(c.globs, func(i, j int) bool {
		if len(c.globs[i].Pattern) != len(c.globs[j].Pattern) {
			return len(c.globs[i].Pattern) > len(c.globs[j].Pattern)
		}
		return c.globs[i].Pattern < c.globs[j].Pattern
	})
	return c
}

// ClassifierOption configures a Classifier.
type ClassifierOption func(*Classifier)

// WithUnclassifiedAssume makes the Classifier return `cls` for paths that match
// nothing, instead of failing closed. Use only with operator opt-in.
func WithUnclassifiedAssume(cls ownership.Class) ClassifierOption {
	return func(c *Classifier) {
		c.policy = UnclassifiedAssume
		c.assume = cls
	}
}

// Classify resolves a path to its effective class. It returns (result, true) on
// a definite answer (exact hit, glob hit, or assume-fallback) and (zero, false)
// when the path is unclassified and the policy is fail-closed.
func (c *Classifier) Classify(path string) (Classified, bool) {
	path = normPath(path)
	// 1. Exact-path EffectiveMap (raise-only ownership authority).
	if entry, ok := c.exact[path]; ok {
		return Classified{Class: entry.Class, Provenance: entry.Provenance, Origin: "exact"}, true
	}
	// 2. Glob rules (most-specific first).
	for _, g := range c.globs {
		if matchGlob(g.Pattern, path) {
			return Classified{Class: g.Class, Provenance: g.Provenance, Origin: "glob"}, true
		}
	}
	// 3. Unclassified policy.
	if c.policy == UnclassifiedAssume {
		return Classified{Class: c.assume, Provenance: "unclassified-assume", Origin: "assume"}, true
	}
	return Classified{}, false
}

// MustClassify is Classify that returns an error instead of ok=false. The seam
// uses this to surface unclassified (fail-closed) paths as a hard error.
func (c *Classifier) MustClassify(path string) (Classified, error) {
	got, ok := c.Classify(path)
	if !ok {
		return Classified{}, fmt.Errorf(
			"substrate: path %q is unclassified (no exact ownership entry and no glob rule); "+
				"fail-closed. Add the path to the platform module ownership rules or a glob rule.",
			path,
		)
	}
	return got, nil
}

// normPath canonicalizes a path for matching: forward slashes, no leading "./".
func normPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return p
}
