// server_resume_test.go — P4 session/resume + session/list at the
// WIRE seam: the closed method table, the createMu critical section,
// typed error mapping, and the fixtures the compat lock requires.
package protocol

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// resumeWireEnv boots a server pair and seeds one session through the
// real engine seam.
func resumeWireEnv(t *testing.T) (*Client, *FileEngine, func()) {
	t.Helper()
	dir := t.TempDir()
	c, e, stop := newServerPair(t, dir, &resumeExec{})
	return c, e, stop
}

func TestSessionResumeWireRoundTrip(t *testing.T) {
	c, e, stop := resumeWireEnv(t)
	defer stop()

	if err := c.Call("initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil); err != nil {
		t.Fatal(err)
	}
	var created struct {
		SessionID string `json:"sessionId"`
		Path      string `json:"path"`
	}
	if err := c.Call("session/create", map[string]any{"sessionId": "sess-wire"}, &created); err != nil {
		t.Fatal(err)
	}
	// Seed durable content through the engine's own log.
	es, err := e.NewSession("", "sess-wire", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := es.Log.AppendPrompt("wire resume prompt"); err != nil {
		t.Fatal(err)
	}
	// D-F1: the seeding create left sess-wire as the engine's ACTIVE
	// session (same-id resume of the active session is refused); one
	// displacing create makes sess-wire resumable-over-the-wire again —
	// the daemon-restart story the wire method exists for.
	if _, err := e.NewSession("", "sess-displaced", io.Discard); err != nil {
		t.Fatal(err)
	}

	var sum ResumeSummary
	if err := c.Call("session/resume", map[string]any{"sessionId": "sess-wire"}, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.SessionID != "sess-wire" || sum.Path != created.Path {
		t.Fatalf("summary = %+v (created path %s)", sum, created.Path)
	}
	if sum.Events != 2 || sum.Title != "wire resume prompt" || len(sum.Messages) != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	// The resumed session is ACTIVE: session/surface serves the
	// recovered surface.
	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	if err := c.Call("session/surface", nil, &surf); err != nil {
		t.Fatal(err)
	}
	if len(surf.Messages) != 1 || surf.Messages[0].Content != "wire resume prompt" {
		t.Fatalf("surface after resume = %+v", surf.Messages)
	}
}

func TestSessionResumeWireUnknownIDTypedError(t *testing.T) {
	c, _, stop := resumeWireEnv(t)
	defer stop()
	if err := c.Call("initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil); err != nil {
		t.Fatal(err)
	}
	err := c.Call("session/resume", map[string]any{"sessionId": "sess-ghost"}, nil)
	perr, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %v, want *protocol.Error", err)
	}
	if perr.Code != ErrInvalidParams {
		t.Fatalf("code = %d, want -32602", perr.Code)
	}
	if !strings.Contains(perr.Message, "not-found") {
		t.Fatalf("message must carry the typed kind: %s", perr.Message)
	}
}

func TestSessionResumeWireHostileIDRejected(t *testing.T) {
	c, _, stop := resumeWireEnv(t)
	defer stop()
	if err := c.Call("initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"../victim", "sess/x", "."} {
		err := c.Call("session/resume", map[string]any{"sessionId": id}, nil)
		perr, ok := err.(*Error)
		if !ok || perr.Code != ErrInvalidParams {
			t.Fatalf("id %q: err = %v, want -32602", id, err)
		}
	}
	// No file effects: nothing escaped the session root.
	if _, err := statIfExists("../victim"); err == nil {
		t.Fatal("hostile id escaped the session root")
	}
}

func TestSessionResumeSupersedesActive(t *testing.T) {
	c, e, stop := resumeWireEnv(t)
	defer stop()
	if err := c.Call("initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call("session/create", map[string]any{"sessionId": "sess-first"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.NewSession("", "sess-second", io.Discard); err != nil {
		t.Fatal(err)
	}
	// Resuming the superseded (NON-active) sess-first makes IT active:
	// surface serves the resumed stream, not the created one. D-F1: the
	// ACTIVE id (sess-second) is not resumable — same-id resume is a
	// typed refusal (see TestSessionResumeActiveIDTypedError).
	if err := c.Call("session/resume", map[string]any{"sessionId": "sess-first"}, nil); err != nil {
		t.Fatal(err)
	}
	var surf struct {
		Messages []session.Message `json:"messages"`
	}
	if err := c.Call("session/surface", nil, &surf); err != nil {
		t.Fatal(err)
	}
	// sess-first was never prompted; assert liveness via jobs/status.
	var jobsOut struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := c.Call("jobs/status", nil, &jobsOut); err != nil {
		t.Fatalf("jobs/status after resume: %v", err)
	}
	if jobsOut.Jobs == nil {
		t.Fatal("jobs/status must serve the resumed session (honest empty list, not -32003)")
	}
}

// TestSessionResumeActiveIDTypedError (D-F1, wire level): resuming the
// session id that is CURRENTLY ACTIVE over the wire is a typed
// -32602 refusal (session-active). Pre-fix it opened a second live log
// on the same durable file and any further append corrupted the stream
// permanently.
func TestSessionResumeActiveIDTypedError(t *testing.T) {
	c, _, stop := resumeWireEnv(t)
	defer stop()
	if err := c.Call("initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Call("session/create", map[string]any{"sessionId": "sess-live"}, nil); err != nil {
		t.Fatal(err)
	}
	err := c.Call("session/resume", map[string]any{"sessionId": "sess-live"}, nil)
	perr, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %v, want *protocol.Error", err)
	}
	if perr.Code != ErrInvalidParams {
		t.Fatalf("code = %d, want -32602", perr.Code)
	}
	if !strings.Contains(perr.Message, "session-active") {
		t.Fatalf("message must carry the typed kind: %s", perr.Message)
	}
	// The active session keeps serving normally after the refusal.
	var jobsOut struct {
		Jobs []json.RawMessage `json:"jobs"`
	}
	if err := c.Call("jobs/status", nil, &jobsOut); err != nil || jobsOut.Jobs == nil {
		t.Fatalf("the active session must remain servable after the refusal: %v", err)
	}
}

func TestSessionListWire(t *testing.T) {
	c, e, stop := resumeWireEnv(t)
	defer stop()
	if err := c.Call("initialize", map[string]any{"protocolVersion": ProtocolVersion}, nil); err != nil {
		t.Fatal(err)
	}
	seedSession(t, e, "sess-alpha", "alpha prompt")
	seedSession(t, e, "sess-beta", "beta prompt")

	var out struct {
		Sessions []SessionEntry `json:"sessions"`
	}
	if err := c.Call("session/list", nil, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Sessions) != 2 {
		t.Fatalf("sessions = %+v", out.Sessions)
	}
	ids := out.Sessions[0].SessionID + "," + out.Sessions[1].SessionID
	if !strings.Contains(ids, "sess-alpha") || !strings.Contains(ids, "sess-beta") {
		t.Fatalf("sessions = %+v", out.Sessions)
	}
	for _, s := range out.Sessions {
		if s.Events != 3 {
			t.Fatalf("entry %s events = %d, want 3", s.SessionID, s.Events)
		}
	}
}

// statIfExists is a tiny helper for escape assertions.
func statIfExists(path string) (any, error) { return os.Stat(path) }
