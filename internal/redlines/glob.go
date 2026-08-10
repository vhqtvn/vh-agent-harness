package redlines

import "strings"

// matchGlob matches a `/`-segmented name (a repo absolute path OR a normalized
// git remote like "github.com/owner/repo") against a single pattern using the
// same grammar the harness uses elsewhere (internal/complexity/glob.go):
//
//   - "**" matches zero or more path segments (doublestar semantics).
//   - "*" matches any run of characters within a single segment (not "/").
//   - all other characters match literally.
//
// It is intentionally a SELF-CONTAINED copy rather than an import of
// internal/complexity: redlines is a security-sensitive leaf and must not pull
// in the complexity-policy package for its matching primitive. The grammar is
// kept identical so operators can reuse one mental model.
//
// Examples:
//
//	matchGlob("/home/me/**",                  "/home/me/repo-a")      == true
//	matchGlob("/home/me/*",                   "/home/me/repo-a/sub")  == false (* does not cross /)
//	matchGlob("github.com/vhqtvn/*",          "github.com/vhqtvn/x")  == true
//	matchGlob("github.com/**/vh-agent-harness", "github.com/o/vh-agent-harness") == true
//	matchGlob("**",                           "/anything/here")       == true
func matchGlob(pattern, name string) bool {
	return matchSegments(splitSegments(pattern), splitSegments(name))
}

// matchAnyGlob reports whether name matches ANY of the patterns. An empty
// pattern slice never matches (callers use the empty slice to mean "all repos"
// at a higher level; that semantics is NOT encoded here).
func matchAnyGlob(name string, patterns []string) bool {
	for _, p := range patterns {
		if matchGlob(p, name) {
			return true
		}
	}
	return false
}

func splitSegments(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "/")
}

// matchSegments is the recursive doublestar core. A "**" segment consumes zero
// or more name segments; any other segment consumes exactly one and must match
// it via single-segment "*" globbing.
func matchSegments(patSegs, nameSegs []string) bool {
	if len(patSegs) == 0 {
		return len(nameSegs) == 0
	}
	if patSegs[0] == "**" {
		rest := patSegs[1:]
		for i := 0; i <= len(nameSegs); i++ {
			if matchSegments(rest, nameSegs[i:]) {
				return true
			}
		}
		return false
	}
	if len(nameSegs) == 0 {
		return false
	}
	if !starMatch(patSegs[0], nameSegs[0]) {
		return false
	}
	return matchSegments(patSegs[1:], nameSegs[1:])
}

// starMatch implements a single-segment "*" glob: "*" matches any run of
// characters except "/". No other special characters.
func starMatch(pat, s string) bool {
	pi, si := 0, 0
	star := -1
	match := 0
	for si < len(s) {
		if pi < len(pat) && pat[pi] == '*' {
			star = pi
			match = si
			pi++
		} else if pi < len(pat) && pat[pi] == s[si] {
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
