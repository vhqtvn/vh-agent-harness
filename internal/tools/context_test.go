// context_test.go — the executing-session context binding: RunTurn
// stamps the executing log's session id into the context so model-facing
// tools (the subagent family) can resolve per-session state without a
// second pipeline.
package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TestRunTurnBindsExecutingSession drives one tool turn and proves the
// tool body sees the executing session id (and that Execute outside a
// turn sees none).
func TestRunTurnBindsExecutingSession(t *testing.T) {
	var sink writeBuffer
	lg := fixedLog(t, "sess-ctx-1", &sink)

	saw := make(chan string, 1)
	p := NewPipeline()
	if err := p.Register(ToolDefinition{
		Name: "probe", IsConcurrencySafe: true,
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			saw <- ExecutingSessionFrom(ctx)
			return "ok", nil
		},
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Outside a turn there is no binding.
	if got := ExecutingSessionFrom(context.Background()); got != "" {
		t.Fatalf("background ctx session = %q, want empty", got)
	}

	ad := &turnAdapter{resp: &adapters.Response{
		Model: "m", Content: "done",
		ToolCalls: []session.ToolCall{{ID: "c1", Name: "probe"}},
	}}
	if _, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{}, "go"); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	select {
	case got := <-saw:
		if got != "sess-ctx-1" {
			t.Fatalf("tool saw session %q, want sess-ctx-1", got)
		}
	default:
		t.Fatal("probe tool never executed")
	}
}
