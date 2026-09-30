// glob_test.go — the glob tool: stdlib filepath.Match semantics
// documented honestly (no `**` recursion), sorted relative-to-root
// output, bounded count with overflow marker, symlink-free walking.
package filetools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedTree(t *testing.T, root string) {
	t.Helper()
	writeFileT(t, root, "a.go", "x")
	writeFileT(t, root, "b.txt", "x")
	writeFileT(t, root, "sub/c.go", "x")
	writeFileT(t, root, "sub/deep/d.go", "x")
	writeFileT(t, root, ".hidden", "x")
}

func TestGlobTopLevelPattern(t *testing.T) {
	p, root := filePipeline(t)
	seedTree(t, root)

	// stdlib Match semantics, documented honestly: '*' does not cross
	// separators, so "*.go" sees only the root level.
	res := runTool(t, p, "glob", `{"pattern":"*.go"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	if res.Content != "a.go\n" {
		t.Fatalf("content = %q, want only a.go", res.Content)
	}

	// A single '*' plus a directory component reaches one level down.
	res = runTool(t, p, "glob", `{"pattern":"sub/*.go"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	if res.Content != "sub/c.go\n" {
		t.Fatalf("content = %q, want sub/c.go", res.Content)
	}

	// Dotfiles are not special for Match: '*' sees them.
	res = runTool(t, p, "glob", `{"pattern":".hidden"}`)
	if res.IsError || res.Content != ".hidden\n" {
		t.Fatalf("dotfile glob = %+v", res)
	}
}

func TestGlobSlashPattern(t *testing.T) {
	p, root := filePipeline(t)
	seedTree(t, root)

	res := runTool(t, p, "glob", `{"pattern":"sub/*/*.go"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	if res.Content != "sub/deep/d.go\n" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestGlobNoMatches(t *testing.T) {
	p, root := filePipeline(t)
	seedTree(t, root)
	res := runTool(t, p, "glob", `{"pattern":"*.rs"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	if res.Content != "(no matches)\n" {
		t.Fatalf("content = %q, want explicit no-matches", res.Content)
	}
}

func TestGlobMultiRootSorted(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	writeFileT(t, r1, "z.txt", "x")
	writeFileT(t, r2, "a.txt", "x")
	p := pipelineWith(t, Config{Roots: []string{r1, r2}})

	res := runTool(t, p, "glob", `{"pattern":"*.txt"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	// Each root walked; results sorted globally; paths relative to
	// their containing root.
	if res.Content != "a.txt\nz.txt\n" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestGlobBoundWithOverflowMarker(t *testing.T) {
	root := t.TempDir()
	p := pipelineWith(t, Config{Roots: []string{root}, MaxGlobResults: 3})
	for i := 0; i < 5; i++ {
		writeFileT(t, root, string(rune('a'+i))+".txt", "x")
	}
	res := runTool(t, p, "glob", `{"pattern":"*.txt"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	if !strings.Contains(res.Content, "[glob: 3 of 5 matches shown — refine the pattern]") {
		t.Fatalf("missing overflow marker: %q", res.Content)
	}
	lines := strings.Split(strings.TrimSpace(res.Content), "\n")
	if len(lines) != 4 { // 3 paths + marker
		t.Fatalf("lines = %d, want 3 paths + marker:\n%s", len(lines), res.Content)
	}
}

func TestGlobSymlinkedDirsNotDescended(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "top.txt", "x")
	writeFileT(t, root, "real/inside.txt", "x")
	outside := t.TempDir()
	writeFileT(t, outside, "outside.txt", "x")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	// One level of directory components: real/inside.txt visible,
	// linked/outside.txt NOT (the walk never follows the symlink).
	res := runTool(t, p, "glob", `{"pattern":"*/*.txt"}`)
	if res.IsError {
		t.Fatalf("glob result = %+v", res)
	}
	if res.Content != "real/inside.txt\n" {
		t.Fatalf("content = %q, want only real/inside.txt (no symlink traversal)", res.Content)
	}
	if strings.Contains(res.Content, "linked/") || strings.Contains(res.Content, "outside.txt") {
		t.Fatalf("symlinked directory was descended: %q", res.Content)
	}
}

func TestGlobErrors(t *testing.T) {
	p, _ := filePipeline(t)
	cases := []struct {
		name, args, wantIn string
	}{
		{"missing args", ``, "args.pattern is required"},
		{"empty pattern", `{"pattern":""}`, "args.pattern is required"},
		{"bad pattern", `{"pattern":"[a-"}`, "malformed pattern"},
		{"unknown field", `{"pattern":"*.go","x":1}`, "invalid args"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runTool(t, p, "glob", tc.args)
			if !res.IsError {
				t.Fatalf("glob(%s) = %+v, want isError", tc.name, res)
			}
			if !strings.Contains(res.Content, tc.wantIn) {
				t.Fatalf("error %q missing %q", res.Content, tc.wantIn)
			}
		})
	}
}
