package cli

// doctor --json release-context routing tests.
//
// The historical-M release flow routes every registered doctor check into a
// release-context class (target | control | both):
//
//   - target: declarations/read from the verification worktree X at M;
//   - control: live resources from the original checkout H (or external
//     env) — X structurally lacks .local/, the .git ledger, and user-global
//     state, so a control verdict from X would be a structural false-FAIL;
//   - both: meaningful on both sides and independently checkable.
//
// The wrapper's G0c target run consumes the JSON envelope and fails closed
// when coverage is incomplete. These tests pin:
//
//   1. the routing registry covers EVERY result name doctor can emit
//      (a static 31-name list mirrored here — adding a check without a
//      class fails this test);
//   2. an unclassified name fails coverage (synthetic FAIL entry);
//   3. the line capture parses result lines (including the two-token
//      "eval.js" name) and the suppress mode forwards nothing;
//   4. a live CLI run emits a pure-JSON envelope whose checks all carry a
//      release-context class.

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// doctorReleaseRoutingRegistry is the static mirror of the result names
// doctor emits today. doctorReleaseRouting (doctor.go) is the canonical
// map; this list forces the two to stay in sync — a new check added to
// doctor.go without a routing entry fails
// TestDoctorReleaseRoutingRegistryComplete, and a routing entry whose name
// doctor can never emit fails it in the other direction.
var doctorReleaseRoutingRegistry = []string{
	// target
	"lineage", "armed-schema", "agents-composition", "overlay-perm",
	"capability-coherence", "gitignore", "auto-classifier",
	"skills", "shipped-pilots", "dead-grants", "cohort-links",
	"f1-envelope", "f1-f2-consistency", "f2-pairs",
	// both
	"managed-drift", "auto-gate-ignore", "behavioral-closure",
	"rewrite-parity", "exec-sandbox-floor",
	// control (config-refs is control-classed: its agent-model {file:} ref
	// targets are live .local state the seam seeds on the original checkout —
	// a checkout of M structurally lacks them, so a target-side verdict would
	// be a structural false-FAIL; see the routing map's comment)
	"node", "eval.js", "subagent-depth", "defer-liveness", "config-refs",
	"staged-errata-content", "dev-stale-embed", "head-progress",
	"complexity-advisory", "recurrence-state", "closeout-reach", "redlines",
}

func TestDoctorReleaseRoutingRegistryComplete(t *testing.T) {
	if len(doctorReleaseRouting) != len(doctorReleaseRoutingRegistry) {
		t.Errorf("routing map has %d entries; registry expects %d — a doctor check was added or removed without syncing the release-context classes",
			len(doctorReleaseRouting), len(doctorReleaseRoutingRegistry))
	}
	for _, name := range doctorReleaseRoutingRegistry {
		class, ok := doctorReleaseRouting[name]
		if !ok {
			t.Errorf("result name %q has NO release-context class — every registered doctor check must carry one (unknown checks fail release-context coverage)", name)
			continue
		}
		switch class {
		case releaseClassTarget, releaseClassControl, releaseClassBoth:
		default:
			t.Errorf("result name %q has non-canonical class %q", name, class)
		}
	}
	for name := range doctorReleaseRouting {
		found := false
		for _, reg := range doctorReleaseRoutingRegistry {
			if reg == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("routing map carries %q which the static registry does not list — update doctorReleaseRoutingRegistry (or remove the stale entry)", name)
		}
	}
}

func TestBuildDoctorJSONReport_ClassifiesKnownChecks(t *testing.T) {
	results := []checkResult{
		{name: "lineage", tier: tierPass, detail: "ok"},
		{name: "defer-liveness", tier: tierFail, detail: "control-side failure"},
		{name: "skills", tier: tierWarn, detail: "stale skill"},
		{name: "dev-stale-embed", tier: tierInfo, detail: "n/a"},
	}
	rep := buildDoctorJSONReport(results, 1, 1)
	if !rep.CoverageComplete || len(rep.Unclassified) != 0 {
		t.Errorf("known checks must be fully covered; coverage=%v unclassified=%v", rep.CoverageComplete, rep.Unclassified)
	}
	if len(rep.Checks) != len(results) {
		t.Fatalf("checks = %d; want %d", len(rep.Checks), len(results))
	}
	byName := map[string]doctorJSONCheck{}
	for _, c := range rep.Checks {
		byName[c.Name] = c
	}
	if c := byName["lineage"]; c.Class != releaseClassTarget {
		t.Errorf("lineage class = %q; want %q", c.Class, releaseClassTarget)
	}
	if c := byName["defer-liveness"]; c.Class != releaseClassControl {
		t.Errorf("defer-liveness class = %q; want %q", c.Class, releaseClassControl)
	}
	if c := byName["skills"]; c.Tier != tierWarn || c.Class != releaseClassTarget {
		t.Errorf("skills = %+v; want WARN/target", c)
	}
	if rep.Problems != 1 || rep.Warnings != 1 || rep.Healthy {
		t.Errorf("problems/warnings/healthy = %d/%d/%v; want 1/1/false", rep.Problems, rep.Warnings, rep.Healthy)
	}
	if rep.SchemaVersion != 1 {
		t.Errorf("schema_version = %d; want 1", rep.SchemaVersion)
	}
}

func TestBuildDoctorJSONReport_UnclassifiedCheckFailsCoverage(t *testing.T) {
	results := []checkResult{
		{name: "lineage", tier: tierPass, detail: "ok"},
		{name: "brand-new-check", tier: tierPass, detail: "unrouted"},
	}
	rep := buildDoctorJSONReport(results, 0, 0)
	if rep.CoverageComplete {
		t.Errorf("an unclassified check must set coverage_complete=false")
	}
	if len(rep.Unclassified) != 1 || rep.Unclassified[0] != "brand-new-check" {
		t.Errorf("unclassified = %v; want [brand-new-check]", rep.Unclassified)
	}
	// A synthetic FAIL entry lands in checks so JSON consumers see the
	// coverage failure as a first-class check.
	var synthetic *doctorJSONCheck
	for i := range rep.Checks {
		if rep.Checks[i].Name == "release-context-coverage" {
			synthetic = &rep.Checks[i]
		}
	}
	if synthetic == nil {
		t.Fatalf("no synthetic release-context-coverage entry; checks=%v", rep.Checks)
	}
	if synthetic.Tier != tierFail {
		t.Errorf("synthetic tier = %q; want FAIL", synthetic.Tier)
	}
	if !strings.Contains(synthetic.Detail, "brand-new-check") {
		t.Errorf("synthetic detail must name the unclassified check; got %q", synthetic.Detail)
	}
	if rep.Problems != 1 || rep.Healthy {
		t.Errorf("unclassified coverage failure must increment problems and clear healthy; problems=%d healthy=%v", rep.Problems, rep.Healthy)
	}
}

func TestDoctorLineCapture_ParsesResultLines(t *testing.T) {
	block := "doctor:\n" +
		"  lineage:\n" +
		"    lineage       PASS  seam lineage present\n" +
		"  environment:\n" +
		"    node         PASS  v20.1.0\n" +
		"    eval.js      PASS  bridge ok\n" +
		"    staged-errata-content FAIL card body missing from note\n" +
		"summary: 1 problem(s), 0 warning(s)\n"

	// Passthrough mode (no --json): everything forwarded AND captured.
	var fwd strings.Builder
	pw := &doctorLineCapture{inner: &fwd, suppress: false}
	if _, err := pw.Write([]byte(block)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if fwd.String() != block {
		t.Errorf("passthrough must forward bytes unmodified")
	}
	assertCaptured := func(res []checkResult) {
		byName := map[string]checkResult{}
		for _, r := range res {
			byName[r.name] = r
		}
		if len(res) != 4 {
			t.Errorf("captured %d results; want 4 (%v)", len(res), res)
		}
		if r := byName["eval.js"]; r.name != "eval.js" || r.tier != tierPass {
			t.Errorf("two-token name eval.js mis-parsed: %+v", r)
		}
		if r := byName["staged-errata-content"]; r.tier != tierFail || r.detail != "card body missing from note" {
			t.Errorf("long-name FAIL mis-parsed: %+v", r)
		}
		if r := byName["node"]; r.detail != "v20.1.0" {
			t.Errorf("node detail mis-parsed: %+v", r)
		}
	}
	assertCaptured(pw.results)

	// Suppress mode (--json): nothing forwarded, still captured.
	var suppressed strings.Builder
	sw := &doctorLineCapture{inner: &suppressed, suppress: true}
	if _, err := sw.Write([]byte(block)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if suppressed.String() != "" {
		t.Errorf("suppress mode must forward nothing; got %q", suppressed.String())
	}
	assertCaptured(sw.results)
}

// TestDoctorJSONFlag_CLI_EmitsPureEnvelope runs the built ceremony binary
// with doctor --json against the real module root and asserts the stdout is
// a pure JSON envelope whose checks all carry a release-context class. The
// repo's health verdict itself is NOT asserted (concurrent sessions may
// legitimately hold dirt); the envelope SHAPE is the contract.
func TestDoctorJSONFlag_CLI_EmitsPureEnvelope(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live binary doctor run in -short mode")
	}
	binDir := t.TempDir()
	copyHarnessBinaryToCeremony(t, binDir)
	bin := filepath.Join(binDir, "bin", "vh-agent-harness")
	root := findModuleRoot(t)

	cmd := exec.Command(bin, "doctor", "--json")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("doctor --json spawn error: %v", err)
		}
		// Non-zero exit is acceptable (errSilent on problems) — the JSON
		// envelope must still be complete on stdout.
	}
	stdout := string(out)
	if !strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("stdout must be a pure JSON envelope (no human preamble); got:\n%.400s", stdout)
	}
	var rep doctorJSONReport
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("envelope must parse: %v\n%.400s", err, stdout)
	}
	if rep.SchemaVersion != 1 {
		t.Errorf("schema_version = %d; want 1", rep.SchemaVersion)
	}
	if !rep.CoverageComplete || len(rep.Unclassified) != 0 {
		t.Errorf("live repo envelope must be fully classified; unclassified=%v", rep.Unclassified)
	}
	if len(rep.Checks) < 25 {
		t.Errorf("expected the real repo to emit most checks; got %d", len(rep.Checks))
	}
	seen := map[string]int{}
	for _, c := range rep.Checks {
		switch c.Class {
		case releaseClassTarget, releaseClassControl, releaseClassBoth:
		default:
			t.Errorf("check %q carries no release-context class (%q)", c.Name, c.Class)
		}
		seen[c.Name]++
		if seen[c.Name] > 1 {
			t.Errorf("check %q emitted twice", c.Name)
		}
	}
}
