// optimize.go defines the optimizer seam: an injected, LLM-call-shaped
// function that turns assembled sections into optimized prompt bytes. The
// real LLM-backed optimizer (via internal/adapters) is future wiring that
// will satisfy this same signature; tests use deterministic fakes only —
// no network, stdlib only.

package prompt

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ToolSummary is one entry of the tool catalog summary handed to the
// optimizer so it knows which tools exist and what they do.
type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ToolCatalog is the sorted, validated set of registered tools.
type ToolCatalog struct {
	Tools []ToolSummary `json:"tools"`
}

// toolNamePattern accepts names whose first and last characters are word
// characters, so \b-anchored reference scans are well-defined.
var toolNamePattern = regexp.MustCompile(`^\w[\w.\-]*\w$|^\w$`)

// NewToolCatalog validates tool names (non-empty, word-bounded, unique)
// and returns the catalog sorted by name.
func NewToolCatalog(tools []ToolSummary) (ToolCatalog, error) {
	seen := make(map[string]struct{}, len(tools))
	out := make([]ToolSummary, 0, len(tools))
	for _, ts := range tools {
		if !toolNamePattern.MatchString(ts.Name) {
			return ToolCatalog{}, fmt.Errorf("prompt: invalid tool name %q", ts.Name)
		}
		if _, dup := seen[ts.Name]; dup {
			return ToolCatalog{}, fmt.Errorf("prompt: duplicate tool name %q", ts.Name)
		}
		seen[ts.Name] = struct{}{}
		out = append(out, ts)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return ToolCatalog{Tools: out}, nil
}

// Names returns the sorted tool names.
func (c ToolCatalog) Names() []string {
	names := make([]string, 0, len(c.Tools))
	for _, ts := range c.Tools {
		names = append(names, ts.Name)
	}
	return names
}

// InvariantsContract is the compile-time mechanical contract the optimized
// output must satisfy. Violations fail compilation — they are never warnings.
type InvariantsContract struct {
	// DelegatedTools is the explicit allowlist of registered tools that
	// may be absent from the optimized output (e.g. documented elsewhere
	// in the deployment). Absence outside this list fails compilation.
	DelegatedTools []string `json:"delegated_tools,omitempty"`
	// MaxGrowthRatio caps output tokens as a multiple of input tokens.
	// Zero means the default of 1.0 (no growth). Ignored when
	// AllowGrowth is true.
	MaxGrowthRatio float64 `json:"max_growth_ratio,omitempty"`
	// AllowGrowth explicitly permits the optimizer to grow the prompt.
	AllowGrowth bool `json:"allow_growth"`
}

func (c InvariantsContract) effectiveRatio() float64 {
	if c.MaxGrowthRatio <= 0 {
		return 1.0
	}
	return c.MaxGrowthRatio
}

// OptimizeInput is everything an optimizer is allowed to see: the
// interpolated sections in render order, the tool catalog summary, and the
// invariants contract it must satisfy.
type OptimizeInput struct {
	Sections   []Section          `json:"sections"`
	Catalog    ToolCatalog        `json:"catalog"`
	Invariants InvariantsContract `json:"invariants"`
}

// SectionAction classifies what the optimizer did with an input section.
type SectionAction string

const (
	ActionPreserved SectionAction = "preserved"
	ActionMerged    SectionAction = "merged"
	ActionDropped   SectionAction = "dropped"
	ActionRewritten SectionAction = "rewritten"
)

// SectionOutcome is the optimizer's per-section report. Every input
// section must be reported exactly once (exact cover, no ghosts).
type SectionOutcome struct {
	Key       string        `json:"key"`
	Action    SectionAction `json:"action"`
	Rationale string        `json:"rationale,omitempty"`
}

// OptimizedPrompt is the optimizer result. Version authority lives in
// VersionedOptimizer (the caller-side wrapper), never here — a fake must
// not be able to misreport its version into the artifact.
type OptimizedPrompt struct {
	// Bytes is the optimized prompt.
	Bytes []byte
	// SectionOutcomes reports the disposition of every input section.
	SectionOutcomes []SectionOutcome
	// ReferencedTools declares tool names the output claims to mention.
	// Every entry must exist in the catalog (or the delegation allowlist);
	// invented names fail compilation.
	ReferencedTools []string
	// Notes are free-form optimizer annotations recorded in the artifact.
	Notes []string
}

// Optimizer is the seam: an LLM-call-shaped injected function. The real
// implementation (adapter-backed, offline, human-triggered) is future
// wiring; the request path never invokes it.
type Optimizer func(ctx context.Context, in OptimizeInput) (OptimizedPrompt, error)

// VersionedOptimizer binds an optimizer to its version string. The version
// participates in the artifact content hash, so an optimizer change
// invalidates every previously compiled artifact.
type VersionedOptimizer struct {
	Version string
	Fn      Optimizer
}

// DedupOptimizerVersion is the version of the reference fake below.
const DedupOptimizerVersion = "dedup-fake/1"

// Dedup is the reference optimizer: a deterministic, mechanical
// dedup/merge fake that makes the whole compile pipeline testable
// end-to-end without any network or model. Sections whose normalized
// bodies are exact duplicates of an earlier section are merged into it.
var Dedup = VersionedOptimizer{
	Version: DedupOptimizerVersion,
	Fn:      DedupOptimizerFn,
}

// DedupOptimizerFn merges sections with byte-identical bodies (after
// whitespace normalization), keeping the earliest section and marking the
// later ones ActionMerged with a mandatory rationale. It also declares the
// tool references actually present in the kept output, so compile-time
// invariants observe post-dedup reality.
func DedupOptimizerFn(_ context.Context, in OptimizeInput) (OptimizedPrompt, error) {
	type anchor struct {
		index int
		body  string
	}
	seen := make(map[string]anchor, len(in.Sections))
	kept := make([]Section, 0, len(in.Sections))
	outcomes := make([]SectionOutcome, 0, len(in.Sections))
	var notes []string

	for i, s := range in.Sections {
		key := strings.Join(strings.Fields(s.Body), " ")
		if prev, dup := seen[key]; dup && key != "" {
			outcomes = append(outcomes, SectionOutcome{
				Key:    s.Key,
				Action: ActionMerged,
				Rationale: fmt.Sprintf(
					"body byte-identical (whitespace-normalized) to section %q (number %d); merged to keep exactly one copy",
					in.Sections[prev.index].Key, in.Sections[prev.index].Number),
			})
			notes = append(notes, fmt.Sprintf("merged %s into %s", s.Key, in.Sections[prev.index].Key))
			continue
		}
		seen[key] = anchor{index: i, body: s.Body}
		kept = append(kept, s)
		outcomes = append(outcomes, SectionOutcome{Key: s.Key, Action: ActionPreserved})
	}

	out := RenderSections(kept)
	return OptimizedPrompt{
		Bytes:           out,
		SectionOutcomes: outcomes,
		ReferencedTools: referencedToolNames(out, in.Catalog),
		Notes:           notes,
	}, nil
}

// referencedToolNames word-scans bytes for catalog tool names. Substring
// hits do not count ("already" does not reference a tool named "read").
func referencedToolNames(b []byte, catalog ToolCatalog) []string {
	var refs []string
	for _, name := range catalog.Names() {
		if wordContains(b, name) {
			refs = append(refs, name)
		}
	}
	return refs
}

// wordContains reports whether name occurs in b as a whole word.
func wordContains(b []byte, name string) bool {
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(name) + `\b`)
	if err != nil {
		return false
	}
	return re.Match(b)
}
