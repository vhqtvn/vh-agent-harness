package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the harness binary version. It is a package var so it can be
// overridden at link time via -ldflags:
//
//	-X github.com/vhqtvn/vh-agent-harness/internal/cli.Version=0.6.0+dev
//
// When not overridden (e.g. `go run` without the Makefile ldflags stamp), this
// fallback is "dev" — an honest "unstamped" marker. `make build` injects the
// git-derived version via -ldflags -X: bare <tag> on an exact-tag commit
// (release), <latest-tag>+dev when HEAD is ahead of the latest tag, or
// 0.0.0+dev when no tags exist. Semver build metadata (+dev) sorts equal to
// the tag, not below, so a dev build says "on top of <tag>" without claiming
// an undecided next version.
var Version = "dev"

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
