// conn.go — the framed transport under the host protocol: one JSON
// object per line (NDJSON) over an io.ReadWriteCloser, with
// mutex-serialized writes (notifications, responses, and handler
// goroutines all share one ordered write side) and a Done channel that
// fires exactly once on terminal close (explicit Close, EOF, or write
// failure) — the disconnect signal the fail-closed approval bridge keys
// on.
//
// The dsh close ladder (shutdown → EOF → SIGTERM → SIGKILL, see
// researches/sources/deepseek-harness/llm-protocols-tools.md §3.8
// client README) is simplified here for stdio: no signals are needed —
// Close drains in-flight requests at the Server layer, rejects new
// ones, then closes the transport.
package protocol

import (
	"bufio"
	"io"
	"sync"
)

// MaxLineBytes is the largest single NDJSON line the transport accepts
// (session events carry whole tool results; 16 MiB is the v1 ceiling).
const MaxLineBytes = 16 << 20

// Conn is a single-reader, serialized-writer NDJSON transport.
type Conn struct {
	rwc  io.ReadWriteCloser
	mu   sync.Mutex // serializes writes
	done chan struct{}
	once sync.Once
	scan *bufio.Scanner
}

// NewConn wraps rwc.
func NewConn(rwc io.ReadWriteCloser) *Conn {
	scan := bufio.NewScanner(rwc)
	scan.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return &Conn{rwc: rwc, done: make(chan struct{}), scan: scan}
}

// ReadLine returns the next raw line (newline stripped). A non-nil error
// is terminal (EOF, closed, or oversized line): the connection is closed
// — idempotently, so Done fires exactly once — before the error is
// returned, because Close is the only thing that fires Done and every
// Done subscriber (the fail-closed approval bridge above all) must see a
// client EOF as a disconnect, not as silence. The caller should stop
// reading. Malformed CONTENT is not an error here — classification is
// the caller's job (skip-with-error-event semantics).
func (c *Conn) ReadLine() ([]byte, error) {
	if c.scan.Scan() {
		return append([]byte(nil), c.scan.Bytes()...), nil
	}
	err := c.scan.Err()
	if err == nil {
		err = io.EOF
	}
	c.Close() // terminal read error = disconnect: fire Done now, not never
	return nil, err
}

// WriteLine writes one framed line (appending the newline). Writes are
// serialized; a failed write terminates the connection.
func (c *Conn) WriteLine(b []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	line := make([]byte, 0, len(b)+1)
	line = append(line, b...)
	line = append(line, '\n')
	if _, err := c.rwc.Write(line); err != nil {
		c.Close()
		return err
	}
	return nil
}

// Notify emits one notification. A write failure closes the connection
// (subscribers such as the approval bridge observe Done and fail closed).
func (c *Conn) Notify(method string, params []byte) error {
	b, err := MarshalNotification(method, params)
	if err != nil {
		return err
	}
	return c.WriteLine(b)
}

// Done is closed exactly once when the connection terminates: explicit
// Close, a terminal read error observed by ReadLine (EOF, closed
// transport, oversized line), or a failed write.
func (c *Conn) Done() <-chan struct{} { return c.done }

// Close terminates the connection. It is idempotent.
func (c *Conn) Close() error {
	var err error
	c.once.Do(func() {
		close(c.done)
		err = c.rwc.Close()
	})
	return err
}
