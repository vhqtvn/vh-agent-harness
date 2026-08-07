package cli

// This file is the REAL-corpus crux proof for the flag-gated-composition fix
// (researches/decisions/flag-gated-composition-fix.md). The synthetic tests in
// compose_agents_test.go prove the gating MECHANISM; this file proves the
// ACTUAL embedded AGENTS.core.md template source — with its real section
// boundaries — gates correctly when composed. It mirrors what the renderer +
// compose pipeline do to the live file:
//
//  1. read the embedded AGENTS.core.md from the corpus;
//  2. run SubstituteHarnessTokens over it (the renderer's preserve-as-is copy
//     resolves canonical {{UPPER_TOKEN}} sentinels while leaving Go-template
//     {{ if .features.backlog }} actions literal);
//  3. compose with a profile that turns features.backlog off;
//  4. assert the backlog section is absent and a non-backlog section survives.

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	corpus "github.com/vhqtvn/vh-agent-harness"
	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
	"github.com/vhqtvn/vh-agent-harness/internal/substrate"
)

// readEmbeddedAgentsCore reads the REAL AGENTS.core.md from the embedded
// corpus and applies the same token-substitution pass the renderer's
// preserve-as-is copy does, producing the exact bytes composeAgentsMd would
// read from a real target's .vh-agent-harness/AGENTS.core.md after a render.
func readEmbeddedAgentsCore(t *testing.T) []byte {
	t.Helper()
	sub, err := fs.Sub(corpus.CoreFS, corpus.CoreDir)
	if err != nil {
		t.Fatalf("embed sub: %v", err)
	}
	raw, err := fs.ReadFile(sub, filepath.ToSlash(filepath.Join(runshape.DirName, "AGENTS.core.md")))
	if err != nil {
		t.Fatalf("read embedded AGENTS.core.md: %v", err)
	}
	return substrate.SubstituteHarnessTokens(raw, nil)
}

// TestComposeAgentsMd_RealCorpusBacklogFalse is the load-bearing crux: the
// REAL embedded AGENTS.core.md, composed with features.backlog:false, must
// produce a composed AGENTS.md WITHOUT the backlog section while keeping the
// non-backlog generic core (bundle-safe-disable: only the feature delta drops).
func TestComposeAgentsMd_RealCorpusBacklogFalse(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), string(readEmbeddedAgentsCore(t)))
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	writeProfile(t, dir, "profile: minimal\nfeatures:\n  backlog: false\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	// The backlog section and its subsections must be EXCLUDED.
	for _, needle := range []string{
		"## Backlog tracking rules",
		"### Canonical files",
		"### Conflict discipline (hybrid split-commit)",
		"### Task formatting rules",
	} {
		if strings.Contains(got, needle) {
			t.Errorf("backlog=false must EXCLUDE %q from the REAL corpus; found in composed AGENTS.md", needle)
		}
	}
	// A non-backlog section that precedes the gate must SURVIVE (the fix must
	// not drop shared content — hermes bundle-safe-disable lesson).
	if !strings.Contains(got, "## OpenCode operating model") {
		t.Errorf("backlog=false must KEEP non-backlog section ## OpenCode operating model")
	}
	if !strings.Contains(got, "DOMAIN-MARKER") {
		t.Errorf("backlog=false must keep the mission half")
	}
	// No template action markers may leak into the composed output.
	if strings.Contains(got, "{{ ") || strings.Contains(got, "{{if") {
		t.Errorf("composed AGENTS.md leaked a template action marker:\n%s", got)
	}
}

// TestComposeAgentsMd_RealCorpusBacklogTrue proves the inverse on the REAL
// corpus: features.backlog:true keeps the backlog section intact.
func TestComposeAgentsMd_RealCorpusBacklogTrue(t *testing.T) {
	dir := t.TempDir()
	src := srcDir(t, dir)
	mustWrite(t, filepath.Join(src, "AGENTS.core.md"), string(readEmbeddedAgentsCore(t)))
	mustWrite(t, filepath.Join(src, "AGENTS.mission.md"), "# Mission\nDOMAIN-MARKER\n")
	writeProfile(t, dir, "profile: minimal\nfeatures:\n  backlog: true\n")

	got, err := composeAndRead(t, dir)
	if err != nil {
		t.Fatalf("composeAgentsMd: %v", err)
	}
	if !strings.Contains(got, "## Backlog tracking rules") {
		t.Errorf("backlog=true must KEEP the backlog section from the REAL corpus")
	}
	if !strings.Contains(got, "### Task formatting rules") {
		t.Errorf("backlog=true must KEEP the backlog subsections from the REAL corpus")
	}
	if strings.Contains(got, "{{ ") {
		t.Errorf("composed AGENTS.md leaked a template action marker:\n%s", got)
	}
}
