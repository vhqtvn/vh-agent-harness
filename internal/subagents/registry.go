// registry.go — the session→Manager registry: the seam model-facing
// subagent tools (internal/tools/subagenttools) resolve the EXECUTING
// session's manager through. The daemon binds one entry per session
// whose model holds the spawn capability: the engine registers root
// managers at session create (and drops them on supersede), and the
// daemon's child-turn executor registers per-turn managers for child
// sessions before their turns run. Entries are wiring state, never
// durable truth — every Manager still rebuilds exclusively from logs.
package subagents

import "sync"

// Registry maps session ids to the Manager bound to that session's log.
// A nil-safe, mutex-guarded map: tool bodies resolve managers
// concurrently with session creation/supersede.
type Registry struct {
	mu sync.Mutex
	m  map[string]*Manager
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{m: map[string]*Manager{}} }

// Put binds (or rebinds) the manager for sessionID.
func (r *Registry) Put(sessionID string, m *Manager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[sessionID] = m
}

// Get returns the manager bound to sessionID, if any.
func (r *Registry) Get(sessionID string) (*Manager, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.m[sessionID]
	return m, ok
}

// Remove drops the binding for sessionID; removing an absent id is a
// no-op.
func (r *Registry) Remove(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, sessionID)
}
