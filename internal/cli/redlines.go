package cli

// redlines.go implements `vh-agent-harness redlines guidance` — the agent's
// LOCAL CONTEXT-LOADING channel for machine-private sensitivity knowledge.
//
// This is the ONE command surface permitted to emit the real registry terms.
// Its PURPOSE is agent self-context-loading: an agent runs it at session start
// (exactly like reading docs/planning/backlog.md) to learn which terms and
// relations it must avoid at GENERATION time. The operator's policy (memo
// Decision 4) holds that local agent context is NOT egress, and the commit gate
// is the mechanical backstop that catches any accidental echo of these terms
// into a commit. This channel distinction is intentional and load-bearing.
//
// ALL other redlines surfaces (scan, status, doctor) stay OPAQUE subj-* only —
// safe to paste into issues/PRs. guidance is the sole exception because
// prevention requires disclosure at the generation boundary: an agent that does
// not know the forbidden relation will eventually write one side of it next to
// the other by accident.
//
// Output is STDOUT ONLY. guidance NEVER writes terms to any file — that would
// hit the generated-file drift problem the brief identified (T3/A2/A5) and
// create a committed-artifact leak vector. No materialization.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vhqtvn/vh-agent-harness/internal/redlines"
)

// redlinesCmd is the parent for redlines verbs: `guidance` (agent context,
// full terms) and `scan` (headless exact-tree scanner, opaque output).
var redlinesCmd = &cobra.Command{
	Use:   "redlines",
	Short: "Machine-local sensitivity context + exact-tree scanner",
	Long: `Manage the machine-local "private redlines" capability: never-committed,
cross-project sensitivity knowledge that protects every repository on a machine
with no per-repo setup.

The registry lives at $XDG_CONFIG_HOME/vh-agent-harness/redlines/registry.yml
(never in any repo). When no registry exists, or when no subject binds the
current repository, every redlines command is a complete NO-OP: exit 0, no
output, no files.

Subcommands:
  guidance    Print the LOCAL agent context for binding subjects (full terms).
              This is the ONE surface that emits real terms; run it at session
              start to learn what to avoid at generation time. Its output is
              private — never paste it into issues, PRs, commits, or logs.
  scan        Scan an EXACT git tree object for violations (opaque output).
              Headless-safe: exit code is the machine contract (0=pass, 1=violation,
              2=fail-closed). Never echoes configured terms.`,
	// No-args prints the parent help and exits 0; an unexpected token is an
	// unknown-command error (same idiom as overlay/root).
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return cmd.Help()
		}
		return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
	},
}

// redlinesGuidanceCmd implements `vh-agent-harness redlines guidance`.
var redlinesGuidanceCmd = &cobra.Command{
	Use:           "guidance",
	Short:         "Print LOCAL agent context for binding redlines (full private terms; stdout only)",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `Print the LOCAL agent context for every redlines subject that binds the
current repository.

This command is the agent's generation-time context-loading channel — run it at
session start, like reading docs/planning/backlog.md, to learn which terms and
relations you must avoid when generating content for this repo.

LEAK-SURFACE DISTINCTION (load-bearing):
  - guidance is the ONE redlines surface that emits the REAL registry terms.
    Local agent context is not egress; the commit gate backstops any accidental
    echo of these terms into a commit. This is why full terms are shown here.
  - Every other redlines surface (scan, status, doctor) stays OPAQUE subj-*
    only, safe to paste into issues and PRs.

Output goes to STDOUT ONLY. guidance never writes terms to any file. Its output
is private — do NOT paste it into issues, PRs, commits, or logs. If you must
reference a finding in a public channel, use the opaque subj-* id only.

When no registry exists, or no subject binds this repo, prints a single inert
line and exits 0 (no terms, no error).`,
	Args: cobra.NoArgs,
	RunE: runRedlinesGuidance,
}

func init() {
	redlinesCmd.AddCommand(redlinesGuidanceCmd)
}

// runRedlinesGuidance is the RunE for `vh-agent-harness redlines guidance`.
func runRedlinesGuidance(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()

	repoRoot, remotes, inGit := resolveRepoForRedlines()
	reg, err := redlines.Load(repoRoot)
	if err != nil {
		// Fail closed: a present-but-invalid/unreadable registry is an opaque
		// error (never echoing terms). The operator must fix the registry.
		fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
		return errSilent{}
	}
	if reg == nil {
		fmt.Fprintln(out, "redlines: no registry configured")
		return nil
	}

	// Filter to subjects that bind this repo. Binding uses the repo path +
	// remotes; a non-git directory still gets path-glob matching (remotes
	// empty). The caller passes the already-loaded subjects.
	var binding []redlines.Subject
	for _, s := range reg.Subjects {
		if s.Binds(repoRoot, remotes) {
			binding = append(binding, s)
		}
	}
	if len(binding) == 0 {
		fmt.Fprintln(out, "redlines: no redlines bind this repository")
		return nil
	}

	printRedlinesGuidance(out, binding, repoRoot, remotes, inGit)
	return nil
}

// resolveRepoForRedlines resolves the current repo root from cwd, its remotes,
// and whether cwd is inside a git work tree. When not in a git repo, repoRoot
// is the absolute cwd (path-glob matching still applies), remotes is empty,
// and inGit is false. repoRoot is always absolute.
func resolveRepoForRedlines() (repoRoot string, remotes []string, inGit bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, false
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	// Resolve the git work-tree root. `git rev-parse --show-toplevel` prints
	// the absolute root when inside a work tree; it exits non-zero otherwise.
	if tl, err := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel").Output(); err == nil {
		root := strings.TrimSpace(string(tl))
		if root != "" {
			remotes, _ = redlines.RepoRemotes(root)
			return root, remotes, true
		}
	}
	// Not a git repo: use cwd as the path; remotes empty (path-glob only).
	return abs, nil, false
}

// printRedlinesGuidance writes the full LOCAL agent context for the binding
// subjects to out (stdout). It emits real terms — this is the sole permitted
// real-term surface. The preamble banner is printed once before any terms and
// states the privacy contract.
func printRedlinesGuidance(out io.Writer, subjects []redlines.Subject, repoRoot string, remotes []string, inGit bool) {
	// --- Preamble banner (printed once, before any terms) ---
	fmt.Fprintln(out, "================================================================================")
	fmt.Fprintln(out, "REDLINES GUIDANCE — LOCAL AGENT CONTEXT (PRIVATE)")
	fmt.Fprintln(out, "================================================================================")
	fmt.Fprintln(out, "This output is LOCAL agent context, NOT a pasteable diagnostic.")
	fmt.Fprintln(out, "The terms below are PRIVATE. Do NOT paste them into issues, PRs, commits,")
	fmt.Fprintln(out, "logs, chat, or any channel outside this machine.")
	fmt.Fprintln(out, "The commit gate mechanically backstops any accidental echo of these terms")
	fmt.Fprintln(out, "into a commit, but prevention starts here: avoid generating them at all.")
	fmt.Fprintln(out, "If you must reference a subject in a public channel, use ONLY its opaque")
	fmt.Fprintln(out, "subj-* id, never the terms.")
	fmt.Fprintln(out)
	if inGit {
		fmt.Fprintf(out, "Repo: %s (remotes: %s)\n", repoRoot, remoteSummary(remotes))
	} else {
		fmt.Fprintf(out, "Repo path: %s (not a git repo; path-glob matching only)\n", repoRoot)
	}
	fmt.Fprintln(out, "Honesty: detection is lexical and best-effort — see the honesty contract at")
	fmt.Fprintln(out, "the bottom. Avoiding the listed terms prevents the common case; it is not a")
	fmt.Fprintln(out, "proof that no sensitive relation can be inferred.")
	fmt.Fprintln(out, "================================================================================")
	fmt.Fprintln(out)

	for _, s := range subjects {
		printRedlinesSubject(out, s, repoRoot, remotes)
		fmt.Fprintln(out)
	}

	// --- Honesty contract (verbatim) ---
	fmt.Fprintln(out, "--------------------------------------------------------------------------------")
	fmt.Fprintln(out, "HONESTY CONTRACT (lexical v1 detection limitations)")
	fmt.Fprintln(out, "--------------------------------------------------------------------------------")
	for _, line := range strings.Split(redlines.HonestyContract, "\n") {
		fmt.Fprintln(out, line)
	}
}

// printRedlinesSubject prints one binding subject's full term set. This is the
// real-term emission — the whole point of the guidance channel.
func printRedlinesSubject(out io.Writer, s redlines.Subject, repoRoot string, remotes []string) {
	fmt.Fprintf(out, "--- %s ---\n", s.ID)
	fmt.Fprintf(out, "  kind: %s\n", s.Kind)

	switch s.Kind {
	case redlines.KindScrubProject:
		fmt.Fprintf(out, "  labels (terms to scrub — ANY appearing in content or paths is a violation):\n")
		for _, l := range s.Labels {
			fmt.Fprintf(out, "    - %s\n", l)
		}
		fmt.Fprintf(out, "  policy: %s\n", s.Policy)

	case redlines.KindForbiddenRelation:
		ambient := s.IsAmbient(repoRoot, remotes)
		fmt.Fprintf(out, "  side_a terms:\n")
		for _, t := range s.SideA {
			fmt.Fprintf(out, "    - %s\n", t)
		}
		fmt.Fprintf(out, "  side_b terms:\n")
		for _, t := range s.SideB {
			fmt.Fprintf(out, "    - %s\n", t)
		}
		if ambient {
			fmt.Fprintf(out, "  AMBIENT: this repo's identity implies side_a, so side_b terms ALONE leak\n")
			fmt.Fprintf(out, "          the relation. Do NOT generate any side_b term in any artifact.\n")
		} else {
			fmt.Fprintf(out, "  Co-occurrence rule: side_a AND side_b co-occurring within ONE file\n")
			fmt.Fprintf(out, "  is a violation. (Cross-unit co-occurrence is a documented non-hit, but avoid\n")
			fmt.Fprintf(out, "  pairing them regardless — the lexical limit is a detector floor, not a goal.)\n")
		}
		if s.Unit == "" {
			fmt.Fprintf(out, "  unit: file (default)\n")
		} else {
			fmt.Fprintf(out, "  unit: %s\n", s.Unit)
		}
	}

	if s.Why != "" {
		// The why line is the operator's private rationale. It is shown here
		// because guidance is the local-context channel; it is NEVER shown by
		// any opaque/diagnostic surface.
		fmt.Fprintf(out, "  why: %s\n", s.Why)
	}
}

// remoteSummary renders a short, non-sensitive summary of the remotes list for
// the banner (e.g. "github.com/org/repo"). It is NOT a term and is safe: it is
// the repo's own identity.
func remoteSummary(remotes []string) string {
	if len(remotes) == 0 {
		return "none"
	}
	return strings.Join(remotes, ", ")
}
