// config.go — vh-agentd command-line surface: flag parsing, fail-closed
// validation, and the credential rule (API key read from the environment
// variable NAMED by --api-key-env — never a literal flag value).
package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// engineVersion is the daemon/engine build identity reported by
// --version alongside the wire ProtocolVersion.
const engineVersion = "0.1.0"

// defaultApprovalTimeoutMs bounds each pending approval by default (dsh
// F-PIPE-2 fail-closed posture: an unanswerable approval denies).
const defaultApprovalTimeoutMs = 30000

// Config is the validated daemon configuration.
type Config struct {
	// Adapter is the normalized provider name ("openaicompat" |
	// "anthropic"); the flags "openai" and "openaicompat" are aliases.
	Adapter string
	Model   string
	BaseURL string
	// APIKeyEnv names the environment variable holding the API key.
	APIKeyEnv string
	// SessionDir is the explicit directory for durable session logs.
	// There is deliberately no default: the daemon refuses to start
	// without it (no silent writes). It is also the home of the
	// compiled-prompt artifact cache (SessionDir/compiled-prompts/) and
	// the scheduler state file (SessionDir/scheduler-state.json), so
	// every durable byte the daemon writes lives under one root.
	SessionDir string
	// MaxTokens overrides the adapter max_tokens default (anthropic
	// requires one; 0 takes the adapter defaults).
	MaxTokens int
	// ApprovalTimeoutMs bounds each pending approval.
	ApprovalTimeoutMs int
	// CacheBreakpoints maps --cache-breakpoints: 0 = caching off
	// (default); 1..4 = explicit Anthropic prompt-cache breakpoint
	// budget, wired into the anthropic adapter's CacheConfig. It is
	// REJECTED for openaicompat (see validateCacheBreakpoints).
	CacheBreakpoints int
}

// usageDoc documents credential handling on the help surface (the key is
// read from the environment only, held in adapter memory, sent only to
// the configured base URL, and never persisted in session logs).
const usageDoc = `vh-agentd — headless agent daemon (host protocol v1 over stdio)

Speaks versioned JSON-RPC over NDJSON on stdin/stdout (stdout is
protocol; diagnostics go to stderr). See docs/native-engine/host-protocol.md.

Credentials:
  The API key is read at startup from the environment variable NAMED by
  --api-key-env (there is intentionally no literal --api-key flag). It is
  held in adapter memory only, sent to the configured --base-url, and
  never written to session logs or diagnostics (refs-not-stored).
  Exception: --compile-prompt runs OFFLINE and never touches the network
  or the key — --api-key-env must still NAME a variable, but the
  variable does not need to be set for a compile run.

Prompt cache (--cache-breakpoints N):
  Anthropic only. 0 (default) keeps explicit caching off; 1-4 sets the
  cache_control breakpoint budget (breakpoint 1: final tool definition,
  breakpoint 2: the system prompt; the adapter documents placement).
  Rejected for --adapter openai: OpenAI-compatible endpoints cache
  IMPLICITLY via prefix matching — there is no breakpoint knob to map.

System prompt (compiled-sysprompt model):
  The daemon serves its system prompt from the compiled artifact under
  <session-dir>/compiled-prompts/ when one matches the current content
  hash, and falls back to raw assembly otherwise (the fallback is logged,
  never silent). Populate the artifact offline with --compile-prompt.

Usage:
  vh-agentd --adapter openai|anthropic --model M --base-url URL
            --api-key-env VAR --session-dir DIR [--max-tokens N]
            [--approval-timeout-ms MS] [--cache-breakpoints N]
            [--compile-prompt] [--version]

  --compile-prompt  run the offline prompt compilation with the current
                    config (reference Dedup optimizer; the real
                    adapter-backed optimizer stays a seam), write the
                    content-hashed artifact under
                    <session-dir>/compiled-prompts/, report to stderr,
                    and exit. No network, no key, no protocol session.
`

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// normalizeAdapter maps flag aliases onto the registered adapter names.
func normalizeAdapter(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "openai", "openaicompat":
		return "openaicompat", true
	case "anthropic":
		return "anthropic", true
	default:
		return "", false
	}
}

// validateCacheBreakpoints checks the --cache-breakpoints value against
// the adapter selection: 0..4 in range; >0 only with anthropic (explicit
// cache_control breakpoints). The openai rejection is a CLEAR error, not
// a silent ignore — OpenAI-compatible caching is implicit, so a
// breakpoint budget on that adapter would be a no-op the operator
// believes is armed.
func validateCacheBreakpoints(adapter string, n int) error {
	if n == 0 {
		return nil
	}
	if n < 0 || n > 4 {
		return fmt.Errorf("invalid --cache-breakpoints %d: must be 0 (off) or 1..4", n)
	}
	if adapter != "anthropic" {
		return fmt.Errorf("--cache-breakpoints %d is not supported with --adapter %s: explicit cache_control breakpoints are Anthropic-only (OpenAI-compatible endpoints cache implicitly via prefix matching; there is no breakpoint knob)", n, adapter)
	}
	return nil
}

// validate checks the parsed flags fail-closed and returns the
// normalized Config.
func validate(adapter, model, baseURL, apiKeyEnv, sessionDir string, maxTokens, approvalTimeoutMs, cacheBreakpoints int) (*Config, error) {
	ad, ok := normalizeAdapter(adapter)
	if !ok {
		return nil, fmt.Errorf("invalid --adapter %q: must be openai (openaicompat) or anthropic", adapter)
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("--model is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("--base-url is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid --base-url %q: must be an absolute http(s) URL", baseURL)
	}
	if apiKeyEnv == "" {
		return nil, fmt.Errorf("--api-key-env is required (the API key is read from the environment only)")
	}
	if !envNameRe.MatchString(apiKeyEnv) {
		return nil, fmt.Errorf("invalid --api-key-env %q: not a valid environment variable name", apiKeyEnv)
	}
	if sessionDir == "" {
		return nil, fmt.Errorf("--session-dir is required (there is no default: the daemon refuses silent session writes)")
	}
	if maxTokens < 0 {
		return nil, fmt.Errorf("invalid --max-tokens %d: must be >= 0", maxTokens)
	}
	if approvalTimeoutMs < 0 {
		return nil, fmt.Errorf("invalid --approval-timeout-ms %d: must be >= 0", approvalTimeoutMs)
	}
	if err := validateCacheBreakpoints(ad, cacheBreakpoints); err != nil {
		return nil, err
	}
	return &Config{
		Adapter:           ad,
		Model:             model,
		BaseURL:           baseURL,
		APIKeyEnv:         apiKeyEnv,
		SessionDir:        sessionDir,
		MaxTokens:         maxTokens,
		ApprovalTimeoutMs: approvalTimeoutMs,
		CacheBreakpoints:  cacheBreakpoints,
	}, nil
}
