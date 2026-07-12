package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/permconfig"
)

// miniConfigForTransform is a minimal rendered opencode.jsonc with a "build"
// agent that has a permission.bash block. It is used as the staged data for
// transform-invocation tests.
const miniConfigForTransform = `{
  "permission": {
    "bash": {
      "*": "ask"
    }
  },
  "agent": {
    "build": {
      "permission": {
        "bash": {
          "*": "ask"
        },
        "task": ["committer"],
        "edit": "allow"
      }
    },
    "committer": {
      "permission": {
        "bash": {
          "*": "deny"
        }
      }
    }
  }
}
`

// writeTransform writes a config-transform.mjs into the target's
// .vh-agent-harness/ directory and returns the target path.
func writeTransform(t *testing.T, source string) string {
	t.Helper()
	target := t.TempDir()
	dir := filepath.Join(target, ".vh-agent-harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config-transform.mjs"), []byte(source), 0o644); err != nil {
		t.Fatalf("write transform: %v", err)
	}
	return target
}

// skipIfNoNode skips the test if Node.js is not available on PATH.
func skipIfNoNode(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node not available: %v", err)
	}
}

// TestApplyConfigTransform_NoFileIsNoOp verifies that when no transform file
// exists, the function returns (nil, nil) — the no-op path.
func TestApplyConfigTransform_NoFileIsNoOp(t *testing.T) {
	target := t.TempDir() // no .vh-agent-harness/config-transform.mjs
	roster := []string{"build", "committer"}
	extra, err := applyConfigTransform(target, []byte(miniConfigForTransform), roster, nil, nil)
	if err != nil {
		t.Fatalf("expected no error for absent transform, got: %v", err)
	}
	if extra != nil {
		t.Fatalf("expected nil extra for absent transform, got: %v", extra)
	}
}

// TestApplyConfigTransform_ValidTransform verifies a well-formed transform that
// grants "./dev.sh *": "allow" to the "build" agent produces the correct typed
// patches. Requires Node.js.
func TestApplyConfigTransform_ValidTransform(t *testing.T) {
	skipIfNoNode(t)
	source := `export default function({ context }) {
  return {
    permissionPatches: [
      {
        agent: "build",
        bash: [{ pattern: "./dev.sh *", decision: "allow" }]
      }
    ]
  };
}
`
	target := writeTransform(t, source)
	roster := []string{"build", "committer"}
	extra, err := applyConfigTransform(target, []byte(miniConfigForTransform), roster, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extra["build"]) != 1 {
		t.Fatalf("expected 1 entry for build, got: %v", extra)
	}
	entry := extra["build"][0]
	if entry.Pattern != "./dev.sh *" || entry.Decision != permconfig.Allow {
		t.Fatalf("expected {./dev.sh *, allow}, got: {%s, %s}", entry.Pattern, entry.Decision)
	}
}

// TestApplyConfigTransform_ForbiddenAPIRejected verifies that a transform using
// process.env is rejected by the source lint BEFORE any Node invocation (no Node
// required for this test).
func TestApplyConfigTransform_ForbiddenAPIRejected(t *testing.T) {
	source := `export default function({ context }) {
  const x = process.env.SECRET;
  return { permissionPatches: [] };
}
`
	target := writeTransform(t, source)
	_, err := applyConfigTransform(target, []byte(miniConfigForTransform), []string{"build"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for forbidden host-API (process.env), got nil")
	}
}

// TestApplyConfigTransform_NamedExport verifies the named 'transform' export is
// accepted (alternative to default export). Requires Node.js.
func TestApplyConfigTransform_NamedExport(t *testing.T) {
	skipIfNoNode(t)
	source := `export function transform({ context }) {
  return {
    permissionPatches: [
      {
        agent: "build",
        bash: [{ pattern: "make *", decision: "allow" }]
      }
    ]
  };
}
`
	target := writeTransform(t, source)
	roster := []string{"build", "committer"}
	extra, err := applyConfigTransform(target, []byte(miniConfigForTransform), roster, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extra["build"]) != 1 || extra["build"][0].Pattern != "make *" {
		t.Fatalf("expected {make *} for build, got: %v", extra)
	}
}

// TestApplyConfigTransform_UnknownAgentFails verifies that a transform targeting
// an agent not in the roster is rejected. Requires Node.js.
func TestApplyConfigTransform_UnknownAgentFails(t *testing.T) {
	skipIfNoNode(t)
	source := `export default function({ context }) {
  return {
    permissionPatches: [
      { agent: "nonexistent", bash: [{ pattern: "x", decision: "allow" }] }
    ]
  };
}
`
	target := writeTransform(t, source)
	roster := []string{"build", "committer"}
	_, err := applyConfigTransform(target, []byte(miniConfigForTransform), roster, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown agent, got nil")
	}
}

// TestApplyConfigTransform_NoOutputIsNoOp verifies that a transform returning
// empty patches (or no patches key) is a valid no-op. Requires Node.js.
func TestApplyConfigTransform_NoOutputIsNoOp(t *testing.T) {
	skipIfNoNode(t)
	source := `export default function({ context }) {
  return { permissionPatches: [] };
}
`
	target := writeTransform(t, source)
	roster := []string{"build", "committer"}
	extra, err := applyConfigTransform(target, []byte(miniConfigForTransform), roster, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(extra) != 0 {
		t.Fatalf("expected empty patches, got: %v", extra)
	}
}

// TestApplyConfigTransform_DocumentedArgShape is the arg-shape pin. It writes a
// transform that READS context.agents, context.packs, and context.features and
// throws (causing Node to exit non-zero) if any of them is missing or wrong.
// This is the test that catches the double-wrapped-context bug: if the runner
// invokes fn({ context: ctx }) instead of fn(ctx), the project transform
// receives { context: { context: { agents, packs, features } } } and
// context.agents is undefined → the transform throws → applyConfigTransform
// returns an error → this test fails loudly. Requires Node.js.
func TestApplyConfigTransform_DocumentedArgShape(t *testing.T) {
	skipIfNoNode(t)
	source := `export default function({ context }) {
  // Pin the documented arg shape: context.{agents,packs,features} at the TOP
  // level of the sole argument. A double-wrapped arg ({context:{context:{...}}})
  // makes context.agents undefined here and the throw below fires.
  if (!context || !Array.isArray(context.agents) || !Array.isArray(context.packs) || typeof context.features !== "object") {
    throw new Error("transform arg shape wrong: expected {context:{agents,packs,features}}, got: " + JSON.stringify({ context }));
  }
  if (!context.agents.includes("build")) {
    throw new Error("context.agents missing 'build': " + JSON.stringify(context.agents));
  }
  if (!context.packs.includes("my-pack")) {
    throw new Error("context.packs missing 'my-pack': " + JSON.stringify(context.packs));
  }
  if (context.features.backlog !== "true") {
    throw new Error("context.features.backlog !== 'true': " + JSON.stringify(context.features));
  }
  return {
    permissionPatches: [
      {
        agent: "build",
        bash: [{ pattern: "./pin-shape.sh *", decision: "allow" }]
      }
    ]
  };
}
`
	target := writeTransform(t, source)
	roster := []string{"build", "committer"}
	answers := map[string]string{"features.backlog": "true"}
	extra, err := applyConfigTransform(target, []byte(miniConfigForTransform), roster, []string{"my-pack"}, answers)
	if err != nil {
		t.Fatalf("transform did not receive the documented arg shape (or failed): %v", err)
	}
	if len(extra["build"]) != 1 {
		t.Fatalf("expected 1 entry for build when arg shape is correct, got: %v", extra)
	}
	entry := extra["build"][0]
	if entry.Pattern != "./pin-shape.sh *" || entry.Decision != permconfig.Allow {
		t.Fatalf("expected {./pin-shape.sh *, allow}, got: {%s, %s}", entry.Pattern, entry.Decision)
	}
}
