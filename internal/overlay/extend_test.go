package overlay

// Unit tests for the prompt-extension mechanism (Slice 2 of the unified
// extension model). Covers:
//   - parseExtensionSnippet: filename contract edge cases.
//   - InjectSlots: anchor injection, missing-snippet no-op, orphan warn,
//     multi-overlay concat, and the provenance contract (managed body stays
//     byte-identical outside the injected region).
//   - InjectExtensionSnippets: staging-level merge pass (target absent →
//     orphan; target present + anchor → injection written back).
//
// Mirrors the fstest.MapFS style of overlay_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// --- parseExtensionSnippet -------------------------------------------------

// TestParseExtensionSnippet_Contract exercises the <base>.extend.<slot>.<ext>
// filename contract across agent/skill/command shapes and rejection cases.
func TestParseExtensionSnippet_Contract(t *testing.T) {
	cases := []struct {
		name       string
		rel        string
		wantTarget string
		wantSlot   string
		wantOk     bool
	}{
		{
			name:       "agent custom-verbs",
			rel:        "agents/build.extend.custom-verbs.md",
			wantTarget: "agents/build.md",
			wantSlot:   "custom-verbs",
			wantOk:     true,
		},
		{
			name:       "skill anchor",
			rel:        "skills/web-dev-loop/SKILL.extend.notes.md",
			wantTarget: "skills/web-dev-loop/SKILL.md",
			wantSlot:   "notes",
			wantOk:     true,
		},
		{
			name:       "slot with hyphens and dots",
			rel:        "commands/frontend.extend.extra.flags.md",
			wantTarget: "commands/frontend.md",
			wantSlot:   "extra.flags",
			wantOk:     true,
		},
		{rel: "agents/build.md", wantOk: false},           // no infix
		{rel: "opencode-append.jsonc", wantOk: false},     // merge-content
		{rel: "callable-graph-snippet.md", wantOk: false}, // merge-content
		{rel: "permission-pack.jsonc", wantOk: false},     // merge-content
		{rel: ".extend.slot.md", wantOk: false},           // empty unitBase
		{rel: "agents/build.extend..md", wantOk: false},   // empty slot text -> rejected
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "" {
				t.Run(c.rel, func(t *testing.T) {
					checkParse(t, c.rel, c.wantTarget, c.wantSlot, c.wantOk)
				})
				return
			}
			checkParse(t, c.rel, c.wantTarget, c.wantSlot, c.wantOk)
		})
	}
}

func checkParse(t *testing.T, rel, wantTarget, wantSlot string, wantOk bool) {
	t.Helper()
	gotTarget, gotSlot, gotOk := parseExtensionSnippet(rel)
	if gotOk != wantOk {
		t.Fatalf("parseExtensionSnippet(%q): ok=%v want %v", rel, gotOk, wantOk)
	}
	if !wantOk {
		return
	}
	if gotTarget != wantTarget {
		t.Errorf("parseExtensionSnippet(%q): target=%q want %q", rel, gotTarget, wantTarget)
	}
	if gotSlot != wantSlot {
		t.Errorf("parseExtensionSnippet(%q): slot=%q want %q", rel, gotSlot, wantSlot)
	}
}

// --- InjectSlots (pure) ----------------------------------------------------

const (
	testAnchor  = "<!-- HARNESS:EXTEND custom-verbs -->"
	managedHead = "managed-head-line\n"
	managedTail = "managed-tail-line\n"
)

// managedBodyWithAnchor builds a managed body that carries the anchor between
// head and tail so tests can assert the head/tail are byte-identical after
// injection (the provenance contract: only the anchor region changes).
func managedBodyWithAnchor(anchor string) string {
	return managedHead + anchor + "\n" + managedTail
}

// TestInjectSlots_AtAnchor injects a snippet at its matching anchor and asserts
// the body lands on the line(s) immediately after the anchor line.
func TestInjectSlots_AtAnchor(t *testing.T) {
	body := managedBodyWithAnchor(testAnchor)
	snippet := "- vh-agent-harness ssh-trust <host> : trust a VPS host key."
	injected, orphans := InjectSlots(body, []SlotInjection{
		{TargetRel: "agents/build.md", Slot: "custom-verbs", Body: snippet},
	})
	if len(orphans) != 0 {
		t.Fatalf("orphans: want 0, got %v", orphans)
	}
	want := managedHead + testAnchor + "\n" + snippet + "\n" + managedTail
	if injected != want {
		t.Errorf("injected body mismatch:\nwant=%q\ngot =%q", want, injected)
	}
}

// TestInjectSlots_MissingSnippetIsNoOp confirms an anchor with no contributing
// snippet stays as the empty marker (NOT an orphan), and the body is unchanged.
func TestInjectSlots_MissingSnippetIsNoOp(t *testing.T) {
	body := managedBodyWithAnchor(testAnchor)
	injected, orphans := InjectSlots(body, nil)
	if len(orphans) != 0 {
		t.Fatalf("orphans: want 0, got %v", orphans)
	}
	if injected != body {
		t.Errorf("missing snippet should be a no-op:\nwant=%q\ngot =%q", body, injected)
	}
}

// TestInjectSlots_OrphanSnippetIsWarned confirms a snippet whose slot has NO
// matching anchor is returned in orphans (warned, never silently dropped) and
// the body is unchanged.
func TestInjectSlots_OrphanSnippetIsWarned(t *testing.T) {
	body := managedBodyWithAnchor(testAnchor)
	injected, orphans := InjectSlots(body, []SlotInjection{
		{Slot: "missing-slot", Body: "orphan-body"},
	})
	if len(orphans) != 1 || orphans[0] != "missing-slot" {
		t.Fatalf("orphans: want [missing-slot], got %v", orphans)
	}
	if injected != body {
		t.Errorf("orphan snippet must not mutate body:\nwant=%q\ngot =%q", body, injected)
	}
}

// TestInjectSlots_EmptyBodyIsNoOpNotOrphan confirms a slot whose Body is
// empty/whitespace is treated as "no snippet" (a no-op), NOT an orphan.
func TestInjectSlots_EmptyBodyIsNoOpNotOrphan(t *testing.T) {
	body := managedBodyWithAnchor(testAnchor)
	for _, b := range []string{"", "   ", "\n\n"} {
		injected, orphans := InjectSlots(body, []SlotInjection{
			{Slot: "custom-verbs", Body: b},
		})
		if len(orphans) != 0 {
			t.Errorf("empty body must not be orphan: body=%q orphans=%v", b, orphans)
		}
		if injected != body {
			t.Errorf("empty body must be a no-op: body=%q\nwant=%q\ngot =%q", b, body, injected)
		}
	}
}

// TestInjectSlots_MultiOverlayConcat confirms two snippets for the same
// (target, slot) are both placed at the anchor in slot-list order. The pure
// InjectSlots places them as consecutive lines; the staging-level pass
// (InjectExtensionSnippets) is where pack-order concatenation joins bodies with
// a blank line separator — that variant is covered by
// TestInjectExtensionSnippets_MultiPackConcat.
func TestInjectSlots_MultiOverlayConcat(t *testing.T) {
	body := managedBodyWithAnchor(testAnchor)
	injected, orphans := InjectSlots(body, []SlotInjection{
		{Slot: "custom-verbs", Body: "PACK-A-BODY", Packs: []string{"a"}},
		{Slot: "custom-verbs", Body: "PACK-B-BODY", Packs: []string{"b"}},
	})
	if len(orphans) != 0 {
		t.Fatalf("orphans: want 0, got %v", orphans)
	}
	want := managedHead + testAnchor + "\n" + "PACK-A-BODY" + "\n" + "PACK-B-BODY" + "\n" + managedTail
	if injected != want {
		t.Errorf("concat mismatch:\nwant=%q\ngot =%q", want, injected)
	}
}

// TestInjectSlots_ManagedRegionStaysByteIdentical asserts the provenance
// contract: the managed body OUTSIDE the injected region is byte-identical
// (managedHead + anchor line + managedTail unchanged); only the snippet lines
// are added. This is the "managed stays platform_managed" guarantee in
// practice — the snippet never edits the managed prose, it only inserts.
func TestInjectSlots_ManagedRegionStaysByteIdentical(t *testing.T) {
	body := managedBodyWithAnchor(testAnchor)
	injected, _ := InjectSlots(body, []SlotInjection{
		{Slot: "custom-verbs", Body: "SNIPPET"},
	})
	lines := splitLines(injected)
	// head line is line 0, anchor is line 1, snippet is line 2, tail is the LAST line.
	if lines[0] != "managed-head-line" {
		t.Errorf("head mutated: %q", lines[0])
	}
	if lines[1] != testAnchor {
		t.Errorf("anchor line mutated: %q", lines[1])
	}
	if lines[2] != "SNIPPET" {
		t.Errorf("snippet not at anchor+1: %q", lines[2])
	}
	if lines[len(lines)-1] != "managed-tail-line" {
		t.Errorf("tail mutated: %q", lines[len(lines)-1])
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// --- InjectExtensionSnippets (staging-level) -------------------------------

// stagingTarget writes a managed target file into staging/.opencode/<targetRel>
// and returns its full path. Mirrors how the core renderer lays down files.
func stagingTarget(t *testing.T, staging, targetRel, body string) string {
	t.Helper()
	live := opencodePrefix + targetRel
	full := filepath.Join(staging, filepath.FromSlash(live))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	return full
}

// TestInjectExtensionSnippets_InjectsAtAnchor renders a single pack with one
// snippet into a staging tree whose target carries the matching anchor, and
// confirms the snippet is written back at the anchor.
func TestInjectExtensionSnippets_InjectsAtAnchor(t *testing.T) {
	staging := t.TempDir()
	targetBody := managedBodyWithAnchor(testAnchor)
	full := stagingTarget(t, staging, "agents/build.md", targetBody)

	pack := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {
				Data: []byte("- vh-agent-harness ssh-trust <host> : trust a VPS host key."),
			},
		},
	}
	report, err := InjectExtensionSnippets(staging, []*Pack{pack}, nil)
	if err != nil {
		t.Fatalf("InjectExtensionSnippets: %v", err)
	}
	if len(report.Orphans) != 0 {
		t.Fatalf("orphans: want 0, got %+v", report.Orphans)
	}
	if len(report.Injected) != 1 {
		t.Fatalf("injected: want 1, got %+v", report.Injected)
	}
	got, rerr := os.ReadFile(full)
	if rerr != nil {
		t.Fatalf("read back target: %v", rerr)
	}
	want := managedHead + testAnchor + "\n" + "- vh-agent-harness ssh-trust <host> : trust a VPS host key." + "\n" + managedTail
	if string(got) != want {
		t.Errorf("target not injected at anchor:\nwant=%q\ngot =%q", want, string(got))
	}
}

// TestInjectExtensionSnippets_TargetAbsentIsOrphan confirms a snippet whose
// target file is NOT present in staging is an orphan (warned), and no file is
// created.
func TestInjectExtensionSnippets_TargetAbsentIsOrphan(t *testing.T) {
	staging := t.TempDir()
	pack := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {
				Data: []byte("orphan because agents/build.md is not rendered"),
			},
		},
	}
	report, err := InjectExtensionSnippets(staging, []*Pack{pack}, nil)
	if err != nil {
		t.Fatalf("InjectExtensionSnippets: %v", err)
	}
	if len(report.Injected) != 0 {
		t.Fatalf("injected: want 0, got %+v", report.Injected)
	}
	if len(report.Orphans) != 1 {
		t.Fatalf("orphans: want 1, got %+v", report.Orphans)
	}
	// No target file should have been created.
	if _, err := os.Stat(filepath.Join(staging, opencodePrefix, "agents", "build.md")); !os.IsNotExist(err) {
		t.Fatalf("orphan must not create a target file; stat err=%v", err)
	}
}

// TestInjectExtensionSnippets_MultiPackConcat confirms two packs contributing the
// same (target, slot) concatenate in pack-list order in the written-back file.
func TestInjectExtensionSnippets_MultiPackConcat(t *testing.T) {
	staging := t.TempDir()
	targetBody := managedBodyWithAnchor(testAnchor)
	full := stagingTarget(t, staging, "agents/build.md", targetBody)

	packA := &Pack{
		Name: "pack-a",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {Data: []byte("BODY-A")},
		},
	}
	packB := &Pack{
		Name: "pack-b",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {Data: []byte("BODY-B")},
		},
	}
	report, err := InjectExtensionSnippets(staging, []*Pack{packA, packB}, nil)
	if err != nil {
		t.Fatalf("InjectExtensionSnippets: %v", err)
	}
	if len(report.Orphans) != 0 {
		t.Fatalf("orphans: want 0, got %+v", report.Orphans)
	}
	got, _ := os.ReadFile(full)
	want := managedHead + testAnchor + "\n" + "BODY-A" + "\n\n" + "BODY-B" + "\n" + managedTail
	if string(got) != want {
		t.Errorf("multi-pack concat mismatch:\nwant=%q\ngot =%q", want, string(got))
	}
}

// TestInjectExtensionSnippets_NoAnchorIsOrphan confirms a snippet whose target
// IS present but carries no matching anchor is an orphan.
func TestInjectExtensionSnippets_NoAnchorIsOrphan(t *testing.T) {
	staging := t.TempDir()
	full := stagingTarget(t, staging, "agents/build.md", "no anchor here\n")

	pack := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {Data: []byte("body for a missing anchor")},
		},
	}
	report, err := InjectExtensionSnippets(staging, []*Pack{pack}, nil)
	if err != nil {
		t.Fatalf("InjectExtensionSnippets: %v", err)
	}
	if len(report.Injected) != 0 {
		t.Fatalf("injected: want 0, got %+v", report.Injected)
	}
	if len(report.Orphans) != 1 {
		t.Fatalf("orphans: want 1, got %+v", report.Orphans)
	}
	// The target file must be unchanged (no anchor → no injection).
	got, _ := os.ReadFile(full)
	if string(got) != "no anchor here\n" {
		t.Errorf("no-anchor target should be unchanged:\nwant=%q\ngot =%q", "no anchor here\n", string(got))
	}
}

// TestExtensionSnippets_LoadsFromPackFS confirms Pack.ExtensionSnippets walks
// the pack FS and returns snippets sorted, skipping merge-content files.
func TestExtensionSnippets_LoadsFromPackFS(t *testing.T) {
	pack := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {Data: []byte("snippet body")},
			"agents/build.md":                     {Data: []byte("unit body")},
			"opencode-append.jsonc":               {Data: []byte("{}")},
			"skills/x/SKILL.extend.notes.md":      {Data: []byte("skill snippet")},
		},
	}
	snips, err := pack.ExtensionSnippets()
	if err != nil {
		t.Fatalf("ExtensionSnippets: %v", err)
	}
	if len(snips) != 2 {
		t.Fatalf("snippets: want 2, got %d (%+v)", len(snips), snips)
	}
	// Sorted by (TargetRel, Slot): agents/build.md < skills/x/SKILL.md
	if snips[0].TargetRel != "agents/build.md" || snips[0].Slot != "custom-verbs" {
		t.Errorf("snips[0] = %+v", snips[0])
	}
	if snips[1].TargetRel != "skills/x/SKILL.md" || snips[1].Slot != "notes" {
		t.Errorf("snips[1] = %+v", snips[1])
	}
}

// TestInjectExtensionSnippets_ResolvesSnippetTokens confirms InjectExtensionSnippets
// applies the SAME 3-token identity substitution the core renderers apply to each
// snippet BODY before injecting it into the managed target. Without this pass a
// snippet carrying {{PROJECT_NAME}} would land as a literal placeholder inside
// the (already token-free) managed file. The snippet body must resolve; the
// managed body's own tokens are unaffected (core already resolved them at render).
func TestInjectExtensionSnippets_ResolvesSnippetTokens(t *testing.T) {
	staging := t.TempDir()
	targetBody := managedBodyWithAnchor(testAnchor)
	full := stagingTarget(t, staging, "agents/build.md", targetBody)

	pack := &Pack{
		Name: "acme",
		FS: fstest.MapFS{
			"agents/build.extend.custom-verbs.md": {
				Data: []byte("- {{PROJECT_NAME}} runtime verb: trust {{COORDINATOR_DIR}} host (slug={{PROJECT_SLUG}}-dev)"),
			},
		},
	}
	answers := map[string]string{
		"project_name":    "Acme",
		"project_slug":    "acme",
		"coordinator_dir": "acme-coordinator",
	}
	report, err := InjectExtensionSnippets(staging, []*Pack{pack}, answers)
	if err != nil {
		t.Fatalf("InjectExtensionSnippets: %v", err)
	}
	if len(report.Injected) != 1 || len(report.Orphans) != 0 {
		t.Fatalf("injected=%d orphans=%d; report=%+v", len(report.Injected), len(report.Orphans), report)
	}
	got, _ := os.ReadFile(full)
	body := string(got)
	for _, tok := range []string{"{{PROJECT_NAME}}", "{{PROJECT_SLUG}}", "{{COORDINATOR_DIR}}"} {
		if strings.Contains(body, tok) {
			t.Errorf("literal sentinel %q survived into managed file after snippet injection:\n%s", tok, body)
		}
	}
	wantSubstrings := []string{
		"- Acme runtime verb: trust acme-coordinator host (slug=acme-dev)",
	}
	for _, w := range wantSubstrings {
		if !strings.Contains(body, w) {
			t.Errorf("injected snippet missing resolved %q; got:\n%s", w, body)
		}
	}
}
