// write.go — the write tool: atomic create/truncate (temp file in the
// SAME directory + fsync + rename) with parent directories created
// ONLY when the resolved parent sits inside a root. Every confinement
// rejection happens in confine.resolveWrite BEFORE any filesystem
// effect.
package filetools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// WriteName is the registered tool name.
const WriteName = "write"

// writeParametersSchema is the adapter-facing argument description.
const writeParametersSchema = `{"type":"object","properties":{` +
	`"path":{"type":"string","description":"file path to create or truncate: relative paths resolve against the first workdir root; absolute paths must be under a configured root; escaping paths and symlink crossings are rejected with zero filesystem effects"},` +
	`"content":{"type":"string","description":"full file content to write (the file is replaced atomically: temp file + fsync + rename; missing parent directories are created only inside the roots)"}}` +
	`,"required":["path","content"],"additionalProperties":false}`

const writeDescription = "Creates or truncates a file with the given content and returns the absolute path plus bytes written. " +
	"The write is atomic (temp file in the target directory, fsync, rename) and missing parent directories are created — but only when the resolved parent is inside the configured workdir roots. " +
	"Paths are confined symlink-safe (fail-closed; rejected writes leave no trace, not even created directories). Not concurrency-safe: runs as an exclusive barrier."

// writeArgs is the typed argument surface.
type writeArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// writeDefinition builds the write ToolDefinition.
func writeDefinition(cfg *Config, roots Roots) tools.ToolDefinition {
	return tools.ToolDefinition{
		Name:              WriteName,
		Description:       writeDescription,
		Parameters:        json.RawMessage(writeParametersSchema),
		IsConcurrencySafe: false, // mutates the tree: exclusive barrier
		TimeoutMs:         10000,
		Execute: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var a writeArgs
			if len(raw) == 0 {
				return "", fmt.Errorf("%s: args.path is required and must be non-empty", WriteName)
			}
			if err := decodeToolArgs(raw, &a); err != nil {
				return "", fmt.Errorf("%s: invalid args: %w", WriteName, err)
			}
			if a.Path == "" {
				return "", fmt.Errorf("%s: args.path is required and must be non-empty", WriteName)
			}
			return writeExecute(roots, a)
		},
	}
}

// writeExecute confines, prepares parents, and atomically writes.
func writeExecute(roots Roots, a writeArgs) (string, error) {
	abs, _, err := roots.resolveWrite(a.Path)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", fmt.Errorf("%s: create parent directories %s: %v", WriteName, parent, err)
	}
	if err := atomicWriteFile(abs, []byte(a.Content), 0o644); err != nil {
		return "", fmt.Errorf("%s: %v", WriteName, err)
	}
	return fmt.Sprintf("wrote %s (%d bytes)\n", abs, len(a.Content)), nil
}

// atomicWriteFile writes data to path ATOMICALLY: a temp file in the
// SAME directory (same-filesystem rename), fsync, then rename over
// the target. The spill store and scheduler state file use the same
// pattern; restated here so the file family is self-contained.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".filetools-write-*")
	if err != nil {
		return fmt.Errorf("temp file in %s: %v", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %v", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("fsync temp file: %v", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %v", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %v", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename into place: %v", err)
	}
	return nil
}
