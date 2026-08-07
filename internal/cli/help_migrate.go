package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
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
  help migrate [version]   Print migration note(s) for a release. This is
                           DOCUMENTATION ONLY — it never modifies files. With no
                           version, it prints the bounded forward path of notes
                           from the locally adopted harness version up to the
                           running binary — the half-open interval
                           (adopted, binary], oldest first; with a version, that
                           single release's note.`,
	// SilenceUsage/SilenceErrors mirror example/doctor/diff so the missing-version
	// path (errSilent) produces only our own message instead of cobra's "Error:"
	// line + usage dump. Normal help topics return nil, so silencing is a no-op
	// there.
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
// print that single release's note (documentation-only), or exit non-zero with
// the available-versions list if no note is bundled for it. More than one
// positional is a usage error.
//
// No version: print the BOUNDED FORWARD PATH — every bundled note whose target
// version falls in the half-open interval (adopted, binary], oldest first, where
// `adopted` is the locally installed harness version detected from the seam
// lineage and `binary` is the running binary Version. A strict released-version
// gate (isCleanReleased) is applied to BOTH endpoints before any numeric
// comparison, because compareVersion/parseSemver strip pre-release/build
// suffixes and are therefore unsafe to authorize a released bound. Releases
// inside the interval that have no bundled note (e.g. a patch with no
// consumer-visible steps) are skipped silently — absence is not an error. There
// is intentionally NO global-latest fallback: a clean released range with no
// notes prints an explicit empty message and exits 0.
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

	// More than one positional is a usage error: the explicit form takes exactly
	// one version and the no-arg form takes none. Do not silently ignore
	// trailing args.
	if len(args) > 1 {
		fmt.Fprintln(errOut, "Usage: vh-agent-harness help migrate [version]")
		fmt.Fprintln(errOut, "Specify at most one version. To inspect the bounded path from your adopted version to this binary, run `vh-agent-harness help migrate` with no argument.")
		return errSilent{}
	}

	// Explicit version requested. Documentation-only single-target lookup; it
	// does not inspect the adopted version or compute an upgrade path.
	if len(args) == 1 {
		return showExplicitNote(out, errOut, notes, versions, args[0])
	}

	// No version: bounded forward path (adopted, binary].
	return showMigrationRange(out, notes, versions)
}

// showExplicitNote prints the single bundled note for the requested version
// (documentation-only) preceded by a banner that steers toward the no-arg
// bounded-path form, or a not-found message + available list when no note is
// bundled for that version.
func showExplicitNote(out, errOut io.Writer, notes map[string][]byte, versions []string, arg string) error {
	ver := normalizeVersion(arg)
	body, ok := notes[ver]
	if !ok {
		fmt.Fprintf(errOut, "No bundled migration note was found for %s.\n", ver)
		fmt.Fprintln(errOut, "Some releases have no consumer-visible migration note.")
		fmt.Fprintln(errOut, "To review the bounded path from your adopted version to this binary, run:")
		fmt.Fprintln(errOut, "  vh-agent-harness help migrate")
		fmt.Fprintf(errOut, "Available migration notes: %s\n", strings.Join(versions, ", "))
		return errSilent{}
	}
	fmt.Fprintf(out, "Documentation only: this is the migration note for upgrading TO %s.\n", ver)
	fmt.Fprintln(out, "This lookup does not inspect your adopted version or compute an upgrade path.")
	fmt.Fprintln(out, "To review the bounded path from your adopted version to this binary, run:")
	fmt.Fprintln(out, "  vh-agent-harness help migrate")
	fmt.Fprintf(out, "--- Migration target: %s ---\n", ver)
	return writeNote(out, body)
}

// showMigrationRange prints the bounded forward migration path (adopted, binary]
// for the no-argument form. It implements the five-case edge matrix documented
// on runHelpMigrate: no-lineage, suffix/non-released endpoint, adopted==binary,
// downgrade (adopted>binary), and the forward path (adopted<binary). The context
// header (adopted + binary versions, the exact interval, and the documentation
// framing) is printed FIRST in every terminal case so the output is always
// self-describing.
func showMigrationRange(out io.Writer, notes map[string][]byte, versions []string) error {
	adopted := normalizeVersion(detectAdoptedVersion())
	binVer := normalizeVersion(Version)

	// Case 1: no adopted harness version was detected from local lineage.
	if adopted == "" {
		fmt.Fprintln(out, "No adopted harness version was detected from local lineage.")
		fmt.Fprintln(out, "A bounded migration path cannot be inferred.")
		fmt.Fprintln(out, "Use vh-agent-harness help migrate <version> to inspect a specific target note.")
		fmt.Fprintf(out, "Available migration notes: %s\n", strings.Join(versions, ", "))
		return errSilent{}
	}

	// Case 2: either endpoint is not a clean released version (carries a
	// pre-release "-" or build "+" suffix, or is otherwise non-semver).
	// compareVersion/parseSemver strip suffixes and so cannot authorize a
	// released bound — refuse to infer rather than silently strip. Name WHICH
	// endpoint failed the clean-released check so the message points at the
	// correct remediation (a stale adopted profile vs a dev binary).
	adoptedClean := isCleanReleased(adopted)
	binaryClean := isCleanReleased(Version)
	if !adoptedClean || !binaryClean {
		fmt.Fprintf(out, "Detected adopted version: %s\n", adopted)
		fmt.Fprintf(out, "Running binary version:   %s\n", binVer)
		switch {
		case !adoptedClean && !binaryClean:
			fmt.Fprintln(out, "Cannot infer a released migration range because neither the adopted profile version nor the running binary version is a clean released version.")
		case !adoptedClean:
			fmt.Fprintln(out, "Cannot infer a released migration range because the adopted profile version is not a clean released version.")
		default:
			fmt.Fprintln(out, "Cannot infer a released migration range because the running binary version is not a clean released version.")
		}
		fmt.Fprintln(out, "Use vh-agent-harness help migrate <version> to inspect a specific released target note.")
		return errSilent{}
	}

	// Both endpoints are clean released versions from here on, so numeric
	// comparison is safe (no suffixes left to strip).
	cmp := compareVersion(adopted, binVer)

	// Case 3: adopted == binary — the installation already matches.
	if cmp == 0 {
		fmt.Fprintf(out, "Detected adopted version: %s\n", adopted)
		fmt.Fprintf(out, "Running binary version:   %s\n", binVer)
		fmt.Fprintln(out, "The installation already matches the running binary. There is no forward migration range.")
		return nil
	}

	// Case 4: adopted > binary — downgrade; forward guidance cannot be inferred.
	if cmp > 0 {
		fmt.Fprintf(out, "Detected adopted version: %s\n", adopted)
		fmt.Fprintf(out, "Running binary version:   %s\n", binVer)
		fmt.Fprintln(out, "Cannot infer forward migration guidance because the adopted version is newer than the running binary.")
		fmt.Fprintln(out, "Use vh-agent-harness help migrate <version> to inspect a specific target note.")
		return errSilent{}
	}

	// Case 5: adopted < binary — the forward path. Print the context header
	// first, then every bundled note whose target falls in (adopted, binary].
	selected := selectNotesInRange(versions, adopted, binVer)
	fmt.Fprintf(out, "Detected adopted version: %s\n", adopted)
	fmt.Fprintf(out, "Running binary version:   %s\n", binVer)
	fmt.Fprintf(out, "Migration range: (%s, %s] — bundled notes whose target falls in this interval, oldest first.\n", adopted, binVer)
	fmt.Fprintln(out, "Only bundled notes are shown; some releases have no consumer-visible migration note. Output is documentation only and never modifies files.")
	if len(selected) == 0 {
		fmt.Fprintf(out, "No bundled consumer-visible migration notes exist for targets in (%s, %s].\n", adopted, binVer)
		return nil
	}
	for _, v := range selected {
		fmt.Fprintf(out, "--- Migration target: %s ---\n", v)
		if err := writeNote(out, notes[v]); err != nil {
			return err
		}
	}
	return nil
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

// normalizeVersion accepts "X.Y.Z", "vX.Y.Z", or "VX.Y.Z" and returns the
// canonical lowercase "vX.Y.Z" form used as the migration-note key. Both cases
// of a leading "v" are stripped and re-added as lowercase, mirroring
// isCleanReleased's case-insensitive handling so an explicit capital-V arg
// (e.g. "help migrate V0.1.8") resolves the same note key as the lowercase
// form. An empty input stays empty.
func normalizeVersion(arg string) string {
	v := strings.TrimSpace(arg)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return "v" + v
}

// cleanReleasedRe matches a clean release version AFTER stripping an optional
// leading "v"/"V": exactly MAJOR.MINOR.PATCH with NO pre-release ("-…") or build
// ("+…") suffix. It is the authority for a released bound.
var cleanReleasedRe = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// isCleanReleased reports whether v is a clean released semver — no pre-release
// or build suffix. It strips an optional leading "v"/"V" and then requires an
// exact match against ^\d+\.\d+\.\d+$. Any suffix ("-dev", "+meta", …) or any
// non-semver string ⇒ false. Use this as the bound authority BEFORE any numeric
// comparison: compareVersion/parseSemver strip suffixes, so they would treat a
// dev build as its release base and are unsafe to authorize a released bound.
func isCleanReleased(v string) bool {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "v")
	s = strings.TrimPrefix(s, "V")
	return cleanReleasedRe.MatchString(s)
}

// selectNotesInRange returns the bundled note versions that fall in the
// half-open interval (low, high] — strictly greater than low, less than or equal
// to high — sorted ascending by numeric semver. Callers MUST gate low and high
// through isCleanReleased first; this helper uses compareVersion only for
// numeric ordering (safe once suffixes are ruled out). The input `versions` is
// the lexicographically-sorted list from migrationIndex; the returned slice is
// re-sorted numerically because lexicographic order is wrong across components
// of differing width (e.g. "v0.10.0" < "v0.2.0" lexicographically).
func selectNotesInRange(versions []string, low, high string) []string {
	var selected []string
	for _, v := range versions {
		if compareVersion(v, low) > 0 && compareVersion(v, high) <= 0 {
			selected = append(selected, v)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		return compareVersion(selected[i], selected[j]) < 0
	})
	return selected
}

// compareVersion returns -1, 0, or 1 comparing two version strings. Pre-release
// suffixes (e.g. "-dev") are stripped before the numeric compare so a release
// always ranks above its dev base. Non-parseable versions sort below parseable
// ones; two non-parseable versions compare lexically. Because suffixes are
// stripped, this is UNSAFE as the authority for a released bound — gate with
// isCleanReleased first and use this only for the numeric ordering afterward.
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
