package cli

// This file pins the `help migrate [version]` feature and the migration-note
// canonical-format enforcement. It exercises the FULL command routing by
// driving rootCmd (via executeCapture), so it covers:
//   - rootCmd → helpCmd.SetHelpCommand wiring
//   - helpCmd.RunE intercepting the "migrate" topic → runHelpMigrate
//   - helpCmd.RunE delegating every other topic to runDefaultHelp (cobra default)
//   - the --help flag path still routing to the command's own Help()
//
// The no-arg form is a BOUNDED FORWARD PATH: every bundled note whose target
// version falls in (adopted, binary], oldest first. Tests drive both endpoints
// (lineage ref for `adopted`, the package var Version for `binary`) so the full
// five-case edge matrix is exercised deterministically — the real dogfood binary
// is a "-dev"/"dev" string, which legitimately hits the "cannot infer" path.
//
// It also scans the embedded migration notes for the canonical heading/format
// contract so a malformed note fails CI rather than shipping silently.

import (
	"regexp"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

const migrateDelim = "--- Migration target:"

// --- help migrate <explicit version> ----------------------------------------

// TestHelpMigrate_ExplicitVersion prints the named note verbatim from the
// embedded copy, preceded by a documentation-only banner on stdout that steers
// toward the no-arg bounded-path form.
func TestHelpMigrate_ExplicitVersion(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "v0.1.8"})
	if err != nil {
		t.Fatalf("help migrate v0.1.8: want nil error, got %v", err)
	}
	// The documentation-only banner MUST precede the note body.
	const banner = "Documentation only: this is the migration note for upgrading TO v0.1.8."
	if !strings.Contains(out, banner) {
		t.Errorf("missing documentation-only banner\n--- output ---\n%s", out)
	}
	if idxBanner, idxNote := strings.Index(out, banner), strings.Index(out, "# Migration: v0.1.8"); idxBanner >= 0 && idxNote >= 0 && idxBanner >= idxNote {
		t.Errorf("documentation-only banner must precede the note body\n--- output ---\n%s", out)
	}
	for _, want := range []string{
		"This lookup does not inspect your adopted version or compute an upgrade path.",
		"To review the bounded path from your adopted version to this binary, run:",
		"  vh-agent-harness help migrate",
		migrateDelim + " v0.1.8 ---",
		"# Migration: v0.1.8",
		"Release class:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestHelpMigrate_VersionNormalization confirms a bare "X.Y.Z" arg is normalized
// to "vX.Y.Z" and resolves to the same note as the explicit v-prefixed form,
// hitting the explicit-version success path (banner present).
func TestHelpMigrate_VersionNormalization(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "0.1.8"})
	if err != nil {
		t.Fatalf("help migrate 0.1.8: want nil error (normalized), got %v", err)
	}
	if !strings.Contains(out, "# Migration: v0.1.8") {
		t.Errorf("normalized 0.1.8 should resolve to v0.1.8 note\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "Documentation only: this is the migration note for upgrading TO v0.1.8.") {
		t.Errorf("normalized explicit-version path must also print the documentation-only banner\n--- output ---\n%s", out)
	}
}

// TestHelpMigrate_VersionNormalization_UppercaseV confirms a capital-V explicit
// arg ("V0.1.8") is normalized to the lowercase canonical "v0.1.8" key and
// resolves the SAME note as the lowercase-v form. normalizeVersion and
// isCleanReleased must agree on case handling; before the fix, only the
// lowercase "v" was stripped and a capital-V arg missed the note-key lookup.
func TestHelpMigrate_VersionNormalization_UppercaseV(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "V0.1.8"})
	if err != nil {
		t.Fatalf("help migrate V0.1.8: want nil error (uppercase-V normalized), got %v", err)
	}
	if !strings.Contains(out, "# Migration: v0.1.8") {
		t.Errorf("normalized V0.1.8 should resolve to the v0.1.8 note\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "Documentation only: this is the migration note for upgrading TO v0.1.8.") {
		t.Errorf("uppercase-V explicit-version path must resolve to the same note as the lowercase-v form (banner present)\n--- output ---\n%s", out)
	}
}

// TestNormalizeVersion is a direct unit test for the case-insensitive
// canonicalization: bare, lowercase-v, and uppercase-V prefixes all collapse to
// the lowercase "vX.Y.Z" key; empty stays empty.
func TestNormalizeVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"0.1.8", "v0.1.8"},
		{"v0.1.8", "v0.1.8"},
		{"V0.1.8", "v0.1.8"},
		{"  V0.2.0  ", "v0.2.0"},
	}
	for _, c := range cases {
		if got := normalizeVersion(c.in); got != c.want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHelpMigrate_ExplicitMissingVersion exits non-zero with the reconciled
// not-found message (errSilent path — clean message, no cobra "Error:"/usage
// dump) and steers toward the no-arg bounded-path form.
func TestHelpMigrate_ExplicitMissingVersion(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "v9.9.9"})
	if err == nil {
		t.Fatal("help migrate v9.9.9: want non-nil error (non-zero exit), got nil")
	}
	for _, want := range []string{
		"No bundled migration note was found for v9.9.9.",
		"Some releases have no consumer-visible migration note.",
		"To review the bounded path from your adopted version to this binary, run:",
		"  vh-agent-harness help migrate",
		"Available migration notes: v0.1.8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	// No cobra "Error:" line or usage dump should leak (SilenceErrors/Usage on
	// helpCmd).
	if strings.Contains(out, "Error:") {
		t.Errorf("errSilent path must not print cobra Error line\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "Global Flags:") {
		t.Errorf("errSilent path must not print a cobra usage dump (Global Flags)\n--- output ---\n%s", out)
	}
}

// TestHelpMigrate_ExplicitTooManyArgs rejects more than one positional as a
// usage error rather than silently ignoring trailing args.
func TestHelpMigrate_ExplicitTooManyArgs(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "v0.1.8", "v0.2.0"})
	if err == nil {
		t.Fatal("help migrate v0.1.8 v0.2.0: want non-nil error (usage error), got nil")
	}
	for _, want := range []string{
		"Usage: vh-agent-harness help migrate [version]",
		"Specify at most one version",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	// A usage error must not dump any note body.
	if strings.Contains(out, migrateDelim) {
		t.Errorf("usage error must not print a migration note body\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "# Migration:") {
		t.Errorf("usage error must not print a migration note body\n--- output ---\n%s", out)
	}
}

// --- help migrate (no version): bounded forward path (adopted, binary] --------

// migrateNoArg is the table helper for the no-arg form. It installs a seam with
// the given adopted version (lineage ref) into a temp dir — or, when adopted is
// empty, leaves a bare temp dir with no install — optionally overrides the
// package var Version (binary endpoint), runs `help migrate` from that dir, and
// returns the merged output + error. binary="" leaves the real (dev) Version.
func migrateNoArg(t *testing.T, adopted, binary string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	if adopted != "" {
		seamInstallInto(t, dir)
		lin, err := lineage.Read(dir)
		if err != nil || lin == nil {
			t.Fatalf("read lineage after seam install: %v (lin=%v)", err, lin)
		}
		// Real release-ref shape: "harness/<Version>" with NO leading "v".
		lin.Template.Ref = "harness/" + adopted
		if err := lin.Write(dir); err != nil {
			t.Fatalf("write lineage ref: %v", err)
		}
	}
	saved := Version
	if binary != "" {
		Version = binary
	}
	defer func() { Version = saved }()
	return executeCaptureCwd(t, dir, []string{"help", "migrate"})
}

// TestHelpMigrate_NoArgRange is the table-driven contract for the no-arg bounded
// forward path. Each row pins one edge of the five-case matrix plus the
// cumulative selection, the context header, exit code, and the delimiter shape.
func TestHelpMigrate_NoArgRange(t *testing.T) {
	tests := []struct {
		name       string
		adopted    string // lineage ref version; "" = no install (no-lineage)
		binary     string // binary Version override; "" = leave the real dev Version
		wantErr    bool
		delimCount int // expected count of "--- Migration target:" lines; -1 = skip
		wants      []string
		notWants   []string
		orderCheck []string // delimiters that must appear in ascending index order
	}{
		{
			// Case 5a: a single note in (adopted, binary]. v0.1.7 has no bundled
			// note (it is the adopted base, not a target); the only target in
			// (v0.1.7, v0.1.8] is v0.1.8.
			name: "single_note_in_range", adopted: "0.1.7", binary: "0.1.8", wantErr: false, delimCount: 1,
			wants: []string{
				"Detected adopted version: v0.1.7",
				"Running binary version:   v0.1.8",
				"Migration range: (v0.1.7, v0.1.8]",
				"oldest first",
				"Only bundled notes are shown",
				"Output is documentation only",
				migrateDelim + " v0.1.8 ---",
				"# Migration: v0.1.8",
			},
		},
		{
			// Case 5b: multiple notes ascending. (v0.1.7, v0.2.0] = v0.1.8, v0.1.9,
			// v0.2.0 — printed oldest first.
			name: "multiple_ascending", adopted: "0.1.7", binary: "0.2.0", wantErr: false, delimCount: 3,
			wants: []string{
				"Migration range: (v0.1.7, v0.2.0]",
				migrateDelim + " v0.1.8 ---",
				migrateDelim + " v0.1.9 ---",
				migrateDelim + " v0.2.0 ---",
			},
			orderCheck: []string{"v0.1.8", "v0.1.9", "v0.2.0"},
		},
		{
			// Case 5c: an intermediate release in range has NO bundled note
			// (v0.15.1) — it is skipped silently; only v0.15.0 and v0.16.0 print.
			name: "intermediate_without_note", adopted: "0.14.0", binary: "0.16.0", wantErr: false, delimCount: 2,
			wants: []string{
				"Migration range: (v0.14.0, v0.16.0]",
				migrateDelim + " v0.15.0 ---",
				migrateDelim + " v0.16.0 ---",
			},
			notWants:   []string{migrateDelim + " v0.15.1 ---"},
			orderCheck: []string{"v0.15.0", "v0.16.0"},
		},
		{
			// Case 5d: clean released range with NO bundled notes. (v0.15.0,
			// v0.15.1] contains only v0.15.1, which has no note → empty message,
			// exit 0, no delimiter.
			name: "empty_valid_range", adopted: "0.15.0", binary: "0.15.1", wantErr: false, delimCount: 0,
			wants: []string{
				"Migration range: (v0.15.0, v0.15.1]",
				"No bundled consumer-visible migration notes exist for targets in (v0.15.0, v0.15.1].",
			},
		},
		{
			// Case 3: adopted == binary → already matches, no range.
			name: "adopted_equals_binary", adopted: "0.1.8", binary: "0.1.8", wantErr: false, delimCount: 0,
			wants: []string{
				"Detected adopted version: v0.1.8",
				"Running binary version:   v0.1.8",
				"The installation already matches the running binary. There is no forward migration range.",
			},
		},
		{
			// Case 4: adopted > binary (downgrade) → cannot infer forward.
			name: "downgrade_adopted_newer", adopted: "0.2.0", binary: "0.1.8", wantErr: true, delimCount: 0,
			wants: []string{
				"Detected adopted version: v0.2.0",
				"Running binary version:   v0.1.8",
				"Cannot infer forward migration guidance because the adopted version is newer than the running binary.",
			},
		},
		{
			// Case 1: no adopted lineage detected (no install). Takes priority
			// over the suffix check even when the binary is a dev string.
			name: "no_lineage", adopted: "", binary: "", wantErr: true, delimCount: 0,
			wants: []string{
				"No adopted harness version was detected from local lineage.",
				"A bounded migration path cannot be inferred.",
				"Use vh-agent-harness help migrate <version> to inspect a specific target note.",
				"Available migration notes: v0.1.8",
			},
		},
		{
			// Case 2 (binary unclean): the binary carries a build suffix ("+dev")
			// → not clean released → cannot infer a released range. The message
			// names the BINARY as the endpoint that failed the clean-released
			// check.
			name: "suffix_binary_cannot_infer", adopted: "0.1.8", binary: "0.6.0+dev", wantErr: true, delimCount: 0,
			wants: []string{
				"Detected adopted version: v0.1.8",
				"Running binary version:   v0.6.0+dev",
				"Cannot infer a released migration range because the running binary version is not a clean released version.",
				"Use vh-agent-harness help migrate <version> to inspect a specific released target note.",
			},
		},
		{
			// Case 2 (adopted unclean): the adopted profile carries a dev suffix
			// ("0.x-dev") while the binary is a clean release → cannot infer a
			// released range. The message names the ADOPTED endpoint as the one
			// that failed the clean-released check (NOT the binary), which is
			// the precise remediation pointer the old blame-the-binary copy got
			// wrong.
			name: "suffix_adopted_cannot_infer", adopted: "0.x-dev", binary: "0.1.8", wantErr: true, delimCount: 0,
			wants: []string{
				"Detected adopted version: v0.x-dev",
				"Running binary version:   v0.1.8",
				"Cannot infer a released migration range because the adopted profile version is not a clean released version.",
				"Use vh-agent-harness help migrate <version> to inspect a specific released target note.",
			},
		},
		{
			// Case 2 (both unclean): BOTH endpoints fail the clean-released check.
			// The message names NEITHER endpoint individually, so it must NOT
			// blame only one side.
			name: "suffix_both_cannot_infer", adopted: "0.x-dev", binary: "0.6.0+dev", wantErr: true, delimCount: 0,
			wants: []string{
				"Detected adopted version: v0.x-dev",
				"Running binary version:   v0.6.0+dev",
				"Cannot infer a released migration range because neither the adopted profile version nor the running binary version is a clean released version.",
				"Use vh-agent-harness help migrate <version> to inspect a specific released target note.",
			},
			notWants: []string{
				"Cannot infer a released migration range because the adopted profile version is not a clean released version.\n",
				"Cannot infer a released migration range because the running binary version is not a clean released version.\n",
			},
		},
		{
			// Case 5e: binary newer than the highest bundled note still selects
			// every note up to the binary (the upper bound is the binary, not
			// the highest note). delimCount is skipped (-1) because the exact
			// count drifts as notes are added each release; the orderCheck plus
			// the v0.16.0 presence already prove the full forward selection.
			name: "binary_newer_than_highest_note", adopted: "0.1.8", binary: "9.9.9", wantErr: false, delimCount: -1,
			wants: []string{
				"Migration range: (v0.1.8, v9.9.9]",
				migrateDelim + " v0.16.0 ---",
			},
			orderCheck: []string{"v0.1.9", "v0.2.0", "v0.16.0"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := migrateNoArg(t, tc.adopted, tc.binary)
			if tc.wantErr && err == nil {
				t.Fatalf("want non-nil error, got nil\n--- output ---\n%s", out)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("want nil error, got %v\n--- output ---\n%s", err, out)
			}
			for _, w := range tc.wants {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q\n--- output ---\n%s", w, out)
				}
			}
			for _, nw := range tc.notWants {
				if strings.Contains(out, nw) {
					t.Errorf("did not want %q\n--- output ---\n%s", nw, out)
				}
			}
			if tc.delimCount >= 0 {
				if got := strings.Count(out, migrateDelim); got != tc.delimCount {
					t.Errorf("delimiter count: want %d, got %d\n--- output ---\n%s", tc.delimCount, got, out)
				}
			}
			if len(tc.orderCheck) > 1 {
				prev := -1
				for _, ver := range tc.orderCheck {
					marker := migrateDelim + " " + ver + " ---"
					idx := strings.Index(out, marker)
					if idx < 0 {
						t.Fatalf("orderCheck: missing delimiter %q\n--- output ---\n%s", marker, out)
					}
					if idx <= prev {
						t.Errorf("delimiter %q at %d not after prev %d (ascending order broken)\n--- output ---\n%s", marker, idx, prev, out)
					}
					prev = idx
				}
			}
		})
	}
}

// TestHelpMigrate_NoArgReleaseRefNormalization locks in that the detected
// release ref ("harness/0.1.8", no leading "v") is normalized to "v0.1.8" for
// both display and the interval/header.
func TestHelpMigrate_NoArgReleaseRefNormalization(t *testing.T) {
	out, err := migrateNoArg(t, "0.1.7", "0.1.9")
	if err != nil {
		t.Fatalf("no-arg release-ref: want nil error, got %v", err)
	}
	if !strings.Contains(out, "Detected adopted version: v0.1.7") {
		t.Errorf("want 'Detected adopted version: v0.1.7' (normalized)\n--- output ---\n%s", out)
	}
}

// --- regression: normal help topics still route to cobra default ------------

// TestHelpMigrate_RegressionDefaultTopics confirms the help-command wrapper did
// NOT break normal help routing. `help guide` must reach the guide command's
// help; `guide --help` and `install --help` must still trigger the --help flag
// path (HelpFunc) on those subcommands.
func TestHelpMigrate_RegressionDefaultTopics(t *testing.T) {
	// `help guide` → runDefaultHelp → rootCmd.Find("guide") → guideCmd.Help().
	out, err := executeCapture(t, []string{"help", "guide"})
	if err != nil {
		t.Fatalf("help guide: want nil error, got %v", err)
	}
	if !strings.Contains(out, "Orient yourself") {
		t.Errorf("help guide should print guide's help (Long text)\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "vh-agent-harness guide") {
		t.Errorf("help guide should print guide's usage line\n--- output ---\n%s", out)
	}

	// `guide --help` → --help flag → HelpFunc → guideCmd help.
	out2, err := executeCapture(t, []string{"guide", "--help"})
	if err != nil {
		t.Fatalf("guide --help: want nil error, got %v", err)
	}
	if !strings.Contains(out2, "Orient yourself") {
		t.Errorf("guide --help should print guide's help\n--- output ---\n%s", out2)
	}

	// `install --help` → --help flag → HelpFunc → installCmd help.
	out3, err := executeCapture(t, []string{"install", "--help"})
	if err != nil {
		t.Fatalf("install --help: want nil error, got %v", err)
	}
	if !strings.Contains(out3, "Render the embedded core corpus into a target directory") {
		t.Errorf("install --help should print install's help (Long text)\n--- output ---\n%s", out3)
	}
}

// --- canonical migration-note format enforcement ----------------------------

// requiredMigrationHeadings is the canonical heading set every migration note
// must contain. A note missing any of these fails the format contract.
var requiredMigrationHeadings = []string{
	"# Migration: ",
	"## Summary",
	"## What changed (consumer-visible only)",
	"## How to migrate (automated)",
	"## What `update` handles for you",
	"## Watch-outs",
	"## Verification commands",
	"## Rollback",
	"## Non-consumer changes",
}

// requiredMigrateSequence is the command sequence the "How to migrate
// (automated)" section must include, in order. Each command must appear in the
// note body.
var requiredMigrateSequence = []string{
	"vh-agent-harness self-update",
	"vh-agent-harness version",
	"vh-agent-harness update --dry-run",
	"vh-agent-harness update",
	"vh-agent-harness doctor",
}

// semverFileRe matches a canonical migration-note filename: vX.Y.Z.md (a release
// version, NOT a dev/pre-release suffix).
var semverFileRe = regexp.MustCompile(`^v\d+\.\d+\.\d+\.md$`)

// TestMigrationNotes_Canonical scans every embedded migration note and pins:
//   - the on-disk filename matches vX.Y.Z.md (release semver only, no -dev);
//   - the note body contains ALL canonical headings;
//   - the note body includes the required migrate command sequence;
//   - when the binary Version is a release (not a dev build), a note for it exists.
//
// This is the Go-test enforcement for the migration-note convention (the brief
// scoped enforcement to Go tests only — no Makefile/GoReleaser hook).
func TestMigrationNotes_Canonical(t *testing.T) {
	notes, versions, err := migrationIndex()
	if err != nil {
		t.Fatalf("migrationIndex: %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one embedded migration note, got none")
	}

	for _, v := range versions {
		fname := v + ".md"
		if !semverFileRe.MatchString(fname) {
			t.Errorf("migration note filename %q must match vX.Y.Z.md (release semver)", fname)
		}
		body := string(notes[v])
		for _, h := range requiredMigrationHeadings {
			if !strings.Contains(body, h) {
				t.Errorf("migration note %s missing required heading %q", fname, h)
			}
		}
		for _, cmd := range requiredMigrateSequence {
			if !strings.Contains(body, cmd) {
				t.Errorf("migration note %s missing required command %q in the migrate sequence", fname, cmd)
			}
		}
	}

	// When this binary is a release build, a note for its exact version must
	// ship. Dev builds (e.g. 0.1.0-dev) have no exact note and are exempt.
	if !strings.Contains(Version, "dev") {
		want := normalizeVersion(Version)
		if _, ok := notes[want]; !ok {
			t.Errorf("binary version %s has no embedded migration note (%s.md missing from %s)", want, want, corpus.MigrationsDir)
		}
	}
}

// TestMigrationNotes_EmbedDirWired is a smoke test that the embed directive is
// wired into the corpus package and points at the right path, so `help migrate`
// cannot silently read zero notes from a misconfigured embed.
func TestMigrationNotes_EmbedDirWired(t *testing.T) {
	if corpus.MigrationsDir != "templates/migrations" {
		t.Errorf("MigrationsDir = %q, want templates/migrations", corpus.MigrationsDir)
	}
	notes, _, err := migrationIndex()
	if err != nil {
		t.Fatalf("migrationIndex failed on the embedded tree: %v", err)
	}
	// Ensure the v0.1.8 seed note is resolvable (guards against a future
	// embed-path rename dropping the seed).
	if _, ok := notes["v0.1.8"]; !ok {
		t.Errorf("seed migration note v0.1.8 not found in embedded index; versions=%v", func() []string {
			ks := make([]string, 0, len(notes))
			for k := range notes {
				ks = append(ks, k)
			}
			return ks
		}())
	}
}
