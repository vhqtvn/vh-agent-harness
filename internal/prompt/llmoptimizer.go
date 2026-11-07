// llmoptimizer.go — the REAL adapter-backed sys-prompt optimizer: the
// missing half of the compiled-sysprompt feature. It is compile-time
// ONLY (invoked by the explicit offline --compile-prompt step, never in
// the request path), builds a deterministic optimization request, makes
// exactly ONE LLM call through an injected call func shaped like
// adapters.Adapter.Call (no concrete adapter package is imported — the
// seam stays injectable; tests drive openaicompat over httptest), and
// treats the model's output as a CANDIDATE: the six mechanical
// invariants in compile.go remain the authority and fail the compile on
// violation. Fail-closed throughout — any call error, empty content, or
// unparseable body is a typed error; there is NO fallback optimization
// inside the optimizer. A failed compile is simply a rerun of the
// command (no retries, no best-of-N — deliberate non-goals).
package prompt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
)

// llmInstructionsSchema is the schema version of the optimizer's
// instructions/output contract. Bump it whenever the request shape or
// the output parsing contract changes materially — the version string
// participates in the artifact content hash, so a bump invalidates
// every previously compiled artifact.
const llmInstructionsSchema = "v1"

// Fail-closed sentinel errors. The optimizer NEVER falls back to a
// mechanical optimization on any of these; the caller reports and the
// operator reruns the compile.
var (
	// ErrLLMOptimizerCallFailed wraps every injected-call failure.
	ErrLLMOptimizerCallFailed = errors.New("prompt: llm optimizer call failed")
	// ErrLLMOptimizerEmptyContent: the assistant message carried no content.
	ErrLLMOptimizerEmptyContent = errors.New("prompt: llm optimizer returned empty content")
	// ErrLLMOptimizerUnparseable: the content is not a strict JSON
	// object mapping section keys to body strings.
	ErrLLMOptimizerUnparseable = errors.New("prompt: llm optimizer output must be a strict JSON object mapping section keys to body strings")
	// ErrLLMOptimizerGhostSection: the object carries a key that is not
	// an input section (parse-time rejection of ghost sections — the
	// compile-time exact-cover invariant would also catch them, but this
	// names the offender precisely).
	ErrLLMOptimizerGhostSection = errors.New("prompt: llm optimizer output contains unknown section keys")
	// ErrLLMOptimizerMisconfigured guards the zero-value optimizer.
	ErrLLMOptimizerMisconfigured = errors.New("prompt: llm optimizer is misconfigured")
)

// LLMCallFunc is the injectable adapter seam: exactly
// adapters.Adapter.Call. Any adapter satisfies it via a method value
// (e.g. ad.Call); tests inject fakes or drive a real openaicompat
// adapter over httptest.
type LLMCallFunc func(ctx context.Context, req *adapters.Request) (*adapters.Response, error)

// LLMOptimizer optimizes assembled sections through one LLM call. It is
// stateless and safe for concurrent use (all per-run state is local).
type LLMOptimizer struct {
	model string
	call  LLMCallFunc
}

// NewLLMOptimizer builds the optimizer over an injected call func. The
// model name is required: it names the model in the request AND derives
// the version string, so an anonymous optimizer cannot produce
// unauditable artifacts.
func NewLLMOptimizer(model string, call LLMCallFunc) (*LLMOptimizer, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("%w: model name is required", ErrLLMOptimizerMisconfigured)
	}
	if call == nil {
		return nil, fmt.Errorf("%w: call func is required", ErrLLMOptimizerMisconfigured)
	}
	return &LLMOptimizer{model: model, call: call}, nil
}

// LLMOptimizerVersion derives the stable version string for a model. It
// is deterministic — derived from the model name and the instructions
// schema version, never self-reported by the model — so the serving
// side can compute the artifact hash for a config without any network
// or credentials.
func LLMOptimizerVersion(model string) string {
	return "llmopt/" + llmInstructionsSchema + "/" + model
}

// Version returns the deterministic optimizer version.
func (o *LLMOptimizer) Version() string { return LLMOptimizerVersion(o.model) }

// Versioned binds the optimizer into the Compile-facing shape. Version
// authority stays here (caller-side), never in the model output.
func (o *LLMOptimizer) Versioned() VersionedOptimizer {
	return VersionedOptimizer{Version: o.Version(), Fn: o.Optimize}
}

// llmSectionPayload is one section as handed to the model: the
// interpolated body plus its provenance, so the model can see (and
// preserve) required sections and ownership.
type llmSectionPayload struct {
	Number   int    `json:"number"`
	Key      string `json:"key"`
	Owner    string `json:"owner"`
	Required bool   `json:"required"`
	Body     string `json:"body"`
}

// buildLLMSystemInstructions states the task and the invariants
// contract the output must satisfy. It is a pure function of the
// contract — deterministic, no environment, no clocks.
func buildLLMSystemInstructions(contract InvariantsContract) string {
	delegated := sortedCopy(contract.DelegatedTools)
	delegatedNote := "none"
	if len(delegated) > 0 {
		delegatedNote = strings.Join(delegated, ", ")
	}
	growth := fmt.Sprintf("total output must not exceed the total input (ratio %.2f)", contract.effectiveRatio())
	if contract.AllowGrowth {
		growth = "growth is explicitly allowed (there is still no reason to pad)"
	}

	var b strings.Builder
	b.WriteString("You optimize the system prompt of a headless agent daemon. You receive the current prompt sections (numbered, with provenance) and the registered tool catalog. Rewrite the sections to be shorter and clearer without losing operational content.\n\n")
	b.WriteString("Output contract — a mechanical checker enforces it and the compile FAILS on violation:\n")
	b.WriteString("1. Respond with ONLY a JSON object mapping section key to its new body string. No commentary. A single markdown fence (```json ... ```) around the object is tolerated.\n")
	b.WriteString("2. Use only the input section keys as object keys. Unknown keys are rejected.\n")
	b.WriteString("3. Sections you omit from the object (or map to an empty string) are dropped. Dropping or rewriting a section marked \"required\" FAILS the compile: return required sections with their body unchanged (whitespace-only differences are tolerated).\n")
	fmt.Fprintf(&b, "4. Size ratchet: %s.\n", growth)
	fmt.Fprintf(&b, "5. Every registered tool must still be referenced by its exact name (whole word) in at least one output body, unless it is delegated (delegated tools: %s).\n", delegatedNote)
	b.WriteString("6. Never mention a tool name that is not in the catalog or the delegation list.\n")
	return b.String()
}

// buildLLMRequest assembles the deterministic optimization request:
// [system: task + invariants contract, user: sections with provenance +
// tool catalog + output schema restatement], temperature pinned to 0,
// no tools, no max-tokens override. Identical inputs yield identical
// payload bytes.
func buildLLMRequest(model string, in OptimizeInput) *adapters.Request {
	sections := make([]llmSectionPayload, 0, len(in.Sections))
	for _, s := range in.Sections {
		sections = append(sections, llmSectionPayload{
			Number:   s.Number,
			Key:      s.Key,
			Owner:    s.Owner,
			Required: s.Required,
			Body:     s.Body,
		})
	}
	payload, err := json.Marshal(struct {
		Sections []llmSectionPayload `json:"sections"`
		Tools    []ToolSummary       `json:"tools"`
	}{Sections: sections, Tools: in.Catalog.Tools})
	if err != nil {
		// struct/slice marshal cannot fail; keep a deterministic escape.
		payload = []byte(`{"sections":null,"tools":null}`)
	}
	zero := 0.0
	return &adapters.Request{
		Model: model,
		Messages: []adapters.Message{
			{Role: "system", Content: buildLLMSystemInstructions(in.Invariants)},
			{Role: "user", Content: `Optimize these sections. Respond with only the JSON object {"<section-key>": "<new body>"}.` + "\n" + string(payload)},
		},
		Temperature: &zero,
		MaxTokens:   0, // adapter default; the size ratchet is enforced by the invariants
	}
}

// stripFence removes ONE optional markdown fence (```json or ```)
// surrounding the payload.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	} else {
		return strings.TrimSpace(strings.TrimPrefix(s, "```json"))
	}
	if end := strings.LastIndex(s, "```"); end >= 0 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
}

// parseSectionMap parses the model content into the strict
// section-key → body map: non-object JSON, non-string values, and ghost
// keys are typed errors.
func parseSectionMap(content string, inputKeys map[string]struct{}) (map[string]string, error) {
	body := stripFence(content)
	var m map[string]string
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLLMOptimizerUnparseable, err)
	}
	var ghosts []string
	for k := range m {
		if _, ok := inputKeys[k]; !ok {
			ghosts = append(ghosts, k)
		}
	}
	if len(ghosts) > 0 {
		sort.Strings(ghosts)
		return nil, fmt.Errorf("%w: %v", ErrLLMOptimizerGhostSection, ghosts)
	}
	return m, nil
}

// Optimize runs the single compile-time LLM call and folds the parsed
// section map into an OptimizedPrompt. Mapping rules:
//
//   - key present, rendered-equivalent body  → ActionPreserved
//   - key present, changed body              → ActionRewritten
//   - key absent / blank body                → ActionDropped, with an
//     EMPTY rationale on purpose: dropping a required section must fail
//     the compile-time invariant (the invariants are the authority —
//     the optimizer never launders a drop with a synthesized rationale).
//
// ReferencedTools declares the catalog names actually present as whole
// words in the output bytes (honest post-optimization reality).
func (o *LLMOptimizer) Optimize(ctx context.Context, in OptimizeInput) (OptimizedPrompt, error) {
	if o.call == nil || strings.TrimSpace(o.model) == "" {
		return OptimizedPrompt{}, fmt.Errorf("%w: model/call func missing (use NewLLMOptimizer)", ErrLLMOptimizerMisconfigured)
	}
	resp, err := o.call(ctx, buildLLMRequest(o.model, in))
	if err != nil {
		return OptimizedPrompt{}, fmt.Errorf("%w: %w", ErrLLMOptimizerCallFailed, err)
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return OptimizedPrompt{}, ErrLLMOptimizerEmptyContent
	}

	inputKeys := make(map[string]struct{}, len(in.Sections))
	for _, s := range in.Sections {
		inputKeys[s.Key] = struct{}{}
	}
	m, err := parseSectionMap(resp.Content, inputKeys)
	if err != nil {
		return OptimizedPrompt{}, err
	}

	kept := make([]Section, 0, len(in.Sections))
	outcomes := make([]SectionOutcome, 0, len(in.Sections))
	rewritten, dropped := 0, 0
	for _, s := range in.Sections {
		body, ok := m[s.Key]
		if !ok || strings.TrimSpace(body) == "" {
			outcomes = append(outcomes, SectionOutcome{Key: s.Key, Action: ActionDropped})
			dropped++
			continue
		}
		ns := s
		ns.Body = body
		kept = append(kept, ns)
		// RenderSections trims trailing whitespace before emitting, so a
		// trailing-whitespace-only delta renders identically — classify
		// it preserved, matching what actually lands in the bytes.
		if strings.TrimRight(body, " \t\r\n") == strings.TrimRight(s.Body, " \t\r\n") {
			outcomes = append(outcomes, SectionOutcome{Key: s.Key, Action: ActionPreserved})
		} else {
			outcomes = append(outcomes, SectionOutcome{Key: s.Key, Action: ActionRewritten})
			rewritten++
		}
	}

	out := RenderSections(kept)
	return OptimizedPrompt{
		Bytes:           out,
		SectionOutcomes: outcomes,
		ReferencedTools: referencedToolNames(out, in.Catalog),
		Notes:           []string{fmt.Sprintf("llmopt: model=%s schema=%s sections rewritten=%d dropped=%d (single call, no retries)", o.model, llmInstructionsSchema, rewritten, dropped)},
	}, nil
}
