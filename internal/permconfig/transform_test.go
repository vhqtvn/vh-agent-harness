package permconfig

import (
	"strings"
	"testing"
)

func TestValidateTransformOutput_EmptyPatches(t *testing.T) {
	raw := []byte(`{}`)
	extra, err := ValidateTransformOutput(raw, []string{"build", "committer"})
	if err != nil {
		t.Fatalf("empty patches: unexpected error %v", err)
	}
	if len(extra) != 0 {
		t.Fatalf("empty patches: expected empty map, got %d agents", len(extra))
	}
}

func TestValidateTransformOutput_ValidPatch(t *testing.T) {
	raw := []byte(`{
		"permissionPatches": [
			{ "agent": "build", "bash": [ { "pattern": "./dev.sh *", "decision": "allow" } ] }
		]
	}`)
	extra, err := ValidateTransformOutput(raw, []string{"build", "committer"})
	if err != nil {
		t.Fatalf("valid patch: unexpected error %v", err)
	}
	entries := extra["build"]
	if len(entries) != 1 || entries[0].Pattern != "./dev.sh *" || entries[0].Decision != Allow {
		t.Fatalf("valid patch: got %+v, want [{./dev.sh * allow}]", entries)
	}
}

func TestValidateTransformOutput_UnknownAgent(t *testing.T) {
	raw := []byte(`{
		"permissionPatches": [
			{ "agent": "nonexistent", "bash": [ { "pattern": "x", "decision": "allow" } ] }
		]
	}`)
	_, err := ValidateTransformOutput(raw, []string{"build"})
	if err == nil {
		t.Fatal("unknown agent: expected error")
	}
	if !strings.Contains(err.Error(), "not in the rendered roster") {
		t.Fatalf("unknown agent: error %q does not mention roster", err.Error())
	}
}

func TestValidateTransformOutput_InvalidDecision(t *testing.T) {
	raw := []byte(`{
		"permissionPatches": [
			{ "agent": "build", "bash": [ { "pattern": "x", "decision": "bogus" } ] }
		]
	}`)
	_, err := ValidateTransformOutput(raw, []string{"build"})
	if err == nil {
		t.Fatal("invalid decision: expected error")
	}
}

func TestValidateTransformOutput_EmptyPattern(t *testing.T) {
	raw := []byte(`{
		"permissionPatches": [
			{ "agent": "build", "bash": [ { "pattern": "", "decision": "allow" } ] }
		]
	}`)
	_, err := ValidateTransformOutput(raw, []string{"build"})
	if err == nil {
		t.Fatal("empty pattern: expected error")
	}
}

func TestValidateTransformOutput_DuplicatePattern(t *testing.T) {
	raw := []byte(`{
		"permissionPatches": [
			{ "agent": "build", "bash": [
				{ "pattern": "x", "decision": "allow" },
				{ "pattern": "x", "decision": "deny" }
			] }
		]
	}`)
	_, err := ValidateTransformOutput(raw, []string{"build"})
	if err == nil {
		t.Fatal("duplicate pattern: expected error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate pattern: error %q does not mention duplicate", err.Error())
	}
}

func TestValidateTransformOutput_ProtectedKeyCollision(t *testing.T) {
	for _, pat := range []string{"*", "vh-agent-harness *", "ls *"} {
		raw := []byte(`{"permissionPatches":[{"agent":"build","bash":[{"pattern":"` + pat + `","decision":"allow"}]}]}`)
		_, err := ValidateTransformOutput(raw, []string{"build"})
		if err == nil {
			t.Fatalf("protected key %q: expected error", pat)
		}
		if !strings.Contains(err.Error(), "protected key") {
			t.Fatalf("protected key %q: error %q does not mention protected key", pat, err.Error())
		}
	}
}

func TestValidateTransformOutput_InvalidJSON(t *testing.T) {
	_, err := ValidateTransformOutput([]byte(`not json at all`), []string{"build"})
	if err == nil {
		t.Fatal("invalid JSON: expected error")
	}
}

func TestValidateTransformOutput_AllThreeDecisions(t *testing.T) {
	raw := []byte(`{
		"permissionPatches": [
			{ "agent": "build", "bash": [
				{ "pattern": "allow-pat", "decision": "allow" },
				{ "pattern": "deny-pat", "decision": "deny" },
				{ "pattern": "ask-pat", "decision": "ask" }
			] }
		]
	}`)
	extra, err := ValidateTransformOutput(raw, []string{"build"})
	if err != nil {
		t.Fatalf("three decisions: unexpected error %v", err)
	}
	if len(extra["build"]) != 3 {
		t.Fatalf("three decisions: got %d entries, want 3", len(extra["build"]))
	}
}

// --- LintTransformSource tests ---

func TestLintTransformSource_Clean(t *testing.T) {
	source := []byte(`
// A perfectly fine transform.
export default function transform({ context }) {
	return { permissionPatches: [] };
}
`)
	if err := LintTransformSource(source); err != nil {
		t.Fatalf("clean source: unexpected error %v", err)
	}
}

func TestLintTransformSource_ForbiddenProcessEnv(t *testing.T) {
	source := []byte(`const token = process.env.SECRET;`)
	err := LintTransformSource(source)
	if err == nil {
		t.Fatal("process.env: expected error")
	}
	if !strings.Contains(err.Error(), "process.env") {
		t.Fatalf("process.env: error %q does not mention the token", err.Error())
	}
}

func TestLintTransformSource_ForbiddenRequire(t *testing.T) {
	source := []byte(`const fs = require("fs");`)
	err := LintTransformSource(source)
	if err == nil {
		t.Fatal("require: expected error")
	}
}

func TestLintTransformSource_ForbiddenMathRandom(t *testing.T) {
	source := []byte(`const id = Math.random();`)
	err := LintTransformSource(source)
	if err == nil {
		t.Fatal("Math.random: expected error")
	}
}
