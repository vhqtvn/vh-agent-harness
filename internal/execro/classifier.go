// Package execro implements the strictly read-only command classifier that
// powers `vh-agent-harness exec-ro`. It is the Go port of shell-guard's
// walkGitGlobals verb-extraction semantics (templates/core/.opencode/plugins/
// shell-guard-core.js), adapted for exec-ro's CURATED-ALLOWLIST + DEFAULT-DENY
// decision model: exec-ro either EXECUTES the command exactly as given or DENIES
// it with a human-readable notice. It NEVER rewrites the command.
//
// exec-ro is allowlisted in opencode.jsonc as `vh-agent-harness exec-ro *`, so
// opencode NEVER prompts for it — exec-ro itself is the ONLY gate for its
// payload and MUST hard-deny dangerous inner commands. See internal/cli/exec_ro.go
// and the exec-ro section of README.agent.md.
package execro

import (
	"path/filepath"
	"strings"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
)

// Verdict is the classifier decision for one command. When Allow is false,
// Notice carries the human-readable explanation printed to stderr by the
// exec-ro CLI (opencode cannot prompt for exec-ro because it is allowlisted,
// so the notice is the operator's only feedback — it MUST explain WHY the
// command was denied and suggest the bare-command alternative).
type Verdict struct {
	Allow  bool
	Notice string
}

// denyFooter is appended to every DENY notice. It explains the allowlist→deny
// asymmetry (exec-ro can only hard-deny; it cannot ask) and points the operator
// at the bare-command alternative, which DOES prompt through the normal
// permission table.
const denyFooter = "\n(exec-ro is allowlisted in opencode.jsonc so opencode never prompts for it; " +
	"exec-ro itself is the only gate and denies anything it cannot prove is read-only. " +
	"To run a pipeline, a mutating command, or anything with shell metacharacters, " +
	"invoke the bare command directly — it will route through the normal permission table.)"

// metachars are the shell metacharacters that exec-ro conservatively refuses.
// Because exec-ro is a fast string-level heuristic (spoofable by complex shell)
// and a separate OS-level exec-sandbox is the authoritative layer, exec-ro
// STRICTLY DENIES any command containing one of these characters and advises
// the bare-command alternative for pipelines/complex logic. The set is: pipe,
// semicolon, ampersand, dollar, backtick, greater-than, less-than, newline.
//
// NOTE: backtick is a literal character inside this Go double-quoted string (it
// is only special in backtick-delimited raw strings); \n is a newline literal.
var metachars = "|;&$`><\n"

// metacharNames maps each refused character to a readable name for the notice.
var metacharNames = map[string]string{
	"|":  "pipe (|)",
	";":  "semicolon (;)",
	"&":  "ampersand (&)",
	"$":  "dollar ($)",
	"`":  "backtick (`)",
	">":  "greater-than (>)",
	"<":  "less-than (<)",
	"\n": "newline",
}

// Classify decides whether cmd is safe to execute read-only inside repoRoot.
// repoRoot is the absolute project root (the harness root / working directory);
// it is the reference for classifying git `-C`/`--git-dir`/`--work-tree` paths.
//
// The decision is CURATED ALLOWLIST + DEFAULT DENY:
//
//   - Empty command → DENY.
//   - Any shell metacharacter → DENY (conservative; advise bare command).
//   - git → the Go walkGitGlobals port extracts the verb past global flags;
//     info requests (`git --help`) ALLOW; mutation verbs DENY; readonly verbs
//     ALLOW (subject to `-C`/`--git-dir`/`--work-tree` path classification);
//     everything else DEFAULT-DENIES.
//   - non-git → the binary must match an entry in the readonly command group
//     (excluding vh-agent-harness-prefixed entries); otherwise DEFAULT-DENY.
//
// Classify NEVER rewrites the command. The CLI either executes cmd exactly as
// given or exits non-zero with the notice.
func Classify(cmd, repoRoot string) Verdict {
	if strings.TrimSpace(cmd) == "" {
		return Verdict{Allow: false, Notice: "exec-ro: empty command." + denyFooter}
	}
	if idx := strings.IndexAny(cmd, metachars); idx >= 0 {
		name := metacharNames[string(cmd[idx])]
		return Verdict{
			Allow: false,
			Notice: "exec-ro: command contains a shell metacharacter (" + name + ") " +
				"that exec-ro cannot safely classify. " +
				"For pipelines, sequences, redirects, or command substitution, " +
				"run the bare command directly." + denyFooter,
		}
	}
	tokens := strings.Fields(cmd)
	return classifyTokens(tokens, repoRoot)
}

// ClassifyArgs classifies an already-split argv (target + args). It is the
// argv-aware variant of Classify for callers (e.g. exec-sandbox's best-effort
// graceful-skip fallback) that hold the REAL argv slice and must not lose args
// to strings.Fields.
//
// B1 fix: exec-sandbox's graceful-skip fallback previously called
// Classify(target, repoRoot) passing ONLY the bare executable. Because the
// per-binary write/exec flag denylist (scanNonGitWriteFlags / scanGitWriteFlags)
// runs over the classifier's token slice, a bare `find` matched the `find *`
// readonly group with an EMPTY arg tail → no flag scan → ALLOW — so mutating
// payloads (`find . -delete`, `sort -o out in`, `git diff --output=x`,
// `sed -n 'w /path' f`) executed unclassified through runDirect with NO kernel
// isolation and NO exec-ro arg-level protection. ClassifyArgs reconstructs the
// FULL argv slice so those args reach the denylist and correctly DENY.
//
// Unlike Classify, ClassifyArgs does NOT run the shell-metacharacter gate: the
// argv is already split by the shell, so a metacharacter inside one arg is a
// LITERAL character (there is no shell left to interpret a pipe/redirect at this
// layer), not a pipeline/redirect/sequence. The git global-flag walker, the
// readonly-verb mutation guard, the git write/exec SUBCOMMAND-flag denylist, and
// the non-git per-binary write/exec flag denylist ALL run over the full argv
// slice — that is the classification contract the graceful-skip fallback needs.
func ClassifyArgs(target string, args []string, repoRoot string) Verdict {
	tokens := append([]string{target}, args...)
	return classifyTokens(tokens, repoRoot)
}

// classifyTokens is the token-level core shared by Classify (which splits a
// command string via strings.Fields) and ClassifyArgs (which holds an
// already-split argv). It runs the git / non-git readonly classification over
// the FULL token slice, including the per-binary write/exec flag denylist. An
// empty token slice DENIES (mirrors Classify's post-Fields empty guard; for
// Classify this is defensive because the TrimSpace check already returned, but
// ClassifyArgs may pass an empty target and must still fail-closed).
func classifyTokens(tokens []string, repoRoot string) Verdict {
	if len(tokens) == 0 {
		return Verdict{Allow: false, Notice: "exec-ro: empty command." + denyFooter}
	}
	if tokens[0] == "git" {
		return classifyGit(tokens, repoRoot)
	}
	return classifyNonGit(tokens)
}

// classifyGit runs the Go walkGitGlobals port over tokens (tokens[0]=="git") and
// decides from the extracted verb. repoRoot anchors path-bearing flag
// classification.
func classifyGit(tokens []string, repoRoot string) Verdict {
	w := walkGitGlobals(tokens, repoRoot)
	if w.deny != "" {
		return Verdict{Allow: false, Notice: "exec-ro: " + w.deny + "." + denyFooter}
	}
	if w.infoOnly {
		// Terminal read-only info request (git --help / git --version).
		return Verdict{Allow: true}
	}
	verb := w.verb
	if verb == "" {
		return Verdict{Allow: false, Notice: "exec-ro: no git verb recognized." + denyFooter}
	}
	if permconfig.GitMutationVerbsSet[verb] {
		return Verdict{
			Allow: false,
			Notice: "exec-ro: git verb '" + verb + "' is a repo mutation and must go through " +
				"the commit-gate wrapper (.opencode/scripts/commit-gate.sh). " +
				"exec-ro denies all mutations." + denyFooter,
		}
	}
	if permconfig.GitReadonlyVerbs[verb] {
		// Post-verb known write/exec-capable SUBCOMMAND-flag deny. The readonly
		// verbs (diff/show/log/grep/cat-file/...) accept subcommand flags that
		// write a file (--output, the b-F1 prompt-free write vector) or invoke
		// an external program: --ext-diff (external diff driver), grep's
		// -O/--open-files-in-pager (pager binary over matching files), and the
		// textconv/filter family --textconv/--filters (configured diff/filter
		// driver programs). Presence alone is fatal (both the `--flag=value`
		// and the bare `--flag` forms; grep's stuck short `-O<binary>` form is
		// handled too). See scanGitWriteFlags for the full rationale and the
		// heuristic-gate threat model.
		if flag := scanGitWriteFlags(verb, tokens[w.verbIdx+1:]); flag != "" {
			return Verdict{
				Allow: false,
				Notice: "exec-ro: git subcommand flag '" + flag + "' " + gitWriteFlagRationale[flag] +
					"; not permitted under exec-ro's read-only contract." + denyFooter,
			}
		}
		return Verdict{Allow: true}
	}
	return Verdict{
		Allow: false,
		Notice: "exec-ro: git verb '" + verb + "' is not in the read-only set; " +
			"default-deny." + denyFooter,
	}
}

// classifyNonGit matches tokens against the readonly command-group patterns
// (excluding vh-agent-harness-prefixed entries so the exec-ro self-entry does
// not create a nesting loophole). A match ALLOWs subject to the per-binary
// write/exec flag denylist (scanNonGitWriteFlags); anything else DEFAULT-DENIES
// with a notice listing the allowed binaries.
//
// The per-binary denylist runs INSIDE the exec-ro classifier ONLY (after the
// readonly match). The readonly group entry (tables.go) is intentionally left
// as `find *` / `sort *` / `sed -n *`: that group also feeds the shell-guard L2
// permission.bash emission for ALL agents, and widening it (e.g. to `sed *` or
// `sed --sandbox *`) would emit a broader prompt-free rule for every agent and
// reopen the write vector at the L2 layer. The flag rules are exec-ro-internal.
func classifyNonGit(tokens []string) Verdict {
	patterns := readonlyInspectionPatterns()
	for _, p := range patterns {
		if matchReadonlyPattern(tokens, p) {
			if notice := scanNonGitWriteFlags(tokens); notice != "" {
				return Verdict{Allow: false, Notice: notice + denyFooter}
			}
			return Verdict{Allow: true}
		}
	}
	var bins []string
	seen := map[string]bool{}
	for _, p := range patterns {
		if len(p.tokens) > 0 && !seen[p.tokens[0]] {
			seen[p.tokens[0]] = true
			bins = append(bins, p.tokens[0])
		}
	}
	return Verdict{
		Allow: false,
		Notice: "exec-ro: command '" + tokens[0] + "' is not in the read-only inspection set " +
			"(allowed binaries: " + strings.Join(bins, ", ") + "). " +
			"For commands outside the read-only surface, run the bare command directly." + denyFooter,
	}
}

// ---------------------------------------------------------------------------
// Git global-flag registry + walker (Go port of shell-guard-core.js).
//
// The registry mirrors GIT_GLOBAL_FLAG_REGISTRY with two exec-ro additions:
//   - pathBearing marks flags whose value redirects git's repo/object-store
//     location (-C, --git-dir, --work-tree); exec-ro classifies those paths
//     against repoRoot (relative → DENY, external → DENY, in-repo → proceed).
//   - info marks terminal read-only info requests (--help/--version) that
//     auto-allow without a verb.
//
// Unlike shell-guard, this port does NOT produce a rewritten/stripped token
// list (shell-guard uses that for INTERNAL allowlist matching only — it never
// emits a rewrite). exec-ro decides from the verb + path classification alone.
// ---------------------------------------------------------------------------

// valueForm describes how a git global flag consumes its value.
type valueForm int

const (
	vfNext       valueForm = iota // consumes the NEXT token (e.g. -C <path>)
	vfEq                          // value attached via = OR as the next token (--git-dir=/x or --git-dir /x)
	vfOptionalEq                  // value optional via =; NEVER consumes a next token (--exec-path / --exec-path=/x)
	vfNone                        // boolean flag, no value (--no-pager)
)

// gitGlobalFlag is one entry in the registry.
type gitGlobalFlag struct {
	flags       []string
	form        valueForm
	pathBearing bool
	info        bool
	// deny marks config/exec-affecting globals (-c, --config-env, --exec-path)
	// whose presence can cause git to execute arbitrary programs (diff.external,
	// core.editor, core.pager, core.askpass, core.hooksPath, alias.*, filter.*
	// clean/smudge; --config-env resolves a config key from an env var with the
	// same risk; --exec-path redirects to attacker-controlled helper binaries).
	// Because exec-ro is allowlisted in opencode.jsonc (opencode never prompts
	// for it), encountering one short-circuits the walker to DENY before the
	// readonly-verb allow check. This is an exec-ro-ONLY divergence from
	// shell-guard-core.js (bare `git -c ...` is path-blind in shell-guard and
	// prompts through the normal permission table, so the JS registry leaves
	// these flags unrestricted).
	deny bool
}

// gitGlobalFlagRegistry is the Go mirror of shell-guard-core.js
// GIT_GLOBAL_FLAG_REGISTRY. Only the repo-location-affecting flags (-C,
// --git-dir, --work-tree) are pathBearing: those are the ones whose value could
// redirect git to an external repository/object-store. The deny field marks
// config/exec-affecting globals (-c, --config-env, --exec-path) whose presence
// can execute arbitrary programs and therefore hard-deny in exec-ro (see the
// gitGlobalFlag.deny doc for the full rationale and the shell-guard divergence).
var gitGlobalFlagRegistry = []gitGlobalFlag{
	{flags: []string{"-C"}, form: vfNext, pathBearing: true},
	{flags: []string{"-c"}, form: vfNext, deny: true},
	{flags: []string{"--git-dir"}, form: vfEq, pathBearing: true},
	{flags: []string{"--work-tree"}, form: vfEq, pathBearing: true},
	{flags: []string{"--namespace"}, form: vfEq},
	{flags: []string{"--config-env"}, form: vfEq, deny: true},
	{flags: []string{"--exec-path"}, form: vfOptionalEq, deny: true},
	{flags: []string{"-p", "--paginate", "-P", "--no-pager"}, form: vfNone},
	{flags: []string{"--no-replace-objects", "--bare"}, form: vfNone},
	{flags: []string{"-v", "--version", "-h", "--help", "--html-path", "--man-path", "--info-path"}, form: vfNone, info: true},
}

// lookupGitGlobalFlag finds token in the registry. Returns the matching entry,
// and when the token is a `--flag=value` attached form, the attached value and
// isAttached=true. Returns ok=false for bare `-`/`--`, non-flag tokens, and
// unknown flags (the walker treats unknown `-flag` tokens as never-strip
// booleans consumed one-at-a-time so a mutation hidden behind an unrecognized
// flag is STILL caught).
func lookupGitGlobalFlag(token string) (entry *gitGlobalFlag, attached string, isAttached, ok bool) {
	if token == "-" || token == "--" || !strings.HasPrefix(token, "-") {
		return nil, "", false, false
	}
	// Exact match first.
	for i := range gitGlobalFlagRegistry {
		e := &gitGlobalFlagRegistry[i]
		for _, f := range e.flags {
			if token == f {
				return e, "", false, true
			}
		}
	}
	// `--flag=value` split for eq / optional-eq forms. eqIdx > 2 so a bare `-x`
	// (no flag name) is not mistaken; only `--…=…` splits here.
	if eqIdx := strings.IndexByte(token, '='); eqIdx > 2 {
		flagPart := token[:eqIdx]
		valuePart := token[eqIdx+1:]
		for i := range gitGlobalFlagRegistry {
			e := &gitGlobalFlagRegistry[i]
			if e.form != vfEq && e.form != vfOptionalEq {
				continue
			}
			for _, f := range e.flags {
				if flagPart == f {
					return e, valuePart, true, true
				}
			}
		}
	}
	return nil, "", false, false
}

// walkResult is the output of walkGitGlobals.
type walkResult struct {
	verb     string // first non-flag token after consuming leading globals; "" if none
	verbIdx  int    // index in tokens where verb was extracted; -1 if no verb was reached
	deny     string // non-empty when a relative/external path-bearing flag was seen
	infoOnly bool   // true when an info flag (--help/--version) was consumed and no verb followed
}

// walkGitGlobals consumes leading git global flags starting at tokens[1]
// (tokens[0] MUST == "git"; the caller guards this). It is the Go port of
// shell-guard-core.js walkGitGlobals, adapted to exec-ro's decision model.
//
// The walker semantics (ported faithfully):
//
//   - Verb boundary: bare `-`, `--` (options terminator → verb is the token
//     after it), or a non-flag token.
//   - Per valueForm, consume the value token(s): vfNext consumes tokens[i+1]
//     and advances 2; vfEq consumes an attached value (advance 1) or, if no `=`
//     is present, defensively consumes the next token (advance 2); vfOptionalEq
//     and vfNone advance 1.
//   - Unknown `-flag` tokens are consumed one-at-a-time (advance 1) so the verb
//     stays reachable for the mutation guard.
//   - info flags set infoOnly=true (no verb follows → auto-allow at the caller).
//   - pathBearing flags classify their value path against repoRoot: relative →
//     deny; absolute-outside-repo → deny; absolute-in-repo → proceed.
func walkGitGlobals(tokens []string, repoRoot string) walkResult {
	var res walkResult
	res.verbIdx = -1
	i := 1
	for i < len(tokens) {
		tok := tokens[i]

		// Verb boundary: bare `-`, `--` (options terminator), or a non-flag token.
		if tok == "-" || tok == "--" || !strings.HasPrefix(tok, "-") {
			if tok == "--" {
				// Options terminator: the verb is the token after it.
				if i+1 < len(tokens) {
					res.verb = tokens[i+1]
					res.verbIdx = i + 1
				}
			} else {
				res.verb = tok
				res.verbIdx = i
			}
			return res
		}

		entry, attached, isAttached, ok := lookupGitGlobalFlag(tok)
		if !ok {
			// Unknown flag: consume 1, proceed (verb stays reachable for the
			// mutation guard — a mutation hidden behind an unrecognized flag is
			// STILL caught because the verb is extracted past it).
			i++
			continue
		}

		// config/exec-affecting globals (-c, --config-env, --exec-path) can
		// execute arbitrary programs via git config (diff.external, core.editor,
		// core.pager, core.askpass, core.hooksPath, alias.*, filter.*
		// clean/smudge) or helper-binary resolution (--exec-path), and
		// --config-env resolves such a key from an env var with the same risk.
		// Because exec-ro is allowlisted in opencode.jsonc (opencode never
		// prompts for it), these MUST hard-deny here, before the readonly-verb
		// allow check below is ever reached. The offending flag is named in the
		// notice (the bare `git` command still prompts through the normal
		// permission table — see denyFooter).
		if entry.deny {
			flagName := tok
			if eq := strings.IndexByte(tok, '='); eq > 0 {
				flagName = tok[:eq]
			}
			res.deny = "git global flag '" + flagName + "' is config/exec-affecting and can " +
				"execute arbitrary programs via git config (diff.external, core.editor, " +
				"core.pager, core.askpass, core.hooksPath, alias.*, filter.*) or helper-binary " +
				"resolution (--exec-path); not permitted under exec-ro"
			return res
		}

		var valueToken string
		hasValue := false
		switch entry.form {
		case vfNext:
			if i+1 >= len(tokens) {
				// Flag needs a value but none follows — malformed; no verb.
				return res
			}
			valueToken = tokens[i+1]
			hasValue = true
			i += 2
		case vfEq:
			if isAttached {
				valueToken = attached
				hasValue = true
				i += 1
			} else {
				// git accepts a space-separated value for --git-dir / --work-tree;
				// defensively consume the next token when no `=` is present.
				if i+1 >= len(tokens) {
					return res
				}
				valueToken = tokens[i+1]
				hasValue = true
				i += 2
			}
		case vfOptionalEq:
			if isAttached {
				valueToken = attached
				hasValue = true
			}
			i += 1
		case vfNone:
			i += 1
		}

		if entry.info {
			res.infoOnly = true
		}
		if entry.pathBearing && hasValue {
			rawPath := unquoteToken(valueToken)
			relative, inRepo := classifyPath(rawPath, repoRoot)
			if relative {
				res.deny = "relative -C/--git-dir/--work-tree paths are not auto-normalized; " +
					"use an absolute path inside the repo or drop the flag"
				return res
			}
			if !inRepo {
				res.deny = "path-bearing git flag points outside the repo; " +
					"exec-ro cannot reach external repositories"
				return res
			}
			// in-repo absolute path: proceed (ignored for classification).
		}
	}
	return res
}

// GitVerbPastGlobals extracts the first git verb past any leading global flags
// from a `git …` token list, treating EVERY recognized global flag (including
// the config/exec-affecting ones -c/--config-env/--exec-path that exec-ro's
// walkGitGlobals hard-denies early) as consume-and-continue. It performs NO
// path classification and NO deny — it exists for the A1 exec backstop
// (internal/cli/exec_git_guard.go denyExecGitMutationPayload), whose only goal
// is to reach the verb so a mutation hidden behind ANY global flag is caught.
//
// It reuses lookupGitGlobalFlag + gitGlobalFlagRegistry (the canonical flag
// table shared with walkGitGlobals and shell-guard-core.js) so there is NO
// second hand-maintained flag list — only the DECISION diverges
// (consume-and-continue vs exec-ro's hard-deny), and that divergence is
// intentional and scoped to this function.
//
// args[0] MUST == "git" (caller guards). Returns "" when no verb is reached
// (only flags consumed, or a value-bearing flag with no following value, or a
// terminal info flag like `--help` with nothing after it).
func GitVerbPastGlobals(args []string) string {
	i := 1
	for i < len(args) {
		tok := args[i]

		// Verb boundary: bare `-`, `--` (options terminator → verb is the token
		// after it), or a non-flag token.
		if tok == "-" || tok == "--" || !strings.HasPrefix(tok, "-") {
			if tok == "--" {
				if i+1 < len(args) {
					return args[i+1]
				}
				return ""
			}
			return tok
		}

		entry, _, isAttached, ok := lookupGitGlobalFlag(tok)
		if !ok {
			// Unknown flag: consume 1, proceed (verb stays reachable — a
			// mutation hidden behind an unrecognized flag is STILL caught
			// because the verb is extracted past it).
			i++
			continue
		}

		switch entry.form {
		case vfNext:
			// Consumes the NEXT token as its value (e.g. -C <path>, -c <kv>).
			i += 2
		case vfEq:
			if isAttached {
				// Value attached via `=` in the same token.
				i += 1
			} else {
				// git accepts a space-separated value for --git-dir / --work-tree
				// / --namespace / --config-env; defensively consume the next
				// token when no `=` is present (mirrors walkGitGlobals).
				i += 2
			}
		case vfOptionalEq:
			// Value optional via `=`; NEVER consumes a separate next token
			// (--exec-path / --exec-path=/x).
			i += 1
		case vfNone:
			i += 1
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Known write/exec-capable git SUBCOMMAND-flag denylist (verb-level heuristic).
//
// This is a SEPARATE check from the global-flag deny above (B-F1, which denies
// -c / --config-env / --exec-path as config/exec-affecting GLOBALS). The verbs
// in git_readonly (diff/log/show/grep/blame/ls-tree/status/ls-files/
// check-ignore/cat-file/show-ref/rev-parse) also accept SUBCOMMAND flags that
// can write a file or execute an external program even though the verb itself
// is "read-only". A completeness sweep across all 12 readonly verbs established
// that the known write/exec-capable subcommand flags are EXACTLY:
//
//   - --output=<path> / --output <path>  writes the command's output to a file.
//     This is the b-F1 prompt-free write vector: `git diff --output=README.agent.md`
//     overwrites a repo file with diff text, and because exec-ro is allowlisted
//     in opencode.jsonc (opencode never prompts), nothing else would stop it.
//     Valid across the diff/show/log family.
//   - --ext-diff                         asks git to invoke the configured
//     external diff driver (diff.external / gitattributes diff=...), which can
//     execute a configured program. Valid across diff/show/log.
//   - --open-files-in-pager[=<pager>]    (grep) opens the matching FILES (not
//     the grep output) in a pager; the <pager> argument is stuck to the flag
//     (`--open-files-in-pager=/usr/bin/touch`) and runs that binary over the
//     matching files. Bare `--open-files-in-pager` runs the DEFAULT pager
//     (core.pager). Either way this is prompt-free exec via the allowlisted
//     path — same class as --ext-diff.
//   - -O                                 (grep) short form of
//     --open-files-in-pager. Bare `-O` uses the default pager; the STUCK short
//     form `-O<pager>` (e.g. `git grep -Oecho pat`, which git accepts and execs)
//     runs <pager> over the matching files. The stuck form is matched ONLY for
//     verb "grep" because diff/log/show use `-O<orderfile>` for a read-only
//     patch orderfile read (git-diff -O<file>) — a verb-agnostic prefix match
//     would false-positive on that legit read-only use.
//   - --textconv                         runs the configured textconv filter
//     driver (diff.<driver>.textconv + gitattributes diff=<driver>), which can
//     execute a program. Valid for cat-file (off by default there) and the
//     diff/show/log family. NOTE: textconv is DEFAULT-ON for diff/log, so
//     denying the explicit --textconv FLAG does not close the bare-verb
//     default-on path (a config+gitattributes-gated residual that the
//     flag-level denylist cannot reach — deferred to exec-sandbox, same as the
//     existing long-tail framing); the flag deny still closes cat-file's
//     off-by-default vector and the explicit diff/log/show invocation.
//     --no-textconv (the DISABLE form) is correctly NOT matched and stays ALLOWED.
//   - --filters                          (cat-file) runs the configured
//     clean/smudge filter driver (filter.<driver>.clean/smudge + gitattributes),
//     which can execute a program. Off by default for cat-file, so denying the
//     flag closes the vector.
//
// All of these are denied for EVERY readonly verb (the scan is post-verb; it is
// verb-scoped only for grep's -O stuck form). Presence alone is fatal: BOTH the
// `--flag=value` attached form and the bare `--flag` form are denied, mirroring
// the B-F1 config-global deny pattern. The DISABLE forms (--no-ext-diff,
// --no-textconv) are correctly NOT matched and stay ALLOWED.
//
// This is a verb-level HEURISTIC DENYLIST of the KNOWN git write/exec vectors,
// NOT a complete defense. The non-git surface (find/sort/sed) carries a
// parallel per-binary denylist below (find/sort write/exec flags; sed
// --sandbox-required and -i-denied) with the SAME residual framing: known
// vectors closed now, long-tail/exec-sandbox-authoritative. The more thorough
// long-term answer for BOTH surfaces is a per-verb/per-binary safe-flag
// ALLOWLIST (deferred — the surface is large and each readonly verb/binary has
// its own flag set), and the diff/log textconv default-on residual shows why a
// flag denylist alone cannot be authoritative. The authoritative layer for the
// long tail of unknown future write/exec-capable flags — and for the
// default-on residuals — is the separate OS-level exec-sandbox; exec-ro is a
// fast string-level heuristic that closes the KNOWN prompt-free git AND non-git
// write/exec vectors now.
// ---------------------------------------------------------------------------

// gitWriteFlags is the denylist of git readonly-verb subcommand flags that write
// output to a file or invoke an external program. See the block comment above
// for the completeness sweep and the per-flag rationale.
var gitWriteFlags = []string{
	"--output",
	"--ext-diff",
	"--open-files-in-pager",
	"-O",
	"--textconv",
	"--filters",
}

// gitWriteFlagRationale names each denylisted flag's read-only violation for the
// operator-facing deny notice.
var gitWriteFlagRationale = map[string]string{
	"--output":              "writes command output to the named file (--output=<path> or --output <path>)",
	"--ext-diff":            "invokes the configured external diff driver, which can execute a program",
	"--open-files-in-pager": "runs an arbitrary binary as a pager over matching files (--open-files-in-pager=<binary>, or bare for the default pager)",
	"-O":                    "short form of --open-files-in-pager; runs a pager binary over matching files (bare -O uses the default pager, -O<binary> the named one)",
	"--textconv":            "runs the configured textconv filter driver (diff.<driver>.textconv), which can execute a program",
	"--filters":             "runs the configured clean/smudge filter driver (filter.<driver>), which can execute a program",
}

// scanGitWriteFlags reports the first KNOWN write/exec-capable git subcommand
// flag present in postVerb (the args AFTER the readonly verb). It matches BOTH
// the `--flag=value` attached form and the bare `--flag` form, and returns the
// bare flag name (e.g. "--output") when found, or "" when none of the denylisted
// flags are present. postVerb MUST be the slice of tokens AFTER the verb
// (tokens[verbIdx+1:]); leading global flags and the verb token itself are
// excluded so a global-flag value cannot trip a false positive.
//
// verb is the walker-extracted readonly verb. It is used ONLY to scope grep's
// -O stuck short form (`-O<pager>`): git grep accepts `-Oecho` as pager=echo
// (verified empirically — see TestClassify), but diff/log/show use
// `-O<orderfile>` for a read-only patch orderfile read, so the stuck-form
// prefix match must be grep-only. Bare `-O` (a gitWriteFlags entry) is matched
// verb-agnostically; on non-grep verbs it is malformed (orderfile requires a
// value) and harmlessly denied.
func scanGitWriteFlags(verb string, postVerb []string) string {
	for _, tok := range postVerb {
		for _, f := range gitWriteFlags {
			if tok == f || strings.HasPrefix(tok, f+"=") {
				return f
			}
		}
		// grep's -O also accepts a STUCK short value (`-O<pager>`, e.g.
		// `git grep -Oecho pat`) which the bare/`=` match above does not
		// cover. git grep parses `-O<pager>` as the pager binary and execs
		// it over the matching files. Scoped to "grep" ONLY because
		// diff/log/show use -O<orderfile> (a read-only orderfile read); a
		// verb-agnostic prefix match would false-positive on that. (Lowercase
		// `-o` / `--only-matching` is a different flag and is not matched:
		// the prefix is "-O" with a capital O.)
		if verb == "grep" && strings.HasPrefix(tok, "-O") {
			return "-O"
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Known write/exec-capable NON-GIT flag denylist (binary-level heuristic).
//
// The readonly command group (tables.go) admits find, sort, and sed as wildcard
// matches (`find *`, `sort *`, `sed -n *`) so ANY trailing args reach the
// binary. Because exec-ro is allowlisted in opencode.jsonc (opencode never
// prompts for it), those wildcards are a prompt-free write/exec surface: find
// accepts -delete/-exec/-fls, sort accepts -o, sed accepts -i and (without
// --sandbox) the e/r/w commands. exec-ro closes the KNOWN vectors with a
// per-binary denylist applied AFTER the readonly match (so the readonly group
// entry stays unchanged — it also feeds the shell-guard L2 permission.bash
// emission for ALL agents, and widening it would emit a broader prompt-free
// rule for every agent and reopen the vector at the L2 layer).
//
// Per-binary treatment (the operator's chosen treatment for D-F1):
//
//   - find: denylist the write/exec flags (single-dash; bare-token match):
//     -delete, -exec, -execdir, -ok, -okdir (delete or exec a program),
//     -fls, -fprint, -fprint0, -fprintf (write listing output to a file).
//     Read-only finds like `find . -name '*.go'` stay ALLOW. (Note: find's
//     -exec/-ok terminators `;` and `+` reach the classifier when quoted/escaped
//     past the metachar gate — `\;` still trips the `;` metachar first, but the
//     `+` terminator is NOT a metachar so `find . -exec rm {} +` reaches this
//     scan and is caught by `-exec`.)
//   - sort: denylist -o (bare) and --output / --output= (attached) — writes
//     sorted output to a file (`sort -o file in` rewrites file in place).
//     `sort -n`, `sort -k2` stay ALLOW. Bare `-o` is an EXACT token match so
//     combined short forms like `-ro` are NOT matched (a residual —
//     exec-sandbox-authoritative, same framing as the git long tail).
//   - sed: REQUIRE --sandbox AND DENY -i. GNU sed's --sandbox disables its
//     e (exec)/r (read-file)/w (write-file) commands, so under exec-ro sed is
//     only read-only with --sandbox present. --sandbox does NOT block
//     -i/--in-place (verified: `sed --sandbox -i 's/x/y/' f` still rewrites f),
//     so -i is independently denied even when --sandbox is present. sed's -i
//     takes an optional backup-extension suffix (`-i`, `-i.bak`); to avoid
//     false-tripping on a different flag, only bare `-i`, `--in-place`,
//     `--in-place=`, and `-i<suffix>` forms where the char after -i is non-alpha
//     are matched (see isSedInPlaceFlag). `sed -n --sandbox '10,20p' f` stays
//     ALLOW.
//
// Presence alone is fatal (both bare and `=`-attached forms matched where
// applicable), mirroring the git write-flag denylist pattern (B-F1/b-F1). This
// is a binary-level HEURISTIC DENYLIST of the KNOWN non-git write/exec vectors,
// NOT a complete defense; the authoritative layer for the long tail of unknown
// future flags is the separate OS-level exec-sandbox.
// ---------------------------------------------------------------------------

// findWriteFlags is the denylist of find flags that delete files, execute an
// external program, or write listing output to a file. find uses single-dash
// flags; bare-token match (presence alone is fatal).
var findWriteFlags = []string{
	"-delete", "-exec", "-execdir", "-ok", "-okdir",
	"-fls", "-fprint", "-fprint0", "-fprintf",
}

// findWriteFlagRationale names each denylisted find flag's read-only violation
// for the operator-facing deny notice.
var findWriteFlagRationale = map[string]string{
	"-delete":  "deletes files/directories that match the search criteria",
	"-exec":    "executes an external program over each match (the + and \\; terminators run it)",
	"-execdir": "executes an external program from each match's directory",
	"-ok":      "executes an external program over each match (prompting variant of -exec)",
	"-okdir":   "executes an external program from each match's directory (prompting variant of -execdir)",
	"-fls":     "writes the listing to a file (like -ls but to the named path)",
	"-fprint":  "writes the matching file names to a file",
	"-fprint0": "writes the matching file names (null-separated) to a file",
	"-fprintf": "writes a formatted listing to a file",
}

// sortWriteFlagRationale names the denylisted sort write flag's read-only
// violation for the operator-facing deny notice. sort's -o is bare-only;
// --output is the long form (bare or --output=path attached).
var sortWriteFlagRationale = map[string]string{
	"-o":       "writes the sorted output to the named file (sort -o file in rewrites file in place)",
	"--output": "long form of -o; writes the sorted output to the named file",
}

// scanNonGitWriteFlags applies the per-binary write/exec flag denylist to a
// non-git readonly command (find / sort / sed). It is invoked AFTER
// matchReadonlyPattern succeeds; returns "" when the command is safe to ALLOW,
// or a notice PREFIX (without the denyFooter — the caller appends it) naming the
// offending flag and the read-only-contract violation.
//
// tokens is the FULL token slice (tokens[0] is the binary). The scan iterates
// ALL tokens (find/sort/sed flags may appear in any position). Mirrors the git
// write-flag denylist contract: presence alone is fatal, both bare and
// `=`-attached forms matched where applicable.
//
// sed ordering: the --sandbox-required check fires BEFORE the -i check, so a
// command missing --sandbox (regardless of -i) reports the --sandbox violation;
// a command with --sandbox present and -i present reports the -i violation.
func scanNonGitWriteFlags(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	switch tokens[0] {
	case "find":
		for _, tok := range tokens[1:] {
			for _, f := range findWriteFlags {
				if tok == f {
					return "exec-ro: find flag '" + f + "' " + findWriteFlagRationale[f] +
						"; not permitted under exec-ro's read-only contract."
				}
			}
		}
	case "sort":
		for _, tok := range tokens[1:] {
			if tok == "-o" {
				return "exec-ro: sort flag '-o' " + sortWriteFlagRationale["-o"] +
					"; not permitted under exec-ro's read-only contract."
			}
			if tok == "--output" || strings.HasPrefix(tok, "--output=") {
				return "exec-ro: sort flag '--output' " + sortWriteFlagRationale["--output"] +
					"; not permitted under exec-ro's read-only contract."
			}
		}
	case "sed":
		hasSandbox := false
		for _, tok := range tokens[1:] {
			if tok == "--sandbox" {
				hasSandbox = true
				break
			}
		}
		if !hasSandbox {
			return "exec-ro: sed under exec-ro requires --sandbox to disable its e/r/w " +
				"(exec/read-file/write-file) commands; not permitted under exec-ro's read-only contract."
		}
		for _, tok := range tokens[1:] {
			if isSedInPlaceFlag(tok) {
				return "exec-ro: sed flag '-i' writes files in place and is not disabled by --sandbox; " +
					"not permitted under exec-ro's read-only contract."
			}
		}
	}
	return ""
}

// isSedInPlaceFlag reports whether tok is a sed -i/--in-place flag form. GNU
// sed's -i takes an optional backup-extension suffix (`-i`, `-i.bak`). --sandbox
// does NOT disable -i (verified), so every -i form is denied when --sandbox is
// present. To avoid false-tripping on a hypothetical future `-iX` flag (the char
// after -i being a letter), only bare `-i`, `--in-place`, `--in-place=`, and
// `-i<suffix>` forms where the char after -i is NON-ALPHA are matched — sed's
// suffix is a backup extension, not a flag letter.
func isSedInPlaceFlag(tok string) bool {
	if tok == "-i" || tok == "--in-place" {
		return true
	}
	if strings.HasPrefix(tok, "--in-place=") {
		return true
	}
	if strings.HasPrefix(tok, "-i") && len(tok) > 2 {
		c := tok[2]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return true
		}
	}
	return false
}

// classifyPath classifies a raw path (already unquoted) against repoRoot.
// Returns (relative, inRepo):
//
//   - relative=true when raw is not absolute (relative paths are DENIED because
//     they defeat normalization; the operator must use an absolute in-repo path
//     or drop the flag).
//   - inRepo=true when raw is absolute and lexically resolves to repoRoot or a
//     path under it (after filepath.Clean). This is a lexical check (no symlink
//     resolution) — exec-ro is a fast heuristic; the authoritative OS-level
//     sandbox is a separate layer.
func classifyPath(raw, repoRoot string) (relative, inRepo bool) {
	if !filepath.IsAbs(raw) {
		return true, false
	}
	abs := filepath.Clean(raw)
	root := filepath.Clean(repoRoot)
	if abs == root {
		return false, true
	}
	if strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return false, true
	}
	return false, false
}

// unquoteToken strips one layer of surrounding single or double quotes (the JS
// walker applies the same before classifying a `-C` value path, so a quoted
// external path is still caught).
func unquoteToken(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// Non-git readonly pattern matching (Go port of shell-guard-core.js
// matchesPattern + the ALLOWED_PATTERNS compilation).
// ---------------------------------------------------------------------------

// compiledPattern is a readonly-command pattern compiled for matching.
type compiledPattern struct {
	pattern  string
	tokens   []string
	wildcard bool
}

// readonlyInspectionCache memoizes the compiled readonly patterns.
var readonlyInspectionCache []compiledPattern

// readonlyInspectionPatterns returns the compiled readonly-group command
// patterns, EXCLUDING any `vh-agent-harness`-prefixed entry (so the exec-ro
// self-entry `vh-agent-harness exec-ro *` does not create a nesting loophole —
// agents must not nest exec-ro calls).
func readonlyInspectionPatterns() []compiledPattern {
	if readonlyInspectionCache != nil {
		return readonlyInspectionCache
	}
	for _, g := range permconfig.CommandGroups {
		if g.Name != "readonly" {
			continue
		}
		for _, cmd := range g.Commands {
			if strings.HasPrefix(cmd, "vh-agent-harness") {
				continue
			}
			readonlyInspectionCache = append(readonlyInspectionCache, compilePattern(cmd))
		}
	}
	return readonlyInspectionCache
}

// compilePattern compiles one command-table entry into a compiledPattern. It is
// the Go port of shell-guard-core.js's ALLOWED_PATTERNS compilation:
//
//   - trimEndStar: strip an optional trailing '*' then trailing whitespace, so
//     `ls *` → tokens ["ls"] and `git diff *` → tokens ["git","diff"].
//   - wildcard: true when the trimmed pattern originally ended with '*'.
func compilePattern(pattern string) compiledPattern {
	noStar := pattern
	if strings.HasSuffix(noStar, "*") {
		noStar = noStar[:len(noStar)-1]
	}
	noStar = strings.TrimRight(noStar, " \t")
	wildcard := strings.HasSuffix(strings.TrimSpace(pattern), "*")
	return compiledPattern{
		pattern:  pattern,
		tokens:   strings.Fields(noStar),
		wildcard: wildcard,
	}
}

// matchReadonlyPattern is the Go port of shell-guard-core.js matchesPattern:
// tokens must be at least as long as the pattern prefix; every prefix token must
// match exactly; a wildcard pattern accepts extra args, an exact pattern
// requires the same length.
func matchReadonlyPattern(tokens []string, p compiledPattern) bool {
	if len(tokens) < len(p.tokens) {
		return false
	}
	for i, t := range p.tokens {
		if tokens[i] != t {
			return false
		}
	}
	return p.wildcard || len(tokens) == len(p.tokens)
}
