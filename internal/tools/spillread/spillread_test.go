// spillread_test.go — the spill_read tool: the model-facing path back
// to spilled bytes, as WINDOWED retrieval. Retrieval goes through
// session.ReadSpillWindowUnder (the daemon-wide walk over per-session
// stores), is hash-validated fail-closed on the FULL file, and serves
// only [offset, offset+length) with the notice bytes reserved INSIDE
// the active policy cap — so a retrieval result always fits inline by
// construction and pages real bytes into context (the pre-windowing
// full-content return was a model-visible no-op: the commit-time policy
// re-spilled it to a byte-identical preview).
package spillread

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/vhqtvn/vh-agent-harness/internal/session"
	"github.com/vhqtvn/vh-agent-harness/internal/tools"
)

// execDef runs the definition (rooted at root, policy cap maxInline)
// against raw args JSON.
func execDef(t *testing.T, root string, maxInline int64, args string) (string, error) {
	t.Helper()
	def := Definition(root, maxInline)
	out, err := def.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		return "", err
	}
	if def.Name != Name || def.IsConcurrencySafe != true || def.TimeoutMs <= 0 {
		t.Fatalf("definition posture wrong: %+v", def)
	}
	return out, nil
}

// windowNoticeRE parses the in-band paging notice.
var windowNoticeRE = regexp.MustCompile(`\[window offset=(\d+) length=(\d+) of (\d+) bytes — adjust offset/length to page\]$`)

// completeNoticeRE parses the explicit terminal notice (the clean end
// at offset == size).
var completeNoticeRE = regexp.MustCompile(`\[window complete: offset=(\d+) of (\d+) bytes — full content delivered\]$`)

// parseWindowNotice extracts (offset, length, size) from a retrieval
// result's trailing notice — exactly the arithmetic a model follows to
// page (next offset = offset+length).
func parseWindowNotice(t *testing.T, out string) (offset, length, size int64) {
	t.Helper()
	m := windowNoticeRE.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("result lacks the window notice: tail %q", tail(out, 160))
	}
	offset, _ = strconv.ParseInt(m[1], 10, 64)
	length, _ = strconv.ParseInt(m[2], 10, 64)
	size, _ = strconv.ParseInt(m[3], 10, 64)
	return offset, length, size
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// pagedContent builds deterministic, non-uniform content so two
// different windows can never be byte-identical.
func pagedContent(n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "line %06d of the paged spill payload\n", i)
	}
	return b.String()[:n]
}

// TestSpillReadWindowedPagingIsNotANoop is the NO-OP KILL TEST: page 1
// (default window) and page 2 (explicit offset from page 1's notice)
// must return DIFFERENT bytes. The pre-fix tool returned the full
// content, which the commit-time SpillPolicy re-spilled — content
// addressing deduped it to the SAME locator — so every page showed a
// byte-identical preview and the model could never page spilled bytes
// into context.
func TestSpillReadWindowedPagingIsNotANoop(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-page")
	const cap = 4096
	content := pagedContent(20000)
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	argsDefault, _ := json.Marshal(map[string]any{"locator": loc})
	page1, err := execDef(t, root, cap, string(argsDefault))
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	off1, len1, size1 := parseWindowNotice(t, page1)
	if off1 != 0 || size1 != int64(len(content)) {
		t.Fatalf("page-1 notice = offset %d length %d of %d", off1, len1, size1)
	}
	// Always-inline by construction: window + notice <= cap.
	if int64(len(page1)) > cap {
		t.Fatalf("page 1 = %d bytes, must stay <= cap %d (inline by construction)", len(page1), cap)
	}
	// Page 1 serves the FIRST bytes of the content (exactly the window
	// length the notice reports).
	if !strings.HasPrefix(page1, content[:int(len1)]) {
		t.Fatalf("page 1 does not start with the content's first %d bytes: %q...", len1, page1[:80])
	}

	// Page 2: follow the notice (next offset = offset+length) — the
	// bytes MUST differ from page 1 (the no-op kill assertion).
	argsPage2, _ := json.Marshal(map[string]any{"locator": loc, "offset": off1 + len1})
	page2, err := execDef(t, root, cap, string(argsPage2))
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if page2 == page1 {
		t.Fatal("page 2 returned the SAME bytes as page 1 — retrieval is a no-op (the pre-fix re-spill bug)")
	}
	off2, _, _ := parseWindowNotice(t, page2)
	if off2 != off1+len1 {
		t.Fatalf("page-2 offset = %d, want %d", off2, off1+len1)
	}
	if int64(len(page2)) > cap {
		t.Fatalf("page 2 = %d bytes, must stay <= cap %d", len(page2), cap)
	}
	// Page 2's window is the content bytes AT the requested offset.
	notice2 := windowNoticeRE.FindString(page2)
	window2 := strings.TrimSuffix(page2, notice2)
	window2 = strings.TrimSuffix(window2, "\n")
	if !strings.HasPrefix(content[int(off2):], window2) {
		t.Fatalf("page-2 window is not the content at offset %d", off2)
	}

	// Page 3: explicit offset beyond page 2's end must keep differing —
	// every window is real, addressable content.
	argsPage3, _ := json.Marshal(map[string]any{"locator": loc, "offset": off2 + 4096})
	page3, err := execDef(t, root, cap, string(argsPage3))
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if page3 == page2 || page3 == page1 {
		t.Fatal("page 3 repeated an earlier page's bytes — paging is not advancing")
	}
}

// TestSpillReadLastPageNoticeCompletesTraversal is the DEAD-END KILL
// TEST for the LAST page: the trailing notice must report the DELIVERED
// window length (the storage layer serves min(winLen, size-offset)),
// never the requested/clamped one. With the requested length, the
// documented arithmetic next_offset = offset+length OVERSHOOTS EOF on a
// short final page and the follow-up call fails closed — a model
// following the notice could never complete a traversal. The test walks
// the WHOLE content purely by notice arithmetic (two full pages plus a
// short last page), then requires the follow-up call at exactly
// offset == size to be a clean explicit terminal notice — not a paging
// notice pointing past EOF, and not an overshoot error.
func TestSpillReadLastPageNoticeCompletesTraversal(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-last")
	const cap = 4096
	// Size chosen so the default window splits it into two full pages
	// plus a SHORT final page (10000 % ~4018 != 0 — the last window
	// delivers fewer bytes than requested).
	content := pagedContent(10000)
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	var got strings.Builder
	off := int64(0)
	for calls := 1; ; calls++ {
		args, _ := json.Marshal(map[string]any{"locator": loc, "offset": off})
		out, err := execDef(t, root, cap, string(args))
		if err != nil {
			t.Fatalf("call %d at offset %d errored — notice arithmetic dead-ended: %v", calls, off, err)
		}
		if m := completeNoticeRE.FindStringSubmatch(out); m != nil {
			// The clean terminal: offset == size, empty window, explicit
			// completion notice — never an overshoot error.
			termOff, _ := strconv.ParseInt(m[1], 10, 64)
			termSize, _ := strconv.ParseInt(m[2], 10, 64)
			if termOff != int64(len(content)) || termSize != int64(len(content)) {
				t.Fatalf("terminal notice = offset %d of %d, want %d of %d", termOff, termSize, len(content), len(content))
			}
			if windowNoticeRE.MatchString(out) {
				t.Fatalf("terminal result must not carry a paging notice: %q", tail(out, 120))
			}
			if got.String() != content {
				t.Fatal("traversal by notice arithmetic did not reassemble the full content")
			}
			if calls < 4 {
				t.Fatalf("traversal took %d calls — expected two full pages + a short last page + the terminal call", calls)
			}
			break
		}
		o, l, sz := parseWindowNotice(t, out)
		if o != off || sz != int64(len(content)) {
			t.Fatalf("call %d notice = offset %d length %d of %d, want offset %d of %d", calls, o, l, sz, off, len(content))
		}
		if l <= 0 {
			t.Fatalf("call %d: non-terminal notice must carry a positive delivered length", calls)
		}
		if o+l > sz {
			t.Fatalf("call %d notice length %d at offset %d exceeds size %d — notice reports the REQUESTED window, not the delivered bytes (last-page dead end)", calls, l, o, sz)
		}
		notice := windowNoticeRE.FindString(out)
		window := strings.TrimSuffix(strings.TrimSuffix(out, notice), "\n")
		if int64(len(window)) != l {
			t.Fatalf("call %d notice length %d != delivered window %d bytes", calls, l, len(window))
		}
		if window != content[o:o+l] {
			t.Fatalf("call %d window is not content[%d:%d]", calls, o, o+l)
		}
		got.WriteString(window)
		off = o + l
	}
}

// TestSpillReadLengthClampedToCap: an explicit length above the active
// policy cap is clamped — the result stays <= cap inline.
func TestSpillReadLengthClampedToCap(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-clamp")
	const cap = 4096
	content := pagedContent(20000)
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"locator": loc, "length": 1 << 20})
	out, err := execDef(t, root, cap, string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if int64(len(out)) > cap {
		t.Fatalf("clamped read = %d bytes, must stay <= cap %d", len(out), cap)
	}
	off, length, _ := parseWindowNotice(t, out)
	if off != 0 || length > cap {
		t.Fatalf("clamped notice = offset %d length %d, want offset 0 length <= %d", off, length, cap)
	}

	// A small explicit length is honored exactly (plus the notice).
	args, _ = json.Marshal(map[string]any{"locator": loc, "length": 500})
	out, err = execDef(t, root, cap, string(args))
	if err != nil {
		t.Fatalf("Execute small: %v", err)
	}
	off, length, _ = parseWindowNotice(t, out)
	if off != 0 || length != 500 {
		t.Fatalf("small notice = offset %d length %d, want 0/500", off, length)
	}
}

// TestSpillReadSmallContentWholeWindow: when the stored content fits a
// single window (content <= cap), the default read returns it whole
// with NO paging notice.
func TestSpillReadSmallContentWholeWindow(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-small")
	content := "small stored payload — fits one window"
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"locator": loc})
	out, err := execDef(t, root, 4096, string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != content {
		t.Fatalf("whole-window read = %q, want the exact content (no notice)", out)
	}
}

func TestSpillReadFailClosed(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-tool2")
	content := []byte(pagedContent(10000))
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Tampered store file: hash mismatch refuses (full-file validation,
	// even for a tiny healthy window).
	tampered := loc
	tampered.SHA256 = strings.Repeat("0", 64)
	args, _ := json.Marshal(map[string]any{"locator": tampered, "length": 10})
	if _, err := execDef(t, root, 4096, string(args)); err == nil {
		t.Fatal("tampered locator must fail closed")
	}

	// Unknown file: refuses.
	ghost := session.SpillLocator{File: "sp-9999999999999999", SHA256: loc.SHA256, Size: loc.Size}
	args, _ = json.Marshal(map[string]any{"locator": ghost})
	if _, err := execDef(t, root, 4096, string(args)); err == nil {
		t.Fatal("unknown spill file must fail closed")
	}

	// Locator without a hash: refuses (nothing to validate against).
	noHash := loc
	noHash.SHA256 = ""
	args, _ = json.Marshal(map[string]any{"locator": noHash})
	if _, err := execDef(t, root, 4096, string(args)); err == nil {
		t.Fatal("hashless locator must fail closed")
	}

	// Bad window args: negative offset / negative length refuse.
	args, _ = json.Marshal(map[string]any{"locator": loc, "offset": -1})
	if _, err := execDef(t, root, 4096, string(args)); err == nil {
		t.Fatal("negative offset must fail closed")
	}
	args, _ = json.Marshal(map[string]any{"locator": loc, "length": -7})
	if _, err := execDef(t, root, 4096, string(args)); err == nil {
		t.Fatal("negative length must fail closed")
	}

	// Bad args shapes.
	for _, bad := range []string{`{}`, `{"locator":{"file":"x"}}`, `{"nope":1}`, `not json`, ``} {
		if _, err := execDef(t, root, 4096, bad); err == nil {
			t.Fatalf("args %q must fail", bad)
		}
	}
}

// TestSpillReadThroughPipeline drives the definition through the real
// pipeline (guards apply — an unknown tool is refused like any other).
func TestSpillReadThroughPipeline(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-pipe")
	content := "through the pipeline"
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"locator": loc})

	p := tools.NewPipeline()
	if err := p.Register(Definition(root, 0)); err != nil {
		t.Fatalf("Register: %v", err)
	}
	res := p.Execute(context.Background(), session.ToolCall{ID: "c1", Name: Name, Args: args})
	if res.IsError || res.Content != content {
		t.Fatalf("pipeline result = %+v", res)
	}

	// Unknown tool: the pipeline's typed refusal (spill_read is not special).
	res = p.Execute(context.Background(), session.ToolCall{ID: "c2", Name: "spill_read_typo", Args: args})
	if !res.IsError {
		t.Fatalf("unknown tool must error: %+v", res)
	}
}

// TestSpillReadAlwaysInlineUnderPolicyCommit proves the construction
// guarantee end-to-end at the commit seam: a windowed retrieval result
// committed through a policy-armed log is NEVER re-spilled (the result
// is <= cap by construction), while the same-size raw content IS.
func TestSpillReadAlwaysInlineUnderPolicyCommit(t *testing.T) {
	root := t.TempDir()
	s := session.NewFileSpillStore(root, "sess-commit")
	const cap = 4096
	content := pagedContent(20000)
	loc, err := s.Write("", []byte(content))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	args, _ := json.Marshal(map[string]any{"locator": loc})
	out, err := execDef(t, root, cap, string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if int64(len(out)) > cap {
		t.Fatalf("retrieval = %d bytes, must be <= cap by construction", len(out))
	}
	// The commit-time decision on the retrieval result: inline.
	p := &session.SpillPolicy{MaxInlineBytes: cap, Store: s}
	if c, spilledLoc, spilled := p.Apply("", out); spilled || spilledLoc != nil || c != out {
		t.Fatalf("retrieval result re-spilled at commit — the no-op bug: spilled=%v", spilled)
	}
}
