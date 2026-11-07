// write_test.go — the write tool: atomic create/truncate with parent
// creation strictly inside the roots, the result contract, and the
// zero-filesystem-effects property of every rejected write.
package filetools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCreatesFile(t *testing.T) {
	p, root := filePipeline(t)
	res := runTool(t, p, "write", `{"path":"out/hello.txt","content":"hello\n"}`)
	if res.IsError {
		t.Fatalf("write result = %+v", res)
	}
	want := "wrote " + filepath.Join(root, "out", "hello.txt") + " (6 bytes)\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q", res.Content, want)
	}
	got, err := os.ReadFile(filepath.Join(root, "out", "hello.txt"))
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("file = %q (err %v), want hello\\n", got, err)
	}
	// No temp residue in the directory (atomic rename contract).
	entries, _ := os.ReadDir(filepath.Join(root, "out"))
	if len(entries) != 1 {
		t.Fatalf("dir entries = %+v, want exactly the target", entries)
	}
}

func TestWriteOverwritesExisting(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "exists.txt", "old-longer-content\n")
	res := runTool(t, p, "write", `{"path":"exists.txt","content":"new\n"}`)
	if res.IsError {
		t.Fatalf("write result = %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(root, "exists.txt"))
	if string(got) != "new\n" {
		t.Fatalf("file = %q, want truncated to new\\n", got)
	}
}

func TestWriteEmptyContent(t *testing.T) {
	p, root := filePipeline(t)
	res := runTool(t, p, "write", `{"path":"empty.txt","content":""}`)
	if res.IsError {
		t.Fatalf("empty write = %+v", res)
	}
	fi, err := os.Stat(filepath.Join(root, "empty.txt"))
	if err != nil || fi.Size() != 0 {
		t.Fatalf("empty file stat = %+v (err %v)", fi, err)
	}
}

func TestWriteAbsoluteInsideRoot(t *testing.T) {
	p, root := filePipeline(t)
	res := runTool(t, p, "write", `{"path":"`+filepath.Join(root, "abs.txt")+`","content":"x"}`)
	if res.IsError {
		t.Fatalf("absolute-in-root write = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "abs.txt")); err != nil {
		t.Fatalf("file missing: %v", err)
	}
}

// TestWriteRejectionsLeaveNoTrace is the confinement battery for the
// mutating side: every rejected write creates neither the file NOR any
// parent directories, inside or outside the roots.
func TestWriteRejectionsLeaveNoTrace(t *testing.T) {
	p, root := filePipeline(t)
	outside := t.TempDir()

	// A symlink inside the root pointing outside.
	if err := os.Symlink(outside, filepath.Join(root, "leak")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name, args, wantIn string
	}{
		{"missing args", ``, "args.path is required"},
		{"empty path", `{"path":"","content":"x"}`, "args.path is required"},
		{"parent climb", `{"path":"../x/new.txt","content":"x"}`, "[escape]"},
		{"deep climb", `{"path":"a/../../x/new.txt","content":"x"}`, "[escape]"},
		{"absolute outside", `{"path":"` + filepath.Join(outside, "new.txt") + `","content":"x"}`, "[outside-roots]"},
		{"symlink dir escape", `{"path":"leak/new.txt","content":"x"}`, "[symlink-escape]"},
		{"unknown field", `{"path":"ok.txt","content":"x","nope":true}`, "invalid args"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runTool(t, p, "write", tc.args)
			if !res.IsError {
				t.Fatalf("write(%s) = %+v, want isError", tc.name, res)
			}
			if !strings.Contains(res.Content, tc.wantIn) {
				t.Fatalf("error %q missing %q", res.Content, tc.wantIn)
			}
		})
	}

	// Zero trace: nothing landed in the root (beyond the symlink
	// fixture itself), nothing outside, no parent dirs created.
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() == "leak" {
			continue
		}
		t.Fatalf("rejected writes left a trace in the root: %s", e.Name())
	}
	outEntries, _ := os.ReadDir(outside)
	if len(outEntries) != 0 {
		t.Fatalf("rejected writes reached outside the root: %+v", outEntries)
	}
}

// TestWriteFinalSymlinkRejected pins the truncation-safety rule: an
// existing symlink AT the target is refused — os.Create would
// truncate whatever it points at.
func TestWriteFinalSymlinkRejected(t *testing.T) {
	p, root := filePipeline(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "precious.txt")
	if err := os.WriteFile(target, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "alias.txt")); err != nil {
		t.Fatal(err)
	}

	res := runTool(t, p, "write", `{"path":"alias.txt","content":"clobber"}`)
	if !res.IsError || !strings.Contains(res.Content, "[symlink-final]") {
		t.Fatalf("final-symlink write = %+v, want typed rejection", res)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "keep me" {
		t.Fatalf("symlink target was modified: %q", got)
	}
}

// TestWriteOverDirectoryRejected: a directory target refuses.
func TestWriteOverDirectoryRejected(t *testing.T) {
	p, root := filePipeline(t)
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := runTool(t, p, "write", `{"path":"dir","content":"x"}`)
	if !res.IsError || !strings.Contains(res.Content, "[is-a-directory]") {
		t.Fatalf("dir write = %+v, want typed rejection", res)
	}
}

// TestWriteFileBlockingParent: an intermediate component that exists
// as a FILE refuses before effects.
func TestWriteFileBlockingParent(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "blocker", "x")
	res := runTool(t, p, "write", `{"path":"blocker/sub/new.txt","content":"x"}`)
	if !res.IsError || !strings.Contains(res.Content, "[not-a-directory]") {
		t.Fatalf("blocked write = %+v, want typed rejection", res)
	}
}
