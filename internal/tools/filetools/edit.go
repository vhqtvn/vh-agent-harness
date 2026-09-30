// edit.go — the edit tool: EXACT old→new string replacement on a
// confined file. No-match and non-unique-without-replaceAll are typed
// isError results (never panics, never a partial write); the file is
// written back atomically and the result carries a unified-diff-style
// snippet (±1 line of context) around the FIRST change plus the
// occurrence count.
package filetools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// EditName is the registered tool name.
const EditName = "edit"

// editParametersSchema is the adapter-facing argument description.
const editParametersSchema = `{"type":"object","properties":{` +
	`"path":{"type":"string","description":"file path to edit: relative paths resolve against the first workdir root; absolute paths must be under a configured root (symlink-safe, fail-closed)"},` +
	`"old":{"type":"string","description":"the EXACT text to replace (must be non-empty; must occur exactly once unless replaceAll is true)"},` +
	`"new":{"type":"string","description":"the replacement text"},` +
	`"replaceAll":{"type":"boolean","description":"replace every occurrence instead of requiring a unique match (default false)"}}` +
	`,"required":["path","old","new"],"additionalProperties":false}`

const editDescription = "Replaces an EXACT substring in a file (no fuzzy matching, no regex) and returns a unified-diff-style snippet of the change plus the occurrence count. " +
	"The old string must appear exactly once, or exactly nowhere-fails: zero matches and multiple-matches-without-replaceAll are typed errors and the file is left untouched. " +
	"The write-back is atomic (temp + fsync + rename). Not concurrency-safe: runs as an exclusive barrier."

// editArgs is the typed argument surface.
type editArgs struct {
	Path       string `json:"path"`
	Old        string `json:"old"`
	New        string `json:"new"`
	ReplaceAll bool   `json:"replaceAll"`
}

// editDefinition builds the edit ToolDefinition.
func editDefinition(cfg *Config, roots Roots) tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:              EditName,
		Description:       editDescription,
		Parameters:        json.RawMessage(editParametersSchema),
		IsConcurrencySafe: false, // mutates the tree: exclusive barrier
		TimeoutMs:         10000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a editArgs
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.path is required and must be non-empty", EditName)
			}
			if err := decodeToolArgs(raw, &a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", EditName, err)
			}
			if a.Path == "" {
				return "", fmt.Errorf("%s: args.path is required and must be non-empty", EditName)
			}
			if a.Old == "" {
				return "", fmt.Errorf("%s: args.old string is required and must be non-empty (to clear a file use write)", EditName)
			}
			return editExecute(roots, a)
		},
	}
}

// editExecute runs the exact-replacement contract.
func editExecute(roots Roots, a editArgs) (string, error) {
	abs, _, err := roots.resolveRead(a.Path)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %v", EditName, err)
	}
	content := string(raw)
	n := strings.Count(content, a.Old)
	if n == 0 {
		return "", fmt.Errorf("%s: old string not found in %s", EditName, a.Path)
	}
	if n > 1 && !a.ReplaceAll {
		return "", fmt.Errorf("%s: old string has %d matches in %s; make it unique or pass replaceAll:true", EditName, n, a.Path)
	}
	replaced := strings.Replace(content, a.Old, a.New, 1)
	if a.ReplaceAll {
		replaced = strings.ReplaceAll(content, a.Old, a.New)
	}
	if err := atomicWriteFile(abs, []byte(replaced), 0o644); err != nil {
		return "", fmt.Errorf("%s: %v", EditName, err)
	}
	noun := "occurrence"
	if n != 1 {
		noun = "occurrences"
	}
	return fmt.Sprintf("replaced %d %s in %s\n%s", n, noun, abs, diffSnippet(content, replaced, a.Old, a.New)), nil
}

// diffSnippet renders a minimal unified-diff-style hunk (±1 context
// line) around the FIRST difference between before and after. The
// span is derived from the old string's position; when old == new the
// block still renders (a no-op change shown honestly).
//
// Result honesty (why this must be total): the caller invokes this
// AFTER atomicWriteFile has already committed the edit. A panic here
// once turned a LANDED shrinking replaceAll into an isError "tool
// panicked" result — the mutation succeeded while the tool claimed
// failure, and a retry then failed "old string not found". Rendering
// is presentation, never validation: it cannot fail the edit.
func diffSnippet(before, after, old, new string) string {
	idx := strings.Index(before, old)
	if idx < 0 {
		// Unreachable via editExecute (n >= 1 checked); defensive.
		idx = 0
	}
	// lineOfByte(upTo) = 1-based line containing byte upTo. It clamps
	// BOTH ends: this renderer runs post-commit (see above), so it
	// must be total over any int — an out-of-range span degrades to
	// the nearest line instead of panicking.
	lineOf := func(s string, upTo int) int {
		if upTo < 0 {
			upTo = 0
		}
		if upTo > len(s) {
			upTo = len(s)
		}
		return strings.Count(s[:upTo], "\n") + 1
	}
	startLine := lineOf(before, idx)
	endLine := lineOf(before, idx+len(old)) - 1 // line of the LAST byte
	if endLine < startLine {
		endLine = startLine
	}
	// The after block starts at the same byte offset (everything
	// before idx is unchanged) and ends where the FIRST replacement's
	// inserted text does: after[idx:idx+len(new)] is exactly the first
	// replacement. Deliberately NOT derived from len(after)-len(before):
	// under replaceAll that total delta folds in EVERY occurrence's
	// shrink/growth, which both smears later occurrences into the
	// first hunk's span and goes negative when the shrink is large
	// enough (the post-commit panic this fix removes). The hunk
	// documents the FIRST change only; later occurrences are later
	// hunks this minimal snippet does not claim to render.
	afterStart := lineOf(after, idx)
	afterEnd := lineOf(after, idx+len(new)) - 1
	if afterEnd < afterStart {
		afterEnd = afterStart
	}

	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	const ctx = 1
	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	bStart := clamp(startLine-ctx, 1, len(beforeLines))
	bEnd := clamp(endLine+ctx, 1, len(beforeLines))
	aStart := clamp(afterStart-ctx, 1, len(afterLines))
	aEnd := clamp(afterEnd+ctx, 1, len(afterLines))

	var sb strings.Builder
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", bStart, bEnd-bStart+1, aStart, aEnd-aStart+1)
	for i := bStart - 1; i < startLine-1; i++ {
		sb.WriteString(" " + beforeLines[i] + "\n")
	}
	for i := startLine - 1; i < endLine; i++ {
		sb.WriteString("-" + beforeLines[i] + "\n")
	}
	for i := afterStart - 1; i < afterEnd; i++ {
		sb.WriteString("+" + afterLines[i] + "\n")
	}
	for i := endLine; i < bEnd; i++ {
		sb.WriteString(" " + beforeLines[i] + "\n")
	}
	return sb.String()
}
