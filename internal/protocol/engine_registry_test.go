// engine_registry_test.go — the engine-side session→manager registry
// binding: root managers are registered at session create and dropped
// at supersede, so the model-facing subagent tools (which resolve
// through the registry) bind to the ACTIVE session.
package protocol

import (
	"path/filepath"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// TestEngineRegistersRootManagerInRegistry covers create-put and
// supersede-remove over the wire harness (the harness constructor has
// already created sess-sub when it returns).
func TestEngineRegistersRootManagerInRegistry(t *testing.T) {
	reg := subagents.NewRegistry()
	h := newSubagentHarnessOpts(t, &countingExecutor{}, subagents.Options{}, reg)
	defer h.close()

	m1, ok := reg.Get("sess-sub")
	if !ok || m1 == nil {
		t.Fatal("root manager not registered after session create")
	}

	// Supersede: a second session drops the first session's binding.
	log2 := filepath.Join(h.dir, "sess-sub-2.jsonl")
	if err := h.client.Call("session/create", map[string]any{
		"path": log2, "sessionId": "sess-sub-2",
	}, nil); err != nil {
		t.Fatalf("session/create 2: %v", err)
	}
	if _, ok := reg.Get("sess-sub"); ok {
		t.Fatal("superseded session still registered")
	}
	if _, ok := reg.Get("sess-sub-2"); !ok {
		t.Fatal("new session manager not registered")
	}
}

// TestEngineWithoutRegistryKeepsSubagents proves the registry is an
// optional seam: a nil registry changes no subagent behavior (the wire
// family works exactly as before — engines that never arm the
// model-facing tools pay nothing).
func TestEngineWithoutRegistryKeepsSubagents(t *testing.T) {
	h := newSubagentHarness(t, &countingExecutor{}, subagents.Options{})
	defer h.close()
	if err := h.client.Call("session/create", map[string]any{
		"path": h.logPath, "sessionId": "sess-sub",
	}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	var out struct {
		Children []subagents.Status `json:"children"`
	}
	if err := h.client.Call("subagent/list", nil, &out); err != nil {
		t.Fatalf("subagent/list: %v", err)
	}
	if len(out.Children) != 0 {
		t.Fatalf("children = %+v, want empty", out.Children)
	}
}
