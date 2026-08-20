package session

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// TestSubagentAppendHelpers covers the slice-8 writer surface: the four
// append helpers assign contiguous seqs, log-only subagent types reject a
// surfaceOp-bearing misuse is not possible via the typed API, and the
// message-bearing types carry the append surfaceOp.
func TestSubagentAppendHelpers(t *testing.T) {
	var buf bytes.Buffer
	lg, err := NewLog(&buf, "parent-1", time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}

	if _, err := lg.AppendSubagentSpawned("subagent-1", SubagentKindOneShot, "go count widgets", 1); err != nil {
		t.Fatalf("AppendSubagentSpawned: %v", err)
	}
	if _, err := lg.AppendSubagentReport("subagent-1", "counted 7 widgets"); err != nil {
		t.Fatalf("AppendSubagentReport: %v", err)
	}
	if _, err := lg.AppendSubagentSettled("subagent-1", JobResultCompleted, ""); err != nil {
		t.Fatalf("AppendSubagentSettled: %v", err)
	}
	events := lg.Events()
	if len(events) != 4 { // header + 3
		t.Fatalf("event count = %d, want 4", len(events))
	}
	for i, ev := range events {
		if ev.Seq != int64(i+1) {
			t.Fatalf("events[%d].Seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
	if events[1].SurfaceOp != nil {
		t.Fatalf("subagent/spawned must be log-only, got surfaceOp %+v", events[1].SurfaceOp)
	}
	if events[3].SurfaceOp != nil {
		t.Fatalf("subagent/settled must be log-only, got surfaceOp %+v", events[3].SurfaceOp)
	}
	if events[2].SurfaceOp == nil || events[2].SurfaceOp.Op != SurfaceOpAppend {
		t.Fatalf("subagent/report must carry an append surfaceOp, got %+v", events[2].SurfaceOp)
	}

	// Provenance tags ride the payloads.
	var rp SubagentPayload
	if err := json.Unmarshal(events[2].Payload, &rp); err != nil {
		t.Fatalf("unmarshal report payload: %v", err)
	}
	if rp.Provenance != SubagentProvenanceReport {
		t.Fatalf("report provenance = %q, want %q", rp.Provenance, SubagentProvenanceReport)
	}
	var sp SubagentPayload
	if err := json.Unmarshal(events[3].Payload, &sp); err != nil {
		t.Fatalf("unmarshal settled payload: %v", err)
	}
	if sp.Provenance != SubagentProvenanceSettled {
		t.Fatalf("settled provenance = %q, want %q", sp.Provenance, SubagentProvenanceSettled)
	}
}

// TestSubagentSurfaceRoles asserts the provenance-clean projection: the
// report enters the parent surface as a USER-role context event and the
// settlement notice contributes NOTHING — no subagent event ever surfaces
// as an assistant message.
func TestSubagentSurfaceRoles(t *testing.T) {
	var buf bytes.Buffer
	lg, err := NewLog(&buf, "parent-1", time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := lg.AppendSubagentSpawned("subagent-1", SubagentKindContinuable, "study the repo", 1); err != nil {
		t.Fatalf("AppendSubagentSpawned: %v", err)
	}
	if _, err := lg.AppendSubagentReport("subagent-1", "the repo has 3 packages"); err != nil {
		t.Fatalf("AppendSubagentReport: %v", err)
	}
	if _, err := lg.AppendSubagentSettled("subagent-1", JobResultCompleted, ""); err != nil {
		t.Fatalf("AppendSubagentSettled: %v", err)
	}

	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("surface length = %d, want 1 (only the report is message-bearing)", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("report surfaced with role %q, want user", msgs[0].Role)
	}
	if msgs[0].Content != "subagent subagent-1 report: the repo has 3 packages" {
		t.Fatalf("report content = %q", msgs[0].Content)
	}

	// The child side: a parent→child message surfaces as a user-role
	// inbox message, in FIFO append order.
	var cbuf bytes.Buffer
	clg, err := NewChildLog(&cbuf, "subagent-1", ChildHeader{ParentSessionID: "parent-1", DelegationDepth: 1, Role: "researcher"}, time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("NewChildLog: %v", err)
	}
	if _, err := clg.AppendSubagentMessage("subagent-1", "parent-1", "study the repo"); err != nil {
		t.Fatalf("AppendSubagentMessage: %v", err)
	}
	if _, err := clg.AppendSubagentMessage("subagent-1", "parent-1", "and the docs"); err != nil {
		t.Fatalf("AppendSubagentMessage: %v", err)
	}
	cmsgs, err := clg.Surface()
	if err != nil {
		t.Fatalf("child Surface: %v", err)
	}
	if len(cmsgs) != 2 {
		t.Fatalf("child surface length = %d, want 2", len(cmsgs))
	}
	for i, want := range []string{"study the repo", "and the docs"} {
		if cmsgs[i].Role != "user" || cmsgs[i].Content != want {
			t.Fatalf("child msgs[%d] = {%s %q}, want {user %q}", i, cmsgs[i].Role, cmsgs[i].Content, want)
		}
	}
}

// TestRootHeaderByteStability pins the additive-within-version-0
// contract: a root header written through the slice-8 path marshals
// byte-identically to the pre-slice-8 shape (no parentSessionId,
// delegationDepth, or role keys), and a child header round-trips through
// replay with its topology fields intact.
func TestRootHeaderByteStability(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	// preSlice8Header is the exact pre-slice-8 HeaderPayload shape (field
	// order included) the root header must stay byte-identical to.
	type preSlice8Header struct {
		SessionID     string    `json:"sessionId"`
		FormatVersion int       `json:"formatVersion"`
		CreatedAt     time.Time `json:"createdAt"`
	}
	want, err := json.Marshal(preSlice8Header{SessionID: "root-1", FormatVersion: SESSION_FORMAT_VERSION, CreatedAt: now})
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	var buf bytes.Buffer
	lg, err := NewLog(&buf, "root-1", now)
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	events := lg.Events()
	var got HeaderPayload
	if err := json.Unmarshal(events[0].Payload, &got); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	if !bytes.Equal(want, gotJSON) {
		t.Fatalf("root header not byte-stable:\nwant: %s\ngot:  %s", want, gotJSON)
	}
	if bytes.Contains(buf.Bytes(), []byte("parentSessionId")) ||
		bytes.Contains(buf.Bytes(), []byte("delegationDepth")) ||
		bytes.Contains(buf.Bytes(), []byte(`"role"`)) {
		t.Fatalf("root header leaked child topology keys: %s", buf.Bytes())
	}

	// Child header round-trip: depth and parent id survive replay (the
	// header is the authoritative depth record across resume).
	path := filepath.Join(t.TempDir(), "child.jsonl")
	clg, err := OpenChildFile(path, "subagent-1", ChildHeader{ParentSessionID: "root-1", DelegationDepth: 2, Role: "researcher"})
	if err != nil {
		t.Fatalf("OpenChildFile: %v", err)
	}
	if err := clg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	replayed, err := ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	var hp HeaderPayload
	if err := json.Unmarshal(replayed[0].Payload, &hp); err != nil {
		t.Fatalf("unmarshal replayed header: %v", err)
	}
	if hp.SessionID != "subagent-1" || hp.ParentSessionID != "root-1" || hp.DelegationDepth != 2 || hp.Role != "researcher" {
		t.Fatalf("child header topology lost across replay: %+v", hp)
	}
}

// TestSubagentLogOnlyFoldContributesNothing asserts spawned/settled fold
// to nothing on the surface even interleaved with messages (the log-only
// plugin-event discipline).
func TestSubagentLogOnlyFoldContributesNothing(t *testing.T) {
	var buf bytes.Buffer
	lg, err := NewLog(&buf, "parent-1", time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("NewLog: %v", err)
	}
	if _, err := lg.AppendSubagentSpawned("subagent-1", SubagentKindOneShot, "p", 1); err != nil {
		t.Fatalf("AppendSubagentSpawned: %v", err)
	}
	if _, err := lg.AppendPrompt("user speaks"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendSubagentSettled("subagent-1", JobResultFailed, "boom"); err != nil {
		t.Fatalf("AppendSubagentSettled: %v", err)
	}
	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "user speaks" {
		t.Fatalf("surface = %+v, want exactly the prompt message", msgs)
	}
}
