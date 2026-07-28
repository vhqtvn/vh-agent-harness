package schema

import (
	"testing"
)

func TestRunShapeValidate(t *testing.T) {
	good := []byte(`runtime:
  backend: docker-compose
lifecycle:
  hooks:
    pre_up: scripts/up.sh
    post_up: scripts/after-up.sh
exec_sandbox:
  min_mode: strict
`)
	if errs := (RunShape{}).Validate(good); len(errs) != 0 {
		t.Fatalf("good run-shape: expected no errors, got %+v", errs)
	}

	// exec_sandbox.min_mode accepts the full enum without error.
	for _, mode := range []string{"off", "best-effort", "strict"} {
		raw := []byte("exec_sandbox:\n  min_mode: " + mode + "\n")
		if errs := (RunShape{}).Validate(raw); len(errs) != 0 {
			t.Fatalf("min_mode=%s: expected no errors, got %+v", mode, errs)
		}
	}

	// forward-compat: an unknown future key alongside a present min_mode is fine.
	if errs := (RunShape{}).Validate([]byte("exec_sandbox:\n  min_mode: strict\n  future_key: x\n")); len(errs) != 0 {
		t.Fatalf("unknown future key + present min_mode: expected no errors, got %+v", errs)
	}

	// FAIL-CLOSED: a misspelled min_mode KEY (min_mode absent in a present block)
	// is flagged, so doctor catches the key-typo hole before runtime.
	keyTypoErrs := (RunShape{}).Validate([]byte("exec_sandbox:\n  min_mdoe: strict\n"))
	ktFields := map[string]bool{}
	for _, e := range keyTypoErrs {
		ktFields[e.Field] = true
	}
	if !ktFields["exec_sandbox.min_mode"] {
		t.Fatalf("misspelled min_mode key: expected exec_sandbox.min_mode error (block present but min_mode absent), got %+v", keyTypoErrs)
	}

	bad := []byte(`runtime:
  backend: k8s
lifecycle:
  hooks:
    pre_up: "docker compose up"   # inline shell, forbidden
    bogus_hook: scripts/x.sh
rogue_key: 1
exec_sandbox:
  min_mode: strcit   # typo — must fail-closed (not silently collapse to off)
`)
	errs := (RunShape{}).Validate(bad)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{
		"runtime.backend",        // bad enum
		"lifecycle.hooks.pre_up", // inline shell
		"lifecycle.hooks.bogus_hook",
		"rogue_key",
		"exec_sandbox.min_mode", // typo rejected (fail-closed floor boundary)
	} {
		if !fields[want] {
			t.Fatalf("missing expected error field %q in %+v", want, errs)
		}
	}
}

func TestRunShapeReconcile_SeedOnly(t *testing.T) {
	platformDefault := []byte(`runtime: {backend: bare}
`)
	// Absent project -> seed (Apply with Merged == platformDefault).
	res, err := (RunShape{}).Reconcile(nil, platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomeApply {
		t.Fatalf("absent project: expected Apply (seed), got %s", res.Outcome)
	}
	if string(res.Merged) != string(platformDefault) {
		t.Fatalf("seed merged != platformDefault")
	}
	// Present project -> Noop (never clobber).
	res2, err := (RunShape{}).Reconcile([]byte(`runtime: {backend: docker-compose}`), platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res2.Outcome != OutcomeNoop {
		t.Fatalf("present project: expected Noop (project_owned), got %s", res2.Outcome)
	}
}

func TestForbiddenPatternsValidate(t *testing.T) {
	// The canonical authoring form is a JSON-valid array (double-quoted keys are
	// valid JS object literals too), so the validator can extract+validate it
	// without a full JS parser. Stricter JS parsing lands with the engine (Slice 5).
	good := []byte(`module.exports = [
  { "pattern": "rm -rf /", "why": "destructive", "alternative": "vh-agent-harness exec rm scoped" }
];
`)
	if errs := (ForbiddenPatternsProject{}).Validate(good); len(errs) != 0 {
		t.Fatalf("good forbidden-patterns: expected no errors, got %+v", errs)
	}
	bad := []byte(`module.exports = [
  { "pattern": "", "why": "" }
];
`)
	errs := (ForbiddenPatternsProject{}).Validate(bad)
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors (empty pattern, empty why), got %+v", errs)
	}
}

func TestRepoReconValidate(t *testing.T) {
	good := []byte(`entrypoints:
  - name: api
    path: apps/api/main.go
packages:
  api:
    lang: python
tests:
  - path: tests/e2e
`)
	if errs := (RepoRecon{}).Validate(good); len(errs) != 0 {
		t.Fatalf("good repo-recon: expected no errors, got %+v", errs)
	}
	bad := []byte(`entrypoints:
  - {}
hotspots: "not-an-array"
rogue: 1
`)
	errs := (RepoRecon{}).Validate(bad)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{"entrypoints[0]", "hotspots", "rogue"} {
		if !fields[want] {
			t.Fatalf("missing expected error field %q in %+v", want, errs)
		}
	}
}

func TestSchemaForPath(t *testing.T) {
	cases := []struct {
		path    string
		want    Type
		present bool
	}{
		{".vh-agent-harness/vh-harness-profile.yml", TypeHarnessProfile, true},
		{"vh-harness-profile.yml", "", false}, // must be under .vh-agent-harness/
		{".vh-agent-harness/run-shape.yml", TypeRunShape, true},
		{"run-shape.yml", "", false}, // must be under .vh-agent-harness/
		{"forbidden-patterns.project.js", TypeForbiddenPatternsProject, true},
		{"repo-recon.image-lane.yml", TypeRepoRecon, true},
		{".vh-agent-harness/complexity-policy.yml", TypeComplexityPolicy, true},
		{"complexity-policy.yml", "", false}, // must be under .vh-agent-harness/
		{".opencode/repo-configs/complexity-dispositions.yml", TypeComplexityDispositions, true},
		{"complexity-dispositions.yml", "", false}, // must be under .opencode/repo-configs/
		{"AGENTS.md", "", false},
	}
	for _, c := range cases {
		s, ok := SchemaForPath(c.path)
		if ok != c.present {
			t.Fatalf("SchemaForPath(%q): present=%v want %v", c.path, ok, c.present)
		}
		if ok && s.Type != c.want {
			t.Fatalf("SchemaForPath(%q): type=%q want %q", c.path, s.Type, c.want)
		}
	}
}

func TestAllReturnsSixSchemas(t *testing.T) {
	all := All()
	if len(all) != 6 {
		t.Fatalf("expected 6 registered schemas, got %d", len(all))
	}
}
