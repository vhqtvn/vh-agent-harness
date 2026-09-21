// read_test.go — the read tool: numbered-lines rendering, offset/
// limit windows, the byte cap + truncation marker, and confinement
// rejections surfacing as typed isError results through the real
// pipeline.
package filetools

import (
	"fmt"
	"strings"
	"testing"
)

func TestReadNumberedLines(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "note.md", "alpha\nbeta\ngamma")

	res := runTool(t, p, "read", `{"path":"note.md"}`)
	if res.IsError {
		t.Fatalf("read result = %+v", res)
	}
	want := "1: alpha\n2: beta\n3: gamma\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q", res.Content, want)
	}
}

func TestReadNoTrailingNewline(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "flat.txt", "one\ntwo")

	res := runTool(t, p, "read", `{"path":"flat.txt"}`)
	if res.IsError {
		t.Fatalf("read result = %+v", res)
	}
	want := "1: one\n2: two\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q (last line without newline still renders)", res.Content, want)
	}
}

func TestReadOffsetLimit(t *testing.T) {
	p, root := filePipeline(t)
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		sb.WriteString(strings.Repeat("x", i))
		sb.WriteByte('\n')
	}
	writeFileT(t, root, "growing.txt", sb.String())

	// offset skips the first N lines (0-based cursor), limit caps the
	// count; with more lines pending the truncation marker names the
	// resume cursor.
	res := runTool(t, p, "read", `{"path":"growing.txt","offset":2,"limit":3}`)
	if res.IsError {
		t.Fatalf("read result = %+v", res)
	}
	want := "3: xxx\n4: xxxx\n5: xxxxx\n[read: truncated — resume with offset=5]\n"
	if res.Content != want {
		t.Fatalf("content = %q, want %q", res.Content, want)
	}

	// A limit that lands exactly at EOF truncates nothing.
	res = runTool(t, p, "read", `{"path":"growing.txt","offset":2,"limit":8}`)
	if res.IsError {
		t.Fatalf("exact-limit read = %+v", res)
	}
	if strings.Contains(res.Content, "[read:") {
		t.Fatalf("limit landing at EOF must not claim truncation: %q", res.Content)
	}

	// offset past EOF: empty window, not an error.
	res = runTool(t, p, "read", `{"path":"growing.txt","offset":99}`)
	if res.IsError || res.Content != "" {
		t.Fatalf("offset-past-EOF = %+v, want empty success", res)
	}
}

func TestReadByteCapTruncationMarker(t *testing.T) {
	root := t.TempDir()
	p := pipelineWith(t, Config{Roots: []string{root}, MaxReadBytes: 40})
	// 8 lines whose rendered form ("N: lineX\n", 8-9 bytes each)
	// overflows a 40-byte cap mid-file.
	var sb strings.Builder
	for i := 0; i < 8; i++ {
		sb.WriteString("line" + string(rune('A'+i)) + "\n")
	}
	writeFileT(t, root, "big.txt", sb.String())

	res := runTool(t, p, "read", `{"path":"big.txt"}`)
	if res.IsError {
		t.Fatalf("read result = %+v", res)
	}
	if !strings.Contains(res.Content, "[read: truncated") {
		t.Fatalf("missing truncation marker: %q", res.Content)
	}
	// The rendered body before the marker fits the cap.
	body := strings.SplitN(res.Content, "\n[read:", 2)[0]
	if len(body) == 0 || len(body) > 40 {
		t.Fatalf("body %d bytes outside the 40-byte cap:\n%s", len(body), res.Content)
	}
	// The resume cursor works: continuing from the named offset yields
	// the remaining lines without re-reading line 1.
	offset := markerOffset(t, res.Content)
	resume := runTool(t, p, "read", fmt.Sprintf(`{"path":"big.txt","offset":%d}`, offset))
	if resume.IsError {
		t.Fatalf("resume read = %+v", resume)
	}
	covered := body + resume.Content
	if strings.Count(covered, "1: ") != 1 || strings.Count(covered, "lineH") != 1 {
		t.Fatalf("paged reads do not cover exactly once:\nfirst: %q\nresume: %q", res.Content, resume.Content)
	}
	for i := 0; i < 8; i++ {
		if !strings.Contains(covered, "line"+string(rune('A'+i))) {
			t.Fatalf("line %d missing across the paged reads:\n%s", i, covered)
		}
	}
}

func TestReadErrors(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "exists.txt", "x")

	cases := []struct {
		name, args, wantIn string
	}{
		{"missing args", ``, "args.path is required"},
		{"empty path", `{"path":""}`, "args.path is required"},
		{"missing file", `{"path":"nope.txt"}`, "nope.txt"},
		{"escape", `{"path":"../outside.txt"}`, "[escape]"},
		{"absolute outside", `{"path":"/etc/shadow"}`, "[outside-roots]"},
		{"negative offset", `{"path":"exists.txt","offset":-1}`, "offset must be"},
		{"negative limit", `{"path":"exists.txt","limit":-1}`, "limit must be"},
		{"unknown field", `{"path":"exists.txt","bogus":1}`, "invalid args"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runTool(t, p, "read", tc.args)
			if !res.IsError {
				t.Fatalf("read(%s) = %+v, want isError", tc.name, res)
			}
			if !strings.Contains(res.Content, tc.wantIn) {
				t.Fatalf("error %q missing %q", res.Content, tc.wantIn)
			}
		})
	}
}

func TestReadNestedAndDirectory(t *testing.T) {
	p, root := filePipeline(t)
	writeFileT(t, root, "sub/deep/f.txt", "deep content\n")
	res := runTool(t, p, "read", `{"path":"sub/deep/f.txt"}`)
	if res.IsError {
		t.Fatalf("nested read = %+v", res)
	}
	if res.Content != "1: deep content\n" {
		t.Fatalf("content = %q", res.Content)
	}
	// Reading a directory NAME under the root errors as a directory.
	res = runTool(t, p, "read", `{"path":"sub"}`)
	if !res.IsError || !strings.Contains(res.Content, "is a directory") {
		t.Fatalf("dir read = %+v, want is-a-directory error", res)
	}
}
