package permconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/jsonc"
)

// TransformContext is the JSON shape passed to the permission transform
// (config-transform.mjs) as { context: <TransformContext> }. It contains NO
// ambient environment, NO secrets, NO file paths, and NO process state — only
// the active pack names, resolved feature values, and the rendered agent roster.
// This is the strict input contract documented in the transform file header.
type TransformContext struct {
	Packs    []string          `json:"packs"`    // active overlay pack names (filename stems)
	Features map[string]string `json:"features"` // resolved feature values (e.g. {"backlog":"true"})
	Agents   []string          `json:"agents"`   // rendered agent names (core + active-pack)
}

// TransformInput is the top-level argument passed to the transform function:
// { context: {...} }. The transform MUST NOT receive any other top-level key.
type TransformInput struct {
	Context TransformContext `json:"context"`
}

// transformBashEntry is one {pattern, decision} pair in the JSON output.
type transformBashEntry struct {
	Pattern  string `json:"pattern"`
	Decision string `json:"decision"`
}

// transformPatch is one {agent, bash: [...]} entry in the JSON output.
type transformPatch struct {
	Agent string               `json:"agent"`
	Bash  []transformBashEntry `json:"bash"`
}

// transformResult is the expected top-level JSON output shape.
type transformResult struct {
	PermissionPatches []transformPatch `json:"permissionPatches"`
}

// ValidateTransformOutput validates the JSON output produced by the permission
// transform (config-transform.mjs) against the strict output contract. This is
// the authoritative build-time gate: the seam calls it immediately after
// running the transform via Node, and doctor inherits it through
// renderSeamStaging (which runs the same pipeline). Any validation failure is a
// LOUD error — the transform never silently produces an invalid config.
//
// Checks (all fail-closed):
//   - Output is valid JSON decoding to { permissionPatches: [{agent, bash:[{pattern, decision}]}] }.
//   - permissionPatches may be empty/absent (no-op transform).
//   - Each agent must be present in the rendered roster.
//   - Each decision must be "allow", "deny", or "ask".
//   - Each pattern must be non-empty.
//   - No duplicate patterns within one agent's bash set.
//   - No protected-key collision ("*", command-group commands, backlog, "vh-agent-harness *").
//
// On success, the validated entries are returned as typed BashEntry slices keyed
// by agent name, ready for EmitWithExtra.
func ValidateTransformOutput(raw []byte, roster []string) (map[string][]BashEntry, error) {
	var result transformResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("transform output is not valid JSON: %w", err)
	}

	rosterSet := make(map[string]bool, len(roster))
	for _, a := range roster {
		rosterSet[a] = true
	}
	protected := protectedBashKeys()

	extra := make(map[string][]BashEntry)
	for i, patch := range result.PermissionPatches {
		if patch.Agent == "" {
			return nil, fmt.Errorf("permissionPatches[%d]: agent is empty", i)
		}
		if !rosterSet[patch.Agent] {
			return nil, fmt.Errorf("permissionPatches[%d]: agent %q is not in the rendered roster", i, patch.Agent)
		}
		seen := make(map[string]bool, len(patch.Bash))
		var entries []BashEntry
		for j, e := range patch.Bash {
			if e.Pattern == "" {
				return nil, fmt.Errorf("permissionPatches[%d].bash[%d]: pattern is empty", i, j)
			}
			dec := Decision(e.Decision)
			if !validDecision(dec) {
				return nil, fmt.Errorf("permissionPatches[%d].bash[%d]: pattern %q decision %q is not allow/deny/ask", i, j, e.Pattern, e.Decision)
			}
			if seen[e.Pattern] {
				return nil, fmt.Errorf("permissionPatches[%d].bash[%d]: duplicate pattern %q for agent %q", i, j, e.Pattern, patch.Agent)
			}
			if protected[e.Pattern] {
				return nil, fmt.Errorf("permissionPatches[%d].bash[%d]: pattern %q collides with a protected key (wildcard, command group, backlog, or vh-agent-harness entry)", i, j, e.Pattern)
			}
			seen[e.Pattern] = true
			entries = append(entries, BashEntry{Pattern: e.Pattern, Decision: dec})
		}
		// Merge with any existing patches for the same agent (multiple patches
		// targeting the same agent should not happen in well-formed output, but
		// we handle it by extending the entry set and re-checking for dups).
		if existing, ok := extra[patch.Agent]; ok {
			for _, ne := range entries {
				for _, ee := range existing {
					if ne.Pattern == ee.Pattern {
						return nil, fmt.Errorf("agent %q: duplicate pattern %q across multiple patches", patch.Agent, ne.Pattern)
					}
				}
			}
			extra[patch.Agent] = append(existing, entries...)
		} else {
			extra[patch.Agent] = entries
		}
	}
	return extra, nil
}

// BuildTransformContext constructs the TransformContext from the rendered config
// and active packs. The seam calls this before invoking the transform via Node.
func BuildTransformContext(agents map[string]any, packNames []string, features map[string]string) TransformInput {
	agentList := make([]string, 0, len(agents))
	for name := range agents {
		agentList = append(agentList, name)
	}
	// Sort for determinism (the transform output must be deterministic, and
	// feeding a stable agent list helps achieve that).
	sortStrings(agentList)
	return TransformInput{
		Context: TransformContext{
			Packs:    packNames,
			Features: features,
			Agents:   agentList,
		},
	}
}

// ExtractRoster parses a rendered opencode.jsonc and returns the sorted list of
// agent names present. The seam calls this to build the roster for
// ValidateTransformOutput and to construct the transform context.
func ExtractRoster(data []byte) ([]string, error) {
	root, err := jsonc.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse opencode.jsonc for roster: %w", err)
	}
	agents, _ := root["agent"].(map[string]any)
	roster := make([]string, 0, len(agents))
	for name := range agents {
		roster = append(roster, name)
	}
	sortStrings(roster)
	return roster, nil
}

// BuildTransformInputFromConfig parses a rendered opencode.jsonc, extracts the
// agent roster, and builds the TransformInput the seam passes to the Node
// runner. Convenience wrapper: parse once, build context.
func BuildTransformInputFromConfig(data []byte, packNames []string, features map[string]string) (TransformInput, error) {
	root, err := jsonc.Parse(data)
	if err != nil {
		return TransformInput{}, fmt.Errorf("parse opencode.jsonc for transform context: %w", err)
	}
	agents, _ := root["agent"].(map[string]any)
	return BuildTransformContext(agents, packNames, features), nil
}

// MarshalTransformInput serializes the transform input to JSON for passing to
// the Node runner.
func MarshalTransformInput(input TransformInput) ([]byte, error) {
	return json.Marshal(input)
}

// TransformContextForTests is a test helper that builds and marshals context.
func TransformContextForTests(agents, packs []string, features map[string]string) []byte {
	agentMap := make(map[string]any, len(agents))
	for _, a := range agents {
		agentMap[a] = true
	}
	input := BuildTransformContext(agentMap, packs, features)
	out, _ := MarshalTransformInput(input)
	return out
}

// sortStrings is a local helper to avoid importing sort in this file when the
// rest of the package already imports it via emit.go.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// hostAPIForbiddenTokens are substrings that, if found in the transform SOURCE,
// indicate host-API access. This lint is ADVISORY DEFENSE-IN-DEPTH, NOT a
// security boundary. It catches obvious host-API usage but can be evaded via
// string concatenation, dynamic imports, etc. The REAL security boundary is Go
// validation of the typed output (ValidateTransformOutput) which runs AFTER the
// transform returns. The transform is trusted project-owned code (same trust
// model as forbidden-patterns.project.js). This lint is intentionally
// conservative: false negatives (missing a creative bypass) are possible, false
// positives are minimized by choosing high-signal tokens.
var hostAPIForbiddenTokens = []string{
	"process.env",
	"process.argv",
	"require(",
	"child_process",
	"net.Socket",
	"net.connect",
	"http.request",
	"https.request",
	"fetch(",
	"XMLHttpRequest",
	"fs.readFile",
	"fs.writeFile",
	"fs.readdir",
	"readFileSync",
	"writeFileSync",
	"execSync",
	"spawnSync",
	"math.random", // lowercased to catch Math.random and math.random
	"date.now",    // lowercased to catch Date.now and date.now — nondeterministic clock access
}

// LintTransformSource scans the transform source for host-API tokens. This is
// advisory defense-in-depth, NOT a security boundary — it catches obvious
// host-API usage but can be evaded via string concatenation, dynamic imports,
// etc. The real security boundary is Go validation of the typed output
// (ValidateTransformOutput) which runs AFTER the transform returns. The
// transform is trusted project-owned code (same trust model as
// forbidden-patterns.project.js). Returns a non-nil error listing the offending
// tokens if any are found.
//
// JS comments are stripped before scanning so documentation that REFERENCES a
// host API (e.g. "do not use process.env") does not produce a false positive.
// String literals are NOT stripped — hiding a call inside a string and eval'ing
// it is adversarial and out of scope for this advisory lint.
func LintTransformSource(source []byte) error {
	code := stripJSComments(string(source))
	lowered := strings.ToLower(code)
	var found []string
	for _, token := range hostAPIForbiddenTokens {
		if strings.Contains(lowered, strings.ToLower(token)) {
			found = append(found, token)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("transform source contains host-API tokens: %s — the transform should not use host APIs (advisory lint; the real gate is Go output validation)", strings.Join(found, ", "))
	}
	return nil
}

// stripJSComments removes // line comments and /* */ block comments from JS
// source. It is string-aware so comment markers inside string literals are
// preserved (a URL like "http://..." does not trigger the // handler). This is a
// local copy of the pattern in internal/schema/forbidden_patterns.go — kept
// local to avoid a cross-package dependency for a build-time lint utility.
func stripJSComments(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	n := len(s)
	inStr := false
	var quote byte
	for i < n {
		c := s[i]
		if inStr {
			b.WriteByte(c)
			if c == '\\' && i+1 < n {
				b.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == quote {
				inStr = false
			}
			i++
			continue
		}
		switch {
		case c == '"' || c == '\'' || c == '`':
			inStr = true
			quote = c
			b.WriteByte(c)
			i++
		case c == '/' && i+1 < n && s[i+1] == '/':
			for i < n && s[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && s[i+1] == '*':
			i += 2
			for i+1 < n && !(s[i] == '*' && s[i+1] == '/') {
				i++
			}
			i += 2
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}
