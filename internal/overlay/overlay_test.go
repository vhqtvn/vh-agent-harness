package overlay

// This file hardens the Slice-4 overlay pack mechanism with dedicated unit
// tests. Until now the overlay package (Pack API + MergeJSONC) was exercised
// only indirectly via the cli seam tests and the spike toys; these tests pin
// its contract directly so a regression in pack listing/opening, unit
// rendering, the JSONC deep-merge edge cases, or the callable-graph append is
// caught at the package level rather than only through an end-to-end seam run.
//
// PACK FIXTURE POLICY (2026-06-25 pre-publish clearance, updated 2026-07-01):
// the harness ships ONE embedded overlay pack — `release` (Phase-3
// capability-installer overlay-integration reference implementation, the first
// shipped pack). web-overlay was relocated to a non-shipped adoption reference
// under docs/adoption-examples/web/, so it is NOT a shipped pack. KnownPacks
// therefore returns ["release"]. To keep exercising the Pack API + merge/render
// contract against a richer shape than the single shipped pack carries, every
// pack-touching test below builds a SYNTHETIC pack from testing/fstest.MapFS
// (mirroring the on-disk layout a real pack would ship) and constructs a *Pack
// directly. The merge/render/lineage logic under test is identical whether the
// fs.FS came from an embed or a MapFS.

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	corpus "github.com/vhqtvn/vh-agent-harness"
)

// knownPackNames is the sorted list of overlay packs shipped under
// templates/overlays. As of the Phase-3 capability-installer overlay
// integration (2026-07-01) the harness ships the `release` pack as its first
// embedded overlay pack, so KnownPacks returns ["release"]. web-overlay remains
// relocated to docs/adoption-examples/web/ and is NOT a shipped pack, so it is
// deliberately absent here. (See TestKnownPacks_ShipsReleasePack for the
// live assertion that pins this fixture to reality.)
var knownPackNames = []string{"release"}

// synthWebStyleFS builds an in-memory fs.FS that mirrors the on-disk layout the
// real web-overlay pack shipped: agent/command/skill UNIT files plus the three
// merge-content/catalog files at the pack root (opencode-append.jsonc,
// callable-graph-snippet.md, permission-pack.jsonc). The opencode-append carries
// the canonical overlay-merge shape (new web-builder/browser-qa agent blocks +
// task-allow injections into the existing build/coordination/project-coordinator
// blocks) so the merge tests stay meaningful against synthetic bytes.
func synthWebStyleFS() fstest.MapFS {
	const appendBody = `{
  "agent": {
    "web-builder": {
      "description": "Implements frontend slices plus browser-smoke wiring",
      "mode": "subagent",
      "prompt": "{file:.opencode/agents/web-builder.md}"
    },
    "browser-qa": {
      "description": "Read-only browser QA specialist",
      "mode": "subagent"
    },
    "build": {"permission": {"task": {"browser-qa": "allow", "web-builder": "allow"}}},
    "coordination": {"permission": {"task": {"browser-qa": "allow", "web-builder": "allow"}}},
    "project-coordinator": {"permission": {"task": {"browser-qa": "allow", "web-builder": "allow"}}}
  }
}`
	const snippetBody = "<!-- synthetic web-style pack (ownership: overlay_extension) -->\n" +
		"## web-overlay specialists\n" +
		"- **browser-qa** (read-only): Playwright traces.\n" +
		"- **web-builder** (editable): frontend slices.\n"
	const permBody = `{
  "web-builder": {"description": "web-builder permission contribution"}
}`
	return fstest.MapFS{
		"agents/browser-qa.md":         &fstest.MapFile{Data: []byte("# browser-qa agent\n")},
		"agents/web-builder.md":        &fstest.MapFile{Data: []byte("# web-builder agent\n")},
		"commands/browser-view.md":     &fstest.MapFile{Data: []byte("# browser-view command\n")},
		"commands/frontend.md":         &fstest.MapFile{Data: []byte("# frontend command\n")},
		"commands/web-smoke.md":        &fstest.MapFile{Data: []byte("# web-smoke command\n")},
		"commands/web-stack-up.md":     &fstest.MapFile{Data: []byte("# web-stack-up command\n")},
		"skills/web-dev-loop/SKILL.md": &fstest.MapFile{Data: []byte("# web-dev-loop skill\n")},
		"skills/web-fixtures/SKILL.md": &fstest.MapFile{Data: []byte("# web-fixtures skill\n")},
		appendFileName:                 &fstest.MapFile{Data: []byte(appendBody)},
		snippetFileName:                &fstest.MapFile{Data: []byte(snippetBody)},
		permissionPackFileName:         &fstest.MapFile{Data: []byte(permBody)},
	}
}

// newSynthPack returns a *Pack backed by synthWebStyleFS, ready for RenderUnits
// / MergeAppend / AppendCallableGraph / MaterializePermissionPack tests.
func newSynthPack(name string) *Pack {
	return &Pack{Name: name, FS: synthWebStyleFS()}
}

// decodeJSON unmarshals b into a generic map, failing the test on a parse error.
func decodeJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode merged output: %v\nbytes: %s", err, b)
	}
	return m
}

// asMap asserts v is a JSON object (map[string]any) and returns it.
func asMap(t *testing.T, v any, path string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: want JSON object, got %T (%v)", path, v, v)
	}
	return m
}

// --- OpenPackFor (project-local resolution) --------------------------------

// TestOpenPackFor_ProjectLocal confirms a pack shipped under the project's
// .vh-agent-harness/overlays/<name>/ is resolved (and shadows the embedded FS,
// which ships no packs anyway). This is the seam that keeps the binary
// domain-free: projects supply their own overlays.
func TestOpenPackFor_ProjectLocal(t *testing.T) {
	target := t.TempDir()
	packDir := filepath.Join(target, filepath.FromSlash(ProjectOverlaysSubdir), "acme")
	if err := os.MkdirAll(filepath.Join(packDir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "agents", "deploy.md"), []byte("# deploy\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pack, err := OpenPackFor(target, "acme")
	if err != nil {
		t.Fatalf("OpenPackFor(project-local): %v", err)
	}
	if pack.Name != "acme" {
		t.Errorf("pack name = %q, want acme", pack.Name)
	}
	if _, err := fs.Stat(pack.FS, "agents/deploy.md"); err != nil {
		t.Errorf("project-local pack FS missing agents/deploy.md: %v", err)
	}
}

// TestOpenPackFor_FallsBackToEmbedded confirms that when the project ships no
// such pack, OpenPackFor falls back to the embedded FS (which ships none, so it
// fails closed exactly like OpenPack).
func TestOpenPackFor_FallsBackToEmbedded(t *testing.T) {
	target := t.TempDir() // no .vh-agent-harness/overlays here
	_, err := OpenPackFor(target, "acme")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("OpenPackFor(no project pack, none embedded): want fs.ErrNotExist, got %v", err)
	}
}

// --- KnownPacks ------------------------------------------------------------

// TestKnownPacks_ShipsReleasePack confirms KnownPacks lists the `release` pack —
// the first shipped embedded overlay pack (Phase-3 capability-installer overlay
// integration reference implementation). web-overlay remains relocated to a
// non-shipped adoption reference; release is the sole shipped pack today.
func TestKnownPacks_ShipsReleasePack(t *testing.T) {
	got, err := KnownPacks()
	if err != nil {
		t.Fatalf("KnownPacks: %v", err)
	}
	if len(got) != 1 || got[0] != "release" {
		t.Fatalf("KnownPacks: expected exactly [release], got %v", got)
	}
}

// TestKnownPacks_MatchesEmbeddedDir confirms the pack list agrees with a direct
// readdir of the embedded overlays tree (the embed source of truth). With no
// pack directories shipped, both must be empty.
func TestKnownPacks_MatchesEmbeddedDir(t *testing.T) {
	sub, err := fs.Sub(corpus.OverlaysFS, corpus.OverlaysDir)
	if err != nil {
		t.Fatalf("fs.Sub overlays: %v", err)
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		t.Fatalf("readdir overlays: %v", err)
	}
	var want []string
	for _, e := range entries {
		if e.IsDir() {
			want = append(want, e.Name())
		}
	}
	got, err := KnownPacks()
	if err != nil {
		t.Fatalf("KnownPacks: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("KnownPacks disagrees with embedded dir: got %v, embedded dirs %v", got, want)
	}
}

// --- OpenPack --------------------------------------------------------------

// TestOpenPack_UnknownNamesFailClosed confirms OpenPack fails closed (wrapping
// fs.ErrNotExist) for any name that is not a shipped pack. This is the contract
// a profile that references a non-existent pack hits. (The `release` pack is the
// one shipped name today; every name listed below is deliberately NOT it.)
func TestOpenPack_UnknownNamesFailClosed(t *testing.T) {
	for _, name := range []string{"web-overlay", "anything", "acme", "acme-cockpit"} {
		t.Run(name, func(t *testing.T) {
			_, err := OpenPack(name)
			if err == nil {
				t.Fatalf("OpenPack(%q): want error (not a shipped pack), got nil", name)
			}
			if !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("OpenPack(%q): error should wrap fs.ErrNotExist; got %v", name, err)
			}
		})
	}
}

// TestOpenPack_UnknownPackFailsClosed confirms an unknown pack name returns an
// error that wraps fs.ErrNotExist (the contract a stale profile entry hits).
func TestOpenPack_UnknownPackFailsClosed(t *testing.T) {
	_, err := OpenPack("nope-not-a-pack")
	if err == nil {
		t.Fatal("OpenPack(unknown): want error, got nil")
	}
	if !strings.Contains(err.Error(), "nope-not-a-pack") {
		t.Errorf("error should name the pack: %v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("error should wrap fs.ErrNotExist; got %v", err)
	}
}

// --- isUnitFile + RenderUnits ----------------------------------------------

// TestIsUnitFile_ClassifiesMergeContent confirms the two merge-content files at
// a pack root are NOT unit files (they are deep-merged/appended, not rendered
// verbatim), while a normal agent/skill/command path IS a unit file.
func TestIsUnitFile_ClassifiesMergeContent(t *testing.T) {
	for _, merge := range []string{appendFileName, snippetFileName, permissionPackFileName, capabilityManifestFileName} {
		if isUnitFile(merge) {
			t.Errorf("isUnitFile(%q): want false (merge-content)", merge)
		}
	}
	for _, unit := range []string{
		"agents/build.md",
		"skills/web-dev-loop/SKILL.md",
		"commands/frontend.md",
		"skills/acme-cockpit.example/.version",
	} {
		if !isUnitFile(unit) {
			t.Errorf("isUnitFile(%q): want true (unit)", unit)
		}
	}
}

// TestRenderUnits_SynthPackEmitsAgentSkillCmdFiles renders a synthetic web-style
// pack into staging and confirms: (1) every expected agent/skill/command unit
// lands under <staging>/.opencode/<pack-rel>, (2) the three merge-content files
// at the pack root are NOT rendered, (3) returned paths are sorted +
// .opencode-prefixed, (4) bytes are copied verbatim.
func TestRenderUnits_SynthPackEmitsAgentSkillCmdFiles(t *testing.T) {
	p := newSynthPack("web-overlay")
	staging := t.TempDir()

	rendered, err := p.RenderUnits(staging, nil)
	if err != nil {
		t.Fatalf("RenderUnits: %v", err)
	}

	wantUnits := []string{
		"agents/browser-qa.md",
		"agents/web-builder.md",
		"commands/browser-view.md",
		"commands/frontend.md",
		"commands/web-smoke.md",
		"commands/web-stack-up.md",
		"skills/web-dev-loop/SKILL.md",
		"skills/web-fixtures/SKILL.md",
	}
	wantLive := make([]string, 0, len(wantUnits))
	for _, rel := range wantUnits {
		wantLive = append(wantLive, opencodePrefix+rel)
	}

	// Sorted invariant.
	gotSorted := append([]string(nil), rendered...)
	for i := 1; i < len(gotSorted); i++ {
		if gotSorted[i-1] > gotSorted[i] {
			t.Errorf("RenderUnits returned unsorted paths: %v", rendered)
			break
		}
	}
	// Every expected unit is rendered + .opencode-prefixed.
	for _, want := range wantLive {
		if !contains(rendered, want) {
			t.Errorf("RenderUnits missing %q; got %v", want, rendered)
		}
		// File exists on disk at <staging>/<want>.
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(want))); err != nil {
			t.Errorf("rendered file %q not on disk: %v", want, err)
		}
	}
	// Merge-content files are NOT rendered (they are merged/appended elsewhere).
	for _, merge := range []string{appendFileName, snippetFileName, permissionPackFileName} {
		bad := opencodePrefix + merge
		if contains(rendered, bad) {
			t.Errorf("RenderUnits must NOT render merge-content file %q", merge)
		}
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(bad))); err == nil {
			t.Errorf("merge-content file %q was rendered to disk; it must not be", bad)
		}
	}
	// Verbatim copy: the rendered agent bytes equal the in-memory source.
	src, serr := fs.ReadFile(p.FS, "agents/browser-qa.md")
	if serr != nil {
		t.Fatalf("read source agent: %v", serr)
	}
	dst, derr := os.ReadFile(filepath.Join(staging, ".opencode/agents/browser-qa.md"))
	if derr != nil {
		t.Fatalf("read rendered agent: %v", derr)
	}
	if string(src) != string(dst) {
		t.Error("RenderUnits did not copy agent bytes verbatim")
	}
}

// TestRenderUnits_VerbatimAcrossPacks renders multiple synthetic packs and
// confirms each rendered unit matches its source byte-for-byte (no template
// substitution, no newline munging).
func TestRenderUnits_VerbatimAcrossPacks(t *testing.T) {
	for _, name := range []string{"alpha-pack", "beta-pack"} {
		t.Run(name, func(t *testing.T) {
			p := newSynthPack(name)
			staging := t.TempDir()
			rendered, err := p.RenderUnits(staging, nil)
			if err != nil {
				t.Fatalf("RenderUnits(%s): %v", name, err)
			}
			for _, live := range rendered {
				rel := strings.TrimPrefix(live, opencodePrefix)
				src, serr := fs.ReadFile(p.FS, rel)
				if serr != nil {
					t.Fatalf("read source %q: %v", rel, serr)
				}
				dst, derr := os.ReadFile(filepath.Join(staging, filepath.FromSlash(live)))
				if derr != nil {
					t.Fatalf("read rendered %q: %v", live, derr)
				}
				if string(src) != string(dst) {
					t.Errorf("pack %s: %q not verbatim (sizes %d vs %d)", name, rel, len(src), len(dst))
				}
			}
		})
	}
}

// contains reports whether s contains x.
func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// --- MergeJSONC: comment + trailing-comma stripping ------------------------

// TestMergeJSONC_StripsLineComments confirms // line comments outside strings
// are stripped before parsing (base + append).
func TestMergeJSONC_StripsLineComments(t *testing.T) {
	base := []byte(`{ // leading object comment
  "a": 1, // a comment
  "b": "value" // b comment
}`)
	out, err := MergeJSONC(base)
	if err != nil {
		t.Fatalf("MergeJSONC line-comment base: %v", err)
	}
	m := decodeJSON(t, out)
	if m["a"].(float64) != 1 {
		t.Errorf("a: got %v, want 1", m["a"])
	}
	if m["b"].(string) != "value" {
		t.Errorf("b: got %v, want value", m["b"])
	}
}

// TestMergeJSONC_StripsBlockComments confirms /* block */ comments (single line
// and spanning lines) are stripped before parsing.
func TestMergeJSONC_StripsBlockComments(t *testing.T) {
	base := []byte(`{ /* file header */
  "a": 1 /* inline */,
  "b": /* dangling */ 2,
  "c": 3 /* multi
line block */
}`)
	out, err := MergeJSONC(base)
	if err != nil {
		t.Fatalf("MergeJSONC block-comment base: %v", err)
	}
	m := decodeJSON(t, out)
	for k, want := range map[string]float64{"a": 1, "b": 2, "c": 3} {
		if m[k].(float64) != want {
			t.Errorf("%s: got %v, want %v", k, m[k], want)
		}
	}
}

// TestMergeJSONC_StripsTrailingCommas confirms trailing commas (object + array)
// are stripped so strict encoding/json accepts JSONC.
func TestMergeJSONC_StripsTrailingCommas(t *testing.T) {
	base := []byte(`{"a": 1, "b": [1, 2, 3,], "c": {"d": 4,},}`)
	out, err := MergeJSONC(base)
	if err != nil {
		t.Fatalf("MergeJSONC trailing-comma base: %v", err)
	}
	m := decodeJSON(t, out)
	if m["a"].(float64) != 1 {
		t.Errorf("a: got %v", m["a"])
	}
	arr := m["b"].([]any)
	if len(arr) != 3 {
		t.Errorf("b length: got %d, want 3", len(arr))
	}
	if asMap(t, m["c"], "c")["d"].(float64) != 4 {
		t.Errorf("c.d: got %v", m["c"])
	}
}

// --- MergeJSONC: string-aware ---------------------------------------------

// TestMergeJSONC_StringAwareComments confirms a // or /* inside a JSON string is
// NOT treated as a comment, while the same sequences outside strings are.
func TestMergeJSONC_StringAwareComments(t *testing.T) {
	base := []byte(`{
  "url": "https://example.com/path",  // real trailing comment
  "regex": "a/*b*/c",                 // not a block comment
  "trailing": "end"                   // last
}`)
	out, err := MergeJSONC(base)
	if err != nil {
		t.Fatalf("MergeJSONC string-aware: %v", err)
	}
	m := decodeJSON(t, out)
	if m["url"].(string) != "https://example.com/path" {
		t.Errorf("url lost // inside string: got %q", m["url"])
	}
	if m["regex"].(string) != "a/*b*/c" {
		t.Errorf("regex lost /* inside string: got %q", m["regex"])
	}
}

// TestMergeJSONC_StringAwareTrailingComma confirms a comma inside a string is
// preserved while a real trailing comma (immediately before } or ]) is dropped.
func TestMergeJSONC_StringAwareTrailingComma(t *testing.T) {
	base := []byte(`{"a": ",", "b": "x",}`)
	out, err := MergeJSONC(base)
	if err != nil {
		t.Fatalf("MergeJSONC: %v", err)
	}
	m := decodeJSON(t, out)
	if m["a"].(string) != "," {
		t.Errorf("comma inside string lost: got %q", m["a"])
	}
}

// --- MergeJSONC: deep-merge semantics -------------------------------------

// TestMergeJSONC_AddsNewTopLevelBlock confirms an append introducing a brand-new
// nested block (e.g. a brand-new agent) is inserted without disturbing siblings.
func TestMergeJSONC_AddsNewTopLevelBlock(t *testing.T) {
	base := []byte(`{"agent": {"build": {"mode": "primary"}}}`)
	append := []byte(`{"agent": {"web-builder": {"mode": "subagent", "steps": 30}}}`)
	out, err := MergeJSONC(base, append)
	if err != nil {
		t.Fatalf("MergeJSONC: %v", err)
	}
	agent := asMap(t, decodeJSON(t, out)["agent"], "agent")
	// Existing block untouched.
	if asMap(t, agent["build"], "build")["mode"] != "primary" {
		t.Errorf("existing build.mode not preserved: %v", agent["build"])
	}
	// New block added.
	wb := asMap(t, agent["web-builder"], "web-builder")
	if wb["mode"] != "subagent" {
		t.Errorf("new web-builder.mode: got %v, want subagent", wb["mode"])
	}
	if wb["steps"].(float64) != 30 {
		t.Errorf("new web-builder.steps: got %v, want 30", wb["steps"])
	}
}

// TestMergeJSONC_InjectsNestedKeysIntoExistingBlock confirms an append that
// targets an EXISTING nested block ADDS keys into it recursively (does NOT
// overwrite the whole block). This is the canonical opencode-append case: adding
// browser-qa/web-builder allow entries into the build/coordination/
// project-coordinator agents' existing task maps.
func TestMergeJSONC_InjectsNestedKeysIntoExistingBlock(t *testing.T) {
	base := []byte(`{
  "agent": {
    "build": {
      "mode": "primary",
      "permission": {"task": {"committer": "allow"}}
    }
  }
}`)
	append := []byte(`{
  "agent": {
    "build": {
      "permission": {"task": {"browser-qa": "allow", "web-builder": "allow"}}
    }
  }
}`)
	out, err := MergeJSONC(base, append)
	if err != nil {
		t.Fatalf("MergeJSONC: %v", err)
	}
	agent := asMap(t, decodeJSON(t, out)["agent"], "agent")
	build := asMap(t, agent["build"], "build")
	// Untouched sibling key preserved.
	if build["mode"] != "primary" {
		t.Errorf("build.mode should be preserved as primary; got %v", build["mode"])
	}
	// Task map has BOTH the original and injected entries (deep merge, not replace).
	task := asMap(t, asMap(t, build["permission"], "permission")["task"], "task")
	for _, want := range []string{"committer", "browser-qa", "web-builder"} {
		if task[want] != "allow" {
			t.Errorf("task[%q] = %v, want allow (injected key must be added, not overwrite the map)", want, task[want])
		}
	}
}

// TestMergeJSONC_ScalarAndArrayOverwrite confirms non-map values (scalars and
// arrays) are overwritten by the append (append wins), since deep-merge only
// recurses for map-vs-map.
func TestMergeJSONC_ScalarAndArrayOverwrite(t *testing.T) {
	base := []byte(`{"steps": 30, "tags": ["a", "b"], "name": "old"}`)
	append := []byte(`{"steps": 50, "tags": ["x"], "name": "new"}`)
	out, err := MergeJSONC(base, append)
	if err != nil {
		t.Fatalf("MergeJSONC: %v", err)
	}
	m := decodeJSON(t, out)
	if m["steps"].(float64) != 50 {
		t.Errorf("scalar overwrite: steps got %v, want 50", m["steps"])
	}
	if m["name"] != "new" {
		t.Errorf("string overwrite: name got %v, want new", m["name"])
	}
	tags := m["tags"].([]any)
	if len(tags) != 1 || tags[0] != "x" {
		t.Errorf("array overwrite: tags got %v, want [x]", tags)
	}
}

// TestMergeJSONC_MultipleAppends confirms variadic appends merge left-to-right,
// later appends winning on conflicts.
func TestMergeJSONC_MultipleAppends(t *testing.T) {
	base := []byte(`{"k": "base"}`)
	a1 := []byte(`{"k": "a1", "only1": true}`)
	a2 := []byte(`{"k": "a2", "only2": true}`)
	out, err := MergeJSONC(base, a1, a2)
	if err != nil {
		t.Fatalf("MergeJSONC: %v", err)
	}
	m := decodeJSON(t, out)
	if m["k"] != "a2" {
		t.Errorf("last append must win: k got %v, want a2", m["k"])
	}
	if m["only1"] != true || m["only2"] != true {
		t.Errorf("non-conflicting keys from both appends must survive: %v", m)
	}
}

// TestMergeJSONC_NullBaseYieldsEmptyMap confirms a JSON null base document yields
// an empty map (never nil) so an append merges cleanly. A null base is the only
// degenerate base parseJSONC accepts; truly-empty or nil bytes are fail-closed
// (json.Unmarshal rejects them), which is the defensible behavior since an empty
// opencode.jsonc is never produced by the core corpus.
func TestMergeJSONC_NullBaseYieldsEmptyMap(t *testing.T) {
	append := []byte(`{"agent": {"new": {}}}`)
	out, err := MergeJSONC([]byte(`null`), append)
	if err != nil {
		t.Fatalf("MergeJSONC null base: %v", err)
	}
	agent := decodeJSON(t, out)["agent"]
	if _, ok := agent.(map[string]any)["new"]; !ok {
		t.Errorf("null base: append not merged into empty map; out=%s", out)
	}
}

// TestMergeJSONC_EmptyBytesRejected confirms empty/nil base bytes are rejected
// (fail-closed) rather than silently treated as an empty document.
func TestMergeJSONC_EmptyBytesRejected(t *testing.T) {
	for _, base := range [][]byte{nil, []byte(``)} {
		if _, err := MergeJSONC(base, []byte(`{"a":1}`)); err == nil {
			t.Errorf("MergeJSONC base=%q: want error (fail-closed), got nil", string(base))
		}
	}
}

// TestMergeJSONC_OutputShape confirms the merged output is indented JSON ending
// in a trailing newline (the contract opencode.jsonc readers rely on).
func TestMergeJSONC_OutputShape(t *testing.T) {
	out, err := MergeJSONC([]byte(`{"a":1}`))
	if err != nil {
		t.Fatalf("MergeJSONC: %v", err)
	}
	if len(out) == 0 || out[len(out)-1] != '\n' {
		t.Errorf("merged output must end with newline; got %q", out)
	}
	if !strings.Contains(string(out), "    ") {
		t.Errorf("merged output must be indented; got %q", out)
	}
}

// TestMergeJSONC_SyntheticOverlayAppend merges the synthetic web-style pack's
// opencode-append.jsonc (carried by newSynthPack) into a minimal base and
// confirms the canonical overlay-merge effects: new agent blocks added
// (web-builder, browser-qa) AND task-allow entries injected into the existing
// build/coordination/project-coordinator blocks without dropping their other
// fields. This ties the unit-level merge contract to pack-shaped bytes (the real
// web-overlay pack was relocated to a non-shipped adoption reference, so the
// same shape is carried here synthetically).
func TestMergeJSONC_SyntheticOverlayAppend(t *testing.T) {
	p := newSynthPack("web-overlay")
	appendBytes, err := fs.ReadFile(p.FS, appendFileName)
	if err != nil {
		t.Fatalf("read synthetic append: %v", err)
	}
	base := []byte(`{
  "agent": {
    "build": {"mode": "primary", "permission": {"task": {"committer": "allow", "*": "deny"}}},
    "coordination": {"permission": {"task": {"*": "deny"}}},
    "project-coordinator": {"permission": {"task": {"*": "deny"}}}
  }
}`)
	out, mErr := MergeJSONC(base, appendBytes)
	if mErr != nil {
		t.Fatalf("MergeJSONC synthetic overlay: %v", mErr)
	}
	agent := asMap(t, decodeJSON(t, out)["agent"], "agent")

	// New agent blocks added verbatim from the pack.
	for _, name := range []string{"web-builder", "browser-qa"} {
		if _, ok := agent[name].(map[string]any); !ok {
			t.Errorf("synthetic overlay merge: agent %q block not added; got %v", name, agent)
		}
	}

	// Existing build block: mode preserved (NOT overwritten) + task entries injected.
	build := asMap(t, agent["build"], "build")
	if build["mode"] != "primary" {
		t.Errorf("synthetic overlay merge clobbered build.mode: got %v", build["mode"])
	}
	buildTask := asMap(t, asMap(t, build["permission"], "permission")["task"], "build.task")
	for _, want := range []string{"committer", "*", "browser-qa", "web-builder"} {
		if _, ok := buildTask[want]; !ok {
			t.Errorf("synthetic overlay merge: build.task missing %q (injected keys must be added, originals preserved); got %v", want, buildTask)
		}
	}

	// The same injection landed on coordination + project-coordinator.
	for _, name := range []string{"coordination", "project-coordinator"} {
		task := asMap(t, asMap(t, asMap(t, agent[name], name)["permission"], "permission")["task"], name+".task")
		for _, want := range []string{"*", "browser-qa", "web-builder"} {
			if _, ok := task[want]; !ok {
				t.Errorf("synthetic overlay merge: %s.task missing %q; got %v", name, want, task)
			}
		}
	}
}

// --- MergeAppend (Pack-level) ----------------------------------------------

// TestMergeAppend_DeepMergesStagedOpencode confirms Pack.MergeAppend deep-merges
// the pack's opencode-append.jsonc into a staged opencode.jsonc, and is a no-op
// when the pack ships no append file.
func TestMergeAppend_DeepMergesStagedOpencode(t *testing.T) {
	t.Run("synth_pack_merges", func(t *testing.T) {
		p := newSynthPack("web-overlay")
		staging := t.TempDir()
		target := filepath.Join(staging, opencodeTargetRel)
		if err := os.WriteFile(target, []byte(`{"agent": {"build": {}}}`+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := p.MergeAppend(staging); err != nil {
			t.Fatalf("MergeAppend: %v", err)
		}
		merged, rerr := os.ReadFile(target)
		if rerr != nil {
			t.Fatal(rerr)
		}
		agent := asMap(t, decodeJSON(t, merged)["agent"], "agent")
		if _, ok := agent["web-builder"]; !ok {
			t.Errorf("MergeAppend did not add web-builder agent; merged=%s", merged)
		}
		if _, ok := agent["browser-qa"]; !ok {
			t.Errorf("MergeAppend did not add browser-qa agent; merged=%s", merged)
		}
	})

	t.Run("noop_pack_no_append", func(t *testing.T) {
		// A pack that contributes skills only (no opencode-append.jsonc) must
		// leave the staged opencode.jsonc untouched. Exercise the no-append path
		// with a synthetic in-memory pack (mirrors how an adoption pack like the
		// relocated acme-cockpit example contributes skills only).
		p := &Pack{
			Name: "synthetic-no-append",
			FS: fstest.MapFS{
				"skills/only/SKILL.md": &fstest.MapFile{Data: []byte("skill")},
			},
		}
		staging := t.TempDir()
		target := filepath.Join(staging, opencodeTargetRel)
		orig := []byte(`{"agent": {}}` + "\n")
		if err := os.WriteFile(target, orig, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := p.MergeAppend(staging); err != nil {
			t.Errorf("MergeAppend on pack with no append should be a no-op; got %v", err)
		}
		got, _ := os.ReadFile(target)
		if string(got) != string(orig) {
			t.Errorf("no-append pack must leave opencode.jsonc untouched; got %s", got)
		}
	})

	t.Run("missing_staged_target_errors", func(t *testing.T) {
		p := newSynthPack("web-overlay")
		staging := t.TempDir() // no opencode.jsonc staged
		if err := p.MergeAppend(staging); err == nil {
			t.Error("MergeAppend must error when staged opencode.jsonc is missing")
		}
	})
}

// --- AppendCallableGraph (Pack-level) --------------------------------------

// TestAppendCallableGraph_AppendsSnippet confirms the pack's callable-graph
// snippet is appended to the staged callable-graph.md, is a no-op when the pack
// ships no snippet, and is a no-op when the core rendered no callable-graph.md.
func TestAppendCallableGraph_AppendsSnippet(t *testing.T) {
	t.Run("synth_pack_appends", func(t *testing.T) {
		p := newSynthPack("web-overlay")
		staging := t.TempDir()
		target := filepath.Join(staging, filepath.FromSlash(callableGraphRel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("CORE-BODY\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := p.AppendCallableGraph(staging); err != nil {
			t.Fatalf("AppendCallableGraph: %v", err)
		}
		got, _ := os.ReadFile(target)
		// Core body preserved, snippet text appended after a blank-line pair.
		if !strings.HasPrefix(string(got), "CORE-BODY\n") {
			t.Errorf("callable-graph core body lost; got %q", got)
		}
		snippet, _ := fs.ReadFile(p.FS, snippetFileName)
		if !strings.Contains(string(got), string(snippet)) {
			t.Errorf("callable-graph snippet not appended; got %q", got)
		}
	})

	t.Run("noop_pack_no_snippet", func(t *testing.T) {
		// A pack without callable-graph-snippet.md must no-op regardless of
		// target. Exercise the no-snippet path with a synthetic in-memory pack
		// (mirrors how an adoption pack like the relocated acme-cockpit example
		// contributes skills only).
		p := &Pack{
			Name: "synthetic-no-snippet",
			FS: fstest.MapFS{
				"skills/only/SKILL.md": &fstest.MapFile{Data: []byte("skill")},
			},
		}
		staging := t.TempDir()
		if err := p.AppendCallableGraph(staging); err != nil {
			t.Errorf("pack without snippet must no-op; got %v", err)
		}
	})

	t.Run("synth_pack_noop_when_target_absent", func(t *testing.T) {
		p := newSynthPack("web-overlay")
		staging := t.TempDir() // core rendered no callable-graph.md
		if err := p.AppendCallableGraph(staging); err != nil {
			t.Errorf("absent callable-graph.md must no-op; got %v", err)
		}
		if _, err := os.Stat(filepath.Join(staging, filepath.FromSlash(callableGraphRel))); err == nil {
			t.Error("AppendCallableGraph must not create callable-graph.md when absent")
		}
	})
}

// --- full pack round-trip on an in-memory FS -------------------------------

// TestRenderUnits_FstestMap confirms RenderUnits works against any fs.FS, not
// only the embedded corpus: a synthetic pack with nested agent/skill/command
// unit files renders all of them under .opencode/ and skips a pack-root
// callable-graph-snippet.md.
func TestRenderUnits_FstestMap(t *testing.T) {
	p := &Pack{
		Name: "synthetic",
		FS: fstest.MapFS{
			"agents/x.md":          &fstest.MapFile{Data: []byte("x-agent")},
			"skills/s/SKILL.md":    &fstest.MapFile{Data: []byte("s-skill")},
			"commands/c.md":        &fstest.MapFile{Data: []byte("c-cmd")},
			"skills/s/sub/deep.md": &fstest.MapFile{Data: []byte("deep")},
			snippetFileName:        &fstest.MapFile{Data: []byte("snip")},
			appendFileName:         &fstest.MapFile{Data: []byte(`{}`)},
			permissionPackFileName: &fstest.MapFile{Data: []byte(`{}`)},
		},
	}
	staging := t.TempDir()
	rendered, err := p.RenderUnits(staging, nil)
	if err != nil {
		t.Fatalf("RenderUnits synthetic: %v", err)
	}
	want := []string{
		".opencode/agents/x.md",
		".opencode/skills/s/SKILL.md",
		".opencode/commands/c.md",
		".opencode/skills/s/sub/deep.md",
	}
	if len(rendered) != len(want) {
		t.Fatalf("rendered count: got %d (%v), want %d (%v)", len(rendered), rendered, len(want), want)
	}
	for _, w := range want {
		if !contains(rendered, w) {
			t.Errorf("missing rendered %q; got %v", w, rendered)
		}
	}
	// Merge-content files excluded.
	if contains(rendered, ".opencode/"+snippetFileName) || contains(rendered, ".opencode/"+appendFileName) || contains(rendered, ".opencode/"+permissionPackFileName) {
		t.Errorf("merge-content files must not render; got %v", rendered)
	}
	// Deeply nested dir created.
	deep, err := os.ReadFile(filepath.Join(staging, ".opencode/skills/s/sub/deep.md"))
	if err != nil {
		t.Errorf("deeply nested unit not rendered: %v", err)
	}
	if string(deep) != "deep" {
		t.Errorf("deep unit bytes: got %q", deep)
	}
}

// --- MaterializePermissionPack (Pack-level) --------------------------------

// TestMaterializePermissionPack_WritesDescriptor confirms the pack's
// permission-pack.jsonc is materialized under the live permission-packs subtree
// and is a no-op when the pack ships no descriptor.
func TestMaterializePermissionPack_WritesDescriptor(t *testing.T) {
	t.Run("synth_pack_materializes", func(t *testing.T) {
		p := newSynthPack("web-overlay")
		staging := t.TempDir()
		liveRel, err := p.MaterializePermissionPack(staging)
		if err != nil {
			t.Fatalf("MaterializePermissionPack: %v", err)
		}
		wantRel := permissionPackRel + "web-overlay.jsonc"
		if liveRel != wantRel {
			t.Errorf("liveRel: got %q want %q", liveRel, wantRel)
		}
		got, rerr := os.ReadFile(filepath.Join(staging, filepath.FromSlash(liveRel)))
		if rerr != nil {
			t.Fatalf("materialized descriptor not on disk: %v", rerr)
		}
		src, _ := fs.ReadFile(p.FS, permissionPackFileName)
		if string(got) != string(src) {
			t.Errorf("permission-pack not verbatim; got %q want %q", got, src)
		}
	})

	t.Run("noop_pack_no_descriptor", func(t *testing.T) {
		p := &Pack{
			Name: "synthetic-no-perm",
			FS: fstest.MapFS{
				"skills/only/SKILL.md": &fstest.MapFile{Data: []byte("skill")},
			},
		}
		staging := t.TempDir()
		liveRel, err := p.MaterializePermissionPack(staging)
		if err != nil {
			t.Fatalf("no-descriptor pack must no-op; got %v", err)
		}
		if liveRel != "" {
			t.Errorf("no-descriptor pack must return empty liveRel; got %q", liveRel)
		}
	})
}

// --- RenderUnits: harness-token substitution (overlay-ADD matches core) ------

// TestRenderUnits_ResolvesHarnessTokens confirms RenderUnits applies the SAME
// 3-token identity substitution the core renderers apply (substrate.
// SubstituteHarnessTokens) to overlay-unit bodies. A unit carrying
// {{PROJECT_NAME}} / {{PROJECT_SLUG}} / {{COORDINATOR_DIR}} must ship resolved
// when answers are supplied; soft placeholders ({{DEMO_VPS_FINGERPRINT}}) stay
// literal. This is the unit-level proof that the overlay-ADD path no longer
// leaks literal harness sentinels (the gap the slice closes).
func TestRenderUnits_ResolvesHarnessTokens(t *testing.T) {
	t.Run("answers_resolve_three_canonical_tokens", func(t *testing.T) {
		p := &Pack{
			Name: "tokenpack",
			FS: fstest.MapFS{
				"agents/runtime-guardian.md": {Data: []byte(
					"You are the {{PROJECT_NAME}} runtime guardian.\n" +
						"coord: .local/{{COORDINATOR_DIR}}/\n" +
						"secret: {{PROJECT_SLUG}}_JWT_SECRET\n" +
						"image: {{PROJECT_SLUG}}-dev-1\n")},
			},
		}
		staging := t.TempDir()
		answers := map[string]string{
			"project_name":    "Acme",
			"project_slug":    "acme",
			"coordinator_dir": "acme-coordinator",
		}
		if _, err := p.RenderUnits(staging, answers); err != nil {
			t.Fatalf("RenderUnits: %v", err)
		}
		got, rerr := os.ReadFile(filepath.Join(staging, ".opencode/agents/runtime-guardian.md"))
		if rerr != nil {
			t.Fatalf("read rendered unit: %v", rerr)
		}
		body := string(got)
		for _, tok := range []string{"{{PROJECT_NAME}}", "{{PROJECT_SLUG}}", "{{COORDINATOR_DIR}}"} {
			if strings.Contains(body, tok) {
				t.Errorf("literal sentinel %q survived into overlay unit:\n%s", tok, body)
			}
		}
		// PROJECT_SLUG is case-aware: '_' -> UPPER, '-' -> lower.
		wantSubstrings := []string{
			"You are the Acme runtime guardian.",
			"coord: .local/acme-coordinator/",
			"secret: ACME_JWT_SECRET",
			"image: acme-dev-1",
		}
		for _, w := range wantSubstrings {
			if !strings.Contains(body, w) {
				t.Errorf("resolved unit missing %q; got:\n%s", w, body)
			}
		}
	})

	t.Run("nil_answers_is_verbatim_noop", func(t *testing.T) {
		// A caller with no answers (e.g. a token-free pack, or a unit test) gets
		// verbatim bytes back: the substitution fast-path short-circuits when no
		// canonical sentinel is present, and even a present sentinel resolves to
		// its answer default only when answers are non-empty. With nil answers a
		// token-bearing body is returned UNCHANGED (no sentinel), matching the
		// documented no-op contract and preserving the legacy verbatim tests.
		src := []byte("plain unit, no sentinel here\n")
		p := &Pack{
			Name: "plainpack",
			FS:   fstest.MapFS{"agents/plain.md": {Data: src}},
		}
		staging := t.TempDir()
		if _, err := p.RenderUnits(staging, nil); err != nil {
			t.Fatalf("RenderUnits: %v", err)
		}
		got, _ := os.ReadFile(filepath.Join(staging, ".opencode/agents/plain.md"))
		if string(got) != string(src) {
			t.Errorf("nil answers must be verbatim; got %q want %q", got, src)
		}
	})
}
