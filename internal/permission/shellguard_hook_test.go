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

	// infra-lifecycle regression matrix (Go -> node -> WASM -> evaluate()).
	// Pins the P1-PERM-002 hardening of the shared inspector builder for the
	// apt-install-ad-hoc / user-group-mutation / ssh-host-key-bypass / scp-upload
	// rules. This is the same gap class as the landed git-lane fix (P1-PERM-001,
	// f88d29f): the 4 rules now use a `command`-EXCLUDING inspector set
	// (INFRA_LIFECYCLE_INSPECTORS) so the `command` executor builtin can no
	// longer carve them out, and the shared chain-guard is upgraded to the full
	// 11-op set (added \n, \r, process-sub <( / >().
	//
	// Anchors:
	//   - FP fix: prose mentioning the trigger in echo/printf args is carved out
	//     (user-group-mutation previously FP'd on the literal `usermod` token in
	//     commit-message prose). echo/printf are used (not rg/grep) so the
	//     inspector carve-out — not the G4 inert-literal classifier — is the path
	//     under test.
	//   - command carve-out closure: `command <verb>` is DENIED for all 4 verbs
	//     (the main fix). `command -v <verb>` lookup is ALSO denied — the
	//     documented trade-off (use which/type, still in the set).
	//   - upgraded chain-guard: \n and process-sub <( / >( smuggled legs that
	//     were NOT caught by the old 7-op guard are now DENIED.
	//   - real-invocation baselines stay DENIED.
	// Each command is passed as a single-element argv so eval.js's
	// argv.join(" ") yields exactly the intended command string.
	infraCases := []struct {
		name string
		cmd  string
		want Action
	}{
		// --- FP fix: inspector carve-out preserved (echo/printf prose) -----
		{
			name: "echo prose with usermod token (infra FP fix)",
			cmd:  `echo "see usermod docs"`,
			want: Allow,
		},
		{
			name: "printf prose with apt-get install token (infra FP fix)",
			cmd:  `printf "apt-get install is banned"`,
			want: Allow,
		},
		{
			name: "echo prose with ssh -o bypass token (infra FP fix)",
			cmd:  `echo "ref: ssh -o StrictHostKeyChecking=no is forbidden"`,
			want: Allow,
		},
		{
			name: "echo prose with scp upload token (infra FP fix)",
			cmd:  `echo "example: scp f user@host:/x is banned"`,
			want: Allow,
		},

		// --- command carve-out closure (THE fix: command <verb> DENIED) ----
		{
			// `command` is a bash builtin that EXECUTES its argument. It is in
			// the shared INSPECTOR_FULL (kept for non-executor project-overlay
			// rules), but the infra-lifecycle carve-out uses a SEPARATE set
			// (INFRA_LIFECYCLE_INSPECTORS) that EXCLUDES `command`. Otherwise
			// `command apt-get install x` is carved out at scan #1 (command in
			// command position → allowIf matches) and the apt install runs.
			name: "command apt-get install denied (executor verb not a safe inspector)",
			cmd:  `command apt-get install foo`,
			want: Deny,
		},
		{
			name: "command usermod denied (executor verb not a safe inspector)",
			cmd:  `command usermod -aG docker alice`,
			want: Deny,
		},
		{
			name: "command ssh -o bypass denied (executor verb not a safe inspector)",
			cmd:  `command ssh -o StrictHostKeyChecking=no host`,
			want: Deny,
		},
		{
			name: "command scp upload denied (executor verb not a safe inspector)",
			cmd:  `command scp file user@host:/path`,
			want: Deny,
		},
		{
			// Documented trade-off of excluding `command`: the benign lookup
			// `command -v usermod` is NO LONGER carved out (denied). An agent
			// that wants the lookup must use `which`/`type` (still in the set).
			// Pinned so a future "fix" cannot silently re-add `command`.
			name: "command -v usermod denied (lookup trade-off; use which/type)",
			cmd:  `command -v usermod`,
			want: Deny,
		},

		// --- upgraded chain-guard: smuggled legs now DENIED ----------------
		{
			name: "echo ; usermod denied (chain-guard semicolon)",
			cmd:  `echo x; usermod -aG docker alice`,
			want: Deny,
		},
		{
			name: "echo && apt-get install denied (chain-guard &&)",
			cmd:  `echo x && apt-get install foo`,
			want: Deny,
		},
		{
			name: "echo | useradd denied (chain-guard pipe)",
			cmd:  `echo x | useradd bob`,
			want: Deny,
		},
		{
			name: "echo $(usermod) denied (chain-guard $())",
			cmd:  `echo $(usermod -aG x y)`,
			want: Deny,
		},
		{
			// Process substitution <(...) runs the inner command with NO list
			// separator. The OLD shared chain-guard lacked <( so this was
			// carved out (echo satisfies the inspector carve-out) and the apt
			// install ran. The upgraded 11-op guard now refuses the carve-out.
			name: "echo <(apt-get install) denied (chain-guard process-sub <(), NEW with upgrade)",
			cmd:  `echo x <(apt-get install foo)`,
			want: Deny,
		},
		{
			name: "echo >(scp upload) denied (chain-guard process-sub >(), NEW with upgrade)",
			cmd:  `echo y >(scp f user@host:/p)`,
			want: Deny,
		},
		{
			// Bash treats a LITERAL newline as a statement separator. The OLD
			// shared chain-guard lacked \n so a smuggled second leg after a
			// newline was carved out (echo satisfies the shell-`-c` inspector
			// carve-out) and the usermod ran. The upgraded guard now refuses it.
			name: "echo newline usermod denied (chain-guard newline separator, NEW with upgrade)",
			cmd:  "vh-agent-harness exec bash -c 'echo step1\nusermod -aG docker alice'",
			want: Deny,
		},
		{
			// CR is the other bash statement separator newly added to the
			// chain-guard char class ([;\n\r&|`]). A smuggled leg after a CR
			// must NOT escape the guard either. Pinned so the slice's "full
			// 11-op" claim is self-verifying for both newline-class bytes.
			name: "echo carriage-return usermod denied (chain-guard CR separator, NEW with upgrade)",
			cmd:  "vh-agent-harness exec bash -c 'echo step1\rusermod -aG docker alice'",
			want: Deny,
		},

		// --- real-invocation baselines (still DENIED) ---------------------
		{
			name: "sudo useradd denied (real invocation baseline)",
			cmd:  `sudo useradd bob`,
			want: Deny,
		},
		{
			name: "ssh -o bypass denied (real invocation baseline)",
			cmd:  `ssh -o StrictHostKeyChecking=no host`,
			want: Deny,
		},
		{
			name: "scp upload denied (real invocation baseline)",
			cmd:  `scp file user@host:/path`,
			want: Deny,
		},
	}
	for _, c := range infraCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, _, evalErr := h.Evaluate(context.Background(), []string{c.cmd})
			if evalErr != nil {
				t.Fatalf("Evaluate(%q) error %v; want verdict %s (no bridge error)", c.cmd, evalErr, c.want)
			}
			// Assert the EXACT verdict: deny cases must be Deny (not Ask/Allow),
			// so a carve-out regression cannot mask as an allow.
			if got != c.want {
				t.Errorf("Evaluate(%q) = %s; want %s", c.cmd, got, c.want)
			}
		})
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
			// walkGitGlobals classifies `--no-pager` as always-strip, so the
			// INTERNAL allowlist matches the stripped `git log -1` form against
			// `git log *` -> allow. Under Option A no rewrite is emitted; the
			// prompt-free path at opencode's L2 is the config-table
			// `git --no-pager log *` rule (load-bearing, not redundant). Must
			// be ALLOW.
			name: "git --no-pager log allowed (internal stripped match, no rewrite)",
			cmd:  `git --no-pager log -1`,
			want: Allow,
		},
		{
			// walkGitGlobals classifies `--no-pager` as always-strip; the
			// internal allowlist matches the stripped `git show HEAD` against
			// `git show *`. No rewrite emitted.
			name: "git --no-pager show allowed (internal stripped match, no rewrite)",
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

		// --- F1 wrapped-mutation-bypass fix (A2 JS source-of-truth) -----------
		//
		// These cases pin the F1 closure at the JS gate: a payload
		// `vh-agent-harness exec git <global-flag> <mutation>` is now denied by
		// detectWrappedGitMutation BEFORE the harness branch's allow. Before the
		// fix, the harness branch returned `{action:"allow"}` for these because
		// the adjacency regex (`\bgit\s+<mutation-verb>`) could not match a flag
		// between `git` and the verb, and walkGitGlobals never ran for wrapped
		// payloads. The Go A1 backstop covers the same shape; these JS cases
		// pin the source-of-truth layer the rendered tree ships.
		{
			name: "vh-agent-harness exec git --no-pager commit denied (F1 wrapped bypass)",
			cmd:  `vh-agent-harness exec git --no-pager commit -m x`,
			want: Deny,
		},
		{
			name: "vh-agent-harness exec git -C /var/x push denied (F1 wrapped bypass)",
			cmd:  `vh-agent-harness exec git -C /var/x push origin main`,
			want: Deny,
		},
		{
			name: "vh-agent-harness exec git --git-dir=/var/x commit denied (F1 wrapped bypass)",
			cmd:  `vh-agent-harness exec git --git-dir=/var/x commit -m x`,
			want: Deny,
		},
		{
			// Space-separated form of --git-dir (e.g. `git --git-dir /x commit`).
			// Same vector as the attached `--git-dir=/x` shape above, but the
			// flag and its value are two separate argv tokens. Pinned explicitly
			// so a future walkGitGlobals / detectWrappedGitMutation refactor
			// cannot silently drop this declared F1 vector (it is currently
			// covered only transitively via the walker's consume-and-continue
			// path for value-bearing globals).
			name: "vh-agent-harness exec git --git-dir /var/x commit denied (F1 wrapped bypass, space-separated)",
			cmd:  `vh-agent-harness exec git --git-dir /var/x commit -m x`,
			want: Deny,
		},
		{
			// Regression guard: the adjacent form (`exec git commit`) was
			// already caught by the regex; the A2 parser must not regress it.
			name: "vh-agent-harness exec git commit denied (adjacency regression)",
			cmd:  `vh-agent-harness exec git commit -m x`,
			want: Deny,
		},
		{
			name: "vh-agent-harness exec git --no-pager push denied (F1 wrapped bypass, push variant)",
			cmd:  `vh-agent-harness exec git --no-pager push origin main`,
			want: Deny,
		},
		{
			// exec-ro path is also covered: it routes through the same harness
			// branch but its payload starts with `--` (optional) then the bare
			// payload. detectWrappedGitMutation handles the exec-ro form too.
			name: "vh-agent-harness exec-ro git --no-pager commit denied (F1 wrapped bypass, exec-ro)",
			cmd:  `vh-agent-harness exec-ro git --no-pager commit -m x`,
			want: Deny,
		},

		// F1 ALLOW controls: the A2 parser must NOT over-deny.
		{
			name: "vh-agent-harness exec git --no-pager status allowed (readonly git + global flag)",
			cmd:  `vh-agent-harness exec git --no-pager status`,
			want: Allow,
		},
		{
			name: "vh-agent-harness exec git -C /var/x status allowed (readonly git + path-bearing flag)",
			cmd:  `vh-agent-harness exec git -C /var/x status`,
			want: Allow,
		},
		{
			// Legitimate non-git mutations through exec MUST stay allowed — A2
			// is git-mutation-scoped only and must not reproduce the over-deny
			// that calling execro.Classify would cause.
			name: "vh-agent-harness exec mkdir tmp/x allowed (non-git mutation, F1 no-over-deny)",
			cmd:  `vh-agent-harness exec mkdir tmp/x`,
			want: Allow,
		},
		{
			name: "vh-agent-harness exec pytest allowed (non-git mutation, F1 no-over-deny)",
			cmd:  `vh-agent-harness exec pytest`,
			want: Allow,
		},
		{
			// Nested-shell git is OUT OF SCOPE for A2's wrapped-mutation parser
			// (it does not parse `bash -c '...'` strings); it stays governed
			// by the existing forbidden-pattern chain-guard scan, which denies
			// the inner git mutation. Pinning both invariants: exec wrapping
			// is allowed (no over-deny) AND the inner mutation is still denied.
			name: "vh-agent-harness exec bash -c 'echo hi' allowed (nested shell OUT OF SCOPE for A2)",
			cmd:  `vh-agent-harness exec bash -c 'echo hi'`,
			want: Allow,
		},

		// Existing-behavior preservation: direct `vh-agent-harness git <verb>`
		// (NOT through exec) is denied at the `startsWith("vh-agent-harness git ")`
		// branch and must NOT conflict with the new wrapped-mutation parser.
		{
			name: "vh-agent-harness git status denied (direct, existing-behavior preservation)",
			cmd:  `vh-agent-harness git status`,
			want: Deny,
		},
		{
			name: "vh-agent-harness git commit denied (direct, existing-behavior preservation)",
			cmd:  `vh-agent-harness git commit -m x`,
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

	// --- isGateWrapperInDevShExec over-block narrowing matrix --------------
	//
	// Pins the narrowing of isGateWrapperInDevShExec: the broad
	// `includes("commit-gate.sh")` deny now has ONE closed, fail-closed
	// exception for syntactically inert static inspection (`bash -n` / `cmp`
	// of the gate script). Neither form can execute or mutate anything.
	//
	// Load-bearing invariants pinned here:
	//   - the two ALLOW forms (bash -n / cmp of commit-gate.sh) now pass the
	//     gate end-to-end through Go -> node -> WASM -> evaluate();
	//   - real wrapper execution of commit-gate.sh through exec is STILL
	//     denied (the exception is narrow, not a blanket allow);
	//   - git-mutation-bypass is intact: wrapped git mutations still DENY
	//     regardless of this exception (scan #1 runs before the harness
	//     branch);
	//   - negative-grammar shapes fail closed: any shell-control syntax,
	//     quoting, `bash -c`, or a smuggled `;`-leg keeps the DENY.
	//
	// Each command is passed as a single-element argv so eval.js's
	// argv.join(" ") yields exactly the intended command string.
	gateInspectionCases := []struct {
		name string
		cmd  string
		want Action
	}{
		// --- MUST ALLOW: inert static inspection of the gate script --------
		{
			// `bash -n` validates syntax ONLY; the script never runs. This is
			// the canonical FP the narrowing drains (previously forced through
			// a tmp/ script indirection per AGENTS command-hygiene rule #2).
			name: "vh-agent-harness exec bash -n commit-gate.sh allowed (inert syntax check)",
			cmd:  `vh-agent-harness exec bash -n .opencode/scripts/commit-gate.sh`,
			want: Allow,
		},
		{
			// `bash -n -- <path>`: the optional POSIX options terminator is
			// accepted by the closed grammar.
			name: "vh-agent-harness exec bash -n -- commit-gate.sh allowed (-- terminator)",
			cmd:  `vh-agent-harness exec bash -n -- .opencode/scripts/commit-gate.sh`,
			want: Allow,
		},
		{
			// `cmp` compares bytes and returns an exit code; it never executes
			// either operand. The canonical dogfood render-check (source vs
			// rendered commit-gate.sh) has BOTH operands ending in
			// commit-gate.sh — the grammar requires "at least one", not
			// "exactly one" (both-ending is fully inert).
			name: "vh-agent-harness exec cmp source-vs-rendered commit-gate.sh allowed (inert compare)",
			cmd:  `vh-agent-harness exec cmp templates/core/.opencode/scripts/commit-gate.sh .opencode/scripts/commit-gate.sh`,
			want: Allow,
		},
		{
			// `cmp` with one gate operand + one arbitrary operand (either
			// order). cmp does not execute either side; this is a one-bit
			// equality probe, negligible vs the Read tool already available.
			name: "vh-agent-harness exec cmp commit-gate.sh other allowed (inert compare, gate first)",
			cmd:  `vh-agent-harness exec cmp .opencode/scripts/commit-gate.sh tmp/scratch/other.sh`,
			want: Allow,
		},
		{
			name: "vh-agent-harness exec cmp other commit-gate.sh allowed (inert compare, gate second)",
			cmd:  `vh-agent-harness exec cmp tmp/scratch/other.sh .opencode/scripts/commit-gate.sh`,
			want: Allow,
		},
		{
			// `cmp -- <a> <b>`: optional POSIX terminator accepted.
			name: "vh-agent-harness exec cmp -- commit-gate.sh other allowed (-- terminator)",
			cmd:  `vh-agent-harness exec cmp -- .opencode/scripts/commit-gate.sh tmp/scratch/other.sh`,
			want: Allow,
		},

		// --- MUST DENY: real wrapper execution still blocked ----------------
		{
			// Direct wrapper invocation of commit-gate.sh (the committer must
			// invoke it DIRECTLY, not through exec). Verb is the gate script
			// path, not bash/cmp → exception does not fire → deny preserved.
			name: "vh-agent-harness exec commit-gate.sh acquire denied (real wrapper execution)",
			cmd:  `vh-agent-harness exec .opencode/scripts/commit-gate.sh acquire --paths-file tmp/x --message-file tmp/y --session-alias A`,
			want: Deny,
		},
		{
			// `bash -c '...commit-gate.sh...'`: the single-quote triggers the
			// forbidden-char fail-closed in the exception, AND this is the
			// load-bearing nested-invocation shape the broad deny was built
			// for. Must stay denied.
			name: "vh-agent-harness exec bash -c commit-gate.sh acquire denied (nested invocation)",
			cmd:  `vh-agent-harness exec bash -c '.opencode/scripts/commit-gate.sh acquire --paths-file tmp/x --message-file tmp/y --session-alias A'`,
			want: Deny,
		},

		// --- MUST DENY: git-mutation-bypass intact (scan #1 covers these) ---
		{
			// Wrapped git mutation past a global flag. detectWrappedGitMutation
			// denies this (F1); the static-inspection exception is irrelevant
			// because there is no commit-gate.sh mention AND scan #1 / F1
			// catch the mutation first. Pinned so the narrowing cannot weaken
			// the mutation surface.
			name: "vh-agent-harness exec git --no-pager commit denied (git-mutation-bypass intact)",
			cmd:  `vh-agent-harness exec git --no-pager commit -m x`,
			want: Deny,
		},
		{
			// Load-bearing evasion: a wrapped git reset buried in bash -c. The
			// git-mutation-bypass regex matches the adjacent `git reset` and
			// the chain-guard refuses the carve-out (the payload is not an
			// inspector). Static-inspection exception is NOT consulted for
			// this (no commit-gate.sh), but pinned here to prove the
			// narrowing coexists cleanly with the mutation backstop.
			name: "vh-agent-harness exec bash -c git reset denied (git-mutation-bypass intact)",
			cmd:  `vh-agent-harness exec bash -c 'git reset --hard'`,
			want: Deny,
		},

		// --- MUST DENY: negative grammar (fail-closed) ---------------------
		{
			// `bash -c` (NOT `bash -n`): the exception allows ONLY `bash -n`.
			// A `cmp` of the gate buried inside bash -c is a nested payload
			// and must NOT be carved out — the single-quote alone trips the
			// forbidden-char fail-closed.
			name: "vh-agent-harness exec bash -c 'cmp a commit-gate.sh' denied (bash -c not bash -n)",
			cmd:  `vh-agent-harness exec bash -c 'cmp a .opencode/scripts/commit-gate.sh'`,
			want: Deny,
		},
		{
			// Smuggled `;`-leg after a valid `bash -n` form. The semicolon (a)
			// trips the forbidden-char fail-closed in the exception AND (b)
			// trips the git-mutation-bypass chain-guard at scan #1, so the
			// composite is denied at BOTH layers. Pinned so the closed
			// grammar cannot be evaded by appending a statement separator.
			name: "vh-agent-harness exec bash -n commit-gate.sh; git commit denied (smuggled ; leg)",
			cmd:  `vh-agent-harness exec bash -n .opencode/scripts/commit-gate.sh; git commit -m x`,
			want: Deny,
		},
		{
			// Bare `bash` (no -n): the exception requires exactly `bash -n`.
			name: "vh-agent-harness exec bash commit-gate.sh denied (bare bash, no -n)",
			cmd:  `vh-agent-harness exec bash .opencode/scripts/commit-gate.sh`,
			want: Deny,
		},
		{
			// cmp with only ONE operand: the grammar requires exactly two.
			name: "vh-agent-harness exec cmp commit-gate.sh denied (cmp needs two operands)",
			cmd:  `vh-agent-harness exec cmp .opencode/scripts/commit-gate.sh`,
			want: Deny,
		},
		{
			// cmp with THREE operands: the grammar requires exactly two. A
			// trailing third token could carry a smuggled payload, so the
			// command-must-END rule fails closed.
			name: "vh-agent-harness exec cmp a b commit-gate.sh denied (cmp must end after two)",
			cmd:  `vh-agent-harness exec cmp .opencode/scripts/commit-gate.sh tmp/a tmp/b`,
			want: Deny,
		},
		{
			// Quoted path: the double-quote trips the forbidden-char
			// fail-closed. Agents must use plain unquoted paths (legitimate
			// commit-gate.sh paths contain no spaces).
			name: `vh-agent-harness exec bash -n "commit-gate.sh" denied (quoted path)`,
			cmd:  `vh-agent-harness exec bash -n ".opencode/scripts/commit-gate.sh"`,
			want: Deny,
		},

		// --- MUST DENY: bash -n grammar rejection (defer-staticinspection) ---
		//
		// The cmp analogs of these grammar checks ARE pinned
		// (cmp-must-end / cmp-needs-two above); the bash -n analogs were not.
		// Both share the SAME closed-grammar checks in
		// isStaticGateInspectionInDevShExec:
		//   - `i + 1 !== tokens.length` — the command must END immediately
		//     after the single permitted path (a trailing token could carry a
		//     smuggled second leg);
		//   - `operand.startsWith("-")` — the single operand must be a plain
		//     path, not an option-like token.
		// Both fail closed (return false) so isGateWrapperInDevShExec keeps
		// the DENY. The logic already rejects these forms correctly; these
		// rows pin the rejection end-to-end through Go -> node -> WASM.
		{
			// Trailing operand (count / must-end branch). Direct analog of the
			// cmp "must end after two" case above. The closed grammar allows
			// exactly ONE operand after `bash -n [--]`; a trailing positional
			// token fails the `i + 1 !== tokens.length` check → exception does
			// not fire → deny preserved.
			name: "vh-agent-harness exec bash -n commit-gate.sh trailing denied (bash -n must end after path)",
			cmd:  `vh-agent-harness exec bash -n .opencode/scripts/commit-gate.sh trailing-arg`,
			want: Deny,
		},
		{
			// Option-like operand (startsWith branch). The closed grammar
			// requires the single operand to be a plain path; any operand that
			// starts with `-` (would be an option, not a path) fails the
			// `operand.startsWith("-")` check even though the string mentions
			// commit-gate.sh. Pinned so a future refactor cannot silently drop
			// the option-guard (the only way to reach this branch for bash is
			// a single option-like operand containing the gate substring).
			name: "vh-agent-harness exec bash -n -commit-gate.sh denied (bash -n option-like operand, startsWith guard)",
			cmd:  `vh-agent-harness exec bash -n -commit-gate.sh`,
			want: Deny,
		},
	}
	for _, c := range gateInspectionCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, _, evalErr := h.Evaluate(context.Background(), []string{c.cmd})
			if evalErr != nil {
				t.Fatalf("Evaluate(%q) error %v; want verdict %s (no bridge error)", c.cmd, evalErr, c.want)
			}
			// Assert the EXACT verdict so a carve-out regression cannot mask
			// as the wrong action.
			if got != c.want {
				t.Errorf("Evaluate(%q) = %s; want %s", c.cmd, got, c.want)
			}
		})
	}

	// --- G4 inert-literal classifier matrix (rg/grep ONLY) ------------------
	//
	// The raw forbidden-pattern scanner sees forbidden literals (e.g. `/tmp`
	// matching system-tmp-access) ANYWHERE in the command string. The G4
	// classifier proves — via the EXISTING bash parser — that a forbidden
	// match appears ONLY in the search-pattern operand of a single simple
	// rg/grep command, in which case evaluate() disregards the match and
	// falls through to the allowlist (which authorizes `rg *`/`grep *`).
	//
	// Load-bearing invariants pinned here:
	//   - Pattern-operand matches ARE suppressed end-to-end through the Go
	//     hook + node bridge + WASM parser (not just unit-tested in JS).
	//   - Path-operand matches, real operations, and any complex shell
	//     construct still DENY: the classifier fails closed.
	//   - Deny-before-allowlist ordering is preserved (the raw scan still
	//     runs first; the classifier only consults on a CONFIRMED match).
	//   - Existing behavior for non-rg/grep unallowlisted commands (deny)
	//     is unchanged.
	//
	// Each command is passed as a single-element argv so eval.js's
	// argv.join(" ") yields exactly the intended command string.
	g4Cases := []struct {
		name string
		cmd  string
		want Action
	}{
		// --- MUST ALLOW: pattern operand, forbidden match is inert ---------
		{
			name: "rg -F -- '/tmp' docs allowed (G4 pattern operand, -- terminator)",
			cmd:  `rg -F -- '/tmp' docs`,
			want: Allow,
		},
		{
			name: "grep -F '/tmp' docs allowed (G4 pattern operand, grep)",
			cmd:  `grep -F '/tmp' docs`,
			want: Allow,
		},
		{
			name: "rg '/tmp' docs allowed (G4 no-flag pattern operand)",
			cmd:  `rg '/tmp' docs`,
			want: Allow,
		},
		{
			name: "rg --fixed-strings '/tmp' file allowed (G4 long-form whitelist flag)",
			cmd:  `rg --fixed-strings '/tmp' file`,
			want: Allow,
		},
		{
			name: "rg '/tmp' file1 file2 allowed (G4 multiple paths)",
			cmd:  `rg '/tmp' file1 file2`,
			want: Allow,
		},

		// --- MUST DENY: forbidden match is a real operation ---------------
		{
			// /tmp is a PATH operand, not a pattern. The regex matches the
			// path token, so the classifier refuses to suppress.
			name: "rg needle /tmp/file denied (G4 /tmp in path operand, not pattern)",
			cmd:  `rg needle /tmp/file`,
			want: Deny,
		},
		{
			// Not rg/grep: classifier returns false, original deny stands.
			name: "cat /tmp/file denied (G4 not rg/grep family)",
			cmd:  `cat /tmp/file`,
			want: Deny,
		},
		{
			// Redirection: root's only named child is redirected_statement,
			// not command → root-structure check fails.
			name: "echo x >/tmp/file denied (G4 redirection construct)",
			cmd:  `echo x >/tmp/file`,
			want: Deny,
		},
		{
			// Chain: root's only named child is list → root-structure check
			// fails, AND the second leg is a real /tmp access.
			name: "echo '/tmp' && cat /tmp/file denied (G4 chain construct)",
			cmd:  `echo '/tmp' && cat /tmp/file`,
			want: Deny,
		},
		{
			// Command substitution inside the pattern string: a
			// command_substitution named descendant appears →
			// hasOnlyAllowedNamedNodes fails closed.
			name: `rg "$(cat /tmp/x)" files denied (G4 command substitution in pattern)`,
			cmd:  `rg "$(cat /tmp/x)" files`,
			want: Deny,
		},
		{
			// Executor wrapper: command_name is vh-agent-harness (not rg/grep)
			// → classifier returns false; original deny stands. The nested
			// payload is intentionally OUT OF SCOPE for the G4 grammar
			// (single simple rg/grep command only).
			name: "vh-agent-harness exec bash -c 'cat /tmp/file' denied (G4 executor wrapper out of scope)",
			cmd:  `vh-agent-harness exec bash -c 'cat /tmp/file'`,
			want: Deny,
		},
		{
			// Forbidden match appears in BOTH a path operand AND would match
			// a pattern: the per-path regex test fails closed.
			name: "rg pattern /tmp /etc denied (G4 forbidden match in path operand)",
			cmd:  `rg pattern /tmp /etc`,
			want: Deny,
		},
		{
			// Combined short flag -Fi is NOT in the closed whitelist
			// {"-F","--fixed-strings"}: classifying it would require a
			// growing option parser. Fail closed → original deny stands.
			name: "rg -Fi '/tmp' file denied (G4 combined short flag not in whitelist)",
			cmd:  `rg -Fi '/tmp' file`,
			want: Deny,
		},
		{
			// Value-taking flag -e is NOT in the whitelist: classifying it
			// would require a value-aware parser. Fail closed.
			name: "rg -e '/tmp' files denied (G4 value-taking flag not in whitelist)",
			cmd:  `rg -e '/tmp' files`,
			want: Deny,
		},
		{
			// Bare `rg PATTERN` reading stdin: no PATH operand, so the
			// pattern vs path role cannot be proven. Grammar requires
			// PATTERN PATH+; reject as ambiguous per design.
			name: "rg '/tmp' denied (G4 no path operand, ambiguous layout)",
			cmd:  `rg '/tmp'`,
			want: Deny,
		},
		{
			// Forbidden match appears in BOTH the pattern AND a path. Even
			// though the pattern is a search string, the path is a real
			// /tmp access → per-path regex test fails closed.
			name: "rg -F '/tmp' /tmp denied (G4 forbidden match in pattern AND path)",
			cmd:  `rg -F '/tmp' /tmp`,
			want: Deny,
		},

		// --- UNCHANGED-BEHAVIOR controls ----------------------------------
		{
			// Non-rg/grep unallowlisted command. The classifier is never
			// consulted (command_name ≠ rg/grep). Original deny path stands.
			name: "nonsense-cmd foo bar denied (G4 control: unchanged non-rg/grep unallowlisted)",
			cmd:  `nonsense-cmd foo bar`,
			want: Deny,
		},
	}
	for _, c := range g4Cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, _, evalErr := h.Evaluate(context.Background(), []string{c.cmd})
			if evalErr != nil {
				t.Fatalf("Evaluate(%q) error %v; want verdict %s (no bridge error)", c.cmd, evalErr, c.want)
			}
			if got != c.want {
				t.Errorf("Evaluate(%q) = %s; want %s", c.cmd, got, c.want)
			}
		})
	}

	// --- RF-B: shell file-authoring deny matrix (Go -> node -> WASM -> evaluate())
	//
	// Pins RF-B: a structural classifier in evaluate() that detects shell
	// file-authoring forms (output redirection `>` / `>>` to a file) and
	// denies them with a Write-tool routing message BEFORE the forbidden-
	// pattern scan runs.
	//
	// Load-bearing invariants pinned here:
	//   - clean output redirects (echo/printf/cat > file, >> append) are
	//     DENIED — previously these were ALLOWED (echo/printf/cat are readonly
	//     and the redirect was invisible to the allowlist check).
	//   - the A4 accepted behavior change: `printf 'see git commit' > file`
	//     is DENIED — previously the git-mutation-bypass inspector carve-out
	//     (printf in command position) exempted the prose AND the redirect was
	//     invisible to the allowlist, so the command was ALLOWED.
	//   - heredoc-to-file with a forbidden token in the body is DENIED for the
	//     RIGHT reason (file-authoring, not the token): previously system-tmp-
	//     access fired on `/tmp` in the heredoc body. The deny REASON is
	//     verified in the standalone check below the table.
	//   - /dev/null redirects are NOT file-authoring (discard) and stay ALLOW.
	//   - bare echo/printf prose (no redirect) is still ALLOWED (inspector
	//     carve-out preserved; RF-B does not touch commands without output
	//     redirects).
	//   - fail-closed: chains (&&) with redirects do NOT fire RF-B — they fall
	//     through to the existing scan unchanged (only-adds-denials: RF-B
	//     never weakens the scan).
	rfbCases := []struct {
		name string
		cmd  string
		want Action
	}{
		// --- MUST DENY: clean output redirect (previously ALLOWED) ---
		{
			name: "echo hello > tmp/x denied (RF-B file-authoring)",
			cmd:  `echo hello > tmp/x`,
			want: Deny,
		},
		{
			// A4 accepted behavior change: printf single-line with git-commit
			// content redirected to a file. Previously ALLOWED (printf carve-out
			// + redirect invisible to allowlist). Now DENIED by RF-B.
			name: "printf git-commit > tmp/x denied (A4 accepted behavior change)",
			cmd:  `printf 'see git commit notes' > tmp/x`,
			want: Deny,
		},
		{
			name: "cat > tmp/x denied (RF-B cat redirect)",
			cmd:  `cat > tmp/x`,
			want: Deny,
		},
		{
			name: "echo value >> tmp/log denied (RF-B append redirect)",
			cmd:  `echo value >> tmp/log`,
			want: Deny,
		},
		{
			// The exact FP scenario: heredoc-to-file with /tmp in the body.
			// Previously DENIED via system-tmp-access (confusing reason). Now
			// DENIED via RF-B file-authoring (actionable reason — verified
			// in the standalone reason check below).
			name: "heredoc-to-file with /tmp in body denied (RF-B drains system-tmp-access FP)",
			cmd:  "cat > tmp/x <<'EOF'\nsee /tmp reference\nEOF",
			want: Deny,
		},

		// --- MUST ALLOW: not file-authoring or excluded ---
		{
			name: "echo hello allowed (no redirect, RF-B does not fire)",
			cmd:  `echo hello`,
			want: Allow,
		},
		{
			// /dev/null discards output — NOT file-authoring. Excluded from
			// RF-B so legitimate output-suppression stays allowed.
			name: "echo hello > /dev/null allowed (devnull excluded from RF-B)",
			cmd:  `echo hello > /dev/null`,
			want: Allow,
		},
		{
			// Quoted /dev/null (single quotes): tree-sitter .text retains the
			// quotes, so the exclusion must use unquoteToken. Pinned so the
			// normalization cannot regress (commit-review F1).
			name: "echo hello > '/dev/null' allowed (quoted devnull, unquoteToken normalization)",
			cmd:  `echo hello > '/dev/null'`,
			want: Allow,
		},
		{
			// Quoted /dev/null (double quotes): same normalization path.
			name: `echo hello > "/dev/null" allowed (quoted devnull, unquoteToken normalization)`,
			cmd:  `echo hello > "/dev/null"`,
			want: Allow,
		},
		{
			// RF-B fd-prefix skip (defer-037): a stderr redirect (`2> file`) is
			// NOT stdout file-authoring. detectShellFileAuthoring returns null
			// for this form via TWO converging fail-closed paths: (1) the
			// closed allow-set check (hasOnlyAllowedNamedNodes) rejects the
			// `file_descriptor` node (the `2`) because it is NOT in
			// FILE_AUTHORING_ALLOWED_NODE_TYPES; (2) even if it were, the
			// fd-prefix loop skips fd-prefixed file_redirects (operator first
			// child is a NAMED file_descriptor node, not anonymous `>`/`>>`).
			// Either way → no output redirect → null → caller falls through to
			// the scan. No forbidden tokens → allowlist `echo *` matches →
			// ALLOW. Pins that stderr redirects are not over-denied as file-
			// authoring (only-adds-denials: RF-B never turns an allow into a
			// deny here). Test pins the OUTCOME (Allow), robust to which
			// internal path returned null.
			name: "echo x 2> tmp/err.log allowed (RF-B fd-prefix skip, stderr redirect)",
			cmd:  `echo x 2> tmp/err.log`,
			want: Allow,
		},
		{
			// Bare echo prose (no redirect). The inspector carve-out still
			// exempts this from git-mutation-bypass. RF-B does NOT fire (no
			// output redirect present). This is the FP drain for non-file-
			// authoring prose: agents use bare echo/printf, which is allowed.
			name: "echo prose with git commit allowed (no redirect, inspector carve-out preserved)",
			cmd:  `echo "see git commit notes"`,
			want: Allow,
		},

		// --- UNCHANGED: RF-B fail-closed (ambiguous/chained) ---
		{
			// Chain: RF-B sees a `list` node in the AST → fail-closed →
			// falls through to scan. No forbidden tokens → allowlist matches
			// echo/cat → ALLOW (unchanged from pre-RF-B behavior). RF-B does
			// not over-deny chained forms.
			name: "echo x && cat > tmp/y allowed (RF-B fail-closed on chain)",
			cmd:  `echo x && cat > tmp/y`,
			want: Allow,
		},
		{
			// RF-B fail-closed cross-boundary (defer-036): the SAME chained
			// redirect shape as the ALLOW baseline directly above, but with a
			// forbidden token (/tmp) in the redirect target. detectShellFileAuthoring
			// fail-closes on the `list` (&&) root → returns null → the existing
			// forbidden-pattern scan runs UNCHANGED and catches the /tmp token
			// via system-tmp-access. This is the cross-boundary invariant pin:
			// when RF-B returns null for a compound/chained form, the closed
			// grammar must NOT weaken the scan — a formerly scan-denied command
			// stays DENIED. (only-adds-denials: RF-B never carves scan denials.)
			name: "echo x && cat > /tmp/y denied (RF-B fail-closed on chain, scan catches /tmp)",
			cmd:  `echo x && cat > /tmp/y`,
			want: Deny,
		},
	}
	for _, c := range rfbCases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got, _, evalErr := h.Evaluate(context.Background(), []string{c.cmd})
			if evalErr != nil {
				t.Fatalf("Evaluate(%q) error %v; want verdict %s (no bridge error)", c.cmd, evalErr, c.want)
			}
			if got != c.want {
				t.Errorf("Evaluate(%q) = %s; want %s", c.cmd, got, c.want)
			}
		})
	}

	// RF-B reason check: the heredoc-to-file drain case must be denied with
	// the file-authoring reason (NOT system-tmp-access). This proves the FP
	// is drained for the right reason — the deny message routes to the Write
	// tool, not to the forbidden-token scan that previously fired on the body.
	{
		cmd := "cat > tmp/x <<'EOF'\nsee /tmp reference\nEOF"
		got, reason, evalErr := h.Evaluate(context.Background(), []string{cmd})
		if evalErr != nil {
			t.Fatalf("Evaluate(heredoc drain) error %v; want Deny (no bridge error)", evalErr)
		}
		if got != Deny {
			t.Fatalf("Evaluate(heredoc drain) = %s; want Deny", got)
		}
		if !strings.Contains(reason, "file-authoring") {
			t.Errorf("heredoc drain deny reason should mention file-authoring; got %q", reason)
		}
		if strings.Contains(reason, "system-tmp-access") {
			t.Errorf("heredoc drain deny reason should NOT mention system-tmp-access (RF-B fires before scan); got %q", reason)
		}
	}

	// --- git global-flag walker matrix ---------------------------------------
	//
	// These cases exercise walkGitGlobals end-to-end through eval.js. They use
	// runNode (NOT h.Evaluate) so the JSON emitted by eval.js can be asserted
	// directly — specifically that NO `rewrite` field is present (Option A: the
	// engine decides allow/deny/ask via parse, it never produces a command
	// rewrite and the plugin wrapper never mutates output.args.command).
	//
	// commandCwd inside eval.js resolves to repoRoot() = scratch (plugins/ ->
	// two up), so `git -C <scratch-abs> ...` is the absolute-commandCwd shape
	// (the conditional strip case). External paths use /var/x deliberately —
	// NEVER /tmp, which system-tmp-access would deny before the walker runs.
	scratchSub := filepath.Join(scratch, "subdir")
	globalFlagCases := []struct {
		name       string
		cmd        string
		wantAction string // "allow" | "deny" | "ask"
	}{
		// 1. always-strip flag -> internal stripped match -> allow. NO rewrite
		//    is emitted (Option A: detect/parse for the decision only).
		{name: "--no-pager diff allow (internal stripped match, no rewrite)", cmd: `git --no-pager diff x`, wantAction: "allow"},
		// 2. combo always + conditional(strip) -> internal stripped match -> allow.
		{name: "--no-pager -C <commandCwd> log allow (no rewrite)", cmd: fmt.Sprintf(`git --no-pager -C %s log`, scratch), wantAction: "allow"},
		// 3. conditional(strip) alone -> internal stripped match -> allow.
		{name: "-C <abs commandCwd> diff allow (no rewrite)", cmd: fmt.Sprintf(`git -C %s diff`, scratch), wantAction: "allow"},
		// 4. conditional(keep): abs in-project subdir != commandCwd -> ask.
		{name: "-C <abs in-project subdir> diff asks", cmd: fmt.Sprintf(`git -C %s diff`, scratchSub), wantAction: "ask"},
		// 5. external -C readonly -> ask (verb extracted, not fully strippable).
		{name: "-C <abs external> diff asks", cmd: `git -C /var/x diff`, wantAction: "ask"},
		// 6. external -C mutation -> deny (mutation-slip guard; must NOT be ask).
		{name: "-C <abs external> commit denied (mutation-slip guard)", cmd: `git -C /var/x commit -m x`, wantAction: "deny"},
		// 7. relative -C -> deny + notice.
		{name: "-C ./subdir diff denied (relative -C notice)", cmd: `git -C ./subdir diff`, wantAction: "deny"},
		// 8. never-strip flag present -> ask.
		{name: "--git-dir=/x diff asks (never-strip flag)", cmd: `git --git-dir=/x diff`, wantAction: "ask"},
		// 9. mutation overrides strippability: abs commandCwd + mutation -> deny.
		{name: "-C <abs commandCwd> commit denied (mutation overrides strip)", cmd: fmt.Sprintf(`git -C %s commit -m x`, scratch), wantAction: "deny"},
		// 10. relative -C + mutation -> deny (relative path wins; mutation never reached).
		{name: "-C . commit denied (relative -C, case 10)", cmd: `git -C . commit -m x`, wantAction: "deny"},
		// Bonus A: --paging=no is not a real flag -> not stripped -> ask.
		{name: "--paging=no log asks (--paging=no is not a real git flag)", cmd: `git --paging=no log`, wantAction: "ask"},
		// Bonus B: info-flag with no verb -> allow (read-only terminal info).
		{name: "--help allowed (info-flag terminal request)", cmd: `git --help`, wantAction: "allow"},
		// Bonus C: --version info-flag -> allow.
		{name: "--version allowed (info-flag terminal request)", cmd: `git --version`, wantAction: "allow"},
		// Bonus D: bare readonly (no globals) -> allow.
		{name: "bare git diff allow", cmd: `git diff`, wantAction: "allow"},
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
			// Option A contract: the engine NEVER emits a `rewrite` field.
			// Assert it is absent (not merely empty) from the JSON.
			jsonRewriteAbsent(t, out)
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

// jsonRewriteAbsent asserts that the single JSON line eval.js emits on stdout
// contains NO "rewrite" field. Under Option A the engine never produces a
// command rewrite (detect/parse drives the decision only); the plugin wrapper
// never writes back to output.args.command. Proving the field is ABSENT (not
// merely empty) nails the contract.
func jsonRewriteAbsent(t *testing.T, stdout string) {
	t.Helper()
	line := strings.TrimSpace(stdout)
	if line == "" {
		t.Fatalf("eval.js produced no stdout; cannot read rewrite")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Fatalf("eval.js stdout not JSON: %q (%v)", line, err)
	}
	if _, ok := raw["rewrite"]; ok {
		t.Errorf("eval.js emitted a `rewrite` field; under Option A it must be absent (stdout=%q)", line)
	}
}
