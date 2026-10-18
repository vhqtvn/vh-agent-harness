// mcpask_test.go — unit tests for the P8.2 ask-by-default machinery:
// the observer's verdicts, the fold's no-absorption composition (the
// a-F4 discharge), armAskObservers' registration shapes, and the run()
// startup posture lines.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/mcp"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// TestMCPAskObserverVerdicts: every mcp_-prefixed name asks (including
// the bare degraded sentinel mcp_<server>); every non-mcp name returns
// the no-objection Allow; the prefix comes from the mcp package's own
// naming constant.
func TestMCPAskObserverVerdicts(t *testing.T) {
	o := newMCPAskObserver()
	if o.Name() != "mcp-ask" {
		t.Fatalf("observer name = %q", o.Name())
	}
	for _, name := range []string{
		"mcp_localmock_echo",
		"mcp_vhmcp_web_search",
		"mcp_deadmock", // the bare degraded sentinel carries the prefix too
		"mcp_",
	} {
		v := o.ObservePreExecute(session.ToolCall{ID: "c1", Name: name})
		if v.Kind != tools.VerdictAsk {
			t.Fatalf("ObservePreExecute(%q) = %v, want ask", name, v.Kind)
		}
		if !strings.Contains(v.Reason, "ask-by-default") {
			t.Fatalf("ask reason for %q must state the posture: %q", name, v.Reason)
		}
	}
	for _, name := range []string{"echo", "run_shell", "read", "skill_load", "subagent_spawn", "mcpancake"} {
		if v := o.ObservePreExecute(session.ToolCall{ID: "c1", Name: name}); v.Kind != tools.VerdictAllow {
			t.Fatalf("ObservePreExecute(%q) = %v, want allow", name, v.Kind)
		}
	}
	// The prefix is the naming constructor's own constant — never a
	// duplicated magic string (D2).
	if !strings.HasPrefix("mcp_anything", mcp.ToolNamePrefix) || mcp.ToolNamePrefix != "mcp_" {
		t.Fatalf("ToolNamePrefix = %q", mcp.ToolNamePrefix)
	}
}

// TestAskingFoldNoAbsorption — the a-F4 CRUX at unit level: inside the
// fold, a source's blanket-Allow for non-matching calls NEVER resolves
// a sibling's ask. The literal two-observer alternative
// ([ask-tools, mcp-ask] as plain siblings) would let mcp-ask's Allow
// silently absorb an ask-tools ask for a named non-MCP tool — the
// regression this fold exists to make impossible. The negative proof
// lives here at the WATERFALL level: registering the two as plain
// siblings in the mission's order demonstrably absorbs the ask-tools
// ask; the fold does not.
func TestAskingFoldNoAbsorption(t *testing.T) {
	fold := newAskingFold(
		newAskToolsObserver([]string{"run_shell"}),
		newMCPAskObserver(),
	)
	if fold.Name() != "ask-tools+mcp-ask" {
		t.Fatalf("fold name = %q", fold.Name())
	}
	// Named non-MCP tool: the ask-tools ask must STAND (the mcp
	// source's Allow must not resolve it).
	if v := fold.ObservePreExecute(session.ToolCall{ID: "c1", Name: "run_shell"}); v.Kind != tools.VerdictAsk {
		t.Fatalf("fold(run_shell) = %v, want ask (the sibling Allow must not absorb it)", v.Kind)
	}
	// MCP tool not named in --ask-tools: the mcp ask must STAND.
	if v := fold.ObservePreExecute(session.ToolCall{ID: "c1", Name: "mcp_mock_echo"}); v.Kind != tools.VerdictAsk {
		t.Fatalf("fold(mcp_mock_echo) = %v, want ask", v.Kind)
	}
	// MCP tool ALSO named in --ask-tools: still ask (last ask wins the
	// reason; waterfall parity).
	v := fold.ObservePreExecute(session.ToolCall{ID: "c1", Name: "mcp_run_shell"})
	if v.Kind != tools.VerdictAsk {
		t.Fatalf("fold(named mcp tool) = %v, want ask", v.Kind)
	}
	// Neither source matches: allow.
	if v := fold.ObservePreExecute(session.ToolCall{ID: "c1", Name: "echo"}); v.Kind != tools.VerdictAllow {
		t.Fatalf("fold(echo) = %v, want allow", v.Kind)
	}

	// The NEGATIVE control (why the fold is required): as plain
	// siblings in a REAL pipeline, the two observers demonstrably
	// interfere — [ask-tools, mcp-ask] absorbs the ask-tools ask for
	// run_shell (the downstream Allow resolves the upstream ask, the
	// tool EXECUTES with no approver configured), and the reversed
	// order absorbs the mcp ask the same way. Both interference
	// directions are pinned so a future "simplification" back to
	// sibling registrations fails this test loudly.
	probe := func(p *tools.Pipeline, name string) tools.Result {
		if err := p.Register(tools.ToolDefinition{
			Name: name,
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				return "EXECUTED", nil
			},
		}); err != nil {
			t.Fatalf("register %s: %v", name, err)
		}
		return p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: name})
	}
	// Mission-order siblings: the named NON-MCP tool's ask is absorbed.
	sib := tools.NewPipeline()
	sib.AddPreObserver(newAskToolsObserver([]string{"run_shell"}))
	sib.AddPreObserver(newMCPAskObserver())
	if res := probe(sib, "run_shell"); res.Denied {
		t.Fatalf("sibling order [ask-tools,mcp-ask] must absorb the run_shell ask (the documented regression); got denial %q — the negative control no longer reproduces, re-examine the fold's rationale", res.DenyReason)
	}
	// Reversed siblings: the MCP namespace ask is absorbed.
	sib2 := tools.NewPipeline()
	sib2.AddPreObserver(newMCPAskObserver())
	sib2.AddPreObserver(newAskToolsObserver([]string{"run_shell"}))
	if res := probe(sib2, "mcp_mock_echo"); res.Denied {
		t.Fatalf("sibling order [mcp-ask,ask-tools] must absorb the mcp ask (the documented regression); got denial %q — the negative control no longer reproduces, re-examine the fold's rationale", res.DenyReason)
	}
	// The fold under the same no-approver conditions: every ask DENIES
	// (fail-closed), nothing is silently absorbed.
	for _, name := range []string{"run_shell", "mcp_mock_echo"} {
		if res := probe(foldPipeline(t), name); !res.Denied {
			t.Fatalf("fold must keep the ask standing for %s (got execution)", name)
		}
	}
}

// foldPipeline builds a pipeline carrying ONLY the folded observers
// (no approver: an unresolved ask must deny).
func foldPipeline(t *testing.T) *tools.Pipeline {
	t.Helper()
	p := tools.NewPipeline()
	p.AddPreObserver(newAskingFold(newAskToolsObserver([]string{"run_shell"}), newMCPAskObserver()))
	return p
}

// TestMCPPostureStartupLines — run() states the mcp posture at startup:
// ask-by-default when MCP is registered, auto-allow only under the
// explicit flag, and NOTHING when there is no MCP. Driven over the
// real run() with an immediately-EOF conn (clean exit 0).
func TestMCPPostureStartupLines(t *testing.T) {
	// A minimal valid MCP config (one stdio mock server) so run()
	// actually registers mcp tools. The mock binary must exist.
	mockBin := filepath.Join(mockBinDir(t), "vh-mockmcp")
	dir := t.TempDir()
	cfgJSON := map[string]any{
		"posturemock": map[string]any{"type": "local", "command": []string{mockBin, "--stdio"}},
	}
	raw, err := json.Marshal(cfgJSON)
	if err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(cfgPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		extraFlags  []string
		wantLine    string
		wantNoMcpLn bool
	}{
		{"ask-by-default", []string{"--mcp-config", cfgPath}, "mcp tools: ask-by-default", false},
		{"auto-allow", []string{"--mcp-config", cfgPath, "--mcp-auto-allow"}, "mcp tools: auto-allow (operator opt-in via --mcp-auto-allow", false},
		{"no mcp", nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb safeBuffer
			args := []string{
				"--adapter", "openai", "--model", "m", "--base-url", "http://127.0.0.1:1",
				"--api-key-env", "VH_AGENTD_TEST_KEY", "--session-dir", t.TempDir(),
			}
			args = append(args, tc.extraFlags...)
			code := run(args, func(string) string { return "k" }, eofConn{}, &out, &errb)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (stderr: %s)", code, errb.String())
			}
			if tc.wantNoMcpLn {
				if strings.Contains(errb.String(), "mcp tools:") {
					t.Fatalf("zero-MCP run must not log an mcp-tools posture line:\n%s", errb.String())
				}
				return
			}
			if !strings.Contains(errb.String(), tc.wantLine) {
				t.Fatalf("startup log lacks %q:\n%s", tc.wantLine, errb.String())
			}
		})
	}
}

// TestAskObserverChainPinned — the registration shapes the REAL
// armAskObservers produces, asserted through the pipeline's
// order-inspection seam on a bare FileEngine (Pipeline() builds
// lazily). Single source ⇒ the bare observer (byte-identical
// single-source behavior: DeniedBy "ask-tools"/"mcp-ask" unchanged for
// the P3.5 battery contracts). Both sources ⇒ exactly ONE folded
// registration.
func TestAskObserverChainPinned(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  *Config
		want []string
	}{
		{"none", &Config{}, nil},
		{"ask-tools only", &Config{AskTools: []string{"run_shell"}}, []string{"ask-tools"}},
		{"mcp only", &Config{MCP: &mcp.Registry{}}, []string{"mcp-ask"}},
		{"mcp auto-allow", &Config{MCP: &mcp.Registry{}, MCPAutoAllow: true}, nil},
		{"both folded", &Config{AskTools: []string{"run_shell"}, MCP: &mcp.Registry{}}, []string{"ask-tools+mcp-ask"}},
		{"auto-allow keeps ask-tools", &Config{AskTools: []string{"run_shell"}, MCP: &mcp.Registry{}, MCPAutoAllow: true}, []string{"ask-tools"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eng := &protocol.FileEngine{}
			armAskObservers(eng, tc.cfg)
			got := eng.Pipeline().PreObserverNames()
			if len(got) != len(tc.want) {
				t.Fatalf("chain = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("chain = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
