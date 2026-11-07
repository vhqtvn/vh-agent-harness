// crux_test.go — THE behavioral-closure crux for the file tool
// family: one logged turn executing write → read → edit → search →
// glob against real files in a t.TempDir root, every call through
// Pipeline.ExecuteLogged into a REAL durable session log, then
// session.ReplayFile → DeriveMessages reproducing the expected
// surface byte-identically. The confinement battery (../ escape,
// absolute-outside-root, symlink escape, rejected-write-leaves-no-
// trace) rides the same logged pipeline: denials are ordinary typed
// isError results and the log replays identically with them included.
package filetools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// TestFileFamilyLoggedTurnCrux is the load-bearing path: real log,
// real pipeline, real files, replay determinism.
func TestFileFamilyLoggedTurnCrux(t *testing.T) {
	root := t.TempDir()
	sessDir := t.TempDir()
	logPath := filepath.Join(sessDir, "sess-crux.jsonl")

	p := pipelineWith(t, Config{Roots: []string{root}})
	lg, err := session.OpenFile(logPath, "sess-crux")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := lg.AppendPrompt("file round-trip"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("AppendTurnBegin: %v", err)
	}

	type step struct {
		id, name, args string
	}
	steps := []step{
		{"c1", "write", `{"path":"docs/guide.md","content":"# Guide\nUse the widget API.\nSee also: examples.\n"}`},
		{"c2", "read", `{"path":"docs/guide.md"}`},
		{"c3", "edit", `{"path":"docs/guide.md","old":"widget API","new":"gadget API"}`},
		{"c4", "search", `{"pattern":"gadget","glob":"*.md"}`},
		{"c5", "glob", `{"pattern":"docs/*.md"}`},
	}
	var results []tools.Result
	for _, s := range steps {
		res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{
			ID: s.id, Name: s.name, Args: json.RawMessage(s.args),
		})
		if err != nil {
			t.Fatalf("ExecuteLogged(%s): %v", s.name, err)
		}
		if res.IsError {
			t.Fatalf("%s result = %+v, want success", s.name, res)
		}
		results = append(results, res)
	}

	// Outcomes are real, not just non-error:
	if !strings.HasPrefix(results[0].Content, "wrote "+filepath.Join(root, "docs", "guide.md")) {
		t.Fatalf("write result = %q", results[0].Content)
	}
	// Read ran BEFORE edit: the content still says "widget".
	if results[1].Content != "1: # Guide\n2: Use the widget API.\n3: See also: examples.\n" {
		t.Fatalf("read result = %q", results[1].Content)
	}
	if !strings.Contains(results[2].Content, "1 occurrence") ||
		!strings.Contains(results[2].Content, "\n-Use the widget API.\n") ||
		!strings.Contains(results[2].Content, "\n+Use the gadget API.\n") ||
		!strings.Contains(results[2].Content, "\n # Guide\n") {
		t.Fatalf("edit result = %q", results[2].Content)
	}
	if results[3].Content != "docs/guide.md:2: Use the gadget API.\n" {
		t.Fatalf("search result = %q", results[3].Content)
	}
	if results[4].Content != "docs/guide.md\n" {
		t.Fatalf("glob result = %q", results[4].Content)
	}

	if _, err := lg.AppendTurnEnd("stop"); err != nil {
		t.Fatalf("AppendTurnEnd: %v", err)
	}

	// Replay determinism: the persisted log reproduces the expected
	// surface byte-identically.
	events, err := session.ReplayFile(logPath)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	want := []session.Message{
		{Role: "user", Content: "file round-trip"},
	}
	for i, s := range steps {
		want = append(want, session.Message{
			Role:       "tool",
			Content:    results[i].Content,
			ToolCallID: s.id,
			Name:       s.name,
		})
	}
	gotB, _ := json.Marshal(msgs)
	wantB, _ := json.Marshal(want)
	if string(gotB) != string(wantB) {
		t.Fatalf("replayed surface differs:\ngot:  %s\nwant: %s", gotB, wantB)
	}

	// The file on disk is the EDITED version (the real effect).
	got, err := os.ReadFile(filepath.Join(root, "docs", "guide.md"))
	if err != nil || string(got) != "# Guide\nUse the gadget API.\nSee also: examples.\n" {
		t.Fatalf("final file = %q (err %v)", got, err)
	}
}

// TestFileFamilyLoggedDenialsCrux: the confinement battery THROUGH the
// logged pipeline — ../ escape, absolute-outside-root, symlink escape
// (a real symlink pointing outside), rejected-write-leaves-no-trace —
// every denial a typed isError result, zero durable side effects, and
// the log still replays byte-identically with the denials in it.
func TestFileFamilyLoggedDenialsCrux(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sessDir := t.TempDir()
	logPath := filepath.Join(sessDir, "sess-denials.jsonl")

	// Real symlink inside the root pointing outside.
	if err := os.WriteFile(filepath.Join(outside, "victim.txt"), []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape-hatch")); err != nil {
		t.Fatal(err)
	}

	p := pipelineWith(t, Config{Roots: []string{root}})
	lg, err := session.OpenFile(logPath, "sess-denials")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := lg.AppendPrompt("denials round-trip"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}

	denials := []struct {
		id, name, args, wantRule string
	}{
		{"d1", "read", `{"path":"../../etc/passwd"}`, "[escape]"},
		{"d2", "read", `{"path":"/etc/passwd"}`, "[outside-roots]"},
		{"d3", "read", `{"path":"escape-hatch/victim.txt"}`, "[symlink-escape]"},
		{"d4", "write", `{"path":"../newdir/deeper/file.txt","content":"x"}`, "[escape]"},
		{"d5", "write", `{"path":"escape-hatch/planted.txt","content":"x"}`, "[symlink-escape]"},
		{"d6", "write", `{"path":"` + filepath.Join(outside, "planted.txt") + `","content":"x"}`, "[outside-roots]"},
	}
	var results []tools.Result
	for _, d := range denials {
		res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{
			ID: d.id, Name: d.name, Args: json.RawMessage(d.args),
		})
		if err != nil {
			t.Fatalf("ExecuteLogged(%s %s): %v", d.name, d.id, err)
		}
		if !res.IsError {
			t.Fatalf("%s %s = %+v, want typed isError denial", d.name, d.id, res)
		}
		if !strings.Contains(res.Content, d.wantRule) {
			t.Fatalf("%s %s denial %q missing rule %s", d.name, d.id, res.Content, d.wantRule)
		}
		results = append(results, res)
	}

	// Rejected writes left NO trace: nothing new inside the root
	// beyond the symlink fixture, nothing outside, no parent dirs.
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != "escape-hatch" {
			t.Fatalf("rejected write left a trace in the root: %s", e.Name())
		}
	}
	outEntries, _ := os.ReadDir(outside)
	if len(outEntries) != 1 { // the victim.txt fixture only
		t.Fatalf("rejected writes reached outside the root: %+v", outEntries)
	}
	got, _ := os.ReadFile(filepath.Join(outside, "victim.txt"))
	if string(got) != "safe" {
		t.Fatalf("outside victim modified: %q", got)
	}

	// Replay with denials in the log is still deterministic and the
	// denials surface as tool messages with their typed text.
	events, err := session.ReplayFile(logPath)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	msgs, err := session.DeriveMessages(events)
	if err != nil {
		t.Fatalf("DeriveMessages: %v", err)
	}
	if len(msgs) != 1+len(denials) {
		t.Fatalf("surface = %d messages, want 1+%d", len(msgs), len(denials))
	}
	for i, d := range denials {
		m := msgs[1+i]
		if m.Role != "tool" || m.ToolCallID != d.id || m.Name != d.name || m.Content != results[i].Content {
			t.Fatalf("denial message %d = %+v, want %s/%s", i, m, d.id, d.name)
		}
	}
}
