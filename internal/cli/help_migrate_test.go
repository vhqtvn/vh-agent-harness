package cli

// This file pins the `help migrate [version]` feature and the migration-note
// canonical-format enforcement. It exercises the FULL command routing by
// driving rootCmd (via executeCapture), so it covers:
//   - rootCmd → helpCmd.SetHelpCommand wiring
//   - helpCmd.RunE intercepting the "migrate" topic → runHelpMigrate
//   - helpCmd.RunE delegating every other topic to runDefaultHelp (cobra default)
//   - the --help flag path still routing to the command's own Help()
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

// --- help migrate <explicit version> ----------------------------------------

// TestHelpMigrate_ExplicitVersion prints the named note verbatim from the
// embedded copy.
func TestHelpMigrate_ExplicitVersion(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "v0.1.8"})
	if err != nil {
		t.Fatalf("help migrate v0.1.8: want nil error, got %v", err)
	}
	// First-line heading + the release-class summary anchor the verbatim body.
	if !strings.Contains(out, "# Migration: v0.1.8") {
		t.Errorf("missing migration heading\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "Release class:") {
		t.Errorf("missing note body (Release class line)\n--- output ---\n%s", out)
	}
}

// TestHelpMigrate_VersionNormalization confirms a bare "X.Y.Z" arg is normalized
// to "vX.Y.Z" and resolves to the same note as the explicit v-prefixed form.
func TestHelpMigrate_VersionNormalization(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "0.1.8"})
	if err != nil {
		t.Fatalf("help migrate 0.1.8: want nil error (normalized), got %v", err)
	}
	if !strings.Contains(out, "# Migration: v0.1.8") {
		t.Errorf("normalized 0.1.8 should resolve to v0.1.8 note\n--- output ---\n%s", out)
	}
}

// TestHelpMigrate_ExplicitMissingVersion exits non-zero and lists the available
// versions (errSilent path — clean message, no cobra "Error:"/usage dump).
func TestHelpMigrate_ExplicitMissingVersion(t *testing.T) {
	out, err := executeCapture(t, []string{"help", "migrate", "v9.9.9"})
	if err == nil {
		t.Fatal("help migrate v9.9.9: want non-nil error (non-zero exit), got nil")
	}
	for _, want := range []string{
		"No migration note for v9.9.9",
		"Available versions:",
		"v0.1.8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
	// No cobra "Error:" line or usage dump should leak (SilenceErrors/Usage on
	// helpCmd). Our own lowercase "Usage: vh-agent-harness help migrate [version]"
	// line IS intentional; the cobra usage dump carries "Global Flags:" / a
	// "Flags:" block, which must not appear.
	if strings.Contains(out, "Error:") {
		t.Errorf("errSilent path must not print cobra Error line\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "Global Flags:") {
		t.Errorf("errSilent path must not print a cobra usage dump (Global Flags)\n--- output ---\n%s", out)
	}
}

// --- help migrate (no version) ----------------------------------------------

// TestHelpMigrate_NoArgNoLineage runs from a directory with no harness install:
// output must clearly state no install was detected and fall back to the latest
// available note.
func TestHelpMigrate_NoArgNoLineage(t *testing.T) {
	dir := t.TempDir()
	out, err := executeCaptureCwd(t, dir, []string{"help", "migrate"})
	if err != nil {
		t.Fatalf("no-arg no-lineage: want nil error, got %v", err)
	}
	for _, want := range []string{
		"No harness installation detected",
		"latest",         // the fallback framing
		"# Migration: v", // the fallback note body
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// TestHelpMigrate_NoArgWithLineage runs from a fixture with an installed seam
// whose lineage ref is forced to harness/v0.1.7. Output must mention BOTH the
// locally adopted version (v0.1.7) and the binary version, and fall back to the
// latest note since no v0.1.7 note is bundled. This pins the context-line +
// single-relevant-note contract (no cumulative-path overclaim).
func TestHelpMigrate_NoArgWithLineage(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	// Force the lineage content-origin ref to an older release (v0.1.7) so the
	// adopted-version detector has something concrete to report.
	lin, err := lineage.Read(root)
	if err != nil || lin == nil {
		t.Fatalf("read lineage after seam install: %v (lin=%v)", err, lin)
	}
	lin.Template.Ref = "harness/v0.1.7"
	if err := lin.Write(root); err != nil {
		t.Fatalf("write lineage ref: %v", err)
	}

	out, err := executeCaptureCwd(t, root, []string{"help", "migrate"})
	if err != nil {
		t.Fatalf("no-arg with-lineage: want nil error, got %v", err)
	}
	// Context line carries both the adopted and binary versions.
	if !strings.Contains(out, "Local adopted version: v0.1.7") {
		t.Errorf("want 'Local adopted version: v0.1.7'\n--- output ---\n%s", out)
	}
	wantBin := "Binary version:       " + normalizeVersion(Version)
	if !strings.Contains(out, wantBin) {
		t.Errorf("want %q\n--- output ---\n%s", wantBin, out)
	}
	// No exact v0.1.7 note exists → explicit fallback message, not a silent swap.
	if !strings.Contains(out, "showing the latest available note") {
		t.Errorf("want fallback-to-latest framing\n--- output ---\n%s", out)
	}
}

// TestHelpMigrate_NoArgAdoptsReleaseRef locks in the normalization-on-detection
// fix. The seam records the lineage ref as "harness/<Version>" where Version
// carries NO leading "v" (e.g. "harness/0.1.8", the real release shape). The
// detector must normalize that to "v0.1.8" so the context line displays
// consistently AND the note lookup actually matches the embedded "v0.1.8" key.
// Without normalization the lookup silently misses and falls back to latest.
func TestHelpMigrate_NoArgAdoptsReleaseRef(t *testing.T) {
	root := t.TempDir()
	seamInstallInto(t, root)
	lin, err := lineage.Read(root)
	if err != nil || lin == nil {
		t.Fatalf("read lineage after seam install: %v (lin=%v)", err, lin)
	}
	// Real release-ref shape: NO leading "v" (exactly how seam.go writes it).
	lin.Template.Ref = "harness/0.1.8"
	if err := lin.Write(root); err != nil {
		t.Fatalf("write lineage ref: %v", err)
	}

	out, err := executeCaptureCwd(t, root, []string{"help", "migrate"})
	if err != nil {
		t.Fatalf("no-arg release-ref: want nil error, got %v", err)
	}
	// Display normalizes the detected ref to the canonical "vX.Y.Z" form.
	if !strings.Contains(out, "Local adopted version: v0.1.8") {
		t.Errorf("want 'Local adopted version: v0.1.8' (normalized)\n--- output ---\n%s", out)
	}
	// The note for the adopted version IS bundled → it is printed directly, NOT
	// the latest-fallback path.
	if !strings.Contains(out, "# Migration: v0.1.8") {
		t.Errorf("want the v0.1.8 note body printed\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "showing the latest available note") {
		t.Errorf("adopted release ref must resolve to its own note, not the latest fallback\n--- output ---\n%s", out)
	}
}

// --- regression: normal help topics still route to cobra default ------------

// TestHelpMigrate_RegressionDefaultTopics confirms the help-command wrapper did
// NOT break normal help routing. `help guide` must reach the guide command's
// help; `guide --help` and `install --help` must still trigger the --help flag
// path (HelpFunc) on those subcommands.
func TestHelpMigrate_RegressionDefaultTopics(t *testing.T) {
	// `help guide` → runDefaultHelp → rootCmd.Find("guide") → guideCmd.Help().
	// guide has a Long, so its help prints the Long ("Orient yourself …"), not
	// the Short; anchor on stable Long text + the guide usage line.
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
