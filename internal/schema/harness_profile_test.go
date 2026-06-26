package schema

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// mustParseProfile unmarshals a harness-profile blob for test assertions.
func mustParseProfile(t *testing.T, raw []byte) harnessProfileData {
	t.Helper()
	var d harnessProfileData
	if err := yaml.Unmarshal(raw, &d); err != nil {
		t.Fatalf("merged output not parseable: %v\n%s", err, raw)
	}
	return d
}

func TestHarnessProfileValidate_Conformant(t *testing.T) {
	raw := []byte(`profile: coordination
modules: [core, web]
features:
  backlog: true
overlays: [web-overlay]
policy_packs: []
`)
	errs := HarnessProfile{}.Validate(raw)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %+v", errs)
	}
}

func TestHarnessProfileValidate_UnknownKeyAndBadEnum(t *testing.T) {
	raw := []byte(`profile: bogus
modules: [core, core, ""]
features: {a: true}
unknown_key: 1
`)
	errs := HarnessProfile{}.Validate(raw)
	// Expect: bad profile enum, duplicate module, empty module, unknown key.
	if len(errs) != 4 {
		t.Fatalf("expected 4 errors, got %d: %+v", len(errs), errs)
	}
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{"profile", "modules[1]", "modules[2]", "unknown_key"} {
		if !fields[want] {
			t.Fatalf("missing expected field %q in %+v", want, errs)
		}
	}
}

func TestHarnessProfileReconcile_CleanAppendUnionAndKeyedMerge(t *testing.T) {
	project := []byte(`profile: coordination
modules: [web]
features:
  backlog: true
overlays: [web-overlay]
policy_packs: []
`)
	platformDefault := []byte(`profile: minimal
modules: [core, docs]
features:
  backlog: false
  safe_defaults: true
overlays: []
policy_packs: []
`)
	res, err := HarnessProfile{}.Reconcile(project, platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomeApply {
		t.Fatalf("expected Apply, got %s (proposals=%+v)", res.Outcome, res.Proposals)
	}
	d := mustParseProfile(t, res.Merged)

	// profile: project selection wins within enum.
	if d.Profile != "coordination" {
		t.Fatalf("profile: expected coordination (project-wins), got %q", d.Profile)
	}
	// modules: union deduped+sorted = [core docs web].
	if got := joinSorted(d.Modules); got != "core,docs,web" {
		t.Fatalf("modules: expected core,docs,web; got %q (%v)", got, d.Modules)
	}
	// overlays: union = [web-overlay].
	if got := joinSorted(d.Overlays); got != "web-overlay" {
		t.Fatalf("overlays: expected web-overlay; got %q (%v)", got, d.Overlays)
	}
	// features: project backlog=true wins; platform safe_defaults=true added.
	if d.Features["backlog"] != true {
		t.Fatalf("features.backlog: expected true (project override), got %v", d.Features["backlog"])
	}
	if d.Features["safe_defaults"] != true {
		t.Fatalf("features.safe_defaults: expected true (platform default added), got %v", d.Features["safe_defaults"])
	}
	if len(d.Features) != 2 {
		t.Fatalf("features: expected exactly 2 keys, got %v", d.Features)
	}
	// Applied notes should mention the merges.
	joined := strings.Join(res.Applied, " | ")
	if !strings.Contains(joined, "modules") || !strings.Contains(joined, "features") {
		t.Fatalf("applied notes missing merge detail: %v", res.Applied)
	}
}

func TestHarnessProfileReconcile_ProfileSeededWhenProjectBlank(t *testing.T) {
	project := []byte(`modules: [web]
`)
	platformDefault := []byte(`profile: minimal
modules: [core]
`)
	res, err := HarnessProfile{}.Reconcile(project, platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomeApply {
		t.Fatalf("expected Apply, got %s", res.Outcome)
	}
	d := mustParseProfile(t, res.Merged)
	// profile absent in project -> seeded from platform default.
	if d.Profile != "minimal" {
		t.Fatalf("profile: expected seeded minimal, got %q", d.Profile)
	}
	// modules union.
	if got := joinSorted(d.Modules); got != "core,web" {
		t.Fatalf("modules: expected core,web; got %q", got)
	}
}

func TestHarnessProfileReconcile_EnumRemovedEmitsStructuredProposal(t *testing.T) {
	// Project selected a profile value the platform's enum no longer offers.
	project := []byte(`profile: experimental
modules: [core]
`)
	platformDefault := []byte(`profile: minimal
modules: [core]
`)
	res, err := HarnessProfile{}.Reconcile(project, platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomePropose {
		t.Fatalf("expected Propose, got %s", res.Outcome)
	}
	if len(res.Proposals) != 1 {
		t.Fatalf("expected exactly 1 proposal, got %+v", res.Proposals)
	}
	p := res.Proposals[0]
	// The proposal MUST name the schema field (dotted path), not a conflict marker.
	if p.Field != "profile" {
		t.Fatalf("proposal field: expected 'profile', got %q", p.Field)
	}
	if p.Kind != "enum_removed" {
		t.Fatalf("proposal kind: expected enum_removed, got %q", p.Kind)
	}
	if p.ProjectValue != "experimental" {
		t.Fatalf("proposal project value: expected experimental, got %v", p.ProjectValue)
	}
	if p.PlatformValue != "minimal" {
		t.Fatalf("proposal platform value: expected minimal, got %v", p.PlatformValue)
	}
	// Envelope must describe the valid enum.
	if !strings.Contains(p.Envelope, "minimal") || !strings.Contains(p.Envelope, "coordination") {
		t.Fatalf("proposal envelope must list enum members: %q", p.Envelope)
	}
	// Merged must be nil (apply blocked).
	if res.Merged != nil {
		t.Fatalf("apply must be blocked on proposal; got merged bytes: %s", res.Merged)
	}
	// Must NOT contain textual conflict markers anywhere in the proposal.
	for _, flat := range []string{p.Field, p.Kind, p.Envelope, p.Hint} {
		if strings.Contains(flat, "<<<<") || strings.Contains(flat, "======") || strings.Contains(flat, ".rej") {
			t.Fatalf("proposal must not emit conflict markers: %+v", p)
		}
	}
}

func TestHarnessProfileReconcile_NoopFastPath(t *testing.T) {
	identical := []byte(`profile: minimal
modules: [core]
`)
	res, err := HarnessProfile{}.Reconcile(identical, identical)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomeNoop {
		t.Fatalf("expected Noop for byte-identical input, got %s", res.Outcome)
	}
	if res.Merged != nil {
		t.Fatalf("Noop must not produce merged bytes")
	}
}

func TestHarnessProfileReconcile_BothEmptyIsNoop(t *testing.T) {
	res, err := HarnessProfile{}.Reconcile(nil, nil)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomeNoop {
		t.Fatalf("expected Noop for both-empty, got %s", res.Outcome)
	}
}

func TestHarnessProfileReconcile_MalformedProjectIsError(t *testing.T) {
	// Not valid YAML at all.
	res, err := HarnessProfile{}.Reconcile([]byte("profile: [unclosed"), []byte("profile: minimal\n"))
	if err == nil {
		t.Fatalf("expected error for malformed project, got result %+v", res)
	}
}

// joinSorted is a tiny test helper to assert union contents deterministically.
func joinSorted(in []string) string {
	out := append([]string(nil), in...)
	// reconciler already sorts; re-sort defensively in case of drift.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return strings.Join(out, ",")
}
