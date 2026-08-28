package permconfig

// ---------------------------------------------------------------------------
// Inter-layer dead-grant detection lint (TrueAI defect 1a). A "dead grant" is
// a permission.bash entry whose configured action can NEVER take effect on any
// command the entry's pattern matches, because the shell-guard engine
// (templates/core/.opencode/plugins/shell-guard.js wrapping
// shell-guard-core.js) hard-denies those commands by THROWING from the
// tool.execute.before hook BEFORE OpenCode consults the per-agent permission
// table. Engine-over-table precedence is intentional defense-in-depth and is
// NOT changed here; this lint only surfaces grants that the precedence
// silently dead-letters, so their authors can repair them.
//
// THE ENGINE MODEL (comparison source). Reachability is derived from THIS
// package's shared model — CommandGroups (tables.go) compiled exactly the way
// shell-guard-core.js compiles ALLOWED_PATTERNS (readonly + git_readonly +
// gate + the "vh-agent-harness *" prefix branch: trimEndStar, whitespace
// token split, token-prefix match with a trailing-wildcard length rule) —
// plus the evaluate() branch structure:
//
//   - git <verb> commands: readonly verbs match the engine allowlist (allow);
//     MUTATION verbs are denied by the forbidden-pattern scan
//     (git-mutation-bypass) and are OUT OF SCOPE here; every other verb falls
//     through to the engine's git "ask" branch, which passes through to the
//     per-agent table — so a table grant for ANY git verb is table-rescuable
//     and is NEVER reported by this lint (modeling caveat b).
//   - "vh-agent-harness <anything>" self-forms hit the harness auto-allow
//     branch and are reachable, EXCEPT (i) `vh-agent-harness git <verb>…`
//     forms (hard-denied: "Git commands must be run directly") and (ii)
//     gate-wrapper forms (an exec/exec-ro payload mentioning commit-gate.sh,
//     excluding the engine's static-inspection grammar: bash -n / cmp /
//     accept-platform / diff) — modeling caveat a.
//   - everything else must intersect the engine allowlist (readonly ∪ gate
//     groups after the git/harness branches above); a pattern with NO
//     allowlisted intersection is hard-denied by the final
//     "Commands outside the read-only inspection surface" branch.
//   - RF-B (shell file-authoring) and forbidden-pattern structural denies are
//     OUT OF SCOPE (modeling caveat c): a grant whose only problem is such a
//     deny is not this lint's finding.
//
// NEVER use internal/execro/classifier.go as the comparison source: it
// deliberately under-approximates the engine surface (readonly-only, no
// vh-agent-harness self-entry, no git_readonly/gate groups) and would
// false-flag every "git diff *"-class grant.
//
// The lint is GENERIC over the three grant channels that can author a bash
// entry: core tables (via LintDeadGrants/resolveRules), overlay
// permission-pack.jsonc agents (visible through resolveRules), and
// transform-contributed ExtraBash (visible in the EMITTED opencode.jsonc that
// LintDeadGrantsInConfig parses — the emitter-side view update/doctor lint).
// ---------------------------------------------------------------------------

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/jsonc"
)

// Deny classes for dead grants. The class names the ENGINE branch that
// hard-denies every command the grant's pattern covers.
const (
	// DenyClassNonAllowlisted: the final engine deny — the command family is
	// outside the read-only inspection surface and not routed through
	// vh-agent-harness.
	DenyClassNonAllowlisted = "engine-hard-deny/non-readonly-command"
	// DenyClassHarnessGit: the "vh-agent-harness git …" deny branch.
	DenyClassHarnessGit = "engine-hard-deny/harness-git-form"
	// DenyClassGateWrapper: the gate-wrapper-through-exec deny branch.
	DenyClassGateWrapper = "engine-hard-deny/gate-wrapper-form"
)

// DeadGrantRemediation is the operator-confirmed remediation wording contract
// (2026-08-28). It names ONLY real remediation paths: remove the grant,
// downgrade the grant, or route through vh-agent-harness exec where the verb
// family is supported. It must NEVER suggest engine-allowlist addition via
// overlay — no allow-side project seam exists (ALLOWED_PATTERNS is closed
// over the generated tables from this package; only forbidden-patterns.
// project.js is project-extensible). Deny-time naming text (the plugin's
// static grant listing) follows the same discipline. The allow-side seam
// feature is tracked separately on card scoped-project-engine-allow-seam.
const DeadGrantRemediation = "remedies: (1) remove the grant, (2) downgrade the grant, or (3) route the command through `vh-agent-harness exec ...` where the verb family is supported"

// DeadGrantFinding describes one detected dead-lettered permission.bash entry.
// It mirrors the LintDeadRules Finding report shape (agent + pattern +
// configured action/value + why-dead), adapted to the inter-layer defect: the
// "shadow" concept becomes the engine deny branch that pre-empts the table.
//
// Agent is the location name the grant was emitted under ("default" for the
// top-level permission.bash block). Pattern is the grant's bash key. EntryValue
// is the configured action (allow/ask/deny). DenyClass is the machine-readable
// engine deny branch (see the DenyClass* constants). Reason is the
// human-readable explanation of the uncovered forms. SourceLine is the
// best-effort 1-based line of the grant key inside the source opencode.jsonc
// (0 when the lint ran over in-memory rules rather than config bytes).
type DeadGrantFinding struct {
	Agent      string
	Pattern    string
	EntryValue string
	DenyClass  string
	Reason     string
	SourceLine int
}

// String formats the finding as an actionable diagnostic, mirroring
// Finding.String()'s role for LintDeadRules. The wording is part of the
// operator-confirmed remediation contract: it must name the agent (STATIC
// configuration attribution — which table owns the grant), the pattern, the
// configured action, and the uncovered forms, and must never attribute the
// grant to an actively-executing agent (the hook input exposes sessionID
// only; there is deliberately no session→agent resolution here).
func (f DeadGrantFinding) String() string {
	return fmt.Sprintf("agent %q pattern %q (action %q) can never take effect — %s",
		f.Agent, f.Pattern, f.EntryValue, f.Reason)
}

// deadGrantReason returns the human-readable why-dead text for a deny class.
func deadGrantReason(class string) string {
	switch class {
	case DenyClassHarnessGit:
		return "vh-agent-harness git forms are hard-denied by shell-guard before the per-agent table is consulted (git commands must run directly, not through the harness binary)"
	case DenyClassGateWrapper:
		return "gate-wrapper invocations under the vh-agent-harness exec trust verbs are hard-denied by shell-guard before the per-agent table is consulted (commit-gate.sh runs only via direct committer invocation)"
	default:
		return "commands in this family are hard-denied by shell-guard before the per-agent table is consulted (outside the read-only inspection surface and not routed through vh-agent-harness)"
	}
}

// harnessBinary is the bare first token of DevShCommand ("vh-agent-harness").
const harnessBinary = "vh-agent-harness"

// engineAllowPattern is one compiled entry of the engine's ALLOWED_PATTERNS
// list, mirroring shell-guard-core.js: {pattern, tokens, wildcard} where
// tokens = trimEndStar(pattern).split(/\s+/).filter(Boolean) and wildcard =
// pattern ends with "*".
type engineAllowPattern struct {
	tokens   []string
	wildcard bool
}

// compileEngineAllowPattern tokenizes one command-table literal (or grant
// pattern) the way the engine compiles ALLOWED_PATTERNS: strip a trailing
// wildcard star plus its preceding whitespace, split on whitespace runs.
func compileEngineAllowPattern(pattern string) engineAllowPattern {
	trimmed := strings.TrimSpace(pattern)
	wildcard := strings.HasSuffix(trimmed, "*")
	prefix := strings.TrimSuffix(strings.TrimRight(trimmed, " \t"), "*")
	return engineAllowPattern{tokens: strings.Fields(prefix), wildcard: wildcard}
}

// engineAllowPatterns is the compiled ALLOWED_PATTERNS roster the engine
// matches non-git commands against: CommandGroups (readonly + git_readonly +
// gate) plus the "vh-agent-harness *" prefix-branch pattern. Built once at
// package init from the Go canonical tables — the shared-model comparison
// source this lint is contractually bound to.
var engineAllowPatterns = func() []engineAllowPattern {
	var out []engineAllowPattern
	for _, group := range CommandGroups {
		for _, cmd := range group.Commands {
			out = append(out, compileEngineAllowPattern(cmd))
		}
	}
	out = append(out, compileEngineAllowPattern(DevShCommand))
	return out
}()

// envVarAssignmentToken mirrors the engine's ENV_VAR_ASSIGNMENT_RE
// (^[A-Z_][A-Z_0-9]*=): the engine strips leading env-var assignments before
// the allowlist check, so `FOO=1 ls x` is judged as `ls x` and a grant
// pattern carrying such a prefix must be judged on its stripped form too.
func envVarAssignmentToken(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false // no '=' or an empty name before it
	}
	for i := 0; i < eq; i++ {
		c := tok[i]
		switch {
		case c >= 'A' && c <= 'Z', c == '_':
			// name characters, valid anywhere
		case c >= '0' && c <= '9':
			if i == 0 {
				return false // first char must be [A-Z_]
			}
		default:
			return false
		}
	}
	return true
}

// grantDeadClass reports whether EVERY command matching the grant pattern is
// hard-denied by the engine before the per-agent table is consulted (dead),
// and if so, which deny class covers it. A false dead-bool means the grant is
// reachable: some command it matches survives the engine (allow or ask), so
// the table action can still take effect — OR the only denies covering it are
// the out-of-scope forbidden-pattern/RF-B structural denies (git mutation
// verbs), which this lint deliberately does not model.
func grantDeadClass(pattern string) (string, bool) {
	g := compileEngineAllowPattern(pattern)
	// Mirror stripLeadingEnvVars: leading env-var assignment tokens are
	// dropped before the allowlist check, so judge the stripped form.
	for len(g.tokens) > 0 && envVarAssignmentToken(g.tokens[0]) {
		g.tokens = g.tokens[1:]
	}
	if len(g.tokens) == 0 {
		return "", false // empty/star-only pattern: not modeled here
	}
	// Non-terminal wildcards (a `*` inside or prefixed to any token — e.g.
	// `*git *` or `cat *=x *`) use glob semantics over the raw command string
	// that this token-prefix model does not represent; treat them as
	// verdict-unknown (reachable) so an exotic pattern can never produce a
	// false-positive doctor FAIL. The trailing wildcard was already stripped
	// by compilation, so any surviving `*` is interior/leading.
	for _, tok := range g.tokens {
		if strings.Contains(tok, "*") {
			return "", false
		}
	}
	switch g.tokens[0] {
	case "git":
		// Modeling caveat (b): git readonly verbs match the engine allowlist;
		// mutation verbs are forbidden-pattern territory (out of scope); every
		// other verb falls to the engine's git "ask" branch and remains
		// table-rescuable. No git-verb grant is ever dead by THIS lint.
		return "", false
	case harnessBinary:
		return harnessGrantDeadClass(g)
	default:
		if engineIntersectsAllowlist(g) {
			return "", false
		}
		return DenyClassNonAllowlisted, true
	}
}

// harnessGrantDeadClass applies the harness auto-allow branch's exceptions
// (modeling caveat a) to a vh-agent-harness-prefixed grant pattern. All such
// forms are engine-allowed EXCEPT vh-agent-harness git forms and gate-wrapper
// forms under the exec trust verbs.
func harnessGrantDeadClass(g engineAllowPattern) (string, bool) {
	if len(g.tokens) < 2 {
		// Bare "vh-agent-harness": the engine allowlist pattern
		// "vh-agent-harness *" matches the bare invocation (wildcard admits
		// zero extra tokens) — reachable.
		return "", false
	}
	if g.tokens[1] == "git" {
		if g.wildcard {
			// "vh-agent-harness git *" (and longer wildcard forms): every
			// matching command carries a token after "git", so the engine's
			// startsWith("vh-agent-harness git ") deny fires every time.
			return DenyClassHarnessGit, true
		}
		// Exact "vh-agent-harness git" alone reaches the allowlist branch
		// (the deny requires a trailing token); any longer exact form
		// ("vh-agent-harness git status") is denied.
		if len(g.tokens) >= 3 {
			return DenyClassHarnessGit, true
		}
		return "", false
	}
	if (g.tokens[1] == "exec" || g.tokens[1] == "exec-ro") &&
		mentionsGateScript(g.tokens) && !isStaticGateInspectionShape(g.tokens) {
		return DenyClassGateWrapper, true
	}
	// Every other harness self-form reaches the auto-allow branch.
	return "", false
}

// mentionsGateScript reports whether any token from the payload position
// (tokens[2:] — after "vh-agent-harness exec|exec-ro") mentions the gate
// script, mirroring isGateWrapperInDevShExec's includes("commit-gate.sh")
// over the command string. Token-level containment is the pattern analog.
func mentionsGateScript(tokens []string) bool {
	for _, tok := range tokens[2:] {
		if strings.Contains(tok, "commit-gate.sh") {
			return true
		}
	}
	return false
}

// isStaticGateInspectionShape mirrors the engine's
// isStaticGateInspectionInDevShExec grammar (the inert forms that cannot
// EXECUTE the gate script and are therefore not denied): `exec bash -n …`
// (syntax check only), `exec cmp …` (byte compare), and the native
// accept-platform / diff path-operand verbs (never execute their operands).
// Patterns fitting these shapes keep some engine-allowed match, so they are
// not dead. Fail-closed to false (deny) on any other shape, exactly like the
// engine.
func isStaticGateInspectionShape(tokens []string) bool {
	if len(tokens) >= 4 && tokens[1] == "exec" && tokens[2] == "bash" && tokens[3] == "-n" {
		return true
	}
	if len(tokens) >= 3 && tokens[1] == "exec" && tokens[2] == "cmp" {
		return true
	}
	if len(tokens) >= 2 && (tokens[1] == "accept-platform" || tokens[1] == "diff") {
		return true
	}
	return false
}

// engineIntersectsAllowlist reports whether SOME command matching the grant
// pattern also matches an engine allowlist pattern (ALLOWED_PATTERNS minus
// the git/harness entries, which are unreachable for a grant whose first
// token is neither "git" nor "vh-agent-harness").
func engineIntersectsAllowlist(g engineAllowPattern) bool {
	for _, a := range engineAllowPatterns {
		if a.tokens[0] == "git" || a.tokens[0] == harnessBinary {
			continue // unreachable for this grant: different first token
		}
		if tokenPrefixCompatible(g, a) {
			return true
		}
	}
	return false
}

// tokenPrefixCompatible reports whether SOME command matches BOTH the grant
// pattern g and the engine allowlist pattern a under the shared token-prefix
// semantics: a command's token list must start with the pattern's tokens,
// and a wildcard pattern additionally admits extra trailing tokens (the
// engine's matchesPattern accepts a token count >= the pattern's, with
// equality required for non-wildcard patterns).
//
// Constructively: if g's tokens extend a's (len(g) >= len(a)), the candidate
// command is g's own tokens — it matches a iff a is wildcard or the token
// lists are equal. If a's tokens extend g's, the candidate is a's tokens —
// it matches g iff g is wildcard. Any other relationship (divergent tokens)
// shares no command.
func tokenPrefixCompatible(g, a engineAllowPattern) bool {
	if len(g.tokens) >= len(a.tokens) {
		if !tokenPrefix(a.tokens, g.tokens) {
			return false
		}
		return a.wildcard || len(g.tokens) == len(a.tokens)
	}
	if !tokenPrefix(g.tokens, a.tokens) {
		return false
	}
	return g.wildcard
}

// tokenPrefix reports whether prefix is a token-wise prefix of tokens.
func tokenPrefix(prefix, tokens []string) bool {
	if len(prefix) > len(tokens) {
		return false
	}
	for i, t := range prefix {
		if t != tokens[i] {
			return false
		}
	}
	return true
}

// LintDeadGrantsForAgent runs the dead-grant lint for ONE agent in the rules
// map, mirroring LintDeadRules' per-agent seam. It lints the EMITTED bash
// block (computeBashBlock) — exactly the entries that land in opencode.jsonc —
// skipping the "*" wildcard (always reachable: it matches engine-allowed
// commands too). A missing agent name returns nil. SourceLine is 0 (the
// rules-map view has no config bytes).
func LintDeadGrantsForAgent(rules map[string]LocationRule, agent string) []DeadGrantFinding {
	rule, ok := rules[agent]
	if !ok {
		return nil
	}
	return lintBashEntries(agent, computeBashBlock(rule, agent, Features{}).entries, nil, 0)
}

// LintDeadGrants lints the resolved core+overlay rule set for every agent —
// the emitter-side view used when no emitted config is at hand. Overlay
// permission-pack.jsonc agents are visible here through resolveRules (their
// location rules merge into the same emission path). Agents are iterated in
// sorted order for deterministic output.
func LintDeadGrants(packs []Pack) []DeadGrantFinding {
	locations, _, _ := resolveRules(packs)
	agents := make([]string, 0, len(locations))
	for name := range locations {
		agents = append(agents, name)
	}
	sort.Strings(agents)
	var findings []DeadGrantFinding
	for _, agent := range agents {
		findings = append(findings, LintDeadGrantsForAgent(locations, agent)...)
	}
	return findings
}

// LintDeadGrantsInConfig lints an EMITTED (or live) opencode.jsonc payload:
// the top-level permission.bash block (attributed to "default") plus every
// rendered agent's permission.bash block. This is the view update/doctor
// consume — it sees core tables, overlay pack agents, AND transform-
// contributed ExtraBash exactly as they were emitted. Findings are sorted by
// (agent, pattern) for deterministic output. A parse error is returned (the
// caller decides the severity); bash values that are not strings are skipped
// defensively.
func LintDeadGrantsInConfig(data []byte) ([]DeadGrantFinding, error) {
	root, err := jsonc.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("permconfig: parse opencode.jsonc for dead-grant lint: %w", err)
	}
	lines := strings.Split(string(data), "\n")

	var findings []DeadGrantFinding
	if perm, ok := root["permission"].(map[string]any); ok {
		if bash, ok := perm["bash"].(map[string]any); ok {
			findings = append(findings, lintBashBlock("default", bash, lines)...)
		}
	}
	if agents, ok := root["agent"].(map[string]any); ok {
		names := make([]string, 0, len(agents))
		for name := range agents {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			agentBlock, ok := agents[name].(map[string]any)
			if !ok {
				continue
			}
			perm, ok := agentBlock["permission"].(map[string]any)
			if !ok {
				continue
			}
			bash, ok := perm["bash"].(map[string]any)
			if !ok {
				continue
			}
			findings = append(findings, lintBashBlock(name, bash, lines)...)
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Agent != findings[j].Agent {
			return findings[i].Agent < findings[j].Agent
		}
		return findings[i].Pattern < findings[j].Pattern
	})
	return findings, nil
}

// lintBashBlock lints one parsed permission.bash object (pattern → action).
// Map iteration order is normalized by sorting the keys first.
func lintBashBlock(agent string, bash map[string]any, lines []string) []DeadGrantFinding {
	keys := make([]string, 0, len(bash))
	for k := range bash {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var entries []orderedEntry
	for _, k := range keys {
		if v, ok := bash[k].(string); ok {
			entries = append(entries, orderedEntry{key: k, val: v})
		}
	}
	return lintBashEntries(agent, entries, lines, 1)
}

// lintBashEntries lints emitted bash entries (shared by the rules-map and
// config-bytes views). lines may be nil (no source-line resolution); base is
// the 1-based offset semantics for configGrantLine (1 = lines are 1-based).
func lintBashEntries(agent string, entries []orderedEntry, lines []string, base int) []DeadGrantFinding {
	var findings []DeadGrantFinding
	for _, e := range entries {
		if e.key == "*" || strings.TrimSpace(e.key) == "" {
			continue
		}
		if class, dead := grantDeadClass(e.key); dead {
			findings = append(findings, DeadGrantFinding{
				Agent:      agent,
				Pattern:    e.key,
				EntryValue: e.val,
				DenyClass:  class,
				Reason:     deadGrantReason(class),
				SourceLine: configGrantLine(agent, e.key, lines, base),
			})
		}
	}
	return findings
}

// configGrantLine resolves the best-effort 1-based line of `"<pattern>":`
// inside the config bytes for the given agent's block. For a named agent it
// is the first pattern-key line at/after the agent's own `"<agent>":` key
// line (the emitted block directly follows the agent key, so the agent's own
// occurrence precedes any later agent's identical pattern). For "default" it
// searches from the LAST top-level (4-space-indented) `"permission":` key —
// encoding/json sorts top-level keys, placing the top-level permission block
// after "agent". Returns 0 when no line resolves (heuristic miss is fine: the
// finding still names agent + pattern exactly).
func configGrantLine(agent, pattern string, lines []string, base int) int {
	if len(lines) == 0 {
		return 0
	}
	patternKey := `"` + pattern + `":`
	start := 0
	if agent == "default" {
		found := -1
		for i, ln := range lines {
			if strings.TrimRight(ln, " \t") == `    "permission": {` {
				found = i
			}
		}
		if found < 0 {
			return 0
		}
		start = found
	} else {
		agentKey := `"` + agent + `":`
		found := -1
		for i, ln := range lines {
			if strings.Contains(ln, agentKey) {
				found = i
				break
			}
		}
		if found < 0 {
			return 0
		}
		start = found
	}
	for i := start; i < len(lines); i++ {
		if strings.Contains(lines[i], patternKey) {
			return i + base
		}
	}
	return 0
}
