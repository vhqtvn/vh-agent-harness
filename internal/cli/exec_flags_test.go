package cli

import "testing"

// TestExec_FlagParsingStopsAtCommand is the regression guard for the `--`
// ergonomics fix: exec sets SetInterspersed(false) so the wrapped command's own
// flags pass through instead of being parsed as exec flags. Without this, a
// human/agent had to write `vh-agent-harness exec -- bash -c '...'` or hit
// "unknown shorthand flag: 'c'". This pins that:
//   - harness flags BEFORE the command are still parsed;
//   - everything from the command token onward (including tokens that collide
//     with real exec flag names) is left as positional args for the command.
func TestExec_FlagParsingStopsAtCommand(t *testing.T) {
	// Save/restore the shared flag state so this test can't leak into others.
	defer func() {
		execFl.service, execFl.workdir, execFl.tty = "", "", false
		_ = execCmd.Flags().Parse(nil)
	}()

	cases := []struct {
		name     string
		argv     []string
		wantWD   string   // exec --workdir consumed (only when before the command)
		wantArgs []string // positional args handed to the wrapped command
	}{
		{
			name:     "command flags pass through without --",
			argv:     []string{"bash", "-c", "echo hi"},
			wantWD:   "",
			wantArgs: []string{"bash", "-c", "echo hi"},
		},
		{
			name:     "arg colliding with a real exec flag is NOT consumed",
			argv:     []string{"echo", "--workdir=HELLO"},
			wantWD:   "",
			wantArgs: []string{"echo", "--workdir=HELLO"},
		},
		{
			name:     "harness flag before the command is still parsed",
			argv:     []string{"--workdir", "/tmp", "echo", "-n", "done"},
			wantWD:   "/tmp",
			wantArgs: []string{"echo", "-n", "done"},
		},
		{
			name:     "explicit -- still works",
			argv:     []string{"--", "ls", "-la"},
			wantWD:   "",
			wantArgs: []string{"ls", "-la"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			execFl.service, execFl.workdir, execFl.tty = "", "", false
			fs := execCmd.Flags()
			if err := fs.Parse(tc.argv); err != nil {
				t.Fatalf("parse %v: %v", tc.argv, err)
			}
			if execFl.workdir != tc.wantWD {
				t.Errorf("workdir = %q, want %q (argv=%v)", execFl.workdir, tc.wantWD, tc.argv)
			}
			got := fs.Args()
			if len(got) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", got, tc.wantArgs)
			}
			for i := range got {
				if got[i] != tc.wantArgs[i] {
					t.Fatalf("args[%d] = %q, want %q (full=%v)", i, got[i], tc.wantArgs[i], got)
				}
			}
		})
	}
}
