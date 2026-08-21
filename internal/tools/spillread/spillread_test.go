// spillread_test.go — the spill_read tool: the model-facing path back
// to spilled bytes. Retrieval goes through session.ReadSpillUnder (the
// daemon-wide walk over per-session stores), is hash-validated
// fail-closed, and caps the returned bytes with an in-band truncation
// notice (run_shell marker style).
package spillread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

func execDef(t *testing.T, root string, maxRead int64, args string) (string, error) {
	t.Helper()
	def := Definition(root, maxRead)
	out, err := def.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		return "", err
	}
	if def.Name != Name || def.IsConcurrencySafe != true || def.TimeoutMs <= 0 {
		t.Fatalf("definition posture wrong: %+v", def)
	}
	return out, nil
}

func TestSpillReadRetrievesFullContentHashValidated(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-tool")
	content := strings.Repeat("R", 100000)
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, err := json.Marshal(map[string]any{"locator": loc})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	out, err := execDef(t, root, 0, string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != content {
		t.Fatalf("retrieved %d bytes, want the full %d", len(out), len(content))
	}
}

func TestSpillReadFailClosed(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-tool2")
	content := []byte("spilled payload")
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Tampered store file: hash mismatch refuses.
	tampered := loc
	tampered.SHA256 = strings.Repeat("0", 64)
	args, _ := json.Marshal(map[string]any{"locator": tampered})
	if _, err := execDef(t, root, 0, string(args)); err == nil {
		t.Fatal("tampered locator must fail closed")
	}

	// Unknown file: refuses.
	ghost := session.SpillLocator{File: "sp-9999999999999999", SHA256: loc.SHA256, Size: loc.Size}
	args, _ = json.Marshal(map[string]any{"locator": ghost})
	if _, err := execDef(t, root, 0, string(args)); err == nil {
		t.Fatal("unknown spill file must fail closed")
	}

	// Locator without a hash: refuses (nothing to validate against).
	noHash := loc
	noHash.SHA256 = ""
	args, _ = json.Marshal(map[string]any{"locator": noHash})
	if _, err := execDef(t, root, 0, string(args)); err == nil {
		t.Fatal("hashless locator must fail closed")
	}

	// Bad args shapes.
	for _, bad := range []string{`{}`, `{"locator":{"file":"x"}}`, `{"nope":1}`, `not json`, ``} {
		if _, err := execDef(t, root, 0, bad); err == nil {
			t.Fatalf("args %q must fail", bad)
		}
	}
}

func TestSpillReadCapsOutputWithNotice(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-cap")
	content := []byte(strings.Repeat("C", 3<<20)) // 3 MiB > the 1 MiB default
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"locator": loc})
	out, err := execDef(t, root, 0, string(args)) // maxRead 0 ⇒ default 1 MiB
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	wantTail := fmt.Sprintf("\n[spill_read: output truncated, %d bytes dropped (cap %dB)]", len(content)-DefaultMaxReadBytes, DefaultMaxReadBytes)
	// The cap bounds the CONTENT; the notice rides on top of it (the
	// same posture as run_shell's in-band truncation marker).
	if len(out) != DefaultMaxReadBytes+len(wantTail) {
		t.Fatalf("output = %d bytes, want capped content %d + notice %d", len(out), DefaultMaxReadBytes, len(wantTail))
	}
	if !strings.HasSuffix(out, wantTail) {
		t.Fatalf("output must end with the truncation notice, got tail %q", out[len(out)-120:])
	}
	if !strings.HasPrefix(out, string(content[:DefaultMaxReadBytes])) {
		t.Fatal("truncated output must be the FIRST bytes of the content")
	}
}

// TestSpillReadThroughPipeline drives the definition through the real
// pipeline (guards apply — an unknown tool is refused like any other).
func TestSpillReadThroughPipeline(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-pipe")
	content := []byte("through the pipeline")
	loc, err := s.Write("", content)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"locator": loc})

	p := tools.NewPipeline()
	if err := p.Register(Definition(root, 0)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: Name, Args: args})
	if res.IsError || res.Content != string(content) {
		t.Fatalf("pipeline result = %+v", res)
	}

	// Unknown tool: the pipeline's typed refusal (spill_read is not special).
	res = p.Execute(context.Background(), session.ToolCall{ID: "c2", Name: "spill_read_typo", Args: args})
	if !res.IsError {
		t.Fatalf("unknown tool must error: %+v", res)
	}
}
