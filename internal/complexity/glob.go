package complexity

import (
	"strings"
)

// matchAnyGlob reports whether a path matches ANY of the glob patterns. Patterns
// use the v1 complexity-policy exclusion grammar:
//
//   - "**" matches zero or more path segments (doublestar semantics).
//   - "*" matches any run of characters within a single segment.
//   - all other characters match literally.
//
// Examples:
//   - ".git/**"          matches ".git/x", ".git/a/b"
//   - ".opencode/**"     matches ".opencode/x", ".opencode/a/b"
//   - "**/__pycache__/**" matches "a/__pycache__/b", "__pycache__/b"
//   - "tmp/**"           matches "tmp/x", "tmp/a/b"
//
// This is a focused matcher for the curated v1 exclusion set; it is NOT a
// general-purpose glob engine.
func matchAnyGlob(path string, patterns []string) bool {
	for _, pat := range patterns {
		if globMatch(pat, path) {
			return true
		}
	}
	return false
}

// globMatch implements the v1 complexity glob grammar. It splits both the
// pattern and the path on "/" and matches segment-wise, where a "**" segment
// matches zero-or-more path segments.
func globMatch(pattern, name string) bool {
	patSegs := splitSegments(pattern)
	nameSegs := splitSegments(name)
	return matchSegments(patSegs, nameSegs)
}

func splitSegments(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "/")
}

// matchSegments is the recursive doublestar core. patSegs[i] == "**" consumes
// zero or more nameSegs.
func matchSegments(patSegs, nameSegs []string) bool {
	// Fast path: no more pattern segments -> match iff no more name segments.
	if len(patSegs) == 0 {
		return len(nameSegs) == 0
	}
	if patSegs[0] == "**" {
		// "**" matches zero segments: try matching the rest of the pattern at
		// every suffix of the name (including the empty suffix).
		rest := patSegs[1:]
		// Try consuming 0..len(nameSegs) segments.
		for i := 0; i <= len(nameSegs); i++ {
			if matchSegments(rest, nameSegs[i:]) {
				return true
			}
		}
		return false
	}
	// Non-"**" segment must consume exactly one name segment.
	if len(nameSegs) == 0 {
		return false
	}
	if !singleSegmentMatch(patSegs[0], nameSegs[0]) {
		return false
	}
	return matchSegments(patSegs[1:], nameSegs[1:])
}

// singleSegmentMatch matches one path segment against one pattern segment,
// where "*" matches any run of characters (but not "/"). No other special
// chars.
func singleSegmentMatch(pat, seg string) bool {
	// Simple "*" wildcard within a segment.
	return starMatch(pat, seg)
}

// starMatch implements a single-segment "*" glob (no cross-segment matching).
func starMatch(pat, str string) bool {
	pi, si := 0, 0
	star := -1
	match := 0
	for si < len(str) {
		if pi < len(pat) && pat[pi] == '*' {
			star = pi
			match = si
			pi++
		} else if pi < len(pat) && pat[pi] == str[si] {
			pi++
			si++
		} else if star != -1 {
			pi = star + 1
			match++
			si = match
		} else {
			return false
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat)
}
