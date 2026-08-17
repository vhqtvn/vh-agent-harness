package internal

// CI workflow-contract tests (P1-CI-002): parse the repo's GitHub Actions
// workflow YAMLs and assert DURABLE PROPERTIES of the CI contract — the
// blocking shape of the Go test gate, the drift-must-fail wiring of the
// render check, the stable aggregate status, and the
// evaluator-before-publication ordering of the release lane.
//
// These are properties, not incidental label/version snapshots: exact Node
// pins, action versions, step names, and PR-comment cosmetics may churn
// freely. What must NOT silently disappear is the contract itself — if a
// workflow edit removes or weakens one of these properties, the matching
// test fails. Parsing the YAML here also proves the changed workflows
// remain valid YAML (a syntax error in a workflow file fails these tests).
//
// The render-check pair assertion accepts the DELIBERATE pattern: a
// `continue-on-error: true` doctor dry-run step followed by a Fail-on-drift
// step asserting the dry-run outcome — drift must FAIL the workflow.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// YAML helpers (generic maps — workflows are too heterogeneous for one struct)
// ---------------------------------------------------------------------------

// repoRoot is derived from THIS file's location (…/internal/ci_contract_test.go),
// not from the test binary's cwd, so the contract tests find the workflows
// under any invocation directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot locate test source file")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

func loadWorkflow(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", name))
	if err != nil {
		t.Fatalf("read workflow %s: %v", name, err)
	}
	var wf map[string]interface{}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("workflow %s is not valid YAML: %v", name, err)
	}
	return wf
}

func workflowJobs(t *testing.T, wf map[string]interface{}) map[string]interface{} {
	t.Helper()
	jobs, ok := wf["jobs"].(map[string]interface{})
	if !ok {
		t.Fatalf("workflow has no jobs map: %+v", wf)
	}
	return jobs
}

func jobDef(t *testing.T, jobs map[string]interface{}, id string) map[string]interface{} {
	t.Helper()
	def, ok := jobs[id].(map[string]interface{})
	if !ok {
		t.Fatalf("job %q not found", id)
	}
	return def
}

func jobSteps(t *testing.T, job map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := job["steps"].([]interface{})
	if !ok {
		t.Fatalf("job has no steps list: %+v", job)
	}
	steps := make([]map[string]interface{}, 0, len(raw))
	for _, s := range raw {
		sm, ok := s.(map[string]interface{})
		if !ok {
			t.Fatalf("step is not a mapping: %+v", s)
		}
		steps = append(steps, sm)
	}
	return steps
}

// stepRun returns a step's `run` payload ("" when absent).
func stepRun(step map[string]interface{}) string {
	r, _ := step["run"].(string)
	return r
}

// stepUses returns a step's `uses` reference ("" when absent).
func stepUses(step map[string]interface{}) string {
	u, _ := step["uses"].(string)
	return u
}

// truthy interprets a YAML scalar as a GitHub Actions boolean.
func truthy(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true"
	}
	return false
}

// ---------------------------------------------------------------------------
// go-test.yml — the full Go suite stays a BLOCKING job matrix
// ---------------------------------------------------------------------------

func TestCIContract_GoTestSuiteIsBlockingMatrix(t *testing.T) {
	wf := loadWorkflow(t, "go-test.yml")

	// The gate fires on PRs and pushes to main.
	on, ok := wf["on"].(map[string]interface{})
	if !ok {
		t.Fatalf("go-test.yml has no `on` trigger map")
	}
	for _, trig := range []string{"pull_request", "push"} {
		if _, present := on[trig]; !present {
			t.Errorf("go-test.yml lost its %s trigger — the Go gate would stop running on %s events", trig, trig)
		}
	}

	jobs := workflowJobs(t, wf)
	gt := jobDef(t, jobs, "go-test")

	// No job in this workflow may swallow failures.
	for id, j := range jobs {
		jm, ok := j.(map[string]interface{})
		if !ok {
			t.Fatalf("job %q is not a mapping", id)
		}
		if truthy(jm["continue-on-error"]) {
			t.Errorf("job %q sets continue-on-error — a designated blocking job must fail the workflow", id)
		}
	}

	// The suite runs as a matrix across the supported Node versions (>= 2
	// legs). Exact pins may churn; the multi-leg matrix shape may not.
	strat, ok := gt["strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("go-test job lost its strategy matrix — the suite must run across supported Node versions")
	}
	matrix, ok := strat["matrix"].(map[string]interface{})
	if !ok {
		t.Fatalf("go-test job lost its matrix")
	}
	versions, ok := matrix["node-version"].([]interface{})
	if !ok || len(versions) < 2 {
		t.Fatalf("go-test matrix must cover at least 2 node-version legs, got: %+v", matrix["node-version"])
	}
	for i, v := range versions {
		s, _ := v.(string)
		if s == "" {
			t.Errorf("go-test matrix leg %d is empty", i)
		}
	}

	// The full suite command runs blocking, with LiveBridge skip-to-red wired
	// (a genuine skip is a red signal, not green — P2-CI-001).
	steps := jobSteps(t, gt)
	found := false
	for _, s := range steps {
		run := stepRun(s)
		if !strings.Contains(run, "go test") {
			continue
		}
		found = true
		if !strings.Contains(run, "go test ./...") {
			t.Errorf("go-test run does not execute the full suite: %q", run)
		}
		if strings.Contains(run, "-short") {
			t.Errorf("go-test run uses -short — the full suite must not be shortened: %q", run)
		}
		if truthy(s["continue-on-error"]) {
			t.Errorf("the full-suite step sets continue-on-error — test failures would not fail the workflow")
		}
		env, _ := s["env"].(map[string]interface{})
		if env["VH_REQUIRE_LIVE_BRIDGE"] != "1" {
			t.Errorf("full-suite step lost VH_REQUIRE_LIVE_BRIDGE=1 — LiveBridge skips would silently pass green")
		}
	}
	if !found {
		t.Fatalf("no step in go-test job runs `go test`")
	}
}

// ---------------------------------------------------------------------------
// go-test.yml — the stable aggregate blocking status (P2)
// ---------------------------------------------------------------------------

func TestCIContract_StableAggregateJob(t *testing.T) {
	wf := loadWorkflow(t, "go-test.yml")
	jobs := workflowJobs(t, wf)
	agg := jobDef(t, jobs, "ci-aggregate")

	// Depends on every designated blocking job: ci-aggregate's needs must
	// cover EVERY other job in this workflow (needs-completeness), so a
	// future job addition that forgets to extend the aggregate fails here
	// instead of silently escaping the stable required-check set.
	needs, ok := agg["needs"].([]interface{})
	if !ok {
		t.Fatalf("ci-aggregate has no needs list")
	}
	needsSet := map[string]bool{}
	for _, n := range needs {
		if s, _ := n.(string); s != "" {
			needsSet[s] = true
		}
	}
	for id := range jobs {
		if id == "ci-aggregate" {
			continue
		}
		if !needsSet[id] {
			t.Errorf("ci-aggregate does not depend on job %q — every other job in go-test.yml must be aggregated or it escapes the stable required-check set", id)
		}
	}
	if !needsSet["go-test"] {
		t.Errorf("ci-aggregate does not depend on go-test — it cannot aggregate that job's outcome")
	}

	// …and runs regardless of dependency outcome, so it can REPORT failures
	// instead of being skipped by them.
	ifc, _ := agg["if"].(string)
	if !strings.Contains(ifc, "always()") {
		t.Errorf("ci-aggregate must use if: always() so a failed dependency still reaches the fail step, got if: %q", ifc)
	}

	steps := jobSteps(t, agg)
	var summary, fail bool
	for _, s := range steps {
		run := stepRun(s)
		if strings.Contains(run, "GITHUB_STEP_SUMMARY") && strings.Contains(run, "needs.go-test.result") {
			summary = true // per-dependency result summary is emitted
		}
		sif, _ := s["if"].(string)
		if strings.Contains(sif, "contains(needs.*.result") &&
			strings.Contains(sif, "failure") &&
			strings.Contains(sif, "cancelled") &&
			strings.Contains(sif, "skipped") &&
			strings.Contains(run, "exit 1") {
			fail = true // fails on failure / cancellation / unexpected skip
		}
	}
	if !summary {
		t.Errorf("ci-aggregate has no step emitting a per-dependency result summary (GITHUB_STEP_SUMMARY + needs.go-test.result)")
	}
	if !fail {
		t.Errorf("ci-aggregate has no fail step covering failure + cancelled + skipped dependency results with a non-zero exit")
	}

	// It summarizes, never replaces: the go-test job itself still exists and
	// stays blocking (asserted by TestCIContract_GoTestSuiteIsBlockingMatrix).
	if _, present := jobs["go-test"]; !present {
		t.Errorf("ci-aggregate exists but the go-test job it aggregates is gone — aggregate must not replace individual statuses")
	}
}

// ---------------------------------------------------------------------------
// render-check.yml — install/render drift must FAIL the workflow
// ---------------------------------------------------------------------------

func TestCIContract_RenderCheckDriftFailsWorkflow(t *testing.T) {
	wf := loadWorkflow(t, "render-check.yml")
	jobs := workflowJobs(t, wf)
	rc := jobDef(t, jobs, "render-check")

	if truthy(rc["continue-on-error"]) {
		t.Errorf("render-check job sets continue-on-error — drift would not fail the workflow")
	}

	steps := jobSteps(t, rc)

	// Sequence: build the current checkout's binary -> install into a scratch
	// dir -> doctor. Steps may be added in between; the sequence may not
	// silently lose a stage.
	idxOf := func(pred func(map[string]interface{}) bool) int {
		for i, s := range steps {
			if pred(s) {
				return i
			}
		}
		return -1
	}
	buildIdx := idxOf(func(s map[string]interface{}) bool {
		r := stepRun(s)
		return strings.Contains(r, "go build") && strings.Contains(r, "cmd/vh-agent-harness")
	})
	installIdx := idxOf(func(s map[string]interface{}) bool {
		r := stepRun(s)
		return strings.Contains(r, "install") && strings.Contains(r, "--target")
	})
	if buildIdx < 0 {
		t.Fatalf("render-check lost its `go build` step — the check must exercise the CURRENT checkout's binary")
	}
	if installIdx < 0 {
		t.Fatalf("render-check lost its `install --target` step — the check must install into a scratch dir")
	}
	if installIdx < buildIdx {
		t.Errorf("render-check installs (step %d) before building (step %d)", installIdx, buildIdx)
	}

	// The DELIBERATE pair: a continue-on-error dry-run step (id X, running
	// doctor) + a later step that fails the workflow when steps.X.outcome !=
	// 'success'. The pair — wired together — is what makes drift fatal while
	// still capturing output for the PR comment.
	dryrunIdx := -1
	dryrunID := ""
	for i, s := range steps {
		if !truthy(s["continue-on-error"]) {
			continue
		}
		id, _ := s["id"].(string)
		if id != "" && strings.Contains(stepRun(s), "doctor") {
			dryrunIdx, dryrunID = i, id
		}
	}
	if dryrunIdx < 0 {
		t.Fatalf("render-check lost its continue-on-error doctor dry-run step (with an id) — the deliberate capture-then-fail pattern is gone")
	}
	failIdx := -1
	for i, s := range steps {
		if i <= dryrunIdx {
			continue
		}
		sif, _ := s["if"].(string)
		if strings.Contains(sif, "steps."+dryrunID+".outcome") &&
			strings.Contains(sif, "success") &&
			strings.Contains(sif, "!=") &&
			strings.Contains(stepRun(s), "exit 1") {
			failIdx = i
		}
	}
	if failIdx < 0 {
		t.Fatalf("render-check lost its Fail-on-drift step asserting steps.%s.outcome != 'success' with a non-zero exit — drift would no longer fail the workflow", dryrunID)
	}
}

// ---------------------------------------------------------------------------
// release.yml — the DEFER/release-readiness evaluator runs BEFORE publication
// ---------------------------------------------------------------------------

func TestCIContract_ReleaseReadinessRunsBeforePublication(t *testing.T) {
	wf := loadWorkflow(t, "release.yml")
	jobs := workflowJobs(t, wf)
	rel := jobDef(t, jobs, "release")

	if truthy(rel["continue-on-error"]) {
		t.Errorf("release job sets continue-on-error — evaluator refusals would not stop publication")
	}

	steps := jobSteps(t, rel)
	evalIdx, pubIdx := -1, -1
	for i, s := range steps {
		if truthy(s["continue-on-error"]) {
			t.Errorf("release step %d (%q) sets continue-on-error — no release-lane step may swallow a refusal", i, s["name"])
		}
		if strings.Contains(stepRun(s), "check-defer-triggers.mjs") && strings.Contains(stepRun(s), "--mode=release") {
			evalIdx = i
		}
		if strings.Contains(stepUses(s), "goreleaser") {
			pubIdx = i
		}
	}
	if evalIdx < 0 {
		t.Fatalf("release.yml lost its DEFER-manifest evaluator step (check-defer-triggers.mjs --mode=release)")
	}
	if pubIdx < 0 {
		t.Fatalf("release.yml lost its GoReleaser publication step")
	}
	if evalIdx > pubIdx {
		t.Fatalf("DEFER evaluator (step %d) runs AFTER GoReleaser publication (step %d) — publication must be refused BEFORE artifacts go out", evalIdx, pubIdx)
	}
}
