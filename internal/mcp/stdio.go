// stdio.go — the LOCAL (stdio) transport: newline-delimited JSON-RPC
// 2.0 over the subprocess's stdin/stdout. Subprocess lifecycle:
//
//   - CLEAN, EXPLICIT env: the same base set the run_shell tool builds
//     (TERM=dumb, PATH, HOME, LANG) — nothing else from the parent
//     survives. The configured env map merges in AFTER the scrub
//     discipline: names matching KEY|SECRET|TOKEN|PASSWORD
//     (case-insensitive) or the engine credential prefix are DROPPED
//     (the shell env policy; the scrub wins). The battery + unit tests
//     pin this from both sides.
//   - one stdout reader goroutine parses each line and routes responses
//     to pending calls BY ID; server-initiated frames (notifications,
//     requests) are ignored (v1 host posture — real servers emit
//     progress/logging notifications DURING a call); a GARBAGE line
//     fails every pending call with a typed error (fail-closed; with
//     barrier scheduling there is at most one in flight —
//     conservative by design).
//   - stderr is captured into a small bounded ring (diagnostics on
//     transport failure), redacted through the server's redactor.
//   - shutdown ladder: close stdin → wait (3s) → Kill → wait. Close is
//     idempotent and never hangs.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// sensitiveEnvPattern mirrors the shell tool's SENSITIVE_ENV_PATTERN
// (/KEY|PASSWORD|SECRET|TOKEN/i) — the name-match drop both policies
// share. Replicated here (not imported) because the shell package's
// copy is intentionally unexported; the two must stay in sync.
var sensitiveEnvPattern = regexp.MustCompile(`(?i)KEY|PASSWORD|SECRET|TOKEN`)

// engineCredentialPrefix mirrors the shell tool's engine-credential
// namespace drop rule.
const engineCredentialPrefix = "VH_AGENT_HARNESS_"

// isSensitiveEnvName reports whether an env NAME is dropped by the
// scrub discipline (secret-shaped OR engine-credential namespace).
func isSensitiveEnvName(name string) bool {
	return sensitiveEnvPattern.MatchString(name) || strings.HasPrefix(name, engineCredentialPrefix)
}

// buildChildEnv constructs the subprocess env: the explicit base set,
// then the configured env merged AFTER the scrub (the scrub wins on
// names — a configured API_KEY never reaches the child).
func buildChildEnv(sc *ServerConfig) []string {
	env := []string{"TERM=dumb"}
	if p := os.Getenv("PATH"); p != "" {
		env = append(env, "PATH="+p)
	} else {
		env = append(env, "PATH=/usr/local/bin:/usr/bin:/bin")
	}
	if h := os.Getenv("HOME"); h != "" {
		env = append(env, "HOME="+h)
	}
	if l := os.Getenv("LANG"); l != "" {
		env = append(env, "LANG="+l)
	} else {
		env = append(env, "LANG=C.UTF-8")
	}
	for k, v := range sc.Env {
		if isSensitiveEnvName(k) {
			continue // scrub wins over the configured env
		}
		env = append(env, k+"="+v)
	}
	return env
}

// ErrClosed is the typed error for calls on a closed client.
var ErrClosed = errors.New("mcp: transport closed")

// StdioClient is one local MCP server subprocess.
type StdioClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	red    redactor
	ids    idSource
	logf   func(string, ...any)
	stderr *boundedLines

	// notifications counts server-initiated frames seen (notifications
	// and requests): the starve-safe observability for the v1 ignore
	// posture — never per-frame logging, never pending-failing.
	notifications atomic.Int64

	writeMu sync.Mutex // serializes stdin writes (one line per request)
	mu      sync.Mutex // guards pending + garbageSeen
	pending map[int64]chan *rpcResponse
	closed  bool

	readerDone chan struct{}
	closeOnce  sync.Once
	closeErr   error
	waitOnce   sync.Once
	waitErr    error
}

// stderrLines is the bounded stderr ring retained for diagnostics.
const stderrLines = 16

// DialStdio starts the server subprocess. The caller owns shutdown via
// Close (Initialize is a separate handshake step).
func DialStdio(sc *ServerConfig, red redactor, logf func(string, ...any)) (*StdioClient, error) {
	if len(sc.Command) == 0 {
		return nil, fmt.Errorf("mcp: local server has no command")
	}
	c := &StdioClient{
		red:        red,
		logf:       logf,
		pending:    map[int64]chan *rpcResponse{},
		stderr:     newBoundedLines(stderrLines),
		readerDone: make(chan struct{}),
	}
	c.cmd = exec.Command(sc.Command[0], sc.Command[1:]...)
	c.cmd.Env = buildChildEnv(sc)
	c.cmd.Stderr = c.stderr
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	c.stdin = stdin
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	if err := c.cmd.Start(); err != nil {
		return nil, errors.New(c.red.Clean(fmt.Sprintf("mcp: start server process: %v", err)))
	}
	go c.readLoop(stdout)
	return c, nil
}

// readLoop is the single stdout reader goroutine. On stdout EOF/error
// it reaps the child (single-waiter discipline — Close shares the same
// waitOnce) and fails every pending call with the exit facts + the
// redacted stderr tail.
func (c *StdioClient) readLoop(r io.Reader) {
	defer close(c.readerDone)
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if line == "" && err != nil {
			c.failAllPending(c.exitError(err))
			return
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed != "" {
			c.handleLine(trimmed)
		}
		if err != nil {
			c.failAllPending(c.exitError(err))
			return
		}
	}
}

// wait reaps the child exactly once (reader-EOF and Close share it).
func (c *StdioClient) wait() error {
	c.waitOnce.Do(func() { c.waitErr = c.cmd.Wait() })
	return c.waitErr
}

// exitError builds the typed transport-death error: exit facts plus the
// redacted stderr tail (a fast-dying child says WHY on stderr).
func (c *StdioClient) exitError(readErr error) error {
	werr := c.wait()
	msg := fmt.Sprintf("mcp: server process exited (%v); stdout read: %v", werr, readErr)
	if tail := c.red.Clean(c.stderr.tail()); tail != "" {
		msg += "; server stderr tail: " + tail
	}
	return errors.New(msg)
}

// handleLine routes one stdout line by JSON-RPC 2.0 frame class:
//
//   - a frame carrying `method` is SERVER-INITIATED (a notification
//     has no id; a server request carries one) → IGNORED under the v1
//     host posture, never failing pending calls. Real servers emit
//     progress/logging notifications DURING a tools/call — exactly
//     when pending is non-empty — so treating them as garbage breaks
//     every chatty server. Observability is starve-safe: count every
//     frame, log only the first (a chatty server can neither stall
//     the reader nor spam the log).
//   - a frame with `id` and a result/error payload is a RESPONSE →
//     existing id correlation.
//   - anything else — unparseable JSON, or a frame with no method and
//     no id (e.g. `{}`), or an id-bearing frame with NO payload (not a
//     valid response) — is garbage → every pending call fails with a
//     typed error (fail-closed).
func (c *StdioClient) handleLine(line string) {
	var resp rpcResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		c.garbageLine(line)
		return
	}
	if resp.Method != "" {
		if c.notifications.Add(1) == 1 && c.logf != nil {
			c.logf("mcp: server-initiated frame (%s) ignored — v1 host posture; further frames counted, not logged", resp.Method)
		}
		return
	}
	if resp.ID == nil || (resp.Result == nil && resp.Error == nil) {
		c.garbageLine(line)
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[*resp.ID]
	delete(c.pending, *resp.ID)
	c.mu.Unlock()
	if ok {
		ch <- &resp // buffered(1): never blocks the reader
	}
}

// garbageLine fails every pending call naming the offending output
// (typed, fail-closed — no call may hang on a broken server); with no
// pending calls it is ignored (a chatty server's junk never broke us).
func (c *StdioClient) garbageLine(line string) {
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n > 0 {
		c.failAllPending(fmt.Errorf("mcp: server sent invalid JSON-RPC output (%q)", c.red.Clean(truncateForError(line))))
	}
}

// failAllPending resolves every pending call with err (reader death or
// garbage — no call may hang on a dead transport).
func (c *StdioClient) failAllPending(err error) {
	c.mu.Lock()
	pending := c.pending
	c.pending = map[int64]chan *rpcResponse{}
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- &rpcResponse{ID: nil, Error: &rpcError{Code: -32000, Message: err.Error()}}
	}
}

// call writes one request and awaits its response bounded by ctx.
func (c *StdioClient) call(ctx context.Context, method string, params json.RawMessage) (*rpcResponse, error) {
	id := c.ids.next()
	req := rpcRequest{JSONRPC: "2.0", ID: &id, Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal %s: %w", method, err)
	}
	ch := make(chan *rpcResponse, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.pending[id] = ch
	c.mu.Unlock()

	c.writeMu.Lock()
	if c.closed {
		c.writeMu.Unlock()
		c.dropPending(id)
		return nil, ErrClosed
	}
	_, werr := c.stdin.Write(append(body, '\n'))
	c.writeMu.Unlock()
	if werr != nil {
		c.dropPending(id)
		return nil, errors.New(c.red.Clean(fmt.Sprintf("mcp: write %s: %v", method, werr)))
	}

	select {
	case <-ctx.Done():
		c.dropPending(id)
		return nil, fmt.Errorf("mcp: %s deadline exceeded: %w", method, ctx.Err())
	case resp := <-ch:
		if resp.Error != nil {
			return nil, errors.New(c.red.Clean(resp.Error.Error()))
		}
		return resp, nil
	}
}

// dropPending removes an abandoned pending entry (timeout/cancel).
func (c *StdioClient) dropPending(id int64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// notify writes one notification (no response expected).
func (c *StdioClient) notify(method string) error {
	req := rpcRequest{JSONRPC: "2.0", Method: method}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mcp: marshal %s: %w", method, err)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.closed {
		return ErrClosed
	}
	if _, err := c.stdin.Write(append(body, '\n')); err != nil {
		return errors.New(c.red.Clean(fmt.Sprintf("mcp: write %s: %v", method, err)))
	}
	return nil
}

// Initialize performs the handshake: initialize → check the negotiated
// version (mismatch logs + proceeds) → notifications/initialized.
func (c *StdioClient) Initialize(ctx context.Context) error {
	p := initializeParams{
		ProtocolVersion: ClientProtocolVersion,
		Capabilities:    map[string]any{},
	}
	p.ClientInfo.Name = "vh-agentd"
	p.ClientInfo.Version = "0.1.0"
	params, _ := json.Marshal(p)
	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err // typed; exit facts + stderr tail already embedded by the reader path
	}
	var initRes initializeResult
	if err := json.Unmarshal(resp.Result, &initRes); err != nil {
		return fmt.Errorf("mcp: initialize result unparseable: %v", err)
	}
	if initRes.ProtocolVersion != ClientProtocolVersion && c.logf != nil {
		c.logf("mcp: server negotiated protocolVersion %s (host speaks %s) — proceeding (tools surface is stable)", initRes.ProtocolVersion, ClientProtocolVersion)
	}
	return c.notify("notifications/initialized")
}

// ListTools fetches the advertisement.
func (c *StdioClient) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := c.call(ctx, "tools/list", json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}
	var res toolsListResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("mcp: tools/list result unparseable: %v", err)
	}
	return res.Tools, nil
}

// CallTool invokes one tool; a tool-level failure (isError) is DATA,
// not an error — the registry maps it to the typed tool error.
func (c *StdioClient) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*CallResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	params, _ := json.Marshal(toolsCallParams{Name: name, Arguments: arguments})
	resp, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}
	var res CallResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("mcp: tools/call(%s) result unparseable: %v", name, err)
	}
	return &res, nil
}

// Close runs the shutdown ladder: close stdin → wait (3s) → kill →
// wait. Idempotent; never hangs.
func (c *StdioClient) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		_ = c.stdin.Close()
		done := make(chan struct{})
		go func() {
			_ = c.wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
		}
		c.failAllPending(ErrClosed)
		c.closeErr = nil
	})
	return c.closeErr
}

// boundedLines is a bounded line ring over a server's stderr.
type boundedLines struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newBoundedLines(max int) *boundedLines { return &boundedLines{max: max} }

func (b *boundedLines) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ln := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if ln == "" {
			continue
		}
		b.lines = append(b.lines, ln)
		if len(b.lines) > b.max {
			b.lines = b.lines[len(b.lines)-b.max:]
		}
	}
	return len(p), nil
}

func (b *boundedLines) tail() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.lines, " | ")
}

// truncateForError bounds an error-embedded fragment.
func truncateForError(s string) string {
	if len(s) > 256 {
		return s[:256] + "…"
	}
	return s
}
