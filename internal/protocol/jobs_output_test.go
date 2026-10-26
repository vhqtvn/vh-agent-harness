// jobs_output_test.go — the jobs/output wire method (P6 tailing) over
// the REAL engine seams: an OutputExecutor streaming into the capture
// channel, mid-flight cursor reads concurrent with the writing job
// body, typed errors (unknown job / ahead / evicted), and the
// strict-params inbound discipline.
package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
)

// gatingStreamExec is an OutputExecutor whose body writes lines, gates
// before a final one, and returns a terminal detail.
type gatingStreamExec struct {
	gate   chan struct{}
	wrote  atomic.Int64
	detail string
}

func (e *gatingStreamExec) Run(context.Context, jobs.Job) error { return nil }

func (e *gatingStreamExec) RunWithOutput(_ context.Context, _ jobs.Job, out io.Writer) (string, error) {
	_, _ = io.WriteString(out, "tick 1\ntick 2\n")
	e.wrote.Add(int64(len("tick 1\ntick 2\n")))
	<-e.gate
	_, _ = io.WriteString(out, "tick 3\n")
	e.wrote.Add(int64(len("tick 3\n")))
	return e.detail, nil
}

func callJobsOutput(t *testing.T, c *Client, jobID string, offset int64) (jobs.OutputChunk, error) {
	t.Helper()
	var chunk jobs.OutputChunk
	err := c.Call("jobs/output", map[string]any{"jobId": jobID, "offset": offset}, &chunk)
	return chunk, err
}

// TestJobsOutput_MidFlightTailAndSettlement drives the full read
// lifecycle over the wire: dispatch → receipt; mid-flight partial tail
// while the job body is gated; release; settled state; post-settle
// read of the remaining tail with cursor arithmetic exact.
func TestJobsOutput_MidFlightTailAndSettlement(t *testing.T) {
	dir := t.TempDir()
	exec := &gatingStreamExec{gate: make(chan struct{}), detail: "cause=exit exitCode=0"}
	client, _, cleanup := newServerPair(t, dir, exec)
	defer cleanup()

	mustCall(t, client, "initialize", map[string]any{"protocolVersion": 1}, nil)
	mustCall(t, client, "session/create", map[string]any{"sessionId": "sess-jout"}, nil)

	var receipt jobs.Receipt
	mustCall(t, client, "session/dispatch", map[string]any{"kind": "stream"}, &receipt)
	if receipt.JobID == "" {
		t.Fatal("empty receipt")
	}

	// Mid-flight: the first two lines are readable while the job is
	// still gated (concurrent reader vs writing job body).
	deadline := time.Now().Add(5 * time.Second)
	var mid jobs.OutputChunk
	for time.Now().Before(deadline) {
		ch, err := callJobsOutput(t, client, receipt.JobID, 0)
		if err == nil && ch.Chunk != "" {
			mid = ch
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if mid.Chunk != "tick 1\ntick 2\n" {
		t.Fatalf("mid-flight chunk = %+v", mid)
	}
	if mid.State != jobs.OutputStateRunning || mid.NextOffset != int64(len("tick 1\ntick 2\n")) || mid.Written != mid.NextOffset {
		t.Fatalf("mid-flight cursor arithmetic = %+v", mid)
	}

	close(exec.gate)
	waitSettled(t, client, receipt.JobID)

	// Continue exactly at the cursor: the remaining tail, then EOF.
	rest, err := callJobsOutput(t, client, receipt.JobID, mid.NextOffset)
	if err != nil {
		t.Fatal(err)
	}
	if rest.Chunk != "tick 3\n" || rest.State != jobs.OutputStateSettled || rest.HasMore {
		t.Fatalf("post-settle chunk = %+v", rest)
	}
	if rest.NextOffset != rest.Written {
		t.Fatalf("cursor %d != written %d", rest.NextOffset, rest.Written)
	}
	eof, err := callJobsOutput(t, client, receipt.JobID, rest.NextOffset)
	if err != nil || eof.Chunk != "" || eof.HasMore {
		t.Fatalf("eof read = %+v err=%v", eof, err)
	}
}

func waitSettled(t *testing.T, c *Client, jobID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var st struct {
			Jobs []jobs.Status `json:"jobs"`
		}
		if err := c.Call("jobs/status", nil, &st); err == nil {
			for _, j := range st.Jobs {
				if j.JobID == jobID && j.State == jobs.StateSettled {
					return
				}
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("job %s did not settle", jobID)
}

// TestJobsOutput_TypedErrors: unknown job, offset ahead, and the
// strict-params discipline (an unknown params field is rejected).
func TestJobsOutput_TypedErrors(t *testing.T) {
	dir := t.TempDir()
	exec := &gatingStreamExec{gate: make(chan struct{})}
	client, _, cleanup := newServerPair(t, dir, exec)
	defer cleanup()
	mustCall(t, client, "initialize", map[string]any{"protocolVersion": 1}, nil)
	mustCall(t, client, "session/create", map[string]any{"sessionId": "sess-jout2"}, nil)

	// Unknown job: -32602 with a typed protocol error.
	err := client.Call("jobs/output", map[string]any{"jobId": "ghost-9", "offset": 0}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams || !strings.Contains(perr.Message, "unknown job") {
		t.Fatalf("err = %v, want -32602 unknown job", err)
	}

	var receipt jobs.Receipt
	mustCall(t, client, "session/dispatch", map[string]any{"kind": "stream"}, &receipt)
	// Give the body a moment to land its first write, then read ahead.
	time.Sleep(50 * time.Millisecond)
	err = client.Call("jobs/output", map[string]any{"jobId": receipt.JobID, "offset": 9999}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams || !strings.Contains(perr.Message, "ahead") {
		t.Fatalf("err = %v, want -32602 ahead", err)
	}
	close(exec.gate)
	waitSettled(t, client, receipt.JobID)

	// Unknown params field: strict decode rejects (v1 strictness).
	err = client.Call("jobs/output", map[string]any{"jobId": receipt.JobID, "offset": 0, "maxChunk": 99}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams {
		t.Fatalf("err = %v, want -32602 for unknown params field", err)
	}

	// No active session ⇒ -32003.
	c2, _, cleanup2 := newPairNoSession(t)
	defer cleanup2()
	mustCall(t, c2, "initialize", map[string]any{"protocolVersion": 1}, nil)
	err = c2.Call("jobs/output", map[string]any{"jobId": "x", "offset": 0}, nil)
	if !errors.As(err, &perr) || perr.Code != ErrNoSession {
		t.Fatalf("err = %v, want -32003", err)
	}
}

// newPairNoSession boots a server pair that never creates a session.
func newPairNoSession(t *testing.T) (*Client, *FileEngine, func()) {
	return newServerPair(t, t.TempDir(), &gatingStreamExec{gate: make(chan struct{})})
}

func mustCall(t *testing.T, c *Client, method string, params any, into any) {
	t.Helper()
	if err := c.Call(method, params, into); err != nil {
		t.Fatalf("%s: %v", method, err)
	}
}

// TestJobsOutput_EvictionTypedErrorOverWire proves the settled+evicted
// case: a producer that wraps the retention window leaves the typed
// evicted error carrying the base in data.
func TestJobsOutput_EvictionTypedErrorOverWire(t *testing.T) {
	dir := t.TempDir()
	eng := &FileEngine{Dir: dir, Executor: &bigProducer{retention: 64}, JobsOpts: jobs.Options{OutputRetentionBytes: 64}}
	svc, cli := net.Pipe()
	srv := NewServer(eng, NewConn(svc), ServerOptions{})
	served := make(chan error, 1)
	go func() { served <- srv.Serve(nil) }()
	client := NewClient(cli)
	defer func() {
		_ = client.Close()
		select {
		case <-served:
		case <-time.After(2 * time.Second):
		}
	}()

	mustCall(t, client, "initialize", map[string]any{"protocolVersion": 1}, nil)
	mustCall(t, client, "session/create", map[string]any{"sessionId": "sess-jout3"}, nil)
	var receipt jobs.Receipt
	mustCall(t, client, "session/dispatch", map[string]any{"kind": "big"}, &receipt)
	waitSettled(t, client, receipt.JobID)

	// Offset 0 fell behind the 64-byte window: typed evicted error with
	// data naming the readable base.
	err := client.Call("jobs/output", map[string]any{"jobId": receipt.JobID, "offset": 0}, nil)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != ErrInvalidParams || !strings.Contains(perr.Message, "retention") {
		t.Fatalf("err = %v, want -32602 evicted", err)
	}
	if len(perr.Data) == 0 || !strings.Contains(string(perr.Data), `"evictedBase"`) {
		t.Fatalf("evicted data missing base: %s", perr.Data)
	}
	// Parse the base out of data and read the retained tail from there.
	var data struct {
		EvictedBase int64 `json:"evictedBase"`
	}
	if err := json.Unmarshal(perr.Data, &data); err != nil {
		t.Fatal(err)
	}
	tail, err := callJobsOutput(t, client, receipt.JobID, data.EvictedBase)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Chunk == "" || !strings.HasSuffix(tail.Chunk, "END\n") {
		t.Fatalf("retained tail = %q", tail.Chunk)
	}
}

// bigProducer writes more than the retention window then settles.
type bigProducer struct{ retention int }

func (e *bigProducer) Run(context.Context, jobs.Job) error { return nil }

func (e *bigProducer) RunWithOutput(_ context.Context, _ jobs.Job, out io.Writer) (string, error) {
	_, _ = io.WriteString(out, strings.Repeat("x", 200)+"\nEND\n")
	return "cause=exit exitCode=0", nil
}
