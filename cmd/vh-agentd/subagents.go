// subagents.go — the daemon's REAL subagents.Executor (slice B2): a
// spawned child runs genuine engine turns, not a scripted fake. Per the
// manager's Executor contract, one Run call = one child turn over the
// child's own durable log:
//
//   - SYSTEM PROMPT: the daemon's served prompt verbatim — the same
//     resolveSystemPrompt result TurnOpts.System carries (compiled
//     artifact when present, raw assembly otherwise). The role hint
//     rides the child header (role-specific prompt variants are
//     deferred; documented).
//   - TOOLS: the SAME lazily-built engine Pipeline the daemon's
//     session/prompt turns use (echo, clock, run_shell — registered in
//     buildServer AFTER the approval bridge is attached, so child tool
//     calls flow the identical guards → approval → dispatch waterfall;
//     an approval a child triggers surfaces on the wire like any other).
//   - TURN PATH: Pipeline.RunTurn with the daemon's TurnOptions (retry
//     ladder armed) and InboxDriven=true — the turn answers the
//     messages already in the child's inbox (the initial prompt and
//     follow-up subagent/message records) instead of appending a new
//     session/prompt.
//
// Concurrency: Run executes on the parent session manager's dispatch
// goroutine (one child turn at a time per session), never on a protocol
// handler goroutine — see internal/protocol/subagents.go for the lock
// ordering note (manager.mu → parent-log write lock, one direction
// only; child turns hold no manager lock at all).
package main

import (
	"context"
	"path/filepath"

	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
)

// subagentsDirName is the daemon's child-log root under the session
// dir: <session-dir>/subagents/<parentSessionID>/<childID>.jsonl. The
// session dir is the daemon's single explicit durable root, so child
// logs live beside their parent logs.
const subagentsDirName = "subagents"

// subagentTurnExecutor is the real Executor: one Run = one engine turn
// over the child log through the daemon's shared pipeline and adapter.
type subagentTurnExecutor struct {
	engine *protocol.FileEngine
}

// Run drives one child turn. The returned error is the manager's
// settlement signal: nil ⇒ a one-shot child settles completed; an error
// settles failed with this text (continuable children never auto-settle
// — settlement stays manager-owned).
func (x subagentTurnExecutor) Run(ctx context.Context, child subagents.Child) error {
	opts := x.engine.TurnOptions()
	opts.InboxDriven = true
	_, err := x.engine.TurnRunner().RunTurn(ctx, child.Log, x.engine.Adapter(), opts, "")
	return err
}

// wireSubagents arms the FileEngine's subagent surface: the real turn
// executor (self-referential — it reads the engine's lazily-built
// pipeline, so the approval-bridge-then-tools composition order in
// buildServer is preserved for child turns too) and the child-log store
// under <session-dir>/subagents.
func wireSubagents(engine *protocol.FileEngine, sessionDir string) {
	engine.SubagentExecutor = subagentTurnExecutor{engine: engine}
	engine.SubagentStore = subagents.NewFileStore(filepath.Join(sessionDir, subagentsDirName))
}
