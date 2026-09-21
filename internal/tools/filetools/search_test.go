// search_test.go — the search tool: stdlib regex content search with
// a `path:LN: line` result shape, optional basename glob filter,
// bounded match counts with an overflow marker, and typed malformed-
// regex errors.
package filetools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchBasicShape(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "a.txt", "the needle here\nnothing\nneedle again\n")
	writeFileT(t, root, "sub/b.txt", "clean\n")

	res := runTool(t, p, "search", `{"pattern":"needle"}`)
	if res.IsError {
		t.Fatalf("search result = %+v", res)
	}
	want := "a.txt:1: the needle here\na.txt:3: needle again\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q", res.Content, want)
	}
}

func TestSearchRegexSemantics(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "log.txt", "ERR bad\nok\nERROR worse\n")

	res := runTool(t, p, "search", `{"pattern":"ERR(OR)?"}`)
	if res.IsError {
		t.Fatalf("search result = %+v", res)
	}
	want := "log.txt:1: ERR bad\nlog.txt:3: ERROR worse\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q", res.Content, want)
	}
}

func TestSearchGlobFilter(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "a.go", "target\n")
	writeFileT(t, root, "b.txt", "target\n")
	writeFileT(t, root, "sub/c.go", "target\n")

	// The filter matches BASE NAMES with stdlib Match semantics, so it
	// applies at any depth (documented: it filters names, not paths).
	res := runTool(t, p, "search", `{"pattern":"target","glob":"*.go"}`)
	if res.IsError {
		t.Fatalf("search result = %+v", res)
	}
	if !strings.Contains(res.Content, "a.go:1: target") || !strings.Contains(res.Content, "sub/c.go:1: target") {
		t.Fatalf("filter dropped depth-matching files: %q", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Fatalf("filter not applied: %q", res.Content)
	}
}

func TestSearchNoMatches(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "a.txt", "quiet\n")
	res := runTool(t, p, "search", `{"pattern":"loud"}`)
	if res.IsError {
		t.Fatalf("search result = %+v", res)
	}
	if res.Content != "(no matches)\n" {
		t.Fatalf("content = %q", res.Content)
	}
}

func TestSearchBoundWithOverflowMarker(t *testing.T) {
	root := t.TempDir()
	p := pipelineWith(t, Config{Roots: []string{root}, MaxSearchMatches: 2})
	var sb strings.Builder
	for i := 1; i <= 5; i++ {
		sb.WriteString("hit\n")
	}
	writeFileT(t, root, "five.txt", sb.String())

	res := runTool(t, p, "search", `{"pattern":"hit"}`)
	if res.IsError {
		t.Fatalf("search result = %+v", res)
	}
	if !strings.Contains(res.Content, "[search: 2 of 5 matches shown — refine the pattern]") {
		t.Fatalf("missing overflow marker: %q", res.Content)
	}
	if strings.Count(res.Content, ": hit") != 2 {
		t.Fatalf("match lines = %d, want the bound 2:\n%s", strings.Count(res.Content, ": hit"), res.Content)
	}
}

func TestSearchMultiRootAndSymlinks(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	writeFileT(t, r1, "one.txt", "findme\n")
	writeFileT(t, r2, "two.txt", "findme\n")
	// A symlinked dir inside r1 pointing outside: never searched.
	outside := t.TempDir()
	writeFileT(t, outside, "leak.txt", "findme\n")
	if err := os.Symlink(outside, filepath.Join(r1, "linked")); err != nil {
		t.Fatal(err)
	}
	p := pipelineWith(t, Config{Roots: []string{r1, r2}})

	res := runTool(t, p, "search", `{"pattern":"findme"}`)
	if res.IsError {
		t.Fatalf("search result = %+v", res)
	}
	want := "one.txt:1: findme\ntwo.txt:1: findme\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q (no symlink leak)", res.Content, want)
	}
}

func TestSearchErrors(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "a.txt", "x")
	cases := []struct {
		name, args, wantIn string
	}{
		{"missing args", ``, "args.pattern is required"},
		{"empty pattern", `{"pattern":""}`, "args.pattern is required"},
		{"malformed regex", `{"pattern":"[a-"}`, "malformed regex"},
		{"malformed filter", `{"pattern":"x","glob":"["}`, "malformed glob filter"},
		{"unknown field", `{"pattern":"x","zz":1}`, "invalid args"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runTool(t, p, "search", tc.args)
			if !res.IsError {
				t.Fatalf("search(%s) = %+v, want isError", tc.name, res)
			}
			if !strings.Contains(res.Content, tc.wantIn) {
				t.Fatalf("error %q missing %q", res.Content, tc.wantIn)
			}
		})
	}
}
