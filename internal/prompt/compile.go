// compile.go implements the offline compilation step and the request-path
// serving rule of the "compiled sys prompt" model:
//
//   - Compile runs the optimizer, checks every mechanical invariant
//     (violations FAIL compilation, never warn), and writes a
//     content-hashed JSON artifact.
//   - LoadCompiled/ServeCompiled only read. A missing or corrupt artifact
//     falls back to raw assembly — explicitly, never silently, and never
//     by re-running optimizer logic on the request path.

package prompt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Artifact schema version. A shape change bumps this and invalidates all
// previously compiled artifacts.
const artifactSchema = 1

// Sentinel errors for LoadCompiled/ServeCompiled callers.
var (
	ErrArtifactNotFound = errors.New("prompt: compiled artifact not found")
	ErrArtifactCorrupt  = errors.New("prompt: compiled artifact corrupt")
)

// ServeSource classifies what ServeCompiled served.
type ServeSource string

const (
	ServeSourceCompiled    ServeSource = "compiled"
	ServeSourceRawAssembly ServeSource = "raw-assembly"
)

// ServeResult reports what ServeCompiled served and, on fallback, why —
// a fallback is never silent.
type ServeResult struct {
	Source ServeSource
	Reason string
}

// Fallback reasons — a fallback is never silent.
const (
	ServeReasonNotFound = "artifact-not-found"
	ServeReasonCorrupt  = "artifact-corrupt"
)

// TokenAccounting records the chars/4 token estimate before and after
// optimization (same heuristic as internal/session compaction budgeting).
type TokenAccounting struct {
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	DeltaTokens  int    `json:"delta_tokens"`
	Estimate     string `json:"estimate"`
}

// SectionRecord is one row of the per-section contribution table — the
// leak detector: sections the optimizer merged or dropped are surfaced
// here for source-level deletion.
type SectionRecord struct {
	Number      int    `json:"number"`
	Key         string `json:"key"`
	Owner       string `json:"owner"`
	Required    bool   `json:"required"`
	CacheStable bool   `json:"cache_stable"`
	InputTokens int    `json:"input_tokens"`
	Action      string `json:"action"`
	Rationale   string `json:"rationale,omitempty"`
}

// InvariantResult records one mechanical invariant check executed at
// compile time.
type InvariantResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Artifact is the compiled sys-prompt artifact persisted as JSON. It is
// fully deterministic: no timestamps, no environment data — identical
// inputs compile to identical file bytes.
type Artifact struct {
	Schema           int               `json:"schema"`
	Hash             string            `json:"hash"`
	BytesSHA256      string            `json:"bytes_sha256"`
	OptimizerVersion string            `json:"optimizer_version"`
	Bytes            []byte            `json:"bytes"`
	Tokens           TokenAccounting   `json:"tokens"`
	Invariants       []InvariantResult `json:"invariants"`
	Sections         []SectionRecord   `json:"sections"`
	DelegatedTools   []string          `json:"delegated_tools,omitempty"`
	MaxGrowthRatio   float64           `json:"max_growth_ratio"`
	AllowGrowth      bool              `json:"allow_growth"`
	Notes            []string          `json:"notes,omitempty"`
}

// InvariantError reports fail-closed invariant violations. No artifact is
// written when it is returned.
type InvariantError struct {
	Violations []string
}

func (e *InvariantError) Error() string {
	return fmt.Sprintf("prompt: compile invariants failed: %s", strings.Join(e.Violations, "; "))
}

// EstimateTokens applies the chars/4 heuristic used consistently with the
// session package's context budgeting (internal/session/compaction.go).
func EstimateTokens(b []byte) int { return len(b) / 4 }

// canonicalInput is the fixed-field-order hashing input. No maps, no
// environment, no timestamps: the hash covers exactly the section set
// content (post-interpolation), the tool catalog, the invariants contract,
// and the optimizer version.
type canonicalInput struct {
	Sections         []Section   `json:"sections"`
	Catalog          ToolCatalog `json:"catalog"`
	DelegatedTools   []string    `json:"delegated_tools"`
	MaxGrowthRatio   float64     `json:"max_growth_ratio"`
	AllowGrowth      bool        `json:"allow_growth"`
	OptimizerVersion string      `json:"optimizer_version"`
}

// InputHash derives the content hash for an (assembler, vars, catalog,
// optimizer version, contract) combination. It interpolates strictly, so
// unknown variables surface here too.
func InputHash(asm *Assembler, vars map[string]string, catalog ToolCatalog, optimizerVersion string, contract InvariantsContract) (string, error) {
	if optimizerVersion == "" {
		return "", errors.New("prompt: optimizer version must be non-empty (unversioned artifacts are unauditable)")
	}
	sections, err := asm.assemble(vars)
	if err != nil {
		return "", err
	}
	return hashCanonical(canonicalInput{
		Sections:         sections,
		Catalog:          catalog,
		DelegatedTools:   sortedCopy(contract.DelegatedTools),
		MaxGrowthRatio:   contract.effectiveRatio(),
		AllowGrowth:      contract.AllowGrowth,
		OptimizerVersion: optimizerVersion,
	}), nil
}

func hashCanonical(in canonicalInput) string {
	blob, err := json.Marshal(in)
	if err != nil {
		// canonicalInput contains only marshalable fields; panic-free fallback.
		blob = []byte(fmt.Sprintf("%#v", in))
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func sortedCopy(ss []string) []string {
	out := append([]string(nil), ss...)
	sort.Strings(out)
	return out
}

// Compile runs the explicit offline optimization step: assemble strictly,
// derive the content hash, reuse an existing valid artifact for that hash
// (no optimizer call), otherwise optimize, check every mechanical
// invariant, and atomically write the artifact. Invariant violations
// return *InvariantError and write nothing.
func Compile(ctx context.Context, asm *Assembler, vars map[string]string, catalog ToolCatalog, opt VersionedOptimizer, contract InvariantsContract, dir string) (*Artifact, error) {
	if opt.Version == "" {
		return nil, errors.New("prompt: optimizer version must be non-empty (unversioned artifacts are unauditable)")
	}
	sections, err := asm.assemble(vars)
	if err != nil {
		return nil, err
	}
	hash := hashCanonical(canonicalInput{
		Sections:         sections,
		Catalog:          catalog,
		DelegatedTools:   sortedCopy(contract.DelegatedTools),
		MaxGrowthRatio:   contract.effectiveRatio(),
		AllowGrowth:      contract.AllowGrowth,
		OptimizerVersion: opt.Version,
	})

	// Compile cache: a valid artifact for this hash short-circuits the
	// optimizer entirely. A corrupt one is recompiled and replaced.
	if existing, err := LoadCompiled(dir, hash); err == nil {
		return existing, nil
	}

	raw := RenderSections(sections)
	inputTokens := EstimateTokens(raw)

	out, err := opt.Fn(ctx, OptimizeInput{
		Sections:   sections,
		Catalog:    catalog,
		Invariants: contract,
	})
	if err != nil {
		return nil, fmt.Errorf("prompt: optimizer %q failed: %w", opt.Version, err)
	}

	results, viol := checkInvariants(sections, catalog, contract, raw, out)
	if len(viol) > 0 {
		return nil, &InvariantError{Violations: viol}
	}

	outputTokens := EstimateTokens(out.Bytes)
	artifact := &Artifact{
		Schema:           artifactSchema,
		Hash:             hash,
		BytesSHA256:      sha256hex(out.Bytes),
		OptimizerVersion: opt.Version,
		Bytes:            out.Bytes,
		Tokens: TokenAccounting{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			DeltaTokens:  inputTokens - outputTokens,
			Estimate:     "chars/4",
		},
		Invariants:     results,
		DelegatedTools: sortedCopy(contract.DelegatedTools),
		MaxGrowthRatio: contract.effectiveRatio(),
		AllowGrowth:    contract.AllowGrowth,
		Notes:          out.Notes,
	}
	outcomeByKey := make(map[string]SectionOutcome, len(out.SectionOutcomes))
	for _, o := range out.SectionOutcomes {
		outcomeByKey[o.Key] = o
	}
	for _, s := range sections {
		rec := SectionRecord{
			Number:      s.Number,
			Key:         s.Key,
			Owner:       s.Owner,
			Required:    s.Required,
			CacheStable: s.CacheStable,
			InputTokens: EstimateTokens(sectionBlock(s)),
			Action:      string(ActionPreserved),
		}
		if o, ok := outcomeByKey[s.Key]; ok {
			rec.Action = string(o.Action)
			rec.Rationale = o.Rationale
		}
		artifact.Sections = append(artifact.Sections, rec)
	}

	if err := writeArtifact(dir, hash, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

// sectionBlock renders one section's block for per-section accounting.
func sectionBlock(s Section) []byte {
	return RenderSections([]Section{s})
}

// checkInvariants enforces every mechanical invariant. Each violation is
// collected (name + detail) so one run reports all failures.
func checkInvariants(sections []Section, catalog ToolCatalog, contract InvariantsContract, raw []byte, out OptimizedPrompt) ([]InvariantResult, []string) {
	var results []InvariantResult
	var viol []string

	passWith := func(name, detail string) {
		results = append(results, InvariantResult{Name: name, Passed: true, Detail: detail})
	}
	failWith := func(name, detail string) {
		results = append(results, InvariantResult{Name: name, Passed: false, Detail: detail})
		viol = append(viol, fmt.Sprintf("%s: %s", name, detail))
	}

	// 1. Every registered tool is referenced (word match) or delegated.
	delegated := make(map[string]bool, len(contract.DelegatedTools))
	for _, d := range contract.DelegatedTools {
		delegated[d] = true
	}
	var missing []string
	referencedCount := 0
	for _, name := range catalog.Names() {
		if wordContains(out.Bytes, name) {
			referencedCount++
			continue
		}
		if delegated[name] {
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) > 0 {
		failWith("tools-referenced-or-delegated",
			fmt.Sprintf("tools absent from output and not delegated: %v", missing))
	} else {
		passWith("tools-referenced-or-delegated",
			fmt.Sprintf("%d/%d tools referenced in output; %d delegated",
				referencedCount, len(catalog.Names()), len(delegatedCheck(catalog, delegated))))
	}

	// 2. No invented tool names: every declared reference must be real.
	known := make(map[string]bool, len(catalog.Tools)+len(delegated))
	for _, ts := range catalog.Tools {
		known[ts.Name] = true
	}
	for d := range delegated {
		known[d] = true
	}
	var invented []string
	for _, r := range out.ReferencedTools {
		if !known[r] {
			invented = append(invented, r)
		}
	}
	if len(invented) > 0 {
		failWith("no-invented-tool-references",
			fmt.Sprintf("referenced tools not in catalog or delegation allowlist: %v", invented))
	} else {
		passWith("no-invented-tool-references", fmt.Sprintf("%d declared references all resolve", len(out.ReferencedTools)))
	}

	// 3. Section report is an exact cover of the input sections.
	inputKeys := make(map[string]Section, len(sections))
	for _, s := range sections {
		inputKeys[s.Key] = s
	}
	reported := make(map[string]bool, len(out.SectionOutcomes))
	var cover []string
	for _, o := range out.SectionOutcomes {
		if _, ok := inputKeys[o.Key]; !ok {
			cover = append(cover, fmt.Sprintf("unknown reported key %q", o.Key))
		}
		if reported[o.Key] {
			cover = append(cover, fmt.Sprintf("duplicate report for key %q", o.Key))
		}
		reported[o.Key] = true
	}
	for _, s := range sections {
		if !reported[s.Key] {
			cover = append(cover, fmt.Sprintf("missing report for key %q", s.Key))
		}
	}
	if len(cover) > 0 {
		failWith("section-report-complete", strings.Join(cover, "; "))
	} else {
		passWith("section-report-complete", fmt.Sprintf("%d sections reported", len(sections)))
	}

	// 4. Required sections preserved or rationaled.
	var unrationaled []string
	for _, o := range out.SectionOutcomes {
		s, ok := inputKeys[o.Key]
		if !ok {
			continue // already flagged by exact cover
		}
		if s.Required && o.Action != ActionPreserved && strings.TrimSpace(o.Rationale) == "" {
			unrationaled = append(unrationaled, s.Key)
		}
	}
	if len(unrationaled) > 0 {
		failWith("required-sections-preserved-or-rationaled",
			fmt.Sprintf("required sections altered without rationale: %v", unrationaled))
	} else {
		passWith("required-sections-preserved-or-rationaled", "all required sections preserved or carry rationale")
	}

	// 5. Token ratchet (regression guard).
	inT, outT := EstimateTokens(raw), EstimateTokens(out.Bytes)
	if contract.AllowGrowth {
		passWith("token-ratchet", fmt.Sprintf("growth explicitly allowed: in=%d out=%d", inT, outT))
	} else if float64(outT) > float64(inT)*contract.effectiveRatio() {
		failWith("token-ratchet",
			fmt.Sprintf("output grew beyond ratio %.2f: in=%d out=%d", contract.effectiveRatio(), inT, outT))
	} else {
		passWith("token-ratchet", fmt.Sprintf("in=%d out=%d ratio<=%.2f", inT, outT, contract.effectiveRatio()))
	}

	// 6. Non-empty output for non-empty input.
	if len(sections) > 0 && len(out.Bytes) == 0 {
		failWith("non-empty-output", "optimizer returned empty bytes for non-empty input")
	} else {
		passWith("non-empty-output", fmt.Sprintf("%d output bytes", len(out.Bytes)))
	}

	return results, viol
}

// delegatedCheck is a tiny helper keeping the pass-detail honest.
func delegatedCheck(catalog ToolCatalog, delegated map[string]bool) []string {
	var ds []string
	for _, name := range catalog.Names() {
		if delegated[name] {
			ds = append(ds, name)
		}
	}
	return ds
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writeArtifact persists the artifact atomically (temp file + rename) as
// <dir>/prompt-<hash>.json.
func writeArtifact(dir, hash string, art *Artifact) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prompt: create artifact dir: %w", err)
	}
	blob, err := json.MarshalIndent(art, "", "  ")
	if err != nil {
		return fmt.Errorf("prompt: marshal artifact: %w", err)
	}
	final := filepath.Join(dir, "prompt-"+hash+".json")
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o644); err != nil {
		return fmt.Errorf("prompt: write artifact temp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("prompt: rename artifact into place: %w", err)
	}
	return nil
}

// LoadCompiled reads and validates the artifact for hash. It verifies the
// schema version, that the stored hash matches the request, and that the
// stored bytes still match their recorded digest — a tampered or
// truncated artifact is ErrArtifactCorrupt, a missing one
// ErrArtifactNotFound.
func LoadCompiled(dir, hash string) (*Artifact, error) {
	blob, err := os.ReadFile(filepath.Join(dir, "prompt-"+hash+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrArtifactNotFound, hash)
		}
		return nil, fmt.Errorf("prompt: read artifact: %w", err)
	}
	var art Artifact
	if err := json.Unmarshal(blob, &art); err != nil {
		return nil, fmt.Errorf("%w: unmarshal: %v", ErrArtifactCorrupt, err)
	}
	if art.Schema != artifactSchema {
		return nil, fmt.Errorf("%w: schema %d != %d", ErrArtifactCorrupt, art.Schema, artifactSchema)
	}
	if art.Hash != hash {
		return nil, fmt.Errorf("%w: stored hash %s != requested %s", ErrArtifactCorrupt, art.Hash, hash)
	}
	if art.BytesSHA256 != sha256hex(art.Bytes) {
		return nil, fmt.Errorf("%w: bytes digest mismatch", ErrArtifactCorrupt)
	}
	return &art, nil
}

// ServeCompiled is the request-path rule: serve compiled bytes for hash,
// or fall back to raw assembly when the artifact is missing or corrupt.
// It never runs optimizer logic and never recomputes on the request path;
// every fallback is explicit via ServeResult.Reason.
func ServeCompiled(dir, hash string, asm *Assembler, vars map[string]string) ([]byte, ServeResult, error) {
	art, err := LoadCompiled(dir, hash)
	if err == nil {
		return art.Bytes, ServeResult{Source: ServeSourceCompiled}, nil
	}
	var reason string
	switch {
	case errors.Is(err, ErrArtifactNotFound):
		reason = ServeReasonNotFound
	case errors.Is(err, ErrArtifactCorrupt):
		reason = ServeReasonCorrupt
	default:
		return nil, ServeResult{}, err
	}
	raw, rerr := asm.Render(vars)
	if rerr != nil {
		return nil, ServeResult{}, rerr
	}
	return raw, ServeResult{Source: ServeSourceRawAssembly, Reason: reason}, nil
}
