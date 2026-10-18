// glob.go — the glob tool: stdlib filepath.Match pattern matching
// under the configured roots. Semantics documented HONESTLY: '*'
// matches any sequence of NON-separator characters only (no '**'
// recursion — compose directory components explicitly, e.g.
// "sub/*.go"), '?' matches one non-separator character, character
// classes [...] work, and dotfiles are not special. The walk never
// follows symlinks (WalkDir Lstat semantics): a symlinked directory
// inside a root is listed by its own name when it matches but never
// descended.
package filetools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// GlobName is the registered tool name.
const GlobName = "glob"

// globParametersSchema is the adapter-facing argument description.
const globParametersSchema = `{"type":"object","properties":{` +
	`"pattern":{"type":"string","description":"glob pattern matched against paths RELATIVE to each workdir root with stdlib filepath.Match semantics: '*' and '?' do not cross '/' (no '**' recursion — write 'sub/*.go' for one level down), '[...]' classes work, dotfiles are not special"}}` +
	`,"required":["pattern"],"additionalProperties":false}`

const globDescription = "Lists files and directories matching a glob pattern under the workdir roots, one path per line, sorted, relative to the containing root. " +
	"stdlib filepath.Match semantics: '*' and '?' never cross '/', there is no '**' (compose directory components explicitly), dotfiles are visible to '*'. " +
	"Symlinked directories are never followed. Results are bounded (500 default) with an explicit overflow marker. Read-only and concurrency-safe."

// globArgs is the typed argument surface.
type globArgs struct {
	Pattern string `json:"pattern"`
}

// globDefinition builds the glob ToolDefinition.
func globDefinition(cfg *Config, roots Roots) tools.ToolDefinition {
	max := cfg.MaxGlobResults // normalized by Definitions
	return tools.ToolDefinition{
		Name:              GlobName,
		Description:       globDescription,
		Parameters:        json.RawMessage(globParametersSchema),
		IsConcurrencySafe: true,
		TimeoutMs:         30000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a globArgs
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.pattern is required and must be non-empty", GlobName)
			}
			if err := decodeToolArgs(raw, &a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", GlobName, err)
			}
			if a.Pattern == "" {
				return "", fmt.Errorf("%s: args.pattern is required and must be non-empty", GlobName)
			}
			if _, err := filepath.Match(a.Pattern, "probe"); err != nil {
				return "", fmt.Errorf("%s: malformed pattern %q: %v", GlobName, a.Pattern, err)
			}
			return globExecute(roots, max, a.Pattern)
		},
	}
}

// globExecute walks every root and matches rel paths against the
// pattern. rel "." (the root itself) is never listed.
func globExecute(roots Roots, max int, pattern string) (string, error) {
	var matches []string
	total := 0
	overflow := false
	for _, root := range roots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", fmt.Errorf("%s: workdir root %s unresolved: %v", GlobName, root, err)
		}
		err = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				// A subtree that cannot be read is skipped, not fatal:
				// the tool still reports what it CAN see.
				return nil
			}
			rel, rerr := filepath.Rel(real, p)
			if rerr != nil || rel == "." {
				return nil
			}
			if ok, merr := filepath.Match(pattern, rel); merr == nil && ok {
				total++
				if len(matches) < max {
					matches = append(matches, rel)
				} else {
					overflow = true
				}
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("%s: walking %s: %v", GlobName, root, err)
		}
	}
	sort.Strings(matches)
	if len(matches) == 0 && !overflow {
		return "(no matches)\n", nil
	}
	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m + "\n")
	}
	if overflow {
		fmt.Fprintf(&sb, "[glob: %d of %d matches shown — refine the pattern]\n", len(matches), total)
	}
	return sb.String(), nil
}

// walkFiles yields every REGULAR file under every root as
// (absolutePath, relToContainingRoot), depth-first, never following
// symlinks (shared by glob's sibling search). Skips unreadable
// subtrees silently-but-visibly: the walk continues.
func walkFiles(roots Roots) []fileEntry {
	var out []fileEntry
	for _, root := range roots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		_ = filepath.WalkDir(real, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.Type().IsRegular() {
				if rel, rerr := filepath.Rel(real, p); rerr == nil {
					out = append(out, fileEntry{abs: p, rel: rel})
				}
			}
			return nil
		})
	}
	return out
}

// fileEntry is one walked file.
type fileEntry struct {
	abs string
	rel string
}
