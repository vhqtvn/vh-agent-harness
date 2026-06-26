package substrate

import (
	"path/filepath"
	"strings"
)

// matchGlob reports whether path matches a glob pattern that supports both
// standard glob meta-characters (* and ?) and the recursive ** segment.
//
// Semantics:
//   - "**" matches zero or more complete path segments (e.g. ".opencode/agents/**"
//     matches ".opencode/agents", ".opencode/agents/build.md", and
//     ".opencode/agents/sub/a.md").
//   - "*" matches any run of characters within a single segment (no "/").
//   - "?" matches a single character within a segment.
//   - All other characters match literally. Paths use "/" separators.
//
// This is the glob the Classifier uses on top of the ownership package's
// exact-path EffectiveMap: spike1's harness-ownership.yml expresses rules as
// globs (e.g. ".opencode/agents/**"), which filepath.Match cannot handle. We
// keep the matcher dependency-free and focused on path-segment globs.
func matchGlob(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)
	// Special-case: a trailing "/**" matches the directory itself AND everything
	// beneath it. Normalize so the recursion sees a clean "**" at the end.
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		// Match if path == base, or path is under base/.
		if path == base {
			return true
		}
		if base == "" {
			return true // pattern was "/**" or "**" -> matches everything
		}
		if strings.HasPrefix(path, base+"/") {
			return true
		}
		return false
	}
	return globMatch(pattern, path)
}

// globMatch is the recursive matcher for patterns possibly containing "**" in
// the middle and single-segment "*"/"?".
func globMatch(pattern, path string) bool {
	// Tokenize on "/"; we match segment by segment so "*" never crosses "/".
	pSegs := strings.Split(pattern, "/")
	tSegs := strings.Split(path, "/")
	return segMatch(pSegs, tSegs)
}

func segMatch(pSegs, tSegs []string) bool {
	for len(pSegs) > 0 {
		if pSegs[0] == "**" {
			// "**" matches zero or more segments. Try consuming 0..len(tSegs)
			// remaining segments.
			if len(pSegs) == 1 {
				return true // trailing "**" matches everything left
			}
			for i := 0; i <= len(tSegs); i++ {
				if segMatch(pSegs[1:], tSegs[i:]) {
					return true
				}
			}
			return false
		}
		if len(tSegs) == 0 {
			return false
		}
		if !singleSegmentMatch(pSegs[0], tSegs[0]) {
			return false
		}
		pSegs = pSegs[1:]
		tSegs = tSegs[1:]
	}
	return len(tSegs) == 0
}

// singleSegmentMatch matches one segment, supporting * and ? but not "/".
func singleSegmentMatch(pat, s string) bool {
	// Reduce to classic wildcard matching over runes within a single segment.
	return wildMatch([]rune(pat), []rune(s))
}

func wildMatch(pat, s []rune) bool {
	for len(pat) > 0 {
		switch pat[0] {
		case '*':
			// Collapse consecutive '*'.
			for len(pat) > 0 && pat[0] == '*' {
				pat = pat[1:]
			}
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(s); i++ {
				if wildMatch(pat, s[i:]) {
					return true
				}
			}
			return false
		case '?':
			if len(s) == 0 {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		default:
			if len(s) == 0 || s[0] != pat[0] {
				return false
			}
			pat = pat[1:]
			s = s[1:]
		}
	}
	return len(s) == 0
}
