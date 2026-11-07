// client.go — the reference host-protocol Client: request/notification
// helpers over one Conn, a pending-response correlation map, and a
// notification handler registry (the subscription seam tests and future
// frontends — e.g. a vh-solara WebUI — build on).
//
// It is also the executable statement of the client-side half of the
// wire contract (docs/native-engine/host-protocol.md): ids are
// client-minted monotonic integers; notifications are dispatched by
// method; responses are correlated strictly by id.
package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Client is the reference protocol client.
type Client struct {
	conn *Conn

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan *Response

	notifMu   sync.Mutex
	handlers  map[string][]func(params json.RawMessage)
	closeOnce sync.Once
}

// NewClient starts the client read loop on rwc.
func NewClient(rwc io.ReadWriteCloser) *Client {
	c := &Client{
		conn:     NewConn(rwc),
		pending:  make(map[int64]chan *Response),
		handlers: make(map[string][]func(params json.RawMessage)),
	}
	go c.readLoop()
	return c
}

// readLoop correlates responses and dispatches notifications until the
// transport ends.
func (c *Client) readLoop() {
	for {
		line, err := c.conn.ReadLine()
		if err != nil {
			c.failPending(err)
			return
		}
		msg, err := ParseLine(line)
		if err != nil {
			continue // skip malformed lines (server discipline mirrors ours)
		}
		switch msg.Kind {
		case KindResponse:
			c.mu.Lock()
			ch, ok := c.pending[msg.Response.ID]
			if ok {
				delete(c.pending, msg.Response.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- msg.Response
			}
		case KindNotification:
			c.notifMu.Lock()
			fns := make([]func(json.RawMessage), len(c.handlers[msg.Notification.Method]))
			copy(fns, c.handlers[msg.Notification.Method])
			c.notifMu.Unlock()
			for _, fn := range fns {
				fn(msg.Notification.Params)
			}
		case KindRequest:
			// v1: the server issues no requests; ignored (forward compat).
		case KindInvalid:
			// Malformed server line: skipped.
		}
	}
}

// Call issues one request and blocks for its response. params may be nil
// (omitted), a struct, or a map; result (when non-nil) is decoded from
// the response result. A protocol error response returns as *Error.
func (c *Client) Call(method string, params any, result any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("protocol client: marshal params: %w", err)
		}
		raw = b
	}
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *Response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	line, err := MarshalRequest(id, method, raw)
	if err != nil {
		return err
	}
	if err := c.conn.WriteLine(line); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("protocol client: send %s: %w", method, err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return resp.Error
		}
		if result != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, result); err != nil {
				return fmt.Errorf("protocol client: decode %s result: %w", method, err)
			}
		}
		return nil
	case <-c.conn.Done():
		return fmt.Errorf("protocol client: connection closed while %s was pending", method)
	}
}

// Notify emits one notification to the server.
func (c *Client) Notify(method string, params any) error {
	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		raw = b
	}
	line, err := MarshalNotification(method, raw)
	if err != nil {
		return err
	}
	return c.conn.WriteLine(line)
}

// OnNotification registers fn for one notification method.
func (c *Client) OnNotification(method string, fn func(params json.RawMessage)) {
	c.notifMu.Lock()
	defer c.notifMu.Unlock()
	c.handlers[method] = append(c.handlers[method], fn)
}

// Done is closed exactly once when the transport terminates (peer exit,
// explicit Close, or a failed write). Frontends that idle on user input
// (e.g. a REPL between turns) select on it to notice daemon death
// without a pending Call — the additive client-half companion of the
// server's close ladder.
func (c *Client) Done() <-chan struct{} { return c.conn.Done() }

// Close terminates the client.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.conn.Close()
	})
	return err
}

// failPending unblocks every waiting Call on transport death.
func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- &Response{ID: id, Error: &Error{
			Code:    ErrEngine,
			Message: fmt.Sprintf("connection closed: %v", err),
		}}
	}
}

// Receipt is the enqueue receipt returned by session/dispatch.
type Receipt struct {
	JobID string `json:"jobId"`
}

// DispatchPrompt enqueues a prompt-shaped background job and returns
// its receipt immediately (the async contract's client half; the
// interpretation of the "prompt" kind belongs to the engine's executor
// wiring — dsh "prompt = enqueue receipt only").
func (c *Client) DispatchPrompt(text string) (*Receipt, error) {
	var rec Receipt
	err := c.Call("session/dispatch", struct {
		Kind    string         `json:"kind"`
		Payload map[string]any `json:"payload,omitempty"`
	}{Kind: "prompt", Payload: map[string]any{"text": text}}, &rec)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}
