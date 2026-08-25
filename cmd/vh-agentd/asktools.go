// asktools.go — the P3.5 daemon-side ask source: --ask-tools routes
// operator-named tools through the approval waterfall. Before this
// flag the shipped daemon emitted NO asks (the docker battery's
// re-scope note: "no ask-observer exists outside tests"), so the
// client's policy engine and y/N responder were never consulted over
// the real binaries.
//
// The insertion is the idiomatic observer seam: a PreExecuteObserver
// returning Verdict ask for matching tool calls. The ask therefore
// rides the REAL waterfall — no shortcut: the ProtocolApprover bridge
// (internal/protocol/approval.go, injected by protocol.NewServer)
// emits approval/request on the wire, the client's interactive/--json
// approver or --policy engine answers, and every unanswerable
// direction (absence, timeout, disconnect) denies fail-closed. Guards
// still run AFTER the waterfall, so an approved call remains subject
// to the deny-only guard layer (monotonic ordering unchanged).
package main

import (
	"fmt"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// parseAskTools splits the --ask-tools flag value: comma-separated
// tool names, trimmed, deduped (first occurrence, order-preserving).
// Empty value → nil (no observer, behavior unchanged). Malformed
// entries (an empty or whitespace-only name — "," trailing garbage)
// are usage errors: a silently-dropped entry would silently narrow
// what the operator asked to route.
func parseAskTools(v string) ([]string, error) {
	if strings.TrimSpace(v) == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, entry := range strings.Split(v, ",") {
		name := strings.TrimSpace(entry)
		if name == "" {
			return nil, fmt.Errorf("invalid --ask-tools %q: contains an empty tool name (comma-separated registered tool names)", v)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, nil
}

// validateAskTools checks every name against the REGISTERED tool set
// (the daemon's own catalog). An unknown name is an error naming it —
// fail-closed at startup: a typo must refuse to serve, never silently
// route nothing.
func validateAskTools(names []string, defs []tools.ToolDefinition) error {
	if len(names) == 0 {
		return nil
	}
	registered := make(map[string]bool, len(defs))
	for _, d := range defs {
		registered[d.Name] = true
	}
	for _, name := range names {
		if !registered[name] {
			return fmt.Errorf("invalid --ask-tools entry %q: not a registered tool (registered: %s)",
				name, strings.Join(toolNames(defs), ", "))
		}
	}
	return nil
}

// toolNames renders the registered catalog for error messages.
func toolNames(defs []tools.ToolDefinition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Name)
	}
	return out
}

// askToolsObserver is the PreExecuteObserver behind --ask-tools: it
// ASKs for calls to the operator-named tools and returns the lattice's
// no-objection Allow verdict for everything else.
//
// Verdict choice (closed lattice; there is no abstain): Allow for
// non-matching tools — in the waterfall an Allow would resolve an
// UPSTREAM ask, but this observer is currently the daemon's only
// pre-observer (nothing is upstream), and the composition-order note
// in compose.go governs future insertions: an upstream-asking
// observer must be added BEFORE this one, or this delegate shape must
// be revisited. Matching tools return Ask — the unresolved ask goes
// to the Approver (the wire bridge) exactly as the fail-closed
// contract requires.
type askToolsObserver struct {
	names map[string]bool
}

// newAskToolsObserver builds the observer over the validated names.
func newAskToolsObserver(names []string) *askToolsObserver {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return &askToolsObserver{names: set}
}

// Name identifies the observer in denial provenance (DeniedBy).
func (o *askToolsObserver) Name() string { return "ask-tools" }

// ObservePreExecute returns the ask verdict for routed tools.
func (o *askToolsObserver) ObservePreExecute(call session.ToolCall) tools.Verdict {
	if o.names[call.Name] {
		return tools.Ask(fmt.Sprintf("tool %s is ask-routed by --ask-tools (operator-configured approval gate)", call.Name))
	}
	return tools.Allow()
}
