package cli

// This file pins the no-args→help behavior added alongside the migration-notes
// feature, and provides a shared cobra-execution capture helper used by the
// help/migrate tests. The helper drives the real rootCmd (the same tree the
// binary dispatches), so it exercises the full routing: SetHelpCommand(helpCmd)
// wiring, the helpCmd.RunE migrate interception, and the --help flag path.

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
)

// executeCapture drives rootCmd with the given args (after resetting the global
// arg vector), capturing combined stdout+stderr into a buffer. It restores
// rootCmd's args and writers on exit so tests do not leak global state. It must
// not be combined with t.Parallel() (runWithCwd / global rootCmd are not
// parallel-safe).
func executeCapture(t *testing.T, args []string) (string, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetArgs([]string{})
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		// Reset parsed flags on root + children so a prior run's --help / --dry-run
		// does not bleed into the next test (cobra parses into persistent state).
		resetCmdFlags(rootCmd)
	}()
	err := rootCmd.Execute()
	return buf.String(), err
}

// resetCmdFlags walks the command tree and resets every flag to its default.
// Cobra keeps parsed flag values on the command between Execute calls; without
// this, a `--help` parsed in one test would shadow the next.
func resetCmdFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *flag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	c.PersistentFlags().VisitAll(func(f *flag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for _, sub := range c.Commands() {
		resetCmdFlags(sub)
	}
}

// executeCaptureCwd is executeCapture run from within dir as the process cwd,
// so cwd-dependent code paths (e.g. detectAdoptedVersion reading lineage from
// the cwd upward) see the fixture tree.
func executeCaptureCwd(t *testing.T, dir string, args []string) (string, error) {
	t.Helper()
	var out string
	var err error
	runWithCwd(t, dir, func() {
		out, err = executeCapture(t, args)
	})
	return out, err
}

// TestRoot_NoArgsPrintsHelp confirms `vh-agent-harness` with no arguments prints
// the root help (exit 0) and that the help surface carries the agent-facing
// orientation block, the upgrade loop, the self-update command, and the
// migration-notes pointer. This is the no-args half of the feature contract.
func TestRoot_NoArgsPrintsHelp(t *testing.T) {
	out, err := executeCapture(t, []string{})
	if err != nil {
		t.Fatalf("no-args: want nil error (exit 0), got %v", err)
	}
	for _, want := range []string{
		"guide",            // agent orientation entry point
		"update --dry-run", // upgrade loop
		"doctor",           // upgrade loop / orientation
		"self-update",      // upgrade loop
		"help migrate",     // migration-notes pointer
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no-args help missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestRoot_UnknownArgIsAnError confirms an unknown top-level token is surfaced
// as an unknown-command error rather than silently printing help, so typos do
// not look like success.
func TestRoot_UnknownArgIsAnError(t *testing.T) {
	_, err := executeCapture(t, []string{"definitely-not-a-command"})
	if err == nil {
		t.Fatal("unknown command: want non-nil error, got nil")
	}
}
