// search.go — the search tool: stdlib regexp content search across
// every regular file under the roots (never following symlinks), with
// an optional basename glob filter (filepath.Match semantics) and a
// `path:LN: line` result shape bounded by an explicit overflow
// marker. Malformed regexes and filters are typed errors.
package filetools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// SearchName is the registered tool name.
const SearchName = "search"

// searchParametersSchema is the adapter-facing argument description.
const searchParametersSchema = `{"type":"object","properties":{` +
	`"pattern":{"type":"string","description":"Go regexp (RE2 syntax) searched in file CONTENTS across the workdir roots"},` +
	`"glob":{"type":"string","description":"optional filename filter matched against each file's BASE NAME with stdlib filepath.Match semantics ('*.go' matches any depth because it filters names, not paths)"}}` +
	`,"required":["pattern"],"additionalProperties":false}`

const searchDescription = "Searches file contents across the workdir roots with a Go regexp (RE2) and returns `path:LN: line` matches, paths relative to their root, bounded (200 matches default) with an explicit overflow marker. " +
	"Optional glob filters by base name (filepath.Match semantics). Binary-safe enough for text work: files are scanned line-wise; symlinked directories are never followed. Read-only and concurrency-safe."

// searchArgs is the typed argument surface.
type searchArgs struct {
	Pattern string `json:"pattern"`
	Glob    string `json:"glob"`
}

// searchDefinition builds the search ToolDefinition.
func searchDefinition(cfg *Config, roots Roots) tools.ToolDefinition {
	max := cfg.MaxSearchMatches // normalized by Definitions
	return tools.ToolDefinition{
		Name:              SearchName,
		Description:       searchDescription,
		Parameters:        json.RawMessage(searchParametersSchema),
		IsConcurrencySafe: true,
		TimeoutMs:         30000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a searchArgs
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.pattern is required and must be non-empty", SearchName)
			}
			if err := decodeToolArgs(raw, &a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", SearchName, err)
			}
			if a.Pattern == "" {
				return "", fmt.Errorf("%s: args.pattern is required and must be non-empty", SearchName)
			}
			re, err := regexp.Compile(a.Pattern)
			if err != nil {
				return "", fmt.Errorf("%s: malformed regex %q: %v", SearchName, a.Pattern, err)
			}
			if a.Glob != "" {
				if _, err := filepath.Match(a.Glob, "probe"); err != nil {
					return "", fmt.Errorf("%s: malformed glob filter %q: %v", SearchName, a.Glob, err)
				}
			}
			return searchExecute(roots, max, re, a.Glob)
		},
	}
}

// searchExecute scans every regular file under the roots. A file that
// cannot be opened or read is skipped with a visible note (the search
// still reports what it can see) — a per-file failure must not eat
// the whole result. The WHOLE tree is scanned even after the bound is
// reached so the overflow marker's total is accurate (the bound caps
// the RESULT, not the scan).
func searchExecute(roots Roots, max int, re *regexp.Regexp, globFilter string) (string, error) {
	var sb strings.Builder
	total := 0
	shown := 0
	for _, fe := range walkFiles(roots) {
		if globFilter != "" {
			if ok, err := filepath.Match(globFilter, filepath.Base(fe.rel)); err != nil || !ok {
				continue
			}
		}
		if err := scanFileMatches(fe.abs, re, func(rel string, lineNo int, line string) {
			total++
			if shown < max {
				fmt.Fprintf(&sb, "%s:%d: %s\n", rel, lineNo, line)
				shown++
			}
		}, fe.rel); err != nil {
			fmt.Fprintf(&sb, "%s: [unreadable: %v]\n", fe.rel, err)
		}
	}
	if total == 0 {
		return "(no matches)\n", nil
	}
	if total > shown {
		fmt.Fprintf(&sb, "[search: %d of %d matches shown — refine the pattern]\n", shown, total)
	}
	return sb.String(), nil
}

// scanFileMatches streams one file line-wise, invoking emit per
// matching line.
func scanFileMatches(path string, re *regexp.Regexp, emit func(rel string, lineNo int, line string), rel string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxScanLine)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if re.MatchString(sc.Text()) {
			emit(rel, lineNo, sc.Text())
		}
	}
	return sc.Err()
}
