// turn_redaction_test.go — finding-2 adversarial crux at the PIPELINE
// seam: a REAL adapter (openaicompat over an httptest provider that
// echoes the API-key value in a 500 body) driven through RunTurn with
// the retry ladder armed. The durable session log (llm/retry
// errorMessage records, the closing turn/end reason) and the returned
// error must all carry [REDACTED] and never the key value — the README's
// "the key is never written to session logs" guarantee must hold even
// against a credential-echoing provider.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters/openaicompat"
	"github.com/vhqtvn/vh-agent-harness/internal/loop"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// TestRunTurnRealAdapterErrorRedactedInLog drives the real adapter →
// real HTTP failure → retry ladder → exhaustion path and asserts no key
// value anywhere in the durable log bytes.
func TestRunTurnRealAdapterErrorRedactedInLog(t *testing.T) {
	const key = "sk-live-0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"unauthorized: key sk-live-0123456789abcdef echoed for tenant-9917; repeat sk-live-0123456789abcdef"}}`))
	}))
	t.Cleanup(srv.Close)
	ad := openaicompat.New(openaicompat.Config{Provider: "mock", BaseURL: srv.URL, Model: "m", APIKey: key})

	var sink writeBuffer
	lg := fixedLog(t, "turn-redact", &sink)
	p := NewPipeline()
	_, err := p.RunTurn(context.Background(), lg, ad, TurnOptions{Model: "m", Retry: ladder(2)}, "leak check")
	var ex *loop.ExhaustedError
	if !errors.As(err, &ex) {
		t.Fatalf("error = %v (%T), want *loop.ExhaustedError (500 is retryable)", err, err)
	}

	// The whole durable log — every llm/retry errorMessage, the closing
	// turn/end reason — must be free of the key value.
	if bytes.Contains(sink.data, []byte(key)) {
		t.Fatalf("API-key value leaked into the session log:\n%s", sink.data)
	}
	if !bytes.Contains(sink.data, []byte("[REDACTED]")) {
		t.Fatalf("log should carry the redaction marker (llm/retry + turn/end):\n%s", sink.data)
	}

	// The retry path actually fired (two durable retry records), so the
	// redaction held through repeated classifications, and the returned
	// exhaustion error is redacted too (it rides the wire as -32000).
	if got := countTypes(lg, session.TypeLLMRetry); got != 2 {
		t.Fatalf("llm/retry records = %d, want 2", got)
	}
	var lastTE session.TurnEndPayload
	events := lg.Events()
	if err := json.Unmarshal(events[len(events)-1].Payload, &lastTE); err != nil {
		t.Fatalf("turn/end payload: %v", err)
	}
	if lastTE.Kind != "error" {
		t.Fatalf("turn/end kind = %q, want error", lastTE.Kind)
	}
	if bytes.Contains([]byte(lastTE.Reason), []byte(key)) {
		t.Fatalf("turn/end reason carries the key value: %s", lastTE.Reason)
	}
	if bytes.Contains([]byte(err.Error()), []byte(key)) {
		t.Fatalf("returned error carries the key value: %s", err)
	}
}
