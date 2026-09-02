// mcp.go — the daemon's P8 MCP-host wiring: --mcp-config / --mcp-timeout-ms
// flag semantics (explicit vs honest-absent default postures, same
// shapes as --skills-dir), the fail-closed timeout validation, and the
// registry construction that launches/initializes every configured
// server at startup.
//
// OPERATOR POSTURE (verbatim invariants this wiring carries):
//
//   - MCP tools are EXTERNAL CANDIDATE INPUT: they join the SAME
//     guarded registry as run_shell — guards, the allow/deny/ask
//     waterfall, the approval bridge, and P3 policy classes all apply
//     BY CONSTRUCTION (they are ordinary ToolDefinitions). No MCP
//     result is trusted authority.
//   - Policy default: MCP tool names match NO allow rule the client
//     ships — under --policy (or interactive) an MCP tool call ASKS
//     and falls to the human responder; operators allow them
//     explicitly by tool name (mcp_<server>_<tool>).
//   - Credentials in server config (url, header values, env values)
//     are redacted exactly like provider keys: startup lines are
//     structurally credential-free (name + type + counts only), and
//     every error surface is scrubbed via adapters.RedactSecret.
//   - A server that will not start, times out, or returns garbage
//     DEGRADES that server (typed error at startup + at call time);
//     the daemon — and every turn — continues. No unbounded waits
//     anywhere: every exchange is deadline-bounded by --mcp-timeout-ms.
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/vhqtvn/vh-agent-harness/internal/mcp"
)

// defaultMCPTimeoutMs bounds one MCP exchange (handshake included) when
// the flag is unset: 60s — a turn can be slow, never hung.
const defaultMCPTimeoutMs = 60000

// maxMCPTimeoutMs caps --mcp-timeout-ms (mirrors run_shell's timeout
// cap): an MCP call may be slow, but no single exchange may claim more
// than 10 minutes of a turn.
const maxMCPTimeoutMs = 600000

// validateMCPTimeout checks the flag fail-closed (the same exit-2
// posture as every sibling flag): negative or above the cap is a usage
// error; 0 means "take the default".
func validateMCPTimeout(ms int) (int, error) {
	switch {
	case ms < 0:
		return 0, fmt.Errorf("invalid --mcp-timeout-ms %d: must be >= 0 (0 takes the %dms default)", ms, defaultMCPTimeoutMs)
	case ms == 0:
		return defaultMCPTimeoutMs, nil
	case ms > maxMCPTimeoutMs:
		return 0, fmt.Errorf("invalid --mcp-timeout-ms %d: must be <= %d (the same cap run_shell enforces — no single MCP exchange may exceed it)", ms, maxMCPTimeoutMs)
	default:
		return ms, nil
	}
}

// setupMCP implements the --mcp-config flag semantics:
//
//   - flag UNSET: the opencode default (~/.config/opencode/
//     opencode.json) when it exists — an honest startup line says so;
//     absent default = zero MCP (one startup line, daemon runs
//     normally).
//   - flag SET: the file must exist and parse (full opencode.json with
//     a .mcp block OR a bare server map) — a missing/invalid
//     explicitly-passed file is a fail-closed exit-2 usage error with a
//     file:line-ish locator, never a silently-empty MCP surface.
//   - either way, a usable config CONNECTS every server NOW (launch
//     stdio subprocesses, initialize remotes, list tools): a degraded
//     server logs its typed reason and stays degraded; the registry
//     closes at daemon exit.
//
// Returns (nil, nil) for the zero-MCP postures.
func setupMCP(flagPath string, timeoutMs int, lg *log.Logger) (*mcp.Registry, error) {
	path := flagPath
	explicit := path != ""
	if !explicit {
		path = mcp.DefaultConfigPath()
		if _, err := os.Stat(path); err != nil {
			lg.Printf("mcp: none (no config at %s — zero MCP tools; pass --mcp-config PATH to enable)", path)
			return nil, nil
		}
		lg.Printf("mcp: using default config at %s (--mcp-config unset)", path)
	}
	cfg, err := mcp.LoadConfigFile(path)
	if err != nil {
		if !explicit {
			// A PRESENT default that fails to parse is honest-refuse
			// territory: the operator did not ask for MCP, but the file
			// they keep is broken — say it loudly, run with zero MCP.
			lg.Printf("mcp: default config at %s failed to parse — running with zero MCP tools: %v", path, err)
			return nil, nil
		}
		return nil, err
	}
	if len(cfg.Servers) == 0 {
		lg.Printf("mcp: zero servers configured in %s — zero MCP tools", path)
		return nil, nil
	}
	return mcp.Connect(cfg, mcp.Options{
		CallTimeoutMs: timeoutMs,
		Logf: func(format string, args ...any) {
			lg.Printf(format, args...)
		},
	}), nil
}
