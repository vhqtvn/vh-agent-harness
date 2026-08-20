// prompt.go — the daemon's compiled-sysprompt wiring (slice B1):
// deterministic assembly of the daemon's system prompt from the real
// config + tool catalog, OFFLINE compilation into a content-hashed
// artifact under <session-dir>/compiled-prompts/ (--compile-prompt),
// and the SERVING rule — compiled bytes when a matching artifact
// exists, raw assembly otherwise. Serving never runs optimizer logic
// and every fallback is reported (never silent); the report goes to
// stderr because stdout is protocol.
package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/adapters"
	"github.com/vhqtvn/vh-agent-harness/internal/prompt"
)

// promptArtifactDirname is the artifact cache directory under the
// session dir. Chosen home: the session dir is ALREADY the daemon's
// explicit durable root (refused-silent-writes contract), so compiled
// prompts — durable, content-hash-addressed, session-independent —
// live beside the logs instead of inventing a second root.
const promptArtifactDirname = "compiled-prompts"

// promptArtifactDir returns <session-dir>/compiled-prompts.
func promptArtifactDir(cfg *Config) string {
	return filepath.Join(cfg.SessionDir, promptArtifactDirname)
}

// buildPromptInputs assembles the daemon's system-prompt inputs from the
// real config and tool advertisements: the section set (identity,
// operating rules, tool guidance), the interpolation variables, and the
// validated tool catalog. Everything is deterministic in the flags —
// identical flags assemble identical bytes (no clocks, no environment).
func buildPromptInputs(cfg *Config, specs []adapters.ToolSpec) (*prompt.Assembler, map[string]string, prompt.ToolCatalog, error) {
	summaries := make([]prompt.ToolSummary, 0, len(specs))
	for _, s := range specs {
		summaries = append(summaries, prompt.ToolSummary{Name: s.Name, Description: s.Description})
	}
	catalog, err := prompt.NewToolCatalog(summaries)
	if err != nil {
		return nil, nil, prompt.ToolCatalog{}, fmt.Errorf("vh-agentd: prompt catalog: %w", err)
	}

	// Tool guidance lists every advertised tool by name (the
	// tools-referenced-or-delegated compile invariant is satisfiable by
	// construction, no delegation needed).
	var tg strings.Builder
	tg.WriteString("Tools are the only side-effect surface. Call them by exact name:\n")
	for _, ts := range catalog.Tools {
		line := fmt.Sprintf("- %s", ts.Name)
		if ts.Description != "" {
			line += ": " + ts.Description
		}
		tg.WriteString(line + "\n")
	}
	tg.WriteString("run_shell output is structured JSON; check its fields instead of guessing.\n")

	asm := prompt.NewAssembler()
	sections := []prompt.Section{
		{
			Number: -100, Key: "identity", Owner: "core", Required: true, CacheStable: true,
			Body: "You are the agent runtime of {{ENGINE}} (engine {{ENGINE_VERSION}}), a headless daemon speaking host protocol v1 over stdio. You are served by model {{MODEL}} via the {{ADAPTER}} adapter.",
		},
		{
			Number: 0, Key: "operating-rules", Owner: "core", Required: true, CacheStable: true,
			Body: "Operating rules: stdout is protocol, never prose. Turns are durable and replayable — say what you did, tersely. Prefer one precise tool call over speculation; report tool failures instead of retrying blindly.",
		},
		{
			Number: 100, Key: "tool-guidance", Owner: "core", Required: true, CacheStable: true,
			Body: tg.String(),
		},
	}
	for _, s := range sections {
		if err := asm.Register(s); err != nil {
			return nil, nil, prompt.ToolCatalog{}, fmt.Errorf("vh-agentd: prompt section %s: %w", s.Key, err)
		}
	}
	vars := map[string]string{
		"ENGINE":         "vh-agentd",
		"ENGINE_VERSION": engineVersion,
		"MODEL":          cfg.Model,
		"ADAPTER":        cfg.Adapter,
	}
	return asm, vars, catalog, nil
}

// promptContract is the compile-time invariants contract. Defaults: no
// delegated tools, no growth (the Dedup fake can only shrink).
func promptContract() prompt.InvariantsContract {
	return prompt.InvariantsContract{MaxGrowthRatio: 1.0}
}

// compilePromptOffline runs the explicit offline optimization step with
// the current config: assemble, optimize via the reference Dedup fake
// (the real adapter-backed optimizer stays a seam — same signature,
// future wiring), check every mechanical invariant, and atomically write
// the content-hashed artifact. It reports to w (stderr — stdout is
// protocol) and touches no network and no credentials.
func compilePromptOffline(ctx context.Context, cfg *Config, specs []adapters.ToolSpec, w io.Writer) error {
	asm, vars, catalog, err := buildPromptInputs(cfg, specs)
	if err != nil {
		return err
	}
	art, err := prompt.Compile(ctx, asm, vars, catalog, prompt.Dedup, promptContract(), promptArtifactDir(cfg))
	if err != nil {
		return fmt.Errorf("vh-agentd: --compile-prompt: %w", err)
	}
	fmt.Fprintf(w, "compiled system prompt: hash=%s bytes=%d tokens in=%d out=%d (delta %d) optimizer=%s\n",
		art.Hash, len(art.Bytes), art.Tokens.InputTokens, art.Tokens.OutputTokens, art.Tokens.DeltaTokens, art.OptimizerVersion)
	fmt.Fprintf(w, "artifact: %s\n", filepath.Join(promptArtifactDir(cfg), "prompt-"+art.Hash+".json"))
	return nil
}

// resolveSystemPrompt applies the SERVING rule for the daemon's system
// prompt: compiled artifact bytes for the current content hash when one
// is present and valid, raw assembly otherwise. The returned ServeResult
// carries the source and, on fallback, the explicit reason (never a
// silent fallback).
func resolveSystemPrompt(cfg *Config, specs []adapters.ToolSpec) (string, prompt.ServeResult, error) {
	asm, vars, catalog, err := buildPromptInputs(cfg, specs)
	if err != nil {
		return "", prompt.ServeResult{}, err
	}
	hash, err := prompt.InputHash(asm, vars, catalog, prompt.DedupOptimizerVersion, promptContract())
	if err != nil {
		return "", prompt.ServeResult{}, fmt.Errorf("vh-agentd: prompt input hash: %w", err)
	}
	b, res, err := prompt.ServeCompiled(promptArtifactDir(cfg), hash, asm, vars)
	if err != nil {
		return "", prompt.ServeResult{}, fmt.Errorf("vh-agentd: serve system prompt: %w", err)
	}
	return string(b), res, nil
}
