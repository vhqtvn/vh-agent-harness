// script.go — the scripted scenario format and its fail-closed loader.
//
// The script is a JSON array of step objects consumed FIFO globally (the
// ordering contract is the test author's: the mock never reorders). Each
// step carries EXACTLY ONE response class:
//
//	{"text": "..."}                          200 assistant text
//	{"tool_calls": [{id,name,args{...}}]}    200 tool calls (both dialects)
//	{"fault": {status, body, retry_after_ms}} error status (+ Retry-After)
//	{"empty": true}                          200 with no content (empty class)
//
// Anything else — wrong top-level shape, ambiguous steps, unknown fields,
// non-object tool args, out-of-range fault statuses — is a fail-closed
// parse error naming the file and the offending step. A malformed script
// is a test bug: the mock must refuse to start (exit 2), never guess.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ToolCallStep is one scripted model-requested tool invocation. Args is a
// JSON OBJECT in the script; the wire encoders project it per dialect
// (OpenAI: string-encoded arguments; Anthropic: an input object).
type ToolCallStep struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// FaultSpec is one scripted error response. Status must be an HTTP error
// status (400..599); Body is written verbatim; RetryAfterMs > 0 emits a
// Retry-After header in the seconds form (ceil).
type FaultSpec struct {
	Status       int    `json:"status"`
	Body         string `json:"body,omitempty"`
	RetryAfterMs int64  `json:"retry_after_ms,omitempty"`
}

// Step is one scripted response. Exactly one class is set per step.
type Step struct {
	Text      string         `json:"text,omitempty"`
	ToolCalls []ToolCallStep `json:"tool_calls,omitempty"`
	Fault     *FaultSpec     `json:"fault,omitempty"`
	Empty     bool           `json:"empty,omitempty"`
}

// stepDecoder is Step with strict unknown-field rejection (the Decoder's
// DisallowUnknownFields recurses into the nested tool-call objects too).
// A step that names an undocumented key is a test bug — fail closed,
// never ignore.
type stepDecoder struct {
	Text      string            `json:"text"`
	ToolCalls []toolCallDecoder `json:"tool_calls"`
	Fault     *FaultSpec        `json:"fault"`
	Empty     bool              `json:"empty"`
}

type toolCallDecoder struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// LoadScript reads and validates the script at path, returning the step
// list. Every failure names the path and the offending step index.
func LoadScript(path string) ([]Step, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mock script %s: cannot read: %w", path, err)
	}
	return parseScript(path, raw)
}

// parseScript validates src as a whole script (test seam). Class
// detection is PRESENCE-based (a `"text":""` key is present-but-empty
// and therefore not a valid class, while `"empty":false` present is not
// a class either) so ambiguous authoring fails closed instead of
// silently collapsing onto whichever value looked set.
func parseScript(path string, raw []byte) ([]Step, error) {
	// Top level must be an array of step objects. Decode via json.RawMessage
	// elements so a non-object step gets a precise message (not a decode
	// error deep inside the stdlib).
	var elems []json.RawMessage
	if err := json.Unmarshal(raw, &elems); err != nil {
		return nil, fmt.Errorf("mock script %s: invalid JSON: top-level value must be an ARRAY of step objects: %w", path, err)
	}
	if len(elems) == 0 {
		return nil, fmt.Errorf("mock script %s: empty array: a script needs at least one step", path)
	}
	steps := make([]Step, 0, len(elems))
	for i, el := range elems {
		var d stepDecoder
		dec := json.NewDecoder(bytes.NewReader(el))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&d); err != nil {
			if strings.Contains(err.Error(), "unknown field") {
				return nil, fmt.Errorf("mock script %s: step %d: unknown field (documented classes: text | tool_calls | fault | empty): %w", path, i+1, err)
			}
			return nil, fmt.Errorf("mock script %s: step %d: not a step object: %w", path, i+1, err)
		}
		// Presence-based class detection: which documented keys appear?
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(el, &probe); err != nil {
			return nil, fmt.Errorf("mock script %s: step %d: not a step object: %w", path, i+1, err)
		}
		present := 0
		for _, key := range []string{"text", "tool_calls", "fault", "empty"} {
			if _, ok := probe[key]; ok {
				present++
			}
		}
		if present == 0 {
			return nil, fmt.Errorf("mock script %s: step %d: no recognized class (set exactly one of text, tool_calls, fault, empty)", path, i+1)
		}
		if present > 1 {
			return nil, fmt.Errorf("mock script %s: step %d: ambiguous: set exactly one of text, tool_calls, fault, empty (found %d classes)", path, i+1, present)
		}
		_, textPresent := probe["text"]
		switch {
		case textPresent:
			if d.Text == "" {
				return nil, fmt.Errorf("mock script %s: step %d: text is present but empty — a text step needs non-empty text (use {\"empty\":true} for the empty-response class)", path, i+1)
			}
			steps = append(steps, Step{Text: d.Text})
		case len(d.ToolCalls) > 0:
			calls := make([]ToolCallStep, 0, len(d.ToolCalls))
			for j, tc := range d.ToolCalls {
				if tc.ID == "" {
					return nil, fmt.Errorf("mock script %s: step %d: tool_calls[%d].id is required (deterministic call ids are the test's assertion anchors)", path, i+1, j)
				}
				if tc.Name == "" {
					return nil, fmt.Errorf("mock script %s: step %d: tool_calls[%d].name is required", path, i+1, j)
				}
				if len(tc.Args) > 0 {
					var argsProbe any
					if err := json.Unmarshal(tc.Args, &argsProbe); err != nil {
						return nil, fmt.Errorf("mock script %s: step %d: tool_calls[%d].args must be a JSON object: %w", path, i+1, j, err)
					}
					if _, ok := argsProbe.(map[string]any); !ok {
						return nil, fmt.Errorf("mock script %s: step %d: tool_calls[%d].args must be a JSON OBJECT (the dialect encoders do the per-wire projection)", path, i+1, j)
					}
				}
				calls = append(calls, ToolCallStep{ID: tc.ID, Name: tc.Name, Args: tc.Args})
			}
			steps = append(steps, Step{ToolCalls: calls})
		case d.Fault != nil:
			f := d.Fault
			if f.Status < 400 || f.Status > 599 {
				return nil, fmt.Errorf("mock script %s: step %d: fault.status %d is not an HTTP error status (400..599)", path, i+1, f.Status)
			}
			if f.RetryAfterMs < 0 {
				return nil, fmt.Errorf("mock script %s: step %d: fault.retry_after_ms must be >= 0, got %d", path, i+1, f.RetryAfterMs)
			}
			steps = append(steps, Step{Fault: f})
		case d.Empty:
			steps = append(steps, Step{Empty: true})
		default:
			// Present-but-falsy keys: {"tool_calls":[]}, {"fault":null},
			// {"empty":false} — a class key that carries no class.
			return nil, fmt.Errorf("mock script %s: step %d: no recognized class (the present key carries no value: tool_calls empty, fault null, or empty false)", path, i+1)
		}
	}
	return steps, nil
}
