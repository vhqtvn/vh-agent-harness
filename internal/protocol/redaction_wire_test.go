// redaction_wire_test.go — finding-2 adversarial crux at the WIRE seam:
// a real adapter (openaicompat over an httptest provider that echoes the
// API-key value in a 500 body) composed into the real FileEngine and
// driven through session/prompt over the protocol. The wire error
// response (-32000 carrying the turn-failure text) AND the durable
// session log (turn/end reason) must carry [REDACTED] and never the key
// value.
package protocol

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

func TestPromptWireErrorRedactsAPIKey(t *testing.T) {
	const key = "sk-live-0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"provider exploded; bearer was sk-live-0123456789abcdef for tenant-5502"}}`))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	eng := &FileEngine{
		Dir:      dir,
		Executor: noopExecutor{},
		// No retry ladder: the 500 closes the turn immediately, so the
		// error text rides straight to the wire.
		Ad:       openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: key}),
		TurnOpts: tools.TurnOptions{Model: "m"},
	}
	p := newWirePair(t, eng)

	if err := p.client.Call("initialize", map[string]any{"protocolVersion": 1}, nil); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var created createResult
	if err := p.client.Call("session/create", map[string]any{"sessionId": "sess-redact"}, &created); err != nil {
		t.Fatalf("session/create: %v", err)
	}

	err := p.client.Call("session/prompt", map[string]any{"text": "trigger"}, nil)
	if err == nil {
		t.Fatal("expected the failing provider to surface a protocol error")
	}
	msg := err.Error()
	if strings.Contains(msg, key) {
		t.Fatalf("API-key value leaked over the wire: %s", msg)
	}
	if !strings.Contains(msg, "[REDACTED]") {
		t.Fatalf("wire error should carry the redaction marker: %s", msg)
	}

	// The durable log's closing turn/end reason is redacted too.
	raw, rerr := os.ReadFile(created.Path)
	if rerr != nil {
		t.Fatalf("read log: %v", rerr)
	}
	if strings.Contains(string(raw), key) {
		t.Fatalf("API-key value leaked into the session log:\n%s", raw)
	}
	if !strings.Contains(string(raw), "[REDACTED]") {
		t.Fatalf("session log should carry the redaction marker (turn/end reason):\n%s", raw)
	}
}
