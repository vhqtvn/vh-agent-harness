package permission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner is a Runner double that returns canned output without spawning
// node. It records what it was called with so tests can assert arg wiring.
type fakeRunner struct {
	stdout     []byte
	stderr     []byte
	exitCode   int
	err        error
	calls      int
	lastArgs   []string
	lastCwd    string
	lastBudget time.Duration
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string, cwd string, timeout time.Duration) ([]byte, []byte, int, error) {
	f.calls++
	f.lastArgs = append([]string(nil), args...)
	f.lastCwd = cwd
	f.lastBudget = timeout
	return f.stdout, f.stderr, f.exitCode, f.err
}

// newMappingHook builds a ShellGuardHook wired to a fake runner with validate
// bypassed, so the JSON->Action mapping can be asserted without a real node.
func newMappingHook(t *testing.T, runner Runner) *ShellGuardHook {
	t.Helper()
	return NewShellGuardHook(t.TempDir(), WithRunner(runner), withBypassValidate())
}

// --- JSON -> Action mapping (fake runner, no node needed) --------------------

func TestShellGuardHook_AllowMapping(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"allow","reason":""}` + "\n")}
	h := newMappingHook(t, r)
	act, reason, err := h.Evaluate(context.Background(), []string{"echo", "hello"})
	if err != nil || act != Allow || reason != "" {
		t.Fatalf("got (%s,%q,%v) want (Allow,\"\",nil)", act, reason, err)
	}
	if r.calls != 1 || len(r.lastArgs) < 2 || r.lastArgs[1] != "echo" {
		t.Errorf("runner args wrong: %v", r.lastArgs)
	}
}

func TestShellGuardHook_DenyMapping(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"deny","reason":"blocked: bad"}`)}
	h := newMappingHook(t, r)
	act, reason, err := h.Evaluate(context.Background(), []string{"rm", "-rf"})
	if err != nil || act != Deny || reason != "blocked: bad" {
		t.Fatalf("got (%s,%q,%v) want (Deny,\"blocked: bad\",nil)", act, reason, err)
	}
}

func TestShellGuardHook_AskMapping(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"ask","reason":"git mutation"}`)}
	h := newMappingHook(t, r)
	act, reason, err := h.Evaluate(context.Background(), []string{"git", "commit"})
	if err != nil || act != Ask || reason != "git mutation" {
		t.Fatalf("got (%s,%q,%v) want (Ask,\"git mutation\",nil)", act, reason, err)
	}
}

func TestShellGuardHook_ExitNonZero_Denies(t *testing.T) {
	r := &fakeRunner{exitCode: 2, stderr: []byte("engine fault\n")}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil {
		t.Fatalf("exit2 must yield (Deny,err); got (%s,%v)", act, err)
	}
	if !strings.Contains(err.Error(), "exited 2") {
		t.Errorf("err should mention exited 2; got %v", err)
	}
}

func TestShellGuardHook_RunnerError_Denies(t *testing.T) {
	r := &fakeRunner{err: context.DeadlineExceeded}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil {
		t.Fatalf("runner err must yield (Deny,err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_MalformedJSON_Denies(t *testing.T) {
	r := &fakeRunner{stdout: []byte("not json at all\n")}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed must yield (Deny,malformed err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_UnknownAction_Denies(t *testing.T) {
	r := &fakeRunner{stdout: []byte(`{"action":"maybe","reason":"?"}`)}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("unknown action must yield (Deny,unknown err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_EmptyStdout_Denies(t *testing.T) {
	r := &fakeRunner{stdout: nil}
	h := newMappingHook(t, r)
	act, _, err := h.Evaluate(context.Background(), []string{"x"})
	if act != Deny || err == nil {
		t.Fatalf("empty stdout must yield (Deny,err); got (%s,%v)", act, err)
	}
}

// --- Availability (real validate, no bypass) ---------------------------------

func TestShellGuardHook_NodeMissing_Denies(t *testing.T) {
	// A path that cannot exist: validate runs `node --version` against it,
	// exec fails, bridgeErr is set, and Evaluate returns a loud Deny.
	root := t.TempDir()
	h := NewShellGuardHook(root, WithNodeBin("/nonexistent/node-binary-xyz-12345"))
	if h.bridgeErr == nil {
		t.Fatalf("expected bridgeErr for missing node, got nil")
	}
	act, _, err := h.Evaluate(context.Background(), []string{"echo", "hi"})
	if act != Deny || err == nil {
		t.Fatalf("node-missing must yield (Deny,err); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_EvalMissing_Denies(t *testing.T) {
	// Real node (present in the devcontainer) but a temp root with NO eval.js.
	// validate() reaches the os.Stat step and fails -> loud Deny.
	root := t.TempDir()
	h := NewShellGuardHook(root)
	if h.bridgeErr == nil {
		t.Skipf("node not available in this env (validate passed unexpectedly); skipping eval-missing probe")
	}
	if !strings.Contains(h.bridgeErr.Error(), "eval.js not found") {
		t.Fatalf("expected eval.js-not-found bridgeErr; got %v", h.bridgeErr)
	}
	act, _, err := h.Evaluate(context.Background(), []string{"echo", "hi"})
	if act != Deny || err != h.bridgeErr {
		t.Fatalf("eval-missing must yield (Deny,bridgeErr); got (%s,%v)", act, err)
	}
}

func TestShellGuardHook_NodeMinVersion(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"v18.0.0\n", 18},
		{"v20.11.1\n", 20},
		{"v24.16.0\n", 24},
		{"v8.17.0\n", 8},
		{"v1.22.0\n", 1},
	}
	for _, c := range cases {
		got, err := parseNodeMajor(c.in)
		if err != nil || got != c.want {
			t.Errorf("parseNodeMajor(%q) = (%d,%v), want (%d,nil)", c.in, got, err, c.want)
		}
	}
	if _, err := parseNodeMajor("garbage"); err == nil {
		t.Errorf("parseNodeMajor(garbage) should error")
	}
}

// --- Live bridge: end-to-end node eval.js + WASM + rules ---------------------

func TestShellGuardHook_LiveBridge(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live node bridge in -short mode")
	}
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping live bridge (JSON-mapping unit tests still cover the hook)")
	}

	// Locate the shipped shell-guard corpus (.opencode) by walking up to the
	// module root. templates/core is the canonical corpus the seam installs.
	modRoot := findModuleRoot(t)
	tmplOpencode := filepath.Join(modRoot, "templates", "core", ".opencode")

	// Stage a scratch install: only the files eval.js pulls in.
	//
	// IMPORTANT: scratch MUST live under the repo (modRoot/tmp), NOT under
	// t.TempDir() (which returns a /tmp/... path). The `system-tmp-access`
	// forbidden rule denies ANY command that references /tmp, so any
	// `git -C <scratch> ...` case would be DENIED by system-tmp-access before
	// the git global-flag walker could classify it. Repo-scoped
	// /home/.../tmp/ paths do NOT trip that rule (the `tmp` is preceded by a
	// word char, failing the rule's boundary class). repoRoot() inside
	// eval.js resolves to scratch (plugins/ -> two up), so scratch doubles as
	// commandCwd for the walker.
	scratchParent := filepath.Join(modRoot, "tmp")
	if err := os.MkdirAll(scratchParent, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", scratchParent, err)
	}
	scratch, err := os.MkdirTemp(scratchParent, "sglive-")
	if err != nil {
		t.Fatalf("mkdtemp %s: %v", scratchParent, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(scratch) })
	scratchOpencode := filepath.Join(scratch, ".opencode")
	files := []string{
		"package.json",
		filepath.Join("repo-configs", "allowed-commands.js"),
		filepath.Join("repo-configs", "forbidden-patterns.js"),
		filepath.Join("repo-configs", "forbidden-patterns.core.js"),
		filepath.Join("plugins", "shell-guard-core.js"),
		filepath.Join("plugins", "shell-guard", "eval.js"),
	}
	for _, rel := range files {
		src := filepath.Join(tmplOpencode, filepath.FromSlash(rel))
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read template %s: %v (template not rendered?)", rel, err)
		}
		dst := filepath.Join(scratchOpencode, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}

	// Install the WASM deps. If npm is unavailable/offline, skip (the
	// JSON-mapping unit tests still cover the Go hook).
	npmBin, npmErr := exec.LookPath("npm")
	if npmErr != nil {
		t.Skip("npm not on PATH; skipping live bridge")
	}
	install := exec.Command(npmBin, "install", "--no-audit", "--no-fund")
	install.Dir = scratchOpencode
	if out, err := install.CombinedOutput(); err != nil {
		t.Skipf("npm install failed (offline?): %v\n%s", err, out)
	}

	evalPath := filepath.Join(scratchOpencode, "plugins", "shell-guard", "eval.js")

	// 1. allow: echo hello (echo is in COMMANDS.readonly).
	out, code := runNode(t, nodeBin, evalPath, scratch, "echo", "hello")
	if code != 0 {
		t.Fatalf("echo hello: exit %d, stdout=%q stderr-led; expected exit 0", code, out)
	}
	if act := jsonAction(t, out); act != "allow" {
		t.Errorf("echo hello: action %q, want allow (stdout=%q)", act, out)
	}

	// 2. deny: apt-get install foo (matches the apt-install-ad-hoc rule).
	out, code = runNode(t, nodeBin, evalPath, scratch, "apt-get", "install", "foo")
	if code != 0 {
		t.Fatalf("apt-get install: exit %d, expected exit 0 (deny is a decision, not a fault)", code)
	}
	if act := jsonAction(t, out); act != "deny" {
		t.Errorf("apt-get install: action %q, want deny (stdout=%q)", act, out)
	}

	// 3. End-to-end through the Go hook (real osExecRunner) against the scratch
	// install: proves the full Go -> node -> JSON -> Action path.
	h := NewShellGuardHook(scratch)
	if h.bridgeErr != nil {
		t.Fatalf("hook validate failed against scratch install: %v", h.bridgeErr)
	}
	act, _, err := h.Evaluate(context.Background(), []string{"echo", "hello"})
	if err != nil || act != Allow {
		t.Errorf("hook Evaluate(echo hello) = (%s,%v), want (Allow,nil)", act, err)
	}
	act, _, err = h.Evaluate(context.Background(), []string{"apt-get", "install", "foo"})
	if err != nil || act != Deny {
		t.Errorf("hook Evaluate(apt-get install foo) = (%s,%v), want (Deny,nil)", act, err)
	}

	// git-mutation-bypass regression matrix (Go -> node -> WASM -> evaluate()).
	// The first two anchors are the FP fix: descriptive prose containing git
	// tokens inside echo/printf args is now carved out by the inspector allowIf
	// union (ALLOW_IF_GIT_MUTATION). The remaining anchors pin the
	// load-bearing security invariants that must NOT regress:
	//   - bash -c / sh -c / vh-agent-harness exec / eval evasion still DENIED,
	//   - every chain-guard shape (&&, |, ;, $()) still DENIED,
	//   - git -C <path> <mutation> still DENIED: walkGitGlobals extracts the
	//     verb past any leading global flag and the UNIFORM mutation-slip guard
	//     denies it before the allowlist is consulted,
	//   - the commit-gate carve-out path still ALLOWS end-to-end.
	// Each command is passed as a single-element argv so eval.js's
	// argv.join(" ") yields exactly the intended command string.
	gitCases := []struct {
		name string
		cmd  string
		want Action
	}{
		{
			name: "echo prose with git checkout/status tokens (FP fix)",
			cmd:  `echo "cleanup: git checkout / git status fix"`,
			want: Allow,
		},
		{
			name: "printf prose with git checkout token (FP fix)",
			cmd:  `printf "see git checkout notes"`,
			want: Allow,
		},
		{
			name: "echo nested quotes with git tokens (FP fix)",
			cmd:  `echo "use 'git checkout' then \"git status\""`,
			want: Allow,
		},
		{
			// `command` is a bash builtin that EXECUTES its argument. It is in
			// the shared INSPECTOR_FULL (other rules carve it out for benign
			// `command -v foo` lookups), but the git-mutation carve-out uses a
			// SEPARATE verb set (GIT_MUTATION_INSPECTORS) that EXCLUDES
			// `command`. Otherwise `command git commit -m x` would be carved
			// out at scan #1 (command in command position → allowIf matches)
			// and the git commit would run. This is the most realistic vector
			// (an agent would naturally write `command git commit`).
			name: "command git commit denied (executor verb not a safe inspector)",
			cmd:  `command git commit -m x`,
			want: Deny,
		},
		{
			name: "command git push denied (executor verb not a safe inspector)",
			cmd:  `command git push origin main`,
			want: Deny,
		},
		{
			name: "bash -c git commit denied",
			cmd:  `bash -c 'git commit -m x'`,
			want: Deny,
		},
		{
			name: "sh -c git push denied",
			cmd:  `sh -c 'git push origin main'`,
			want: Deny,
		},
		{
			name: "vh-agent-harness exec bash -c git reset denied (load-bearing evasion)",
			cmd:  `vh-agent-harness exec bash -c 'git reset --hard'`,
			want: Deny,
		},
		{
			name: "eval git checkout denied",
			cmd:  `eval 'git checkout x'`,
			want: Deny,
		},
		{
			name: "echo then && git push denied (chain-guard)",
			cmd:  `echo hi && git push origin`,
			want: Deny,
		},
		{
			name: "cat piped to git commit denied (chain-guard pipe)",
			cmd:  `cat x | git commit -F -`,
			want: Deny,
		},
		{
			name: "echo semicolon git push denied (chain-guard semicolon)",
			cmd:  `echo x; git push`,
			want: Deny,
		},
		{
			name: "echo command-substitution git push denied (chain-guard $())",
			cmd:  `echo $(git push)`,
			want: Deny,
		},
		{
			// Bash process substitution `<(...)` runs the inner command with NO
			// list separator, so `echo x <(git commit ...)` would otherwise be
			// carved out (echo satisfies the inspector carve-out) and the git
			// commit would run. The chain-guard alternation now includes `<(`.
			name: "echo process-sub git commit denied (chain-guard process-sub <())",
			cmd:  `echo x <(git commit -m pwned)`,
			want: Deny,
		},
		{
			name: "echo process-sub git push denied (chain-guard process-sub >())",
			cmd:  `echo y >(git push origin main)`,
			want: Deny,
		},
		{
			// Bash treats a LITERAL newline as a statement separator (equivalent
			// to `;`). A smuggled second leg after a newline must NOT escape the
			// chain-guard: `echo step1\ngit commit` would otherwise be carved out
			// (echo satisfies the shell-`-c` inspector carve-out) and the git
			// commit would run. The char class now includes \n.
			name: "echo newline git commit denied (chain-guard newline separator)",
			cmd:  "vh-agent-harness exec bash -c 'echo step1\ngit commit -m y'",
			want: Deny,
		},
		{
			name: "echo newline git push denied (chain-guard newline separator)",
			cmd:  "vh-agent-harness exec bash -c 'echo step1\ngit push origin main'",
			want: Deny,
		},
		{
			name: "printf newline git reset denied (chain-guard newline separator)",
			cmd:  "vh-agent-harness exec bash -c 'printf done\ngit reset --hard'",
			want: Deny,
		},
		{
			// Relative -C: walkGitGlobals denies ANY relative -C path (`.`,
			// `..`, subdir) with an actionable notice, because normalizing a
			// relative path would require resolving against commandCwd and
			// invites symlink / `..` / normalization bugs. The mutation verb
			// behind it is therefore never reached; this is a deliberate
			// risk-reduction over the old normalizeGitC path (which also denied
			// this form, just via a different mechanism).
			name: "git -C . commit denied (relative -C notice)",
			cmd:  `git -C . commit -m x`,
			want: Deny,
		},
		{
			name: "git commit with inspector-substring arg denied (unanchored-shell-c bypass vector)",
			cmd:  `vh-agent-harness exec git commit -m "bash -c 'echo ok'"`,
			want: Deny,
		},
		{
			name: "git push with inspector-substring arg denied (unanchored-shell-c bypass vector)",
			cmd:  `vh-agent-harness exec git push origin main "bash -c 'echo x'"`,
			want: Deny,
		},
		{
			name: "git reset with inspector-substring arg denied (unanchored-shell-c bypass vector)",
			cmd:  `vh-agent-harness exec git reset --hard "sh -c 'cat y'"`,
			want: Deny,
		},
		{
			// F1 fix: a `;`-composite chaining a git mutation after a
			// commit-gate.sh prefix. Mechanism this closes: alt-A
			// `COMMIT_GATE_PREFIX` previously had NO chain-guard, so scan #1's
			// allowIf matched the `.opencode/scripts/commit-gate.sh` prefix and
			// carved the whole composite out → forbidden=null → parseCommands
			// split on `;` → the per-command re-scan only fires for tokens[0]
			// ==="git", so the `vh-agent-harness exec bash -c '...'` leg was
			// never re-scanned and matched the `vh-agent-harness *` allowlist →
			// ALLOW. Now alt-A carries the SAME chain-guard as alt-B (the
			// leading negative lookahead over the whole string), so the `;`
			// refuses the carve-out at scan #1 → DENY.
			name: "commit-gate.sh ; vh-agent-harness exec git commit denied (F1 alt-A chain-guard fix)",
			cmd:  `.opencode/scripts/commit-gate.sh acquire; vh-agent-harness exec bash -c 'git commit -m pwned'`,
			want: Deny,
		},
		{
			// Defense-in-depth: the `&&` composite against the commit-gate
			// prefix. Once alt-A carries the chain-guard, the `&&` also
			// refuses the carve-out at scan #1 → DENY (no second-leg
			// vh-agent-harness exec wrapping needed).
			name: "commit-gate.sh && git push denied (alt-A chain-guard, && composite)",
			cmd:  `.opencode/scripts/commit-gate.sh acquire && git push origin main`,
			want: Deny,
		},
		{
			// Defense-in-depth: the `|` composite against the commit-gate
			// prefix. The `|` is in the chain-guard char class, so the carve-out
			// is refused at scan #1 → DENY.
			name: "commit-gate.sh | git commit denied (alt-A chain-guard, pipe composite)",
			cmd:  `.opencode/scripts/commit-gate.sh acquire | git commit -F -`,
			want: Deny,
		},
		{
			name: "commit-gate.sh acquire allowed (carve-out preserved)",
			cmd:  `.opencode/scripts/commit-gate.sh acquire --paths-file .git/commit-gate/paths-x --message-file .git/commit-gate/msg-x --session-alias A`,
			want: Allow,
		},
		{
			// Mutation-slip guard: `git --no-pager commit` puts an `always`-strip
			// global flag between `git` and the mutation verb. walkGitGlobals
			// extracts verb=`commit` past the flag and the UNIFORM mutation-slip
			// guard denies it before the allowlist sees it. Must be DENY.
			name: "git --no-pager commit denied (mutation-slip guard via walkGitGlobals)",
			cmd:  `git --no-pager commit -m x`,
			want: Deny,
		},
		{
			// `--paging=no` is NOT a real git flag (absent from `git --help`).
			// walkGitGlobals treats it as an unknown `-flag` (never-strip
			// boolean, consumed in place), so the verb `push` is still
			// extracted and the mutation-slip guard denies it. Pins the bonus
			// fix that REMOVED `--paging=no` from GIT_SAFE_GLOBAL_FLAGS (the old
			// set was `["--no-pager","--paging=no"]` — the second entry was a
			// fiction). The correct `always`-strip set is now
			// `["-p","--paginate","-P","--no-pager"]`.
			name: "git --paging=no push denied (unknown global flag; mutation-slip guard)",
			cmd:  `git --paging=no push origin main`,
			want: Deny,
		},
		{
			// walkGitGlobals strips the `always`-policy `--no-pager` global and
			// rewrites `git --no-pager log -1` -> `git log -1`; the allowlist
			// then matches `git log *`. The config-table `git --no-pager log *`
			// entry is now belt-and-suspenders. Must be ALLOW.
			name: "git --no-pager log allowed (walkGitGlobals rewrites to `git log -1`)",
			cmd:  `git --no-pager log -1`,
			want: Allow,
		},
		{
			// walkGitGlobals strips `--no-pager` and rewrites to
			// `git show HEAD`, matched by `git show *`.
			name: "git --no-pager show allowed (walkGitGlobals rewrites to `git show HEAD`)",
			cmd:  `git --no-pager show HEAD`,
			want: Allow,
		},
		{
			// Multi-flag readonly form. walkGitGlobals consumes `--no-pager`
			// (always, strip) then `--paging=no` (unknown -> never-strip
			// boolean). NOT fully strippable -> NO rewrite -> allowlist sees the
			// ORIGINAL two-flag form -> blocked -> routing hint ASK (the walker
			// extracted verb=`log`, a known-readonly verb, so the hint fires).
			// This CHANGED from the old design, which hard-DENIED: previously
			// `--no-pager` polluted GIT_READONLY_SUBCOMMANDS (the 12
			// `git --no-pager <sub> *` config entries each contributed
			// `--no-pager` as parts[1]), so the
			// `!GIT_READONLY_SUBCOMMANDS.has(blocked[1])` clause was false and
			// evaluate() fell through to a hard deny (over-block). The
			// walker-based hint now correctly routes a benign read to ASK. The
			// operator still sees a prompt, but no longer a hard deny.
			name: "git --no-pager --paging=no log asked (walker verb=log, not fully strippable; was Deny pre-walker)",
			cmd:  `git --no-pager --paging=no log`,
			want: Ask,
		},
		{
			// Multi-flag mutation: walkGitGlobals consumes `--no-pager` (strip)
			// and `--paging=no` (unknown, never-strip), then extracts verb=
			// `commit`. The UNIFORM mutation-slip guard denies before the
			// allowlist is consulted. Pins that an unknown flag between `git`
			// and a mutation verb does NOT hide the mutation.
			name: "git --no-pager --paging=no commit denied (mutation-slip guard past unknown flag)",
			cmd:  `git --no-pager --paging=no commit`,
			want: Deny,
		},
		{
			// Order-independence of the mutation-slip guard: swapping the
			// unknown flag and the `--no-pager` flag still extracts verb=`push`
			// and denies. The walker consumes any leading run of globals
			// regardless of order before identifying the verb.
			name: "git --paging=no --no-pager push denied (order-independent mutation-slip guard)",
			cmd:  `git --paging=no --no-pager push origin main`,
			want: Deny,
		},
	}
	for _, c := range gitCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, _, evalErr := h.Evaluate(context.Background(), []string{c.cmd})
			if evalErr != nil {
				t.Fatalf("Evaluate(%q) error %v; want verdict %s (no bridge error)", c.cmd, evalErr, c.want)
			}
			// Assert the EXACT verdict: deny cases must be Deny (not Ask/Allow),
			// so the committer-flow allowlist carve-out cannot mask a regression.
			if got != c.want {
				t.Errorf("Evaluate(%q) = %s; want %s", c.cmd, got, c.want)
			}
		})
	}

	// --- git global-flag walker matrix ---------------------------------------
	//
	// These cases exercise walkGitGlobals end-to-end through eval.js. They use
	// runNode (NOT h.Evaluate) so the rewrite field emitted by eval.js can be
	// asserted directly — the Go hook struct only surfaces action/reason; the
	// write-back of rewrite into output.args.command happens in the JS plugin's
	// tool.execute.before wrapper (shell-guard.js), one layer above eval.js, so
	// eval.js emits rewrite as a hint and the wrapper consumes it.
	//
	// commandCwd inside eval.js resolves to repoRoot() = scratch (plugins/ ->
	// two up), so `git -C <scratch-abs> ...` is the absolute-commandCwd shape
	// (the conditional strip case). External paths use /var/x deliberately —
	// NEVER /tmp, which system-tmp-access would deny before the walker runs.
	scratchSub := filepath.Join(scratch, "subdir")
	globalFlagCases := []struct {
		name        string
		cmd         string
		wantAction  string // "allow" | "deny" | "ask"
		wantRewrite string // "" asserts eval.js emitted NO rewrite field
	}{
		// 1. always-strip flag -> rewrite -> allow.
		{name: "--no-pager diff rewrites to `git diff x`", cmd: `git --no-pager diff x`, wantAction: "allow", wantRewrite: `git diff x`},
		// 2. combo always + conditional(strip) -> rewrite -> allow.
		{name: "--no-pager -C <commandCwd> log rewrites to `git log`", cmd: fmt.Sprintf(`git --no-pager -C %s log`, scratch), wantAction: "allow", wantRewrite: `git log`},
		// 3. conditional(strip) alone -> rewrite -> allow.
		{name: "-C <abs commandCwd> diff rewrites to `git diff`", cmd: fmt.Sprintf(`git -C %s diff`, scratch), wantAction: "allow", wantRewrite: `git diff`},
		// 4. conditional(keep): abs in-project subdir != commandCwd -> no rewrite -> ask.
		{name: "-C <abs in-project subdir> diff asks (no rewrite)", cmd: fmt.Sprintf(`git -C %s diff`, scratchSub), wantAction: "ask", wantRewrite: ""},
		// 5. external -C readonly -> ask (verb extracted, not fully strippable).
		{name: "-C <abs external> diff asks", cmd: `git -C /var/x diff`, wantAction: "ask", wantRewrite: ""},
		// 6. external -C mutation -> deny (mutation-slip guard; must NOT be ask).
		{name: "-C <abs external> commit denied (mutation-slip guard)", cmd: `git -C /var/x commit -m x`, wantAction: "deny", wantRewrite: ""},
		// 7. relative -C -> deny + notice.
		{name: "-C ./subdir diff denied (relative -C notice)", cmd: `git -C ./subdir diff`, wantAction: "deny", wantRewrite: ""},
		// 8. never-strip flag present -> no rewrite -> ask.
		{name: "--git-dir=/x diff asks (never-strip flag blocks rewrite)", cmd: `git --git-dir=/x diff`, wantAction: "ask", wantRewrite: ""},
		// 9. mutation overrides strippability: abs commandCwd + mutation -> deny.
		{name: "-C <abs commandCwd> commit denied (mutation overrides strip)", cmd: fmt.Sprintf(`git -C %s commit -m x`, scratch), wantAction: "deny", wantRewrite: ""},
		// 10. relative -C + mutation -> deny (relative path wins; mutation never reached).
		{name: "-C . commit denied (relative -C, case 10)", cmd: `git -C . commit -m x`, wantAction: "deny", wantRewrite: ""},
		// Bonus A: --paging=no is not a real flag -> not stripped -> ask.
		{name: "--paging=no log asks (--paging=no is not a real git flag)", cmd: `git --paging=no log`, wantAction: "ask", wantRewrite: ""},
		// Bonus B: info-flag with no verb -> allow (read-only terminal info).
		{name: "--help allowed (info-flag terminal request)", cmd: `git --help`, wantAction: "allow", wantRewrite: ""},
		// Bonus C: --version info-flag -> allow.
		{name: "--version allowed (info-flag terminal request)", cmd: `git --version`, wantAction: "allow", wantRewrite: ""},
		// Bonus D: bare readonly (no globals) -> allow with NO rewrite.
		{name: "bare git diff allow with no rewrite", cmd: `git diff`, wantAction: "allow", wantRewrite: ""},
	}
	for _, c := range globalFlagCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			out, code := runNode(t, nodeBin, evalPath, scratch, c.cmd)
			if code != 0 {
				t.Fatalf("runNode exit %d stdout=%q; expected exit 0 (decision, not engine fault)", code, out)
			}
			act := jsonAction(t, out)
			if act != c.wantAction {
				t.Errorf("action = %q, want %q (cmd=%q; stdout=%q)", act, c.wantAction, c.cmd, out)
			}
			rw := jsonRewrite(t, out)
			if rw != c.wantRewrite {
				t.Errorf("rewrite = %q, want %q (cmd=%q; stdout=%q)", rw, c.wantRewrite, c.cmd, out)
			}
		})
	}
}

// findModuleRoot walks up from cwd until it finds go.mod. go test runs with
// cwd = the package source dir (.../internal/permission), so the module root
// (where templates/ lives) is a few parents up.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("go.mod not found walking up from %s", dir)
	return ""
}

// runNode runs `node evalPath args...` with cwd=scratch and returns (stdout,
// exit code). It fails the test only on a spawn error, not on a non-zero exit
// (eval.js exit 0 for decisions, exit 2 for engine faults — both assertable).
func runNode(t *testing.T, nodeBin, evalPath, cwd string, args ...string) (string, int) {
	t.Helper()
	full := append([]string{evalPath}, args...)
	cmd := exec.Command(nodeBin, full...)
	cmd.Dir = cwd
	var stdout strings.Builder
	cmd.Stdout = &stdout
	var stderr strings.Builder
	cmd.Stderr = &stderr
	// Bound the run so a hung WASM load fails the test instead of hanging CI.
	timer := time.AfterFunc(30*time.Second, func() { _ = cmd.Process.Kill() })
	defer timer.Stop()
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdout.String(), exitErr.ExitCode()
		}
		t.Fatalf("node spawn error: %v\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), 0
}

// jsonAction extracts the "action" field from the single JSON line eval.js
// emits on stdout. Fails the test on malformed output.
func jsonAction(t *testing.T, stdout string) string {
	t.Helper()
	line := strings.TrimSpace(stdout)
	if line == "" {
		t.Fatalf("eval.js produced no stdout; cannot read action")
	}
	// jsonAction is intentionally simple; the hook's json.Unmarshal path is
	// covered separately by the mapping tests.
	var res struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		t.Fatalf("eval.js stdout not JSON: %q (%v)", line, err)
	}
	return res.Action
}

// jsonRewrite extracts the "rewrite" field from the single JSON line eval.js
// emits. Returns "" both when the field is absent (eval.js omits rewrite when
// there is nothing to rewrite, so the wrapper leaves output.args.command
// untouched) and when it is explicitly empty. The distinction does not matter
// to callers: both mean "no command rewrite occurred".
func jsonRewrite(t *testing.T, stdout string) string {
	t.Helper()
	line := strings.TrimSpace(stdout)
	if line == "" {
		t.Fatalf("eval.js produced no stdout; cannot read rewrite")
	}
	var res struct {
		Rewrite string `json:"rewrite"`
	}
	if err := json.Unmarshal([]byte(line), &res); err != nil {
		t.Fatalf("eval.js stdout not JSON: %q (%v)", line, err)
	}
	return res.Rewrite
}
