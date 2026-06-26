package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the harness binary version. It is a package var so it can be
// overridden at link time via -ldflags:
//
//	-X github.com/vhqtvn/vh-agent-harness/internal/cli.Version=1.0.0
//
// When not overridden, the dev default below is used.
var Version = "0.1.0-dev"

// BuildLabel is the human-readable build/slice tag shown alongside Version.
// Override at link time:
//
//	-X github.com/vhqtvn/vh-agent-harness/internal/cli.BuildLabel=<commit-sha-or-tag>
var BuildLabel = "dev"

// VersionString returns the canonical "<version> (<label>)" display string.
func VersionString() string {
	return Version + " (" + BuildLabel + ")"
}

var versionCmd = &cobra.Command{
	Use:           "version",
	Short:         "Print the vh-agent-harness version and build label",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), VersionString())
		return nil
	},
}
