// subagents.go — the daemon's REAL subagents.Executor (slice B2), now
// RECURSION-CAPABLE (the child-of-child slice): a spawned child runs
// genuine engine turns, not a scripted fake, and any child below the
// delegation-depth fence runs its turns with the same MODEL-FACING
// spawn capability armed — so a child's model can spawn grandchildren
// through the subagent_spawn/subagent_send tools, recursively, until
// the fence. Per the manager's Executor contract, one Run call = one
// child turn over the child's own durable log:
//
//   - SYSTEM PROMPT: the daemon's served prompt verbatim — the same
//     resolveSystemPrompt result TurnOpts.System carries (compiled
//     artifact when present, raw assembly otherwise). The role hint
//     rides the child header (role-specific prompt variants are
//     deferred; documented).
//
//   - TOOLS: the SAME lazily-built engine Pipeline the daemon's
//     session/prompt turns use (echo, clock, run_shell — registered in
//     buildServer AFTER the approval bridge is attached, so child tool
//     calls flow the identical guards → approval → dispatch waterfall;
//     an approval a child triggers surfaces on the wire like any other).
//
//   - TURN PATH: Pipeline.RunTurn with the daemon's TurnOptions (retry
//     ladder armed) and InboxDriven=true — the turn answers the
//     messages already in the child's inbox (the initial prompt and
//     follow-up subagent/message records) instead of appending a new
//     session/prompt.
//
//   - RECURSION (child-of-child): before the child's turn, Run binds a
//     manager for the CHILD's own log (the child acting as parent of
//     grandchildren) into the session registry — the same registry the
//     model-facing subagent tools resolve through — and advertises the
//     subagent tool family to the child's model while the child's depth
//     is below the fence (SpecsForDepth strips it at the fence:
//     capability absence over refusal; a hallucinated call still meets
//     the manager's typed depth-fence error). Grandchild logs compose
//     naturally through the SAME store: <store>/<childID>/<gcID>.jsonl
//     with tree-unique ids (sess-a → sess-a.1 → sess-a.1.2).
//
//     Per-turn manager lifecycle (deliberate simplicity): the child's
//     manager is rebuilt from the child log each turn — state is a fold
//     over durable events, so no cross-turn in-memory state exists —
//     and at turn end Run DRAINS it before stopping: every grandchild
//     turn THIS turn queued reaches its post-run disposition before the
//     manager dies, so a queued follow-up turn can never be silently
//     dropped by a manager rebuild. The cost is one fold + one
//     dispatch goroutine per child turn; the alternative (a cached
//     manager per child) needs a settle hook the executor does not own.
//
// Concurrency: Run executes on the parent session manager's dispatch
// goroutine (one child turn at a time per session), never on a protocol
// handler goroutine — see internal/protocol/subagents.go for the lock
// ordering note (manager.mu → parent-log write lock, one direction
// only; child turns hold no manager lock at all). A grandchild turn
// runs on the CHILD's manager dispatch goroutine and never touches its
// parent's manager lock; the one-shot spawn tool WAITS on a settlement
// channel (no lock held), so nested chains drain bottom-up without
// cycles. Stop does not cancel executing turns (cancellation
// propagation stays a documented non-goal).
package main

import (
	"context"
	"path/filepath"

	"github.com/vhqtvn/vh-agent-harness/internal/jobs"
	"github.com/vhqtvn/vh-agent-harness/internal/protocol"
	"github.com/vhqtvn/vh-agent-harness/internal/subagents"
	"github.com/vhqtvn/vh-agent-harness/internal/tools/subagenttools"
)

// subagentsDirName is the daemon's child-log root under the session
// dir: <session-dir>/subagents/<parentSessionID>/<childID>.jsonl. The
// session dir is the daemon's single explicit durable root, so child
// logs live beside their parent logs — and GRANDCHILD logs nest one
// directory deeper under the same root (the store keys on the OWNING
// parent session id, so sess-a.1's children live under
// subagents/sess-a.1/).
const subagentsDirName = "subagents"

// subagentTurnExecutor is the real Executor: one Run = one engine turn
// over the child log through the daemon's shared pipeline and adapter,
// with the child's own spawn capability armed below the fence.
type subagentTurnExecutor struct {
	engine *protocol.FileEngine
	// reg is the session→manager registry the model-facing subagent
	// tools resolve through (the same instance the engine holds).
	reg *subagents.Registry
	// jobsReg is the session→jobs registry the run_shell background
	// dispatcher resolves through (P6): a child turn binds a jobs
	// manager for the CHILD's own log for the duration of the turn, so
	// a child model's background dispatch lands on the child's log.
	jobsReg *jobsRegistry
}

// Run drives one child turn. The returned error is the manager's
// settlement signal: nil ⇒ a one-shot child settles completed; an error
// settles failed with this text (continuable children never auto-settle
// — settlement stays manager-owned).
func (x subagentTurnExecutor) Run(ctx context.Context, child subagents.Child) error {
	opts := x.engine.TurnOptions()
	opts.InboxDriven = true

	// Recursion wiring: bind a manager for the CHILD's own log (it is
	// the parent-of-grandchildren surface) for the duration of this
	// turn, and advertise the model-facing family while the child sits
	// below the fence. Managers are bound at EVERY depth — at the fence
	// the advertising is stripped but a hallucinated call still meets
	// the manager's typed depth-fence refusal (logged, isError, zero
	// durable effects) rather than an opaque "not armed" error.
	store := x.engine.SubagentStore
	if store == nil {
		store = subagents.NewFileStore(filepath.Join(x.engine.Dir, subagentsDirName))
	}
	maxDepth := subagents.EffectiveMaxDepth(x.engine.SubagentOpts.MaxDelegationDepth)
	childMgr, err := subagents.NewManager(child.Log, x.engine.SubagentExecutor, store, x.engine.SubagentOpts)
	if err != nil {
		// A child log the manager cannot fold is fail-closed: the turn
		// refuses rather than running an unmanaged child.
		return err
	}
	x.reg.Put(child.ID, childMgr)
	// P6 child-turn jobs binding (mirrors childMgr exactly): a jobs
	// manager over the CHILD's own log, so run_shell background calls
	// from the child's model dispatch onto the child's log (the events
	// and reports belong to the child session). Drained and stopped
	// with the same lifecycle as the subagent manager below.
	var childJobs *jobs.Manager
	if x.jobsReg != nil {
		childJobs, err = jobs.NewManager(child.Log, x.engine.Executor, x.engine.JobsOpts)
		if err != nil {
			return err
		}
		x.jobsReg.Put(child.ID, childJobs)
	}
	defer func() {
		// Drain-then-stop: every grandchild turn this child turn queued
		// reaches its post-run disposition before the manager goes away
		// (a rebuild never drops a queued follow-up turn).
		childMgr.Drain()
		childMgr.Stop()
		x.reg.Remove(child.ID)
		if childJobs != nil {
			// Same discipline for the child's background jobs: every
			// dispatched job this turn settles before the manager dies.
			childJobs.Drain()
			childJobs.Stop()
			x.jobsReg.Remove(child.ID)
		}
	}()
	opts.Tools = subagenttools.SpecsForDepth(opts.Tools, child.Depth, maxDepth)

	_, err = x.engine.TurnRunner().RunTurn(ctx, child.Log, x.engine.Adapter(), opts, "")
	return err
}

// wireSubagents arms the FileEngine's subagent surface: the real turn
// executor (self-referential — it reads the engine's lazily-built
// pipeline, so the approval-bridge-then-tools composition order in
// buildServer is preserved for child turns too), the child-log store
// under <session-dir>/subagents, the session registry binding both the
// engine (root sessions) and the executor (child sessions) write into
// and the model-facing subagent tools read from, and (P6) the jobs
// registry the child-turn executor binds child jobs managers into.
func wireSubagents(engine *protocol.FileEngine, sessionDir string, reg *subagents.Registry, jobsReg *jobsRegistry) {
	engine.SubagentExecutor = subagentTurnExecutor{engine: engine, reg: reg, jobsReg: jobsReg}
	engine.SubagentStore = subagents.NewFileStore(filepath.Join(sessionDir, subagentsDirName))
	engine.SubagentRegistry = reg
}
