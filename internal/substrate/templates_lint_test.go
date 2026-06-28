package substrate

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness" // root package: embed vars corpus.CoreFS / corpus.CoreDir
)

// renderDirectiveRe matches template-author comment lines that read like
// instructions to the renderer ("strip … on render", "remove … on render",
// "the renderer will …"). The renderer is an allowlist-tight token pass and
// does NO comment-stripping/normalization (renderer.go SubstituteHarnessTokens),
// so any such directive ships LITERALLY into the consumer's repo. This is the
// exact trap that caused the original stale-CLAUDE.md bug (a comment like
// "# strip this on render" shipped verbatim). The regex is intentionally narrow:
// it targets directive phrasing, not the word "renderer" in general prose. See
// docs/ai/template-authoring.md for the principle.
var renderDirectiveRe = regexp.MustCompile(`(?i)(strip|remove|delete)\b.{0,40}\bon render\b|the renderer (will|strips?|removes?|deletes?)`)

// TestCorpus_NoRendererDirectiveComments is the W5/Q4 build-time guard. It scans
// EVERY embedded templates/core/ template for known-bad unenforceable
// conventions (comment lines that read like render directives) and FAILS the
// build if found. This is deliberately a narrow guard against the specific
// stale-CLAUDE.md trap — it is NOT a second renderer and performs no
// normalization. To extend coverage to a new footgun class, add a pattern here.
func TestCorpus_NoRendererDirectiveComments(t *testing.T) {
	coreSub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("fs.Sub(core): %v", err)
	}
	var violations []string
	walkErr := fs.WalkDir(coreSub, ".", func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		body, rerr := fs.ReadFile(coreSub, p)
		if rerr != nil {
			return nil
		}
		for i, line := range strings.Split(string(body), "\n") {
			if renderDirectiveRe.MatchString(line) {
				violations = append(violations, fmt.Sprintf("%s:%d: %s", p, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk core corpus: %v", walkErr)
	}
	if len(violations) > 0 {
		t.Fatalf("renderer-directed comment(s) in corpus — the renderer does NOT honor these and they will ship literally (the stale-CLAUDE.md footgun). Remove the directive; see docs/ai/template-authoring.md. %d violation(s):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}

// TestRenderDirectiveRegex_detectsKnownBad proves the guard above is meaningful
// (matches the canonical bad lines) and precise (does not flag benign prose that
// merely mentions the renderer or "render"). Without this, a typo in the regex
// could silently turn the lint into a no-op.
func TestRenderDirectiveRegex_detectsKnownBad(t *testing.T) {
	bad := []string{
		"# strip this line on render",
		"<!-- remove this block on render -->",
		"// the renderer will delete the following section",
		"The renderer strips comments, so this is safe.", // the original trap
		"# delete the marker on render",
	}
	good := []string{
		"Tokens resolve at render time.",
		"The renderer is an allowlist-tight token pass.",
		"// This is a normal doc comment for human readers.",
		"rendered files are written to the target tree",
		"# Build with: go build ./...",
	}
	for _, s := range bad {
		if !renderDirectiveRe.MatchString(s) {
			t.Errorf("regex should FLAG known-bad directive: %q", s)
		}
	}
	for _, s := range good {
		if renderDirectiveRe.MatchString(s) {
			t.Errorf("regex should NOT flag benign line (false positive): %q", s)
		}
	}
}
