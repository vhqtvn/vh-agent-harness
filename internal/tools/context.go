// context.go — the executing-session context binding. RunTurn stamps
// the executing log's session id into the context it hands tool bodies,
// so a model-facing tool registered on a SHARED pipeline (the daemon
// has exactly one) can still resolve per-session state — the subagent
// family looks up the executing session's manager through it. The value
// is the identity string only, never a log handle: a tool cannot
// scribble on the executing session's durable stream through the
// context.
package tools

import "context"

// executingSessionKey is the context key type (unexported, per the
// context convention).
type executingSessionKey struct{}

// WithExecutingSession returns ctx carrying sessionID as the executing
// session. RunTurn applies this automatically; tests and embeddings may
// apply it to drive tool bodies outside a full turn.
func WithExecutingSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, executingSessionKey{}, sessionID)
}

// ExecutingSessionFrom returns the executing session id in ctx, or ""
// when absent (a call outside any turn).
func ExecutingSessionFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(executingSessionKey{}).(string)
	return v
}
