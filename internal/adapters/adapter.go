// Package adapters defines the native engine's LLM adapter boundary: a
// narrow, provider-neutral single-call abstraction plus a registry.
// Slice 1 is non-streaming only; retry lives at a later, durable layer —
// never inside an adapter.
package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
)

// The adapter boundary speaks the session surface types directly: the
// LLM-visible message surface IS the session projection.
type (
	// Message is one LLM-visible message (alias of session.Message).
	Message = session.Message
	// ToolCall is one model-requested tool invocation (alias of session.ToolCall).
	ToolCall = session.ToolCall
	// Usage is the provider token-usage envelope (alias of session.Usage).
	Usage = session.Usage
)

// ToolSpec is one tool advertisement sent with a request.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// Request is one non-streaming chat-completions call: the derived message
// surface plus call parameters.
type Request struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	Temperature *float64
	MaxTokens   int
}

// Response is the normalized non-streaming result: assistant content,
// requested tool calls, usage, and the provider finish reason.
type Response struct {
	Model        string
	Content      string
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason string
}

// Adapter is a named LLM provider adapter talking ONLY to its configured
// base URL. Config (including credentials) arrives via the constructor —
// adapters never read the environment.
type Adapter interface {
	Name() string
	Call(ctx context.Context, req *Request) (*Response, error)
}

// Registry maps provider names to adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry returns an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

// Register adds an adapter under its Name(); duplicate names are rejected.
func (r *Registry) Register(a Adapter) error {
	if a == nil {
		return errors.New("adapters: cannot register a nil adapter")
	}
	name := a.Name()
	if name == "" {
		return errors.New("adapters: cannot register an adapter with an empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.adapters[name]; dup {
		return fmt.Errorf("adapters: provider %q is already registered", name)
	}
	r.adapters[name] = a
	return nil
}

// Get looks an adapter up by provider name.
func (r *Registry) Get(name string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	return a, ok
}

// Names returns the registered provider names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
