// wire_test.go — slice 5 step 1 (red): the NDJSON wire codec.
//
// Grammar (dsh SDK transport pattern,
// researches/sources/deepseek-harness/llm-protocols-tools.md §2.8/F-SDK-1):
//
//	id + method  → request
//	id only      → response (result or error)
//	method only  → notification
package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWireRequestRoundTrip(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":1}}`
	msg, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindRequest {
		t.Fatalf("kind = %v, want request", msg.Kind)
	}
	if msg.Request.ID != 7 || msg.Request.Method != "initialize" {
		t.Fatalf("decoded = %+v", msg.Request)
	}
	var p struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.Unmarshal(msg.Request.Params, &p); err != nil {
		t.Fatalf("params: %v", err)
	}
	if p.ProtocolVersion != 1 {
		t.Fatalf("protocolVersion = %d", p.ProtocolVersion)
	}

	enc, err := MarshalRequest(7, "initialize", json.RawMessage(`{"protocolVersion":1}`))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(enc) != line {
		t.Fatalf("re-encode mismatch:\n got %s\nwant %s", enc, line)
	}
}

func TestWireRequestWithoutParams(t *testing.T) {
	msg, err := ParseLine([]byte(`{"jsonrpc":"2.0","id":1,"method":"jobs/status"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindRequest || msg.Request.Params != nil {
		t.Fatalf("decoded = %+v kind=%v", msg.Request, msg.Kind)
	}
	enc, err := MarshalRequest(1, "jobs/status", nil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(enc) != `{"jsonrpc":"2.0","id":1,"method":"jobs/status"}` {
		t.Fatalf("encode = %s", enc)
	}
}

func TestWireResponseResultRoundTrip(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":3,"result":{"jobId":"background-1"}}`
	msg, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindResponse {
		t.Fatalf("kind = %v, want response", msg.Kind)
	}
	if msg.Response.Error != nil {
		t.Fatalf("unexpected error: %+v", msg.Response.Error)
	}
	enc, err := MarshalResponse(3, json.RawMessage(`{"jobId":"background-1"}`))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(enc) != line {
		t.Fatalf("re-encode mismatch:\n got %s\nwant %s", enc, line)
	}
}

func TestWireResponseErrorRoundTrip(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found: nope"}}`
	msg, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindResponse {
		t.Fatalf("kind = %v", msg.Kind)
	}
	if msg.Response.Error == nil || msg.Response.Error.Code != -32601 {
		t.Fatalf("error = %+v", msg.Response.Error)
	}
	if msg.Response.Result != nil {
		t.Fatalf("result should be absent on error response, got %s", msg.Response.Result)
	}
	enc, err := MarshalResponseError(2, -32601, "method not found: nope", nil)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(enc) != line {
		t.Fatalf("re-encode mismatch:\n got %s\nwant %s", enc, line)
	}
}

func TestWireNotificationRoundTrip(t *testing.T) {
	line := `{"jsonrpc":"2.0","method":"session/event","params":{"seq":4,"type":"job/settled"}}`
	msg, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindNotification {
		t.Fatalf("kind = %v, want notification", msg.Kind)
	}
	if msg.Notification.Method != "session/event" {
		t.Fatalf("method = %s", msg.Notification.Method)
	}
	enc, err := MarshalNotification("session/event", json.RawMessage(`{"seq":4,"type":"job/settled"}`))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(enc) != line {
		t.Fatalf("re-encode mismatch:\n got %s\nwant %s", enc, line)
	}
}

func TestWireUnknownFieldRejected(t *testing.T) {
	// Fail-closed inbound strictness: an envelope field outside the
	// grammar is an INVALID request, not a silently-ignored extra.
	msg, err := ParseLine([]byte(`{"jsonrpc":"2.0","id":5,"method":"initialize","bogus":true}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindInvalid {
		t.Fatalf("kind = %v, want invalid (unknown field rejected)", msg.Kind)
	}
	if !msg.HasID || msg.InvalidID != 5 {
		t.Fatalf("invalid message should stay attributable: %+v", msg)
	}
}

func TestWireMalformedLine(t *testing.T) {
	for _, bad := range []string{
		`not json at all`,
		`{"jsonrpc":"2.0","id":`, // truncated
		`[1,2,3]`,                // not an object
		`"a string"`,             // not an object
	} {
		if _, err := ParseLine([]byte(bad)); err == nil {
			t.Fatalf("ParseLine(%q) should fail", bad)
		}
	}
}

func TestWireJSONRPCVersionRequired(t *testing.T) {
	for _, bad := range []string{
		`{"id":1,"method":"initialize"}`,                 // missing
		`{"jsonrpc":"1.0","id":1,"method":"initialize"}`, // wrong
		`{"jsonrpc":2.0,"id":1,"method":"initialize"}`,   // not a string
	} {
		msg, err := ParseLine([]byte(bad))
		if err != nil {
			t.Fatalf("ParseLine(%q): %v", bad, err)
		}
		if msg.Kind != KindInvalid {
			t.Fatalf("ParseLine(%q) kind = %v, want invalid", bad, msg.Kind)
		}
	}
}

func TestWireShapeContradictionsInvalid(t *testing.T) {
	cases := []struct {
		line string
		why  string
	}{
		{`{"jsonrpc":"2.0","id":1,"method":"m","result":{}}`, "request carrying result"},
		{`{"jsonrpc":"2.0","id":1,"method":"m","error":{"code":-1,"message":"x"}}`, "request carrying error"},
		{`{"jsonrpc":"2.0","id":1}`, "response with neither result nor error"},
		{`{"jsonrpc":"2.0","id":1,"result":{},"error":{"code":-1,"message":"x"}}`, "result and error together"},
		{`{"jsonrpc":"2.0"}`, "neither id nor method"},
		{`{"jsonrpc":"2.0","method":"m","params":{},"result":{}}`, "notification carrying result"},
	}
	for _, c := range cases {
		msg, err := ParseLine([]byte(c.line))
		if err != nil {
			t.Fatalf("ParseLine(%q): %v", c.line, err)
		}
		if msg.Kind != KindInvalid {
			t.Fatalf("%s: kind = %v, want invalid", c.why, msg.Kind)
		}
	}
}

func TestWireStringIDInvalid(t *testing.T) {
	// v1 constrains ids to integers (spec: framing). A string id is a
	// shape violation, not a parse error — the line is attributable when
	// the id is the only offender we can still read, but we classify any
	// non-integer id as invalid; the server skips such lines loudly.
	msg, err := ParseLine([]byte(`{"jsonrpc":"2.0","id":"abc","method":"initialize"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindInvalid {
		t.Fatalf("kind = %v, want invalid (string id)", msg.Kind)
	}
}

func TestWireNegativeIDInvalid(t *testing.T) {
	// id must be >= 0: the server uses -1 to mark "unattributable" in
	// invalid messages, so a negative wire id would be ambiguous.
	msg, err := ParseLine([]byte(`{"jsonrpc":"2.0","id":-4,"method":"m"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindInvalid {
		t.Fatalf("kind = %v, want invalid (negative id)", msg.Kind)
	}
}

func TestWireErrorWithOptionalData(t *testing.T) {
	line := `{"jsonrpc":"2.0","id":9,"error":{"code":-32002,"message":"protocol version mismatch","data":{"client":2,"server":1}}}`
	enc, err := MarshalResponseError(9, -32002, "protocol version mismatch",
		json.RawMessage(`{"client":2,"server":1}`))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(enc) != line {
		t.Fatalf("encode mismatch:\n got %s\nwant %s", enc, line)
	}
	msg, err := ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if msg.Kind != KindResponse || msg.Response.Error == nil || msg.Response.Error.Code != -32002 {
		t.Fatalf("decoded = %+v", msg.Response)
	}
	if strings.TrimSpace(string(msg.Response.Error.Data)) != `{"client":2,"server":1}` {
		t.Fatalf("data = %s", msg.Response.Error.Data)
	}
}
