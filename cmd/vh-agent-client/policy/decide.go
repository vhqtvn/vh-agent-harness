// decide.go — the P3 decision core: the HARD-DENY invariant classes
// and the fixed decision order.
//
// DECISION ORDER (fixed; no rule file can change it):
//
//  1. HARD-DENY classes — HardDeny(tool, args) below. These run
//     BEFORE allow-rules are even consulted; an allow-rule that tries
//     to cover a hard-deny shape never gets the chance.
//  2. allow-rules — first matching [[allow]] entry wins.
//  3. default ASK — anything unmatched falls through to the human /
//     --json responder EXACTLY as with no --policy flag. The policy
//     engine never auto-approves something unmatched.
//
// THE FOUR HARD-DENY CLASSES (plus the fail-closed args-shape rule,
// the plain-word provability gate, the closed exec-intermediary
// tripwire set, and the fail-closed env-option rule):
//
//  0. Unrecognized args on a known-risky tool (run_shell, read,
//     write, edit): the args must parse into the tool's known shape.
//     Uncertainty DENIES — an args shape the engine cannot reason
//     about is never eligible for an auto-allow.
//  1. Secret env-var writes: any env NAME being assigned (NAME=value
//     token shapes in a run_shell command, or an "env" arg on any
//     tool) matching the engine's env-scrub pattern
//     (?i)KEY|PASSWORD|SECRET|TOKEN (mirrored from
//     internal/tools/shell sensitiveEnvPattern) or carrying the
//     VH_AGENT_HARNESS_ credential prefix.
//  2. run_shell git-mutation: argv[0] git (basename included, env
//     prefixes stripped, compound segments split — including the
//     single '&' asynchronous separator, since bash -c executes BOTH
//     sides of `a & b`) with a subcommand in the closed mutation set
//     below. Read-only git (status/log/diff/show/grep) stays
//     rule-eligible. git with flags before the subcommand, no
//     subcommand, or a non-plain-word subcommand/argv0 denies (cannot
//     identify → deny). A git-headed segment whose words carry
//     substitution metacharacters ($, backtick, parens) denies —
//     `git log $(git push)` executes the substitution, so the words
//     are not provably read-only. Wrapper shapes (sudo git push,
//     nohup git push, sh -c "git push …") carry a non-git argv[0]:
//     a word-level git-adjacency scan denies any segment containing
//     BOTH the word git and a mutation subcommand — deliberately
//     over-approximating (prose like `echo about git push` denies
//     too; the deny direction is the fail-closed direction).
//     2a. PLAIN-WORD PROVABILITY GATE (the anti-evasion genus closure):
//     a run_shell segment is allow-eligible only if EVERY word is
//     PLAIN — after edge-quote trimming, matching the conservative
//     identifier grammar ^[A-Za-z0-9_./:@+=-]+$ (no interior quotes,
//     no $, no backtick, no parens, no other shell metacharacters).
//     Any non-plain word anywhere in any segment is UNIDENTIFIABLE
//     lexically and HARD-DENIES; no allow rule can rescue it. This
//     single rule closes the quote-fragmentation genus (a `sh -c`
//     body whose words carry interior quote runs — git spelled as
//     two quoted fragments — so no fragment matches git or the
//     mutation set) and the substitution-wrapped genus (echo of a
//     $(git push) or backtick-substitution — argv[0] is
//     plain and non-git, so adjacency cannot see the mutation) at any
//     encoding, any position. It runs AFTER the git classes so their
//     richer deny reasons (mutation/substitution) stay primary for
//     git-headed shapes, and catches every remaining segment.
//     CONSEQUENCE (accepted, documented): the deny-direction
//     over-approximation WIDENS — `echo $(date)`, `ls $(pwd)`, `git
//     diff HEAD~1` (`~` is outside the grammar) now hard-deny under
//     any policy: v1 has NO policy path for command substitution or
//     non-grammar words in run_shell. Per the established
//     uncertainty posture (unidentifiable = deny, never ask): the
//     human can still run such a command themselves outside the
//     client, or the operator drops --policy for interactive use.
//     Plain-word false positives remain (`echo about git push`
//     denies via adjacency).
//     2b. EXEC-INTERMEDIARY TRIPWIRES (CLOSED CLASS): the exec bridges
//     that assemble a child argv the segment scans cannot follow.
//     Runs FIRST among the run_shell command classes (before the
//     git classes and gate 2a — an exec-bridge word always trips,
//     so its reason wins even when the git scans would co-fire).
//     The words `xargs`, `parallel` (GNU parallel), `-exec`,
//     `-execdir`, `-ok`, `-okdir` deny wherever they appear as
//     standalone words in ANY segment (path-qualified forms
//     included, via path.Base) — position-independent, so no
//     displacement wrapper (`nohup xargs git`, `command`/`nice`/
//     `time`/`stdbuf`/`setsid xargs …`, `env -i xargs …`, `env env
//     xargs …`, `sh -c 'xargs git'`) can move the bridge out of
//     scanning position: a pipe puts the mutation word and the word
//     git in DIFFERENT segments (`echo push | xargs git` shows no
//     single-segment git+mutation adjacency), and bash lets xargs /
//     parallel assemble the child `git push` from words the scan
//     cannot recombine; `-ok`/`-okdir` are find's prompt-exec
//     variants of the `-exec`/`-execdir` bridges. Additionally any
//     single word matching ^git-[a-z][a-z0-9-]*$ (dashed-form git
//     dispatch: `git-push origin main` puts the subcommand inside
//     argv[0], never in the subcommand position the mutation scan
//     reads; the argv0 position is basename-checked, so a path form
//     trips too). The suffix set stays broad — ANY git-* word trips.
//     Documented over-approximation (deny direction): the closed
//     word set and git-* words deny even as benign whole-word
//     mentions (`man git-push`, `git log --grep git-push`, `echo
//     about xargs`); an embedded substring like `--grep=git-push`
//     and the words `xargus`/`parallels` do NOT trip — the match is
//     anchored whole-word (exact or basename), never substring; the
//     git-* regex is LOWERCASE-only (`git-Push` does not trip —
//     git-core dispatch binaries are lowercase). These are
//     CLOSED-CLASS LEXICAL TRIPWIRES, NOT PROOFS — see the CONTRACT
//     note below.
//     2c. ENV-OPTION FAIL-CLOSED (the stripEnvPrefix boundary): after
//     stripping leading NAME=value assignments and at most one
//     literal `env`, an env OPTION word (`-i`, `-u NAME`, `--`)
//     leaves the child environment/argv unidentifiable to the
//     lexical scan (`echo push | env -i xargs git` made `-i` the
//     argv[0] and hid the bridge) — the segment denies (uncertainty
//     denies). Boundary: `env FOO=bar git status` (assignments then
//     a plain argv[0]) strips cleanly and stays identifiable through
//     the normal classes.
//  3. Confinement-escape path shapes PROVABLE from args alone:
//     ".." segments in file-tool path args and run_shell workdir, and
//     traversal-shaped words in a run_shell command ("..", "../x",
//     "a/../b"). BOUNDARY (honest): absolute paths are NOT deniable
//     client-side — the daemon's configured roots are not knowable
//     here, so absolute paths fall to rule matching (a relative-root
//     rule prefix never matches them → ask). Regex ".." in
//     search/glob patterns is NOT scanned (it is grammar, not a
//     path).
//  4. Sandbox-mode escalation: the closed vocabulary
//     {danger-full-access substring in any arg value; sandbox /
//     sandbox_mode / sandboxPermissions override KEYS at any nesting
//     depth; "--sandbox…"-shaped flags or a "sandbox=" env prefix in
//     a run_shell command}. Per-call sandbox override does not exist
//     on the tool surface; requesting it is an escape attempt.
//
// All command-text analysis is LEXICAL (whitespace fields; no quoting
// or expansion semantics). That is deliberately conservative in the
// DENY direction: quoted text can only add segments/words, every
// added word must itself be plain (gate 2a) or the whole call denies,
// and quoting evasions of argv0 or a git subcommand produce non-plain
// words, which DENY.
//
// CONTRACT (honest): the hard-deny classes above are CLOSED-CLASS
// LEXICAL TRIPWIRES, NOT PROOFS. The engine denies a closed
// vocabulary of shapes it can identify; it CANNOT prove a command
// string will not invoke git (or anything else) through arbitrary
// intermediaries — make, scripts, compiled helpers. The daemon's
// sandbox (--sandbox read-only|workspace-write) is the actual
// security boundary for filesystem/network effects; this approval
// policy is defense-in-depth in front of it. Running --policy with
// --sandbox off means auto-approve of whatever the model writes —
// an explicit operator risk decision. The classes are client-side
// tripwires in front of the daemon's own guards/sandbox — they are
// not the enforced boundary.
package policy

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// sensitiveEnvPattern mirrors internal/tools/shell
// (name-match substring drop, case-insensitive).
var sensitiveEnvPattern = regexp.MustCompile(`(?i)KEY|PASSWORD|SECRET|TOKEN`)

// engineCredentialPrefix mirrors internal/tools/shell.
const engineCredentialPrefix = "VH_AGENT_HARNESS_"

// envAssignRe matches a word shaped NAME= (an env-assignment token).
var envAssignRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=`)

// segmentSplitRe splits a command into segments on shell control
// operators (&&, ||, ;, |, newline, and the single '&' asynchronous
// list separator — bash -c runs BOTH sides of `a & b`, so an
// &-separated tail is a real command segment, not part of the head).
// LEXICAL, without quote awareness (a separator inside quotes still
// splits; that can only add checked segments — conservative
// direction).
var segmentSplitRe = regexp.MustCompile(`&&|&|\|\||;|\||\n`)

// gitMutationSubcommands is the CLOSED git-mutation set. Anything
// outside it (status, log, diff, show, grep, describe, ...) stays
// rule-eligible; the daemon's own policy/sandbox remains the backstop
// for exotic subcommands.
var gitMutationSubcommands = map[string]bool{
	"add": true, "am": true, "apply": true, "branch": true,
	"checkout": true, "cherry-pick": true, "clean": true, "clone": true,
	"commit": true, "config": true, "fetch": true, "merge": true,
	"mv": true, "pull": true, "push": true, "rebase": true,
	"reset": true, "restore": true, "revert": true, "rm": true,
	"sparse-checkout": true, "stash": true, "submodule": true,
	"switch": true, "tag": true, "worktree": true, "update-ref": true,
	"symbolic-ref": true, "gc": true, "prune": true,
}

// HardDeny reports whether (tool, args) falls in a hard-deny class and
// why. It is pure: no I/O, no clock, deterministic in (tool, args).
func HardDeny(tool string, args json.RawMessage) (reason string, denied bool) {
	if r, ok := unrecognizedArgs(tool, args); ok {
		return r, true
	}
	if r, ok := secretEnvWrite(tool, args); ok {
		return r, true
	}
	// Class 2b runs FIRST among the run_shell command classes: an
	// exec-bridge word (any xargs/parallel/-exec/-execdir/-ok/-okdir
	// word, git-* dashed form) always trips, so its reason wins even
	// when the git scans below would co-fire on the same command.
	// Class 2c (env options) follows: an env OPTION word leaves the
	// segment unidentifiable after the env-prefix strip.
	if r, ok := execIntermediary(tool, args); ok {
		return r, true
	}
	if r, ok := envOptionPrefix(tool, args); ok {
		return r, true
	}
	if r, ok := gitMutation(tool, args); ok {
		return r, true
	}
	if r, ok := unidentifiableCommandWords(tool, args); ok {
		return r, true
	}
	if r, ok := confinementEscape(tool, args); ok {
		return r, true
	}
	if r, ok := sandboxEscalation(tool, args); ok {
		return r, true
	}
	return "", false
}

// DecisionKind is the closed outcome vocabulary.
type DecisionKind string

const (
	DecisionAllow DecisionKind = "allow"
	DecisionDeny  DecisionKind = "deny" // HARD-DENY (a rule never denies)
	DecisionAsk   DecisionKind = "ask"  // fall through to the human/--json responder
)

// Decision is one policy verdict over (tool, args).
type Decision struct {
	Kind DecisionKind
	// Reason carries the hard-deny reason (Kind == DecisionDeny).
	Reason string
	// Rule is the [[allow]] entry that produced an allow (provenance);
	// nil for deny/ask.
	Rule *Rule
}

// Decide applies the FIXED decision order: hard-deny classes →
// allow-rules (first match wins) → ask. It never auto-approves an
// unmatched call and never consults rules for a hard-denied one.
func (p *Policy) Decide(tool string, args json.RawMessage) Decision {
	if reason, denied := HardDeny(tool, args); denied {
		return Decision{Kind: DecisionDeny, Reason: reason}
	}
	if p != nil {
		for i := range p.rules {
			if ruleMatches(&p.rules[i], tool, args) {
				return Decision{Kind: DecisionAllow, Rule: &p.rules[i]}
			}
		}
	}
	return Decision{Kind: DecisionAsk}
}

// ruleMatches reports whether one allow-rule covers (tool, args).
func ruleMatches(r *Rule, tool string, args json.RawMessage) bool {
	if !toolPatternMatch(r.Tool, tool) {
		return false
	}
	switch {
	case r.Argv0 != "":
		return argv0AllSegments(r.Argv0, args)
	case r.Path != "":
		return pathPrefixMatch(r.Path, stringValue(rawJSON(args, "path")))
	default:
		return true // broad allow for the tool pattern, behind hard-deny
	}
}

// toolPatternMatch: exact name, or "prefix:*" glob (matches tool names
// starting "prefix:").
func toolPatternMatch(pattern, tool string) bool {
	if prefix, ok := strings.CutSuffix(pattern, ":*"); ok {
		return strings.HasPrefix(tool, prefix+":")
	}
	return pattern == tool
}

// pathPrefixMatch: rooted-prefix semantics — a slash-terminated prefix
// matches by string prefix; a bare prefix matches itself or children.
func pathPrefixMatch(rulePrefix, argPath string) bool {
	if strings.HasSuffix(rulePrefix, "/") {
		return strings.HasPrefix(argPath, rulePrefix)
	}
	return argPath == rulePrefix || strings.HasPrefix(argPath, rulePrefix+"/")
}

// argv0AllSegments: an argv0-constrained rule requires EVERY command
// segment's (env-stripped) argv[0] to equal the constraint. Hard-deny
// has already guaranteed every argv0 is an identifiable plain word, so
// a mismatch here means "not covered" (ask), never "unknown".
func argv0AllSegments(want string, args json.RawMessage) bool {
	cmd := stringValue(rawJSON(args, "command"))
	for _, seg := range commandSegments(cmd) {
		words := stripEnvPrefix(strings.Fields(seg))
		if len(words) == 0 {
			continue
		}
		if words[0] != want && path.Base(words[0]) != want {
			return false
		}
	}
	return true
}

// --- class 0: unrecognized args on known-risky tools -------------------------

// argShape is the closed args shape of one known-risky tool: the
// required keys and the typed-optional keys. An args object with a
// missing required key, a mistyped known key, or ANY unknown key is
// UNRECOGNIZED (deny) — the shape is the schema.
type argShape struct {
	required []string
	optional map[string]string // key → json type ("string" | "number" | "boolean")
}

var riskyArgShapes = map[string]argShape{
	"run_shell": {
		required: []string{"command"},
		optional: map[string]string{"timeout_ms": "number", "workdir": "string"},
	},
	"read": {
		required: []string{"path"},
		optional: map[string]string{"offset": "number", "limit": "number"},
	},
	"write": {
		required: []string{"path"},
		optional: map[string]string{"content": "string"},
	},
	"edit": {
		required: []string{"path"},
		optional: map[string]string{"old": "string", "new": "string", "replaceAll": "boolean"},
	},
}

// unrecognizedArgs denies known-risky tools whose args are not the
// tool's known shape.
func unrecognizedArgs(tool string, args json.RawMessage) (string, bool) {
	shape, risky := riskyArgShapes[tool]
	if !risky {
		return "", false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(args, &obj); err != nil {
		return fmt.Sprintf("unrecognized %s args (not a JSON object): uncertainty denies", tool), true
	}
	for _, key := range shape.required {
		raw, ok := obj[key]
		if !ok || !isJSONString(raw) || strings.TrimSpace(stringValue(raw)) == "" {
			return fmt.Sprintf("unrecognized %s args: %q must be a non-empty string: uncertainty denies", tool, key), true
		}
	}
	for key := range obj {
		typ, known := shape.optional[key]
		isReq := false
		for _, r := range shape.required {
			if r == key {
				isReq = true
			}
		}
		if !known && !isReq {
			return fmt.Sprintf("unrecognized %s args: unknown key %q: uncertainty denies", tool, key), true
		}
		if known && !hasJSONType(obj[key], typ) {
			return fmt.Sprintf("unrecognized %s args: key %q must be %s: uncertainty denies", tool, key, typ), true
		}
	}
	return "", false
}

func isJSONString(raw json.RawMessage) bool {
	var s string
	return json.Unmarshal(raw, &s) == nil
}

func stringValue(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func hasJSONType(raw json.RawMessage, typ string) bool {
	switch typ {
	case "string":
		return isJSONString(raw)
	case "number":
		var n float64
		return json.Unmarshal(raw, &n) == nil
	case "boolean":
		var b bool
		return json.Unmarshal(raw, &b) == nil
	}
	return false
}

// --- class 1: secret env-var writes ------------------------------------------

// secretEnvWrite denies env-var writes whose NAME matches the scrub
// pattern or the engine credential prefix.
func secretEnvWrite(tool string, args json.RawMessage) (string, bool) {
	if name, ok := secretEnvNameInCommand(tool, args); ok {
		return fmt.Sprintf("secret env write: %s (hard-deny)", name), true
	}
	if name, ok := secretEnvNameInEnvArg(args); ok {
		return fmt.Sprintf("secret env write in env arg: %s (hard-deny)", name), true
	}
	return "", false
}

// secretEnvNameInCommand scans a run_shell command's words for
// NAME=… assignment shapes with a secret-shaped NAME.
func secretEnvNameInCommand(tool string, args json.RawMessage) (string, bool) {
	if tool != "run_shell" {
		return "", false
	}
	cmd := stringValue(rawJSON(args, "command"))
	for _, w := range strings.Fields(cmd) {
		m := envAssignRe.FindStringSubmatch(w)
		if m == nil {
			continue
		}
		if isSecretEnvName(m[1]) {
			return m[1], true
		}
	}
	return "", false
}

// secretEnvNameInEnvArg scans an "env" arg (array of NAME=value
// strings, or an object keyed by NAME) on ANY tool.
func secretEnvNameInEnvArg(args json.RawMessage) (string, bool) {
	raw := rawJSON(args, "env")
	if len(raw) == 0 {
		return "", false
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, e := range arr {
			m := envAssignRe.FindStringSubmatch(e)
			if m != nil && isSecretEnvName(m[1]) {
				return m[1], true
			}
		}
		return "", false
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		for name := range obj {
			if isSecretEnvName(name) {
				return name, true
			}
		}
	}
	return "", false
}

// isSecretEnvName mirrors the shell tool's dual scrub.
func isSecretEnvName(name string) bool {
	return sensitiveEnvPattern.MatchString(name) || strings.HasPrefix(name, engineCredentialPrefix)
}

// rawJSON decodes args as an object and returns the raw value of key.
func rawJSON(args json.RawMessage, key string) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(args, &obj); err != nil {
		return nil
	}
	return obj[key]
}

// --- class 2: git mutation ---------------------------------------------------

// gitMutation denies run_shell commands whose (lexically extracted)
// argv[0] is git with a mutating subcommand, or whose shape prevents
// identifying either word.
func gitMutation(tool string, args json.RawMessage) (string, bool) {
	if tool != "run_shell" {
		return "", false
	}
	cmd := stringValue(rawJSON(args, "command"))
	for _, seg := range commandSegments(cmd) {
		if r, ok := gitMutationSegment(seg); ok {
			return r, true
		}
	}
	return "", false
}

// commandSegments splits a command lexically on control operators.
func commandSegments(cmd string) []string {
	return segmentSplitRe.Split(cmd, -1)
}

// gitMutationSegment checks ONE command segment.
func gitMutationSegment(seg string) (string, bool) {
	words := stripEnvPrefix(strings.Fields(seg))
	if len(words) == 0 {
		return "", false // an empty segment is not a command
	}
	argv0 := words[0]
	// Round-4 A-F1: the plainness grammar here is the COMMAND-word
	// grammar (plainCommandWordRe — no `%`, `=` allowed), replacing a
	// stale cross-reference to parse.go's isPlainWord (the rule-VALUE
	// grammar). Deliberately UNTRIMMED — no edge-quote trimming at
	// this checkpoint: an edge-quoted argv[0] denies right here,
	// which is what keeps quoted-tail segments (`echo "a && b"`)
	// unidentifiable; gate 2a's trimmed variant (isPlainCommandWord)
	// is the later, wider net.
	if !plainCommandWordRe.MatchString(argv0) {
		return fmt.Sprintf("unidentifiable argv[0] %q in command (quoting/metacharacters deny): %s", argv0, seg), true
	}
	if path.Base(argv0) != "git" {
		// F3: wrapper shapes (sudo git push, nohup git push,
		// sh -c "git push …") hide the mutation behind a non-git
		// argv[0]. Word-level git-adjacency: BOTH the word git and a
		// mutation subcommand in one segment deny. Deliberate
		// over-approximation — `echo about git push` denies too
		// (deny direction; documented).
		if sub, adjacent := gitAdjacentMutation(words); adjacent {
			return fmt.Sprintf("git-adjacent mutation (the words git and git %s co-occur behind argv[0] %q): %s (hard-deny)", sub, argv0, seg), true
		}
		return "", false
	}
	rest := words[1:]
	if len(rest) == 0 {
		return "git with no subcommand (shape unrecognized): uncertainty denies", true
	}
	// F2: a git-headed segment never carries substitution
	// metacharacters in ANY word — `git log $(git push)` executes
	// the substitution; the words are not provably read-only.
	if hasSubstitutionMetachar(rest) {
		return fmt.Sprintf("git command carries substitution metacharacters (%s): %s (hard-deny)", substitutionMetachars, seg), true
	}
	sub := rest[0]
	if strings.HasPrefix(sub, "-") {
		return "git with flags before the subcommand (subcommand unidentifiable): uncertainty denies", true
	}
	// Round-4 A-F1 (same alignment as the argv[0] check above): the
	// command-word grammar, UNTRIMMED — an edge-quoted subcommand
	// denies here too, so `git "push"` can never parse as read-only.
	if !plainCommandWordRe.MatchString(sub) {
		return fmt.Sprintf("git subcommand %q is not a plain word (shape unrecognized): uncertainty denies", sub), true
	}
	if gitMutationSubcommands[sub] {
		return fmt.Sprintf("git %s is in the hard-deny mutation class", sub), true
	}
	return "", false
}

// substitutionMetachars are the lexical markers of command/parameter
// substitution. In a git-headed segment ANY word carrying one denies:
// the executed words are not provably what they appear to be.
const substitutionMetachars = "$`()"

// hasSubstitutionMetachar reports whether any word carries a
// substitution metacharacter.
func hasSubstitutionMetachar(words []string) bool {
	for _, w := range words {
		if strings.ContainsAny(w, substitutionMetachars) {
			return true
		}
	}
	return false
}

// gitAdjacentMutation reports whether words contain BOTH the word git
// (quote-trimmed basename — `sh -c "git push"` lexically yields the
// word `"git`) and a word in the closed mutation set: the wrapper
// shapes whose argv[0] is not git. Over-approximates deliberately
// (prose holding both words denies too) — the deny direction is the
// fail-closed direction.
func gitAdjacentMutation(words []string) (string, bool) {
	mutation := ""
	hasGit := false
	for _, w := range words {
		tw := trimQuoteRunes(w)
		if path.Base(tw) == "git" {
			hasGit = true
		}
		if gitMutationSubcommands[tw] {
			mutation = tw
		}
	}
	if hasGit && mutation != "" {
		return mutation, true
	}
	return "", false
}

// trimQuoteRunes strips leading/trailing quote characters so a
// lexically-quoted word is still recognizable in the adjacency scan.
// LEXICAL ONLY — no escape semantics; the trim can only make a word
// MORE recognizable, and every recognized shape denies.
func trimQuoteRunes(s string) string {
	return strings.Trim(s, `"'`)
}

// --- class 2a: plain-word provability gate -----------------------------------

// plainCommandWordRe is the conservative identifier grammar a command
// word must match — AFTER edge-quote trimming — to be identifiable:
// letters, digits, and `_ . / : @ + = -` only. Anything else is
// lexically unidentifiable: interior quote runs (quote-fragmented
// words), substitution markers ($, backtick, parens), and every other
// shell metacharacter or uncommon rune (~, %, ^, comma, glob,
// redirect, …). `=` is IN the grammar so `NAME=value` env-assignment
// words stay identifiable (the secret-env class scans them by NAME
// shape).
var plainCommandWordRe = regexp.MustCompile(`^[A-Za-z0-9_./:@+=-]+$`)

// isPlainCommandWord reports whether one command word is identifiable:
// edge-quote trimmed, then matched against the conservative grammar.
func isPlainCommandWord(w string) bool {
	return plainCommandWordRe.MatchString(trimQuoteRunes(w))
}

// unidentifiableCommandWords denies a run_shell command in which ANY
// word of ANY segment is not a plain command word: the executed words
// are not provably what they appear to be (bash may re-quote,
// concatenate, or substitute them), so the segment is never
// allow-eligible — unidentifiable denies, and no rule can rescue it.
// Deliberate, accepted consequence: benign substitutions and
// non-grammar words (`echo $(date)`, `ls $(pwd)`, `git diff HEAD~1`)
// deny too; v1 has no policy path for them (see the class 2a note in
// the file header).
func unidentifiableCommandWords(tool string, args json.RawMessage) (string, bool) {
	if tool != "run_shell" {
		return "", false
	}
	cmd := stringValue(rawJSON(args, "command"))
	for _, seg := range commandSegments(cmd) {
		for _, w := range strings.Fields(seg) {
			if !isPlainCommandWord(w) {
				return fmt.Sprintf("unidentifiable word %q in command segment (quoting/substitution/metacharacters deny; only plain words are provable): %s", w, seg), true
			}
		}
	}
	return "", false
}

// stripEnvPrefix drops leading NAME=value assignment words and at most
// one leading "env" word (the `env NAME=v git …` / `NAME=v env git …`
// invocation shapes). FAIL-CLOSED BOUNDARY (class 2c, round 4): the
// strip itself cannot express "what follows is an env option" — a
// remainder headed by an option word (`-i`, `-u NAME`, `--`) is
// unidentifiable and denied by envOptionPrefix; this function is only
// the transparent-prefix peel for the clean shapes.
func stripEnvPrefix(words []string) []string {
	strippedEnv := false
	for len(words) > 0 {
		w := words[0]
		if envAssignRe.MatchString(w) {
			words = words[1:]
			continue
		}
		if w == "env" && !strippedEnv {
			strippedEnv = true
			words = words[1:]
			continue
		}
		break
	}
	return words
}

// --- class 2b: exec-intermediary tripwires (closed class) ---------------------

// gitDashedWordRe matches the dashed-form git dispatch words:
// `git-push` (any `git-<sub>` subcommand spelled as one word)
// dispatches `git push` — the subcommand lives inside argv[0], never
// in the subcommand position the mutation scan reads. ANY such word
// trips (suffix set deliberately broad): the deny-direction
// over-approximation covers benign whole-word mentions (`man
// git-push`, `git help git-push`); an embedded substring
// (`--grep=git-push`) does NOT trip — the match is anchored
// whole-word. The regex is LOWERCASE-only: `git-Push` does not trip
// (git-core dispatch binaries are lowercase; a mixed-case shape is
// not a git dispatch and falls to the git-adjacency scan or the
// disclosed arbitrary-intermediary residual).
var gitDashedWordRe = regexp.MustCompile(`^git-[a-z][a-z0-9-]*$`)

// execIntermediaryWords is the CLOSED set of exec-bridge words denied
// WHEREVER they appear as standalone words in any segment
// (path-qualified forms included, via path.Base): xargs and GNU
// parallel assemble a child argv from words/stdin the lexical segment
// scan cannot recombine; -ok/-okdir are find's prompt-exec variants
// of the -exec/-execdir bridges. The word match is EXACT (or
// basename) — `xargus`/`parallels` do not trip, and an embedded
// substring never does. Position-independence is the point (round 4):
// a displacement wrapper (nohup/command/nice/time/stdbuf/setsid,
// `env -i`, double `env`, an sh -c body) can no longer move the
// bridge out of scanning position — the word xargs anywhere in any
// segment denies.
var execIntermediaryWords = map[string]bool{
	"xargs": true, "parallel": true, "-ok": true, "-okdir": true,
}

// execIntermediary denies the CLOSED exec-intermediary vocabulary:
// the exec bridges that assemble a child argv the lexical segment
// scan cannot follow. Runs before the git classes so an exec-bridge
// word always trips regardless of what else the command shows.
//
// CLOSED-CLASS LEXICAL TRIPWIRES, NOT PROOFS: this vocabulary is
// enumerable and denied; encoded, translated, or
// undisplaced-vocabulary forms beyond the closed set, and arbitrary
// intermediaries (make, scripts, compiled helpers), are not provable
// from a command string — the daemon's sandbox is the actual security
// boundary; this engine is defense-in-depth (see the CONTRACT note in
// the file header).
func execIntermediary(tool string, args json.RawMessage) (string, bool) {
	if tool != "run_shell" {
		return "", false
	}
	cmd := stringValue(rawJSON(args, "command"))
	for _, seg := range commandSegments(cmd) {
		words := stripEnvPrefix(strings.Fields(seg))
		if len(words) == 0 {
			continue
		}
		// argv[0] xargs (edge-quote trim + path.Base, mirroring the
		// git class's argv0 treatment): xargs assembles its child
		// argv from words/stdin — `echo push | xargs git` recombines
		// into `git push` behind the scan. (Since round 4 the
		// word-level scan below is the general position-independent
		// backstop; this argv[0] check keeps its specific message.)
		argv0 := trimQuoteRunes(words[0])
		if path.Base(argv0) == "xargs" {
			return fmt.Sprintf("exec intermediary: argv[0] %q is xargs — it assembles a child argv from words/stdin the lexical scan cannot follow: %s (hard-deny)", words[0], seg), true
		}
		// Dashed-form dispatch from the argv0 position: a PATH form
		// (`/usr/lib/git-core/git-push origin main`) basename-trips
		// like the git class's argv0 treatment.
		if gitDashedWordRe.MatchString(path.Base(argv0)) {
			return fmt.Sprintf("git dashed-form: argv[0] %q dispatches a git subcommand inside the word, invisible to the subcommand scan: %s (hard-deny)", words[0], seg), true
		}
		for _, w := range words {
			tw := trimQuoteRunes(w)
			if tw == "-exec" || tw == "-execdir" {
				return fmt.Sprintf("exec intermediary: word %q is a find exec bridge, wherever it appears: %s (hard-deny)", w, seg), true
			}
			// Position-independent exec-bridge words (round 4): the
			// displacement wrappers move the bridge out of argv[0];
			// the word-level scan denies the closed set wherever it
			// appears, path-qualified forms included.
			if base := path.Base(tw); execIntermediaryWords[base] {
				return fmt.Sprintf("exec intermediary: word %q is an exec bridge (%s), wherever it appears: %s (hard-deny)", w, base, seg), true
			}
			if gitDashedWordRe.MatchString(tw) {
				return fmt.Sprintf("git dashed-form: word %q dispatches a git subcommand inside the word, invisible to the subcommand scan: %s (hard-deny)", w, seg), true
			}
		}
	}
	return "", false
}

// --- class 2c: env-option fail-closed (stripEnvPrefix boundary) --------------

// envOptionPrefix denies a run_shell segment whose env prefix is
// followed by an OPTION word: stripEnvPrefix removes leading NAME=
// assignments and at most one literal `env`, but an env option (`-i`,
// `-u NAME`, `--`) rewires the child environment/argv in ways the
// lexical scan cannot follow — `echo push | env -i xargs git` made
// `-i` the argv[0] and hid the bridge (round-4 B-F1). The segment is
// UNIDENTIFIABLE: uncertainty denies.
//
// Boundary: `env FOO=bar git status` (assignments then a plain
// argv[0]) strips cleanly and stays identifiable through the normal
// classes; `env -- xargs git` denies via the exec-intermediary word
// scan regardless (that class runs first, so the tripwire reason
// wins).
func envOptionPrefix(tool string, args json.RawMessage) (string, bool) {
	if tool != "run_shell" {
		return "", false
	}
	cmd := stringValue(rawJSON(args, "command"))
	for _, seg := range commandSegments(cmd) {
		// Mirror stripEnvPrefix's grammar, tracking whether any prefix
		// word was consumed and what follows it.
		fields := strings.Fields(seg)
		i, consumed, strippedEnv := 0, false, false
		for i < len(fields) {
			w := fields[i]
			if envAssignRe.MatchString(w) {
				i++
				consumed = true
				continue
			}
			if w == "env" && !strippedEnv {
				strippedEnv = true
				i++
				consumed = true
				continue
			}
			break
		}
		if consumed && i < len(fields) && strings.HasPrefix(fields[i], "-") {
			return fmt.Sprintf("env option %q after the env prefix: the child environment/argv is unidentifiable lexically (uncertainty denies): %s", fields[i], seg), true
		}
	}
	return "", false
}

// --- class 3: confinement-escape path shapes --------------------------------

// confinementEscape denies traversal shapes provable from args alone.
func confinementEscape(tool string, args json.RawMessage) (string, bool) {
	if tool == "read" || tool == "write" || tool == "edit" {
		if p := stringValue(rawJSON(args, "path")); hasDotDotSegment(p) {
			return fmt.Sprintf("path traversal %q escapes confinement (hard-deny)", p), true
		}
	}
	if tool == "run_shell" {
		if wd := stringValue(rawJSON(args, "workdir")); hasDotDotSegment(wd) {
			return fmt.Sprintf("workdir traversal %q escapes confinement (hard-deny)", wd), true
		}
		cmd := stringValue(rawJSON(args, "command"))
		for _, w := range strings.Fields(cmd) {
			if isTraversalWord(w) {
				return fmt.Sprintf("traversal-shaped command word %q (hard-deny)", w), true
			}
		}
	}
	return "", false
}

// hasDotDotSegment reports whether any /-separated segment is "..".
func hasDotDotSegment(p string) bool {
	if p == "" {
		return false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// isTraversalWord matches command words shaped like climbing out:
// "..", "../x", "a/../b".
func isTraversalWord(w string) bool {
	return w == ".." || strings.HasPrefix(w, "../") || strings.Contains(w, "/../")
}

// --- class 4: sandbox-mode escalation ----------------------------------------

// sandboxEscalation denies the closed escalation vocabulary.
func sandboxEscalation(tool string, args json.RawMessage) (string, bool) {
	if tool == "run_shell" {
		cmd := stringValue(rawJSON(args, "command"))
		for _, w := range strings.Fields(cmd) {
			if strings.HasPrefix(w, "-") && strings.Contains(strings.ToLower(w), "sandbox") {
				return fmt.Sprintf("sandbox escalation: command word %q requests a sandbox override (hard-deny)", w), true
			}
			if m := envAssignRe.FindStringSubmatch(w); m != nil && strings.ToLower(m[1]) == "sandbox" {
				return fmt.Sprintf("sandbox escalation: env prefix %q requests a sandbox override (hard-deny)", w), true
			}
		}
	}
	if key, ok := sandboxOverrideKey(args); ok {
		return fmt.Sprintf("sandbox escalation: override key %q does not exist on the tool surface (hard-deny)", key), true
	}
	if containsDangerFullAccess(args) {
		return "sandbox escalation: args literally request danger-full-access (hard-deny)", true
	}
	return "", false
}

// sandboxOverrideKey walks args (any depth) for an object key that
// normalizes to the closed override-key vocabulary.
func sandboxOverrideKey(args json.RawMessage) (string, bool) {
	var walk func(v any) (string, bool)
	walk = func(v any) (string, bool) {
		switch t := v.(type) {
		case map[string]any:
			for k, sub := range t {
				if isSandboxOverrideKey(k) {
					return k, true
				}
				if key, ok := walk(sub); ok {
					return key, true
				}
			}
		case []any:
			for _, sub := range t {
				if key, ok := walk(sub); ok {
					return key, true
				}
			}
		}
		return "", false
	}
	var root any
	if err := json.Unmarshal(args, &root); err != nil {
		return "", false
	}
	return walk(root)
}

func isSandboxOverrideKey(k string) bool {
	norm := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(k))
	switch norm {
	case "sandbox", "sandboxmode", "sandboxpermissions":
		return true
	}
	return false
}

// containsDangerFullAccess reports whether any string value anywhere
// in args contains the literal escalation token.
func containsDangerFullAccess(args json.RawMessage) bool {
	var walk func(v any) bool
	walk = func(v any) bool {
		switch t := v.(type) {
		case string:
			return strings.Contains(t, "danger-full-access")
		case map[string]any:
			for _, sub := range t {
				if walk(sub) {
					return true
				}
			}
		case []any:
			for _, sub := range t {
				if walk(sub) {
					return true
				}
			}
		}
		return false
	}
	var root any
	if err := json.Unmarshal(args, &root); err != nil {
		return false
	}
	return walk(root)
}
