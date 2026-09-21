// Package prompt implements the "compiled sys prompt" model for the native
// engine: system-prompt assembly is deterministic and cheap, optimization is
// an EXPLICIT OFFLINE step that runs ahead of serving, and the request path
// only ever serves previously compiled bytes or falls back to raw assembly.
// No optimizer logic runs per request.
//
// The assembler follows the dsh kernel section pattern: ordered numbered
// sections where -100 is harness identity, 0 is persona, 100–199 is tool
// guidance; gaps are allowed, sections sort by number, and same-numbered
// sections keep registration order.
package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Section is one numbered, provenance-carrying prompt section.
type Section struct {
	// Number orders the section in the render. Gaps are allowed;
	// same-numbered sections keep registration order.
	Number int
	// Key is the unique stable identifier of the section.
	Key string
	// Owner records provenance: which layer registered the section
	// (e.g. "core", "overlay/<pack>", "profile").
	Owner string
	// Body is the raw section text. It may contain {{variable}}
	// placeholders that are interpolated at render time (strictly).
	Body string
	// Required marks a section the optimizer may only drop or merge
	// when it records a rationale in the compiled artifact.
	Required bool
	// CacheStable marks a section whose content is stable across
	// requests (prefix-cache friendly). It is surfaced verbatim in the
	// compiled artifact for accounting; it never reorders sections.
	CacheStable bool
}

// Assembler accumulates numbered sections and renders deterministic bytes.
// The zero value is not usable; call NewAssembler.
type Assembler struct {
	sections []Section
	keys     map[string]struct{}
}

// NewAssembler returns an empty prompt assembler.
func NewAssembler() *Assembler {
	return &Assembler{keys: make(map[string]struct{})}
}

// Register adds a section. Duplicate or empty keys are rejected.
func (a *Assembler) Register(s Section) error {
	if strings.TrimSpace(s.Key) == "" {
		return errors.New("prompt: section key must be non-empty")
	}
	if s.Key != strings.TrimSpace(s.Key) {
		return fmt.Errorf("prompt: section key %q must not carry surrounding whitespace", s.Key)
	}
	if _, dup := a.keys[s.Key]; dup {
		return fmt.Errorf("prompt: duplicate section key %q", s.Key)
	}
	a.keys[s.Key] = struct{}{}
	a.sections = append(a.sections, s)
	return nil
}

// Len reports the number of registered sections.
func (a *Assembler) Len() int { return len(a.sections) }

// assembleForTest interpolates and orders sections. It is the shared
// interior of Render, InputHash, and Compile (used by in-package tests).
func (a *Assembler) assembleForTest(vars map[string]string) ([]Section, error) {
	return a.assemble(vars)
}

// assemble interpolates every section body (strictly) and returns the
// sections in render order: ascending Number, ties in registration order.
func (a *Assembler) assemble(vars map[string]string) ([]Section, error) {
	ordered := make([]Section, len(a.sections))
	copy(ordered, a.sections)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Number < ordered[j].Number })

	out := make([]Section, 0, len(ordered))
	for _, s := range ordered {
		body, err := interpolate(s, vars)
		if err != nil {
			return nil, err
		}
		s.Body = body
		out = append(out, s)
	}
	return out, nil
}

// interpolate performs strict {{variable}} substitution on one section.
// Unknown variables and unterminated placeholders are errors; there is no
// escape hatch because prompts are machine-assembled, not hand-typed.
func interpolate(s Section, vars map[string]string) (string, error) {
	var b strings.Builder
	body := s.Body
	for {
		open := strings.Index(body, "{{")
		if open < 0 {
			b.WriteString(body)
			return b.String(), nil
		}
		closeIdx := strings.Index(body[open:], "}}")
		if closeIdx < 0 {
			return "", fmt.Errorf("prompt: section %q (number %d): unterminated {{ placeholder", s.Key, s.Number)
		}
		closeIdx += open
		name := body[open+2 : closeIdx]
		if strings.TrimSpace(name) == "" {
			return "", fmt.Errorf("prompt: section %q (number %d): empty {{}} placeholder", s.Key, s.Number)
		}
		val, ok := vars[name]
		if !ok {
			return "", fmt.Errorf("prompt: section %q (number %d): unknown variable %q (strict interpolation)", s.Key, s.Number, name)
		}
		b.WriteString(body[:open])
		b.WriteString(val)
		body = body[closeIdx+2:]
	}
}

// Render assembles the final deterministic bytes for serving: sections in
// order, each emitted as a provenance header followed by the interpolated
// body. Sections whose interpolated body is blank are skipped entirely.
func (a *Assembler) Render(vars map[string]string) ([]byte, error) {
	sections, err := a.assemble(vars)
	if err != nil {
		return nil, err
	}
	return RenderSections(sections), nil
}

// RenderSections renders already-interpolated sections to deterministic
// bytes. It is shared by Render (raw assembly) and optimizer fakes that
// re-render a subset of sections, so both paths emit the same block shape:
//
//	# section <Number> | key=<Key> | owner=<Owner>\n
//	<body>\n\n
func RenderSections(sections []Section) []byte {
	var buf bytes.Buffer
	for _, s := range sections {
		body := strings.TrimRight(s.Body, " \t\r\n")
		if strings.TrimSpace(body) == "" {
			continue
		}
		owner := s.Owner
		if owner == "" {
			owner = "unknown"
		}
		fmt.Fprintf(&buf, "# section %d | key=%s | owner=%s\n%s\n\n", s.Number, s.Key, owner, body)
	}
	return buf.Bytes()
}
