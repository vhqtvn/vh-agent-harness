package subagents

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TestReplayDeterminismParentChildLifecycle is the slice-8 acceptance
// crux: a FULL parent+child lifecycle — one-shot spawn → run → report →
// auto-settle, then continuable spawn → run → follow-up message → run →
// explicit settle — written through the Manager to FILES must, after
// close + re-open + replay, reproduce the identical derived surfaces
// byte-for-byte (parent AND child), the identical event streams, and the
// identical fold states. What the live process saw is exactly what replay
// reconstructs.
func TestReplayDeterminismParentChildLifecycle(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)

	parentPath := filepath.Join(dir, "parent.jsonl")
	parent, err := session.OpenFile(parentPath, "parent-1")
	if err != nil {
		t.Fatalf("parent log: %v", err)
	}
	m, err := NewManager(parent, &echoExecutor{}, store, Options{})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// One-shot: spawn → run → report → auto-settle.
	if _, err := m.Spawn(session.SubagentKindOneShot, "count widgets", ""); err != nil {
		t.Fatalf("Spawn one-shot: %v", err)
	}
	m.Drain()

	// Continuable: spawn → run → waiting → follow-up → run → explicit settle.
	if _, err := m.Spawn(session.SubagentKindContinuable, "study the repo", "researcher"); err != nil {
		t.Fatalf("Spawn continuable: %v", err)
	}
	m.Drain()
	if err := m.SendMessage("parent-1.2", "now the docs"); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	m.Drain()
	if err := m.Settle("parent-1.2", nil); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	liveParentEvents := parent.Events()
	liveParentMsgs, err := parent.Surface()
	if err != nil {
		t.Fatalf("parent Surface: %v", err)
	}
	liveChild2, err := store.ReopenChild("parent-1", "parent-1.2")
	if err != nil {
		t.Fatalf("ReopenChild: %v", err)
	}
	liveChild2Events := liveChild2.Events()
	liveChild2Msgs, err := liveChild2.Surface()
	if err != nil {
		t.Fatalf("child Surface: %v", err)
	}
	liveSnap := m.Snapshot()

	if err := parent.Close(); err != nil {
		t.Fatalf("close parent: %v", err)
	}
	if err := liveChild2.Close(); err != nil {
		t.Fatalf("close child: %v", err)
	}

	// Replay both logs from disk.
	replayedParent, err := session.ReplayFile(parentPath)
	if err != nil {
		t.Fatalf("ReplayFile parent: %v", err)
	}
	replayedChild2, err := session.ReplayFile(filepath.Join(dir, "parent-1", "parent-1.2.jsonl"))
	if err != nil {
		t.Fatalf("ReplayFile child: %v", err)
	}

	// Event streams reconstruct exactly (seq, type, surfaceOp, payload bytes).
	if !reflect.DeepEqual(liveParentEvents, replayedParent) {
		t.Fatalf("parent event stream not reproduced: live %d events, replay %d", len(liveParentEvents), len(replayedParent))
	}
	if !reflect.DeepEqual(liveChild2Events, replayedChild2) {
		t.Fatalf("child event stream not reproduced: live %d events, replay %d", len(liveChild2Events), len(replayedChild2))
	}

	// Surfaces reproduce byte-for-byte.
	assertSurfaceBytes(t, "parent", liveParentMsgs, replayedParent)
	assertSurfaceBytes(t, "child", liveChild2Msgs, replayedChild2)

	// Fold states reproduce: both children settled; the fold over the
	// replayed log equals the live Snapshot states.
	foldedLive, err := FoldSubagents(liveParentEvents)
	if err != nil {
		t.Fatalf("FoldSubagents live: %v", err)
	}
	foldedReplay, err := FoldSubagents(replayedParent)
	if err != nil {
		t.Fatalf("FoldSubagents replay: %v", err)
	}
	if !reflect.DeepEqual(foldedLive, foldedReplay) {
		t.Fatalf("fold not reproduced across replay:\nlive:   %+v\nreplay: %+v", foldedLive, foldedReplay)
	}
	if len(foldedReplay) != 2 {
		t.Fatalf("fold children = %d, want 2", len(foldedReplay))
	}
	for i, want := range []struct{ id, result string }{
		{"parent-1.1", session.JobResultCompleted},
		{"parent-1.2", session.JobResultCompleted},
	} {
		if foldedReplay[i].ChildID != want.id || foldedReplay[i].State != StateSettled || foldedReplay[i].SettledResult != want.result {
			t.Fatalf("folded[%d] = %+v, want settled %s %s", i, foldedReplay[i], want.id, want.result)
		}
		if liveSnap[i].State != StateSettled {
			t.Fatalf("live snapshot[%d].State = %q, want settled", i, liveSnap[i].State)
		}
	}

	// Provenance on the replayed surface: the ONLY parent-surface
	// messages are the three user-role reports (one per distinct child
	// output: the one-shot's single turn, the continuable's two turns) —
	// never assistant.
	replayMsgs, err := session.DeriveMessages(replayedParent)
	if err != nil {
		t.Fatalf("DeriveMessages replayed parent: %v", err)
	}
	if len(replayMsgs) != 3 {
		t.Fatalf("replayed parent surface length = %d, want 3 reports", len(replayMsgs))
	}
	for i, msg := range replayMsgs {
		if msg.Role != "user" {
			t.Fatalf("replayed parent msgs[%d].Role = %q, want user (provenance-clean)", i, msg.Role)
		}
	}
	wantReports := []string{
		"subagent parent-1.1 report: did: count widgets",
		"subagent parent-1.2 report: did: study the repo",
		"subagent parent-1.2 report: did: now the docs",
	}
	for i, want := range wantReports {
		if replayMsgs[i].Content != want {
			t.Fatalf("report[%d] = %q, want %q", i, replayMsgs[i].Content, want)
		}
	}

	// Child surface reconstructs the FIFO inbox: prompt, follow-up, and
	// the two assistant turns interleaved in delivery order.
	childMsgs, err := session.DeriveMessages(replayedChild2)
	if err != nil {
		t.Fatalf("DeriveMessages replayed child: %v", err)
	}
	wantChild := []struct{ role, content string }{
		{"user", "study the repo"}, {"assistant", "did: study the repo"},
		{"user", "now the docs"}, {"assistant", "did: now the docs"},
	}
	if len(childMsgs) != len(wantChild) {
		t.Fatalf("replayed child surface length = %d, want %d", len(childMsgs), len(wantChild))
	}
	for i, w := range wantChild {
		if childMsgs[i].Role != w.role || childMsgs[i].Content != w.content {
			t.Fatalf("replayed child msgs[%d] = {%s %q}, want {%s %q}", i, childMsgs[i].Role, childMsgs[i].Content, w.role, w.content)
		}
	}
}

// assertSurfaceBytes fails t when the replayed event list does not derive
// a byte-identical message surface to the live one.
func assertSurfaceBytes(t *testing.T, name string, live []session.Message, replayEvents []session.Event) {
	t.Helper()
	replay, err := session.DeriveMessages(replayEvents)
	if err != nil {
		t.Fatalf("DeriveMessages replayed %s: %v", name, err)
	}
	liveJSON, err := json.Marshal(live)
	if err != nil {
		t.Fatalf("marshal live %s surface: %v", name, err)
	}
	replayJSON, err := json.Marshal(replay)
	if err != nil {
		t.Fatalf("marshal replayed %s surface: %v", name, err)
	}
	if !bytes.Equal(liveJSON, replayJSON) {
		t.Fatalf("%s replay determinism failed:\nlive:   %s\nreplay: %s", name, liveJSON, replayJSON)
	}
}
