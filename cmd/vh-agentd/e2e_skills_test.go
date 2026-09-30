// e2e_skills_test.go — the P7 skills crux at the Go level: a real turn
// through buildServer's engine exercising skill_load (whole body + tier-3
// ref), the log-only skill/loaded provenance events, and THE replay
// invariant — after MUTATING the catalog on disk, replay of the
// persisted log reproduces byte-identical events and surface, because
// replay derives tool output from the LOG, never disk. This is the Go
// twin of the battery scenario's replay-after-mutation assertion.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/skills"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/skillload"
)

// TestSkillLoadLoggedReplayUsesLoggedContentNotDisk is the slice crux:
// the tool result content IS the logged tool/result — mutating the
// catalog files after the run changes NOTHING about replay.
func TestSkillLoadLoggedReplayUsesLoggedContentNotDisk(t *testing.T) {
	root := catFixture(t)
	cat, _ := skills.Load(root)
	dir := t.TempDir()
	cfg := skillsCfg(t, dir, cat)
	svc, cli := net.Pipe()
	defer cli.Close()
	_, _, tracker, _ := buildServer(cfg, "test-key", svc)

	// Tier 1 advertised: the SERVED system prompt carries the sanitized
	// catalog lines (journal-level proof happens in the battery; here we
	// pin the serving-side fact directly).
	sys := tracker.TurnOptions().System
	if !strings.Contains(sys, "- clean-one: The clean fixture skill for validation.") {
		t.Fatalf("served system prompt missing the clean-one tier-1 line:\n%s", sys)
	}
	if strings.Contains(sys, "<angle>") {
		t.Fatal("unsanitized angle tokens in the served system prompt")
	}

	// Create the session THROUGH the tracker, exactly as the protocol
	// server does in production — the tracker binds the session's log
	// into the sessionLogRegistry the skill_load provenance sink
	// resolves through.
	es, err := tracker.NewSession("", "sess-skills-crux", io.Discard)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	ad := &scriptedAdapter{next: func(calls int) *adapters.Response {
		switch calls {
		case 0:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{
					ID: "call_skill1", Name: skillload.Name,
					Args: json.RawMessage(`{"name":"clean-one"}`),
				}},
			}
		case 1:
			return &adapters.Response{
				Model: "fake-model", FinishReason: "tool_calls",
				ToolCalls: []session.ToolCall{{
					ID: "call_skill2", Name: skillload.Name,
					Args: json.RawMessage(`{"name":"clean-one","ref":"references/note.md"}`),
				}},
			}
		default:
			return &adapters.Response{Model: "fake-model", FinishReason: "stop", Content: "skills round trip complete"}
		}
	}}

	ctx := context.Background()
	r1, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "load the clean skill")
	if err != nil {
		t.Fatalf("RunTurn 1: %v", err)
	}
	if len(r1.Results) != 1 || r1.Results[0].IsError {
		t.Fatalf("turn-1 results = %+v", r1.Results)
	}
	body1 := r1.Results[0].Content
	if !strings.Contains(body1, "# Clean one") || !strings.Contains(body1, "Body instructions here.") {
		t.Fatalf("whole-body load missing body: %q", body1)
	}
	if !strings.Contains(body1, "allowed-tools ceiling: run_shell, read (intersected with the registry — never a grant)") {
		t.Fatalf("ceiling footer missing: %q", body1)
	}

	r2, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "load its tier-three reference")
	if err != nil {
		t.Fatalf("RunTurn 2: %v", err)
	}
	if len(r2.Results) != 1 || r2.Results[0].IsError {
		t.Fatalf("turn-2 results = %+v", r2.Results)
	}
	if !strings.Contains(r2.Results[0].Content, "tier-three reference payload") {
		t.Fatalf("tier-3 ref load missing payload: %q", r2.Results[0].Content)
	}

	r3, err := tracker.TurnRunner().RunTurn(ctx, es.Log, ad, tracker.TurnOptions(), "done")
	if err != nil {
		t.Fatalf("RunTurn 3: %v", err)
	}
	if r3.Response == nil || !strings.Contains(r3.Response.Content, "skills round trip complete") {
		t.Fatalf("turn-3 final response = %+v", r3.Response)
	}

	// The log-only provenance events: two skill/loaded records with
	// name/ref/sha256 cross-checkable against the tool results.
	var loaded []session.SkillLoadedPayload
	for _, ev := range es.Log.Events() {
		if ev.Type != session.TypeSkillLoaded {
			continue
		}
		var p session.SkillLoadedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("skill/loaded payload: %v", err)
		}
		loaded = append(loaded, p)
	}
	if len(loaded) != 2 {
		t.Fatalf("skill/loaded events = %d, want 2: %+v", len(loaded), loaded)
	}
	if loaded[0].Name != "clean-one" || loaded[0].Ref != "" {
		t.Fatalf("event 1 = %+v", loaded[0])
	}
	if loaded[1].Name != "clean-one" || loaded[1].Ref != "references/note.md" {
		t.Fatalf("event 2 = %+v", loaded[1])
	}
	sum := sha256.Sum256([]byte(contentPortionOf(body1)))
	if loaded[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("event sha %s does not match the returned content hash %s", loaded[0].SHA256, hex.EncodeToString(sum[:]))
	}

	// Replay determinism BEFORE mutation (baseline).
	liveEvents := es.Log.Events()
	liveMsgs, err := es.Log.Surface()
	if err != nil {
		t.Fatalf("live surface: %v", err)
	}
	surfaceJSON := func(msgs []session.Message) []byte {
		b, _ := json.Marshal(msgs)
		return b
	}

	// THE CRUX: mutate the catalog on disk — replace the SKILL.md body
	// and the tier-3 reference with DIFFERENT bytes — then replay the
	// persisted log. Everything must stay byte-identical: replay uses
	// logged content, never disk.
	if err := os.WriteFile(filepath.Join(root, "clean-one", "SKILL.md"), []byte("---\nname: clean-one\ndescription: MUTATED description.\n---\n\n# MUTATED BODY — post-run tampering\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "clean-one", "references", "note.md"), []byte("MUTATED reference payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(es.Path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	replayed, err := session.Replay(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Replay after mutation: %v", err)
	}
	liveJSON, _ := json.Marshal(liveEvents)
	replayJSON, _ := json.Marshal(replayed)
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatal("replayed events differ from the live log after disk mutation")
	}
	replayMsgs, err := session.DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages after mutation: %v", err)
	}
	if !bytes.Equal(surfaceJSON(liveMsgs), surfaceJSON(replayMsgs)) {
		t.Fatal("replayed surface differs from the live surface after disk mutation")
	}
	for _, m := range replayMsgs {
		if strings.Contains(m.Content, "MUTATED") {
			t.Fatal("mutated disk content leaked into the replayed surface — replay must derive from the log, never disk")
		}
	}
	foundBody, foundRef := false, false
	for _, m := range replayMsgs {
		if strings.Contains(m.Content, "Body instructions here.") {
			foundBody = true
		}
		if strings.Contains(m.Content, "tier-three reference payload") {
			foundRef = true
		}
	}
	if !foundBody || !foundRef {
		t.Fatalf("replayed surface missing the ORIGINAL logged body (%v) or reference (%v)", foundBody, foundRef)
	}
}

// contentPortionOf strips the trailing ceiling footer to recover the
// exact content the model received (sha basis; mirrors the skillload
// package test helper).
func contentPortionOf(out string) string {
	if i := strings.LastIndex(out, "\n\n---\n"); i >= 0 {
		return out[:i]
	}
	return out
}
