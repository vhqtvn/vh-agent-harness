// approval_test.go — slice 5 step 3 (red): the ProtocolApprover bridge.
//
// Model (docs/native-engine/host-protocol.md "Approval interaction";
// dsh F-PIPE-2 fail-closed one-shot approval): ask → approval/request
// notification → approval/respond → decision. Absent, unanswerable,
// timed-out, or disconnected approval is ALWAYS a deny.
package protocol

import (
	"bufio"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// approvalPeer drives a ProtocolApprover from the wire side.
type approvalPeer struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
	notes  chan *Notification
}

func newApprovalPeer(t *testing.T, timeout time.Duration) (*ProtocolApprover, *approvalPeer, func()) {
	t.Helper()
	svc, cli := net.Pipe()
	conn := NewConn(svc)
	approver := NewProtocolApprover(conn, timeout)
	peer := &approvalPeer{t: t, conn: cli, reader: bufio.NewReader(cli), notes: make(chan *Notification, 16)}
	go func() {
		for {
			line, err := peer.reader.ReadString('\n')
			if err != nil {
				return
			}
			msg, perr := ParseLine([]byte(line[:len(line)-1]))
			if perr != nil || msg.Kind != KindNotification {
				continue
			}
			peer.notes <- msg.Notification
		}
	}()
	cleanup := func() { _ = cli.Close(); _ = conn.Close() }
	return approver, peer, cleanup
}

func readApprovalRequest(t *testing.T, peer *approvalPeer) (string, session.ToolCall, string) {
	t.Helper()
	select {
	case note := <-peer.notes:
		if note.Method != "approval/request" {
			t.Fatalf("expected approval/request, got %s", note.Method)
		}
		var p struct {
			ApprovalID string           `json:"approvalId"`
			Call       session.ToolCall `json:"call"`
			Reason     string           `json:"reason"`
		}
		if err := json.Unmarshal(note.Params, &p); err != nil {
			t.Fatalf("params: %v (%s)", err, note.Params)
		}
		return p.ApprovalID, p.Call, p.Reason
	case <-time.After(2 * time.Second):
		t.Fatal("no approval/request notification arrived")
		return "", session.ToolCall{}, ""
	}
}

func TestApprovalGrantOverWire(t *testing.T) {
	approver, peer, stop := newApprovalPeer(t, 0)
	defer stop()

	call := session.ToolCall{ID: "call-1", Name: "write_file", Args: json.RawMessage(`{"path":"a.txt"}`)}
	decCh := make(chan tools.ApprovalDecision, 1)
	go func() {
		decCh <- approver.Approve(nil, call, "mutates workspace")
	}()

	id, gotCall, reason := readApprovalRequest(t, peer)
	if id == "" || gotCall.ID != "call-1" || gotCall.Name != "write_file" || reason != "mutates workspace" {
		t.Fatalf("approval/request = %q %+v %q", id, gotCall, reason)
	}
	if err := approver.Respond(id, true, ""); err != nil {
		t.Fatalf("respond: %v", err)
	}
	select {
	case d := <-decCh:
		if !d.Allow {
			t.Fatalf("decision = %+v, want allow", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Approve did not return after grant")
	}
}

func TestApprovalDenyOverWire(t *testing.T) {
	approver, peer, stop := newApprovalPeer(t, 0)
	defer stop()

	decCh := make(chan tools.ApprovalDecision, 1)
	go func() {
		decCh <- approver.Approve(nil, session.ToolCall{ID: "c", Name: "rm"}, "dangerous")
	}()
	id, _, _ := readApprovalRequest(t, peer)
	if err := approver.Respond(id, false, "not today"); err != nil {
		t.Fatalf("respond: %v", err)
	}
	d := <-decCh
	if d.Allow || d.Reason != "not today" {
		t.Fatalf("decision = %+v, want deny with reason", d)
	}
}

func TestApprovalTimeoutDenies(t *testing.T) {
	approver, peer, stop := newApprovalPeer(t, time.Second)
	defer stop()
	timeoutCh := make(chan time.Time)
	approver.after = func(time.Duration) <-chan time.Time { return timeoutCh }

	decCh := make(chan tools.ApprovalDecision, 1)
	go func() {
		decCh <- approver.Approve(nil, session.ToolCall{ID: "c", Name: "n"}, "r")
	}()
	readApprovalRequest(t, peer) // Approve is parked, notification drained
	close(timeoutCh)             // fire the injected deadline deterministically

	d := <-decCh
	if d.Allow {
		t.Fatal("timeout must deny")
	}
	if d.Reason == "" {
		t.Fatal("timeout deny should carry a reason")
	}
	// A late respond for the expired approval is refused.
	if err := approver.Respond("approval-1", true, ""); err == nil {
		t.Fatal("respond after timeout should fail")
	}
}

func TestApprovalDisconnectDeniesAllPending(t *testing.T) {
	svc, cli := net.Pipe()
	conn := NewConn(svc)
	approver := NewProtocolApprover(conn, 0) // no timeout: only disconnect

	const n = 2
	decCh := make(chan tools.ApprovalDecision, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			decCh <- approver.Approve(nil, session.ToolCall{ID: "c", Name: "n"}, "r")
		}(i)
	}
	// Drain the two approval/request notifications so Approve is parked.
	done := make(chan struct{})
	go func() {
		r := bufio.NewReader(cli)
		for i := 0; i < n; i++ {
			if _, err := r.ReadString('\n'); err != nil {
				return
			}
		}
		close(done)
	}()
	<-done

	_ = conn.Close() // the disconnect

	for i := 0; i < n; i++ {
		d := <-decCh
		if d.Allow {
			t.Fatal("disconnect must deny every pending approval")
		}
	}
	_ = cli.Close()
}

func TestApprovalRespondUnknownID(t *testing.T) {
	svc, cli := net.Pipe()
	defer cli.Close()
	approver := NewProtocolApprover(NewConn(svc), 0)
	if err := approver.Respond("approval-nope", true, ""); err == nil {
		t.Fatal("unknown approval id should error")
	}
}

func TestApprovalDoubleRespondFails(t *testing.T) {
	approver, peer, stop := newApprovalPeer(t, 0)
	defer stop()

	decCh := make(chan tools.ApprovalDecision, 1)
	go func() {
		decCh <- approver.Approve(nil, session.ToolCall{ID: "c", Name: "n"}, "r")
	}()
	id, _, _ := readApprovalRequest(t, peer)
	if err := approver.Respond(id, true, ""); err != nil {
		t.Fatalf("first respond: %v", err)
	}
	<-decCh
	if err := approver.Respond(id, true, ""); err == nil {
		t.Fatal("second respond for a resolved approval should fail")
	}
}
