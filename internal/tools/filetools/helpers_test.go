// helpers_test.go — shared test plumbing for the file tool family.
package filetools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// filePipeline builds the family pipeline over a fresh root with
// default bounds.
func filePipeline(t *testing.T) (*tools.Pipeline, string) {
	t.Helper()
	root := t.TempDir()
	return pipelineWith(t, Config{Roots: []string{root}}), root
}

// pipelineWith builds the family pipeline over an explicit Config.
func pipelineWith(t *testing.T, cfg Config) *tools.Pipeline {
	t.Helper()
	p := tools.NewPipeline()
	for _, d := range Definitions(cfg) {
		if err := p.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Name, err)
		}
	}
	return p
}

// writeFileT writes content at root/rel, creating parent directories.
func writeFileT(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// call builds a session.ToolCall.
func call(name, args string) session.ToolCall {
	return session.ToolCall{ID: "c-" + name, Name: name, Args: json.RawMessage(args)}
}

// runTool executes one call through the real pipeline.
func runTool(t *testing.T, p *tools.Pipeline, name, args string) tools.Result {
	t.Helper()
	return p.Execute(context.Background(), call(name, args))
}

// markerOffset extracts the resume cursor from a truncation marker
// ("... offset=N ...").
func markerOffset(t *testing.T, content string) int {
	t.Helper()
	i := strings.Index(content, "offset=")
	if i < 0 {
		t.Fatalf("no offset= in %q", content)
	}
	rest := content[i+len("offset="):]
	end := strings.IndexAny(rest, " ]")
	if end < 0 {
		end = len(rest)
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		t.Fatalf("offset= not numeric in %q: %v", content, err)
	}
	return n
}
