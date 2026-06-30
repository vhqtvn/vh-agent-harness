package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// helpCmd REPLACES Cobra's auto-generated help command so that the special topic
// `help migrate [version]` routes to the embedded migration-note renderer, while
// EVERY other help topic (`help install`, `help guide`, `help`, `install --help`,
// `guide --help`, …) delegates to Cobra's normal help behavior unchanged.
//
// `migrate` is intentionally NOT a top-level command and does NOT appear in the
// root command list: it is an interception inside the help command only. This
// keeps the command surface free of a top-level `migrate` verb while still
// exposing release migration notes through the natural `help <topic>` surface.
//
// The `--help` flag path is unaffected by this replacement (it routes through
// the HelpFunc, not the help command), so `guide --help` / `install --help`
// behave exactly as before.
var helpCmd = &cobra.Command{
	Use:   "help [command]",
	Short: "Help about any command (special topic: help migrate [version])",
	Long: `Help provides help for any command in the application. Type
"vh-agent-harness help [path to command]" for full details.

Special topic:
  help migrate [version]   Print the migration note for a release. This is
                           DOCUMENTATION ONLY — it never modifies files. With no
                           version, it prints the note for the locally adopted
                           harness version (detected from lineage), or the latest
                           available note when none matches.`,
	// SilenceUsage/SilenceErrors mirror example/doctor/diff so the missing-version
	// path (errSilent) produces only our own "No migration note / available
	// versions" message instead of cobra's "Error:" line + usage dump. Normal
	// help topics return nil, so silencing is a no-op there.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && args[0] == "migrate" {
			return runHelpMigrate(cmd, args[1:])
		}
		return runDefaultHelp(cmd, args)
	},
}

// runDefaultHelp replicates Cobra's built-in help-command routing: find the
// target command for args (e.g. "guide", "install", or empty for root) and print
// its help; report an unknown help topic otherwise. Used for every help topic
// EXCEPT the intercepted "migrate" topic, so `help guide` / `help install` /
// bare `help` behave identically to Cobra's default.
func runDefaultHelp(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	target, _, e := cmd.Root().Find(args)
	if target == nil || e != nil {
		fmt.Fprintf(out, "Unknown help topic %#q\n", args)
		return cmd.Root().Usage()
	}
	return target.Help()
}

// runHelpMigrate implements `help migrate [version]`. It reads ONLY from the
// embedded migration notes (never the live filesystem), so the output is stable
// for a given binary regardless of CWD contents.
//
// Explicit version (vX.Y.Z or X.Y.Z, normalized by adding the "v" prefix):
// print that note, or exit non-zero with the available-versions list if missing.
//
// No version: detect the locally adopted version from the seam lineage (the same
// lineage source `status` reads — content origin recorded as `harness/<version>`
// in lineage.yml template.ref) and show a context line plus the single relevant
// note. When the adopted version has no exact note (e.g. a dev build like
// 0.1.0-dev, or a release we have not backfilled a note for), fall back to the
// latest embedded note with an explicit message. It never claims a cumulative
// path — just the one relevant note.
func runHelpMigrate(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	notes, versions, err := migrationIndex()
	if err != nil {
		fmt.Fprintln(errOut, "error: reading embedded migration notes:", err)
		return errSilent{}
	}
	if len(versions) == 0 {
		fmt.Fprintln(out, "No migration notes are bundled with this binary.")
		return nil
	}
	latest := latestVersion(versions)

	// Explicit version requested.
	if len(args) > 0 {
		ver := normalizeVersion(args[0])
		body, ok := notes[ver]
		if !ok {
			fmt.Fprintf(errOut, "No migration note for %s.\n\n", ver)
			fmt.Fprintln(errOut, "Available versions:")
			for _, v := range versions {
				fmt.Fprintf(errOut, "  %s\n", v)
			}
			fmt.Fprintln(errOut)
			fmt.Fprintln(errOut, "Usage: vh-agent-harness help migrate [version]")
			return errSilent{}
		}
		return writeNote(out, body)
	}

	// No version: detect the locally adopted version from lineage. The lineage ref
	// is recorded as "harness/<Version>" where Version carries no leading "v"
	// (e.g. "harness/0.1.8"), so normalize the detected value to the canonical
	// "vX.Y.Z" form used both for display and for the note-key lookup below.
	adopted := normalizeVersion(detectAdoptedVersion())
	binVer := normalizeVersion(Version)
	if adopted == "" {
		fmt.Fprintln(out, "No harness installation detected in this directory (or any parent).")
		fmt.Fprintf(out, "Showing the latest available migration note (binary version %s).\n", binVer)
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Latest note: %s\n", latest)
		fmt.Fprintln(out)
		return writeNote(out, notes[latest])
	}

	fmt.Fprintf(out, "Local adopted version: %s\n", adopted)
	fmt.Fprintf(out, "Binary version:       %s\n", binVer)
	if body, ok := notes[adopted]; ok {
		fmt.Fprintln(out)
		return writeNote(out, body)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "No migration note for the local adopted version %s; showing the latest available note (%s) instead.\n", adopted, latest)
	fmt.Fprintln(out)
	return writeNote(out, notes[latest])
}

// migrationIndex reads the embedded templates/migrations tree and returns a map
// of version ("vX.Y.Z", derived from the filename) -> note body, plus the sorted
// list of versions. Mirrors the embed-only index pattern used by exampleIndex.
func migrationIndex() (map[string][]byte, []string, error) {
	sub, err := fs.Sub(corpus.MigrationsFS, corpus.MigrationsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	index := map[string][]byte{}
	var versions []string
	err = fs.WalkDir(sub, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		body, rerr := fs.ReadFile(sub, rel)
		if rerr != nil {
			return rerr
		}
		ver := strings.TrimSuffix(rel, ".md")
		index[ver] = body
		versions = append(versions, ver)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("walk embedded migrations: %w", err)
	}
	sort.Strings(versions)
	return index, versions, nil
}

// detectAdoptedVersion resolves the locally adopted harness version from the
// seam lineage, reusing the SAME lineage source `status` reads (runshape.
// FindForRoot + lineage.Read + lin.Template.Ref). Returns "" when no install is
// detected in (or above) the CWD, or the lineage is absent/unreadable.
func detectAdoptedVersion() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	root, _, err := runshape.FindForRoot(cwd)
	if err != nil || root == "" {
		return ""
	}
	lin, err := lineage.Read(root)
	if err != nil || lin == nil {
		return ""
	}
	return strings.TrimPrefix(lin.Template.Ref, "harness/")
}

// normalizeVersion accepts "X.Y.Z" or "vX.Y.Z" and returns the canonical
// "vX.Y.Z" form used as the migration-note key. An empty input stays empty.
func normalizeVersion(arg string) string {
	v := strings.TrimSpace(arg)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// latestVersion returns the highest semver from the list (each entry "vX.Y.Z",
// possibly with a pre-release suffix like "-dev" on the patch). Non-semver
// entries sort below semver ones.
func latestVersion(versions []string) string {
	best := ""
	for _, v := range versions {
		if compareVersion(v, best) > 0 {
			best = v
		}
	}
	return best
}

// compareVersion returns -1, 0, or 1 comparing two version strings. Pre-release
// suffixes (e.g. "-dev") are stripped before the numeric compare so a release
// always ranks above its dev base. Non-parseable versions sort below parseable
// ones; two non-parseable versions compare lexically.
func compareVersion(a, b string) int {
	av, aok := parseSemver(a)
	bv, bok := parseSemver(b)
	if !aok && !bok {
		return strings.Compare(a, b)
	}
	if !aok {
		return -1
	}
	if !bok {
		return 1
	}
	for i := 0; i < 3; i++ {
		if av[i] != bv[i] {
			if av[i] < bv[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseSemver parses "vX.Y.Z" (with an optional pre-release/build suffix on the
// patch) into a [3]int. Returns ok=false when it does not match MAJOR.MINOR.PATCH.
func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	s := strings.TrimPrefix(v, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		if i == 2 {
			// Strip a pre-release ("-dev") or build ("+meta") suffix on patch.
			if idx := strings.IndexAny(p, "-+"); idx >= 0 {
				p = p[:idx]
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// writeNote writes a migration-note body to out verbatim.
func writeNote(out io.Writer, body []byte) error {
	_, err := out.Write(body)
	return err
}
