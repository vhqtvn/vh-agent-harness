// spill_test.go — the commit-time oversize spill integration: the
// pipeline consults the session log's armed SpillPolicy when a tool
// result is COMMITTED (single ExecuteLogged and the batch phase-3 path),
// rewriting oversize content to a bounded preview + notice and stamping
// the payload's spill fields. Small results stay byte-identical with
// and without a policy (library default = today's behavior), replay is
// byte-identical, and replay never depends on the spill files.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// spillLog arms a log with a real FileSpillStore policy under dir.
func spillLog(t *testing.T, dir, id string, sink *writeBuffer, maxInline int64) *session.Log {
	t.Helper()
	lg, err := session.NewLog(sink, id, time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	lg.SetSpillPolicy(&session.SpillPolicy{
		MaxInlineBytes: maxInline,
		Store:          session.NewFileSpillStore(dir, id),
	})
	return lg
}

// toolResultPayloads decodes every tool/result payload from the log.
func toolResultPayloads(t *testing.T, lg *session.Log) []session.ToolResultPayload {
	t.Helper()
	var out []session.ToolResultPayload
	for _, ev := range lg.Events() {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var p session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("payload: %v", err)
		}
		out = append(out, p)
	}
	return out
}

// TestSpillAtCommitExecuteLogged: a single oversize result is rewritten
// at commit — payload carries Spilled + SpillLocator, the returned
// Result (what the wire and the turn report see) carries the preview,
// and the store holds the FULL content hash-validated.
func TestSpillAtCommitExecuteLogged(t *testing.T) {
	dir := t.TempDir()
	var sink writeBuffer
	lg := spillLog(t, dir, "sess-commit", &sink, 4096)

	full := strings.Repeat("s", 5000)
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "big_echo", IsConcurrencySafe: true,
		Execute: func(context.Context, json.RawMessage) (string, error) {
			return full, nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	res, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{ID: "c1", Name: "big_echo"})
	if err != nil {
		t.Fatalf("ExecuteLogged: %v", err)
	}
	if res.IsError {
		t.Fatalf("result errored: %+v", res)
	}
	if len(res.Content) > 4096 {
		t.Fatalf("returned result content = %d bytes, must be the bounded preview", len(res.Content))
	}
	if !strings.Contains(res.Content, "[spilled 5000 bytes:") || !strings.Contains(res.Content, "read via spill_read") {
		t.Fatalf("preview missing the notice: %q", res.Content[:minInt(200, len(res.Content))])
	}

	trs := toolResultPayloads(t, lg)
	if len(trs) != 1 {
		t.Fatalf("tool results = %d", len(trs))
	}
	tr := trs[0]
	if !tr.Spilled || tr.SpillLocator == nil {
		t.Fatalf("payload not marked spilled: %+v", tr)
	}
	back, err := session.NewFileSpillStore(dir, "sess-commit").Read(*tr.SpillLocator)
	if err != nil || string(back) != full {
		t.Fatalf("spill readback = %d bytes err=%v, want the FULL content", len(back), err)
	}
}

// TestSpillByteStabilityWithoutPolicyAndForSmallResults: the library
// default (no policy) and a policy-armed log with SMALL results produce
// byte-identical logs — no event-shape change without omitempty-gated
// spill fields.
func TestSpillByteStabilityWithoutPolicyAndForSmallResults(t *testing.T) {
	runOne := func(armed bool) []byte {
		t.Helper()
		var sink writeBuffer
		var lg *session.Log
		if armed {
			lg = spillLog(t, t.TempDir(), "sess-stab", &sink, 4096)
		} else {
			var err error
			lg, err = session.NewLog(&sink, "sess-stab", time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("NewLog: %v", err)
			}
		}
		p := NewPipeline()
		if err := p.Register(ToolDefinition{
			Name: "small", IsConcurrencySafe: true,
			Execute: func(context.Context, json.RawMessage) (string, error) {
				return "small result", nil
			},
		}); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if _, err := p.ExecuteLogged(context.Background(), lg, session.ToolCall{ID: "c1", Name: "small"}); err != nil {
			t.Fatalf("ExecuteLogged: %v", err)
		}
		return sink.data
	}
	bare, armed := runOne(false), runOne(true)
	if !bytes.Equal(bare, armed) {
		t.Fatalf("small-result logs differ between no-policy and armed:\nbare:  %s\narmed: %s", bare, armed)
	}
	if bytes.Contains(bare, []byte("spilled")) {
		t.Fatalf("small result must not carry spill fields: %s", bare)
	}
}

// TestSpillBatchReplayDeterministicAndFileIndependent (requirement 5 +
// the crux shape): a turn whose tool result exceeds the threshold logs
// a spilled event with preview + locator; replaying the persisted bytes
// is byte-identical; DELETING the spill files changes nothing about
// replay (spill files are sidecar state — loss degrades retrieval, not
// replay integrity).
func TestSpillBatchReplayDeterministicAndFileIndependent(t *testing.T) {
	dir := t.TempDir()
	var sink writeBuffer
	lg := spillLog(t, dir, "sess-batch", &sink, 2048)

	full := strings.Repeat("b", 3000)
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "big", IsConcurrencySafe: true,
		Execute: func(context.Context, json.RawMessage) (string, error) {
			return full, nil
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ad := &turnAdapter{resp: &adapters.Response{
		Model: "mock-1", FinishReason: "tool_calls",
		ToolCalls: []session.ToolCall{{ID: "call_big", Name: "big"}},
	}}
	report, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "mock-1", Tools: specsOf(p)}, "run the big one")
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(report.Results) != 1 || len(report.Results[0].Content) > 2048 {
		t.Fatalf("report results = %+v", report.Results)
	}

	trs := toolResultPayloads(t, lg)
	if len(trs) != 1 || !trs[0].Spilled || trs[0].SpillLocator == nil {
		t.Fatalf("payloads = %+v", trs)
	}

	// The spilled preview+notice IS the surface message (locator visible
	// to the model through the log alone).
	replayDeterminism(t, &sink, lg)

	// Byte-identical replay with the spill files GONE: fold the same
	// events, derive the same surface, no file access.
	spillDir := filepath.Join(dir, "sess-batch.spill")
	if err := os.RemoveAll(spillDir); err != nil {
		t.Fatalf("remove spill dir: %v", err)
	}
	replayed, err := session.Replay(bytes.NewReader(sink.data))
	if err != nil {
		t.Fatalf("Replay after spill-file loss: %v", err)
	}
	msgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages after spill-file loss: %v", err)
	}
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Content, "[spilled 3000 bytes:") {
		t.Fatalf("replayed surface lost the spilled preview: %+v", msgs)
	}

	// Retrieval after loss fails closed (retrieval, not replay, degrades).
	if _, err := session.ReadSpillUnder(dir, *trs[0].SpillLocator); err == nil {
		t.Fatal("spill retrieval after file loss must fail closed")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
