// scheduler.go — batch execution of model-requested tool calls under the
// concurrency policy (dsh agent-loop scheduler semantics, simplified for
// the in-process slice 3):
//
//   - concurrency-safe calls run in a bounded rolling pool (cap
//     DefaultMaxParallel = 10 by default);
//   - each non-safe call is a BARRIER: it starts only after the preceding
//     pool has fully drained and it runs alone;
//   - only dispatch/body overlaps — intents and results stay MODEL-ORDERED:
//     every tool/call is committed (in model order) before any dispatch,
//     and tool/result events are appended in model order as results
//     become available in that order, so the persisted log NEVER depends
//     on execution timing and deterministic replay holds by construction.
package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// segment is one scheduling unit: a maximal run of consecutive
// concurrency-safe calls (pool), or exactly one non-safe call (barrier).
type segment struct {
	barrier bool
	idxs    []int
}

// segmentBatch splits the batch by concurrency classification. The calls
// slice is consulted in model order; classification is unary per call
// (no sibling comparison), so a call whose safety depends on its siblings
// classifies non-safe, conservatively.
func (p *Pipeline) segmentBatch(calls []session.ToolCall) []segment {
	var segs []segment
	for i, c := range calls {
		if p.concurrencySafe(c.Name) {
			if n := len(segs); n > 0 && !segs[n-1].barrier {
				segs[n-1].idxs = append(segs[n-1].idxs, i)
				continue
			}
			segs = append(segs, segment{idxs: []int{i}})
			continue
		}
		segs = append(segs, segment{barrier: true, idxs: []int{i}})
	}
	return segs
}

func (p *Pipeline) concurrencySafe(name string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defs[name].IsConcurrencySafe
}

// ExecuteBatchLogged runs the full batch choreography on lg:
//
//  1. tool/call for EVERY call, in MODEL order, before any execution —
//     fail-closed: if any intent cannot be logged, NO execution starts;
//  2. execution per the concurrency policy (pool with cap; barriers
//     alone), every call through the full Pipeline.Execute lattice;
//  3. tool/result appended in MODEL order (a result is committed only
//     once every earlier result has committed), regardless of the order
//     bodies completed in.
//
// The returned results slice is model-ordered and index-aligned with
// calls. ExecuteBatchLogged is safe to call concurrently with other
// Pipeline reads; the session log itself is touched only by the calling
// goroutine (workers never log).
func (p *Pipeline) ExecuteBatchLogged(ctx context.Context, lg *session.Log, calls []session.ToolCall) ([]Result, error) {
	return p.executeBatch(ctx, lg, calls, p.maxParallel)
}

func (p *Pipeline) executeBatch(ctx context.Context, lg *session.Log, calls []session.ToolCall, maxParallel int) ([]Result, error) {
	// Phase 1: durable intents, model order, fail-closed.
	for _, c := range calls {
		if _, err := lg.AppendToolCall(c.ID, c.Name, c.Args); err != nil {
			return nil, fmt.Errorf("tools: log tool/call (pre-execution, batch): %w", err)
		}
	}

	// Phase 2: execute per policy into fixed model-order slots.
	results := make([]Result, len(calls))
	done := make([]chan struct{}, len(calls))
	for i := range done {
		done[i] = make(chan struct{})
	}
	for _, seg := range p.segmentBatch(calls) {
		if seg.barrier {
			i := seg.idxs[0]
			results[i] = p.Execute(ctx, calls[i])
			close(done[i])
			continue
		}
		sem := make(chan struct{}, maxParallel)
		var wg sync.WaitGroup
		for _, i := range seg.idxs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				results[i] = p.Execute(ctx, calls[i])
				close(done[i])
			}(i)
		}
		wg.Wait()
	}

	// Phase 3: model-ordered commits — result i is appended only after
	// results 0..i-1 have appended, so a fast body behind a slow sibling
	// waits its turn and the log stays timing-independent. logResult is
	// the commit seam: an armed spill policy rewrites oversize content
	// to a bounded preview in place (results[i] carries the preview).
	for i := range calls {
		<-done[i]
		if err := logResult(lg, &results[i]); err != nil {
			return nil, fmt.Errorf("tools: log tool/result (batch): %w", err)
		}
	}
	return results, nil
}
