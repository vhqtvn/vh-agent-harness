// read.go — the read tool: numbered-lines rendering of a confined
// file with offset/limit line windows and a byte-capped output
// (run_shell's cap + in-band marker pattern, restated for file
// content).
package filetools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// ReadName is the registered tool name.
const ReadName = "read"

// readParametersSchema is the adapter-facing argument description.
const readParametersSchema = `{"type":"object","properties":{` +
	`"path":{"type":"string","description":"file path to read: relative paths resolve against the first workdir root; absolute paths must be under a configured root; paths escaping the roots or crossing symlinks out of them are rejected"},` +
	`"offset":{"type":"integer","description":"number of leading lines to skip (default 0; a truncation marker names the resume cursor)"},` +
	`"limit":{"type":"integer","description":"maximum number of lines to return (default 0 = no line limit; the byte cap still applies)"}}` +
	`,"required":["path"],"additionalProperties":false}`

const readDescription = "Reads a text file and returns it as 1-based numbered lines (`LN: content`). " +
	"Optional offset skips leading lines and limit caps the line count; output is byte-capped (64KiB default) and a `[read: truncated … offset=N]` marker names the resume cursor when cut. " +
	"Paths are confined to the configured workdir roots (symlink-safe, fail-closed). Read-only and concurrency-safe."

// readArgs is the typed argument surface.
type readArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// readDefinition builds the read ToolDefinition.
func readDefinition(cfg *Config, roots Roots) tools.ToolDefinition {
	cap := cfg.MaxReadBytes // normalized by Definitions
	return tools.ToolDefinition{
		Name:              ReadName,
		Description:       readDescription,
		Parameters:        json.RawMessage(readParametersSchema),
		IsConcurrencySafe: true,
		TimeoutMs:         10000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a readArgs
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.path is required and must be non-empty", ReadName)
			}
			if err := decodeToolArgs(raw, &a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", ReadName, err)
			}
			if a.Path == "" {
				return "", fmt.Errorf("%s: args.path is required and must be non-empty", ReadName)
			}
			if a.Offset < 0 {
				return "", fmt.Errorf("%s: offset must be >= 0 (got %d)", ReadName, a.Offset)
			}
			if a.Limit < 0 {
				return "", fmt.Errorf("%s: limit must be >= 0 (got %d)", ReadName, a.Limit)
			}
			return readExecute(roots, cap, a)
		},
	}
}

// maxScanLine bounds one line for the scanner (long-line guard): the
// read cap bounds the OUTPUT, so a single line may exceed it; lines
// beyond this bound are a typed error instead of a silent split.
const maxScanLine = 4 * 1024 * 1024

// readExecute renders the confined file as numbered lines under the
// byte cap. Truncation (byte cap or line limit reached with more data
// pending) appends an explicit marker naming the resume cursor — the
// 1-based number of the first line NOT shown (for a byte-cut line,
// the partially shown line itself).
func readExecute(roots Roots, byteCap int64, a readArgs) (string, error) {
	abs, _, err := roots.resolveRead(a.Path)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %v", ReadName, err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("%s: %s is a directory (list it with glob instead)", ReadName, a.Path)
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", fmt.Errorf("%s: %v", ReadName, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxScanLine)

	var out bytes.Buffer
	shown := 0
	truncated := false
	lineNo := 0 // 1-based file line number of the line just scanned
	for sc.Scan() {
		lineNo++
		if lineNo <= a.Offset {
			continue // before the window
		}
		if a.Limit > 0 && shown >= a.Limit {
			truncated = true // line-limit window closed with data pending
			break
		}
		rendered := fmt.Sprintf("%d: %s\n", lineNo, sc.Text())
		if int64(out.Len())+int64(len(rendered)) > byteCap {
			if out.Len() == 0 {
				// First rendered line alone exceeds the cap: include
				// it byte-cut so the call still shows something; the
				// resume cursor points at this (partially shown) line.
				out.WriteString(rendered[:byteCap])
			}
			truncated = true
			break
		}
		out.WriteString(rendered)
		shown++
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("%s: scanning %s: %v (lines longer than %d bytes are refused, not split)", ReadName, a.Path, err, maxScanLine)
	}
	if truncated {
		// A byte-cut line leaves no trailing newline — separate the
		// marker from content with one.
		if b := out.Bytes(); len(b) > 0 && b[len(b)-1] != '\n' {
			out.WriteByte('\n')
		}
		// Resume cursor: offset=N skips lines 1..N and starts at N+1,
		// so the cursor is the LAST line already consumed. For a
		// between-lines cut that is lineNo-1 (lineNo is the first
		// UNSEEN line); for a byte-cut line it is lineNo-1 as well
		// (that line is re-read whole).
		out.WriteString(fmt.Sprintf("[read: truncated — resume with offset=%d]\n", lineNo-1))
	}
	return out.String(), nil
}

// decodeToolArgs decodes strict JSON args (unknown fields rejected —
// the daemon tool convention).
func decodeToolArgs(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return fmt.Errorf("args are required")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}
