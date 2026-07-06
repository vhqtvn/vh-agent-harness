package execro

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestClassify covers every classification branch called out in the exec-ro
// execution brief: valid git readonly, git mutations, external/relative/in-repo
// -C, valid non-git readonly, invalid/mutating non-git, and shell metacharacters.
//
// repoRoot is a t.TempDir() absolute path; the in-repo -C case uses a lexical
// subdir of it (classifyPath is purely lexical, so the subdir need not exist
// on disk).
func TestClassify(t *testing.T) {
	repoRoot := t.TempDir()
	repoSub := filepath.Join(repoRoot, "sub")

	tests := []struct {
		name string
		cmd  string
		want bool // true = ALLOW, false = DENY
	}{
		// Valid git readonly.
		{"git log ALLOW", "git log", true},
		{"git log --oneline ALLOW", "git log --oneline", true},
		{"git --no-pager show ALLOW", "git --no-pager show", true},
		{"git --no-pager log --oneline ALLOW", "git --no-pager log --oneline", true},
		{"git status ALLOW", "git status", true},
		{"git diff ALLOW", "git diff", true},
		{"git --help info ALLOW", "git --help", true},
		{"git --version info ALLOW", "git --version", true},

		// Git mutations (default-deny).
		{"git commit DENY", "git commit", false},
		{"git rm DENY", "git rm", false},
		{"git push DENY", "git push", false},
		{"git reset DENY", "git reset", false},
		{"git checkout DENY", "git checkout", false},
		{"git --no-pager commit DENY (mutation past global flag)", "git --no-pager commit", false},

		// Config/exec-affecting git globals MUST hard-deny before the readonly
		// verb is ever allowed (exec-ro is allowlisted in opencode.jsonc, so it
		// cannot prompt; these globals can execute arbitrary programs via git
		// config / helper binaries). See TestClassify_ConfigGlobalDeny for the
		// notice-naming assertions. B-F1 regression: the original bypass ran
		// `git -c diff.external=/usr/bin/touch diff` and reached the verb allow.
		{"git -c diff.external=/usr/bin/touch diff DENY (B-F1)", "git -c diff.external=/usr/bin/touch diff", false},
		{"git -c core.editor=/usr/bin/vim commit DENY", "git -c core.editor=/usr/bin/vim commit", false},
		{"git -c alias.x='!/usr/bin/touch' log DENY", "git -c alias.x='!/usr/bin/touch' log", false},
		{"git --config-env=diff.external=MYVAR diff DENY", "git --config-env=diff.external=MYVAR diff", false},
		{"git --exec-path diff DENY", "git --exec-path diff", false},
		{"git --exec-path=/x diff DENY", "git --exec-path=/x diff", false},

		// External -C (default-deny + notice).
		{"git -C /external status DENY", "git -C /external status", false},
		{"git -C ../ diff DENY (relative)", "git -C ../ diff", false},
		{"git --git-dir=/external/x status DENY", "git --git-dir=/external/x status", false},

		// Relative -C (default-deny + notice).
		{"git -C ./sub status DENY (relative)", "git -C ./sub status", false},
		{"git -C . commit DENY (relative)", "git -C . commit", false},

		// In-project -C (absolute path under repoRoot) ALLOW.
		{"git -C <repoRoot> status ALLOW", "git -C " + repoRoot + " status", true},
		{"git -C <repoRoot/sub> log ALLOW", "git -C " + repoSub + " log", true},

		// Valid non-git readonly.
		{"ls -la ALLOW", "ls -la", true},
		{"ls ALLOW", "ls", true},
		{"jq . foo.json ALLOW", "jq . foo.json", true},
		{"grep foo bar ALLOW", "grep foo bar", true},
		{"rg pattern ALLOW", "rg pattern", true},
		{"wc -l file ALLOW", "wc -l file", true},
		{"head -n 5 file ALLOW", "head -n 5 file", true},
		{"find . -name x ALLOW", "find . -name x", true},

		// Invalid/mutating non-git (default-deny).
		{"npm install DENY", "npm install", false},
		{"rm foo DENY", "rm foo", false},
		{"curl http://x DENY", "curl http://x", false},
		{"python script.py DENY", "python script.py", false},

		// Shell metacharacters (conservative deny).
		{"git log | grep foo DENY (pipe)", "git log | grep foo", false},
		{"echo $(pwd) DENY (dollar)", "echo $(pwd)", false},
		{"cat a > b DENY (redirect)", "cat a > b", false},
		{"a && b DENY (ampersand)", "a && b", false},
		{"a ; b DENY (semicolon)", "a ; b", false},
		{"echo hi DENY (no metachar, but echo is readonly) ALLOW", "echo hi", true},

		// Empty.
		{"empty DENY", "", false},
		{"whitespace-only DENY", "   ", false},

		// A-F4: exec_ro.go joins argv with a space before classifying, so a
		// single arg containing a space (e.g. `--format=%H %s`) is re-split by
		// strings.Fields into separate trailing tokens. The security boundary is
		// over-deny: splitting can only expose MORE tokens to the classifier
		// (never hide a mutation), so a readonly verb with space-split args stays
		// a safe ALLOW. These cases lock that space-join behavior.
		{"git log --format=%H %s ALLOW (space-split arg, readonly verb)", "git log --format=%H %s", true},
		{"git log --format=\"%H %s\" ALLOW (quoted space-split arg)", "git log --format=\"%H %s\"", true},

		// b-F1 write-flag: git readonly verbs accept subcommand flags that write
		// a file (--output) or invoke an external program (--ext-diff). exec-ro
		// must DENY them (presence alone is fatal, both the = and bare forms).
		// See TestClassify_WriteFlagDeny for the notice-naming assertions.
		{"git diff --output=README.agent.md DENY (b-F1 = form)", "git diff --output=README.agent.md", false},
		{"git diff --output README.agent.md DENY (b-F1 space form)", "git diff --output README.agent.md", false},
		{"git show --output=/tmp/x DENY (other readonly verb)", "git show --output=/tmp/x", false},
		{"git log --output=foo DENY (other readonly verb)", "git log --output=foo", false},
		{"git diff --ext-diff DENY (external driver)", "git diff --ext-diff", false},
		{"git show --ext-diff=foo DENY (= form of ext-diff)", "git show --ext-diff=foo", false},
		// Controls: write-flags absent, readonly verb stays ALLOW.
		{"git diff HEAD ALLOW (control, no write-flag)", "git diff HEAD", true},
		{"git diff --stat ALLOW (control, --stat is not a write-flag)", "git diff --stat", true},
		{"git diff --output-dir=x ALLOW (control, --output-dir is not --output)", "git diff --output-dir=x", true},

		// grep -O / --open-files-in-pager: runs a pager binary over the matching
		// FILES (prompt-free exec via the allowlisted path — same class as
		// --ext-diff). Presence alone is fatal across bare -O (default pager),
		// the stuck short -O<binary> form (verified git accepts it), bare
		// --open-files-in-pager (default pager), and --open-files-in-pager=<binary>.
		// See TestClassify_WriteFlagDeny for the notice-naming assertions.
		{"git grep --open-files-in-pager=/usr/bin/touch foo DENY (= form)", "git grep --open-files-in-pager=/usr/bin/touch foo", false},
		{"git grep --open-files-in-pager foo DENY (bare long, default pager)", "git grep --open-files-in-pager foo", false},
		{"git grep -O foo DENY (short bare form, default pager)", "git grep -O foo", false},
		{"git grep -O/usr/bin/touch foo DENY (short stuck form)", "git grep -O/usr/bin/touch foo", false},
		{"git grep -Oecho foo DENY (short stuck form, harmless binary)", "git grep -Oecho foo", false},

		// textconv/filter family: cat-file and diff/show/log run configured
		// diff/filter driver programs (config-gated external exec, same class
		// as --ext-diff). The DISABLE forms stay ALLOW (controls below).
		{"git cat-file --textconv HEAD DENY (textconv exec)", "git cat-file --textconv HEAD", false},
		{"git cat-file --filters HEAD DENY (filter exec)", "git cat-file --filters HEAD", false},
		{"git diff --textconv DENY (textconv on diff family)", "git diff --textconv", false},
		{"git log --textconv DENY (textconv on log)", "git log --textconv", false},

		// Controls: denying -O across readonly verbs must NOT false-trip on
		// grep's normal flags, on diff's -O<orderfile> (a read-only orderfile
		// read — the stuck -O prefix match is grep-scoped on purpose), on the
		// lowercase -o (only-matching), or on the DISABLE forms.
		{"git grep foo ALLOW (control, grep is readonly, no exec flag)", "git grep foo", true},
		{"git grep -i foo ALLOW (control, -i is not an exec flag)", "git grep -i foo", true},
		{"git grep -o foo ALLOW (control, lowercase -o is only-matching, not -O)", "git grep -o foo", true},
		{"git grep -l foo ALLOW (control, -l is files-with-matches)", "git grep -l foo", true},
		{"git log --oneline ALLOW (control, -O absent, --oneline unaffected)", "git log --oneline", true},
		{"git diff -Oorderfile.txt ALLOW (control, diff -O<orderfile> is read-only)", "git diff -Oorderfile.txt", true},
		{"git diff --no-textconv ALLOW (control, DISABLE form is safe)", "git diff --no-textconv", true},
		{"git diff --no-ext-diff ALLOW (control, DISABLE form is safe)", "git diff --no-ext-diff", true},

		// D-F1: non-git readonly binaries (find/sort/sed) accept flags that
		// delete files, execute a program, or write output to a file. exec-ro
		// MUST DENY them (presence alone is fatal). See
		// TestClassify_NonGitWriteFlagDeny for the notice-naming assertions.
		// Note: `\;`-terminated forms (find -execdir true \;, find -ok rm {} \;)
		// trip the `;` metachar gate first — they DENY either way; the clean
		// (no-`\;`) flag-naming cases live in TestClassify_NonGitWriteFlagDeny.
		{"find . -delete DENY (D-F1)", "find . -delete", false},
		{"find . -exec rm {} + DENY (D-F1, + is not a metachar)", "find . -exec rm {} +", false},
		{"find . -execdir true \\; DENY (D-F1, \\; trips ; metachar first)", "find . -execdir true \\;", false},
		{"find . -fls /path DENY (D-F1)", "find . -fls /path", false},
		{"find . -ok rm {} \\; DENY (D-F1, \\; trips ; metachar first)", "find . -ok rm {} \\;", false},
		{"sort -o out in DENY (D-F1)", "sort -o out in", false},
		{"sort --output=out in DENY (D-F1)", "sort --output=out in", false},
		{"sed -n '10p' f DENY (D-F1, no --sandbox)", "sed -n '10p' f", false},
		{"sed -n --sandbox -i f DENY (D-F1, -i not blocked by --sandbox)", "sed -n --sandbox -i f", false},
		{"sed -n -i f DENY (D-F1, both missing --sandbox AND -i)", "sed -n -i f", false},
		// Controls: non-git read-only flags stay ALLOW.
		{"find . -name '*.go' ALLOW (control, D-F1)", "find . -name '*.go'", true},
		{"find . -type f ALLOW (control, D-F1)", "find . -type f", true},
		{"sort -n f ALLOW (control, D-F1)", "sort -n f", true},
		{"sort -k2 f ALLOW (control, D-F1)", "sort -k2 f", true},
		{"sed -n --sandbox '10,20p' f ALLOW (control, D-F1)", "sed -n --sandbox '10,20p' f", true},
		{"sed -n --sandbox 's/x/y/' f ALLOW (control, D-F1)", "sed -n --sandbox 's/x/y/' f", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.cmd, repoRoot)
			if got.Allow != tt.want {
				t.Errorf("Classify(%q) Allow = %v, want %v; notice=%q", tt.cmd, got.Allow, tt.want, got.Notice)
			}
			// A DENIED verdict MUST carry a non-empty notice (opencode cannot
			// prompt for an allowlisted exec-ro invocation, so the notice is the
			// operator's only feedback).
			if !got.Allow && got.Notice == "" {
				t.Errorf("Classify(%q) denied with EMPTY notice — exec-ro must explain every deny", tt.cmd)
			}
		})
	}
}

// TestClassify_DenyNoticeFooter asserts every deny notice carries the denyFooter
// explanation (the allowlist→deny asymmetry + bare-command suggestion). This is
// the operator-facing contract: exec-ro can only hard-deny, so the notice must
// explain WHY and point at the alternative.
func TestClassify_DenyNoticeFooter(t *testing.T) {
	repoRoot := t.TempDir()
	denied := []string{
		"git commit",
		"npm install",
		"git log | grep foo",
		"git -C /external status",
		"nonexistent-binary",
	}
	for _, cmd := range denied {
		got := Classify(cmd, repoRoot)
		if got.Allow {
			t.Errorf("Classify(%q) unexpectedly allowed", cmd)
			continue
		}
		if !strings.Contains(got.Notice, "bare command") {
			t.Errorf("Classify(%q) deny notice missing bare-command suggestion: %q", cmd, got.Notice)
		}
	}
}

// TestClassify_ConfigGlobalDeny locks the B-F1 fix: config/exec-affecting git
// globals (-c, --config-env, --exec-path) MUST hard-deny under exec-ro (which is
// allowlisted in opencode.jsonc and so cannot prompt), and the deny notice MUST
// name the offending flag so the operator knows which global tripped it. The
// notice must also carry the standard denyFooter bare-command suggestion.
//
// This is the exec-ro-ONLY divergence from shell-guard-core.js: bare
// `git -c ...` is path-blind in shell-guard and prompts through the normal
// permission table, so the JS global-flag registry is intentionally left
// unrestricted there. The fix lives entirely in this Go classifier.
func TestClassify_ConfigGlobalDeny(t *testing.T) {
	repoRoot := t.TempDir()
	cases := []struct {
		cmd      string
		wantFlag string // the offending flag name that must appear in the notice
	}{
		{"git -c diff.external=/usr/bin/touch diff", "-c"},
		{"git -c core.editor=/usr/bin/vim commit", "-c"},
		{"git -c alias.x='!/usr/bin/touch' log", "-c"},
		{"git --config-env=diff.external=MYVAR diff", "--config-env"},
		{"git --config-env diff.external=MYVAR diff", "--config-env"},
		{"git --exec-path diff", "--exec-path"},
		{"git --exec-path=/x diff", "--exec-path"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			got := Classify(tc.cmd, repoRoot)
			if got.Allow {
				t.Fatalf("Classify(%q) ALLOWED — config/exec global must hard-deny under exec-ro", tc.cmd)
			}
			if !strings.Contains(got.Notice, "'"+tc.wantFlag+"'") {
				t.Errorf("Classify(%q) deny notice does not name the offending flag %q: %q", tc.cmd, tc.wantFlag, got.Notice)
			}
			if !strings.Contains(got.Notice, "config/exec-affecting") {
				t.Errorf("Classify(%q) deny notice missing the config/exec-affecting rationale: %q", tc.cmd, got.Notice)
			}
			if !strings.Contains(got.Notice, "bare command") {
				t.Errorf("Classify(%q) deny notice missing bare-command suggestion: %q", tc.cmd, got.Notice)
			}
		})
	}
}

// TestClassify_WriteFlagDeny locks the b-F1 write-flag fix: git readonly verbs
// (diff/show/log/...) accept SUBCOMMAND flags that write a file (--output, the
// prompt-free write vector) or invoke an external program (--ext-diff). Because
// exec-ro is allowlisted in opencode.jsonc (opencode never prompts for it), the
// presence of either flag MUST hard-deny before the readonly-verb ALLOW, and the
// deny notice MUST name the offending flag, state the read-only-contract
// violation, and carry the standard denyFooter bare-command suggestion. Both the
// `--flag=value` attached form and the bare `--flag` (space-separated value) form
// are denied (presence alone is fatal), mirroring the B-F1 config-global pattern.
//
// This is a verb-level HEURISTIC DENYLIST of the KNOWN write vectors, not a
// per-verb safe-flag allowlist (deferred); exec-sandbox is the authoritative
// layer for the long tail of unknown future flags.
func TestClassify_WriteFlagDeny(t *testing.T) {
	repoRoot := t.TempDir()
	cases := []struct {
		cmd      string
		wantFlag string // the offending flag name that must appear in the notice
	}{
		// --output (the b-F1 prompt-free write vector): both forms, multiple verbs.
		{"git diff --output=README.agent.md", "--output"},
		{"git diff --output README.agent.md", "--output"},
		{"git show --output=/tmp/x", "--output"},
		{"git log --output=foo", "--output"},
		// --ext-diff (external diff driver can execute a configured program).
		{"git diff --ext-diff", "--ext-diff"},
		{"git show --ext-diff=foo", "--ext-diff"},
		{"git log --ext-diff", "--ext-diff"},
		// --open-files-in-pager (grep: runs a pager binary over matching files).
		// Both the = attached form and the bare (default-pager) form.
		{"git grep --open-files-in-pager=/usr/bin/touch foo", "--open-files-in-pager"},
		{"git grep --open-files-in-pager foo", "--open-files-in-pager"},
		{"git grep --open-files-in-pager", "--open-files-in-pager"},
		// -O (grep short form): bare (default pager) and the stuck -O<binary>
		// form (verified git accepts and execs it).
		{"git grep -O foo", "-O"},
		{"git grep -O/usr/bin/touch foo", "-O"},
		{"git grep -Oecho foo", "-O"},
		// --textconv / --filters (cat-file + diff/show/log family): configured
		// diff/filter driver programs — config-gated external exec, same class
		// as --ext-diff. Found by the completeness sweep across all 12 readonly
		// verbs (the reviewer's original "exactly 4" claim missed these).
		{"git cat-file --textconv HEAD", "--textconv"},
		{"git cat-file --filters HEAD", "--filters"},
		{"git diff --textconv", "--textconv"},
		{"git log --textconv", "--textconv"},
		{"git show --textconv", "--textconv"},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			got := Classify(tc.cmd, repoRoot)
			if got.Allow {
				t.Fatalf("Classify(%q) ALLOWED — write/exec subcommand flag must hard-deny under exec-ro", tc.cmd)
			}
			if !strings.Contains(got.Notice, "'"+tc.wantFlag+"'") {
				t.Errorf("Classify(%q) deny notice does not name the offending flag %q: %q", tc.cmd, tc.wantFlag, got.Notice)
			}
			if !strings.Contains(got.Notice, "read-only contract") {
				t.Errorf("Classify(%q) deny notice missing the read-only-contract rationale: %q", tc.cmd, got.Notice)
			}
			if !strings.Contains(got.Notice, "bare command") {
				t.Errorf("Classify(%q) deny notice missing bare-command suggestion: %q", tc.cmd, got.Notice)
			}
		})
	}
}

// TestClassify_WriteFlagNoFalsePositive locks the controls: a readonly verb
// without a denylisted write-flag MUST still ALLOW. This guards against an
// over-broad flag match (e.g. treating --stat, --output-dir, or a bare ref/path
// as a write-flag) regressing the readonly surface.
func TestClassify_WriteFlagNoFalsePositive(t *testing.T) {
	repoRoot := t.TempDir()
	allow := []string{
		"git diff HEAD",
		"git diff --stat",
		"git diff --output-dir=x", // --output-dir is NOT --output
		"git show HEAD",
		"git log --oneline",
		"git diff --name-only",
		// grep normal flags must not be over-denied.
		"git grep foo",
		"git grep -i foo",
		"git grep -o foo", // lowercase -o (only-matching), NOT capital -O
		"git grep -l foo",
		"git grep -e foo -- bar", // -e pattern + pathspec sep, no exec flag
		// diff's -O<orderfile> is a read-only orderfile read (the grep-scoped
		// stuck -O match must NOT fire on diff).
		"git diff -Oorderfile.txt",
		// DISABLE forms of the exec-capable flags are safe and stay ALLOWED.
		"git diff --no-textconv",
		"git diff --no-ext-diff",
	}
	for _, cmd := range allow {
		got := Classify(cmd, repoRoot)
		if !got.Allow {
			t.Errorf("Classify(%q) denied — control case must ALLOW (false positive on write-flag scan?): %q", cmd, got.Notice)
		}
	}
}

// TestClassify_NonGitWriteFlagDeny locks the D-F1 fix: the non-git readonly
// binaries (find/sort/sed) accept flags that delete files, execute a program,
// or write output to a file. Because exec-ro is allowlisted in opencode.jsonc
// (opencode never prompts for it), the presence of such a flag MUST hard-deny
// after the readonly pattern match, and the deny notice MUST name the offending
// flag, state the read-only-contract violation, and carry the standard
// denyFooter bare-command suggestion. sed additionally requires --sandbox
// (which disables its e/r/w commands) AND independently denies -i (which
// --sandbox does NOT disable). Mirrors TestClassify_WriteFlagDeny for the git
// side. This is a binary-level HEURISTIC DENYLIST of the KNOWN non-git
// write/exec vectors; exec-sandbox is the authoritative layer for the long
// tail of unknown flags.
func TestClassify_NonGitWriteFlagDeny(t *testing.T) {
	repoRoot := t.TempDir()
	cases := []struct {
		cmd         string
		wantSubstr  string // a substring the notice MUST contain (flag or message)
		sandboxCase bool   // true = sed-without-sandbox case (assert "--sandbox" required message)
	}{
		// find write/exec flags (clean cases WITHOUT the `\;` terminator, which
		// would trip the `;` metachar gate first and bypass this scan).
		{"find . -delete", "'-delete'", false},
		{"find . -exec rm {} +", "'-exec'", false}, // `+` is NOT a metachar; reaches the scan
		{"find . -execdir true", "'-execdir'", false},
		{"find . -fls /path", "'-fls'", false},
		{"find . -ok echo {}", "'-ok'", false},
		{"find . -okdir true", "'-okdir'", false},
		{"find . -fprint /path", "'-fprint'", false},
		{"find . -fprint0 /path", "'-fprint0'", false},
		{"find . -fprintf /path %p", "'-fprintf'", false},
		// sort write flag.
		{"sort -o out in", "'-o'", false},
		{"sort --output out in", "'--output'", false},
		{"sort --output=out in", "'--output'", false},
		// sed: --sandbox required (absent) — fires first regardless of -i.
		{"sed -n '10p' f", "--sandbox", true},
		{"sed -n 'w /path' f", "--sandbox", true}, // the b-F1-style sed write vector
		{"sed -n 'e cmd' f", "--sandbox", true},   // the sed exec vector
		{"sed -n -i f", "--sandbox", true},        // both missing; sandbox check fires first
		// sed: --sandbox present but -i present (the -i is NOT disabled by --sandbox).
		{"sed -n --sandbox -i f", "'-i'", false},
		{"sed -n --sandbox --in-place f", "'-i'", false},
	}
	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			got := Classify(tc.cmd, repoRoot)
			if got.Allow {
				t.Fatalf("Classify(%q) ALLOWED — non-git write/exec flag must hard-deny under exec-ro", tc.cmd)
			}
			if !strings.Contains(got.Notice, tc.wantSubstr) {
				t.Errorf("Classify(%q) deny notice does not contain expected substring %q: %q", tc.cmd, tc.wantSubstr, got.Notice)
			}
			if !strings.Contains(got.Notice, "read-only contract") {
				t.Errorf("Classify(%q) deny notice missing the read-only-contract rationale: %q", tc.cmd, got.Notice)
			}
			if tc.sandboxCase {
				if !strings.Contains(got.Notice, "requires --sandbox") {
					t.Errorf("Classify(%q) deny notice missing the '--sandbox required' message: %q", tc.cmd, got.Notice)
				}
			}
			if !strings.Contains(got.Notice, "bare command") {
				t.Errorf("Classify(%q) deny notice missing bare-command suggestion: %q", tc.cmd, got.Notice)
			}
		})
	}
}

// TestClassify_NonGitWriteFlagNoFalsePositive locks the controls: a non-git
// readonly command WITHOUT a denylisted write/exec flag MUST still ALLOW. This
// guards against an over-broad flag match (e.g. treating find's `-name`/`-type`,
// sort's `-n`/`-k`, or a bare path as a write-flag) regressing the non-git
// readonly surface, and against the sed --sandbox requirement false-tripping on
// a safe `sed -n --sandbox ...` invocation.
func TestClassify_NonGitWriteFlagNoFalsePositive(t *testing.T) {
	repoRoot := t.TempDir()
	allow := []string{
		// find read-only predicates.
		"find . -name x",
		"find . -name '*.go'",
		"find . -type f",
		"find . -maxdepth 2 -name x",
		"find . -print",
		// sort read-only.
		"sort -n f",
		"sort -k2 f",
		"sort -r -u f",
		// sed with --sandbox (the only ALLOWED sed form). Note: the readonly
		// group entry is `sed -n *`, so --sandbox must come AFTER -n.
		"sed -n --sandbox '10,20p' f",
		"sed -n --sandbox 's/x/y/' f",
	}
	for _, cmd := range allow {
		got := Classify(cmd, repoRoot)
		if !got.Allow {
			t.Errorf("Classify(%q) denied — control case must ALLOW (false positive on non-git write-flag scan?): %q", cmd, got.Notice)
		}
	}
}

// TestClassifyArgs locks the B1 fix: the argv-aware ClassifyArgs entrypoint
// (target + args) MUST run the per-binary write/exec flag denylist over the FULL
// argv slice, mirroring what Classify catches via strings.Fields. This is the
// regression guard for exec-sandbox's best-effort graceful-skip fallback, which
// previously called Classify(target) and lost the args — so `find . -delete`
// classified only the bare `find` (ALLOW) and the mutating payload executed
// unclassified via runDirect with NO kernel isolation. ClassifyArgs preserves
// the real argv so the denylist fires.
//
// The mutating cases (all MUST DENY) mirror the strings.Fields-driven cases in
// TestClassify / TestClassify_NonGitWriteFlagDeny; the ALLOW controls mirror
// TestClassify_NonGitWriteFlagNoFalsePositive. The refactor-equivalence
// sub-test locks that Classify("find . -delete") and ClassifyArgs("find", [...])
// return the SAME verdict (the refactor extracted a shared token-level core, so
// the two entrypoints must agree on identical argv).
func TestClassifyArgs(t *testing.T) {
	repoRoot := t.TempDir()

	t.Run("deny_mutating_args", func(t *testing.T) {
		denyCases := []struct {
			name   string
			target string
			args   []string
		}{
			// find: write/exec flags (the exact B1 reachable bypass payload).
			{"find -delete", "find", []string{".", "-delete"}},
			{"find -exec (plus terminator)", "find", []string{".", "-exec", "rm", "{}", "+"}},
			{"find -execdir", "find", []string{".", "-execdir", "true"}},
			{"find -fls", "find", []string{".", "-fls", "/path"}},
			// sort: write flag.
			{"sort -o", "sort", []string{"-o", "out", "in"}},
			{"sort --output", "sort", []string{"--output", "out", "in"}},
			{"sort --output=attached", "sort", []string{"--output=out", "in"}},
			// git: readonly verb + write/exec SUBCOMMAND flag (b-F1 vector).
			{"git diff --output=attached", "git", []string{"diff", "--output=x"}},
			{"git diff --output space", "git", []string{"diff", "--output", "x"}},
			{"git diff --ext-diff", "git", []string{"diff", "--ext-diff"}},
			{"git grep -O (pager exec)", "git", []string{"grep", "-O", "foo"}},
			// sed: --sandbox required (absent fires first); and the b-F1-style
			// sed write vector via the `w` command (only reachable as an arg
			// because the space in 'w /path' is a literal inside the arg, not
			// a token boundary in the argv form).
			{"sed without --sandbox", "sed", []string{"-n", "10p", "f"}},
			{"sed w-command write vector", "sed", []string{"-n", "w /path", "f"}},
			{"sed e-command exec vector", "sed", []string{"-n", "e cmd", "f"}},
			{"sed -i not blocked by --sandbox", "sed", []string{"-n", "--sandbox", "-i", "f"}},
			// git mutations (default-deny at the verb level).
			{"git commit", "git", []string{"commit"}},
			{"git push", "git", []string{"push"}},
		}
		for _, tc := range denyCases {
			t.Run(tc.name, func(t *testing.T) {
				got := ClassifyArgs(tc.target, tc.args, repoRoot)
				if got.Allow {
					t.Fatalf("ClassifyArgs(%q, %v) ALLOWED — mutating args must hard-deny (B1 bypass)", tc.target, tc.args)
				}
				if got.Notice == "" {
					t.Fatalf("ClassifyArgs(%q, %v) denied with EMPTY notice — exec-ro must explain every deny", tc.target, tc.args)
				}
			})
		}
	})

	t.Run("allow_readonly_controls", func(t *testing.T) {
		allowCases := []struct {
			name   string
			target string
			args   []string
		}{
			{"find readonly", "find", []string{".", "-name", "*.go"}},
			{"find readonly type", "find", []string{".", "-type", "f"}},
			{"sort readonly", "sort", []string{"-n", "f"}},
			{"git status", "git", []string{"status"}},
			{"git log oneline", "git", []string{"log", "--oneline"}},
			{"git diff HEAD", "git", []string{"diff", "HEAD"}},
			{"sed with --sandbox", "sed", []string{"-n", "--sandbox", "10,20p", "f"}},
		}
		for _, tc := range allowCases {
			t.Run(tc.name, func(t *testing.T) {
				got := ClassifyArgs(tc.target, tc.args, repoRoot)
				if !got.Allow {
					t.Errorf("ClassifyArgs(%q, %v) denied — control case must ALLOW (false positive?): %q", tc.target, tc.args, got.Notice)
				}
			})
		}
	})

	t.Run("refactor_equivalence_Classify_vs_ClassifyArgs", func(t *testing.T) {
		// The refactor extracted a shared token-level core (classifyTokens);
		// Classify and ClassifyArgs must return IDENTICAL verdicts for argv
		// that split identically. `find . -delete` splits via strings.Fields
		// into ["find", ".", "-delete"], which equals the ClassifyArgs target
		// "find" + args [".", "-delete"]. Both must DENY (the B1 payload). We
		// compare both Allow AND Notice so the refactor cannot silently change
		// the notice wording either.
		strVerdict := Classify("find . -delete", repoRoot)
		argsVerdict := ClassifyArgs("find", []string{".", "-delete"}, repoRoot)
		if strVerdict.Allow != argsVerdict.Allow {
			t.Fatalf("refactor equivalence broken: Classify(\"find . -delete\").Allow=%v but ClassifyArgs(\"find\", [...]).Allow=%v",
				strVerdict.Allow, argsVerdict.Allow)
		}
		if strVerdict.Allow {
			t.Fatalf("refactor equivalence check used a case that should DENY but both ALLOWED — pick a mutating payload")
		}
		if strVerdict.Notice != argsVerdict.Notice {
			t.Fatalf("refactor equivalence broken: notices differ.\nClassify   : %q\nClassifyArgs: %q", strVerdict.Notice, argsVerdict.Notice)
		}
	})
}
