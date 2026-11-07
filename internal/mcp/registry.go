// registry.go — the Registry: the servers → discovered-tools state and
// the namespaced guarded tool definitions the daemon registers in the
// engine's pipeline.
//
// GUARDED-BY-CONSTRUCTION: the registry's ONLY export surface to the
// engine is []tools.ToolDefinition. Registering those defs puts every
// MCP tool behind the SAME waterfall, guards, approval bridge, and P3
// policy classes as run_shell — MCP is EXTERNAL CANDIDATE INPUT, never
// a trusted execution path, and no MCP result is authority.
//
// DEGRADED POSTURE (disclosed choice): a server that fails at startup
// (won't start, initialize timeout, garbage) contributes NO tools, and
// its namespace is RESERVED as ONE sentinel tool named mcp_<server>
// whose description states the degradation and whose call returns the
// typed "server degraded" error. This keeps the advertised surface
// stable and gives a model that hallucinates the server's tool prefix
// an actionable, typed answer instead of a bare unknown-tool — while a
// hallucinated full tool name still honestly reports unknown (the tool
// list was never known). Relaunch of a dead server is a daemon
// restart (documented non-goal: mid-life supervision).
//
// NAMING: mcp_<server>_<tool>, both segments sanitized to [a-z0-9_]
// (lowercase, collapse runs); collisions get a deterministic _N suffix
// (servers iterate in sorted order, tools in server order).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/skills"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// DefaultCallTimeoutMs is the per-call MCP exchange bound when the
// daemon flag is unset (60s: a turn can be slow, never hung).
const DefaultCallTimeoutMs = 60000

// ToolNamePrefix is the namespace marker every MCP-sourced tool name
// carries: mcp_<server>_<tool> for discovered tools and the bare
// mcp_<server> for a degraded server's sentinel. The prefix IS the
// naming scheme — no registry of "which tools are MCP" exists or is
// needed beyond it. The daemon's ask-by-default observer
// (cmd/vh-agentd/mcpask.go) keys on exactly this constant, so the
// observer and the naming constructor can never drift apart.
const ToolNamePrefix = "mcp_"

// Options configures one Registry.
type Options struct {
	// CallTimeoutMs bounds EVERY client exchange (handshake included);
	// <= 0 ⇒ DefaultCallTimeoutMs. The same value lands on each
	// ToolDefinition.TimeoutMs — the pipeline's dispatch bound.
	CallTimeoutMs int
	// Logf receives the honest startup/diagnostic lines (stderr-bound
	// at the daemon). It must NEVER receive a URL, header value, or env
	// value — every line here is structurally credential-free.
	Logf func(format string, args ...any)
}

func (o Options) timeout() time.Duration {
	ms := o.CallTimeoutMs
	if ms <= 0 {
		ms = DefaultCallTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

func (o Options) timeoutMs() int {
	ms := o.CallTimeoutMs
	if ms <= 0 {
		ms = DefaultCallTimeoutMs
	}
	return ms
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

// serverState is one server's registry entry.
type serverState struct {
	cfg      *ServerConfig
	red      redactor
	pid      int    // stdio child pid (0 for remote/degraded) — test seam
	degraded string // "" when healthy; otherwise the redacted reason

	mu      sync.Mutex
	client  Client
	toolDef []tools.ToolDefinition // namespaced defs (healthy only)
}

// Registry owns every connected MCP server for one daemon lifetime.
type Registry struct {
	opts    Options
	mu      sync.RWMutex
	servers map[string]*serverState
	names   []string // sorted
}

// Connect launches and initializes every configured server. It NEVER
// fails whole: a server-level failure degrades that server (typed
// reason logged + surfaced at call time); the rest connect. Tool
// mapping happens here (unmappable schemas skip the tool with a
// warning — the server stays up).
func Connect(cfg *Config, opts Options) *Registry {
	r := &Registry{opts: opts, servers: map[string]*serverState{}, names: append([]string(nil), cfg.Names...)}
	sort.Strings(r.names)
	// Pass 1 — materialize EVERY state first: tool mapping consults the
	// whole registry (collision-safe naming across servers), so no
	// lookup may race a not-yet-visited name.
	for _, name := range r.names {
		sc := cfg.Servers[name]
		r.servers[name] = &serverState{cfg: sc, red: newRedactor(sc)}
	}
	// Pass 2 — connect each server; a failure degrades only it.
	for _, name := range r.names {
		st := r.servers[name]
		sc := st.cfg
		if note := unknownKeysNote(sc); note != "" {
			opts.logf("mcp: %s: %s", name, note)
		}
		client, toolsList, err := r.connectOne(name, sc, st)
		if err != nil {
			st.degraded = err.Error() // already redacted at construction
			opts.logf("mcp: %s DEGRADED: %s (no tools from this server; the mcp_%s name carries this error — relaunch is a daemon restart)", name, st.degraded, sanitizeName(name))
			continue
		}
		st.client = client
		st.toolDef = r.mapTools(name, sc, toolsList)
		kind := sc.Type
		if kind == TransportRemote {
			kind = "remote"
		} else {
			kind = "stdio"
		}
		opts.logf("mcp: %s (%s) up — %d tool(s)", name, kind, len(st.toolDef))
	}
	// Summary line: counts only (credential-free by construction).
	degraded := len(r.Degraded())
	total := r.ToolCount()
	opts.logf("mcp: %d server(s) connected, %d degraded; %d tool(s) registered as mcp_<server>_<tool> (per-call timeout %dms)", len(r.names)-degraded, degraded, total, opts.timeoutMs())
	return r
}

// connectOne performs launch + handshake + tools/list for one server,
// bounded by the call timeout. The returned error is REDACTED (typed,
// credential-free).
func (r *Registry) connectOne(name string, sc *ServerConfig, st *serverState) (Client, []Tool, error) {
	kind := sc.Type
	var client Client
	if sc.Type == TransportLocal {
		c, err := DialStdio(sc, st.red, r.opts.Logf)
		if err != nil {
			return nil, nil, err // already redacted
		}
		client = c
		st.pid = c.cmd.Process.Pid
	} else {
		kind = "remote"
		client = NewHTTPClient(sc, st.red, r.opts.Logf)
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.opts.timeout())
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("%s", st.red.Clean(err.Error()))
	}
	toolsList, err := client.ListTools(ctx)
	if err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("%s", st.red.Clean(fmt.Errorf("%s: tools/list: %w", kind, err).Error()))
	}
	return client, toolsList, nil
}

// mapTools namespaces one healthy server's tools into guarded defs.
// Unmappable schemas skip THAT tool with a warning (fail-closed
// per-tool, never per-server). The shared taken map makes collisions
// deterministic across the sorted server order.
func (r *Registry) mapTools(name string, sc *ServerConfig, list []Tool) []tools.ToolDefinition {
	taken := r.takenNames()
	var defs []tools.ToolDefinition
	for _, tl := range list {
		params, err := MapInputSchema(tl.InputSchema)
		if err != nil {
			r.opts.logf("mcp: %s: skipping tool %q: %v (server stays up; the tool is not advertised)", name, tl.Name, err)
			continue
		}
		defName := namespacedName(name, tl.Name, taken)
		orig := tl.Name
		srv := name
		client := r.servers[name].client
		timeout := r.opts.timeout()
		defs = append(defs, tools.ToolDefinition{
			Name:        defName,
			Description: "(" + srv + ") " + skills.SanitizeDescription(tl.Description),
			Parameters:  params,
			// Conservative: an MCP tool is arbitrary server-side work —
			// classify concurrency-UNSAFE so the scheduler drains the
			// parallel pool around every MCP call (exclusive barrier).
			IsConcurrencySafe: false,
			TimeoutMs:         r.opts.timeoutMs(),
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				ctx2, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				res, err := client.CallTool(ctx2, orig, args)
				if err != nil {
					return "", err // typed + redacted at the transport
				}
				if res.IsError {
					return "", fmt.Errorf("mcp tool %s reported an error: %s", defName, JoinContent(res))
				}
				return JoinContent(res), nil
			},
		})
	}
	return defs
}

// takenNames snapshots every registered def name (collision-safe
// suffixing across servers and sentinels).
func (r *Registry) takenNames() map[string]bool {
	taken := map[string]bool{}
	for _, name := range r.names {
		for _, d := range r.servers[name].toolDef {
			taken[d.Name] = true
		}
	}
	return taken
}

// ToolDefinitions returns every namespaced guarded definition: healthy
// servers' tools plus one degraded sentinel per degraded server.
func (r *Registry) ToolDefinitions() []tools.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	taken := map[string]bool{}
	for _, name := range r.names {
		for _, d := range r.servers[name].toolDef {
			taken[d.Name] = true
		}
	}
	var defs []tools.ToolDefinition
	for _, name := range r.names {
		st := r.servers[name]
		if st.degraded == "" {
			defs = append(defs, st.toolDef...)
			continue
		}
		defs = append(defs, r.degradedSentinel(name, st, taken))
	}
	return defs
}

// degradedSentinel is the reserved-namespace marker for a degraded
// server: description carries the redacted reason; the call is the
// typed degraded error (bounded, fail-closed — never a hang).
func (r *Registry) degradedSentinel(name string, st *serverState, taken map[string]bool) tools.ToolDefinition {
	sentinel := namespacedName(name, "", taken)
	reason := st.degraded
	return tools.ToolDefinition{
		Name:        sentinel,
		Description: "(" + name + ") MCP server degraded: " + skills.SanitizeDescription(reason) + " — no tools from this server are available (relaunch is a daemon restart)",
		Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		// The sentinel performs no I/O; it only reports the typed state.
		IsConcurrencySafe: true,
		TimeoutMs:         5000,
		Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "", fmt.Errorf("mcp: server %s is degraded: %s (this call performed no MCP exchange; relaunch requires a daemon restart)", name, reason)
		},
	}
}

// Degraded lists the degraded server names (sorted).
func (r *Registry) Degraded() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []string
	for _, name := range r.names {
		if r.servers[name].degraded != "" {
			out = append(out, name)
		}
	}
	return out
}

// ToolCount totals the healthy servers' discovered (mapped) tools.
func (r *Registry) ToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, name := range r.names {
		if r.servers[name].degraded == "" {
			n += len(r.servers[name].toolDef)
		}
	}
	return n
}

// Refresh re-lists tools from every HEALTHY server and rebuilds the
// namespaced definitions (degraded servers keep their sentinel — v1
// performs no reconnect; relaunch is a daemon restart). DOCUMENTED
// SEAM: the daemon does not wire this yet (tool lists refresh at
// startup only); it exists for later wiring and is exercised here by
// tests.
func (r *Registry) Refresh(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for _, name := range r.names {
		st := r.servers[name]
		if st.degraded != "" || st.client == nil {
			continue
		}
		list, err := st.client.ListTools(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("mcp: refresh %s: %w", name, err)
			}
			continue
		}
		st.toolDef = r.mapTools(name, st.cfg, list)
	}
	return firstErr
}

// Close stops every connected transport (subprocess teardown ladder /
// HTTP no-op). Idempotent.
func (r *Registry) Close() error {
	r.mu.RLock()
	states := make([]*serverState, 0, len(r.servers))
	for _, name := range r.names {
		states = append(states, r.servers[name])
	}
	r.mu.RUnlock()
	for _, st := range states {
		st.mu.Lock()
		if st.client != nil {
			_ = st.client.Close()
			st.client = nil
		}
		st.mu.Unlock()
	}
	return nil
}

// serverPID exposes the stdio child's pid for lifecycle tests (0 for
// remote servers and degraded entries).
func (r *Registry) serverPID(name string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if st, ok := r.servers[name]; ok {
		return st.pid
	}
	return 0
}

// namespacedName builds mcp_<server>_<tool> with sanitization and a
// deterministic collision suffix (_2, _3, ...). An empty tool segment
// yields the bare mcp_<server> namespace (the degraded sentinel).
func namespacedName(server, tool string, taken map[string]bool) string {
	base := ToolNamePrefix + sanitizeName(server)
	if tool != "" {
		base += "_" + sanitizeName(tool)
	}
	name := base
	for n := 2; taken[name]; n++ {
		name = fmt.Sprintf("%s_%d", base, n)
	}
	taken[name] = true
	return name
}

// unknownKeysNote renders the ignored-config-keys note ("" when none).
func unknownKeysNote(sc *ServerConfig) string {
	if len(sc.UnknownKeys) == 0 {
		return ""
	}
	return "ignoring unknown config keys: " + strings.Join(sc.UnknownKeys, ", ")
}
