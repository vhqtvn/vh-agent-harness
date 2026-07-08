package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/runshape"
)

// The generic docs surfaced by `vh-agent-harness docs` are embed-only under
// templates/docs; these keys must stay available offline from any CWD. Adding
// or renaming a doc file changes this set intentionally.
var wantDocKeys = []string{
	"opencode-memory-model",
	"opencode-prompt-guide",
	"opencode-session-workflow",
	"opencode-skills",
	"temporary-files",
}

// TestDocsIndex_EmbeddedKeys pins the embedded key set (basename minus .md) and
// that each doc has a non-empty body.
func TestDocsIndex_EmbeddedKeys(t *testing.T) {
	index, keys, err := docsIndex()
	if err != nil {
		t.Fatalf("docsIndex: %v", err)
	}
	for _, k := range wantDocKeys {
		body, ok := index[k]
		if !ok {
			t.Errorf("missing embedded doc key %q (have %v)", k, keys)
			continue
		}
		if len(body) == 0 {
			t.Errorf("embedded doc %q is empty", k)
		}
	}
	if len(index) != len(wantDocKeys) {
		t.Errorf("doc key count = %d, want %d (%v)", len(index), len(wantDocKeys), keys)
	}
}

// TestDocs_ListNoArg lists every key from an empty target (no overrides), so no
// key is marked [override].
func TestDocs_ListNoArg(t *testing.T) {
	out, err := executeCapture(t, []string{"docs", "--target", t.TempDir()})
	if err != nil {
		t.Fatalf("docs list: %v", err)
	}
	for _, k := range wantDocKeys {
		if !strings.Contains(out, k) {
			t.Errorf("list missing key %q\n--- output ---\n%s", k, out)
		}
	}
	if strings.Contains(out, "[override]") {
		t.Errorf("empty target should show no [override] marker\n--- output ---\n%s", out)
	}
}

// TestDocs_PrintEmbedded prints the embedded copy when no override applies.
func TestDocs_PrintEmbedded(t *testing.T) {
	out, err := executeCapture(t, []string{"docs", "--target", t.TempDir(), "opencode-memory-model"})
	if err != nil {
		t.Fatalf("docs opencode-memory-model: %v", err)
	}
	if !strings.Contains(out, "# OpenCode Memory Model") {
		t.Errorf("embedded body missing heading\n--- output ---\n%s", out)
	}
}

// TestDocs_KeyNormalization accepts a .md suffix and a path prefix, resolving to
// the same key.
func TestDocs_KeyNormalization(t *testing.T) {
	for _, arg := range []string{"temporary-files", "temporary-files.md", "templates/docs/temporary-files.md"} {
		out, err := executeCapture(t, []string{"docs", "--target", t.TempDir(), arg})
		if err != nil {
			t.Fatalf("docs %q: %v", arg, err)
		}
		if !strings.Contains(out, "Temporary Files") {
			t.Errorf("arg %q did not resolve to temporary-files\n--- output ---\n%s", arg, out)
		}
	}
}

// TestDocs_UnknownKeyErrors returns a non-nil error and lists valid keys.
func TestDocs_UnknownKeyErrors(t *testing.T) {
	out, err := executeCapture(t, []string{"docs", "--target", t.TempDir(), "no-such-doc"})
	if err == nil {
		t.Fatal("unknown key: want non-nil error, got nil")
	}
	if !strings.Contains(out, "no doc for") {
		t.Errorf("unknown key should explain + list keys\n--- output ---\n%s", out)
	}
}

// TestDocs_OverrideServesLiveFile confirms an explicit override serves the live
// on-disk file's content instead of the embedded copy, and marks it in the list.
func TestDocs_OverrideServesLiveFile(t *testing.T) {
	target := t.TempDir()
	cfgDir := filepath.Join(target, runshape.DirName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A live source file with content that is NOT in the embedded copy.
	liveRel := "my-docs/memory.md"
	livePath := filepath.Join(target, filepath.FromSlash(liveRel))
	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		t.Fatal(err)
	}
	const sentinel = "LIVE-OVERRIDE-CONTENT-42"
	if err := os.WriteFile(livePath, []byte("# Live\n"+sentinel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := "overrides:\n  opencode-memory-model: " + liveRel + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "docs-overrides.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := executeCapture(t, []string{"docs", "--target", target, "opencode-memory-model"})
	if err != nil {
		t.Fatalf("docs with override: %v", err)
	}
	if !strings.Contains(out, sentinel) {
		t.Errorf("override should serve live file content\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "# OpenCode Memory Model") {
		t.Errorf("override should NOT serve the embedded copy\n--- output ---\n%s", out)
	}

	list, err := executeCapture(t, []string{"docs", "--target", target})
	if err != nil {
		t.Fatalf("docs list with override: %v", err)
	}
	if !strings.Contains(list, "[override]") {
		t.Errorf("list should mark the overridden key\n--- output ---\n%s", list)
	}
}

// TestMaterializeContextDocs_WritesAlwaysOnDocs confirms the always-on context
// docs are materialized into .vh-agent-harness/docs/ with the embedded content
// (opencode.jsonc instructions[] points there).
func TestMaterializeContextDocs_WritesAlwaysOnDocs(t *testing.T) {
	target := t.TempDir()
	written, err := materializeContextDocs(target)
	if err != nil {
		t.Fatalf("materializeContextDocs: %v", err)
	}
	if len(written) != len(contextDocKeys) {
		t.Fatalf("wrote %d docs, want %d", len(written), len(contextDocKeys))
	}
	for _, key := range contextDocKeys {
		p := filepath.Join(target, runshape.DirName, contextDocsSubdir, key+".md")
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			t.Fatalf("materialized doc %q not readable: %v", key, rerr)
		}
		if len(body) == 0 {
			t.Errorf("materialized doc %q is empty", key)
		}
	}
}

// TestMaterializeContextDocs_HonorsOverride confirms materialization serves the
// live override source (keeps this repo's dogfood copy in sync on update).
func TestMaterializeContextDocs_HonorsOverride(t *testing.T) {
	target := t.TempDir()
	cfgDir := filepath.Join(target, runshape.DirName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	key := contextDocKeys[0]
	liveRel := "src/live.md"
	livePath := filepath.Join(target, filepath.FromSlash(liveRel))
	if err := os.MkdirAll(filepath.Dir(livePath), 0o755); err != nil {
		t.Fatal(err)
	}
	const sentinel = "MATERIALIZE-OVERRIDE-99"
	if err := os.WriteFile(livePath, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "docs-overrides.yml"),
		[]byte("overrides:\n  "+key+": "+liveRel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := materializeContextDocs(target); err != nil {
		t.Fatalf("materializeContextDocs: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(cfgDir, contextDocsSubdir, key+".md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != sentinel {
		t.Errorf("materialized override content = %q, want %q", string(body), sentinel)
	}
}

// TestDocs_OverrideMissingFileErrors confirms a configured override pointing at
// a missing file is a loud error, not a silent fall-back to embedded.
func TestDocs_OverrideMissingFileErrors(t *testing.T) {
	target := t.TempDir()
	cfgDir := filepath.Join(target, runshape.DirName)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "overrides:\n  opencode-memory-model: does/not/exist.md\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "docs-overrides.yml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := executeCapture(t, []string{"docs", "--target", target, "opencode-memory-model"})
	if err == nil {
		t.Fatal("override at missing file: want non-nil error, got nil")
	}
}
