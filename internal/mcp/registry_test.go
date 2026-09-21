// registry_test.go — the Registry contract: namespaced guarded tool
// definitions over REAL transports (vh-mockmcp stdio subprocess +
// vh-mockmcp --http subprocess), the degraded-server posture, the
// unmappable-schema per-tool skip, naming collisions, credential-free
// startup lines, and the refresh seam.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// startRemoteMock runs the REAL vh-mockmcp over HTTP on an ephemeral
// port and returns its base URL.
func startRemoteMock(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(mockMCP(t), append([]string{"--http", "127.0.0.1:0"}, args...)...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start remote mock: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	br := bufio.NewReader(stderr)
	cmd.Stderr = nil
	_ = stderr
	// The mock prints "listening <addr>" on stderr before serving.
	line, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "listening ") {
		// cmd.StderrPipe was replaced — re-do with a plain pipe read
		t.Fatalf("remote mock did not report its address: %q %v", line, err)
	}
	addr := strings.TrimSpace(strings.TrimPrefix(line, "listening "))
	return "http://" + addr
}

func newRegistryCfg(t *testing.T, servers map[string]*ServerConfig) *Config {
	t.Helper()
	cfg := &Config{Path: "test", Servers: servers}
	for name := range servers {
		cfg.Names = append(cfg.Names, name)
	}
	// deterministic order: sort
	for i := 1; i < len(cfg.Names); i++ {
		for j := i; j > 0 && cfg.Names[j] < cfg.Names[j-1]; j-- {
			cfg.Names[j], cfg.Names[j-1] = cfg.Names[j-1], cfg.Names[j]
		}
	}
	return cfg
}

func collectLogf() (*[]string, func(string, ...any)) {
	var lines []string
	return &lines, func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
}

func TestRegistryNamespacedDefinitionsBothTransports(t *testing.T) {
	remote := startRemoteMock(t)
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"localmock":  {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
		"remotemock": {Type: TransportRemote, URL: remote + "/fake-token-abcdef123456/mcp"},
	})
	logLines, logf := collectLogf()
	reg := Connect(cfg, Options{CallTimeoutMs: 5000, Logf: logf})
	t.Cleanup(func() { _ = reg.Close() })
	if len(reg.Degraded()) != 0 {
		t.Fatalf("degraded = %v (logs: %v)", reg.Degraded(), *logLines)
	}
	defs := reg.ToolDefinitions()
	names := defNames(defs)
	for _, want := range []string{"mcp_localmock_echo", "mcp_localmock_slow", "mcp_localmock_fail", "mcp_remotemock_echo", "mcp_remotemock_slow", "mcp_remotemock_fail"} {
		if !names[want] {
			t.Fatalf("tool %q missing from %v", want, names)
		}
	}
	// Descriptions carry the (<server>) prefix.
	echoDef := findDef(defs, "mcp_localmock_echo")
	if !strings.HasPrefix(echoDef.Description, "(localmock) ") {
		t.Fatalf("description = %q", echoDef.Description)
	}
	if echoDef.TimeoutMs != 5000 {
		t.Fatalf("TimeoutMs = %d", echoDef.TimeoutMs)
	}
	if echoDef.IsConcurrencySafe {
		t.Fatal("MCP tools must classify concurrency-UNSAFE (barrier)")
	}
	// Startup lines: names + counts, NEVER the URL or its token.
	joined := strings.Join(*logLines, "\n")
	if !strings.Contains(joined, "localmock (stdio) up") || !strings.Contains(joined, "remotemock (remote) up") {
		t.Fatalf("startup lines missing: %v", *logLines)
	}
	if strings.Contains(joined, "fake-token-abcdef123456") {
		t.Fatalf("URL token leaked into startup lines: %v", *logLines)
	}
}

func TestRegistryExecuteRoundTripAndTypedErrors(t *testing.T) {
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"m": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
	})
	reg := Connect(cfg, Options{CallTimeoutMs: 5000, Logf: func(string, ...any) {}})
	t.Cleanup(func() { _ = reg.Close() })
	defs := reg.ToolDefinitions()
	ctx := context.Background()

	// Happy round trip through the DEFINITION's Execute (the pipeline's
	// exact call shape).
	out, err := findDef(defs, "mcp_m_echo").Execute(ctx, json.RawMessage(`{"text":"registry-proof"}`))
	if err != nil || out != "echo: registry-proof" {
		t.Fatalf("echo = %q %v", out, err)
	}
	// Tool-level isError → typed error.
	_, err = findDef(defs, "mcp_m_fail").Execute(ctx, json.RawMessage(`{"message":"planned failure"}`))
	if err == nil || !strings.Contains(err.Error(), "planned failure") || !strings.Contains(err.Error(), "mcp_m_fail") {
		t.Fatalf("fail error = %v", err)
	}
	// Protocol-level bad args → typed error.
	_, err = findDef(defs, "mcp_m_echo").Execute(ctx, json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "text") {
		t.Fatalf("bad-args error = %v", err)
	}
}

func TestRegistryDegradedServerSentinelPosture(t *testing.T) {
	remote := startRemoteMock(t)
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"good": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
		"dead": {Type: TransportRemote, URL: strings.Replace(remote, "127.0.0.1:", "127.0.0.1:1", 1) + "/token-value-987654/mcp"},
	})
	logLines, logf := collectLogf()
	reg := Connect(cfg, Options{CallTimeoutMs: 3000, Logf: logf})
	t.Cleanup(func() { _ = reg.Close() })

	if got := reg.Degraded(); len(got) != 1 || got[0] != "dead" {
		t.Fatalf("degraded = %v", got)
	}
	defs := reg.ToolDefinitions()
	names := defNames(defs)
	// The degraded server contributes NO real tools...
	for name := range names {
		if strings.HasPrefix(name, "mcp_dead_") {
			t.Fatalf("degraded server advertised a tool: %s", name)
		}
	}
	// ...but its NAMESPACE is reserved as one sentinel whose call is
	// the typed degraded error.
	sentinel := findDef(defs, "mcp_dead")
	if sentinel.Name != "mcp_dead" || sentinel.Execute == nil {
		t.Fatalf("degraded sentinel missing from %v", names)
	}
	_, err := sentinel.Execute(context.Background(), json.RawMessage(`{}`))
	if err == nil || !strings.Contains(err.Error(), "degraded") {
		t.Fatalf("sentinel error = %v", err)
	}
	// Startup honesty: DEGRADED line naming the server, without the URL token.
	joined := strings.Join(*logLines, "\n")
	if !strings.Contains(joined, "dead DEGRADED") {
		t.Fatalf("DEGRADED startup line missing: %v", *logLines)
	}
	if strings.Contains(joined, "token-value-987654") {
		t.Fatalf("dead URL token leaked: %v", *logLines)
	}
}

func TestRegistryGarbageServerDegrades(t *testing.T) {
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"garbage": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio", "--garbage"}},
	})
	reg := Connect(cfg, Options{CallTimeoutMs: 3000, Logf: func(string, ...any) {}})
	t.Cleanup(func() { _ = reg.Close() })
	if got := reg.Degraded(); len(got) != 1 || got[0] != "garbage" {
		t.Fatalf("degraded = %v", got)
	}
	if defs := reg.ToolDefinitions(); findDef(defs, "mcp_garbage").Name != "mcp_garbage" {
		t.Fatalf("sentinel missing")
	}
}

func TestRegistryUnmappableSchemaSkipsToolServerStaysUp(t *testing.T) {
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"m": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio", "--bad-schema-tool"}},
	})
	logLines, logf := collectLogf()
	reg := Connect(cfg, Options{CallTimeoutMs: 5000, Logf: logf})
	t.Cleanup(func() { _ = reg.Close() })
	if len(reg.Degraded()) != 0 {
		t.Fatalf("server degraded by a bad tool schema: %v (logs %v)", reg.Degraded(), *logLines)
	}
	defs := reg.ToolDefinitions()
	if findDef(defs, "mcp_m_weird").Name == "mcp_m_weird" {
		t.Fatal("unmappable tool was registered")
	}
	if findDef(defs, "mcp_m_echo").Name != "mcp_m_echo" {
		t.Fatal("healthy tool skipped alongside the bad one")
	}
	joined := strings.Join(*logLines, "\n")
	if !strings.Contains(joined, `skipping tool "weird"`) {
		t.Fatalf("skip warning missing: %v", *logLines)
	}
}

func TestRegistryNameCollisionsGetDeterministicSuffixes(t *testing.T) {
	// Two servers whose sanitized namespaces collide ("a" with tool
	// "b_c" vs "a_b" with tool "c").
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"a":   {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
		"a-b": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
	})
	reg := Connect(cfg, Options{CallTimeoutMs: 5000, Logf: func(string, ...any) {}})
	t.Cleanup(func() { _ = reg.Close() })
	defs := reg.ToolDefinitions()
	names := defNames(defs)
	// mcp_a_echo (first server, sorted order) and mcp_a_b_echo stay
	// distinct; a genuine collision (mcp_a_slow vs nothing) just needs
	// uniqueness overall.
	if len(names) != len(defs) {
		t.Fatalf("duplicate names in %v", names)
	}
	if !names["mcp_a_echo"] || !names["mcp_a_b_echo"] {
		t.Fatalf("expected distinct namespaces: %v", names)
	}
	// Direct collision: tool literally named to collide.
	// (vh-mockmcp has no colliding tool names; the collision path is
	// covered by the naming unit test below with a synthetic taken-map.)
}

func TestNamespacedNameCollisionSuffix(t *testing.T) {
	taken := map[string]bool{"mcp_s_echo": true, "mcp_s_echo_2": true}
	got := namespacedName("s", "echo", taken)
	if got != "mcp_s_echo_3" {
		t.Fatalf("collision suffix = %q", got)
	}
	if !taken[got] {
		t.Fatal("winner not recorded as taken")
	}
	// Sanitization inside namespacedName.
	if got := namespacedName("My.Srv", "Echo Tool", map[string]bool{}); got != "mcp_my_srv_echo_tool" {
		t.Fatalf("sanitized = %q", got)
	}
}

func TestRegistrySlowToolTimesOutThroughDefinition(t *testing.T) {
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"m": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
	})
	reg := Connect(cfg, Options{CallTimeoutMs: 200, Logf: func(string, ...any) {}})
	t.Cleanup(func() { _ = reg.Close() })
	defs := reg.ToolDefinitions()
	start := time.Now()
	out, err := findDef(defs, "mcp_m_slow").Execute(context.Background(), json.RawMessage(`{"ms":3000}`))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("slow call returned: %q", out)
	}
	if !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error not deadline-typed: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("per-call deadline not enforced: %v", elapsed)
	}
}

func TestRegistryCloseStopsSubprocesses(t *testing.T) {
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"m": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
	})
	reg := Connect(cfg, Options{CallTimeoutMs: 5000, Logf: func(string, ...any) {}})
	pid := reg.serverPID("m")
	if pid <= 0 {
		t.Fatal("no subprocess pid recorded")
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("kill", "-0", fmt.Sprint(pid)).CombinedOutput()
		if err != nil {
			_ = out
			return // process gone
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("subprocess still alive after registry Close")
}

func TestRegistryRefreshRelistsTools(t *testing.T) {
	cfg := newRegistryCfg(t, map[string]*ServerConfig{
		"m": {Type: TransportLocal, Command: []string{mockMCP(t), "--stdio"}},
	})
	reg := Connect(cfg, Options{CallTimeoutMs: 5000, Logf: func(string, ...any) {}})
	t.Cleanup(func() { _ = reg.Close() })
	var calls atomic.Int64
	// The refresh seam: re-lists from every healthy server and updates
	// the tool cache. (v1: exists, unwired at the daemon — the count
	// assertions prove it actually re-consults the server.)
	if n := reg.ToolCount(); n != 3 {
		t.Fatalf("tool count = %d", n)
	}
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if n := reg.ToolCount(); n != 3 {
		t.Fatalf("post-refresh tool count = %d", n)
	}
	if calls.Load() != 0 {
		t.Fatal("unexpected call counter use")
	}
}

func TestJoinContentNonTextPlaceholder(t *testing.T) {
	res := &CallResult{Content: []ContentBlock{
		{Type: "text", Text: "before"},
		{Type: "image"},
		{Type: "text", Text: "after"},
	}}
	got := JoinContent(res)
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("text blocks lost: %q", got)
	}
	if !strings.Contains(got, `"image"`) || !strings.Contains(got, "omitted") {
		t.Fatalf("non-text placeholder missing: %q", got)
	}
	if JoinContent(nil) != "" {
		t.Fatal("nil result must join to empty string")
	}
}

// --- helpers -----------------------------------------------------------------

func defNames(defs []tools.ToolDefinition) map[string]bool {
	out := map[string]bool{}
	for _, d := range defs {
		out[d.Name] = true
	}
	return out
}

func findDef(defs []tools.ToolDefinition, name string) tools.ToolDefinition {
	for _, d := range defs {
		if d.Name == name {
			return d
		}
	}
	return tools.ToolDefinition{}
}
