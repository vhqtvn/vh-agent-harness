package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// denyGuard is a Pipeline Guard using the existing slice-3 vocabulary:
// deny-only, verdict via error, identity recorded in the result.
type denyGuard struct {
	pattern string
}

func (d denyGuard) Name() string { return "shell-danger-scanner" }

func (d denyGuard) Check(call session.ToolCall) error {
	if call.Name != Name {
		return nil
	}
	var a struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(call.Args, &a); err != nil {
		return fmt.Errorf("unparseable args: %v", err)
	}
	if strings.Contains(a.Command, d.pattern) {
		return fmt.Errorf("command contains dangerous pattern %q", d.pattern)
	}
	return nil
}

// newPipeline builds a real tools.Pipeline with run_shell registered
// (default config) — the same lattice wave B will compose.
func newPipeline(t *testing.T, cfg Config) (*tools.Pipeline, tools.ToolDefinition) {
	t.Helper()
	def := Definition(cfg)
	p := tools.NewPipeline()
	if err := p.Register(def); err != nil {
		t.Fatalf("register run_shell: %v", err)
	}
	return p, def
}

// TestPipelineGuardDeniesAndNeverRuns: a deny-guard (existing
// vocabulary) blocks a dangerous-looking command; the result carries
// the typed denial metadata (Denied/DeniedBy/DenyReason + isError) and
// — the load-bearing part — the process NEVER ran (no side-effect
// marker file, pre-execution denial).
func TestPipelineGuardDeniesAndNeverRuns(t *testing.T) {
	p, _ := newPipeline(t, Config{})
	p.AddGuard(denyGuard{pattern: "rm -rf"})

	marker := filepath.Join(t.TempDir(), "MUST-NOT-EXIST")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	call := session.ToolCall{
		ID:   "call-deny-1",
		Name: Name,
		Args: json.RawMessage(fmt.Sprintf(`{"command":"rm -rf / && touch %q"}`, marker)),
	}
	res := p.Execute(ctx, call)

	if !res.IsError {
		t.Fatalf("denied result must be isError: %+v", res)
	}
	if !res.Denied || res.DeniedBy != "shell-danger-scanner" || res.DenyReason == "" {
		t.Fatalf("typed denial metadata missing: %+v", res)
	}
	if !strings.Contains(res.Content, "denied by guard shell-danger-scanner") {
		t.Fatalf("content = %q, want guard denial provenance", res.Content)
	}
	if res.TimedOut {
		t.Fatalf("denial must not classify as timeout: %+v", res)
	}
	if _, err := statExists(marker); err == nil {
		t.Fatalf("denied command executed anyway (marker %s exists)", marker)
	}
}

// TestPipelineExecutesStructuredOutcome: through the REAL pipeline, a
// clean run returns the JSON outcome as content, isError=false.
func TestPipelineExecutesStructuredOutcome(t *testing.T) {
	p, _ := newPipeline(t, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := p.Execute(ctx, session.ToolCall{
		ID:   "call-ok-1",
		Name: Name,
		Args: json.RawMessage(`{"command":"printf pipeline-hello"}`),
	})
	if res.IsError {
		t.Fatalf("clean run became an error: %+v (%s)", res, res.Content)
	}
	var o Outcome
	if err := json.Unmarshal([]byte(res.Content), &o); err != nil {
		t.Fatalf("content is not the outcome JSON: %v (%s)", err, res.Content)
	}
	if o.Cause != CauseExit || o.ExitCode != 0 || o.Stdout != "pipeline-hello" {
		t.Fatalf("outcome = %+v", o)
	}
}

// TestPipelineIsErrorNormalizationOnTimeout: a per-call timeout
// surfaces as an isError result (the body's typed TimeoutError text),
// distinct from the pipeline-level Result.TimedOut (reserved for the
// def-level cap).
func TestPipelineIsErrorNormalizationOnTimeout(t *testing.T) {
	p, _ := newPipeline(t, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := p.Execute(ctx, session.ToolCall{
		ID:   "call-timeout-1",
		Name: Name,
		Args: json.RawMessage(`{"command":"sleep 5","timeout_ms":300}`),
	})
	if !res.IsError {
		t.Fatalf("timeout must normalize to isError: %+v", res)
	}
	if res.TimedOut {
		t.Fatalf("body-level timeout must not set the pipeline-level TimedOut fact (cap-only): %+v", res)
	}
	if !strings.Contains(res.Content, "timed out after 300ms") {
		t.Fatalf("content = %q, want the typed timeout fact", res.Content)
	}
	if strings.Contains(res.Content, "sleep 5 failed") && !strings.Contains(res.Content, "timed out") {
		t.Fatalf("timeout conflated with ordinary error text: %q", res.Content)
	}
}

// TestPipelineNonZeroExitIsNotError: exit 3 is a normal structured
// outcome (isError=false) — the dsh orthogonal-facts rule.
func TestPipelineNonZeroExitIsNotError(t *testing.T) {
	p, _ := newPipeline(t, Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := p.Execute(ctx, session.ToolCall{
		ID:   "call-exit3-1",
		Name: Name,
		Args: json.RawMessage(`{"command":"exit 3"}`),
	})
	if res.IsError {
		t.Fatalf("exit 3 misclassified as tool error: %+v (%s)", res, res.Content)
	}
	var o Outcome
	if err := json.Unmarshal([]byte(res.Content), &o); err != nil || o.ExitCode != 3 || o.Cause != CauseExit {
		t.Fatalf("outcome = %+v err=%v content=%s", o, err, res.Content)
	}
}

// TestDefinitionShape: the binding design facts — name, barrier
// semantics, cap backstop, schema.
func TestDefinitionShape(t *testing.T) {
	def := Definition(Config{})
	if def.Name != Name {
		t.Fatalf("name = %q", def.Name)
	}
	if def.IsConcurrencySafe {
		t.Fatalf("run_shell must NOT be concurrency-safe (exclusive barrier)")
	}
	if def.TimeoutMs != DefaultMaxTimeoutMs {
		t.Fatalf("TimeoutMs = %d, want the cap backstop %d", def.TimeoutMs, DefaultMaxTimeoutMs)
	}
	if def.Execute == nil || def.Parameters == nil || def.Description == "" {
		t.Fatalf("incomplete definition: %+v", def)
	}
	var schema map[string]any
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("parameters not valid JSON: %v", err)
	}
	if !strings.Contains(def.Description, "NO confinement") {
		t.Fatalf("description must state the no-sandbox default loudly: %q", def.Description)
	}
}

// TestSpawnErrorThroughPipeline: a spawn-class failure (missing shell)
// normalizes to isError through the real pipeline.
func TestSpawnErrorThroughPipeline(t *testing.T) {
	p, _ := newPipeline(t, Config{ShellPath: "/nonexistent/shell-binary-xyz"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res := p.Execute(ctx, session.ToolCall{
		ID:   "call-spawn-1",
		Name: Name,
		Args: json.RawMessage(`{"command":"true"}`),
	})
	if !res.IsError || !strings.Contains(res.Content, "spawn failed") {
		t.Fatalf("spawn error misclassified: %+v (%s)", res, res.Content)
	}
}

func statExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	return false, err
}
