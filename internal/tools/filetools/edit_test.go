// edit_test.go — the edit tool: EXACT old→new string replacement
// with typed no-match / non-unique outcomes (never panics), an atomic
// write-back, and a unified-diff-style result snippet.
package filetools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditSingleReplacement(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "code.go", "package main\n\nfunc old() {}\n")

	res := runTool(t, p, "edit", `{"path":"code.go","old":"func old() {}","new":"func new() {}"}`)
	if res.IsError {
		t.Fatalf("edit result = %+v", res)
	}
	if !strings.Contains(res.Content, "1 occurrence") {
		t.Fatalf("result must state occurrences: %q", res.Content)
	}
	// Diff snippet: a @@ hunk header, the removed and added lines, and
	// one line of context on each side (here: the blank line 2).
	for _, want := range []string{"@@", "\n-func old() {}\n", "\n+func new() {}\n", "\n \n"} {
		if !strings.Contains(res.Content, want) {
			t.Fatalf("diff missing %q:\n%s", want, res.Content)
		}
	}
	got, _ := os.ReadFile(filepath.Join(root, "code.go"))
	if string(got) != "package main\n\nfunc new() {}\n" {
		t.Fatalf("file = %q", got)
	}
	// No temp residue.
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("dir entries = %+v, want exactly the target", entries)
	}
}

func TestEditReplaceAll(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "todo.txt", "a TODO here\nTODO there\nno todo")

	res := runTool(t, p, "edit", `{"path":"todo.txt","old":"TODO","new":"DONE","replaceAll":true}`)
	if res.IsError {
		t.Fatalf("edit result = %+v", res)
	}
	if !strings.Contains(res.Content, "2 occurrences") {
		t.Fatalf("result must state occurrences: %q", res.Content)
	}
	got, _ := os.ReadFile(filepath.Join(root, "todo.txt"))
	if string(got) != "a DONE here\nDONE there\nno todo" {
		t.Fatalf("file = %q", got)
	}
}

func TestEditMultiLineBlocks(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "m.txt", "keep\nold line 1\nold line 2\nkeep2\n")

	res := runTool(t, p, "edit", `{"path":"m.txt","old":"old line 1\nold line 2","new":"fresh 1\nfresh 2"}`)
	if res.IsError {
		t.Fatalf("edit result = %+v", res)
	}
	got, _ := os.ReadFile(filepath.Join(root, "m.txt"))
	if string(got) != "keep\nfresh 1\nfresh 2\nkeep2\n" {
		t.Fatalf("file = %q", got)
	}
	if !strings.Contains(res.Content, "-old line 1") || !strings.Contains(res.Content, "+fresh 1") {
		t.Fatalf("multi-line diff missing block lines:\n%s", res.Content)
	}
}

func TestEditTypedErrors(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "haystack.txt", "needle once\n")
	writeFileT(t, root, "multi.txt", "x x x\n")

	cases := []struct {
		name, args, wantIn string
	}{
		{"missing args", ``, "args.path is required"},
		{"empty old", `{"path":"haystack.txt","old":"","new":"y"}`, "old string is required"},
		{"no match", `{"path":"haystack.txt","old":"absent","new":"y"}`, "not found"},
		{"non-unique", `{"path":"multi.txt","old":"x","new":"y"}`, "3 matches"},
		{"missing file", `{"path":"nope.txt","old":"a","new":"b"}`, "nope.txt"},
		{"escape", `{"path":"../m.txt","old":"a","new":"b"}`, "[escape]"},
		{"unknown field", `{"path":"haystack.txt","old":"a","new":"b","zzz":1}`, "invalid args"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runTool(t, p, "edit", tc.args)
			if !res.IsError {
				t.Fatalf("edit(%s) = %+v, want isError", tc.name, res)
			}
			if !strings.Contains(res.Content, tc.wantIn) {
				t.Fatalf("error %q missing %q", res.Content, tc.wantIn)
			}
		})
	}
}

// TestEditNonUniqueSuggestsReplaceAll: the typed error names the
// escape hatch.
func TestEditNonUniqueSuggestsReplaceAll(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "multi.txt", "x x x\n")
	res := runTool(t, p, "edit", `{"path":"multi.txt","old":"x","new":"y"}`)
	if !res.IsError || !strings.Contains(res.Content, "replaceAll") {
		t.Fatalf("error = %q, want replaceAll hint", res.Content)
	}
	// File untouched by the failed edit.
	got, _ := os.ReadFile(filepath.Join(root, "multi.txt"))
	if string(got) != "x x x\n" {
		t.Fatalf("failed edit modified the file: %q", got)
	}
}

// TestEditOldEqualsNew: replacing with identical text still counts as
// an edit (idempotent; the diff shows no change).
func TestEditOldEqualsNew(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "same.txt", "stable\n")
	res := runTool(t, p, "edit", `{"path":"same.txt","old":"stable","new":"stable"}`)
	if res.IsError {
		t.Fatalf("same-text edit = %+v", res)
	}
	if !strings.Contains(res.Content, "1 occurrence") {
		t.Fatalf("result = %q", res.Content)
	}
}

func TestEditOverSymlinkRejected(t *testing.T) {
	p, root := filePipeline(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "t.txt")
	if err := os.WriteFile(target, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "s.txt")); err != nil {
		t.Fatal(err)
	}
	res := runTool(t, p, "edit", `{"path":"s.txt","old":"v","new":"w"}`)
	if !res.IsError || !strings.Contains(res.Content, "[symlink-final]") {
		t.Fatalf("symlink edit = %+v, want typed rejection", res)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "v" {
		t.Fatalf("symlink target modified: %q", got)
	}
}

// TestEditReplaceAllShrinkRegression: replaceAll with a SHRINKING
// replacement once drove diffSnippet's after-span computation negative
// (total delta from EVERY occurrence dragged into the first hunk's
// end) — an index panic AFTER atomicWriteFile had already committed
// the edit, so the tool reported isError "tool panicked" for a
// mutation that HAD succeeded, and a retry failed "old string not
// found". The snippet must render the FIRST change honestly and the
// renderer must be total: it runs post-commit and may never turn a
// landed edit into an error result.
func TestEditReplaceAllShrinkRegression(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "todo.txt", "keep1\nTODO a\nmid\nTODO b\nend TODO\n")

	res := runTool(t, p, "edit", `{"path":"todo.txt","old":"TODO","new":"","replaceAll":true}`)
	if res.IsError {
		t.Fatalf("shrinking replaceAll = isError (content %q) — the edit committed but the result lied", res.Content)
	}
	if !strings.Contains(res.Content, "3 occurrences") {
		t.Fatalf("result must state occurrences: %q", res.Content)
	}
	got, _ := os.ReadFile(filepath.Join(root, "todo.txt"))
	if string(got) != "keep1\n a\nmid\n b\nend \n" {
		t.Fatalf("file = %q", got)
	}
	// The hunk documents the FIRST change only (±1 context line);
	// later occurrences are not smeared into the first hunk's span.
	want := "@@ -1,3 +1,3 @@\n keep1\n-TODO a\n+ a\n mid\n"
	if !strings.HasSuffix(res.Content, want) {
		t.Fatalf("snippet = %q, want suffix %q", res.Content, want)
	}
}

// TestEditReplaceAllShrinkBoundaries: deletion-by-replaceAll at the
// very start and very end of the file — the after-span stays in
// bounds when the replacement region touches byte 0 or EOF.
func TestEditReplaceAllShrinkBoundaries(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "start.txt", "XX one\nXX two\nplain\n")
	writeFileT(t, root, "end.txt", "plain\nfin ZEN\n")

	for _, tc := range []struct{ path, old, want string }{
		{"start.txt", "XX", " one\n two\nplain\n"},
		{"end.txt", "ZEN", "plain\nfin \n"},
	} {
		res := runTool(t, p, "edit", fmt.Sprintf(`{"path":%q,"old":%q,"new":"","replaceAll":true}`, tc.path, tc.old))
		if res.IsError {
			t.Fatalf("%s shrink-at-boundary = isError (content %q)", tc.path, res.Content)
		}
		if !strings.Contains(res.Content, "@@ -") {
			t.Fatalf("%s snippet missing hunk header: %q", tc.path, res.Content)
		}
		got, _ := os.ReadFile(filepath.Join(root, tc.path))
		if string(got) != tc.want {
			t.Fatalf("%s = %q, want %q", tc.path, got, tc.want)
		}
	}
}
