package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/lineage"
)

// srcDir returns the .vh-agent-harness/ source dir under a temp project, created.
func srcDir(t *testing.T, dir string) string {
	t.Helper()
	d := filepath.Join(dir, ".vh-agent-harness")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	return d
}

// TestComposeAgentsMd_OptInPreservesHandAuthored confirms the compose is opt-in:
// with no .vh-agent-harness/AGENTS.mission.md source, a project's hand-authored
// root AGENTS.md is left alone.
func TestComposeAgentsMd_OptInPreservesHandAuthored(t *testing.T) {
	dir := t.TempDir()
	hand := "# my hand-authored AGENTS\nDOMAIN\n"
	mustWrite(t, filepath.Join(dir, "AGENTS.md"), hand)
	mustWrite(t, filepath.Join(srcDir(t, dir), "AGENTS.core.md"), "# Core Rules\n")

	if err := composeAgentsMd(dir); err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if string(got) != hand {
		t.Errorf("no-mission: AGENTS.md was modified:\n%s", got)
	}
}

// TestComposeAgentsMd_Composes confirms that with a mission source present,
// AGENTS.md = AGENTS.core.md + blank line + AGENTS.mission.md (sources read from
// .vh-agent-harness/, output written to the root).
func TestComposeAgentsMd_Composes(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), "# Core Rules\nGENERIC\n")
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")

	if err := composeAgentsMd(dir); err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	s := string(got)
	if !strings.HasPrefix(s, "# Core Rules\nGENERIC") {
		t.Errorf("composed AGENTS.md should start with core:\n%s", s)
	}
	if !strings.Contains(s, "DOMAIN-MARKER") {
		t.Errorf("composed AGENTS.md should contain the mission:\n%s", s)
	}
	if i, j := strings.Index(s, "GENERIC"), strings.Index(s, "DOMAIN-MARKER"); i < 0 || j < 0 || i > j {
		t.Errorf("core must precede mission (core@%d, mission@%d)", i, j)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- feature-gated composition (flag-gated-composition-fix.md) ---------------
//
// The AGENTS.core.md source carries Go text/template conditionals
// ({{ if .features.backlog }}...{{ end }}) that the renderer's preserve-as-is
// copy leaves literal in the rendered file. composeAgentsMd must evaluate them
// at compose time so features.backlog:false suppresses the backlog section in
// the composed AGENTS.md. These tests are the load-bearing crux for that fix:
// they prove a render with the flag off drops the gated section, while the
// flag on (or the corpus default) keeps it.

const gatedCoreBody = "# Core Rules\nGENERIC\n{{ if .features.backlog }}\n## Backlog tracking rules\nBACKLOG-MARKER\n{{ end }}\n"

// TestComposeAgentsMd_BacklogGateTrue proves the gated section is PRESENT when
// features.backlog resolves true (explicit project override).
func TestComposeAgentsMd_BacklogGateTrue(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), gatedCoreBody)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	writeProfile(t, dir, "profile: minimal\nfeatures:\n  backlog: true\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if !strings.Contains(got, "BACKLOG-MARKER") {
		t.Errorf("backlog=true must include the gated section:\n%s", got)
	}
	if !strings.Contains(got, "## Backlog tracking rules") {
		t.Errorf("backlog=true must include the section heading:\n%s", got)
	}
	// The template action markers must NOT leak into the composed output.
	if strings.Contains(got, "{{ ") || strings.Contains(got, "{{if") {
		t.Errorf("composed AGENTS.md leaked a template action marker:\n%s", got)
	}
}

// TestComposeAgentsMd_BacklogGateFalse proves the gated section is ABSENT when
// features.backlog resolves false. This is the bug the fix targets: before the
// fix the section shipped regardless of the flag.
func TestComposeAgentsMd_BacklogGateFalse(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), gatedCoreBody)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	writeProfile(t, dir, "profile: minimal\nfeatures:\n  backlog: false\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if strings.Contains(got, "BACKLOG-MARKER") {
		t.Errorf("backlog=false must EXCLUDE the gated section:\n%s", got)
	}
	if strings.Contains(got, "## Backlog tracking rules") {
		t.Errorf("backlog=false must EXCLUDE the section heading:\n%s", got)
	}
	// The non-gated generic core content must survive (bundle-safe-disable:
	// suppress only the feature delta, never shared content).
	if !strings.Contains(got, "GENERIC") {
		t.Errorf("backlog=false must still keep the generic core content:\n%s", got)
	}
	if !strings.Contains(got, "DOMAIN-MARKER") {
		t.Errorf("backlog=false must still keep the mission content:\n%s", got)
	}
}

// TestComposeAgentsMd_BacklogGateDefault proves the corpus default
// (features.backlog=true, the shipped platform default) keeps the section when
// the project has not written a profile — backward-compatible with every
// existing consumer that never touched the flag.
func TestComposeAgentsMd_BacklogGateDefault(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), gatedCoreBody)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	// No profile written -> reconciledFeatures returns the corpus default.

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if !strings.Contains(got, "BACKLOG-MARKER") {
		t.Errorf("corpus default (backlog=true) must include the gated section:\n%s", got)
	}
}

// composeAndRead runs composeAgentsMd and returns the composed AGENTS.md body.
func composeAndRead(t *testing.T, dir string) (string, error) {
	t.Helper()
	if err := composeAgentsMd(dir); err != nil {
		return "", err
	}
	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read composed AGENTS.md: %v", err)
	}
	return string(got), nil
}

// --- answer-threading (renderAgentsCoreTemplate data-context widening) --------
//
// renderAgentsCoreTemplate historically built a features-ONLY data context, so a
// future {{ .project_name }} / {{ .project_slug }} / {{ .coordinator_dir }}
// Go-template action in AGENTS.core.md would render as the silent <no value>
// footgun (documented at the call site). The widening threads installRenderAnswers
// alongside the features map so those dot-actions resolve to the SAME value the
// renderer's SubstituteHarnessTokens pass resolved the equivalent UPPER sentinels
// to. This test is the load-bearing crux: it proves (1) a {{ .project_name }}
// action resolves non-empty, (2) coordinator_dir defaults to "coordinator", and
// (3) a {{ if .features.backlog }} action in the SAME core STILL evaluates
// correctly — i.e. threading answers did NOT switch to missingkey=error (which
// would break the features zero-value-falsy semantics).

// TestComposeAgentsMd_AnswersThreadedAndFeaturesPreserved proves the data
// context widening closes the <no value> footgun while preserving the features
// gating semantics. Greenfield temp dir (no lineage, no profile):
// installRenderAnswers falls back to defaultAnswers (project_name = dir basename),
// coordinatorDirOrDefault returns "coordinator", and reconciledFeatures returns
// the corpus default (backlog=true).
func TestComposeAgentsMd_AnswersThreadedAndFeaturesPreserved(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	// A core carrying BOTH a non-features dot-action (the footgun this fix
	// closes) and a feature conditional (the mechanism this fix must preserve).
	core := "# Core\n" +
		"NAME=[{{ .project_name }}]\n" +
		"SLUG=[{{ .project_slug }}]\n" +
		"COORD=[{{ .coordinator_dir }}]\n" +
		"{{ if .features.backlog }}BACKLOG-PRESENT\n{{ end }}\n"
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), core)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	// The dot-actions must resolve to concrete values, NOT render as the silent
	// <no value> footgun.
	if strings.Contains(got, "<no value>") {
		t.Errorf("threaded answers must resolve dot-actions; found <no value>:\n%s", got)
	}
	// project_name / project_slug resolve to the temp dir basename (defaultAnswers
	// derives both from filepath.Base(target)).
	expectedName := filepath.Base(dir)
	if !strings.Contains(got, "NAME=["+expectedName+"]") {
		t.Errorf("project_name must resolve to dir basename %q; got:\n%s", expectedName, got)
	}
	if !strings.Contains(got, "SLUG=["+expectedName+"]") {
		t.Errorf("project_slug must resolve to dir basename %q; got:\n%s", expectedName, got)
	}
	// coordinator_dir resolves to the substrate-mirrored default "coordinator"
	// (no lineage answer in a greenfield temp dir).
	if !strings.Contains(got, "COORD=[coordinator]") {
		t.Errorf("coordinator_dir must default to \"coordinator\"; got:\n%s", got)
	}
	// The feature conditional must STILL evaluate (backlog=true corpus default),
	// proving the widening preserved the features zero-value-falsy semantics
	// (no missingkey=error switch).
	if !strings.Contains(got, "BACKLOG-PRESENT") {
		t.Errorf("features.backlog (corpus default true) must still include the gated section:\n%s", got)
	}
	if !strings.Contains(got, "DOMAIN-MARKER") {
		t.Errorf("mission half must survive:\n%s", got)
	}
	// No template action markers may leak into the composed output.
	if strings.Contains(got, "{{ ") || strings.Contains(got, "{{if") {
		t.Errorf("composed AGENTS.md leaked a template action marker:\n%s", got)
	}
}

// TestComposeAgentsMd_AnswersFromLineageRecord proves the lineage-present branch
// of installRenderAnswers (the "lineage-or-default" equivalence half of the
// widening): when a lineage.yml carries distinct install-identity values, the
// dot-actions emit THOSE values (not the defaultAnswers dir-basename fallback),
// and coordinatorDirOrDefault returns the lineage's coordinator_dir (not the
// "coordinator" default). This closes the test gap where only the greenfield
// default-answers path was exercised.
func TestComposeAgentsMd_AnswersFromLineageRecord(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	// Seed a lineage record with DISTINCT install-identity values so a pass via
	// defaultAnswers (dir basename / "coordinator") would be detectable: every
	// asserted value differs from the fallback.
	lin := &lineage.Lineage{
		LineageVersion: "1",
		Answers: lineage.AnswersRef{
			Values: map[string]string{
				"project_name":    "lin-proj",
				"project_slug":    "lin-slug",
				"coordinator_dir": "lin-coord",
			},
		},
	}
	if err := lin.Write(dir); err != nil {
		t.Fatalf("seed lineage: %v", err)
	}
	core := "# Core\n" +
		"NAME=[{{ .project_name }}]\n" +
		"SLUG=[{{ .project_slug }}]\n" +
		"COORD=[{{ .coordinator_dir }}]\n" +
		"{{ if .features.backlog }}BACKLOG-PRESENT\n{{ end }}\n"
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), core)
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if strings.Contains(got, "<no value>") {
		t.Errorf("threaded answers must resolve dot-actions; found <no value>:\n%s", got)
	}
	// The lineage values must win over the defaultAnswers fallback.
	if !strings.Contains(got, "NAME=[lin-proj]") {
		t.Errorf("project_name must come from the lineage record (lin-proj), not defaultAnswers:\n%s", got)
	}
	if !strings.Contains(got, "SLUG=[lin-slug]") {
		t.Errorf("project_slug must come from the lineage record (lin-slug):\n%s", got)
	}
	if !strings.Contains(got, "COORD=[lin-coord]") {
		t.Errorf("coordinator_dir must come from the lineage record (lin-coord), not the \"coordinator\" default:\n%s", got)
	}
	// The feature conditional must still evaluate (corpus default backlog=true),
	// proving the widening preserved the features zero-value-falsy semantics on
	// the lineage-present path too.
	if !strings.Contains(got, "BACKLOG-PRESENT") {
		t.Errorf("features.backlog (corpus default true) must still include the gated section:\n%s", got)
	}
	if strings.Contains(got, "{{ ") || strings.Contains(got, "{{if") {
		t.Errorf("composed AGENTS.md leaked a template action marker:\n%s", got)
	}
}
