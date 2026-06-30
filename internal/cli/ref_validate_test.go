package cli

// Phase 4 capability-installer post-render validation tests.
//
// These pin the contract of validateRenderedRefs (internal/cli/ref_validate.go),
// the defense-in-depth assertion layer that runs AFTER permconfig.Emit inside
// renderSeamStaging. By the time it runs, Phase 3's present-agent filter has
// already pruned optional task edges, so any surviving dangling reference is a
// HARD inconsistency and MUST fail closed.
//
// Coverage:
//   - No-op: a fully-consistent config (every task target rendered, every prompt
//     file present) validates clean (nil error).
//   - Dangling permission.task: an orchestrator whose task allowlist names an
//     agent that did not render -> fail-closed error naming the source agent,
//     the ref kind, and the missing target.
//   - Dangling prompt: an agent whose prompt references an .opencode/agents/<x>.md
//     file that conditional rendering removed -> fail-closed error.
//   - "*" wildcard is never flagged (it is a decision, not a reference).
//
// The end-to-end no-op proof (a real seam install/doctor stays HEALTHY because
// the validation passes on the dogfood render) is already covered by
// TestSeamRender_BackwardCompatDefaultRendersAllCapabilities and
// TestSeamDoctor_HealthyAfterInstall in seam_cli_test.go / capability_render_test.go,
// since the validation is invoked inside renderSeamStaging.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeAgentPromptFile seeds <staging>/.opencode/agents/<name>.md so a prompt
// reference resolves. Returns the staged path.
func writeAgentPromptFile(t *testing.T, staging, name string) string {
	t.Helper()
	p := filepath.Join(staging, ".opencode", "agents", name+".md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir agent prompt dir: %v", err)
	}
	if err := os.WriteFile(p, []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatalf("write agent prompt file %s: %v", name, err)
	}
	return p
}

// TestValidateRenderedRefs_NoopWhenConsistent is the headline no-op case: when
// every task target is a rendered agent and every prompt file is present, the
// validation returns nil (the dogfood render today). This is the contract that
// keeps the Phase 4 assertion layer transparent to a healthy render.
func TestValidateRenderedRefs_NoopWhenConsistent(t *testing.T) {
	staging := t.TempDir()
	// Seed the prompt files referenced below so both resolve.
	writeAgentPromptFile(t, staging, "build")
	writeAgentPromptFile(t, staging, "committer")
	const cfg = `{
  "agent": {
    "build": {
      "prompt": "{file:.opencode/agents/build.md}",
      "permission": { "task": { "*": "deny", "committer": "allow" } }
    },
    "committer": {
      "prompt": "{file:.opencode/agents/committer.md}",
      "permission": { "task": { "*": "deny" } }
    }
  }
}`
	if err := validateRenderedRefs(staging, []byte(cfg)); err != nil {
		t.Fatalf("a consistent config must validate clean (no-op); got: %v", err)
	}
}

// TestValidateRenderedRefs_DanglingTaskRefFailsClosed proves a hard reference —
// an orchestrator's permission.task naming an agent that did not render — fails
// closed with a message naming the source agent, the ref kind, and the missing
// target. This is the headline failure mode (a): a capability manifest declaring
// a hard dep whose agent cluster did not fully render, leaving an orchestrator
// allow entry pointing at a delegate that is not there.
func TestValidateRenderedRefs_DanglingTaskRefFailsClosed(t *testing.T) {
	staging := t.TempDir()
	// build's prompt resolves (so ONLY the task ref dangles); committer is
	// referenced in build's task allowlist but did NOT render.
	writeAgentPromptFile(t, staging, "build")
	const cfg = `{
  "agent": {
    "build": {
      "prompt": "{file:.opencode/agents/build.md}",
      "permission": { "task": { "*": "deny", "committer": "allow" } }
    }
  }
}`
	err := validateRenderedRefs(staging, []byte(cfg))
	if err == nil {
		t.Fatal("expected fail-closed error for dangling task ref (build -> absent committer), got nil")
	}
	for _, want := range []string{"committer", "build", "permission.task"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q; got: %v", want, err)
		}
	}
}

// TestValidateRenderedRefs_DanglingPromptFailsClosed proves the second hard
// reference failure mode (b): an agent whose prompt references an
// .opencode/agents/<x>.md file that conditional rendering removed fails closed.
// build is rendered but its prompt target is absent from the staging tree.
func TestValidateRenderedRefs_DanglingPromptFailsClosed(t *testing.T) {
	staging := t.TempDir()
	// Deliberately do NOT seed build.md — the prompt target is missing.
	const cfg = `{
  "agent": {
    "build": {
      "prompt": "{file:.opencode/agents/build.md}",
      "permission": { "task": { "*": "deny" } }
    }
  }
}`
	err := validateRenderedRefs(staging, []byte(cfg))
	if err == nil {
		t.Fatal("expected fail-closed error for dangling prompt ref, got nil")
	}
	for _, want := range []string{".opencode/agents/build.md", "build", "prompt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q; got: %v", want, err)
		}
	}
}

// TestValidateRenderedRefs_WildcardNeverFlagged confirms the "*" task wildcard
// (a decision applied to all delegates, not a reference to a named agent) is
// never treated as dangling even when no agent named "*" renders.
func TestValidateRenderedRefs_WildcardNeverFlagged(t *testing.T) {
	staging := t.TempDir()
	writeAgentPromptFile(t, staging, "build")
	const cfg = `{
  "agent": {
    "build": {
      "prompt": "{file:.opencode/agents/build.md}",
      "permission": { "task": { "*": "deny" } }
    }
  }
}`
	if err := validateRenderedRefs(staging, []byte(cfg)); err != nil {
		t.Errorf("\"*\" wildcard must never be flagged as a dangling ref; got: %v", err)
	}
}

// TestValidateRenderedRefs_ReportsAllDanglingRefs confirms the validator
// surfaces EVERY inconsistency in one pass (not just the first), so a single
// failed render gives the operator the complete repair list. Two dangling task
// refs + one dangling prompt ref must all appear in the message.
func TestValidateRenderedRefs_ReportsAllDanglingRefs(t *testing.T) {
	staging := t.TempDir()
	// build's prompt resolves; its task refs to "ghost" and "phantom" dangle;
	// coordination's prompt target is missing.
	writeAgentPromptFile(t, staging, "build")
	const cfg = `{
  "agent": {
    "build": {
      "prompt": "{file:.opencode/agents/build.md}",
      "permission": { "task": { "*": "deny", "ghost": "allow", "phantom": "allow" } }
    },
    "coordination": {
      "prompt": "{file:.opencode/agents/coordination.md}",
      "permission": { "task": { "*": "deny" } }
    }
  }
}`
	err := validateRenderedRefs(staging, []byte(cfg))
	if err == nil {
		t.Fatal("expected fail-closed error listing multiple dangling refs, got nil")
	}
	// All three dangling refs must appear in the single error message.
	for _, want := range []string{"ghost", "phantom", ".opencode/agents/coordination.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name dangling ref %q; got: %v", want, err)
		}
	}
	if !strings.Contains(err.Error(), "3 dangling") {
		t.Errorf("error must report the count of dangling refs; got: %v", err)
	}
}

// TestValidateRenderedRefs_DanglingPromptUnderNonAgentsPathFailsClosed pins the
// WIDENED prompt contract (ORCH-F1): a "{file:<path>}" prompt reference must
// resolve regardless of path — not only under .opencode/agents/. A config whose
// prompt points at a non-.opencode/agents/ path that does NOT resolve (here
// .opencode/prompts/missing.md) must fail closed. Before the widening the
// validator silently skipped every non-.opencode/agents/ prompt ref, so a
// dangling ref like this passed validation despite the declared fail-closed
// semantics.
func TestValidateRenderedRefs_DanglingPromptUnderNonAgentsPathFailsClosed(t *testing.T) {
	staging := t.TempDir()
	// Deliberately do NOT seed .opencode/prompts/missing.md — the prompt target
	// is a non-.opencode/agents/ path that does not resolve.
	const cfg = `{
  "agent": {
    "build": {
      "prompt": "{file:.opencode/prompts/missing.md}",
      "permission": { "task": { "*": "deny" } }
    }
  }
}`
	err := validateRenderedRefs(staging, []byte(cfg))
	if err == nil {
		t.Fatal("expected fail-closed error for dangling non-.opencode/agents/ prompt ref, got nil")
	}
	for _, want := range []string{".opencode/prompts/missing.md", "build", "prompt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error must name %q; got: %v", want, err)
		}
	}
}
