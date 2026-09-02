// Package tools implements the tool-execution pipeline on top of the
// slice-1 registry: pre-execution intent logging, the allow/deny/ask
// waterfall with fail-closed approval, monotonic deny-only ToolGuards,
// around-dispatch with per-tool timeouts, post-execute observers over the
// frozen canonical result, and (slice 3) the bounded scheduler + RunTurn
// orchestration. The logging choreography invariant holds throughout:
// tool/call is logged BEFORE execution, tool/result (frozen canonical
// content, isError on failure, typed outcome metadata) AFTER.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// DefaultMaxParallel is the default rolling-pool cap for concurrency-safe
// tool calls in one batch (dsh maxParallelToolCalls, default 10).
const DefaultMaxParallel = 10

// ToolDefinition is one registered tool: identity, a JSON-schema-ish
// argument description, concurrency/timeout policy, and the execute body
// returning canonical content.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	// IsConcurrencySafe marks the tool as safe to run concurrently with
	// other calls in the same model batch (a read-only probe usually is;
	// anything mutating shared state is not). Unsafe calls execute as
	// barriers: alone, with the parallel pool drained around them.
	// Unknown tools classify as unsafe (conservative).
	IsConcurrencySafe bool
	// TimeoutMs bounds one dispatch; 0 means no timeout. On expiry the
	// body's context is canceled and the result records the ORTHOGONAL
	// timedOut fact (never conflated with ordinary error text).
	TimeoutMs int
	Execute   func(ctx context.Context, args json.RawMessage) (string, error)
}

// Spec returns the adapter-facing advertisement for this tool.
func (t ToolDefinition) Spec() adapters.ToolSpec {
	return adapters.ToolSpec{Name: t.Name, Description: t.Description, Parameters: t.Parameters}
}

// Guard is the monotonic owner-policy layer: a deny-only pre-execution
// check over a tool call. A non-nil error vetoes execution. Guards run
// AFTER the pre-execute waterfall (they are the final, non-overridable
// denial) and CANNOT modify the call — the interface takes the call by
// value with detached args and returns only an error (verdict-only,
// force-allow unrepresentable; dsh F-EXT-1/F-PIPE-1). When a guard fires,
// its Name is recorded in the result provenance (DeniedBy).
type Guard interface {
	Name() string
	Check(call session.ToolCall) error
}

// Result is the frozen canonical outcome of one tool invocation. Tool
// failures are normalized into isError results, never thrown errors. The
// slice-3 fields are typed outcome metadata: the denial marker with
// identity + reason, the orthogonal timeout fact, and replace provenance.
type Result struct {
	CallID  string `json:"callId"`
	Name    string `json:"name"`
	Content string `json:"content"`
	IsError bool   `json:"isError"`
	// Denied marks a pre-execution denial (waterfall, approval, or guard).
	Denied bool `json:"denied,omitempty"`
	// DeniedBy names the denying observer or guard (identity provenance).
	DeniedBy string `json:"deniedBy,omitempty"`
	// DenyReason carries the denial justification.
	DenyReason string `json:"denyReason,omitempty"`
	// TimedOut classifies a timeout failure (orthogonal cause fact).
	TimedOut bool `json:"timedOut,omitempty"`
	// ReplacedBy names the post-execute observer that replaced the content.
	ReplacedBy string `json:"replacedBy,omitempty"`
}

// PipelineOptions configures one Pipeline. The zero value is the
// production default: no approver (asks fail closed), real clock, and
// DefaultMaxParallel.
type PipelineOptions struct {
	// Approver resolves ask verdicts; nil ⇒ unresolved asks DENY.
	Approver Approver
	// Clock times the dispatch timeout; nil ⇒ RealClock.
	Clock Clock
	// MaxParallel caps the rolling pool for concurrency-safe batch calls;
	// 0 ⇒ DefaultMaxParallel.
	MaxParallel int
}

// Pipeline is the tool registry + decision lattice + dispatch.
type Pipeline struct {
	mu            sync.RWMutex
	defs          map[string]ToolDefinition
	guards        []Guard
	preObservers  []PreExecuteObserver
	postObservers []PostExecuteObserver
	approver      Approver
	clock         Clock
	maxParallel   int
}

// NewPipeline returns an empty pipeline with default options.
func NewPipeline() *Pipeline {
	return NewPipelineWithOptions(PipelineOptions{})
}

// NewPipelineWithOptions returns an empty pipeline bound to opts. The
// decision lattice (approver, clock, pool cap) is frozen at construction;
// guards and observers are added afterwards via the Add* methods.
func NewPipelineWithOptions(opts PipelineOptions) *Pipeline {
	clock := opts.Clock
	if clock == nil {
		clock = RealClock{}
	}
	maxParallel := opts.MaxParallel
	if maxParallel <= 0 {
		maxParallel = DefaultMaxParallel
	}
	return &Pipeline{
		defs:        make(map[string]ToolDefinition),
		approver:    opts.Approver,
		clock:       clock,
		maxParallel: maxParallel,
	}
}

// Register validates and adds a tool definition; duplicates are rejected.
func (p *Pipeline) Register(def ToolDefinition) error {
	if def.Name == "" {
		return errors.New("tools: tool name is required")
	}
	if def.Execute == nil {
		return fmt.Errorf("tools: tool %q has no Execute function", def.Name)
	}
	if def.TimeoutMs < 0 {
		return fmt.Errorf("tools: tool %q has a negative TimeoutMs (%d)", def.Name, def.TimeoutMs)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, dup := p.defs[def.Name]; dup {
		return fmt.Errorf("tools: tool %q is already registered", def.Name)
	}
	p.defs[def.Name] = def
	return nil
}

// AddGuard appends a deny-only guard consulted after the waterfall.
func (p *Pipeline) AddGuard(g Guard) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.guards = append(p.guards, g)
}

// AddPreObserver appends one observer to the ordered pre-execute
// waterfall (downstream verdicts may resolve upstream asks; nothing
// resolves a deny).
func (p *Pipeline) AddPreObserver(o PreExecuteObserver) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.preObservers = append(p.preObservers, o)
}

// AddPostObserver appends one post-execute observer over executed
// outcomes (accept or replace-with-provenance; isError unflippable).
func (p *Pipeline) AddPostObserver(o PostExecuteObserver) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.postObservers = append(p.postObservers, o)
}

// Definitions returns the registered tools sorted by name.
func (p *Pipeline) Definitions() []ToolDefinition {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ToolDefinition, 0, len(p.defs))
	for _, def := range p.defs {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// PreObserverNames returns the registered pre-execute observers' names
// in registration order. Inspection seam only: it exists so a composer
// (the daemon's ask posture) can PIN the observer chain order — the
// waterfall is order-sensitive (a downstream allow resolves an upstream
// ask), and an unordered composition is a silent security regression.
func (p *Pipeline) PreObserverNames() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, 0, len(p.preObservers))
	for _, o := range p.preObservers {
		out = append(out, o.Name())
	}
	return out
}

// policy is the immutable snapshot of the decision lattice consulted by
// one Execute (taken under RLock so execution itself runs lock-free and
// Pipeline.Execute is safe for concurrent use).
type policy struct {
	def           ToolDefinition
	known         bool
	guards        []Guard
	preObservers  []PreExecuteObserver
	postObservers []PostExecuteObserver
	approver      Approver
	clock         Clock
}

func (p *Pipeline) snapshot(call session.ToolCall) policy {
	p.mu.RLock()
	defer p.mu.RUnlock()
	def, known := p.defs[call.Name]
	return policy{
		def:           def,
		known:         known,
		guards:        append([]Guard(nil), p.guards...),
		preObservers:  append([]PreExecuteObserver(nil), p.preObservers...),
		postObservers: append([]PostExecuteObserver(nil), p.postObservers...),
		approver:      p.approver,
		clock:         p.clock,
	}
}

// detach returns a copy of call whose Args do not share backing storage
// with the original — every observer and guard gets its own detached
// args, so byte-level scribbling in one cannot reach another or the
// executor (dsh "args are detached and frozen").
func detach(call session.ToolCall) session.ToolCall {
	call.Args = append(json.RawMessage(nil), call.Args...)
	return call
}

// Execute runs one tool call through the full pipeline:
//
//	tool/call (logged by ExecuteLogged, PRE-execution)
//	  → pre-execute waterfall (allow/deny/ask, fail-closed approval)
//	  → monotonic ToolGuards (deny-only, AFTER the waterfall)
//	  → around-dispatch (per-tool TimeoutMs via the injected clock)
//	  → post-execute observers (accept / replace-with-provenance)
//	tool/result (frozen canonical content + typed outcome metadata)
//
// It always returns a Result — unknown tool, waterfall denial, approval
// denial, guard veto, timeout, execution error, and panic are all
// normalized to isError results (the pipeline never crashes on a tool).
func (p *Pipeline) Execute(ctx context.Context, call session.ToolCall) Result {
	res := Result{CallID: call.ID, Name: call.Name}
	pol := p.snapshot(call)

	if !pol.known {
		res.IsError = true
		res.Content = fmt.Sprintf("unknown tool: %s", call.Name)
		return res
	}

	// 1. Pre-execute waterfall.
	if who, reason, denied := runWaterfall(ctx, pol, call); denied {
		denyResult(&res, who, reason)
		return res
	}

	// 2. Monotonic guards — the final, non-overridable denial layer.
	for _, g := range pol.guards {
		if err := g.Check(detach(call)); err != nil {
			res.IsError = true
			res.Denied = true
			res.DeniedBy = g.Name()
			res.DenyReason = err.Error()
			res.Content = fmt.Sprintf("denied by guard %s: %v", g.Name(), err)
			return res
		}
	}

	// 3. Around-dispatch with the per-tool timeout.
	content, err, timedOut := dispatch(ctx, pol.def, call, pol.clock)
	switch {
	case timedOut:
		res.IsError = true
		res.TimedOut = true
		res.Content = fmt.Sprintf("tool %s timed out after %dms", call.Name, pol.def.TimeoutMs)
	case err != nil:
		res.IsError = true
		res.Content = fmt.Sprintf("tool %s failed: %v", call.Name, err)
	default:
		res.Content = content
	}

	// 4. Post-execute observers — even on error results (never on the
	// denial paths above: those returned before dispatch).
	for _, o := range pol.postObservers {
		o.ObservePostExecute(detach(call), &PostResult{res: &res, observer: o.Name()})
	}
	return res
}

// runWaterfall folds the ordered observers into one decision. deny is
// absorbing (anywhere in the chain denies); an ask stands until a
// DOWNSTREAM allow resolves it; a surviving ask goes to the Approver and
// fails closed to deny when none is configured or it refuses. An unknown
// verdict kind also fails closed.
func runWaterfall(ctx context.Context, pol policy, call session.ToolCall) (who, reason string, denied bool) {
	askBy, askReason := "", ""
	for _, o := range pol.preObservers {
		v := o.ObservePreExecute(detach(call))
		switch v.Kind {
		case VerdictDeny:
			return o.Name(), v.Reason, true
		case VerdictAsk:
			askBy, askReason = o.Name(), v.Reason
		case VerdictAllow:
			askBy, askReason = "", "" // downstream allow resolves the ask
		default:
			return o.Name(), fmt.Sprintf("observer returned invalid verdict kind %q", v.Kind), true
		}
	}
	if askBy == "" {
		return "", "", false
	}
	if pol.approver == nil {
		return askBy, fmt.Sprintf("approval required (%s) but no approver is configured", askReason), true
	}
	d := pol.approver.Approve(ctx, detach(call), askReason)
	if !d.Allow {
		return askBy, fmt.Sprintf("approval denied: %s", d.Reason), true
	}
	return "", "", false
}

// denyResult marks res as a denial: isError + the typed marker + reason
// provenance.
func denyResult(res *Result, who, reason string) {
	res.IsError = true
	res.Denied = true
	res.DeniedBy = who
	res.DenyReason = reason
	res.Content = fmt.Sprintf("denied by %s: %s", who, reason)
}

// dispatch runs the execute body under the per-tool timeout. It returns
// (content, err, timedOut); timedOut is the orthogonal cause fact, set
// only when the deadline fired. On timeout the body's context is canceled
// and its late result is discarded — a body that ignores its context may
// outlive the call but can never affect the result.
func dispatch(ctx context.Context, def ToolDefinition, call session.ToolCall, clock Clock) (string, error, bool) {
	if def.TimeoutMs <= 0 {
		content, err := runTool(def, ctx, call)
		return content, err, false
	}
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	timeoutCh := clock.After(time.Duration(def.TimeoutMs) * time.Millisecond)
	type outcome struct {
		content string
		err     error
	}
	done := make(chan outcome, 1) // buffered: the body's write never blocks after a timeout return
	go func() {
		c, e := runTool(def, ctx2, call)
		done <- outcome{c, e}
	}()
	select {
	case o := <-done:
		return o.content, o.err, false
	case <-timeoutCh:
		return "", nil, true
	}
}

// runTool invokes the execute body, normalizing panics into errors.
func runTool(def ToolDefinition, ctx context.Context, call session.ToolCall) (content string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool panicked: %v", r)
		}
	}()
	return def.Execute(ctx, call.Args)
}

// ExecuteLogged runs one tool call with the logging choreography:
// tool/call is logged PRE-execution (fail-closed: if the intent cannot be
// logged, execution never starts), then the frozen result with its typed
// outcome metadata is logged after.
func (p *Pipeline) ExecuteLogged(ctx context.Context, lg *session.Log, call session.ToolCall) (Result, error) {
	if _, err := lg.AppendToolCall(call.ID, call.Name, call.Args); err != nil {
		return Result{}, fmt.Errorf("tools: log tool/call (pre-execution): %w", err)
	}
	res := p.Execute(ctx, call)
	if err := logResult(lg, &res); err != nil {
		return res, fmt.Errorf("tools: log tool/result: %w", err)
	}
	return res, nil
}

// logResult appends the frozen canonical result with full metadata. It
// is the COMMIT seam: when the log carries an armed spill policy, an
// oversize content is rewritten IN PLACE (res.Content becomes the
// bounded preview + notice, so the returned result — the turn report
// and the wire — sees exactly what was committed) and the payload
// carries the additive spill fields. With no policy armed the bytes are
// identical to the pre-spill shape (SpillPolicy.Apply's inline path is
// byte-stable). A spill failure falls back to inline silently — a
// sidecar write must never fail the tool result.
func logResult(lg *session.Log, res *Result) error {
	var loc *session.SpillLocator
	if pol := lg.SpillPolicy(); pol != nil {
		res.Content, loc, _ = pol.Apply("", res.Content) // loc != nil iff spilled
	}
	_, err := lg.AppendToolResultMeta(session.ToolResultPayload{
		CallID:       res.CallID,
		Name:         res.Name,
		Content:      res.Content,
		IsError:      res.IsError,
		Denied:       res.Denied,
		DeniedBy:     res.DeniedBy,
		DenyReason:   res.DenyReason,
		TimedOut:     res.TimedOut,
		ReplacedBy:   res.ReplacedBy,
		Spilled:      loc != nil,
		SpillLocator: loc,
	})
	return err
}
