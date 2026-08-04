package corpus

import (
	"bytes"
	"testing"
)

// TestCommitReviewerSeatsByteIdentical guards the load-bearing property that
// the four commit-reviewer seat prompts (a/b/c/d) never silently drift apart.
// Apart from each leaf's distinct `description:` frontmatter line, the four
// embedded sources must be byte-identical.
//
// This is a deliberate drift-prevention test, chosen over a generator (which
// would add maintenance machinery for four rarely-changing files) and over a
// renderer change (no per-invocation payoff). If a future edit changes one seat
// without the others, this test fails and surfaces the divergence.
func TestCommitReviewerSeatsByteIdentical(t *testing.T) {
	seats := []string{"a", "b", "c", "d"}
	base := "templates/core/.opencode/agents/commit-reviewer-"
	stripped := make([][]byte, len(seats))
	for i, s := range seats {
		b, err := CoreFS.ReadFile(base + s + ".md")
		if err != nil {
			t.Fatalf("read commit-reviewer-%s: %v", s, err)
		}
		stripped[i] = stripDescriptionLine(b)
	}
	for i := 1; i < len(stripped); i++ {
		if !bytes.Equal(stripped[0], stripped[i]) {
			t.Errorf("commit-reviewer-%s drifts from commit-reviewer-%s after stripping the description: line", seats[i], seats[0])
		}
	}
}

// stripDescriptionLine drops the single frontmatter line whose first column is
// `description:`. It is anchored to line start, so indented body occurrences
// (e.g. `    - evidence.description:`) are preserved and the only stripped line
// is the seat-specific frontmatter description.
func stripDescriptionLine(b []byte) []byte {
	lines := bytes.Split(b, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	for _, ln := range lines {
		if bytes.HasPrefix(ln, []byte("description:")) {
			continue
		}
		out = append(out, ln)
	}
	return bytes.Join(out, []byte("\n"))
}
