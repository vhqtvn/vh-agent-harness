package schema

import (
	"strings"
	"testing"
)

func TestComplexityPolicyValidate(t *testing.T) {
	good := []byte(`version: 1
enabled: true
defaults:
  event_file_lines: 350
  snapshot_file_lines: 500
per_language:
  ".go": {}
  ".js":
    event_file_lines: 400
exclude:
  event_paths:
    - "tmp/**"
  snapshot_paths:
    - ".opencode/**"
  snapshot_suffixes:
    - "_test.go"
doctor:
  max_candidates: 10
recurrence:
  enabled: false
`)
	if errs := (ComplexityPolicy{}).Validate(good); len(errs) != 0 {
		t.Fatalf("good complexity-policy: expected no errors, got %+v", errs)
	}

	// Bad: wrong version, zero thresholds, bad per_language override, zero
	// max_candidates, and a stray top-level key.
	bad := []byte(`version: 2
enabled: true
defaults:
  event_file_lines: 0
  snapshot_file_lines: -5
per_language:
  ".go":
    event_file_lines: 0
rogue_key: 1
doctor:
  max_candidates: 0
`)
	errs := (ComplexityPolicy{}).Validate(bad)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{
		"version",
		"defaults.event_file_lines",
		"defaults.snapshot_file_lines",
		`per_language[".go"].event_file_lines`,
		"doctor.max_candidates",
		"rogue_key",
	} {
		if !fields[want] {
			t.Fatalf("missing expected error field %q in %+v", want, errs)
		}
	}
}

func TestComplexityPolicyReconcile(t *testing.T) {
	platformDefault := []byte(`version: 1
enabled: true
defaults:
  event_file_lines: 350
  snapshot_file_lines: 500
per_language:
  ".go": {}
  ".js": {}
exclude:
  event_paths:
    - "tmp/**"
  snapshot_paths:
    - ".opencode/**"
  snapshot_suffixes:
    - "_test.go"
doctor:
  max_candidates: 10
recurrence:
  enabled: false
`)

	t.Run("noop when identical", func(t *testing.T) {
		res, err := (ComplexityPolicy{}).Reconcile(platformDefault, platformDefault)
		if err != nil {
			t.Fatalf("reconcile errored: %v", err)
		}
		if res.Outcome != OutcomeNoop {
			t.Fatalf("identical: expected Noop, got %s", res.Outcome)
		}
	})

	t.Run("apply merges per_language override + adds exclusion", func(t *testing.T) {
		project := []byte(`version: 1
enabled: false
defaults:
  event_file_lines: 350
  snapshot_file_lines: 600
per_language:
  ".go":
    snapshot_file_lines: 800
  ".py": {}
exclude:
  event_paths:
    - "logs/**"
  snapshot_paths:
    - ".opencode/**"
  snapshot_suffixes:
    - "_test.go"
doctor:
  max_candidates: 5
recurrence:
  enabled: true
`)
		res, err := (ComplexityPolicy{}).Reconcile(project, platformDefault)
		if err != nil {
			t.Fatalf("reconcile errored: %v", err)
		}
		if res.Outcome != OutcomeApply {
			t.Fatalf("expected Apply, got %s", res.Outcome)
		}
		merged := string(res.Merged)
		// project overrides retained
		if !strings.Contains(merged, "snapshot_file_lines: 600") {
			t.Fatalf("merged missing project defaults.snapshot_file_lines=600:\n%s", merged)
		}
		// new platform key (.js) merged in
		if !strings.Contains(merged, ".js: {}") {
			t.Fatalf("merged missing platform per_language .js:\n%s", merged)
		}
		// project-added exclusion retained + platform exclusion retained
		if !strings.Contains(merged, "logs/**") {
			t.Fatalf("merged missing project event_paths logs/**:\n%s", merged)
		}
		if !strings.Contains(merged, "tmp/**") {
			t.Fatalf("merged missing platform event_paths tmp/**:\n%s", merged)
		}
		// re-validating the merged output must be clean
		if errs := (ComplexityPolicy{}).Validate(res.Merged); len(errs) != 0 {
			t.Fatalf("merged output is invalid: %+v", errs)
		}
	})

	t.Run("absent project seeds from default", func(t *testing.T) {
		res, err := (ComplexityPolicy{}).Reconcile(nil, platformDefault)
		if err != nil {
			t.Fatalf("reconcile errored: %v", err)
		}
		if res.Outcome != OutcomeApply {
			t.Fatalf("absent project: expected Apply (seed), got %s", res.Outcome)
		}
	})
}

func TestComplexityDispositionsValidate_BlankSeed(t *testing.T) {
	blank := []byte("version: 1\ndispositions: []\n")
	if errs := (ComplexityDispositions{}).Validate(blank); len(errs) != 0 {
		t.Fatalf("blank seed: expected no errors, got %+v", errs)
	}
}

func TestComplexityDispositionsValidate_AcceptCohesive(t *testing.T) {
	good := []byte(`version: 1
dispositions:
  - disposition_id: D-001
    logical_artifact: "internal/cli/doctor.go"
    paths:
      - "internal/cli/doctor.go"
    decision: accept-as-cohesive
    signal:
      metric_kind: file_loc
      observations:
        - path: "internal/cli/doctor.go"
    accept_rationale:
      responsibility: "doctor entrypoint"
      organizing_spine: "named-check pipeline"
      cohesion_evidence: "all checks share runDoctor"
      internal_boundaries: "per-check helpers"
      extraction_tradeoff: "high coupling cost"
      verification_seams: "seam_cli_test.go"
      reconsider_trigger: "exceeds 3000 lines"
`)
	if errs := (ComplexityDispositions{}).Validate(good); len(errs) != 0 {
		t.Fatalf("good accept-as-cohesive: expected no errors, got %+v", errs)
	}
}

func TestComplexityDispositionsValidate_SplitDefer(t *testing.T) {
	good := []byte(`version: 1
dispositions:
  - disposition_id: D-002
    logical_artifact: "state-lib.js"
    paths:
      - "templates/core/.opencode/scripts/state-lib.js"
    decision: split-defer
    signal:
      metric_kind: file_loc
      observations:
        - path: "templates/core/.opencode/scripts/state-lib.js"
    defer_link:
      task_id: "P2-COMPLEX-001"
      target_boundary: "state module"
      trigger: "next touch of state-lib.js"
      validation_plan: "snapshot stays green"
`)
	if errs := (ComplexityDispositions{}).Validate(good); len(errs) != 0 {
		t.Fatalf("good split-defer: expected no errors, got %+v", errs)
	}
}

func TestComplexityDispositionsValidate_StructuralErrors(t *testing.T) {
	// Missing rationale fields, observation path not in paths, dup disposition_id,
	// invalid decision, metric_kind, and a stray top-level key.
	bad := []byte(`version: 1
rogue: 1
dispositions:
  - disposition_id: D-001
    logical_artifact: ""
    paths: []
    decision: maybe
    signal:
      metric_kind: cyclomatic
      observations:
        - path: "not/in/paths.go"
    accept_rationale:
      responsibility: ""
  - disposition_id: D-001
    logical_artifact: "dup"
    paths: ["a.go"]
    decision: accept-as-cohesive
    signal:
      metric_kind: file_loc
      observations:
        - path: "a.go"
    accept_rationale:
      responsibility: r
      organizing_spine: r
      cohesion_evidence: r
      internal_boundaries: r
      extraction_tradeoff: r
      verification_seams: r
      reconsider_trigger: r
`)
	errs := (ComplexityDispositions{}).Validate(bad)
	fields := map[string]bool{}
	for _, e := range errs {
		fields[e.Field] = true
	}
	for _, want := range []string{
		"rogue",
		"dispositions[0].logical_artifact",
		"dispositions[0].paths",
		"dispositions[0].decision",
		"dispositions[0].signal.metric_kind",
		"dispositions[0].signal.observations[0].path",
		// NOTE: disposition[0] has invalid decision "maybe", so neither
		// accept-as-cohesive nor split-defer branch runs — accept_rationale
		// fields are NOT validated when the decision is unrecognized. The
		// cross-field rationale rules are covered in
		// TestComplexityDispositionsValidate_CrossFieldRules with a VALID
		// decision so the branch actually executes.
		"dispositions[1].disposition_id", // duplicate
	} {
		if !fields[want] {
			t.Fatalf("missing expected error field %q in %+v", want, errs)
		}
	}
}

func TestComplexityDispositionsValidate_CrossFieldRules(t *testing.T) {
	// accept-as-cohesive with a defer_link present -> error.
	bad := []byte(`version: 1
dispositions:
  - disposition_id: D-003
    logical_artifact: "x"
    paths: ["x.go"]
    decision: accept-as-cohesive
    signal:
      metric_kind: file_loc
      observations:
        - path: "x.go"
    accept_rationale:
      responsibility: r
      organizing_spine: r
      cohesion_evidence: r
      internal_boundaries: r
      extraction_tradeoff: r
      verification_seams: r
      reconsider_trigger: r
    defer_link:
      task_id: "T"
      target_boundary: "b"
      trigger: "t"
      validation_plan: "v"
`)
	errs := (ComplexityDispositions{}).Validate(bad)
	found := false
	for _, e := range errs {
		if e.Field == "dispositions[0].defer_link" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected defer_link error for accept-as-cohesive, got %+v", errs)
	}

	// split-defer with accept_rationale present -> error.
	bad2 := []byte(`version: 1
dispositions:
  - disposition_id: D-004
    logical_artifact: "y"
    paths: ["y.go"]
    decision: split-defer
    signal:
      metric_kind: file_loc
      observations:
        - path: "y.go"
    accept_rationale:
      responsibility: r
    defer_link:
      task_id: "T"
      target_boundary: "b"
      trigger: "t"
      validation_plan: "v"
`)
	errs = (ComplexityDispositions{}).Validate(bad2)
	found = false
	for _, e := range errs {
		if e.Field == "dispositions[0].accept_rationale" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected accept_rationale error for split-defer, got %+v", errs)
	}
}

func TestComplexityDispositionsReconcile_SeedOnly(t *testing.T) {
	platformDefault := []byte("version: 1\ndispositions: []\n")
	// Absent -> seed.
	res, err := (ComplexityDispositions{}).Reconcile(nil, platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res.Outcome != OutcomeApply {
		t.Fatalf("absent: expected Apply (seed), got %s", res.Outcome)
	}
	// Present -> Noop (project_owned).
	res2, err := (ComplexityDispositions{}).Reconcile(platformDefault, platformDefault)
	if err != nil {
		t.Fatalf("reconcile errored: %v", err)
	}
	if res2.Outcome != OutcomeNoop {
		t.Fatalf("present: expected Noop, got %s", res2.Outcome)
	}
}
