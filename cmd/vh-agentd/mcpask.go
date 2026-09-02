// mcpask.go — the P8.2 daemon-side ask source: the mcp_ namespace is
// ASK-BY-DEFAULT.
//
// WHY (the operator's live finding, proven before the fix): with no
// --ask-tools and no --policy, a model's call to an mcp_* tool
// EXECUTED — a real network round-trip with zero approval. run_shell
// is Landlock-sandboxed under confinement (network denied); MCP tools
// are NOT sandboxed — they are raw external network egress with the
// same allow-by-default as run_shell and none of the containment.
// Un-sandboxed external tools must not silently auto-execute. The fix
// is one PreExecuteObserver returning ask for every mcp_-prefixed
// call: the unresolved ask rides the REAL waterfall to the approval
// bridge (approval/request on the wire), where the client's --policy
// engine, interactive responder, or nothing (fail-closed deny) answers.
// Hard-deny classes still apply client-side at the ask, exactly as for
// --ask-tools routing.
//
// COMPOSITION — the a-F4 defer, discharged by FOLD, not by order
// (divergence from the literal plan, deliberately): runWaterfall
// resolves an upstream ask on a DOWNSTREAM Allow
// ("case VerdictAllow: askBy, askReason = "", """ in
// internal/tools/tools.go). Both ask sources here return the lattice's
// no-objection Allow for non-matching calls, so TWO sibling observers
// cannot compose in EITHER order:
//
//   - [ask-tools, mcp-ask]: mcp-ask's Allow for non-mcp names resolves
//     an ask-tools ask — `--ask-tools run_shell` silently stops asking
//     whenever MCP is configured (a security regression in a gate the
//     operator explicitly armed);
//   - [mcp-ask, ask-tools]: ask-tools' blanket-Allow absorbs the mcp
//     ask for tools it does not name — the exact auto-allow defect
//     this slice exists to kill.
//
// The closed verdict lattice has no abstain, so the only composable
// shape is ONE registration carrying every ask source: armAskObservers
// registers the bare observer when a single source is armed
// (byte-identical single-source behavior, DeniedBy unchanged) and a
// FOLD ("ask-tools+mcp-ask") when both are — inside the fold, a
// source's Allow never resolves a sibling's ask. There is no insertion
// order left to get wrong; the remaining pin is the order-inspection
// test (TestAskObserverChainPinned) asserting the registered chain.
package main

import (
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/mcp"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// mcpAskObserver is the PreExecuteObserver behind the ask-by-default
// posture: it ASKs for calls whose name carries the MCP namespace
// prefix (internal/mcp.ToolNamePrefix — the naming constructor's own
// constant, never a duplicated magic string) and returns the lattice's
// no-objection Allow for everything else. The degraded-server sentinel
// (bare mcp_<server>) carries the prefix too and therefore also asks —
// the namespace posture is uniform even though the sentinel performs
// no I/O; simpler to reason about than a carve-out, and an approved
// sentinel call merely returns the typed degraded error.
type mcpAskObserver struct{}

// newMCPAskObserver builds the namespace observer.
func newMCPAskObserver() *mcpAskObserver { return &mcpAskObserver{} }

// Name identifies the observer in denial provenance (DeniedBy).
func (o *mcpAskObserver) Name() string { return "mcp-ask" }

// mcpAskReason is the ask justification surfaced on the wire and in
// denial provenance.
func mcpAskReason(call session.ToolCall) string {
	return "tool " + call.Name + " is an MCP server tool (mcp namespace is ask-by-default: un-sandboxed external egress — allow it via the client approval / --policy exact tool name, or start the daemon with --mcp-auto-allow to opt back into auto-allow)"
}

// ObservePreExecute returns the ask verdict for MCP-namespaced calls.
func (o *mcpAskObserver) ObservePreExecute(call session.ToolCall) tools.Verdict {
	if strings.HasPrefix(call.Name, mcp.ToolNamePrefix) {
		return tools.Ask(mcpAskReason(call))
	}
	return tools.Allow()
}

// askingFold composes multiple ask sources into ONE PreExecuteObserver
// so no sibling's Allow can resolve another's ask (see the file-header
// composition note). Sources evaluate in order; deny (none exist today
// — sources are ask-only) would pass through immediately; the LAST ask
// wins the reason (runWaterfall parity); a source's Allow is a no-op
// inside the fold. With no ask from any source the fold allows.
type askingFold struct {
	name string
	srcs []tools.PreExecuteObserver
}

// newAskingFold folds srcs; the fold's Name joins the source names so
// denial provenance stays honest about WHICH gate chain denied.
func newAskingFold(srcs ...tools.PreExecuteObserver) *askingFold {
	names := make([]string, 0, len(srcs))
	for _, s := range srcs {
		names = append(names, s.Name())
	}
	return &askingFold{name: strings.Join(names, "+"), srcs: srcs}
}

// Name identifies the folded chain in denial provenance.
func (f *askingFold) Name() string { return f.name }

// ObservePreExecute folds the sources' verdicts into one.
func (f *askingFold) ObservePreExecute(call session.ToolCall) tools.Verdict {
	askReason := ""
	for _, src := range f.srcs {
		v := src.ObservePreExecute(call)
		switch v.Kind {
		case tools.VerdictDeny:
			return v
		case tools.VerdictAsk:
			askReason = v.Reason // last ask wins (waterfall parity)
		default:
			// A source's Allow never resolves a sibling's ask inside
			// the fold — that is the fold's entire reason to exist.
		}
	}
	if askReason == "" {
		return tools.Allow()
	}
	return tools.Ask(askReason)
}

// armAskObservers is the daemon's ONE pre-execute-observer registration
// site — called from buildServer right after tool registration, so the
// ask posture is a property of the COMPOSITION (every buildServer
// caller gets it), never of the entrypoint's flag path:
//
//   - --ask-tools set (validated against the registered catalog by
//     run() before serving): the ask-tools observer, unchanged;
//   - MCP tools registered and NOT --mcp-auto-allow: the mcp-ask
//     observer (the P8.2 default posture);
//   - both: the fold "ask-tools+mcp-ask" — one registration, no
//     order-sensitive pair;
//   - --mcp-auto-allow (operator opt-in, default FALSE): the mcp-ask
//     observer is NOT registered — the pre-P8.2 allow-by-default
//     posture, now an explicit choice logged at startup.
func armAskObservers(p *protocol.FileEngine, cfg *Config) {
	var srcs []tools.PreExecuteObserver
	if len(cfg.AskTools) > 0 {
		srcs = append(srcs, newAskToolsObserver(cfg.AskTools))
	}
	if cfg.MCP != nil && !cfg.MCPAutoAllow {
		srcs = append(srcs, newMCPAskObserver())
	}
	switch len(srcs) {
	case 0:
		// No ask source armed: no observer, pre-P3.5 behavior.
	case 1:
		p.Pipeline().AddPreObserver(srcs[0])
	default:
		p.Pipeline().AddPreObserver(newAskingFold(srcs...))
	}
}
