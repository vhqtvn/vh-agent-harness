// derive_p4_test.go — P4 derived-title and usage-sum folds: the pure
// replay projections behind session/resume and session/list. Titles are
// DERIVED (no new events, no new state): the first user prompt,
// single-lined, truncated. Usage is the SUM of every llm/response
// usage envelope — logged since slice 1 (non-omitempty), so the sum is
// replay-derivable for ALL logs, pre-P4 included.
package session

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// newTitleLog opens an in-memory log for the derive folds.
func newTitleLog(t *testing.T) *Log {
	t.Helper()
	lg, err := NewLog(&bytes.Buffer{}, "sess-t", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return lg
}

func TestDeriveTitleFirstUserPrompt(t *testing.T) {
	lg := newTitleLog(t)
	if _, err := lg.Append(TypeLLMRequest, nil, LLMRequestPayload{Model: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := lg.AppendPrompt("  fix the   parser bug  "); err != nil {
		t.Fatal(err)
	}
	if _, err := lg.AppendPrompt("second prompt is not the title"); err != nil {
		t.Fatal(err)
	}
	if got := DeriveTitle(lg.Events()); got != "fix the parser bug" {
		t.Fatalf("DeriveTitle = %q, want whitespace-collapsed first prompt", got)
	}
}

func TestDeriveTitleTruncation(t *testing.T) {
	lg := newTitleLog(t)
	long := strings.Repeat("word ", 40) // 200 runes
	if _, err := lg.AppendPrompt(long); err != nil {
		t.Fatal(err)
	}
	title := DeriveTitle(lg.Events())
	if want := compactTitleRef(long, TitleMaxRunes); title != want {
		t.Fatalf("DeriveTitle truncation:\n got %q\nwant %q", title, want)
	}
	if n := len([]rune(title)); n > TitleMaxRunes+1 {
		t.Fatalf("title longer than max+ellipsis: %d runes", n)
	}
	if !strings.HasSuffix(title, "…") {
		t.Fatalf("truncated title must end with ellipsis: %q", title)
	}
}

func TestDeriveTitleNoPrompt(t *testing.T) {
	lg := newTitleLog(t)
	if _, err := lg.AppendLLMResponse("m", "assistant words are never the title", nil, Usage{}); err != nil {
		t.Fatal(err)
	}
	if got := DeriveTitle(lg.Events()); got != "" {
		t.Fatalf("DeriveTitle with no user prompt = %q, want empty", got)
	}
}

func TestDeriveTitleMultilinePromptIsSingleLine(t *testing.T) {
	lg := newTitleLog(t)
	if _, err := lg.AppendPrompt("line one\nline two\twith tabs\n\nline three"); err != nil {
		t.Fatal(err)
	}
	got := DeriveTitle(lg.Events())
	if strings.ContainsAny(got, "\n\t") {
		t.Fatalf("title must be single-line: %q", got)
	}
	if got != "line one line two with tabs line three" {
		t.Fatalf("title = %q", got)
	}
}

func TestSumUsage(t *testing.T) {
	lg := newTitleLog(t)
	if _, err := lg.AppendLLMResponse("m", "one", nil, Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}); err != nil {
		t.Fatal(err)
	}
	if _, err := lg.AppendLLMResponse("m", "two", nil, Usage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}); err != nil {
		t.Fatal(err)
	}
	// A non-response event must contribute nothing.
	if _, err := lg.AppendToolCall("c1", "echo", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	got := SumUsage(lg.Events())
	want := Usage{PromptTokens: 17, CompletionTokens: 8, TotalTokens: 25}
	if got != want {
		t.Fatalf("SumUsage = %+v, want %+v", got, want)
	}
}

func TestSumUsageEmptyIsZero(t *testing.T) {
	lg := newTitleLog(t)
	if got := SumUsage(lg.Events()); got != (Usage{}) {
		t.Fatalf("SumUsage over a header-only log = %+v, want zero", got)
	}
}

// compactTitleRef re-derives the title rule independently for the
// truncation expectation (the single production reference lives in
// derive.go).
func compactTitleRef(s string, max int) string {
	out := strings.Join(strings.Fields(s), " ")
	r := []rune(out)
	if len(r) <= max {
		return out
	}
	return string(r[:max]) + "…"
}
