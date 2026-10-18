// seed_test.go — fork turn-prefix seeding (B2): the child log receives
// EXACTLY the parent's last-n COMPLETED turns' surface messages (errored
// brackets excluded), before the initial prompt; the seeded log replays
// byte-identically; and the legacy no-seed spawn stays byte-identical to
// the pre-B2 spawned record (no seedTurns field).
package subagents

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// appendParentTurn appends one completed parent turn (prompt +
// assistant reply) and returns nothing — the durable shape
// session/prompt handlers produce.
func appendParentTurn(t *testing.T, lg *session.Log, prompt, reply string) {
	t.Helper()
	if _, err := lg.AppendTurnBegin(); err != nil {
		t.Fatalf("turn/begin: %v", err)
	}
	if _, err := lg.AppendPrompt(prompt); err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if _, err := lg.AppendLLMResponse("test-model", reply, nil, session.Usage{TotalTokens: 1}); err != nil {
		t.Fatalf("response: %v", err)
	}
	if _, err := lg.AppendTurnEnd(""); err != nil {
		t.Fatalf("turn/end: %v", err)
	}
}

// parentEventsOfTypes returns the parent's events filtered to the seed
// vocabulary, for expected-seed construction.
func parentEventsOfTypes(events []session.Event, types ...string) []session.Event {
	want := map[string]bool{}
	for _, t := range types {
		want[t] = true
	}
	var out []session.Event
	for _, ev := range events {
		if want[ev.Type] {
			out = append(out, ev)
		}
	}
	return out
}

func TestSpawnSeedsLastCompletedTurns(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "sess-seed", store, &echoExecutor{}, Options{})
	appendParentTurn(t, parent, "first task", "first reply")
	appendParentTurn(t, parent, "second task", "second reply")
	// An ERRORED bracket after the completed ones: NOT seed material.
	if _, err := parent.AppendTurnBegin(); err != nil {
		t.Fatalf("bad turn/begin: %v", err)
	}
	if _, err := parent.AppendPrompt("doomed task"); err != nil {
		t.Fatalf("bad prompt: %v", err)
	}
	if _, err := parent.AppendTurnEndKind("error", "adapter down"); err != nil {
		t.Fatalf("bad turn/end: %v", err)
	}

	rec, err := m.SpawnWithOptions(SpawnOptions{
		Kind: session.SubagentKindOneShot, Prompt: "child task", SeedFromParent: 2,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	m.Drain()

	child := store.logs["sess-seed/"+rec.ChildID]
	evs := child.Events()
	if len(evs) == 0 || evs[0].Type != session.TypeSessionHeader {
		t.Fatalf("child log has no header")
	}
	// Expected seeds: the message events of BOTH completed turns in log
	// order — 4 events (2 prompts + 2 replies). The ERRORED bracket's
	// prompt is excluded by span selection (it is not inside any
	// completed turn), so it is dropped from the expectation too.
	var wantSeeds []session.Event
	for _, ev := range parentEventsOfTypes(parent.Events(), session.TypeSessionPrompt, session.TypeLLMResponse) {
		if bytes.Contains(ev.Payload, []byte("doomed task")) {
			continue
		}
		wantSeeds = append(wantSeeds, ev)
	}
	if len(wantSeeds) != 4 {
		t.Fatalf("test construction: expected 4 seedable parent message events, got %d", len(wantSeeds))
	}

	i := 1
	for _, w := range wantSeeds {
		if evs[i].Type != w.Type {
			t.Fatalf("seed[%d] type = %s, want %s", i, evs[i].Type, w.Type)
		}
		if !bytes.Equal(evs[i].Payload, w.Payload) {
			t.Fatalf("seed[%d] payload not verbatim:\n got %s\nwant %s", i, evs[i].Payload, w.Payload)
		}
		i++
	}
	// After the seeds: the initial prompt inbox message, then the turn
	// the echo executor ran.
	if evs[i].Type != session.TypeSubagentMessage {
		t.Fatalf("event after seeds = %s, want subagent/message", evs[i].Type)
	}
	var mp session.SubagentPayload
	if err := json.Unmarshal(evs[i].Payload, &mp); err != nil {
		t.Fatalf("message payload: %v", err)
	}
	if mp.Text != "child task" {
		t.Fatalf("initial prompt = %q, want %q", mp.Text, "child task")
	}

	// The spawned descriptor records the number of seeded turns.
	var sp session.SubagentPayload
	if err := json.Unmarshal(parent.Events()[len(parent.Events())-3].Payload, &sp); err != nil {
		t.Fatalf("spawned payload: %v", err)
	}
	// find the spawned event robustly (report/settle follow it)
	var seeded int = -1
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSpawned {
			if err := json.Unmarshal(ev.Payload, &sp); err != nil {
				t.Fatalf("spawned payload: %v", err)
			}
			seeded = sp.SeedTurns
		}
	}
	if seeded != 2 {
		t.Fatalf("seedTurns = %d, want 2", seeded)
	}
	_ = mp
}

func TestSpawnSeedMoreThanAvailableSeedsAll(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "sess-seed1", store, &echoExecutor{}, Options{})
	appendParentTurn(t, parent, "only task", "only reply")

	rec, err := m.SpawnWithOptions(SpawnOptions{
		Kind: session.SubagentKindOneShot, Prompt: "go", SeedFromParent: 5,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	m.Drain()
	evs := store.logs["sess-seed1/"+rec.ChildID].Events()
	// header + 2 seeds (prompt, response) + initial message + executor turn (3)
	if len(evs) != 7 {
		t.Fatalf("child events = %d, want 7: %v", len(evs), evs)
	}
	for _, ev := range parent.Events() {
		if ev.Type != session.TypeSubagentSpawned {
			continue
		}
		var sp session.SubagentPayload
		if err := json.Unmarshal(ev.Payload, &sp); err != nil {
			t.Fatalf("spawned payload: %v", err)
		}
		if sp.SeedTurns != 1 {
			t.Fatalf("seedTurns = %d, want 1 (all available)", sp.SeedTurns)
		}
	}
}

func TestSpawnSeedNoneWhenParentHasNoCompletedTurns(t *testing.T) {
	store := newMemStore()
	m, parent := newTestManager(t, "sess-seed0", store, &echoExecutor{}, Options{})
	// An UNTERMINATED bracket only — no completed turns at all.
	if _, err := parent.AppendTurnBegin(); err != nil {
		t.Fatalf("turn/begin: %v", err)
	}

	rec, err := m.SpawnWithOptions(SpawnOptions{
		Kind: session.SubagentKindOneShot, Prompt: "cold start", SeedFromParent: 3,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	m.Drain()
	evs := store.logs["sess-seed0/"+rec.ChildID].Events()
	if evs[1].Type != session.TypeSubagentMessage {
		t.Fatalf("first post-header event = %s, want subagent/message (no seeds)", evs[1].Type)
	}
	var sp session.SubagentPayload
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSpawned {
			_ = json.Unmarshal(ev.Payload, &sp)
		}
	}
	if sp.SeedTurns != 0 {
		t.Fatalf("seedTurns = %d, want 0", sp.SeedTurns)
	}
	// omitempty: seedTurns=0 must not appear in the durable record bytes.
	for _, ev := range parent.Events() {
		if ev.Type == session.TypeSubagentSpawned && bytes.Contains(ev.Payload, []byte("seedTurns")) {
			t.Fatalf("seedTurns present in spawned payload despite 0: %s", ev.Payload)
		}
	}
}

func TestSeedFileStoreReplayByteIdentical(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	parent, err := session.OpenFile(filepath.Join(dir, "sess-fs.jsonl"), "sess-fs")
	if err != nil {
		t.Fatalf("parent log: %v", err)
	}
	m, err := NewManager(parent, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	appendParentTurn(t, parent, "durable task", "durable reply")
	rec, err := m.SpawnWithOptions(SpawnOptions{
		Kind: session.SubagentKindOneShot, Prompt: "seeded child", SeedFromParent: 1,
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	m.Drain()

	childPath := filepath.Join(dir, "sess-fs", rec.ChildID+".jsonl")
	resumed, err := session.ResumeFile(childPath)
	if err != nil {
		t.Fatalf("resume child: %v", err)
	}
	replayed := resumed.Events()

	// Byte-identical replay: each replayed event re-marshals to exactly
	// its file line, and the fold matches the live child log handle's
	// surface.
	raw, err := os.ReadFile(childPath)
	if err != nil {
		t.Fatalf("read child log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSuffix(raw, []byte("\n")), []byte("\n"))
	if len(lines) != len(replayed) {
		t.Fatalf("lines=%d replayed=%d", len(lines), len(replayed))
	}
	for i, ev := range replayed {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal replayed[%d]: %v", i, err)
		}
		if !bytes.Equal(bytes.TrimSpace(lines[i]), b) {
			t.Fatalf("replay byte drift at line %d:\n got %s\nwant %s", i+1, lines[i], b)
		}
	}
	// The live child log (reopened by the manager during spawn) folds to
	// the same surface as the replay.
	liveChild, err := store.ReopenChild("sess-fs", rec.ChildID)
	if err != nil {
		t.Fatalf("reopen child: %v", err)
	}
	ls, err := liveChild.Surface()
	if err != nil {
		t.Fatalf("live surface: %v", err)
	}
	rsFold, err := session.FoldSurface(replayed)
	if err != nil {
		t.Fatalf("replay surface: %v", err)
	}
	rs := rsFold.Messages
	if len(ls) != len(rs) {
		t.Fatalf("surface lengths: live=%d replay=%d", len(ls), len(rs))
	}
	for i := range ls {
		lb, _ := json.Marshal(ls[i])
		rb, _ := json.Marshal(rs[i])
		if !bytes.Equal(lb, rb) {
			t.Fatalf("surface[%d]: live=%s replay=%s", i, lb, rb)
		}
	}
	// Determinism: re-seeding the same parent state into a fresh child
	// log produces identical seed bytes.
	seedOnly := func() []byte {
		buf := &bytes.Buffer{}
		clg, err := session.NewChildLog(buf, "sess-fs.clone", session.ChildHeader{ParentSessionID: "sess-fs", DelegationDepth: 1}, time.Unix(0, 0).UTC())
		if err != nil {
			t.Fatalf("clone log: %v", err)
		}
		if _, err := appendSeedTurns(clg, parent.Events(), 1); err != nil {
			t.Fatalf("seed clone: %v", err)
		}
		return buf.Bytes()
	}
	if !bytes.Equal(seedOnly(), seedOnly()) {
		t.Fatal("seeding is not deterministic for identical parent state")
	}
}
