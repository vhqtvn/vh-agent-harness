// e2e_subagent_recursion_test.go — the child-of-child crux: the
// MODEL-FACING subagent_spawn/subagent_send tool family, recursively.
//
//  1. TestDaemonSubagentRecursionDepth2 (CRUX): over the daemon's real
//     wire composition, the ROOT session's model calls subagent_spawn
//     (tool) → the child's model calls subagent_spawn (tool) → the
//     grandchild runs and settles → the child settles with its report →
//     all THREE logs replay byte-identically; folds equal live
//     snapshots; reports are user-role only.
//  2. TestDaemonSubagentRecursionFence: at the depth fence the
//     depth-maxed session's ADVERTISED tools exclude the family
//     (capability absence) and a hallucinated call still gets the typed
//     isError depth-fence refusal with zero durable effects.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// recursionLLM scripts the three-adapter-call chain and records each
// call's ADVERTISED tool names (the capability-absence assertion):
//
//	call 1 (root prompt):    content "root done" + subagent_spawn(child)
//	call 2 (child turn):     content "child studied with grandchild" +
//	                         subagent_spawn(grandchild)
//	call 3 (grandchild):     plain content "grandchild report text"
//	spare:                   plain "spare"
//
// In fenceMode call 3 instead returns content "grandchild at fence"
// PLUS a hallucinated subagent_spawn call the depth-maxed session was
// never advertised — proving the typed refusal path.
func recursionLLM(t *testing.T, fenceMode bool) (*httptest.Server, *[][]string) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	var toolsPerCall [][]string
	mk := func(content string, tool map[string]any, finish string) map[string]any {
		msg := map[string]any{"role": "assistant", "content": content}
		if tool != nil {
			msg["tool_calls"] = []map[string]any{tool}
		}
		return map[string]any{
			"id": "chatcmpl-rec", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": msg, "finish_reason": finish,
			}},
			"usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 2, "total_tokens": 4},
		}
	}
	spawnCall := func(id, prompt string) map[string]any {
		return map[string]any{
			"id": id, "type": "function",
			"function": map[string]any{
				"name":      "subagent_spawn",
				"arguments": `{"prompt":"` + prompt + `","mode":"oneshot"}`,
			},
		}
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var decoded struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		_ = json.Unmarshal(body, &decoded)
		names := make([]string, 0, len(decoded.Tools))
		for _, tl := range decoded.Tools {
			names = append(names, tl.Function.Name)
		}
		mu.Lock()
		calls++
		n := calls
		toolsPerCall = append(toolsPerCall, names)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			_ = json.NewEncoder(w).Encode(mk("root done", spawnCall("call-root", "child task"), "tool_calls"))
		case 2:
			_ = json.NewEncoder(w).Encode(mk("child studied with grandchild", spawnCall("call-child", "grandchild task"), "tool_calls"))
		case 3:
			if fenceMode {
				_ = json.NewEncoder(w).Encode(mk("grandchild at fence", spawnCall("call-fence", "one too deep"), "tool_calls"))
			} else {
				_ = json.NewEncoder(w).Encode(mk("grandchild report text", nil, "stop"))
			}
		default:
			_ = json.NewEncoder(w).Encode(mk("spare", nil, "stop"))
		}
	})), &toolsPerCall
}

// toolResultFor finds the tool/result payload for callID in events.
func toolResultFor(t *testing.T, events []session.Event, callID string) session.ToolResultPayload {
	t.Helper()
	for _, ev := range events {
		if ev.Type != session.TypeToolResult {
			continue
		}
		var tr session.ToolResultPayload
		if err := json.Unmarshal(ev.Payload, &tr); err != nil {
			t.Fatalf("tool result payload: %v", err)
		}
		if tr.CallID == callID {
			return tr
		}
	}
	t.Fatalf("no tool/result for %s in %d events", callID, len(events))
	return session.ToolResultPayload{}
}

// TestDaemonSubagentRecursionDepth2 is the child-of-child CRUX over the
// daemon seams: root model → subagent_spawn → child model →
// subagent_spawn → grandchild settles → child settles → root turn
// completes; three durable logs; replay + fold + provenance invariants.
func TestDaemonSubagentRecursionDepth2(t *testing.T) {
	llm, toolsPerCall := recursionLLM(t, false)
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)

	svc, cli := net.Pipe()
	defer cli.Close()
	srv, eng, _, _ := buildServer(cfg, "test-key", svc)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
		}
	}()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v", err)
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{
		"path": filepath.Join(cfg.SessionDir, "sess-rec.jsonl"), "sessionId": "sess-rec",
	}, nil))

	// ROOT TURN: the model's subagent_spawn tool call blocks through the
	// whole chain (child → grandchild → settle → settle) before the
	// prompt response returns.
	var turn struct {
		Content string `json:"content"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "delegate this down"}, &turn))
	if turn.Content != "root done" {
		t.Fatalf("root turn content = %q", turn.Content)
	}

	// Three durable logs at the composed store layout:
	//   sess-rec.jsonl, subagents/sess-rec/sess-rec.1.jsonl,
	//   subagents/sess-rec.1/sess-rec.1.1.jsonl (tree-unique ids).
	rootPath := filepath.Join(cfg.SessionDir, "sess-rec.jsonl")
	childPath := filepath.Join(cfg.SessionDir, "subagents", "sess-rec", "sess-rec.1.jsonl")
	gcPath := filepath.Join(cfg.SessionDir, "subagents", "sess-rec.1", "sess-rec.1.1.jsonl")
	for _, p := range []string{rootPath, childPath, gcPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("log missing: %s: %v", p, err)
		}
	}

	// CRUX REPLAY TRIPLE: all three logs replay byte-identically.
	rootEvents := assertReplayByteIdentical(t, rootPath, "root")
	childEvents := assertReplayByteIdentical(t, childPath, "child")
	gcEvents := assertReplayByteIdentical(t, gcPath, "grandchild")

	// Grandchild header: depth 2, parent sess-rec.1 (authoritative
	// depth propagation through the tool family).
	var gh session.HeaderPayload
	if err := json.Unmarshal(gcEvents[0].Payload, &gh); err != nil {
		t.Fatalf("grandchild header: %v", err)
	}
	if gh.SessionID != "sess-rec.1.1" || gh.ParentSessionID != "sess-rec.1" || gh.DelegationDepth != 2 {
		t.Fatalf("grandchild header = %+v", gh)
	}

	// The root's tool result carries the CHILD's report (the one-shot
	// contract), and the child's tool result carries the GRANDCHILD's.
	rootTR := toolResultFor(t, rootEvents, "call-root")
	if rootTR.IsError {
		t.Fatalf("root subagent_spawn isError: %s", rootTR.Content)
	}
	var rootOut struct {
		ChildID string `json:"childId"`
		Result  string `json:"result"`
		Report  string `json:"report"`
	}
	if err := json.Unmarshal([]byte(rootTR.Content), &rootOut); err != nil {
		t.Fatalf("root tool result not JSON: %q", rootTR.Content)
	}
	if rootOut.ChildID != "sess-rec.1" || rootOut.Result != "completed" || rootOut.Report != "child studied with grandchild" {
		t.Fatalf("root tool result = %+v", rootOut)
	}
	childTR := toolResultFor(t, childEvents, "call-child")
	if childTR.IsError {
		t.Fatalf("child subagent_spawn isError: %s", childTR.Content)
	}
	var childOut struct {
		Report string `json:"report"`
	}
	if err := json.Unmarshal([]byte(childTR.Content), &childOut); err != nil || childOut.Report != "grandchild report text" {
		t.Fatalf("child tool result = %q (%v)", childTR.Content, err)
	}

	// Fold == live: root fold equals the live root-manager snapshot
	// (resolved through the engine's registry — the same seam the tool
	// family resolves through).
	rootMgr, ok := eng.SubagentRegistry.Get("sess-rec")
	if !ok {
		t.Fatal("root manager not in the engine registry")
	}
	live := rootMgr.Snapshot()
	folded, err := subagents.FoldSubagents(rootEvents)
	if err != nil {
		t.Fatalf("fold root: %v", err)
	}
	if len(folded) != 1 || len(live) != 1 || folded[0] != live[0] {
		t.Fatalf("root fold %+v != live %+v", folded, live)
	}
	if live[0].SettledResult != "completed" || live[0].Depth != 1 {
		t.Fatalf("root live = %+v", live[0])
	}

	// Child fold == reconstructed-live (the child's manager is per-turn
	// by design; rebuilding it from the child log must agree with the
	// fold — the reconstruction seam).
	childFold, err := subagents.FoldSubagents(childEvents)
	if err != nil {
		t.Fatalf("fold child: %v", err)
	}
	childLog, err := session.ResumeFile(childPath)
	if err != nil {
		t.Fatalf("resume child: %v", err)
	}
	childMgr, err := subagents.NewManager(childLog, noopChildExecutor{}, eng.SubagentStore, eng.SubagentOpts)
	if err != nil {
		t.Fatalf("rebuild child manager: %v", err)
	}
	childLive := childMgr.Snapshot()
	childMgr.Stop()
	if len(childFold) != 1 || len(childLive) != 1 || childFold[0] != childLive[0] {
		t.Fatalf("child fold %+v != rebuilt live %+v", childFold, childLive)
	}
	if childLive[0].Depth != 2 || childLive[0].SettledResult != "completed" {
		t.Fatalf("child live = %+v", childLive[0])
	}

	// Reports are USER-role only, in both parent surfaces.
	assertReportUserRole(t, rootEvents, "sess-rec.1", "child studied with grandchild")
	assertReportUserRole(t, childEvents, "sess-rec.1.1", "grandchild report text")

	// Every session below the fence was ADVERTISED the family (root
	// call 1, child call 2, grandchild call 3 — depth 2 < 3).
	gotTools := *toolsPerCall
	if len(gotTools) != 3 {
		t.Fatalf("adapter calls = %d, want 3", len(gotTools))
	}
	for i, names := range gotTools {
		found := false
		for _, n := range names {
			if n == "subagent_spawn" {
				found = true
			}
		}
		if !found {
			t.Fatalf("call %d advertised tools = %v, want subagent_spawn present", i+1, names)
		}
	}
}

// assertReportUserRole proves the subagent/report landed in the parent
// surface as a USER-role message with the expected relay text and never
// as assistant words.
func assertReportUserRole(t *testing.T, parentEvents []session.Event, childID, content string) {
	t.Helper()
	msgs, err := session.DeriveMessages(parentEvents)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	want := "subagent " + childID + " report: " + content
	foundUser := false
	for _, m := range msgs {
		if m.Content == want {
			if m.Role == "user" {
				foundUser = true
			} else {
				t.Fatalf("report %q surfaced with role %q (want user only)", want, m.Role)
			}
		}
	}
	if !foundUser {
		t.Fatalf("user-role report %q missing from surface: %+v", want, msgs)
	}
}

// noopChildExecutor rebuilds nothing — the reconstruction test only
// needs the manager's fold/snapshot, never a live child turn.
type noopChildExecutor struct{}

func (noopChildExecutor) Run(ctx context.Context, child subagents.Child) error { return nil }

// TestDaemonSubagentRecursionFence: with maxDepth 2 the grandchild (at
// depth 2) is NOT advertised the family, and a hallucinated call still
// gets the typed isError depth-fence refusal with zero durable effects.
func TestDaemonSubagentRecursionFence(t *testing.T) {
	llm, toolsPerCall := recursionLLM(t, true)
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)

	svc, cli := net.Pipe()
	defer cli.Close()
	srv, eng, _, _ := buildServer(cfg, "test-key", svc)
	eng.SubagentOpts.MaxDelegationDepth = 2 // shrink the fence before any session
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
		}
	}()

	if err := client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := client.Call("session/create", map[string]any{
		"path": filepath.Join(cfg.SessionDir, "sess-fence.jsonl"), "sessionId": "sess-fence",
	}, nil); err != nil {
		t.Fatalf("session/create: %v", err)
	}
	var turn struct {
		Content string `json:"content"`
	}
	if err := client.Call("session/prompt", map[string]any{"text": "delegate to the fence"}, &turn); err != nil {
		t.Fatalf("session/prompt: %v", err)
	}
	if turn.Content != "root done" {
		t.Fatalf("root turn content = %q", turn.Content)
	}

	rootPath := filepath.Join(cfg.SessionDir, "sess-fence.jsonl")
	childPath := filepath.Join(cfg.SessionDir, "subagents", "sess-fence", "sess-fence.1.jsonl")
	gcPath := filepath.Join(cfg.SessionDir, "subagents", "sess-fence.1", "sess-fence.1.1.jsonl")

	// Call 3 (grandchild, depth 2 == maxDepth) was NOT advertised the
	// family — capability absence over refusal.
	gotTools := *toolsPerCall
	if len(gotTools) != 3 {
		t.Fatalf("adapter calls = %d, want 3", len(gotTools))
	}
	for _, n := range gotTools[2] {
		if n == "subagent_spawn" || n == "subagent_send" {
			t.Fatalf("depth-maxed session advertised %s: %v", n, gotTools[2])
		}
	}

	// The hallucinated call at the fence: typed isError refusal naming
	// the fence, in the GRANDCHILD's log (its model made the call); the
	// child still completes and reports.
	rootEvents := assertReplayByteIdentical(t, rootPath, "root")
	gcEvents := assertReplayByteIdentical(t, gcPath, "grandchild")
	tr := toolResultFor(t, gcEvents, "call-fence")
	if !tr.IsError {
		t.Fatalf("fence call result = %+v, want isError", tr)
	}
	if !strings.Contains(tr.Content, "depth fence") {
		t.Fatalf("fence text = %q, want the depth-fence refusal", tr.Content)
	}

	// Zero durable effects on refusal: the grandchild spawned nothing —
	// no subagent/spawned in its log, no great-grandchild directory.
	for _, ev := range gcEvents {
		if ev.Type == session.TypeSubagentSpawned {
			t.Fatalf("fence refusal left a spawned event: %+v", ev)
		}
	}
	if _, err := os.Stat(filepath.Join(cfg.SessionDir, "subagents", "sess-fence.1.1")); !os.IsNotExist(err) {
		t.Fatalf("great-grandchild store dir exists after fence refusal: %v", err)
	}

	// The chain itself still completed: child settled completed with
	// its report (the grandchild's fenced turn still produced content),
	// root fold shows the tree.
	childEvents := assertReplayByteIdentical(t, childPath, "child")
	assertReportUserRole(t, childEvents, "sess-fence.1.1", "grandchild at fence")
	assertReportUserRole(t, rootEvents, "sess-fence.1", "child studied with grandchild")
}
