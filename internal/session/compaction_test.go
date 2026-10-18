package session

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

// buildCompactableLog appends a 5-message surface:
//
//	seq2 user "one one one" · seq3 user "two two two" · seq4 assistant "answer A"
//	seq5 user "three three" · seq6 assistant "answer B"
func buildCompactableLog(t *testing.T) *Log {
	t.Helper()
	lg := newLogOrFail(t)
	steps := []struct {
		kind, text string
	}{
		{"prompt", "one one one"},
		{"prompt", "two two two"},
		{"assistant", "answer A"},
		{"prompt", "three three"},
		{"assistant", "answer B"},
	}
	for _, st := range steps {
		switch st.kind {
		case "prompt":
			if _, err := lg.AppendPrompt(st.text); err != nil {
				t.Fatalf("AppendPrompt: %v", err)
			}
		case "assistant":
			if _, err := lg.AppendLLMResponse("mock-1", st.text, nil, Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}); err != nil {
				t.Fatalf("AppendLLMResponse: %v", err)
			}
		}
	}
	return lg
}

// concatSummarizer is a deterministic in-process summarizer (NO real LLM):
// it condenses each shadowed message to its first 3 chars behind a marker,
// so its output is genuinely smaller than the shadowed span.
func concatSummarizer(shadowed []Message) (string, error) {
	out := "[summary:"
	for i, m := range shadowed {
		if i > 0 {
			out += "|"
		}
		c := m.Content
		if len(c) > 3 {
			c = c[:3]
		}
		out += c
	}
	return out + "]", nil
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestCompactReplacesRegionWithCitations(t *testing.T) {
	lg := buildCompactableLog(t)

	var seen [][]Message
	sum := func(sh []Message) (string, error) {
		cp := make([]Message, len(sh))
		copy(cp, sh)
		seen = append(seen, cp)
		return concatSummarizer(sh)
	}

	res, err := lg.Compact(sum, CompactOptions{RetainTailMessages: 2})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Region: shadow [0,3) = one, two, answerA; retain three, answerB.
	if res.ShadowedRange != [2]int{0, 3} {
		t.Fatalf("ShadowedRange = %v, want [0 3]", res.ShadowedRange)
	}
	wantSeqs := []int64{2, 3, 4}
	if len(res.SourceEventSeqs) != len(wantSeqs) {
		t.Fatalf("SourceEventSeqs = %v, want %v", res.SourceEventSeqs, wantSeqs)
	}
	for i, s := range wantSeqs {
		if res.SourceEventSeqs[i] != s {
			t.Fatalf("SourceEventSeqs = %v, want %v", res.SourceEventSeqs, wantSeqs)
		}
	}
	if res.ReplaceGeneration != 1 {
		t.Fatalf("ReplaceGeneration = %d, want 1", res.ReplaceGeneration)
	}

	// The summarizer saw exactly the shadowed messages.
	if len(seen) != 1 || len(seen[0]) != 3 || seen[0][0].Content != "one one one" || seen[0][2].Content != "answer A" {
		t.Fatalf("summarizer input = %+v, want the 3 shadowed messages", seen)
	}

	// Event vocabulary order: compaction/start -> compaction/summary -> compaction/end.
	evs := lg.Events()
	n := len(evs)
	if evs[n-3].Type != TypeCompactionStart || evs[n-2].Type != TypeCompactionSummary || evs[n-1].Type != TypeCompactionEnd {
		t.Fatalf("tail event types = %q %q %q", evs[n-3].Type, evs[n-2].Type, evs[n-1].Type)
	}
	if evs[n-2].Seq != res.SummarySeq {
		t.Fatalf("summary event seq = %d, want %d", evs[n-2].Seq, res.SummarySeq)
	}
	if evs[n-2].SurfaceOp == nil || evs[n-2].SurfaceOp.Op != SurfaceOpReplace || evs[n-2].SurfaceOp.Start != 0 || evs[n-2].SurfaceOp.End != 3 {
		t.Fatalf("summary surfaceOp = %+v, want replace [0,3)", evs[n-2].SurfaceOp)
	}
	var ep CompactionEndPayload
	if err := json.Unmarshal(evs[n-1].Payload, &ep); err != nil {
		t.Fatalf("compaction/end payload: %v", err)
	}
	if ep.SummarySeq != res.SummarySeq || ep.ReplaceGeneration != 1 {
		t.Fatalf("compaction/end payload = %+v", ep)
	}

	// Surface: [user summary, user three, assistant answerB]; start/end are log-only.
	msgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("post-compaction surface length = %d, want 3: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "user" || msgs[0].Content != "[summary:one|two|ans]" {
		t.Fatalf("summary message = %+v", msgs[0])
	}
	if msgs[1].Content != "three three" || msgs[2].Content != "answer B" {
		t.Fatalf("retained tail drifted: %+v", msgs)
	}
	gen, err := lg.ReplaceGeneration()
	if err != nil {
		t.Fatalf("ReplaceGeneration: %v", err)
	}
	if gen != 1 {
		t.Fatalf("live ReplaceGeneration = %d, want 1", gen)
	}
}

func TestCompactSummaryMustBeSmaller(t *testing.T) {
	lg := buildCompactableLog(t)
	before, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}

	// Shadowed chars = 11 + 11 + 8 = 30. A summary of >= 30 chars must be refused.
	big := make([]byte, 30)
	for i := range big {
		big[i] = 'x'
	}
	_, err = lg.Compact(func([]Message) (string, error) { return string(big), nil }, CompactOptions{RetainTailMessages: 2})
	var stl *SummaryTooLargeError
	if err == nil {
		t.Fatal("expected SummaryTooLargeError when the summary is not smaller")
	}
	if _, ok := err.(*SummaryTooLargeError); !ok {
		t.Fatalf("expected *SummaryTooLargeError, got %T: %v", err, err)
	}
	stl = err.(*SummaryTooLargeError)
	if stl.SummaryChars != 30 || stl.ShadowedChars != 30 {
		t.Fatalf("SummaryTooLargeError fields = %+v", stl)
	}

	// Equal length is also refused ("not smaller").
	_, err = lg.Compact(func([]Message) (string, error) { return "0123456789012345678901234567890", nil }, CompactOptions{RetainTailMessages: 2, Reason: "equal"})
	if _, ok := err.(*SummaryTooLargeError); !ok {
		t.Fatalf("equal-length summary must be refused, got %T: %v", err, err)
	}

	// Surface unchanged; generation unchanged; the log ends with the unmatched
	// compaction/start of the FIRST attempt — the durable lock (dsh semantic:
	// a crash or refusal between start and end leaves the lock visible).
	after, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	if !bytes.Equal(mustJSON(t, before), mustJSON(t, after)) {
		t.Fatalf("refused compaction must not change the surface:\n% s\n% s", mustJSON(t, before), mustJSON(t, after))
	}
	if gen, _ := lg.ReplaceGeneration(); gen != 0 {
		t.Fatalf("refused compaction must not bump replaceGeneration, got %d", gen)
	}
	evs := lg.Events()
	found := 0
	for _, ev := range evs {
		switch ev.Type {
		case TypeCompactionStart:
			found++
		case TypeCompactionSummary, TypeCompactionEnd:
			t.Fatalf("no summary/end events may land on refusal, got %s", ev.Type)
		}
	}
	if found == 0 {
		t.Fatal("expected at least one unmatched compaction/start lock event")
	}
}

func TestCompactRegionGuards(t *testing.T) {
	lg := buildCompactableLog(t)
	if _, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 5}); err == nil {
		t.Fatal("retaining the whole surface must be refused (nothing to shadow)")
	}
	if _, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: -1}); err == nil {
		t.Fatal("negative retain tail must be refused")
	}
	small := newLogOrFail(t)
	if _, err := small.AppendPrompt("only"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := small.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 2}); err == nil {
		t.Fatal("surface smaller than retain+1 must be refused")
	}
}

func TestReplaceGenerationMonotonicAcrossCompactions(t *testing.T) {
	lg := buildCompactableLog(t)
	if _, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 2}); err != nil {
		t.Fatalf("first Compact: %v", err)
	}
	if gen, _ := lg.ReplaceGeneration(); gen != 1 {
		t.Fatalf("generation after first compact = %d, want 1", gen)
	}
	// Regrow the surface, then compact again.
	if _, err := lg.AppendPrompt("four four"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := lg.AppendLLMResponse("mock-1", "answer D", nil, Usage{}); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}
	res, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 2})
	if err != nil {
		t.Fatalf("second Compact: %v", err)
	}
	if res.ReplaceGeneration != 2 {
		t.Fatalf("generation after second compact = %d, want 2", res.ReplaceGeneration)
	}
	if gen, _ := lg.ReplaceGeneration(); gen != 2 {
		t.Fatalf("live generation = %d, want 2", gen)
	}
}

func TestReplayDeterminismAcrossCompactionAndUnfold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compacted.jsonl")
	lg, err := OpenFile(path, "sess-compact-replay")
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer lg.Close()

	// Same 5-message surface as buildCompactableLog, file-backed.
	for _, st := range []struct {
		kind, text string
	}{
		{"prompt", "one one one"},
		{"prompt", "two two two"},
		{"assistant", "answer A"},
		{"prompt", "three three"},
		{"assistant", "answer B"},
	} {
		if st.kind == "prompt" {
			if _, err := lg.AppendPrompt(st.text); err != nil {
				t.Fatalf("AppendPrompt: %v", err)
			}
		} else if _, err := lg.AppendLLMResponse("mock-1", st.text, nil, Usage{PromptTokens: 5}); err != nil {
			t.Fatalf("AppendLLMResponse: %v", err)
		}
	}
	preCompaction, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}

	res, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 2, Reason: "pressure"})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	// Post-compaction growth must replay on top of the compacted surface.
	if _, err := lg.AppendPrompt("four four"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	liveMsgs, err := lg.Surface()
	if err != nil {
		t.Fatalf("Surface: %v", err)
	}
	liveGen, err := lg.ReplaceGeneration()
	if err != nil {
		t.Fatalf("ReplaceGeneration: %v", err)
	}
	if err := lg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	replayed, err := ReplayFile(path)
	if err != nil {
		t.Fatalf("ReplayFile: %v", err)
	}
	replayMsgs, err := DeriveMessages(replayed)
	if err != nil {
		t.Fatalf("DeriveMessages(replayed): %v", err)
	}
	if !bytes.Equal(mustJSON(t, liveMsgs), mustJSON(t, replayMsgs)) {
		t.Fatalf("replay must reproduce the post-compaction surface exactly:\nlive:   %s\nreplay: %s", mustJSON(t, liveMsgs), mustJSON(t, replayMsgs))
	}
	fold, err := FoldSurface(replayed)
	if err != nil {
		t.Fatalf("FoldSurface(replayed): %v", err)
	}
	if fold.ReplaceGeneration != liveGen || liveGen != 1 {
		t.Fatalf("replay must reproduce replaceGeneration: replay=%d live=%d", fold.ReplaceGeneration, liveGen)
	}

	// Unfold: the cited source events re-derive the pre-compaction surface.
	unfolded, err := Unfold(replayed, res.SummarySeq)
	if err != nil {
		t.Fatalf("Unfold: %v", err)
	}
	if !bytes.Equal(mustJSON(t, preCompaction), mustJSON(t, unfolded)) {
		t.Fatalf("unfold must reproduce the pre-compaction surface:\npre:      %s\nunfolded: %s", mustJSON(t, preCompaction), mustJSON(t, unfolded))
	}
}

func TestUnfoldRejectsBadCitations(t *testing.T) {
	lg := buildCompactableLog(t)
	res, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 2})
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	events := lg.Events()

	if _, err := Unfold(events, 4242); err == nil {
		t.Fatal("Unfold must refuse an unknown summary seq")
	}

	// Tamper with the summary payload's sourceEventSeqs: citations no longer
	// match the fold's origin seqs, so unfold must refuse.
	idx := -1
	for i, ev := range events {
		if ev.Seq == res.SummarySeq {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("summary event %d not found", res.SummarySeq)
	}
	var sp CompactionSummaryPayload
	if err := json.Unmarshal(events[idx].Payload, &sp); err != nil {
		t.Fatalf("summary payload: %v", err)
	}
	sp.SourceEventSeqs = []int64{2, 3, 999}
	bad, err := json.Marshal(sp)
	if err != nil {
		t.Fatalf("marshal tampered payload: %v", err)
	}
	tampered := make([]Event, len(events))
	copy(tampered, events)
	tampered[idx].Payload = bad
	if _, err := Unfold(tampered, res.SummarySeq); err == nil {
		t.Fatal("Unfold must refuse citations that do not match the shadowed events")
	}
}

func TestSurfacePressureAndTrigger(t *testing.T) {
	cfg := SessionConfig{ContextBudgetTokens: 1000}

	// Heuristic only: 3600 chars / 4 = 900 tokens -> 0.9 pressure.
	lg := newLogOrFail(t)
	big := make([]byte, 3600)
	for i := range big {
		big[i] = 'a'
	}
	if _, err := lg.AppendPrompt(string(big)); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	p, err := SurfacePressure(lg.Events(), cfg)
	if err != nil {
		t.Fatalf("SurfacePressure: %v", err)
	}
	if p < 0.89 || p > 0.91 {
		t.Fatalf("heuristic pressure = %f, want ~0.9", p)
	}
	ok, _, err := ShouldCompact(lg.Events(), cfg)
	if err != nil {
		t.Fatalf("ShouldCompact: %v", err)
	}
	if !ok {
		t.Fatal("pressure 0.9 must exceed the default 0.8 threshold")
	}

	// Usage anchor: tiny heuristic, but the provider saw 900 prompt tokens.
	anchored := newLogOrFail(t)
	if _, err := anchored.AppendPrompt("tiny"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	if _, err := anchored.AppendLLMResponse("mock-1", "ok", nil, Usage{PromptTokens: 900, CompletionTokens: 1, TotalTokens: 901}); err != nil {
		t.Fatalf("AppendLLMResponse: %v", err)
	}
	p, err = SurfacePressure(anchored.Events(), cfg)
	if err != nil {
		t.Fatalf("SurfacePressure: %v", err)
	}
	if p < 0.89 || p > 0.91 {
		t.Fatalf("anchored pressure = %f, want ~0.9 (usage wins over the tiny heuristic)", p)
	}

	// Quiet surface: no trigger.
	quiet := newLogOrFail(t)
	if _, err := quiet.AppendPrompt("hi"); err != nil {
		t.Fatalf("AppendPrompt: %v", err)
	}
	ok, p, err = ShouldCompact(quiet.Events(), cfg)
	if err != nil {
		t.Fatalf("ShouldCompact: %v", err)
	}
	if ok || p >= 0.8 {
		t.Fatalf("quiet surface must stay below threshold: ok=%v p=%f", ok, p)
	}

	// Fail-loud config hygiene (dsh C8): a missing budget is a config error.
	if _, err := SurfacePressure(quiet.Events(), SessionConfig{}); err == nil {
		t.Fatal("SurfacePressure must refuse a zero token budget")
	}
	if _, _, err := ShouldCompact(quiet.Events(), SessionConfig{ContextBudgetTokens: -1}); err == nil {
		t.Fatal("ShouldCompact must refuse a negative token budget")
	}
}

func TestCompactionStartPayloadCarriesRange(t *testing.T) {
	lg := buildCompactableLog(t)
	if _, err := lg.Compact(concatSummarizer, CompactOptions{RetainTailMessages: 2, Reason: "test"}); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	evs := lg.Events()
	var sp CompactionStartPayload
	if err := json.Unmarshal(evs[len(evs)-3].Payload, &sp); err != nil {
		t.Fatalf("compaction/start payload: %v", err)
	}
	if sp.Reason != "test" || sp.ShadowedRange != [2]int{0, 3} {
		t.Fatalf("compaction/start payload = %+v", sp)
	}
}
