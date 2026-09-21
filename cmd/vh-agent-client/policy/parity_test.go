// parity_test.go — the P3.5 parity additions (the opencode deny floor
// vs the native policy engine; the audited contract lives in
// tmp/p3.5-parity.md). Three additions, each pinned red-first:
//
//  1. the 15 git PLUMBING verbs missing from the closed mutation set
//     (opencode's boundary is hyphen-aware — `(?![\w-])` — so every
//     hyphenated mutating plumbing command must be enumerated as a full
//     token; 30 → 45 verbs);
//  2. the infra-lifecycle hard-deny class (apt/apt-get install-class,
//     the user-management family, ssh host-key bypass options, scp);
//  3. the system-temp write-authoring floor in run_shell (redirection
//     operators + tee targeting absolute paths under /tmp, /var/tmp,
//     /dev/shm — the client-side floor; in-root vs out-of-root is the
//     daemon sandbox's job).
//
// Deny-direction over-approximations (prose mentions deny too; no
// inspector carve-outs) are pinned AS SUCH — the boundary cases prove
// the classes are closed, not substring-matching.
package policy

import (
	"encoding/json"
	"strings"
	"testing"
)

// wantDenyNotReason asserts a hard-deny whose reason does NOT contain
// the key — the reason-distinction pin (e.g. a relative redirection
// denies via the plain-word gate, not via the system-temp class).
func wantDenyNotReason(t *testing.T, name, tool, argsJSON, reasonKey string) {
	t.Helper()
	reason, denied := HardDeny(tool, json.RawMessage(argsJSON))
	if !denied {
		t.Fatalf("%s: expected a hard-deny, got none (tool=%s args=%s)", name, tool, argsJSON)
	}
	if strings.Contains(reason, reasonKey) {
		t.Fatalf("%s: reason %q must NOT contain %q (wrong class fired)", name, reason, reasonKey)
	}
}

// --- 1. the 15 git plumbing verbs ---------------------------------------------

// TestGitMutationPlumbingVerbsDeny (P3.5 parity gap 1): every verb in
// opencode's 35-verb GIT_MUTATION_VERBS set that the native 30-verb set
// lacked denies after the addition — space form, adjacency form, and
// the count grows to exactly 45.
func TestGitMutationPlumbingVerbsDeny(t *testing.T) {
	for _, sub := range []string{
		"add--interactive", "checkout--worker", "checkout-index",
		"commit-graph", "commit-tree", "merge-file", "merge-index",
		"merge-octopus", "merge-one-file", "merge-ours", "merge-recursive",
		"merge-resolve", "merge-subtree", "merge-tree", "update-index",
	} {
		wantDeny(t, "git "+sub, "run_shell", `{"command":"git `+sub+`"}`, "git")
	}
	// Wrapper shape (word-level git-adjacency): the verb joining the
	// closed set automatically extends the adjacency scan.
	wantDeny(t, "sudo git commit-tree", "run_shell", `{"command":"sudo git commit-tree HEAD"}`, "git-adjacent")
	// The git-core dashed dispatch form was already covered
	// (exec-intermediary git-* tripwire); it must still deny with that
	// class's reason. (A bare `merge-tree` is NOT a git dispatch
	// binary — git installs `git-merge-tree` — and stays out of scope.)
	wantDeny(t, "git-core dashed dispatch", "run_shell", `{"command":"git-merge-tree --write-our base our their"}`, "dashed-form")
}

// TestGitMutationVerbSetCount45 pins the audited count (30 + 15 = 45;
// the parity report's contradiction note: the mission said "31", the
// source had 30 — the docs state the auditable number).
func TestGitMutationVerbSetCount45(t *testing.T) {
	if got := len(gitMutationSubcommands); got != 45 {
		t.Fatalf("closed git-mutation set has %d verbs, want 45 (30 pre-P3.5 + the 15 plumbing verbs)", got)
	}
	for _, sub := range []string{"commit-tree", "merge-tree", "update-index", "checkout-index", "add--interactive", "commit-graph", "checkout--worker", "merge-one-file"} {
		if !gitMutationSubcommands[sub] {
			t.Fatalf("verb %q missing from the closed mutation set", sub)
		}
	}
	// Read-only git stays rule-eligible.
	for _, sub := range []string{"status", "log", "diff", "show", "grep", "ls-files", "cat-file"} {
		wantNoDeny(t, "read-only git "+sub, "run_shell", `{"command":"git `+sub+`"}`)
	}
}

// --- 2. infra-lifecycle class ---------------------------------------------------

// TestInfraLifecycleAptInstallClass (P3.5 parity gap 2): apt/apt-get
// with a subcommand in the CLOSED install-class set denies; read-only
// apt (list/search/policy/show) stays rule-eligible.
func TestInfraLifecycleAptInstallClass(t *testing.T) {
	for _, sub := range []string{"install", "reinstall", "upgrade", "full-upgrade", "remove", "purge", "autoremove"} {
		wantDeny(t, "apt "+sub, "run_shell", `{"command":"apt `+sub+` curl"}`, "infra-lifecycle")
		wantDeny(t, "apt-get "+sub, "run_shell", `{"command":"apt-get `+sub+` curl"}`, "infra-lifecycle")
	}
	// Flags between argv0 and the subcommand (the adjacency shape, like
	// the git class): both the apt word and the subcommand word in one
	// segment deny — wrappers included.
	wantDeny(t, "apt-get -y install", "run_shell", `{"command":"apt-get -y install curl"}`, "infra-lifecycle")
	wantDeny(t, "sudo apt upgrade", "run_shell", `{"command":"sudo apt upgrade"}`, "infra-lifecycle")
	// Compound tail: every segment is checked.
	wantDeny(t, "compound hides apt", "run_shell", `{"command":"ls && apt-get remove nano"}`, "infra-lifecycle")

	// Boundaries (closed set): read-only apt subcommands and the
	// distinct apt-cache/apt-key argv0s stay rule-eligible.
	for _, sub := range []string{"list", "search", "policy", "show"} {
		wantNoDeny(t, "read-only apt "+sub, "run_shell", `{"command":"apt `+sub+`"}`)
	}
	wantNoDeny(t, "apt-cache is not apt", "run_shell", `{"command":"apt-cache policy curl"}`)
	wantNoDeny(t, "apt-key is not apt", "run_shell", `{"command":"apt-key list"}`)
	// "install" prose without an apt word in the segment does not trip.
	wantNoDeny(t, "install prose", "run_shell", `{"command":"echo install the package"}`)
}

// TestInfraLifecycleUserManagement: the user/group mutation family
// denies word-anywhere, basename-aware (path-qualified and sudo-wrapped
// forms included). Over-approximation pinned: a benign whole-word
// mention denies too (deny direction; no inspector carve-outs).
func TestInfraLifecycleUserManagement(t *testing.T) {
	for _, w := range []string{"usermod", "useradd", "groupadd", "groupmod", "groupdel", "userdel", "deluser"} {
		wantDeny(t, w, "run_shell", `{"command":"`+w+` bob"}`, "infra-lifecycle")
	}
	wantDeny(t, "sudo useradd", "run_shell", `{"command":"sudo useradd -m bob"}`, "infra-lifecycle")
	wantDeny(t, "path-qualified", "run_shell", `{"command":"/usr/sbin/userdel bob"}`, "infra-lifecycle")
	// Whole-word anchoring, never substring: similar-shaped words do not trip.
	wantNoDeny(t, "userslist is not the family", "run_shell", `{"command":"echo userslist"}`)
	wantNoDeny(t, "delusernotes is not the family", "run_shell", `{"command":"cat delusernotes.txt"}`)
}

// TestInfraLifecycleSSHHostKeyBypass: the UNION of the opencode rule
// (StrictHostKeyChecking=no, UserKnownHostsFile=/dev/null) and the
// mission text (accept-new) — detached `-o` value, attached
// `-o<opt>=…`, and bare value words all deny. Strict= yes/accept-new
// absent ⇒ ssh itself stays rule-eligible.
func TestInfraLifecycleSSHHostKeyBypass(t *testing.T) {
	wantDeny(t, "ssh -o detached no", "run_shell", `{"command":"ssh -o StrictHostKeyChecking=no host"}`, "infra-lifecycle")
	wantDeny(t, "ssh -o attached no", "run_shell", `{"command":"ssh -oStrictHostKeyChecking=no host"}`, "infra-lifecycle")
	wantDeny(t, "ssh accept-new", "run_shell", `{"command":"ssh -o StrictHostKeyChecking=accept-new host"}`, "infra-lifecycle")
	wantDeny(t, "ssh known-hosts null", "run_shell", `{"command":"ssh -o UserKnownHostsFile=/dev/null host"}`, "infra-lifecycle")
	wantDeny(t, "scp-shaped too", "run_shell", `{"command":"ssh -o UserKnownHostsFile=/dev/null host ls"}`, "infra-lifecycle")

	// Boundaries: ordinary ssh is not the class's business.
	wantNoDeny(t, "plain ssh", "run_shell", `{"command":"ssh host ls"}`)
	wantNoDeny(t, "benign option", "run_shell", `{"command":"ssh -o BatchMode=yes host"}`)
	// StrictHostKeyChecking=YES (the safe setting) is NOT this class —
	// but it DOES deny via the pre-existing secret-env class (the NAME
	// matches the KEY scrub pattern): pinned as a documented
	// co-fire/over-approximation of class 1, not an infra trip.
	wantDenyNotReason(t, "strict yes is secret-env, not infra", "run_shell", `{"command":"ssh -o StrictHostKeyChecking=yes host"}`, "infra-lifecycle")
	wantNoDeny(t, "user file", "run_shell", `{"command":"ssh -o UserKnownHostsFile=/etc/ssh/known host"}`)
}

// TestInfraLifecycleScpAnyInvocation: scp denies ANY invocation
// (word-anywhere, basename-aware — the parity contract; a benign
// mention denies too, deny direction documented in decide.go).
func TestInfraLifecycleScpAnyInvocation(t *testing.T) {
	wantDeny(t, "scp upload", "run_shell", `{"command":"scp file host:/srv/"}`, "infra-lifecycle")
	wantDeny(t, "path scp", "run_shell", `{"command":"/usr/bin/scp a b:/x"}`, "infra-lifecycle")
	wantDeny(t, "scp prose (over-approx)", "run_shell", `{"command":"echo scp is how files moved"}`, "infra-lifecycle")
	// Whole-word anchoring: substrings never trip.
	wantNoDeny(t, "scpnotes is not scp", "run_shell", `{"command":"cat scpnotes.txt"}`)
}

// --- 3. system-temp write authoring ---------------------------------------------

// TestSystemTempWriteAuthoringRedirection: redirection operators
// (attached or detached target) pointing an absolute path under /tmp,
// /var/tmp, or /dev/shm deny with the class reason (not the incidental
// plain-word-gate reason — the reason-gap half of parity gap 3).
func TestSystemTempWriteAuthoringRedirection(t *testing.T) {
	for _, c := range []struct{ name, cmd string }{
		{"attached >", "echo hi >/tmp/x"},
		{"detached >", "echo hi > /tmp/x"},
		{"detached >>", "echo hi >> /tmp/x"},
		{"attached >>", "echo hi >>/tmp/x"},
		{"stderr 2>", "cmd 2> /tmp/err"},
		{"stderr 2>>", "cmd 2>>/tmp/err"},
		{"both &>", "cmd &> /tmp/all"},
		{"both &>>", "cmd &>> /tmp/all"},
		{"var/tmp", "echo hi > /var/tmp/x"},
		{"dev/shm", "echo hi > /dev/shm/x"},
		{"tmp itself", "echo hi > /tmp"},
	} {
		wantDeny(t, c.name, "run_shell", `{"command":"`+c.cmd+`"}`, "system-temp")
	}
	// Compound tail: the redirection segment is checked.
	wantDeny(t, "compound hides temp write", "run_shell", `{"command":"ls && echo x > /tmp/f"}`, "system-temp")
	// DOCUMENTED BOUNDARY: shapes whose operator itself carries a
	// split character — the `>&` and clobber `>|`/`>>|` families —
	// lexically over-split (the single-pass segment split cuts at
	// EVERY `&` and `|`, so these operators NEVER keep their target
	// pairing): the operator lands in one segment's tail, the target
	// in the next, and the generic plain-word gate denies the bare
	// ">" word. Outcome identical (deny), reason generic — pinned
	// honestly rather than fragile cross-segment correlation. The
	// operators stay in the class's closed list (harmless, and a
	// future segmenter change would light them up); `&>`/`&>>` keep
	// the class reason (the operator stays WITH the target after the
	// split).
	wantDenyNotReason(t, "attached >& over-split corner", "run_shell", `{"command":"cmd >&/tmp/all"}`, "system-temp")
	wantDenyNotReason(t, "detached >& over-split corner", "run_shell", `{"command":"cmd >& /tmp/all"}`, "system-temp")
	wantDenyNotReason(t, "bare clobber >| over-split corner", "run_shell", `{"command":"cmd >| /tmp/over"}`, "system-temp")
	wantDenyNotReason(t, "bare append-clobber >>| corner", "run_shell", `{"command":"cmd >>|/tmp/over"}`, "system-temp")
}

// TestSystemTempWriteAuthoringTee: tee targeting the closed roots
// denies (the outcome-gap half of parity gap 3 — all plain words, so
// nothing else denies it today).
func TestSystemTempWriteAuthoringTee(t *testing.T) {
	wantDeny(t, "tee /tmp", "run_shell", `{"command":"echo x | tee /tmp/log"}`, "system-temp")
	wantDeny(t, "tee /var/tmp", "run_shell", `{"command":"cmd | tee /var/tmp/log"}`, "system-temp")
	wantDeny(t, "tee /dev/shm", "run_shell", `{"command":"cmd | tee /dev/shm/log"}`, "system-temp")
	wantDeny(t, "tee -a /tmp", "run_shell", `{"command":"cmd | tee -a /tmp/log"}`, "system-temp")
}

// TestSystemTempWriteBoundaries: the class is the CLOSED client-side
// /tmp floor — relative targets and READS of the roots are not its
// business (reads fall to rules; relative writes are the daemon
// sandbox's job; a relative redirection still denies via the plain-word
// gate, but NOT with this class's reason).
func TestSystemTempWriteBoundaries(t *testing.T) {
	wantNoDeny(t, "read /tmp", "run_shell", `{"command":"ls -la /tmp"}`)
	wantNoDeny(t, "read /var/tmp file", "run_shell", `{"command":"cat /var/tmp/x"}`)
	wantNoDeny(t, "tee relative", "run_shell", `{"command":"cmd | tee out.txt"}`)
	wantNoDeny(t, "write tool under /tmp is a path rule", "write", `{"path":"/tmp/build/out.bin","content":"x"}`)
	// /tmpfoo is NOT under /tmp (rooted-prefix semantics).
	wantNoDeny(t, "tmp-adjacent name", "run_shell", `{"command":"echo x | tee /tmpfoo/log"}`)
	// A relative redirection denies via the plain-word gate (2a), NOT
	// via the system-temp class — the reason distinction.
	wantDenyNotReason(t, "relative redirect not this class", "run_shell", `{"command":"echo hi > out.txt"}`, "system-temp")
	// Process substitution is out of scope by contract (gate 2a owns it).
	wantDenyNotReason(t, "process substitution not this class", "run_shell", `{"command":"diff <(ls) <(ls -a)"}`, "system-temp")
}
