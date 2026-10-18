// reviewfix_test.go — the regression battery for the three review
// BLOCKs against the P3 policy slice (hotfix pass). Written RED
// first: against the defective code, the F2/F3 cases show the
// evasions ALLOWING through a matching rule and the F1 case shows the
// cross-key parse error citing a byte offset and line text that do
// not belong to the line it names.
package policy

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- F2 (critical, security): single '&' and argument-position
// substitution must not smuggle a git mutation past an argv0=git rule.
// bash -c executes BOTH sides of `a & b` (asynchronous list), so an
// &-separated tail is a real command segment.

func TestReviewF2SingleAmpersandEvadesGitRule(t *testing.T) {
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")

	// `git status & git push …`: pre-fix the single '&' did not split
	// segments, so the whole command was ONE read-only-headed segment
	// → rule-matched → ALLOW (the evasion).
	d := decideOf(t, gitRule, "run_shell", `{"command":"git status & git push origin HEAD:main"}`)
	if d.Kind != DecisionDeny {
		t.Fatalf("F2: single-& git mutation must HARD-DENY behind the argv0=git rule, got %+v", d)
	}
	if !strings.Contains(d.Reason, "git") {
		t.Fatalf("F2: deny reason must name git, got %q", d.Reason)
	}

	// Same shape with the mutation first.
	d = decideOf(t, gitRule, "run_shell", `{"command":"git push origin HEAD:main & git status"}`)
	if d.Kind != DecisionDeny {
		t.Fatalf("F2: single-& mutation-first must HARD-DENY, got %+v", d)
	}
}

func TestReviewF2ArgumentPositionSubstitutionEvadesGitRule(t *testing.T) {
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")

	// `git log $(git push)`: git-headed segment whose non-first words
	// carry command substitution. Pre-fix only the subcommand word was
	// shape-checked (log: read-only) → rule-matched → ALLOW.
	d := decideOf(t, gitRule, "run_shell", `{"command":"git log $(git push)"}`)
	if d.Kind != DecisionDeny {
		t.Fatalf("F2: argument-position $(…) substitution must HARD-DENY, got %+v", d)
	}
	if !strings.Contains(d.Reason, "substitution") {
		t.Fatalf("F2: deny reason must name substitution, got %q", d.Reason)
	}

	// Backtick shape of the same evasion.
	d = decideOf(t, gitRule, "run_shell", `{"command":"git log --format=`+"`git push`"+`"}`)
	if d.Kind != DecisionDeny {
		t.Fatalf("F2: argument-position backtick substitution must HARD-DENY, got %+v", d)
	}

	// Substitution on the flag-bearing tail of a read-only subcommand.
	d = decideOf(t, gitRule, "run_shell", `{"command":"git diff HEAD@{$(git push)}"}`)
	if d.Kind != DecisionDeny {
		t.Fatalf("F2: substitution in a later git argument must HARD-DENY, got %+v", d)
	}

	// Boundary: clean read-only git (incl. compound) stays allowed.
	for _, cmd := range []string{"git status", "git log -3 --oneline", "git status && git log"} {
		if d := decideOf(t, gitRule, "run_shell", `{"command":"`+cmd+`"}`); d.Kind != DecisionAllow {
			t.Fatalf("clean read-only command %q must stay rule-eligible, got %+v", cmd, d)
		}
	}
}

// --- F3 (major, contract_drift): wrapper shapes hide git mutation
// behind a non-git argv[0]. The README claims git mutation stays
// behind the hard-deny classes even under a broad run_shell allow.

func TestReviewF3WrapperShapesEvadeBroadRunShellRule(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		{"sh -c single-string body", `{"command":"sh -c \"git push origin main --force\""}`},
		{"sudo wrapper", `{"command":"sudo git push"}`},
		{"nohup wrapper", `{"command":"nohup git push"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("F3 %s: must HARD-DENY under a broad run_shell allow, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "git") {
			t.Fatalf("F3 %s: deny reason must name git, got %q", c.name, d.Reason)
		}
	}
}

func TestReviewF3GitAdjacencyBoundary(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	// Documented false-positive direction (accepted): prose holding
	// both the word git and a mutation subcommand denies — the
	// deny-direction over-approximation, documented in decide.go and
	// README.agent.md.
	if d := decideOf(t, broad, "run_shell", `{"command":"echo about git push"}`); d.Kind != DecisionDeny {
		t.Fatalf("F3: git-adjacent prose must deny (documented over-approximation), got %+v", d)
	}

	// The word git WITHOUT a mutation subcommand stays allowed.
	if d := decideOf(t, broad, "run_shell", `{"command":"echo about git status"}`); d.Kind != DecisionAllow {
		t.Fatalf("F3: git word without a mutation subcommand must stay allowed, got %+v", d)
	}
	// Wrapper around READ-ONLY git stays allowed.
	if d := decideOf(t, broad, "run_shell", `{"command":"sudo git status"}`); d.Kind != DecisionAllow {
		t.Fatalf("F3: wrapper around read-only git must stay allowed, got %+v", d)
	}
	// Mutation word WITHOUT the word git stays allowed (the class is
	// git-adjacency, not mutation-word scanning).
	if d := decideOf(t, broad, "run_shell", `{"command":"echo about push"}`); d.Kind != DecisionAllow {
		t.Fatalf("F3: mutation word without the git word must stay allowed, got %+v", d)
	}
}

// --- F1 (major, contract): a cross-key parse error must cite the
// line, byte offset, and verbatim text of the SAME key line — the one
// laterOf selected — not line-from-one-key with byte/text from the
// other.

// =============================================================================
// ROUND 2 — close the evasion GENUS (quote-fragmentation and
// substitution-wrapped mutation), not just the reported shapes.
//
// The principled boundary: a run_shell segment is allow-eligible only
// if EVERY word is PLAIN — after edge-quote trimming, matching the
// conservative identifier grammar ^[A-Za-z0-9_./:@+=-]+$. Any
// non-plain word anywhere (interior quotes, $, backtick, parens, or
// any other shell metacharacter) is UNIDENTIFIABLE lexically, and
// unidentifiable HARD-DENIES: no rule can rescue it. This one rule
// closes the whole fragmentation/substitution family at any encoding,
// any position — and WIDENS the deny-direction over-approximation
// (echo $(date), ls $(pwd) now deny too; documented, accepted).
// =============================================================================

// --- the four reported BLOCK shapes (round 2). Pre-fix, each ALLOWED
// through a broad run_shell rule — the failure receipts below were
// captured showing {allow …} before the fix landed.

func TestReviewR2QuoteFragmentationBypass(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	// C1: `sh -c 'gi''t pu''sh'` — strings.Fields yields 'gi''t /
	// pu''sh'; edge-quote trimming cannot recover git/push from
	// interior-quote fragments, so the adjacency scan missed and the
	// broad rule ALLOWED. bash concatenates the quoted fragments and
	// executes `git push`.
	d := decideOf(t, broad, "run_shell", `{"command":"sh -c 'gi''t pu''sh'"}`)
	if d.Kind != DecisionDeny {
		t.Fatalf("R2 C1: quote-fragmented git push must HARD-DENY under a broad run_shell allow, got %+v", d)
	}
	if !strings.Contains(d.Reason, "unidentifiable") {
		t.Fatalf("R2 C1: deny reason must name unidentifiability, got %q", d.Reason)
	}
}

func TestReviewR2SubstitutionWrappedMutation(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	// C2, three shapes: argv[0] is plain and NOT git; the mutation
	// lives inside a command substitution the lexical adjacency scan
	// cannot see ($(git != git). bash executes the substitution.
	for _, c := range []struct{ name, args string }{
		{"$() substitution", `{"command":"echo $(git push origin main)"}`},
		{"backtick substitution", "{\"command\":\"echo `git push`\"}"},
		{"nested inside sh -c", `{"command":"sh -c \"echo $(git push)\""}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R2 C2 %s: substitution-wrapped git push must HARD-DENY under a broad run_shell allow, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "unidentifiable") {
			t.Fatalf("R2 C2 %s: deny reason must name unidentifiability, got %q", c.name, d.Reason)
		}
	}
}

// --- genus sweeps: the whole fragmentation/substitution family denies,
// at any encoding, any position — including shapes with NO git word at
// all (unidentifiability denies; the widened, accepted cost).

func TestReviewR2GenusSweepsDeny(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	denies := []struct{ name, args string }{
		// mid-word quote fragmentation, any quote kind, any position
		{"fragmented argv0 top-level", `{"command":"gi''t push"}`},
		{"fragmented argv0 double quotes", `{"command":"gi\"t\" push"}`},
		{"fragmented subcommand", `{"command":"git pu''sh"}`},
		{"fragmented inside sh -c (subcommand)", `{"command":"sh -c \"git pu''sh\""}`},
		{"escaped-quote fragmentation inside sh -c", `{"command":"sh -c \"gi\\\"t pu\\\"sh\""}`},
		{"fragmented argv0 inside sh -c", `{"command":"sh -c 'gi''t push'"}`},
		// backticks in any position
		{"backtick anywhere", "{\"command\":\"echo x`git push`\"}"},
		{"benign backtick substitution (no git)", "{\"command\":\"echo `date`\"}"},
		{"backtick nested in sh -c", "{\"command\":\"sh -c \\\"echo `git push`\\\"\"}"},
		// $-family in any word of any segment
		{"bare $VAR", `{"command":"echo $HOME"}`},
		{"braced ${...}", `{"command":"echo ${HOME}"}`},
		{"$() with no git at all (widened)", `{"command":"echo $(date)"}`},
		{"$() in a later word (widened)", `{"command":"ls $(pwd)"}`},
		// paren fragments
		{"paren-wrapped", `{"command":"echo (git push)"}`},
		// multiple segments where only ONE is dirty
		{"clean head, substitution tail", `{"command":"echo hi && echo $(git push)"}`},
		{"clean head, benign-substitution tail (widened)", `{"command":"echo hi && echo $(date)"}`},
		{"clean head, fragmented tail", `{"command":"echo hi && sh -c 'gi''t pu''sh'"}`},
	}
	for _, c := range denies {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R2 genus %s: must HARD-DENY, got %+v", c.name, d)
		}
	}
}

func TestReviewR2BoundaryStaysAllowed(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")
	for _, c := range []struct{ name, args string }{
		{"plain echo", `{"command":"echo hello"}`},
		{"ls with flags and abs path", `{"command":"ls -la /tmp"}`},
		{"git prose without mutation word", `{"command":"echo about git status"}`},
		{"compound plain", `{"command":"go test ./... && go vet ./..."}`},
		{"benign env prefix", `{"command":"FOO=bar ls"}`},
	} {
		if d := decideOf(t, broad, "run_shell", c.args); d.Kind != DecisionAllow {
			t.Fatalf("R2 boundary %s: plain command must stay allowed, got %+v", c.name, d)
		}
	}

	// argv0-constrained rule: clean read-only git stays allowed.
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	for _, cmd := range []string{"git status", "git log -3 --oneline", "git status && git log"} {
		if d := decideOf(t, gitRule, "run_shell", `{"command":"`+cmd+`"}`); d.Kind != DecisionAllow {
			t.Fatalf("R2 boundary argv0: %q must stay allowed, got %+v", cmd, d)
		}
	}

	// Widened over-approximation pinned as INTENDED: a non-plain word
	// denies even in an otherwise-clean read-only git command. `~` is
	// outside the conservative identifier grammar, so `HEAD~1` is
	// unidentifiable — deny (the human runs it themselves, or the
	// operator drops --policy for interactive use; no v1 policy path).
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git diff HEAD~1"}`); d.Kind != DecisionDeny {
		t.Fatalf("R2 widened boundary: git diff HEAD~1 must now DENY (~ is outside the plain-word grammar), got %+v", d)
	}
}

// =============================================================================
// ROUND 3 — exec-intermediary tripwires (closed class) + honest
// contract reframe.
//
// Round-3 BLOCK (critical): the git scans are SEGMENT-LOCAL, so a
// pipe assembles a mutation across segments — `echo push | xargs
// git` puts the mutation word in segment 1 (no git word) and `git`
// in segment 2 (no mutation word); adjacency needs BOTH in ONE
// segment; every word is plain so gate 2a passes; a broad
// run_shell rule ALLOWED — and bash lets xargs assemble
// `git push`. Same genus (folded from the F2 defer list):
// dashed-form dispatch `git-push origin main` (subcommand inside
// argv[0], never in the subcommand position) and find's
// `-exec`/`-execdir` bridges.
//
// Fix under test: hard-deny class 2b (see decide.go) — argv[0]
// xargs per segment, the words -exec/-execdir anywhere, any word
// matching ^git-[a-z][a-z0-9-]*$. CLOSED-CLASS LEXICAL TRIPWIRES,
// NOT PROOFS (README + decide.go header state the reframe).
// =============================================================================

// The two F1 shapes. Pre-fix both ALLOWED through the broad rule
// (RED receipts captured showing {allow …} before the fix landed).
func TestReviewR3CrossSegmentPipeAssemblyDenied(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		{"xargs assembles git (F1 shape 1)", `{"command":"echo push | xargs git"}`},
		{"xargs assembles git with args (F1 shape 2)", `{"command":"echo push origin main | xargs git"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R3 %s: must HARD-DENY under a broad run_shell allow, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "exec intermediary") {
			t.Fatalf("R3 %s: deny reason must name exec intermediary, got %q", c.name, d.Reason)
		}
	}

	// Path-form argv0 and env-prefixed xargs trip the same way
	// (edge-quote trim + path.Base + env-prefix strip, mirroring the
	// git class's argv0 treatment).
	for _, c := range []struct{ name, args string }{
		{"path-form xargs", `{"command":"/usr/bin/xargs git"}`},
		{"env-prefixed xargs", `{"command":"FOO=bar xargs git"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny || !strings.Contains(d.Reason, "exec intermediary") {
			t.Fatalf("R3 %s: must HARD-DENY as exec intermediary, got %+v", c.name, d)
		}
	}
}

// find's exec bridges: the word -exec/-execdir trips regardless of
// position — even when the git-adjacency scan would ALSO catch the
// shape (its reason is then the adjacency one; both are hard-denies).
func TestReviewR3FindExecBridgeDenied(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		{"find -exec git push (mission shape; adjacency co-fires)", `{"command":"find . -name x -exec git push \\;"}`},
		{"find -exec non-git child", `{"command":"find . -name x -exec rm {} \\;"}`},
		{"find -execdir", `{"command":"find . -execdir ls \\;"}`},
		{"-exec as a bare word (position-independent)", `{"command":"echo -exec"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R3 %s: must HARD-DENY, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "exec intermediary") {
			t.Fatalf("R3 %s: deny reason must name exec intermediary, got %q", c.name, d.Reason)
		}
	}
}

// Dashed-form git dispatch (the folded F2 defer genus): the
// subcommand lives inside argv[0], never in the subcommand position
// the mutation scan reads. ANY git-* word trips — the accepted
// deny-direction over-approximation: benign mentions deny too.
func TestReviewR3GitDashedFormDenied(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		{"dashed dispatch with args", `{"command":"git-push origin main"}`},
		{"dashed dispatch alone", `{"command":"git-push"}`},
		{"benign mention: man page", `{"command":"man git-push"}`},
		{"benign mention: git help", `{"command":"git help git-push"}`},
		{"benign mention: echo prose", `{"command":"echo about git-push"}`},
		// grep pattern as a STANDALONE word — the word git-push
		// appears whole, so it trips (accepted over-approximation,
		// pinned per the round-3 block).
		{"grep pattern as standalone word", `{"command":"git log --grep git-push"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R3 %s: must HARD-DENY under a broad run_shell allow, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "git dashed-form") {
			t.Fatalf("R3 %s: deny reason must name git dashed-form, got %q", c.name, d.Reason)
		}
	}

	// Under the argv0=git rule the same shapes deny (no rule rescues
	// a hard-deny class).
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git-push origin main"}`); d.Kind != DecisionDeny {
		t.Fatalf("R3: argv0=git rule must not rescue dashed-form dispatch, got %+v", d)
	}

	// Dashed dispatch from a PATH argv0 (basename trips, mirroring
	// the git class's argv0 treatment): the real git-core helpers
	// live exactly there.
	if d := decideOf(t, broad, "run_shell", `{"command":"/usr/lib/git-core/git-push origin main"}`); d.Kind != DecisionDeny || !strings.Contains(d.Reason, "git dashed-form") {
		t.Fatalf("R3: path-form dashed dispatch must HARD-DENY as git dashed-form, got %+v", d)
	}
}

// Boundary pins that must STAY allowed: the tripwires are
// whole-word scans over segments — plain pipes without an
// intermediary stay rule-eligible, and embedded SUBSTRINGS
// (`--grep=git-push`) do not trip (the regex is anchored whole-word).
func TestReviewR3BoundaryStaysAllowed(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")
	for _, c := range []struct{ name, args string }{
		{"plain echo", `{"command":"echo hello"}`},
		{"ls with flags and abs path", `{"command":"ls -la /tmp"}`},
		// A plain pipe WITHOUT an intermediary: cat/grep argv0s, no
		// -exec words, no git-* words — still rule-eligible.
		{"plain pipe without intermediary", `{"command":"cat package.json | grep go"}`},
	} {
		if d := decideOf(t, broad, "run_shell", c.args); d.Kind != DecisionAllow {
			t.Fatalf("R3 boundary %s: must stay allowed under the broad rule, got %+v", c.name, d)
		}
	}

	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	// git status under the argv0 rule (mission boundary pin).
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git status"}`); d.Kind != DecisionAllow {
		t.Fatalf("R3 boundary: git status under argv0=git must stay allowed, got %+v", d)
	}
	// The =-form keeps git-push as a SUBSTRING of one ---shaped
	// word; the anchored whole-word regex does not match it, so this
	// read-only git command stays rule-eligible (honest boundary of
	// the over-approximation).
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git log --grep=git-push"}`); d.Kind != DecisionAllow {
		t.Fatalf("R3 boundary: --grep=git-push (substring, not whole word) must stay allowed, got %+v", d)
	}

	// `echo about git push` already denies via git-adjacency (F3/R2
	// pin) — re-pinned here per the round-3 mission: keep.
	if d := decideOf(t, broad, "run_shell", `{"command":"echo about git push"}`); d.Kind != DecisionDeny {
		t.Fatalf("R3 boundary: git-adjacent prose must keep denying, got %+v", d)
	}
}

// =============================================================================
// ROUND 4 — position-independent exec-intermediary words + fail-closed
// env options.
//
// Round-4 BLOCKs (all critical) proved the round-3 xargs tripwire was
// argv0-POSITION-ONLY, so any word that displaces xargs out of argv[0]
// sidesteps it, and stripEnvPrefix silently mishandled env options:
//
//   - B-F1: `echo push | env -i xargs git` — stripEnvPrefix removed
//     only the literal `env`; `-i` became argv[0]; xargs was never
//     checked → ALLOW; the pipe executes `git push`.
//   - C-F1: `echo push | nohup xargs git` (also command/nice/time/
//     stdbuf/setsid wrappers, `env env xargs git`) → ALLOW.
//   - D-F1: same displacement plus `sh -c 'xargs git'` (edge-quoted
//     body) — falsified the README claims "denied wherever they
//     appear" and "xargs denies under ANY rule".
//
// Fix under test (see decide.go): the word-level scan (where
// -exec/-execdir already tripped) now denies the closed set
// {xargs, parallel, -ok, -okdir} wherever the word appears
// (path.Base included — position-independent = unfalsifiable by
// displacement); and an env OPTION word after the env prefix
// (`env -i …`, `-u NAME`, `--`) leaves the segment unidentifiable →
// uncertainty denies (fail-closed stripEnvPrefix boundary).
// =============================================================================

// The B-F1/C-F1/D-F1 displacement shapes. Pre-fix each ALLOWED
// through the broad rule (RED receipts captured below showing
// {allow …} before the fix landed).
func TestReviewR4DisplacedXargsWordDenied(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		// B-F1: env -i displaces xargs out of argv[0]; the pipe
		// assembles `git push` in the child.
		{"B-F1 env -i displaces xargs (pipe assembles git push)", `{"command":"echo push | env -i xargs git"}`},
		{"B-F1 env -i displaces xargs (no pipe)", `{"command":"env -i xargs git"}`},
		// C-F1: the displacement wrappers.
		{"C-F1 nohup", `{"command":"echo push | nohup xargs git"}`},
		{"C-F1 command", `{"command":"echo push | command xargs git"}`},
		{"C-F1 nice", `{"command":"echo push | nice xargs git"}`},
		{"C-F1 time", `{"command":"echo push | time xargs git"}`},
		{"C-F1 stdbuf", `{"command":"echo push | stdbuf -o0 xargs git"}`},
		{"C-F1 setsid", `{"command":"echo push | setsid xargs git"}`},
		{"C-F1 double env", `{"command":"env env xargs git"}`},
		// D-F1: the sh -c edge-quoted body — trimQuoteRunes recovers
		// the word xargs from 'xargs.
		{"D-F1 sh -c edge-quoted body", `{"command":"sh -c 'xargs git'"}`},
		// Path-qualified displacement: basename trips mid-segment.
		{"path-qualified xargs behind wrapper", `{"command":"echo push | nohup /usr/bin/xargs git"}`},
		// Boundary pin from the mission: `env -- xargs git` denies via
		// the word scan (the exec-intermediary class runs before the
		// env-option rule, so the tripwire reason wins).
		{"env -- xargs git (word scan wins)", `{"command":"env -- xargs git"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R4 %s: must HARD-DENY under a broad run_shell allow, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "exec intermediary") {
			t.Fatalf("R4 %s: deny reason must name exec intermediary, got %q", c.name, d.Reason)
		}
	}
}

// The folded genera: GNU parallel (D-F4) and find's prompt-exec
// variants -ok/-okdir. The isolated shapes avoid every other deny
// class (no git word, no mutation word, plain words only) so ONLY the
// new tripwire can deny them — pre-fix they ALLOWED (RED receipts
// captured).
func TestReviewR4ParallelAndPromptExecVariantsDenied(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		{"D-F4 parallel assembles children from stdin", `{"command":"ls | parallel cat"}`},
		{"D-F4 path-qualified parallel", `{"command":"seq 3 | /usr/bin/parallel echo"}`},
		{"find -ok prompt-exec (isolated plain-word shape)", `{"command":"find . -ok ls"}`},
		{"find -okdir prompt-exec (isolated plain-word shape)", `{"command":"find . -okdir ls"}`},
		// Realistic terminator shapes: pre-fix these denied only via
		// the plain-word gate (`{}`/`\` are unplain) — post-fix the
		// tripwire must run FIRST, so the reason names the exec
		// intermediary, not generic unidentifiability.
		{"find -ok realistic terminator shape", `{"command":"find . -name x -ok rm {} \\;"}`},
		{"find -okdir realistic terminator shape", `{"command":"find . -okdir ls {} \\;"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R4 %s: must HARD-DENY, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "exec intermediary") {
			t.Fatalf("R4 %s: deny reason must name exec intermediary (the tripwire outranks the plain-word gate), got %q", c.name, d.Reason)
		}
	}
}

// Part 2 — stripEnvPrefix fail-closed: an env OPTION word after the
// env prefix leaves the child argv/environment unidentifiable to the
// lexical scan → uncertainty denies. Pre-fix every shape here ALLOWED
// (RED receipts captured).
func TestReviewR4EnvOptionsFailClosed(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")

	for _, c := range []struct{ name, args string }{
		{"env -i clears the environment", `{"command":"env -i git status"}`},
		{"env -u unsets a name", `{"command":"env -u FOO git status"}`},
		{"env -- separator", `{"command":"env -- git status"}`},
		{"assignment then env option", `{"command":"FOO=bar env -i ls"}`},
		{"env option in a compound tail", `{"command":"echo hi && env -i ls"}`},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R4 env-option %s: must HARD-DENY (env options are unidentifiable; uncertainty denies), got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, "env option") {
			t.Fatalf("R4 env-option %s: deny reason must name the env option, got %q", c.name, d.Reason)
		}
	}
}

// Boundary pins that must STAY allowed: the tripwires are
// exact/basename WORD matches (never substring), plain pipes without
// an intermediary stay rule-eligible, and the Part-2 boundary —
// assignments then a plain argv[0] — stays identifiable through the
// normal classes.
func TestReviewR4BoundaryStaysAllowed(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")
	for _, c := range []struct{ name, args string }{
		{"plain echo", `{"command":"echo hello"}`},
		{"plain pipe without intermediary", `{"command":"cat x | grep y"}`},
		// Word-level matching is exact/basename, NOT substring:
		// xargus/parallels do not trip.
		{"xargus is not xargs", `{"command":"xargus foo"}`},
		{"parallels is not parallel", `{"command":"parallels foo"}`},
		// `ok` prose without the dash is not -ok.
		{"ok prose word is not -ok", `{"command":"echo ok things"}`},
	} {
		if d := decideOf(t, broad, "run_shell", c.args); d.Kind != DecisionAllow {
			t.Fatalf("R4 boundary %s: must stay allowed under the broad rule, got %+v", c.name, d)
		}
	}

	// Part-2 boundary (mission pin): assignments then a plain argv[0]
	// strip cleanly and stay identifiable; the transparent `env`
	// prefix keeps working through the normal classes.
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")
	for _, cmd := range []string{"env FOO=bar git status", "env git status"} {
		if d := decideOf(t, gitRule, "run_shell", `{"command":"`+cmd+`"}`); d.Kind != DecisionAllow {
			t.Fatalf("R4 boundary: %q must stay allowed under the argv0=git rule, got %+v", cmd, d)
		}
	}
}

// A-F1 fold: the gitMutationSegment plainness checks were a stale
// cross-reference to parse.go's isPlainWord (the rule-VALUE grammar,
// which allows `%` and rejects `=`); they now use the command-word
// grammar, deliberately UNTRIMMED — an edge-quoted argv[0]/subcommand
// still denies in this class (that is what keeps quoted-tail segments
// like `echo "a && b"` unidentifiable; gate 2a's trimmed variant is
// unchanged). No pinned deny may flip.
func TestReviewR4Argv0SubcommandGrammarAligned(t *testing.T) {
	broad := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\n")
	gitRule := mustPolicy(t, "[[allow]]\ntool = \"run_shell\"\nargv0 = \"git\"\n")

	// Still deny — the quoted-evasion pins must not regress.
	for _, c := range []struct{ name, args, reasonKey string }{
		{"quoted subcommand evasion", `{"command":"git \"pu\"sh\""}`, "git"},
		{"quoted argv0 evasion", `{"command":"gi\"t\" push"}`, "unidentifiable"},
		{"quoted tail segment stays unidentifiable", `{"command":"echo \"a && b\""}`, "unidentifiable"},
		{"edge-quoted mutation subcommand", `{"command":"git \"push\""}`, "git"},
		// `%` leaves the command-word grammar → argv0 unidentifiable
		// (pre-fix this denied only later, via gate 2a).
		{"percent argv0 unidentifiable", `{"command":"fo%o ls"}`, "unidentifiable"},
	} {
		d := decideOf(t, broad, "run_shell", c.args)
		if d.Kind != DecisionDeny {
			t.Fatalf("R4 A-F1 %s: must keep HARD-DENYING, got %+v", c.name, d)
		}
		if !strings.Contains(d.Reason, c.reasonKey) {
			t.Fatalf("R4 A-F1 %s: deny reason must contain %q, got %q", c.name, c.reasonKey, d.Reason)
		}
	}

	// Grammar alignment delta, pinned: `=` is IN the command-word
	// grammar (so NAME=value-shaped words stay identifiable for the
	// secret-env scan), and `git a=b` is not a git subcommand — bash
	// itself rejects it ("not a git command"), so it is
	// rule-eligible. Pre-fix this denied as "not a plain word" (RED
	// receipt captured).
	if d := decideOf(t, gitRule, "run_shell", `{"command":"git a=b"}`); d.Kind != DecisionAllow {
		t.Fatalf("R4 A-F1: git a=b must be rule-eligible under argv0=git (bash rejects it; grammar-aligned), got %+v", d)
	}
}

func TestReviewF1CrossKeyErrorCarriesSelectedKeysLineByteText(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantLine int
		wantText string
		wantByte int
	}{
		// path line precedes tool line: laterOf selects the TOOL line
		// — pre-fix the error named the tool LINE NUMBER but carried
		// the path line's byte offset and text.
		{"path first, tool later",
			"[[allow]]\npath = \"docs/\"\ntool = \"echo\"\n",
			3, "tool = \"echo\"", len("[[allow]]\npath = \"docs/\"\n")},
		// argv0 line precedes tool line: same contract.
		{"argv0 first, tool later",
			"[[allow]]\nargv0 = \"git\"\ntool = \"read\"\n",
			3, "tool = \"read\"", len("[[allow]]\nargv0 = \"git\"\n")},
		// Mirror ordering (constraint later): laterOf selects the
		// constraint line — already consistent pre-fix, pinned here so
		// the contract cannot regress in either direction.
		{"tool first, path later",
			"[[allow]]\ntool = \"echo\"\npath = \"docs/\"\n",
			3, "path = \"docs/\"", len("[[allow]]\ntool = \"echo\"\n")},
		{"tool first, argv0 later",
			"[[allow]]\ntool = \"read\"\nargv0 = \"git\"\n",
			3, "argv0 = \"git\"", len("[[allow]]\ntool = \"read\"\n")},
	}
	for _, c := range cases {
		_, err := Parse("test.policy", []byte(c.src))
		if err == nil {
			t.Fatalf("%s: expected a cross-key parse error, got none (src=%q)", c.name, c.src)
		}
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("%s: want *ParseError, got %T: %v", c.name, err, err)
		}
		if pe.Line != c.wantLine {
			t.Fatalf("%s: Line = %d, want %d (%v)", c.name, pe.Line, c.wantLine, err)
		}
		if pe.Text != c.wantText {
			t.Fatalf("%s: Text = %q, want the selected key's line %q verbatim", c.name, pe.Text, c.wantText)
		}
		if pe.Byte != c.wantByte {
			t.Fatalf("%s: Byte = %d, want %d (start of line %d)", c.name, pe.Byte, c.wantByte, c.wantLine)
		}
		// The rendered error must bind all three to that same line
		// (Error() renders the text quoted — compare the %q form).
		if !strings.Contains(pe.Error(), "test.policy:3") || !strings.Contains(pe.Error(), fmt.Sprintf("%q", c.wantText)) {
			t.Fatalf("%s: Error() must cite file:line and the selected line's text, got %q", c.name, pe.Error())
		}
	}
}
