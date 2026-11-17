// Package subagenttools implements the MODEL-FACING subagent tool
// family: subagent_spawn and subagent_send. Until this package the only
// spawn surface was the PROTOCOL family (subagent/spawn|send|list,
// client-driven); this family gives any session's MODEL the same
// capability — recursively, so a child can spawn grandchildren and a
// grandchild great-grandchildren, up to the existing delegation-depth
// fence.
//
// Placement (import-cycle note): this is a leaf package under
// internal/tools importing BOTH internal/tools and internal/subagents —
// legal because neither imports the other (internal/subagents must
// never import internal/tools: the manager is a policy-free lifecycle
// component).
//
// Session binding: the definitions are registered on the daemon's ONE
// shared pipeline, so a tool body cannot close over a session. Instead
// RunTurn binds the executing session id into the context
// (tools.WithExecutingSession) and the body resolves the session's
// Manager through the injected *subagents.Registry — the engine
// registers root-session managers, the daemon's child-turn executor
// registers per-turn managers for child sessions (see
// cmd/vh-agentd/subagents.go).
//
// Capability shape (dsh discipline — capability absence over refusal):
// the family is ADVERTISED to a session's model only while that
// session's depth < maxDepth (SpecsForDepth strips it at the fence); a
// depth-maxed model is simply not offered the tool. The family stays
// registered on the shared pipeline, so a hallucinated call at any
// depth still gets a typed isError refusal — the manager's own depth
// fence, never a panic, never silent, zero durable effects.
//
// One-shot semantics: subagent_spawn with mode "oneshot" BLOCKS until
// the child settles and returns the child's report as the tool result
// (the report has already landed in the executing session's log as a
// user-role subagent/report; the tool result carries the same content —
// provenance stays clean, the transcript never re-credits it as
// assistant words). Continuable spawns return the child id immediately;
// subagent_send delivers follow-ups (one queued child turn per send);
// settlement reports arrive as events on the executing session, the
// same shape the protocol surface produces.
package subagenttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// Tool names (the model-facing vocabulary; the protocol family keeps
// its own subagent/spawn|send|list wire names).
const (
	SpawnName = "subagent_spawn"
	SendName  = "subagent_send"
)

// spawnArgs is the typed argument surface of subagent_spawn.
type spawnArgs struct {
	Prompt string `json:"prompt"`
	Mode   string `json:"mode"`
	Role   string `json:"role,omitempty"`
}

// sendArgs is the typed argument surface of subagent_send.
type sendArgs struct {
	ChildID string `json:"childId"`
	Message string `json:"message"`
}

// spawnResult is the one-shot tool result envelope: terminal state plus
// the child's report content.
type spawnResult struct {
	ChildID string `json:"childId"`
	Result  string `json:"result"`
	Reason  string `json:"reason,omitempty"`
	Report  string `json:"report"`
}

// decodeStrict decodes args with unknown-field rejection (the daemon
// tool convention).
func decodeArgs(args json.RawMessage, v any) error {
	if len(args) == 0 {
		return errors.New("args are required")
	}
	dec := json.NewDecoder(strings.NewReader(string(args)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// resolve returns the manager bound to the context's executing session.
func resolve(ctx context.Context, reg *subagents.Registry) (*subagents.Manager, error) {
	sessionID := tools.ExecutingSessionFrom(ctx)
	if sessionID == "" {
		return nil, errors.New("no executing session in tool context (subagent tools run inside turns only)")
	}
	m, ok := reg.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("no subagent manager bound to session %q (subagents not armed for this session)", sessionID)
	}
	return m, nil
}

// Definitions returns the model-facing subagent tool family bound to
// reg. Both tools are concurrency barriers (they mutate the manager's
// spawn queue and one-shot spawn blocks for a child turn's lifetime).
// subagent_spawn carries no dispatch timeout on purpose: a one-shot
// wait is bounded by the child's turn, which has no natural deadline
// (cancellation propagation is the subagents slice's documented
// non-goal); the body still honors context cancellation.
func Definitions(reg *subagents.Registry) []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Name: SpawnName,
			Description: "Delegates a task to a fresh child agent session. mode \"oneshot\" (default) runs the child to completion and returns " +
				"its report as this tool's result; mode \"continuable\" starts a persistent child and returns its childId immediately — send it follow-ups " +
				"with subagent_send. Children can themselves spawn children, up to the delegation depth fence.",
			Parameters: json.RawMessage(`{"type":"object","properties":{` +
				`"prompt":{"type":"string","description":"the task for the child session (required, non-empty)"},` +
				`"mode":{"type":"string","enum":["oneshot","continuable"],"description":"oneshot (default) blocks until the child settles and returns its report; continuable returns a childId for follow-ups"},` +
				`"role":{"type":"string","description":"optional role/persona hint carried on the child header"}}` +
				`,"required":["prompt"],"additionalProperties":false}`),
			IsConcurrencySafe: false,
			TimeoutMs:         0,
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var a spawnArgs
				if err := decodeArgs(args, &a); err != nil {
					return "", fmt.Errorf("%s: invalid args: %w", SpawnName, err)
				}
				if a.Prompt == "" {
					return "", fmt.Errorf("%s: prompt is required and must be non-empty", SpawnName)
				}
				kind := session.SubagentKindOneShot
				switch a.Mode {
				case "", "oneshot":
				case "continuable":
					kind = session.SubagentKindContinuable
				default:
					return "", fmt.Errorf("%s: mode must be \"oneshot\" or \"continuable\" (got %q)", SpawnName, a.Mode)
				}
				m, err := resolve(ctx, reg)
				if err != nil {
					return "", fmt.Errorf("%s: %w", SpawnName, err)
				}
				receipt, err := m.SpawnWithOptions(subagents.SpawnOptions{
					Kind: kind, Prompt: a.Prompt, Role: a.Role,
				})
				if err != nil {
					return "", err // the manager's typed refusal (depth fence et al.)
				}
				if kind == session.SubagentKindContinuable {
					return marshalResult(spawnResult{ChildID: receipt.ChildID})
				}
				// One-shot: block until the child settles; the child's
				// report is the tool result (it also already landed as a
				// user-role subagent/report in this session's log).
				awaited, err := m.AwaitChild(ctx, receipt.ChildID)
				if err != nil {
					return "", fmt.Errorf("%s: awaiting child %s: %w", SpawnName, receipt.ChildID, err)
				}
				return marshalResult(spawnResult{
					ChildID: awaited.ChildID,
					Result:  awaited.Result,
					Reason:  awaited.Reason,
					Report:  awaited.Report,
				})
			},
		},
		{
			Name: SendName,
			Description: "Delivers one follow-up message to a continuable child session started by subagent_spawn; the child runs one turn per " +
				"send and its replies arrive as subagent reports. Continuable, not-yet-settled children only.",
			Parameters: json.RawMessage(`{"type":"object","properties":{` +
				`"childId":{"type":"string","description":"the childId returned by subagent_spawn (mode continuable)"},` +
				`"message":{"type":"string","description":"the follow-up message (required, non-empty)"}}` +
				`,"required":["childId","message"],"additionalProperties":false}`),
			IsConcurrencySafe: false,
			TimeoutMs:         5000,
			Execute: func(ctx context.Context, args json.RawMessage) (string, error) {
				var a sendArgs
				if err := decodeArgs(args, &a); err != nil {
					return "", fmt.Errorf("%s: invalid args: %w", SendName, err)
				}
				if a.ChildID == "" {
					return "", fmt.Errorf("%s: childId is required and must be non-empty", SendName)
				}
				if a.Message == "" {
					return "", fmt.Errorf("%s: message is required and must be non-empty", SendName)
				}
				m, err := resolve(ctx, reg)
				if err != nil {
					return "", fmt.Errorf("%s: %w", SendName, err)
				}
				if err := m.SendMessage(a.ChildID, a.Message); err != nil {
					return "", err
				}
				return `{"queued":true}`, nil
			},
		},
	}
}

// marshalResult is the compact JSON convention for tool results.
func marshalResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("subagenttools: marshal result: %w", err)
	}
	return string(b), nil
}

// SpecsForDepth implements the capability-absence fence at the
// ADVERTISING layer: it returns specs with the subagent family stripped
// when a session at depth can no longer spawn (depth >= maxDepth, the
// same line the manager's spawn fence enforces). Below the fence the
// list is returned verbatim, order preserved.
func SpecsForDepth(specs []adapters.ToolSpec, depth, maxDepth int) []adapters.ToolSpec {
	if depth < maxDepth {
		return specs
	}
	out := make([]adapters.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if s.Name == SpawnName || s.Name == SendName {
			continue
		}
		out = append(out, s)
	}
	return out
}
