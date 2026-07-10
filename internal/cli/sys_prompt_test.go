package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The named system prompts surfaced by `vh-agent-harness sys-prompt` are
// embed-only under templates/sys-prompts; these keys must stay available offline
// from any CWD. Adding or renaming a prompt file changes this set intentionally.
var wantSysPromptKeys = []string{
	"auto-gate-classifier",
}

// TestSysPromptIndex_EmbeddedKeys pins the embedded key set (basename minus .md)
// and that each prompt has a non-empty body.
func TestSysPromptIndex_EmbeddedKeys(t *testing.T) {
	index, keys, err := sysPromptIndex()
	if err != nil {
		t.Fatalf("sysPromptIndex: %v", err)
	}
	for _, k := range wantSysPromptKeys {
		body, ok := index[k]
		if !ok {
			t.Errorf("missing embedded sys-prompt key %q (have %v)", k, keys)
			continue
		}
		if len(body) == 0 {
			t.Errorf("embedded sys-prompt %q is empty", k)
		}
	}
	if len(index) != len(wantSysPromptKeys) {
		t.Errorf("sys-prompt key count = %d, want %d (%v)", len(index), len(wantSysPromptKeys), keys)
	}
}

// TestSysPrompt_ListNoArg lists every key from an empty target (no overrides),
// so no key is marked [override].
func TestSysPrompt_ListNoArg(t *testing.T) {
	out, err := executeCapture(t, []string{"sys-prompt", "--target", t.TempDir()})
	if err != nil {
		t.Fatalf("sys-prompt list: %v", err)
	}
	for _, k := range wantSysPromptKeys {
		if !strings.Contains(out, k) {
			t.Errorf("list missing key %q\n--- output ---\n%s", k, out)
		}
	}
	if strings.Contains(out, "[override]") {
		t.Errorf("empty target should show no [override] marker\n--- output ---\n%s", out)
	}
}

// TestSysPrompt_PrintEmbedded prints the embedded copy when no override applies.
func TestSysPrompt_PrintEmbedded(t *testing.T) {
	out, err := executeCapture(t, []string{"sys-prompt", "--target", t.TempDir(), "auto-gate-classifier"})
	if err != nil {
		t.Fatalf("sys-prompt auto-gate-classifier: %v", err)
	}
	if !strings.Contains(out, "You are a security monitor") {
		t.Errorf("embedded body missing known phrase\n--- output ---\n%s", out)
	}
}

// TestSysPrompt_KeyNormalization accepts a .md suffix and a path prefix,
// resolving to the same key.
func TestSysPrompt_KeyNormalization(t *testing.T) {
	for _, arg := range []string{
		"auto-gate-classifier",
		"auto-gate-classifier.md",
		"templates/sys-prompts/auto-gate-classifier.md",
	} {
		out, err := executeCapture(t, []string{"sys-prompt", "--target", t.TempDir(), arg})
		if err != nil {
			t.Fatalf("sys-prompt %q: %v", arg, err)
		}
		if !strings.Contains(out, "You are a security monitor") {
			t.Errorf("arg %q did not resolve to auto-gate-classifier\n--- output ---\n%s", arg, out)
		}
	}
}

// TestSysPrompt_UnknownKeyErrors returns a non-nil error and lists valid keys.
func TestSysPrompt_UnknownKeyErrors(t *testing.T) {
	out, err := executeCapture(t, []string{"sys-prompt", "--target", t.TempDir(), "no-such-prompt"})
	if err == nil {
		t.Fatal("unknown key: want non-nil error, got nil")
	}
	if !strings.Contains(out, "no sys-prompt for") {
		t.Errorf("unknown key should explain + list keys\n--- output ---\n%s", out)
	}
}

// TestSysPrompt_OverrideServesLiveFile confirms a live file at
// <target>/.opencode/sys-prompts/<name>.md supersedes the embedded copy, and is
// marked in the list.
func TestSysPrompt_OverrideServesLiveFile(t *testing.T) {
	target := t.TempDir()
	liveDir := filepath.Join(target, ".opencode", "sys-prompts")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const sentinel = "LIVE-OVERRIDE-CONTENT-42"
	if err := os.WriteFile(
		filepath.Join(liveDir, "auto-gate-classifier.md"),
		[]byte("# Live\n"+sentinel+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	out, err := executeCapture(t, []string{"sys-prompt", "--target", target, "auto-gate-classifier"})
	if err != nil {
		t.Fatalf("sys-prompt with override: %v", err)
	}
	if !strings.Contains(out, sentinel) {
		t.Errorf("override should serve live file content\n--- output ---\n%s", out)
	}
	if strings.Contains(out, "You are a security monitor") {
		t.Errorf("override should NOT serve the embedded copy\n--- output ---\n%s", out)
	}

	list, err := executeCapture(t, []string{"sys-prompt", "--target", target})
	if err != nil {
		t.Fatalf("sys-prompt list with override: %v", err)
	}
	if !strings.Contains(list, "[override]") {
		t.Errorf("list should mark the overridden key\n--- output ---\n%s", list)
	}
}
