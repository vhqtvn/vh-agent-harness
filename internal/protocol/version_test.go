// version_test.go — slice 5 step 2 (red): protocolVersion negotiation in
// `initialize`. Mismatch is a hard error with NO partial state (the
// server stays uninitialized until a matching handshake lands).
package protocol

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"
)

// pipeClient is a minimal raw wire peer for driving one Server.
type pipeClient struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
}

func newPipeClient(t *testing.T) (*pipeClient, *Server, func()) {
	t.Helper()
	eng := &stubEngine{}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	done := make(chan error, 1)
	go func() { done <- srv.Serve(nil) }()
	pc := &pipeClient{t: t, conn: cli, reader: bufio.NewReader(cli)}
	cleanup := func() {
		_ = cli.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("server did not exit")
		}
	}
	return pc, srv, cleanup
}

// send writes one raw NDJSON line.
func (pc *pipeClient) send(line string) {
	pc.t.Helper()
	_ = pc.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := pc.conn.Write([]byte(line + "\n")); err != nil {
		pc.t.Fatalf("send %s: %v", line, err)
	}
}

// recv reads one raw NDJSON line (without the newline).
func (pc *pipeClient) recv() string {
	pc.t.Helper()
	_ = pc.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := pc.reader.ReadString('\n')
	if err != nil {
		pc.t.Fatalf("recv: %v", err)
	}
	return line[:len(line)-1]
}

// roundtrip sends a request line and returns the decoded response.
func (pc *pipeClient) roundtrip(line string) *Response {
	pc.t.Helper()
	pc.send(line)
	msg, err := ParseLine([]byte(pc.recv()))
	if err != nil {
		pc.t.Fatalf("response undecodable: %v (%s)", err, line)
	}
	if msg.Kind != KindResponse {
		pc.t.Fatalf("expected response, got kind=%v (%+v)", msg.Kind, msg)
	}
	return msg.Response
}

func TestInitializeMatch(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()

	resp := pc.roundtrip(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)
	if resp.Error != nil {
		t.Fatalf("initialize failed: %+v", resp.Error)
	}
	var res struct {
		ProtocolVersion int `json:"protocolVersion"`
		ServerInfo      struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
		Capabilities struct {
			Approval           bool `json:"approval"`
			Jobs               bool `json:"jobs"`
			EventNotifications bool `json:"eventNotifications"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("result: %v (%s)", err, resp.Result)
	}
	if res.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocolVersion = %d, want %d", res.ProtocolVersion, ProtocolVersion)
	}
	if res.ServerInfo.Name != "vh-agent-harness" {
		t.Fatalf("serverInfo.name = %s", res.ServerInfo.Name)
	}
	if !res.Capabilities.Approval || !res.Capabilities.Jobs || !res.Capabilities.EventNotifications {
		t.Fatalf("capabilities = %+v", res.Capabilities)
	}
}

func TestInitializeMismatch(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()

	resp := pc.roundtrip(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":2}}`)
	if resp.Error == nil || resp.Error.Code != ErrVersionMismatch {
		t.Fatalf("mismatch should error %d, got %+v", ErrVersionMismatch, resp.Error)
	}
	// data carries both sides for diagnosis.
	var d struct {
		Server int `json:"server"`
	}
	if err := json.Unmarshal(resp.Error.Data, &d); err != nil || d.Server != 1 {
		t.Fatalf("error data = %s (%v)", resp.Error.Data, err)
	}

	// NO partial state: the server is still uninitialized. A later
	// method call is rejected with "initialize required"...
	resp2 := pc.roundtrip(`{"jsonrpc":"2.0","id":2,"method":"jobs/status"}`)
	if resp2.Error == nil || resp2.Error.Code != ErrInitializeRequired {
		t.Fatalf("post-mismatch method should require initialize, got %+v", resp2.Error)
	}
	// ...and a retry with the right version succeeds cleanly.
	resp3 := pc.roundtrip(`{"jsonrpc":"2.0","id":3,"method":"initialize","params":{"protocolVersion":1}}`)
	if resp3.Error != nil {
		t.Fatalf("retry initialize failed: %+v", resp3.Error)
	}
}

func TestInitializeRequiredBeforeOtherMethods(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()

	resp := pc.roundtrip(`{"jsonrpc":"2.0","id":1,"method":"session/surface"}`)
	if resp.Error == nil || resp.Error.Code != ErrInitializeRequired {
		t.Fatalf("expected initialize-required error, got %+v", resp.Error)
	}
}

func TestInitializeIdempotent(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()

	for i := int64(1); i <= 2; i++ {
		resp := pc.roundtrip(`{"jsonrpc":"2.0","id":` + itoa(i) + `,"method":"initialize","params":{"protocolVersion":1}}`)
		if resp.Error != nil {
			t.Fatalf("initialize #%d failed: %+v", i, resp.Error)
		}
	}
}

func TestUnknownMethodNotFound(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()
	pc.roundtrip(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)

	resp := pc.roundtrip(`{"jsonrpc":"2.0","id":2,"method":"session/nope"}`)
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("unknown method should be -32601, got %+v", resp.Error)
	}
}

func TestMalformedLineSkippedWithErrorEvent(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()
	pc.roundtrip(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)

	// A garbage line must NOT kill the connection: it is skipped and a
	// protocol/error notification is emitted (dsh malformed-line skip).
	pc.send(`this is not json`)
	note := pc.recv()
	msg, err := ParseLine([]byte(note))
	if err != nil {
		t.Fatalf("notification undecodable: %v (%s)", err, note)
	}
	if msg.Kind != KindNotification || msg.Notification.Method != "protocol/error" {
		t.Fatalf("expected protocol/error notification, got %+v", msg)
	}
	var p struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(msg.Notification.Params, &p); err != nil || p.Code != -32700 {
		t.Fatalf("protocol/error params = %s (%v)", msg.Notification.Params, err)
	}

	// The connection is still alive.
	resp := pc.roundtrip(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":1}}`)
	if resp.Error != nil {
		t.Fatalf("connection should survive a malformed line: %+v", resp.Error)
	}
}

func TestInvalidAttributableRejected(t *testing.T) {
	pc, _, stop := newPipeClient(t)
	defer stop()
	pc.roundtrip(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`)

	// Unknown envelope field on an attributable line: -32600 response.
	resp := pc.roundtrip(`{"jsonrpc":"2.0","id":9,"method":"jobs/status","extra":1}`)
	if resp.Error == nil || resp.Error.Code != -32600 || resp.ID != 9 {
		t.Fatalf("unknown-field line should answer -32600 with id, got %+v", resp)
	}

	// Unattributable shape violation: protocol/error notification.
	pc.send(`{"jsonrpc":"2.0","bogus":true}`)
	note := pc.recv()
	msg, err := ParseLine([]byte(note))
	if err != nil || msg.Kind != KindNotification || msg.Notification.Method != "protocol/error" {
		t.Fatalf("expected protocol/error notification, got err=%v %+v", err, msg)
	}
}

func itoa(i int64) string {
	b, _ := json.Marshal(i)
	return string(b)
}
