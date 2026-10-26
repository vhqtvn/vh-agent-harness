// e2e_subagent_test.go — slice B2 crux tests: the daemon's REAL
// subagent executor. Three levels:
//
//  1. TestChildProcessSubagentOneshot (CRUX): the actual binary — a
//     parent session spawns a SEEDED one-shot child over the wire; the
//     child runs a REAL turn (openaicompat adapter → fake LLM → echo
//     tool through the daemon pipeline); the child's report lands in
//     the parent surface as a user-role message; BOTH logs replay
//     byte-identically.
//  2. TestDaemonSubagentFoldMatchesLiveSnapshot: in-process
//     buildServer composition — the durable fold of the parent log
//     equals the live manager Snapshot() after a real child turn.
//  3. TestDaemonSubagentContinuableOverWire: in-process wire — two
//     sends ⇒ a batched real turn each (three completed child turns).
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// capturedRequest is one fake-LLM call: the decoded chat messages plus
// the call ordinal.
type capturedRequest struct {
	n       int
	role    string
	content string
}

// subagentLLM scripts the parent and child adapter calls:
//
//	call 1 (parent session/prompt): plain content "parent context ready"
//	call 2 (child turn): content "child studied the repo" + an echo
//	  tool call — content AND tool_calls in one message (the daemon
//	  path supports both) — then spare plain content.
//
// It records every request's last message for seed-flow assertions.
func subagentLLM(t *testing.T) (*httptest.Server, *[]capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	var reqs []capturedRequest
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("fake llm read: %v", err)
			return
		}
		var decoded struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		last := capturedRequest{}
		if err := json.Unmarshal(body, &decoded); err == nil && len(decoded.Messages) > 0 {
			m := decoded.Messages[len(decoded.Messages)-1]
			last.role, last.content = m.Role, m.Content
		}
		mu.Lock()
		calls++
		n := calls
		reqs = append(reqs, capturedRequest{n: n, role: last.role, content: last.content})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-parent", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0, "message": map[string]any{
						"role": "assistant", "content": "parent context ready",
					},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5},
			})
			return
		}
		if n == 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "chatcmpl-child", "object": "chat.completion", "model": "fake-model",
				"choices": []map[string]any{{
					"index": 0, "message": map[string]any{
						"role":    "assistant",
						"content": "child studied the repo",
						"tool_calls": []map[string]any{{
							"id": "call-sub", "type": "function",
							"function": map[string]any{"name": "echo", "arguments": `{"text":"hello-subagent"}`},
						}},
					},
					"finish_reason": "tool_calls",
				}},
				"usage": map[string]any{"prompt_tokens": 4, "completion_tokens": 3, "total_tokens": 7},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-spare", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{"role": "assistant", "content": "spare"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	})), &reqs
}

// assertReplayByteIdentical replays path and proves every event
// re-marshals to exactly its own file line.
func assertReplayByteIdentical(t *testing.T, path, what string) []session.Event {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s log: %v", what, err)
	}
	lg, err := session.ResumeFile(path)
	if err != nil {
		t.Fatalf("resume %s log: %v", what, err)
	}
	events := lg.Events()
	var rebuilt []byte
	for _, ev := range events {
		b, merr := json.Marshal(ev)
		if merr != nil {
			t.Fatalf("re-marshal %s seq %d: %v", what, ev.Seq, merr)
		}
		rebuilt = append(rebuilt, b...)
		rebuilt = append(rebuilt, '\n')
	}
	if !bytes.Equal(raw, rebuilt) {
		t.Fatalf("%s replay not byte-identical:\nfile:    %s\nrebuilt: %s", what, raw, rebuilt)
	}
	return events
}

// waitFor polls cond until true or the deadline, failing with label.
func waitFor(t *testing.T, label string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", label)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestChildProcessSubagentOneshot is the B2 CRUX at the real-binary
// level: spawn a seeded one-shot over the wire → the child runs a real
// adapter+tool turn inside the daemon → its report lands in the parent
// surface (user-role) → both logs replay byte-identically.
func TestChildProcessSubagentOneshot(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and spawns the daemon binary")
	}

	llm, reqs := subagentLLM(t)
	defer llm.Close()

	bin := filepath.Join(t.TempDir(), "vh-agentd")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vh-agentd")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	sessDir := filepath.Join(t.TempDir(), "sessions")
	logPath := filepath.Join(sessDir, "sess-b2.jsonl")
	cmd := exec.Command(bin,
		"--adapter", "openai",
		"--model", "fake-model",
		"--base-url", llm.URL,
		"--api-key-env", "VH_AGENTD_TEST_KEY",
		"--session-dir", sessDir,
	)
	cmd.Env = append(os.Environ(), "VH_AGENTD_TEST_KEY=k")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var childStderr syncBuf
	cmd.Stderr = &childStderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	client := protocol.NewClient(childPipes{stdout.(io.ReadCloser), stdin.(io.WriteCloser)})

	rec := &eventRecorder{}
	client.OnNotification("session/event", func(params json.RawMessage) {
		var ev struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(params, &ev); err == nil {
			rec.add(ev.Type)
		}
	})

	waitExit := make(chan error, 1)
	go func() { waitExit <- cmd.Wait() }()
	defer func() {
		_ = client.Close()
		if cmd.ProcessState != nil {
			return
		}
		select {
		case <-waitExit:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			t.Errorf("child did not exit; stderr:\n%s", childStderr.String())
		}
	}()

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("call: %v (child stderr:\n%s)", err, childStderr.String())
		}
	}
	must(client.Call("initialize", map[string]any{"protocolVersion": 1}, nil))
	must(client.Call("session/create", map[string]any{"path": logPath, "sessionId": "sess-b2"}, nil))
	must(client.Call("session/subscribe", nil, nil))

	// Parent turn first (call 1): gives the seed material.
	var turn struct {
		Content string `json:"content"`
	}
	must(client.Call("session/prompt", map[string]any{"text": "parent asks for a study"}, &turn))
	if turn.Content != "parent context ready" {
		t.Fatalf("parent turn content = %q", turn.Content)
	}

	// Spawn the SEEDED one-shot child (call 2): receipt immediate.
	var spawn struct {
		ChildID string `json:"childId"`
	}
	must(client.Call("subagent/spawn", map[string]any{
		"role": "researcher", "prompt": "study the repo", "mode": "oneshot", "seedFromParent": 1,
	}, &spawn))
	if spawn.ChildID != "sess-b2.1" {
		t.Fatalf("childId = %q, want sess-b2.1", spawn.ChildID)
	}

	// The child settles (report precedes settlement on the stream).
	waitFor(t, "subagent settle", func() bool { return rec.has("subagent/settled") })
	if !rec.has("subagent/report") {
		t.Fatalf("subagent/report missing from stream: %v", rec.snapshot())
	}

	// The report landed in the parent SURFACE as a user-role message.
	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	must(client.Call("session/surface", nil, &surf))
	wantReport := "subagent " + spawn.ChildID + " report: child studied the repo"
	found := false
	for _, m := range surf.Messages {
		if m.Role == "user" && m.Content == wantReport {
			found = true
		}
	}
	if !found {
		t.Fatalf("report %q not in parent surface: %+v", wantReport, surf.Messages)
	}

	// List folds the durable truth: one settled completed child.
	var list struct {
		Children []subagents.Status `json:"children"`
	}
	must(client.Call("subagent/list", nil, &list))
	if len(list.Children) != 1 || list.Children[0].State != "settled" ||
		list.Children[0].SettledResult != "completed" || list.Children[0].ContentSeq == 0 {
		t.Fatalf("subagent/list = %+v", list.Children)
	}

	// The seeded context actually reached the adapter: the child call's
	// LAST message is the inbox prompt, and the request BEFORE it
	// carried the parent's seeded turn (asserted via the recorded
	// last-message contents).
	waitFor(t, "2 adapter calls", func() bool { return len(*reqs) >= 2 })
	if (*reqs)[1].role != "user" || (*reqs)[1].content != "study the repo" {
		t.Fatalf("child adapter call last message = %+v, want the inbox prompt", (*reqs)[1])
	}

	// Close ladder → clean exit.
	if err := stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	select {
	case werr := <-waitExit:
		if werr != nil {
			t.Fatalf("child exit error: %v (stderr:\n%s)", werr, childStderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("child did not exit after EOF (stderr:\n%s)", childStderr.String())
	}

	// CRUX REPLAY PAIR: parent and child logs both replay
	// byte-identically.
	parentEvents := assertReplayByteIdentical(t, logPath, "parent")
	childPath := filepath.Join(sessDir, "subagents", "sess-b2", spawn.ChildID+".jsonl")
	childEvents := assertReplayByteIdentical(t, childPath, "child")

	// Child log shape: header, seed pair (prompt+response), inbox
	// message, then the child's own inbox-driven turn (NO second
	// session/prompt) with the real echo round-trip.
	want := []string{
		session.TypeSessionHeader,
		session.TypeSessionPrompt, // seed: the parent's prompt
		session.TypeLLMResponse,   // seed: the parent's reply
		session.TypeSubagentMessage,
		session.TypeTurnBegin,
		session.TypeLLMRequest,  // the child's adapter call
		session.TypeLLMResponse, // content + echo tool call
		session.TypeToolCall,
		session.TypeToolResult,
		session.TypeTurnEnd,
	}
	if len(childEvents) != len(want) {
		t.Fatalf("child events = %d, want %d: %v", len(childEvents), len(want), childEvents)
	}
	for i, w := range want {
		if childEvents[i].Type != w {
			t.Fatalf("child event[%d] = %s, want %s", i, childEvents[i].Type, w)
		}
	}
	var tr session.ToolResultPayload
	if err := json.Unmarshal(childEvents[8].Payload, &tr); err != nil {
		t.Fatalf("tool result payload: %v", err)
	}
	if tr.Content != "hello-subagent" {
		t.Fatalf("echo result = %q, want hello-subagent", tr.Content)
	}

	// The parent fold survives cold replay (report user-role included).
	msgs, err := session.DeriveMessages(parentEvents)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	found = false
	for _, m := range msgs {
		if m.Role == "user" && m.Content == wantReport {
			found = true
		}
	}
	if !found {
		t.Fatalf("report missing from replayed parent surface: %+v", msgs)
	}
}

// TestDaemonSubagentFoldMatchesLiveSnapshot: in-process composition —
// after a REAL child turn (fake LLM through the real adapter), the fold
// of the durable parent log equals the live manager Snapshot() and the
// wire-shaped FoldStatus.
func TestDaemonSubagentFoldMatchesLiveSnapshot(t *testing.T) {
	llm, _ := subagentLLM(t)
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)

	svc, cli := net.Pipe()
	defer cli.Close()
	_, eng, _, _ := buildServer(cfg, "test-key", svc)

	es, err := eng.NewSession(filepath.Join(cfg.SessionDir, "sess-fold.jsonl"), "sess-fold", io.Discard)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if es.Subagents == nil {
		t.Fatal("subagent surface not wired by buildServer")
	}
	rec, err := es.Subagents.SpawnWithOptions(subagents.SpawnOptions{
		Kind: session.SubagentKindOneShot, Prompt: "study quietly", Role: "researcher",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Live: wait for the one-shot to settle on the LIVE snapshot.
	waitFor(t, "live settle", func() bool {
		for _, d := range es.Subagents.Snapshot() {
			if d.ChildID == rec.ChildID && d.State == "settled" {
				return true
			}
		}
		return false
	})

	live := es.Subagents.Snapshot()
	if len(live) != 1 {
		t.Fatalf("live snapshot = %+v", live)
	}
	folded, err := subagents.FoldSubagents(es.Log.Events())
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	if len(folded) != 1 {
		t.Fatalf("fold = %+v", folded)
	}
	if folded[0] != live[0] {
		t.Fatalf("fold %+v != live %+v", folded[0], live[0])
	}
	if live[0].SettledResult != "completed" || live[0].Prompt != "study quietly" || live[0].Depth != 1 {
		t.Fatalf("live descriptor = %+v", live[0])
	}
	status, err := subagents.FoldStatus(es.Log.Events())
	if err != nil {
		t.Fatalf("fold status: %v", err)
	}
	if len(status) != 1 || status[0].ChildID != live[0].ChildID || status[0].State != "settled" || status[0].ContentSeq == 0 {
		t.Fatalf("wire status = %+v", status)
	}
	es.Subagents.Stop()
}

// TestDaemonSubagentContinuableOverWire: in-process wire — a
// continuable child runs one REAL turn per send: two sends ⇒ three
// completed child turns, the child stays waiting, and every turn is a
// genuine adapter call (distinct scripted content per call).
func TestDaemonSubagentContinuableOverWire(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-cont", "object": "chat.completion", "model": "fake-model",
			"choices": []map[string]any{{
				"index": 0, "message": map[string]any{
					"role": "assistant", "content": "ack-" + string(rune('0'+n)),
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		})
	}))
	defer llm.Close()
	cfg := testConfig(t, "openai", llm.URL)

	svc, cli := net.Pipe()
	srv, _, _, _ := buildServer(cfg, "test-key", svc)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := protocol.NewClient(cli)
	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
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
		"path": filepath.Join(cfg.SessionDir, "sess-cont.jsonl"), "sessionId": "sess-cont",
	}, nil))

	var spawn struct {
		ChildID string `json:"childId"`
	}
	must(client.Call("subagent/spawn", map[string]any{
		"prompt": "stand by for tasks", "mode": "continuable",
	}, &spawn))

	countTurnEnds := func() int {
		lg, err := session.ResumeFile(filepath.Join(cfg.SessionDir, "subagents", "sess-cont", spawn.ChildID+".jsonl"))
		if err != nil {
			return -1
		}
		n := 0
		for _, ev := range lg.Events() {
			if ev.Type == session.TypeTurnEnd {
				n++
			}
		}
		return n
	}
	waitFor(t, "initial child turn", func() bool { return countTurnEnds() == 1 })

	for i, msg := range []string{"task one", "task two"} {
		var send struct {
			Queued bool `json:"queued"`
		}
		must(client.Call("subagent/send", map[string]any{
			"childId": spawn.ChildID, "message": msg,
		}, &send))
		if !send.Queued {
			t.Fatalf("send[%d] not queued", i)
		}
		want := i + 2
		waitFor(t, "child turn after send", func() bool { return countTurnEnds() == want })
	}

	// Three completed turns, child still waiting (settlement is
	// manager-owned, not send-driven).
	if n := countTurnEnds(); n != 3 {
		t.Fatalf("child turns = %d, want 3", n)
	}
	var list struct {
		Children []subagents.Status `json:"children"`
	}
	must(client.Call("subagent/list", nil, &list))
	if len(list.Children) != 1 || list.Children[0].State != "waiting" {
		t.Fatalf("list = %+v, want one waiting child", list.Children)
	}
	if list.Children[0].ContentSeq == 0 {
		t.Fatalf("no report relayed for continuable child: %+v", list.Children[0])
	}
}
